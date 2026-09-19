import React from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import { App } from './App'
import { LocaleProvider } from './features/i18n/LocaleProvider'
import {
  applyLocaleToDocument,
  readStoredLocale,
} from './features/i18n/locale'
import { ThemeProvider } from './features/theme/ThemeProvider'
import {
  applyThemeToDocument,
  readStoredTheme,
} from './features/theme/theme'

applyThemeToDocument(readStoredTheme())
applyLocaleToDocument(readStoredLocale())

const container = document.getElementById('root')
if (!container) {
  throw new Error('root element missing')
}
createRoot(container).render(
  <React.StrictMode>
    <ThemeProvider>
      <LocaleProvider>
        <App />
      </LocaleProvider>
    </ThemeProvider>
  </React.StrictMode>,
)
