import { formatShortcut } from '../../lib/platform'
import { en, type MessageKey } from '../i18n/messages'

export type CommandId =
  | 'save'
  | 'save-all'
  | 'close-tab'
  | 'new-file'
  | 'open-file'
  | 'toggle-terminal'
  | 'toggle-explorer'
  | 'focus-explorer'
  | 'close-secondary'
  | 'show-command-palette'
  | 'open-docs'

export type PaletteCommand = {
  id: CommandId
  label: string
  keywords?: string
  shortcut?: string
}

type PaletteCommandDef = {
  id: CommandId
  labelKey: MessageKey
  keywords?: string
  shortcut?: () => string | undefined
}

const PALETTE_COMMAND_DEFS: PaletteCommandDef[] = [
  {
    id: 'save',
    labelKey: 'palette.save',
    keywords: 'write disk persist',
    shortcut: () => formatShortcut('S'),
  },
  {
    id: 'save-all',
    labelKey: 'palette.saveAll',
    keywords: 'write disk all buffers',
    shortcut: () => formatShortcut('S', { alt: true }),
  },
  {
    id: 'close-tab',
    labelKey: 'palette.closeTab',
    keywords: 'close editor tab',
    shortcut: () => formatShortcut('W'),
  },
  {
    id: 'new-file',
    labelKey: 'palette.newFile',
    keywords: 'create file document',
  },
  {
    id: 'open-file',
    labelKey: 'palette.openFile',
    keywords: 'open path focus explorer',
  },
  {
    id: 'toggle-terminal',
    labelKey: 'palette.toggleTerminal',
    keywords: 'pty shell integrated panel',
    shortcut: () => formatShortcut('`', { forceCtrl: true }),
  },
  {
    id: 'toggle-explorer',
    labelKey: 'palette.toggleExplorer',
    keywords: 'sidebar files tree',
  },
  {
    id: 'focus-explorer',
    labelKey: 'palette.focusExplorer',
    keywords: 'files tree navigate',
  },
  {
    id: 'close-secondary',
    labelKey: 'palette.closeSecondary',
    keywords: 'close secondary side column',
  },
  {
    id: 'open-docs',
    labelKey: 'palette.openDocs',
    keywords: 'docs help guide documentation how-to case study',
  },
]

export function getPaletteCommands(
  t: (key: MessageKey) => string,
): PaletteCommand[] {
  return PALETTE_COMMAND_DEFS.map((d) => ({
    id: d.id,
    label: t(d.labelKey),
    keywords: d.keywords,
    shortcut: d.shortcut?.(),
  }))
}

export const PALETTE_COMMANDS: PaletteCommand[] = getPaletteCommands(
  (key) => en[key],
)

function normalize(s: string): string {
  return s.trim().toLowerCase()
}

export function filterCommands(
  query: string,
  commands: PaletteCommand[] = PALETTE_COMMANDS,
): PaletteCommand[] {
  const q = normalize(query)
  if (!q) return commands
  return commands.filter((c) => {
    const hay = normalize(`${c.id} ${c.label} ${c.keywords || ''}`)
    return hay.includes(q)
  })
}

export function isModKey(e: { metaKey: boolean; ctrlKey: boolean }): boolean {
  return e.metaKey || e.ctrlKey
}

export type ShortcutAction =
  | 'save'
  | 'close-tab'
  | 'command-palette'
  | 'quick-open'
  | 'save-all'
  | null

export function resolveShortcut(e: {
  key: string
  metaKey: boolean
  ctrlKey: boolean
  shiftKey: boolean
  altKey: boolean
}): ShortcutAction {
  if (!isModKey(e)) return null
  const key = e.key.length === 1 ? e.key.toLowerCase() : e.key
  if ((key === 'p' || key === 'P') && e.shiftKey) return 'command-palette'
  if (key === 'p' || key === 'P') return 'quick-open'
  if ((key === 's' || key === 'S') && e.altKey) return 'save-all'
  if (key === 's' || key === 'S') return 'save'
  if (key === 'w' || key === 'W') return 'close-tab'
  return null
}
