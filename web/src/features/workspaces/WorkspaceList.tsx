import { useCallback, useState } from 'react'
import { Folder, FolderOpen, Plus } from 'lucide-react'
import type { Workspace } from '../../lib/api'
import {
  copyToClipboard,
  isSessionWorkspace,
  removeWorkspace,
  renameWorkspace,
  revealInOS,
} from '../../lib/api'
import { ConfirmDialog } from '../explorer/ConfirmDialog'
import { ExplorerContextMenu } from '../explorer/ExplorerContextMenu'
import type { ContextMenuItem } from '../explorer/contextMenuItems'
import { useLocale } from '../i18n/LocaleProvider'
import { PanelHeader } from '../shell/PanelHeader'
import { SHELL } from '../shell/shellTokens'
import { revealInOSLabel } from '../../lib/platform'
import { OpenWorkspaceModal } from './OpenWorkspaceModal'

type Props = {
  workspaces: Workspace[]
  activeId?: string
  onSwitch: (id: string) => void
  onOpenPath: (path: string) => void
  onRemoved?: () => void
  busy?: boolean
}

type MenuState = {
  x: number
  y: number
  ws: Workspace
  items: ContextMenuItem[]
}

function menuItemsFor(
  isActive: boolean,
  labels: {
    focus: string
    open: string
    rename: string
    copyPath: string
    remove: string
  },
): ContextMenuItem[] {
  return [
    {
      id: 'open-to-side',
      label: isActive ? labels.focus : labels.open,
    },
    { id: 'rename', label: labels.rename },
    { id: 'separator', label: '' },
    { id: 'reveal-in-os', label: revealInOSLabel() },
    { id: 'copy-path', label: labels.copyPath },
    { id: 'separator', label: '' },
    {
      id: 'remove-folder-from-workspace',
      label: labels.remove,
      danger: true,
    },
  ]
}

export function WorkspaceList({
  workspaces,
  activeId,
  onSwitch,
  onOpenPath,
  onRemoved,
  busy,
}: Props) {
  const { t } = useLocale()
  const folders = workspaces.filter((w) => !isSessionWorkspace(w))
  const [openModal, setOpenModal] = useState(false)
  const [menu, setMenu] = useState<MenuState | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editName, setEditName] = useState('')
  const [removeTarget, setRemoveTarget] = useState<Workspace | null>(null)
  const [removing, setRemoving] = useState(false)

  const openMenu = useCallback(
    (e: React.MouseEvent, ws: Workspace) => {
      e.preventDefault()
      e.stopPropagation()
      const isActive = ws.id === activeId
      // Menu key / Shift+F10 report 0,0 — fall back to the row's own box.
      const rect = e.currentTarget.getBoundingClientRect()
      setMenu({
        x: e.clientX || Math.round(rect.left) + 8,
        y: e.clientY || Math.round(rect.bottom),
        ws,
        items: menuItemsFor(isActive, {
          focus: t('workspace.focusWorkspace'),
          open: t('workspace.openWorkspace'),
          rename: t('workspace.rename'),
          copyPath: t('workspace.copyPath'),
          remove: t('workspace.removeFromList'),
        }),
      })
    },
    [activeId, t],
  )

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
      setActionError(e instanceof Error ? e.message : String(e))
    }
  }, [editName, editingId, onRemoved])

  const confirmRemove = useCallback(async () => {
    if (!removeTarget || removing) return
    setRemoving(true)
    setActionError(null)
    try {
      await removeWorkspace(removeTarget.id)
      setRemoveTarget(null)
      onRemoved?.()
    } catch (e) {
      setActionError(e instanceof Error ? e.message : String(e))
      setRemoveTarget(null)
    } finally {
      setRemoving(false)
    }
  }, [onRemoved, removeTarget, removing])

  const runMenu = useCallback(
    async (actionId: string) => {
      if (!menu) return
      const { ws } = menu
      setMenu(null)
      setActionError(null)
      try {
        switch (actionId) {
          case 'open-to-side':
            onSwitch(ws.id)
            return
          case 'rename':
            setEditingId(ws.id)
            setEditName(ws.name)
            return
          case 'reveal-in-os':
            await revealInOS(ws.id, '')
            return
          case 'copy-path':
            await copyToClipboard(ws.root_path)
            return
          case 'remove-folder-from-workspace':
            setRemoveTarget(ws)
            return
          default:
            return
        }
      } catch (e) {
        setActionError(e instanceof Error ? e.message : String(e))
      }
    },
    [menu, onSwitch],
  )

  return (
    <div data-testid="workspace-list" className="flex h-full min-h-0 flex-col overflow-hidden">
      <PanelHeader
        title={t('workspace.workspaces')}
        dense
        actions={
          <button
            type="button"
            data-testid="add-workspace-btn"
            disabled={busy}
            onClick={() => setOpenModal(true)}
            title={t('workspace.open')}
            aria-label={t('workspace.open')}
            aria-haspopup="dialog"
            className="shell-primary-button inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-md shadow-sm disabled:opacity-40"
          >
            <Plus size={13} strokeWidth={2.5} aria-hidden />
          </button>
        }
      />

      <ul
        data-testid="workspace-folder-list"
        className="explorer-scroll min-h-0 flex-1 overflow-auto px-1 py-1"
        aria-label={t('workspace.workspaces')}
      >
        {folders.length === 0 && (
          <li data-testid="workspace-list-empty" className="px-2 py-3">
            <p className="text-[12px] font-medium text-shell-text">
              {t('workspace.noRecent')}
            </p>
            <p className="mt-1 text-[11px] leading-relaxed text-shell-muted">
              {t('workspace.folderHint')}
            </p>
            <button
              type="button"
              data-testid="workspace-list-empty-open"
              disabled={busy}
              onClick={() => setOpenModal(true)}
              aria-haspopup="dialog"
              className="shell-primary-button mt-2.5 flex w-full items-center justify-center gap-1.5 rounded-md px-2 py-1.5 text-[11px] font-semibold disabled:opacity-40"
            >
              <FolderOpen size={SHELL.iconXs} aria-hidden />
              {t('workspace.open')}
            </button>
          </li>
        )}
        {folders.map((ws) => {
          const isActive = ws.id === activeId
          const isEditing = editingId === ws.id
          return (
            <li key={ws.id}>
              {isEditing ? (
                <div className="mb-0.5 flex items-center gap-1 rounded-md bg-shell-active px-1.5 py-1">
                  <input
                    data-testid={`workspace-rename-input-${ws.id}`}
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
                  data-testid={`workspace-item-${ws.id}`}
                  data-workspace-id={ws.id}
                  data-workspace-name={ws.name}
                  data-active={isActive ? 'true' : 'false'}
                  aria-current={isActive ? 'true' : undefined}
                  disabled={busy}
                  onClick={() => onSwitch(ws.id)}
                  onContextMenu={(e) => openMenu(e, ws)}
                  title={`${ws.name}\n${ws.root_path}`}
                  className="shell-list-item group mb-0.5 flex w-full min-w-0 items-center gap-2 rounded-md px-2 py-1.5 text-left"
                >
                  {isActive ? (
                    <FolderOpen
                      size={SHELL.iconXs}
                      strokeWidth={1.75}
                      className="shrink-0 text-shell-text"
                      aria-hidden
                    />
                  ) : (
                    <Folder
                      size={SHELL.iconXs}
                      strokeWidth={1.75}
                      className="shrink-0 text-shell-muted group-hover:text-shell-text"
                      aria-hidden
                    />
                  )}
                  <span className="min-w-0 flex-1">
                    <span
                      className={`block truncate text-[12px] font-medium leading-tight transition-colors ${
                        isActive
                          ? 'text-shell-text'
                          : 'text-shell-text/90 group-hover:text-shell-text'
                      }`}
                    >
                      {ws.name}
                      {ws.broken ? ' (missing)' : ''}
                    </span>
                    <span className="mt-0.5 block truncate font-mono text-[11px] leading-tight text-shell-muted">
                      {ws.root_path}
                    </span>
                  </span>
                </button>
              )}
            </li>
          )
        })}
      </ul>

      {actionError && (
        <p
          data-testid="add-workspace-error"
          role="alert"
          className="shrink-0 border-t border-shell-border px-2 py-1.5 text-[11px] leading-snug text-red-400"
        >
          {actionError}
        </p>
      )}

      {menu && (
        <ExplorerContextMenu
          x={menu.x}
          y={menu.y}
          items={menu.items}
          onAction={(id) => void runMenu(id)}
          onClose={() => setMenu(null)}
        />
      )}

      {openModal && (
        <OpenWorkspaceModal
          workspaces={workspaces}
          busy={busy}
          onOpenPath={onOpenPath}
          onSwitch={onSwitch}
          onClose={() => setOpenModal(false)}
        />
      )}

      {removeTarget && (
        <ConfirmDialog
          title={t('workspace.removeWorkspaceConfirm')}
          message={t('workspace.removeWorkspaceMessage', {
            name: removeTarget.name,
          })}
          confirmLabel={
            removing ? t('workspace.removing') : t('workspace.removeFromList')
          }
          danger
          onConfirm={() => void confirmRemove()}
          onCancel={() => {
            if (!removing) setRemoveTarget(null)
          }}
        />
      )}
    </div>
  )
}
