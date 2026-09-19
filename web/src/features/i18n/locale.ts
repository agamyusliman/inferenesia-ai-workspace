export type LocaleId = 'en' | 'id'

export const LOCALE_STORAGE_KEY = 'inferenesia-locale'
export const LOCALE_STORAGE_KEY_TYPO = 'infernesia-locale'
export const LOCALE_STORAGE_KEY_LEGACY = 'yura-ai-locale'

export const LOCALE_IDS: LocaleId[] = ['en', 'id']

const DEFAULT_LOCALE: LocaleId = 'en'

export function parseLocaleId(raw: string | null | undefined): LocaleId {
  if (raw == null) return DEFAULT_LOCALE
  const v = String(raw).trim().toLowerCase()
  if (v === 'en' || v === 'id') return v
  if (v === 'en-us' || v === 'en-gb' || v === 'english') return 'en'
  if (v === 'id-id' || v === 'in' || v === 'indonesian' || v === 'indonesia') return 'id'
  return DEFAULT_LOCALE
}

export function localeLabel(id: LocaleId): string {
  switch (id) {
    case 'en':
      return 'English'
    case 'id':
      return 'Indonesia'
    default: {
      const _exhaustive: never = id
      return _exhaustive
    }
  }
}

export function readStoredLocale(): LocaleId {
  if (typeof window === 'undefined' || !window.localStorage) {
    return DEFAULT_LOCALE
  }
  try {
    const next = window.localStorage.getItem(LOCALE_STORAGE_KEY)
    if (next != null) return parseLocaleId(next)
    const typo = window.localStorage.getItem(LOCALE_STORAGE_KEY_TYPO)
    if (typo != null) return parseLocaleId(typo)
    const legacy = window.localStorage.getItem(LOCALE_STORAGE_KEY_LEGACY)
    if (legacy != null) return parseLocaleId(legacy)
    return DEFAULT_LOCALE
  } catch {
    return DEFAULT_LOCALE
  }
}

export function writeStoredLocale(id: LocaleId): void {
  if (typeof window === 'undefined' || !window.localStorage) return
  try {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, id)
  } catch {
    /* storage unavailable */
  }
}

export function applyLocaleToDocument(id: LocaleId): void {
  if (typeof document === 'undefined') return
  document.documentElement.lang = id
}
