import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { Download, X } from 'lucide-react'
import {
  buildDiffPreview,
  diffLineClass,
  downloadFilename,
  shouldRenderAsDiff,
} from './codeBlockUtils'
import { HtmlCodeEditor } from './HtmlCodeEditor'
import { SHELL } from '../shell/shellTokens'

type Props = {
  text: string
  language?: string
  path?: string
  editable?: boolean
  onChange?: (value: string) => void
  onSave?: (value: string) => void
  onClose: () => void
}

export function CodePreviewModal({
  text,
  language = '',
  path,
  editable = false,
  onChange,
  onSave,
  onClose,
}: Props) {
  const [draft, setDraft] = useState(text)
  const isDiff = shouldRenderAsDiff(language, text)
  const title =
    path ||
    (isDiff ? 'Diff preview' : language ? `${language} code` : 'Code preview')
  const filename = downloadFilename(language, path, isDiff)
  const body = editable ? draft : text

  useEffect(() => {
    setDraft(text)
  }, [text])

  useEffect(() => {
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        e.stopPropagation()
        onClose()
      }
    }
    window.addEventListener('keydown', onKey, true)
    return () => {
      document.body.style.overflow = prev
      window.removeEventListener('keydown', onKey, true)
    }
  }, [onClose])

  const onDownload = () => {
    const blob = new Blob([body], { type: 'text/plain;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    a.rel = 'noopener'
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  }

  const diff = isDiff ? buildDiffPreview(body, Number.MAX_SAFE_INTEGER) : null

  const node = (
    <div
      data-testid="chat-code-preview-modal"
      role="dialog"
      aria-modal="true"
      aria-label={title}
      className="fixed inset-0 z-[200] flex items-center justify-center bg-black/70 p-3 backdrop-blur-[1px] sm:p-6"
    >
      <div
        className="flex max-h-[min(92vh,900px)] w-full max-w-5xl flex-col overflow-hidden rounded-lg border border-shell-border bg-shell-panel shadow-2xl"
        role="document"
      >
        <div className="flex shrink-0 items-center gap-2 border-b border-shell-border px-3 py-2">
          <div className="min-w-0 flex-1">
            <h2 className="truncate text-sm font-medium text-shell-text">{title}</h2>
            <p className="truncate text-[10px] text-shell-muted">
              {isDiff
                ? `diff · +${diff?.stats.additions || 0} −${diff?.stats.deletions || 0}`
                : editable
                  ? `${language || 'text'} · editable`
                  : language || 'text'}
              {path ? ` · ${path}` : ''}
            </p>
          </div>
          <button
            type="button"
            data-testid="chat-code-modal-download"
            onClick={onDownload}
            className="inline-flex items-center gap-1 rounded border border-shell-border px-2 py-1 text-[11px] text-shell-muted hover:bg-shell-border/40 hover:text-shell-text"
            title={`Download ${filename}`}
          >
            <Download size={SHELL.iconXs} />
            Download
          </button>
          <button
            type="button"
            data-testid="chat-code-modal-close"
            onClick={onClose}
            className="rounded p-1 text-shell-muted hover:bg-shell-border/40 hover:text-shell-text"
            aria-label="Close"
          >
            <X size={SHELL.iconSm} />
          </button>
        </div>
        <div
          className="flex h-[min(70vh,640px)] min-h-[20rem] min-w-0 flex-1 flex-col overflow-hidden"
          style={{ background: '#1e1e1e', color: '#d4d4d4' }}
        >
          {diff ? (
            <pre
              data-testid="chat-code-modal-diff"
              className="m-0 h-full overflow-auto p-0 font-mono text-[12px] leading-snug"
              style={{ background: '#1e1e1e', color: '#d4d4d4' }}
            >
              {diff.lines.map((line, i) => (
                <div
                  key={i}
                  className={`whitespace-pre-wrap break-all px-3 py-0.5 ${diffLineClass(line.kind)}`}
                >
                  {line.text || ' '}
                </div>
              ))}
            </pre>
          ) : body ? (
            <HtmlCodeEditor
              testId="chat-code-modal-editor"
              value={body}
              readOnly={!editable}
              label={language || 'HTML'}
              showToolbar
              onChange={(v) => {
                setDraft(v)
                onChange?.(v)
              }}
              onSave={() => {
                onSave?.(draft)
              }}
            />
          ) : (
            <p
              data-testid="chat-code-modal-empty"
              className="p-4 text-sm"
              style={{ color: '#858585' }}
            >
              No source content.
            </p>
          )}
        </div>
      </div>
    </div>
  )

  if (typeof document === 'undefined') return node
  return createPortal(node, document.body)
}
