import { useEffect, useRef, useState } from 'react'
import { ChevronDown, ChevronRight, Brain } from 'lucide-react'

type Props = {
  content: string
  streaming?: boolean
  defaultOpen?: boolean
}

export function ThinkingBlock({
  content,
  streaming = false,
  defaultOpen = false,
}: Props) {
  const [open, setOpen] = useState(defaultOpen || streaming)
  const bodyRef = useRef<HTMLDivElement | null>(null)
  const wasStreaming = useRef(streaming)

  useEffect(() => {
    if (streaming) {
      setOpen(true)
      wasStreaming.current = true
      return
    }
    if (wasStreaming.current && !streaming) {
      wasStreaming.current = false
    }
  }, [streaming])

  useEffect(() => {
    if (!streaming || !open) return
    const el = bodyRef.current
    if (!el) return
    el.scrollTop = el.scrollHeight
  }, [content, streaming, open])

  if (!content?.trim()) return null
  const Icon = open ? ChevronDown : ChevronRight

  return (
    <div
      data-testid="chat-thinking-block"
      data-open={open ? 'true' : 'false'}
      data-streaming={streaming ? 'true' : 'false'}
      className="mb-2 overflow-hidden rounded-md border border-violet-500/25 bg-violet-950/20"
    >
      <button
        type="button"
        data-testid="chat-thinking-toggle"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-1.5 px-2 py-1.5 text-left text-[10px] font-semibold uppercase tracking-wide text-violet-200/90 transition hover:bg-violet-500/10"
      >
        <Icon size={12} className="shrink-0 opacity-80" aria-hidden />
        <Brain
          size={12}
          className={`shrink-0 text-violet-300/90 ${streaming ? 'animate-pulse' : ''}`}
          aria-hidden
        />
        <span>{streaming ? 'Reasoning' : 'Thinking'}</span>
        {streaming && (
          <span
            data-testid="chat-thinking-live"
            className="ml-1 font-normal normal-case text-violet-300/70"
          >
            · live
          </span>
        )}
        <span className="ml-auto font-normal normal-case text-violet-300/60">
          {open ? 'Hide' : 'Show'}
        </span>
      </button>
      {open && (
        <div
          ref={bodyRef}
          data-testid="chat-thinking-content"
          className="max-h-48 overflow-y-auto border-t border-violet-500/20 px-2.5 py-2 font-mono text-[11px] leading-relaxed text-violet-100/80 whitespace-pre-wrap break-words"
        >
          {content.trim()}
          {streaming && (
            <span
              data-testid="chat-thinking-caret"
              className="ml-0.5 inline-block h-3 w-1.5 translate-y-0.5 animate-pulse bg-violet-300/80 align-middle"
              aria-hidden
            />
          )}
        </div>
      )}
    </div>
  )
}
