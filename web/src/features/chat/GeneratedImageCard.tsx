import { Image as ImageIcon, LayoutPanelLeft, ZoomIn } from 'lucide-react'
import { SHELL } from '../shell/shellTokens'
import type { WebPreviewSeed } from '../web-preview/webPreview'

type Props = {
  src: string
  alt?: string
  onClick?: (src: string, alt: string) => void
  onOpenWebPreview?: (seed: WebPreviewSeed) => void
  chatMessageId?: string
  workspaceId?: string
  imageIndex?: number
}

const btnClass =
  'inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium text-shell-muted transition hover:bg-shell-border/50 hover:text-shell-text'

export function GeneratedImageCard({
  src,
  alt = '',
  onClick,
  onOpenWebPreview,
  chatMessageId,
  workspaceId,
  imageIndex = 0,
}: Props) {
  const label = alt || 'Generated image'
  const openPlayground = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    if (!onOpenWebPreview || !src.trim()) return
    const linkId =
      chatMessageId && imageIndex > 0
        ? `${chatMessageId}#img${imageIndex + 1}`
        : chatMessageId
    onOpenWebPreview({
      mode: 'image',
      imageUrl: src,
      imageCaption: label,
      title: label,
      prefer: 'edit',
      nonce: Date.now(),
      chatMessageId: linkId,
      scopeId: workspaceId || undefined,
    })
  }

  return (
    <div
      data-testid="chat-generated-image"
      data-src={src.slice(0, 64)}
      className="group/img my-1.5 flex w-full max-w-sm flex-col overflow-hidden rounded-lg border border-shell-border bg-shell-panel/50 text-left transition hover:border-shell-accent/50 hover:bg-shell-panel"
    >
      <button
        type="button"
        title="Click to preview"
        onClick={() => onClick?.(src, label)}
        className="relative flex max-h-72 min-h-[8rem] w-full items-center justify-center bg-[#0b1016] p-2"
      >
        <img
          src={src}
          alt={label}
          className="max-h-64 max-w-full object-contain"
          loading="lazy"
          decoding="async"
        />
        <span className="pointer-events-none absolute right-1.5 top-1.5 inline-flex items-center gap-0.5 rounded bg-black/55 px-1.5 py-0.5 text-[10px] text-white opacity-0 transition group-hover/img:opacity-100">
          <ZoomIn size={SHELL.iconXs} aria-hidden />
          Preview
        </span>
      </button>
      <div className="flex items-center justify-between gap-1.5 border-t border-shell-border/60 px-2 py-1">
        <span className="flex min-w-0 items-center gap-1.5 text-[10px] text-shell-muted">
          <ImageIcon
            size={SHELL.iconXs}
            className="shrink-0 text-shell-accent"
            aria-hidden
          />
          <span className="truncate">{label}</span>
        </span>
        {onOpenWebPreview ? (
          <button
            type="button"
            data-testid="chat-image-open-playground"
            onMouseDown={(e) => e.stopPropagation()}
            onClick={openPlayground}
            className={btnClass}
            title={`Open in playground · ${label}`}
          >
            <LayoutPanelLeft size={10} aria-hidden />
            Open in playground
          </button>
        ) : null}
      </div>
    </div>
  )
}

export function GeneratedImageCards({
  images,
  onClick,
  onOpenWebPreview,
  chatMessageId,
  workspaceId,
}: {
  images: { src: string; alt: string }[]
  onClick?: (src: string, alt: string) => void
  onOpenWebPreview?: (seed: WebPreviewSeed) => void
  chatMessageId?: string
  workspaceId?: string
}) {
  if (!images.length) return null
  return (
    <div data-testid="chat-generated-images" className="mt-1 flex flex-col gap-1.5">
      {images.map((img, i) => (
        <GeneratedImageCard
          key={`${img.alt || 'img'}-${i}`}
          src={img.src}
          alt={img.alt || `generated-${i + 1}`}
          onClick={onClick}
          onOpenWebPreview={onOpenWebPreview}
          chatMessageId={chatMessageId}
          workspaceId={workspaceId}
          imageIndex={i}
        />
      ))}
    </div>
  )
}
