import { isMermaidFence } from '../chat/messageChrome'
import {
  applyCanvasSeed,
  clearCanvases,
  htmlFingerprint,
  loadCanvases,
  newCanvasId,
  saveCanvases,
  storageSaveError,
  type CanvasEntry,
  type CanvasSaveResult,
} from '../web-preview/canvasStore'
import {
  DEFAULT_IMAGE_STUDIO_OPTIONS,
  IMAGE_STUDIO_HISTORY_MAX,
  isImageStudioAspect,
  type GalleryCanvasItem,
  type ImageStudioHistoryItem,
  type ImageStudioOptions,
} from './galleryTypes'
import { emptyHtmlDeck } from './slides/htmlDeck'
import { BLANK_MARKDOWN_DOC } from './markdownDoc'
import { emptyTableDoc, serializeTableDoc } from './tableDoc'
import { emptyTimelineDoc, serializeTimelineDoc } from './timelineDoc'

const HTML_KEY_PREFIX = 'inferenesia-html-canvases:'
const DIAGRAM_KEY_PREFIX = 'inferenesia-diagram-canvases:'
const DIAGRAM_KEY_PREFIX_TYPO = 'infernesia-diagram-canvases:'
const IMAGE_KEY_PREFIX = 'inferenesia-image-canvases:'
const IMAGE_KEY_PREFIX_TYPO = 'infernesia-image-canvases:'
const EXCALIDRAW_KEY_PREFIX = 'inferenesia-excalidraw-canvases:'
const SLIDES_KEY_PREFIX = 'inferenesia-slides-canvases:'
const MARKDOWN_KEY_PREFIX = 'inferenesia-markdown-canvases:'
const TABLE_KEY_PREFIX = 'inferenesia-table-canvases:'
const TIMELINE_KEY_PREFIX = 'inferenesia-timeline-canvases:'

export type SessionLabel = { id: string; name: string }

export type DiagramEntry = {
  id: string
  title: string
  source: string
  createdAt: number
  updatedAt: number
  chatMessageId?: string
}

export type ImageEntry = {
  id: string
  title: string
  prompt: string
  agentPrompt: string
  imageUrl: string
  caption: string
  options: ImageStudioOptions
  history: ImageStudioHistoryItem[]
  createdAt: number
  updatedAt: number
  chatMessageId?: string
}

export type ExcalidrawEntry = {
  id: string
  title: string
  content: string
  createdAt: number
  updatedAt: number
  chatMessageId?: string
}

export type SlidesEntry = {
  id: string
  title: string
  content: string
  createdAt: number
  updatedAt: number
  chatMessageId?: string
}

/**
 * Markdown / Table / Timeline all persist one text payload per entry, so they
 * share a storage shape instead of three near-identical copies.
 */
export type DocEntry = {
  id: string
  title: string
  content: string
  createdAt: number
  updatedAt: number
  chatMessageId?: string
}

export type DocKind = 'markdown' | 'table' | 'timeline'

export const DOC_KINDS: DocKind[] = ['markdown', 'table', 'timeline']

const DOC_KEY_PREFIX: Record<DocKind, string> = {
  markdown: MARKDOWN_KEY_PREFIX,
  table: TABLE_KEY_PREFIX,
  timeline: TIMELINE_KEY_PREFIX,
}

const DOC_ID_PREFIX: Record<DocKind, string> = {
  markdown: 'md',
  table: 'tb',
  timeline: 'tl',
}

const DOC_FALLBACK_TITLE: Record<DocKind, string> = {
  markdown: 'Document',
  table: 'Table',
  timeline: 'Timeline',
}

const DOC_LIMIT = 80

function docStorageKey(kind: DocKind, sessionId: string): string {
  return DOC_KEY_PREFIX[kind] + (sessionId || 'global')
}

function safeParseDocs(kind: DocKind, raw: string | null): DocEntry[] {
  if (!raw) return []
  try {
    const v = JSON.parse(raw) as unknown
    if (!Array.isArray(v)) return []
    return v
      .filter((x) => x && typeof x === 'object')
      .map((x) => {
        const o = x as Record<string, unknown>
        return {
          id: String(o.id || ''),
          title: String(o.title || DOC_FALLBACK_TITLE[kind]),
          content: String(o.content || ''),
          createdAt: Number(o.createdAt) || 0,
          updatedAt: Number(o.updatedAt) || Number(o.createdAt) || 0,
          chatMessageId:
            typeof o.chatMessageId === 'string' ? o.chatMessageId : undefined,
        }
      })
      .filter((d) => d.id)
  } catch {
    return []
  }
}

export function loadDocs(kind: DocKind, sessionId: string): DocEntry[] {
  if (typeof window === 'undefined' || !window.localStorage) return []
  try {
    return safeParseDocs(
      kind,
      window.localStorage.getItem(docStorageKey(kind, sessionId)),
    )
  } catch {
    return []
  }
}

export type DocSaveResult = { ok: boolean; error?: string }

export function saveDocs(
  kind: DocKind,
  sessionId: string,
  list: DocEntry[],
): DocSaveResult {
  if (typeof window === 'undefined' || !window.localStorage) {
    return { ok: false, error: 'No localStorage' }
  }
  const trimmed = list
    .slice()
    .sort((a, b) => b.updatedAt - a.updatedAt)
    .slice(0, DOC_LIMIT)
  try {
    window.localStorage.setItem(
      docStorageKey(kind, sessionId),
      JSON.stringify(trimmed),
    )
    return { ok: true }
  } catch (e) {
    return { ok: false, error: storageSaveError('this document', e) }
  }
}

export function clearDocs(kind: DocKind, sessionId: string): void {
  if (typeof window === 'undefined' || !window.localStorage) return
  try {
    window.localStorage.removeItem(docStorageKey(kind, sessionId))
  } catch {
    void 0
  }
}

export function loadDocCanvas(
  kind: DocKind,
  sessionId: string,
  docId: string,
): DocEntry | null {
  return loadDocs(kind, sessionId).find((d) => d.id === docId) || null
}

export function updateDocCanvas(
  kind: DocKind,
  sessionId: string,
  docId: string,
  patch: Partial<Pick<DocEntry, 'content' | 'title' | 'updatedAt'>>,
): (DocEntry & { saveResult?: DocSaveResult }) | null {
  const list = loadDocs(kind, sessionId)
  const idx = list.findIndex((d) => d.id === docId)
  if (idx < 0) return null
  const prev = list[idx]
  const contentChanged =
    patch.content !== undefined && patch.content !== prev.content
  const titleChanged = patch.title !== undefined && patch.title !== prev.title
  if (!contentChanged && !titleChanged && patch.updatedAt === undefined) {
    return prev
  }
  const next: DocEntry = {
    ...prev,
    ...patch,
    updatedAt:
      patch.updatedAt ??
      (contentChanged || titleChanged ? Date.now() : prev.updatedAt),
  }
  const out = list.slice()
  out[idx] = next
  const saveResult = saveDocs(kind, sessionId, out)
  return { ...next, saveResult }
}

export function deleteDocCanvas(
  kind: DocKind,
  sessionId: string,
  docId: string,
): boolean {
  const list = loadDocs(kind, sessionId)
  const next = list.filter((d) => d.id !== docId)
  if (next.length === list.length) return false
  saveDocs(kind, sessionId, next)
  return true
}

function docEntryToGallery(
  kind: DocKind,
  entry: DocEntry,
  sessionId: string,
  sessionName: string,
): GalleryCanvasItem {
  const createdAt = entry.createdAt || entry.updatedAt || 0
  return {
    id: `${sessionId}::${kind}::${entry.id}`,
    kind,
    title: entry.title || DOC_FALLBACK_TITLE[kind],
    sessionId,
    sessionName,
    updatedAt: entry.updatedAt || createdAt,
    createdAt,
    source: entry.content,
    chatMessageId: entry.chatMessageId,
  }
}

function blankDocContent(kind: DocKind, title: string): string {
  if (kind === 'markdown') {
    return title
      ? BLANK_MARKDOWN_DOC.replace('# New document', `# ${title}`)
      : BLANK_MARKDOWN_DOC
  }
  if (kind === 'table') return serializeTableDoc(emptyTableDoc())
  return serializeTimelineDoc(emptyTimelineDoc(title))
}

export function createGalleryDocCanvas(
  kind: DocKind,
  opts?: { title?: string; scopeId?: string; content?: string },
): GalleryCanvasItem {
  const sessionId = opts?.scopeId || GALLERY_GLOBAL_SCOPE
  const now = Date.now()
  const title =
    opts?.title ||
    `${DOC_FALLBACK_TITLE[kind]} ${new Date().toLocaleTimeString()}`
  const entry: DocEntry = {
    id: `${DOC_ID_PREFIX[kind]}_${now.toString(36)}_${Math.random().toString(36).slice(2, 6)}`,
    title,
    content: opts?.content || blankDocContent(kind, opts?.title || ''),
    createdAt: now,
    updatedAt: now,
  }
  const saved = saveDocs(kind, sessionId, [entry, ...loadDocs(kind, sessionId)])
  if (!saved.ok) {
    console.error(`[${kind}] create save failed`, saved.error)
  }
  return docEntryToGallery(
    kind,
    entry,
    sessionId,
    sessionId === GALLERY_GLOBAL_SCOPE ? 'Library' : sessionId,
  )
}

function parseImageHistory(raw: unknown): ImageStudioHistoryItem[] {
  if (!Array.isArray(raw)) return []
  const out: ImageStudioHistoryItem[] = []
  for (const x of raw) {
    if (!x || typeof x !== 'object') continue
    const o = x as Record<string, unknown>
    const imageUrl = String(o.imageUrl || '').trim()
    if (!imageUrl) continue
    out.push({
      id: String(o.id || `h_${out.length}`),
      imageUrl,
      prompt: String(o.prompt || ''),
      agentPrompt:
        typeof o.agentPrompt === 'string' ? o.agentPrompt : undefined,
      caption: String(o.caption || ''),
      createdAt: Number(o.createdAt) || 0,
      options: o.options ? parseImageOptions(o.options) : undefined,
    })
    if (out.length >= IMAGE_STUDIO_HISTORY_MAX) break
  }
  return out
}

function diagramStorageKey(sessionId: string): string {
  return DIAGRAM_KEY_PREFIX + (sessionId || 'global')
}

function diagramStorageKeyTypo(sessionId: string): string {
  return DIAGRAM_KEY_PREFIX_TYPO + (sessionId || 'global')
}

function safeParseDiagrams(raw: string | null): DiagramEntry[] {
  if (!raw) return []
  try {
    const v = JSON.parse(raw) as unknown
    if (!Array.isArray(v)) return []
    return v
      .filter((x) => x && typeof x === 'object')
      .map((x) => {
        const o = x as Record<string, unknown>
        return {
          id: String(o.id || ''),
          title: String(o.title || 'Diagram'),
          source: String(o.source || ''),
          createdAt: Number(o.createdAt) || 0,
          updatedAt: Number(o.updatedAt) || Number(o.createdAt) || 0,
          chatMessageId:
            typeof o.chatMessageId === 'string' ? o.chatMessageId : undefined,
        }
      })
      .filter((d) => d.id && d.source.trim())
  } catch {
    return []
  }
}

export function loadDiagrams(sessionId: string): DiagramEntry[] {
  if (typeof window === 'undefined' || !window.localStorage) return []
  try {
    const raw =
      window.localStorage.getItem(diagramStorageKey(sessionId)) ??
      window.localStorage.getItem(diagramStorageKeyTypo(sessionId))
    return safeParseDiagrams(raw)
  } catch {
    return []
  }
}

export type DiagramSaveResult = { ok: boolean; error?: string }

export function saveDiagrams(
  sessionId: string,
  list: DiagramEntry[],
): DiagramSaveResult {
  if (typeof window === 'undefined' || !window.localStorage) {
    return { ok: false, error: 'No localStorage' }
  }
  try {
    const trimmed = list
      .slice()
      .sort((a, b) => b.updatedAt - a.updatedAt)
      .slice(0, 60)
    window.localStorage.setItem(
      diagramStorageKey(sessionId),
      JSON.stringify(trimmed),
    )
    return { ok: true }
  } catch (e) {
    return { ok: false, error: storageSaveError('this diagram', e) }
  }
}

export function clearDiagrams(sessionId: string): void {
  if (typeof window === 'undefined' || !window.localStorage) return
  try {
    window.localStorage.removeItem(diagramStorageKey(sessionId))
  } catch {
    void 0
  }
}

function imageStorageKey(sessionId: string): string {
  return IMAGE_KEY_PREFIX + (sessionId || 'global')
}

function imageStorageKeyTypo(sessionId: string): string {
  return IMAGE_KEY_PREFIX_TYPO + (sessionId || 'global')
}

function parseImageOptions(raw: unknown): ImageStudioOptions {
  const base = { ...DEFAULT_IMAGE_STUDIO_OPTIONS }
  if (!raw || typeof raw !== 'object') return base
  const o = raw as Record<string, unknown>
  if (o.outputMode === 'image_only' || o.outputMode === 'image_text') {
    base.outputMode = o.outputMode
  }
  if (typeof o.temperature === 'number' && Number.isFinite(o.temperature)) {
    base.temperature = Math.min(2, Math.max(0, o.temperature))
  }
  if (isImageStudioAspect(o.aspect)) {
    base.aspect = o.aspect
  }
  if (o.thinking === 'low' || o.thinking === 'medium' || o.thinking === 'high') {
    base.thinking = o.thinking
  }
  return base
}

function safeParseImages(raw: string | null): ImageEntry[] {
  if (!raw) return []
  try {
    const v = JSON.parse(raw) as unknown
    if (!Array.isArray(v)) return []
    return v
      .filter((x) => x && typeof x === 'object')
      .map((x) => {
        const o = x as Record<string, unknown>
        return {
          id: String(o.id || ''),
          title: String(o.title || 'Image'),
          prompt: String(o.prompt || ''),
          agentPrompt: String(o.agentPrompt || ''),
          imageUrl: String(o.imageUrl || ''),
          caption: String(o.caption || ''),
          options: parseImageOptions(o.options),
          history: parseImageHistory(o.history),
          createdAt: Number(o.createdAt) || 0,
          updatedAt: Number(o.updatedAt) || Number(o.createdAt) || 0,
          chatMessageId:
            typeof o.chatMessageId === 'string' ? o.chatMessageId : undefined,
        }
      })
      .filter((d) => d.id)
  } catch {
    return []
  }
}

export function loadImages(sessionId: string): ImageEntry[] {
  if (typeof window === 'undefined' || !window.localStorage) return []
  try {
    const raw =
      window.localStorage.getItem(imageStorageKey(sessionId)) ??
      window.localStorage.getItem(imageStorageKeyTypo(sessionId))
    return safeParseImages(raw)
  } catch {
    return []
  }
}

export type ImageSaveResult = { ok: boolean; error?: string }

const IMAGE_LIMIT = 40

/**
 * Persists images verbatim. Generated pictures are data URLs and they ARE the
 * document: neither rewriting them to a placeholder nor deleting older entries
 * is an acceptable way to fit the quota. A failed write leaves previously
 * stored images untouched and reports the failure to the caller.
 */
export function saveImages(
  sessionId: string,
  list: ImageEntry[],
): ImageSaveResult {
  if (typeof window === 'undefined' || !window.localStorage) {
    return { ok: false, error: 'No localStorage' }
  }
  const trimmed = list
    .slice()
    .sort((a, b) => b.updatedAt - a.updatedAt)
    .slice(0, IMAGE_LIMIT)
  try {
    window.localStorage.setItem(
      imageStorageKey(sessionId),
      JSON.stringify(trimmed),
    )
    return { ok: true }
  } catch (e) {
    return {
      ok: false,
      error:
        'Browser storage is full — this image was not saved. Download the images you need, then delete some from the Playground.' +
        (e instanceof Error ? ` (${e.message})` : ''),
    }
  }
}

export function clearImages(sessionId: string): void {
  if (typeof window === 'undefined' || !window.localStorage) return
  try {
    window.localStorage.removeItem(imageStorageKey(sessionId))
  } catch {
    void 0
  }
}

function excalidrawStorageKey(sessionId: string): string {
  return EXCALIDRAW_KEY_PREFIX + (sessionId || 'global')
}

function safeParseExcalidraw(raw: string | null): ExcalidrawEntry[] {
  if (!raw) return []
  try {
    const v = JSON.parse(raw) as unknown
    if (!Array.isArray(v)) return []
    return v
      .filter((x) => x && typeof x === 'object')
      .map((x) => {
        const o = x as Record<string, unknown>
        return {
          id: String(o.id || ''),
          title: String(o.title || 'Whiteboard'),
          content: String(o.content || ''),
          createdAt: Number(o.createdAt) || 0,
          updatedAt: Number(o.updatedAt) || Number(o.createdAt) || 0,
          chatMessageId:
            typeof o.chatMessageId === 'string' ? o.chatMessageId : undefined,
        }
      })
      .filter((d) => d.id)
  } catch {
    return []
  }
}

export function loadExcalidraws(sessionId: string): ExcalidrawEntry[] {
  if (typeof window === 'undefined' || !window.localStorage) return []
  try {
    return safeParseExcalidraw(
      window.localStorage.getItem(excalidrawStorageKey(sessionId)),
    )
  } catch {
    return []
  }
}

export type ExcalidrawSaveResult = { ok: boolean; error?: string }

export function saveExcalidraws(
  sessionId: string,
  list: ExcalidrawEntry[],
): ExcalidrawSaveResult {
  if (typeof window === 'undefined' || !window.localStorage) {
    return { ok: false, error: 'No localStorage' }
  }
  try {
    const trimmed = list
      .slice()
      .sort((a, b) => b.updatedAt - a.updatedAt)
      .slice(0, 60)
    window.localStorage.setItem(
      excalidrawStorageKey(sessionId),
      JSON.stringify(trimmed),
    )
    return { ok: true }
  } catch (e) {
    return { ok: false, error: storageSaveError('this whiteboard', e) }
  }
}

export function clearExcalidraws(sessionId: string): void {
  if (typeof window === 'undefined' || !window.localStorage) return
  try {
    window.localStorage.removeItem(excalidrawStorageKey(sessionId))
  } catch {
    void 0
  }
}

function slidesStorageKey(sessionId: string): string {
  return SLIDES_KEY_PREFIX + (sessionId || 'global')
}

function safeParseSlides(raw: string | null): SlidesEntry[] {
  if (!raw) return []
  try {
    const v = JSON.parse(raw) as unknown
    if (!Array.isArray(v)) return []
    return v
      .filter((x) => x && typeof x === 'object')
      .map((x) => {
        const o = x as Record<string, unknown>
        return {
          id: String(o.id || ''),
          title: String(o.title || 'Deck'),
          content: String(o.content || ''),
          createdAt: Number(o.createdAt) || 0,
          updatedAt: Number(o.updatedAt) || Number(o.createdAt) || 0,
          chatMessageId:
            typeof o.chatMessageId === 'string' ? o.chatMessageId : undefined,
        }
      })
      .filter((d) => d.id)
  } catch {
    return []
  }
}

export function loadSlides(sessionId: string): SlidesEntry[] {
  if (typeof window === 'undefined' || !window.localStorage) return []
  try {
    return safeParseSlides(
      window.localStorage.getItem(slidesStorageKey(sessionId)),
    )
  } catch {
    return []
  }
}

export type SlidesSaveResult = { ok: boolean; error?: string }

const SLIDES_LIMIT = 40

/**
 * Persists decks verbatim. Embedded images are part of the slide, so a quota
 * failure neither rewrites `src="data:…"` to a placeholder nor deletes older
 * decks: the previous storage is left untouched and the failure is reported.
 */
export function saveSlides(
  sessionId: string,
  list: SlidesEntry[],
): SlidesSaveResult {
  if (typeof window === 'undefined' || !window.localStorage) {
    return { ok: false, error: 'No localStorage' }
  }
  const trimmed = list
    .slice()
    .sort((a, b) => b.updatedAt - a.updatedAt)
    .slice(0, SLIDES_LIMIT)
  try {
    window.localStorage.setItem(
      slidesStorageKey(sessionId),
      JSON.stringify(trimmed),
    )
    return { ok: true }
  } catch (e) {
    return {
      ok: false,
      error:
        'Browser storage is full — this deck was not saved. Export it now, then delete something from the Playground.' +
        (e instanceof Error ? ` (${e.message})` : ''),
    }
  }
}

export function clearSlides(sessionId: string): void {
  if (typeof window === 'undefined' || !window.localStorage) return
  try {
    window.localStorage.removeItem(slidesStorageKey(sessionId))
  } catch {
    void 0
  }
}

export function purgeOrphanCanvasStorage(liveSessionIds: string[]): {
  removedHtmlKeys: number
  removedDiagramKeys: number
  removedImageKeys: number
  removedExcalidrawKeys: number
  removedSlidesKeys: number
  removedDocKeys: number
} {
  if (typeof window === 'undefined' || !window.localStorage) {
    return {
      removedHtmlKeys: 0,
      removedDiagramKeys: 0,
      removedImageKeys: 0,
      removedExcalidrawKeys: 0,
      removedSlidesKeys: 0,
      removedDocKeys: 0,
    }
  }
  const known = liveSessionIds.filter(Boolean)
  if (known.length === 0) {
    return {
      removedHtmlKeys: 0,
      removedDiagramKeys: 0,
      removedImageKeys: 0,
      removedExcalidrawKeys: 0,
      removedSlidesKeys: 0,
      removedDocKeys: 0,
    }
  }
  const live = new Set(known)
  live.add('global')
  let removedHtmlKeys = 0
  let removedDiagramKeys = 0
  let removedImageKeys = 0
  let removedExcalidrawKeys = 0
  let removedSlidesKeys = 0
  let removedDocKeys = 0
  const toRemove: string[] = []
  try {
    for (let i = 0; i < window.localStorage.length; i += 1) {
      const key = window.localStorage.key(i)
      if (!key) continue
      if (key.startsWith(HTML_KEY_PREFIX)) {
        const sid = key.slice(HTML_KEY_PREFIX.length) || 'global'
        if (!live.has(sid)) {
          toRemove.push(key)
          removedHtmlKeys += 1
        }
      } else if (key.startsWith(DIAGRAM_KEY_PREFIX)) {
        const sid = key.slice(DIAGRAM_KEY_PREFIX.length) || 'global'
        if (!live.has(sid)) {
          toRemove.push(key)
          removedDiagramKeys += 1
        }
      } else if (key.startsWith(IMAGE_KEY_PREFIX)) {
        const sid = key.slice(IMAGE_KEY_PREFIX.length) || 'global'
        if (!live.has(sid)) {
          toRemove.push(key)
          removedImageKeys += 1
        }
      } else if (key.startsWith(EXCALIDRAW_KEY_PREFIX)) {
        const sid = key.slice(EXCALIDRAW_KEY_PREFIX.length) || 'global'
        if (!live.has(sid)) {
          toRemove.push(key)
          removedExcalidrawKeys += 1
        }
      } else if (key.startsWith(SLIDES_KEY_PREFIX)) {
        const sid = key.slice(SLIDES_KEY_PREFIX.length) || 'global'
        if (!live.has(sid)) {
          toRemove.push(key)
          removedSlidesKeys += 1
        }
      } else if (
        key.startsWith(MARKDOWN_KEY_PREFIX) ||
        key.startsWith(TABLE_KEY_PREFIX) ||
        key.startsWith(TIMELINE_KEY_PREFIX)
      ) {
        const sid = key.slice(key.indexOf(':') + 1) || 'global'
        if (!live.has(sid)) {
          toRemove.push(key)
          removedDocKeys += 1
        }
      }
    }
    for (const k of toRemove) {
      window.localStorage.removeItem(k)
    }
  } catch {
    void 0
  }
  return {
    removedHtmlKeys,
    removedDiagramKeys,
    removedImageKeys,
    removedExcalidrawKeys,
    removedSlidesKeys,
    removedDocKeys,
  }
}

export function clearAllCanvasStorageForSession(sessionId: string): void {
  clearCanvases(sessionId)
  clearDiagrams(sessionId)
  clearImages(sessionId)
  clearExcalidraws(sessionId)
  clearSlides(sessionId)
  clearDocs('markdown', sessionId)
  clearDocs('table', sessionId)
  clearDocs('timeline', sessionId)
}

export const GALLERY_GLOBAL_SCOPE = 'global'

export function removeSessionCopiesOfLibraryHtml(
  sessionIds: string[],
): number {
  if (typeof window === 'undefined' || !window.localStorage) return 0
  const lib = loadCanvases(GALLERY_GLOBAL_SCOPE)
  if (lib.length === 0) return 0
  const libFps = new Set(
    lib.map((c) => htmlFingerprint(c.html)).filter((fp) => fp.length >= 32),
  )
  if (libFps.size === 0) return 0
  let removed = 0
  for (const sid of sessionIds) {
    if (!sid || sid === GALLERY_GLOBAL_SCOPE) continue
    const list = loadCanvases(sid)
    const next = list.filter((c) => {
      const fp = htmlFingerprint(c.html)
      if (fp.length >= 32 && libFps.has(fp)) {
        removed += 1
        return false
      }
      return true
    })
    if (next.length !== list.length) {
      saveCanvases(sid, next)
    }
  }
  return removed
}

export function listGalleryCanvases(
  sessions: SessionLabel[],
): GalleryCanvasItem[] {
  if (typeof window === 'undefined' || !window.localStorage) return []

  const liveIds = new Set(sessions.map((s) => s.id))
  liveIds.add(GALLERY_GLOBAL_SCOPE)
  const nameById = new Map(sessions.map((s) => [s.id, s.name || s.id]))
  nameById.set(GALLERY_GLOBAL_SCOPE, 'Library')
  const items: GalleryCanvasItem[] = []

  removeSessionCopiesOfLibraryHtml(sessions.map((s) => s.id))

  const scopes = [
    { id: GALLERY_GLOBAL_SCOPE, name: 'Library' },
    ...sessions.map((s) => ({ id: s.id, name: s.name || s.id })),
  ]
  for (const s of scopes) {
    const sessionName = nameById.get(s.id) || s.name
    for (const c of loadCanvases(s.id)) {
      items.push(htmlEntryToGallery(c, s.id, sessionName))
    }
    for (const d of loadDiagrams(s.id)) {
      items.push(diagramEntryToGallery(d, s.id, sessionName))
    }
    for (const img of loadImages(s.id)) {
      items.push(imageEntryToGallery(img, s.id, sessionName))
    }
    for (const ex of loadExcalidraws(s.id)) {
      items.push(excalidrawEntryToGallery(ex, s.id, sessionName))
    }
    for (const deck of loadSlides(s.id)) {
      items.push(slidesEntryToGallery(deck, s.id, sessionName))
    }
    for (const kind of DOC_KINDS) {
      for (const doc of loadDocs(kind, s.id)) {
        items.push(docEntryToGallery(kind, doc, s.id, sessionName))
      }
    }
  }
  return items
    .filter((it) => liveIds.has(it.sessionId))
    .sort((a, b) => b.updatedAt - a.updatedAt)
}

const BLANK_HTML =
  '<!DOCTYPE html>\n<html lang="en">\n<head>\n  <meta charset="utf-8" />\n  <title>New playground</title>\n  <style>body{font-family:system-ui,sans-serif;padding:1.5rem;line-height:1.5}</style>\n</head>\n<body>\n  <h1>New playground</h1>\n  <p>Describe changes in the instruction bar below.</p>\n</body>\n</html>\n'

const BLANK_DIAGRAM = 'flowchart TD\n  A[Start] --> B[Edit me]\n'

const BLANK_EXCALIDRAW = `${JSON.stringify(
  {
    type: 'excalidraw',
    version: 2,
    source: 'inferenesia',
    elements: [],
    appState: {},
    files: {},
  },
  null,
  2,
)}\n`

export function createGalleryHtmlCanvas(opts?: {
  title?: string
  scopeId?: string
}): GalleryCanvasItem {
  const sessionId = opts?.scopeId || GALLERY_GLOBAL_SCOPE
  const now = Date.now()
  const id = newCanvasId()
  const entry: CanvasEntry = {
    id,
    title: opts?.title || `Canvas ${new Date().toLocaleTimeString()}`,
    html: BLANK_HTML,
    createdAt: now,
    updatedAt: now,
  }
  const list = [entry, ...loadCanvases(sessionId)]
  saveCanvases(sessionId, list)
  return htmlEntryToGallery(
    entry,
    sessionId,
    sessionId === GALLERY_GLOBAL_SCOPE ? 'Library' : sessionId,
  )
}

export function upsertGalleryHtmlFromOpen(opts: {
  html: string
  title?: string
  path?: string
  scopeId?: string
  canvasId?: string
  chatMessageId?: string
}): GalleryCanvasItem {
  const sessionId = opts.scopeId || GALLERY_GLOBAL_SCOPE
  const list = loadCanvases(sessionId)
  const applied = applyCanvasSeed(list, {
    html: opts.html,
    title: opts.title,
    path: opts.path,
    prefer: opts.canvasId ? 'edit' : 'new',
    canvasId: opts.canvasId,
    chatMessageId: opts.chatMessageId,
    sourceHtml: opts.html,
  })
  saveCanvases(sessionId, applied.list)
  const entry =
    applied.list.find((c) => c.id === applied.activeId) || applied.list[0]
  return htmlEntryToGallery(
    entry,
    sessionId,
    sessionId === GALLERY_GLOBAL_SCOPE ? 'Library' : sessionId,
  )
}

export function upsertGalleryDiagramFromOpen(opts: {
  source: string
  title?: string
  scopeId?: string
  canvasId?: string
  chatMessageId?: string
}): GalleryCanvasItem {
  const sessionId = opts.scopeId || GALLERY_GLOBAL_SCOPE
  const list = loadDiagrams(sessionId)
  const src = (opts.source || '').trim()
  const h = hashSource(src)
  const byId = opts.canvasId
    ? list.find((d) => d.id === opts.canvasId)
    : undefined
  const byMsg = opts.chatMessageId
    ? list.find((d) => d.chatMessageId === opts.chatMessageId)
    : undefined
  const byHash = list.find((d) => hashSource(d.source) === h)
  const existing = byId || byMsg || byHash
  const now = Date.now()
  if (existing) {
    const next = list.map((d) =>
      d.id === existing.id
        ? {
            ...d,
            source: src || d.source,
            title:
              opts.title && !isPlaceholderDiagramTitle(opts.title)
                ? opts.title
                : d.title,
            chatMessageId: opts.chatMessageId || d.chatMessageId,
            updatedAt: src && src !== d.source ? now : d.updatedAt,
          }
        : d,
    )
    saveDiagrams(sessionId, next)
    const entry = next.find((d) => d.id === existing.id) || existing
    return diagramEntryToGallery(
      entry,
      sessionId,
      sessionId === GALLERY_GLOBAL_SCOPE ? 'Library' : sessionId,
    )
  }
  const id = `dg_${now.toString(36)}_${Math.random().toString(36).slice(2, 6)}`
  const entry: DiagramEntry = {
    id,
    title:
      (opts.title && !isPlaceholderDiagramTitle(opts.title) && opts.title) ||
      titleFromMermaid(src, `Diagram ${new Date().toLocaleTimeString()}`),
    source: src || BLANK_DIAGRAM,
    createdAt: now,
    updatedAt: now,
    chatMessageId: opts.chatMessageId,
  }
  saveDiagrams(sessionId, [entry, ...list])
  return diagramEntryToGallery(
    entry,
    sessionId,
    sessionId === GALLERY_GLOBAL_SCOPE ? 'Library' : sessionId,
  )
}

function isPlaceholderDiagramTitle(title: string): boolean {
  const t = title.trim()
  if (!t) return true
  return /^(diagram|mermaid|flowchart|graph)(\s|$)/i.test(t)
}

export function createGalleryDiagramCanvas(opts?: {
  title?: string
  scopeId?: string
  source?: string
}): GalleryCanvasItem {
  const sessionId = opts?.scopeId || GALLERY_GLOBAL_SCOPE
  const now = Date.now()
  const id = `dg_${now.toString(36)}_${Math.random().toString(36).slice(2, 6)}`
  const entry: DiagramEntry = {
    id,
    title: opts?.title || `Diagram ${new Date().toLocaleTimeString()}`,
    source: opts?.source || BLANK_DIAGRAM,
    createdAt: now,
    updatedAt: now,
  }
  const list = [entry, ...loadDiagrams(sessionId)]
  saveDiagrams(sessionId, list)
  return diagramEntryToGallery(
    entry,
    sessionId,
    sessionId === GALLERY_GLOBAL_SCOPE ? 'Library' : sessionId,
  )
}

export function createGalleryExcalidrawCanvas(opts?: {
  title?: string
  scopeId?: string
  content?: string
}): GalleryCanvasItem {
  const sessionId = opts?.scopeId || GALLERY_GLOBAL_SCOPE
  const now = Date.now()
  const id = `ex_${now.toString(36)}_${Math.random().toString(36).slice(2, 6)}`
  const entry: ExcalidrawEntry = {
    id,
    title: opts?.title || `Whiteboard ${new Date().toLocaleTimeString()}`,
    content: opts?.content || BLANK_EXCALIDRAW,
    createdAt: now,
    updatedAt: now,
  }
  const list = [entry, ...loadExcalidraws(sessionId)]
  saveExcalidraws(sessionId, list)
  return excalidrawEntryToGallery(
    entry,
    sessionId,
    sessionId === GALLERY_GLOBAL_SCOPE ? 'Library' : sessionId,
  )
}

export function createGallerySlidesCanvas(opts?: {
  title?: string
  scopeId?: string
  content?: string
}): GalleryCanvasItem {
  const sessionId = opts?.scopeId || GALLERY_GLOBAL_SCOPE
  const now = Date.now()
  const id = `sl_${now.toString(36)}_${Math.random().toString(36).slice(2, 6)}`
  const title = opts?.title || `Deck ${new Date().toLocaleTimeString()}`
  const entry: SlidesEntry = {
    id,
    title,
    content: opts?.content || emptyHtmlDeck(title),
    createdAt: now,
    updatedAt: now,
  }
  const list = [entry, ...loadSlides(sessionId)]
  const saved = saveSlides(sessionId, list)
  if (!saved.ok) {
    console.error('[slides] create save failed', saved.error)
  }
  return slidesEntryToGallery(
    entry,
    sessionId,
    sessionId === GALLERY_GLOBAL_SCOPE ? 'Library' : sessionId,
  )
}

export function createGalleryImageCanvas(opts?: {
  title?: string
  scopeId?: string
  prompt?: string
  options?: ImageStudioOptions
}): GalleryCanvasItem {
  const sessionId = opts?.scopeId || GALLERY_GLOBAL_SCOPE
  const now = Date.now()
  const id = `img_${now.toString(36)}_${Math.random().toString(36).slice(2, 6)}`
  const entry: ImageEntry = {
    id,
    title: opts?.title || `Image ${new Date().toLocaleTimeString()}`,
    prompt: opts?.prompt || '',
    agentPrompt: '',
    imageUrl: '',
    caption: '',
    options: opts?.options
      ? { ...DEFAULT_IMAGE_STUDIO_OPTIONS, ...opts.options }
      : { ...DEFAULT_IMAGE_STUDIO_OPTIONS },
    history: [],
    createdAt: now,
    updatedAt: now,
  }
  const list = [entry, ...loadImages(sessionId)]
  saveImages(sessionId, list)
  return imageEntryToGallery(
    entry,
    sessionId,
    sessionId === GALLERY_GLOBAL_SCOPE ? 'Library' : sessionId,
  )
}

export function upsertGalleryImageFromOpen(opts: {
  imageUrl: string
  title?: string
  prompt?: string
  caption?: string
  scopeId?: string
  canvasId?: string
  chatMessageId?: string
  options?: ImageStudioOptions
}): GalleryCanvasItem | null {
  const imageUrl = (opts.imageUrl || '').trim()
  if (!imageUrl) return null
  const sessionId = opts.scopeId || GALLERY_GLOBAL_SCOPE
  const list = loadImages(sessionId)
  const byId = opts.canvasId
    ? list.find((d) => d.id === opts.canvasId)
    : undefined
  const byMsg =
    opts.chatMessageId
      ? list.find(
          (d) =>
            d.chatMessageId === opts.chatMessageId &&
            (!imageUrl || d.imageUrl === imageUrl),
        ) || list.find((d) => d.chatMessageId === opts.chatMessageId)
      : undefined
  const byUrl = list.find((d) => d.imageUrl === imageUrl)
  const existing = byId || byMsg || byUrl
  const now = Date.now()
  const title =
    (opts.title && opts.title.trim()) ||
    (opts.caption && opts.caption.trim()) ||
    `Image ${new Date().toLocaleTimeString()}`
  if (existing) {
    const next = list.map((d) =>
      d.id === existing.id
        ? {
            ...d,
            imageUrl,
            title:
              opts.title && opts.title.trim() && !/^generated image/i.test(opts.title)
                ? opts.title.trim()
                : d.title || title,
            prompt: opts.prompt !== undefined ? opts.prompt : d.prompt,
            caption:
              opts.caption !== undefined
                ? opts.caption
                : d.caption || opts.caption || '',
            chatMessageId: opts.chatMessageId || d.chatMessageId,
            options: opts.options
              ? { ...DEFAULT_IMAGE_STUDIO_OPTIONS, ...opts.options }
              : d.options,
            updatedAt: imageUrl !== d.imageUrl ? now : d.updatedAt,
          }
        : d,
    )
    saveImages(sessionId, next)
    const entry = next.find((d) => d.id === existing.id) || existing
    return imageEntryToGallery(
      entry,
      sessionId,
      sessionId === GALLERY_GLOBAL_SCOPE ? 'Library' : sessionId,
    )
  }
  const id = `img_${now.toString(36)}_${Math.random().toString(36).slice(2, 6)}`
  const entry: ImageEntry = {
    id,
    title,
    prompt: opts.prompt || '',
    agentPrompt: '',
    imageUrl,
    caption: opts.caption || '',
    options: opts.options
      ? { ...DEFAULT_IMAGE_STUDIO_OPTIONS, ...opts.options }
      : { ...DEFAULT_IMAGE_STUDIO_OPTIONS },
    history: [],
    createdAt: now,
    updatedAt: now,
    chatMessageId: opts.chatMessageId,
  }
  saveImages(sessionId, [entry, ...list])
  return imageEntryToGallery(
    entry,
    sessionId,
    sessionId === GALLERY_GLOBAL_SCOPE ? 'Library' : sessionId,
  )
}

export function listHtmlCanvasesAcrossSessions(
  sessions: SessionLabel[],
): GalleryCanvasItem[] {
  return listGalleryCanvases(sessions).filter((i) => i.kind === 'html')
}

function htmlEntryToGallery(
  c: CanvasEntry,
  sessionId: string,
  sessionName: string,
): GalleryCanvasItem {
  const createdAt = c.createdAt || c.updatedAt || 0
  const updatedAt = c.updatedAt || createdAt
  return {
    id: `${sessionId}::html::${c.id}`,
    kind: 'html',
    title: c.title || 'Playground',
    sessionId,
    sessionName,
    updatedAt,
    createdAt,
    html: c.html,
    path: c.path,
    chatMessageId: c.chatMessageId,
  }
}

function diagramEntryToGallery(
  d: DiagramEntry,
  sessionId: string,
  sessionName: string,
): GalleryCanvasItem {
  const createdAt = d.createdAt || d.updatedAt || 0
  const updatedAt = d.updatedAt || createdAt
  return {
    id: `${sessionId}::diagram::${d.id}`,
    kind: 'diagram',
    title: d.title || 'Diagram',
    sessionId,
    sessionName,
    updatedAt,
    createdAt,
    source: d.source,
    chatMessageId: d.chatMessageId,
  }
}

function imageEntryToGallery(
  img: ImageEntry,
  sessionId: string,
  sessionName: string,
): GalleryCanvasItem {
  const createdAt = img.createdAt || img.updatedAt || 0
  const updatedAt = img.updatedAt || createdAt
  return {
    id: `${sessionId}::image::${img.id}`,
    kind: 'image',
    title: img.title || 'Image',
    sessionId,
    sessionName,
    updatedAt,
    createdAt,
    prompt: img.prompt,
    agentPrompt: img.agentPrompt,
    imageUrl: img.imageUrl,
    caption: img.caption,
    imageOptions: img.options,
    imageHistory: img.history || [],
    chatMessageId: img.chatMessageId,
  }
}

function excalidrawEntryToGallery(
  entry: ExcalidrawEntry,
  sessionId: string,
  sessionName: string,
): GalleryCanvasItem {
  const createdAt = entry.createdAt || entry.updatedAt || 0
  const updatedAt = entry.updatedAt || createdAt
  return {
    id: `${sessionId}::excalidraw::${entry.id}`,
    kind: 'excalidraw',
    title: entry.title || 'Whiteboard',
    sessionId,
    sessionName,
    updatedAt,
    createdAt,
    source: entry.content,
    chatMessageId: entry.chatMessageId,
  }
}

function slidesEntryToGallery(
  entry: SlidesEntry,
  sessionId: string,
  sessionName: string,
): GalleryCanvasItem {
  const createdAt = entry.createdAt || entry.updatedAt || 0
  const updatedAt = entry.updatedAt || createdAt
  return {
    id: `${sessionId}::slides::${entry.id}`,
    kind: 'slides',
    title: entry.title || 'Deck',
    sessionId,
    sessionName,
    updatedAt,
    createdAt,
    source: entry.content,
    chatMessageId: entry.chatMessageId,
  }
}

export function parseGalleryId(galleryId: string): {
  sessionId: string
  kind: string
  localId: string
} | null {
  const parts = galleryId.split('::')
  if (parts.length === 2) {
    return {
      sessionId: parts[0] || '',
      kind: 'html',
      localId: parts[1] || '',
    }
  }
  if (parts.length >= 3) {
    return {
      sessionId: parts[0] || '',
      kind: parts[1] || 'html',
      localId: parts.slice(2).join('::'),
    }
  }
  return null
}

export function loadHtmlCanvas(
  sessionId: string,
  canvasId: string,
): CanvasEntry | null {
  return loadCanvases(sessionId).find((c) => c.id === canvasId) || null
}

export function updateHtmlCanvas(
  sessionId: string,
  canvasId: string,
  patch: Partial<Pick<CanvasEntry, 'html' | 'title' | 'updatedAt'>>,
): (CanvasEntry & { saveResult?: CanvasSaveResult }) | null {
  const list = loadCanvases(sessionId)
  const idx = list.findIndex((c) => c.id === canvasId)
  if (idx < 0) return null
  const prev = list[idx]
  const htmlChanged =
    patch.html !== undefined && patch.html !== prev.html
  const titleChanged =
    patch.title !== undefined && patch.title !== prev.title
  if (!htmlChanged && !titleChanged && patch.updatedAt === undefined) {
    return prev
  }
  const next = {
    ...prev,
    ...patch,
    updatedAt:
      patch.updatedAt ??
      (htmlChanged || titleChanged ? Date.now() : prev.updatedAt),
  }
  const out = list.slice()
  out[idx] = next
  return { ...next, saveResult: saveCanvases(sessionId, out) }
}

export function deleteHtmlCanvas(
  sessionId: string,
  canvasId: string,
): boolean {
  const list = loadCanvases(sessionId)
  const next = list.filter((c) => c.id !== canvasId)
  if (next.length === list.length) return false
  saveCanvases(sessionId, next)
  return true
}

export function loadDiagramCanvas(
  sessionId: string,
  diagramId: string,
): DiagramEntry | null {
  return loadDiagrams(sessionId).find((d) => d.id === diagramId) || null
}

export function updateDiagramCanvas(
  sessionId: string,
  diagramId: string,
  patch: Partial<Pick<DiagramEntry, 'source' | 'title' | 'updatedAt'>>,
): (DiagramEntry & { saveResult?: DiagramSaveResult }) | null {
  const list = loadDiagrams(sessionId)
  const idx = list.findIndex((d) => d.id === diagramId)
  if (idx < 0) return null
  const prev = list[idx]
  const sourceChanged =
    patch.source !== undefined && patch.source !== prev.source
  const titleChanged =
    patch.title !== undefined && patch.title !== prev.title
  if (!sourceChanged && !titleChanged && patch.updatedAt === undefined) {
    return prev
  }
  const next = {
    ...prev,
    ...patch,
    updatedAt:
      patch.updatedAt ??
      (sourceChanged || titleChanged ? Date.now() : prev.updatedAt),
  }
  const out = list.slice()
  out[idx] = next
  return { ...next, saveResult: saveDiagrams(sessionId, out) }
}

export function deleteDiagramCanvas(
  sessionId: string,
  diagramId: string,
): boolean {
  const list = loadDiagrams(sessionId)
  const next = list.filter((d) => d.id !== diagramId)
  if (next.length === list.length) return false
  saveDiagrams(sessionId, next)
  return true
}

export function loadImageCanvas(
  sessionId: string,
  imageId: string,
): ImageEntry | null {
  return loadImages(sessionId).find((d) => d.id === imageId) || null
}

export function updateImageCanvas(
  sessionId: string,
  imageId: string,
  patch: Partial<
    Pick<
      ImageEntry,
      | 'title'
      | 'prompt'
      | 'agentPrompt'
      | 'imageUrl'
      | 'caption'
      | 'options'
      | 'history'
      | 'updatedAt'
    >
  >,
): (ImageEntry & { saveResult?: ImageSaveResult }) | null {
  const list = loadImages(sessionId)
  const idx = list.findIndex((d) => d.id === imageId)
  if (idx < 0) return null
  const prev = list[idx]
  const contentChanged =
    (patch.prompt !== undefined && patch.prompt !== prev.prompt) ||
    (patch.agentPrompt !== undefined &&
      patch.agentPrompt !== prev.agentPrompt) ||
    (patch.imageUrl !== undefined && patch.imageUrl !== prev.imageUrl) ||
    (patch.caption !== undefined && patch.caption !== prev.caption) ||
    patch.options !== undefined ||
    patch.history !== undefined ||
    (patch.title !== undefined && patch.title !== prev.title)
  if (!contentChanged && patch.updatedAt === undefined) {
    return prev
  }
  const next: ImageEntry = {
    ...prev,
    ...patch,
    options: patch.options
      ? { ...DEFAULT_IMAGE_STUDIO_OPTIONS, ...patch.options }
      : prev.options,
    history: patch.history
      ? patch.history.slice(0, IMAGE_STUDIO_HISTORY_MAX)
      : prev.history || [],
    updatedAt:
      patch.updatedAt ?? (contentChanged ? Date.now() : prev.updatedAt),
  }
  const out = list.slice()
  out[idx] = next
  // Newest-first ordering means the edited image is never the one evicted;
  // `saveResult` carries any quota warning for the caller to show.
  return { ...next, saveResult: saveImages(sessionId, out) }
}

export function pushImageHistoryVersion(
  sessionId: string,
  imageId: string,
  nextVersion: {
    imageUrl: string
    prompt: string
    agentPrompt: string
    caption: string
    options: ImageStudioOptions
    title?: string
  },
): (ImageEntry & { saveResult?: ImageSaveResult }) | null {
  const prev = loadImageCanvas(sessionId, imageId)
  if (!prev) return null
  const now = Date.now()
  let history = [...(prev.history || [])]
  if (prev.imageUrl && prev.imageUrl !== nextVersion.imageUrl) {
    const snap: ImageStudioHistoryItem = {
      id: `h_${now.toString(36)}_${Math.random().toString(36).slice(2, 6)}`,
      imageUrl: prev.imageUrl,
      prompt: prev.prompt,
      agentPrompt: prev.agentPrompt || undefined,
      caption: prev.caption,
      createdAt: prev.updatedAt || prev.createdAt || now,
      options: prev.options,
    }
    history = [snap, ...history.filter((h) => h.imageUrl !== prev.imageUrl)]
  }
  history = history.slice(0, IMAGE_STUDIO_HISTORY_MAX)
  return updateImageCanvas(sessionId, imageId, {
    prompt: nextVersion.prompt,
    agentPrompt: nextVersion.agentPrompt,
    imageUrl: nextVersion.imageUrl,
    caption: nextVersion.caption,
    options: nextVersion.options,
    history,
    ...(nextVersion.title ? { title: nextVersion.title } : {}),
  })
}

export function restoreImageHistoryVersion(
  sessionId: string,
  imageId: string,
  historyId: string,
): (ImageEntry & { saveResult?: ImageSaveResult }) | null {
  const prev = loadImageCanvas(sessionId, imageId)
  if (!prev) return null
  const item = (prev.history || []).find((h) => h.id === historyId)
  if (!item) return null
  const now = Date.now()
  let history = [...(prev.history || [])].filter((h) => h.id !== historyId)
  if (prev.imageUrl && prev.imageUrl !== item.imageUrl) {
    history = [
      {
        id: `h_${now.toString(36)}_${Math.random().toString(36).slice(2, 6)}`,
        imageUrl: prev.imageUrl,
        prompt: prev.prompt,
        agentPrompt: prev.agentPrompt || undefined,
        caption: prev.caption,
        createdAt: prev.updatedAt || prev.createdAt || now,
        options: prev.options,
      },
      ...history,
    ]
  }
  return updateImageCanvas(sessionId, imageId, {
    imageUrl: item.imageUrl,
    prompt: item.prompt,
    agentPrompt: item.agentPrompt || '',
    caption: item.caption,
    options: item.options
      ? { ...DEFAULT_IMAGE_STUDIO_OPTIONS, ...item.options }
      : prev.options,
    history: history.slice(0, IMAGE_STUDIO_HISTORY_MAX),
  })
}

export function deleteImageHistoryVersion(
  sessionId: string,
  imageId: string,
  historyId: string,
): (ImageEntry & { saveResult?: ImageSaveResult }) | null {
  const prev = loadImageCanvas(sessionId, imageId)
  if (!prev || !historyId) return null
  const nextHistory = (prev.history || []).filter((h) => h.id !== historyId)
  if (nextHistory.length === (prev.history || []).length) return null
  return updateImageCanvas(sessionId, imageId, {
    history: nextHistory,
  })
}

export function loadExcalidrawCanvas(
  sessionId: string,
  canvasId: string,
): ExcalidrawEntry | null {
  return loadExcalidraws(sessionId).find((d) => d.id === canvasId) || null
}

export function updateExcalidrawCanvas(
  sessionId: string,
  canvasId: string,
  patch: Partial<Pick<ExcalidrawEntry, 'content' | 'title' | 'updatedAt'>>,
): (ExcalidrawEntry & { saveResult?: ExcalidrawSaveResult }) | null {
  const list = loadExcalidraws(sessionId)
  const idx = list.findIndex((c) => c.id === canvasId)
  if (idx < 0) return null
  const prev = list[idx]
  const contentChanged =
    patch.content !== undefined && patch.content !== prev.content
  const titleChanged =
    patch.title !== undefined && patch.title !== prev.title
  if (!contentChanged && !titleChanged && patch.updatedAt === undefined) {
    return prev
  }
  const next = {
    ...prev,
    ...patch,
    updatedAt:
      patch.updatedAt ??
      (contentChanged || titleChanged ? Date.now() : prev.updatedAt),
  }
  const out = list.slice()
  out[idx] = next
  return { ...next, saveResult: saveExcalidraws(sessionId, out) }
}

export function deleteExcalidrawCanvas(
  sessionId: string,
  canvasId: string,
): boolean {
  const list = loadExcalidraws(sessionId)
  const next = list.filter((c) => c.id !== canvasId)
  if (next.length === list.length) return false
  saveExcalidraws(sessionId, next)
  return true
}

export function loadSlidesCanvas(
  sessionId: string,
  canvasId: string,
): SlidesEntry | null {
  return loadSlides(sessionId).find((d) => d.id === canvasId) || null
}

export function updateSlidesCanvas(
  sessionId: string,
  canvasId: string,
  patch: Partial<Pick<SlidesEntry, 'content' | 'title' | 'updatedAt'>>,
): (SlidesEntry & { saveResult?: SlidesSaveResult }) | null {
  const list = loadSlides(sessionId)
  const idx = list.findIndex((c) => c.id === canvasId)
  if (idx < 0) return null
  const prev = list[idx]
  const contentChanged =
    patch.content !== undefined && patch.content !== prev.content
  const titleChanged =
    patch.title !== undefined && patch.title !== prev.title
  if (!contentChanged && !titleChanged && patch.updatedAt === undefined) {
    return prev
  }
  const next = {
    ...prev,
    ...patch,
    updatedAt:
      patch.updatedAt ??
      (contentChanged || titleChanged ? Date.now() : prev.updatedAt),
  }
  const out = list.slice()
  out[idx] = next
  // The edited deck is always the newest entry, so it survives any eviction;
  // `saveResult` still carries the warning for the caller to surface.
  return { ...next, saveResult: saveSlides(sessionId, out) }
}

export function deleteSlidesCanvas(
  sessionId: string,
  canvasId: string,
): boolean {
  const list = loadSlides(sessionId)
  const next = list.filter((c) => c.id !== canvasId)
  if (next.length === list.length) return false
  saveSlides(sessionId, next)
  return true
}

export function deleteImageCanvas(
  sessionId: string,
  imageId: string,
): boolean {
  const list = loadImages(sessionId)
  const next = list.filter((d) => d.id !== imageId)
  if (next.length === list.length) return false
  saveImages(sessionId, next)
  return true
}

export function extractImageFromMarkdown(md: string): {
  imageUrl: string
  caption: string
} {
  const text = (md || '').trim()
  if (!text) return { imageUrl: '', caption: '' }
  const mdImg = /!\[[^\]]*\]\((data:image\/[^)\s]+|https?:\/\/[^)\s]+)\)/i.exec(
    text,
  )
  if (mdImg?.[1]) {
    const imageUrl = mdImg[1].trim()
    const caption = text
      .replace(mdImg[0], '')
      .replace(/\n{2,}/g, '\n')
      .trim()
    return { imageUrl, caption }
  }
  const dataOnly = /(data:image\/[a-zA-Z0-9.+-]+;base64,[A-Za-z0-9+/=\s]+)/i.exec(
    text,
  )
  if (dataOnly?.[1]) {
    const imageUrl = dataOnly[1].replace(/\s+/g, '')
    const caption = text.replace(dataOnly[0], '').trim()
    return { imageUrl, caption }
  }
  const httpOnly = /(https?:\/\/\S+\.(?:png|jpe?g|webp|gif)(?:\?\S*)?)/i.exec(
    text,
  )
  if (httpOnly?.[1]) {
    return {
      imageUrl: httpOnly[1],
      caption: text.replace(httpOnly[0], '').trim(),
    }
  }
  return { imageUrl: '', caption: text }
}

const FENCE_RE =
  /```(?:mermaid|mmd)?\s*\n([\s\S]*?)```/gi

function titleFromMermaid(source: string, fallback: string): string {
  const s = source.trim()
  const first = s.split('\n').find((l) => l.trim() && !l.trim().startsWith('%%'))
  if (!first) return fallback
  const t = first.replace(/^(flowchart|graph|sequenceDiagram|erDiagram|classDiagram|stateDiagram(?:-v2)?|gantt|pie|mindmap)\b/i, '').trim()
  if (t.length > 2 && t.length < 60) return t
  const line = first.trim().slice(0, 48)
  return line || fallback
}

export function indexDiagramsFromChatMessages(
  sessionId: string,
  messages: Array<{ id?: string; role?: string; content?: string }>,
): number {
  const existing = loadDiagrams(sessionId)
  const byMsgLink = new Map(
    existing
      .filter((d) => d.chatMessageId)
      .map((d) => [d.chatMessageId as string, d]),
  )
  const byHash = new Map(existing.map((d) => [hashSource(d.source), d]))
  let added = 0
  const next = existing.slice()

  for (const m of messages) {
    if ((m.role || '') !== 'assistant') continue
    const content = m.content || ''
    if (!content.includes('```') && !isMermaidFence('', content)) continue
    FENCE_RE.lastIndex = 0
    let match: RegExpExecArray | null
    let block = 0
    while ((match = FENCE_RE.exec(content)) !== null) {
      const body = (match[1] || '').trim()
      if (!body || !isMermaidFence('mermaid', body)) continue
      block += 1
      const h = hashSource(body)
      const linkId = m.id
        ? block > 1
          ? `${m.id}#mmd${block}`
          : m.id
        : ''
      if (linkId && byMsgLink.has(linkId)) continue
      if (byHash.has(h)) continue
      const stamp = Date.now()
      const id = `dg_${stamp.toString(36)}_${block}_${Math.random().toString(36).slice(2, 6)}`
      const entry: DiagramEntry = {
        id,
        title: titleFromMermaid(body, `Diagram ${block}`),
        source: body,
        createdAt: stamp,
        updatedAt: stamp,
        chatMessageId: linkId || undefined,
      }
      next.push(entry)
      byHash.set(h, entry)
      if (linkId) byMsgLink.set(linkId, entry)
      added += 1
    }
  }

  if (added > 0) saveDiagrams(sessionId, next)
  return added
}

function hashSource(s: string): string {
  const t = s.replace(/\s+/g, ' ').trim()
  let h = 0
  for (let i = 0; i < t.length; i += 1) {
    h = (Math.imul(31, h) + t.charCodeAt(i)) | 0
  }
  return `${t.length}_${h}`
}

export function formatGalleryTime(ts: number): string {
  if (!ts) return '—'
  try {
    return new Date(ts).toLocaleString()
  } catch {
    return '—'
  }
}
