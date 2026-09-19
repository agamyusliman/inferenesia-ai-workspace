import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type MouseEvent as ReactMouseEvent,
} from 'react'
import { ChevronDown, ChevronRight, Columns2 } from 'lucide-react'
import { Tree, type NodeApi, type NodeRendererProps } from 'react-arborist'
import {
  addFolderToWorkspace,
  copyPathFS,
  copyToClipboard,
  createFile,
  createFolder,
  deletePath,
  listDir,
  movePathFS,
  removeWorkspace,
  renamePath,
  resolvePath,
  revealInOS,
  type DirEntry,
} from '../../lib/api'
import {
  isRenderablePreviewPath,
  supportsMarkdownPreview,
  type EditorViewMode,
} from '../editor/editorMode'
import { useLocale } from '../i18n/LocaleProvider'
import { ConfirmDialog } from './ConfirmDialog'
import {
  baseName as pathBaseName,
  buildContextMenuItems,
  parentRel,
  type ContextMenuActionId,
  type ContextMenuItem,
  type ContextTargetKind,
} from './contextMenuItems'
import { ExplorerContextMenu } from './ExplorerContextMenu'
import { FindInFolderPanel } from './FindInFolderPanel'
import { fileIcon } from './fileIcons'
import { InlineNameInput } from './InlineNameInput'

/** In-app explorer tree clipboard (not OS clipboard) for cut/copy/paste. */
type TreeClipboard = {
  mode: 'cut' | 'copy'
  path: string
  isDir: boolean
  name: string
}

export type ExplorerNode = {
  id: string
  name: string
  path: string
  isDir: boolean
  /** Folders only: array (possibly empty while loading). Files omit this so they are leaves. */
  children?: ExplorerNode[]
  loaded?: boolean
}

export type OpenFileOptions = {
  mode?: EditorViewMode
  newTab?: boolean
}

type Props = {
  workspaceId?: string
  workspaceRootLabel?: string
  /** Absolute workspace root path (for virtual root context + status). */
  workspaceRootPath?: string
  selectedPath?: string
  /** Single-click: open/replace active tab. */
  onSelectFile: (path: string, options?: OpenFileOptions) => void
  /** Double-click: open in a new tab. */
  onOpenInNewTab?: (path: string, options?: OpenFileOptions) => void
  onOpenToSide?: (path: string) => void
  /**
   * Open integrated terminal with cwd = selected folder (or workspace root).
   * VAL-IDE-012 / multi-session: each call opens a new terminal tab.
   */
  onOpenInTerminal?: (folderRelPath: string) => void
  /** Called after workspace registry mutation (add/remove folder). */
  onWorkspaceChanged?: () => void
  /** Surface errors to the shell banner. */
  onError?: (message: string) => void
  refreshKey?: string | number
}

type MenuState = {
  x: number
  y: number
  kind: ContextTargetKind
  path: string
  name: string
  isDir: boolean
  items: ContextMenuItem[]
}

type InlineMode =
  | { type: 'create-file'; parent: string }
  | { type: 'create-folder'; parent: string }
  | { type: 'rename'; path: string; isDir: boolean }

function entryToNode(entry: DirEntry): ExplorerNode {
  if (entry.is_dir) {
    return {
      id: entry.path,
      name: entry.name,
      path: entry.path,
      isDir: true,
      children: [],
      loaded: false,
    }
  }
  return {
    id: entry.path,
    name: entry.name,
    path: entry.path,
    isDir: false,
  }
}

function sortEntries(entries: DirEntry[] | null | undefined): DirEntry[] {
  const list = Array.isArray(entries) ? entries : []
  return [...list].sort((a, b) => {
    if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1
    return a.name.localeCompare(b.name, undefined, { sensitivity: 'base' })
  })
}

function updateFolderChildren(
  nodes: ExplorerNode[],
  folderPath: string,
  children: ExplorerNode[],
): ExplorerNode[] {
  return nodes.map((n) => {
    if (n.path === folderPath && n.isDir) {
      return { ...n, children, loaded: true }
    }
    if (n.isDir && n.children?.length) {
      return {
        ...n,
        children: updateFolderChildren(n.children, folderPath, children),
      }
    }
    return n
  })
}

export function ExplorerTree({
  workspaceId,
  workspaceRootLabel,
  workspaceRootPath,
  selectedPath,
  onSelectFile,
  onOpenInNewTab,
  onOpenToSide,
  onOpenInTerminal,
  onWorkspaceChanged,
  onError,
  refreshKey,
}: Props) {
  const { t } = useLocale()
  const [data, setData] = useState<ExplorerNode[]>([])
  const [loadingRoot, setLoadingRoot] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const containerRef = useRef<HTMLDivElement | null>(null)
  const [size, setSize] = useState({ width: 240, height: 400 })
  const callbacksRef = useRef({
    onSelectFile,
    onOpenInNewTab,
    onOpenToSide,
    onOpenInTerminal,
    onWorkspaceChanged,
    onError,
  })
  callbacksRef.current = {
    onSelectFile,
    onOpenInNewTab,
    onOpenToSide,
    onOpenInTerminal,
    onWorkspaceChanged,
    onError,
  }
  // Delay single-click open so double-click can cancel and open a new tab instead.
  const clickTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const [menu, setMenu] = useState<MenuState | null>(null)
  const [inline, setInline] = useState<InlineMode | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<{
    path: string
    name: string
    isDir: boolean
  } | null>(null)
  const [clipboard, setClipboard] = useState<TreeClipboard | null>(null)
  const [findFolder, setFindFolder] = useState<{
    folder: string
    label: string
  } | null>(null)

  useEffect(() => {
    return () => {
      if (clickTimerRef.current) clearTimeout(clickTimerRef.current)
    }
  }, [])

  const measure = useCallback(() => {
    const el = containerRef.current
    if (!el) return
    const r = el.getBoundingClientRect()
    const w = Math.max(120, Math.floor(r.width))
    const h = Math.max(80, Math.floor(r.height))
    setSize((prev) => (prev.width === w && prev.height === h ? prev : { width: w, height: h }))
  }, [])

  useLayoutEffect(() => {
    measure()
    const el = containerRef.current
    if (!el || typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(() => measure())
    ro.observe(el)
    return () => ro.disconnect()
  }, [measure])

  const reportError = useCallback((msg: string) => {
    setError(msg)
    callbacksRef.current.onError?.(msg)
  }, [])

  const loadRoot = useCallback(async () => {
    if (!workspaceRootLabel) {
      setData([])
      return
    }
    setLoadingRoot(true)
    setError(null)
    try {
      const entries = sortEntries(await listDir(workspaceId || '', ''))
      setData(entries.map(entryToNode))
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      setData([])
    } finally {
      setLoadingRoot(false)
    }
  }, [workspaceId, workspaceRootLabel])

  useEffect(() => {
    void loadRoot()
  }, [loadRoot, refreshKey])

  const loadFolder = useCallback(
    async (folderPath: string) => {
      try {
        const entries = sortEntries(await listDir(workspaceId || '', folderPath))
        const children = entries.map(entryToNode)
        setData((prev) => updateFolderChildren(prev, folderPath, children))
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e))
      }
    },
    [workspaceId],
  )

  const refreshParent = useCallback(
    async (parentPath: string) => {
      if (!parentPath) {
        await loadRoot()
        return
      }
      await loadFolder(parentPath)
    },
    [loadFolder, loadRoot],
  )

  const onToggle = useCallback(
    (id: string) => {
      setData((prev) => {
        const find = (nodes: ExplorerNode[]): ExplorerNode | null => {
          for (const n of nodes) {
            if (n.id === id) return n
            if (n.children) {
              const hit = find(n.children)
              if (hit) return hit
            }
          }
          return null
        }
        const node = find(prev)
        if (node?.isDir && !node.loaded) {
          void loadFolder(node.path)
        }
        return prev
      })
    },
    [loadFolder],
  )

  const openMenu = useCallback(
    (e: ReactMouseEvent, kind: ContextTargetKind, path: string, name: string, isDir: boolean) => {
      e.preventDefault()
      e.stopPropagation()
      const hasMultiple =
        kind === 'file' && supportsMarkdownPreview(path)
      const canPreview = kind === 'file' && isRenderablePreviewPath(path)
      const items = buildContextMenuItems({
        kind,
        path,
        name,
        hasMultipleOpenModes: hasMultiple,
        canOpenPreview: canPreview,
        canPaste: !!clipboard,
        openModes: hasMultiple
          ? [
              { id: 'open-with-edit', label: 'Text' },
              { id: 'open-with-preview', label: 'Preview' },
            ]
          : undefined,
      })
      setMenu({ x: e.clientX, y: e.clientY, kind, path, name, isDir, items })
      setInline(null)
    },
    [clipboard],
  )

  const runMenuAction = useCallback(
    async (actionId: string) => {
      if (!menu) return
      const { path, kind, isDir, name } = menu
      setMenu(null)

      const wid = workspaceId || ''
      try {
        switch (actionId as ContextMenuActionId) {
          case 'new-file': {
            const parent = kind === 'root' ? '' : path
            setInline({ type: 'create-file', parent })
            return
          }
          case 'new-folder': {
            const parent = kind === 'root' ? '' : path
            setInline({ type: 'create-folder', parent })
            return
          }
          case 'add-folder-to-workspace': {
            await addFolderToWorkspace(wid, path)
            callbacksRef.current.onWorkspaceChanged?.()
            return
          }
          case 'remove-folder-from-workspace': {
            if (!wid) {
              reportError('No workspace id to remove')
              return
            }
            await removeWorkspace(wid)
            callbacksRef.current.onWorkspaceChanged?.()
            return
          }
          case 'copy-path': {
            const info = await resolvePath(wid, path)
            await copyToClipboard(info.absolute)
            return
          }
          case 'copy-relative-path': {
            const rel = path || (await resolvePath(wid, path)).relative
            await copyToClipboard(rel || '.')
            return
          }
          case 'reveal-in-os': {
            await revealInOS(wid, path)
            return
          }
          case 'rename': {
            setInline({ type: 'rename', path, isDir })
            return
          }
          case 'delete': {
            setDeleteTarget({ path, name, isDir })
            return
          }
          case 'open-to-side': {
            callbacksRef.current.onOpenToSide?.(path)
            return
          }
          case 'open-preview': {
            // VAL-IDE-026: open renderable file in preview mode as a new tab.
            callbacksRef.current.onSelectFile(path, { mode: 'preview', newTab: true })
            return
          }
          case 'open-with-edit': {
            callbacksRef.current.onSelectFile(path, { mode: 'edit', newTab: true })
            return
          }
          case 'open-with-preview': {
            callbacksRef.current.onSelectFile(path, { mode: 'preview', newTab: true })
            return
          }
          case 'open-in-terminal': {
            // Root → empty rel path (workspace cwd); folder → its rel path.
            callbacksRef.current.onOpenInTerminal?.(kind === 'root' ? '' : path)
            return
          }
          case 'find-in-folder': {
            setFindFolder({
              folder: kind === 'root' ? '' : path,
              label: kind === 'root' ? workspaceRootLabel || 'workspace' : name || path,
            })
            return
          }
          case 'cut': {
            setClipboard({ mode: 'cut', path, isDir, name })
            return
          }
          case 'copy': {
            setClipboard({ mode: 'copy', path, isDir, name })
            return
          }
          case 'paste': {
            if (!clipboard) {
              reportError('Nothing to paste')
              return
            }
            const destParent = kind === 'root' ? '' : path
            const res =
              clipboard.mode === 'cut'
                ? await movePathFS(wid, clipboard.path, destParent)
                : await copyPathFS(wid, clipboard.path, destParent)
            if (clipboard.mode === 'cut') {
              // Clear clipboard after successful cut (like VS Code).
              setClipboard(null)
              // Refresh both source parent and dest.
              await refreshParent(parentRel(clipboard.path))
            }
            await refreshParent(destParent)
            if (!res.is_dir && res.path) {
              callbacksRef.current.onSelectFile(res.path)
            }
            return
          }
          default:
            return
        }
      } catch (e) {
        reportError(e instanceof Error ? e.message : String(e))
      }
    },
    [clipboard, menu, refreshParent, reportError, workspaceId, workspaceRootLabel],
  )

  const submitInline = useCallback(
    async (name: string) => {
      if (!inline) return
      const wid = workspaceId || ''
      const current = inline
      setInline(null)
      try {
        if (current.type === 'create-file') {
          const res = await createFile(wid, current.parent, name)
          await refreshParent(current.parent)
          callbacksRef.current.onSelectFile(res.path, { mode: 'edit', newTab: true })
          return
        }
        if (current.type === 'create-folder') {
          await createFolder(wid, current.parent, name)
          await refreshParent(current.parent)
          return
        }
        if (current.type === 'rename') {
          const res = await renamePath(wid, current.path, name)
          await refreshParent(parentRel(current.path))
          if (!current.isDir) {
            callbacksRef.current.onSelectFile(res.path)
          }
        }
      } catch (e) {
        reportError(e instanceof Error ? e.message : String(e))
      }
    },
    [inline, refreshParent, reportError, workspaceId],
  )

  const confirmDelete = useCallback(async () => {
    if (!deleteTarget) return
    const { path } = deleteTarget
    setDeleteTarget(null)
    try {
      await deletePath(workspaceId || '', path)
      await refreshParent(parentRel(path))
    } catch (e) {
      reportError(e instanceof Error ? e.message : String(e))
    }
  }, [deleteTarget, refreshParent, reportError, workspaceId])

  const Node = useCallback(
    ({ node, style, dragHandle }: NodeRendererProps<ExplorerNode>) => {
      const isDir = node.data.isDir
      const isOpen = node.isOpen
      const isSelected = node.isSelected || selectedPath === node.data.path
      const renaming =
        inline?.type === 'rename' && inline.path === node.data.path

      return (
        <div
          ref={dragHandle}
          style={style as CSSProperties}
          className={`group flex h-full items-center gap-1 pr-1 text-[12px] leading-none ${
            isSelected
              ? 'bg-shell-active text-shell-text'
              : 'text-shell-muted hover:bg-shell-border/25 hover:text-shell-text'
          } ${
            clipboard?.mode === 'cut' && clipboard.path === node.data.path
              ? 'opacity-50'
              : ''
          }`}
          data-testid={isDir ? `tree-dir-${node.data.path}` : `tree-file-${node.data.path}`}
          data-path={node.data.path}
          data-selected={isSelected ? 'true' : 'false'}
          data-cut={
            clipboard?.mode === 'cut' && clipboard.path === node.data.path
              ? 'true'
              : undefined
          }
          data-expanded={isDir ? (isOpen ? 'true' : 'false') : undefined}
          onContextMenu={(e) => {
            openMenu(e, isDir ? 'folder' : 'file', node.data.path, node.data.name, isDir)
          }}
          onClick={(e) => {
            e.stopPropagation()
            if (renaming) return
            if (isDir) {
              node.toggle()
              if (!node.data.loaded && !node.isOpen) {
                void loadFolder(node.data.path)
              }
              return
            }
            node.select()
            if (clickTimerRef.current) clearTimeout(clickTimerRef.current)
            const path = node.data.path
            clickTimerRef.current = setTimeout(() => {
              clickTimerRef.current = null
              callbacksRef.current.onSelectFile(path)
            }, 220)
          }}
          onDoubleClick={(e) => {
            e.stopPropagation()
            e.preventDefault()
            if (isDir || renaming) return
            if (clickTimerRef.current) {
              clearTimeout(clickTimerRef.current)
              clickTimerRef.current = null
            }
            node.select()
            if (callbacksRef.current.onOpenInNewTab) {
              callbacksRef.current.onOpenInNewTab(node.data.path)
            } else {
              callbacksRef.current.onSelectFile(node.data.path)
            }
          }}
        >
          {isDir ? (
            <span className="flex w-3.5 shrink-0 items-center justify-center text-shell-muted">
              {isOpen ? (
                <ChevronDown size={12} aria-hidden />
              ) : (
                <ChevronRight size={12} aria-hidden />
              )}
            </span>
          ) : (
            <span className="w-3.5 shrink-0" aria-hidden />
          )}

          {fileIcon(node.data.name, isDir, isOpen)}

          {renaming ? (
            <div className="min-w-0 flex-1 pl-0.5">
              <InlineNameInput
                initialValue={node.data.name}
                testId={`rename-input-${node.data.path}`}
                onSubmit={(n) => void submitInline(n)}
                onCancel={() => setInline(null)}
              />
            </div>
          ) : (
            <span className="min-w-0 flex-1 truncate pl-0.5" title={node.data.path}>
              {node.data.name}
            </span>
          )}

          {!isDir && !renaming && callbacksRef.current.onOpenToSide && (
            <button
              type="button"
              data-testid={`open-to-side-${node.data.path}`}
              title={t('explorer.openToSide')}
              aria-label={`${t('explorer.openToSide')}: ${node.data.name}`}
              onClick={(e) => {
                e.stopPropagation()
                callbacksRef.current.onOpenToSide?.(node.data.path)
              }}
              className="ml-auto shrink-0 rounded p-0.5 text-shell-muted opacity-0 hover:bg-shell-border/50 hover:text-shell-text group-hover:opacity-100 focus:opacity-100"
            >
              <Columns2 size={12} aria-hidden />
            </button>
          )}
        </div>
      )
    },
    [clipboard, inline, loadFolder, openMenu, selectedPath, submitInline, t],
  )

  const creatingHere =
    inline &&
    (inline.type === 'create-file' || inline.type === 'create-folder') &&
    inline.parent === ''

  const createUnderFolder =
    inline &&
    (inline.type === 'create-file' || inline.type === 'create-folder') &&
    inline.parent !== ''
      ? inline.parent
      : null

  return (
    <div data-testid="explorer-tree" className="flex h-full min-h-0 flex-col overflow-hidden">
      {workspaceRootLabel && (
        <div
          data-testid="explorer-filter-row"
          className="flex h-10 shrink-0 items-center border-b border-shell-border px-1.5"
        >
          <input
            data-testid="explorer-filter"
            type="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t('explorer.filterPlaceholder')}
            className="h-6 w-full rounded border border-shell-border bg-shell-bg px-1.5 text-[11px] leading-none text-shell-text outline-none placeholder:text-shell-muted/70 focus:border-shell-accent"
          />
        </div>
      )}

      {workspaceRootLabel && (
        <div
          data-testid="explorer-workspace-root"
          data-path=""
          className="flex h-10 shrink-0 items-center gap-1 border-b border-shell-border px-2 text-[11px] text-shell-text/90 hover:bg-shell-border/20"
          title={workspaceRootPath || workspaceRootLabel}
          onContextMenu={(e) =>
            openMenu(e, 'root', '', workspaceRootLabel, true)
          }
        >
          <span
            data-testid="explorer-root-label"
            className="truncate font-medium leading-none"
          >
            {workspaceRootLabel}
          </span>
          <span className="ml-auto text-[10px] leading-none text-shell-muted">root</span>
        </div>
      )}

      <div
        ref={containerRef}
        data-testid="explorer-panel-body"
        className="explorer-scroll min-h-0 min-w-0 flex-1 overflow-hidden"
        onContextMenu={(e) => {
          if (!workspaceRootLabel) return
          const t = e.target as HTMLElement | null
          if (t?.closest?.('[data-path]')) return
          openMenu(e, 'root', '', workspaceRootLabel, true)
        }}
      >
        {!workspaceRootLabel && (
          <p className="px-2 py-3 text-xs text-shell-muted">{t('explorer.openWorkspace')}</p>
        )}
        {workspaceRootLabel && loadingRoot && data.length === 0 && (
          <p className="px-2 py-3 text-xs text-shell-muted">{t('common.loading')}</p>
        )}
        {error && (
          <p className="px-2 py-2 text-xs text-red-400" data-testid="explorer-error">
            {error}
          </p>
        )}
        {creatingHere && (
          <div className="border-b border-shell-border px-2 py-1" data-testid="inline-create-root">
            <InlineNameInput
              initialValue={inline.type === 'create-file' ? 'untitled.txt' : 'New Folder'}
              testId={
                inline.type === 'create-file' ? 'new-file-input' : 'new-folder-input'
              }
              onSubmit={(n) => void submitInline(n)}
              onCancel={() => setInline(null)}
            />
          </div>
        )}
        {createUnderFolder && (
          <div
            className="border-b border-shell-border px-2 py-1 text-[11px] text-shell-muted"
            data-testid="inline-create-hint"
          >
            Creating in {createUnderFolder}/
            <InlineNameInput
              initialValue={
                inline?.type === 'create-file' ? 'untitled.txt' : 'New Folder'
              }
              testId={
                inline?.type === 'create-file' ? 'new-file-input' : 'new-folder-input'
              }
              onSubmit={(n) => void submitInline(n)}
              onCancel={() => setInline(null)}
            />
          </div>
        )}
        {workspaceRootLabel && !loadingRoot && data.length === 0 && !error && !creatingHere && (
          <p
            className="px-2 py-3 text-xs text-shell-muted"
            data-testid="explorer-empty-hint"
            onContextMenu={(e) => {
              e.preventDefault()
              e.stopPropagation()
              openMenu(e, 'root', '', workspaceRootLabel, true)
            }}
          >
            {t('explorer.emptyFolder')}
          </p>
        )}
        {workspaceRootLabel && data.length > 0 && (
          <div
            data-testid="explorer-root-list"
            className="h-full w-full min-h-0 min-w-0"
            onContextMenu={(e) => {
              const t = e.target as HTMLElement | null
              if (t?.closest?.('[data-path]')) return
              openMenu(e, 'root', '', workspaceRootLabel, true)
            }}
          >
            <Tree
              data={data}
              width={size.width}
              height={size.height}
              indent={12}
              rowHeight={22}
              overscanCount={8}
              openByDefault={false}
              disableDrag
              disableDrop
              disableEdit
              disableMultiSelection
              selection={selectedPath}
              searchTerm={search}
              searchMatch={(node, term) =>
                node.data.name.toLowerCase().includes(term.toLowerCase())
              }
              onToggle={onToggle}
              onActivate={(node: NodeApi<ExplorerNode>) => {
                if (!node.data.isDir) {
                  callbacksRef.current.onSelectFile(node.data.path)
                }
              }}
              className="explorer-arborist"
            >
              {Node}
            </Tree>
          </div>
        )}
      </div>

      {menu && (
        <ExplorerContextMenu
          x={menu.x}
          y={menu.y}
          items={menu.items}
          onAction={(id) => void runMenuAction(id)}
          onClose={() => setMenu(null)}
        />
      )}

      {findFolder && workspaceId && (
        <FindInFolderPanel
          workspaceId={workspaceId}
          folder={findFolder.folder}
          folderLabel={findFolder.label}
          onOpenResult={(p) => {
            callbacksRef.current.onSelectFile(p)
          }}
          onClose={() => setFindFolder(null)}
        />
      )}

      {deleteTarget && (
        <ConfirmDialog
          title={
            deleteTarget.isDir
              ? t('explorer.deleteFolder')
              : t('explorer.deleteFile')
          }
          message={
            deleteTarget.isDir
              ? t('explorer.confirmDeleteFolder', { name: deleteTarget.name })
              : t('explorer.confirmDelete', {
                  name: deleteTarget.name || pathBaseName(deleteTarget.path),
                })
          }
          danger
          onConfirm={() => void confirmDelete()}
          onCancel={() => setDeleteTarget(null)}
        />
      )}
    </div>
  )
}
