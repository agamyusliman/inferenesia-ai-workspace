import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import {
  Code2,
  Download,
  Upload,
  RefreshCw,
  Eye,
  EyeOff,
  CalendarRange,
  FileText,
  Image as ImageIcon,
  Laptop,
  LayoutTemplate,
  Monitor,
  PenLine,
  Plus,
  Presentation,
  RotateCw,
  Save,
  Search,
  Smartphone,
  Table2,
  Tablet,
  Workflow,
  X,
  type LucideIcon,
} from 'lucide-react'
import type { Workspace } from '../../lib/api'
import { getChatHistory, isSessionWorkspace } from '../../lib/api'
import { MermaidDiagram } from '../diagrams/MermaidDiagram'
import { ConfirmDialog } from '../explorer/ConfirmDialog'
import { CodeEditor } from '../editor/CodeEditor'
import { useLocale } from '../i18n/LocaleProvider'
import type { MessageKey } from '../i18n/messages'
import { PanelHeader } from '../shell/PanelHeader'
import { SHELL } from '../shell/shellTokens'
import { VerticalSplitter } from '../shell/VerticalSplitter'
import { readUiSession, writeUiSession } from '../shell/uiSession'
import {
  DEVICE_PRESETS,
  type DeviceId,
  type ScreenOrientation,
  defaultOrientation,
  frameChromeClass,
  frameSize,
  getDevicePreset,
  outerFrameSize,
  scaleToFit,
  toggleOrientation,
} from '../web-preview/devicePresets'
import { CanvasAgentPanel } from '../web-preview/CanvasAgentPanel'
import {
  isPlaceholderCanvasTitle,
  titleFromHtml,
} from '../web-preview/canvasStore'
import { buildPreviewSrcDoc } from '../web-preview/webPreview'
import { ExcalidrawCanvas } from '../canvas/ExcalidrawCanvas'
import { parseExcalidrawContent } from '../canvas/excalidrawDoc'
import { DiagramAgentPanel } from './DiagramAgentPanel'
import { ExcalidrawAgentPanel } from './ExcalidrawAgentPanel'
import { ImageStudioPanel } from './ImageStudioPanel'
import { SlidesPanel } from './slides/SlidesPanel'
import { MarkdownDocPanel } from './MarkdownDocPanel'
import { TableDocPanel } from './TableDocPanel'
import { TimelineDocPanel } from './TimelineDocPanel'
import {
  createGalleryDiagramCanvas,
  createGalleryExcalidrawCanvas,
  createGalleryHtmlCanvas,
  createGalleryImageCanvas,
  createGallerySlidesCanvas,
  createGalleryDocCanvas,
  deleteDiagramCanvas,
  deleteDocCanvas,
  deleteExcalidrawCanvas,
  deleteHtmlCanvas,
  deleteImageCanvas,
  deleteSlidesCanvas,
  formatGalleryTime,
  GALLERY_GLOBAL_SCOPE,
  indexDiagramsFromChatMessages,
  listGalleryCanvases,
  loadDiagramCanvas,
  loadExcalidrawCanvas,
  loadHtmlCanvas,
  loadImageCanvas,
  loadDocCanvas,
  loadSlidesCanvas,
  parseGalleryId,
  purgeOrphanCanvasStorage,
  pushImageHistoryVersion,
  deleteImageHistoryVersion,
  restoreImageHistoryVersion,
  updateDiagramCanvas,
  updateExcalidrawCanvas,
  updateHtmlCanvas,
  updateImageCanvas,
  updateDocCanvas,
  updateSlidesCanvas,
  type DocKind,
} from './galleryStore'
import {
  DEFAULT_IMAGE_STUDIO_OPTIONS,
  GALLERY_KIND_FILTERS,
  GALLERY_KIND_LABEL,
  type GalleryCanvasItem,
  type GalleryCanvasKind,
  type ImageStudioHistoryItem,
  type ImageStudioOptions,
} from './galleryTypes'

const toolH = 'h-6'
const segIdle =
  `inline-flex ${toolH} min-w-[1.5rem] items-center justify-center gap-0.5 px-1.5 text-[10px] text-shell-muted transition hover:text-shell-text disabled:opacity-35`
const segActive =
  `inline-flex ${toolH} min-w-[1.5rem] items-center justify-center gap-0.5 bg-shell-active px-1.5 text-[10px] text-shell-accent`

type Props = {
  workspaces: Workspace[]
  busy?: boolean
  onOpenSession?: (sessionId: string) => void
  focusId?: string | null
  onFocusConsumed?: () => void
  onStatusLabelChange?: (label: string | undefined) => void
}

type CtxMenu = { x: number; y: number; item: GalleryCanvasItem }

const DEVICE_ICONS: Record<DeviceId, typeof Smartphone> = {
  mobile: Smartphone,
  tablet: Tablet,
  laptop: Laptop,
  desktop: Monitor,
}

const DEVICE_LABEL_KEYS: Record<DeviceId, MessageKey> = {
  mobile: 'webPreview.device.mobile',
  tablet: 'webPreview.device.tablet',
  laptop: 'webPreview.device.laptop',
  desktop: 'webPreview.device.desktop',
}

const DOC_KIND_FALLBACK_TITLE: Record<DocKind, string> = {
  markdown: 'Document',
  table: 'Table',
  timeline: 'Timeline',
}

/** Narrows a parsed gallery id kind to the text-document playgrounds. */
function isDocKind(kind: string): kind is DocKind {
  return kind === 'markdown' || kind === 'table' || kind === 'timeline'
}

export function CanvasGalleryPanel({
  workspaces,
  onOpenSession,
  focusId,
  onFocusConsumed,
  onStatusLabelChange,
}: Props) {
  const { t } = useLocale()
  const stageRef = useRef<HTMLDivElement | null>(null)
  const htmlSplitRef = useRef<HTMLDivElement | null>(null)
  const sessions = useMemo(
    () =>
      workspaces
        .filter(isSessionWorkspace)
        .slice()
        .sort((a, b) => {
          const ta = a.last_opened_at ? Date.parse(a.last_opened_at) : 0
          const tb = b.last_opened_at ? Date.parse(b.last_opened_at) : 0
          return (Number.isFinite(tb) ? tb : 0) - (Number.isFinite(ta) ? ta : 0)
        })
        .map((w) => {
          let when = ''
          if (w.last_opened_at) {
            const ms = Date.parse(w.last_opened_at)
            if (Number.isFinite(ms) && ms > 0) {
              try {
                when = new Date(ms).toLocaleString()
              } catch {
                when = ''
              }
            }
          }
          if (!when) {
            const idMatch = /^ws_sess_(\d+)$/.exec(w.id || '')
            if (idMatch) {
              const digits = idMatch[1]
              const ms =
                digits.length > 13
                  ? Number(digits.slice(0, -6))
                  : Number(digits)
              if (Number.isFinite(ms) && ms > 0) {
                try {
                  when = new Date(ms).toLocaleString()
                } catch {
                  when = ''
                }
              }
            }
          }
          return { id: w.id, name: w.name || w.id, when }
        }),
    [workspaces],
  )

  const [query, setQuery] = useState('')
  const [kindFilter, setKindFilter] = useState<GalleryCanvasKind | 'all'>(
    'all',
  )
  const [items, setItems] = useState<GalleryCanvasItem[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(() => {
    const s = readUiSession()
    return s?.gallerySelectedId ?? null
  })
  const [deleteTarget, setDeleteTarget] = useState<GalleryCanvasItem | null>(
    null,
  )
  const [createWizardKind, setCreateWizardKind] = useState<GalleryCanvasKind | null>(
    null,
  )
  const [createScopeId, setCreateScopeId] = useState(GALLERY_GLOBAL_SCOPE)
  const [createSessionQuery, setCreateSessionQuery] = useState('')
  const [createTitle, setCreateTitle] = useState('')
  const [ctxMenu, setCtxMenu] = useState<CtxMenu | null>(null)
  const [listWidth, setListWidth] = useState(280)
  const [showCode, setShowCode] = useState(true)
  const [showPreview, setShowPreview] = useState(true)
  const [deviceId, setDeviceId] = useState<DeviceId>('desktop')
  const [orientation, setOrientation] = useState<ScreenOrientation>(
    defaultOrientation('desktop'),
  )
  const [stageSize, setStageSize] = useState({ w: 800, h: 600 })
  const [editorWidth, setEditorWidth] = useState(380)
  const [htmlNarrow, setHtmlNarrow] = useState(false)
  const [htmlDraft, setHtmlDraft] = useState('')
  const [htmlLive, setHtmlLive] = useState('')
  const [diagramSource, setDiagramSource] = useState('')
  const [reloadKey, setReloadKey] = useState(0)
  const [indexing, setIndexing] = useState(false)
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [renameDraft, setRenameDraft] = useState('')
  const [saveFlash, setSaveFlash] = useState(false)
  const [savedHtml, setSavedHtml] = useState('')
  const [savedDiagram, setSavedDiagram] = useState('')
  const [excalidrawContent, setExcalidrawContent] = useState('')
  const [excalidrawRev, setExcalidrawRev] = useState(0)
  const [slidesContent, setSlidesContent] = useState('')
  const [docContent, setDocContent] = useState('')
  const [savedDocContent, setSavedDocContent] = useState('')
  const [saveError, setSaveError] = useState<string | null>(null)
  const htmlImportRef = useRef<HTMLInputElement | null>(null)
  const [pendingHtmlImport, setPendingHtmlImport] = useState<{ html: string; galleryId: string; name: string } | null>(null)
  const [imagePrompt, setImagePrompt] = useState('')
  const [imageAgentPrompt, setImageAgentPrompt] = useState('')
  const [imageUrl, setImageUrl] = useState('')
  const [imageResultAspect, setImageResultAspect] = useState<
    ImageStudioOptions['aspect'] | undefined
  >(undefined)
  const [imageCaption, setImageCaption] = useState('')
  const [imageHistory, setImageHistory] = useState<ImageStudioHistoryItem[]>(
    [],
  )
  const [imageOptions, setImageOptions] = useState<ImageStudioOptions>({
    ...DEFAULT_IMAGE_STUDIO_OPTIONS,
  })
  const htmlDraftRef = useRef('')
  const diagramSourceRef = useRef('')
  const docContentRef = useRef('')
  const selectedRef = useRef<GalleryCanvasItem | null>(null)
  const saveFlashTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  htmlDraftRef.current = htmlDraft
  diagramSourceRef.current = diagramSource
  docContentRef.current = docContent

  useEffect(() => {
    const el = stageRef.current
    if (!el || !showPreview) return
    const apply = (w: number, h: number) => {
      if (w <= 0 || h <= 0) return
      setStageSize((prev) =>
        prev.w === w && prev.h === h ? prev : { w, h },
      )
    }
    if (typeof ResizeObserver === 'undefined') {
      apply(el.clientWidth, el.clientHeight)
      return
    }
    const ro = new ResizeObserver((entries) => {
      const cr = entries[0]?.contentRect
      if (!cr) return
      apply(cr.width, cr.height)
    })
    ro.observe(el)
    apply(el.clientWidth, el.clientHeight)
    return () => ro.disconnect()
  }, [selectedId, showPreview, showCode, editorWidth])


  const refresh = useCallback(() => {
    purgeOrphanCanvasStorage(sessions.map((s) => s.id))
    setItems(listGalleryCanvases(sessions))
  }, [sessions])

  useEffect(() => {
    refresh()
  }, [refresh])

  useEffect(() => {
    writeUiSession({ gallerySelectedId: selectedId })
  }, [selectedId])

  useEffect(() => {
    if (!selectedId) return
    if (items.some((it) => it.id === selectedId)) return
    const parsed = parseGalleryId(selectedId)
    if (!parsed) {
      setSelectedId(null)
      return
    }
    const stillInStorage = isDocKind(parsed.kind)
      ? !!loadDocCanvas(parsed.kind, parsed.sessionId, parsed.localId)
      : parsed.kind === 'diagram'
        ? !!loadDiagramCanvas(parsed.sessionId, parsed.localId)
        : parsed.kind === 'html'
          ? !!loadHtmlCanvas(parsed.sessionId, parsed.localId)
          : parsed.kind === 'image'
            ? !!loadImageCanvas(parsed.sessionId, parsed.localId)
            : parsed.kind === 'excalidraw'
              ? !!loadExcalidrawCanvas(parsed.sessionId, parsed.localId)
              : parsed.kind === 'slides'
                ? !!loadSlidesCanvas(parsed.sessionId, parsed.localId)
                : false
    if (stillInStorage) return
    setSelectedId(null)
  }, [items, selectedId])

  useEffect(() => {
    if (!focusId) return
    refresh()
    setSelectedId(focusId)
    onFocusConsumed?.()
  }, [focusId, onFocusConsumed, refresh])

  useEffect(() => {
    if (sessions.length === 0) return
    let cancelled = false
    setIndexing(true)
    void (async () => {
      try {
        await Promise.all(
          sessions.map(async (s) => {
            try {
              const hist = await getChatHistory(s.id)
              indexDiagramsFromChatMessages(s.id, hist.messages || [])
            } catch {
              void 0
            }
          }),
        )
      } finally {
        if (!cancelled) {
          setIndexing(false)
          refresh()
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [sessions, refresh])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return items.filter((it) => {
      if (kindFilter !== 'all' && it.kind !== kindFilter) return false
      if (!q) return true
      return (
        it.title.toLowerCase().includes(q) ||
        it.sessionName.toLowerCase().includes(q) ||
        it.kind.includes(q) ||
        it.sessionId.toLowerCase().includes(q)
      )
    })
  }, [items, kindFilter, query])

  const kindCounts = useMemo(() => {
    const counts: Partial<Record<GalleryCanvasKind, number>> = {}
    for (const it of items) counts[it.kind] = (counts[it.kind] || 0) + 1
    return counts
  }, [items])

  const selected = useMemo(() => {
    if (!selectedId) return null
    const fromList = items.find((i) => i.id === selectedId)
    if (fromList) return fromList
    const parsed = parseGalleryId(selectedId)
    if (!parsed) return null
    const sessionName =
      sessions.find((s) => s.id === parsed.sessionId)?.name ||
      (parsed.sessionId === GALLERY_GLOBAL_SCOPE
        ? 'Library'
        : parsed.sessionId)
    if (parsed.kind === 'diagram') {
      const entry = loadDiagramCanvas(parsed.sessionId, parsed.localId)
      if (!entry) return null
      return {
        id: selectedId,
        kind: 'diagram' as const,
        title: entry.title || 'Diagram',
        sessionId: parsed.sessionId,
        sessionName,
        updatedAt: entry.updatedAt || entry.createdAt || 0,
        createdAt: entry.createdAt || 0,
        source: entry.source,
        chatMessageId: entry.chatMessageId,
      }
    }
    if (parsed.kind === 'html') {
      const entry = loadHtmlCanvas(parsed.sessionId, parsed.localId)
      if (!entry) return null
      return {
        id: selectedId,
        kind: 'html' as const,
        title: entry.title || 'Playground',
        sessionId: parsed.sessionId,
        sessionName,
        updatedAt: entry.updatedAt || entry.createdAt || 0,
        createdAt: entry.createdAt || 0,
        html: entry.html,
        path: entry.path,
        chatMessageId: entry.chatMessageId,
      }
    }
    if (parsed.kind === 'image') {
      const entry = loadImageCanvas(parsed.sessionId, parsed.localId)
      if (!entry) return null
      return {
        id: selectedId,
        kind: 'image' as const,
        title: entry.title || 'Image',
        sessionId: parsed.sessionId,
        sessionName,
        updatedAt: entry.updatedAt || entry.createdAt || 0,
        createdAt: entry.createdAt || 0,
        prompt: entry.prompt,
        agentPrompt: entry.agentPrompt,
        imageUrl: entry.imageUrl,
        caption: entry.caption,
        imageOptions: entry.options,
        imageHistory: entry.history || [],
        chatMessageId: entry.chatMessageId,
      }
    }
    if (parsed.kind === 'excalidraw') {
      const entry = loadExcalidrawCanvas(parsed.sessionId, parsed.localId)
      if (!entry) return null
      return {
        id: selectedId,
        kind: 'excalidraw' as const,
        title: entry.title || 'Whiteboard',
        sessionId: parsed.sessionId,
        sessionName,
        updatedAt: entry.updatedAt || entry.createdAt || 0,
        createdAt: entry.createdAt || 0,
        source: entry.content,
        chatMessageId: entry.chatMessageId,
      }
    }
    if (parsed.kind === 'slides') {
      const entry = loadSlidesCanvas(parsed.sessionId, parsed.localId)
      if (!entry) return null
      return {
        id: selectedId,
        kind: 'slides' as const,
        title: entry.title || 'Deck',
        sessionId: parsed.sessionId,
        sessionName,
        updatedAt: entry.updatedAt || entry.createdAt || 0,
        createdAt: entry.createdAt || 0,
        source: entry.content,
        chatMessageId: entry.chatMessageId,
      }
    }
    if (isDocKind(parsed.kind)) {
      const entry = loadDocCanvas(parsed.kind, parsed.sessionId, parsed.localId)
      if (!entry) return null
      return {
        id: selectedId,
        kind: parsed.kind,
        title: entry.title || DOC_KIND_FALLBACK_TITLE[parsed.kind],
        sessionId: parsed.sessionId,
        sessionName,
        updatedAt: entry.updatedAt || entry.createdAt || 0,
        createdAt: entry.createdAt || 0,
        source: entry.content,
        chatMessageId: entry.chatMessageId,
      }
    }
    return null
  }, [items, selectedId, sessions])
  selectedRef.current = selected
  useEffect(() => {
    const el = htmlSplitRef.current
    if (!el || selected?.kind !== 'html') return
    const apply = (w: number) => {
      if (w <= 0) return
      // Below this width a code+preview split leaves neither pane usable.
      const next = w < 720
      setHtmlNarrow((prev) => (prev === next ? prev : next))
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
  }, [selected?.id, selected?.kind])

  useEffect(() => {
    if (!onStatusLabelChange) return
    if (!selected) {
      onStatusLabelChange(`playground: ${t('canvasGallery.library')}`)
      return
    }
    if (selected.sessionId === GALLERY_GLOBAL_SCOPE) {
      onStatusLabelChange(`playground: ${t('canvasGallery.library')}`)
      return
    }
    onStatusLabelChange(
      `playground: ${selected.sessionName || selected.sessionId}`,
    )
  }, [onStatusLabelChange, selected, t])

  useEffect(() => {
    return () => {
      onStatusLabelChange?.(undefined)
    }
  }, [onStatusLabelChange])

  useEffect(() => {
    return () => {
      if (saveFlashTimerRef.current) clearTimeout(saveFlashTimerRef.current)
    }
  }, [])

  const flashSaved = useCallback(() => {
    setSaveFlash(true)
    if (saveFlashTimerRef.current) clearTimeout(saveFlashTimerRef.current)
    saveFlashTimerRef.current = setTimeout(() => setSaveFlash(false), 1200)
  }, [])

  // One reset then one load per selection: every kind previously repeated the
  // full clear list, and branches drifted out of sync (stale image history).
  useEffect(() => {
    htmlDraftRef.current = ''
    diagramSourceRef.current = ''
    docContentRef.current = ''
    setHtmlDraft('')
    setHtmlLive('')
    setSavedHtml('')
    setDiagramSource('')
    setSavedDiagram('')
    setExcalidrawContent('')
    setSlidesContent('')
    setDocContent('')
    setSavedDocContent('')
    setSaveError(null)
    setImagePrompt('')
    setImageAgentPrompt('')
    setImageUrl('')
    setImageResultAspect(undefined)
    setImageCaption('')
    setImageHistory([])
    setImageOptions({ ...DEFAULT_IMAGE_STUDIO_OPTIONS })

    const parsed = selectedId ? parseGalleryId(selectedId) : null
    if (!parsed) return

    if (parsed.kind === 'html') {
      const html = loadHtmlCanvas(parsed.sessionId, parsed.localId)?.html || ''
      htmlDraftRef.current = html
      setHtmlDraft(html)
      setHtmlLive(html)
      setSavedHtml(html)
      setReloadKey((k) => k + 1)
      return
    }
    if (parsed.kind === 'diagram') {
      const source =
        loadDiagramCanvas(parsed.sessionId, parsed.localId)?.source || ''
      diagramSourceRef.current = source
      setDiagramSource(source)
      setSavedDiagram(source)
      return
    }
    if (parsed.kind === 'image') {
      const entry = loadImageCanvas(parsed.sessionId, parsed.localId)
      setImagePrompt(entry?.prompt || '')
      setImageAgentPrompt(entry?.agentPrompt || '')
      setImageUrl(entry?.imageUrl || '')
      setImageResultAspect(entry?.options?.aspect)
      setImageCaption(entry?.caption || '')
      setImageHistory(entry?.history || [])
      if (entry?.options) {
        setImageOptions({ ...DEFAULT_IMAGE_STUDIO_OPTIONS, ...entry.options })
      }
      return
    }
    if (parsed.kind === 'excalidraw') {
      setExcalidrawContent(
        loadExcalidrawCanvas(parsed.sessionId, parsed.localId)?.content || '',
      )
      setExcalidrawRev(0)
      return
    }
    if (parsed.kind === 'slides') {
      setSlidesContent(
        loadSlidesCanvas(parsed.sessionId, parsed.localId)?.content || '',
      )
      return
    }
    if (isDocKind(parsed.kind)) {
      const content =
        loadDocCanvas(parsed.kind, parsed.sessionId, parsed.localId)?.content ||
        ''
      docContentRef.current = content
      setDocContent(content)
      setSavedDocContent(content)
    }
  }, [selectedId])

  const applyHtmlFromAgent = useCallback(
    (html: string, opts?: { final?: boolean; canvasId?: string }) => {
      const targetId = opts?.canvasId || selectedId
      if (!targetId) return
      const parsed = parseGalleryId(targetId)
      if (!parsed || parsed.kind !== 'html') return
      const isViewing = selectedId === targetId
      if (isViewing) {
        setHtmlDraft(html)
        setHtmlLive(html)
        if (opts?.final) setReloadKey((k) => k + 1)
      }
      if (opts?.final) {
        const entry = loadHtmlCanvas(parsed.sessionId, parsed.localId)
        const derived = titleFromHtml(html)
        const patch: { html: string; title?: string } = { html }
        if (
          isPlaceholderCanvasTitle(entry?.title) &&
          derived &&
          !isPlaceholderCanvasTitle(derived)
        ) {
          patch.title = derived
        }
        const saved = updateHtmlCanvas(parsed.sessionId, parsed.localId, patch)
        // The agent turn is the new persisted baseline; without this the
        // editor keeps showing "unsaved" for content already on disk. A quota
        // failure must NOT move that baseline — the draft stays unsaved.
        if (saved?.saveResult && !saved.saveResult.ok) {
          setSaveError(saved.saveResult.error ||
            'Could not save — browser storage is full')
        } else {
          if (saved && isViewing) setSavedHtml(saved.html)
          setSaveError(null)
        }
        refresh()
      }
    },
    [refresh, selectedId],
  )

  const applyDiagramFromAgent = useCallback(
    (source: string, opts?: { final?: boolean; canvasId?: string }) => {
      const targetId = opts?.canvasId || selectedId
      if (!targetId) return
      const parsed = parseGalleryId(targetId)
      if (!parsed || parsed.kind !== 'diagram') return
      const isViewing = selectedId === targetId
      if (isViewing) setDiagramSource(source)
      if (opts?.final) {
        const entry = loadDiagramCanvas(parsed.sessionId, parsed.localId)
        const firstLine = source
          .split('\n')
          .map((l) => l.trim())
          .find((l) => l && !l.startsWith('%%'))
        const patch: { source: string; title?: string } = { source }
        if (
          isPlaceholderCanvasTitle(entry?.title) &&
          firstLine &&
          firstLine.length > 2 &&
          firstLine.length < 80
        ) {
          patch.title =
            firstLine
              .replace(
                /^(flowchart|sequenceDiagram|classDiagram|erDiagram|stateDiagram(?:-v2)?|gantt|pie|mindmap|timeline|gitGraph)\s*/i,
                '',
              )
              .trim() || firstLine
        }
        const saved = updateDiagramCanvas(
          parsed.sessionId,
          parsed.localId,
          patch,
        )
        if (saved?.saveResult && !saved.saveResult.ok) {
          setSaveError(saved.saveResult.error ||
            'Could not save — browser storage is full')
        } else {
          if (saved && isViewing) setSavedDiagram(saved.source)
          setSaveError(null)
        }
        refresh()
      }
    },
    [refresh, selectedId],
  )

  const startRename = (item: GalleryCanvasItem) => {
    setRenamingId(item.id)
    setRenameDraft(item.title)
    setCtxMenu(null)
  }

  const commitRename = useCallback(() => {
    if (!renamingId) return
    const title = renameDraft.trim()
    const item = items.find((i) => i.id === renamingId)
    setRenamingId(null)
    setRenameDraft('')
    if (!item || !title || title === item.title) return
    const parsed = parseGalleryId(item.id)
    if (!parsed) return
    if (isDocKind(parsed.kind) && parsed.kind === item.kind) {
      updateDocCanvas(parsed.kind, parsed.sessionId, parsed.localId, { title })
    } else if (item.kind === 'html' && parsed.kind === 'html') {
      updateHtmlCanvas(parsed.sessionId, parsed.localId, { title })
    } else if (item.kind === 'diagram' && parsed.kind === 'diagram') {
      updateDiagramCanvas(parsed.sessionId, parsed.localId, { title })
    } else if (item.kind === 'image' && parsed.kind === 'image') {
      updateImageCanvas(parsed.sessionId, parsed.localId, { title })
    } else if (item.kind === 'excalidraw' && parsed.kind === 'excalidraw') {
      updateExcalidrawCanvas(parsed.sessionId, parsed.localId, { title })
    } else if (item.kind === 'slides' && parsed.kind === 'slides') {
      updateSlidesCanvas(parsed.sessionId, parsed.localId, { title })
    }
    refresh()
  }, [items, refresh, renameDraft, renamingId])

  const persistHtml = useCallback(
    (
      html: string,
      opts?: {
        bumpPreview?: boolean
        flash?: boolean
        galleryId?: string
      },
    ) => {
      const galleryId = opts?.galleryId || selectedRef.current?.id || selectedId
      if (!galleryId) return false
      const parsed = parseGalleryId(galleryId)
      if (!parsed || parsed.kind !== 'html') return false
      htmlDraftRef.current = html
      setHtmlDraft(html)
      if (opts?.bumpPreview !== false) {
        setHtmlLive(html)
        setReloadKey((k) => k + 1)
      }
      const saved = updateHtmlCanvas(parsed.sessionId, parsed.localId, { html })
      if (!saved) return false
      if (saved.saveResult && !saved.saveResult.ok) {
        setSaveError(saved.saveResult.error ||
            'Could not save — browser storage is full')
        return false
      }
      setSavedHtml(html)
      setSaveError(null)
      refresh()
      if (opts?.flash) flashSaved()
      return true
    },
    [flashSaved, refresh, selectedId],
  )

  const persistDiagram = useCallback(
    (
      source: string,
      opts?: { flash?: boolean; galleryId?: string },
    ) => {
      const galleryId = opts?.galleryId || selectedRef.current?.id || selectedId
      if (!galleryId) return false
      const parsed = parseGalleryId(galleryId)
      if (!parsed || parsed.kind !== 'diagram') return false
      diagramSourceRef.current = source
      setDiagramSource(source)
      const saved = updateDiagramCanvas(parsed.sessionId, parsed.localId, {
        source,
      })
      if (!saved) return false
      if (saved.saveResult && !saved.saveResult.ok) {
        setSaveError(saved.saveResult.error ||
            'Could not save — browser storage is full')
        return false
      }
      setSavedDiagram(source)
      setSaveError(null)
      refresh()
      if (opts?.flash) flashSaved()
      return true
    },
    [flashSaved, refresh, selectedId],
  )

  const htmlDirty = selected?.kind === 'html' && htmlDraft !== savedHtml
  const diagramDirty =
    selected?.kind === 'diagram' && diagramSource !== savedDiagram
  const docDirty =
    !!selected && isDocKind(selected.kind) && docContent !== savedDocContent

  const onHtmlDraftChange = useCallback((value: string) => {
    htmlDraftRef.current = value
    setHtmlDraft(value)
    setHtmlLive(value)
  }, [])

  const importHtml = async (file: File | undefined) => {
    const target = selectedRef.current
    if (!file || target?.kind !== 'html') return
    if (file.size > 5 * 1024 * 1024) {
      setSaveError(t('playground.htmlTooLarge'))
      return
    }
    try {
      const html = await file.text()
      if (!html.trim()) throw new Error(t('playground.htmlEmpty'))
      setPendingHtmlImport({ html, galleryId: target.id, name: file.name })
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : String(error))
    }
  }

  const downloadHtml = () => {
    const url = URL.createObjectURL(new Blob([htmlDraftRef.current], { type: 'text/html;charset=utf-8' }))
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `${(selected?.title || 'Playground').replace(/[<>:"/\\|?*\u0000-\u001f]/g, '_')}.html`
    anchor.click()
    window.setTimeout(() => URL.revokeObjectURL(url), 1000)
  }

  const onDiagramSourceChange = useCallback((source: string) => {
    diagramSourceRef.current = source
    setDiagramSource(source)
  }, [])

  const onDiagramSourceSave = useCallback(
    (source: string) => {
      const id = selectedRef.current?.id
      if (!id) return
      persistDiagram(source, { flash: true, galleryId: id })
    },
    [persistDiagram],
  )

  const confirmDelete = () => {
    if (!deleteTarget) return
    const parsed = parseGalleryId(deleteTarget.id)
    if (parsed) {
      if (isDocKind(parsed.kind) && parsed.kind === deleteTarget.kind) {
        deleteDocCanvas(parsed.kind, parsed.sessionId, parsed.localId)
      } else if (deleteTarget.kind === 'html' && parsed.kind === 'html') {
        deleteHtmlCanvas(parsed.sessionId, parsed.localId)
      } else if (deleteTarget.kind === 'diagram' && parsed.kind === 'diagram') {
        deleteDiagramCanvas(parsed.sessionId, parsed.localId)
      } else if (deleteTarget.kind === 'image' && parsed.kind === 'image') {
        deleteImageCanvas(parsed.sessionId, parsed.localId)
      } else if (
        deleteTarget.kind === 'excalidraw' &&
        parsed.kind === 'excalidraw'
      ) {
        deleteExcalidrawCanvas(parsed.sessionId, parsed.localId)
      } else if (deleteTarget.kind === 'slides' && parsed.kind === 'slides') {
        deleteSlidesCanvas(parsed.sessionId, parsed.localId)
      }
    }
    if (selectedId === deleteTarget.id) setSelectedId(null)
    setDeleteTarget(null)
    setCtxMenu(null)
    refresh()
  }

  const openCreateWizard = (kind: GalleryCanvasKind) => {
    setCreateWizardKind(kind)
    setCreateScopeId(GALLERY_GLOBAL_SCOPE)
    setCreateSessionQuery('')
    setCreateTitle('')
  }

  const closeCreateWizard = () => {
    setCreateWizardKind(null)
    setCreateScopeId(GALLERY_GLOBAL_SCOPE)
    setCreateSessionQuery('')
    setCreateTitle('')
  }

  const confirmCreateCanvas = () => {
    if (!createWizardKind) return
    const scopeId = createScopeId || GALLERY_GLOBAL_SCOPE
    const title = createTitle.trim() || undefined
    let created: GalleryCanvasItem | null = null
    try {
      created = isDocKind(createWizardKind)
        ? createGalleryDocCanvas(createWizardKind, { scopeId, title })
        : createWizardKind === 'html'
          ? createGalleryHtmlCanvas({ scopeId, title })
          : createWizardKind === 'diagram'
            ? createGalleryDiagramCanvas({ scopeId, title })
            : createWizardKind === 'excalidraw'
              ? createGalleryExcalidrawCanvas({ scopeId, title })
              : createWizardKind === 'slides'
                ? createGallerySlidesCanvas({ scopeId, title })
                : createGalleryImageCanvas({ scopeId, title })
    } catch (err) {
      console.error('[confirmCreateCanvas] create failed:', err)
      setSaveError(
        err instanceof Error ? err.message : 'Could not create the playground',
      )
      return
    }
    if (!created) return
    closeCreateWizard()
    refresh()
    setSelectedId(created.id)
  }

  /**
   * Editor/grid edits stay in memory and mark the document dirty. The captured
   * `galleryId` prevents a late keystroke from a just-closed document
   * overwriting the draft of the one now on screen.
   */
  const onDocContentChange = useCallback(
    (next: string, galleryId: string) => {
      if ((selectedRef.current?.id || selectedId) !== galleryId) return
      docContentRef.current = next
      setDocContent(next)
    },
    [selectedId],
  )

  const persistDoc = useCallback(
    (content: string, opts?: { galleryId?: string; flash?: boolean }) => {
      const galleryId = opts?.galleryId || selectedRef.current?.id || selectedId
      if (!galleryId) return false
      const parsed = parseGalleryId(galleryId)
      if (!parsed || !isDocKind(parsed.kind)) return false
      const isViewing = (selectedRef.current?.id || selectedId) === galleryId
      if (isViewing) {
        docContentRef.current = content
        setDocContent(content)
      }
      const saved = updateDocCanvas(
        parsed.kind,
        parsed.sessionId,
        parsed.localId,
        { content },
      )
      if (!saved) {
        setSaveError('This playground no longer exists in storage')
        return false
      }
      if (saved.saveResult && !saved.saveResult.ok) {
        setSaveError(saved.saveResult.error || 'Could not save — storage is full')
        return false
      }
      if (isViewing) {
        setSavedDocContent(content)
        setSaveError(null)
      }
      refresh()
      if (opts?.flash !== false) flashSaved()
      return true
    },
    [flashSaved, refresh, selectedId],
  )

  /** Agent results are persisted immediately — the reply is the save point. */
  const applyDocFromAgent = useCallback(
    (content: string, opts: { canvasId: string }) => {
      persistDoc(content, { galleryId: opts.canvasId, flash: false })
    },
    [persistDoc],
  )

  const onSlidesContentChange = useCallback(
    (json: string, galleryId: string) => {
      const parsed = parseGalleryId(galleryId)
      if (!parsed || parsed.kind !== 'slides') return
      if ((selectedRef.current?.id || selectedId) === galleryId) {
        setSlidesContent(json)
      }
      const saved = updateSlidesCanvas(parsed.sessionId, parsed.localId, {
        content: json,
      })
      const result = saved?.saveResult
      if (result && !result.ok) {
        console.error('[slides] save failed', result.error)
        setSaveError(
          result.error ||
            'Could not save the deck — browser storage is full. Export it, then delete something.',
        )
      } else {
        setSaveError(null)
      }
      refresh()
    },
    [refresh, selectedId],
  )

  /**
   * `galleryId` is captured at render by the caller. Resolving the target from
   * `selectedRef` instead would misroute a late change (the last shape before a
   * switch) into whatever canvas is selected by the time the callback fires.
   */
  const onExcalidrawChange = useCallback(
    (json: string, galleryId: string, opts?: { remount?: boolean }) => {
      const parsed = parseGalleryId(galleryId)
      if (!parsed || parsed.kind !== 'excalidraw') return
      const isViewing = (selectedRef.current?.id || selectedId) === galleryId
      if (isViewing) {
        setExcalidrawContent(json)
        if (opts?.remount) setExcalidrawRev((r) => r + 1)
      }
      const saved = updateExcalidrawCanvas(parsed.sessionId, parsed.localId, {
        content: json,
      })
      if (saved?.saveResult && !saved.saveResult.ok) {
        setSaveError(saved.saveResult.error ||
            'Could not save — browser storage is full')
      } else {
        setSaveError(null)
      }
      refresh()
    },
    [refresh, selectedId],
  )

  const excalidrawDoc = useMemo(
    () => parseExcalidrawContent(excalidrawContent).data,
    [excalidrawContent],
  )

  const filteredCreateSessions = useMemo(() => {
    const q = createSessionQuery.trim().toLowerCase()
    if (!q) return sessions
    return sessions.filter(
      (s) =>
        s.name.toLowerCase().includes(q) || s.id.toLowerCase().includes(q),
    )
  }, [createSessionQuery, sessions])

  const applyImageResult = useCallback(
    (
      result: {
        prompt: string
        agentPrompt: string
        imageUrl: string
        caption: string
        options: ImageStudioOptions
        title?: string
      },
      opts?: { final?: boolean; canvasId?: string },
    ) => {
      const targetId = opts?.canvasId || selectedRef.current?.id || selectedId
      if (!targetId) return
      const parsed = parseGalleryId(targetId)
      if (!parsed || parsed.kind !== 'image') return
      const isViewing =
        (selectedRef.current?.id || selectedId) === targetId
      if (opts?.final) {
        const entry = loadImageCanvas(parsed.sessionId, parsed.localId)
        const title =
          result.title &&
          (!entry?.title ||
            /^image(\s|$)/i.test(entry.title) ||
            entry.title.startsWith('Image '))
            ? result.title
            : undefined
        const saved = pushImageHistoryVersion(parsed.sessionId, parsed.localId, {
          prompt: result.prompt,
          agentPrompt: result.agentPrompt,
          imageUrl: result.imageUrl,
          caption: result.caption,
          options: result.options,
          title,
        })
        if (saved?.saveResult && !saved.saveResult.ok) {
          // A generated image that did not reach storage is lost on reload;
          // say so instead of pretending the turn succeeded.
          console.error('[image] save failed', saved.saveResult.error)
          setSaveError(saved.saveResult.error || null)
        } else {
          setSaveError(null)
        }
        if (isViewing) {
          setImagePrompt(result.prompt)
          setImageAgentPrompt(
            saved?.agentPrompt || result.agentPrompt,
          )
          setImageUrl(result.imageUrl)
          setImageResultAspect(
            saved?.options?.aspect || result.options.aspect,
          )
          setImageCaption(result.caption)
          setImageHistory(saved?.history || entry?.history || [])
          setImageOptions(
            saved?.options
              ? { ...DEFAULT_IMAGE_STUDIO_OPTIONS, ...saved.options }
              : result.options,
          )
        }
        refresh()
        return
      }
      if (isViewing) {
        setImagePrompt(result.prompt)
        setImageAgentPrompt(result.agentPrompt)
        setImageUrl(result.imageUrl)
        setImageResultAspect(result.options.aspect)
        setImageCaption(result.caption)
      }
    },
    [refresh, selectedId],
  )

  const onImageOptionsChange = useCallback(
    (next: ImageStudioOptions) => {
      setImageOptions(next)
      if (!selectedId) return
      const parsed = parseGalleryId(selectedId)
      if (!parsed || parsed.kind !== 'image') return
      updateImageCanvas(parsed.sessionId, parsed.localId, { options: next })
    },
    [selectedId],
  )

  const onRestoreImageHistory = useCallback(
    (historyId: string) => {
      if (!historyId || !selectedId) return
      const parsed = parseGalleryId(selectedId)
      if (!parsed || parsed.kind !== 'image') return
      const saved = restoreImageHistoryVersion(
        parsed.sessionId,
        parsed.localId,
        historyId,
      )
      if (!saved) return
      setImagePrompt(saved.prompt)
      setImageAgentPrompt(saved.agentPrompt || '')
      setImageUrl(saved.imageUrl)
      setImageResultAspect(saved.options?.aspect)
      setImageCaption(saved.caption)
      setImageHistory(saved.history || [])
      refresh()
    },
    [refresh, selectedId],
  )

  const onDeleteImageHistory = useCallback(
    (historyId: string) => {
      if (!historyId || !selectedId) return
      const parsed = parseGalleryId(selectedId)
      if (!parsed || parsed.kind !== 'image') return
      const saved = deleteImageHistoryVersion(
        parsed.sessionId,
        parsed.localId,
        historyId,
      )
      if (!saved) return
      setImageHistory(saved.history || [])
      refresh()
    },
    [refresh, selectedId],
  )

  const createOptions = useMemo(
    () => galleryCreateOptions(t),
    [t],
  )

  const srcDoc = useMemo(() => buildPreviewSrcDoc(htmlLive), [htmlLive])

  const isLibraryScope = selected?.sessionId === GALLERY_GLOBAL_SCOPE
  const docContextId = isLibraryScope ? undefined : selected?.sessionId
  const docScopeLabel = isLibraryScope
    ? t('canvasGallery.library')
    : selected?.sessionName || selected?.sessionId || ''

  const device = getDevicePreset(deviceId)
  const frame = frameSize(device, orientation)
  const outer = outerFrameSize(frame, device.chrome)
  const scale = scaleToFit(
    outer.width,
    outer.height,
    stageSize.w,
    stageSize.h,
    28,
  )

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
  const selectDevice = (id: DeviceId) => {
    setDeviceId(id)
    setOrientation(defaultOrientation(id))
  }

  return (
    <div
      data-testid="canvas-gallery"
      className="flex h-full min-h-0 min-w-0 flex-1 overflow-hidden bg-shell-bg"
    >
      <aside
        style={{ width: `clamp(12.5rem, ${listWidth}px, min(26.25rem, 42%))` }}
        className="flex min-h-0 min-w-[12.5rem] shrink-0 flex-col border-r border-shell-border bg-shell-panel"
      >
        <PanelHeader
          title={t('nav.canvas')}
          dense
          actions={
            <button
              type="button"
              data-testid="canvas-gallery-create-btn"
              onClick={() => setSelectedId(null)}
              title={t('canvasGallery.newCanvas')}
              aria-label={t('canvasGallery.newCanvas')}
              className="shell-primary-button inline-flex h-5 w-5 items-center justify-center rounded shadow-sm"
            >
              <Plus size={11} strokeWidth={2.5} aria-hidden />
            </button>
          }
        />
        <div className="shrink-0 space-y-1.5 border-b border-shell-border px-2 py-2">
          <div className="relative flex items-center">
            <Search
              size={12}
              className="pointer-events-none absolute left-2 text-shell-muted"
              aria-hidden
            />
            <input
              id="canvas-gallery-search"
              data-testid="canvas-gallery-search"
              type="search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t('canvasGallery.search')}
              aria-label={t('canvasGallery.search')}
              className="h-7 w-full rounded-md border border-shell-border bg-shell-bg pl-7 pr-2 text-[11px] text-shell-text outline-none placeholder:text-shell-muted focus:border-shell-accent"
            />
          </div>
          {/* Nine kinds in a ~200px rail: one scrollable strip beats five
              wrapped rows eating the list. */}
          <div
            data-testid="canvas-gallery-filters"
            className="shell-scroll -mx-0.5 flex gap-1 overflow-x-auto px-0.5 pb-0.5"
            role="group"
            aria-label={t('canvasGallery.allKinds')}
          >
            {GALLERY_KIND_FILTERS.map((k) => {
              const active = kindFilter === k
              const count = k === 'all' ? items.length : kindCounts[k] || 0
              return (
                <button
                  key={k}
                  type="button"
                  data-testid={`canvas-gallery-filter-${k}`}
                  data-active={active ? 'true' : 'false'}
                  data-count={count}
                  aria-pressed={active}
                  onClick={() => setKindFilter(k)}
                  className={`inline-flex shrink-0 items-center gap-1 rounded border px-1.5 py-0.5 text-[10px] font-medium transition ${
                    active
                      ? 'border-shell-accent bg-shell-active text-shell-accent'
                      : 'border-shell-border text-shell-muted hover:bg-shell-hover hover:text-shell-text'
                  }`}
                >
                  {k === 'all' ? t('canvasGallery.allKinds') : GALLERY_KIND_LABEL[k]}
                  <span className="tabular-nums opacity-70">{count}</span>
                </button>
              )
            })}
          </div>
          {indexing && (
            <p className="px-0.5 text-[10px] text-shell-muted">
              {t('canvasGallery.indexing')}
            </p>
          )}
        </div>
        <ul
          data-testid="canvas-gallery-list"
          className="explorer-scroll min-h-0 flex-1 overflow-auto p-1"
        >
          {filtered.length === 0 && (
            <li className="px-2 py-4 text-center text-[11px] text-shell-muted">
              {t('canvasGallery.empty')}
            </li>
          )}
          {filtered.map((it) => {
            const active = it.id === selectedId
            const renaming = renamingId === it.id
            return (
              <li key={it.id} className="mb-0.5">
                {renaming ? (
                  <div
                    data-testid={`canvas-gallery-rename-${it.id}`}
                    className="rounded-md border border-shell-accent bg-shell-active px-2 py-1.5"
                  >
                    <input
                      data-testid={`canvas-gallery-rename-input-${it.id}`}
                      autoFocus
                      value={renameDraft}
                      onChange={(e) => setRenameDraft(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          e.preventDefault()
                          commitRename()
                        }
                        if (e.key === 'Escape') {
                          e.preventDefault()
                          setRenamingId(null)
                          setRenameDraft('')
                        }
                      }}
                      onBlur={() => commitRename()}
                      className="w-full rounded border border-shell-border bg-shell-bg px-1.5 py-0.5 text-[11px] text-shell-text outline-none focus:border-shell-accent"
                    />
                  </div>
                ) : (
                  <button
                    type="button"
                    data-testid={`canvas-gallery-item-${it.id}`}
                    data-active={active ? 'true' : 'false'}
                    data-kind={it.kind}
                    aria-current={active ? 'true' : undefined}
                    onClick={() => setSelectedId(it.id)}
                    onContextMenu={(e) => {
                      e.preventDefault()
                      e.stopPropagation()
                      setCtxMenu({ x: e.clientX, y: e.clientY, item: it })
                    }}
                    title={`${it.title} — right-click for actions`}
                    className="shell-list-item group w-full min-w-0 rounded-md px-2 py-1.5 text-left"
                  >
                    <div className="min-w-0 truncate text-[11px] font-medium text-shell-text">
                      {it.title}
                    </div>
                    <div className="mt-0.5 flex min-w-0 items-center gap-1 text-[10px] text-shell-muted">
                      <span className="shrink-0 rounded border border-shell-border px-1 py-px text-[9px] font-medium uppercase tracking-wide">
                        {GALLERY_KIND_LABEL[it.kind]}
                      </span>
                      <span className="min-w-0 flex-1 truncate">
                        {it.sessionId === GALLERY_GLOBAL_SCOPE
                          ? t('canvasGallery.library')
                          : it.sessionName}
                      </span>
                      <span className="shrink-0 tabular-nums">
                        {formatGalleryTime(it.updatedAt)}
                      </span>
                    </div>
                  </button>
                )}
              </li>
            )
          })}
        </ul>
      </aside>

      <VerticalSplitter
        testId="canvas-gallery-splitter"
        value={listWidth}
        onChange={(w) => setListWidth(Math.max(200, Math.min(420, w)))}
        growSide="left"
        minOpposite={320}
        aria-label="Resize canvas list"
      />

      <section className="playground-workbench min-h-0 min-w-0 flex-1 overflow-hidden">
        {/* Doc panels render their own error row; this covers the other kinds
            so a storage failure is never only a console message. */}
        {saveError && (!selected || !isDocKind(selected.kind)) ? (
          <div
            data-testid="canvas-gallery-storage-error"
            role="alert"
            className="flex shrink-0 items-start gap-1.5 border-b border-shell-border bg-shell-panel px-2 py-1.5 text-[10px] text-red-400"
          >
            <span className="min-w-0 flex-1 break-words">{saveError}</span>
            <button
              type="button"
              data-testid="canvas-gallery-storage-error-dismiss"
              onClick={() => setSaveError(null)}
              title={t('action.close')}
              aria-label={t('action.close')}
              className="shrink-0 rounded px-1 text-shell-muted transition hover:text-shell-text"
            >
              <X size={11} strokeWidth={2} aria-hidden />
            </button>
          </div>
        ) : null}
        {!selected ? (
          <div
            data-testid="canvas-gallery-empty"
            className="shell-scroll flex min-h-0 flex-1 flex-col items-center justify-center overflow-auto px-6 py-8"
          >
            <div className="mx-auto w-full max-w-2xl">
              <p className="text-center text-sm font-medium text-shell-text">
                {t('canvasGallery.pickTitle')}
              </p>
              <p className="mx-auto mt-1 max-w-md text-center text-[12px] text-shell-muted">
                {t('canvasGallery.pickBody')}
              </p>
              <div
                data-testid="canvas-gallery-kind-grid"
                className="mt-5 grid grid-cols-[repeat(auto-fill,minmax(10rem,1fr))] gap-2"
              >
                {createOptions.map((opt) => {
                  const Icon = opt.icon
                  return (
                    <button
                      key={opt.kind}
                      type="button"
                      data-testid={`canvas-gallery-create-${opt.kind}`}
                      data-kind={opt.kind}
                      onClick={() => openCreateWizard(opt.kind)}
                      className="flex min-h-[5.5rem] flex-col items-start gap-2 rounded-lg border border-shell-border bg-shell-panel p-3 text-left transition hover:border-shell-accent hover:bg-shell-hover"
                    >
                      <span className="flex h-8 w-8 items-center justify-center rounded-md bg-shell-bg text-shell-accent">
                        <Icon size={SHELL.iconSm} aria-hidden />
                      </span>
                      <span className="min-w-0">
                        <span className="block text-[12px] font-medium text-shell-text">
                          {opt.title}
                        </span>
                        <span className="mt-0.5 block text-[10px] leading-snug text-shell-muted">
                          {opt.hint}
                        </span>
                      </span>
                    </button>
                  )
                })}
              </div>
            </div>
          </div>
        ) : selected.kind === 'diagram' ? (
          <>
            <div className="shell-scroll flex min-h-0 flex-1 flex-col overflow-hidden">
              <MermaidDiagram
                source={diagramSource}
                testId="canvas-gallery-mermaid"
                downloadName={`${selected.title || 'diagram'}.svg`}
                fill
                flush
                headerTitle={
                  selected.sessionId === GALLERY_GLOBAL_SCOPE
                    ? selected.title
                    : `${selected.title} · ${selected.sessionName}`
                }
                className="min-h-0 flex-1"
                editorPath={`gallery://${selected.id}.mmd`}
                onSourceChange={onDiagramSourceChange}
                onSourceSave={onDiagramSourceSave}
                dirty={diagramDirty}
                saveFlash={saveFlash}
                saveLabel={t('action.save')}
                unsavedLabel={t('editor.unsaved')}
                savedLabel={t('canvasGallery.saved')}
                saveTitle={t('editor.saveTitle')}
                onSaveClick={() => {
                  persistDiagram(diagramSourceRef.current, {
                    flash: true,
                    galleryId: selected.id,
                  })
                }}
              />
            </div>
            <DiagramAgentPanel
              contextId={
                selected.sessionId === GALLERY_GLOBAL_SCOPE
                  ? undefined
                  : selected.sessionId
              }
              canvasKey={selected.id}
              canvasEntrySessionId={selected.sessionId}
              currentSource={diagramSource}
              onApplySource={applyDiagramFromAgent}
            />
          </>
        ) : selected.kind === 'image' ? (
          <ImageStudioPanel
            contextId={
              selected.sessionId === GALLERY_GLOBAL_SCOPE
                ? undefined
                : selected.sessionId
            }
            canvasKey={selected.id}
            canvasEntrySessionId={selected.sessionId}
            title={
              selected.sessionId === GALLERY_GLOBAL_SCOPE
                ? selected.title
                : `${selected.title} · ${selected.sessionName}`
            }
            prompt={imagePrompt}
            agentPrompt={imageAgentPrompt}
            imageUrl={imageUrl}
            imageAspect={imageResultAspect}
            caption={imageCaption}
            history={imageHistory}
            options={imageOptions}
            onOptionsChange={onImageOptionsChange}
            onResult={applyImageResult}
            onRestoreHistory={onRestoreImageHistory}
            onDeleteHistory={onDeleteImageHistory}
          />
        ) : selected.kind === 'slides' ? (
          <SlidesPanel
            contextId={
              selected.sessionId === GALLERY_GLOBAL_SCOPE
                ? undefined
                : selected.sessionId
            }
            canvasKey={selected.id}
            canvasEntrySessionId={selected.sessionId}
            title={
              selected.sessionId === GALLERY_GLOBAL_SCOPE
                ? selected.title
                : `${selected.title} · ${selected.sessionName}`
            }
            contextLabel={
              selected.sessionId === GALLERY_GLOBAL_SCOPE
                ? selected.title
                : selected.sessionName
            }
            content={slidesContent}
            onContentChange={(json) =>
              onSlidesContentChange(json, selected.id)
            }
          />
        ) : selected.kind === 'excalidraw' ? (
          <div
            data-testid="canvas-gallery-excalidraw"
            className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden"
          >
            <div className="shell-scroll flex min-h-0 flex-1 flex-col overflow-hidden">
              <ExcalidrawCanvas
                key={`${selected.id}:${excalidrawRev}`}
                path={`gallery://${selected.id}.excalidraw`}
                exportName={selected.title}
                content={excalidrawContent}
                onChange={(json) => onExcalidrawChange(json, selected.id)}
                chromeTitle={
                  selected.sessionId === GALLERY_GLOBAL_SCOPE
                    ? selected.title
                    : `${selected.title} · ${selected.sessionName}`
                }
                showAIBar={false}
                testId="canvas-gallery-excalidraw-canvas"
              />
            </div>
            <ExcalidrawAgentPanel
              contextId={
                selected.sessionId === GALLERY_GLOBAL_SCOPE
                  ? undefined
                  : selected.sessionId
              }
              canvasKey={selected.id}
              canvasEntrySessionId={selected.sessionId}
              document={excalidrawDoc}
              elements={excalidrawDoc.elements}
              onApplied={(serialized) => {
                onExcalidrawChange(serialized, selected.id, { remount: true })
              }}
            />
          </div>
        ) : selected.kind === 'markdown' ? (
          <MarkdownDocPanel
            contextId={docContextId}
            canvasKey={selected.id}
            canvasEntrySessionId={selected.sessionId}
            title={selected.title}
            scopeLabel={docScopeLabel}
            content={docContent}
            dirty={docDirty}
            saveFlash={saveFlash}
            error={saveError}
            onDismissError={() => setSaveError(null)}
            onContentChange={(next) => onDocContentChange(next, selected.id)}
            onSave={(next) => persistDoc(next, { galleryId: selected.id })}
            onApplyFromAgent={applyDocFromAgent}
          />
        ) : selected.kind === 'table' ? (
          <TableDocPanel
            contextId={docContextId}
            canvasKey={selected.id}
            canvasEntrySessionId={selected.sessionId}
            title={selected.title}
            scopeLabel={docScopeLabel}
            content={docContent}
            dirty={docDirty}
            saveFlash={saveFlash}
            error={saveError}
            onDismissError={() => setSaveError(null)}
            onContentChange={(next) => onDocContentChange(next, selected.id)}
            onSave={(next) => persistDoc(next, { galleryId: selected.id })}
            onApplyFromAgent={applyDocFromAgent}
          />
        ) : selected.kind === 'timeline' ? (
          <TimelineDocPanel
            contextId={docContextId}
            canvasKey={selected.id}
            canvasEntrySessionId={selected.sessionId}
            title={selected.title}
            scopeLabel={docScopeLabel}
            content={docContent}
            dirty={docDirty}
            saveFlash={saveFlash}
            error={saveError}
            onDismissError={() => setSaveError(null)}
            onContentChange={(next) => onDocContentChange(next, selected.id)}
            onSave={(next) => persistDoc(next, { galleryId: selected.id })}
            onApplyFromAgent={applyDocFromAgent}
          />
        ) : (
          <>
            <div
              className="flex min-h-10 shrink-0 flex-wrap items-center gap-1.5 border-b border-shell-border bg-shell-panel px-2 py-1"
              data-testid="canvas-gallery-html-header"
            >
              <span
                className="min-w-0 flex-1 truncate text-[11px] font-semibold text-shell-text"
                title={selected.title}
              >
                {selected.title}
                <span className="font-normal text-shell-muted">
                  {' · '}
                  {selected.sessionId === GALLERY_GLOBAL_SCOPE
                    ? t('canvasGallery.library')
                    : selected.sessionName}
                  {selected.path ? ` · ${selected.path}` : ''}
                </span>
              </span>
              <div
                className="ml-auto flex min-w-0 flex-wrap items-center justify-end gap-1.5"
                data-testid="canvas-gallery-html-toolbar"
              >
                <input ref={htmlImportRef} type="file" accept=".html,.htm,text/html" className="hidden" data-testid="canvas-gallery-html-file" onChange={(event) => { void importHtml(event.target.files?.[0]); event.target.value = '' }} />
                <button type="button" data-testid="canvas-gallery-html-import" className={segIdle} title={t('playground.importHtml')} aria-label={t('playground.importHtml')} onClick={() => htmlImportRef.current?.click()}><Upload size={13} /></button>
                <button type="button" data-testid="canvas-gallery-html-download" className={segIdle} title={t('playground.downloadSource')} aria-label={t('playground.downloadSource')} onClick={downloadHtml}><Download size={13} /></button>
                <button type="button" data-testid="canvas-gallery-html-refresh" className={segIdle} disabled={!showPreview} title={t('action.refresh')} aria-label={t('action.refresh')} onClick={() => setReloadKey((key) => key + 1)}><RefreshCw size={13} /></button>
                <button type="button" data-testid="canvas-gallery-html-save" className={htmlDirty ? segActive : segIdle} disabled={!htmlDirty} title={t('editor.saveTitle')} aria-label={t('action.save')} onClick={() => persistHtml(htmlDraftRef.current, { galleryId: selected.id, flash: true })}><Save size={13} /></button>
                {showPreview && (
                  <div
                    className={`inline-flex ${toolH} overflow-hidden rounded-md border border-shell-border bg-shell-bg`}
                    role="group"
                    aria-label="Device frame"
                  >
                    {DEVICE_PRESETS.map((d, i) => {
                      const Icon = DEVICE_ICONS[d.id]
                      const active = deviceId === d.id
                      return (
                        <button
                          key={d.id}
                          type="button"
                          data-testid={`canvas-gallery-device-${d.id}`}
                          data-active={active ? 'true' : 'false'}
                          title={t(DEVICE_LABEL_KEYS[d.id])}
                          aria-label={t(DEVICE_LABEL_KEYS[d.id])}
                          onClick={() => selectDevice(d.id)}
                          className={`${active ? segActive : segIdle} ${
                            i < DEVICE_PRESETS.length - 1
                              ? 'border-r border-shell-border'
                              : ''
                          }`}
                        >
                          <Icon size={12} strokeWidth={2} aria-hidden />
                        </button>
                      )
                    })}
                    <button
                      type="button"
                      data-testid="canvas-gallery-rotate"
                      title={
                        orientation === 'portrait' ? 'Landscape' : 'Portrait'
                      }
                      aria-label="Rotate"
                      onClick={() =>
                        setOrientation((o) => toggleOrientation(o))
                      }
                      className={`${segIdle} border-l border-shell-border`}
                    >
                      <RotateCw size={12} strokeWidth={2} aria-hidden />
                    </button>
                  </div>
                )}

                <div
                  className={`inline-flex ${toolH} overflow-hidden rounded-md border border-shell-border bg-shell-bg`}
                  role="group"
                  aria-label="Preview and code"
                >
                  <button
                    type="button"
                    data-testid="canvas-gallery-toggle-preview"
                    data-active={showPreview ? 'true' : 'false'}
                    onClick={togglePreview}
                    title="Preview"
                    aria-label="Preview"
                    aria-pressed={showPreview}
                    className={`${showPreview ? segActive : segIdle} border-r border-shell-border`}
                  >
                    {showPreview ? (
                      <Eye size={12} strokeWidth={2} aria-hidden />
                    ) : (
                      <EyeOff size={12} strokeWidth={2} aria-hidden />
                    )}
                  </button>
                  <button
                    type="button"
                    data-testid="canvas-gallery-toggle-code"
                    data-active={showCode ? 'true' : 'false'}
                    onClick={toggleCode}
                    title="Code"
                    aria-label="Code"
                    aria-pressed={showCode}
                    className={showCode ? segActive : segIdle}
                  >
                    <Code2 size={12} strokeWidth={2} aria-hidden />
                  </button>
                </div>
              </div>
            </div>

            <div
              ref={htmlSplitRef}
              data-stacked={htmlNarrow && showCode && showPreview ? 'true' : 'false'}
              className={`shell-scroll flex min-h-0 min-w-0 flex-1 overflow-hidden ${
                htmlNarrow && showCode && showPreview ? 'flex-col' : 'flex-row'
              }`}
            >
              {showCode && (
                <>
                  <div
                    style={
                      htmlNarrow && showPreview
                        ? { flex: '0 0 auto', height: '45%', minHeight: 132 }
                        : showPreview
                          ? { width: editorWidth, flex: 'none' }
                          : { flex: '1 1 auto', minWidth: 0 }
                    }
                    className={`flex min-h-0 min-w-0 flex-col bg-[#1e1e1e] ${
                      htmlNarrow && showPreview
                        ? 'w-full border-b border-shell-border'
                        : 'min-w-[180px] border-r border-shell-border'
                    }`}
                  >
                    <div className="flex h-7 shrink-0 items-center justify-between gap-2 border-b border-shell-border bg-shell-panel px-2">
                      <span className="flex min-w-0 items-center gap-1.5 text-[10px] text-shell-muted">
                        {htmlDirty && (
                          <span
                            data-testid="canvas-gallery-html-editor-dirty"
                            className="inline-block h-1.5 w-1.5 shrink-0 rounded-full bg-shell-accent"
                            title={t('editor.unsaved')}
                            aria-label={t('editor.unsaved')}
                          />
                        )}
                        <span className="truncate font-mono uppercase tracking-wide">
                          HTML
                        </span>
                        <span
                          className={`truncate ${
                            htmlDirty || saveFlash
                              ? 'text-shell-accent'
                              : 'text-shell-muted'
                          }`}
                        >
                          {htmlDirty
                            ? t('editor.unsaved')
                            : saveFlash
                              ? t('canvasGallery.saved')
                              : t('canvasGallery.saveHint')}
                        </span>
                      </span>
                      <button
                        type="button"
                        data-testid="canvas-gallery-html-editor-save"
                        data-dirty={htmlDirty ? 'true' : 'false'}
                        title={t('editor.saveTitle')}
                        aria-label={t('action.save')}
                        disabled={!htmlDirty}
                        onClick={() => {
                          persistHtml(htmlDraftRef.current, {
                            flash: true,
                            galleryId: selected.id,
                          })
                        }}
                        className={`inline-flex items-center gap-0.5 text-[10px] transition ${
                          htmlDirty
                            ? 'text-shell-accent hover:underline'
                            : 'text-shell-muted'
                        } disabled:cursor-default disabled:no-underline`}
                      >
                        <Save size={10} strokeWidth={2} aria-hidden />
                        {t('action.save')}
                      </button>
                    </div>
                    <CodeEditor
                      path={`gallery://${selected.id}.html`}
                      value={htmlDraft}
                      dirty={htmlDirty}
                      onChange={onHtmlDraftChange}
                      onSave={(v) => {
                        persistHtml(v, {
                          flash: true,
                          galleryId: selected.id,
                        })
                      }}
                      testId="canvas-gallery-editor"
                      fontSize={12}
                    />
                  </div>
                  {showPreview && !htmlNarrow && (
                    <VerticalSplitter
                      testId="canvas-gallery-editor-splitter"
                      value={editorWidth}
                      onChange={(w) =>
                        setEditorWidth(Math.max(180, Math.min(720, w)))
                      }
                      growSide="left"
                      minOpposite={200}
                      aria-label="Resize code editor"
                    />
                  )}
                </>
              )}
              {showPreview && (
                <div
                  ref={stageRef}
                  data-testid="canvas-gallery-stage"
                  className="relative flex min-h-[10rem] min-w-0 flex-1 items-center justify-center overflow-hidden bg-shell-bg p-3"
                >
                  <div
                    data-testid="canvas-gallery-frame-scale"
                    data-scale={String(scale)}
                    style={{
                      width: outer.width * scale,
                      height: outer.height * scale,
                    }}
                    className="relative shrink-0 overflow-hidden"
                  >
                    <div
                      data-testid="canvas-gallery-device-frame"
                      data-device={device.id}
                      data-orientation={orientation}
                      className={`absolute left-0 top-0 box-border origin-top-left overflow-hidden ${frameChromeClass(device.chrome)}`}
                      style={{
                        width: outer.width,
                        height: outer.height,
                        padding: outer.bezel,
                        transform: `scale(${scale})`,
                      }}
                    >
                      <div
                        className="h-full w-full overflow-hidden rounded-[1.35rem] bg-white"
                        style={{
                          width: frame.width,
                          height: frame.height,
                        }}
                      >
                        <iframe
                          key={reloadKey}
                          data-testid="canvas-gallery-iframe"
                          title={selected.title}
                          sandbox="allow-scripts allow-forms allow-modals"
                          srcDoc={srcDoc}
                          className="block border-0 bg-white"
                          style={{
                            width: frame.width,
                            height: frame.height,
                          }}
                        />
                      </div>
                    </div>
                  </div>
                </div>
              )}
            </div>

            <CanvasAgentPanel
              contextId={
                selected.sessionId === GALLERY_GLOBAL_SCOPE
                  ? undefined
                  : selected.sessionId
              }
              canvasKey={selected.id}
              canvasEntrySessionId={selected.sessionId}
              currentHtml={htmlDraft || htmlLive}
              onApplyHtml={applyHtmlFromAgent}
            />
          </>
        )}
        {pendingHtmlImport && (
          <ConfirmDialog
            title={t('playground.importHtml')}
            message={t('playground.importHtmlConfirm', { name: pendingHtmlImport.name })}
            confirmLabel={t('action.apply')}
            onCancel={() => setPendingHtmlImport(null)}
            onConfirm={() => {
              if (selectedRef.current?.id === pendingHtmlImport.galleryId) onHtmlDraftChange(pendingHtmlImport.html)
              setPendingHtmlImport(null)
            }}
          />
        )}
      </section>

      {createWizardKind &&
        typeof document !== 'undefined' &&
        createPortal(
        <div
          data-testid="canvas-gallery-create-wizard-backdrop"
          className="fixed inset-0 z-[110] flex items-center justify-center bg-black/50 p-4"
          role="presentation"
          onClick={closeCreateWizard}
        >
          <div
            data-testid="canvas-gallery-create-wizard"
            data-kind={createWizardKind}
            role="dialog"
            aria-modal="true"
            aria-labelledby="canvas-gallery-create-wizard-title"
            className="flex max-h-[85vh] w-full max-w-md flex-col overflow-hidden rounded-md border border-shell-border bg-shell-panel shadow-xl"
            onClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => {
              if (e.key === 'Escape') {
                e.stopPropagation()
                closeCreateWizard()
              }
            }}
          >
            <div className="shrink-0 border-b border-shell-border px-4 py-3">
              <h2
                id="canvas-gallery-create-wizard-title"
                className="flex items-center gap-1.5 text-sm font-medium text-shell-text"
              >
                {t('canvasGallery.createWizardTitle')}
                <span className="rounded border border-shell-border px-1 py-px text-[9px] font-medium uppercase tracking-wide text-shell-muted">
                  {GALLERY_KIND_LABEL[createWizardKind]}
                </span>
              </h2>
              <p className="mt-1 text-[11px] leading-relaxed text-shell-muted">
                {t('canvasGallery.createWizardBody')}
              </p>
            </div>
            <div className="shell-scroll min-h-0 flex-1 space-y-3 overflow-y-auto px-4 py-3">
              <div>
                <label
                  htmlFor="canvas-gallery-create-wizard-title"
                  className="mb-1 block text-[10px] font-semibold uppercase tracking-wide text-shell-muted"
                >
                  {t('canvasGallery.createWizardTitleField')}
                </label>
                <input
                  id="canvas-gallery-create-wizard-title"
                  data-testid="canvas-gallery-create-wizard-title"
                  type="text"
                  autoFocus
                  value={createTitle}
                  onChange={(e) => setCreateTitle(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      e.preventDefault()
                      confirmCreateCanvas()
                    }
                  }}
                  placeholder={t('canvasGallery.createWizardTitlePlaceholder')}
                  className="w-full rounded-md border border-shell-border bg-shell-bg px-2 py-1.5 text-[11px] text-shell-text outline-none placeholder:text-shell-muted focus:border-shell-accent"
                />
              </div>

              <div>
                <p className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
                  {t('canvasGallery.createWizardScope')}
                </p>
                <button
                  type="button"
                  role="radio"
                  aria-checked={createScopeId === GALLERY_GLOBAL_SCOPE}
                  data-testid="canvas-gallery-create-wizard-library"
                  onClick={() => setCreateScopeId(GALLERY_GLOBAL_SCOPE)}
                  className={`flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-[11px] transition ${
                    createScopeId === GALLERY_GLOBAL_SCOPE
                      ? 'bg-shell-active text-shell-text'
                      : 'text-shell-text hover:bg-shell-hover'
                  }`}
                >
                  <span
                    className={`flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded-full border ${
                      createScopeId === GALLERY_GLOBAL_SCOPE
                        ? 'border-shell-accent'
                        : 'border-shell-border'
                    }`}
                    aria-hidden
                  >
                    {createScopeId === GALLERY_GLOBAL_SCOPE ? (
                      <span className="h-1.5 w-1.5 rounded-full bg-shell-accent" />
                    ) : null}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block font-medium">
                      {t('canvasGallery.createWizardNoSession')}
                    </span>
                    <span className="block text-[10px] text-shell-muted">
                      {t('canvasGallery.createWizardNoSessionHint')}
                    </span>
                  </span>
                </button>
              </div>

              <div>
                <label className="relative flex items-center">
                  <Search
                    size={12}
                    className="pointer-events-none absolute left-2 text-shell-muted"
                    aria-hidden
                  />
                  <input
                    data-testid="canvas-gallery-create-wizard-search"
                    type="text"
                    value={createSessionQuery}
                    onChange={(e) => setCreateSessionQuery(e.target.value)}
                    placeholder={t('canvasGallery.createWizardSearch')}
                    className="w-full rounded-md border border-shell-border bg-shell-bg py-1.5 pl-7 pr-2 text-[11px] text-shell-text outline-none placeholder:text-shell-muted focus:border-shell-accent"
                  />
                </label>
                <ul
                  data-testid="canvas-gallery-create-wizard-list"
                  className="explorer-scroll mt-1.5 max-h-56 space-y-0.5 overflow-y-auto"
                  role="radiogroup"
                  aria-label={t('canvasGallery.createWizardSearch')}
                >
                  {filteredCreateSessions.map((s) => {
                    const active = createScopeId === s.id
                    return (
                      <li key={s.id}>
                        <button
                          type="button"
                          role="radio"
                          aria-checked={active}
                          data-testid={`canvas-gallery-create-wizard-session-${s.id}`}
                          onClick={() => setCreateScopeId(s.id)}
                          className={`flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-[11px] transition ${
                            active
                              ? 'bg-shell-active text-shell-text'
                              : 'text-shell-text hover:bg-shell-hover'
                          }`}
                        >
                          <span
                            className={`flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded-full border ${
                              active
                                ? 'border-shell-accent'
                                : 'border-shell-border'
                            }`}
                            aria-hidden
                          >
                            {active ? (
                              <span className="h-1.5 w-1.5 rounded-full bg-shell-accent" />
                            ) : null}
                          </span>
                          <span className="min-w-0 flex-1">
                            <span className="block truncate font-medium">
                              {s.name}
                            </span>
                            {s.when ? (
                              <span className="block truncate text-[10px] text-shell-muted">
                                {s.when}
                              </span>
                            ) : null}
                          </span>
                        </button>
                      </li>
                    )
                  })}
                  {filteredCreateSessions.length === 0 ? (
                    <li className="px-2 py-3 text-center text-[10px] text-shell-muted">
                      {createSessionQuery.trim()
                        ? t('canvasGallery.createWizardNoMatches')
                        : t('canvasGallery.createWizardNoSessions')}
                    </li>
                  ) : null}
                </ul>
              </div>
            </div>
            <div className="flex shrink-0 items-center justify-between gap-2 border-t border-shell-border px-4 py-3">
              <button
                type="button"
                data-testid="canvas-gallery-create-wizard-cancel"
                className="rounded border border-shell-border px-3 py-1 text-xs text-shell-text hover:bg-shell-hover"
                onClick={closeCreateWizard}
              >
                {t('action.cancel')}
              </button>
              <button
                type="button"
                data-testid="canvas-gallery-create-wizard-create"
                className="shell-primary-button rounded px-3 py-1 text-xs font-medium"
                onClick={confirmCreateCanvas}
              >
                {t('canvasGallery.createWizardCreate')}
              </button>
            </div>
          </div>
        </div>,
          document.body,
        )}

      {deleteTarget && (
        <ConfirmDialog
          title={t('canvasGallery.deleteConfirm')}
          message={t('canvasGallery.deleteMessage', {
            name: deleteTarget.title,
          })}
          confirmLabel={t('action.delete')}
          danger
          onConfirm={confirmDelete}
          onCancel={() => setDeleteTarget(null)}
        />
      )}

      {ctxMenu &&
        typeof document !== 'undefined' &&
        createPortal(
          <GalleryContextMenu
            x={ctxMenu.x}
            y={ctxMenu.y}
            item={ctxMenu.item}
            openLabel={t('canvasGallery.ctxOpen')}
            openSessionLabel={t('canvasGallery.ctxOpenSession')}
            renameLabel={t('canvasGallery.ctxRename')}
            deleteLabel={t('canvasGallery.ctxDelete')}
            canOpenSession={
              !!onOpenSession &&
              ctxMenu.item.sessionId !== GALLERY_GLOBAL_SCOPE
            }
            onClose={() => setCtxMenu(null)}
            onOpen={() => {
              setSelectedId(ctxMenu.item.id)
              setCtxMenu(null)
            }}
            onOpenSession={() => {
              onOpenSession?.(ctxMenu.item.sessionId)
              setCtxMenu(null)
            }}
            onRename={() => startRename(ctxMenu.item)}
            onDelete={() => {
              setDeleteTarget(ctxMenu.item)
              setCtxMenu(null)
            }}
          />,
          document.body,
        )}
    </div>
  )
}

type GalleryCreateOption = {
  kind: GalleryCanvasKind
  icon: LucideIcon
  title: string
  hint: string
}

const GALLERY_KIND_ICON: Record<GalleryCanvasKind, LucideIcon> = {
  html: LayoutTemplate,
  diagram: Workflow,
  image: ImageIcon,
  excalidraw: PenLine,
  slides: Presentation,
  markdown: FileText,
  table: Table2,
  timeline: CalendarRange,
}

const GALLERY_KIND_COPY: Record<
  GalleryCanvasKind,
  { title: MessageKey; hint: MessageKey }
> = {
  html: {
    title: 'canvasGallery.kindHtml',
    hint: 'canvasGallery.kindHtmlHint',
  },
  diagram: {
    title: 'canvasGallery.kindDiagram',
    hint: 'canvasGallery.kindDiagramHint',
  },
  image: {
    title: 'canvasGallery.kindImage',
    hint: 'canvasGallery.kindImageHint',
  },
  excalidraw: {
    title: 'canvasGallery.kindExcalidraw',
    hint: 'canvasGallery.kindExcalidrawHint',
  },
  slides: {
    title: 'canvasGallery.kindSlides',
    hint: 'canvasGallery.kindSlidesHint',
  },
  markdown: {
    title: 'canvasGallery.kindMarkdown',
    hint: 'canvasGallery.kindMarkdownHint',
  },
  table: {
    title: 'canvasGallery.kindTable',
    hint: 'canvasGallery.kindTableHint',
  },
  timeline: {
    title: 'canvasGallery.kindTimeline',
    hint: 'canvasGallery.kindTimelineHint',
  },
}

const GALLERY_CREATE_ORDER: GalleryCanvasKind[] = [
  'html',
  'diagram',
  'image',
  'excalidraw',
  'slides',
  'markdown',
  'table',
  'timeline',
]

function galleryCreateOptions(
  t: (key: MessageKey) => string,
): GalleryCreateOption[] {
  return GALLERY_CREATE_ORDER.map((kind) => ({
    kind,
    icon: GALLERY_KIND_ICON[kind],
    title: t(GALLERY_KIND_COPY[kind].title),
    hint: t(GALLERY_KIND_COPY[kind].hint),
  }))
}

function GalleryContextMenu({
  x,
  y,
  item,
  openLabel,
  openSessionLabel,
  renameLabel,
  deleteLabel,
  canOpenSession,
  onClose,
  onOpen,
  onOpenSession,
  onRename,
  onDelete,
}: {
  x: number
  y: number
  item: GalleryCanvasItem
  openLabel: string
  openSessionLabel: string
  renameLabel: string
  deleteLabel: string
  canOpenSession: boolean
  onClose: () => void
  onOpen: () => void
  onOpenSession: () => void
  onRename: () => void
  onDelete: () => void
}) {
  const ref = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    const onDown = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) onClose()
    }
    window.addEventListener('keydown', onKey)
    window.addEventListener('mousedown', onDown)
    return () => {
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('mousedown', onDown)
    }
  }, [onClose])

  const left = Math.min(
    x,
    Math.max(8, (typeof window !== 'undefined' ? window.innerWidth : x) - 180),
  )
  const top = Math.min(
    y,
    Math.max(8, (typeof window !== 'undefined' ? window.innerHeight : y) - 160),
  )
  const itemCls =
    'flex w-full items-center px-2.5 py-1.5 text-left text-[11px] text-shell-text hover:bg-shell-hover disabled:opacity-40'

  return (
    <div
      ref={ref}
      data-testid="canvas-gallery-context-menu"
      role="menu"
      aria-label={item.title}
      className="fixed z-[220] min-w-[10rem] rounded-md border border-shell-border bg-shell-panel py-0.5 shadow-lg shadow-black/40"
      style={{ left, top }}
    >
      <button
        type="button"
        role="menuitem"
        data-testid="canvas-gallery-ctx-open"
        className={itemCls}
        onClick={onOpen}
      >
        {openLabel}
      </button>
      <button
        type="button"
        role="menuitem"
        data-testid="canvas-gallery-ctx-rename"
        className={itemCls}
        onClick={onRename}
      >
        {renameLabel}
      </button>
      {canOpenSession && (
        <button
          type="button"
          role="menuitem"
          data-testid="canvas-gallery-ctx-open-session"
          className={itemCls}
          onClick={onOpenSession}
        >
          {openSessionLabel}
        </button>
      )}
      <div role="separator" className="my-0.5 border-t border-shell-border" />
      <button
        type="button"
        role="menuitem"
        data-testid="canvas-gallery-ctx-delete"
        className={`${itemCls} text-red-300 hover:text-red-200`}
        onClick={onDelete}
      >
        {deleteLabel}
      </button>
    </div>
  )
}
