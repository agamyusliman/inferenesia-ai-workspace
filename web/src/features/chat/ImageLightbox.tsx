import { useEffect } from 'react'
import { Download, X, ZoomIn } from 'lucide-react'
import { imageDownloadFilename } from './messageActions'
import { SHELL } from '../shell/shellTokens'

type Props = {
  src: string
  alt?: string
  sessionName?: string
  onClose: () => void
}

export function ImageLightbox({ src, alt, sessionName, onClose }: Props) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        e.stopPropagation()
        onClose()
      }
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [onClose])

  const onDownload = () => {
    const a = document.createElement('a')
    a.href = src
    a.download = imageDownloadFilename(src, alt, 0, sessionName)
    a.rel = 'noopener'
    a.target = '_blank'
    document.body.appendChild(a)
    a.click()
    a.remove()
  }

  return (
    <div
      data-testid="chat-image-lightbox"
      role="dialog"
      aria-modal="true"
      aria-label={alt || 'Image preview'}
      className="fixed inset-0 z-[80] flex items-center justify-center bg-black/75 p-4 backdrop-blur-[2px] transition-opacity duration-150"
      onClick={onClose}
    >
      <div
        className="relative max-h-[90vh] max-w-[min(960px,95vw)]"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-2 flex items-center justify-between gap-2 rounded-t-lg border border-b-0 border-shell-border bg-shell-panel px-3 py-1.5">
          <span className="flex min-w-0 items-center gap-1.5 truncate text-[11px] text-shell-muted">
            <ZoomIn size={SHELL.iconXs} className="shrink-0 text-shell-accent" aria-hidden />
            <span className="truncate" title={alt || src}>
              {alt || 'Image preview'}
            </span>
          </span>
          <div className="flex shrink-0 items-center gap-1">
            <button
              type="button"
              data-testid="chat-image-lightbox-download"
              onClick={onDownload}
              className="inline-flex items-center gap-1 rounded px-1.5 py-1 text-[10px] text-shell-muted transition hover:bg-shell-border/50 hover:text-shell-text"
              title="Download image"
            >
              <Download size={SHELL.iconXs} />
              Download
            </button>
            <button
              type="button"
              data-testid="chat-image-lightbox-close"
              onClick={onClose}
              className="rounded p-1 text-shell-muted transition hover:bg-shell-border/50 hover:text-shell-text"
              aria-label="Close preview"
            >
              <X size={SHELL.iconSm} />
            </button>
          </div>
        </div>
        <img
          data-testid="chat-image-lightbox-img"
          src={src}
          alt={alt || 'Preview'}
          className="max-h-[80vh] max-w-full rounded-b-lg border border-shell-border bg-shell-bg object-contain shadow-xl"
        />
      </div>
    </div>
  )
}
