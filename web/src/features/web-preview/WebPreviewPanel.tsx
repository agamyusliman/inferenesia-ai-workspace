import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import {
  ArrowLeft,
  Code2,
  Eye,
  EyeOff,
  History,
  Laptop,
  Monitor,
  Plus,
  RefreshCw,
  RotateCw,
  Save,
  Smartphone,
  Tablet,
} from 'lucide-react'

import { formatShortcut } from '../../lib/platform'
import { ConfirmDialog } from '../explorer/ConfirmDialog'
import { CodeEditor } from '../editor/CodeEditor'
import { useLocale } from '../i18n/LocaleProvider'
import type { MessageKey } from '../i18n/messages'
import { PanelHeader } from '../shell/PanelHeader'
import { VerticalSplitter } from '../shell/VerticalSplitter'
import { SHELL } from '../shell/shellTokens'
import { CanvasAgentPanel } from './CanvasAgentPanel'
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
} from './devicePresets'
import {
  applyCanvasSeed,
  canvasHistorySubtitle,
  findCanvasByHtml,
  LIBRARY_SCOPE_ID,
  loadCanvases,
  newCanvasId,
  prepareCanvasSwitch,
  saveCanvases,
  titleFromHtml,
  type CanvasEntry,
} from './canvasStore'
import {
  buildPreviewSrcDoc,
  iframeSandboxForMode,
  type WebPreviewSeed,
} from './webPreview'
import { inferenesiaSnippetDownloadName } from '../chat/codeBlockUtils'

type Props = {
  workspaceId?: string
  workspaceLabel?: string
  storageScopeId?: string
  seed?: WebPreviewSeed | null
  onBack?: () => void
  onHtmlSynced?: () => void
}

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

const BLANK_HTML =
  '<!DOCTYPE html>\n<html lang="en">\n<head>\n  <meta charset="utf-8" />\n  <title>New playground</title>\n</head>\n<body>\n  <h1>New playground</h1>\n</body>\n</html>\n'

export function WebPreviewPanel({
  workspaceId,
  workspaceLabel,
  storageScopeId,
  seed,
  onBack,
  onHtmlSynced,
}: Props) {
  const { t } = useLocale()
  const stageRef = useRef<HTMLDivElement>(null)
  const iframeRef = useRef<HTMLIFrameElement | null>(null)
  const lastSrcDocRef = useRef('')
  const [scopeId, setScopeId] = useState(
    () => seed?.scopeId || storageScopeId || workspaceId || 'global',
  )

  const [canvases, setCanvases] = useState<CanvasEntry[]>(() =>
    loadCanvases(scopeId),
  )
  const [activeId, setActiveId] = useState<string | null>(
    () => loadCanvases(scopeId)[0]?.id ?? null,
  )
  const [historyOpen, setHistoryOpen] = useState(false)
  const [historyWidth, setHistoryWidth] = useState(208)

  const [showCode, setShowCode] = useState(true)
  const [showPreview, setShowPreview] = useState(true)
  const [deviceId, setDeviceId] = useState<DeviceId>('desktop')
  const [orientation, setOrientation] = useState<ScreenOrientation>(
    defaultOrientation('desktop'),
  )
  const [filePath, setFilePath] = useState(
    () => loadCanvases(scopeId)[0]?.path || '',
  )
  const initialHtml =
    loadCanvases(scopeId)[0]?.html ||
    '<!DOCTYPE html>\n<html lang="en">\n<head>\n  <meta charset="utf-8" />\n  <title>Preview</title>\n</head>\n<body>\n  <h1>Hello</h1>\n</body>\n</html>\n'
  const [htmlDraft, setHtmlDraft] = useState(initialHtml)
  const [htmlLive, setHtmlLive] = useState(initialHtml)
  const [reloadKey, setReloadKey] = useState(0)
  const [stageSize, setStageSize] = useState({ w: 800, h: 600 })
  const [editorWidth, setEditorWidth] = useState(420)
  const [ctxMenu, setCtxMenu] = useState<{
    x: number
    y: number
    canvasId: string
  } | null>(null)
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [renameDraft, setRenameDraft] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<CanvasEntry | null>(null)

  const seedNonceRef = useRef<number | undefined>(undefined)
  const activeIdRef = useRef(activeId)
  const canvasesRef = useRef(canvases)
  const htmlDraftRef = useRef(htmlDraft)
  const filePathRef = useRef(filePath)
  const suppressEditorChangeRef = useRef(false)
  const previewThrottleRef = useRef<number | null>(null)
  const pendingPreviewHtmlRef = useRef<string | null>(null)
  activeIdRef.current = activeId
  canvasesRef.current = canvases
  htmlDraftRef.current = htmlDraft
  filePathRef.current = filePath

  const bumpPreview = useCallback((html: string) => {
    const next = buildPreviewSrcDoc(html)
    lastSrcDocRef.current = next
    iframeRef.current = null
    setHtmlLive(html)
    setReloadKey((k) => k + 1)
  }, [])

  const schedulePreviewBump = useCallback(
    (html: string, immediate = false) => {
      pendingPreviewHtmlRef.current = html
      if (immediate) {
        if (previewThrottleRef.current != null) {
          window.clearTimeout(previewThrottleRef.current)
          previewThrottleRef.current = null
        }
        bumpPreview(html)
        return
      }
      if (previewThrottleRef.current != null) return
      previewThrottleRef.current = window.setTimeout(() => {
        previewThrottleRef.current = null
        const pending = pendingPreviewHtmlRef.current
        if (pending != null) bumpPreview(pending)
      }, 380)
    },
    [bumpPreview],
  )

  const activateEntry = useCallback(
    (entry: CanvasEntry) => {
      suppressEditorChangeRef.current = true
      activeIdRef.current = entry.id
      setActiveId(entry.id)
      setHtmlDraft(entry.html)
      htmlDraftRef.current = entry.html
      setFilePath(entry.path || '')
      filePathRef.current = entry.path || ''
      bumpPreview(entry.html)
      queueMicrotask(() => {
        suppressEditorChangeRef.current = false
      })
    },
    [bumpPreview],
  )

  useEffect(() => {
    const next =
      seed?.scopeId || storageScopeId || workspaceId || 'global'
    setScopeId((prev) => (prev === next ? prev : next))
  }, [seed?.scopeId, seed?.nonce, storageScopeId, workspaceId])

  useEffect(() => {
    const list = loadCanvases(scopeId)
    setCanvases(list)
    canvasesRef.current = list
    const preferId =
      (seed?.scopeId === scopeId && seed?.canvasId) || activeIdRef.current
    const preferred =
      (preferId && list.find((c) => c.id === preferId)) || list[0]
    if (preferred) {
      activateEntry(preferred)
    } else {
      setActiveId(null)
      activeIdRef.current = null
    }
  }, [scopeId, activateEntry, seed?.scopeId, seed?.canvasId])

  useEffect(() => {
    if (!seed) return
    if (seed.nonce != null && seed.nonce === seedNonceRef.current) return
    if (!seed.html) return

    let writeScope =
      seed.scopeId || storageScopeId || workspaceId || scopeId || 'global'
    let canvasId = seed.canvasId
    if (!seed.scopeId) {
      const lib = findCanvasByHtml(LIBRARY_SCOPE_ID, seed.html)
      if (lib) {
        writeScope = LIBRARY_SCOPE_ID
        canvasId = canvasId || lib.id
      }
    }

    seedNonceRef.current = seed.nonce
    if (writeScope !== scopeId) {
      setScopeId(writeScope)
    }

    const baseList = loadCanvases(writeScope)
    const applied = applyCanvasSeed(baseList, {
      html: seed.html,
      path: seed.path,
      title: seed.title,
      prefer: seed.prefer || 'edit',
      activeId:
        canvasId ||
        (writeScope === scopeId ? activeIdRef.current : null),
      chatMessageId: seed.chatMessageId,
      sourceHtml: seed.sourceHtml || seed.html,
      canvasId,
    })
    setCanvases(applied.list)
    canvasesRef.current = applied.list
    saveCanvases(writeScope, applied.list)
    const entry = applied.list.find((c) => c.id === applied.activeId)
    if (entry) {
      activateEntry(entry)
      setShowCode(true)
      setShowPreview(true)
    }
  }, [seed, storageScopeId, workspaceId, activateEntry])

  useEffect(() => {
    const el = stageRef.current
    if (!el || !showPreview) return
    const ro = new ResizeObserver((entries) => {
      const cr = entries[0]?.contentRect
      if (!cr) return
      setStageSize({ w: cr.width, h: cr.height })
    })
    ro.observe(el)
    setStageSize({ w: el.clientWidth, h: el.clientHeight })
    return () => ro.disconnect()
  }, [showPreview, showCode, historyOpen])

  const device = getDevicePreset(deviceId)
  const frame = frameSize(device, orientation)
  const outer = outerFrameSize(frame, device.chrome)
  const scale = scaleToFit(outer.width, outer.height, stageSize.w, stageSize.h, 28)

  const selectDevice = (id: DeviceId) => {
    setDeviceId(id)
    setOrientation(defaultOrientation(id))
  }

  const persistActive = useCallback(
    (html: string, path?: string) => {
      const id = activeIdRef.current
      if (!id) return
      setCanvases((prev) => {
        const cur = prev.find((c) => c.id === id)
        if (!cur) return prev
        const nextPath = path ?? cur.path
        if (cur.html === html && (nextPath || '') === (cur.path || '')) {
          return prev
        }
        const next = prev.map((c) =>
          c.id === id
            ? {
                ...c,
                html,
                path: nextPath,
                title: titleFromHtml(html, nextPath) || c.title,
                updatedAt: Date.now(),
                sourceHtml: c.chatMessageId
                  ? c.sourceHtml || c.html
                  : c.sourceHtml,
              }
            : c,
        )
        canvasesRef.current = next
        saveCanvases(scopeId, next)
        return next
      })
    },
    [scopeId],
  )

  const applyHtml = useCallback(() => {
    const html = htmlDraftRef.current
    persistActive(html, filePathRef.current || undefined)
    bumpPreview(html)
    onHtmlSynced?.()
  }, [persistActive, bumpPreview, onHtmlSynced])

  const onEditorChange = (v: string) => {
    if (suppressEditorChangeRef.current) return
    htmlDraftRef.current = v
    setHtmlDraft(v)
  }

  const selectCanvas = (id: string) => {
    setCtxMenu(null)
    const fromId = activeIdRef.current
    if (fromId === id) {
      const same = canvasesRef.current.find((c) => c.id === id)
      if (same) activateEntry(same)
      return
    }
    const { list, entry, dirty } = prepareCanvasSwitch(canvasesRef.current, {
      fromId,
      toId: id,
      draftHtml: htmlDraftRef.current,
      draftPath: filePathRef.current || undefined,
    })
    if (!entry) return
    if (dirty) {
      setCanvases(list)
      canvasesRef.current = list
      saveCanvases(scopeId, list)
    }
    activateEntry(entry)
  }

  const createBlank = () => {
    const fromId = activeIdRef.current
    if (fromId) {
      const { list, dirty } = prepareCanvasSwitch(canvasesRef.current, {
        fromId,
        toId: fromId,
        draftHtml: htmlDraftRef.current,
        draftPath: filePathRef.current || undefined,
      })
      if (dirty) {
        canvasesRef.current = list
      }
    }
    const now = Date.now()
    const entry: CanvasEntry = {
      id: newCanvasId(),
      title: `Playground ${new Date().toLocaleTimeString()}`,
      html: BLANK_HTML,
      createdAt: now,
      updatedAt: now,
    }
    const next = [entry, ...canvasesRef.current]
    setCanvases(next)
    canvasesRef.current = next
    saveCanvases(scopeId, next)
    activateEntry(entry)
  }

  const deleteCanvas = (id: string) => {
    const next = canvasesRef.current.filter((c) => c.id !== id)
    setCanvases(next)
    canvasesRef.current = next
    saveCanvases(scopeId, next)
    setCtxMenu(null)
    if (renamingId === id) {
      setRenamingId(null)
      setRenameDraft('')
    }
    if (activeIdRef.current === id) {
      const first = next[0]
      if (first) activateEntry(first)
      else {
        setActiveId(null)
        activeIdRef.current = null
        setHtmlDraft('')
        setHtmlLive('')
        htmlDraftRef.current = ''
        setFilePath('')
        filePathRef.current = ''
        bumpPreview('')
      }
    }
  }

  const startRenameCanvas = (id: string) => {
    const entry = canvasesRef.current.find((c) => c.id === id)
    if (!entry) return
    setCtxMenu(null)
    setRenamingId(id)
    setRenameDraft(entry.title)
  }

  const commitRenameCanvas = () => {
    const id = renamingId
    if (!id) return
    const title = renameDraft.trim()
    setRenamingId(null)
    setRenameDraft('')
    if (!title) return
    const next = canvasesRef.current.map((c) =>
      c.id === id ? { ...c, title, updatedAt: Date.now() } : c,
    )
    setCanvases(next)
    canvasesRef.current = next
    saveCanvases(scopeId, next)
  }

  const downloadCanvas = (entry: CanvasEntry) => {
    const ordered = canvasesRef.current
      .slice()
      .sort((a, b) => a.createdAt - b.createdAt)
    const idx = ordered.findIndex((c) => c.id === entry.id)
    const sessionLabel = (workspaceLabel || '')
      .replace(/^Session\s*[·•\-–—]\s*/i, '')
      .replace(/^Sesi\s*[·•\-–—]\s*/i, '')
      .replace(/^Workspace\s*[·•\-–—]\s*/i, '')
      .trim()
    const name = entry.path?.split(/[/\\]/).pop()
      ? entry.path!.split(/[/\\]/).pop()!
      : inferenesiaSnippetDownloadName({
          sessionName: sessionLabel || entry.title,
          canvasId: entry.id,
          canvasIndex: idx >= 0 ? idx + 1 : 1,
          ext: 'html',
        })
    const blob = new Blob([entry.html], { type: 'text/html;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = name.endsWith('.html') ? name : `${name}.html`
    a.rel = 'noopener'
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
    setCtxMenu(null)
  }

  const runCtxAction = (action: string, canvasId: string) => {
    const entry = canvasesRef.current.find((c) => c.id === canvasId)
    if (!entry) {
      setCtxMenu(null)
      return
    }
    if (action === 'download') {
      downloadCanvas(entry)
      return
    }
    if (action === 'delete') {
      setCtxMenu(null)
      setDeleteTarget(entry)
      return
    }
    if (action === 'rename') {
      startRenameCanvas(canvasId)
      return
    }
    if (action === 'open') {
      selectCanvas(canvasId)
      return
    }
    setCtxMenu(null)
  }

  const editorPathKey = activeId
    ? `canvas://${activeId}/${filePath || 'preview.html'}`
    : filePath || 'preview.html'

  const reload = () => {
    bumpPreview(htmlLive)
  }

  const toggleHistory = () => {
    setHistoryOpen((v) => !v)
  }

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

  const sandbox = iframeSandboxForMode('html')

  const onIframeMount = useCallback((node: HTMLIFrameElement | null) => {
    iframeRef.current = node
    if (!node) return
    const next = lastSrcDocRef.current
    if (!next) return
    requestAnimationFrame(() => {
      if (iframeRef.current === node) {
        node.srcdoc = next
      }
    })
  }, [])

  const activeTitle =
    canvases.find((c) => c.id === activeId)?.title || t('webPreview.title')

  const subtitle = useMemo(() => {
    const parts: string[] = []
    if (workspaceLabel) parts.push(workspaceLabel)
    parts.push(activeTitle)
    if (showPreview) {
      parts.push(`${frame.width}×${frame.height}`)
    }
    return parts.join(' · ')
  }, [workspaceLabel, activeTitle, frame, showPreview])

  const applyHtmlFromAgent = useCallback(
    (html: string, opts?: { final?: boolean; canvasId?: string }) => {
      const final = opts?.final === true
      const fromKey = opts?.canvasId?.startsWith('preview:')
        ? opts.canvasId.split(':').slice(2).join(':')
        : opts?.canvasId
      let id =
        (fromKey && fromKey !== 'active' ? fromKey : null) ||
        activeIdRef.current
      if (!id) {
        const now = Date.now()
        const entry: CanvasEntry = {
          id: newCanvasId(),
          title: titleFromHtml(html) || 'Playground',
          html,
          createdAt: now,
          updatedAt: now,
          sourceHtml: html,
        }
        const next = [entry, ...canvasesRef.current]
        setCanvases(next)
        canvasesRef.current = next
        saveCanvases(scopeId, next)
        activeIdRef.current = entry.id
        setActiveId(entry.id)
        id = entry.id
      }

      const isViewing = activeIdRef.current === id
      if (isViewing) {
        htmlDraftRef.current = html
        suppressEditorChangeRef.current = true
        setHtmlDraft(html)
      }

      if (final) {
        setCanvases((prev) => {
          const cur = prev.find((c) => c.id === id)
          if (!cur) return prev
          if (cur.html === html) return prev
          const next = prev.map((c) =>
            c.id === id
              ? {
                  ...c,
                  html,
                  title: titleFromHtml(html, c.path) || c.title,
                  updatedAt: Date.now(),
                  sourceHtml: c.sourceHtml || c.html,
                }
              : c,
          )
          canvasesRef.current = next
          saveCanvases(scopeId, next)
          return next
        })
        if (isViewing) {
          schedulePreviewBump(html, true)
          onHtmlSynced?.()
        }
      } else if (isViewing) {
        schedulePreviewBump(html, false)
      }
      if (isViewing) {
        queueMicrotask(() => {
          suppressEditorChangeRef.current = false
        })
      }
    },
    [schedulePreviewBump, onHtmlSynced, scopeId],
  )

  return (
    <div
      data-testid="web-preview-panel"
      data-active-canvas={activeId || ''}
      className="flex h-full min-h-0 w-full min-w-0 flex-1 flex-col overflow-hidden bg-shell-bg"
    >
      <PanelHeader
        testId="web-preview-header"
        title={t('webPreview.title')}
        subtitle={subtitle}
        leading={
          onBack ? (
            <button
              type="button"
              data-testid="web-preview-back"
              onClick={onBack}
              className="flex shrink-0 items-center gap-1 rounded px-1.5 py-1 text-left text-[11px] font-medium text-shell-accent transition hover:bg-shell-accent/15 hover:text-shell-accent"
              title={t('webPreview.backToChat')}
            >
              <ArrowLeft size={14} className="shrink-0" aria-hidden />
              <span>Back</span>
            </button>
          ) : undefined
        }
        actions={
          <div className="flex items-center gap-1">
            <button
              type="button"
              data-testid="web-preview-new-canvas"
              title="New playground"
              onClick={createBlank}
              className="rounded p-1.5 text-shell-muted transition hover:bg-shell-border/40 hover:text-shell-text"
            >
              <Plus size={SHELL.iconSm} strokeWidth={1.75} />
            </button>
            {showPreview && (
              <button
                type="button"
                data-testid="web-preview-reload"
                title={t('action.refresh')}
                onClick={reload}
                className="rounded p-1.5 text-shell-muted transition hover:bg-shell-border/40 hover:text-shell-text"
              >
                <RefreshCw size={SHELL.iconSm} strokeWidth={1.75} />
              </button>
            )}
          </div>
        }
      />

      <div
        className={`${SHELL.toolbarClass} flex-wrap gap-y-1`}
        data-testid="web-preview-toolbar"
      >
        <div
          className="flex items-center gap-0.5 rounded-md border border-shell-border/80 p-0.5"
          role="group"
        >
          <button
            type="button"
            data-testid="web-preview-toggle-history"
            data-active={historyOpen ? 'true' : 'false'}
            onClick={toggleHistory}
            className={`flex items-center gap-1 rounded px-2 py-1 text-[11px] transition ${
              historyOpen
                ? 'bg-shell-active text-shell-text'
                : 'text-shell-muted hover:bg-shell-border/30 hover:text-shell-text'
            }`}
            title="Playground history"
          >
            <History size={12} strokeWidth={1.75} aria-hidden />
            History
          </button>
          <button
            type="button"
            data-testid="web-preview-toggle-code"
            data-active={showCode ? 'true' : 'false'}
            onClick={toggleCode}
            className={`flex items-center gap-1 rounded px-2 py-1 text-[11px] transition ${
              showCode
                ? 'bg-shell-active text-shell-text'
                : 'text-shell-muted hover:bg-shell-border/30 hover:text-shell-text'
            }`}
          >
            <Code2 size={12} strokeWidth={1.75} aria-hidden />
            Code
          </button>
          <button
            type="button"
            data-testid="web-preview-toggle-preview"
            data-active={showPreview ? 'true' : 'false'}
            onClick={togglePreview}
            className={`flex items-center gap-1 rounded px-2 py-1 text-[11px] transition ${
              showPreview
                ? 'bg-shell-active text-shell-text'
                : 'text-shell-muted hover:bg-shell-border/30 hover:text-shell-text'
            }`}
          >
            {showPreview ? (
              <Eye size={12} strokeWidth={1.75} aria-hidden />
            ) : (
              <EyeOff size={12} strokeWidth={1.75} aria-hidden />
            )}
            Preview
          </button>
        </div>

        {showPreview && (
          <div
            className="ml-auto flex items-center gap-0.5 rounded-md border border-shell-border/80 p-0.5"
            role="group"
          >
            {DEVICE_PRESETS.map((d) => {
              const Icon = DEVICE_ICONS[d.id]
              const active = deviceId === d.id
              return (
                <button
                  key={d.id}
                  type="button"
                  data-testid={`web-preview-device-${d.id}`}
                  data-active={active ? 'true' : 'false'}
                  title={t(DEVICE_LABEL_KEYS[d.id])}
                  onClick={() => selectDevice(d.id)}
                  className={`flex items-center gap-1 rounded px-2 py-1 text-[11px] transition ${
                    active
                      ? 'bg-shell-active text-shell-text'
                      : 'text-shell-muted hover:bg-shell-border/30 hover:text-shell-text'
                  }`}
                >
                  <Icon size={12} strokeWidth={1.75} aria-hidden />
                  <span className="hidden sm:inline">
                    {t(DEVICE_LABEL_KEYS[d.id])}
                  </span>
                </button>
              )
            })}
            <button
              type="button"
              data-testid="web-preview-rotate"
              onClick={() => setOrientation((o) => toggleOrientation(o))}
              className="flex items-center gap-1 rounded px-2 py-1 text-[11px] text-shell-muted transition hover:bg-shell-border/30 hover:text-shell-text"
            >
              <RotateCw size={12} strokeWidth={1.75} aria-hidden />
            </button>
          </div>
        )}
      </div>

      <div
        data-testid="web-preview-main"
        className="flex min-h-0 flex-1 overflow-hidden"
      >
        {historyOpen && (
          <>
            <aside
              data-testid="web-preview-history"
              style={{ width: historyWidth, flex: 'none' }}
              className="flex min-h-0 min-w-[140px] max-w-[360px] shrink-0 flex-col border-r border-shell-border bg-shell-panel"
            >
              <div className="border-b border-shell-border px-2 py-1.5 text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
                Canvas history
              </div>
              <ul className="min-h-0 flex-1 overflow-auto p-1">
                {canvases.length === 0 && (
                  <li className="px-2 py-3 text-[11px] text-shell-muted">
                    No canvases yet. Open from chat or create new.
                  </li>
                )}
                {[...canvases]
                  .sort((a, b) => {
                    const ub = b.updatedAt || b.createdAt || 0
                    const ua = a.updatedAt || a.createdAt || 0
                    if (ub !== ua) return ub - ua
                    return (b.createdAt || 0) - (a.createdAt || 0)
                  })
                  .map((c) => {
                  const active = c.id === activeId
                  const isRenaming = renamingId === c.id
                  return (
                    <li key={c.id} className="mb-0.5">
                      {isRenaming ? (
                        <div
                          className={`rounded-md px-1.5 py-1 ${
                            active ? 'bg-shell-active' : 'bg-shell-border/20'
                          }`}
                        >
                          <input
                            data-testid={`canvas-rename-input-${c.id}`}
                            autoFocus
                            value={renameDraft}
                            onChange={(e) => setRenameDraft(e.target.value)}
                            onKeyDown={(e) => {
                              if (e.key === 'Enter') {
                                e.preventDefault()
                                commitRenameCanvas()
                              }
                              if (e.key === 'Escape') {
                                setRenamingId(null)
                                setRenameDraft('')
                              }
                            }}
                            onBlur={() => commitRenameCanvas()}
                            className="w-full rounded border border-shell-border bg-shell-bg px-1.5 py-0.5 text-[11px] text-shell-text outline-none focus:border-shell-accent/50"
                          />
                        </div>
                      ) : (
                        <button
                          type="button"
                          data-testid={`canvas-history-${c.id}`}
                          data-active={active ? 'true' : 'false'}
                          data-canvas-id={c.id}
                          onClick={() => selectCanvas(c.id)}
                          onContextMenu={(e) => {
                            e.preventDefault()
                            e.stopPropagation()
                            setCtxMenu({
                              x: e.clientX,
                              y: e.clientY,
                              canvasId: c.id,
                            })
                          }}
                          title={`${c.title} — right-click for rename / delete`}
                          className="shell-list-item group w-full min-w-0 rounded-md px-2 py-1.5 text-left"
                        >
                          <div
                            className={`truncate text-[11px] font-medium transition-colors ${
                              active
                                ? 'text-shell-text'
                                : 'text-shell-text/90 group-hover:text-shell-text'
                            }`}
                          >
                            {c.title}
                          </div>
                          <div
                            className={`truncate text-[10px] transition-colors ${
                              active
                                ? 'text-shell-accent'
                                : 'text-shell-muted group-hover:text-shell-text/75'
                            }`}
                          >
                            {canvasHistorySubtitle(c)}
                          </div>
                        </button>
                      )}
                    </li>
                  )
                })}
              </ul>
            </aside>
            <VerticalSplitter
              testId="web-preview-history-splitter"
              value={historyWidth}
              onChange={(w) =>
                setHistoryWidth(Math.max(140, Math.min(360, w)))
              }
              growSide="left"
              minOpposite={320}
              aria-label="Resize playground history"
            />
          </>
        )}

        <div
          data-testid="web-preview-workbench"
          className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden"
        >
          <div
            data-testid="web-preview-row-editor"
            className="flex min-h-0 min-w-0 flex-1 overflow-hidden"
          >
            {showCode && (
              <>
                <div
                  data-testid="web-preview-html-editor"
                  style={
                    showPreview
                      ? { width: editorWidth, flex: 'none' }
                      : { flex: '1 1 auto', minWidth: 0 }
                  }
                  className="flex min-h-0 min-w-[180px] flex-col border-r border-shell-border bg-[#1e1e1e]"
                >
                  <div className="flex shrink-0 items-center justify-between gap-2 border-b border-shell-border bg-shell-panel px-2 py-1.5">
                    <span className="min-w-0 truncate text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
                      {activeTitle}
                    </span>
                    <button
                      type="button"
                      data-testid="web-preview-html-apply"
                      onClick={applyHtml}
                      title={`${t('action.save')} (${formatShortcut('S')})`}
                      className="inline-flex shrink-0 items-center gap-1 rounded bg-shell-accent/20 px-1.5 py-0.5 text-[10px] font-medium text-shell-accent transition hover:bg-shell-accent/30"
                    >
                      <Save size={12} strokeWidth={1.75} aria-hidden />
                      <span className="sr-only">{t('action.save')}</span>
                    </button>
                  </div>
                  <CodeEditor
                    key={activeId || 'none'}
                    path={editorPathKey}
                    value={htmlDraft}
                    onChange={onEditorChange}
                    onSave={applyHtml}
                    testId="web-preview-html-monaco"
                  />
                </div>
                {showPreview && (
                  <VerticalSplitter
                    testId="web-preview-editor-splitter"
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

            {showPreview ? (
              <div
                ref={stageRef}
                data-testid="web-preview-stage"
                className="relative flex min-h-0 min-w-0 flex-1 items-center justify-center overflow-hidden bg-[radial-gradient(ellipse_at_center,_var(--tw-gradient-stops))] from-shell-panel/80 via-shell-bg to-shell-bg p-4"
              >
                <div
                  data-testid="web-preview-frame-scale"
                  style={{
                    width: outer.width * scale,
                    height: outer.height * scale,
                  }}
                  className="relative shrink-0 overflow-hidden"
                >
                  <div
                    data-testid="web-preview-device-frame"
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
                      className="overflow-hidden rounded-[1.35rem] bg-white"
                      style={{
                        width: frame.width,
                        height: frame.height,
                      }}
                    >
                      <iframe
                        key={`iframe-${activeId || 'x'}-${reloadKey}`}
                        ref={onIframeMount}
                        data-testid="web-preview-iframe"
                        data-canvas-id={activeId || ''}
                        data-reload={reloadKey}
                        title={t('webPreview.iframeTitle')}
                        sandbox={sandbox}
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
            ) : (
              !showCode && (
                <div className="flex min-w-0 flex-1 items-center justify-center text-[11px] text-shell-muted">
                  Enable Code or Preview
                </div>
              )
            )}
          </div>

          <div
            data-testid="web-preview-row-agent"
            className="shrink-0 border-t border-shell-border"
          >
            <CanvasAgentPanel
              contextId={workspaceId}
              canvasKey={
                activeId
                  ? `preview:${scopeId}:${activeId}`
                  : `preview:${scopeId}:active`
              }
              currentHtml={htmlDraft || htmlLive}
              onApplyHtml={applyHtmlFromAgent}
              className="w-full border-0"
            />
          </div>
        </div>
      </div>

      {ctxMenu &&
        typeof document !== 'undefined' &&
        createPortal(
          <CanvasHistoryMenu
            x={ctxMenu.x}
            y={ctxMenu.y}
            onClose={() => setCtxMenu(null)}
            onAction={(action) => runCtxAction(action, ctxMenu.canvasId)}
          />,
          document.body,
        )}

      {deleteTarget && (
        <ConfirmDialog
          title={t('webPreview.deleteCanvasConfirm')}
          message={t('webPreview.deleteCanvasMessage', {
            name: deleteTarget.title || 'Playground',
          })}
          confirmLabel={t('webPreview.deleteCanvas')}
          danger
          onConfirm={() => {
            const id = deleteTarget.id
            setDeleteTarget(null)
            deleteCanvas(id)
          }}
          onCancel={() => setDeleteTarget(null)}
        />
      )}
    </div>
  )
}

function CanvasHistoryMenu({
  x,
  y,
  onClose,
  onAction,
}: {
  x: number
  y: number
  onClose: () => void
  onAction: (id: string) => void
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
  const item =
    'flex w-full items-center px-2.5 py-1.5 text-left text-[11px] text-shell-text hover:bg-shell-border/40'

  return (
    <div
      ref={ref}
      data-testid="canvas-history-context-menu"
      role="menu"
      className="fixed z-[220] min-w-[10rem] rounded-md border border-shell-border bg-shell-panel py-0.5 shadow-lg shadow-black/40"
      style={{ left, top }}
    >
      <button
        type="button"
        role="menuitem"
        className={item}
        onClick={() => onAction('open')}
      >
        Open
      </button>
      <button
        type="button"
        role="menuitem"
        data-testid="canvas-ctx-rename"
        className={item}
        onClick={() => onAction('rename')}
      >
        Rename
      </button>
      <button
        type="button"
        role="menuitem"
        className={item}
        onClick={() => onAction('download')}
      >
        Download HTML
      </button>
      <div role="separator" className="my-0.5 border-t border-shell-border" />
      <button
        type="button"
        role="menuitem"
        className={`${item} text-red-300 hover:text-red-200`}
        onClick={() => onAction('delete')}
      >
        Delete
      </button>
    </div>
  )
}
