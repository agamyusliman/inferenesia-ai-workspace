import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import {
  Check,
  Copy,
  Download,
  FileText,
  Image as ImageIcon,
  RotateCcw,
  Trash2,
} from 'lucide-react'
import { useLocale } from '../i18n/LocaleProvider'
import {
  assistantMarkdownFilename,
  imageDownloadFilename,
  messageActionFlags,
} from './messageActions'
import { extractMarkdownImages, textForCopy } from './messageChrome'

export type MessageActionBarProps = {
  role: 'user' | 'assistant' | 'system'
  content: string
  messageId: string
  visible: boolean
  streaming?: boolean
  downloadName?: string
  sessionName?: string
  messageIndex?: number
  onDelete?: (messageId: string) => void
  onUndoFromHere?: (messageId: string) => void
  onPreviewImage?: (src: string, alt: string) => void
  className?: string
}

const iconBtn =
  'inline-flex h-4 w-4 shrink-0 items-center justify-center rounded-sm text-shell-muted transition hover:bg-shell-border/40 hover:text-shell-text focus-visible:outline focus-visible:outline-1 focus-visible:outline-offset-1 focus-visible:outline-shell-muted/50'

function FloatingTip({
  anchor,
  label,
}: {
  anchor: HTMLElement | null
  label: string
}) {
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null)

  useEffect(() => {
    if (!anchor) {
      setPos(null)
      return
    }
    const place = () => {
      const r = anchor.getBoundingClientRect()
      setPos({ left: r.left + r.width / 2, top: r.bottom + 6 })
    }
    place()
    window.addEventListener('scroll', place, true)
    window.addEventListener('resize', place)
    return () => {
      window.removeEventListener('scroll', place, true)
      window.removeEventListener('resize', place)
    }
  }, [anchor, label])

  if (!anchor || !pos || typeof document === 'undefined') return null

  return createPortal(
    <span
      role="tooltip"
      data-testid="chat-msg-action-tooltip"
      className="pointer-events-none fixed z-[9999] -translate-x-1/2 whitespace-nowrap rounded border border-shell-border bg-shell-panel px-1.5 py-0.5 text-[10px] font-normal normal-case tracking-normal text-shell-text shadow-lg"
      style={{ left: pos.left, top: pos.top }}
    >
      {label}
    </span>,
    document.body,
  )
}

function IconAction({
  testId,
  label,
  onClick,
  className,
  children,
}: {
  testId: string
  label: string
  onClick: () => void
  className?: string
  children: ReactNode
}) {
  const ref = useRef<HTMLButtonElement | null>(null)
  const [tip, setTip] = useState(false)

  return (
    <>
      <button
        ref={ref}
        type="button"
        data-testid={testId}
        onClick={onClick}
        onMouseEnter={() => setTip(true)}
        onMouseLeave={() => setTip(false)}
        onFocus={() => setTip(true)}
        onBlur={() => setTip(false)}
        className={`${iconBtn} ${className || ''}`}
        title={label}
        aria-label={label}
      >
        {children}
      </button>
      {tip && <FloatingTip anchor={ref.current} label={label} />}
    </>
  )
}

export function MessageActionBar({
  role,
  content,
  messageId,
  visible,
  streaming,
  downloadName,
  sessionName,
  messageIndex,
  onDelete,
  onUndoFromHere,
  onPreviewImage,
  className,
}: MessageActionBarProps) {
  const { t } = useLocale()
  const [copied, setCopied] = useState(false)
  const [feedback, setFeedback] = useState<string | null>(null)
  const images = extractMarkdownImages(content)
  const flags = messageActionFlags({
    role,
    streaming,
    hasImages: images.length > 0,
  })

  const flash = useCallback((label: string) => {
    setFeedback(label)
    window.setTimeout(() => setFeedback(null), 1400)
  }, [])

  const onCopy = useCallback(async () => {
    const text = textForCopy(content)
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      flash(t('chat.copied'))
      window.setTimeout(() => setCopied(false), 1200)
    } catch {
      flash(t('chat.copyFailed'))
    }
  }, [content, flash, t])

  const onDownloadMd = useCallback(() => {
    const text = textForCopy(content)
    const blob = new Blob([text], { type: 'text/markdown;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download =
      downloadName ||
      assistantMarkdownFilename(messageId, sessionName, messageIndex)
    a.rel = 'noopener'
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
    flash(t('chat.downloadedMd'))
  }, [content, downloadName, messageId, sessionName, messageIndex, flash, t])

  const onDownloadImage = useCallback(() => {
    const first = images[0]
    if (!first) return
    const a = document.createElement('a')
    a.href = first.src
    a.download = imageDownloadFilename(first.src, first.alt, 0, sessionName)
    a.rel = 'noopener'
    a.target = '_blank'
    document.body.appendChild(a)
    a.click()
    a.remove()
    flash(t('chat.imageDownloadStarted'))
  }, [images, sessionName, flash, t])

  if (!visible) return null

  const copyLabel = copied
    ? t('chat.copied')
    : role === 'assistant'
      ? t('chat.copyMarkdown')
      : t('chat.copyMessage')

  return (
    <div
      data-testid="chat-msg-actions"
      data-role={role}
      data-message-id={messageId}
      className={`pointer-events-auto relative z-30 flex max-w-full flex-nowrap items-center gap-0.5 overflow-visible ${className || ''}`}
    >
      {flags.copy && (
        <IconAction
          testId="chat-msg-action-copy"
          label={copyLabel}
          onClick={() => void onCopy()}
        >
          {copied ? (
            <Check size={11} className="text-emerald-400" />
          ) : (
            <Copy size={11} />
          )}
        </IconAction>
      )}
      {flags.downloadMd && (
        <IconAction
          testId="chat-msg-action-download-md"
          label={t('chat.downloadMd')}
          onClick={onDownloadMd}
        >
          <FileText size={11} />
        </IconAction>
      )}
      {flags.previewImage && (
        <IconAction
          testId="chat-msg-action-preview-image"
          label={t('chat.previewImage')}
          onClick={() => {
            const first = images[0]
            if (first) onPreviewImage?.(first.src, first.alt)
          }}
        >
          <ImageIcon size={11} />
        </IconAction>
      )}
      {flags.downloadImage && (
        <IconAction
          testId="chat-msg-action-download-image"
          label={t('chat.downloadImage')}
          onClick={onDownloadImage}
        >
          <Download size={11} />
        </IconAction>
      )}
      {flags.undoFromHere && onUndoFromHere && (
        <IconAction
          testId="chat-msg-action-undo-from-here"
          label={t('chat.undoFromHere')}
          onClick={() => onUndoFromHere(messageId)}
        >
          <RotateCcw size={11} />
        </IconAction>
      )}
      {flags.delete && onDelete && (
        <IconAction
          testId="chat-msg-action-delete"
          label={t('chat.deleteMessage')}
          onClick={() => {
            onDelete(messageId)
            flash(t('chat.deleted'))
          }}
          className="text-red-300/90 hover:bg-red-500/15 hover:text-red-200"
        >
          <Trash2 size={11} />
        </IconAction>
      )}
      {feedback && (
        <span
          data-testid="chat-msg-action-feedback"
          className="ml-0.5 max-w-[4.5rem] truncate text-[9px] leading-none text-emerald-400"
          aria-live="polite"
        >
          {feedback}
        </span>
      )}
    </div>
  )
}
