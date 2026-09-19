import { buildLiveBlockSrcDoc } from '../chat/liveBlocks'

export type PreviewSourceMode = 'url' | 'file' | 'html' | 'diagram' | 'image'

export type WebPreviewSeed = {
  mode: PreviewSourceMode
  html?: string
  diagramSource?: string
  imageUrl?: string
  imagePrompt?: string
  imageCaption?: string
  path?: string
  url?: string
  title?: string
  prefer?: 'edit' | 'new'
  nonce?: number
  chatMessageId?: string
  sourceHtml?: string
  scopeId?: string
  canvasId?: string
}

export function isPreviewableHtmlLanguage(lang: string | null | undefined): boolean {
  if (!lang) return false
  const l = lang.trim().toLowerCase()
  return (
    l === 'html' ||
    l === 'htm' ||
    l === 'html:preview' ||
    l === 'live-block' ||
    l === 'yura-live' ||
    l.startsWith('html:')
  )
}

export function looksLikeHtmlDocument(text: string): boolean {
  const head = (text || '').slice(0, 800).trimStart().toLowerCase()
  if (!head) return false
  return (
    head.startsWith('<!doctype') ||
    head.startsWith('<html') ||
    head.includes('<body') ||
    (head.startsWith('<') && (head.includes('<div') || head.includes('<section') || head.includes('<main')))
  )
}

export function normalizePreviewUrl(raw: string): string | null {
  const s = (raw || '').trim()
  if (!s) return null
  const lower = s.toLowerCase()
  if (lower.startsWith('javascript:') || lower.startsWith('data:')) {
    return null
  }
  const absScheme = /^([a-z][a-z0-9+.-]*):\/\//i.exec(s)
  if (absScheme) {
    const scheme = absScheme[1].toLowerCase()
    if (scheme !== 'http' && scheme !== 'https') return null
  }
  const candidate =
    lower.startsWith('http://') || lower.startsWith('https://') ? s : `http://${s}`
  try {
    const u = new URL(candidate)
    if (u.protocol !== 'http:' && u.protocol !== 'https:') return null
    return u.toString()
  } catch {
    return null
  }
}

export function isFullHtmlDocument(html: string): boolean {
  const head = (html || '').slice(0, 512).trimStart().toLowerCase()
  return (
    head.startsWith('<!doctype') ||
    head.startsWith('<html') ||
    head.startsWith('<!DOCTYPE'.toLowerCase())
  )
}

export function extractHtmlDocument(text: string): string | null {
  const raw = (text || '').trim()
  if (!raw) return null
  const fence = /```(?:html|htm)?\s*\n([\s\S]*?)```/i.exec(raw)
  if (fence?.[1]) {
    const body = fence[1].trim()
    if (isFullHtmlDocument(body) || looksLikeHtmlDocument(body)) return body
  }
  if (isFullHtmlDocument(raw) || looksLikeHtmlDocument(raw)) return raw
  const idx = raw.search(/<!DOCTYPE\s+html|<html[\s>]/i)
  if (idx >= 0) {
    const slice = raw.slice(idx).trim()
    if (isFullHtmlDocument(slice) || looksLikeHtmlDocument(slice)) return slice
  }
  return null
}

export function buildPreviewSrcDoc(html: string): string {
  const body = typeof html === 'string' ? html : ''
  if (isFullHtmlDocument(body)) {
    return body
  }
  return buildLiveBlockSrcDoc(body)
}

export const DEFAULT_HTML_CANDIDATES = [
  'index.html',
  'index.htm',
  'preview.html',
  'public/index.html',
  'dist/index.html',
] as const

export function pickDefaultHtmlPath(names: string[]): string | null {
  const set = new Set(names.map((n) => n.replace(/\\/g, '/')))
  for (const c of DEFAULT_HTML_CANDIDATES) {
    if (set.has(c)) return c
  }
  for (const n of set) {
    if (n.endsWith('.html') || n.endsWith('.htm')) return n
  }
  return null
}

export function iframeSandboxForMode(mode: PreviewSourceMode): string {
  if (mode === 'url') {
    return 'allow-scripts allow-forms allow-modals allow-popups allow-same-origin'
  }
  return 'allow-scripts allow-forms allow-modals'
}
