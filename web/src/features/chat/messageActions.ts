/**
 * Pure helpers for per-message chat actions (VAL-CHAT-016..020).
 * Kept free of React so node:test can run without a DOM.
 */

export type ActionableMessage = {
  id: string
  role: 'user' | 'assistant' | 'system' | string
  content?: string
}

/** Message ids eligible for undo-from-here / delete in a list. */
export function indexOfMessageId<T extends { id: string }>(
  messages: T[],
  messageId: string,
): number {
  if (!messageId) return -1
  return messages.findIndex((m) => m.id === messageId)
}

/**
 * Truncate history from messageId inclusive to the end (undo-from-here).
 * Returns a new array; original is not mutated.
 * Empty/missing id → original list.
 */
export function truncateFromMessageId<T extends { id: string }>(
  messages: T[],
  messageId: string,
): T[] {
  const idx = indexOfMessageId(messages, messageId)
  if (idx < 0) return messages.slice()
  return messages.slice(0, idx)
}

/**
 * Delete a single message by id. Returns a new array.
 * Missing id → original list (copy).
 */
export function deleteMessageById<T extends { id: string }>(
  messages: T[],
  messageId: string,
): T[] {
  const idx = indexOfMessageId(messages, messageId)
  if (idx < 0) return messages.slice()
  return [...messages.slice(0, idx), ...messages.slice(idx + 1)]
}

/**
 * Which actions appear on the per-message action bar.
 * Confirm is only recommended for bulk-dangerous ops (undo-from-here removes multiple).
 */
export function messageActionFlags(opts: {
  role: string
  hasImages?: boolean
  streaming?: boolean
}): {
  copy: boolean
  downloadMd: boolean
  undoFromHere: boolean
  delete: boolean
  previewImage: boolean
  downloadImage: boolean
  confirmUndoFromHere: boolean
  confirmDelete: boolean
} {
  const role = opts.role || ''
  const streaming = !!opts.streaming
  const user = role === 'user'
  const assistant = role === 'assistant'
  const hasImages = !!opts.hasImages
  return {
    copy: !streaming && (user || assistant),
    downloadMd: !streaming && assistant,
    // Undo-from-here is meaningful for any message that can be a truncate point.
    undoFromHere: !streaming && (user || assistant),
    delete: !streaming && (user || assistant),
    previewImage: !streaming && hasImages,
    downloadImage: !streaming && hasImages,
    // Bulk-dangerous: truncating may drop many messages → confirm.
    confirmUndoFromHere: true,
    // Single-message delete: no modal confirm (clear feedback toast/status only).
    confirmDelete: false,
  }
}

function sanitizeSegment(raw: string, fallback: string): string {
  const s = (raw || '')
    .replace(/[<>:"/\\|?*\u0000-\u001f]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
  if (!s) return fallback
  return s.slice(0, 80).replace(/[. ]+$/g, '') || fallback
}

function cleanSessionLabel(name?: string | null): string {
  return sanitizeSegment(
    (name || '')
      .replace(/^Session\s*[·•\-–—]\s*/i, '')
      .replace(/^Sesi\s*[·•\-–—]\s*/i, '')
      .replace(/^Workspace\s*[·•\-–—]\s*/i, '')
      .trim() || 'chat',
    'chat',
  )
}

export function assistantMarkdownFilename(
  messageId: string,
  sessionName?: string | null,
  messageIndex?: number | null,
): string {
  const session = cleanSessionLabel(sessionName)
  let idPart = ''
  if (messageIndex != null && Number.isFinite(messageIndex) && messageIndex > 0) {
    idPart = String(Math.floor(messageIndex))
  } else {
    const safe = (messageId || 'msg').replace(/[^\w.-]+/g, '-')
    idPart = safe.slice(-12) || '1'
  }
  return `Inferenesia - ${session} - ${idPart}.md`
}

export function imageDownloadFilename(
  src: string,
  alt?: string,
  index = 0,
  sessionName?: string | null,
): string {
  const session = cleanSessionLabel(sessionName)
  let ext = 'png'
  const mime = /^data:image\/([\w+.-]+);/i.exec(src || '')
  if (mime?.[1]) {
    ext = mime[1].split('+')[0] || 'png'
    if (ext === 'jpeg') ext = 'jpg'
  } else {
    try {
      const u = new URL(src)
      const base = u.pathname.split('/').filter(Boolean).pop() || ''
      const m = /\.(\w{2,5})$/.exec(base)
      if (m) ext = m[1]
    } catch {
      void 0
    }
  }
  const fromAlt = sanitizeSegment(alt || '', '')
  if (fromAlt && fromAlt !== 'session') {
    const stem = fromAlt.includes('.') ? fromAlt.replace(/\.\w+$/, '') : fromAlt
    return `Inferenesia - ${session} - ${stem}.${ext}`
  }
  return `Inferenesia - ${session} - image-${index + 1}.${ext}`
}

/** Payload shape for durable replace history (mirrors DesktopChatMessage). */
export function toPersistableMessages(
  messages: ActionableMessage[],
): { id: string; role: string; content: string }[] {
  return messages
    .filter((m) => m.role === 'user' || m.role === 'assistant')
    .map((m) => ({
      id: m.id,
      role: m.role,
      content: m.content || '',
    }))
}
