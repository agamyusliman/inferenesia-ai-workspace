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
  applyThemeToDocument,
  nextThemeId,
  readStoredTheme,
  writeStoredTheme,
  type ThemeId,
} from './theme'

export type ThemeContextValue = {
  theme: ThemeId
  setTheme: (t: ThemeId) => void
  cycleTheme: () => void
}

const ThemeContext = createContext<ThemeContextValue | null>(null)

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<ThemeId>(() => {
    const initial = readStoredTheme()
    applyThemeToDocument(initial)
    return initial
  })

  useEffect(() => {
    applyThemeToDocument(theme)
  }, [theme])

  const setTheme = useCallback((t: ThemeId) => {
    writeStoredTheme(t)
    applyThemeToDocument(t)
    setThemeState(t)
  }, [])

  const cycleTheme = useCallback(() => {
    setThemeState((cur) => {
      const next = nextThemeId(cur)
      writeStoredTheme(next)
      applyThemeToDocument(next)
      return next
    })
  }, [])

  const value = useMemo(
    () => ({ theme, setTheme, cycleTheme }),
    [theme, setTheme, cycleTheme],
  )

  return (
    <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
  )
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext)
  if (!ctx) {
    const theme = readStoredTheme()
    return {
      theme,
      setTheme: (t: ThemeId) => {
        writeStoredTheme(t)
        applyThemeToDocument(t)
      },
      cycleTheme: () => {
        const next = nextThemeId(theme)
        writeStoredTheme(next)
        applyThemeToDocument(next)
      },
    }
  }
  return ctx
}
