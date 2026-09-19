import {
  FormEvent,
  useCallback,
  useEffect,
  useRef,
  useState,
  useSyncExternalStore,
} from 'react'
import { streamChat, type ActiveSessionView, type ChatEvent } from '../../lib/api'
import {
  PaperclipButton,
  PreSendAttachmentChips,
} from '../chat/AttachmentChips'
import {
  buildAttachmentPayload,
  canSendWithAttachments,
  composePromptWithAttachments,
  processAttachmentFile,
  removeAttachmentById,
  type ChatAttachment,
} from '../chat/chatAttachments'
import {
  modelPickerDisabledWhileStreaming,
  stickyFromActive,
  stickyRequestFields,
  type StickySession,
} from '../chat/chatReliability'
import { GeneratingLabel } from '../chat/GeneratingLabel'
import { ModelPicker } from '../chat/ModelPicker'
import { useLocale } from '../i18n/LocaleProvider'
import { isMermaidFence } from '../chat/messageChrome'
import {
  clearCanvasAgentJob,
  getCanvasAgentJob,
  setCanvasAgentJob,
  subscribeCanvasAgentJobs,
} from '../web-preview/canvasAgentJobs'
import {
  looksLikePatchResponse,
  resolveCanvasAgentReply,
} from '../web-preview/canvasPatch'
import { resolvePlaygroundScope } from './playgroundScope'

export type DiagramDslHint = {
  id: string
  label: string
  dsl: string
  note?: string
}

export const DIAGRAM_DSL_HINTS: DiagramDslHint[] = [
  { id: 'flowchart', label: 'Flowchart', dsl: 'flowchart', note: 'Flow / DF' },
  {
    id: 'sequence',
    label: 'Sequence',
    dsl: 'sequenceDiagram',
    note: 'API / interaction',
  },
  { id: 'er', label: 'ERD', dsl: 'erDiagram', note: 'Entity relations' },
  {
    id: 'state',
    label: 'State',
    dsl: 'stateDiagram-v2',
    note: 'Lifecycle',
  },
  {
    id: 'class',
    label: 'Class',
    dsl: 'classDiagram',
    note: 'Domain model',
  },
  { id: 'mindmap', label: 'Mindmap', dsl: 'mindmap', note: 'Ideas' },
  { id: 'c4', label: 'C4', dsl: 'C4Context', note: 'Architecture' },
  {
    id: 'activity',
    label: 'Activity',
    dsl: 'flowchart',
    note: 'As flowchart',
  },
]

type Props = {
  contextId?: string
  canvasKey?: string
  canvasEntrySessionId?: string
  currentSource: string
  onApplySource: (
    source: string,
    opts?: { final?: boolean; canvasId?: string },
  ) => void
  className?: string
}

type LiveStream = {
  abort: AbortController
  apply: (
    source: string,
    opts?: { final?: boolean; canvasId?: string },
  ) => void
}

const liveStreams = new Map<string, LiveStream>()

function buildDiagramAgentPrompt(
  instruction: string,
  currentSource: string,
  preferredDsl?: string,
): string {
  const clipped =
    currentSource.length > 80_000
      ? currentSource.slice(0, 80_000) + '\n%% truncated'
      : currentSource
  const userAsk = instruction.trim() || '(see attachments)'
  return [
    'You are a SURGICAL patch editor for ONE existing Mermaid diagram.',
    'You do NOT rewrite the whole diagram. You emit SEARCH/REPLACE patches that the app applies to the current Mermaid source.',
    '',
    '## Output format (required for normal edits)',
    'Return one or more patches using EXACTLY this syntax (no markdown fences around the patches):',
    '',
    '<<<<<<< SEARCH',
    '[exact contiguous snippet from Current Mermaid source]',
    '=======',
    '[replacement snippet]',
    '>>>>>>> REPLACE',
    '',
    'Rules for patches:',
    '- SEARCH must be copied EXACTLY from Current Mermaid source (same node IDs, arrows, whitespace when possible).',
    '- Each SEARCH should be the smallest unique region (often a single node line or edge line).',
    '- Multiple SEARCH/REPLACE blocks are OK for disjoint edits.',
    '- If the user only changes wording/redaction of one shape: patch only that label text; keep topology identical.',
    preferredDsl
      ? `- Preferred DSL chip: ${preferredDsl}. Use only if converting is requested; otherwise keep current diagram type.`
      : '- Keep the same diagram type unless the user asks to convert.',
    '',
    '## Full rewrite exception',
    'Only if the user clearly asks to rebuild / redesign / convert / start over, return the COMPLETE Mermaid source in a single ```mermaid fence instead of patches.',
    '',
    '## Forbidden',
    '- Returning a brand-new diagram when the user asked for a small text/structure tweak',
    '- Renaming all node IDs, rewiring edges, or dropping unmentioned nodes',
    '- Preamble / explanations outside patches',
    '',
    '## User request',
    userAsk,
    '',
    '## Current Mermaid source (source of truth — patch this)',
    '```mermaid',
    clipped || 'flowchart TD\n  A[Start] --> B[End]',
    '```',
  ].join('\n')
}

function extractMermaidSource(text: string): string | null {
  const fence = /```(?:mermaid|mmd)?\s*\n([\s\S]*?)```/i.exec(text)
  if (fence?.[1]?.trim()) return fence[1].trim()
  const open = /```(?:mermaid|mmd)?\s*\n([\s\S]*)$/i.exec(text)
  if (open?.[1] && open[1].trim().length > 20) return open[1].trim()
  const raw = text.trim()
  if (isMermaidFence('', raw) || isMermaidFence('mermaid', raw)) return raw
  return null
}

function useCanvasAgentJob(canvasKey: string) {
  return useSyncExternalStore(
    subscribeCanvasAgentJobs,
    () => getCanvasAgentJob(canvasKey),
    () => getCanvasAgentJob(canvasKey),
  )
}

export function DiagramAgentPanel({
  contextId,
  canvasKey = 'diagram-default',
  canvasEntrySessionId,
  currentSource,
  onApplySource,
  className = '',
}: Props) {
  const { t } = useLocale()
  const playgroundScope = resolvePlaygroundScope('diagram', canvasEntrySessionId, contextId)
  const job = useCanvasAgentJob(canvasKey)
  const busy = job.busy
  const status = job.status
  const error = job.error

  const [input, setInput] = useState('')
  const [attachments, setAttachments] = useState<ChatAttachment[]>([])
  const [dslHint, setDslHint] = useState<string | null>(null)
  const attachInputRef = useRef<HTMLInputElement | null>(null)
  const stickyRef = useRef<StickySession>({})
  const sourceRef = useRef(currentSource)
  sourceRef.current = currentSource
  const applyRef = useRef(onApplySource)
  applyRef.current = onApplySource
  const lastLiveRef = useRef('')

  useEffect(() => {
    const stream = liveStreams.get(canvasKey)
    if (stream) {
      stream.apply = (source, opts) => applyRef.current(source, opts)
    }
  }, [canvasKey, onApplySource])

  const onActiveChange = useCallback(
    (label: string, view?: ActiveSessionView) => {
      if (view) stickyRef.current = stickyFromActive(view)
      else if (label) stickyRef.current = { ...stickyRef.current, label }
    },
    [],
  )

  const removeAttachment = (id: string) => {
    setAttachments((prev) => removeAttachmentById(prev, id))
  }

  const onPickAttachments = useCallback(async (files: FileList | null) => {
    if (!files?.length) return
    const list = Array.from(files)
    const placeholders: ChatAttachment[] = list.map((f, i) => ({
      id: `pending-${Date.now()}-${i}`,
      name: f.name || 'attachment',
      size: f.size || 0,
      mime: f.type || '',
      kind: 'binary',
      loading: true,
    }))
    setAttachments((prev) => [...prev, ...placeholders])
    const results = await Promise.all(list.map((f) => processAttachmentFile(f)))
    setAttachments((prev) => {
      const withoutPending = prev.filter(
        (a) => !a.loading || !a.id.startsWith('pending-'),
      )
      const cleaned = withoutPending.filter(
        (a) => !placeholders.some((p) => p.id === a.id),
      )
      return [...cleaned, ...results]
    })
  }, [])

  const stop = useCallback(() => {
    const stream = liveStreams.get(canvasKey)
    stream?.abort.abort()
    liveStreams.delete(canvasKey)
    setCanvasAgentJob(canvasKey, { busy: false, status: null, error: null })
    clearCanvasAgentJob(canvasKey)
  }, [canvasKey])

  const send = useCallback(async () => {
    if (busy) return
    if (!canSendWithAttachments(input, attachments, [])) return
    const instruction = input.trim()
    const attachPayload = buildAttachmentPayload(attachments)
    const userFacing = composePromptWithAttachments(instruction, attachPayload)
    const jobKey = canvasKey
    const applyAtStart = applyRef.current
    setInput('')
    setAttachments([])
    lastLiveRef.current = ''
    const ac = new AbortController()
    liveStreams.set(jobKey, {
      abort: ac,
      apply: (source, opts) =>
        applyAtStart(source, { ...opts, canvasId: jobKey }),
    })
    setCanvasAgentJob(jobKey, { busy: true, status: null, error: null })
    let assembled = ''
    let streamError = ''
    const baseSource = sourceRef.current
    const stickyFields = stickyRequestFields(stickyRef.current)
    const preferred = DIAGRAM_DSL_HINTS.find((h) => h.id === dslHint)
    try {
      await streamChat(
        {
          prompt: buildDiagramAgentPrompt(
            userFacing,
            baseSource,
            preferred ? `${preferred.dsl} (${preferred.label})` : undefined,
          ),
          no_tools: true,
          agent_kind: 'diagram',
          playground_id: playgroundScope.playgroundId,
          session_id: playgroundScope.mode === 'session' ? playgroundScope.sessionId : undefined,
          workspace_id: contextId,
          images: attachPayload.images.length
            ? attachPayload.images.map((img) => ({
                name: img.name,
                media_type: img.media_type,
                data_url: img.data_url,
              }))
            : undefined,
          ...stickyFields,
        },
        (ev: ChatEvent) => {
          const stream = liveStreams.get(jobKey)
          if (stream?.abort !== ac || ac.signal.aborted) return
          if (ev.type === 'TokenDelta' && ev.delta) {
            assembled += ev.delta
            if (!looksLikePatchResponse(assembled)) {
              const live = extractMermaidSource(assembled)
              if (live && live !== lastLiveRef.current) {
                lastLiveRef.current = live
                stream.apply(live, { final: false, canvasId: jobKey })
              }
            }
          }
          if (ev.type === 'Done' && ev.final) {
            assembled = ev.final
          }
          if (ev.type === 'Error' && ev.error) {
            streamError = ev.error
            setCanvasAgentJob(jobKey, { error: ev.error })
          }
        },
        ac.signal,
      )
      const stream = liveStreams.get(jobKey)
      if (stream?.abort !== ac || ac.signal.aborted) return
      if (streamError) throw new Error(streamError)
      const full = extractMermaidSource(assembled)
      const resolved = resolveCanvasAgentReply(baseSource, assembled, full)
      if (resolved.ok) {
        stream?.apply(resolved.next, { final: true, canvasId: jobKey })
        setCanvasAgentJob(jobKey, {
          busy: false,
          status:
            resolved.mode === 'patches'
              ? `Updated (${resolved.applied} patch${resolved.applied === 1 ? '' : 'es'})`
              : 'Updated',
          error: null,
        })
        window.setTimeout(() => {
          const cur = getCanvasAgentJob(jobKey)
          if (cur.status?.startsWith('Updated')) {
            setCanvasAgentJob(jobKey, { status: null })
            if (!cur.busy && !cur.error) clearCanvasAgentJob(jobKey)
          }
        }, 1400)
      } else if (assembled.trim()) {
        setCanvasAgentJob(jobKey, {
          busy: false,
          status: null,
          error: resolved.error || 'No patches or Mermaid source in reply',
        })
      } else {
        throw new Error(t('playground.emptyAgentResult'))
      }
    } catch (e) {
      if (liveStreams.get(jobKey)?.abort !== ac) return
      if ((e as Error)?.name === 'AbortError') {
        setCanvasAgentJob(jobKey, { busy: false, status: null, error: null })
        clearCanvasAgentJob(jobKey)
        return
      }
      setCanvasAgentJob(jobKey, {
        busy: false,
        status: null,
        error: e instanceof Error ? e.message : String(e),
      })
    } finally {
      if (liveStreams.get(jobKey)?.abort === ac) liveStreams.delete(jobKey)
    }
  }, [attachments, busy, canvasKey, dslHint, input, contextId, t])

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    void send()
  }

  const canSend = canSendWithAttachments(input, attachments, [])

  return (
    <div
      data-testid="diagram-agent"
      className={`playground-agent-dock ${className}`}
    >
      {(error || (status && !busy)) && (
        <div className="flex min-h-[20px] items-center justify-end gap-2 overflow-hidden px-3 pt-2 text-[10px]">
          {error ? (
            <span
              data-testid="diagram-agent-error"
              role="alert"
              className="min-w-0 max-w-full whitespace-pre-wrap break-words text-shell-text"
              title={error}
            >
              {error}
            </span>
          ) : status ? (
            <span
              data-testid="diagram-agent-status"
              className="min-w-0 max-w-full truncate text-shell-accent"
            >
              {status}
            </span>
          ) : null}
        </div>
      )}

      <div
        data-testid="diagram-dsl-hints"
        className="flex flex-wrap gap-1 px-3 pb-0.5 pt-2"
      >
        {DIAGRAM_DSL_HINTS.map((h) => {
          const active = dslHint === h.id
          return (
            <button
              key={h.id}
              type="button"
              data-testid={`diagram-dsl-${h.id}`}
              title={h.note ? `${h.dsl} — ${h.note}` : h.dsl}
              disabled={busy}
              onClick={() => setDslHint((cur) => (cur === h.id ? null : h.id))}
              className={`rounded-full border px-2 py-0.5 text-[10px] font-medium transition disabled:opacity-50 ${
                active
                  ? 'border-shell-accent bg-shell-active text-shell-accent'
                  : 'border-shell-border text-shell-muted hover:bg-shell-hover hover:text-shell-text'
              }`}
            >
              {h.label}
            </button>
          )
        })}
      </div>

      <PreSendAttachmentChips
        attachments={attachments}
        onRemove={removeAttachment}
        disabled={busy}
      />

      <form
        data-testid="diagram-agent-form"
        onSubmit={onSubmit}
        className="playground-agent-form"
      >
        <textarea
          data-testid="diagram-agent-input"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault()
              void send()
            }
          }}
          rows={2}
          disabled={busy}
          placeholder={
            dslHint
              ? `Edit as ${DIAGRAM_DSL_HINTS.find((h) => h.id === dslHint)?.dsl}…`
              : 'Edit diagram… (not session chat)'
          }
          className="playground-composer-input disabled:opacity-60"
        />
        <div
          data-testid="diagram-agent-toolbar"
          className="mt-1.5 flex flex-wrap items-center justify-between gap-2"
        >
          <div className="flex min-w-0 flex-wrap items-center gap-1.5">
            <PaperclipButton
              inputRef={attachInputRef}
              disabled={busy}
              onPick={(files) => void onPickAttachments(files)}
            />
            <ModelPicker
              disabled={modelPickerDisabledWhileStreaming(busy)}
              onActiveChange={onActiveChange}
            />
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            {busy && (
              <>
                <GeneratingLabel
                  variant="composer"
                  className="!text-[10px]"
                />
                <button
                  type="button"
                  data-testid="diagram-agent-stop"
                  onClick={stop}
                  className="rounded-md border border-shell-border px-2 py-1 text-[11px] text-shell-muted hover:bg-shell-hover"
                  title="Stop"
                >
                  Stop
                </button>
              </>
            )}
            <button
              type="submit"
              data-testid="diagram-agent-send"
              disabled={!canSend || busy}
              title="Update diagram"
              className="shell-primary-button rounded-md px-3 py-1 text-[11px] font-medium disabled:opacity-40"
            >
              Send
            </button>
          </div>
        </div>
      </form>
    </div>
  )
}
