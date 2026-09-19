import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { MessageSquare, Plus, Search, X } from 'lucide-react'
import type { HubState, Workspace } from '../../lib/api'
import {
  createSessionWorkspace,
  isSessionWorkspace,
  removeWorkspace,
  renameWorkspace,
} from '../../lib/api'
import { clearAllCanvasStorageForSession } from '../canvas-gallery/galleryStore'
import { ConfirmDialog } from '../explorer/ConfirmDialog'
import { useLocale } from '../i18n/LocaleProvider'
import { PanelHeader } from '../shell/PanelHeader'
import { SHELL } from '../shell/shellTokens'

function sessionIdTimeMs(id: string): number {
  const idMatch = /^ws_sess_(\d+)$/.exec(id || '')
  if (!idMatch) return 0
  const digits = idMatch[1]
  const ms =
    digits.length > 13 ? Number(digits.slice(0, -6)) : Number(digits)
  return Number.isFinite(ms) && ms > 0 ? ms : 0
}

function sessionOpenedMs(ws: Workspace): number {
  if (ws.last_opened_at) {
    const t = Date.parse(ws.last_opened_at)
    if (!Number.isNaN(t) && t > 0) return t
  }
  return sessionIdTimeMs(ws.id)
}

function sessionRecencyKey(ws: Workspace): number {
  const opened = sessionOpenedMs(ws)
  if (opened > 0) return opened
  return sessionIdTimeMs(ws.id)
}

function sessionSubtitle(ws: Workspace): string {
  const ms = sessionOpenedMs(ws)
  if (ms > 0) {
    try {
      // Compact stamp: full toLocaleString() clips in a narrow sidebar.
      return new Date(ms).toLocaleString(undefined, {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      })
    } catch {
      return ''
    }
  }
  return ''
}

type Props = {
  workspaces: Workspace[]
  activeId?: string
  onSwitch: (id: string, hub?: HubState) => void | Promise<void>
  onDraftNew?: () => void
  onRemoved?: () => void
  busy?: boolean
}

type CtxMenu = { x: number; y: number; ws: Workspace }

export function SessionList({
  workspaces,
  activeId,
  onSwitch,
  onDraftNew,
  onRemoved,
  busy,
}: Props) {
  const { t } = useLocale()
  const sessions = useMemo(
    () =>
      workspaces
        .filter(isSessionWorkspace)
        .slice()
        .sort((a, b) => sessionRecencyKey(b) - sessionRecencyKey(a)),
    [workspaces],
  )
  const [query, setQuery] = useState('')
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editName, setEditName] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<Workspace | null>(null)
  const [ctxMenu, setCtxMenu] = useState<CtxMenu | null>(null)
  const searchRef = useRef<HTMLInputElement | null>(null)

  const clearQuery = useCallback(() => {
    setQuery('')
    searchRef.current?.focus()
  }, [])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return sessions
    return sessions.filter((ws) => {
      const name = (ws.name || '').toLowerCase()
      const id = (ws.id || '').toLowerCase()
      return name.includes(q) || id.includes(q)
    })
  }, [query, sessions])

  const onCreate = async () => {
    if (busy || creating) return
    if (onDraftNew) {
      onDraftNew()
      return
    }
    setCreating(true)
    setError(null)
    try {
      const res = await createSessionWorkspace()
      await Promise.resolve(onSwitch(res.workspace.id, res.hub))
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setCreating(false)
    }
  }

  const startRename = (ws: Workspace) => {
    setCtxMenu(null)
    setEditingId(ws.id)
    setEditName(ws.name)
    setError(null)
  }

  const commitRename = useCallback(async () => {
    if (!editingId) return
    const name = editName.trim()
    if (!name) {
      setEditingId(null)
      return
    }
    try {
      await renameWorkspace(editingId, name)
      setEditingId(null)
      onRemoved?.()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }, [editName, editingId, onRemoved])

  const openDelete = (ws: Workspace) => {
    setCtxMenu(null)
    setDeleteTarget(ws)
    setError(null)
  }

  const confirmDelete = async () => {
    if (!deleteTarget || deleting) return
    const id = deleteTarget.id
    setDeleting(true)
    setError(null)
    try {
      await removeWorkspace(id)
      clearAllCanvasStorageForSession(id)
      setDeleteTarget(null)
      onRemoved?.()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      setDeleteTarget(null)
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div data-testid="session-list" className="flex h-full min-h-0 flex-col overflow-hidden">
      <PanelHeader
        title={t('workspace.sessions')}
        dense
        actions={
          <button
            type="button"
            data-testid="session-create-btn"
            disabled={busy || creating || deleting}
            onClick={() => void onCreate()}
            title={
              creating ? t('workspace.creating') : t('workspace.newSession')
            }
            aria-label={
              creating ? t('workspace.creating') : t('workspace.newSession')
            }
            className="shell-primary-button inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-md shadow-sm disabled:opacity-40"
          >
            <Plus size={13} strokeWidth={2.5} aria-hidden />
          </button>
        }
      />

      <div
        data-testid="session-search-toolbar"
        className="shrink-0 border-b border-shell-border px-1.5 py-1.5"
      >
        <label className="relative flex items-center">
          <span className="sr-only">{t('workspace.searchSessions')}</span>
          <Search
            size={SHELL.iconXs}
            className="pointer-events-none absolute left-2 text-shell-muted"
            aria-hidden
          />
          <input
            ref={searchRef}
            data-testid="session-search-input"
            type="text"
            inputMode="search"
            autoComplete="off"
            spellCheck={false}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Escape' && query) {
                e.stopPropagation()
                setQuery('')
              }
            }}
            placeholder={t('workspace.searchSessions')}
            className="w-full rounded-md border border-shell-border bg-shell-bg py-1.5 pl-7 pr-7 text-[12px] text-shell-text outline-none placeholder:text-shell-muted focus:border-shell-accent/50"
          />
          {query.trim() ? (
            <button
              type="button"
              data-testid="session-search-clear"
              title={t('action.clear')}
              aria-label={t('action.clear')}
              onClick={clearQuery}
              className="absolute right-1 rounded p-1 text-shell-muted hover:bg-shell-hover hover:text-shell-text"
            >
              <X size={12} aria-hidden />
            </button>
          ) : null}
        </label>
      </div>

      <ul
        data-testid="session-tab-list"
        className="explorer-scroll min-h-0 flex-1 overflow-auto px-1 py-1"
        aria-label={t('workspace.sessions')}
      >
        {sessions.length === 0 && (
          <li
            data-testid="session-list-empty"
            className="px-2 py-3 text-[11px] leading-relaxed text-shell-muted"
          >
            {t('workspace.noSessions')}
          </li>
        )}
        {sessions.length > 0 && filtered.length === 0 && (
          <li data-testid="session-search-empty" className="px-2 py-3">
            <p className="text-[11px] leading-relaxed text-shell-muted">
              {t('workspace.noSessionMatches')}
            </p>
            <button
              type="button"
              data-testid="session-search-empty-clear"
              onClick={clearQuery}
              className="mt-1.5 rounded px-1.5 py-0.5 text-[11px] font-medium text-shell-accent hover:bg-shell-hover"
            >
              {t('action.clear')}
            </button>
          </li>
        )}
        {filtered.map((ws) => {
          const isActive = ws.id === activeId
          const isEditing = editingId === ws.id
          return (
            <li key={ws.id} className="mb-0.5">
              {isEditing ? (
                <div className="flex items-center gap-1 rounded-md bg-shell-active px-1.5 py-1">
                  <input
                    data-testid={`session-rename-input-${ws.id}`}
                    autoFocus
                    value={editName}
                    onChange={(e) => setEditName(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') void commitRename()
                      if (e.key === 'Escape') setEditingId(null)
                    }}
                    onBlur={() => void commitRename()}
                    className="min-w-0 flex-1 rounded border border-shell-border bg-shell-bg px-1.5 py-0.5 text-xs text-shell-text outline-none"
                  />
                </div>
              ) : (
                <button
                  type="button"
                  aria-current={isActive ? 'true' : undefined}
                  data-testid={`session-item-${ws.id}`}
                  data-active={isActive ? 'true' : 'false'}
                  disabled={busy || deleting}
                  onClick={() => void Promise.resolve(onSwitch(ws.id))}
                  onContextMenu={(e) => {
                    e.preventDefault()
                    e.stopPropagation()
                    // Keyboard menu key reports 0,0 — anchor to the row instead.
                    const rect = e.currentTarget.getBoundingClientRect()
                    setCtxMenu({
                      x: e.clientX || Math.round(rect.left) + 8,
                      y: e.clientY || Math.round(rect.bottom),
                      ws,
                    })
                  }}
                  title={ws.name}
                  className="shell-list-item group flex w-full min-w-0 items-center gap-2 rounded-md px-2 py-1.5 text-left"
                >
                  <MessageSquare
                    size={SHELL.iconXs}
                    strokeWidth={1.75}
                    className={`shrink-0 transition-colors ${
                      isActive
                        ? 'text-shell-text'
                        : 'text-shell-muted group-hover:text-shell-text'
                    }`}
                    aria-hidden
                  />
                  <span className="min-w-0 flex-1">
                    <span
                      className={`block truncate text-[12px] font-medium leading-tight transition-colors ${
                        isActive
                          ? 'text-shell-text'
                          : 'text-shell-text/90 group-hover:text-shell-text'
                      }`}
                    >
                      {ws.name}
                    </span>
                    <span
                      data-testid={`session-item-subtitle-${ws.id}`}
                      className="mt-0.5 block truncate text-[11px] leading-tight text-shell-muted"
                    >
                      {sessionSubtitle(ws) || '—'}
                    </span>
                  </span>
                </button>
              )}
            </li>
          )
        })}
      </ul>

      {error && (
        <p
          data-testid="session-list-error"
          role="alert"
          className="shrink-0 border-t border-shell-border px-2 py-1.5 text-[11px] leading-snug text-red-400"
        >
          {error}
        </p>
      )}

      {deleteTarget && (
        <ConfirmDialog
          title={t('workspace.deleteSessionConfirm')}
          message={t('workspace.deleteSessionMessage', {
            name: deleteTarget.name,
          })}
          confirmLabel={
            deleting ? t('workspace.deleting') : t('action.delete')
          }
          onConfirm={() => void confirmDelete()}
          onCancel={() => {
            if (!deleting) setDeleteTarget(null)
          }}
        />
      )}

      {ctxMenu &&
        typeof document !== 'undefined' &&
        createPortal(
          <SessionContextMenu
            x={ctxMenu.x}
            y={ctxMenu.y}
            name={ctxMenu.ws.name}
            onClose={() => setCtxMenu(null)}
            onRename={() => startRename(ctxMenu.ws)}
            onDelete={() => openDelete(ctxMenu.ws)}
            renameLabel={t('workspace.renameSession')}
            deleteLabel={t('workspace.deleteSession')}
          />,
          document.body,
        )}
    </div>
  )
}

function SessionContextMenu({
  x,
  y,
  name,
  onClose,
  onRename,
  onDelete,
  renameLabel,
  deleteLabel,
}: {
  x: number
  y: number
  name: string
  onClose: () => void
  onRename: () => void
  onDelete: () => void
  renameLabel: string
  deleteLabel: string
}) {
  const ref = useRef<HTMLDivElement | null>(null)
  const firstItemRef = useRef<HTMLButtonElement | null>(null)
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

  // Menu key / Shift+F10 opens this without a pointer — land focus inside it.
  useEffect(() => {
    firstItemRef.current?.focus()
  }, [])

  const left = Math.min(
    x,
    Math.max(8, (typeof window !== 'undefined' ? window.innerWidth : x) - 180),
  )
  const top = Math.min(
    y,
    Math.max(8, (typeof window !== 'undefined' ? window.innerHeight : y) - 100),
  )
  const item =
    'flex w-full items-center px-2.5 py-1.5 text-left text-[11px] text-shell-text hover:bg-shell-border/40'

  return (
    <div
      ref={ref}
      data-testid="session-context-menu"
      role="menu"
      aria-label={name}
      className="fixed z-[220] min-w-[10rem] rounded-md border border-shell-border bg-shell-panel py-0.5 shadow-lg shadow-black/40"
      style={{ left, top }}
    >
      <button
        type="button"
        role="menuitem"
        ref={firstItemRef}
        data-testid="session-ctx-rename"
        className={item}
        onClick={onRename}
      >
        {renameLabel}
      </button>
      <div role="separator" className="my-0.5 border-t border-shell-border" />
      <button
        type="button"
        role="menuitem"
        data-testid="session-ctx-delete"
        className={`${item} text-red-300 hover:text-red-200`}
        onClick={onDelete}
      >
        {deleteLabel}
      </button>
    </div>
  )
}
