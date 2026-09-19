import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { Monitor, Smartphone, Tablet, Laptop, X, RotateCw } from 'lucide-react'
import { buildPreviewSrcDoc } from '../web-preview/webPreview'
import {
  DEVICE_PRESETS,
  type DeviceId,
  type ScreenOrientation,
  defaultOrientation,
  frameSize,
  getDevicePreset,
  scaleToFit,
  toggleOrientation,
} from '../web-preview/devicePresets'
import { SHELL } from '../shell/shellTokens'

type Props = {
  html: string
  path?: string
  onClose: () => void
}

const ICONS: Record<DeviceId, typeof Smartphone> = {
  mobile: Smartphone,
  tablet: Tablet,
  laptop: Laptop,
  desktop: Monitor,
}

export function HtmlCanvasModal({ html, path, onClose }: Props) {
  const [deviceId, setDeviceId] = useState<DeviceId>('desktop')
  const [orientation, setOrientation] = useState<ScreenOrientation>(
    defaultOrientation('desktop'),
  )
  const device = getDevicePreset(deviceId)
  const frame = frameSize(device, orientation)
  const stageW =
    typeof window !== 'undefined' ? Math.min(window.innerWidth - 64, 1100) : 900
  const stageH =
    typeof window !== 'undefined' ? Math.min(window.innerHeight - 140, 720) : 560
  const scale = scaleToFit(frame.width, frame.height, stageW, stageH, 16)
  const srcDoc = buildPreviewSrcDoc(html)

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

  const node = (
    <div
      data-testid="chat-html-canvas-modal"
      role="dialog"
      aria-modal="true"
      aria-label="HTML canvas preview"
      className="fixed inset-0 z-[200] flex items-center justify-center bg-black/75 p-3 backdrop-blur-[1px] sm:p-5"
    >
      <div
        className="flex max-h-[min(94vh,920px)] w-full max-w-6xl flex-col overflow-hidden rounded-lg border border-shell-border bg-shell-panel shadow-2xl"
        role="document"
      >
        <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-shell-border px-3 py-2">
          <div className="min-w-0 flex-1">
            <h2 className="truncate text-sm font-medium text-shell-text">
              Preview
            </h2>
            <p className="truncate text-[10px] text-shell-muted">
              {path || 'HTML'} · {frame.width}×{frame.height} · {orientation}
            </p>
          </div>
          <div className="flex items-center gap-0.5 rounded-md border border-shell-border/80 p-0.5">
            {DEVICE_PRESETS.map((d) => {
              const Icon = ICONS[d.id]
              const active = deviceId === d.id
              return (
                <button
                  key={d.id}
                  type="button"
                  data-testid={`canvas-modal-device-${d.id}`}
                  data-active={active ? 'true' : 'false'}
                  title={d.label}
                  onClick={() => {
                    setDeviceId(d.id)
                    setOrientation(defaultOrientation(d.id))
                  }}
                  className={`rounded p-1.5 transition ${
                    active
                      ? 'bg-shell-active text-shell-text'
                      : 'text-shell-muted hover:bg-shell-border/30 hover:text-shell-text'
                  }`}
                >
                  <Icon size={SHELL.iconSm} strokeWidth={1.75} />
                </button>
              )
            })}
            <button
              type="button"
              data-testid="canvas-modal-rotate"
              title="Rotate"
              onClick={() => setOrientation((o) => toggleOrientation(o))}
              className="rounded p-1.5 text-shell-muted transition hover:bg-shell-border/30 hover:text-shell-text"
            >
              <RotateCw size={SHELL.iconSm} strokeWidth={1.75} />
            </button>
          </div>
          <button
            type="button"
            data-testid="canvas-modal-close"
            onClick={onClose}
            className="rounded p-1 text-shell-muted hover:bg-shell-border/40 hover:text-shell-text"
            aria-label="Close"
          >
            <X size={SHELL.iconSm} />
          </button>
        </div>
        <div
          data-testid="canvas-modal-stage"
          className="flex min-h-0 flex-1 items-center justify-center overflow-auto bg-[radial-gradient(ellipse_at_center,_var(--tw-gradient-stops))] from-shell-panel via-shell-bg to-shell-bg p-4"
        >
          <div
            style={{
              width: frame.width * scale,
              height: frame.height * scale,
            }}
            className="relative shrink-0"
          >
            <div
              className="absolute left-0 top-0 origin-top-left overflow-hidden rounded-lg border border-shell-border bg-white shadow-2xl"
              style={{
                width: frame.width,
                height: frame.height,
                transform: `scale(${scale})`,
              }}
            >
              <iframe
                data-testid="canvas-modal-iframe"
                title="HTML canvas"
                srcDoc={srcDoc}
                sandbox="allow-scripts allow-forms allow-modals"
                className="h-full w-full border-0 bg-white"
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  )

  if (typeof document === 'undefined') return node
  return createPortal(node, document.body)
}
