/**
 * Desktop API client. Prefers Wails Go bindings when available; falls back to
 * localhost hub HTTP (core.Service Handler) for browser/agent-browser and Vite.
 * Renderer never calls LLM providers directly (VAL-DESK-008).
 */

export type Workspace = {
  id: string
  name: string
  root_path: string
  kind?: 'folder' | 'session' | string
  pinned?: boolean
  last_opened_at?: string
  broken?: boolean
}

export function isSessionWorkspace(ws: Workspace | null | undefined): boolean {
  if (!ws) return false
  if (ws.kind === 'session') return true
  if (ws.id === 'ws_session' || ws.id.startsWith('ws_sess_')) return true
  const p = (ws.root_path || '').replace(/\\/g, '/')
  return p.includes('/session-scratch') || p.includes('/sessions/')
}

export type DirEntry = {
  name: string
  path: string
  is_dir: boolean
}

export type FSResult = {
  path: string
  absolute?: string
  is_dir?: boolean
  message?: string
}

export type PathInfo = {
  relative: string
  absolute: string
  exists: boolean
  is_dir: boolean
}

/** Find in Folder hit (VAL-IDE-008). */
export type FindHit = {
  path: string
  line?: number
  text?: string
  kind: 'content' | 'path' | string
}

export type HubState = {
  product: string
  active_id: string
  active?: Workspace | null
  workspaces: Workspace[]
  config_home: string
  selected_file?: string
  chat_context?: string
  status_message?: string
  /** Plan Mode active → agent write tools denied (VAL-PLAN-001). */
  plan_mode_active?: boolean
}

/** Plan Mode snapshot from core.Service (VAL-PLAN-001/007). */
export type PlanModeState = {
  active: boolean
  mode: string
  reason?: string
  product?: string
}

export type FileMention = {
  path: string
  name: string
}

export type ChatEventType =
  | 'TokenDelta'
  | 'ToolStart'
  | 'ToolEnd'
  | 'ToolError'
  | 'FileChanged'
  | 'UndoStackChanged'
  | 'NeedsApproval'
  | 'PlanProposed'
  | 'TodoUpdated'
  | 'MissionStatus'
  | 'TaskSpawned'
  | 'TaskDone'
  | 'BrowserStatus'
  | 'Done'
  | 'Cancelled'
  | 'Error'
  | 'Blocked'
  | 'RunStarted'

export type ChatEvent = {
  type: ChatEventType
  delta?: string
  text?: string
  name?: string
  path?: string
  detail?: string
  final?: string
  error?: string
  can_undo?: boolean
  can_redo?: boolean
  /** Done metadata: active profile after the turn (VAL-PROV-005). */
  profile?: string
  /** Done metadata: resolved model id (VAL-PROV-005). */
  model?: string
  /** Done metadata: request host, never key (VAL-PROV-006). */
  host?: string
  /** NeedsApproval: resolve via resolveApproval(id, approved) (VAL-GIT-005+). */
  approval_id?: string
  approval_kind?: string
  approval_force?: boolean
  /** Usage / latency on Done when available (VAL-CHAT-012). Never includes secrets. */
  tokens?: number
  tokens_prompt?: number
  tokens_completion?: number
  latency_ms?: number
  /** Optional reasoning/thinking delta or block (VAL-CHAT-013). */
  thinking?: string
  run_id?: string
}

/** PlanProposed payload for Approve/Reject dialog (VAL-PLAN-003/004). */
export type PlanProposalView = {
  id: string
  title?: string
  summary?: string
  body?: string
  steps: string[]
  status: string
  created_at?: string
}

/** Session todo item (pending → in_progress → completed; VAL-PLAN-005). */
export type TodoItem = {
  id: string
  content: string
  status: 'pending' | 'in_progress' | 'completed' | 'cancelled' | string
  priority?: string
}

export type TodoListView = {
  todos: TodoItem[]
  count: number
  product?: string
}

export type PlanApproveResult = {
  plan: PlanProposalView
  todos: TodoListView
  mode: PlanModeState
  hub: HubState
  seeded: number
}

export type PlanRejectResult = {
  plan: PlanProposalView
  todos: TodoListView
  mode: PlanModeState
  hub: HubState
}

/** Vision/reference image part (data URL) for paperclip attachments (VAL-CHAT-022). */
export type ChatImagePart = {
  name?: string
  media_type?: string
  data_url: string
}

export type ChatRequest = {
  prompt: string
  mentions?: string[]
  /** Up to 4 vision/reference images as data URLs (VAL-CHAT-022 / VAL-CHAT-025 refs). */
  images?: ChatImagePart[]
  model?: string
  profile?: string
  no_tools?: boolean
  max_tool_rounds?: number
  /** Image gen toggle → core gateway /images/generations when supported (VAL-CHAT-025). */
  generate_image?: boolean
  /** Optional size hint for image gen ("1024x1024", "1792x1024", …). */
  image_size?: string
  /** Optional temperature for image gen (0–2). */
  image_temperature?: number
  /** Optional Responses reasoning effort: low | medium | high. */
  image_reasoning_effort?: string
  /** Prefer image-only output with minimal caption. */
  image_only?: boolean
  /**
   * Ephemeral continue prompt: core injects for the model but does NOT store
   * as a user history row (VAL-CHAT-026). With generate_image, skips transcript.
   */
  ephemeral?: boolean
  /** Marks auto-continue follow-up turns (with ephemeral). */
  continue?: boolean
  /** Skip loading workspace chat history into model messages (slides/canvas isolation). */
  no_history?: boolean
  /** Explicit chat session ID for history loading (session-bound playground mode). */
  session_id?: string
  /** Playground-scoped history key. When set, playground history is loaded/merged/persisted separately. */
  playground_id?: string
  /** Agent kind selects the generation system prompt and context budget. */
  agent_kind?: 'coding' | 'slides' | 'excalidraw' | 'diagram' | 'html' | 'image' | 'markdown' | 'table' | 'timeline'
  workspace_id?: string
}

/** Durable one-thread-per-workspace chat history (VAL-DESK-014). */
export type DesktopChatMessage = {
  id?: string
  role: 'user' | 'assistant' | 'system' | string
  content: string
  mentions?: string[]
  /** Per-message meta row (tokens/latency/stream status) restored from
   * sessions.db after reload (misc-chat-meta-persist-history). Assistant
   * messages only. Never carries secrets. */
  meta?: DesktopChatMeta
  /** Persisted tool-call blocks rehydrated after reload (VAL-CHAT-014). */
  tools?: DesktopToolBlock[]
  /** Persisted subagent/task blocks rehydrated after reload (VAL-CHAT-015). */
  tasks?: DesktopTaskBlock[]
  /** Persisted reasoning content rehydrated after reload (VAL-CHAT-013). */
  thinking?: string
  /** Non-secret routing labels for the assistant turn. */
  model?: string
  profile?: string
  host?: string
}

/** Persisted per-message meta for the ChatMetaRow (VAL-CHAT-012). */
export type DesktopChatMeta = {
  tokens?: number
  tokens_prompt?: number
  tokens_completion?: number
  latency_ms?: number
  stream_status?: string
  mode?: string
  profile?: string
  model?: string
}

/** Persisted tool-call block (VAL-CHAT-014). */
export type DesktopToolBlock = {
  id?: string
  name?: string
  status?: string
  detail?: string
  result?: string
}

/** Persisted subagent/task block (VAL-CHAT-015). */
export type DesktopTaskBlock = {
  id?: string
  category?: string
  status?: string
  label?: string
  detail?: string
}

export type DesktopChatHistory = {
  workspace_id: string
  session_id?: string
  model?: string
  profile?: string
  messages: DesktopChatMessage[]
}

/** Result of durable single-message delete or undo-from-here (VAL-CHAT-017/018). */
export type ChatMessageActionResult = {
  workspace_id: string
  session_id?: string
  messages: DesktopChatMessage[]
  removed: number
  action: string
  message?: string
}

export type UndoStackState = {
  can_undo: boolean
  can_redo: boolean
  undo_depth: number
  redo_depth: number
  revert_agent_label: string
  restore_to_head_label: string
  undo_label: string
  redo_label: string
  message?: string
}

export type UndoActionResult = {
  message: string
  noop: boolean
  paths?: string[]
  stack: UndoStackState
}

export type RestoreToHeadResult = {
  message: string
  label: string
  path?: string
  warning: string
}

/** WriteGateway agent-change timeline row (VAL-DESK-012; not git history). */
export type AgentChangeEntry = {
  index: number
  unit_id: string
  turn_id?: string
  created_at?: string
  paths: string[]
  sources: string[]
  file_count: number
  kind: 'agent_turn' | 'user_edit' | 'mixed' | string
  summary: string
}

/** Desktop agent-changes timeline payload (VAL-DESK-012). */
export type AgentChangeHistory = {
  title: string
  subtitle: string
  empty_message: string
  entries: AgentChangeEntry[]
  count: number
  source: string
}

export type ProfileView = {
  id: string
  name: string
  type: string
  /** Effective wire format: "openai" | "anthropic" (P9-prov). Derived from
   *  api_format or type when the backend omits it. */
  api_format?: string
  base_host?: string
  /** Full base path with credentials stripped (VAL-PROV-001). */
  base_url_masked?: string
  current_model?: string
  default_model?: string
  has_key: boolean
  is_default: boolean
  is_gateway?: boolean
}

export type OnboardingPath = {
  id: string
  title: string
  description: string
  requires_other: boolean
}

export type OnboardingState = {
  done: boolean
  paths: OnboardingPath[]
  has_tempai_key: boolean
  has_byok: boolean
  recommended: string
}

/** One optional Token Saver (RTK / Headroom / Caveman / Ponytail; VAL-SAVER-001). */
export type TokenSaverStatus = {
  id: string
  name: string
  description: string
  enabled: boolean
  installed: boolean
  /** "ready" | "not_installed" */
  status: string
  detail?: string
  command?: string
  base_url?: string
}

/** Settings → Token Saver section payload (defaults off; VAL-SAVER-001/002/007). */
export type TokenSaversView = {
  product: string
  config_home: string
  savers: TokenSaverStatus[]
  note?: string
}

export type SetTokenSaverRequest = {
  id: string
  enabled?: boolean
  command?: string
  base_url?: string
}

/** Settings → Live Blocks opt-in toggle payload (P9-live). Default off. */
export type LiveBlocksView = {
  product: string
  enabled: boolean
  note?: string
}

export type SetLiveBlocksRequest = {
  enabled: boolean
}

export type MCPServerView = {
  name: string
  type: string
  enabled: boolean
  ok: boolean
  error?: string
  tool_names?: string[]
  tool_count: number
  builtin?: boolean
  layer?: string
  managed?: boolean
  command?: string
  base_url?: string
}

export type MCPStatusView = {
  servers: MCPServerView[]
  total_servers: number
  enabled_count: number
  healthy_count: number
  total_tools: number
}

export type MCPServerAction =
  | { action: 'toggle'; name: string; enabled: boolean }
  | { action: 'add'; name: string; server: Record<string, unknown> }
  | { action: 'remove'; name: string }
  | { action: 'reload' }

export type AutonomyView = {
  level: string
  description: string
}

export type SetAutonomyRequest = {
  level: string
}

export type SettingsView = {
  product: string
  config_home: string
  sections: { id: string; title: string; description: string }[]
  profiles: ProfileView[]
  default_profile: string
  selected_profile?: string
  models?: { id: string; owned_by?: string }[]
  models_error?: string
  workspace: {
    active_id?: string
    active_name?: string
    root_path?: string
    undo_is_pre_agent_dirty: boolean
    undo_note: string
  }
  labels: {
    revert_agent: string
    restore_to_head: string
    undo: string
    redo: string
  }
  onboarding?: OnboardingState
  /** Optional Token Saver section (embedded for settings panel). */
  token_savers?: TokenSaversView
  /** Optional Live Blocks opt-in toggle (embedded for settings panel; P9-live). */
  live_blocks?: LiveBlocksView
  /** Optional MCP servers section (status + tools; embedded for settings panel). */
  mcp?: MCPStatusView
  autonomy?: AutonomyView
}

export type AddProviderRequest = {
  id?: string
  name?: string
  type?: string
  /** Wire format override: "openai" | "anthropic" (P9-prov). When set, drives
   *  the adapter even if type is openai_compatible (e.g. LiteLLM Claude host). */
  api_format?: string
  base_url: string
  api_key?: string
  api_key_env?: string
  default_model?: string
  set_default?: boolean
  onboarding_path?: string
}

export type TestProviderResult = {
  ok: boolean
  profile: string
  model?: string
  host?: string
  preview?: string
  error?: string
  latency_ms?: number
}

/** Mid-session active provider/model (VAL-PROV-005, VAL-PROV-011). */
export type ActiveSessionView = {
  profile: string
  model?: string
  host?: string
  profile_name?: string
  is_gateway?: boolean
  profiles?: ProfileView[]
  models?: { id: string; owned_by?: string }[]
  models_error?: string
}

export type SetActiveProviderRequest = {
  profile: string
  model?: string
  set_default?: boolean
}

/** Integrated terminal session (internal/pty via core.Service). */
export type TerminalSession = {
  id: string
  title?: string
  command?: string
  cwd: string
  status: string
  pid: number
  exit_code?: number | null
}

export type TerminalReadResult = {
  session: TerminalSession
  output: string
}

export type TerminalStopResult = {
  session: TerminalSession
  message: string
  stopped: boolean
}

export type TerminalStartRequest = {
  cwd?: string
  workspace_id?: string
  title?: string
  shell?: string
}

/** Global terminal snippet under ~/.inferenesia (VAL-IDE-036). */
export type TerminalSnippet = {
  id: string
  name: string
  body: string
  created_at?: string
  updated_at?: string
}

export type TerminalSnippetList = {
  path: string
  snippets: TerminalSnippet[]
}

export type TerminalSnippetSaveRequest = {
  id?: string
  name: string
  body: string
}

/** One file from git status --porcelain (VAL-GIT-001). */
export type GitFileStatus = {
  path: string
  index: string
  worktree: string
  staged: boolean
  unstaged: boolean
  untracked: boolean
  status_label: string
  original?: string
}

/** Nested repo under a multi-project workspace (multi source-control). */
export type GitRepoRef = {
  id: string
  name: string
  rel: string
  root: string
  branch?: string
  dirty_count?: number
  is_repo: boolean
}

/** Desktop Git panel status snapshot (VAL-GIT-001). */
export type GitStatus = {
  branch: string
  is_repo: boolean
  root: string
  repo_id?: string
  repo_name?: string
  files: GitFileStatus[]
  staged_count: number
  unstaged_count: number
  untracked_count: number
  porcelain: string
  upstream?: string
  ahead?: number
  behind?: number
  message?: string
  repos?: GitRepoRef[]
}

/** Unified diff for one path (VAL-GIT-002). */
export type GitDiffResult = {
  path: string
  staged: boolean
  content: string
  empty: boolean
  message?: string
}

/** Stage/commit/fetch/pull result with refreshed status when successful. */
export type GitOpResult = {
  ok: boolean
  message: string
  detail?: string
  hash?: string
  repo_id?: string
  status?: GitStatus
}

export type GitStageRequest = {
  repo_id?: string
  paths?: string[]
  path?: string
}

export type GitCommitRequest = {
  repo_id?: string
  message: string
}

export type GitGraphCommit = {
  sha: string
  short: string
  subject: string
  body?: string
  author_name: string
  author_email?: string
  author_date: string
  parents: string[]
  refs?: string[]
  lane: number
}

export type GitGraphHead = {
  name: string
  sha: string
  remote?: boolean
  current?: boolean
}

export type GitGraphResult = {
  repo_id?: string
  branch: string
  commits: GitGraphCommit[]
  heads: GitGraphHead[]
  truncated?: boolean
  message?: string
}

export type GitBranchInfo = {
  name: string
  sha: string
  short_sha?: string
  remote: boolean
  current: boolean
  subject?: string
  date?: string
}

export type GitBranchList = {
  repo_id?: string
  current: string
  local: GitBranchInfo[]
  remote: GitBranchInfo[]
  message?: string
}

export type GitCheckoutRequest = {
  repo_id?: string
  name: string
  create?: boolean
}

export type GitWorkflowRules = {
  label?: string
  default_branch?: string
  commit_style?: string
  commit_template?: string
  before_commit?: string[]
  never_commit?: string[]
  push?: {
    require_confirm?: boolean
    allow_force?: boolean
    remote?: string
    protected_branches?: string[]
  }
  deploy?: {
    enabled?: boolean
    when?: string
    command?: string
    steps?: string[]
    env_hint?: string
  }
  rules?: string
  agent_notes?: string
}

export type GitWorkflow = {
  version: number
  defaults: GitWorkflowRules
  repos?: Record<string, GitWorkflowRules>
  source_path?: string
  exists?: boolean
}

const API_BASE =
  (import.meta.env.VITE_HUB_API as string | undefined)?.replace(/\/$/, '') || ''

type GoMain = {
  BrandName?: () => Promise<string> | string
  GetHubState?: () => Promise<HubState> | HubState
  ListWorkspaces?: () => Promise<Workspace[]> | Workspace[]
  OpenWorkspace?: (path: string) => Promise<Workspace> | Workspace
  PickDirectory?: () => Promise<string> | string
  SwitchWorkspace?: (id: string) => Promise<Workspace> | Workspace
  ListDir?: (workspaceID: string, rel: string) => Promise<DirEntry[]> | DirEntry[]
  SelectFile?: (rel: string) => Promise<HubState> | HubState
  ReadFile?: (workspaceID: string, rel: string) => Promise<string> | string
  SaveFile?: (
    workspaceID: string,
    rel: string,
    content: string,
  ) => Promise<FSResult> | FSResult
  CreateFile?: (
    workspaceID: string,
    parent: string,
    name: string,
  ) => Promise<FSResult> | FSResult
  CreateFolder?: (
    workspaceID: string,
    parent: string,
    name: string,
  ) => Promise<FSResult> | FSResult
  RenamePath?: (
    workspaceID: string,
    rel: string,
    newName: string,
  ) => Promise<FSResult> | FSResult
  DeletePath?: (workspaceID: string, rel: string) => Promise<FSResult> | FSResult
  CopyPath?: (
    workspaceID: string,
    src: string,
    destParent: string,
  ) => Promise<FSResult> | FSResult
  MovePath?: (
    workspaceID: string,
    src: string,
    destParent: string,
  ) => Promise<FSResult> | FSResult
  FindInFolder?: (
    workspaceID: string,
    folder: string,
    query: string,
    limit: number,
  ) => Promise<FindHit[]> | FindHit[]
  ResolvePath?: (workspaceID: string, rel: string) => Promise<PathInfo> | PathInfo
  RevealInOS?: (workspaceID: string, rel: string) => Promise<FSResult> | FSResult
  RemoveWorkspace?: (id: string) => Promise<void> | void
  AddFolderToWorkspace?: (
    workspaceID: string,
    path: string,
  ) => Promise<Workspace> | Workspace
  GetUndoStack?: () => Promise<UndoStackState> | UndoStackState
  UndoLast?: () => Promise<UndoActionResult> | UndoActionResult
  RedoLast?: () => Promise<UndoActionResult> | UndoActionResult
  RevertAgent?: () => Promise<UndoActionResult> | UndoActionResult
  RestoreToHEAD?: (path: string) => Promise<RestoreToHeadResult> | RestoreToHeadResult
  GetAgentChangeHistory?: () => Promise<AgentChangeHistory> | AgentChangeHistory
  GetSettings?: (profile: string) => Promise<SettingsView> | SettingsView
  GetTokenSavers?: () => Promise<TokenSaversView> | TokenSaversView
  SetTokenSaver?: (
    req: SetTokenSaverRequest,
  ) => Promise<TokenSaversView> | TokenSaversView
  GetLiveBlocks?: () => Promise<LiveBlocksView> | LiveBlocksView
  SetLiveBlocks?: (
    req: SetLiveBlocksRequest,
  ) => Promise<LiveBlocksView> | LiveBlocksView
  AddProvider?: (
    req: AddProviderRequest,
  ) => Promise<SettingsView> | SettingsView
  TestProvider?: (profile: string) => Promise<TestProviderResult> | TestProviderResult
  GetOnboarding?: () => Promise<OnboardingState> | OnboardingState
  CompleteOnboarding?: (
    path: string,
  ) => Promise<OnboardingState> | OnboardingState
  GetActiveSession?: () => Promise<ActiveSessionView> | ActiveSessionView
  SetActiveProvider?: (
    req: SetActiveProviderRequest,
  ) => Promise<ActiveSessionView> | ActiveSessionView
  GetChatHistory?: (
    workspaceID: string,
  ) => Promise<DesktopChatHistory> | DesktopChatHistory
  DeleteChatMessage?: (
    workspaceID: string,
    messageID: string,
  ) => Promise<ChatMessageActionResult> | ChatMessageActionResult
  TruncateChatFromHere?: (
    workspaceID: string,
    messageID: string,
  ) => Promise<ChatMessageActionResult> | ChatMessageActionResult
  SetChatHistory?: (
    workspaceID: string,
    messages: DesktopChatMessage[],
  ) => Promise<DesktopChatHistory> | DesktopChatHistory
  GetChatSession?: () => Promise<ChatRequest & { messages?: DesktopChatMessage[] }> | unknown
  ListMentionFiles?: (
    query: string,
    limit: number,
  ) => Promise<FileMention[]> | FileMention[]
  TerminalStart?: (
    req: TerminalStartRequest,
  ) => Promise<TerminalSession> | TerminalSession
  TerminalList?: () => Promise<TerminalSession[]> | TerminalSession[]
  TerminalWrite?: (req: { id: string; data: string }) => Promise<void> | void
  TerminalResize?: (req: {
    id: string
    cols: number
    rows: number
  }) => Promise<void> | void
  TerminalRead?: (id: string) => Promise<TerminalReadResult> | TerminalReadResult
  TerminalStop?: (id: string) => Promise<TerminalStopResult> | TerminalStopResult
  ListTerminalSnippets?: () => Promise<TerminalSnippetList> | TerminalSnippetList
  SaveTerminalSnippet?: (
    req: TerminalSnippetSaveRequest,
  ) => Promise<TerminalSnippet> | TerminalSnippet
  DeleteTerminalSnippet?: (
    id: string,
  ) => Promise<TerminalSnippetList> | TerminalSnippetList
  ListGitRepos?: () => Promise<GitRepoRef[]> | GitRepoRef[]
  GetGitStatus?: (repoID: string) => Promise<GitStatus> | GitStatus
  GetGitDiff?: (
    repoID: string,
    path: string,
    staged: boolean,
  ) => Promise<GitDiffResult> | GitDiffResult
  GetGitGraph?: (repoID: string, limit: number) => Promise<GitGraphResult> | GitGraphResult
  GitStage?: (req: GitStageRequest) => Promise<GitOpResult> | GitOpResult
  GitUnstage?: (req: GitStageRequest) => Promise<GitOpResult> | GitOpResult
  GitCommit?: (req: GitCommitRequest) => Promise<GitOpResult> | GitOpResult
  GitFetch?: (repoID: string) => Promise<GitOpResult> | GitOpResult
  GitPull?: (repoID: string) => Promise<GitOpResult> | GitOpResult
  GitPush?: (req: GitPushRequest) => Promise<GitOpResult> | GitOpResult
  GitDestructive?: (req: GitDestructiveRequest) => Promise<GitOpResult> | GitOpResult
  ResolveApproval?: (dec: { id: string; approved: boolean }) => Promise<void> | void
  GetPendingApproval?: () => Promise<PendingApproval | null> | PendingApproval | null
  ListGitBranches?: (repoID: string) => Promise<GitBranchList> | GitBranchList
  GitCheckout?: (req: GitCheckoutRequest) => Promise<GitOpResult> | GitOpResult
  GetGitWorkflow?: () => Promise<GitWorkflow> | GitWorkflow
  SaveGitWorkflow?: (w: GitWorkflow) => Promise<GitWorkflow> | GitWorkflow
  EnsureGitWorkflow?: () => Promise<GitWorkflow> | GitWorkflow
  GetGitActions?: (
    repoID: string,
    limit: number,
  ) => Promise<GitActionsResult> | GitActionsResult
  OpenGitActionsRun?: (
    url: string,
  ) => Promise<{ ok: boolean; url?: string; message?: string }> | {
    ok: boolean
    url?: string
    message?: string
  }
  CanvasAI?: (
    req: CanvasAIRequest,
  ) => Promise<CanvasAIResult> | CanvasAIResult
}

function goMain(): GoMain | null {
  const w = window as unknown as { go?: { main?: { App?: GoMain } } }
  return w.go?.main?.App ?? null
}

async function httpJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const url = `${API_BASE}${path}`
  const res = await fetch(url, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers || {}),
    },
  })
  const body = await res.json().catch(() => ({}))
  if (!res.ok) {
    const msg = (body as { error?: string; message?: string }).error
      || (body as { message?: string }).message
      || res.statusText
    throw new Error(msg || `HTTP ${res.status}`)
  }
  return body as T
}

export async function fetchPlaygroundImage(url: string): Promise<Blob> {
  const res = await fetch(`${API_BASE}/api/playground/image`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url }),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(body.error || `Image download failed (HTTP ${res.status})`)
  }
  return res.blob()
}

export async function getHubState(): Promise<HubState> {
  const g = goMain()
  if (g?.GetHubState) {
    return g.GetHubState()
  }
  return httpJSON<HubState>('/api/hub')
}

export async function openSessionWorkspace(): Promise<Workspace> {
  const res = await httpJSON<{ workspace: Workspace }>('/api/workspaces/session', {
    method: 'POST',
    body: JSON.stringify({ create: false }),
  })
  return res.workspace
}

export async function createSessionWorkspace(
  name?: string,
): Promise<{ workspace: Workspace; hub?: HubState }> {
  const res = await httpJSON<{ workspace: Workspace; hub?: HubState }>(
    '/api/workspaces/session',
    {
      method: 'POST',
      body: JSON.stringify({ create: true, name: name || '' }),
    },
  )
  return res
}

export async function renameWorkspace(id: string, name: string): Promise<Workspace> {
  const res = await httpJSON<{ workspace: Workspace }>('/api/workspaces/rename', {
    method: 'POST',
    body: JSON.stringify({ id, name }),
  })
  return res.workspace
}

export async function openWorkspace(
  path: string,
): Promise<{ workspace: Workspace; hub?: HubState }> {
  const g = goMain()
  if (g?.OpenWorkspace) {
    const workspace = await g.OpenWorkspace(path)
    return { workspace }
  }
  const res = await httpJSON<{ workspace: Workspace; hub?: HubState }>(
    '/api/workspaces/open',
    {
      method: 'POST',
      body: JSON.stringify({ path }),
    },
  )
  return res
}

/** Native folder picker. Empty string = cancelled. */
export async function pickDirectory(): Promise<string> {
  const g = goMain()
  if (g?.PickDirectory) {
    return g.PickDirectory()
  }
  const res = await httpJSON<{ path?: string; cancelled?: boolean }>(
    '/api/workspaces/browse',
    { method: 'POST', body: '{}' },
  )
  return (res.path || '').trim()
}

export async function switchWorkspace(
  id: string,
): Promise<{ workspace: Workspace; hub?: HubState }> {
  const g = goMain()
  if (g?.SwitchWorkspace) {
    const workspace = await g.SwitchWorkspace(id)
    return { workspace }
  }
  const res = await httpJSON<{ workspace: Workspace; hub?: HubState }>(
    '/api/workspaces/switch',
    {
      method: 'POST',
      body: JSON.stringify({ id }),
    },
  )
  return res
}

export async function listDir(workspaceID: string, rel: string): Promise<DirEntry[]> {
  const g = goMain()
  if (g?.ListDir) {
    const raw = await g.ListDir(workspaceID, rel)
    return Array.isArray(raw) ? raw : []
  }
  const q = new URLSearchParams()
  if (rel) q.set('rel', rel)
  if (workspaceID) q.set('workspace_id', workspaceID)
  const qs = q.toString()
  const raw = await httpJSON<DirEntry[] | null>(`/api/fs/list${qs ? `?${qs}` : ''}`)
  return Array.isArray(raw) ? raw : []
}

export async function selectFile(rel: string): Promise<HubState> {
  const g = goMain()
  if (g?.SelectFile) {
    return g.SelectFile(rel)
  }
  return httpJSON<HubState>('/api/fs/select', {
    method: 'POST',
    body: JSON.stringify({ path: rel }),
  })
}

export async function readFile(workspaceID: string, path: string): Promise<string> {
  const g = goMain()
  if (g?.ReadFile) {
    return g.ReadFile(workspaceID, path)
  }
  const q = new URLSearchParams({ path })
  if (workspaceID) q.set('workspace_id', workspaceID)
  const res = await httpJSON<{ content: string }>(`/api/fs/read?${q}`)
  return res.content
}

/** Persist editor buffer as a user edit via core.Service / WriteGateway (VAL-IDE-017/025). */
export async function saveFile(
  workspaceID: string,
  path: string,
  content: string,
): Promise<FSResult> {
  const g = goMain()
  if (g?.SaveFile) {
    return g.SaveFile(workspaceID, path, content)
  }
  return httpJSON<FSResult>('/api/fs/write', {
    method: 'POST',
    body: JSON.stringify({ workspace_id: workspaceID, path, content }),
  })
}

export async function getUndoStack(): Promise<UndoStackState> {
  const g = goMain()
  if (g?.GetUndoStack) {
    return g.GetUndoStack()
  }
  return httpJSON<UndoStackState>('/api/undo/stack')
}

export async function undoLast(): Promise<UndoActionResult> {
  const g = goMain()
  if (g?.UndoLast) {
    return g.UndoLast()
  }
  return httpJSON<UndoActionResult>('/api/undo', { method: 'POST', body: '{}' })
}

export async function redoLast(): Promise<UndoActionResult> {
  const g = goMain()
  if (g?.RedoLast) {
    return g.RedoLast()
  }
  return httpJSON<UndoActionResult>('/api/redo', { method: 'POST', body: '{}' })
}

export async function revertAgent(): Promise<UndoActionResult> {
  const g = goMain()
  if (g?.RevertAgent) {
    return g.RevertAgent()
  }
  return httpJSON<UndoActionResult>('/api/revert-agent', { method: 'POST', body: '{}' })
}

export async function restoreToHead(path: string): Promise<RestoreToHeadResult> {
  const g = goMain()
  if (g?.RestoreToHEAD) {
    return g.RestoreToHEAD(path)
  }
  return httpJSON<RestoreToHeadResult>('/api/restore-to-head', {
    method: 'POST',
    body: JSON.stringify({ path }),
  })
}

/** WriteGateway agent-change timeline — not raw git history (VAL-DESK-012). */
export async function getAgentChangeHistory(): Promise<AgentChangeHistory> {
  const g = goMain()
  if (g?.GetAgentChangeHistory) {
    return g.GetAgentChangeHistory()
  }
  return httpJSON<AgentChangeHistory>('/api/agent-changes')
}

export type FileTimelineEntry = {
  id: string
  source: 'local' | 'git' | string
  kind?: string
  summary: string
  author?: string
  created_at?: string
  sha?: string
  short_sha?: string
  unit_id?: string
  paths?: string[]
}

export type FileTimeline = {
  path: string
  entries: FileTimelineEntry[]
  count: number
  local_count: number
  git_count: number
  message?: string
  empty_message?: string
}

export async function getFileTimeline(
  path: string,
  limit = 50,
): Promise<FileTimeline> {
  const q = new URLSearchParams()
  if (path) q.set('path', path)
  if (limit) q.set('limit', String(limit))
  return httpJSON<FileTimeline>(`/api/file-timeline?${q}`)
}

export type FileTimelineChange = {
  path: string
  source: string
  title: string
  original: string
  modified: string
  label_original: string
  label_modified: string
  message?: string
  unit_id?: string
  sha?: string
}

export async function getFileTimelineChange(opts: {
  path: string
  source: string
  unit_id?: string
  sha?: string
}): Promise<FileTimelineChange> {
  const q = new URLSearchParams()
  q.set('path', opts.path)
  q.set('source', opts.source)
  if (opts.unit_id) q.set('unit_id', opts.unit_id)
  if (opts.sha) q.set('sha', opts.sha)
  return httpJSON<FileTimelineChange>(`/api/file-timeline/change?${q}`)
}

export async function getSettings(profile = ''): Promise<SettingsView> {
  const g = goMain()
  if (g?.GetSettings) {
    return g.GetSettings(profile)
  }
  const q = profile ? `?profile=${encodeURIComponent(profile)}` : ''
  return httpJSON<SettingsView>(`/api/settings${q}`)
}

/** Settings → Token Saver list + install status (VAL-SAVER-001). */
export async function getTokenSavers(): Promise<TokenSaversView> {
  const g = goMain()
  if (g?.GetTokenSavers) {
    return g.GetTokenSavers()
  }
  return httpJSON<TokenSaversView>('/api/token-savers')
}

/**
 * Persist one Token Saver toggle under ~/.inferenesia (VAL-SAVER-002/007).
 * Enabling a missing binary does not fail — UI shows Not installed.
 */
export async function setTokenSaver(
  req: SetTokenSaverRequest,
): Promise<TokenSaversView> {
  const g = goMain()
  if (g?.SetTokenSaver) {
    return g.SetTokenSaver(req)
  }
  return httpJSON<TokenSaversView>('/api/token-savers', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

/** Settings → Live Blocks opt-in toggle (P9-live). Default off. */
export async function getLiveBlocks(): Promise<LiveBlocksView> {
  const g = goMain()
  if (g?.GetLiveBlocks) {
    return g.GetLiveBlocks()
  }
  return httpJSON<LiveBlocksView>('/api/live-blocks')
}

/** Persist the Live Blocks toggle under ~/.inferenesia (P9-live). */
export async function setLiveBlocks(
  req: SetLiveBlocksRequest,
): Promise<LiveBlocksView> {
  const g = goMain()
  if (g?.SetLiveBlocks) {
    return g.SetLiveBlocks(req)
  }
  return httpJSON<LiveBlocksView>('/api/live-blocks', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

export async function getMCPStatus(): Promise<MCPStatusView> {
  return httpJSON<MCPStatusView>('/api/mcp/status')
}

export async function mcpServerAction(
  req: MCPServerAction,
): Promise<MCPStatusView> {
  return httpJSON<MCPStatusView>('/api/mcp/servers', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

export async function getAutonomy(): Promise<AutonomyView> {
  return httpJSON<AutonomyView>('/api/autonomy')
}

export async function setAutonomy(
  req: SetAutonomyRequest,
): Promise<AutonomyView> {
  return httpJSON<AutonomyView>('/api/autonomy', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

/** Add a BYOK openai_compatible profile (VAL-PROV-003). Key is never returned. */
export async function addProvider(
  req: AddProviderRequest,
): Promise<SettingsView> {
  const g = goMain()
  if (g?.AddProvider) {
    return g.AddProvider(req)
  }
  return httpJSON<SettingsView>('/api/providers', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

/** Minimal chat completion test against a profile (VAL-PROV-004). */
export async function testProvider(profile: string): Promise<TestProviderResult> {
  const g = goMain()
  if (g?.TestProvider) {
    return g.TestProvider(profile)
  }
  return httpJSON<TestProviderResult>('/api/providers/test', {
    method: 'POST',
    body: JSON.stringify({ profile }),
  })
}

/** First-run dual-path state (temp-ai vs BYOK; VAL-PROV-010). */
export async function getOnboarding(): Promise<OnboardingState> {
  const g = goMain()
  if (g?.GetOnboarding) {
    return g.GetOnboarding()
  }
  return httpJSON<OnboardingState>('/api/onboarding')
}

export async function completeOnboarding(path: string): Promise<OnboardingState> {
  const g = goMain()
  if (g?.CompleteOnboarding) {
    return g.CompleteOnboarding(path)
  }
  return httpJSON<OnboardingState>('/api/onboarding/complete', {
    method: 'POST',
    body: JSON.stringify({ path }),
  })
}

/** Mid-session active provider/model for chat header picker (VAL-PROV-005). */
export async function getActiveSession(): Promise<ActiveSessionView> {
  const g = goMain()
  if (g?.GetActiveSession) {
    return g.GetActiveSession()
  }
  return httpJSON<ActiveSessionView>('/api/session/active')
}

/**
 * Switch active provider/model mid-session without app restart (VAL-PROV-005/011).
 * Does not clear chat history. Next streamChat turn uses the new profile/model.
 */
export async function setActiveProvider(
  req: SetActiveProviderRequest,
): Promise<ActiveSessionView> {
  const g = goMain()
  if (g?.SetActiveProvider) {
    return g.SetActiveProvider(req)
  }
  return httpJSON<ActiveSessionView>('/api/session/active', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

export async function listMentions(query = '', limit = 80): Promise<FileMention[]> {
  const g = goMain()
  if (g?.ListMentionFiles) {
    return g.ListMentionFiles(query, limit)
  }
  const q = new URLSearchParams()
  if (query) q.set('q', query)
  if (limit) q.set('limit', String(limit))
  const qs = q.toString()
  return httpJSON<FileMention[]>(`/api/mentions${qs ? `?${qs}` : ''}`)
}

export async function createFile(
  workspaceID: string,
  parent: string,
  name: string,
): Promise<FSResult> {
  const g = goMain()
  if (g?.CreateFile) {
    return g.CreateFile(workspaceID, parent, name)
  }
  return httpJSON<FSResult>('/api/fs/create-file', {
    method: 'POST',
    body: JSON.stringify({ workspace_id: workspaceID, parent, name }),
  })
}

export async function createFolder(
  workspaceID: string,
  parent: string,
  name: string,
): Promise<FSResult> {
  const g = goMain()
  if (g?.CreateFolder) {
    return g.CreateFolder(workspaceID, parent, name)
  }
  return httpJSON<FSResult>('/api/fs/create-folder', {
    method: 'POST',
    body: JSON.stringify({ workspace_id: workspaceID, parent, name }),
  })
}

export async function renamePath(
  workspaceID: string,
  path: string,
  newName: string,
): Promise<FSResult> {
  const g = goMain()
  if (g?.RenamePath) {
    return g.RenamePath(workspaceID, path, newName)
  }
  return httpJSON<FSResult>('/api/fs/rename', {
    method: 'POST',
    body: JSON.stringify({ workspace_id: workspaceID, path, new_name: newName }),
  })
}

export async function deletePath(workspaceID: string, path: string): Promise<FSResult> {
  const g = goMain()
  if (g?.DeletePath) {
    return g.DeletePath(workspaceID, path)
  }
  return httpJSON<FSResult>('/api/fs/delete', {
    method: 'POST',
    body: JSON.stringify({ workspace_id: workspaceID, path }),
  })
}

/** Clipboard paste (copy mode) via core.Service / WriteGateway (VAL-IDE-013). */
export async function copyPathFS(
  workspaceID: string,
  src: string,
  destParent: string,
): Promise<FSResult> {
  const g = goMain()
  if (g?.CopyPath) {
    return g.CopyPath(workspaceID, src, destParent)
  }
  return httpJSON<FSResult>('/api/fs/copy', {
    method: 'POST',
    body: JSON.stringify({
      workspace_id: workspaceID,
      src,
      dest_parent: destParent,
    }),
  })
}

/** Clipboard paste (cut/move mode) via core.Service / WriteGateway (VAL-IDE-013). */
export async function movePathFS(
  workspaceID: string,
  src: string,
  destParent: string,
): Promise<FSResult> {
  const g = goMain()
  if (g?.MovePath) {
    return g.MovePath(workspaceID, src, destParent)
  }
  return httpJSON<FSResult>('/api/fs/move', {
    method: 'POST',
    body: JSON.stringify({
      workspace_id: workspaceID,
      src,
      dest_parent: destParent,
    }),
  })
}

/**
 * Find in Folder: content + path search scoped under folder (VAL-IDE-008).
 * Results never include paths outside the selected folder.
 */
export async function findInFolder(
  workspaceID: string,
  folder: string,
  query: string,
  limit = 200,
): Promise<FindHit[]> {
  const g = goMain()
  if (g?.FindInFolder) {
    return g.FindInFolder(workspaceID, folder, query, limit)
  }
  const q = new URLSearchParams()
  if (workspaceID) q.set('workspace_id', workspaceID)
  if (folder) q.set('folder', folder)
  if (query) q.set('q', query)
  if (limit) q.set('limit', String(limit))
  return httpJSON<FindHit[]>(`/api/fs/find?${q}`)
}

export async function resolvePath(workspaceID: string, path: string): Promise<PathInfo> {
  const g = goMain()
  if (g?.ResolvePath) {
    return g.ResolvePath(workspaceID, path)
  }
  const q = new URLSearchParams()
  if (path) q.set('path', path)
  if (workspaceID) q.set('workspace_id', workspaceID)
  const qs = q.toString()
  return httpJSON<PathInfo>(`/api/fs/resolve${qs ? `?${qs}` : ''}`)
}

export async function revealInOS(workspaceID: string, path: string): Promise<FSResult> {
  const g = goMain()
  if (g?.RevealInOS) {
    return g.RevealInOS(workspaceID, path)
  }
  return httpJSON<FSResult>('/api/fs/reveal', {
    method: 'POST',
    body: JSON.stringify({ workspace_id: workspaceID, path }),
  })
}

export async function removeWorkspace(id: string): Promise<void> {
  const g = goMain()
  if (g?.RemoveWorkspace) {
    await g.RemoveWorkspace(id)
    return
  }
  await httpJSON<{ ok: boolean }>('/api/workspaces/remove', {
    method: 'POST',
    body: JSON.stringify({ id }),
  })
}

export async function addFolderToWorkspace(
  workspaceID: string,
  path: string,
): Promise<Workspace> {
  const g = goMain()
  if (g?.AddFolderToWorkspace) {
    return g.AddFolderToWorkspace(workspaceID, path)
  }
  const res = await httpJSON<{ workspace: Workspace }>('/api/workspaces/add-folder', {
    method: 'POST',
    body: JSON.stringify({ workspace_id: workspaceID, path }),
  })
  return res.workspace
}

/** Copy text to the system clipboard (for Copy Path / Copy Relative Path). */
export async function copyToClipboard(text: string): Promise<void> {
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text)
    return
  }
  // Fallback for restricted contexts
  const ta = document.createElement('textarea')
  ta.value = text
  ta.style.position = 'fixed'
  ta.style.left = '-9999px'
  document.body.appendChild(ta)
  ta.select()
  document.execCommand('copy')
  document.body.removeChild(ta)
}

// --- Integrated terminal (internal/pty via core.Service; VAL-IDE-023/024) ---

export async function terminalStart(
  req: TerminalStartRequest = {},
): Promise<TerminalSession> {
  const g = goMain()
  if (g?.TerminalStart) {
    return g.TerminalStart(req)
  }
  return httpJSON<TerminalSession>('/api/terminal/start', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

export async function terminalList(): Promise<TerminalSession[]> {
  const g = goMain()
  if (g?.TerminalList) {
    return g.TerminalList()
  }
  return httpJSON<TerminalSession[]>('/api/terminal/list')
}

export async function terminalWrite(id: string, data: string): Promise<void> {
  const g = goMain()
  if (g?.TerminalWrite) {
    await g.TerminalWrite({ id, data })
    return
  }
  await httpJSON<{ ok: boolean }>('/api/terminal/write', {
    method: 'POST',
    body: JSON.stringify({ id, data }),
  })
}

/** Push xterm FitAddon size into the PTY (avoids zsh partial-line "%" on new tabs). */
export async function terminalResize(
  id: string,
  cols: number,
  rows: number,
): Promise<void> {
  if (!id || cols <= 0 || rows <= 0) return
  const g = goMain()
  if (g?.TerminalResize) {
    await g.TerminalResize({ id, cols, rows })
    return
  }
  await httpJSON<{ ok: boolean }>('/api/terminal/resize', {
    method: 'POST',
    body: JSON.stringify({ id, cols, rows }),
  })
}

export async function terminalRead(id: string): Promise<TerminalReadResult> {
  const g = goMain()
  if (g?.TerminalRead) {
    return g.TerminalRead(id)
  }
  return httpJSON<TerminalReadResult>(
    `/api/terminal/read?id=${encodeURIComponent(id)}`,
  )
}

export async function terminalStop(id: string): Promise<TerminalStopResult> {
  const g = goMain()
  if (g?.TerminalStop) {
    return g.TerminalStop(id)
  }
  return httpJSON<TerminalStopResult>('/api/terminal/stop', {
    method: 'POST',
    body: JSON.stringify({ id }),
  })
}

// --- Global terminal snippets under ~/.inferenesia (VAL-IDE-036) ---

export async function listTerminalSnippets(): Promise<TerminalSnippetList> {
  const g = goMain()
  if (g?.ListTerminalSnippets) {
    return g.ListTerminalSnippets()
  }
  return httpJSON<TerminalSnippetList>('/api/snippets/terminal')
}

export async function saveTerminalSnippet(
  req: TerminalSnippetSaveRequest,
): Promise<TerminalSnippet> {
  const g = goMain()
  if (g?.SaveTerminalSnippet) {
    return g.SaveTerminalSnippet(req)
  }
  return httpJSON<TerminalSnippet>('/api/snippets/terminal', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

export async function deleteTerminalSnippet(
  id: string,
): Promise<TerminalSnippetList> {
  const g = goMain()
  if (g?.DeleteTerminalSnippet) {
    return g.DeleteTerminalSnippet(id)
  }
  return httpJSON<TerminalSnippetList>('/api/snippets/terminal/delete', {
    method: 'POST',
    body: JSON.stringify({ id }),
  })
}

export type TerminalStreamEvent =
  | { type: 'snapshot'; id: string; output: string; session?: TerminalSession }
  | { type: 'delta'; id: string; delta: string }

/**
 * Live PTY output via SSE (GET /api/terminal/stream?id=).
 * Falls back to polling terminalRead when EventSource is unavailable.
 */
export function streamTerminal(
  id: string,
  onEvent: (ev: TerminalStreamEvent) => void,
  signal?: AbortSignal,
): void {
  const url = `${API_BASE}/api/terminal/stream?id=${encodeURIComponent(id)}`
  // Prefer fetch+stream so we share AbortSignal and work under restricted CSP.
  void (async () => {
    try {
      const res = await fetch(url, { signal })
      if (!res.ok || !res.body) {
        // Poll fallback
        await pollTerminal(id, onEvent, signal)
        return
      }
      const reader = res.body.getReader()
      const decoder = new TextDecoder()
      let buf = ''
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        if (signal?.aborted) break
        buf += decoder.decode(value, { stream: true })
        let sep: number
        while ((sep = buf.indexOf('\n\n')) >= 0) {
          const chunk = buf.slice(0, sep)
          buf = buf.slice(sep + 2)
          for (const line of chunk.split('\n')) {
            const trimmed = line.trim()
            if (!trimmed.startsWith('data:')) continue
            const raw = trimmed.slice(5).trim()
            if (!raw) continue
            try {
              onEvent(JSON.parse(raw) as TerminalStreamEvent)
            } catch {
              // ignore
            }
          }
        }
      }
    } catch (e) {
      if (signal?.aborted) return
      // Last resort: poll
      try {
        await pollTerminal(id, onEvent, signal)
      } catch {
        // ignore
      }
    }
  })()
}

async function pollTerminal(
  id: string,
  onEvent: (ev: TerminalStreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  let last = ''
  while (!signal?.aborted) {
    const res = await terminalRead(id)
    if (res.output !== last) {
      if (last === '') {
        onEvent({ type: 'snapshot', id, output: res.output, session: res.session })
      } else if (res.output.startsWith(last)) {
        onEvent({ type: 'delta', id, delta: res.output.slice(last.length) })
      } else {
        onEvent({ type: 'snapshot', id, output: res.output, session: res.session })
      }
      last = res.output
    }
    await new Promise((r) => setTimeout(r, 200))
  }
}

/** List nested git repos under workspace (multi source-control). */
export async function listGitRepos(): Promise<GitRepoRef[]> {
  const g = goMain()
  if (g?.ListGitRepos) {
    return g.ListGitRepos()
  }
  const res = await httpJSON<{ repos: GitRepoRef[] }>('/api/git/repos')
  return res.repos || []
}

/** Git panel status (branch + porcelain files; VAL-GIT-001). */
export async function getGitStatus(repoID = ''): Promise<GitStatus> {
  const g = goMain()
  if (g?.GetGitStatus) {
    return g.GetGitStatus(repoID)
  }
  const q = repoID ? `?repo_id=${encodeURIComponent(repoID)}` : ''
  return httpJSON<GitStatus>(`/api/git/status${q}`)
}

/** File diff for the Git panel (VAL-GIT-002). */
export async function getGitDiff(
  path: string,
  staged = false,
  repoID = '',
): Promise<GitDiffResult> {
  const g = goMain()
  if (g?.GetGitDiff) {
    return g.GetGitDiff(repoID, path, staged)
  }
  const q = new URLSearchParams({ path })
  if (staged) q.set('staged', '1')
  if (repoID) q.set('repo_id', repoID)
  return httpJSON<GitDiffResult>(`/api/git/diff?${q}`)
}

/** Commit graph for Graph tab. */
export async function getGitGraph(
  repoID = '',
  limit = 80,
): Promise<GitGraphResult> {
  const g = goMain()
  if (g?.GetGitGraph) {
    return g.GetGitGraph(repoID, limit)
  }
  const q = new URLSearchParams()
  if (repoID) q.set('repo_id', repoID)
  if (limit) q.set('limit', String(limit))
  const qs = q.toString()
  return httpJSON<GitGraphResult>(`/api/git/graph${qs ? `?${qs}` : ''}`)
}

/** Stage path(s) (VAL-GIT-003). */
export async function gitStage(req: GitStageRequest): Promise<GitOpResult> {
  const g = goMain()
  if (g?.GitStage) {
    return g.GitStage(req)
  }
  return httpJSON<GitOpResult>('/api/git/stage', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

/** Unstage path(s) (VAL-GIT-003). */
export async function gitUnstage(req: GitStageRequest): Promise<GitOpResult> {
  const g = goMain()
  if (g?.GitUnstage) {
    return g.GitUnstage(req)
  }
  return httpJSON<GitOpResult>('/api/git/unstage', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

/** Commit staged changes with a message (VAL-GIT-004). */
export async function gitCommit(req: GitCommitRequest): Promise<GitOpResult> {
  const g = goMain()
  if (g?.GitCommit) {
    return g.GitCommit(req)
  }
  return httpJSON<GitOpResult>('/api/git/commit', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

/** Fetch remote with clear success/failure (VAL-GIT-010). */
export async function gitFetch(repoID = ''): Promise<GitOpResult> {
  const g = goMain()
  if (g?.GitFetch) {
    return g.GitFetch(repoID)
  }
  return httpJSON<GitOpResult>('/api/git/fetch', {
    method: 'POST',
    body: JSON.stringify({ repo_id: repoID || undefined }),
  })
}

/** Pull remote with clear success/failure (VAL-GIT-010). */
export async function gitPull(repoID = ''): Promise<GitOpResult> {
  const g = goMain()
  if (g?.GitPull) {
    return g.GitPull(repoID)
  }
  return httpJSON<GitOpResult>('/api/git/pull', {
    method: 'POST',
    body: JSON.stringify({ repo_id: repoID || undefined }),
  })
}

export type GitPushRequest = {
  repo_id?: string
  remote?: string
  force?: boolean
  force_with_lease?: boolean
  /** Must be true after UI confirmation dialog (VAL-GIT-011). */
  confirmed: boolean
}

export type GitDestructiveRequest = {
  repo_id?: string
  kind: 'reset_hard' | 'branch_delete' | string
  ref?: string
  force_delete?: boolean
  confirmed: boolean
}

export type PendingApproval = {
  id: string
  kind: string
  summary: string
  detail?: string
  repo_id?: string
  force?: boolean
  created_at?: string
}

/** Non-force (or confirmed force) push from UI (VAL-GIT-011 / VAL-GIT-007). */
export async function gitPush(req: GitPushRequest): Promise<GitOpResult> {
  const g = goMain()
  if (g?.GitPush) {
    return g.GitPush(req)
  }
  return httpJSON<GitOpResult>('/api/git/push', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

/** Confirmed destructive git op from UI (VAL-GIT-006). */
export async function gitDestructive(
  req: GitDestructiveRequest,
): Promise<GitOpResult> {
  const g = goMain()
  if (g?.GitDestructive) {
    return g.GitDestructive(req)
  }
  return httpJSON<GitOpResult>('/api/git/destructive', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

/** Resolve NeedsApproval (agent commit / force-push / destructive) (VAL-GIT-005+). */
export async function resolveApproval(
  id: string,
  approved: boolean,
): Promise<{ ok: boolean }> {
  const g = goMain()
  if (g?.ResolveApproval) {
    await g.ResolveApproval({ id, approved })
    return { ok: true }
  }
  return httpJSON<{ ok: boolean }>('/api/approvals/resolve', {
    method: 'POST',
    body: JSON.stringify({ id, approved }),
  })
}

/** Latest pending NeedsApproval request if any. */
export async function getPendingApproval(): Promise<PendingApproval | null> {
  const g = goMain()
  if (g?.GetPendingApproval) {
    const p = await g.GetPendingApproval()
    return p || null
  }
  const res = await httpJSON<{ pending: PendingApproval | null }>(
    '/api/approvals/pending',
  )
  return res.pending || null
}

/** List branches for checkout UI. */
export async function listGitBranches(repoID = ''): Promise<GitBranchList> {
  const g = goMain()
  if (g?.ListGitBranches) {
    return g.ListGitBranches(repoID)
  }
  const q = repoID ? `?repo_id=${encodeURIComponent(repoID)}` : ''
  return httpJSON<GitBranchList>(`/api/git/branches${q}`)
}

/** Switch or create branch. */
export async function gitCheckout(
  req: GitCheckoutRequest,
): Promise<GitOpResult> {
  const g = goMain()
  if (g?.GitCheckout) {
    return g.GitCheckout(req)
  }
  return httpJSON<GitOpResult>('/api/git/checkout', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

/** Load per-workspace git/deploy workflow (agent rules). */
export async function getGitWorkflow(): Promise<GitWorkflow> {
  const g = goMain()
  if (g?.GetGitWorkflow) {
    return g.GetGitWorkflow()
  }
  return httpJSON<GitWorkflow>('/api/git/workflow')
}

/** Save per-workspace git/deploy workflow. */
export async function saveGitWorkflow(w: GitWorkflow): Promise<GitWorkflow> {
  const g = goMain()
  if (g?.SaveGitWorkflow) {
    return g.SaveGitWorkflow(w)
  }
  return httpJSON<GitWorkflow>('/api/git/workflow', {
    method: 'POST',
    body: JSON.stringify(w),
  })
}

/** Create default workflow file if missing. */
export async function ensureGitWorkflow(): Promise<GitWorkflow> {
  const g = goMain()
  if (g?.EnsureGitWorkflow) {
    return g.EnsureGitWorkflow()
  }
  return httpJSON<GitWorkflow>('/api/git/workflow/ensure', {
    method: 'POST',
    body: '{}',
  })
}

/** One GitHub Actions workflow run (VAL-GHA). */
export type GitWorkflowRun = {
  id: number
  name: string
  display_title?: string
  status: string
  conclusion?: string
  branch?: string
  event?: string
  html_url: string
  created_at?: string
  updated_at?: string
  relative_time?: string
  /** queued | in_progress | success | failure | cancelled | skipped | unknown */
  badge: string
}

/** Git panel Actions tab payload (never includes tokens). */
export type GitActionsResult = {
  ok: boolean
  availability: string
  message: string
  detail?: string
  owner?: string
  repo?: string
  remote_url?: string
  auth_method?: string
  runs: GitWorkflowRun[]
  fetched_at?: string
  repo_id?: string
  limit?: number
}

/** List recent GitHub Actions runs for the selected repo (VAL-GHA-002/005). */
export async function getGitActions(
  repoID = '',
  limit = 20,
): Promise<GitActionsResult> {
  const g = goMain()
  if (g?.GetGitActions) {
    return g.GetGitActions(repoID, limit)
  }
  const q = new URLSearchParams()
  if (repoID) q.set('repo_id', repoID)
  if (limit) q.set('limit', String(limit))
  const qs = q.toString()
  return httpJSON<GitActionsResult>(`/api/git/actions${qs ? `?${qs}` : ''}`)
}

/** Open a validated github.com workflow run URL (VAL-GHA-004). */
export async function openGitActionsRun(
  url: string,
): Promise<{ ok: boolean; url?: string; message?: string }> {
  const g = goMain()
  if (g?.OpenGitActionsRun) {
    return g.OpenGitActionsRun(url)
  }
  return httpJSON('/api/git/actions/open', {
    method: 'POST',
    body: JSON.stringify({ url }),
  })
}

/**
 * Load durable chat history for a workspace (VAL-DESK-014).
 * Backed by core.Service + sessions.db, not React-only state.
 */
export async function getChatHistory(
  workspaceID = '',
): Promise<DesktopChatHistory> {
  const g = goMain()
  if (g?.GetChatHistory) {
    return g.GetChatHistory(workspaceID)
  }
  const q = workspaceID
    ? `?workspace_id=${encodeURIComponent(workspaceID)}`
    : ''
  return httpJSON<DesktopChatHistory>(`/api/chat/history${q}`)
}

/** Delete one chat message (user or assistant) and persist (VAL-CHAT-018). */
export async function deleteChatMessage(
  workspaceID: string,
  messageID: string,
): Promise<ChatMessageActionResult> {
  const g = goMain()
  if (g?.DeleteChatMessage) {
    return g.DeleteChatMessage(workspaceID, messageID)
  }
  return httpJSON<ChatMessageActionResult>('/api/chat/message/delete', {
    method: 'POST',
    body: JSON.stringify({ workspace_id: workspaceID, message_id: messageID }),
  })
}

/** Undo-from-here: truncate durable history from message id to end (VAL-CHAT-017). */
export async function truncateChatFromHere(
  workspaceID: string,
  messageID: string,
): Promise<ChatMessageActionResult> {
  const g = goMain()
  if (g?.TruncateChatFromHere) {
    return g.TruncateChatFromHere(workspaceID, messageID)
  }
  return httpJSON<ChatMessageActionResult>('/api/chat/message/truncate', {
    method: 'POST',
    body: JSON.stringify({ workspace_id: workspaceID, message_id: messageID }),
  })
}

/** Replace durable workspace chat messages. */
export async function setChatHistory(
  workspaceID: string,
  messages: DesktopChatMessage[],
): Promise<DesktopChatHistory> {
  const g = goMain()
  if (g?.SetChatHistory) {
    return g.SetChatHistory(workspaceID, messages)
  }
  return httpJSON<DesktopChatHistory>('/api/chat/history', {
    method: 'POST',
    body: JSON.stringify({ workspace_id: workspaceID, messages }),
  })
}

/** Current Plan Mode snapshot (VAL-PLAN-001). */
export async function getPlanMode(): Promise<PlanModeState> {
  return httpJSON<PlanModeState>('/api/plan/mode')
}

/** Enter read-only Plan Mode — agent write tools denied (VAL-PLAN-001). */
export async function enterPlanMode(
  reason = 'user',
): Promise<{ plan: PlanModeState; hub: HubState }> {
  return httpJSON<{ plan: PlanModeState; hub: HubState }>('/api/plan/enter', {
    method: 'POST',
    body: JSON.stringify({ reason }),
  })
}

/** Exit Plan Mode (approve | reject | cancel | user) — re-enable writes (VAL-PLAN-007). */
export async function exitPlanMode(
  reason = 'user',
): Promise<{ plan: PlanModeState; hub: HubState }> {
  return httpJSON<{ plan: PlanModeState; hub: HubState }>('/api/plan/exit', {
    method: 'POST',
    body: JSON.stringify({ reason }),
  })
}

/** Pending PlanProposed proposal for Approve/Reject UI (VAL-PLAN-003). */
export async function getPendingPlan(): Promise<PlanProposalView | null> {
  const res = await httpJSON<{ pending: PlanProposalView | null }>(
    '/api/plan/pending',
  )
  return res.pending || null
}

/**
 * Propose a plan (hub/dev/tests) — same core path as agent propose_plan tool.
 * Emits pending for PlanProposalDialog Approve/Reject (VAL-PLAN-003/004).
 */
export async function proposePlan(input: {
  title?: string
  summary?: string
  body?: string
  steps: string[]
}): Promise<{ pending: PlanProposalView; hub?: HubState }> {
  return httpJSON<{ pending: PlanProposalView; hub?: HubState }>(
    '/api/plan/propose',
    {
      method: 'POST',
      body: JSON.stringify(input),
    },
  )
}

/** Approve proposed plan → seed todos (pending) and exit Plan Mode (VAL-PLAN-003). */
export async function approvePlan(): Promise<PlanApproveResult> {
  return httpJSON<PlanApproveResult>('/api/plan/approve', {
    method: 'POST',
    body: '{}',
  })
}

/** Reject proposed plan — no todos created; panel unchanged (VAL-PLAN-004). */
export async function rejectPlan(): Promise<PlanRejectResult> {
  return httpJSON<PlanRejectResult>('/api/plan/reject', {
    method: 'POST',
    body: '{}',
  })
}

/** Session todo checklist for the todo panel (VAL-PLAN-005). */
export async function getTodos(): Promise<TodoListView> {
  return httpJSON<TodoListView>('/api/todos')
}

// --- Mission panel (VAL-MISSION-001..006) ---

export type MissionEvidence = {
  id: string
  kind: string
  summary: string
  content?: string
  command?: string
  exit_code?: number
  gate_name?: string
  created_at?: string
}

export type MissionGate = {
  name: string
  command: string
  passed: boolean
  exit_code: number
  output?: string
  ran_at?: string
  duration_ms?: number
}

export type MissionView = {
  id: string
  workspace_id?: string
  goal: string
  budget_tokens?: number
  tokens_used?: number
  status: string
  blocker_reason?: string
  acceptance_criteria?: string[]
  evidence: MissionEvidence[]
  gates: MissionGate[]
  created_at?: string
  updated_at?: string
  completed_at?: string
  product?: string
}

export type MissionListView = {
  missions: MissionView[]
  count: number
  product?: string
}

export type MissionCompleteResult = {
  ok: boolean
  mission: MissionView
  message?: string
  error?: string
}

/** List missions for the mission panel (VAL-MISSION-001). */
export async function listMissions(): Promise<MissionListView> {
  return httpJSON<MissionListView>('/api/missions')
}

/** Create a mission with goal + optional budget (VAL-MISSION-001). */
export async function createMission(input: {
  goal: string
  budget_tokens?: number
  workspace_id?: string
  acceptance_criteria?: string[]
}): Promise<{ mission: MissionView; missions?: MissionListView }> {
  return httpJSON<{ mission: MissionView; missions?: MissionListView }>(
    '/api/missions',
    {
      method: 'POST',
      body: JSON.stringify(input),
    },
  )
}

/** Attach verification evidence (VAL-MISSION-003); secrets redacted server-side. */
export async function attachMissionEvidence(input: {
  id: string
  kind?: string
  summary: string
  content?: string
  command?: string
  exit_code?: number
  gate_name?: string
}): Promise<{ mission: MissionView }> {
  return httpJSON<{ mission: MissionView }>('/api/missions/evidence', {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

/**
 * Mark mission complete — blocked without evidence (VAL-MISSION-002).
 * Returns ok=false + message containing "evidence required" when denied.
 */
export async function completeMission(
  id: string,
): Promise<MissionCompleteResult> {
  return httpJSON<MissionCompleteResult>('/api/missions/complete', {
    method: 'POST',
    body: JSON.stringify({ id }),
  })
}

/** Set mission blocked with reason (VAL-MISSION-004). */
export async function blockMission(
  id: string,
  reason: string,
): Promise<{ mission: MissionView }> {
  return httpJSON<{ mission: MissionView }>('/api/missions/block', {
    method: 'POST',
    body: JSON.stringify({ id, reason }),
  })
}

/** Activate a pending mission. */
export async function activateMission(
  id: string,
): Promise<{ mission: MissionView }> {
  return httpJSON<{ mission: MissionView }>('/api/missions/activate', {
    method: 'POST',
    body: JSON.stringify({ id }),
  })
}

export async function cancelMission(
  id: string,
): Promise<{ mission: MissionView; missions?: MissionListView }> {
  return httpJSON<{ mission: MissionView; missions?: MissionListView }>(
    '/api/missions/cancel',
    {
      method: 'POST',
      body: JSON.stringify({ id }),
    },
  )
}

export async function deleteMission(
  id: string,
): Promise<{ ok: boolean; missions?: MissionListView }> {
  return httpJSON<{ ok: boolean; missions?: MissionListView }>(
    '/api/missions/delete',
    {
      method: 'POST',
      body: JSON.stringify({ id }),
    },
  )
}

/** Run verification gates (go test, go vet, typecheck/build) — VAL-MISSION-005. */
export async function runMissionGates(
  id: string,
  gates?: string[],
): Promise<{ mission: MissionView }> {
  return httpJSON<{ mission: MissionView }>('/api/missions/gates', {
    method: 'POST',
    body: JSON.stringify({ id, gates }),
  })
}

/** Get one mission by id. */
export async function getMission(id: string): Promise<MissionView> {
  return httpJSON<MissionView>(
    `/api/missions/get?id=${encodeURIComponent(id)}`,
  )
}

/** Orchestrator task row for the Background tasks panel (VAL-ORCH-010). */
export type TaskView = {
  task_id: string
  workspace_id?: string
  category: string
  status: string
  description?: string
  prompt?: string
  summary?: string
  error?: string
  elapsed_ms: number
  started_at?: string
  ended_at?: string
  created_at?: string
}

export type TaskListView = {
  tasks: TaskView[]
  product?: string
}

export type TaskCategoryInfo = {
  name: string
  description: string
  read_only: boolean
  allows_write: boolean
  tools: string[]
  assertions?: string[]
}

/** List orchestrator tasks for the task panel (subscriber only). */
export async function listTasks(): Promise<TaskListView> {
  return httpJSON<TaskListView>('/api/tasks')
}

/** Cancel in-flight task by id (VAL-ORCH-006). */
export async function cancelTask(
  id: string,
): Promise<{ task: TaskView; tasks?: TaskListView }> {
  return httpJSON<{ task: TaskView; tasks?: TaskListView }>(
    '/api/tasks/cancel',
    {
      method: 'POST',
      body: JSON.stringify({ id }),
    },
  )
}

/** Resume cancelled/failed task (VAL-ORCH-007). */
export async function resumeTask(
  id: string,
): Promise<{ task: TaskView; tasks?: TaskListView }> {
  return httpJSON<{ task: TaskView; tasks?: TaskListView }>(
    '/api/tasks/resume',
    {
      method: 'POST',
      body: JSON.stringify({ id }),
    },
  )
}

/** Enumerate task categories (VAL-ORCH-008). */
export async function listTaskCategories(): Promise<{
  categories: TaskCategoryInfo[]
}> {
  return httpJSON<{ categories: TaskCategoryInfo[] }>('/api/tasks/categories')
}

/** BrowserStatus lifecycle snapshot for the desktop panel (VAL-BRW-008). */
export type BrowserStatusView = {
  engine?: string
  state: string
  cdp_url?: string
  cdp_port?: number
  fallback?: boolean
  reason?: string
  url?: string
  detail?: string
  at?: string
  wait_min_ms?: number
  wait_max_ms?: number
}

export type BrowserEngineAvail = {
  engine: string
  ok: boolean
  detail?: string
  install?: string
}

export type BrowserStatusListView = {
  product?: string
  current: BrowserStatusView
  history: BrowserStatusView[]
  engines?: BrowserEngineAvail[]
}

/** GET /api/browser/status — panel subscriber only (no TS browser runtime). */
export async function getBrowserStatus(): Promise<BrowserStatusListView> {
  return httpJSON<BrowserStatusListView>('/api/browser/status')
}

/** POST /api/browser/close — stop active engine via core.Service. */
export async function browserClose(): Promise<BrowserStatusListView> {
  return httpJSON<BrowserStatusListView>('/api/browser/close', {
    method: 'POST',
    body: JSON.stringify({}),
  })
}

/** Replace full todo list (optional; agent tools usually own mutations). */
export async function replaceTodos(todos: TodoItem[]): Promise<TodoListView> {
  return httpJSON<TodoListView>('/api/todos', {
    method: 'POST',
    body: JSON.stringify({ todos }),
  })
}

/** Update one todo status/content (pending → in_progress → completed). */
export async function updateTodo(
  id: string,
  opts: { status?: string; content?: string } = {},
): Promise<{ todo: TodoItem; todos: TodoListView }> {
  return httpJSON<{ todo: TodoItem; todos: TodoListView }>('/api/todos/update', {
    method: 'POST',
    body: JSON.stringify({ id, ...opts }),
  })
}

/**
 * Stream a chat turn via core.Service SSE (POST /api/chat/stream).
 * TokenDelta events update the UI incrementally. Never fetches ai.temp.web.id
 * from the renderer — Go hub is the only outbound LLM path.
 */
export async function streamChat(
  req: ChatRequest,
  onEvent: (ev: ChatEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const url = `${API_BASE}/api/chat/stream`
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
    signal,
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    const msg = (body as { error?: string }).error || res.statusText
    throw new Error(msg || `HTTP ${res.status}`)
  }
  if (!res.body) {
    throw new Error('chat stream: empty body')
  }
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buf = ''
  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buf += decoder.decode(value, { stream: true })
    let sep: number
    while ((sep = buf.indexOf('\n\n')) >= 0) {
      const chunk = buf.slice(0, sep)
      buf = buf.slice(sep + 2)
      const lines = chunk.split('\n')
      for (const line of lines) {
        const trimmed = line.trim()
        if (!trimmed.startsWith('data:')) continue
        const raw = trimmed.slice(5).trim()
        if (!raw || raw === '[DONE]') continue
        try {
          const ev = JSON.parse(raw) as ChatEvent
          onEvent(ev)
        } catch {
          void 0
        }
      }
    }
  }
}

/** Explicit Stop for a detached backend chat run (does not rely on fetch abort). */
export async function cancelChatRun(runId: string): Promise<boolean> {
  if (!runId) return false
  const res = await fetch(`${API_BASE}/api/chat/cancel`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ run_id: runId }),
  })
  if (!res.ok) return false
  const body = (await res.json().catch(() => ({}))) as { cancelled?: boolean }
  return !!body.cancelled
}

/**
 * File change event pushed from the fsnotify watcher when an external process
 * (CLI agent, OS, editor) mutates a file in the active workspace. Used to
 * refresh the desktop explorer via push, not polling (VAL-CROSS-003).
 */
export type FileChangeEvent = {
  /** "write" | "create" | "remove" | "rename" (best-effort) */
  op: string
  /** Workspace-relative slash path (for explorer refresh / select). */
  path: string
  /** Absolute path (for diagnostics; not used by renderer fs ops). */
  absolute?: string
  /** True when the file no longer exists after the event. */
  removed?: boolean
  /** True when the mutated path is a directory. */
  is_dir?: boolean
  /** "external" for CLI/OS writes; "internal" for desktop WriteGateway echoes. */
  source: string
}

/**
 * Subscribe to workspace file change events via SSE (GET /api/fs/events).
 *
 * The desktop explorer subscribes on mount and refreshes the tree on each
 * push event so a file written by the CLI agent appears within ~2s without
 * polling (VAL-CROSS-003). The returned cleanup function closes the stream.
 *
 * Falls back to no-op when EventSource/fetch streaming is unavailable; the
 * explorer remains usable (manual refresh) in that degraded case.
 */
export function streamFileEvents(
  onEvent: (ev: FileChangeEvent) => void,
  signal?: AbortSignal,
): () => void {
  const url = `${API_BASE}/api/fs/events`
  let stopped = false
  let reader: ReadableStreamDefaultReader<Uint8Array> | null = null

  const run = async () => {
    try {
      const res = await fetch(url, { signal })
      if (!res.ok || !res.body) {
        // Degraded: no streaming; explorer falls back to manual refresh.
        return
      }
      reader = res.body.getReader()
      const decoder = new TextDecoder()
      let buf = ''
      while (!stopped) {
        const { done, value } = await reader.read()
        if (done) break
        buf += decoder.decode(value, { stream: true })
        let sep: number
        while ((sep = buf.indexOf('\n\n')) >= 0) {
          const chunk = buf.slice(0, sep)
          buf = buf.slice(sep + 2)
          const lines = chunk.split('\n')
          for (const line of lines) {
            const trimmed = line.trim()
            if (!trimmed.startsWith('data:')) continue
            const raw = trimmed.slice(5).trim()
            if (!raw || raw === '[DONE]') continue
            try {
              onEvent(JSON.parse(raw) as FileChangeEvent)
            } catch {
              // ignore malformed SSE frames
            }
          }
        }
      }
    } catch {
      // Aborted or network error: silently stop. The explorer still works.
    }
  }

  void run()

  return () => {
    stopped = true
    if (reader) {
      try {
        void reader.cancel()
      } catch {
        // ignore
      }
    }
  }
}

// --- Canvas AI generate/edit (D5b) ---

export type CanvasAIMode = 'generate' | 'edit'

export type CanvasAIRequest = {
  prompt: string
  mode?: CanvasAIMode
  existing_elements_json?: string
  profile?: string
  model?: string
}

export type CanvasAIResult = {
  ok: boolean
  elements_json?: string
  raw_text?: string
  mode?: CanvasAIMode
  profile?: string
  model?: string
  error?: string
  latency_ms?: number
}

export async function canvasAI(
  req: CanvasAIRequest,
): Promise<CanvasAIResult> {
  const g = goMain()
  if (g?.CanvasAI) {
    return g.CanvasAI(req)
  }
  return httpJSON<CanvasAIResult>('/api/canvas/ai', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}
