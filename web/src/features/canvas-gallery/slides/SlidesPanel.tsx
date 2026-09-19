import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from 'react'
import { createPortal } from 'react-dom'
import {
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  ChevronsDown,
  ChevronsUp,
  Code2,
  Copy,
  Download,
  ExternalLink,
  FilePlus2,
  FileText,
  Loader2,
  Plus,
  Presentation,
  Settings2,
  Trash2,
  X,
} from 'lucide-react'
import { ConfirmDialog } from '../../explorer/ConfirmDialog'
import { useLocale } from '../../i18n/LocaleProvider'
import type { MessageKey } from '../../i18n/messages'
import {
  deleteSlideFromHtml,
  duplicateSlideInHtml,
  emptyHtmlDeck,
  forcePlaceImageInSlide,
  insertBlankSlide,
  isLegacyExcalidrawSlides,
  listSlidesFromHtml,
  presentSrcDoc,
  previewSrcDoc,
  reorderSlidesInHtml,
  SLIDE_H,
  SLIDE_W,
  type SlideMeta,
} from './htmlDeck'
import {
  getSlideImageJob,
  runSlideImageJobs,
  stopSlideImageJobs,
  subscribeSlideImageJobs,
} from './slideImageJobs'
import {
  getCanvasAgentJob,
  subscribeCanvasAgentJobs,
} from '../../web-preview/canvasAgentJobs'
import { SlidesAgentPanel, abortSlidesAgent } from './SlidesAgentPanel'
import { SlideContextMenu, SlideMenuButton } from './SlideContextMenu'
import { SlidesSetupWizard, deckNeedsWizard } from './SlidesSetupWizard'
import { markWizardDone } from './slidesWizard'
import { exportToPdf, exportToPptx } from './slidesExport'
import { resolvePlaygroundScope } from '../playgroundScope'
import { HtmlCodeEditor } from '../../chat/HtmlCodeEditor'
import { processAttachmentFile } from '../../chat/chatAttachments'

type ExportFormat = 'pdf' | 'pptx-hybrid' | 'pptx-visual' | 'gslides'

const GOOGLE_SLIDES_IMPORT_URL = 'https://docs.google.com/presentation/u/0/create'

const EXPORT_ITEMS: {
  fmt: ExportFormat
  labelKey: MessageKey
  hintKey: MessageKey
  icon: typeof FileText
  recommended?: boolean
}[] = [
  {
    fmt: 'pptx-visual',
    labelKey: 'slides.export.pptxVisual',
    hintKey: 'slides.export.pptxVisualHint',
    icon: Presentation,
    recommended: true,
  },
  {
    fmt: 'pptx-hybrid',
    labelKey: 'slides.export.pptxEditable',
    hintKey: 'slides.export.pptxEditableHint',
    icon: Presentation,
  },
  {
    fmt: 'pdf',
    labelKey: 'slides.export.pdf',
    hintKey: 'slides.export.pdfHint',
    icon: FileText,
  },
  {
    fmt: 'gslides',
    labelKey: 'slides.export.gslides',
    hintKey: 'slides.export.gslidesHint',
    icon: ExternalLink,
  },
]

function ExportMenu({
  html,
  title,
  slides,
  contextLabel,
}: {
  html: string
  title: string
  slides: SlideMeta[]
  contextLabel?: string
}) {
  const { t } = useLocale()
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState<ExportFormat | null>(null)
  const [error, setError] = useState<string | null>(null)
  // Set after a Google Slides export so the importer link is rendered as an
  // anchor the user clicks. Opening the tab straight from the async export
  // would be a programmatic popup with no user gesture, which browsers block.
  const [importPrompt, setImportPrompt] = useState(false)
  const rootRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!open) return
    const onDoc = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    const onEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDoc)
    document.addEventListener('keydown', onEsc)
    return () => {
      document.removeEventListener('mousedown', onDoc)
      document.removeEventListener('keydown', onEsc)
    }
  }, [open])

  const handleClick = async (fmt: ExportFormat) => {
    setOpen(false)
    setError(null)
    setImportPrompt(false)
    setBusy(fmt)
    try {
      if (fmt === 'pdf') {
        exportToPdf(html)
      } else {
        // Google Slides has no unauthenticated import API, so the honest
        // workflow is a .pptx plus a link to Google's importer. No OAuth is
        // implied or faked. Its .pptx is the editable build, because an
        // imported deck of flat pictures would not be usable in Slides.
        await exportToPptx(
          slides,
          title,
          html,
          fmt === 'pptx-visual' ? 'visual' : 'hybrid',
          contextLabel,
        )
        if (fmt === 'gslides') setImportPrompt(true)
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(null)
    }
  }

  return (
    <div ref={rootRef} className="relative">
      <button
        type="button"
        data-testid="slides-export-trigger"
        onClick={() => setOpen((v) => !v)}
        disabled={busy !== null || slides.length === 0}
        className="inline-flex shrink-0 items-center gap-1 rounded-md border border-shell-border px-2 py-1 text-[11px] text-shell-text hover:bg-shell-hover disabled:opacity-40"
        title={t('slides.export.title')}
      >
        {busy ? (
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
        ) : (
          <Download className="h-3.5 w-3.5" />
        )}
        {busy ? t('slides.export.busy') : t('slides.export.title')}
        <ChevronDown className="h-3 w-3 opacity-60" />
      </button>
      {open && (
        <div
          data-testid="slides-export-menu"
          className="absolute right-0 top-full z-50 mt-1 w-[290px] rounded-md border border-shell-border bg-shell-panel py-1 shadow-xl"
        >
          <div className="px-2 pb-1 text-[10px] uppercase tracking-wide text-shell-muted">
            {t('slides.export.title')} · {slides.length} slide
            {slides.length === 1 ? '' : 's'}
          </div>
          {EXPORT_ITEMS.map((it) => {
            const Icon = it.icon
            return (
              <button
                key={it.fmt}
                type="button"
                data-testid={`slides-export-${it.fmt}`}
                onClick={() => void handleClick(it.fmt)}
                className="flex w-full items-start gap-2 px-2 py-1.5 text-left text-[11px] text-shell-text hover:bg-shell-hover"
              >
                <Icon className="mt-0.5 h-3.5 w-3.5 shrink-0 text-shell-muted" />
                <span className="flex min-w-0 flex-col">
                  <span className="flex items-center gap-1.5">
                    <span className="font-medium">{t(it.labelKey)}</span>
                    {it.recommended && (
                      <span
                        data-testid="slides-export-recommended"
                        className="rounded-sm border border-shell-border px-1 py-px text-[9px] font-medium uppercase tracking-wide text-shell-muted"
                      >
                        {t('slides.export.recommended')}
                      </span>
                    )}
                  </span>
                  <span className="text-[10px] leading-snug text-shell-muted">
                    {t(it.hintKey)}
                  </span>
                </span>
              </button>
            )
          })}
          <p
            data-testid="slides-export-fidelity-note"
            className="mt-1 border-t border-shell-border px-2 pt-1.5 text-[10px] leading-snug text-shell-muted"
          >
            {t('slides.export.fidelityNote')}
          </p>
        </div>
      )}
      {importPrompt && (
        <div
          data-testid="slides-export-notice"
          role="status"
          className="absolute right-0 top-full z-50 mt-1 w-[290px] rounded-md border border-shell-border bg-shell-panel p-2 text-[10px] leading-snug text-shell-text shadow-xl"
        >
          {t('slides.export.gslidesStep')}
          <div className="mt-1.5 flex items-center gap-1.5">
            <a
              data-testid="slides-export-gslides-open"
              href={GOOGLE_SLIDES_IMPORT_URL}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 rounded border border-shell-border px-2 py-0.5 text-[10px] text-shell-text hover:bg-shell-hover"
            >
              <ExternalLink className="h-3 w-3" />
              {t('slides.export.openGoogleSlides')}
            </a>
            <button
              type="button"
              onClick={() => setImportPrompt(false)}
              className="rounded border border-shell-border px-2 py-0.5 text-[10px] text-shell-muted hover:bg-shell-hover"
            >
              {t('slides.export.dismiss')}
            </button>
          </div>
        </div>
      )}
      {error && (
        <div
          data-testid="slides-export-error"
          role="alert"
          className="absolute right-0 top-full z-50 mt-1 w-[290px] rounded-md border border-red-500/40 bg-shell-panel p-2 text-[10px] leading-snug text-red-300 shadow-xl"
        >
          <span className="font-semibold">{t('slides.export.failed')}.</span>{' '}
          {error}
          <button
            type="button"
            onClick={() => setError(null)}
            className="mt-1.5 block rounded border border-shell-border px-2 py-0.5 text-[10px] text-shell-muted hover:bg-shell-hover"
          >
            {t('slides.export.dismiss')}
          </button>
        </div>
      )}
    </div>
  )
}

type Props = {
  contextId?: string
  canvasKey: string
  canvasEntrySessionId?: string
  title: string
  content: string
  onContentChange: (content: string) => void
  contextLabel?: string
}

function looksLikeHtmlDeck(raw: string): boolean {
  if (!raw) return false
  if (/inferenesia-deck/i.test(raw)) return true
  if (/<!DOCTYPE\s+html|<html[\s>]/i.test(raw)) return true
  if (/<section\b[^>]*\bslide\b/i.test(raw)) return true
  if (/class\s*=\s*["'][^"']*\bslide\b/i.test(raw)) return true
  return false
}

function normalizeDeckContent(content: string, title: string): string {
  const raw = (content || '').trim()
  if (isLegacyExcalidrawSlides(raw)) {
    return emptyHtmlDeck(title || 'Untitled deck')
  }
  if (looksLikeHtmlDeck(raw)) return raw
  if (!raw) return emptyHtmlDeck(title || 'Untitled deck')
  return raw
}

export function SlidesPanel({
  contextId,
  canvasKey,
  canvasEntrySessionId,
  title,
  content,
  onContentChange,
  contextLabel,
}: Props) {
  const playgroundScope = resolvePlaygroundScope(
    'slides',
    canvasEntrySessionId,
    contextId,
  )
  const html = useMemo(
    () => normalizeDeckContent(content, title),
    [content, title],
  )
  const slides = useMemo(() => listSlidesFromHtml(html), [html])

  const [activeId, setActiveId] = useState<string | null>(
    slides[0]?.id ?? null,
  )
  const [refIds, setRefIds] = useState<string[]>([])
  const [slideshow, setSlideshow] = useState(false)
  const [showSource, setShowSource] = useState(false)
  const [sourceDraft, setSourceDraft] = useState(html)
  const [sourceDirty, setSourceDirty] = useState(false)
  const [stageDrag, setStageDrag] = useState(false)
  const [forceEditor, setForceEditor] = useState(false)
  const [thumbReady, setThumbReady] = useState<Record<string, boolean>>({})
  const [pendingDelete, setPendingDelete] = useState<string | null>(null)
  const [slideError, setSlideError] = useState<string | null>(null)
  const [menuOpen, setMenuOpen] = useState(false)
  const { t } = useLocale()
  const stageRef = useRef<HTMLDivElement | null>(null)
  const workAreaRef = useRef<HTMLDivElement | null>(null)
  const [scale, setScale] = useState(0.5)
  const [filmstripW, setFilmstripW] = useState(140)
  const [dragId, setDragId] = useState<string | null>(null)
  const [dragOverId, setDragOverId] = useState<string | null>(null)
  const [ctxMenu, setCtxMenu] = useState<{
    slideId: string
    x: number
    y: number
  } | null>(null)
  const skipExternal = useRef(false)
  const migrated = useRef(false)
  const imageJobsStarted = useRef(false)
  const resizingStrip = useRef(false)

  const imageJob = useSyncExternalStore(
    (cb) => subscribeSlideImageJobs(canvasKey, cb),
    () => getSlideImageJob(canvasKey),
    () => getSlideImageJob(canvasKey),
  )

  const agentJob = useSyncExternalStore(
    (cb) => subscribeCanvasAgentJobs(cb),
    () => getCanvasAgentJob(canvasKey),
    () => getCanvasAgentJob(canvasKey),
  )

  const showWizard = !forceEditor && deckNeedsWizard(content || html)

  useEffect(() => {
    setForceEditor(false)
    setThumbReady({})
    setMenuOpen(false)
    setShowSource(false)
    setSourceDraft(html)
    setSourceDirty(false)
    imageJobsStarted.current = false
  }, [canvasKey])
  useEffect(() => {
    if (!sourceDirty) setSourceDraft(html)
  }, [html, sourceDirty])

  useEffect(() => {
    setMenuOpen(false)
  }, [activeId])

  useEffect(() => {
    if (!slideshow) return
    const handler = (e: MessageEvent) => {
      if (e.data === 'inferenesia:present:exit') setSlideshow(false)
    }
    window.addEventListener('message', handler)
    return () => window.removeEventListener('message', handler)
  }, [slideshow])

  useEffect(() => {
    if (!ctxMenu) return
    const close = () => setCtxMenu(null)
    window.addEventListener('click', close)
    window.addEventListener('contextmenu', close)
    window.addEventListener('scroll', close, true)
    return () => {
      window.removeEventListener('click', close)
      window.removeEventListener('contextmenu', close)
      window.removeEventListener('scroll', close, true)
    }
  }, [ctxMenu])

  useEffect(() => {
    setThumbReady((prev) => {
      const next: Record<string, boolean> = {}
      for (const s of slides) {
        if (prev[s.id]) next[s.id] = prev[s.id]
      }
      return next
    })
  }, [slides])

  useEffect(() => {
    const pending = slides.filter((s) => !thumbReady[s.id])
    if (!pending.length) return
    const timers = pending.map((s) =>
      window.setTimeout(() => {
        setThumbReady((prev) =>
          prev[s.id] ? prev : { ...prev, [s.id]: true },
        )
      }, 3500),
    )
    return () => {
      for (const t of timers) window.clearTimeout(t)
    }
  }, [slides, thumbReady])

  useEffect(() => {
    if (migrated.current) return
    const raw = (content || '').trim()
    if (!raw) return
    if (isLegacyExcalidrawSlides(raw)) {
      migrated.current = true
      onContentChange(emptyHtmlDeck(title || 'Untitled deck'))
      return
    }
    if (looksLikeHtmlDeck(raw)) {
      migrated.current = true
    }
  }, [content, onContentChange, title])

  useEffect(() => {
    if (skipExternal.current) {
      skipExternal.current = false
      return
    }
    if (slides.length && !slides.some((s) => s.id === activeId)) {
      setActiveId(slides[0]?.id ?? null)
    }
  }, [slides, activeId])

  useEffect(() => {
    setRefIds((prev) => prev.filter((id) => slides.some((s) => s.id === id)))
  }, [slides])

  const commit = useCallback(
    (next: string) => {
      skipExternal.current = true
      onContentChange(next)
    },
    [onContentChange],
  )

  const activeIndex = Math.max(
    0,
    slides.findIndex((s) => s.id === activeId),
  )
  const activeSlide: SlideMeta | undefined =
    slides[activeIndex] || slides[0]

  useEffect(() => {
    const el = stageRef.current
    if (!el) return
    const fit = () => {
      const w = el.clientWidth
      const h = el.clientHeight
      if (w < 8 || h < 8) return
      const s = Math.min(w / SLIDE_W, h / SLIDE_H) * 0.96
      setScale(s > 0.05 ? s : 0.05)
    }
    fit()
    const ro = new ResizeObserver(fit)
    ro.observe(el)
    return () => ro.disconnect()
  }, [activeId, html])

  const addBlank = () => {
    const { html: next, slideId } = insertBlankSlide(html, activeId)
    commit(next)
    setActiveId(slideId)
  }

  const performDelete = useCallback(
    (id: string) => {
      abortSlidesAgent(canvasKey)
      stopSlideImageJobs(canvasKey)
      stopSlideImageJobs(`${canvasKey}::img`)
      const next = deleteSlideFromHtml(html, id)
      if (!next) return
      const remaining = listSlidesFromHtml(next)
      if (remaining.some((s) => s.id === id)) return
      if (activeId === id) {
        const nextActive =
          remaining[
            Math.min(activeIndex, Math.max(0, remaining.length - 1))
          ]?.id ?? null
        setActiveId(nextActive)
      }
      setRefIds((prev) => prev.filter((x) => x !== id))
      setThumbReady((prev) => {
        if (!(id in prev)) return prev
        const copy = { ...prev }
        delete copy[id]
        return copy
      })
      commit(next)
    },
    [activeId, activeIndex, canvasKey, commit, html],
  )

  const deleteSlide = (id: string) => {
    setPendingDelete(id)
  }

  const moveSlideTo = (id: string, pos: 'top' | 'bottom') => {
    const ids = slides.map((s) => s.id)
    const without = ids.filter((x) => x !== id)
    if (without.length === 0) return
    const ordered = pos === 'top' ? [id, ...without] : [...without, id]
    const next = reorderSlidesInHtml(html, ordered)
    if (next) commit(next)
  }

  const duplicateSlide = (id: string) => {
    const result = duplicateSlideInHtml(html, id)
    if (!result) {
      setSlideError(t('slides.duplicateFailed'))
      return
    }
    commit(result.html)
    setActiveId(result.slideId)
  }

  // Nudge one position. Drag-and-drop covers long moves, but a keyboard or
  // trackpad user reordering a 20-slide deck needs a single-step action.
  const nudgeSlide = (id: string, delta: -1 | 1) => {
    const ids = slides.map((s) => s.id)
    const from = ids.indexOf(id)
    const to = from + delta
    if (from < 0 || to < 0 || to >= ids.length) return
    const next = [...ids]
    next.splice(from, 1)
    next.splice(to, 0, id)
    const nextHtml = reorderSlidesInHtml(html, next)
    if (nextHtml) commit(nextHtml)
  }

  const openSourceForSlide = (id: string) => {
    setActiveId(id)
    if (!sourceDirty) setSourceDraft(html)
    setShowSource(true)
  }

  const onFilmstripResizeStart = (e: React.MouseEvent) => {
    e.preventDefault()
    resizingStrip.current = true
    const startX = e.clientX
    const startW = filmstripW
    const onMove = (ev: MouseEvent) => {
      if (!resizingStrip.current) return
      const available = workAreaRef.current?.clientWidth ?? 600
      const maxWidth = Math.min(240, Math.max(96, available - 360))
      const next = Math.min(
        maxWidth,
        Math.max(96, startW + (ev.clientX - startX)),
      )
      setFilmstripW(next)
    }
    const onUp = () => {
      resizingStrip.current = false
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }

  const onThumbDragStart = (id: string) => (e: React.DragEvent) => {
    setDragId(id)
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', id)
  }

  const onThumbDragOver = (id: string) => (e: React.DragEvent) => {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
    if (dragOverId !== id) setDragOverId(id)
  }

  const onThumbDrop = (targetId: string) => (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    const sourceId = dragId || e.dataTransfer.getData('text/plain')
    setDragId(null)
    setDragOverId(null)
    if (!sourceId || sourceId === targetId) return
    const ids = slides.map((s) => s.id)
    const from = ids.indexOf(sourceId)
    const to = ids.indexOf(targetId)
    if (from < 0 || to < 0) return
    if (new Set(ids).size !== ids.length) return
    const nextIds = [...ids]
    nextIds.splice(from, 1)
    nextIds.splice(to, 0, sourceId)
    if (nextIds.every((id, i) => id === ids[i])) return
    const nextHtml = reorderSlidesInHtml(html, nextIds)
    if (!nextHtml) return
    if (listSlidesFromHtml(nextHtml).length !== ids.length) return
    commit(nextHtml)
  }

  const onThumbDragEnd = () => {
    setDragId(null)
    setDragOverId(null)
  }

  const toggleRef = (id: string) => {
    setRefIds((prev) =>
      prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id],
    )
  }

  const go = (delta: number) => {
    if (!slides.length) return
    const i = (activeIndex + delta + slides.length) % slides.length
    setActiveId(slides[i].id)
  }

  const previewDoc = useMemo(
    () => previewSrcDoc(html, activeId),
    [html, activeId],
  )
  const presentDoc = useMemo(() => presentSrcDoc(html), [html])

  const thumbSrc = useCallback(
    (slideId: string) => previewSrcDoc(html, slideId, { thumb: true }),
    [html],
  )

  const slotStatus = useCallback(
    (slideId: string) =>
      imageJob.slots.find((s) => s.slideId === slideId)?.status,
    [imageJob.slots],
  )

  useEffect(() => {
    if (!slideshow) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setSlideshow(false)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [slideshow])

  const toggleSource = () => {
    if (!showSource && !sourceDirty) setSourceDraft(html)
    setShowSource((visible) => !visible)
  }

  const applySource = () => {
    commit(sourceDraft)
    setSourceDirty(false)
    setShowSource(false)
  }

  const cancelSource = () => {
    setSourceDraft(html)
    setSourceDirty(false)
    setShowSource(false)
  }

  const onWizardComplete = useCallback(
    (
      nextHtml: string,
      opts?: {
        runAiImages?: boolean
        imageSticky?: { profile?: string; model?: string }
      },
    ) => {
      setForceEditor(true)
      commit(nextHtml)
      const nextSlides = listSlidesFromHtml(nextHtml)
      setActiveId(nextSlides[0]?.id ?? null)
      if (opts?.runAiImages && !imageJobsStarted.current) {
        imageJobsStarted.current = true
        void runSlideImageJobs({
          canvasKey,
          html: nextHtml,
          contextId,
          playgroundId: playgroundScope.playgroundId,
          sticky: opts.imageSticky,
          onHtml: (h) => {
            skipExternal.current = true
            onContentChange(h)
          },
        })
      }
    },
    [canvasKey, commit, onContentChange, playgroundScope, contextId],
  )

  const onWizardSkip = useCallback(() => {
    setForceEditor(true)
    const seed = markWizardDone(
      looksLikeHtmlDeck(content) ? content : emptyHtmlDeck(title || 'Untitled deck'),
    )
    if (seed !== content) commit(seed)
  }, [commit, content, title])

  const onStageDrop = async (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setStageDrag(false)
    const files = Array.from(e.dataTransfer.files || []).filter((f) =>
      f.type.startsWith('image/'),
    )
    if (!files.length || !activeId) return
    const att = await processAttachmentFile(files[0])
    const src = att.dataUrl || att.previewUrl || ''
    if (!src) return
    const next = forcePlaceImageInSlide(
      html,
      activeId,
      { id: att.id, src, alt: att.name },
      'right',
    )
    if (next) commit(next)
  }

  if (showWizard) {
    return (
      <SlidesSetupWizard
        contextId={contextId}
        canvasKey={canvasKey}
        canvasEntrySessionId={canvasEntrySessionId}
        title={title}
        onComplete={onWizardComplete}
        onSkip={onWizardSkip}
      />
    )
  }

  return (
    <div
      data-testid="slides-panel"
      className="playground-workbench bg-shell-bg"
    >
      <div className="flex h-10 shrink-0 items-center gap-2 overflow-hidden border-b border-shell-border bg-shell-panel px-2">
        <span className="min-w-0 truncate text-[11px] font-semibold uppercase tracking-wide text-shell-muted">
          {title || 'Slides'}
        </span>
        <span className="shrink-0 text-[10px] tabular-nums text-shell-muted">
          {slides.length} slide{slides.length === 1 ? '' : 's'} · {SLIDE_W}×
          {SLIDE_H}
        </span>
        {imageJob.busy ? (
          <span
            data-testid="slides-image-job-status"
            className="inline-flex min-w-0 items-center gap-1 truncate text-[10px] text-shell-accent"
          >
            <Loader2 className="h-3 w-3 shrink-0 animate-spin" />
            Generating images…
          </span>
        ) : null}
        <div className="ml-auto flex shrink-0 items-center gap-1">
          <button
            type="button"
            data-testid="slides-present"
            onClick={() => setSlideshow(true)}
            className="inline-flex items-center gap-1 rounded-md border border-shell-border bg-shell-active px-2 py-1 text-[11px] font-medium text-shell-text hover:bg-shell-hover"
            title={`${t('slides.present')} — ${t('slides.presentHint')}`}
          >
            <Presentation className="h-3.5 w-3.5" />
            <span className="hidden sm:inline">{t('slides.present')}</span>
          </button>
          <ExportMenu html={html} title={title} slides={slides} contextLabel={contextLabel} />
        </div>
      </div>

      <div ref={workAreaRef} className="flex min-h-0 min-w-0 flex-1 overflow-hidden">
        <div
          data-testid="slides-filmstrip"
          className="shell-scroll flex shrink-0 flex-col gap-1 overflow-y-auto border-r border-shell-border bg-shell-bg p-1.5"
          style={{ width: filmstripW, maxWidth: '30%' }}
        >
          {slides.map((s, i) => {
            const active = s.id === activeId
            const isRef = refIds.includes(s.id)
            const ready = Boolean(thumbReady[s.id])
            const imgSt = slotStatus(s.id)
            const isDragOver = dragOverId === s.id && dragId !== s.id
            const thumbScale = Math.max(
              0.06,
              Math.min(0.16, (filmstripW - 20) / SLIDE_W),
            )
            return (
              <div
                key={s.id}
                data-testid={`slides-thumb-${s.id}`}
                draggable
                onDragStart={onThumbDragStart(s.id)}
                onDragOver={onThumbDragOver(s.id)}
                onDrop={onThumbDrop(s.id)}
                onDragEnd={onThumbDragEnd}
                onContextMenu={(e) => {
                  e.preventDefault()
                  e.stopPropagation()
                  setCtxMenu({ slideId: s.id, x: e.clientX, y: e.clientY })
                }}
                className={`rounded-md border bg-shell-panel p-1 transition ${
                  active
                    ? 'border-shell-accent bg-shell-active'
                    : isDragOver
                      ? 'border-shell-accent bg-shell-hover'
                      : 'border-shell-border hover:bg-shell-hover'
                } ${dragId === s.id ? 'opacity-50' : ''}`}
              >
                <div className="mb-1 flex items-center gap-1">
                  <button
                    type="button"
                    onClick={() => setActiveId(s.id)}
                    className="min-w-0 flex-1 truncate text-left text-[10px] font-medium text-shell-text"
                    title={`${i + 1}. ${s.title} · ${s.layout}`}
                  >
                    <span className="text-shell-muted">{i + 1}.</span>{' '}
                    <span className="text-shell-muted">{s.layout}</span>
                  </button>
                  <input
                    type="checkbox"
                    data-testid={`slides-ref-${s.id}`}
                    checked={isRef}
                    onChange={() => toggleRef(s.id)}
                    title="Use as ref"
                    aria-label="Use as ref"
                    className="h-3 w-3 shrink-0 rounded border-shell-border"
                  />
                </div>
                <button
                  type="button"
                  onClick={() => setActiveId(s.id)}
                  className="w-full text-left"
                >
                  <div
                    className="relative w-full overflow-hidden rounded border border-shell-border bg-shell-bg"
                    style={{ aspectRatio: `${SLIDE_W} / ${SLIDE_H}` }}
                  >
                    {!ready && (
                      <div className="absolute inset-0 z-[1] flex items-center justify-center bg-shell-panel">
                        <Loader2 className="h-3.5 w-3.5 animate-spin text-shell-muted" />
                      </div>
                    )}
                    {imgSt === 'loading' && (
                      <div className="absolute bottom-0.5 left-0.5 right-0.5 z-[2] rounded bg-black/70 px-1 py-0.5 text-center text-[8px] font-medium text-sky-300">
                        Gen image…
                      </div>
                    )}
                    <iframe
                      title={`thumb-${s.id}`}
                      sandbox="allow-scripts"
                      srcDoc={thumbSrc(s.id)}
                      onLoad={() =>
                        setThumbReady((prev) =>
                          prev[s.id] ? prev : { ...prev, [s.id]: true },
                        )
                      }
                      className="pointer-events-none absolute left-0 top-0 origin-top-left border-0"
                      style={{
                        width: SLIDE_W,
                        height: SLIDE_H,
                        transform: `scale(${thumbScale})`,
                        opacity: ready ? 1 : 0,
                      }}
                    />
                  </div>
                  <div className="mt-1 truncate text-[10px] text-shell-text">
                    {s.title}
                  </div>
                </button>
              </div>
            )
          })}
          <button
            type="button"
            data-testid="slides-filmstrip-add"
            onClick={addBlank}
            className="flex items-center justify-center gap-1 rounded-md border border-dashed border-shell-border py-2 text-[10px] text-shell-muted hover:bg-shell-hover hover:text-shell-text"
          >
            <Plus className="h-3 w-3" />
            {t('slides.addBlank')}
          </button>
        </div>
        <div
          role="separator"
          aria-orientation="vertical"
          data-testid="slides-filmstrip-resizer"
          onMouseDown={onFilmstripResizeStart}
          className="group relative w-1.5 shrink-0 cursor-col-resize bg-transparent hover:bg-shell-hover"
          title="Resize filmstrip"
        >
          <div className="absolute inset-y-0 left-1/2 w-px -translate-x-1/2 bg-shell-border group-hover:bg-shell-accent" />
        </div>

        <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
          <div className="flex h-9 shrink-0 items-center justify-between gap-2 border-b border-shell-border bg-shell-panel px-2">
            <div className="flex min-w-0 items-center gap-1">
              <button
                type="button"
                data-testid="slides-prev"
                onClick={() => go(-1)}
                disabled={slides.length < 2}
                className="rounded border border-shell-border p-1 text-shell-muted hover:bg-shell-hover disabled:opacity-40"
              >
                <ChevronLeft className="h-3.5 w-3.5" />
              </button>
              <button
                type="button"
                data-testid="slides-next"
                onClick={() => go(1)}
                disabled={slides.length < 2}
                className="rounded border border-shell-border p-1 text-shell-muted hover:bg-shell-hover disabled:opacity-40"
              >
                <ChevronRight className="h-3.5 w-3.5" />
              </button>
              <span className="ml-1 truncate text-[11px] text-shell-muted">
                {activeSlide
                  ? `${activeIndex + 1}/${slides.length} · ${activeSlide.title}`
                  : t('slides.noSlides')}
              </span>
            </div>
            <button
              type="button"
              data-testid="slides-source"
              onClick={toggleSource}
              aria-pressed={showSource}
              className={`inline-flex shrink-0 items-center gap-1 rounded-md border px-2 py-1 text-[11px] ${showSource ? 'border-shell-accent bg-shell-active text-shell-text' : 'border-shell-border text-shell-muted hover:bg-shell-hover'}`}
              title={showSource ? 'Hide HTML source' : 'Edit HTML source'}
            >
              <Code2 className="h-3.5 w-3.5" />
              <span className="hidden lg:inline">Source</span>
            </button>
          </div>

          <div className="flex min-h-0 min-w-0 flex-1 overflow-hidden">
            <div
              ref={stageRef}
              data-testid="slides-stage"
              className="relative min-h-0 min-w-0 flex-1 overflow-hidden bg-shell-bg"
              onDragEnter={(e) => {
                if (e.dataTransfer.types.includes('Files')) {
                  e.preventDefault()
                  setStageDrag(true)
                }
              }}
              onDragOver={(e) => {
                if (e.dataTransfer.types.includes('Files')) {
                  e.preventDefault()
                  setStageDrag(true)
                }
              }}
              onDragLeave={() => setStageDrag(false)}
              onDrop={(e) => void onStageDrop(e)}
            >
              {stageDrag && (
                <div className="pointer-events-none absolute inset-0 z-10 flex items-center justify-center border-2 border-dashed border-shell-accent bg-shell-active text-xs font-medium text-shell-accent">
                  Drop image onto slide
                </div>
              )}
              <div className="absolute inset-0 flex items-center justify-center p-3">
                <div
                  className="relative shrink-0 overflow-hidden rounded-lg border border-shell-border bg-shell-panel shadow-2xl shadow-black/50"
                  style={{
                    width: SLIDE_W * scale,
                    height: SLIDE_H * scale,
                  }}
                >
                  <iframe
                    key={`${activeId || 'empty'}-${html.length}`}
                    title="slide-preview"
                    data-testid="slides-preview-iframe"
                    sandbox="allow-scripts"
                    srcDoc={previewDoc}
                    className="pointer-events-none absolute left-0 top-0 border-0 bg-transparent"
                    style={{
                      width: SLIDE_W,
                      height: SLIDE_H,
                      transform: `scale(${scale})`,
                      transformOrigin: 'top left',
                    }}
                  />
                  {activeId && (
                    <SlideMenuButton
                      busy={agentJob.busy}
                      onClick={() => setMenuOpen((v) => !v)}
                    />
                  )}
                  {activeId && menuOpen && (
                    <SlideContextMenu
                      canvasKey={canvasKey}
                      slideId={activeId}
                      busy={agentJob.busy}
                      onClose={() => setMenuOpen(false)}
                    />
                  )}
                </div>
              </div>
            </div>

            {showSource && (
              <aside
                data-testid="slides-source-pane"
                className="flex min-h-0 w-[46%] min-w-[180px] max-w-[560px] shrink-0 flex-col border-l border-shell-border bg-shell-panel"
              >
                <div className="flex h-9 shrink-0 items-center gap-1 overflow-x-auto border-b border-shell-border px-2">
                  <span className="mr-auto shrink-0 text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
                    deck.html
                  </span>
                  <button
                    type="button"
                    data-testid="slides-source-setup"
                    onClick={() => {
                      setShowSource(false)
                      setForceEditor(false)
                    }}
                    className="inline-flex shrink-0 items-center gap-1 rounded-md border border-shell-border px-2 py-1 text-[11px] text-shell-muted hover:bg-shell-hover"
                    title="Buka setup wizard"
                  >
                    <Settings2 className="h-3 w-3" />
                    Setup
                  </button>
                  <button
                    type="button"
                    data-testid="slides-source-blank"
                    onClick={() => {
                      const { html: next, slideId } = insertBlankSlide(
                        sourceDraft,
                        activeId,
                      )
                      setSourceDraft(next)
                      setSourceDirty(true)
                      setActiveId(slideId)
                    }}
                    className="inline-flex shrink-0 items-center gap-1 rounded-md border border-shell-border px-2 py-1 text-[11px] text-shell-text hover:bg-shell-hover"
                    title={t('slides.addBlank')}
                  >
                    <FilePlus2 className="h-3 w-3" />
                    {t('slides.addBlank')}
                  </button>
                  <button
                    type="button"
                    data-testid="slides-source-delete"
                    onClick={() => {
                      if (!activeId) return
                      const next = deleteSlideFromHtml(sourceDraft, activeId)
                      if (!next) return
                      const remaining = listSlidesFromHtml(next)
                      setSourceDraft(next)
                      setSourceDirty(true)
                      if (remaining.some((s) => s.id === activeId)) return
                      setActiveId(remaining[0]?.id ?? null)
                    }}
                    disabled={!activeId || listSlidesFromHtml(sourceDraft).length <= 1}
                    className="inline-flex shrink-0 items-center rounded-md border border-shell-border p-1 text-shell-muted hover:bg-shell-hover disabled:opacity-40"
                    title="Hapus slide aktif"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                  <button
                    type="button"
                    onClick={toggleSource}
                    className="shrink-0 rounded p-1 text-shell-muted hover:bg-shell-hover"
                    title="Hide source"
                  >
                    <X className="h-4 w-4" />
                  </button>
                </div>
                <div className="min-h-0 flex-1 overflow-hidden">
                  <HtmlCodeEditor
                    value={sourceDraft}
                    onChange={(next) => {
                      setSourceDraft(next)
                      setSourceDirty(true)
                    }}
                    onSave={applySource}
                    testId="slides-source"
                    label="deck.html"
                    showToolbar={false}
                  />
                </div>
                <div className="flex h-10 shrink-0 items-center justify-between gap-2 border-t border-shell-border px-2">
                  <span className="truncate text-[10px] text-shell-muted">
                    {sourceDirty ? 'Unapplied changes' : 'Source matches preview'}
                  </span>
                  <div className="flex shrink-0 items-center gap-1.5">
                    <button
                      type="button"
                      onClick={cancelSource}
                      className="rounded-md border border-shell-border px-3 py-1 text-[11px] text-shell-muted hover:bg-shell-hover"
                    >
                      Cancel
                    </button>
                    <button
                      type="button"
                      data-testid="slides-source-apply"
                      onClick={applySource}
                      disabled={!sourceDirty}
                      className="shell-primary-button rounded-md px-3 py-1 text-[11px] font-semibold disabled:opacity-40"
                    >
                      Apply
                    </button>
                  </div>
                </div>
              </aside>
            )}
          </div>
        </div>
      </div>

      <SlidesAgentPanel
        contextId={contextId}
        canvasKey={canvasKey}
        canvasEntrySessionId={canvasEntrySessionId}
        html={html}
        activeSlideId={activeId}
        refSlideIds={refIds}
        onApplyHtml={commit}
      />


      {ctxMenu &&
        createPortal(
          <div
            className="fixed z-[95] min-w-[160px] overflow-hidden rounded-lg border border-shell-border bg-shell-panel py-1 shadow-2xl"
            style={{ left: ctxMenu.x, top: ctxMenu.y }}
            data-testid="slides-ctx-menu"
            onClick={(e) => e.stopPropagation()}
            onContextMenu={(e) => e.preventDefault()}
          >
            <button
              type="button"
              data-testid="slides-ctx-edit-code"
              className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[11px] text-shell-text hover:bg-shell-hover"
              onClick={() => {
                openSourceForSlide(ctxMenu.slideId)
                setCtxMenu(null)
              }}
            >
              <Code2 className="h-3 w-3" />
              {t('slides.editHtml')}
            </button>
            <button
              type="button"
              data-testid="slides-ctx-duplicate"
              className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[11px] text-shell-text hover:bg-shell-hover"
              onClick={() => {
                duplicateSlide(ctxMenu.slideId)
                setCtxMenu(null)
              }}
            >
              <Copy className="h-3 w-3" />
              {t('slides.duplicate')}
            </button>
            <div className="my-1 border-t border-shell-border" />
            <button
              type="button"
              data-testid="slides-ctx-move-up"
              disabled={slides.findIndex((s) => s.id === ctxMenu.slideId) <= 0}
              className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[11px] text-shell-text hover:bg-shell-hover disabled:opacity-40"
              onClick={() => {
                nudgeSlide(ctxMenu.slideId, -1)
                setCtxMenu(null)
              }}
            >
              <ChevronLeft className="h-3 w-3 rotate-90" />
              {t('slides.moveUp')}
            </button>
            <button
              type="button"
              data-testid="slides-ctx-move-down"
              disabled={
                slides.findIndex((s) => s.id === ctxMenu.slideId) >=
                slides.length - 1
              }
              className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[11px] text-shell-text hover:bg-shell-hover disabled:opacity-40"
              onClick={() => {
                nudgeSlide(ctxMenu.slideId, 1)
                setCtxMenu(null)
              }}
            >
              <ChevronRight className="h-3 w-3 rotate-90" />
              {t('slides.moveDown')}
            </button>
            <button
              type="button"
              data-testid="slides-ctx-move-top"
              className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[11px] text-shell-text hover:bg-shell-hover"
              onClick={() => {
                moveSlideTo(ctxMenu.slideId, 'top')
                setCtxMenu(null)
              }}
            >
              <ChevronsUp className="h-3 w-3" />
              {t('slides.moveTop')}
            </button>
            <button
              type="button"
              data-testid="slides-ctx-move-bottom"
              className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[11px] text-shell-text hover:bg-shell-hover"
              onClick={() => {
                moveSlideTo(ctxMenu.slideId, 'bottom')
                setCtxMenu(null)
              }}
            >
              <ChevronsDown className="h-3 w-3" />
              {t('slides.moveBottom')}
            </button>
            <div className="my-1 border-t border-shell-border" />
            <button
              type="button"
              data-testid="slides-ctx-delete"
              disabled={slides.length <= 1}
              className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[11px] text-red-400 hover:bg-red-500/10 disabled:opacity-40"
              onClick={() => {
                deleteSlide(ctxMenu.slideId)
                setCtxMenu(null)
              }}
            >
              <Trash2 className="h-3 w-3" />
              {t('slides.deleteConfirm')}
            </button>
          </div>,
          document.body,
        )}

      {slideshow &&
        createPortal(
          <div
            className="fixed inset-0 z-[90] bg-black"
            data-testid="slides-present-overlay"
            tabIndex={-1}
            onKeyDown={(e) => {
              if (e.key === 'Escape') setSlideshow(false)
            }}
          >
            <button
              type="button"
              onClick={() => setSlideshow(false)}
              className="absolute right-3 top-3 z-10 rounded-md border border-white/20 bg-black/50 p-2 text-white hover:bg-black/70"
              title="Exit (Esc)"
            >
              <X className="h-4 w-4" />
            </button>
            <iframe
              title="slides-present"
              sandbox="allow-scripts"
              srcDoc={presentDoc}
              className="h-full w-full border-0"
            />
          </div>,
          document.body,
        )}

      {slideError && (
        <div
          data-testid="slides-error"
          role="alert"
          className="pointer-events-auto absolute bottom-3 left-1/2 z-[96] -translate-x-1/2 rounded-md border border-red-500/40 bg-shell-panel px-3 py-2 text-[11px] text-red-300 shadow-xl"
        >
          {slideError}
          <button
            type="button"
            onClick={() => setSlideError(null)}
            className="ml-2 rounded border border-shell-border px-2 py-0.5 text-[10px] text-shell-muted hover:bg-shell-hover"
          >
            {t('slides.export.dismiss')}
          </button>
        </div>
      )}

      {pendingDelete &&
        createPortal(
          <ConfirmDialog
            title={t('slides.deleteConfirm')}
            message={t('slides.deleteMessage')}
            confirmLabel={t('action.delete')}
            danger
            onConfirm={() => {
              performDelete(pendingDelete)
              setPendingDelete(null)
            }}
            onCancel={() => setPendingDelete(null)}
          />,
          document.body,
        )}
    </div>
  )
}
