import { revealInOSLabel } from '../../lib/platform'

export type ContextTargetKind = 'root' | 'folder' | 'file'

export type ContextMenuActionId =
  | 'new-file'
  | 'new-folder'
  | 'add-folder-to-workspace'
  | 'remove-folder-from-workspace'
  | 'find-in-folder'
  | 'copy-path'
  | 'copy-relative-path'
  | 'cut'
  | 'copy'
  | 'paste'
  | 'reveal-in-os'
  | 'rename'
  | 'delete'
  | 'open-with-edit'
  | 'open-with-preview'
  | 'open-preview'
  | 'open-to-side'
  | 'open-in-terminal'
  | 'separator'

export type ContextMenuItem = {
  id: ContextMenuActionId
  label: string
  disabled?: boolean
  danger?: boolean
  /** Nested Open With submenu; only present for multi-mode files. */
  children?: ContextMenuItem[]
}

export type BuildMenuInput = {
  kind: ContextTargetKind
  /** Workspace-relative path (empty for virtual root). */
  path: string
  /** Basename for display (optional). */
  name?: string
  /** True when file supports Markdown (or other) preview mode. */
  hasMultipleOpenModes?: boolean
  /** Open-with mode labels when multiple modes exist. */
  openModes?: { id: 'open-with-edit' | 'open-with-preview'; label: string }[]
  /**
   * True when the file is renderable (Markdown/Mermaid/image) and should show
   * "Open Preview" (VAL-IDE-026). Distinct from Open With multi-mode.
   */
  canOpenPreview?: boolean
  revealLabel?: string
  canPaste?: boolean
}

const sep = (): ContextMenuItem => ({ id: 'separator', label: '' })

/**
 * Build Pack A menu items for a tree target.
 * Open With is omitted when only a single editor mode exists (VAL-IDE-029).
 */
export function buildContextMenuItems(input: BuildMenuInput): ContextMenuItem[] {
  const reveal = input.revealLabel || revealInOSLabel()

  const items: ContextMenuItem[] = []

  if (input.kind === 'root' || input.kind === 'folder') {
    items.push(
      { id: 'new-file', label: 'New File' },
      { id: 'new-folder', label: 'New Folder' },
    )
  }

  if (input.kind === 'folder') {
    items.push({ id: 'add-folder-to-workspace', label: 'Add Folder to Workspace' })
  }

  if (input.kind === 'root') {
    items.push({
      id: 'remove-folder-from-workspace',
      label: 'Remove from Workspace',
    })
  }

  // Find in Folder (VAL-IDE-008) + Open in Integrated Terminal (VAL-IDE-012).
  if (input.kind === 'root' || input.kind === 'folder') {
    items.push({ id: 'find-in-folder', label: 'Find in Folder' })
    items.push({
      id: 'open-in-terminal',
      label: 'Open in Integrated Terminal',
    })
  }

  if (items.length) items.push(sep())

  if (input.kind === 'file') {
    items.push({ id: 'open-to-side', label: 'Open to the Side' })
    if (input.canOpenPreview) {
      items.push({ id: 'open-preview', label: 'Open Preview' })
    }
    if (input.hasMultipleOpenModes) {
      const modes =
        input.openModes && input.openModes.length > 0
          ? input.openModes
          : [
              { id: 'open-with-edit' as const, label: 'Text' },
              { id: 'open-with-preview' as const, label: 'Preview' },
            ]
      items.push({
        id: 'open-with-edit',
        label: 'Open With…',
        children: modes.map((m) => ({ id: m.id, label: m.label })),
      })
    }
    items.push(sep())
  }

  // Tree clipboard cut/copy/paste (VAL-IDE-013).
  const clipboardItems: ContextMenuItem[] = []
  if (input.kind !== 'root') {
    clipboardItems.push(
      { id: 'cut', label: 'Cut' },
      { id: 'copy', label: 'Copy' },
    )
  }
  if (input.kind === 'root' || input.kind === 'folder') {
    clipboardItems.push({
      id: 'paste',
      label: 'Paste',
      disabled: !input.canPaste,
    })
  }
  if (clipboardItems.length) {
    items.push(...clipboardItems)
    items.push(sep())
  }

  items.push(
    { id: 'copy-path', label: 'Copy Path' },
    { id: 'copy-relative-path', label: 'Copy Relative Path' },
    { id: 'reveal-in-os', label: reveal },
  )

  if (input.kind !== 'root') {
    items.push(sep())
    items.push({ id: 'rename', label: 'Rename' })
    items.push({ id: 'delete', label: 'Delete', danger: true })
  }

  return items
}

/** Auto-rename preference for pure UI preview (mirrors Go uniqueDestName). */
export function suggestCopyName(preferred: string, existing: Set<string>): string {
  if (!existing.has(preferred)) return preferred
  const i = preferred.lastIndexOf('.')
  const hasExt = i > 0
  const base = hasExt ? preferred.slice(0, i) : preferred
  const ext = hasExt ? preferred.slice(i) : ''
  let candidate = `${base} copy${ext}`
  if (!existing.has(candidate)) return candidate
  for (let n = 2; n < 10_000; n++) {
    candidate = `${base} copy ${n}${ext}`
    if (!existing.has(candidate)) return candidate
  }
  return `${base} copy ${Date.now()}${ext}`
}

/** Parent directory (slash path) for a relative path; empty string = workspace root. */
export function parentRel(path: string): string {
  const p = path.replace(/\\/g, '/').replace(/\/+$/, '')
  const i = p.lastIndexOf('/')
  if (i < 0) return ''
  return p.slice(0, i)
}

/** Basename of a slash path. */
export function baseName(path: string): string {
  const p = path.replace(/\\/g, '/').replace(/\/+$/, '')
  const i = p.lastIndexOf('/')
  return i < 0 ? p : p.slice(i + 1)
}

/** Join parent + name with forward slashes. */
export function joinRel(parent: string, name: string): string {
  const p = parent.replace(/\\/g, '/').replace(/\/+$/, '')
  if (!p) return name
  return `${p}/${name}`
}
