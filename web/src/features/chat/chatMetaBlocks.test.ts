import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  formatLatency,
  formatMetaParts,
  formatTokens,
  groupToolBlocks,
  parseTaskEvent,
  parseToolEvent,
  resolveThinkingAndAnswer,
  shouldShowMetaRow,
  splitThinkingContent,
  truncateResult,
  upsertTaskBlock,
  upsertToolBlock,
  type ToolCallBlock,
} from './chatMetaBlocks'

describe('MCP tool telemetry parse (VAL-MCP-008)', () => {
  it('parses namespaced MCP tool events like built-ins', () => {
    const start = parseToolEvent('ToolStart', {
      name: 'mcp.echo.echo',
      detail: 'message=telemetry',
    })
    assert.equal(start.name, 'mcp.echo.echo')
    assert.equal(start.status, 'running')

    const end = parseToolEvent('ToolEnd', {
      name: 'mcp.echo.echo',
      detail: 'message=telemetry → echo:telemetry',
    })
    assert.equal(end.name, 'mcp.echo.echo')
    assert.equal(end.status, 'done')
    assert.match(end.result || '', /echo:telemetry/)

    const err = parseToolEvent('ToolError', {
      name: 'mcp.echo.echo',
      detail: '→ unknown tool "mcp.echo.echo"',
    })
    assert.equal(err.name, 'mcp.echo.echo')
    assert.equal(err.status, 'error')

    let list: ToolCallBlock[] = []
    list = upsertToolBlock(list, start)
    list = upsertToolBlock(list, end)
    assert.equal(list.length, 1)
    assert.equal(list[0].status, 'done')
    assert.equal(list[0].name, 'mcp.echo.echo')
  })
})

describe('assistant meta row (VAL-CHAT-012)', () => {
  it('formats tokens total and prompt+completion with thousand dots', () => {
    assert.equal(formatTokens({ tokens: 420 }), '420 tokens')
    assert.equal(formatTokens({ tokens: 108091 }), '108.091 tokens')
    assert.equal(formatTokens({ tokensPrompt: 100, tokensCompletion: 50 }), '150 tokens (100+50)')
    assert.equal(
      formatTokens({ tokensPrompt: 100000, tokensCompletion: 8091 }),
      '108.091 tokens (100.000+8.091)',
    )
    assert.equal(formatTokens({}), '')
    assert.equal(formatTokens(null), '')
  })

  it('formats latency ms, seconds, and minutes', () => {
    assert.equal(formatLatency({ latencyMs: 87 }), '87 ms')
    assert.equal(formatLatency({ latencyMs: 1500 }), '1.5 s')
    assert.equal(formatLatency({ latencyMs: 12500 }), '13 s')
    assert.equal(formatLatency({ latencyMs: 135000 }), '2m 15s')
    assert.equal(formatLatency({ latencyMs: 60000 }), '1m')
    assert.equal(formatLatency({}), '')
  })

  it('builds meta parts with mode, profile:model, tokens, latency (done omitted)', () => {
    const parts = formatMetaParts({
      mode: 'Agent',
      profile: 'tempai',
      model: 'grok-4.5',
      streamStatus: 'done',
      tokens: 200,
      latencyMs: 320,
    })
    assert.deepEqual(parts, ['Agent', 'tempai: grok-4.5', '200 tokens', '320 ms'])
  })

  it('keeps stopped/error status visible in footer', () => {
    const parts = formatMetaParts({
      mode: 'Agent',
      streamStatus: 'stopped',
      tokens: 10,
    })
    assert.ok(parts.includes('stopped'))
    assert.ok(parts.includes('Agent'))
  })

  it('omits empty meta; shows when mode/tokens/latency present', () => {
    assert.equal(shouldShowMetaRow({}), false)
    assert.equal(shouldShowMetaRow({ tokens: 10 }), true)
    assert.equal(shouldShowMetaRow({ latencyMs: 1 }), true)
    assert.equal(shouldShowMetaRow({ mode: 'Plan' }), true)
    assert.equal(shouldShowMetaRow({ streamStatus: 'generating' }), true)
    assert.equal(shouldShowMetaRow({ streamStatus: 'idle' }), false)
  })
})

describe('collapsible thinking split (VAL-CHAT-013)', () => {
  it('splits <thinking> tags from final answer', () => {
    const { thinking, answer } = splitThinkingContent(
      '<thinking>\nplan steps\n</thinking>\n\n## Answer\nDone.',
    )
    assert.equal(thinking, 'plan steps')
    assert.match(answer, /## Answer/)
    assert.doesNotMatch(answer, /plan steps/)
  })

  it('splits <think> tags', () => {
    const { thinking, answer } = splitThinkingContent('<think>r1</think>final')
    assert.equal(thinking, 'r1')
    assert.equal(answer, 'final')
  })

  it('splits fenced thinking/reasoning blocks', () => {
    const { thinking, answer } = splitThinkingContent(
      '```thinking\nwhy\n```\n\nReal answer',
    )
    assert.equal(thinking, 'why')
    assert.equal(answer, 'Real answer')
  })

  it('uses explicit thinking field when provided', () => {
    const r = resolveThinkingAndAnswer('visible answer', 'hidden reason')
    assert.equal(r.thinking, 'hidden reason')
    assert.equal(r.answer, 'visible answer')
  })

  it('returns empty thinking when none present', () => {
    const r = splitThinkingContent('just prose')
    assert.equal(r.thinking, '')
    assert.equal(r.answer, 'just prose')
  })

  it('keeps thinking separate so UI can collapse without losing answer', () => {
    const raw =
      '<thinking>\nI will use write_file next\n</thinking>\n\nFile created successfully.'
    const { thinking, answer } = splitThinkingContent(raw)
    assert.equal(thinking.includes('write_file'), true)
    assert.equal(answer.includes('File created'), true)
    assert.equal(answer.includes('<thinking>'), false)
    assert.notEqual(thinking, answer)
  })

  it('streams unclosed <thinking> as live reasoning (no answer yet)', () => {
    const { thinking, answer } = splitThinkingContent(
      '<thinking>\nI am planning step 1…\nstep 2',
    )
    assert.match(thinking, /planning step 1/)
    assert.match(thinking, /step 2/)
    assert.equal(answer, '')
  })

  it('streams unclosed ```thinking fence as live reasoning', () => {
    const { thinking, answer } = splitThinkingContent(
      '```thinking\nwhy this approach',
    )
    assert.equal(thinking, 'why this approach')
    assert.equal(answer, '')
  })

  it('after close tag, further stream text becomes answer', () => {
    const { thinking, answer } = splitThinkingContent(
      '<thinking>plan</thinking>\n\nHere is the fix',
    )
    assert.equal(thinking, 'plan')
    assert.equal(answer, 'Here is the fix')
  })
})

describe('tool call blocks (VAL-CHAT-014)', () => {
  it('parses ToolStart name + detail', () => {
    const t = parseToolEvent('ToolStart', { detail: 'write_file path=hello.txt' })
    assert.equal(t.name, 'write_file')
    assert.equal(t.status, 'running')
    assert.match(t.detail || '', /hello/)
  })

  it('parses ToolEnd with arrow result and Name field', () => {
    const t = parseToolEvent('ToolEnd', {
      name: 'shell',
      detail: 'echo tool-ok → exit 0\ntool-ok',
    })
    assert.equal(t.name, 'shell')
    assert.equal(t.status, 'done')
    assert.match(t.result || '', /tool-ok/)
  })

  it('parses ToolError status', () => {
    const t = parseToolEvent('ToolError', {
      name: 'write_file',
      detail: 'path=../x',
      error: 'sandbox denial',
    })
    assert.equal(t.status, 'error')
  })

  it('upserts running tool then marks done', () => {
    let list = upsertToolBlock([], parseToolEvent('ToolStart', { detail: 'shell cmd' }))
    assert.equal(list.length, 1)
    assert.equal(list[0].status, 'running')
    list = upsertToolBlock(
      list,
      parseToolEvent('ToolEnd', { name: 'shell', detail: 'cmd → ok' }),
    )
    assert.equal(list.length, 1)
    assert.equal(list[0].status, 'done')
    assert.equal(list[0].result, 'ok')
  })

  it('truncates long results for display', () => {
    assert.equal(truncateResult('short'), 'short')
    assert.equal(truncateResult('x'.repeat(400), 20).endsWith('…'), true)
  })

  it('groups consecutive same-name tools into one accordion row', () => {
    const tools: ToolCallBlock[] = [
      { id: '1', name: 'list_dir', status: 'done', detail: 'src' },
      { id: '2', name: 'list_dir', status: 'done', detail: 'web' },
      { id: '3', name: 'list_dir', status: 'done', detail: 'docs' },
      { id: '4', name: 'read_file', status: 'done', detail: 'a.go' },
      { id: '5', name: 'read_file', status: 'done', detail: 'b.go' },
      { id: '6', name: 'shell', status: 'done', detail: 'go test' },
    ]
    const g = groupToolBlocks(tools)
    assert.equal(g.length, 3)
    assert.equal(g[0].name, 'list_dir')
    assert.equal(g[0].tools.length, 3)
    assert.equal(g[1].name, 'read_file')
    assert.equal(g[1].tools.length, 2)
    assert.equal(g[2].name, 'shell')
    assert.equal(g[2].tools.length, 1)
  })

  it('does not merge non-consecutive same names', () => {
    const tools: ToolCallBlock[] = [
      { id: '1', name: 'list_dir', status: 'done' },
      { id: '2', name: 'read_file', status: 'done' },
      { id: '3', name: 'list_dir', status: 'done' },
    ]
    const g = groupToolBlocks(tools)
    assert.equal(g.length, 3)
  })

  it('group status prefers running then error', () => {
    const g = groupToolBlocks([
      { id: '1', name: 'read_file', status: 'done' },
      { id: '2', name: 'read_file', status: 'error' },
    ])
    assert.equal(g[0].status, 'error')
    const g2 = groupToolBlocks([
      { id: '1', name: 'read_file', status: 'done' },
      { id: '2', name: 'read_file', status: 'running' },
    ])
    assert.equal(g2[0].status, 'running')
  })
})

describe('subagent/task blocks (VAL-CHAT-015)', () => {
  it('parses TaskSpawned structured detail', () => {
    const t = parseTaskEvent('TaskSpawned', {
      detail: 't1|explore|running|scan auth',
    })
    assert.equal(t.id, 't1')
    assert.equal(t.category, 'explore')
    assert.equal(t.status, 'running')
    assert.equal(t.label, 'scan auth')
  })

  it('parses TaskDone and upserts by id', () => {
    let list = upsertTaskBlock(
      [],
      parseTaskEvent('TaskSpawned', { detail: 't9|explore|running|a' }),
    )
    list = upsertTaskBlock(
      list,
      parseTaskEvent('TaskDone', { detail: 't9|explore|completed|a' }),
    )
    assert.equal(list.length, 1)
    assert.equal(list[0].status, 'completed')
  })

  it('defaults status when missing', () => {
    const spawn = parseTaskEvent('TaskSpawned', { detail: 'tx' })
    assert.equal(spawn.status, 'running')
    const done = parseTaskEvent('TaskDone', { detail: 'ty' })
    assert.equal(done.status, 'completed')
  })
})
