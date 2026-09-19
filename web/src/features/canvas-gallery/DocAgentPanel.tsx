import { FormEvent, useCallback, useEffect, useRef, useState, useSyncExternalStore } from 'react'
import { streamChat, type ActiveSessionView, type ChatEvent } from '../../lib/api'
import { PaperclipButton, PreSendAttachmentChips } from '../chat/AttachmentChips'
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
} from '../web-preview/canvasAgentJobs'
import {
  applyDocAgentReply,
  buildDocAgentPrompt,
  docAgentSource,
  DOC_AGENT_HINTS,
  type DocAgentKind,
} from './docAgent'
import { resolvePlaygroundScope } from './playgroundScope'

type Props = {
  kind: DocAgentKind
  contextId?: string
  canvasKey: string
  canvasEntrySessionId?: string
  /** Stored document payload (markdown text, or table/timeline JSON). */
  currentContent: string
  onApply: (content: string, opts: { canvasId: string }) => void
  placeholder: string
  className?: string
}

const liveStreams = new Map<string, AbortController>()

function useCanvasAgentJob(canvasKey: string) {
  return useSyncExternalStore(
    subscribeCanvasAgentJobs,
    () => getCanvasAgentJob(canvasKey),
    () => getCanvasAgentJob(canvasKey),
  )
}

export function DocAgentPanel({
  kind,
  contextId,
  canvasKey,
  canvasEntrySessionId,
  currentContent,
  onApply,
  placeholder,
  className = '',
}: Props) {
  const { t } = useLocale()
  const playgroundScope = resolvePlaygroundScope(
    kind,
    canvasEntrySessionId,
    contextId,
  )
  const job = useCanvasAgentJob(canvasKey)
  const busy = job.busy
  const status = job.status
  const error = job.error

  const [input, setInput] = useState('')
  const [attachments, setAttachments] = useState<ChatAttachment[]>([])
  const [hintId, setHintId] = useState<string | null>(null)
  const attachInputRef = useRef<HTMLInputElement | null>(null)
  const stickyRef = useRef<StickySession>({})
  const contentRef = useRef(currentContent)
  contentRef.current = currentContent
  const applyRef = useRef(onApply)
  applyRef.current = onApply

  const hints = DOC_AGENT_HINTS[kind]

  useEffect(() => {
    setHintId(null)
  }, [canvasKey])

  const onActiveChange = useCallback(
    (label: string, view?: ActiveSessionView) => {
      if (view) stickyRef.current = stickyFromActive(view)
      else if (label) stickyRef.current = { ...stickyRef.current, label }
    },
    [],
  )

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
      const cleaned = prev.filter(
        (a) => !placeholders.some((p) => p.id === a.id),
      )
      return [...cleaned, ...results]
    })
  }, [])

  const stop = useCallback(() => {
    liveStreams.get(canvasKey)?.abort()
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
    const storedBefore = contentRef.current
    const hint = hints.find((h) => h.id === hintId)
    setInput('')
    setAttachments([])
    const ac = new AbortController()
    liveStreams.set(jobKey, ac)
    setCanvasAgentJob(jobKey, { busy: true, status: null, error: null })
    let assembled = ''
    const stickyFields = stickyRequestFields(stickyRef.current)
    try {
      await streamChat(
        {
          prompt: buildDocAgentPrompt(
            kind,
            userFacing,
            docAgentSource(kind, storedBefore),
            hint,
          ),
          no_tools: true,
          agent_kind: kind,
          playground_id: playgroundScope.playgroundId,
          session_id:
            playgroundScope.mode === 'session'
              ? playgroundScope.sessionId
              : undefined,
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
          if (!liveStreams.has(jobKey)) return
          if (ev.type === 'TokenDelta' && ev.delta) assembled += ev.delta
          if (ev.type === 'Done' && ev.final) assembled = ev.final
          if (ev.type === 'Error' && ev.error) {
            setCanvasAgentJob(jobKey, { error: ev.error })
          }
        },
        ac.signal,
      )
      if (!assembled.trim()) {
        setCanvasAgentJob(jobKey, { busy: false, status: null })
        clearCanvasAgentJob(jobKey)
        return
      }
      const applied = applyDocAgentReply(kind, storedBefore, assembled)
      if (!applied.ok) {
        setCanvasAgentJob(jobKey, {
          busy: false,
          status: null,
          error: applied.error,
        })
        return
      }
      applyRef.current(applied.content, { canvasId: jobKey })
      setCanvasAgentJob(jobKey, {
        busy: false,
        status:
          applied.mode === 'patches'
            ? `${t('docAgent.updated')} (${applied.applied})`
            : t('docAgent.updated'),
        error: null,
      })
      window.setTimeout(() => {
        const cur = getCanvasAgentJob(jobKey)
        if (cur.status?.startsWith(t('docAgent.updated'))) {
          setCanvasAgentJob(jobKey, { status: null })
          if (!cur.busy && !cur.error) clearCanvasAgentJob(jobKey)
        }
      }, 1400)
    } catch (e) {
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
      liveStreams.delete(jobKey)
    }
  }, [
    attachments,
    busy,
    canvasKey,
    contextId,
    hintId,
    hints,
    input,
    kind,
    playgroundScope.mode,
    playgroundScope.playgroundId,
    playgroundScope.sessionId,
    t,
  ])

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    void send()
  }

  const canSend = canSendWithAttachments(input, attachments, [])

  return (
    <div
      data-testid={`${kind}-agent`}
      className={`playground-agent-dock ${className}`}
    >
      {(error || (status && !busy)) && (
        <div className="flex min-h-[20px] items-center justify-end gap-2 overflow-hidden px-3 pt-2 text-[10px]">
          {error ? (
            <span
              data-testid={`${kind}-agent-error`}
              className="min-w-0 max-w-full truncate text-red-400"
              title={error}
            >
              {error}
            </span>
          ) : (
            <span
              data-testid={`${kind}-agent-status`}
              className="min-w-0 max-w-full truncate text-shell-accent"
            >
              {status}
            </span>
          )}
        </div>
      )}

      <div
        data-testid={`${kind}-agent-hints`}
        className="flex flex-wrap gap-1 px-3 pb-0.5 pt-2"
      >
        {hints.map((h) => {
          const active = hintId === h.id
          return (
            <button
              key={h.id}
              type="button"
              data-testid={`${kind}-agent-hint-${h.id}`}
              data-active={active ? 'true' : 'false'}
              title={h.note ? `${h.instruction} — ${h.note}` : h.instruction}
              disabled={busy}
              onClick={() => setHintId((cur) => (cur === h.id ? null : h.id))}
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
        onRemove={(id) =>
          setAttachments((prev) => removeAttachmentById(prev, id))
        }
        disabled={busy}
      />

      <form
        data-testid={`${kind}-agent-form`}
        onSubmit={onSubmit}
        className="playground-agent-form"
      >
        <textarea
          data-testid={`${kind}-agent-input`}
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
          placeholder={placeholder}
          className="playground-composer-input disabled:opacity-60"
        />
        <div className="mt-1.5 flex flex-wrap items-center justify-between gap-2">
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
                <GeneratingLabel variant="composer" className="!text-[10px]" />
                <button
                  type="button"
                  data-testid={`${kind}-agent-stop`}
                  onClick={stop}
                  className="rounded-md border border-shell-border px-2 py-1 text-[11px] text-shell-muted hover:bg-shell-hover"
                  title={t('action.stop')}
                >
                  {t('action.stop')}
                </button>
              </>
            )}
            <button
              type="submit"
              data-testid={`${kind}-agent-send`}
              disabled={!canSend || busy}
              title={t('action.send')}
              className="shell-primary-button rounded-md px-3 py-1 text-[11px] font-medium disabled:opacity-40"
            >
              {t('action.send')}
            </button>
          </div>
        </div>
      </form>
    </div>
  )
}
