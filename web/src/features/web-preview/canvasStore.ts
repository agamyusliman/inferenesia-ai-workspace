export type CanvasEntry = {
  id: string
  title: string
  html: string
  path?: string
  createdAt: number
  updatedAt: number
  chatMessageId?: string
  sourceHtml?: string
}

export type CanvasPrefer = 'edit' | 'new'

const KEY_PREFIX = 'inferenesia-html-canvases:'
const KEY_PREFIX_TYPO = 'infernesia-html-canvases:'

function storageKey(scopeId: string): string {
  return KEY_PREFIX + (scopeId || 'global')
}

function storageKeyTypo(scopeId: string): string {
  return KEY_PREFIX_TYPO + (scopeId || 'global')
}

function safeParse(raw: string | null): CanvasEntry[] {
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
          title: String(o.title || 'Playground'),
          html: String(o.html || ''),
          path: typeof o.path === 'string' ? o.path : undefined,
          createdAt: Number(o.createdAt) || 0,
          updatedAt: Number(o.updatedAt) || Number(o.createdAt) || 0,
          chatMessageId:
            typeof o.chatMessageId === 'string' ? o.chatMessageId : undefined,
          sourceHtml: typeof o.sourceHtml === 'string' ? o.sourceHtml : undefined,
        }
      })
      .filter((c) => c.id && c.html)
  } catch {
    return []
  }
}

export function loadCanvases(scopeId: string): CanvasEntry[] {
  if (typeof window === 'undefined' || !window.localStorage) return []
  try {
    const raw =
      window.localStorage.getItem(storageKey(scopeId)) ??
      window.localStorage.getItem(storageKeyTypo(scopeId))
    return safeParse(raw)
  } catch {
    return []
  }
}

export type CanvasSaveResult = { ok: boolean; error?: string }

export function storageSaveError(what: string, e: unknown): string {
  return (
    `Browser storage is full — ${what} was not saved. Export it now, then delete something from the Playground.` +
    (e instanceof Error && e.message ? ` (${e.message})` : '')
  )
}

/**
 * A quota failure leaves previous storage untouched and is reported, so the
 * caller must keep the draft unsaved rather than flashing "Saved".
 */
export function saveCanvases(
  scopeId: string,
  list: CanvasEntry[],
): CanvasSaveResult {
  if (typeof window === 'undefined' || !window.localStorage) {
    return { ok: false, error: 'No localStorage' }
  }
  try {
    const trimmed = list
      .slice()
      .sort((a, b) => b.updatedAt - a.updatedAt)
      .slice(0, 40)
    window.localStorage.setItem(storageKey(scopeId), JSON.stringify(trimmed))
    return { ok: true }
  } catch (e) {
    return { ok: false, error: storageSaveError('this page', e) }
  }
}

export function clearCanvases(scopeId: string): void {
  if (typeof window === 'undefined' || !window.localStorage) return
  try {
    window.localStorage.removeItem(storageKey(scopeId))
  } catch {
    void 0
  }
}

export function newCanvasId(): string {
  return `cv_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`
}

export function titleFromHtml(html: string, path?: string): string {
  if (path) {
    const base = path.split(/[/\\]/).pop() || path
    if (base && !/^preview\.html?$/i.test(base) && !/^chat-inline/i.test(base)) {
      return base.slice(0, 80)
    }
  }
  const title = /<title[^>]*>([^<]*)<\/title>/i.exec(html || '')?.[1]?.trim()
  if (title) return title.slice(0, 80)
  const h1 = /<h1[^>]*>([\s\S]*?)<\/h1>/i.exec(html || '')?.[1]
  if (h1) {
    const plain = h1.replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim()
    if (plain) return plain.slice(0, 80)
  }
  return `Playground ${new Date().toLocaleTimeString()}`
}

export function isPlaceholderCanvasTitle(title: string | undefined): boolean {
  const t = (title || '').trim()
  if (!t) return true
  if (/^(html|htm|canvas|playground|preview|code|new canvas|new playground)$/i.test(t))
    return true
  if (/^Canvas\s/i.test(t)) return true
  if (/^Playground\s/i.test(t)) return true
  if (/^Diagram\s/i.test(t)) return true
  return false
}

function isGenericCanvasTitle(title: string | undefined): boolean {
  return isPlaceholderCanvasTitle(title)
}

export function htmlFingerprint(html: string): string {
  const s = (html || '').replace(/\s+/g, ' ').trim()
  return s.slice(0, 4000)
}

export function findCanvasByHtml(
  scopeId: string,
  html: string,
): CanvasEntry | null {
  const fp = htmlFingerprint(html)
  if (!fp || fp.length < 32) return null
  return loadCanvases(scopeId).find((c) => htmlFingerprint(c.html) === fp) || null
}

export const LIBRARY_SCOPE_ID = 'global'

function normalizeHtmlBody(html: string): string {
  return (html || '').replace(/\s+/g, ' ').trim()
}

export function findSupersededCanvasId(
  list: CanvasEntry[],
  nextHtml: string,
): string | null {
  const next = normalizeHtmlBody(nextHtml)
  if (next.length < 32) return null
  let best: { id: string; len: number } | null = null
  for (const c of list) {
    const prev = normalizeHtmlBody(c.html)
    if (prev.length < 16) continue
    if (prev === next) return c.id
    if (next.length > prev.length && next.startsWith(prev)) {
      if (!best || prev.length > best.len) best = { id: c.id, len: prev.length }
      continue
    }
    if (prev.length > next.length && prev.startsWith(next)) {
      if (!best || prev.length > best.len) best = { id: c.id, len: prev.length }
    }
  }
  return best?.id ?? null
}

export function canvasHistorySubtitle(entry: CanvasEntry): string {
  if (entry.path) return entry.path
  try {
    const ts = entry.updatedAt || entry.createdAt
    return new Date(ts).toLocaleString()
  } catch {
    return 'In-memory canvas'
  }
}

export function applyCanvasSeed(
  list: CanvasEntry[],
  opts: {
    html: string
    path?: string
    title?: string
    prefer?: CanvasPrefer
    activeId?: string | null
    chatMessageId?: string
    sourceHtml?: string
    canvasId?: string
  },
): { list: CanvasEntry[]; activeId: string } {
  const now = Date.now()
  const derived = titleFromHtml(opts.html, opts.path)
  const title = isGenericCanvasTitle(opts.title) ? derived : (opts.title || derived)
  const prefer = opts.prefer || 'edit'
  const fp = htmlFingerprint(opts.html)
  const link = {
    chatMessageId: opts.chatMessageId,
    sourceHtml: opts.sourceHtml || opts.html,
  }

  if (opts.canvasId) {
    const byId = list.find((c) => c.id === opts.canvasId)
    if (byId) {
      if (htmlFingerprint(byId.html) === fp) {
        return { list, activeId: byId.id }
      }
      const next = list.map((c) =>
        c.id === byId.id
          ? {
              ...c,
              html: opts.html,
              path: opts.path || c.path,
              title: title || c.title,
              updatedAt: now,
              ...link,
            }
          : c,
      )
      return { list: next, activeId: byId.id }
    }
  }

  if (opts.chatMessageId) {
    const byMsg = list.find((c) => c.chatMessageId === opts.chatMessageId)
    if (byMsg) {
      if (htmlFingerprint(byMsg.html) === fp) {
        return { list, activeId: byMsg.id }
      }
      const next = list.map((c) =>
        c.id === byMsg.id
          ? {
              ...c,
              html: opts.html,
              path: opts.path || c.path,
              title: title || c.title,
              updatedAt: now,
              ...link,
            }
          : c,
      )
      return { list: next, activeId: byMsg.id }
    }
  }

  const supersededId = findSupersededCanvasId(list, opts.html)
  if (supersededId) {
    const prev = list.find((c) => c.id === supersededId)
    if (prev) {
      const prevNorm = normalizeHtmlBody(prev.html)
      const nextNorm = normalizeHtmlBody(opts.html)
      if (prevNorm === nextNorm) {
        return { list, activeId: prev.id }
      }
      if (nextNorm.length >= prevNorm.length) {
        const next = list.map((c) =>
          c.id === supersededId
            ? {
                ...c,
                html: opts.html,
                path: opts.path || c.path,
                title: title || c.title,
                updatedAt: now,
                ...link,
              }
            : c,
        )
        return { list: next, activeId: supersededId }
      }
      return { list, activeId: prev.id }
    }
  }

  if (prefer === 'new') {
    const same = list.find(
      (c) =>
        (opts.path && c.path === opts.path) ||
        htmlFingerprint(c.html) === fp,
    )
    if (same) {
      const next = list.map((c) =>
        c.id === same.id
          ? {
              ...c,
              chatMessageId: link.chatMessageId || c.chatMessageId,
              sourceHtml: link.sourceHtml || c.sourceHtml,
            }
          : c,
      )
      return { list: next, activeId: same.id }
    }
    const entry: CanvasEntry = {
      id: newCanvasId(),
      title,
      html: opts.html,
      path: opts.path,
      createdAt: now,
      updatedAt: now,
      ...link,
    }
    return { list: [entry, ...list], activeId: entry.id }
  }

  if (opts.path) {
    const idx = list.findIndex((c) => c.path === opts.path)
    if (idx >= 0) {
      const prev = list[idx]
      if (htmlFingerprint(prev.html) === fp) {
        const next = list.slice()
        next[idx] = {
          ...prev,
          chatMessageId: link.chatMessageId || prev.chatMessageId,
          sourceHtml: link.sourceHtml || prev.sourceHtml,
        }
        return { list: next, activeId: prev.id }
      }
      const next = list.slice()
      next[idx] = {
        ...prev,
        html: opts.html,
        title: title || prev.title,
        updatedAt: now,
        ...link,
      }
      return { list: next, activeId: next[idx].id }
    }
  }

  if (opts.activeId) {
    const idx = list.findIndex((c) => c.id === opts.activeId)
    if (idx >= 0) {
      const prev = list[idx]
      if (htmlFingerprint(prev.html) === fp && !opts.path) {
        const next = list.slice()
        next[idx] = {
          ...prev,
          chatMessageId: link.chatMessageId || prev.chatMessageId,
          sourceHtml: link.sourceHtml || prev.sourceHtml,
        }
        return { list: next, activeId: prev.id }
      }
      const next = list.slice()
      next[idx] = {
        ...prev,
        html: opts.html,
        path: opts.path || prev.path,
        title: title || prev.title,
        updatedAt: now,
        ...link,
      }
      return { list: next, activeId: next[idx].id }
    }
  }

  const same = list.find((c) => htmlFingerprint(c.html) === fp)
  if (same && !opts.path) {
    const next = list.map((c) =>
      c.id === same.id
        ? {
            ...c,
            chatMessageId: link.chatMessageId || c.chatMessageId,
            sourceHtml: link.sourceHtml || c.sourceHtml,
          }
        : c,
    )
    return { list: next, activeId: same.id }
  }

  const entry: CanvasEntry = {
    id: newCanvasId(),
    title,
    html: opts.html,
    path: opts.path,
    createdAt: now,
    updatedAt: now,
    ...link,
  }
  return { list: [entry, ...list], activeId: entry.id }
}

export function replaceHtmlInMarkdown(
  content: string,
  fromHtml: string,
  toHtml: string,
): string | null {
  if (!fromHtml || fromHtml === toHtml) return null
  if (!content.includes(fromHtml)) return null
  return content.replace(fromHtml, toHtml)
}

export function prepareCanvasSwitch(
  list: CanvasEntry[],
  opts: {
    fromId: string | null
    toId: string
    draftHtml: string
    draftPath?: string
  },
): { list: CanvasEntry[]; entry: CanvasEntry | null; dirty: boolean } {
  let next = list
  let dirty = false
  if (opts.fromId && opts.fromId !== opts.toId) {
    const from = list.find((c) => c.id === opts.fromId)
    if (from) {
      const pathChanged =
        Boolean(opts.draftPath) && opts.draftPath !== (from.path || '')
      const htmlChanged = from.html !== opts.draftHtml
      if (htmlChanged || pathChanged) {
        dirty = true
        next = list.map((c) =>
          c.id === opts.fromId
            ? {
                ...c,
                html: opts.draftHtml,
                path: opts.draftPath || c.path,
                title:
                  titleFromHtml(opts.draftHtml, opts.draftPath || c.path) ||
                  c.title,
                updatedAt: Date.now(),
              }
            : c,
        )
      }
    }
  }
  const entry = next.find((c) => c.id === opts.toId) ?? null
  return { list: next, entry, dirty }
}
