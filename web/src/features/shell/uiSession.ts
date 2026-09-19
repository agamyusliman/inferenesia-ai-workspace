import type { NavId } from './Sidebar'

const STORAGE_KEY = 'inferenesia-ui-session'
const STORAGE_KEY_TYPO = 'infernesia-ui-session'
const STORAGE_KEY_LEGACY = 'yura-ai-ui-session'

export type UiSessionSnapshot = {
  nav?: NavId
  chatBind?: 'hub' | 'none'
  chatVisible?: boolean
  explorerVisible?: boolean
  terminalOpen?: boolean
  workspaceId?: string
  openPaths?: string[]
  activePath?: string | null
  gallerySelectedId?: string | null
}

const NAV_IDS = new Set<string>([
  'workspaces',
  'sessions',
  'canvas',
  'files',
  'history',
  'git',
  'chat',
  'settings',
  'docs',
  'preview',
  'todos',
  'missions',
  'tasks',
  'browser',
])

function isNavId(v: unknown): v is NavId {
  return typeof v === 'string' && NAV_IDS.has(v)
}

export function readUiSession(): UiSessionSnapshot | null {
  if (typeof window === 'undefined' || !window.localStorage) return null
  try {
    const raw =
      window.localStorage.getItem(STORAGE_KEY) ??
      window.localStorage.getItem(STORAGE_KEY_TYPO) ??
      window.localStorage.getItem(STORAGE_KEY_LEGACY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as UiSessionSnapshot
    if (!parsed || typeof parsed !== 'object') return null
    const out: UiSessionSnapshot = {}
    if (isNavId(parsed.nav)) out.nav = parsed.nav
    if (parsed.chatBind === 'hub' || parsed.chatBind === 'none') {
      out.chatBind = parsed.chatBind
    }
    if (typeof parsed.chatVisible === 'boolean') out.chatVisible = parsed.chatVisible
    if (typeof parsed.explorerVisible === 'boolean') {
      out.explorerVisible = parsed.explorerVisible
    }
    if (typeof parsed.terminalOpen === 'boolean') out.terminalOpen = parsed.terminalOpen
    if (typeof parsed.workspaceId === 'string') out.workspaceId = parsed.workspaceId
    if (Array.isArray(parsed.openPaths)) {
      out.openPaths = parsed.openPaths.filter((p) => typeof p === 'string' && p.length > 0)
    }
    if (parsed.activePath === null || typeof parsed.activePath === 'string') {
      out.activePath = parsed.activePath
    }
    if (
      parsed.gallerySelectedId === null ||
      typeof parsed.gallerySelectedId === 'string'
    ) {
      out.gallerySelectedId = parsed.gallerySelectedId
    }
    return out
  } catch {
    return null
  }
}

export function writeUiSession(snap: UiSessionSnapshot): void {
  if (typeof window === 'undefined' || !window.localStorage) return
  try {
    const prev = readUiSession() || {}
    const next: UiSessionSnapshot = { ...prev, ...snap }
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
  } catch {
    void 0
  }
}

export function initialNavFromSession(): NavId {
  const s = readUiSession()
  if (s?.nav && s.nav !== 'preview') return s.nav
  return 'workspaces'
}

export function initialChatBindFromSession(): 'hub' | 'none' {
  const s = readUiSession()
  return s?.chatBind === 'none' ? 'none' : 'hub'
}
