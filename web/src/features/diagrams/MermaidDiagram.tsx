import { useCallback, useEffect, useId, useRef, useState } from 'react'
import {
  ChevronDown,
  Code2,
  Download,
  Eye,
  EyeOff,
  LayoutPanelLeft,
  Palette,
  Maximize2,
  Save,
  Upload,
  ZoomIn,
  ZoomOut,
} from 'lucide-react'
import { CodeEditor } from '../editor/CodeEditor'
import { pickTextFile } from '../canvas-gallery/docPanelChrome'
import { VerticalSplitter } from '../shell/VerticalSplitter'
import { useTheme } from '../theme/ThemeProvider'
import { useLocale } from '../i18n/LocaleProvider'
import {
  isLikelyIncompleteMermaid,
  mermaidJpgBg,
  mermaidStageBg,
  mermaidThemeFromAppTheme,
  normalizeMermaidSource,
  renderMermaidSvg,
  type MermaidThemeId,
} from './mermaidRender'

type Props = {
  source: string
  testId?: string
  className?: string
  showSourceFallback?: boolean
  downloadName?: string
  fill?: boolean
  flush?: boolean
  headerTitle?: string
  onSourceChange?: (source: string) => void
  onSourceSave?: (source: string) => void
  editorPath?: string
  onOpenInCanvas?: () => void
  openInCanvasLabel?: string
  dirty?: boolean
  saveFlash?: boolean
  onSaveClick?: () => void
  saveLabel?: string
  unsavedLabel?: string
  savedLabel?: string
  saveTitle?: string
}

type ExportKind = 'svg' | 'png' | 'jpg'
type MenuId = 'style' | 'download' | null

const ZOOM_STEP = 0.2

const STYLE_OPTIONS: {
  id: MermaidThemeId
  label: string
  hint: string
}[] = [
  {
    id: 'dark',
    label: 'App dark',
    hint: 'Matches dark UI theme',
  },
  {
    id: 'warm',
    label: 'App warm',
    hint: 'Matches warm UI theme',
  },
  {
    id: 'print',
    label: 'Print B&W',
    hint: 'White bg, black lines (light theme / thesis)',
  },
]

function downloadBlob(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

function downloadSvg(svg: string, name: string) {
  downloadBlob(
    new Blob([svg], { type: 'image/svg+xml;charset=utf-8' }),
    name.endsWith('.svg') ? name : `${name}.svg`,
  )
}

function ensureSvgXml(svg: string): string {
  let s = (svg || '').trim()
  if (!s) return s
  if (!/^<\?xml/i.test(s) && !s.includes('xmlns=')) {
    s = s.replace(
      /<svg\b([^>]*)>/i,
      '<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"$1>',
    )
  } else if (!s.includes('xmlns=')) {
    s = s.replace(/<svg\b/, '<svg xmlns="http://www.w3.org/2000/svg"')
  }
  return s
}

function measureSvgEl(el: SVGSVGElement): { w: number; h: number } {
  const vb = el.viewBox?.baseVal
  if (vb && vb.width > 0 && vb.height > 0) {
    return { w: vb.width, h: vb.height }
  }
  try {
    const b = el.getBBox()
    if (b.width > 0 && b.height > 0) {
      return { w: Math.ceil(b.width + 16), h: Math.ceil(b.height + 16) }
    }
  } catch {
    void 0
  }
  const w =
    Number.parseFloat(el.getAttribute('width') || '') ||
    el.clientWidth ||
    el.getBoundingClientRect().width ||
    800
  const h =
    Number.parseFloat(el.getAttribute('height') || '') ||
    el.clientHeight ||
    el.getBoundingClientRect().height ||
    600
  return { w: Math.max(1, Math.ceil(w)), h: Math.max(1, Math.ceil(h)) }
}

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const el = new Image()
    el.onload = () => resolve(el)
    el.onerror = () => reject(new Error('SVG image load failed'))
    el.src = src
  })
}

async function rasterizeFromHost(
  host: HTMLElement | null,
  svgMarkup: string,
  kind: 'png' | 'jpg',
  bg: string | null,
): Promise<Blob> {
  const scale = 2
  const live = host?.querySelector('svg') as SVGSVGElement | null
  let xml = ensureSvgXml(svgMarkup)
  let w = 800
  let h = 600

  if (live) {
    const size = measureSvgEl(live)
    w = size.w
    h = size.h
    const clone = live.cloneNode(true) as SVGSVGElement
    if (!clone.getAttribute('xmlns')) {
      clone.setAttribute('xmlns', 'http://www.w3.org/2000/svg')
    }
    if (!clone.getAttribute('viewBox') && w > 0 && h > 0) {
      clone.setAttribute('viewBox', `0 0 ${w} ${h}`)
    }
    clone.setAttribute('width', String(w))
    clone.setAttribute('height', String(h))
    xml = new XMLSerializer().serializeToString(clone)
    if (!xml.includes('xmlns=')) {
      xml = ensureSvgXml(xml)
    }
  } else {
    const vb = /viewBox="\s*([\d.]+)\s+([\d.]+)\s+([\d.]+)\s+([\d.]+)\s*"/i.exec(
      xml,
    )
    if (vb) {
      w = Math.max(1, Math.ceil(Number(vb[3])))
      h = Math.max(1, Math.ceil(Number(vb[4])))
    }
  }

  const canvas = document.createElement('canvas')
  canvas.width = Math.max(1, Math.ceil(w * scale))
  canvas.height = Math.max(1, Math.ceil(h * scale))
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('Canvas unavailable')

  if (bg) {
    ctx.fillStyle = bg
    ctx.fillRect(0, 0, canvas.width, canvas.height)
  } else {
    ctx.clearRect(0, 0, canvas.width, canvas.height)
  }

  const dataUrl = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(xml)}`
  let img: HTMLImageElement
  try {
    img = await loadImage(dataUrl)
  } catch {
    const blobUrl = URL.createObjectURL(
      new Blob([xml], { type: 'image/svg+xml;charset=utf-8' }),
    )
    try {
      img = await loadImage(blobUrl)
    } finally {
      URL.revokeObjectURL(blobUrl)
    }
  }

  ctx.drawImage(img, 0, 0, canvas.width, canvas.height)

  const mime = kind === 'jpg' ? 'image/jpeg' : 'image/png'
  const quality = kind === 'jpg' ? 0.92 : undefined
  const blob = await new Promise<Blob>((resolve, reject) => {
    canvas.toBlob(
      (b) => (b ? resolve(b) : reject(new Error('Export failed'))),
      mime,
      quality,
    )
  })
  return blob
}

const toolH = 'h-6'
const segIdle =
  `inline-flex ${toolH} min-w-[1.5rem] items-center justify-center px-1.5 text-shell-muted transition hover:text-shell-text disabled:opacity-35`
const segActive =
  `inline-flex ${toolH} min-w-[1.5rem] items-center justify-center bg-shell-active px-1.5 text-shell-accent`

export function MermaidDiagram({
  source,
  testId = 'mermaid-diagram',
  className = '',
  showSourceFallback = true,
  downloadName = 'diagram.svg',
  fill = false,
  flush = false,
  headerTitle,
  onSourceChange,
  onSourceSave,
  editorPath,
  onOpenInCanvas,
  openInCanvasLabel = 'Open in playground',
  dirty = false,
  saveFlash = false,
  onSaveClick,
  saveLabel = 'Save',
  unsavedLabel = 'Unsaved',
  savedLabel = 'Saved',
  saveTitle,
}: Props) {
  const { theme: appTheme } = useTheme()
  const { t } = useLocale()
  const reactId = useId().replace(/:/g, '')
  const editable = typeof onSourceChange === 'function'
  const [draft, setDraft] = useState(source)
  const code = normalizeMermaidSource(editable ? draft : source)
  const rootRef = useRef<HTMLDivElement | null>(null)
  const [svg, setSvg] = useState('')
  const [status, setStatus] = useState<'idle' | 'loading' | 'ready' | 'error'>(
    'idle',
  )
  const [error, setError] = useState<string | null>(null)
  const [zoom, setZoom] = useState(1)
  const [fitView, setFitView] = useState(true)
  const [renderSize, setRenderSize] = useState({ w: 800, h: 600 })
  const [viewportSize, setViewportSize] = useState({ w: 800, h: 600 })
  const previewRef = useRef<HTMLDivElement | null>(null)
  const [exportError, setExportError] = useState<string | null>(null)
  const [showPreview, setShowPreview] = useState(true)
  const [showCode, setShowCode] = useState(() => editable)
  const [editorWidth, setEditorWidth] = useState(380)
  const splitRef = useRef<HTMLDivElement | null>(null)
  const [narrow, setNarrow] = useState(false)
  const [theme, setTheme] = useState<MermaidThemeId>(() =>
    mermaidThemeFromAppTheme(appTheme),
  )
  const [menu, setMenu] = useState<MenuId>(null)
  const [exporting, setExporting] = useState(false)

  const draftRef = useRef(draft)
  draftRef.current = draft
  const dirtyRef = useRef(false)
  const pathRef = useRef(editorPath)
  const onSourceChangeRef = useRef(onSourceChange)
  const onSourceSaveRef = useRef(onSourceSave)
  onSourceChangeRef.current = onSourceChange
  onSourceSaveRef.current = onSourceSave

  useEffect(() => {
    if (pathRef.current !== editorPath) {
      pathRef.current = editorPath
      dirtyRef.current = false
      setDraft(source)
      draftRef.current = source
      return
    }
    if (!dirtyRef.current) {
      setDraft(source)
      draftRef.current = source
    }
  }, [editorPath, source])

  const toggleCode = () => {
    setShowCode((v) => {
      if (v && !showPreview) return true
      return !v
    })
  }
  const togglePreview = () => {
    setShowPreview((v) => {
      if (v && !showCode) return true
      return !v
    })
  }

  useEffect(() => {
    const el = splitRef.current
    if (!el) return
    const apply = (w: number) => {
      if (w <= 0) return
      const next = w < 720
      setNarrow((prev) => (prev === next ? prev : next))
    }
    if (typeof ResizeObserver === 'undefined') {
      apply(el.clientWidth)
      return
    }
    const ro = new ResizeObserver((entries) => {
      const cr = entries[0]?.contentRect
      if (cr) apply(cr.width)
    })
    ro.observe(el)
    apply(el.clientWidth)
    return () => ro.disconnect()
  }, [])

  useEffect(() => {
    const viewport = previewRef.current
    const diagram = viewport?.querySelector('svg')
    if (!viewport || !diagram) return
    setRenderSize(measureSvgEl(diagram))
    const measure = () => setViewportSize({ w: viewport.clientWidth, h: viewport.clientHeight })
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(viewport)
    return () => observer.disconnect()
  }, [svg, showPreview])

  useEffect(() => {
    setTheme(mermaidThemeFromAppTheme(appTheme))
  }, [appTheme])

  useEffect(() => {
    let cancelled = false
    setZoom(1)
    setFitView(true)
    setExportError(null)
    if (!code) {
      setSvg('')
      setStatus('idle')
      setError(null)
      return
    }
    if (isLikelyIncompleteMermaid(code)) {
      setSvg('')
      setStatus('loading')
      setError(null)
      return
    }

    setStatus('loading')
    setError(null)
    const t = window.setTimeout(() => {
      void renderMermaidSvg(code, theme).then((res) => {
        if (cancelled) return
        if (res.error === 'incomplete') {
          setSvg('')
          setStatus('loading')
          setError(null)
          return
        }
        if (res.error || !res.svg) {
          setSvg('')
          setStatus('error')
          setError(res.error || 'Render failed')
          return
        }
        setSvg(res.svg)
        setStatus('ready')
        setError(null)
      })
    }, 80)

    return () => {
      cancelled = true
      window.clearTimeout(t)
    }
  }, [code, theme])

  useEffect(() => {
    if (!menu) return
    const onDown = (e: MouseEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setMenu(null)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setMenu(null)
    }
    window.addEventListener('mousedown', onDown)
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('mousedown', onDown)
      window.removeEventListener('keydown', onKey)
    }
  }, [menu])

  const fitScale = Math.min(1, Math.max(0.05, (viewportSize.w - 24) / renderSize.w), Math.max(0.05, (viewportSize.h - 24) / renderSize.h))
  const displayZoom = fitView ? fitScale : zoom
  const zoomIn = () => {
    setZoom(Math.min(4, Math.round((displayZoom + ZOOM_STEP) * 100) / 100))
    setFitView(false)
  }
  const zoomOut = () => {
    setZoom(Math.max(0.05, Math.round((displayZoom - ZOOM_STEP) * 100) / 100))
    setFitView(false)
  }

  const exportAs = useCallback(
    async (kind: ExportKind) => {
      if (!svg || exporting) return
      setMenu(null)
      const base = downloadName.replace(/\.(svg|png|jpe?g)$/i, '')
      const themeTag =
        theme === 'print' ? '-print' : theme === 'warm' ? '-warm' : ''
      setExportError(null)
      try {
        if (kind === 'svg') {
          downloadSvg(svg, `${base}${themeTag}.svg`)
          return
        }
        setExporting(true)
        const host = rootRef.current?.querySelector(
          `[data-testid="${testId}-svg"]`,
        ) as HTMLElement | null
        const bg = kind === 'jpg' ? mermaidJpgBg(theme) : null
        const blob = await rasterizeFromHost(host, svg, kind, bg)
        downloadBlob(blob, `${base}${themeTag}.${kind}`)
      } catch (e) {
        setExportError(e instanceof Error ? e.message : String(e))
      } finally {
        setExporting(false)
      }
    },
    [downloadName, exporting, svg, testId, theme],
  )

  const importSource = useCallback(async () => {
    if (!editable) return
    setMenu(null)
    setExportError(null)
    const picked = await pickTextFile('.mmd,.mermaid,.txt,text/plain')
    if (!picked) return
    if (picked.error) {
      setExportError(picked.error)
      return
    }
    const next = picked.text.replace(/^\uFEFF/, '').trim()
    if (!next) {
      setExportError(`${picked.name} is empty`)
      return
    }
    // The file replaces the editor draft; the user reviews it and Saves, so a
    // bad import never overwrites the stored diagram on its own.
    dirtyRef.current = true
    draftRef.current = next
    setDraft(next)
    onSourceChangeRef.current?.(next)
  }, [editable])

  const canPreview = status === 'ready' && !!svg
  const stageBgCss = mermaidStageBg(theme)
  const styleLabel =
    STYLE_OPTIONS.find((o) => o.id === theme)?.label || 'Style'
  const split = showCode && showPreview
  const stacked = split && narrow

  const shellClass = flush
    ? 'my-0 flex h-full min-h-0 flex-col overflow-hidden rounded-none border-0 bg-transparent'
    : fill
      ? 'my-0 flex h-full min-h-0 flex-col overflow-hidden rounded-md border border-shell-border bg-shell-panel'
      : 'my-1.5 block overflow-hidden rounded-md border border-shell-border bg-shell-panel'

  return (
    <div
      ref={rootRef}
      data-testid={testId}
      data-mermaid-status={status}
      data-mermaid-id={reactId}
      data-mermaid-theme={theme}
      data-mermaid-preview={showPreview ? 'true' : 'false'}
      data-mermaid-code={showCode ? 'true' : 'false'}
      data-mermaid-stacked={stacked ? 'true' : 'false'}
      className={`mermaid-diagram relative ${shellClass} ${className}`}
    >
      <div className="flex min-h-10 shrink-0 flex-wrap items-center gap-1.5 border-b border-shell-border bg-shell-panel px-2 py-1">
        <span
          className={`min-w-0 flex-1 truncate text-[11px] font-semibold tracking-wide ${
            headerTitle
              ? 'normal-case text-shell-text'
              : 'uppercase text-shell-accent text-[10px]'
          }`}
          title={headerTitle || 'Mermaid diagram'}
        >
          {headerTitle || 'Mermaid diagram'}
        </span>
        <div className="ml-auto flex min-w-0 flex-wrap items-center justify-end gap-1.5">
          {status === 'loading' && showPreview && (
            <span
              data-testid={`${testId}-loading`}
              className="text-[10px] text-shell-muted"
            >
              Rendering…
            </span>
          )}
          {status === 'error' && showPreview && (
            <span
              data-testid={`${testId}-error`}
              role="status"
              className="max-w-[7rem] truncate text-[10px] text-shell-muted"
              title={error || ''}
            >
              Preview failed
            </span>
          )}

          <div
            data-testid={`${testId}-zoom-group`}
            className={`inline-flex ${toolH} items-center overflow-hidden rounded-md border border-shell-border bg-shell-bg`}
          >
            <button
              type="button"
              data-testid={`${testId}-zoom-out`}
              className={segIdle}
              disabled={!canPreview || !showPreview}
              title="Zoom out"
              aria-label="Zoom out"
              onClick={zoomOut}
            >
              <ZoomOut size={12} strokeWidth={2} aria-hidden />
            </button>
            <button
              type="button"
              data-testid={`${testId}-zoom-label`}
              className={`inline-flex ${toolH} min-w-[2.4rem] items-center justify-center border-x border-shell-border px-1 font-mono text-[9px] tabular-nums text-shell-muted hover:text-shell-text`}
              title={t('playground.actualSize')}
              disabled={!canPreview || !showPreview}
              onClick={() => { setZoom(1); setFitView(false) }}
            >
              {Math.round(displayZoom * 100)}%
            </button>
            <button
              type="button"
              data-testid={`${testId}-zoom-in`}
              className={segIdle}
              disabled={!canPreview || !showPreview}
              title="Zoom in"
              aria-label="Zoom in"
              onClick={zoomIn}
            >
              <ZoomIn size={12} strokeWidth={2} aria-hidden />
            </button>
            <button
              type="button"
              data-testid={`${testId}-fit`}
              className={`${fitView ? segActive : segIdle} border-l border-shell-border`}
              disabled={!canPreview || !showPreview}
              aria-pressed={fitView}
              title={t('playground.fitView')}
              aria-label={t('playground.fitView')}
              onClick={() => setFitView(true)}
            >
              <Maximize2 size={12} aria-hidden />
            </button>
          </div>

          <div className="relative">
            <button
              type="button"
              data-testid={`${testId}-style-btn`}
              className={`inline-flex ${toolH} items-center gap-1 rounded-md border border-shell-border bg-shell-bg px-1.5 text-[10px] text-shell-muted transition hover:text-shell-text`}
              title="Diagram style"
              aria-haspopup="menu"
              aria-expanded={menu === 'style'}
              onClick={() => setMenu((m) => (m === 'style' ? null : 'style'))}
            >
              <Palette size={11} strokeWidth={2} aria-hidden />
              <span className="max-w-[4.5rem] truncate">{styleLabel}</span>
              <ChevronDown size={10} aria-hidden />
            </button>
            {menu === 'style' && (
              <div
                data-testid={`${testId}-style-menu`}
                role="menu"
                className="absolute right-0 top-full z-30 mt-1 min-w-[11rem] overflow-hidden rounded-md border border-shell-border bg-shell-panel py-0.5 shadow-lg shadow-black/30"
              >
                {STYLE_OPTIONS.map((opt) => (
                  <button
                    key={opt.id}
                    type="button"
                    role="menuitemradio"
                    aria-checked={theme === opt.id}
                    data-testid={`${testId}-style-${opt.id}`}
                    className={`flex w-full flex-col items-start px-2.5 py-1.5 text-left hover:bg-shell-hover ${
                      theme === opt.id ? 'bg-shell-active' : ''
                    }`}
                    onClick={() => {
                      setTheme(opt.id)
                      setMenu(null)
                    }}
                  >
                    <span className="text-[11px] font-medium text-shell-text">
                      {opt.label}
                    </span>
                    <span className="text-[9px] text-shell-muted">{opt.hint}</span>
                  </button>
                ))}
              </div>
            )}
          </div>

          <div className="relative">
            <button
              type="button"
              data-testid={`${testId}-download`}
              className={`inline-flex ${toolH} items-center gap-1 rounded-md border border-shell-border bg-shell-bg px-1.5 text-shell-muted transition hover:text-shell-text disabled:opacity-35`}
              disabled={!canPreview || exporting}
              title="Download"
              aria-haspopup="menu"
              aria-expanded={menu === 'download'}
              onClick={() =>
                setMenu((m) => (m === 'download' ? null : 'download'))
              }
            >
              <Download size={12} strokeWidth={2} aria-hidden />
              <ChevronDown size={10} aria-hidden />
            </button>
            {menu === 'download' && (
              <div
                data-testid={`${testId}-download-menu`}
                role="menu"
                className="absolute right-0 top-full z-30 mt-1 min-w-[12.5rem] overflow-hidden rounded-md border border-shell-border bg-shell-panel py-0.5 shadow-lg shadow-black/30"
              >
                <button
                  type="button"
                  role="menuitem"
                  data-testid={`${testId}-download-svg`}
                  className="flex w-full flex-col items-start px-2.5 py-1.5 text-left hover:bg-shell-hover"
                  onClick={() => void exportAs('svg')}
                >
                  <span className="text-[11px] font-medium text-shell-text">
                    SVG
                  </span>
                  <span className="text-[9px] text-shell-muted">
                    Vector, scalable
                  </span>
                </button>
                <button
                  type="button"
                  role="menuitem"
                  data-testid={`${testId}-download-png`}
                  className="flex w-full flex-col items-start px-2.5 py-1.5 text-left hover:bg-shell-hover"
                  onClick={() => void exportAs('png')}
                >
                  <span className="text-[11px] font-medium text-shell-text">
                    PNG
                  </span>
                  <span className="text-[9px] text-shell-muted">
                    Transparent background
                  </span>
                </button>
                <button
                  type="button"
                  role="menuitem"
                  data-testid={`${testId}-download-jpg`}
                  className="flex w-full flex-col items-start px-2.5 py-1.5 text-left hover:bg-shell-hover"
                  onClick={() => void exportAs('jpg')}
                >
                  <span className="text-[11px] font-medium text-shell-text">
                    JPG
                  </span>
                  <span className="text-[9px] text-shell-muted">
                    Solid background (print-ready)
                  </span>
                </button>
              </div>
            )}
          </div>


          {editable && (
            <button
              type="button"
              data-testid={`${testId}-import-source`}
              title={t('docPanel.import')}
              aria-label={t('docPanel.import')}
              className={`inline-flex ${toolH} items-center gap-1 rounded-md border border-shell-border px-1.5 text-[10px] text-shell-muted hover:text-shell-text`}
              onClick={() => void importSource()}
            >
              <Upload size={11} strokeWidth={2} aria-hidden />
              .mmd
            </button>
          )}
          <button
            type="button"
            data-testid={`${testId}-download-source`}
            disabled={!code}
            title={t('playground.downloadSource')}
            className={`inline-flex ${toolH} items-center rounded-md border border-shell-border px-1.5 text-[10px] text-shell-muted hover:text-shell-text disabled:opacity-40`}
            onClick={() => downloadBlob(new Blob([code], { type: 'text/plain;charset=utf-8' }), `${downloadName.replace(/\.(svg|png|jpe?g|mmd)$/i, '')}.mmd`)}
          >
            .mmd
          </button>
          <div
            data-testid={`${testId}-view-switch`}
            className={`inline-flex ${toolH} overflow-hidden rounded-md border border-shell-border bg-shell-bg`}
            role="group"
            aria-label="Preview and code"
          >
            <button
              type="button"
              data-testid={`${testId}-view-preview`}
              data-active={showPreview ? 'true' : 'false'}
              className={`${showPreview ? segActive : segIdle} border-r border-shell-border`}
              title="Preview"
              aria-label="Preview"
              aria-pressed={showPreview}
              onClick={togglePreview}
            >
              {showPreview ? (
                <Eye size={12} strokeWidth={2} aria-hidden />
              ) : (
                <EyeOff size={12} strokeWidth={2} aria-hidden />
              )}
            </button>
            <button
              type="button"
              data-testid={`${testId}-view-code`}
              data-active={showCode ? 'true' : 'false'}
              className={showCode ? segActive : segIdle}
              title="Source code"
              aria-label="Source code"
              aria-pressed={showCode}
              onClick={toggleCode}
            >
              <Code2 size={12} strokeWidth={2} aria-hidden />
            </button>
          </div>
          {onOpenInCanvas && (
            <button
              type="button"
              data-testid={`${testId}-open-canvas`}
              onMouseDown={(e) => e.stopPropagation()}
              onClick={(e) => {
                e.preventDefault()
                e.stopPropagation()
                onOpenInCanvas()
              }}
              title={openInCanvasLabel}
              className={`inline-flex ${toolH} items-center gap-1 rounded-md border border-shell-border bg-shell-bg px-1.5 text-[10px] text-shell-muted transition hover:text-shell-text`}
            >
              <LayoutPanelLeft size={11} strokeWidth={2} aria-hidden />
              <span className="hidden sm:inline">{openInCanvasLabel}</span>
            </button>
          )}
        </div>
      </div>
      {exportError && (
        <p role="alert" data-testid={`${testId}-export-error`} className="shrink-0 border-b border-shell-border px-3 py-2 text-[11px] text-shell-text">
          {t('playground.exportFailed')}: {exportError}
        </p>
      )}

      <div
        ref={splitRef}
        className={`shell-scroll flex min-h-0 min-w-0 overflow-hidden ${
          stacked ? 'flex-col' : 'flex-row'
        } ${fill ? 'flex-1' : 'min-h-[12rem]'}`}
      >
        {showCode && (
          <>
            <div
              data-testid={`${testId}-code-pane`}
              style={
                stacked
                  ? { flex: '0 0 auto', height: '45%', minHeight: 132 }
                  : split
                    ? { width: editorWidth, flex: 'none' }
                    : { flex: '1 1 auto', minWidth: 0 }
              }
              className={`flex min-h-0 min-w-0 flex-col bg-shell-bg ${
                stacked
                  ? 'w-full border-b border-shell-border'
                  : 'min-w-[160px] border-r border-shell-border'
              }`}
            >
              {editable ? (
                <div
                  data-testid={`${testId}-code-editor`}
                  className="flex min-h-0 flex-1 flex-col"
                >
                  <div className="flex h-7 shrink-0 items-center justify-between gap-2 border-b border-shell-border bg-shell-panel px-2">
                    <span className="flex min-w-0 items-center gap-1.5 text-[10px] text-shell-muted">
                      {dirty && (
                        <span
                          data-testid={`${testId}-editor-dirty`}
                          className="inline-block h-1.5 w-1.5 shrink-0 rounded-full bg-shell-accent"
                          title={unsavedLabel}
                          aria-label={unsavedLabel}
                        />
                      )}
                      <span className="truncate font-mono uppercase tracking-wide">
                        Mermaid
                      </span>
                      <span
                        className={`truncate ${
                          dirty || saveFlash
                            ? 'text-shell-accent'
                            : 'text-shell-muted'
                        }`}
                      >
                        {dirty
                          ? unsavedLabel
                          : saveFlash
                            ? savedLabel
                            : saveTitle || saveLabel}
                      </span>
                    </span>
                    {onSaveClick && (
                      <button
                        type="button"
                        data-testid={`${testId}-editor-save`}
                        data-dirty={dirty ? 'true' : 'false'}
                        title={saveTitle || saveLabel}
                        aria-label={saveLabel}
                        disabled={!dirty}
                        onClick={() => onSaveClick()}
                        className={`inline-flex items-center gap-0.5 text-[10px] transition ${
                          dirty
                            ? 'text-shell-accent hover:underline'
                            : 'text-shell-muted'
                        } disabled:cursor-default disabled:no-underline`}
                      >
                        <Save size={10} strokeWidth={2} aria-hidden />
                        {saveLabel}
                      </button>
                    )}
                  </div>
                  <CodeEditor
                    path={editorPath || `${testId}.mmd`}
                    value={draft}
                    dirty={dirty}
                    onChange={(v) => {
                      dirtyRef.current = true
                      draftRef.current = v
                      setDraft(v)
                      onSourceChangeRef.current?.(v)
                    }}
                    onSave={(v) => {
                      const next = v ?? draftRef.current
                      dirtyRef.current = false
                      draftRef.current = next
                      setDraft(next)
                      onSourceSaveRef.current?.(next)
                    }}
                    testId={`${testId}-monaco`}
                    fontSize={12}
                  />
                </div>
              ) : (
                <pre
                  data-testid={`${testId}-code`}
                  className="shell-scroll m-0 min-h-0 flex-1 overflow-auto bg-[var(--shell-code-bg)] p-2.5"
                >
                  <code className="font-mono text-[11px] leading-snug text-shell-text">
                    {code || '/* empty */'}
                  </code>
                </pre>
              )}
            </div>
            {split && !stacked && (
              <VerticalSplitter
                testId={`${testId}-editor-splitter`}
                value={editorWidth}
                onChange={(w) =>
                  setEditorWidth(Math.max(160, Math.min(720, w)))
                }
                growSide="left"
                minOpposite={180}
                aria-label="Resize code editor"
              />
            )}
          </>
        )}

        {showPreview && (
          <div
            className="relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden"
            data-testid={`${testId}-preview-pane`}
          >
            {status === 'ready' && svg ? (
              <div
                ref={previewRef}
                data-testid={`${testId}-svg`}
                className="mermaid-svg-host shell-scroll flex min-h-0 flex-1 overflow-auto p-3"
                style={{ backgroundColor: stageBgCss }}
              >
                <div style={{ flexShrink: 0, width: renderSize.w * displayZoom, height: renderSize.h * displayZoom, margin: 'auto' }}>
                  <div
                    className="origin-top-left [&_svg]:block [&_svg]:max-w-none"
                    style={{ transform: `scale(${displayZoom})`, width: renderSize.w, height: renderSize.h }}
                  >
                    <div dangerouslySetInnerHTML={{ __html: svg }} />
                  </div>
                </div>
              </div>
            ) : null}

            {(status === 'loading' || status === 'idle') && !svg ? (
              <div
                data-testid={`${testId}-placeholder`}
                className="flex min-h-0 flex-1 items-center justify-center px-3 py-4 text-[11px] text-shell-muted"
                style={{ backgroundColor: stageBgCss }}
              >
                {code ? 'Building diagram…' : 'Empty diagram'}
              </div>
            ) : null}

            {status === 'error' && showSourceFallback && !showCode ? (
              <pre
                data-testid={`${testId}-source`}
                className="m-0 min-h-0 flex-1 overflow-auto bg-shell-bg p-2.5"
              >
                <code className="font-mono text-[11px] leading-snug text-shell-text">
                  {code}
                </code>
              </pre>
            ) : null}

            {status === 'error' && error ? (
              <p role="alert" className="max-h-32 shrink-0 overflow-auto whitespace-pre-wrap border-t border-shell-border px-2.5 py-2 text-[11px] text-shell-text">
                {error}
              </p>
            ) : null}
          </div>
        )}
      </div>
    </div>
  )
}
