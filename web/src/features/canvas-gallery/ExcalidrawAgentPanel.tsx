import {
  FormEvent,
  useCallback,
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
import {
  elementsJSONForRequest,
  extractElementsFromModelText,
  mergeCanvasAIResult,
} from '../canvas/canvasAI'
import {
  emptyExcalidrawDocument,
  parseExcalidrawContent,
  type ExcalidrawDocumentData,
} from '../canvas/excalidrawDoc'
import {
  clearCanvasAgentJob,
  getCanvasAgentJob,
  setCanvasAgentJob,
  subscribeCanvasAgentJobs,
} from '../web-preview/canvasAgentJobs'
import { resolvePlaygroundScope } from './playgroundScope'

export type ExcalidrawStyleHint = {
  id: string
  label: string
  note?: string
}

export const EXCALIDRAW_STYLE_HINTS: ExcalidrawStyleHint[] = [
  { id: 'flowchart', label: 'Flowchart', note: 'Process steps' },
  { id: 'architecture', label: 'Architecture', note: 'Boxes + arrows' },
  { id: 'mindmap', label: 'Mind map', note: 'Central topic' },
  { id: 'sequence', label: 'Sequence', note: 'Actors + messages' },
  { id: 'er', label: 'ERD', note: 'Entities' },
  { id: 'wireframe', label: 'Wireframe', note: 'UI sketch' },
  { id: 'freeform', label: 'Freeform', note: 'Any sketch' },
]

type Props = {
  contextId?: string
  canvasKey?: string
  canvasEntrySessionId?: string
  document: ExcalidrawDocumentData
  elements: readonly unknown[]
  onApplied: (serialized: string) => void
  className?: string
}

type LiveStream = {
  abort: AbortController
}

const liveStreams = new Map<string, LiveStream>()

function buildExcalidrawAgentPrompt(
  instruction: string,
  elements: readonly unknown[],
  styleHint?: string,
): string {
  const count = Array.isArray(elements) ? elements.length : 0
  const mode = count > 0 ? 'edit' : 'generate'
  const existing =
    mode === 'edit' ? elementsJSONForRequest(elements) : ''
  const clipped =
    existing.length > 24_000
      ? existing.slice(0, 24_000) + '\n/* truncated */'
      : existing
  const userAsk = instruction.trim() || '(see attachments)'
  const styleLine = styleHint
    ? `- Preferred style chip: ${styleHint}. Bias layout and shapes toward that diagram style unless the user asks otherwise.`
    : '- Choose a clear layout that matches the user request.'

  return [
    'You are an Excalidraw diagram generator for the Inferenesia Playground whiteboard.',
    mode === 'generate'
      ? 'Create a NEW diagram as a JSON array of Excalidraw elements.'
      : 'EDIT the existing diagram: return a COMPLETE updated JSON array of Excalidraw elements (not a patch).',
    '',
    '## Output format (required)',
    'Return ONLY a JSON array of Excalidraw elements (or a JSON object with an "elements" key).',
    'You may wrap in a single ```json or ```excalidraw fence. No prose outside the JSON.',
    '',
    'Each element MUST include:',
    '- id: string (unique, stable like "n1", "e1")',
    '- type: rectangle | ellipse | diamond | arrow | line | text | freedraw',
    '- x, y: number',
    '- width, height: number (for rectangle/ellipse/diamond/text)',
    '- points: [[x,y],...] for arrow/line/freedraw (relative to x,y)',
    '- text + fontSize for type=text',
    '- strokeColor, backgroundColor optional',
    '- isDeleted: false, groupIds: []',
    '',
    styleLine,
    mode === 'edit'
      ? '- Keep unmentioned shapes when the user asks for a small change; only rewrite topology if they ask.'
      : '- Prefer readable spacing (gaps ≥ 40px) and short labels.',
    '',
    '## User request',
    userAsk,
    mode === 'edit'
      ? [
          '',
          '## Current elements (source of truth — return full updated array)',
          clipped || '[]',
        ].join('\n')
      : '',
  ]
    .filter(Boolean)
    .join('\n')
}

function useCanvasAgentJob(canvasKey: string) {
  return useSyncExternalStore(
    subscribeCanvasAgentJobs,
    () => getCanvasAgentJob(canvasKey),
    () => getCanvasAgentJob(canvasKey),
  )
}

export function ExcalidrawAgentPanel({
  contextId,
  canvasKey = 'excalidraw-default',
  canvasEntrySessionId,
  document,
  elements,
  onApplied,
  className = '',
}: Props) {
  const playgroundScope = resolvePlaygroundScope('excalidraw', canvasEntrySessionId, contextId)
  const job = useCanvasAgentJob(canvasKey)
  const busy = job.busy
  const status = job.status
  const error = job.error

  const [input, setInput] = useState('')
  const [attachments, setAttachments] = useState<ChatAttachment[]>([])
  const [styleHint, setStyleHint] = useState<string | null>(null)
  const attachInputRef = useRef<HTMLInputElement | null>(null)
  const stickyRef = useRef<StickySession>({})
  const elementsRef = useRef(elements)
  elementsRef.current = elements
  const documentRef = useRef(document)
  documentRef.current = document
  const applyRef = useRef(onApplied)
  applyRef.current = onApplied

  const elementCount = Array.isArray(elements) ? elements.length : 0
  const modeLabel = elementCount > 0 ? 'Apply' : 'Generate'

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
    const els = elementsRef.current
    const baseDoc = documentRef.current || emptyExcalidrawDocument()
    setInput('')
    setAttachments([])
    const ac = new AbortController()
    liveStreams.set(jobKey, { abort: ac })
    setCanvasAgentJob(jobKey, { busy: true, status: null, error: null })
    let assembled = ''
    let streamError = ''
    const stickyFields = stickyRequestFields(stickyRef.current)
    const preferred = EXCALIDRAW_STYLE_HINTS.find((h) => h.id === styleHint)
    try {
      await streamChat(
        {
          prompt: buildExcalidrawAgentPrompt(
            userFacing,
            els,
            preferred
              ? `${preferred.label}${preferred.note ? ` (${preferred.note})` : ''}`
              : undefined,
          ),
          no_tools: true,
          agent_kind: 'excalidraw',
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
          if (liveStreams.get(jobKey)?.abort !== ac || ac.signal.aborted) return
          if (ev.type === 'TokenDelta' && ev.delta) {
            assembled += ev.delta
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
      if (liveStreams.get(jobKey)?.abort !== ac || ac.signal.aborted) return
      if (streamError) throw new Error(streamError)
      const extracted = extractElementsFromModelText(assembled)
      if (!extracted.ok) {
        setCanvasAgentJob(jobKey, {
          busy: false,
          status: null,
          error: extracted.error || 'No Excalidraw elements in reply',
        })
        return
      }
      const fakeResult = {
        ok: true,
        elements_json: JSON.stringify(extracted.elements),
      }
      const merged = mergeCanvasAIResult(fakeResult, baseDoc)
      if ('error' in merged) {
        setCanvasAgentJob(jobKey, {
          busy: false,
          status: null,
          error: merged.error,
        })
        return
      }
      applyRef.current(merged.serialized)
      setCanvasAgentJob(jobKey, {
        busy: false,
        status: elementCount > 0 ? 'Updated' : 'Generated',
        error: null,
      })
      window.setTimeout(() => {
        const cur = getCanvasAgentJob(jobKey)
        if (cur.status === 'Updated' || cur.status === 'Generated') {
          setCanvasAgentJob(jobKey, { status: null })
          if (!cur.busy && !cur.error) clearCanvasAgentJob(jobKey)
        }
      }, 1400)
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
  }, [attachments, busy, canvasKey, elementCount, input, styleHint, contextId])

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    void send()
  }

  const canSend = canSendWithAttachments(input, attachments, [])

  return (
    <div
      data-testid="excalidraw-agent"
      className={`playground-agent-dock ${className}`}
    >
      {(error || (status && !busy)) && (
        <div className="flex min-h-[20px] items-center justify-end gap-2 overflow-hidden px-3 pt-2 text-[10px]">
          {error ? (
            <span
              data-testid="excalidraw-agent-error"
              role="alert"
              className="min-w-0 max-w-full whitespace-pre-wrap break-words text-shell-text"
              title={error}
            >
              {error}
            </span>
          ) : status ? (
            <span
              data-testid="excalidraw-agent-status"
              className="min-w-0 max-w-full truncate text-shell-accent"
            >
              {status}
            </span>
          ) : null}
        </div>
      )}

      <div
        data-testid="excalidraw-style-hints"
        className="flex flex-wrap gap-1 px-3 pb-0.5 pt-2"
      >
        {EXCALIDRAW_STYLE_HINTS.map((h) => {
          const active = styleHint === h.id
          return (
            <button
              key={h.id}
              type="button"
              data-testid={`excalidraw-style-${h.id}`}
              title={h.note ? `${h.label} — ${h.note}` : h.label}
              disabled={busy}
              onClick={() =>
                setStyleHint((cur) => (cur === h.id ? null : h.id))
              }
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
        data-testid="excalidraw-agent-form"
        onSubmit={onSubmit}
        className="playground-agent-form"
      >
        <textarea
          data-testid="excalidraw-agent-input"
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
            styleHint
              ? `${modeLabel} as ${EXCALIDRAW_STYLE_HINTS.find((h) => h.id === styleHint)?.label}…`
              : elementCount > 0
                ? 'Edit whiteboard… (not session chat)'
                : 'Describe diagram… (not session chat)'
          }
          className="playground-composer-input disabled:opacity-60"
        />
        <div
          data-testid="excalidraw-agent-toolbar"
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
                  data-testid="excalidraw-agent-stop"
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
              data-testid="excalidraw-agent-send"
              disabled={!canSend || busy}
              title={modeLabel}
              className="shell-primary-button rounded-md px-3 py-1 text-[11px] font-medium disabled:opacity-40"
            >
              {modeLabel}
            </button>
          </div>
        </div>
      </form>
    </div>
  )
}

export function documentFromSerialized(content: string): ExcalidrawDocumentData {
  return parseExcalidrawContent(content).data
}
