/**
 * Pure helpers for assistant meta row, collapsible thinking, tool-call blocks,
 * and subagent/task blocks (VAL-CHAT-012..015).
 * Kept free of React so node:test can run without a DOM.
 */

export type StreamStatusKind = 'generating' | 'streaming' | 'stopped' | 'done' | 'error' | 'idle'

export type ChatMeta = {
  /** Total tokens when available (prompt+completion or single total). */
  tokens?: number
  tokensPrompt?: number
  tokensCompletion?: number
  /** Wall-clock latency for the turn in milliseconds. */
  latencyMs?: number
  /** Stream status label (generating / done / stopped / …). */
  streamStatus?: StreamStatusKind | string
  /** Agent / Plan / Image (composer mode for this turn). */
  mode?: string
  /** Provider profile id (e.g. tempai, byok). */
  profile?: string
  /** Model id (e.g. grok-4.5). */
  model?: string
}

export type ToolCallStatus = 'running' | 'done' | 'error'

export type ToolCallBlock = {
  id: string
  name: string
  status: ToolCallStatus
  detail?: string
  result?: string
}

export type TaskBlockStatus = 'running' | 'queued' | 'completed' | 'failed' | 'cancelled' | string

export type TaskBlock = {
  id: string
  category?: string
  status: TaskBlockStatus
  label?: string
  detail?: string
}

export function formatTokenCount(n: number): string {
  if (!Number.isFinite(n)) return '0'
  const sign = n < 0 ? '-' : ''
  const abs = Math.trunc(Math.abs(n))
  return sign + String(abs).replace(/\B(?=(\d{3})+(?!\d))/g, '.')
}

/** Format tokens for the meta row (empty when unavailable). */
export function formatTokens(meta: ChatMeta | undefined | null): string {
  if (!meta) return ''
  if (meta.tokens != null && meta.tokens > 0) {
    return `${formatTokenCount(meta.tokens)} tokens`
  }
  if (
    (meta.tokensPrompt != null && meta.tokensPrompt > 0) ||
    (meta.tokensCompletion != null && meta.tokensCompletion > 0)
  ) {
    const p = meta.tokensPrompt ?? 0
    const c = meta.tokensCompletion ?? 0
    const total = p + c
    if (total <= 0) return ''
    if (p > 0 && c > 0) {
      return `${formatTokenCount(total)} tokens (${formatTokenCount(p)}+${formatTokenCount(c)})`
    }
    return `${formatTokenCount(total)} tokens`
  }
  return ''
}

export function formatLatency(meta: ChatMeta | undefined | null): string {
  if (!meta || meta.latencyMs == null || meta.latencyMs < 0) return ''
  const ms = meta.latencyMs
  if (ms < 1000) return `${ms} ms`
  const totalSec = ms / 1000
  if (totalSec < 60) {
    if (totalSec < 10) return `${totalSec.toFixed(1)} s`
    return `${Math.round(totalSec)} s`
  }
  const mins = Math.floor(totalSec / 60)
  const secs = Math.round(totalSec % 60)
  if (secs === 0) return `${mins}m`
  if (secs === 60) return `${mins + 1}m`
  return `${mins}m ${secs}s`
}

/**
 * Build ordered meta-row parts (OpenCode-style footer):
 * mode · profile · model · status · tokens · latency
 */
export function formatMetaParts(meta: ChatMeta | undefined | null): string[] {
  if (!meta) return []
  const parts: string[] = []
  const mode = (meta.mode || '').trim()
  if (mode) parts.push(mode)
  const profile = (meta.profile || '').trim()
  const model = (meta.model || '').trim()
  if (profile && model) {
    parts.push(`${profile}: ${model}`)
  } else if (model) {
    parts.push(model)
  } else if (profile) {
    parts.push(profile)
  }
  const status = (meta.streamStatus || '').trim()
  if (status && status !== 'idle' && status !== 'done') {
    // "done" is implied by the footer existing; keep stopped/error/generating visible.
    parts.push(status)
  }
  const tok = formatTokens(meta)
  if (tok) parts.push(tok)
  const lat = formatLatency(meta)
  if (lat) parts.push(lat)
  return parts
}

/** Visible when any footer field is present (mode/model/tokens/latency/status). */
export function shouldShowMetaRow(meta: ChatMeta | undefined | null): boolean {
  return formatMetaParts(meta).length > 0
}

/**
 * Split model reasoning/thinking from the final answer (VAL-CHAT-013).
 * Closed tags/fences + unclosed open tags/fences (live stream mid-thinking).
 */
export function splitThinkingContent(content: string | undefined | null): {
  thinking: string
  answer: string
} {
  if (!content) return { thinking: '', answer: '' }
  let rest = content
  const thinkingChunks: string[] = []

  const tagRe = /<\s*(thinking|think)\s*>([\s\S]*?)<\s*\/\s*\1\s*>/gi
  rest = rest.replace(tagRe, (_full, _tag: string, body: string) => {
    const t = body.trim()
    if (t) thinkingChunks.push(t)
    return ''
  })

  const fenceRe = /```(?:thinking|reasoning|thought)\s*\n?([\s\S]*?)```/gi
  rest = rest.replace(fenceRe, (_full, body: string) => {
    const t = String(body || '').trim()
    if (t) thinkingChunks.push(t)
    return ''
  })

  // Unclosed open tag while streaming: everything after open tag is still thinking.
  const openTag = rest.match(/<\s*(thinking|think)\s*>/i)
  if (openTag && openTag.index != null) {
    const after = rest.slice(openTag.index + openTag[0].length)
    const before = rest.slice(0, openTag.index)
    if (after.trim()) thinkingChunks.push(after.replace(/\s+$/, ''))
    rest = before
  } else {
    // Unclosed fence: ```thinking / ```reasoning still open.
    const openFence = rest.match(/```(?:thinking|reasoning|thought)\s*\n?/i)
    if (openFence && openFence.index != null) {
      const after = rest.slice(openFence.index + openFence[0].length)
      const before = rest.slice(0, openFence.index)
      if (after.trim()) thinkingChunks.push(after.replace(/\s+$/, ''))
      rest = before
    }
  }

  return {
    thinking: thinkingChunks.join('\n\n').trim(),
    answer: rest.replace(/^\s*\n+/, '').replace(/\n+\s*$/, '').trim(),
  }
}

/** Prefer explicit thinking field; else peel markers out of content. */
export function resolveThinkingAndAnswer(
  content: string | undefined | null,
  explicitThinking?: string | null,
): { thinking: string; answer: string } {
  const explicit = (explicitThinking || '').trim()
  if (explicit) {
    // Still strip tags from answer so they don't double-render.
    const split = splitThinkingContent(content)
    return {
      thinking: explicit + (split.thinking ? `\n\n${split.thinking}` : ''),
      answer: split.answer || (content || '').trim(),
    }
  }
  return splitThinkingContent(content)
}

/**
 * Parse ToolStart/ToolEnd/ToolError detail from hub events.
 * Go toolLineWriter emits "name detail…" or structured Name + Detail.
 */
export function parseToolEvent(
  type: 'ToolStart' | 'ToolEnd' | 'ToolError' | string,
  opts: { name?: string; detail?: string; error?: string; id?: string },
): ToolCallBlock {
  let name = (opts.name || '').trim()
  let detail = (opts.detail || '').trim()
  if (!name && detail) {
    const sp = detail.indexOf(' ')
    if (sp > 0) {
      name = detail.slice(0, sp)
      detail = detail.slice(sp + 1).trim()
    } else {
      name = detail
      detail = ''
    }
  }
  // ToolEnd lines often look like: "name detail → result"
  let result: string | undefined
  if (type === 'ToolEnd' || type === 'ToolError') {
    const arrow = detail.indexOf('→')
    if (arrow >= 0) {
      result = detail.slice(arrow + 1).trim()
      detail = detail.slice(0, arrow).trim()
      // detail may still start with tool name when Name was empty earlier
      if (!opts.name && detail.startsWith(name + ' ')) {
        detail = detail.slice(name.length).trim()
      } else if (!opts.name && detail === name) {
        detail = ''
      }
    }
    if (!result && opts.error) {
      result = opts.error
    }
  }
  if (!name) name = 'tool'
  const status: ToolCallStatus =
    type === 'ToolError' ? 'error' : type === 'ToolEnd' ? 'done' : 'running'
  const id = opts.id || `${name}-${status}-${hashShort(name + detail + (result || ''))}`
  return { id, name, status, detail: detail || undefined, result }
}

/**
 * Merge a parsed tool event into the list (match by name when id unknown;
 * running → done/error updates the same open block).
 */
export function upsertToolBlock(list: ToolCallBlock[], next: ToolCallBlock): ToolCallBlock[] {
  const out = [...list]
  // Prefer matching a running block with same name.
  let idx = out.findIndex((t) => t.status === 'running' && t.name === next.name)
  if (idx < 0 && next.id) {
    idx = out.findIndex((t) => t.id === next.id)
  }
  if (idx >= 0) {
    out[idx] = {
      ...out[idx],
      ...next,
      id: out[idx].id,
      detail: next.detail ?? out[idx].detail,
      result: next.result ?? out[idx].result,
    }
    return out
  }
  out.push(next)
  return out
}

/**
 * Parse TaskSpawned/TaskDone detail from hub:
 * "task_id|category|status|label" (see orchestrator_desk).
 */
export function parseTaskEvent(
  type: 'TaskSpawned' | 'TaskDone' | string,
  opts: { detail?: string; text?: string; name?: string },
): TaskBlock {
  const raw = (opts.detail || '').trim()
  const parts = raw.split('|')
  const id = (parts[0] || opts.name || `task-${Date.now()}`).trim() || 'task'
  const category = (parts[1] || '').trim() || undefined
  let status = (parts[2] || opts.text || '').trim()
  if (!status) {
    status = type === 'TaskDone' ? 'completed' : 'running'
  }
  const label = (parts[3] || '').trim() || undefined
  // Rest after first 4 segments may hold extra detail.
  const extra = parts.slice(4).join('|').trim()
  return {
    id,
    category,
    status,
    label,
    detail: extra || undefined,
  }
}

/** Upsert task by id (spawned → done updates). */
export function upsertTaskBlock(list: TaskBlock[], next: TaskBlock): TaskBlock[] {
  const out = [...list]
  const idx = out.findIndex((t) => t.id === next.id)
  if (idx >= 0) {
    out[idx] = { ...out[idx], ...next }
    return out
  }
  out.push(next)
  return out
}

/** Tool block status label for UI. */
export function toolStatusLabel(status: ToolCallStatus | string): string {
  switch (status) {
    case 'running':
      return 'running'
    case 'done':
      return 'done'
    case 'error':
      return 'error'
    default:
      return String(status || '')
  }
}

export function toolsActivityStatus(opts: {
  tools: ToolCallBlock[]
  streaming?: boolean
}): { kind: 'running' | 'active' | 'error' | 'done'; label: string } {
  const tools = opts.tools || []
  const total = tools.length
  const runN = tools.filter((t) => t.status === 'running').length
  const errN = tools.filter((t) => t.status === 'error').length
  const doneN = tools.filter((t) => t.status === 'done').length
  if (runN > 0) {
    return {
      kind: 'running',
      label: total > 1 ? `running ${runN}/${total}` : 'running',
    }
  }
  if (errN > 0 && !opts.streaming) {
    return {
      kind: 'error',
      label: errN === total ? 'error' : `error ${errN}/${total}`,
    }
  }
  if (opts.streaming) {
    return {
      kind: 'active',
      label: doneN > 0 && total > 0 ? `${doneN}/${total}` : 'working',
    }
  }
  if (errN > 0) {
    return {
      kind: 'error',
      label: errN === total ? 'error' : `error ${errN}/${total}`,
    }
  }
  return { kind: 'done', label: 'done' }
}

/** One UI row: either a single tool call or a group of same-name calls. */
export type ToolCallGroup = {
  key: string
  name: string
  status: ToolCallStatus
  tools: ToolCallBlock[]
}

/**
 * Collapse consecutive same-name tool calls into one accordion group.
 * Non-consecutive names stay separate (preserves chronological tool order).
 */
export function groupToolBlocks(tools: ToolCallBlock[] | undefined | null): ToolCallGroup[] {
  if (!tools || tools.length === 0) return []
  const groups: ToolCallGroup[] = []
  for (const t of tools) {
    const name = (t.name || 'tool').trim() || 'tool'
    const last = groups[groups.length - 1]
    if (last && last.name === name) {
      last.tools.push(t)
      last.status = mergeToolGroupStatus(last.tools)
      continue
    }
    groups.push({
      key: t.id || `${name}-${groups.length}`,
      name,
      status: t.status,
      tools: [t],
    })
  }
  return groups
}

function mergeToolGroupStatus(tools: ToolCallBlock[]): ToolCallStatus {
  if (tools.some((t) => t.status === 'running')) return 'running'
  if (tools.some((t) => t.status === 'error')) return 'error'
  return 'done'
}

/** Task block status label for UI. */
export function taskStatusLabel(status: TaskBlockStatus | string): string {
  const s = String(status || '').toLowerCase()
  if (!s) return ''
  return s
}

/** Truncate long tool results for the card body. */
export function truncateResult(text: string | undefined | null, max = 280): string {
  if (!text) return ''
  const t = text.trim()
  if (t.length <= max) return t
  return t.slice(0, max) + '…'
}

function hashShort(s: string): string {
  let h = 0
  for (let i = 0; i < s.length; i++) {
    h = (h * 31 + s.charCodeAt(i)) | 0
  }
  return Math.abs(h).toString(36).slice(0, 6)
}
