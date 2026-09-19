import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import {
  applyLocaleToDocument,
  readStoredLocale,
  writeStoredLocale,
  type LocaleId,
} from './locale'
import type { MessageKey } from './messages'
import { translate } from './t'

export type LocaleContextValue = {
  locale: LocaleId
  setLocale: (id: LocaleId) => void
  t: (key: MessageKey, vars?: Record<string, string | number>) => string
}

const LocaleContext = createContext<LocaleContextValue | null>(null)

export function LocaleProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<LocaleId>(() => {
    const initial = readStoredLocale()
    applyLocaleToDocument(initial)
    return initial
  })

  useEffect(() => {
    applyLocaleToDocument(locale)
  }, [locale])

  const setLocale = useCallback((id: LocaleId) => {
    writeStoredLocale(id)
    applyLocaleToDocument(id)
    setLocaleState(id)
  }, [])

  const t = useCallback(
    (key: MessageKey, vars?: Record<string, string | number>) =>
      translate(locale, key, vars),
    [locale],
  )

  const value = useMemo(
    () => ({ locale, setLocale, t }),
    [locale, setLocale, t],
  )

  return (
    <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>
  )
}

export function useLocale(): LocaleContextValue {
  const ctx = useContext(LocaleContext)
  if (!ctx) {
    const locale = readStoredLocale()
    return {
      locale,
      setLocale: (id: LocaleId) => {
        writeStoredLocale(id)
        applyLocaleToDocument(id)
      },
      t: (key, vars) => translate(locale, key, vars),
    }
  }
  return ctx
}
