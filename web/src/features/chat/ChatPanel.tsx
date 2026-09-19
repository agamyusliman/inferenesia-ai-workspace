import {
  FormEvent,
  KeyboardEvent,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react'
import {
  Bug,
  CornerDownLeft,
  Flag,
  FolderGit2,
  GitCompareArrows,
  Globe,
  ListChecks,
  ListTodo,
  ListTree,
  MessagesSquare,
  type LucideIcon,
} from 'lucide-react'
import {
  deleteChatMessage,
  enterPlanMode,
  exitPlanMode,
  getChatHistory,
  getLiveBlocks,
  getPendingPlan,
  getPlanMode,
  listMentions,
  resolveApproval,
  setChatHistory,
  cancelChatRun,
  streamChat,
  truncateChatFromHere,
  type ChatEvent,
  type DesktopChatMessage,
  type FileMention,
  type PlanProposalView,
  type TodoListView,
} from '../../lib/api'
import {
  loadCanvases,
  replaceHtmlInMarkdown,
  saveCanvases,
} from '../web-preview/canvasStore'
import {
  deleteMessageById,
  truncateFromMessageId,
} from './messageActions'
import { ConfirmDialog } from '../explorer/ConfirmDialog'
import { PlanProposalDialog } from '../plan/PlanProposalDialog'
import { applyStoppedByUser, composerKeyAction } from './chatStreamUx'
import {
  parseTaskEvent,
  parseToolEvent,
  upsertTaskBlock,
  upsertToolBlock,
  type ChatMeta,
  type TaskBlock,
  type ToolCallBlock,
} from './chatMetaBlocks'
import { ChatMessageCard } from './ChatMessageCard'
import { MentionPicker } from './MentionPicker'
import { ModelPicker } from './ModelPicker'
import { MCPBadge, useMCPStatus } from './MCPBadge'
import { AutonomyPicker } from './AutonomyPicker'
import { SlashCommandPicker } from './SlashCommandPicker'
import {
  applySlashInsert,
  filterSlashCommands,
  matchTrailingSlash,
  type SlashCommand,
} from './slashCommands'
import { generateHandoffSummary } from './handoffSummary'
import { VerticalSplitter } from '../shell/VerticalSplitter'
import { TodoPanel } from '../plan/TodoPanel'
import { MissionPanel } from '../mission/MissionPanel'
import { TaskPanel } from '../task/TaskPanel'
import { BrowserPanel } from '../browser/BrowserPanel'
import { SHELL } from '../shell/shellTokens'
import { PaperclipButton, PreSendAttachmentChips } from './AttachmentChips'
import {
  buildAttachmentPayload,
  buildTokenOrderedImages,
  buildTokenModeInlineText,
  canSendWithAttachments,
  composePromptWithAttachments,
  detectRemovedTokens,
  extractClipboardImages,
  findTokenAtCaret,
  insertTextAtCursor,
  parseImageTokens,
  processAttachmentFile,
  removeAttachmentById,
  renumberImageTokens,
  stripBrokenTokenFragments,
  type BubbleAttachmentChip,
  type ChatAttachment,
} from './chatAttachments'
import {
  IMAGE_GEN_TOGGLE_HINT,
  capImageGenRefs,
  imageGenDegradeContent,
  isImageGenUnsupported,
  promptHasImageSlash,
  shouldRequestImageGen,
  stripImageSlashPrefix,
} from './chatImageGen'
import { canAutoContinue, ephemeralContinueRequest } from './chatAutoContinue'
import { shouldAutoScroll, updateScrollLock } from './chatSmartScroll'
import {
  isRunStale,
  modelPickerDisabledWhileStreaming,
  nextGenerationToken,
  shouldRunAutoContinue,
  stickyFromActive,
  stickyRequestFields,
  workspaceSwitchChatReset,
  type StickySession,
} from './chatReliability'
import type { ActiveSessionView } from '../../lib/api'
import { useLocale } from '../i18n/LocaleProvider'
import type { MessageKey } from '../i18n/messages'

type ApprovalPrompt = {
  id: string
  kind: string
  summary: string
  detail?: string
  force?: boolean
}

export type ChatMessage = {
  id: string
  role: 'user' | 'assistant' | 'system'
  content: string
  mentions?: string[]
  /** Post-send paperclip chips (VAL-CHAT-024). */
  attachments?: BubbleAttachmentChip[]
  streaming?: boolean
  /** User stopped mid-stream via Stop or Esc (VAL-CHAT-003/004). */
  stopped?: boolean
  /** Failed turn — error bubble (VAL-CHAT-028). */
  error?: boolean
  errorText?: string
  /** Auto-continue follow-up in flight (VAL-CHAT-026). */
  continuing?: boolean
  /** Resolved model/profile from Done event (VAL-PROV-005). */
  model?: string
  profile?: string
  host?: string
  /** Tokens / latency / stream status (VAL-CHAT-012). */
  meta?: ChatMeta
  /** Reasoning content separate from final answer (VAL-CHAT-013). */
  thinking?: string
  /** Tool call blocks with status (VAL-CHAT-014). */
  tools?: ToolCallBlock[]
  /** Subagent/task progress blocks (VAL-CHAT-015). */
  tasks?: TaskBlock[]
}

type Props = {
  /** Active workspace id — loads durable thread from core.Service (VAL-DESK-014). */
  workspaceId?: string
  chatContext?: string
  needsSession?: boolean
  onEnsureSession?: (firstUserMessage?: string) => Promise<string | undefined>
  onCreateNewSession?: () => Promise<string | undefined>
  onMaybeTitleSession?: (workspaceId: string, sourceText: string) => void
  onFileChanged?: (path?: string) => void
  onUndoStackChanged?: (canUndo: boolean, canRedo: boolean) => void
  onPlanModeChange?: (active: boolean) => void
  onPlanProposed?: (plan: PlanProposalView) => void
  onPlanDecided?: (kind: 'approve' | 'reject', todos?: TodoListView) => void
  onTodoUpdated?: () => void
  onMissionStatus?: () => void
  onTaskEvent?: () => void
  onBrowserStatus?: () => void
  onOpenWebPreview?: (seed: import('../web-preview/webPreview').WebPreviewSeed) => void
  htmlSyncNonce?: number
  todoRefreshKey?: number
  missionRefreshKey?: number
  taskRefreshKey?: number
  browserRefreshKey?: number
}

type ChatSidePanel = 'todos' | 'missions' | 'tasks' | 'browser' | null

let msgSeq = 0
function nextId(prefix: string) {
  msgSeq += 1
  return `${prefix}-${msgSeq}-${Date.now()}`
}

/**
 * Empty-state starter prompts. Draft insertion only — never auto-submits.
 * Folder workspaces get repo-aware starters; session chat gets generic ones so
 * the copy never implies filesystem access the agent does not have.
 */
type ChatStarter = {
  id: string
  labelKey: MessageKey
  promptKey: MessageKey
  Icon: LucideIcon
}

const FOLDER_STARTERS: ChatStarter[] = [
  {
    id: 'explain-repo',
    labelKey: 'chat.starterExplainRepoLabel',
    promptKey: 'chat.starterExplainRepoPrompt',
    Icon: FolderGit2,
  },
  {
    id: 'review-diff',
    labelKey: 'chat.starterReviewDiffLabel',
    promptKey: 'chat.starterReviewDiffPrompt',
    Icon: GitCompareArrows,
  },
]

const SESSION_STARTERS: ChatStarter[] = [
  {
    id: 'plan-build',
    labelKey: 'chat.starterPlanLabel',
    promptKey: 'chat.starterPlanPrompt',
    Icon: ListChecks,
  },
  {
    id: 'debug-error',
    labelKey: 'chat.starterDebugLabel',
    promptKey: 'chat.starterDebugPrompt',
    Icon: Bug,
  },
]

export function ChatPanel({
  workspaceId,
  chatContext,
  needsSession = false,
  onEnsureSession,
  onCreateNewSession,
  onMaybeTitleSession,
  onFileChanged,
  onUndoStackChanged,
  onPlanModeChange,
  onPlanProposed,
  onPlanDecided,
  onTodoUpdated,
  onMissionStatus,
  onTaskEvent,
  onBrowserStatus,
  onOpenWebPreview,
  htmlSyncNonce = 0,
  todoRefreshKey = 0,
  missionRefreshKey = 0,
  taskRefreshKey = 0,
  browserRefreshKey = 0,
}: Props) {
  const { t } = useLocale()
  const mcpStatus = useMCPStatus()
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [historyLoading, setHistoryLoading] = useState(false)
  const [input, setInput] = useState('')
  const inputRef = useRef(input)
  useEffect(() => {
    inputRef.current = input
  }, [input])
  const [mentions, setMentions] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [sidePanel, setSidePanel] = useState<ChatSidePanel>(null)
  const [error, setError] = useState<string | null>(null)
  const [pickerOpen, setPickerOpen] = useState(false)
  const [pickerQuery, setPickerQuery] = useState('')
  const [pickerItems, setPickerItems] = useState<FileMention[]>([])
  const [pickerLoading, setPickerLoading] = useState(false)
  const [slashOpen, setSlashOpen] = useState(false)
  const [slashQuery, setSlashQuery] = useState('')
  const [slashIndex, setSlashIndex] = useState(0)
  const [sidePanelWidth, setSidePanelWidth] = useState(288)
  const [activeLabel, setActiveLabel] = useState('')
  const [approval, setApproval] = useState<ApprovalPrompt | null>(null)
  const [approvalBusy, setApprovalBusy] = useState(false)
  const [handoffBusy, setHandoffBusy] = useState(false)
  /** Plan Mode read-only indicator (VAL-PLAN-001). */
  const [planMode, setPlanMode] = useState(false)
  const [planBusy, setPlanBusy] = useState(false)
  /** Pending PlanProposed dialog (VAL-PLAN-003/004). */
  const [proposal, setProposal] = useState<PlanProposalView | null>(null)
  const abortRef = useRef<AbortController | null>(null)
  const runIdRef = useRef<string | null>(null)
  /** In-flight assistant id so Stop/Esc can apply _(stopped by user)_ (VAL-CHAT-003/004). */
  const streamingAssistantIdRef = useRef<string | null>(null)
  const listRef = useRef<HTMLDivElement | null>(null)
  const taRef = useRef<HTMLTextAreaElement | null>(null)
  const loadGen = useRef(0)
  const preserveNextBindRef = useRef(false)
  const pendingHandoffRef = useRef<string | null>(null)
  const prevWorkspaceIdRef = useRef<string | undefined>(workspaceId)
  const sendRef = useRef<((text?: string) => Promise<void>) | null>(null)
  /**
   * Generation token: bumped on Stop/Esc and workspace switch so abandoned
   * promises cannot re-lock busy (VAL-CHATBUG-003..005, VAL-CHATBUG-008).
   */
  const genTokenRef = useRef(0)
  /**
   * Sticky mid-session model/profile for the next send (VAL-CHATBUG-001/002).
   * Updated by ModelPicker; pinned on streamChat so the turn uses the chosen route.
   */
  const stickyRef = useRef<StickySession>({})
  /** Confirm only for bulk-dangerous undo-from-here (VAL-CHAT-017). */
  const [truncateTarget, setTruncateTarget] = useState<{
    id: string
    preview: string
  } | null>(null)
  const [truncateStage, setTruncateStage] = useState<{
    removed: ChatMessage[]
    snapshotBefore: ChatMessage[]
    seedText: string
  } | null>(null)
  const [actionBusy, setActionBusy] = useState(false)
  const [actionFeedback, setActionFeedback] = useState<string | null>(null)
  /** Pre-send paperclip attachments (VAL-CHAT-021..024). */
  const [attachments, setAttachments] = useState<ChatAttachment[]>([])
  const attachInputRef = useRef<HTMLInputElement | null>(null)
  /** Image gen toggle → generate_image on stream (VAL-CHAT-025). */
  const [imageGenOn, setImageGenOn] = useState(false)
  const imageGenOnceRef = useRef(false)
  const [liveBlocksOn, setLiveBlocksOn] = useState(false)
  /**
   * Non-blocking gateway-blocked toast (VAL-CROSS-009): emitted when the
   * temp-ai gateway is unreachable or returns 401/5xx. Brand-correct message
   * ("Inferenesia: gateway unavailable"), never the API key. User can retry or
   * switch profile; the toast does not block the composer.
   */
  const [gatewayBlocked, setGatewayBlocked] = useState<{
    message: string
    detail?: string
    host?: string
    at: number
  } | null>(null)
  /** Smart scroll: lock when user scrolls up (VAL-CHAT-029). */
  const scrollLockedRef = useRef(false)
  /** Auto-continue rounds used for the active assistant turn (VAL-CHAT-026). */
  const continueRoundsRef = useRef(0)
  /** User-requested stop cancels further auto-continue rounds. */
  const stopContinueRef = useRef(false)

  const mapHistoryMessages = useCallback(
    (
      msgs: {
        id?: string
        role: string
        content?: string
        mentions?: string[]
        meta?: import('../../lib/api').DesktopChatMeta
        tools?: import('../../lib/api').DesktopToolBlock[]
        tasks?: import('../../lib/api').DesktopTaskBlock[]
        thinking?: string
        model?: string
        profile?: string
        host?: string
      }[],
      wid: string,
    ) =>
      (msgs || [])
        .filter((m) => m.role === 'user' || m.role === 'assistant')
        .map((m, i) => {
          const restored: ChatMessage = {
            id: m.id || `hist-${i}-${wid}`,
            role: m.role as 'user' | 'assistant',
            content: m.content || '',
            mentions: m.mentions,
          }
          // Rehydrate persisted per-message meta + tool/task blocks so a
          // reloaded assistant message shows its ChatMetaRow + tool/task cards,
          // not only a bare stream-status done (misc-chat-meta-persist-history).
          if (m.role === 'assistant') {
            if (m.meta) {
              restored.meta = {
                tokens: m.meta.tokens,
                tokensPrompt: m.meta.tokens_prompt,
                tokensCompletion: m.meta.tokens_completion,
                latencyMs: m.meta.latency_ms,
                streamStatus: m.meta.stream_status,
                mode: m.meta.mode,
                profile: m.meta.profile || m.profile,
                model: m.meta.model || m.model,
              }
            } else if (m.model || m.profile) {
              restored.meta = {
                profile: m.profile,
                model: m.model,
                streamStatus: 'done',
              }
            }
            if (m.tools && m.tools.length > 0) {
              restored.tools = m.tools.map((t) => ({
                id: t.id || `${t.name || 'tool'}-${t.status || 'done'}`,
                name: t.name || 'tool',
                status: (t.status as ToolCallBlock['status']) || 'done',
                detail: t.detail,
                result: t.result,
              }))
            }
            if (m.tasks && m.tasks.length > 0) {
              restored.tasks = m.tasks.map((tk) => ({
                id: tk.id || 'task',
                category: tk.category,
                status: (tk.status as TaskBlock['status']) || 'completed',
                label: tk.label,
                detail: tk.detail,
              }))
            }
            if (m.thinking) {
              restored.thinking = m.thinking
            }
            if (m.model) restored.model = m.model
            if (m.profile) restored.profile = m.profile
            if (m.host) restored.host = m.host
          }
          return restored
        })
        // Hide empty assistant shells (tool-round placeholders without text/tools/thinking).
        .filter((m) => {
          if (m.role !== 'assistant') return true
          if ((m.content || '').trim()) return true
          if ((m.thinking || '').trim()) return true
          if (m.tools && m.tools.length > 0) return true
          if (m.tasks && m.tasks.length > 0) return true
          if (m.streaming || m.stopped || m.error) return true
          return false
        }),
    [],
  )

  const flashAction = useCallback((text: string) => {
    setActionFeedback(text)
    window.setTimeout(() => setActionFeedback(null), 2200)
  }, [])

  /**
   * Durable store keys history rows as m-0..m-N. Live UI may use nextId keys
   * (u-… / a-…). Resolve the store id by position so delete/truncate hit SQLite.
   */
  const durableMessageId = useCallback(
    (messageId: string, list: ChatMessage[]) => {
      const idx = list.findIndex((m) => m.id === messageId)
      if (idx >= 0) return `m-${idx}`
      return messageId
    },
    [],
  )

  const handleDeleteMessage = useCallback(
    async (messageId: string) => {
      if (!workspaceId || actionBusy || busy) return
      setActionBusy(true)
      setError(null)
      const storeId = durableMessageId(messageId, messages)
      // Optimistic UI; durable store via core.Service.
      setMessages((prev) => deleteMessageById(prev, messageId))
      try {
        const res = await deleteChatMessage(workspaceId, storeId)
        setMessages(mapHistoryMessages(res.messages || [], workspaceId))
        flashAction(res.message || (res.removed > 0 ? 'Message deleted' : 'Nothing to delete'))
      } catch (e) {
        // Reload history on failure.
        try {
          const hist = await getChatHistory(workspaceId)
          setMessages(mapHistoryMessages(hist.messages || [], workspaceId))
        } catch {
          /* ignore */
        }
        setError(e instanceof Error ? e.message : String(e))
      } finally {
        setActionBusy(false)
      }
    },
    [
      workspaceId,
      actionBusy,
      busy,
      messages,
      durableMessageId,
      mapHistoryMessages,
      flashAction,
    ],
  )

  const requestUndoFromHere = useCallback(
    (messageId: string) => {
      if (!workspaceId || actionBusy || busy) return
      const idx = messages.findIndex((m) => m.id === messageId)
      const count = idx >= 0 ? messages.length - idx : 0
      setTruncateTarget({
        id: messageId,
        preview:
          count > 1
            ? `Remove this message and ${count - 1} after it from the workspace thread? This cannot be undone.`
            : `Remove this message from the workspace thread? This cannot be undone.`,
      })
    },
    [workspaceId, actionBusy, busy, messages],
  )

  const confirmUndoFromHere = useCallback(async () => {
    if (!workspaceId || !truncateTarget || actionBusy) return
    const messageId = truncateTarget.id
    const idx = messages.findIndex((m) => m.id === messageId)
    const removed = idx >= 0 ? messages.slice(idx) : []
    const snapshotBefore = messages.slice()
    const seedMsg =
      [...removed].reverse().find((m) => m.role === 'user') ||
      removed.find((m) => m.role === 'user') ||
      removed[0]
    const seedText = (seedMsg?.content || '').trim()
    const storeId = durableMessageId(messageId, messages)
    setTruncateTarget(null)
    setActionBusy(true)
    setError(null)
    setMessages((prev) => truncateFromMessageId(prev, messageId))
    if (removed.length > 0) {
      setTruncateStage({
        removed,
        snapshotBefore,
        seedText,
      })
      if (seedText && !input.trim()) {
        setInput(seedText)
      }
    }
    try {
      const res = await truncateChatFromHere(workspaceId, storeId)
      setMessages(mapHistoryMessages(res.messages || [], workspaceId))
      flashAction(
        res.message ||
          (res.removed > 0
            ? `Removed ${res.removed} message(s) — restore or resend below`
            : 'Nothing to truncate'),
      )
    } catch (e) {
      try {
        const hist = await getChatHistory(workspaceId)
        setMessages(mapHistoryMessages(hist.messages || [], workspaceId))
      } catch {
        /* ignore */
      }
      setTruncateStage(null)
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setActionBusy(false)
    }
  }, [
    workspaceId,
    truncateTarget,
    actionBusy,
    messages,
    durableMessageId,
    mapHistoryMessages,
    flashAction,
    input,
  ])

  const dismissTruncateStage = useCallback(() => {
    setTruncateStage(null)
  }, [])

  const restoreTruncateStage = useCallback(async () => {
    if (!workspaceId || !truncateStage || actionBusy) return
    setActionBusy(true)
    setError(null)
    try {
      const payload: DesktopChatMessage[] = truncateStage.snapshotBefore.map(
        (m, i) => ({
          id: `m-${i}`,
          role: m.role,
          content: m.content || '',
        }),
      )
      const res = await setChatHistory(workspaceId, payload)
      setMessages(mapHistoryMessages(res.messages || payload, workspaceId))
      setTruncateStage(null)
      flashAction('Chat restored')
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setActionBusy(false)
    }
  }, [
    workspaceId,
    truncateStage,
    actionBusy,
    mapHistoryMessages,
    flashAction,
  ])

  const resendTruncateStage = useCallback(() => {
    if (!truncateStage) return
    const text = truncateStage.seedText
    if (text) setInput(text)
    setTruncateStage(null)
    window.setTimeout(() => {
      taRef.current?.focus()
    }, 0)
  }, [truncateStage])

  const stopGeneration = useCallback(() => {
    stopContinueRef.current = true
    genTokenRef.current = nextGenerationToken(genTokenRef.current)
    const id = streamingAssistantIdRef.current
    streamingAssistantIdRef.current = null
    const runId = runIdRef.current
    runIdRef.current = null
    if (id) {
      setMessages((prev) =>
        prev.map((m) =>
          m.id === id
            ? {
                ...m,
                streaming: false,
                stopped: true,
                continuing: false,
                content: applyStoppedByUser(m.content),
                meta: { ...m.meta, streamStatus: 'stopped' },
              }
            : m,
        ),
      )
    }
    setBusy(false)
    if (runId) {
      void cancelChatRun(runId)
    }
    abortRef.current?.abort()
    abortRef.current = null
  }, [])

  // Esc stops generation only when focus is inside the chat panel (VAL-CHAT-004).
  // Do NOT listen on window — Esc in explorer/settings/browser/layout would false-stop.
  useEffect(() => {
    if (!busy) return
    const onEsc = (e: globalThis.KeyboardEvent) => {
      if (e.key !== 'Escape') return
      if (proposal || approval || truncateTarget) return
      const t = e.target as HTMLElement | null
      const inChat = !!t?.closest?.('[data-testid="chat-panel"]')
      if (!inChat) return
      // Ignore Esc that is closing nested pickers/menus first.
      if (t?.closest?.('[role="dialog"], [role="listbox"], [role="menu"]')) return
      e.preventDefault()
      stopGeneration()
    }
    window.addEventListener('keydown', onEsc, true)
    return () => window.removeEventListener('keydown', onEsc, true)
  }, [busy, stopGeneration, proposal, approval, truncateTarget])

  useEffect(() => {
    let cancelled = false
    void getPlanMode()
      .then((st) => {
        if (!cancelled) {
          setPlanMode(!!st.active)
          onPlanModeChange?.(!!st.active)
        }
      })
      .catch(() => {
        /* hub may be starting */
      })
    return () => {
      cancelled = true
    }
  }, [onPlanModeChange])

  // Live Blocks opt-in toggle (P9-live): load once on mount and on window focus
  // so a Settings toggle is picked up without a full reload.
  useEffect(() => {
    let cancelled = false
    const load = () => {
      void getLiveBlocks()
        .then((v) => {
          if (!cancelled) setLiveBlocksOn(!!v.enabled)
        })
        .catch(() => {
          /* hub may be starting */
        })
    }
    load()
    const onFocus = () => load()
    window.addEventListener('focus', onFocus)
    return () => {
      cancelled = true
      window.removeEventListener('focus', onFocus)
    }
  }, [])

  // Recover / surface pending PlanProposed when dialog is closed but a plan is waiting
  // (hub propose, page reload, or SSE lag). Poll lightly so Approve/Reject can open
  // without requiring a live LLM stream for agent-browser (VAL-PLAN-003/004).
  const lastNotifiedPlanId = useRef<string | null>(null)
  useEffect(() => {
    let cancelled = false
    const pull = async () => {
      try {
        const p = await getPendingPlan()
        if (cancelled) return
        if (!p) return
        setProposal((prev) => (prev?.id === p.id ? prev : p))
        if (lastNotifiedPlanId.current !== p.id) {
          lastNotifiedPlanId.current = p.id
          onPlanProposed?.(p)
        }
      } catch {
        /* hub may be starting */
      }
    }
    void pull()
    const id = window.setInterval(() => {
      void pull()
    }, 1500)
    return () => {
      cancelled = true
      window.clearInterval(id)
    }
  }, [onPlanProposed])

  const togglePlanMode = useCallback(async () => {
    if (planBusy) return
    setPlanBusy(true)
    setError(null)
    try {
      if (planMode) {
        const res = await exitPlanMode('cancel')
        setPlanMode(!!res.plan?.active)
        onPlanModeChange?.(!!res.plan?.active)
      } else {
        const res = await enterPlanMode('user')
        setPlanMode(!!res.plan?.active)
        onPlanModeChange?.(!!res.plan?.active)
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setPlanBusy(false)
    }
  }, [planBusy, planMode, onPlanModeChange])

  const onActiveChange = useCallback((label: string, view?: ActiveSessionView) => {
    setActiveLabel(label)
    // Sticky session for next send (VAL-CHATBUG-001/002).
    if (view) {
      stickyRef.current = stickyFromActive(view)
    } else if (label) {
      // Fallback: parse "name · model" when view omitted.
      const parts = label.split('·').map((p) => p.trim())
      stickyRef.current = {
        ...stickyRef.current,
        label,
        ...(parts[1] ? { model: parts[1] } : {}),
      }
    }
  }, [])

  // Track user scroll position → lock/unlock auto-scroll (VAL-CHAT-029).
  useEffect(() => {
    const el = listRef.current
    if (!el) return
    const onScroll = () => {
      const metrics = {
        scrollTop: el.scrollTop,
        scrollHeight: el.scrollHeight,
        clientHeight: el.clientHeight,
      }
      scrollLockedRef.current = updateScrollLock(metrics, scrollLockedRef.current)
    }
    el.addEventListener('scroll', onScroll, { passive: true })
    return () => el.removeEventListener('scroll', onScroll)
  }, [])

  // Auto-scroll to latest only when unlocked (VAL-CHAT-029).
  useEffect(() => {
    if (!shouldAutoScroll(scrollLockedRef.current)) return
    const el = listRef.current
    if (!el) return
    el.scrollTo({ top: el.scrollHeight })
  }, [messages])

  useEffect(() => {
    if (!needsSession) return
    const id = window.setTimeout(() => {
      taRef.current?.focus()
    }, 0)
    return () => window.clearTimeout(id)
  }, [needsSession])

  useEffect(() => {
    const gen = ++loadGen.current

    if (workspaceId && preserveNextBindRef.current) {
      preserveNextBindRef.current = false
      setHistoryLoading(false)
      return
    }

    const reset = workspaceSwitchChatReset(genTokenRef.current)
    genTokenRef.current = reset.nextToken
    abortRef.current?.abort()
    abortRef.current = null
    runIdRef.current = null
    streamingAssistantIdRef.current = reset.streamingAssistantId
    setBusy(reset.busy)
    setError(null)
    setInput('')
    setMentions([])
    setAttachments([])
    setPickerOpen(false)
    setMessages([])
    setTruncateTarget(null)
    setTruncateStage(null)
    setProposal(null)
    setApproval(null)
    setActionFeedback(null)

    if (!workspaceId) {
      setHistoryLoading(false)
      const id = window.setTimeout(() => {
        taRef.current?.focus()
      }, 0)
      return () => window.clearTimeout(id)
    }

    setHistoryLoading(true)
    void getChatHistory(workspaceId)
      .then((hist) => {
        if (gen !== loadGen.current) return
        if (hist.workspace_id && hist.workspace_id !== workspaceId) return
        setMessages(mapHistoryMessages(hist.messages || [], workspaceId))
      })
      .catch(() => {
        if (gen !== loadGen.current) return
        setMessages([])
      })
      .finally(() => {
        if (gen === loadGen.current) setHistoryLoading(false)
      })
  }, [workspaceId, mapHistoryMessages])

  useEffect(() => {
    if (
      pendingHandoffRef.current &&
      workspaceId &&
      workspaceId !== prevWorkspaceIdRef.current
    ) {
      const text = pendingHandoffRef.current
      pendingHandoffRef.current = null
      prevWorkspaceIdRef.current = workspaceId
      setInput(text)
      window.setTimeout(() => {
        void sendRef.current?.(text)
      }, 100)
      return
    }
    prevWorkspaceIdRef.current = workspaceId
  }, [workspaceId])

  useEffect(() => {
    if (!htmlSyncNonce || !workspaceId) return
    const list = loadCanvases(workspaceId)
    const linked = list.filter((c) => c.chatMessageId && c.sourceHtml)
    if (linked.length === 0) return

    setMessages((prev) => {
      let changed = false
      const syncedCanvasIds = new Set<string>()
      const next = prev.map((m) => {
        let content = m.content || ''
        let hit = false
        for (const c of linked) {
          if (c.chatMessageId !== m.id || !c.sourceHtml) continue
          if (c.sourceHtml === c.html) continue
          const replaced = replaceHtmlInMarkdown(content, c.sourceHtml, c.html)
          if (replaced != null) {
            content = replaced
            hit = true
            syncedCanvasIds.add(c.id)
          }
        }
        if (!hit) return m
        changed = true
        return { ...m, content }
      })
      if (!changed) return prev

      if (syncedCanvasIds.size > 0) {
        const canvases = loadCanvases(workspaceId).map((c) =>
          syncedCanvasIds.has(c.id) ? { ...c, sourceHtml: c.html } : c,
        )
        saveCanvases(workspaceId, canvases)
      }

      const payload: DesktopChatMessage[] = next
        .filter((m) => m.role === 'user' || m.role === 'assistant')
        .map((m) => ({
          id: m.id,
          role: m.role,
          content: m.content || '',
          mentions: m.mentions,
        }))
      void setChatHistory(workspaceId, payload).catch(() => undefined)
      return next
    })
  }, [htmlSyncNonce, workspaceId])

  // Load mention candidates when @ picker is open.
  useEffect(() => {
    if (!pickerOpen) return
    let cancelled = false
    setPickerLoading(true)
    void listMentions(pickerQuery, 60)
      .then((items) => {
        if (!cancelled) setPickerItems(items)
      })
      .catch(() => {
        if (!cancelled) setPickerItems([])
      })
      .finally(() => {
        if (!cancelled) setPickerLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [pickerOpen, pickerQuery])

  const mentionDetectTimer = useRef<number | null>(null)
  const slashItems = slashOpen ? filterSlashCommands(slashQuery) : []

  const onInputChange = useCallback((value: string) => {
    const prevInput = inputRef.current
    inputRef.current = value

    const removed = detectRemovedTokens(prevInput, value)
    if (removed.length > 0) {
      setAttachments((prev) => {
        let imgIdx = 0
        const toRemove = new Set(removed)
        const removeIds = new Set<string>()
        for (const a of prev) {
          if (a.kind === 'image') {
            imgIdx += 1
            if (toRemove.has(imgIdx)) removeIds.add(a.id)
          }
        }
        if (removeIds.size === 0) return prev
        return prev.filter((a) => !removeIds.has(a.id))
      })
    }

    const cleaned = stripBrokenTokenFragments(value)
    const renumbered = renumberImageTokens(cleaned)
    if (renumbered !== value) {
      inputRef.current = renumbered
      setInput(renumbered)
    } else {
      setInput(value)
    }

    if (mentionDetectTimer.current != null) {
      window.clearTimeout(mentionDetectTimer.current)
    }
    mentionDetectTimer.current = window.setTimeout(() => {
      const at = (renumbered || value).match(/(?:^|\s)@([^\s@]*)$/)
      if (at) {
        setPickerOpen(true)
        setPickerQuery(at[1] || '')
        setSlashOpen(false)
        setSlashQuery('')
        return
      }
      setPickerOpen(false)
      setPickerQuery('')
      const slash = matchTrailingSlash(renumbered || value)
      if (slash) {
        setSlashOpen(true)
        setSlashQuery(slash.query)
        setSlashIndex(0)
      } else {
        setSlashOpen(false)
        setSlashQuery('')
      }
    }, 60)
  }, [])

  /**
   * Empty-state starter: seeds the composer draft and focuses it. Never sends —
   * the user still reviews/edits and presses Enter. An existing non-empty draft
   * is left untouched so a half-typed message is not destroyed.
   */
  const applyStarter = useCallback(
    (prompt: string) => {
      if (!inputRef.current.trim()) onInputChange(prompt)
      taRef.current?.focus()
      // Caret to end after React commits the seeded value.
      window.setTimeout(() => {
        const ta = taRef.current
        if (ta) ta.setSelectionRange(ta.value.length, ta.value.length)
      }, 0)
    },
    [onInputChange],
  )

  const attachMention = (file: FileMention) => {
    const path = file.path
    setMentions((prev) => (prev.includes(path) ? prev : [...prev, path]))
    setInput((prev) => prev.replace(/(?:^|\s)@([^\s@]*)$/, (match) => {
      const lead = match.startsWith(' ') || match.startsWith('\n') ? match[0] : ''
      return `${lead}@${path} `
    }))
    setPickerOpen(false)
    setPickerQuery('')
    taRef.current?.focus()
  }

  const applySlashCommand = useCallback(
    (cmd: SlashCommand) => {
      setSlashOpen(false)
      setSlashQuery('')
      if (cmd.action === 'plan') {
        setInput((prev) => applySlashInsert(prev, ''))
        void togglePlanMode()
        taRef.current?.focus()
        return
      }
      if (cmd.action === 'todos' || cmd.action === 'missions' || cmd.action === 'tasks' || cmd.action === 'browser') {
        setInput((prev) => applySlashInsert(prev, ''))
        setSidePanel(cmd.action)
        taRef.current?.focus()
        return
      }
      if (cmd.action === 'image-gen') {
        imageGenOnceRef.current = true
        setInput((prev) => applySlashInsert(prev, cmd.insert || '/image '))
        taRef.current?.focus()
        return
      }
      if (cmd.action === 'handoff') {
        setInput((prev) => applySlashInsert(prev, ''))
        setHandoffBusy(true)
        void generateHandoffSummary({
          messages,
          workspaceId,
        }).then((result) => {
          setHandoffBusy(false)
          if (!result.ok || !result.text) {
            setError(result.error || 'Handoff: no context to summarize.')
            taRef.current?.focus()
            return
          }
          if (!onCreateNewSession) {
            setInput(result.text)
            taRef.current?.focus()
            return
          }
          pendingHandoffRef.current = result.text
          void onCreateNewSession().catch((e) => {
            pendingHandoffRef.current = null
            setError(e instanceof Error ? e.message : String(e))
          })
        })
        return
      }
      if (cmd.action === 'clear') {
        setInput((prev) => applySlashInsert(prev, ''))
        if (!workspaceId) {
          setMessages([])
          taRef.current?.focus()
          return
        }
        void setChatHistory(workspaceId, [])
          .then(() => setMessages([]))
          .catch((e) => setError(e instanceof Error ? e.message : String(e)))
        taRef.current?.focus()
        return
      }
      if (cmd.action === 'compact') {
        setInput((prev) => applySlashInsert(prev, ''))
        if (!workspaceId || messages.length === 0) {
          setError('Compact: no messages to summarize.')
          taRef.current?.focus()
          return
        }
        void generateHandoffSummary({ messages, workspaceId }).then((result) => {
          if (!result.ok || !result.text) {
            setError(result.error || 'Compact: failed to generate summary.')
            taRef.current?.focus()
            return
          }
          const summaryMessage: DesktopChatMessage = {
            id: `compact-${Date.now()}`,
            role: 'user',
            content: result.text,
            meta: { mode: 'compact' as string },
          }
          void setChatHistory(workspaceId, [summaryMessage])
            .then((hist) => {
              setMessages(mapHistoryMessages(hist.messages || [summaryMessage], workspaceId))
              taRef.current?.focus()
            })
            .catch((e) => setError(e instanceof Error ? e.message : String(e)))
        })
        return
      }
      if (cmd.action === 'usage') {
        setInput((prev) => applySlashInsert(prev, ''))
        const totalTokens = messages.reduce((sum, m) => {
          const t = m.meta?.tokens ?? 0
          const p = m.meta?.tokensPrompt ?? 0
          const c = m.meta?.tokensCompletion ?? 0
          return sum + (t || (p + c))
        }, 0)
        const userMsgs = messages.filter((m) => m.role === 'user').length
        const assistantMsgs = messages.filter((m) => m.role === 'assistant').length
        setError(
          `Session usage: ${totalTokens.toLocaleString()} tokens across ${messages.length} messages (${userMsgs} user, ${assistantMsgs} assistant).`,
        )
        taRef.current?.focus()
        return
      }
      if (cmd.action === 'diagnostics') {
        setInput((prev) =>
          applySlashInsert(
            prev,
            'Run diagnostics on this workspace: execute `go vet ./...` and `npm run typecheck` in the web/ directory. Report any errors found.',
          ),
        )
        taRef.current?.focus()
        return
      }
      if (cmd.action === 'init') {
        setInput((prev) =>
          applySlashInsert(
            prev,
            'Initialize AGENTS.md for this workspace: scan the project structure, identify the tech stack, coding conventions, key directories, build commands, and hard invariants. Write a comprehensive AGENTS.md file at the workspace root.',
          ),
        )
        taRef.current?.focus()
        return
      }
      if (cmd.action === 'review') {
        setInput((prev) =>
          applySlashInsert(
            prev,
            'Review recent changes in this workspace: run `git diff` to see uncommitted changes, check for code quality issues, security vulnerabilities, type safety problems, and adherence to existing patterns. Report findings with file paths and line references.',
          ),
        )
        taRef.current?.focus()
        return
      }
      if (cmd.action === 'share') {
        setInput((prev) => applySlashInsert(prev, ''))
        if (messages.length === 0) {
          setError('Share: no messages to export.')
          taRef.current?.focus()
          return
        }
        const lines: string[] = [`# Session Export — ${new Date().toISOString()}`, '']
        for (const msg of messages) {
          if (msg.role === 'user') lines.push(`## User\n\n${msg.content}\n`)
          else if (msg.role === 'assistant') lines.push(`## Assistant\n\n${msg.content}\n`)
        }
        const md = lines.join('\n')
        void navigator.clipboard
          .writeText(md)
          .then(() => setError('Session exported to clipboard as markdown.'))
          .catch(() => setError('Share: clipboard write failed.'))
        taRef.current?.focus()
        return
      }
      if (cmd.action === 'model') {
        setInput((prev) => applySlashInsert(prev, ''))
        const picker = document.querySelector<HTMLElement>('[data-testid="model-picker"]')
        if (picker) picker.click()
        taRef.current?.focus()
        return
      }
      const fallback = cmd.insert || `/${cmd.name} `
      setInput((prev) => applySlashInsert(prev, fallback))
      taRef.current?.focus()
    },
    [togglePlanMode, messages, workspaceId, onCreateNewSession, mapHistoryMessages],
  )

  const removeMention = (path: string) => {
    setMentions((prev) => prev.filter((p) => p !== path))
  }

  const removeAttachment = (id: string) => {
    setAttachments((prev) => removeAttachmentById(prev, id))
  }

  const removeImageAttachmentByTokenIndex = (tokenIndex: number) => {
    if (tokenIndex < 1) return
    setAttachments((prev) => {
      let imgCount = 0
      let targetId: string | null = null
      for (const a of prev) {
        if (a.kind === 'image') {
          imgCount += 1
          if (imgCount === tokenIndex) {
            targetId = a.id
            break
          }
        }
      }
      if (!targetId) return prev
      return removeAttachmentById(prev, targetId)
    })
  }

  const onPickAttachments = useCallback(async (files: FileList | null) => {
    if (!files || files.length === 0) return
    const list = Array.from(files)
    // Placeholder chips while reading (VAL-CHAT-021).
    const placeholders: ChatAttachment[] = list.map((f, i) => ({
      id: `pending-${Date.now()}-${i}`,
      name: f.name || 'file',
      size: f.size || 0,
      mime: f.type || '',
      kind: 'binary',
      loading: true,
    }))
    setAttachments((prev) => [...prev, ...placeholders])
    const results = await Promise.all(list.map((f) => processAttachmentFile(f)))
    setAttachments((prev) => {
      // Drop placeholders we added for this batch (by pending- prefix + position).
      const withoutPending = prev.filter((a) => !a.loading || !a.id.startsWith('pending-'))
      // Also drop the exact placeholders if they were not cleared (race-safe merge).
      const cleaned = withoutPending.filter(
        (a) => !placeholders.some((p) => p.id === a.id),
      )
      return [...cleaned, ...results]
    })
  }, [])

  const onPaste = useCallback(
    async (e: React.ClipboardEvent<HTMLTextAreaElement>) => {
      const files = extractClipboardImages(e.clipboardData)
      if (!files) return

      e.preventDefault()
      const ta = taRef.current
      if (!ta) return

      const imageAttachments = attachments.filter((a) => a.kind === 'image')
      const startIndex = imageAttachments.length + 1

      const placeholders: ChatAttachment[] = files.map((f, i) => ({
        id: `pending-${Date.now()}-${i}`,
        name: f.name || `paste-${startIndex + i}.png`,
        size: f.size || 0,
        mime: f.type || '',
        kind: 'binary',
        loading: true,
      }))
      setAttachments((prev) => [...prev, ...placeholders])

      const results = await Promise.all(files.map((f) => processAttachmentFile(f)))
      setAttachments((prev) => {
        const cleaned = prev.filter((a) => !a.loading || !a.id.startsWith('pending-'))
        const final = cleaned.filter((a) => !placeholders.some((p) => p.id === a.id))
        return [...final, ...results]
      })

      const readyImageCount = results.filter((r) => r.kind === 'image' && !r.error).length
      for (let i = 0; i < readyImageCount; i += 1) {
        const token = `[Image ${startIndex + i}]`
        const { value, caret } = insertTextAtCursor(ta, token)
        setInput(value)
        requestAnimationFrame(() => {
          ta.setSelectionRange(caret, caret)
        })
        await new Promise((r) => setTimeout(r, 0))
      }
    },
    [attachments],
  )

  const send = async (overrideText?: string) => {
    const typed = (overrideText ?? input).trim()
    if (!canSendWithAttachments(typed, attachments, mentions)) return
    // Refuse while any attachment is still loading.
    if (attachments.some((a) => a.loading)) return

    if (busy) {
      stopGeneration()
    }

    // Own this generation; Stop/workspace switch bumps the token to abandon us.
    const myToken = nextGenerationToken(genTokenRef.current)
    genTokenRef.current = myToken

    let boundWorkspaceId = workspaceId
    const createdSessionThisSend = !boundWorkspaceId && !!onEnsureSession
    if (!boundWorkspaceId && onEnsureSession) {
      preserveNextBindRef.current = true
      try {
        boundWorkspaceId = await onEnsureSession()
      } catch (e) {
        preserveNextBindRef.current = false
        setError(e instanceof Error ? e.message : String(e))
        return
      }
      if (!boundWorkspaceId) {
        preserveNextBindRef.current = false
        setError('Could not create a session')
        return
      }
    }

    setError(null)
    setBusy(true)
    setPickerOpen(false)
    stopContinueRef.current = false
    continueRoundsRef.current = 0
    // Clear any prior gateway-blocked toast on a new send so a retry
    // does not leave a stale banner (VAL-CROSS-009).
    setGatewayBlocked(null)
    setTruncateStage(null)
    // New user send → unlock scroll so generation follows (VAL-CHAT-029).
    scrollLockedRef.current = false

    const attachPayload = buildAttachmentPayload(attachments)
    const slashImage =
      imageGenOnceRef.current || promptHasImageSlash(typed)
    imageGenOnceRef.current = false
    const imagePromptBody = stripImageSlashPrefix(typed)
    const userTextForPrompt = slashImage ? imagePromptBody || typed : typed

    const tokens = parseImageTokens(userTextForPrompt)
    let finalInlineText = attachPayload.inlineText
    let finalImages = attachPayload.images
    if (tokens.length > 0) {
      const tokenResult = buildTokenOrderedImages(userTextForPrompt, attachments)
      finalImages = tokenResult.images
      finalInlineText = buildTokenModeInlineText(attachments, tokenResult.usedAttachmentNames)
    }

    const prompt = composePromptWithAttachments(userTextForPrompt, {
      inlineText: finalInlineText,
      images: finalImages,
      bubbleChips: attachPayload.bubbleChips,
    })
    const wantImageGen = shouldRequestImageGen(
      imageGenOn,
      slashImage ? imagePromptBody || prompt || typed : prompt || typed,
      slashImage,
    )
    const displayContent =
      (slashImage ? imagePromptBody || typed : typed) ||
      (attachPayload.bubbleChips.length
        ? `(${attachPayload.bubbleChips.length} attachment${attachPayload.bubbleChips.length > 1 ? 's' : ''})`
        : mentions.length
          ? '(file context only)'
          : '')

    // Pin sticky model/profile for this turn (VAL-CHATBUG-001/002).
    const stickyFields = stickyRequestFields(stickyRef.current)

    const userMsg: ChatMessage = {
      id: nextId('u'),
      role: 'user',
      content: displayContent || '(attachment)',
      mentions: [...mentions],
      attachments: attachPayload.bubbleChips,
    }
    const turnMode = planMode ? 'Plan' : wantImageGen ? 'Image' : 'Agent'
    const assistantId = nextId('a')
    streamingAssistantIdRef.current = assistantId
    setMessages((prev) => [
      ...prev,
      userMsg,
      {
        id: assistantId,
        role: 'assistant',
        content: '',
        streaming: true,
        model: stickyFields.model,
        profile: stickyFields.profile,
        meta: {
          mode: turnMode,
          model: stickyFields.model,
          profile: stickyFields.profile,
          streamStatus: 'generating',
        },
      },
    ])
    setInput('')
    const sentMentions = [...mentions]
    // Cap refs for image gen / vision at 4 (VAL-CHAT-022/025).
    const sentImages = capImageGenRefs(
      finalImages.map((img) => ({
        name: img.name,
        media_type: img.media_type,
        data_url: img.data_url,
      })),
    )
    setMentions([])
    setAttachments([])

    const runStream = async (
      req: Parameters<typeof streamChat>[0],
      opts: { continueRound: boolean },
    ): Promise<{ content: string; stopped: boolean; error: boolean; profile?: string; model?: string }> => {
      // Abandon if Stop/workspace already superseded this send (VAL-CHATBUG-005).
      if (isRunStale(myToken, genTokenRef.current)) {
        return { content: '', stopped: true, error: false }
      }
      const ac = new AbortController()
      abortRef.current = ac
      let lastContent = ''
      let stopped = false
      let errored = false
      let profile: string | undefined
      let model: string | undefined

      const applyEvent = (ev: ChatEvent) => {
        if (ev.type === 'RunStarted' && ev.run_id) {
          runIdRef.current = ev.run_id
        }
        // Drop late events after cancel/workspace switch so UI does not re-stick.
        if (isRunStale(myToken, genTokenRef.current)) return
        if (ev.type === 'TokenDelta' && ev.delta) {
          setMessages((prev) =>
            prev.map((m) => {
              if (m.id !== assistantId) return m
              // Always append deltas (including continue rounds) onto current content.
              const nextContent = (m.content || '') + ev.delta
              lastContent = nextContent
              return {
                ...m,
                content: nextContent,
                streaming: true,
                continuing: opts.continueRound,
              }
            }),
          )
        }
        if (ev.thinking) {
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantId
                ? {
                    ...m,
                    thinking: (m.thinking || '') + ev.thinking,
                    streaming: true,
                    continuing: opts.continueRound,
                  }
                : m,
            ),
          )
        }
        if (ev.type === 'Done') {
          setMessages((prev) =>
            prev.map((m) => {
              if (m.id !== assistantId) return m
              const tokens =
                ev.tokens ??
                (ev.tokens_prompt != null || ev.tokens_completion != null
                  ? (ev.tokens_prompt || 0) + (ev.tokens_completion || 0)
                  : m.meta?.tokens)
              const meta: ChatMeta = {
                ...m.meta,
                tokens,
                tokensPrompt: ev.tokens_prompt ?? m.meta?.tokensPrompt,
                tokensCompletion: ev.tokens_completion ?? m.meta?.tokensCompletion,
                latencyMs: ev.latency_ms ?? m.meta?.latencyMs,
                streamStatus: 'done',
                mode: m.meta?.mode || turnMode,
                profile: ev.profile || m.profile || m.meta?.profile || stickyFields.profile,
                model: ev.model || m.model || m.meta?.model || stickyFields.model,
              }
              // Prefer stream-assembled content; Done.final fills when stream was empty.
              // For continue rounds, append Done.final only when it was not already
              // streamed as TokenDeltas (avoids double text).
              let content = m.content || ''
              const finalText = ev.final || ev.text || ''
              if (finalText) {
                if (!content) {
                  content = finalText
                } else if (finalText.length > 8000 || content.length > 8000) {
                  content = finalText
                } else if (
                  content.includes(finalText) ||
                  content.endsWith(finalText)
                ) {
                  // already have it from deltas
                } else if (opts.continueRound) {
                  content = `${content.trimEnd()}\n\n${finalText}`
                } else {
                  content = finalText
                }
              }
              lastContent = content
              profile = ev.profile || m.profile
              model = ev.model || m.model
              return {
                ...m,
                content,
                streaming: false,
                stopped: false,
                continuing: false,
                error: false,
                model: ev.model || m.model,
                profile: ev.profile || m.profile,
                host: ev.host || m.host,
                thinking: ev.thinking ? (m.thinking || '') + ev.thinking : m.thinking,
                meta,
              }
            }),
          )
          if (ev.model || ev.profile) {
            const label = `${ev.profile || ''}${ev.model ? ` · ${ev.model}` : ''}`
            if (label.trim()) setActiveLabel(label)
            // Keep sticky aligned with Done routing metadata.
            stickyRef.current = stickyFromActive({
              profile: ev.profile || stickyRef.current.profile,
              model: ev.model || stickyRef.current.model,
              profile_name: stickyRef.current.label,
            })
          }
        }
        if (ev.type === 'Cancelled') {
          // Only mark "stopped by user" when this client aborted (Stop button / Esc in chat).
          // Hub may also emit Cancelled on disconnect/restart — treat as interrupted, not user stop.
          const intentional = stopContinueRef.current || ac.signal.aborted
          stopped = true
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantId
                ? {
                    ...m,
                    content: intentional
                      ? applyStoppedByUser(ev.final || ev.text || m.content)
                      : m.content || ev.final || ev.text || '',
                    streaming: false,
                    stopped: intentional,
                    continuing: false,
                    model: ev.model || m.model,
                    profile: ev.profile || m.profile,
                    host: ev.host || m.host,
                    meta: {
                      ...m.meta,
                      latencyMs: ev.latency_ms ?? m.meta?.latencyMs,
                      tokens: ev.tokens ?? m.meta?.tokens,
                      streamStatus: intentional ? 'stopped' : 'error',
                    },
                    error: intentional ? m.error : true,
                    errorText: intentional
                      ? m.errorText
                      : 'Stream interrupted (connection closed). Retry send if needed.',
                  }
                : m,
            ),
          )
        }
        if (ev.type === 'Error') {
          errored = true
          const errText = ev.error || 'chat error'
          const imageFail = wantImageGen || isImageGenUnsupported(errText)
          const visionFail = !imageFail && sentImages.length > 0 && /image input|does not support.*image|vision|clipboard/i.test(errText)
          const friendlyVision = visionFail
            ? 'The selected model does not support image input. Remove the attached images or switch to a vision-capable model.'
            : null
          const bubble = imageFail
            ? imageGenDegradeContent(errText)
            : visionFail
              ? friendlyVision!
              : `(error) ${errText}`
          setError(imageFail ? bubble : visionFail ? friendlyVision! : errText)
          if (imageFail) {
            // Auto-off so the next send is not stuck in Image mode.
            setImageGenOn(false)
          }
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantId
                ? {
                    ...m,
                    // Keep prior content if any; otherwise leave body empty — error bubble owns the text.
                    content: m.content && m.content.trim() && m.content.trim() !== bubble
                      ? m.content
                      : '',
                    error: true,
                    errorText: bubble,
                    streaming: false,
                    continuing: false,
                    meta: {
                      ...m.meta,
                      streamStatus: 'error',
                      mode: imageFail ? 'Image' : m.meta?.mode,
                    },
                  }
                : m,
            ),
          )
        }
        if (ev.type === 'FileChanged') {
          onFileChanged?.(ev.path)
        }
        if (ev.type === 'UndoStackChanged') {
          onUndoStackChanged?.(!!ev.can_undo, !!ev.can_redo)
        }
        if (ev.type === 'ToolStart' || ev.type === 'ToolEnd' || ev.type === 'ToolError') {
          const block = parseToolEvent(ev.type, {
            name: ev.name,
            detail: ev.detail,
            error: ev.error,
          })
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantId
                ? { ...m, tools: upsertToolBlock(m.tools || [], block), streaming: true }
                : m,
            ),
          )
        }
        if (ev.type === 'NeedsApproval' && ev.approval_id) {
          setApproval({
            id: ev.approval_id,
            kind: ev.approval_kind || ev.name || 'git',
            summary: ev.detail || 'Agent git operation requires confirmation',
            detail: ev.text,
            force: !!ev.approval_force,
          })
        }
        if (ev.type === 'PlanProposed') {
          void getPendingPlan()
            .then((p) => {
              if (p) {
                setProposal(p)
                onPlanProposed?.(p)
              }
            })
            .catch(() => {
              /* hub may lag */
            })
        }
        if (ev.type === 'TodoUpdated') {
          onTodoUpdated?.()
          setSidePanel((p) => p ?? 'todos')
        }
        if (ev.type === 'MissionStatus') {
          onMissionStatus?.()
          setSidePanel((p) => p ?? 'missions')
        }
        if (ev.type === 'TaskSpawned' || ev.type === 'TaskDone') {
          const block = parseTaskEvent(ev.type, {
            detail: ev.detail,
            text: ev.text,
            name: ev.name,
          })
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantId
                ? { ...m, tasks: upsertTaskBlock(m.tasks || [], block), streaming: true }
                : m,
            ),
          )
          onTaskEvent?.()
          setSidePanel((p) => p ?? 'tasks')
        }
        if (ev.type === 'BrowserStatus') {
          onBrowserStatus?.()
          setSidePanel((p) => p ?? 'browser')
        }
        // VAL-CROSS-009: gateway-down (tempai unreachable / 401 / 5xx) emits a
        // non-blocking Blocked toast (with a retry/switch-profile hint) so the
        // user is never left waiting on a silent fail or faked success.
        // The error string is brand-correct ("Inferenesia: gateway unavailable")
        // and never includes the API key value.
        if (ev.type === 'Blocked') {
          const blockedText = ev.error || ev.text || 'gateway unavailable'
          setGatewayBlocked({
            message: blockedText,
            detail: ev.detail,
            host: ev.host,
            at: Date.now(),
          })
        }
      }

      try {
        await streamChat(req, applyEvent, ac.signal)
      } catch (e) {
        if (isRunStale(myToken, genTokenRef.current) || (e as Error).name === 'AbortError') {
          stopped = true
          // If still our token, mark the bubble stopped; otherwise stopGeneration already did.
          if (!isRunStale(myToken, genTokenRef.current)) {
            setMessages((prev) =>
              prev.map((m) =>
                m.id === assistantId
                  ? {
                      ...m,
                      streaming: false,
                      stopped: true,
                      continuing: false,
                      content: m.stopped ? m.content : applyStoppedByUser(m.content),
                    }
                  : m,
              ),
            )
          }
        } else {
          errored = true
          const msg = e instanceof Error ? e.message : String(e)
          const imageFail = wantImageGen || isImageGenUnsupported(msg)
          const bubble = imageFail
            ? imageGenDegradeContent(msg)
            : `(error) ${msg}`
          setError(imageFail ? bubble : msg)
          if (imageFail) setImageGenOn(false)
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantId
                ? {
                    ...m,
                    streaming: false,
                    continuing: false,
                    error: true,
                    errorText: bubble,
                    content:
                      m.content && m.content.trim() && m.content.trim() !== bubble
                        ? m.content
                        : '',
                    meta: {
                      ...m.meta,
                      streamStatus: 'error',
                      mode: imageFail ? 'Image' : m.meta?.mode,
                    },
                  }
                : m,
            ),
          )
        }
      } finally {
        if (abortRef.current === ac) {
          abortRef.current = null
        }
      }

      // Re-read content from state via lastContent (sync applyEvent updates).
      return { content: lastContent, stopped, error: errored, profile, model }
    }

    try {
      const first = await runStream(
        {
          prompt: prompt || (sentImages.length ? '[image attachment(s)]' : ''),
          mentions: sentMentions,
          images: sentImages.length ? sentImages : undefined,
          generate_image: wantImageGen || undefined,
          // Image gen is a single turn; tools not used.
          no_tools: wantImageGen || undefined,
          workspace_id: boundWorkspaceId || undefined,
          // Sticky session profile/model for this turn (VAL-CHATBUG-001).
          ...stickyFields,
        },
        { continueRound: false },
      )

      let assistantForTitle = first.content || ''

      // Auto-continue incomplete / fake-tool answers (VAL-CHAT-026 / VAL-CHATBUG-007).
      // Skip for image-gen turns. Ephemeral continue is NOT shown as a user bubble.
      if (
        shouldRunAutoContinue({
          stopContinue: stopContinueRef.current,
          stopped: first.stopped,
          error: first.error,
          wantImageGen,
          stale: isRunStale(myToken, genTokenRef.current),
        })
      ) {
        let content = first.content
        // Prefer sticky + Done routing for continue; session sticky wins on empty.
        let profile = first.profile || stickyFields.profile
        let model = first.model || stickyFields.model
        while (
          shouldRunAutoContinue({
            stopContinue: stopContinueRef.current,
            stopped: false,
            error: false,
            wantImageGen: false,
            stale: isRunStale(myToken, genTokenRef.current),
          }) &&
          canAutoContinue(continueRoundsRef.current, content, {
            stopped: false,
            error: false,
          })
        ) {
          continueRoundsRef.current += 1
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantId
                ? {
                    ...m,
                    streaming: true,
                    continuing: true,
                    meta: { ...m.meta, streamStatus: 'continuing' },
                  }
                : m,
            ),
          )
          const contReq = {
            ...ephemeralContinueRequest({ model, profile }),
            ...stickyRequestFields(stickyRef.current),
            ...(model ? { model } : {}),
            ...(profile ? { profile } : {}),
            workspace_id: boundWorkspaceId || undefined,
          }
          const next = await runStream(contReq, { continueRound: true })
          // Prefer accumulated content from deltas/Done; fall back to prior.
          content = next.content || content
          profile = next.profile || profile
          model = next.model || model
          if (next.stopped || next.error || stopContinueRef.current) break
          if (isRunStale(myToken, genTokenRef.current)) break
        }
        assistantForTitle = content || assistantForTitle
      }

      if (
        createdSessionThisSend &&
        boundWorkspaceId &&
        onMaybeTitleSession &&
        !first.error &&
        !first.stopped &&
        assistantForTitle.trim()
      ) {
        onMaybeTitleSession(boundWorkspaceId, assistantForTitle)
      }
    } finally {
      // Only clear busy / streaming if this generation still owns the UI
      // (Stop already set busy=false; a newer send owns a later token).
      if (!isRunStale(myToken, genTokenRef.current)) {
        setBusy(false)
        streamingAssistantIdRef.current = null
        setMessages((prev) =>
          prev.map((m) =>
            m.id === assistantId
              ? { ...m, streaming: false, continuing: false }
              : m,
          ),
        )
      }
    }
  }

  sendRef.current = async (text?: string) => { void send(text) }

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    void send()
  }

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (slashOpen) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setSlashIndex((i) => Math.min(i + 1, Math.max(0, slashItems.length - 1)))
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        setSlashIndex((i) => Math.max(0, i - 1))
        return
      }
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault()
        const cmd = slashItems[slashIndex] || slashItems[0]
        if (cmd) applySlashCommand(cmd)
        return
      }
      if (e.key === 'Tab') {
        e.preventDefault()
        const cmd = slashItems[slashIndex] || slashItems[0]
        if (cmd) applySlashCommand(cmd)
        return
      }
      if (e.key === 'Escape') {
        e.preventDefault()
        setSlashOpen(false)
        return
      }
    }
    if (e.key === 'Backspace' || e.key === 'Delete') {
      const ta = e.currentTarget
      const caret = ta.selectionStart ?? 0
      const selEnd = ta.selectionEnd ?? 0
      if (caret === selEnd) {
        const dir = e.key === 'Backspace' ? 'backspace' : 'delete'
        const token = findTokenAtCaret(input, caret, dir)
        if (token) {
          e.preventDefault()
          const newValue = input.slice(0, token.start) + input.slice(token.end)
          setInput(newValue)
          const newCaret = token.start
          requestAnimationFrame(() => {
            ta.setSelectionRange(newCaret, newCaret)
          })
          removeImageAttachmentByTokenIndex(token.index)
          return
        }
      }
    }
    const action = composerKeyAction(e.key, {
      shiftKey: e.shiftKey,
      busy,
      pickerOpen: pickerOpen || slashOpen,
    })
    if (action === 'send') {
      e.preventDefault()
      void send()
      return
    }
    if (action === 'stop') {
      e.preventDefault()
      stopGeneration()
      return
    }
    if (e.key === 'Escape' && pickerOpen) {
      setPickerOpen(false)
    }
  }

  const decideApproval = async (approved: boolean) => {
    if (!approval || approvalBusy) return
    setApprovalBusy(true)
    try {
      await resolveApproval(approval.id, approved)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setApproval(null)
      setApprovalBusy(false)
    }
  }

  return (
    <section
      data-testid="chat-panel"
      className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden overflow-x-hidden bg-shell-panel/40"
    >
      {proposal && (
        <PlanProposalDialog
          proposal={proposal}
          onApproved={(todos) => {
            setProposal(null)
            setPlanMode(false)
            onPlanModeChange?.(false)
            onPlanDecided?.('approve', todos)
            onTodoUpdated?.()
            setSidePanel('todos')
          }}
          onRejected={() => {
            setProposal(null)
            setPlanMode(false)
            onPlanModeChange?.(false)
            onPlanDecided?.('reject')
            onTodoUpdated?.()
          }}
          onError={(msg) => setError(msg)}
        />
      )}
      {approval && (
        <div data-testid="needs-approval-dialog" data-kind={approval.kind}>
          <ConfirmDialog
            title={
              approval.force || approval.kind === 'force_push'
                ? 'Force push confirmation'
                : approval.kind === 'reset_hard' || approval.kind === 'branch_delete'
                  ? 'Destructive git operation'
                  : approval.kind === 'commit'
                    ? 'Agent commit confirmation'
                    : 'Confirm agent git action'
            }
            message={
              (approval.summary || 'Approve this git operation?') +
              (approval.detail ? `\n\n${approval.detail}` : '')
            }
            confirmLabel={approvalBusy ? '…' : 'Approve'}
            cancelLabel="Reject"
            danger={
              !!approval.force ||
              approval.kind === 'force_push' ||
              approval.kind === 'reset_hard' ||
              approval.kind === 'branch_delete'
            }
            onConfirm={() => void decideApproval(true)}
            onCancel={() => void decideApproval(false)}
          />
        </div>
      )}
      <div
        data-testid="chat-header"
        className="flex h-10 shrink-0 items-center gap-2 overflow-x-auto border-b border-shell-border bg-shell-panel px-2"
      >
        <h2 className="shrink-0 text-[11px] font-semibold uppercase tracking-wide text-shell-muted">
          {t('chat.title')}
        </h2>
        {(() => {
          const ctx = chatContext || t('chat.noWorkspace')
          const isSessionCtx = /^session\b/i.test(ctx) || /^sesi\b/i.test(ctx)
          const isWorkspaceCtx = /^workspace\b/i.test(ctx)
          const isPick = /pick a session/i.test(ctx)
          const BadgeIcon = isSessionCtx
            ? MessagesSquare
            : isWorkspaceCtx
              ? FolderGit2
              : null
          return (
            <span
              data-testid="chat-context-badge"
              data-kind={
                isSessionCtx
                  ? 'session'
                  : isWorkspaceCtx
                    ? 'workspace'
                    : isPick
                      ? 'pick'
                      : 'other'
              }
              title={ctx}
              className={`inline-flex min-w-[4.5rem] max-w-[12rem] shrink items-center gap-1 rounded border px-1.5 py-0.5 text-[10px] font-medium ${
                isSessionCtx || isWorkspaceCtx
                  ? 'border-shell-border bg-shell-active text-shell-text'
                  : 'border-transparent bg-shell-border/40 text-shell-muted'
              }`}
            >
              {BadgeIcon && (
                <BadgeIcon size={SHELL.iconXs} className="shrink-0 opacity-70" aria-hidden />
              )}
              <span className="truncate">{ctx}</span>
            </span>
          )
        })()}
        <span
          data-testid="chat-active-label"
          className="min-w-0 flex-1 truncate text-[10px] text-shell-muted"
          title={activeLabel || 'Active model'}
        >
          {activeLabel}
        </span>
        {planMode && (
          <span
            data-testid="plan-mode-badge"
            className="shrink-0 rounded bg-amber-500/20 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-amber-300"
            title="Plan Mode active: agent write tools denied"
          >
            Plan Mode
          </span>
        )}
        <button
          type="button"
          data-testid="plan-mode-toggle"
          data-active={planMode ? 'true' : 'false'}
          title={
            planMode
              ? 'Exit Plan Mode (re-enable writes)'
              : 'Enter Plan Mode (read-only explore)'
          }
          disabled={planBusy}
          onClick={() => void togglePlanMode()}
          className={`shrink-0 rounded border px-1.5 py-0.5 text-[10px] font-medium transition disabled:opacity-50 ${
            planMode
              ? 'border-amber-500/50 bg-amber-500/10 text-amber-200 hover:bg-amber-500/20'
              : 'border-shell-border text-shell-muted hover:bg-shell-border/40 hover:text-shell-text'
          }`}
        >
          {planBusy ? '…' : planMode ? 'Exit Plan' : 'Plan'}
        </button>
        {(
          [
            { id: 'todos' as const, label: 'Todos', Icon: ListTodo },
            { id: 'missions' as const, label: 'Missions', Icon: Flag },
            { id: 'tasks' as const, label: 'Tasks', Icon: ListTree },
            { id: 'browser' as const, label: 'Browser', Icon: Globe },
          ] as const
        ).map(({ id, label, Icon }) => {
          const active = sidePanel === id
          return (
            <button
              key={id}
              type="button"
              data-testid={`chat-side-${id}`}
              data-active={active ? 'true' : 'false'}
              title={label}
              aria-label={label}
              aria-pressed={active}
              onClick={() => setSidePanel((p) => (p === id ? null : id))}
              className={`inline-flex h-6 w-6 shrink-0 items-center justify-center rounded border transition ${
                active
                  ? 'border-shell-accent/50 bg-shell-accent/15 text-shell-accent'
                  : 'border-shell-border text-shell-muted hover:bg-shell-border/40 hover:text-shell-text'
              }`}
            >
              <Icon size={SHELL.iconXs} aria-hidden />
            </button>
          )
        })}
      </div>

      <div className="flex min-h-0 min-w-0 flex-1 overflow-hidden">
      <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
      <div
        ref={listRef}
        data-testid="chat-messages"
        className="chat-scroll min-h-0 min-w-0 flex-1 overflow-y-auto overflow-x-hidden px-3 py-2"
      >
      <div className="mx-auto flex min-h-full w-full max-w-3xl flex-col space-y-3">
        {historyLoading && (
          <p data-testid="chat-history-loading" className="text-xs text-shell-muted">
            {t('chat.restoring')}
          </p>
        )}
        {!historyLoading && messages.length === 0 && (
          <div
            data-testid="chat-empty-state"
            data-needs-session={needsSession ? 'true' : 'false'}
            className="flex min-h-[12rem] flex-1 flex-col items-center justify-center gap-3 px-4 py-6 text-center"
          >
            <span
              className="inline-flex h-10 w-10 items-center justify-center rounded-lg border border-shell-border bg-shell-panel text-shell-muted"
              aria-hidden
            >
              <MessagesSquare size={20} />
            </span>
            <div className="space-y-1">
              <h3 className="text-sm font-semibold text-shell-text">
                {needsSession ? t('chat.draftSessionTitle') : t('chat.emptyTitle')}
              </h3>
              <p className="mx-auto max-w-[34rem] text-xs leading-relaxed text-shell-muted">
                {needsSession ? t('chat.draftSessionBody') : t('chat.emptyBody')}
              </p>
            </div>
            {(() => {
              // Folder workspaces get repo-aware starters; session chat stays generic
              // so the copy never implies filesystem access the agent does not have.
              const isFolderWorkspace =
                !needsSession && /^workspace\b/i.test(chatContext || '')
              const starters = isFolderWorkspace ? FOLDER_STARTERS : SESSION_STARTERS
              return (
                <div
                  data-testid="chat-starters"
                  data-scope={isFolderWorkspace ? 'workspace' : 'session'}
                  className="w-full max-w-[34rem] space-y-2"
                >
                  <p className="text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
                    {t('chat.starterHeading')}
                  </p>
                  {/* Flex-wrap, not sm: breakpoints — chat can be a narrow dock at any viewport. */}
                  <div className="flex flex-wrap gap-1.5">
                    {starters.map(({ id, labelKey, promptKey, Icon }) => (
                      <button
                        key={id}
                        type="button"
                        data-testid={`chat-starter-${id}`}
                        onClick={() => applyStarter(t(promptKey))}
                        title={t(promptKey)}
                        className="shell-transition flex min-w-0 flex-1 basis-40 items-center gap-2 rounded-md border border-shell-border bg-shell-panel px-2.5 py-2 text-left text-[12px] text-shell-text hover:border-shell-accent hover:bg-shell-hover"
                      >
                        <Icon size={SHELL.iconSm} className="shrink-0 text-shell-muted" aria-hidden />
                        <span className="truncate">{t(labelKey)}</span>
                      </button>
                    ))}
                  </div>
                  <p className="text-[10px] text-shell-muted">{t('chat.starterHint')}</p>
                </div>
              )
            })()}
          </div>
        )}
        {messages.map((m) => (
          <ChatMessageCard
            key={m.id}
            message={m}
            onDelete={(id) => void handleDeleteMessage(id)}
            onUndoFromHere={requestUndoFromHere}
            liveBlocksEnabled={liveBlocksOn}
            onOpenWebPreview={onOpenWebPreview}
            sessionName={
              (chatContext || '')
                .replace(/^Session\s*[·•\-–—]\s*/i, '')
                .replace(/^Sesi\s*[·•\-–—]\s*/i, '')
                .replace(/^Workspace\s*[·•\-–—]\s*/i, '')
                .trim() || chatContext
            }
            workspaceId={workspaceId}
          />
        ))}
      </div>
      </div>

      {actionFeedback && (
        <div
          data-testid="chat-action-feedback"
          className="border-t border-shell-border bg-shell-panel/80 px-3 py-1 text-[11px] text-emerald-400"
          aria-live="polite"
        >
          {actionFeedback}
        </div>
      )}

      {error && (
        <div data-testid="chat-error" className="border-t border-red-900 px-3 py-1 text-[11px] text-red-300">
          {error}
        </div>
      )}

      {gatewayBlocked && (
        <div
          data-testid="chat-gateway-blocked-toast"
          role="status"
          aria-live="polite"
          className="flex items-start gap-2 border-t border-amber-900/60 bg-amber-950/40 px-3 py-2 text-[11px] text-amber-200"
        >
          <span className="mt-[1px] shrink-0 text-amber-300" aria-hidden="true">
            ⚠
          </span>
          <div className="min-w-0 flex-1">
            <div className="font-medium text-amber-100">{gatewayBlocked.message}</div>
            {(gatewayBlocked.detail || gatewayBlocked.host) && (
              <div className="mt-0.5 truncate text-amber-300/80">
                {gatewayBlocked.detail ? gatewayBlocked.detail : null}
                {gatewayBlocked.host ? ` — host ${gatewayBlocked.host}` : null}
              </div>
            )}
            <div className="mt-1 text-amber-300/70">
              Retry, switch profile, or check TEMP_AI_BASE_URL / TEMP_AI_API_KEY.
            </div>
          </div>
          <button
            type="button"
            data-testid="chat-gateway-blocked-dismiss"
            onClick={() => setGatewayBlocked(null)}
            className="ml-2 shrink-0 rounded px-1.5 py-0.5 text-amber-300 hover:bg-amber-900/40 hover:text-amber-100"
            aria-label="Dismiss gateway error"
          >
            ✕
          </button>
        </div>
      )}

      {truncateTarget && (
        <ConfirmDialog
          title="Undo from here"
          message={truncateTarget.preview}
          confirmLabel="Truncate"
          cancelLabel="Cancel"
          danger
          onConfirm={() => void confirmUndoFromHere()}
          onCancel={() => setTruncateTarget(null)}
        />
      )}

      {truncateStage && (
        <div
          data-testid="chat-truncate-stage"
          className="border-t border-amber-900/50 bg-amber-950/25 px-3 py-2"
        >
          <div className="flex min-w-0 flex-wrap items-start justify-between gap-2">
            <div className="min-w-0 flex-1">
              <div className="text-[10px] font-semibold uppercase tracking-wide text-amber-200/90">
                Truncated · {truncateStage.removed.length} message
                {truncateStage.removed.length === 1 ? '' : 's'}
              </div>
              <p
                className="mt-0.5 line-clamp-2 break-words text-[11px] leading-snug text-shell-text/90"
                title={truncateStage.seedText}
              >
                {truncateStage.seedText ||
                  truncateStage.removed
                    .map((m) => (m.content || '').trim())
                    .filter(Boolean)
                    .slice(0, 1)
                    .join('') ||
                  '(empty)'}
              </p>
            </div>
            <div className="flex shrink-0 flex-wrap items-center gap-1">
              <button
                type="button"
                data-testid="chat-truncate-restore"
                disabled={actionBusy}
                onClick={() => void restoreTruncateStage()}
                className="rounded border border-shell-border bg-shell-bg px-2 py-0.5 text-[10px] text-shell-text hover:bg-shell-border/30 disabled:opacity-40"
              >
                Restore
              </button>
              <button
                type="button"
                data-testid="chat-truncate-resend"
                disabled={actionBusy || !truncateStage.seedText}
                onClick={() => resendTruncateStage()}
                className="rounded border border-shell-accent/50 bg-shell-accent/15 px-2 py-0.5 text-[10px] text-shell-accent hover:bg-shell-accent/25 disabled:opacity-40"
              >
                Edit & resend
              </button>
              <button
                type="button"
                data-testid="chat-truncate-dismiss"
                disabled={actionBusy}
                onClick={() => dismissTruncateStage()}
                className="rounded px-1.5 py-0.5 text-[10px] text-shell-muted hover:bg-shell-border/30 hover:text-shell-text disabled:opacity-40"
              >
                Dismiss
              </button>
            </div>
          </div>
        </div>
      )}

      {mentions.length > 0 && (
        <div data-testid="chat-attached-mentions" className="flex flex-wrap gap-1 border-t border-shell-border px-3 py-1.5">
          {mentions.map((p) => (
            <button
              key={p}
              type="button"
              data-testid="mention-chip"
              onClick={() => removeMention(p)}
              className="rounded bg-shell-accent/20 px-1.5 py-0.5 font-mono text-[10px] text-shell-accent hover:bg-shell-accent/30"
              title="Remove mention"
            >
              @{p}
            </button>
          ))}
        </div>
      )}

      {/* Pre-send paperclip chips with remove (VAL-CHAT-021). */}
      <PreSendAttachmentChips
        attachments={attachments}
        onRemove={removeAttachment}
        disabled={false}
      />

      <form
        data-testid="chat-composer"
        onSubmit={onSubmit}
        className="relative border-t border-shell-border p-2"
      >
        {pickerOpen && (
          <MentionPicker
            items={pickerItems}
            loading={pickerLoading}
            query={pickerQuery}
            onSelect={attachMention}
            onClose={() => setPickerOpen(false)}
          />
        )}
        {slashOpen && (
          <SlashCommandPicker
            items={slashItems}
            query={slashQuery}
            activeIndex={slashIndex}
            onSelect={applySlashCommand}
            onClose={() => setSlashOpen(false)}
            onHoverIndex={setSlashIndex}
          />
        )}
        <div className="mx-auto w-full max-w-3xl">
        <div className="shell-transition rounded-lg border border-shell-border bg-shell-bg p-1.5 focus-within:border-shell-accent focus-within:ring-1 focus-within:ring-shell-accent">
        <label htmlFor="chat-input-textarea" className="sr-only">
          {t('chat.inputLabel')}
        </label>
        <textarea
          ref={taRef}
          id="chat-input-textarea"
          data-testid="chat-input"
          value={input}
          onChange={(e) => {
            onInputChange(e.target.value)
          }}
          onKeyDown={onKeyDown}
          onPaste={onPaste}
          rows={3}
          placeholder={
            busy ? t('chat.placeholderBusy') : t('chat.placeholder')
          }
          aria-busy={busy || undefined}
          className="w-full resize-none border-0 bg-transparent px-1.5 py-1 text-[13px] leading-relaxed text-shell-text placeholder:text-shell-muted focus:outline-none focus-visible:outline-none"
        />
        <div
          data-testid="chat-composer-toolbar"
          className="mt-1.5 flex flex-wrap items-center justify-between gap-2 border-t border-shell-border px-0.5 pt-1.5"
        >
          <div className="flex min-w-0 flex-wrap items-center gap-1.5">
            <PaperclipButton
              inputRef={attachInputRef}
              disabled={false}
              onPick={(files) => void onPickAttachments(files)}
            />
            <button
              type="button"
              data-testid="chat-image-gen-toggle"
              data-active={imageGenOn ? 'true' : 'false'}
              title={IMAGE_GEN_TOGGLE_HINT}
              disabled={false}
              aria-pressed={imageGenOn}
              onClick={() => setImageGenOn((v) => !v)}
              className={`shell-transition shrink-0 rounded-md border px-2 py-1 text-[10px] font-medium disabled:opacity-50 ${
                imageGenOn
                  ? 'border-shell-accent/60 bg-shell-accent/15 text-shell-accent'
                  : 'border-shell-border text-shell-muted hover:bg-shell-border/40 hover:text-shell-text'
              }`}
            >
              {t('chat.imageGen')}
            </button>
            <ModelPicker
              disabled={modelPickerDisabledWhileStreaming(busy)}
              onActiveChange={onActiveChange}
            />
            <span
              className="hidden rounded-md border border-shell-border px-2 py-1 text-[10px] text-shell-muted sm:inline"
              title="Agent mode (tools enabled via core)"
              data-testid="chat-mode-badge"
            >
              Agent
            </span>
            <MCPBadge status={mcpStatus} />
            <AutonomyPicker />
          </div>
          <div className="flex shrink-0 items-center gap-1">
            {handoffBusy && (
              <span className="text-[11px] text-shell-muted animate-pulse">
                Generating handoff…
              </span>
            )}
            {busy && (
              <button
                type="button"
                data-testid="chat-stop"
                onClick={() => stopGeneration()}
                className="shell-transition rounded-md border border-shell-border px-2 py-1 text-[11px] text-shell-muted hover:bg-shell-border/30 hover:text-shell-text"
                title={t('chat.stopTitle')}
              >
                {t('action.stop')}
              </button>
            )}
            <button
              type="submit"
              data-testid="chat-send"
              disabled={!canSendWithAttachments(input, attachments, mentions)}
              title={
                busy
                  ? t('chat.stopTitle')
                  : t('chat.sendTitle')
              }
              className="shell-primary-button inline-flex items-center gap-1.5 rounded-md px-3 py-1 text-[11px] font-semibold disabled:opacity-40"
            >
              <CornerDownLeft size={SHELL.iconXs} aria-hidden />
              {busy && canSendWithAttachments(input, attachments, mentions)
                ? t('chat.interruptSend')
                : t('action.send')}
            </button>
          </div>
        </div>
        </div>
        <p
          data-testid="chat-composer-hint"
          className="mt-1 px-1 text-[10px] text-shell-muted"
        >
          {busy ? t('chat.composerHintBusy') : t('chat.composerHint')}
        </p>
        </div>
      </form>
      </div>

      {sidePanel && (
        <>
          <VerticalSplitter
            testId="splitter-chat-drawer"
            value={sidePanelWidth}
            onChange={(n) =>
              setSidePanelWidth(Math.max(220, Math.min(520, Math.round(n))))
            }
            growSide="right"
            minOpposite={240}
            aria-label="Resize side panel"
          />
          <aside
            data-testid={`chat-drawer-${sidePanel}`}
            style={{ width: sidePanelWidth }}
            className="flex min-h-0 shrink-0 flex-col overflow-hidden border-l border-shell-border bg-shell-bg"
          >
            <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
              {sidePanel === 'todos' && (
                <TodoPanel refreshKey={todoRefreshKey} onClose={() => setSidePanel(null)} />
              )}
              {sidePanel === 'missions' && (
                <MissionPanel
                  refreshKey={missionRefreshKey}
                  workspaceId={workspaceId}
                  onClose={() => setSidePanel(null)}
                />
              )}
              {sidePanel === 'tasks' && (
                <TaskPanel refreshKey={taskRefreshKey} onClose={() => setSidePanel(null)} />
              )}
              {sidePanel === 'browser' && (
                <BrowserPanel
                  refreshKey={browserRefreshKey}
                  onClose={() => setSidePanel(null)}
                />
              )}
            </div>
          </aside>
        </>
      )}
      </div>
    </section>
  )
}
