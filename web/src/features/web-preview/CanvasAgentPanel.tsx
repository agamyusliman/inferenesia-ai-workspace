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
import {
  clearCanvasAgentJob,
  getCanvasAgentJob,
  setCanvasAgentJob,
  subscribeCanvasAgentJobs,
} from './canvasAgentJobs'
import {
  looksLikePatchResponse,
  resolveCanvasAgentReply,
} from './canvasPatch'
import { extractHtmlDocument } from './webPreview'
import { resolvePlaygroundScope } from '../canvas-gallery/playgroundScope'

type Props = {
  contextId?: string
  canvasKey?: string
  canvasEntrySessionId?: string
  currentHtml: string
  onApplyHtml: (
    html: string,
    opts?: { final?: boolean; canvasId?: string },
  ) => void
  className?: string
}

type LiveStream = {
  abort: AbortController
  apply: (html: string, opts?: { final?: boolean; canvasId?: string }) => void
}

const liveStreams = new Map<string, LiveStream>()

function buildAgentPrompt(instruction: string, currentHtml: string): string {
  const html = currentHtml || '<!DOCTYPE html><html><body></body></html>'
  const clipped =
    html.length > 120_000
      ? html.slice(0, 120_000) + '\n<!-- truncated for context -->'
      : html
  const userAsk = instruction.trim() || '(see attachments)'
  return [
    'You are a SURGICAL patch editor for ONE existing HTML canvas.',
    'You do NOT rewrite the page. You emit SEARCH/REPLACE patches that the app applies to the current HTML.',
    '',
    '## Output format (required for normal edits)',
    'Return one or more patches using EXACTLY this syntax (no markdown fences around the patches):',
    '',
    '<<<<<<< SEARCH',
    '[exact contiguous snippet from Current HTML]',
    '=======',
    '[replacement snippet]',
    '>>>>>>> REPLACE',
    '',
    'Rules for patches:',
    '- SEARCH must be copied EXACTLY from Current HTML (same whitespace/indentation when possible).',
    '- Each SEARCH should be the smallest unique region that covers the change (a few lines, not the whole file).',
    '- You may emit multiple SEARCH/REPLACE blocks if several disjoint spots must change.',
    '- Do not invent new global CSS, fonts, or layout unless the user asked.',
    '- If the user only asks to change navbar / one section / one string / one color: only patch that region.',
    '',
    '## Full rewrite exception',
    'Only if the user clearly asks for a full redesign / rebuild / from scratch / completely new page, return a complete HTML document in a single ```html fence instead of patches.',
    '',
    '## Forbidden',
    '- Returning a full redesigned HTML when the user asked for a small change',
    '- Changing structure, classes, or copy that the user did not mention',
    '- Preamble / explanations outside patches (or outside the single full-html fence on redesign)',
    '',
    '## User request',
    userAsk,
    '',
    '## Current HTML (source of truth — patch this)',
    '```html',
    clipped,
    '```',
  ].join('\n')
}

function extractStreamingHtml(text: string): string | null {
  const full = extractHtmlDocument(text)
  if (full) return full
  const openFence = /```(?:html|htm)?\s*\n([\s\S]*)$/i.exec(text)
  if (openFence?.[1] && openFence[1].trim().length > 80) {
    return openFence[1]
  }
  const idx = text.search(/<!DOCTYPE\s+html|<html[\s>]/i)
  if (idx >= 0 && text.length - idx > 80) {
    return text.slice(idx)
  }
  return null
}

function useCanvasAgentJob(canvasKey: string) {
  return useSyncExternalStore(
    subscribeCanvasAgentJobs,
    () => getCanvasAgentJob(canvasKey),
    () => getCanvasAgentJob(canvasKey),
  )
}

export function CanvasAgentPanel({
  contextId,
  canvasKey = 'html-default',
  canvasEntrySessionId,
  currentHtml,
  onApplyHtml,
  className = '',
}: Props) {
  const { t } = useLocale()
  const playgroundScope = resolvePlaygroundScope('html', canvasEntrySessionId, contextId)
  const job = useCanvasAgentJob(canvasKey)
  const busy = job.busy
  const status = job.status
  const error = job.error

  const [input, setInput] = useState('')
  const [attachments, setAttachments] = useState<ChatAttachment[]>([])
  const attachInputRef = useRef<HTMLInputElement | null>(null)
  const stickyRef = useRef<StickySession>({})
  const htmlRef = useRef(currentHtml)
  htmlRef.current = currentHtml
  const applyRef = useRef(onApplyHtml)
  applyRef.current = onApplyHtml
  const lastLiveRef = useRef('')

  useEffect(() => {
    const stream = liveStreams.get(canvasKey)
    if (stream) {
      stream.apply = (html, opts) => applyRef.current(html, opts)
    }
  }, [canvasKey, onApplyHtml])

  const onActiveChange = useCallback(
    (label: string, view?: ActiveSessionView) => {
      if (view) {
        stickyRef.current = stickyFromActive(view)
      } else if (label) {
        stickyRef.current = {
          ...stickyRef.current,
          label,
        }
      }
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
      apply: (html, opts) =>
        applyAtStart(html, { ...opts, canvasId: jobKey }),
    })
    setCanvasAgentJob(jobKey, { busy: true, status: null, error: null })
    let assembled = ''
    let streamError = ''
    const baseHtml = htmlRef.current
    const stickyFields = stickyRequestFields(stickyRef.current)
    try {
      await streamChat(
        {
          prompt: buildAgentPrompt(userFacing, baseHtml),
          no_tools: true,
          agent_kind: 'html',
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
              const live = extractStreamingHtml(assembled)
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
      const full =
        extractHtmlDocument(assembled) || extractStreamingHtml(assembled)
      const resolved = resolveCanvasAgentReply(baseHtml, assembled, full)
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
          error: resolved.error || 'No patches or HTML document in reply',
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
  }, [attachments, busy, canvasKey, input, contextId, t])

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    void send()
  }

  const canSend = canSendWithAttachments(input, attachments, [])

  return (
    <div
      data-testid="web-preview-agent"
      className={`playground-agent-dock ${className}`}
    >
      {(error || (status && !busy)) && (
        <div className="flex min-h-[20px] items-center justify-end gap-2 overflow-hidden px-3 pt-2 text-[10px]">
          {error ? (
            <span
              data-testid="web-preview-agent-error"
              role="alert"
              className="min-w-0 max-w-full whitespace-pre-wrap break-words text-shell-text"
              title={error}
            >
              {error}
            </span>
          ) : status ? (
            <span
              data-testid="web-preview-agent-status"
              className="min-w-0 max-w-full truncate text-shell-accent"
            >
              {status}
            </span>
          ) : null}
        </div>
      )}

      <PreSendAttachmentChips
        attachments={attachments}
        onRemove={removeAttachment}
        disabled={busy}
      />

      <form
        data-testid="web-preview-agent-form"
        onSubmit={onSubmit}
        className="playground-agent-form"
      >
        <textarea
          data-testid="web-preview-agent-input"
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
          placeholder="Edit active canvas… (not session chat)"
          className="playground-composer-input disabled:opacity-60"
        />
        <div
          data-testid="web-preview-agent-toolbar"
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
                  data-testid="web-preview-agent-stop"
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
              data-testid="web-preview-agent-send"
              disabled={!canSend || busy}
              title="Update active canvas"
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
