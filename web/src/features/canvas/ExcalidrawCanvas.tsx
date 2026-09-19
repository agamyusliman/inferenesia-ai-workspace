import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ComponentType,
  type ReactNode,
} from 'react'
import {
  Download,
  FileDown,
  Image as ImageIcon,
  Maximize2,
  Upload,
} from 'lucide-react'
import {
  parseExcalidrawContent,
  serializeExcalidraw,
  toExcalidrawInitialData,
  type ExcalidrawDocumentData,
} from './excalidrawDoc'
import { CanvasAIBar } from './CanvasAIBar'
import { useTheme } from '../theme/ThemeProvider'
import { canvasThemeFor } from '../theme/theme'
import { ConfirmDialog } from '../explorer/ConfirmDialog'
import { useLocale } from '../i18n/LocaleProvider'

import '@excalidraw/excalidraw/index.css'

/* ------------------------------------------------------------------ */
/* Excalidraw type stubs (typed loosely to avoid coupling to internal  */
/* types that the package re-exports under different paths per minor). */
/* ------------------------------------------------------------------ */

type ExcalidrawOnChange = (
  elements: readonly unknown[],
  appState: Record<string, unknown>,
  files: Record<string, unknown>,
) => void

type CanvasActionsOpts = {
  changeViewBackgroundColor?: boolean
  clearCanvas?: boolean
  export?: boolean | { saveFileToDisk?: boolean }
  loadScene?: boolean
  saveToActiveFile?: boolean
  toggleTheme?: boolean | null
  saveAsImage?: boolean
}

type ExcalidrawUIOptions = {
  canvasActions?: CanvasActionsOpts
  tools?: { image?: boolean }
  welcomeScreen?: boolean
}

/** Subset of ExcalidrawImperativeAPI we use. */
type ImperativeAPI = {
  getSceneElements: () => readonly unknown[]
  getAppState: () => Record<string, unknown>
  getFiles: () => Record<string, unknown>
  scrollToContent: (
    target?: unknown,
    opts?: { fitToViewport?: boolean; viewportZoomFactor?: number; animate?: boolean },
  ) => void
}

type ExcalidrawComponentProps = {
  initialData?: unknown
  onChange?: ExcalidrawOnChange
  theme?: 'light' | 'dark'
  name?: string
  UIOptions?: ExcalidrawUIOptions
  aiEnabled?: boolean
  zenModeEnabled?: boolean
  gridModeEnabled?: boolean
  viewModeEnabled?: boolean
  excalidrawAPI?: (api: ImperativeAPI) => void
  renderTopRightUI?: (isMobile: boolean, appState: unknown) => ReactNode
  renderSidebar?: () => ReactNode
  langCode?: string
}

// Dynamic import: Excalidraw is a 2MB+ module – lazy() requires runtime import
const LazyExcalidraw = lazy(async () => {
  const mod = await import('@excalidraw/excalidraw')
  const Comp = mod.Excalidraw as ComponentType<ExcalidrawComponentProps>
  return { default: Comp }
})

const DEBOUNCE_MS = 400

const EMBED_HIDE_CSS = `
  .excalidraw .layer-ui__wrapper__top-right .default-sidebar-trigger,
  .excalidraw .layer-ui__wrapper__top-right .sidebar-trigger,
  .excalidraw button[title="Library"],
  .excalidraw button[aria-label="Library"],
  .excalidraw button[title="Help"],
  .excalidraw button[aria-label="Help"],
  .excalidraw .help-icon,
  .excalidraw .HintViewer,
  .excalidraw .ToolIcon__keybinding + .ToolIcon_type_button[title*="Live"],
  .excalidraw button[title*="Live collaboration"],
  .excalidraw button[aria-label*="Live collaboration"],
  .excalidraw .main-menu-trigger,
  .excalidraw .search-menu-container,
  .excalidraw .welcome-screen-center,
  .excalidraw .welcome-screen-menu-hint,
  .excalidraw .welcome-screen-decor,
  .excalidraw .App-toolbar-content .encrypted-icon,
  .excalidraw .github-corner,
  .excalidraw .FixedSideContainer_side_top .Stack_horizontal > a
  {
    display: none !important;
  }
`

function baseName(path: string): string {
  const parts = path.split(/[/\\]/)
  return parts[parts.length - 1] || path
}

function stemName(path: string): string {
  const b = baseName(path)
  const dot = b.lastIndexOf('.')
  return dot > 0 ? b.slice(0, dot) : b
}

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

/* ------------------------------------------------------------------ */
/* Export helpers – loaded lazily from @excalidraw/excalidraw          */
/* ------------------------------------------------------------------ */

type ExportFns = {
  exportToBlob: (opts: {
    elements: readonly unknown[]
    appState?: Record<string, unknown>
    files: Record<string, unknown> | null
    mimeType?: string
    quality?: number
  }) => Promise<Blob>
  exportToSvg: (opts: {
    elements: readonly unknown[]
    appState?: Record<string, unknown>
    files: Record<string, unknown> | null
  }) => Promise<SVGSVGElement>
}

let _exportFns: ExportFns | null = null

// Dynamic import: export utilities share the same 2MB+ chunk – deferred until
// user actually exports to avoid loading eagerly in embed/gallery profiles.
async function getExportFns(): Promise<ExportFns> {
  if (_exportFns) return _exportFns
  const mod = await import('@excalidraw/excalidraw')
  _exportFns = {
    exportToBlob: (mod as unknown as ExportFns).exportToBlob,
    exportToSvg: (mod as unknown as ExportFns).exportToSvg,
  }
  return _exportFns
}

/* ------------------------------------------------------------------ */
/* Public types                                                       */
/* ------------------------------------------------------------------ */

export type ExcalidrawUIProfile = 'full' | 'embed' | 'present'

export type ExcalidrawCanvasProps = {
  path: string
  content: string
  onChange: (json: string) => void
  testId?: string
  chromeTitle?: string
  exportName?: string
  showAIBar?: boolean
  uiProfile?: ExcalidrawUIProfile
  hideChrome?: boolean
}

/* ------------------------------------------------------------------ */
/* UI options per profile                                             */
/* ------------------------------------------------------------------ */

function uiOptionsForProfile(
  profile: ExcalidrawUIProfile,
): ExcalidrawUIOptions {
  if (profile === 'present') {
    return {
      canvasActions: {
        changeViewBackgroundColor: false,
        clearCanvas: false,
        export: false,
        loadScene: false,
        saveToActiveFile: false,
        toggleTheme: false,
        saveAsImage: false,
      },
      tools: { image: false },
      welcomeScreen: false,
    }
  }
  if (profile === 'full') {
    return {
      canvasActions: {
        loadScene: false,
        saveToActiveFile: false,
        export: { saveFileToDisk: true },
        saveAsImage: true,
        clearCanvas: true,
        changeViewBackgroundColor: true,
        toggleTheme: false,
      },
      tools: { image: true },
      welcomeScreen: false,
    }
  }
  // embed – disable native export (we provide our own chrome)
  return {
    canvasActions: {
      loadScene: false,
      saveToActiveFile: false,
      export: false,
      saveAsImage: false,
      clearCanvas: true,
      changeViewBackgroundColor: true,
      toggleTheme: false,
    },
    tools: { image: true },
    welcomeScreen: false,
  }
}
/* ------------------------------------------------------------------ */
/* Chrome button style                                                */
/* ------------------------------------------------------------------ */

const chromeBtn =
  'inline-flex h-5 items-center gap-0.5 rounded border border-shell-border/60 bg-shell-bg/80 px-1 text-[10px] text-shell-muted transition hover:text-shell-text hover:border-shell-border disabled:opacity-35'

/* ------------------------------------------------------------------ */
/* Component                                                          */
/* ------------------------------------------------------------------ */

export function ExcalidrawCanvas({
  path,
  content,
  onChange,
  testId = 'excalidraw-canvas',
  chromeTitle = 'Canvas',
  exportName,
  showAIBar = true,
  uiProfile = 'embed',
  hideChrome = false,
}: ExcalidrawCanvasProps) {
  const { theme } = useTheme()
  const { t } = useLocale()
  const excalidrawTheme = canvasThemeFor(theme)
  const lastSerializedRef = useRef<string>('')
  const onChangeRef = useRef(onChange)
  onChangeRef.current = onChange
  const apiRef = useRef<ImperativeAPI | null>(null)

  /* ---- live state refs (always current, never stale in closures) -- */
  const liveElementsRef = useRef<readonly unknown[]>([])
  const liveAppStateRef = useRef<Record<string, unknown>>({})
  const liveFilesRef = useRef<Record<string, unknown>>({})

  const parseResult = useMemo(() => parseExcalidrawContent(content), [content])
  const seedData: ExcalidrawDocumentData = parseResult.data
  const softError = parseResult.ok ? undefined : parseResult.error
  const seedDataRef = useRef(seedData)
  seedDataRef.current = seedData

  const [revision, setRevision] = useState(0)

  /* ---- hasElements: driven by onChange, not stale seed data -------- */
  const [hasElements, setHasElements] = useState(() =>
    seedData.elements.some((el: unknown) => {
      if (el && typeof el === 'object' && 'isDeleted' in el) return !el.isDeleted
      return true
    }),
  )

  /* ---- export / import state -------------------------------------- */
  const [exportError, setExportError] = useState<string | null>(null)
  const [exporting, setExporting] = useState(false)
  const [confirmImport, setConfirmImport] = useState<{
    data: ExcalidrawDocumentData
    fileName: string
  } | null>(null)
  const fileInputRef = useRef<HTMLInputElement | null>(null)

  /* ---- Pending debounce write ------------------------------------- */
  /* The pending write captures onChange/meta/elements at scheduling    */
  /* time so that flush always writes to the correct document, even if  */
  /* path changes between scheduling and flush.                        */
  const pendingWriteRef = useRef<{
    timer: ReturnType<typeof setTimeout>
    flush: () => void
  } | null>(null)

  /** Cancel any pending debounced write without flushing it. */
  const cancelPending = useCallback(() => {
    const pending = pendingWriteRef.current
    if (!pending) return
    clearTimeout(pending.timer)
    pendingWriteRef.current = null
  }, [])

  /** Flush + clear any pending debounced write. */
  const flushPending = useCallback(() => {
    const pending = pendingWriteRef.current
    if (!pending) return
    clearTimeout(pending.timer)
    pendingWriteRef.current = null
    pending.flush()
  }, [])

  useEffect(() => {
    lastSerializedRef.current = serializeExcalidraw(seedDataRef.current)
  }, [path])

  /* Flush pending write on unmount */
  useEffect(() => {
    return () => {
      const pending = pendingWriteRef.current
      if (pending) {
        clearTimeout(pending.timer)
        pendingWriteRef.current = null
        pending.flush()
      }
    }
  }, [])

  /* Flush pending write on path change (before new seed loads) */
  const prevPathRef = useRef(path)
  useEffect(() => {
    if (prevPathRef.current !== path) {
      flushPending()
      prevPathRef.current = path
    }
  }, [path, flushPending])

  const initialData = useMemo(
    () => toExcalidrawInitialData(seedData),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [path, revision],
  )

  const excalidrawKey = `${path}:${revision}`
  const uiOptions = useMemo(() => uiOptionsForProfile(uiProfile), [uiProfile])

  const handleChange = useCallback<ExcalidrawOnChange>(
    (elements, appState, files) => {
      liveElementsRef.current = elements
      liveAppStateRef.current = appState
      liveFilesRef.current = (files || {}) as Record<string, unknown>

      // Update hasElements reactively from live data
      const live = (elements as unknown[]).some((el: unknown) => {
        if (el && typeof el === 'object' && 'isDeleted' in el) return !el.isDeleted
        return true
      })
      setHasElements(live)

      // Capture everything at scheduling time so flush writes to the
      // correct document even if path/onChange change before the timer.
      const capturedOnChange = onChangeRef.current
      const meta = seedDataRef.current
      const capturedElements = [...elements]
      const capturedAppState = appState as Record<string, unknown>
      const capturedFiles = (files || {}) as Record<string, unknown>

      cancelPending()

      const flush = () => {
        const next = serializeExcalidraw({
          type: meta.type,
          version: meta.version,
          source: meta.source,
          elements: capturedElements,
          appState: capturedAppState,
          files: capturedFiles,
        })
        if (next === lastSerializedRef.current) return
        lastSerializedRef.current = next
        capturedOnChange(next)
      }

      const timer = setTimeout(() => {
        pendingWriteRef.current = null
        flush()
      }, DEBOUNCE_MS)

      pendingWriteRef.current = { timer, flush }
    },
    [cancelPending],
  )

  const handleAIApplied = useCallback((serialized: string) => {
    cancelPending()
    lastSerializedRef.current = serialized
    const parsed = parseExcalidrawContent(serialized)
    if (parsed.ok) {
      seedDataRef.current = parsed.data
      liveElementsRef.current = parsed.data.elements
      liveAppStateRef.current = parsed.data.appState
      liveFilesRef.current = parsed.data.files
      setHasElements(
        parsed.data.elements.some((el: unknown) => {
          if (el && typeof el === 'object' && 'isDeleted' in el) return !el.isDeleted
          return true
        }),
      )
    }
    onChangeRef.current(serialized)
    setRevision((r) => r + 1)
  }, [cancelPending])

  const handleAPI = useCallback((api: ImperativeAPI) => {
    apiRef.current = api
  }, [])

  const renderTopRightUI = useCallback(() => null, [])

  /* ---------------------------------------------------------------- */
  /* Chrome actions                                                   */
  /* ---------------------------------------------------------------- */

  /** Collect current scene data from the imperative API or live refs. */
  const collectScene = useCallback(() => {
    const api = apiRef.current
    const elements = api
      ? api.getSceneElements()
      : liveElementsRef.current
    const appState = api
      ? api.getAppState()
      : liveAppStateRef.current
    const files = api
      ? api.getFiles()
      : liveFilesRef.current
    return { elements, appState, files }
  }, [])

  /* -- Fit to content ---------------------------------------------- */
  const fitToContent = useCallback(() => {
    apiRef.current?.scrollToContent(undefined, {
      fitToViewport: true,
      viewportZoomFactor: 0.9,
      animate: true,
    })
  }, [])

  /* -- Export .excalidraw ------------------------------------------- */
  const downloadStem = useMemo(() => {
    const raw = (exportName || '').trim() || stemName(path)
    const tail = raw.split('::').pop() || raw
    const safe = tail.replace(/[\\/:*?"<>|]+/g, '-').trim()
    return safe || 'canvas'
  }, [exportName, path])

  const exportExcalidraw = useCallback(() => {
    setExportError(null)
    try {
      const { elements, appState, files } = collectScene()
      const meta = seedDataRef.current
      const json = serializeExcalidraw({
        type: meta.type,
        version: meta.version,
        source: meta.source,
        elements: [...elements],
        appState,
        files,
      })
      const blob = new Blob([json], { type: 'application/json' })
      downloadBlob(blob, `${downloadStem}.excalidraw`)
    } catch (e) {
      setExportError(e instanceof Error ? e.message : String(e))
    }
  }, [collectScene, path])

  /* -- Export PNG --------------------------------------------------- */
  const exportPng = useCallback(async () => {
    setExportError(null)
    setExporting(true)
    try {
      const { elements, appState, files } = collectScene()
      const { exportToBlob } = await getExportFns()
      const blob = await exportToBlob({
        elements,
        appState,
        files,
        mimeType: 'image/png',
      })
      downloadBlob(blob, `${downloadStem}.png`)
    } catch (e) {
      setExportError(e instanceof Error ? e.message : String(e))
    } finally {
      setExporting(false)
    }
  }, [collectScene, path])

  /* -- Export SVG --------------------------------------------------- */
  const exportSvg = useCallback(async () => {
    setExportError(null)
    setExporting(true)
    try {
      const { elements, appState, files } = collectScene()
      const { exportToSvg } = await getExportFns()
      const svgEl = await exportToSvg({ elements, appState, files })
      const svg = svgEl.outerHTML
      const blob = new Blob([svg], { type: 'image/svg+xml;charset=utf-8' })
      downloadBlob(blob, `${downloadStem}.svg`)
    } catch (e) {
      setExportError(e instanceof Error ? e.message : String(e))
    } finally {
      setExporting(false)
    }
  }, [collectScene, path])

  /* -- Import .excalidraw ------------------------------------------ */
  const triggerImport = useCallback(() => {
    fileInputRef.current?.click()
  }, [])

  const applyImport = useCallback(
    (data: ExcalidrawDocumentData) => {
      cancelPending()
      const serialized = serializeExcalidraw(data)
      lastSerializedRef.current = serialized
      seedDataRef.current = data
      liveElementsRef.current = data.elements
      liveAppStateRef.current = data.appState
      liveFilesRef.current = data.files
      setHasElements(
        data.elements.some((el: unknown) => {
          if (el && typeof el === 'object' && 'isDeleted' in el) return !el.isDeleted
          return true
        }),
      )
      onChangeRef.current(serialized)
      setRevision((r) => r + 1)
      setConfirmImport(null)
    },
    [cancelPending],
  )

  const handleImportFile = useCallback(
    async (e: React.ChangeEvent<HTMLInputElement>) => {
      setExportError(null)
      const file = e.target.files?.[0]
      // Reset so the same file can be re-selected
      if (fileInputRef.current) fileInputRef.current.value = ''
      if (!file) return

      let text: string
      try {
        text = await file.text()
      } catch (err) {
        setExportError(`Failed to read file: ${err instanceof Error ? err.message : String(err)}`)
        return
      }
      const parsed = parseExcalidrawContent(text)
      if (!parsed.ok) {
        setExportError(parsed.error)
        return
      }

      // If current canvas has elements, confirm before replacing
      const current = collectScene()
      const hasContent = (current.elements as unknown[]).filter(
        (el: unknown) => {
          if (el && typeof el === 'object' && 'isDeleted' in el) {
            return !el.isDeleted
          }
          return true
        },
      ).length > 0

      if (hasContent) {
        setConfirmImport({ data: parsed.data, fileName: file.name })
      } else {
        applyImport(parsed.data)
      }
    },
    [collectScene, applyImport],
  )

  return (
    <div
      data-testid={testId}
      data-path={path}
      data-mode="canvas"
      data-ui-profile={uiProfile}
      className="flex h-full min-h-0 w-full min-w-0 flex-1 flex-col overflow-hidden bg-shell-bg"
    >
      {!hideChrome && (
        <div
          data-testid={`${testId}-chrome`}
          className="flex h-7 shrink-0 items-center gap-2 border-b border-shell-border bg-shell-panel/90 px-2 text-[11px] text-shell-muted"
        >
          <span className="font-medium text-shell-text/90">{chromeTitle}</span>
          <span className="text-shell-border">·</span>
          <span className="min-w-0 truncate" title={path}>
            {baseName(path)}
          </span>

          {/* ---- Chrome toolbar buttons ---- */}
          <span className="flex-1" />

          <button
            type="button"
            data-testid={`${testId}-import`}
            className={chromeBtn}
            title={t('canvas.importExcalidraw')}
            aria-label={t('canvas.importExcalidraw')}
            onClick={triggerImport}
          >
            <Upload size={10} strokeWidth={2} aria-hidden />
            <span className="hidden sm:inline">{t('canvas.import')}</span>
          </button>

          <button
            type="button"
            data-testid={`${testId}-export-excalidraw`}
            className={chromeBtn}
            title={t('canvas.exportExcalidraw')}
            aria-label={t('canvas.exportExcalidraw')}
            disabled={!hasElements || exporting}
            onClick={exportExcalidraw}
          >
            <FileDown size={10} strokeWidth={2} aria-hidden />
            <span className="hidden sm:inline">.excalidraw</span>
          </button>

          <button
            type="button"
            data-testid={`${testId}-export-png`}
            className={chromeBtn}
            title={t('canvas.exportPng')}
            aria-label={t('canvas.exportPng')}
            disabled={!hasElements || exporting}
            onClick={() => void exportPng()}
          >
            <ImageIcon size={10} strokeWidth={2} aria-hidden />
            <span className="hidden sm:inline">PNG</span>
          </button>

          <button
            type="button"
            data-testid={`${testId}-export-svg`}
            className={chromeBtn}
            title={t('canvas.exportSvg')}
            aria-label={t('canvas.exportSvg')}
            disabled={!hasElements || exporting}
            onClick={() => void exportSvg()}
          >
            <Download size={10} strokeWidth={2} aria-hidden />
            <span className="hidden sm:inline">SVG</span>
          </button>

          <button
            type="button"
            data-testid={`${testId}-fit`}
            className={chromeBtn}
            title={t('canvas.fitToContent')}
            aria-label={t('canvas.fitToContent')}
            disabled={!hasElements}
            onClick={fitToContent}
          >
            <Maximize2 size={10} strokeWidth={2} aria-hidden />
          </button>

          {softError && (
            <span
              data-testid={`${testId}-soft-error`}
              role="alert"
              className="ml-1 truncate text-[10px] text-amber-400/90"
              title={softError}
            >
              {t('canvas.invalidSource')}
            </span>
          )}
        </div>
      )}

      {exportError && (
        <p
          role="alert"
          data-testid={`${testId}-export-error`}
          className="shrink-0 border-b border-shell-border px-3 py-1.5 text-[11px] text-red-400"
        >
          {t('playground.exportFailed')}: {exportError}
        </p>
      )}

      <div className="relative min-h-0 min-w-0 flex-1 overflow-hidden [&_.excalidraw]:h-full [&_.excalidraw]:w-full">
        {uiProfile !== 'full' && (
          <style data-excalidraw-embed-hide>{EMBED_HIDE_CSS}</style>
        )}
        <Suspense
          fallback={
            <div
              data-testid={`${testId}-loading`}
              role="status"
              aria-live="polite"
              className="flex h-full items-center justify-center text-[12px] text-shell-muted"
            >
              {t('canvas.loading')}
            </div>
          }
        >
          <LazyExcalidraw
            key={`${excalidrawKey}:${excalidrawTheme}:${uiProfile}`}
            theme={excalidrawTheme}
            name={baseName(path)}
            initialData={initialData}
            onChange={handleChange}
            UIOptions={uiOptions}
            aiEnabled={false}
            viewModeEnabled={uiProfile === 'present'}
            zenModeEnabled={uiProfile === 'present'}
            excalidrawAPI={handleAPI}
            renderTopRightUI={
              uiProfile === 'embed' || uiProfile === 'present'
                ? renderTopRightUI
                : undefined
            }
          />
        </Suspense>
      </div>

      {showAIBar && uiProfile !== 'present' ? (
        <CanvasAIBar
          path={path}
          elements={liveElementsRef.current}
          document={seedDataRef.current}
          onApplied={handleAIApplied}
        />
      ) : null}

      {/* Hidden file input for import */}
      <input
        ref={fileInputRef}
        type="file"
        accept=".excalidraw,.json"
        className="hidden"
        aria-hidden="true"
        tabIndex={-1}
        onChange={(e) => void handleImportFile(e)}
      />

      {/* Confirm import dialog */}
      {confirmImport && (
        <ConfirmDialog
          title={t('canvas.importConfirmTitle')}
          message={t('canvas.importConfirmMessage', {
            fileName: confirmImport.fileName,
          })}
          confirmLabel={t('canvas.importConfirmAction')}
          cancelLabel={t('action.cancel')}
          danger={false}
          onConfirm={() => applyImport(confirmImport.data)}
          onCancel={() => setConfirmImport(null)}
        />
      )}
    </div>
  )
}
