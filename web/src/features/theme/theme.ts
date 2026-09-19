export type ThemeId = 'warm' | 'dark' | 'light'

export const THEME_STORAGE_KEY = 'inferenesia-theme'
export const THEME_STORAGE_KEY_TYPO = 'infernesia-theme'
export const THEME_STORAGE_KEY_LEGACY = 'yura-ai-theme'

export const THEME_IDS: ThemeId[] = ['warm', 'dark', 'light']

const DEFAULT_THEME: ThemeId = 'dark'

export function parseThemeId(raw: string | null | undefined): ThemeId {
  if (raw == null) return DEFAULT_THEME
  const v = String(raw).trim().toLowerCase()
  if (v === 'warm' || v === 'dark' || v === 'light') return v
  return DEFAULT_THEME
}

export function themeLabel(id: ThemeId): string {
  switch (id) {
    case 'warm':
      return 'Warm'
    case 'dark':
      return 'Dark'
    case 'light':
      return 'Light'
    default: {
      const _exhaustive: never = id
      return _exhaustive
    }
  }
}

export function readStoredTheme(): ThemeId {
  if (typeof window === 'undefined' || !window.localStorage) {
    return DEFAULT_THEME
  }
  try {
    const next = window.localStorage.getItem(THEME_STORAGE_KEY)
    if (next != null) return parseThemeId(next)
    const typo = window.localStorage.getItem(THEME_STORAGE_KEY_TYPO)
    if (typo != null) return parseThemeId(typo)
    const legacy = window.localStorage.getItem(THEME_STORAGE_KEY_LEGACY)
    if (legacy != null) return parseThemeId(legacy)
    return DEFAULT_THEME
  } catch {
    return DEFAULT_THEME
  }
}

export function writeStoredTheme(id: ThemeId): void {
  if (typeof window === 'undefined' || !window.localStorage) return
  try {
    window.localStorage.setItem(THEME_STORAGE_KEY, id)
  } catch {
    /* storage unavailable */
  }
}

export function applyThemeToDocument(id: ThemeId): void {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  root.dataset.theme = id
  root.setAttribute('data-theme', id)
}

export function canvasThemeFor(id: ThemeId): 'light' | 'dark' {
  return id === 'light' ? 'light' : 'dark'
}

export function monacoThemeFor(id: ThemeId): 'vs-dark' | 'light' {
  return id === 'light' ? 'light' : 'vs-dark'
}

export function nextThemeId(current: ThemeId): ThemeId {
  const i = THEME_IDS.indexOf(current)
  if (i < 0) return DEFAULT_THEME
  return THEME_IDS[(i + 1) % THEME_IDS.length]!
}
