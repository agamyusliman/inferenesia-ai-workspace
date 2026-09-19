import type { RefObject } from 'react'
import { Paperclip, X } from 'lucide-react'
import {
  attachmentIconLabel,
  formatBytes,
  type AttachmentKind,
  type BubbleAttachmentChip,
  type ChatAttachment,
} from './chatAttachments'

type PreSendProps = {
  attachments: ChatAttachment[]
  onRemove: (id: string) => void
  disabled?: boolean
}

/**
 * Pre-send paperclip chips with name/size/icon and remove (✕) (VAL-CHAT-021).
 * Images show a small thumbnail when previewUrl is set.
 */
export function PreSendAttachmentChips({ attachments, onRemove, disabled }: PreSendProps) {
  if (!attachments.length) return null
  let imageIndex = 0
  return (
    <div
      data-testid="chat-attachment-chips"
      className="flex flex-wrap gap-1.5 border-t border-shell-border px-3 py-1.5"
    >
      {attachments.map((a) => {
        const isImage = a.kind === 'image'
        if (isImage) imageIndex += 1
        const imageLabel = isImage ? `[Image ${imageIndex}]` : null
        return (
        <div
          key={a.id}
          data-testid="chat-attachment-chip"
          data-kind={a.kind}
          data-loading={a.loading ? 'true' : 'false'}
          data-name={a.name}
          className="group flex max-w-full items-center gap-1.5 rounded-md border border-shell-border bg-shell-panel px-1.5 py-1 text-[10px] text-shell-text"
          title={a.error || a.name}
        >
          {a.kind === 'image' && a.previewUrl ? (
            <span className="relative shrink-0">
              <img
                src={a.previewUrl}
                alt=""
                data-testid="chat-attachment-thumb"
                className="h-7 w-7 rounded object-cover"
              />
              {imageLabel && (
                <span className="absolute -left-1 -top-1 rounded bg-shell-accent px-0.5 text-[8px] font-bold leading-tight text-shell-bg">
                  {imageLabel}
                </span>
              )}
            </span>
          ) : (
            <span
              data-testid="chat-attachment-icon"
              className="flex h-7 w-7 shrink-0 items-center justify-center rounded bg-shell-bg font-mono text-[9px] font-semibold text-shell-muted"
            >
              {attachmentIconLabel(a.kind)}
            </span>
          )}
          <span className="min-w-0 flex-1">
            <span className="block max-w-[10rem] truncate font-medium" title={a.name}>
              {a.name}
            </span>
            <span className="block text-shell-muted">
              {a.loading ? 'Reading…' : a.error ? a.error : formatBytes(a.size)}
            </span>
          </span>
          <button
            type="button"
            data-testid="chat-attachment-remove"
            disabled={disabled}
            onClick={() => onRemove(a.id)}
            className="shrink-0 rounded p-0.5 text-shell-muted hover:bg-shell-border/40 hover:text-shell-text disabled:opacity-40"
            title="Remove attachment"
            aria-label={`Remove ${a.name}`}
          >
            <X className="h-3.5 w-3.5" strokeWidth={2} />
          </button>
        </div>
        )
      })}
    </div>
  )
}

type BubbleProps = {
  chips: BubbleAttachmentChip[]
}

/**
 * Post-send attachment chips on the user bubble (VAL-CHAT-024).
 * No remove — already sent.
 */
export function BubbleAttachmentChips({ chips }: BubbleProps) {
  if (!chips?.length) return null
  return (
    <div data-testid="chat-msg-attachments" className="mb-1.5 flex flex-wrap gap-1">
      {chips.map((c) => (
        <span
          key={c.id}
          data-testid="chat-msg-attachment-chip"
          data-kind={c.kind}
          data-name={c.name}
          className="inline-flex max-w-full items-center gap-1 rounded bg-shell-panel px-1.5 py-0.5 text-[10px] text-shell-accent"
          title={`${c.name} (${formatBytes(c.size)})`}
        >
          {c.kind === 'image' && c.previewUrl ? (
            <img
              src={c.previewUrl}
              alt=""
              className="h-4 w-4 rounded object-cover"
              data-testid="chat-msg-attachment-thumb"
            />
          ) : (
            <span className="font-mono text-[9px] text-shell-muted">
              {attachmentIconLabel(c.kind as AttachmentKind)}
            </span>
          )}
          <span className="max-w-[8rem] truncate">{c.name}</span>
        </span>
      ))}
    </div>
  )
}

type PaperclipProps = {
  onPick: (files: FileList | null) => void
  disabled?: boolean
  inputRef?: RefObject<HTMLInputElement | null>
  accept?: string
  title?: string
  'aria-label'?: string
  testId?: string
  inputTestId?: string
}

/** Hidden file input + paperclip trigger (VAL-CHAT-021). */
export function PaperclipButton({
  onPick,
  disabled,
  inputRef,
  accept = 'image/*,.pdf,.txt,.md,.json,.ts,.tsx,.js,.jsx,.go,.py,.rs,.css,.html,.yml,.yaml,.toml,.csv,.log,text/*',
  title = 'Attach files (images, PDF, text, code)',
  'aria-label': ariaLabel = 'Attach files',
  testId = 'chat-attach-btn',
  inputTestId = 'chat-attach-input',
}: PaperclipProps) {
  return (
    <>
      <input
        ref={inputRef}
        type="file"
        multiple
        data-testid={inputTestId}
        className="hidden"
        accept={accept}
        onChange={(e) => {
          onPick(e.target.files)
          e.target.value = ''
        }}
      />
      <button
        type="button"
        data-testid={testId}
        disabled={disabled}
        title={title}
        aria-label={ariaLabel}
        onClick={() => inputRef?.current?.click()}
        className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-lg border border-shell-border text-shell-muted hover:bg-shell-border/30 hover:text-shell-text disabled:opacity-40"
      >
        <Paperclip className="h-3.5 w-3.5" strokeWidth={2} />
      </button>
    </>
  )
}
