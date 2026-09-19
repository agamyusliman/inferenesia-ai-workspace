import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Trash2 } from 'lucide-react'
import {
  deleteTerminalSnippet,
  listTerminalSnippets,
  saveTerminalSnippet,
  streamTerminal,
  terminalStart,
  terminalStop,
  terminalWrite,
  type TerminalSession,
  type TerminalSnippet,
} from '../../lib/api'
import { ExplorerContextMenu } from '../explorer/ExplorerContextMenu'
import type { ContextMenuItem } from '../explorer/contextMenuItems'
import { useLocale } from '../i18n/LocaleProvider'
import { HorizontalSplitter } from './HorizontalSplitter'
import { TerminalSurface } from './TerminalSurface'
import { LAYOUT_LIMITS } from './useLayoutConfig'
import { VerticalSplitter } from './VerticalSplitter'

const MAX_PANES = 4
type PaneLayout = 'row' | 'col' | 'grid'
const LAYOUT_KEY = 'inferenesia-terminal-pane-layout'
const LAYOUT_KEY_TYPO = 'infernesia-terminal-pane-layout'
const LAYOUT_KEY_LEGACY = 'yura-ai-terminal-pane-layout'

export type OpenTerminalRequest = {
  cwd?: string
  workspaceId?: string
  title?: string
  nonce?: number
  command?: string
}

type Props = {
  open: boolean
  onClose?: () => void
  cwdHint?: string
  workspaceId?: string
  openRequest?: OpenTerminalRequest | null
  height?: number
  onHeightChange?: (height: number) => void
  fill?: boolean
}

type LocalSession = TerminalSession & {
  /** Raw PTY output (ANSI preserved for xterm). */
  output: string
}

const SNIPPET_WIDTH_KEY = 'inferenesia-terminal-snippet-width'
const SNIPPET_WIDTH_KEY_TYPO = 'infernesia-terminal-snippet-width'
const SNIPPET_WIDTH_KEY_LEGACY = 'yura-ai-terminal-snippet-width'
const SNIPPET_WIDTH_MIN = 160
const SNIPPET_WIDTH_MAX = 420
const SNIPPET_WIDTH_DEFAULT = 220

function loadPaneLayout(): PaneLayout {
  try {
    const v =
      sessionStorage.getItem(LAYOUT_KEY) ??
      sessionStorage.getItem(LAYOUT_KEY_TYPO) ??
      sessionStorage.getItem(LAYOUT_KEY_LEGACY)
    if (v === 'row' || v === 'col' || v === 'grid') return v
  } catch {
    // ignore
  }
  return 'row'
}

function loadSnippetWidth(): number {
  try {
    const raw =
      sessionStorage.getItem(SNIPPET_WIDTH_KEY) ??
      sessionStorage.getItem(SNIPPET_WIDTH_KEY_TYPO) ??
      sessionStorage.getItem(SNIPPET_WIDTH_KEY_LEGACY)
    const v = Number(raw)
    if (
      Number.isFinite(v) &&
      v >= SNIPPET_WIDTH_MIN &&
      v <= SNIPPET_WIDTH_MAX
    ) {
      return Math.round(v)
    }
  } catch {
    // ignore
  }
  return SNIPPET_WIDTH_DEFAULT
}

export function TerminalPanel({
  open,
  onClose,
  cwdHint,
  workspaceId,
  openRequest,
  height,
  onHeightChange,
  fill = false,
}: Props) {
  const { t } = useLocale()
  const [sessions, setSessions] = useState<LocalSession[]>([])
  const [paneIds, setPaneIds] = useState<string[]>([])
  const [focusId, setFocusId] = useState<string | null>(null)
  const [paneLayout, setPaneLayout] = useState<PaneLayout>(loadPaneLayout)
  const [tabMenu, setTabMenu] = useState<{
    x: number
    y: number
    sessionId: string
  } | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [snippets, setSnippets] = useState<TerminalSnippet[]>([])
  const [snippetPath, setSnippetPath] = useState<string>('')
  const [snippetOpen, setSnippetOpen] = useState(() => {
    if (typeof window === 'undefined') return false
    return window.matchMedia('(min-width: 768px)').matches
  })
  const [snippetFormOpen, setSnippetFormOpen] = useState(false)
  const [snippetName, setSnippetName] = useState('')
  const [snippetBody, setSnippetBody] = useState('')
  const [snippetBusy, setSnippetBusy] = useState(false)
  const [snippetWidth, setSnippetWidth] = useState(loadSnippetWidth)

  const abortsRef = useRef<Map<string, AbortController>>(new Map())
  const lastOpenNonce = useRef<number | undefined>(undefined)
  const startedOnce = useRef(false)
  const bodyRef = useRef<HTMLDivElement | null>(null)
  const rowRef = useRef<HTMLDivElement | null>(null)

  const panelHeight = Math.max(
    LAYOUT_LIMITS.terminalHeightMin,
    Math.min(
      LAYOUT_LIMITS.terminalHeightMax,
      Math.round(height ?? 208),
    ),
  )

  const visiblePanes = useMemo(() => {
    const ids = paneIds.filter((id) => sessions.some((s) => s.id === id))
    if (ids.length > 0) return ids
    const first = sessions[0]?.id
    return first ? [first] : []
  }, [paneIds, sessions])

  const multiPane = visiblePanes.length > 1
  const focusedId =
    (focusId && sessions.some((s) => s.id === focusId) ? focusId : null) ||
    visiblePanes[0] ||
    sessions[0]?.id ||
    null

  const setLayout = (layout: PaneLayout) => {
    setPaneLayout(layout)
    try {
      sessionStorage.setItem(LAYOUT_KEY, layout)
    } catch {
      // ignore
    }
  }

  const ensurePane = useCallback((id: string) => {
    setPaneIds((prev) => {
      if (prev.includes(id)) return prev
      if (prev.length === 0) return [id]
      if (prev.length >= MAX_PANES) {
        const next = [...prev]
        next[next.length - 1] = id
        return next
      }
      return [...prev, id]
    })
    setFocusId(id)
  }, [])

  const removeFromPanes = useCallback((id: string) => {
    setPaneIds((prev) => {
      const next = prev.filter((p) => p !== id)
      return next
    })
  }, [])

  const movePane = useCallback((id: string, dir: -1 | 1) => {
    setPaneIds((prev) => {
      const i = prev.indexOf(id)
      if (i < 0) return prev
      const j = i + dir
      if (j < 0 || j >= prev.length) return prev
      const next = [...prev]
      ;[next[i], next[j]] = [next[j], next[i]]
      return next
    })
  }, [])

  const sendRaw = useCallback(async (id: string, data: string) => {
    if (!id || !data) return
    try {
      await terminalWrite(id, data)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }, [])

  const attachStream = useCallback((id: string) => {
    abortsRef.current.get(id)?.abort()
    const ac = new AbortController()
    abortsRef.current.set(id, ac)
    streamTerminal(
      id,
      (ev) => {
        if (ev.type === 'snapshot') {
          const output = ev.output || ''
          setSessions((prev) =>
            prev.map((s) =>
              s.id === id
                ? {
                    ...s,
                    ...(ev.session || {}),
                    output,
                  }
                : s,
            ),
          )
          return
        }
        if (ev.type === 'delta') {
          const delta = ev.delta || ''
          setSessions((prev) =>
            prev.map((s) =>
              s.id === id
                ? {
                    ...s,
                    output: (s.output || '') + delta,
                  }
                : s,
            ),
          )
        }
      },
      ac.signal,
    )
  }, [])

  const preferredCwd = cwdHint || ''

  const startSession = useCallback(
    async (opts?: { cwd?: string; title?: string; workspaceId?: string }) => {
      setBusy(true)
      setError(null)
      try {
        const sess = await terminalStart({
          cwd: opts?.cwd ?? preferredCwd,
          workspace_id: opts?.workspaceId || workspaceId || '',
          title: opts?.title,
        })
        const local: LocalSession = { ...sess, output: '' }
        setSessions((prev) => [...prev, local])
        setPaneIds((prev) => (prev.length === 0 ? [sess.id] : prev))
        setFocusId(sess.id)
        attachStream(sess.id)
        return sess
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e))
        return null
      } finally {
        setBusy(false)
      }
    },
    [attachStream, workspaceId, preferredCwd],
  )

  // Explorer "Open in Integrated Terminal"
  useEffect(() => {
    if (!open || !openRequest) return
    if (
      openRequest.nonce !== undefined &&
      openRequest.nonce === lastOpenNonce.current
    ) {
      return
    }
    lastOpenNonce.current = openRequest.nonce
    startedOnce.current = true
    void startSession({
      cwd: openRequest.cwd,
      title: openRequest.title,
      workspaceId: openRequest.workspaceId || workspaceId,
    }).then((sess) => {
      if (sess && openRequest.command) {
        void sendRaw(sess.id, openRequest.command + '\n')
      }
    })
  }, [open, openRequest, startSession, workspaceId, sendRaw])

  // Auto-start one session when the panel first opens.
  useEffect(() => {
    if (!open) return
    if (sessions.length > 0 || startedOnce.current) return
    if (
      openRequest?.nonce !== undefined &&
      openRequest.nonce !== lastOpenNonce.current
    ) {
      return
    }
    startedOnce.current = true
    void startSession({ workspaceId })
  }, [open, sessions.length, startSession, workspaceId, openRequest])

  // Cleanup streams on unmount.
  useEffect(() => {
    return () => {
      for (const ac of abortsRef.current.values()) {
        ac.abort()
      }
      abortsRef.current.clear()
    }
  }, [])

  useEffect(() => {
    setPaneIds((prev) => prev.filter((id) => sessions.some((s) => s.id === id)))
  }, [sessions])

  // Persist snippet sidebar width for the session.
  useEffect(() => {
    try {
      sessionStorage.setItem(
        SNIPPET_WIDTH_KEY,
        String(Math.round(snippetWidth)),
      )
    } catch {
      // ignore
    }
  }, [snippetWidth])

  const onSnippetWidthChange = (next: number) => {
    const row = rowRef.current
    const maxByRow = row
      ? Math.min(
          SNIPPET_WIDTH_MAX,
          Math.max(SNIPPET_WIDTH_MIN, row.getBoundingClientRect().width - 180),
        )
      : SNIPPET_WIDTH_MAX
    const clamped = Math.min(
      maxByRow,
      Math.max(SNIPPET_WIDTH_MIN, Math.round(next)),
    )
    setSnippetWidth(clamped)
  }

  const refreshSnippets = useCallback(async () => {
    try {
      const list = await listTerminalSnippets()
      setSnippets(list.snippets || [])
      setSnippetPath(list.path || '')
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }, [])

  // Load global snippets when panel opens (defaults have no secrets).
  useEffect(() => {
    if (!open) return
    void refreshSnippets()
  }, [open, refreshSnippets])

  const onNewTerminal = () => {
    void startSession({ workspaceId })
  }

  /**
   * Paste snippet body into the focused terminal without executing
   * (strips trailing newline so the user can edit before Enter).
   */
  const onPasteSnippet = (sn: TerminalSnippet) => {
    const target = focusedId
    if (!target || !sn.body) return
    const body = sn.body.replace(/\n+$/, '')
    if (!body) return
    void sendRaw(target, body)
  }

  const onRunSnippet = (sn: TerminalSnippet) => {
    const target = focusedId
    if (!target || !sn.body) return
    let body = sn.body
    if (!body.endsWith('\n')) body = body + '\n'
    void sendRaw(target, body)
  }

  const onSaveSnippet = async () => {
    const name = snippetName.trim()
    let body = snippetBody
    if (!name || !body.trim()) {
      setError('Snippet name and body are required')
      return
    }
    // Ensure a trailing newline so insert behaves like a ready-to-run command.
    if (!body.endsWith('\n')) body = body + '\n'
    setSnippetBusy(true)
    setError(null)
    try {
      await saveTerminalSnippet({ name, body })
      setSnippetName('')
      setSnippetBody('')
      setSnippetFormOpen(false)
      await refreshSnippets()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setSnippetBusy(false)
    }
  }

  const openSnippetForm = () => {
    setSnippetName('')
    setSnippetBody('')
    setSnippetFormOpen(true)
  }

  const onDeleteSnippet = async (id: string) => {
    setSnippetBusy(true)
    setError(null)
    try {
      const list = await deleteTerminalSnippet(id)
      setSnippets(list.snippets || [])
      setSnippetPath(list.path || '')
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setSnippetBusy(false)
    }
  }

  const addPane = async (sessionId?: string) => {
    if (sessionId) {
      ensurePane(sessionId)
      return
    }
    if (visiblePanes.length >= MAX_PANES) {
      setError(`Max ${MAX_PANES} panes`)
      return
    }
    const unused = sessions.find((s) => !visiblePanes.includes(s.id))
    if (unused) {
      ensurePane(unused.id)
      return
    }
    const created = await startSession({ workspaceId })
    if (created) ensurePane(created.id)
  }

  const onToggleSplit = async () => {
    if (multiPane) {
      const keep = focusedId || visiblePanes[0]
      setPaneIds(keep ? [keep] : [])
      return
    }
    await addPane()
  }

  const onCloseSession = (id: string) => {
    abortsRef.current.get(id)?.abort()
    abortsRef.current.delete(id)
    removeFromPanes(id)

    let becameEmpty = false
    let nextFocus: string | null = focusId
    setSessions((prev) => {
      const next = prev.filter((s) => s.id !== id)
      becameEmpty = next.length === 0
      if (focusId === id) nextFocus = next[0]?.id || null
      return next
    })
    if (focusId === id) setFocusId(nextFocus)

    void terminalStop(id).catch(() => {
      // ignore
    })

    if (becameEmpty) {
      startedOnce.current = false
      setPaneIds([])
      setFocusId(null)
      onClose?.()
    }
  }

  const sendInterrupt = () => {
    if (!focusedId) return
    void sendRaw(focusedId, '\u0003')
  }

  const tabMenuItems = (sessionId: string): ContextMenuItem[] => {
    const inPane = visiblePanes.includes(sessionId)
    const paneIdx = visiblePanes.indexOf(sessionId)
    return [
      {
        id: 'open-to-side' as const,
        label: inPane ? 'Focus in pane' : 'Open in new pane',
      },
      {
        id: 'open-in-terminal' as const,
        label: 'Open only this pane',
      },
      { id: 'separator' as const, label: '' },
      {
        id: 'copy-path' as const,
        label: 'Move pane left',
        disabled: !inPane || paneIdx <= 0,
      },
      {
        id: 'copy-relative-path' as const,
        label: 'Move pane right',
        disabled: !inPane || paneIdx < 0 || paneIdx >= visiblePanes.length - 1,
      },
      {
        id: 'cut' as const,
        label: 'Remove from panes',
        disabled: !inPane || visiblePanes.length <= 1,
      },
      { id: 'separator' as const, label: '' },
      {
        id: 'open-with-edit' as const,
        label: 'Align: side by side',
      },
      {
        id: 'open-with-preview' as const,
        label: 'Align: stacked',
      },
      {
        id: 'open-preview' as const,
        label: 'Align: grid (2×2)',
      },
      { id: 'separator' as const, label: '' },
      {
        id: 'delete' as const,
        label: 'Close session',
        danger: true,
      },
    ]
  }

  const runTabMenu = (actionId: string) => {
    if (!tabMenu) return
    const id = tabMenu.sessionId
    setTabMenu(null)
    switch (actionId) {
      case 'open-to-side':
        if (visiblePanes.includes(id)) setFocusId(id)
        else void addPane(id)
        return
      case 'open-in-terminal':
        setPaneIds([id])
        setFocusId(id)
        return
      case 'copy-path':
        movePane(id, -1)
        return
      case 'copy-relative-path':
        movePane(id, 1)
        return
      case 'cut':
        removeFromPanes(id)
        if (focusId === id) {
          setFocusId(visiblePanes.find((p) => p !== id) || null)
        }
        return
      case 'open-with-edit':
        setLayout('row')
        return
      case 'open-with-preview':
        setLayout('col')
        return
      case 'open-preview':
        setLayout('grid')
        return
      case 'delete':
        onCloseSession(id)
        return
      default:
        return
    }
  }

  if (!open) return null

  const paneGridClass =
    paneLayout === 'col'
      ? 'flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden'
      : paneLayout === 'grid' && visiblePanes.length > 2
        ? 'grid min-h-0 min-w-0 flex-1 grid-cols-2 grid-rows-2 overflow-hidden'
        : 'flex min-h-0 min-w-0 flex-1 flex-row overflow-hidden'

  return (
    <div
      data-testid="terminal-panel"
      data-open="true"
      data-fill={fill ? 'true' : 'false'}
      data-input-mode="xterm-surface"
      data-split={multiPane ? 'true' : 'false'}
      data-pane-count={visiblePanes.length}
      data-pane-layout={paneLayout}
      data-height={fill ? undefined : panelHeight}
      style={fill ? { flex: '1 1 auto', minHeight: 0 } : { height: panelHeight }}
      className={
        fill
          ? 'flex min-h-0 flex-1 flex-col bg-[#0d1117]'
          : 'flex shrink-0 flex-col border-t border-shell-border bg-[#0d1117]'
      }
    >
      {!fill && onHeightChange && (
        <HorizontalSplitter
          testId="splitter-terminal"
          value={panelHeight}
          onChange={onHeightChange}
          growSide="top"
          minOpposite={LAYOUT_LIMITS.mainHeightMin}
          aria-label="Resize terminal panel height"
        />
      )}
      <div
        data-testid="terminal-panel-header"
        className="flex h-10 shrink-0 items-center gap-1 border-b border-shell-border bg-shell-panel px-2 text-[11px] text-shell-muted"
      >
        <span className="shrink-0 px-0.5 text-[11px] font-semibold uppercase tracking-wide text-shell-muted">
          {t('nav.terminal')}
        </span>
        <div
          data-testid="terminal-tabs"
          className="flex min-w-0 flex-1 items-center gap-0.5 overflow-x-auto"
        >
          {sessions.map((s) => {
            const inPane = visiblePanes.includes(s.id)
            const paneIndex = visiblePanes.indexOf(s.id)
            const selected = focusedId === s.id
            return (
              <div
                key={s.id}
                data-testid={`terminal-tab-${s.id}`}
                data-active={selected ? 'true' : 'false'}
                data-in-pane={inPane ? 'true' : 'false'}
                data-pane-index={paneIndex >= 0 ? paneIndex : undefined}
                className={`group flex max-w-[10rem] shrink-0 items-center gap-0.5 rounded px-1.5 py-0.5 ${
                  selected
                    ? 'bg-shell-active text-shell-text'
                    : inPane
                      ? 'bg-shell-border/30 text-shell-text'
                      : 'text-shell-muted hover:bg-shell-border/30 hover:text-shell-text'
                }`}
                onContextMenu={(e) => {
                  e.preventDefault()
                  e.stopPropagation()
                  setTabMenu({ x: e.clientX, y: e.clientY, sessionId: s.id })
                }}
              >
                <button
                  type="button"
                  className="min-w-0 truncate"
                  title={`${s.title || s.id} · ${s.cwd}${
                    inPane ? ` · pane ${paneIndex + 1}` : ''
                  } · right-click for actions`}
                  onClick={() => {
                    if (inPane) {
                      setFocusId(s.id)
                      return
                    }
                    if (multiPane && visiblePanes.length < MAX_PANES) {
                      ensurePane(s.id)
                      return
                    }
                    setPaneIds([s.id])
                    setFocusId(s.id)
                  }}
                >
                  {s.title || s.id.slice(-8)}
                  {inPane ? (
                    <span className="ml-0.5 text-[9px] opacity-60">{paneIndex + 1}</span>
                  ) : null}
                </button>
                <button
                  type="button"
                  data-testid={`terminal-close-tab-${s.id}`}
                  title={t('terminal.closeSessionTitle')}
                  className="rounded px-0.5 opacity-60 hover:bg-shell-border/50 hover:opacity-100"
                  onClick={(e) => {
                    e.stopPropagation()
                    onCloseSession(s.id)
                  }}
                >
                  ×
                </button>
              </div>
            )
          })}
        </div>
        <button
          type="button"
          data-testid="terminal-new"
          title={t('terminal.newSession')}
          disabled={busy}
          onClick={onNewTerminal}
          className="rounded border border-shell-border px-1.5 py-0.5 text-shell-muted hover:bg-shell-border/40 hover:text-shell-text disabled:opacity-40"
        >
          +
        </button>
        <button
          type="button"
          data-testid="terminal-split"
          title={
            multiPane
              ? 'Collapse to single pane'
              : `Add pane (max ${MAX_PANES})`
          }
          data-active={multiPane ? 'true' : 'false'}
          disabled={busy}
          onClick={() => void onToggleSplit()}
          className={`rounded border border-shell-border px-1.5 py-0.5 font-mono text-shell-muted hover:bg-shell-border/40 hover:text-shell-text disabled:opacity-40 ${
            multiPane ? 'bg-shell-active text-shell-text' : ''
          }`}
        >
          ‖
        </button>
        <button
          type="button"
          data-testid="terminal-interrupt"
          disabled={!focusedId}
          title={t('terminal.sigint')}
          onClick={sendInterrupt}
          className="rounded border border-shell-border px-1.5 py-0.5 text-shell-muted hover:bg-red-950/50 hover:text-red-300 disabled:opacity-40"
        >
          ^C
        </button>
        <button
          type="button"
          data-testid="terminal-snippets-toggle"
          title={t('terminal.snippetsTitle')}
          data-active={snippetOpen ? 'true' : 'false'}
          onClick={() => setSnippetOpen((v) => !v)}
          className={`rounded border border-shell-border px-1.5 py-0.5 text-shell-muted hover:bg-shell-border/40 hover:text-shell-text ${
            snippetOpen ? 'bg-shell-active text-shell-text' : ''
          }`}
        >
          {t('terminal.snippets')}
        </button>
        {onClose && (
          <button
            type="button"
            data-testid="terminal-panel-close"
            onClick={onClose}
            title={t('terminal.hidePanel')}
            className="rounded px-1.5 py-0.5 text-shell-muted hover:bg-shell-border/40 hover:text-shell-text"
          >
            ✕
          </button>
        )}
      </div>

      {error && (
        <div
          data-testid="terminal-error"
          className="border-b border-red-900/60 bg-red-950/40 px-2 py-0.5 text-[11px] text-red-300"
        >
          {error}
        </div>
      )}

      <div
        ref={rowRef}
        className="flex min-h-0 flex-1 flex-row overflow-hidden"
      >
        <div
          data-testid="terminal-panel-body"
          ref={bodyRef}
          className={paneGridClass}
        >
          {visiblePanes.length === 0 ? (
            <div className="flex flex-1 items-center justify-center text-[11px] text-shell-muted">
              {busy ? 'Starting shell…' : 'No terminal session.'}
            </div>
          ) : (
            visiblePanes.map((pid, idx) => {
              const sess = sessions.find((s) => s.id === pid)
              if (!sess) return null
              return (
                <div
                  key={pid}
                  data-testid={`terminal-pane-${idx}`}
                  className={`flex min-h-0 min-w-0 flex-col overflow-hidden ${
                    multiPane && paneLayout !== 'grid'
                      ? 'border-shell-border/40 ' +
                        (paneLayout === 'col' ? 'border-t first:border-t-0' : 'border-l first:border-l-0')
                      : multiPane
                        ? 'border border-shell-border/30'
                        : ''
                  }`}
                  style={
                    paneLayout === 'grid' && visiblePanes.length > 2
                      ? { minHeight: 0, minWidth: 0 }
                      : multiPane
                        ? { flex: '1 1 0', minWidth: 0, minHeight: 0 }
                        : { flex: '1 1 auto', minWidth: 0 }
                  }
                >
                  <div
                    data-testid={idx === 0 ? 'terminal-cwd' : `terminal-cwd-${idx}`}
                    className="shrink-0 truncate border-b border-shell-border/50 px-2 py-0.5 font-mono text-[10px] text-shell-muted"
                    title={sess.cwd}
                  >
                    {multiPane ? `P${idx + 1} · ` : ''}
                    cwd: {sess.cwd || cwdHint || '(none)'}
                    {sess.status ? ` · ${sess.status}` : ''}
                  </div>
                  <TerminalSurface
                    sessionId={sess.id}
                    output={sess.output || ''}
                    focused={focusedId === sess.id}
                    onFocus={() => setFocusId(sess.id)}
                    onData={(data) => {
                      void sendRaw(sess.id, data)
                    }}
                    testId={idx === 0 ? 'terminal-surface' : `terminal-surface-${idx}`}
                  />
                </div>
              )
            })
          )}

          <div
            data-testid="terminal-output"
            className="sr-only"
            aria-live="polite"
          >
            {sessions
              .find((s) => s.id === focusedId)
              ?.output?.replace(/\x1b\[[0-9;?]*[ -/]*[@-~]/g, '')
              .slice(-400) ||
              (busy ? 'Starting shell…' : 'No terminal session.')}
          </div>
        </div>

        {/* Right snippet sidebar (VAL-IDE-036) — width resizable vs terminal */}
        {snippetOpen && (
          <>
            <VerticalSplitter
              testId="splitter-terminal-snippets"
              value={snippetWidth}
              onChange={onSnippetWidthChange}
              growSide="right"
              minOpposite={180}
              aria-label="Resize snippets sidebar"
            />
            <aside
              data-testid="terminal-snippets"
              data-width={snippetWidth}
              style={{ width: snippetWidth, flex: '0 0 auto' }}
              className="flex min-w-0 shrink-0 flex-col overflow-hidden border-l border-shell-border bg-shell-panel text-[11px] text-shell-text"
            >
              <div className="flex shrink-0 items-center gap-0.5 border-b border-shell-border px-1.5 py-1">
                <span className="min-w-0 flex-1 truncate font-semibold uppercase tracking-wide text-shell-muted">
                  {t('terminal.snippets')}
                </span>
                <button
                  type="button"
                  data-testid="terminal-snippet-add"
                  title={t('terminal.newSnippet')}
                  onClick={openSnippetForm}
                  className="rounded border border-shell-border px-1.5 py-0.5 font-semibold text-shell-muted hover:bg-shell-border/40 hover:text-shell-text"
                >
                  +
                </button>
                <button
                  type="button"
                  data-testid="terminal-snippets-close"
                  title={t('terminal.closeSnippets')}
                  onClick={() => setSnippetOpen(false)}
                  className="rounded px-1 text-shell-muted hover:bg-shell-border/40 hover:text-shell-text"
                >
                  ×
                </button>
              </div>
              <div
                data-testid="terminal-snippets-path"
                className="shrink-0 truncate border-b border-shell-border/50 px-1.5 py-0.5 font-mono text-[9px] text-shell-muted"
                title={snippetPath}
              >
                {snippetPath
                  ? snippetPath.replace(/^.*\/\.(?:inferenesia|infernesia|yura-ai)\//, (m) =>
                      m.includes('inferenesia')
                        ? '~/.inferenesia/'
                        : m.includes('infernesia')
                          ? '~/.infernesia/'
                          : '~/.yura-ai/',
                    )
                  : '~/.inferenesia/snippets/…'}
              </div>
              <ul
                data-testid="terminal-snippets-list"
                className="min-h-0 flex-1 space-y-0.5 overflow-y-auto px-1 py-1"
              >
                {snippets.length === 0 && (
                  <li className="px-1 text-shell-muted">No snippets yet.</li>
                )}
                {snippets.map((sn) => (
                  <li
                    key={sn.id}
                    data-testid={`terminal-snippet-${sn.id}`}
                    className="flex items-center justify-between gap-1 rounded border border-shell-border/60 bg-[#0d1117] px-1.5 py-1"
                  >
                    {/* Title left — label only, never auto-runs. */}
                    <span
                      className="min-w-0 flex-1 truncate font-medium text-shell-text select-text"
                      title={sn.body.slice(0, 160)}
                    >
                      {sn.name}
                    </span>
                    {/* Run / Paste / trash on one row with the title (justify-between). */}
                    <div className="flex shrink-0 items-center gap-0.5">
                      <button
                        type="button"
                        data-testid={`terminal-snippet-run-${sn.id}`}
                        title={t('terminal.runSnippet')}
                        disabled={!focusedId || snippetBusy}
                        onClick={() => onRunSnippet(sn)}
                        className="rounded border border-emerald-900/60 bg-emerald-950/30 px-1.5 py-0.5 text-[10px] font-medium text-emerald-300 hover:bg-emerald-950/60 disabled:opacity-40"
                      >
                        Run
                      </button>
                      <button
                        type="button"
                        data-testid={`terminal-snippet-paste-${sn.id}`}
                        title={t('terminal.pasteSnippet')}
                        disabled={!focusedId || snippetBusy}
                        onClick={() => onPasteSnippet(sn)}
                        className="rounded border border-sky-900/60 bg-sky-950/30 px-1.5 py-0.5 text-[10px] font-medium text-sky-300 hover:bg-sky-950/60 disabled:opacity-40"
                      >
                        Paste
                      </button>
                      <button
                        type="button"
                        data-testid={`terminal-snippet-insert-${sn.id}`}
                        title="Insert (same as Run)"
                        disabled={!focusedId || snippetBusy}
                        onClick={() => onRunSnippet(sn)}
                        className="sr-only"
                      >
                        Insert
                      </button>
                      <button
                        type="button"
                        data-testid={`terminal-snippet-delete-${sn.id}`}
                        title={t('terminal.deleteSnippet')}
                        disabled={snippetBusy}
                        onClick={() => void onDeleteSnippet(sn.id)}
                        className="rounded p-0.5 text-shell-muted hover:bg-red-950/50 hover:text-red-300 disabled:opacity-40"
                        aria-label={`Delete snippet ${sn.name}`}
                      >
                        <Trash2 className="h-3 w-3" strokeWidth={2} />
                      </button>
                    </div>
                  </li>
                ))}
              </ul>
            </aside>
          </>
        )}
      </div>

      {/* New snippet modal form (saves space in the right sidebar) */}
      {snippetFormOpen && (
        <div
          data-testid="terminal-snippet-modal"
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/55 p-4"
          role="dialog"
          aria-modal="true"
          aria-label="Save terminal snippet"
          onClick={(e) => {
            if (e.target === e.currentTarget && !snippetBusy) {
              setSnippetFormOpen(false)
            }
          }}
        >
          <div
            data-testid="terminal-snippet-save-form"
            className="w-full max-w-sm rounded-lg border border-shell-border bg-shell-panel p-3 shadow-xl text-[12px] text-shell-text"
          >
            <div className="mb-2 flex items-center justify-between gap-2">
              <h3 className="font-semibold text-shell-text">New snippet</h3>
              <button
                type="button"
                data-testid="terminal-snippet-modal-close"
                title="Close"
                disabled={snippetBusy}
                onClick={() => setSnippetFormOpen(false)}
                className="rounded px-1.5 text-shell-muted hover:bg-shell-border/40 hover:text-shell-text disabled:opacity-40"
              >
                ×
              </button>
            </div>
            <p className="mb-2 text-[10px] text-shell-muted">
              Stored under ~/.inferenesia. Do not paste secrets or API keys.
            </p>
            <div className="flex flex-col gap-2">
              <label className="flex flex-col gap-0.5">
                <span className="text-[10px] uppercase text-shell-muted">Name</span>
                <input
                  data-testid="terminal-snippet-name"
                  type="text"
                  autoFocus
                  value={snippetName}
                  onChange={(e) => setSnippetName(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Escape') setSnippetFormOpen(false)
                  }}
                  placeholder="e.g. Git status"
                  className="w-full rounded border border-shell-border bg-[#0d1117] px-2 py-1.5 text-[12px] text-shell-text outline-none focus:border-sky-700"
                />
              </label>
              <label className="flex flex-col gap-0.5">
                <span className="text-[10px] uppercase text-shell-muted">Body</span>
                <textarea
                  data-testid="terminal-snippet-body"
                  rows={3}
                  value={snippetBody}
                  onChange={(e) => setSnippetBody(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Escape') setSnippetFormOpen(false)
                    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                      e.preventDefault()
                      void onSaveSnippet()
                    }
                  }}
                  placeholder="command to insert (no secrets)"
                  className="w-full resize-y rounded border border-shell-border bg-[#0d1117] px-2 py-1.5 font-mono text-[12px] text-shell-text outline-none focus:border-sky-700"
                />
              </label>
              <div className="mt-1 flex justify-end gap-1.5">
                <button
                  type="button"
                  data-testid="terminal-snippet-cancel"
                  disabled={snippetBusy}
                  onClick={() => setSnippetFormOpen(false)}
                  className="rounded border border-shell-border px-2.5 py-1 text-shell-muted hover:bg-shell-border/40 hover:text-shell-text disabled:opacity-40"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  data-testid="terminal-snippet-save"
                  disabled={snippetBusy}
                  onClick={() => void onSaveSnippet()}
                  className="rounded border border-shell-border bg-shell-active px-2.5 py-1 text-shell-text hover:bg-shell-border/50 disabled:opacity-40"
                >
                  Save
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {tabMenu && (
        <ExplorerContextMenu
          x={tabMenu.x}
          y={tabMenu.y}
          items={tabMenuItems(tabMenu.sessionId)}
          onAction={runTabMenu}
          onClose={() => setTabMenu(null)}
        />
      )}
    </div>
  )
}
