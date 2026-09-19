export type SlashKind = 'tool' | 'config' | 'panel'

export type SlashCommand = {
  id: string
  name: string
  label: string
  description: string
  kind: SlashKind
  insert?: string
  action?:
    | 'plan'
    | 'todos'
    | 'missions'
    | 'tasks'
    | 'browser'
    | 'image-gen'
    | 'handoff'
    | 'clear'
    | 'compact'
    | 'usage'
    | 'diagnostics'
    | 'init'
    | 'review'
    | 'share'
    | 'model'
}
export const SLASH_COMMANDS: SlashCommand[] = [
  {
    id: 'cfg-plan',
    name: 'plan',
    label: 'Plan Mode',
    description: 'Toggle Plan Mode (read-only explore)',
    kind: 'config',
    action: 'plan',
  },
  {
    id: 'cfg-handoff',
    name: 'handoff',
    label: 'Handoff',
    description: 'Create context summary for continuing in a new session',
    kind: 'config',
    action: 'handoff',
  },
  {
    id: 'cfg-clear',
    name: 'clear',
    label: 'Clear chat',
    description: 'Clear all messages in this session (keep session)',
    kind: 'config',
    action: 'clear',
  },
  {
    id: 'cfg-compact',
    name: 'compact',
    label: 'Compact',
    description: 'Summarize context to reduce token usage',
    kind: 'config',
    action: 'compact',
  },
  {
    id: 'cfg-usage',
    name: 'usage',
    label: 'Token usage',
    description: 'Show token/context usage for this session',
    kind: 'config',
    action: 'usage',
  },
  {
    id: 'cfg-diagnostics',
    name: 'diagnostics',
    label: 'Diagnostics',
    description: 'Run go vet + typecheck + build check',
    kind: 'config',
    action: 'diagnostics',
  },
  {
    id: 'cfg-init',
    name: 'init',
    label: 'Init workspace',
    description: 'Initialize AGENTS.md knowledge base for workspace',
    kind: 'config',
    action: 'init',
  },
  {
    id: 'cfg-review',
    name: 'review',
    label: 'Code review',
    description: 'Review recent changes for quality, security, and bugs',
    kind: 'config',
    action: 'review',
  },
  {
    id: 'cfg-share',
    name: 'share',
    label: 'Share session',
    description: 'Export session as markdown to clipboard',
    kind: 'config',
    action: 'share',
  },
  {
    id: 'cfg-model',
    name: 'model',
    label: 'Switch model',
    description: 'Quick switch model/provider',
    kind: 'config',
    action: 'model',
  },
  {
    id: 'cfg-todos',
    name: 'todos',
    label: 'Todos panel',
    description: 'Open session todo checklist',
    kind: 'panel',
    action: 'todos',
  },
  {
    id: 'cfg-missions',
    name: 'missions',
    label: 'Missions panel',
    description: 'Open Mission / Goal panel',
    kind: 'panel',
    action: 'missions',
  },
  {
    id: 'cfg-tasks',
    name: 'tasks',
    label: 'Tasks panel',
    description: 'Open background task panel',
    kind: 'panel',
    action: 'tasks',
  },
  {
    id: 'cfg-browser',
    name: 'browser',
    label: 'Browser panel',
    description: 'Open browser status panel',
    kind: 'panel',
    action: 'browser',
  },
  {
    id: 'cfg-image',
    name: 'image',
    label: 'Image gen',
    description: 'Generate image for this message (no button toggle)',
    kind: 'config',
    action: 'image-gen',
    insert: '/image ',
  },
  {
    id: 'tool-read_file',
    name: 'read_file',
    label: 'read_file',
    description: 'Read a file (or list if path is a directory)',
    kind: 'tool',
    insert: 'Use tool `read_file` on path ',
  },
  {
    id: 'tool-grep',
    name: 'grep',
    label: 'grep',
    description: 'Search file contents (ripgrep-style)',
    kind: 'tool',
    insert: 'Use tool `grep` with pattern ',
  },
  {
    id: 'tool-list_dir',
    name: 'list_dir',
    label: 'list_dir',
    description: 'List directory entries',
    kind: 'tool',
    insert: 'Use tool `list_dir` on path ',
  },
  {
    id: 'tool-write_file',
    name: 'write_file',
    label: 'write_file',
    description: 'Write/create a file via WriteGateway',
    kind: 'tool',
    insert: 'Use tool `write_file` to write ',
  },
  {
    id: 'tool-shell',
    name: 'shell',
    label: 'shell',
    description: 'Run a shell command in the workspace',
    kind: 'tool',
    insert: 'Use tool `shell` with command ',
  },
  {
    id: 'tool-propose_plan',
    name: 'propose_plan',
    label: 'propose_plan',
    description: 'Propose a multi-step plan for user approval',
    kind: 'tool',
    insert: 'Use tool `propose_plan` with steps for: ',
  },
  {
    id: 'tool-todo_write',
    name: 'todo_write',
    label: 'todo_write',
    description: 'Replace session todo checklist',
    kind: 'tool',
    insert: 'Use tool `todo_write` to set todos for: ',
  },
  {
    id: 'tool-todo_update',
    name: 'todo_update',
    label: 'todo_update',
    description: 'Update one todo status/content',
    kind: 'tool',
    insert: 'Use tool `todo_update` to mark ',
  },
  {
    id: 'tool-todo_list',
    name: 'todo_list',
    label: 'todo_list',
    description: 'List current session todos',
    kind: 'tool',
    insert: 'Use tool `todo_list` and summarize the checklist.',
  },
  {
    id: 'tool-create_goal',
    name: 'create_goal',
    label: 'create_goal',
    description: 'Create a Mission/Goal',
    kind: 'tool',
    insert: 'Use tool `create_goal` with objective: ',
  },
  {
    id: 'tool-get_goal',
    name: 'get_goal',
    label: 'get_goal',
    description: 'Get mission status by id',
    kind: 'tool',
    insert: 'Use tool `get_goal` for mission ',
  },
  {
    id: 'tool-update_goal',
    name: 'update_goal',
    label: 'update_goal',
    description: 'Activate / block / complete a mission',
    kind: 'tool',
    insert: 'Use tool `update_goal` to ',
  },
  {
    id: 'tool-attach_mission_evidence',
    name: 'attach_mission_evidence',
    label: 'attach_mission_evidence',
    description: 'Attach evidence to a mission',
    kind: 'tool',
    insert: 'Use tool `attach_mission_evidence` with summary ',
  },
  {
    id: 'tool-task',
    name: 'task',
    label: 'task',
    description: 'Spawn parallel explore subagent (depth 1)',
    kind: 'tool',
    insert: 'Use tool `task` to explore: ',
  },
  {
    id: 'tool-skill',
    name: 'skill',
    label: 'skill',
    description: 'Load a progressive SKILL.md',
    kind: 'tool',
    insert: 'Use tool `skill` to load ',
  },
  {
    id: 'tool-multibrain_read',
    name: 'multibrain_read',
    label: 'multibrain_read',
    description: 'Read Multi Brain session/index',
    kind: 'tool',
    insert: 'Use tool `multibrain_read` for bucket ',
  },
  {
    id: 'tool-multibrain_append',
    name: 'multibrain_append',
    label: 'multibrain_append',
    description: 'Append Multi Brain memory entry',
    kind: 'tool',
    insert: 'Use tool `multibrain_append` with note: ',
  },
  {
    id: 'tool-git_status',
    name: 'git_status',
    label: 'git_status',
    description: 'Git status (agent hooks isolation)',
    kind: 'tool',
    insert: 'Use tool `git_status` and summarize changes.',
  },
  {
    id: 'tool-git_diff',
    name: 'git_diff',
    label: 'git_diff',
    description: 'Git diff',
    kind: 'tool',
    insert: 'Use tool `git_diff` for path ',
  },
  {
    id: 'tool-git_commit',
    name: 'git_commit',
    label: 'git_commit',
    description: 'Git commit (may need approval)',
    kind: 'tool',
    insert: 'Use tool `git_commit` with message: ',
  },
  {
    id: 'tool-terminal_list',
    name: 'terminal_list',
    label: 'terminal_list',
    description: 'List integrated terminal sessions',
    kind: 'tool',
    insert: 'Use tool `terminal_list`.',
  },
  {
    id: 'tool-terminal_read',
    name: 'terminal_read',
    label: 'terminal_read',
    description: 'Read terminal session output',
    kind: 'tool',
    insert: 'Use tool `terminal_read` for session ',
  },
  {
    id: 'tool-browser_set_engine',
    name: 'browser_set_engine',
    label: 'browser_set_engine',
    description: 'Launch engine: stealth|e2e|debug|camoufox (+ headless true/false)',
    kind: 'tool',
    insert:
      'Use tool `browser_set_engine` with engine "stealth" and headless true (or false for visible window).',
  },
  {
    id: 'tool-browser_status',
    name: 'browser_status',
    label: 'browser_status',
    description: 'Browser engine status',
    kind: 'tool',
    insert: 'Use tool `browser_status`.',
  },
  {
    id: 'tool-browser_navigate',
    name: 'browser_navigate',
    label: 'browser_navigate',
    description: 'Navigate browser to URL',
    kind: 'tool',
    insert: 'Use tool `browser_navigate` to ',
  },
  {
    id: 'tool-browser_snapshot',
    name: 'browser_snapshot',
    label: 'browser_snapshot',
    description: 'Page title + text for the model',
    kind: 'tool',
    insert: 'Use tool `browser_snapshot`.',
  },
  {
    id: 'tool-browser_screenshot',
    name: 'browser_screenshot',
    label: 'browser_screenshot',
    description: 'PNG path + size (not base64 to model)',
    kind: 'tool',
    insert: 'Use tool `browser_screenshot`.',
  },
  {
    id: 'tool-browser_close',
    name: 'browser_close',
    label: 'browser_close',
    description: 'Stop browser engine',
    kind: 'tool',
    insert: 'Use tool `browser_close`.',
  },
  {
    id: 'tool-browser_wait_human',
    name: 'browser_wait_human',
    label: 'browser_wait_human',
    description: 'Pause for captcha/MFA handoff',
    kind: 'tool',
    insert: 'Use tool `browser_wait_human` with reason ',
  },
  {
    id: 'tool-create_diagram',
    name: 'create_diagram',
    label: 'create_diagram',
    description: 'Create Mermaid/DBML diagram file',
    kind: 'tool',
    insert: 'Use tool `create_diagram` for ',
  },
]

export function filterSlashCommands(query: string, limit = 12): SlashCommand[] {
  const q = (query || '').trim().toLowerCase().replace(/^\//, '')
  const list = !q
    ? SLASH_COMMANDS
    : SLASH_COMMANDS.filter((c) => {
        const hay = `${c.name} ${c.label} ${c.description} ${c.kind}`.toLowerCase()
        return hay.includes(q) || c.name.startsWith(q)
      })
  list.sort((a, b) => {
    const ap = a.name.startsWith(q) ? 0 : 1
    const bp = b.name.startsWith(q) ? 0 : 1
    if (ap !== bp) return ap - bp
    if (a.kind !== b.kind) {
      const order = { config: 0, panel: 1, tool: 2 } as const
      return order[a.kind] - order[b.kind]
    }
    return a.name.localeCompare(b.name)
  })
  return list.slice(0, limit)
}
export function matchTrailingSlash(value: string): { query: string; start: number } | null {
  const m = value.match(/(?:^|[\s\n])\/([^\s]*)$/)
  if (!m) return null
  const full = m[0]
  const query = m[1] || ''
  const start = value.length - full.length + (full.startsWith(' ') || full.startsWith('\n') ? 1 : 0)
  return { query, start }
}

export function applySlashInsert(value: string, insert: string): string {
  const hit = matchTrailingSlash(value)
  if (!hit) return value + insert
  const before = value.slice(0, hit.start)
  const lead =
    before.length === 0 || before.endsWith('\n') || before.endsWith(' ') ? '' : ' '
  return `${before}${lead}${insert}`
}
