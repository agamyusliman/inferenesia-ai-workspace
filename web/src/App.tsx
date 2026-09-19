import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ArrowLeft, GitBranch, MessageSquareCode } from 'lucide-react'
import {
  createFile,
  getFileTimelineChange,
  getHubState,
  createSessionWorkspace,
  isSessionWorkspace,
  openWorkspace,
  renameWorkspace,
  readFile,
  saveFile,
  selectFile,
  streamFileEvents,
  switchWorkspace,
  type FileTimelineEntry,
  type HubState,
} from './lib/api'
import { CommandPalette } from './features/command-palette/CommandPalette'
import {
  resolveShortcut,
  type CommandId,
} from './features/command-palette/commands'
import { CloseDirtyDialog } from './features/editor/CloseDirtyDialog'
import { defaultViewMode } from './features/editor/editorMode'
import {
  ExplorerTree,
  type OpenFileOptions,
} from './features/explorer/ExplorerTree'
import { MainPane } from './features/shell/MainPane'
import { Sidebar, type NavId } from './features/shell/Sidebar'
import { StatusBar } from './features/shell/StatusBar'
import {
  TerminalPanel,
  type OpenTerminalRequest,
} from './features/shell/TerminalPanel'
import { HorizontalSplitter } from './features/shell/HorizontalSplitter'
import { VerticalSplitter } from './features/shell/VerticalSplitter'
import { useLayoutConfig } from './features/shell/useLayoutConfig'
import type { EditorColumn } from './features/shell/SplitEditorArea'
import { GitPanel } from './features/git/GitPanel'
import {
  FileTimeline,
  type TimelineDiffRequest,
} from './features/history/FileTimeline'

import { DocsPanel } from './features/docs/DocsPanel'
import { getDocBySlug, parseDocDeepLink } from './features/docs/catalog'
import { CanvasGalleryPanel } from './features/canvas-gallery/CanvasGalleryPanel'
import {
  upsertGalleryDiagramFromOpen,
  upsertGalleryHtmlFromOpen,
  upsertGalleryImageFromOpen,
} from './features/canvas-gallery/galleryStore'
import type { WebPreviewSeed } from './features/web-preview/webPreview'
import {
  initialChatBindFromSession,
  initialNavFromSession,
  readUiSession,
  writeUiSession,
} from './features/shell/uiSession'
import { SettingsPanel } from './features/settings/SettingsPanel'
import { useTheme } from './features/theme/ThemeProvider'
import { SessionList } from './features/workspaces/SessionList'
import {
  deriveSessionTitle,
  isPlaceholderSessionName,
} from './features/workspaces/sessionTitle'
import { WorkspaceList } from './features/workspaces/WorkspaceList'

function tabId(path: string) {
  return `tab:${path}`
}

function colId(path: string, side: 'primary' | 'secondary') {
  return `${side}:${path}`
}

function baseName(path: string) {
  const parts = path.split(/[/\\]/)
  return parts[parts.length - 1] || path
}

function isTabDirty(tab: EditorColumn): boolean {
  if (tab.kind === 'diff') return false
  if (tab.lastSavedContent === undefined) return false
  return tab.content !== tab.lastSavedContent
}

function makeDiffTab(req: TimelineDiffRequest): EditorColumn {
  const entry = req.entry
  const ch = req.change
  const id = `diff:${entry.id}:${req.path}`
  const label =
    entry.short_sha ||
    (entry.unit_id ? entry.unit_id.slice(0, 8) : entry.id.slice(0, 12))
  const displayPath = `${req.path} @ ${label}`
  return {
    id,
    path: displayPath,
    content: ch.modified || '',
    lastSavedContent: ch.modified || '',
    kind: 'diff',
    diff: {
      original: ch.original || '',
      modified: ch.modified || '',
      labelOriginal: ch.label_original || 'before',
      labelModified: ch.label_modified || 'after',
      title: ch.title || entry.summary,
      entryId: entry.id,
      source: entry.source,
      path: req.path,
    },
  }
}

function makeTab(
  path: string,
  content: string,
  preferredMode?: EditorColumn['preferredMode'],
): EditorColumn {
  return {
    id: tabId(path),
    path,
    content,
    lastSavedContent: content,
    preferredMode,
  }
}

export function App() {
  const { theme } = useTheme()
  const [nav, setNav] = useState<NavId>(() => initialNavFromSession())
  const [chatBind, setChatBind] = useState<'hub' | 'none'>(() =>
    initialChatBindFromSession(),
  )
  const [galleryFocusId, setGalleryFocusId] = useState<string | null>(null)
  const [playgroundStatusLabel, setPlaygroundStatusLabel] = useState<
    string | undefined
  >(undefined)
  const [htmlSyncNonce, setHtmlSyncNonce] = useState(0)
  const [docsInitialSlug, setDocsInitialSlug] = useState<string | undefined>(
    () => {
      if (typeof window === 'undefined') return undefined
      const slug = parseDocDeepLink(window.location.search)
      return slug && getDocBySlug(slug) ? slug : undefined
    },
  )
  const [hub, setHub] = useState<HubState | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [refreshKey, setRefreshKey] = useState(0)
  const [undoRefreshKey, setUndoRefreshKey] = useState(0)
  /** Footer git strip + Git panel deep-link (branches/graph). */
  const [gitFocusTab, setGitFocusTab] = useState<
    'diff' | 'graph' | 'branches' | 'workflow' | null
  >(null)
  const layout = useLayoutConfig()

  /** Multi-tab open editors (primary column). */
  const [tabs, setTabs] = useState<EditorColumn[]>([])
  const [activeTabId, setActiveTabId] = useState<string | null>(null)
  const [secondary, setSecondary] = useState<EditorColumn | null>(null)
  /** Pending close of a dirty tab (VAL-IDE-019). */
  const [pendingCloseId, setPendingCloseId] = useState<string | null>(null)
  /** Command palette open state (VAL-IDE-022). */
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [paletteMode, setPaletteMode] = useState<'commands' | 'files'>(
    'commands',
  )
  /** Integrated terminal panel toggle (palette Toggle Terminal). */
  const [terminalOpen, setTerminalOpen] = useState(() => {
    return readUiSession()?.terminalOpen === true
  })
  /** Explorer Open in Integrated Terminal → new multi-tab session. */
  const [terminalOpenRequest, setTerminalOpenRequest] =
    useState<OpenTerminalRequest | null>(null)
  /** Plan Mode active — status bar + hub indicator (VAL-PLAN-001). */
  const [planModeActive, setPlanModeActive] = useState(false)
  const uiRestoredRef = useRef(false)
  /** Bump todo panel on PlanProposed / TodoUpdated / approve (VAL-PLAN-003/005). */
  const [todoRefreshKey, setTodoRefreshKey] = useState(0)
  /** Bump mission panel on MissionStatus events (VAL-MISSION-006). */
  const [missionRefreshKey, setMissionRefreshKey] = useState(0)
  /** Bump task panel on TaskSpawned/TaskDone events (VAL-ORCH-010). */
  const [taskRefreshKey, setTaskRefreshKey] = useState(0)
  /** Bump browser panel on BrowserStatus events (VAL-BRW-008). */
  const [browserRefreshKey, setBrowserRefreshKey] = useState(0)
  /** Hide explorer panel (palette Toggle Explorer). */
  const [explorerVisible, setExplorerVisible] = useState(() => {
    const s = readUiSession()
    return s?.explorerVisible !== false
  })
  const [chatVisible, setChatVisible] = useState(() => {
    const s = readUiSession()
    return s?.chatVisible !== false
  })
  /** Which editor column last received focus (for Cmd+S target). */
  const [focusedColumn, setFocusedColumn] = useState<'primary' | 'secondary'>(
    'primary',
  )

  const primary = useMemo(
    () => tabs.find((t) => t.id === activeTabId) || tabs[0] || null,
    [tabs, activeTabId],
  )

  const pendingCloseTab = useMemo(
    () => (pendingCloseId ? tabs.find((t) => t.id === pendingCloseId) || null : null),
    [pendingCloseId, tabs],
  )

  const refresh = useCallback(async () => {
    try {
      const state = await getHubState()
      setHub(state)
      setPlanModeActive(!!state.plan_mode_active)
      setError(null)
      if (typeof document !== 'undefined' && state.product) {
        document.title = state.product
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  useEffect(() => {
    if (!hub || uiRestoredRef.current) return
    uiRestoredRef.current = true
    let cancelled = false
    const snap = readUiSession()
    void (async () => {
      try {
        let state = hub
        if (
          snap?.workspaceId &&
          snap.workspaceId !== hub.active_id &&
          (hub.workspaces || []).some((w) => w.id === snap.workspaceId)
        ) {
          const res = await switchWorkspace(snap.workspaceId)
          state = res.hub || (await getHubState())
          if (cancelled) return
          setHub(state)
          setPlanModeActive(!!state.plan_mode_active)
          setRefreshKey((k) => k + 1)
        }
        if (cancelled) return

        const restoredNav =
          snap?.nav && snap.nav !== 'preview'
            ? snap.nav
            : isSessionWorkspace(state.active)
              ? 'sessions'
              : 'files'
        setNav(restoredNav)
        setChatBind(
          snap?.chatBind === 'none' && restoredNav === 'sessions'
            ? 'none'
            : 'hub',
        )
        if (typeof snap?.chatVisible === 'boolean') {
          setChatVisible(snap.chatVisible)
        } else {
          setChatVisible(true)
        }
        if (typeof snap?.explorerVisible === 'boolean') {
          setExplorerVisible(snap.explorerVisible)
        }
        if (typeof snap?.terminalOpen === 'boolean') {
          setTerminalOpen(snap.terminalOpen)
        }

        if (
          !isSessionWorkspace(state.active) &&
          snap?.openPaths &&
          snap.openPaths.length > 0
        ) {
          const restored: EditorColumn[] = []
          for (const p of snap.openPaths.slice(0, 12)) {
            try {
              const content = await readFile(state.active_id, p)
              restored.push(makeTab(p, content))
            } catch {
              void 0
            }
          }
          if (cancelled || restored.length === 0) return
          setTabs(restored)
          const want =
            snap.activePath && restored.some((t) => t.path === snap.activePath)
              ? snap.activePath
              : restored[0].path
          setActiveTabId(tabId(want!))
          setNav(
            restoredNav === 'sessions' || restoredNav === 'workspaces'
              ? 'files'
              : restoredNav,
          )
        }
      } catch {
        void 0
      }
    })()
    return () => {
      cancelled = true
    }
  }, [hub])

  useEffect(() => {
    if (!uiRestoredRef.current) return
    const openPaths = tabs
      .filter((t) => t.kind !== 'diff')
      .map((t) => t.path)
    const active = tabs.find((t) => t.id === activeTabId)
    writeUiSession({
      nav,
      chatBind,
      chatVisible,
      explorerVisible,
      terminalOpen,
      workspaceId: hub?.active_id,
      openPaths,
      activePath: active?.kind === 'diff' ? null : active?.path ?? null,
    })
  }, [
    nav,
    chatBind,
    chatVisible,
    explorerVisible,
    terminalOpen,
    hub?.active_id,
    tabs,
    activeTabId,
  ])

  const loadContent = useCallback(
    async (path: string, workspaceId?: string): Promise<string | null> => {
      try {
        return await readFile(workspaceId || hub?.active_id || '', path)
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e))
        return null
      }
    },
    [hub?.active_id],
  )

  const clearEditors = () => {
    setTabs([])
    setActiveTabId(null)
    setSecondary(null)
    setPendingCloseId(null)
  }

  const applyBoundWorkspace = useCallback((state: HubState) => {
    setHub(state)
    setPlanModeActive(!!state.plan_mode_active)
    clearEditors()
    setTerminalOpen(false)
    setGitFocusTab(null)
    setTimelineNav(null)
    setRefreshKey((k) => k + 1)
    setUndoRefreshKey((k) => k + 1)
    setTodoRefreshKey((k) => k + 1)
    setMissionRefreshKey((k) => k + 1)
    setTaskRefreshKey((k) => k + 1)
    setBrowserRefreshKey((k) => k + 1)
    setChatBind('hub')
    if (isSessionWorkspace(state.active)) {
      setNav('sessions')
    } else {
      setNav('files')
      setExplorerVisible(true)
    }
  }, [])

  const onOpenPath = async (path: string) => {
    setBusy(true)
    try {
      const res = await openWorkspace(path)
      const state = res.hub || (await getHubState())
      applyBoundWorkspace(state)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const onSwitch = async (id: string, hubFromCaller?: HubState) => {
    if (hubFromCaller && hubFromCaller.active_id === id) {
      setChatBind('hub')
      setChatVisible(true)
      applyBoundWorkspace(hubFromCaller)
      return
    }
    if (hub?.active_id === id) {
      setChatBind('hub')
      setChatVisible(true)
      if (isSessionWorkspace(hub.active)) setNav('sessions')
      else {
        setNav('files')
        setExplorerVisible(true)
      }
      return
    }
    setBusy(true)
    try {
      const res = await switchWorkspace(id)
      const state = res.hub || (await getHubState())
      setChatBind('hub')
      setChatVisible(true)
      applyBoundWorkspace(state)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  /**
   * Open a file: activate existing tab, or add a new tab (does not replace).
   * VAL-IDE-018: second open adds a tab. preferredMode covers Open With / Open Preview.
   */
  const onSelectFile = async (path: string, options?: OpenFileOptions) => {
    if (options?.newTab) {
      await onOpenInNewTab(path, options)
      return
    }
    // Single-click now also adds/activates tabs (multi-tab by default for VAL-IDE-018).
    await onOpenInNewTab(path, options)
  }

  /**
   * Open as an additional tab (or focus existing tab for that path).
   * Does not replace other open tabs; preserves unsaved buffers (VAL-IDE-018).
   */
  const onOpenInNewTab = useCallback(
    async (path: string, options?: OpenFileOptions) => {
      try {
        const state = await selectFile(path)
        setHub(state)
        const preferredMode = options?.mode
        const id = tabId(path)

        // Functional update: if already open, activate without clobbering dirty buffer.
        let alreadyOpen = false
        setTabs((prev) => {
          const existing = prev.find((t) => t.path === path)
          if (existing) {
            alreadyOpen = true
            if (preferredMode && preferredMode !== existing.preferredMode) {
              return prev.map((t) =>
                t.path === path
                  ? { ...t, preferredMode: preferredMode ?? t.preferredMode }
                  : t,
              )
            }
            return prev
          }
          return prev
        })
        if (alreadyOpen) {
          setActiveTabId(id)
          return
        }

        const content = await loadContent(path, state.active_id)
        if (content === null) return
        const tab = makeTab(
          path,
          content,
          preferredMode ?? defaultViewMode(path),
        )
        setTabs((prev) => {
          if (prev.some((t) => t.path === path)) {
            return prev.map((t) =>
              t.path === path
                ? {
                    ...t,
                    preferredMode: preferredMode ?? t.preferredMode,
                  }
                : t,
            )
          }
          return [...prev, tab]
        })
        setActiveTabId(tab.id)
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e))
      }
    },
    [loadContent],
  )

  const onOpenToSide = async (path: string) => {
    try {
      if (!primary) {
        await onOpenInNewTab(path)
        return
      }
      const content = await loadContent(path, hub?.active_id)
      if (content === null) return
      setSecondary({
        id: colId(path, 'secondary'),
        path,
        content,
        lastSavedContent: content,
        preferredMode: defaultViewMode(path),
      })
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }

  const onActivateTab = (id: string) => {
    setActiveTabId(id)
    const tab = tabs.find((t) => t.id === id)
    if (tab) {
      void selectFile(tab.path).then(setHub).catch(() => {})
    }
  }

  /** Actually remove a tab (after dirty confirm if needed). */
  const forceCloseTab = useCallback((id: string) => {
    setTabs((prev) => {
      const idx = prev.findIndex((t) => t.id === id)
      if (idx < 0) return prev
      const next = prev.filter((t) => t.id !== id)
      setActiveTabId((cur) => {
        if (cur !== id) return cur
        if (next.length === 0) return null
        return next[Math.min(idx, next.length - 1)].id
      })
      if (next.length === 0) {
        setSecondary(null)
      }
      return next
    })
    setPendingCloseId((cur) => (cur === id ? null : cur))
  }, [])

  const onCloseTab = (id: string) => {
    const tab = tabs.find((t) => t.id === id)
    if (!tab) return
    if (isTabDirty(tab)) {
      setPendingCloseId(id)
      return
    }
    forceCloseTab(id)
  }

  const onCloseSecondary = () => {
    setSecondary(null)
  }

  const onPrimaryChange = (content: string) => {
    const id = activeTabId || primary?.id
    if (!id) return
    setTabs((prev) => prev.map((t) => (t.id === id ? { ...t, content } : t)))
  }

  const onPrimaryViewState = (state: unknown) => {
    const id = activeTabId || primary?.id
    if (!id) return
    setTabs((prev) => prev.map((t) => (t.id === id ? { ...t, viewState: state } : t)))
  }

  const onSecondaryViewState = (state: unknown) => {
    setSecondary((p) => (p ? { ...p, viewState: state } : p))
  }

  /** Persist buffer to disk via core.Service user-edit save (VAL-IDE-017). */
  const saveTab = useCallback(
    async (tab: EditorColumn): Promise<boolean> => {
      const ws = hub?.active_id || ''
      try {
        await saveFile(ws, tab.path, tab.content)
        setTabs((prev) =>
          prev.map((t) =>
            t.id === tab.id
              ? { ...t, lastSavedContent: t.content }
              : t.path === tab.path
                ? { ...t, lastSavedContent: t.content }
                : t,
          ),
        )
        setSecondary((s) =>
          s && s.path === tab.path
            ? { ...s, content: tab.content, lastSavedContent: tab.content }
            : s,
        )
        setUndoRefreshKey((k) => k + 1)
        void refresh()
        return true
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e))
        return false
      }
    },
    [hub?.active_id, refresh],
  )

  /** Also persist secondary split dirty buffer when it differs from primary. */
  const saveSecondary = useCallback(async (): Promise<boolean> => {
    if (!secondary) return true
    const ws = hub?.active_id || ''
    // Skip if primary tab for same path already saved latest content.
    const primarySame = tabs.find((t) => t.path === secondary.path)
    if (
      primarySame &&
      primarySame.content === secondary.content &&
      primarySame.lastSavedContent === secondary.content
    ) {
      return true
    }
    try {
      await saveFile(ws, secondary.path, secondary.content)
      setSecondary((s) =>
        s ? { ...s, lastSavedContent: s.content } : s,
      )
      setTabs((prev) =>
        prev.map((t) =>
          t.path === secondary.path
            ? {
                ...t,
                content: secondary.content,
                lastSavedContent: secondary.content,
              }
            : t,
        ),
      )
      setUndoRefreshKey((k) => k + 1)
      void refresh()
      return true
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      return false
    }
  }, [secondary, hub?.active_id, tabs, refresh])

  const onSaveActive = useCallback(async () => {
    // Save the focused dirty column first (VAL-IDE-021); fall back to primary.
    if (
      focusedColumn === 'secondary' &&
      secondary &&
      secondary.lastSavedContent !== undefined &&
      secondary.content !== secondary.lastSavedContent
    ) {
      await saveSecondary()
      return
    }
    if (primary) {
      await saveTab(primary)
      return
    }
    if (secondary) {
      await saveSecondary()
    }
  }, [focusedColumn, primary, secondary, saveTab, saveSecondary])

  const onSaveAll = useCallback(async () => {
    const dirty = tabs.filter(isTabDirty)
    for (const t of dirty) {
      const ok = await saveTab(t)
      if (!ok) return
    }
    if (
      secondary &&
      secondary.lastSavedContent !== undefined &&
      secondary.content !== secondary.lastSavedContent
    ) {
      await saveSecondary()
    }
  }, [tabs, secondary, saveTab, saveSecondary])

  /** Open integrated terminal at folder (new session tab; multi-tab). */
  const onOpenInTerminal = useCallback((folderRelPath: string) => {
    setTerminalOpen(true)
    setTerminalOpenRequest({
      cwd: folderRelPath || '',
      workspaceId: hub?.active_id,
      title: folderRelPath ? folderRelPath.split(/[/\\]/).pop() : undefined,
      nonce: Date.now(),
    })
  }, [hub?.active_id])

  /** Create untitled file at workspace root (palette New File). */
  const onNewFileFromPalette = useCallback(async () => {
    const ws = hub?.active_id || ''
    if (!ws) {
      setError('Open a workspace before creating a file')
      return
    }
    const stamp = Date.now().toString(36)
    const name = `untitled-${stamp}.txt`
    try {
      const res = await createFile(ws, '', name)
      const path = res.path || name
      setRefreshKey((k) => k + 1)
      setUndoRefreshKey((k) => k + 1)
      await onOpenInNewTab(path)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }, [hub?.active_id, onOpenInNewTab])

  const runCommand = useCallback(
    (id: string) => {
      const sessionMode = isSessionWorkspace(hub?.active)
      const cmd = id as CommandId
      switch (cmd) {
        case 'save':
          void onSaveActive()
          break
        case 'save-all':
          void onSaveAll()
          break
        case 'close-tab': {
          const idToClose = activeTabId || primary?.id
          if (idToClose) onCloseTab(idToClose)
          break
        }
        case 'new-file':
          void onNewFileFromPalette()
          break
        case 'open-file':
          setNav(sessionMode ? 'sessions' : 'files')
          setExplorerVisible(true)
          break
        case 'toggle-terminal':
          setTerminalOpen((v) => !v)
          break
        case 'toggle-explorer':
          setExplorerVisible((v) => !v)
          break
        case 'focus-explorer':
          setNav(sessionMode ? 'sessions' : 'files')
          setExplorerVisible(true)
          break
        case 'close-secondary':
          setSecondary(null)
          break
        case 'open-docs':
          setTerminalOpen(false)
          setNav('docs')
          break
        default:
          break
      }
    },
    [
      hub?.active,
      onSaveActive,
      onSaveAll,
      activeTabId,
      primary?.id,
      onCloseTab,
      onNewFileFromPalette,
    ],
  )

  useEffect(() => {
    if (typeof window === 'undefined') return
    const slug = parseDocDeepLink(window.location.search)
    if (!slug || !getDocBySlug(slug)) return
    setDocsInitialSlug(slug)
    setNav('docs')
  }, [])

  // Global shortcuts: mod+S save, mod+W close tab, mod+Shift+P palette
  // (VAL-IDE-017/021/022). Undo/redo left to Monaco (VAL-IDE-020).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      // While dirty dialog is open, do not steal keys (dialog owns Esc / buttons).
      if (pendingCloseId) return
      // When palette is open, only Esc is handled by the palette component.
      if (paletteOpen) return

      const action = resolveShortcut(e)
      if (!action) return

      // Do not hijack when typing in inputs outside Monaco/contenteditable editors
      // except for palette open (always global).
      const t = e.target as HTMLElement | null
      const tag = t?.tagName?.toLowerCase()
      const isFormField =
        tag === 'input' ||
        tag === 'textarea' ||
        tag === 'select' ||
        (t?.isContentEditable ?? false)
      // Monaco uses textarea; allow save/close/palette there.
      const inMonaco =
        !!t?.closest?.('.monaco-editor') ||
        t?.classList?.contains('inputarea')

      if (action === 'command-palette' || action === 'quick-open') {
        e.preventDefault()
        e.stopPropagation()
        setPaletteMode(action === 'quick-open' ? 'files' : 'commands')
        setPaletteOpen(true)
        return
      }

      if (isFormField && !inMonaco) {
        // Still allow save/close from plain inputs if desired? Prefer not —
        // only Monaco + body get save/close to avoid fighting chat input.
        return
      }

      if (action === 'save') {
        e.preventDefault()
        void onSaveActive()
        return
      }
      if (action === 'save-all') {
        e.preventDefault()
        void onSaveAll()
        return
      }
      if (action === 'close-tab') {
        e.preventDefault()
        const idToClose = activeTabId || primary?.id
        if (idToClose) onCloseTab(idToClose)
      }
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [
    pendingCloseId,
    paletteOpen,
    onSaveActive,
    onSaveAll,
    activeTabId,
    primary?.id,
    onCloseTab,
  ])

  const reloadEditorContent = useCallback(
    async (path?: string) => {
      const target = path || hub?.selected_file
      if (!target) return
      const ws = hub?.active_id || ''
      try {
        const content = await readFile(ws, target)
        setTabs((prev) =>
          prev.map((t) => {
            if (t.path !== target) return t
            // Do not clobber a dirty local buffer on external change silently —
            // only update when clean, or always refresh lastSaved for dirty dialog accuracy.
            if (isTabDirty(t)) {
              return { ...t, lastSavedContent: content }
            }
            return { ...t, content, lastSavedContent: content }
          }),
        )
        setSecondary((s) => {
          if (!s || s.path !== target) return s
          if (
            s.lastSavedContent !== undefined &&
            s.content !== s.lastSavedContent
          ) {
            return { ...s, lastSavedContent: content }
          }
          return { ...s, content, lastSavedContent: content }
        })
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e))
      }
    },
    [hub?.active_id, hub?.selected_file],
  )

  const onFileChanged = async (path?: string) => {
    setUndoRefreshKey((k) => k + 1)
    setRefreshKey((k) => k + 1)
    await reloadEditorContent(path)
  }

  /**
   * Subscribe to external file change events (CLI agent / OS / editor writes)
   * via SSE so the desktop explorer refreshes via push, not polling
   * (VAL-CROSS-003). Reconnects when the active workspace changes. The
   * cleanup function closes the stream on unmount / workspace switch.
   *
   * Desktop WriteGateway mutations are suppressed server-side so this stream
   * only fires for external writes (CLI agent, OS, other editors), avoiding
   * double-refresh with the chat-panel FileChanged path.
   */
  const onFileChangedRef = useRef(onFileChanged)
  onFileChangedRef.current = onFileChanged
  useEffect(() => {
    if (!hub?.active_id) return
    const cleanup = streamFileEvents((ev) => {
      // Bump explorer refresh key (reloads tree from disk via ListDir) and
      // reload any open editor buffer for the mutated path.
      void onFileChangedRef.current(ev.removed ? undefined : ev.path)
    })
    return cleanup
  }, [hub?.active_id])

  const onDirtyDialogSave = async () => {
    if (!pendingCloseTab) return
    const ok = await saveTab(pendingCloseTab)
    if (ok) {
      forceCloseTab(pendingCloseTab.id)
    }
  }

  const onDirtyDialogDontSave = () => {
    if (!pendingCloseTab) return
    forceCloseTab(pendingCloseTab.id)
  }

  const onDirtyDialogCancel = () => {
    setPendingCloseId(null)
  }

  const product = hub?.product || 'Inferenesia'
  const active = hub?.active
  const activeIsSession = isSessionWorkspace(active)
  const hasFolderWorkspace = !!active && !activeIsSession
  const showTerminal = terminalOpen
  const effectiveNav =
    nav === 'todos' || nav === 'missions' || nav === 'tasks' || nav === 'browser'
      ? 'sessions'
      : nav === 'chat'
        ? activeIsSession
          ? 'sessions'
          : 'files'
        : nav === 'files' || nav === 'git'
          ? activeIsSession
            ? 'sessions'
            : nav
          : nav
  const showSettings = !showTerminal && effectiveNav === 'settings'
  const showDocs = !showTerminal && effectiveNav === 'docs'
  const showCanvasGallery = !showTerminal && effectiveNav === 'canvas'
  const showSessionsNav = !showTerminal && effectiveNav === 'sessions'
  // Git/workspace SCM chrome only while a folder workspace is active AND the
  // user is on a workspace surface (files / git / history / workspaces rail).
  // Sessions, Playground, Docs, Settings: fully hide git (no leftover status).
  const showFolderGitStatus =
    hasFolderWorkspace &&
    !showTerminal &&
    !showSettings &&
    !showDocs &&
    !showCanvasGallery &&
    !showSessionsNav &&
    (effectiveNav === 'files' ||
      effectiveNav === 'git' ||
      effectiveNav === 'history' ||
      effectiveNav === 'workspaces')
  const showGit =
    !showTerminal &&
    effectiveNav === 'git' &&
    hasFolderWorkspace
  const showLeftPanel =
    !showTerminal &&
    !showSettings &&
    !showDocs &&
    !showCanvasGallery &&
    explorerVisible &&
    (effectiveNav === 'workspaces' ||
      effectiveNav === 'sessions' ||
      (hasFolderWorkspace &&
        (effectiveNav === 'files' ||
          effectiveNav === 'git' ||
          effectiveNav === 'history')))

  const needsFolderWorkspace =
    effectiveNav === 'workspaces' && !hasFolderWorkspace
  const chatWorkspaceId =
    needsFolderWorkspace || chatBind === 'none' ? undefined : hub?.active_id
  const needsSession =
    effectiveNav === 'sessions' &&
    (chatBind === 'none' || !chatWorkspaceId || !activeIsSession)
  const chatContextLabel = (() => {
    if (needsFolderWorkspace) return 'Open workspace'
    if (needsSession) return 'New chat'
    if (chatBind === 'none' || !chatWorkspaceId) {
      return hub?.chat_context || undefined
    }
    if (activeIsSession && active) {
      return `Session · ${active.name}`
    }
    if (active && !activeIsSession) {
      return `Workspace · ${active.name}`
    }
    return hub?.chat_context
  })()

  const ensureSessionForChat = useCallback(
    async (_firstUserMessage?: string): Promise<string | undefined> => {
      const res = await createSessionWorkspace()
      const id = res.workspace?.id
      if (!id) return undefined
      if (res.hub) {
        applyBoundWorkspace(res.hub)
      } else {
        await onSwitch(id)
      }
      setChatBind('hub')
      setNav('sessions')
      setRefreshKey((k) => k + 1)
      return id
    },
    [applyBoundWorkspace, onSwitch],
  )

  const createNewSessionForHandoff = useCallback(
    async (): Promise<string | undefined> => {
      const res = await createSessionWorkspace('Handoff')
      const id = res.workspace?.id
      if (!id) return undefined
      if (res.hub) {
        applyBoundWorkspace(res.hub)
      } else {
        await onSwitch(id)
      }
      setChatBind('hub')
      setNav('sessions')
      setRefreshKey((k) => k + 1)
      return id
    },
    [applyBoundWorkspace, onSwitch],
  )

  const maybeTitleSession = useCallback(
    (workspaceId: string, sourceText: string) => {
      const id = workspaceId.trim()
      if (!id) return
      const title = deriveSessionTitle(sourceText)
      if (!title) return
      const current = hub?.workspaces?.find((w) => w.id === id)
      if (current && !isPlaceholderSessionName(current.name)) return
      void renameWorkspace(id, title)
        .then(() => {
          setRefreshKey((k) => k + 1)
          void refresh()
        })
        .catch(() => undefined)
    },
    [hub?.workspaces, refresh],
  )

  const onDraftNewSession = useCallback(() => {
    setTerminalOpen(false)
    setNav('sessions')
    setChatBind('none')
    setChatVisible(true)
    setRefreshKey((k) => k + 1)
  }, [])

  const onPrimaryNav = useCallback(
    (id: NavId) => {
      setTerminalOpen(false)
      if (id === 'sessions') {
        setNav('sessions')
        setChatBind('none')
        setChatVisible(true)
        return
      }
      if (id === 'workspaces') {
        setNav('workspaces')
        setChatBind('hub')
        setChatVisible(true)
        return
      }
      if (id === 'canvas') {
        setNav('canvas')
        setChatVisible(false)
        return
      }
      setChatBind('hub')
      setNav(id)
    },
    [hub, onSwitch],
  )

  const openCanvasGalleryFromChat = useCallback((seed: WebPreviewSeed) => {
    let itemId: string | null = null
    if (seed.mode === 'diagram' && seed.diagramSource) {
      const item = upsertGalleryDiagramFromOpen({
        source: seed.diagramSource,
        title: seed.title,
        scopeId: seed.scopeId,
        canvasId: seed.canvasId,
        chatMessageId: seed.chatMessageId,
      })
      itemId = item.id
    } else if (seed.mode === 'image' && seed.imageUrl) {
      const item = upsertGalleryImageFromOpen({
        imageUrl: seed.imageUrl,
        title: seed.title,
        prompt: seed.imagePrompt,
        caption: seed.imageCaption,
        scopeId: seed.scopeId,
        canvasId: seed.canvasId,
        chatMessageId: seed.chatMessageId,
      })
      itemId = item?.id || null
    } else if (seed.html) {
      const item = upsertGalleryHtmlFromOpen({
        html: seed.html,
        title: seed.title,
        path: seed.path,
        scopeId: seed.scopeId,
        canvasId: seed.canvasId,
        chatMessageId: seed.chatMessageId,
      })
      itemId = item.id
    }
    if (!itemId) return
    setGalleryFocusId(itemId)
    setNav('canvas')
    setChatVisible(false)
    setTerminalOpen(false)
    setHtmlSyncNonce((n) => n + 1)
  }, [])

  const timelinePath =
    (primary?.kind === 'diff' ? primary.diff?.path : primary?.path) ||
    hub?.selected_file ||
    ''
  const selectedPath = timelinePath
  const timelineActiveEntryId =
    primary?.kind === 'diff' ? primary.diff?.entryId || null : null
  const [timelineNav, setTimelineNav] = useState<{
    path: string
    entries: FileTimelineEntry[]
    index: number
  } | null>(null)

  const leftPanelContent =
    effectiveNav === 'workspaces' ? (
      <WorkspaceList
        workspaces={hub?.workspaces || []}
        activeId={hub?.active_id}
        onSwitch={onSwitch}
        onOpenPath={onOpenPath}
        onRemoved={() => {
          void refresh()
          setRefreshKey((k) => k + 1)
        }}
        busy={busy}
      />
    ) : effectiveNav === 'sessions' ? (
      <SessionList
        workspaces={hub?.workspaces || []}
        activeId={chatBind === 'none' ? undefined : hub?.active_id}
        onSwitch={onSwitch}
        onDraftNew={onDraftNewSession}
        onRemoved={() => {
          setChatBind('none')
          void refresh().then(() => {
            setRefreshKey((k) => k + 1)
            setTodoRefreshKey((k) => k + 1)
            setMissionRefreshKey((k) => k + 1)
            setTaskRefreshKey((k) => k + 1)
            setBrowserRefreshKey((k) => k + 1)
          })
        }}
        busy={busy}
      />
    ) : activeIsSession ? null : (
      <div className="flex h-full min-h-0 flex-col">
        <div className="flex h-10 shrink-0 items-center justify-between gap-1 border-b border-shell-border px-1">
          <button
            type="button"
            data-testid="back-to-workspaces"
            onClick={() => setNav('workspaces')}
            className="flex shrink-0 items-center gap-1 rounded px-1.5 py-1 text-left text-[11px] font-medium text-shell-accent transition hover:bg-shell-accent/15 hover:text-shell-accent"
            title="Back to workspace list"
          >
            <ArrowLeft size={14} className="shrink-0" aria-hidden />
            <span>Back</span>
          </button>
          <div className="flex shrink-0 items-center gap-0.5">
            <button
              type="button"
              data-testid="workspace-chat-tab"
              onClick={() => {
                if (showGit) {
                  setNav('files')
                  setChatVisible(true)
                  return
                }
                setChatVisible((v) => !v)
              }}
              className={`flex shrink-0 items-center gap-1 rounded px-1.5 py-1 text-[11px] font-medium transition ${
                !showGit && chatVisible
                  ? 'bg-shell-active text-shell-text'
                  : 'text-shell-muted hover:bg-shell-border/40 hover:text-shell-text'
              }`}
              title={
                showGit
                  ? 'Back to editor + chat'
                  : chatVisible
                    ? 'Hide chat panel'
                    : 'Show chat panel'
              }
              aria-pressed={!showGit && chatVisible}
            >
              <MessageSquareCode size={14} className="shrink-0" aria-hidden />
              <span className="hidden sm:inline">Chat</span>
            </button>
            <button
              type="button"
              data-testid="workspace-git-tab"
              onClick={() => setNav('git')}
              className={`flex shrink-0 items-center gap-1 rounded px-1.5 py-1 text-[11px] font-medium transition ${
                showGit
                  ? 'bg-shell-active text-shell-text'
                  : 'text-shell-muted hover:bg-shell-border/40 hover:text-shell-text'
              }`}
              title="Git for active workspace"
            >
              <GitBranch size={14} className="shrink-0" aria-hidden />
              <span className="hidden sm:inline">Git</span>
            </button>
          </div>
        </div>
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
          <div className="min-h-0 min-w-0 flex-1 overflow-hidden">
            <ExplorerTree
              workspaceId={chatWorkspaceId}
              workspaceRootLabel={active?.name || active?.root_path}
              workspaceRootPath={active?.root_path}
              selectedPath={selectedPath}
              onSelectFile={(path, opts) => void onSelectFile(path, opts)}
              onOpenInNewTab={(path, opts) => void onOpenInNewTab(path, opts)}
              onOpenToSide={onOpenToSide}
              onOpenInTerminal={onOpenInTerminal}
              onWorkspaceChanged={() => {
                void refresh()
                setRefreshKey((k) => k + 1)
              }}
              onError={(msg) => setError(msg)}
              refreshKey={refreshKey}
            />
          </div>
          {timelinePath ? (
            <>
              <HorizontalSplitter
                testId="splitter-timeline"
                value={layout.timelineHeight}
                onChange={layout.setTimelineHeight}
                growSide="top"
                minOpposite={120}
                aria-label="Resize timeline panel"
              />
              <div
                data-testid="timeline-panel-shell"
                style={{
                  height: layout.timelineHeight,
                  flex: '0 0 auto',
                }}
                className="min-h-0 overflow-hidden border-t border-shell-border"
              >
                <FileTimeline
                  path={timelinePath}
                  activeEntryId={timelineActiveEntryId}
                  refreshKey={undoRefreshKey + refreshKey}
                  onOpenPath={(path) => void onOpenInNewTab(path)}
                  onOpenDiff={(req) => {
                    const tab = makeDiffTab(req)
                    setTimelineNav({
                      path: req.path,
                      entries: req.entries,
                      index: req.index,
                    })
                    setTabs((prev) => {
                      const existing = prev.find((t) => t.id === tab.id)
                      if (existing) return prev
                      return [...prev, tab]
                    })
                    setActiveTabId(tab.id)
                    setNav('files')
                  }}
                />
              </div>
            </>
          ) : null}
        </div>
      </div>
    )

  const explorerPanel = showLeftPanel ? (
    <div
      data-testid="left-panel"
      data-explorer-dock={layout.explorerDock}
      data-explorer-width={layout.explorerWidth}
      style={{
        width: layout.explorerWidth,
        maxWidth: '45vw',
        flex: '0 0 auto',
        minWidth: 0,
      }}
      className="flex min-h-0 shrink-0 flex-col overflow-hidden border-r border-shell-border bg-shell-panel"
    >
      {leftPanelContent}
    </div>
  ) : null

  const explorerSplitter = showLeftPanel ? (
    <VerticalSplitter
      testId="splitter-explorer"
      value={layout.explorerWidth}
      onChange={layout.setExplorerWidth}
      growSide={layout.explorerDock === 'left' ? 'left' : 'right'}
      aria-label="Resize left panel"
    />
  ) : null

  const main = (
    <MainPane
      selectedFile={selectedPath}
      chatContext={chatContextLabel}
      undoRefreshKey={undoRefreshKey}
      chatWidth={layout.chatWidth}
      chatHeight={layout.chatHeight}
      chatDock={layout.chatDock}
      chatVisible={chatVisible}
      onChatWidthChange={layout.setChatWidth}
      onChatHeightChange={layout.setChatHeight}
      canDiffPrevChange={
        !!timelineNav &&
        primary?.kind === 'diff' &&
        timelineNav.index < timelineNav.entries.length - 1
      }
      canDiffNextChange={
        !!timelineNav && primary?.kind === 'diff' && timelineNav.index > 0
      }
      onDiffPrevChange={() => {
        if (!timelineNav) return
        const next = timelineNav.index + 1
        if (next >= timelineNav.entries.length) return
        const entry = timelineNav.entries[next]
        void (async () => {
          try {
            const change = await getFileTimelineChange({
              path: timelineNav.path,
              source: entry.source,
              unit_id: entry.unit_id,
              sha: entry.sha,
            })
            const tab = makeDiffTab({
              path: timelineNav.path,
              entry,
              change,
              entries: timelineNav.entries,
              index: next,
            })
            setTimelineNav({ ...timelineNav, index: next })
            setTabs((prev) => {
              const without = prev.filter((t) => t.id !== primary?.id)
              if (without.some((t) => t.id === tab.id)) return without
              return [...without, tab]
            })
            setActiveTabId(tab.id)
          } catch (e) {
            setError(e instanceof Error ? e.message : String(e))
          }
        })()
      }}
      onDiffNextChange={() => {
        if (!timelineNav) return
        const next = timelineNav.index - 1
        if (next < 0) return
        const entry = timelineNav.entries[next]
        void (async () => {
          try {
            const change = await getFileTimelineChange({
              path: timelineNav.path,
              source: entry.source,
              unit_id: entry.unit_id,
              sha: entry.sha,
            })
            const tab = makeDiffTab({
              path: timelineNav.path,
              entry,
              change,
              entries: timelineNav.entries,
              index: next,
            })
            setTimelineNav({ ...timelineNav, index: next })
            setTabs((prev) => {
              const without = prev.filter((t) => t.id !== primary?.id)
              if (without.some((t) => t.id === tab.id)) return without
              return [...without, tab]
            })
            setActiveTabId(tab.id)
          } catch (e) {
            setError(e instanceof Error ? e.message : String(e))
          }
        })()
      }}
      explorerDock={layout.explorerDock}
      tabs={tabs}
      activeTabId={activeTabId}
      onActivateTab={onActivateTab}
      onCloseTab={onCloseTab}
      primary={primary}
      secondary={secondary}
      onPrimaryChange={onPrimaryChange}
      onSecondaryChange={(content) =>
        setSecondary((p) => (p ? { ...p, content } : p))
      }
      onPrimaryViewState={onPrimaryViewState}
      onSecondaryViewState={onSecondaryViewState}
      onCloseSecondary={onCloseSecondary}
      onFocusColumn={setFocusedColumn}
      onSaveActive={() => void onSaveActive()}
      onFileChanged={(p) => void onFileChanged(p)}
      onAfterUndo={() => void onFileChanged()}
      workspaceId={chatWorkspaceId}
      needsSession={needsSession}
      onEnsureSession={ensureSessionForChat}
      onCreateNewSession={createNewSessionForHandoff}
      onMaybeTitleSession={maybeTitleSession}
      needsFolderWorkspace={needsFolderWorkspace}
      workspaces={hub?.workspaces || []}
      workspaceBusy={busy}
      onOpenWorkspacePath={onOpenPath}
      onSwitchWorkspace={onSwitch}
      onPlanModeChange={setPlanModeActive}
      onPlanProposed={() => {
        setTodoRefreshKey((k) => k + 1)
      }}
      onTodoUpdated={() => {
        setTodoRefreshKey((k) => k + 1)
      }}
      onPlanDecided={(kind) => {
        setTodoRefreshKey((k) => k + 1)
        if (kind === 'approve') {
          setPlanModeActive(false)
        } else {
          setPlanModeActive(false)
        }
        void refresh()
      }}
      onMissionStatus={() => {
        setMissionRefreshKey((k) => k + 1)
      }}
      onTaskEvent={() => {
        setTaskRefreshKey((k) => k + 1)
      }}
      onBrowserStatus={() => {
        setBrowserRefreshKey((k) => k + 1)
      }}
      onOpenWebPreview={openCanvasGalleryFromChat}
      htmlSyncNonce={htmlSyncNonce}
      todoRefreshKey={todoRefreshKey}
      missionRefreshKey={missionRefreshKey}
      taskRefreshKey={taskRefreshKey}
      browserRefreshKey={browserRefreshKey}
    />
  )

  const gitPanel = (
    <GitPanel
      refreshKey={refreshKey}
      workspaceId={chatWorkspaceId}
      focusTab={gitFocusTab}
      onFocusTabConsumed={() => setGitFocusTab(null)}
      onOpenPath={(path) => {
        setNav('files')
        void onOpenInNewTab(path)
      }}
    />
  )

  const overlayOpen =
    showTerminal ||
    showSettings ||
    showDocs ||
    showGit ||
    showCanvasGallery

  const overlayContent = showTerminal ? (
    <div
      data-testid="terminal-standalone"
      className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-shell-bg"
    >
      <TerminalPanel
        open
        onClose={() => setTerminalOpen(false)}
        cwdHint={hasFolderWorkspace ? active?.root_path : undefined}
        workspaceId={hasFolderWorkspace ? hub?.active_id : undefined}
        openRequest={terminalOpenRequest}
        height={layout.terminalHeight}
        onHeightChange={layout.setTerminalHeight}
        fill
      />
    </div>
  ) : showSettings ? (
    <SettingsPanel
      onOpenTerminalWithCommand={(command, title) => {
        setTerminalOpen(true)
        setTerminalOpenRequest({
          title,
          workspaceId: hub?.active_id,
          nonce: Date.now(),
          command,
        })
      }}
    />
  ) : showDocs ? (
    <DocsPanel initialSlug={docsInitialSlug} />
  ) : showCanvasGallery ? (
    <CanvasGalleryPanel
      workspaces={hub?.workspaces || []}
      busy={busy}
      focusId={galleryFocusId}
      onFocusConsumed={() => setGalleryFocusId(null)}
      onStatusLabelChange={setPlaygroundStatusLabel}
      onOpenSession={(sessionId) => {
        void onSwitch(sessionId)
        setChatBind('hub')
        setNav('sessions')
        setChatVisible(true)
      }}
    />
  ) : showGit ? (
    gitPanel
  ) : null

  const centerContent = (
    <>
      <div
        className={
          overlayOpen
            ? 'hidden'
            : 'flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden'
        }
        aria-hidden={overlayOpen}
      >
        {main}
      </div>
      {overlayOpen ? overlayContent : null}
    </>
  )

  return (
    <div
      data-testid="app-shell"
      data-theme={theme}
      data-explorer-dock={layout.explorerDock}
      data-chat-dock={layout.chatDock}
      data-sidebar-width={48}
      data-explorer-width={layout.explorerWidth}
      className="shell-transition flex h-full min-h-0 min-h-[100dvh] min-w-0 w-full flex-col overflow-hidden bg-shell-bg text-shell-text"
    >
      <div className="flex min-h-0 min-w-0 flex-1 overflow-hidden">
        <Sidebar
          active={nav}
          onChange={onPrimaryNav}
          product={product}
          terminalOpen={terminalOpen}
          onToggleTerminal={() => setTerminalOpen((v) => !v)}
        />

        {layout.explorerDock === 'left' && (
          <>
            {explorerPanel}
            {explorerSplitter}
            {centerContent}
          </>
        )}

        {layout.explorerDock === 'right' && (
          <>
            {centerContent}
            {explorerSplitter}
            {explorerPanel}
          </>
        )}
      </div>

      {error && (
        <div
          data-testid="error-banner"
          className="border-t border-red-900 bg-red-950/50 px-3 py-1 text-xs text-red-300"
        >
          {error}
        </div>
      )}

      {pendingCloseTab && (
        <CloseDirtyDialog
          fileName={baseName(pendingCloseTab.path)}
          onSave={() => void onDirtyDialogSave()}
          onDontSave={onDirtyDialogDontSave}
          onCancel={onDirtyDialogCancel}
        />
      )}

      <CommandPalette
        open={paletteOpen}
        mode={paletteMode}
        onClose={() => setPaletteOpen(false)}
        onRun={runCommand}
      />

      <StatusBar
        product={product}
        message={showFolderGitStatus ? hub?.status_message : undefined}
        workspaceName={
          showCanvasGallery
            ? playgroundStatusLabel || `playground: Library`
            : showSessionsNav
              ? activeIsSession
                ? `session: ${active?.name || ''}`
                : undefined
              : showFolderGitStatus && active && !activeIsSession
                ? active.name
                : activeIsSession
                  ? `session: ${active?.name || ''}`
                  : undefined
        }
        planModeActive={planModeActive || !!hub?.plan_mode_active}
        explorerDock={layout.explorerDock}
        chatDock={layout.chatDock}
        onExplorerDock={layout.setExplorerDock}
        onChatDock={layout.setChatDock}
        onResetLayout={layout.resetLayout}
      />
    </div>
  )
}
