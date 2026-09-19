import { isExcalidrawPath } from '../canvas/excalidrawDoc'
import { languageFromPath } from './languageFromPath'

export type EditorViewMode = 'edit' | 'preview'

export { isExcalidrawPath }

export function supportsExcalidrawCanvas(path: string): boolean {
  return isExcalidrawPath(path)
}

export function supportsMarkdownPreview(path: string): boolean {
  const lang = languageFromPath(path)
  return lang === 'markdown'
}

export function isRenderablePreviewPath(path: string): boolean {
  if (supportsMarkdownPreview(path)) return true
  if (supportsExcalidrawCanvas(path)) return true
  const base = (path.split(/[/\\]/).pop() || path).toLowerCase()
  if (base.endsWith('.mmd') || base.endsWith('.mermaid')) return true
  if (/\.(png|jpe?g|gif|webp|svg|bmp|ico)$/i.test(base)) return true
  return false
}

export function defaultViewMode(path: string): EditorViewMode {
  if (supportsExcalidrawCanvas(path)) return 'preview'
  if (isRenderablePreviewPath(path) && !supportsMarkdownPreview(path)) {
    const base = (path.split(/[/\\]/).pop() || path).toLowerCase()
    if (/\.(png|jpe?g|gif|webp|svg|bmp|ico)$/i.test(base)) return 'preview'
  }
  return 'edit'
}

export function cycleMode(mode: EditorViewMode, allowPreview: boolean): EditorViewMode {
  if (!allowPreview) return 'edit'
  return mode === 'edit' ? 'preview' : 'edit'
}
