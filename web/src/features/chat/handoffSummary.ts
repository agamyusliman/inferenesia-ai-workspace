import { getGitStatus, getTodos, type GitStatus, type TodoListView } from '../../lib/api'
import type { ChatMessage } from './ChatPanel'

type HandoffInput = {
  messages: ChatMessage[]
  workspaceId?: string
  sessionTitle?: string
}

type HandoffResult = {
  text: string
  ok: boolean
  error?: string
}

/**
 * Generate a handoff context summary for continuing work in a new session.
 * Gathers user requests (verbatim), todos, git status, and assistant work
 * summary. Returns plain text safe to paste into a new session.
 */
export async function generateHandoffSummary(input: HandoffInput): Promise<HandoffResult> {
  const { messages, workspaceId, sessionTitle } = input

  if (messages.length === 0) {
    return { text: '', ok: false, error: 'No messages in this session to hand off.' }
  }

  const [todosResult, gitResult] = await Promise.allSettled([
    getTodos(),
    getGitStatus(),
  ])

  const todos: TodoListView | null =
    todosResult.status === 'fulfilled' ? todosResult.value : null
  const git: GitStatus | null =
    gitResult.status === 'fulfilled' ? gitResult.value : null

  const userRequests = extractUserRequests(messages)
  const workCompleted = extractWorkCompleted(messages)
  const assistantFindings = extractKeyFindings(messages)
  const gitBlock = formatGit(git)
  const filesBlock = formatFiles(git, messages)

  const lines: string[] = []

  lines.push('HANDOFF CONTEXT')
  lines.push('===============')
  lines.push('')

  // USER REQUESTS (AS-IS)
  lines.push('USER REQUESTS (AS-IS)')
  lines.push('---------------------')
  if (userRequests.length > 0) {
    for (const req of userRequests) {
      lines.push(`- ${req}`)
    }
  } else {
    lines.push('- (no explicit user requests detected)')
  }
  lines.push('')

  // GOAL
  lines.push('GOAL')
  lines.push('----')
  const lastUserRequest = userRequests[userRequests.length - 1]
  lines.push(lastUserRequest || 'Continue from the work described below.')
  lines.push('')

  // WORK COMPLETED
  lines.push('WORK COMPLETED')
  lines.push('--------------')
  if (workCompleted.length > 0) {
    for (const item of workCompleted) {
      lines.push(`- ${item}`)
    }
  } else {
    lines.push('- (no completed work identified yet)')
  }
  lines.push('')

  // CURRENT STATE
  lines.push('CURRENT STATE')
  lines.push('-------------')
  if (git) {
    lines.push(`- Git branch: ${git.branch}`)
    if (git.upstream) {
      lines.push(`- Upstream: ${git.upstream}`)
      if (git.ahead) lines.push(`- Ahead by ${git.ahead} commits`)
      if (git.behind) lines.push(`- Behind by ${git.behind} commits`)
    }
    lines.push(
      `- Uncommitted: ${git.staged_count} staged, ${git.unstaged_count} unstaged, ${git.untracked_count} untracked`,
    )
  } else {
    lines.push('- Git status unavailable')
  }
  if (workspaceId) {
    lines.push(`- Workspace: ${workspaceId}`)
  }
  if (sessionTitle) {
    lines.push(`- Session: ${sessionTitle}`)
  }
  lines.push('')

  // PENDING TASKS
  lines.push('PENDING TASKS')
  lines.push('-------------')
  if (todos && todos.todos.length > 0) {
    const pending = todos.todos.filter(
      (t) => t.status === 'pending' || t.status === 'in_progress',
    )
    const completed = todos.todos.filter((t) => t.status === 'completed')
    if (pending.length > 0) {
      for (const t of pending) {
        const marker = t.status === 'in_progress' ? '[in_progress]' : '[pending]'
        lines.push(`- ${marker} ${t.content}`)
      }
    }
    if (completed.length > 0) {
      lines.push(`- (${completed.length} completed todos omitted)`)
    }
    if (pending.length === 0 && completed.length > 0) {
      lines.push('- All todos completed.')
    }
  } else {
    lines.push('- (no todos set)')
  }
  lines.push('')

  // KEY FILES
  lines.push('KEY FILES')
  lines.push('---------')
  if (filesBlock.length > 0) {
    const unique = [...new Set(filesBlock)].slice(0, 10)
    for (const f of unique) {
      lines.push(`- ${f}`)
    }
  } else {
    lines.push('- (no specific files identified)')
  }
  lines.push('')

  // IMPORTANT DECISIONS / FINDINGS
  lines.push('IMPORTANT DECISIONS / FINDINGS')
  lines.push('------------------------------')
  if (assistantFindings.length > 0) {
    for (const f of assistantFindings) {
      lines.push(`- ${f}`)
    }
  } else {
    lines.push('- (none identified)')
  }
  lines.push('')

  // CONTEXT FOR CONTINUATION
  lines.push('CONTEXT FOR CONTINUATION')
  lines.push('------------------------')
  lines.push('- This summary was auto-generated from the session above.')
  lines.push('- Review USER REQUESTS for exact requirements (verbatim).')
  lines.push('- Check git status and uncommitted changes before continuing.')
  if (gitBlock) {
    lines.push(`- Git porcelain: ${gitBlock}`)
  }
  lines.push('')

  return { text: lines.join('\n'), ok: true }
}

function extractUserRequests(messages: ChatMessage[]): string[] {
  const requests: string[] = []
  for (const msg of messages) {
    if (msg.role !== 'user') continue
    if (msg.error) continue
    const text = msg.content.trim()
    if (!text) continue
    if (text.startsWith('Use tool `')) continue
    if (text.startsWith('/')) continue
    requests.push(text)
  }
  return requests
}

function extractWorkCompleted(messages: ChatMessage[]): string[] {
  const items: string[] = []
  for (const msg of messages) {
    if (msg.role !== 'assistant') continue
    if (msg.error) continue
    const text = msg.content.trim()
    if (!text || text.length < 10) continue

    const sentences = text.split(/[.\n]/).map((s) => s.trim()).filter(Boolean)
    for (const s of sentences) {
      const lower = s.toLowerCase()
      if (
        lower.startsWith('i ') ||
        lower.startsWith('i\'') ||
        lower.includes('created ') ||
        lower.includes('added ') ||
        lower.includes('fixed ') ||
        lower.includes('updated ') ||
        lower.includes('removed ') ||
        lower.includes('implemented ') ||
        lower.includes('shipped ') ||
        lower.includes('wired ') ||
        lower.includes('refactored ')
      ) {
        const truncated = s.length > 200 ? s.slice(0, 200) + '...' : s
        items.push(truncated)
        if (items.length >= 15) break
      }
    }
    if (items.length >= 15) break
  }
  return items
}

function extractKeyFindings(messages: ChatMessage[]): string[] {
  const findings: string[] = []
  for (const msg of messages) {
    if (msg.role !== 'assistant') continue
    const text = msg.content.trim()
    if (!text) continue

    const sentences = text.split(/[.\n]/).map((s) => s.trim()).filter(Boolean)
    for (const s of sentences) {
      const lower = s.toLowerCase()
      if (
        lower.includes('decision:') ||
        lower.includes('trade-off') ||
        lower.includes('tradeoff') ||
        lower.includes('important:') ||
        lower.includes('note:') ||
        lower.includes('warning:') ||
        lower.includes('gotcha') ||
        lower.includes('caveat') ||
        lower.includes('constraint') ||
        lower.includes('pattern:') ||
        lower.includes('convention')
      ) {
        const truncated = s.length > 200 ? s.slice(0, 200) + '...' : s
        findings.push(truncated)
        if (findings.length >= 10) break
      }
    }
    if (findings.length >= 10) break
  }
  return findings
}

function formatGit(git: GitStatus | null): string {
  if (!git || !git.is_repo) return ''
  return git.porcelain || ''
}

function formatFiles(git: GitStatus | null, messages: ChatMessage[]): string[] {
  const files = new Set<string>()

  if (git && git.files) {
    for (const f of git.files) {
      files.add(f.path)
    }
  }

  for (const msg of messages) {
    if (msg.mentions) {
      for (const m of msg.mentions) {
        files.add(m)
      }
    }
  }

  for (const msg of messages) {
    const matches = msg.content.match(
      /(?:web\/src\/|internal\/|cmd\/|docs\/)[^\s'"`)]+/g,
    )
    if (matches) {
      for (const m of matches) {
        files.add(m)
      }
    }
  }

  return [...files]
}
