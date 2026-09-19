/**
 * Pure helpers for paperclip chat attachments (VAL-CHAT-021..024).
 * Images → vision data URLs (max 4); PDF → client text extract; text/code → inline.
 * No second agent loop — only builds payload for core.Service stream.
 */

/** Max image parts sent as vision/reference data URLs (VAL-CHAT-022). */
export const MAX_VISION_IMAGES = 4

/** Soft cap for inlined text/PDF extract (characters). */
export const MAX_INLINE_CHARS = 80_000

/** Soft cap for image bytes before base64 (~6 MB decoded). */
export const MAX_IMAGE_BYTES = 6 * 1024 * 1024

export type AttachmentKind = 'image' | 'pdf' | 'text' | 'code' | 'binary'

/** One staged (pre-send) or bubble (post-send) chip. */
export type ChatAttachment = {
  id: string
  name: string
  size: number
  mime: string
  kind: AttachmentKind
  /** Object URL or data URL for image thumbnail preview. */
  previewUrl?: string
  /** Vision path: data:image/...;base64,... (only images; max 4 used). */
  dataUrl?: string
  /** Inlined text for PDF / text / code. */
  textContent?: string
  /** True while FileReader / PDF extract is running. */
  loading?: boolean
  /** User-visible process error (still may keep chip). */
  error?: string
}

/** Outbound vision part accepted by core ChatRequest.images. */
export type VisionImagePart = {
  name: string
  media_type: string
  data_url: string
}

/** Built prompt fragments + vision parts ready for streamChat. */
export type AttachmentSendPayload = {
  /** Extra prompt text (PDF policy + inlined files). Empty when none. */
  inlineText: string
  /** Up to MAX_VISION_IMAGES data-URL image parts. */
  images: VisionImagePart[]
  /** Chips to show on the user bubble after send (no large data). */
  bubbleChips: BubbleAttachmentChip[]
}

/** Lightweight chip on the user message after send (VAL-CHAT-024). */
export type BubbleAttachmentChip = {
  id: string
  name: string
  size: number
  kind: AttachmentKind
  mime: string
  /** Thumbnail only for images (data URL or blob URL kept briefly). */
  previewUrl?: string
}

let attachSeq = 0
export function nextAttachmentId(): string {
  attachSeq += 1
  return `att-${attachSeq}-${Date.now()}`
}

const CODE_EXTS = new Set([
  'ts',
  'tsx',
  'js',
  'jsx',
  'mjs',
  'cjs',
  'json',
  'go',
  'py',
  'rs',
  'java',
  'kt',
  'swift',
  'c',
  'cc',
  'cpp',
  'h',
  'hpp',
  'cs',
  'rb',
  'php',
  'sh',
  'bash',
  'zsh',
  'fish',
  'ps1',
  'sql',
  'yml',
  'yaml',
  'toml',
  'ini',
  'cfg',
  'env',
  'md',
  'mdx',
  'css',
  'scss',
  'less',
  'html',
  'htm',
  'xml',
  'svg',
  'vue',
  'svelte',
  'dockerfile',
  'makefile',
  'gradle',
  'cmake',
  'r',
  'lua',
  'vim',
  'el',
])

const TEXT_EXTS = new Set([
  'txt',
  'text',
  'log',
  'csv',
  'tsv',
  'md',
  'rst',
  'org',
  'rtf',
  'nfo',
])

export function extOf(name: string): string {
  const base = name.split(/[/\\]/).pop() || name
  if (base.toLowerCase() === 'dockerfile' || base.toLowerCase() === 'makefile') {
    return base.toLowerCase()
  }
  const i = base.lastIndexOf('.')
  if (i < 0) return ''
  return base.slice(i + 1).toLowerCase()
}

export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return '0 B'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(n < 10_240 ? 1 : 0)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

/** Classify a browser File for attachment handling (VAL-CHAT-021..024). */
export function classifyAttachment(file: {
  name: string
  type?: string
  size?: number
}): AttachmentKind {
  const mime = (file.type || '').toLowerCase()
  const ext = extOf(file.name)
  if (mime.startsWith('image/') || ['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'svg'].includes(ext)) {
    return 'image'
  }
  if (mime === 'application/pdf' || ext === 'pdf') {
    return 'pdf'
  }
  if (CODE_EXTS.has(ext) || mime.includes('javascript') || mime.includes('json') || mime.includes('typescript')) {
    return 'code'
  }
  if (
    TEXT_EXTS.has(ext) ||
    mime.startsWith('text/') ||
    mime === 'application/json' ||
    mime === 'application/xml'
  ) {
    return 'text'
  }
  // Treat unknown small files as text attempt when extension is empty-ish
  if (!ext && mime === '') return 'text'
  return 'binary'
}

/** Icon label for chips (no lucide dependency in pure tests). */
export function attachmentIconLabel(kind: AttachmentKind): string {
  switch (kind) {
    case 'image':
      return 'IMG'
    case 'pdf':
      return 'PDF'
    case 'code':
      return 'CODE'
    case 'text':
      return 'TXT'
    default:
      return 'FILE'
  }
}

/**
 * PDF analysis-only policy preamble (VAL-CHAT-023).
 * Explicitly forbids inventing local shell/tools for the PDF.
 */
export function pdfAnalysisPolicy(fileName: string): string {
  const safe = fileName.replace(/[\r\n]/g, ' ').trim() || 'document.pdf'
  return (
    `[Attached PDF: ${safe} — client-side text extract via pdf.js for analysis only. ` +
    `Reason only over the extracted text below. Do not claim to open local shell, ` +
    `filesystem tools, or a PDF viewer for this file; no local shell is available for this attachment.]`
  )
}

export function truncateInline(text: string, max = MAX_INLINE_CHARS): string {
  if (text.length <= max) return text
  return text.slice(0, max) + '\n…(truncated)'
}

/** Pick up to MAX_VISION_IMAGES image attachments that have data URLs. */
export function selectVisionImages(attachments: ChatAttachment[]): ChatAttachment[] {
  const images = attachments.filter(
    (a) => a.kind === 'image' && !!a.dataUrl && !a.error && !a.loading,
  )
  return images.slice(0, MAX_VISION_IMAGES)
}

/**
 * Build prompt inline text + vision parts from ready attachments (VAL-CHAT-022..024).
 * Does not include the user's free-text prompt — caller concatenates.
 */
export function buildAttachmentPayload(attachments: ChatAttachment[]): AttachmentSendPayload {
  const ready = attachments.filter((a) => !a.loading)
  const images = selectVisionImages(ready).map((a) => ({
    name: a.name,
    media_type: a.mime || guessImageMime(a.name),
    data_url: a.dataUrl!,
  }))

  const parts: string[] = []
  for (const a of ready) {
    if (a.kind === 'image') {
      // Vision path carries pixels; add a short reference in text for models/history.
      const used = images.some((i) => i.name === a.name && i.data_url === a.dataUrl)
      parts.push(
        used
          ? `[Attached image: ${a.name} (${formatBytes(a.size)}) — included as vision/reference data URL.]`
          : `[Attached image: ${a.name} (${formatBytes(a.size)}) — not sent as vision (limit ${MAX_VISION_IMAGES} images).]`,
      )
      continue
    }
    if (a.kind === 'pdf') {
      const body = truncateInline(a.textContent || '')
      parts.push(
        `${pdfAnalysisPolicy(a.name)}\n\n### Extracted text from ${a.name}\n\`\`\`\n${body || '(no extractable text)'}\n\`\`\``,
      )
      continue
    }
    if (a.kind === 'text' || a.kind === 'code') {
      const body = truncateInline(a.textContent || '')
      const fence = a.kind === 'code' ? extOf(a.name) || '' : ''
      parts.push(
        `### Attached ${a.kind} file: ${a.name} (${formatBytes(a.size)})\n\`\`\`${fence}\n${body}\n\`\`\``,
      )
      continue
    }
    parts.push(
      `[Attached binary file: ${a.name} (${formatBytes(a.size)}, ${a.mime || 'unknown type'}) — content not inlined.]`,
    )
  }

  const bubbleChips: BubbleAttachmentChip[] = ready.map((a) => ({
    id: a.id,
    name: a.name,
    size: a.size,
    kind: a.kind,
    mime: a.mime,
    previewUrl: a.kind === 'image' ? a.previewUrl || a.dataUrl : undefined,
  }))

  return {
    inlineText: parts.join('\n\n'),
    images,
    bubbleChips,
  }
}

export function guessImageMime(name: string): string {
  const ext = extOf(name)
  switch (ext) {
    case 'jpg':
    case 'jpeg':
      return 'image/jpeg'
    case 'gif':
      return 'image/gif'
    case 'webp':
      return 'image/webp'
    case 'bmp':
      return 'image/bmp'
    case 'svg':
      return 'image/svg+xml'
    default:
      return 'image/png'
  }
}

// ---------------------------------------------------------------------------
// Clipboard paste → [Image N] token support
// ---------------------------------------------------------------------------

/**
 * Insert text at the current selection in a textarea, returning the new value
 * and new caret position. Does NOT mutate the DOM directly — caller sets state.
 */
export function insertTextAtCursor(
  textarea: HTMLTextAreaElement,
  text: string,
): { value: string; caret: number } {
  const start = textarea.selectionStart ?? textarea.value.length
  const end = textarea.selectionEnd ?? textarea.value.length
  const before = textarea.value.slice(0, start)
  const after = textarea.value.slice(end)
  // Ensure spacing around the token if not at line start.
  const needsLeadingSpace = before.length > 0 && !/\s$/.test(before)
  const token = `${needsLeadingSpace ? ' ' : ''}${text}`
  const value = `${before}${token}${after}`
  const caret = start + token.length
  return { value, caret }
}

/**
 * Regex matching [Image N] tokens (1-indexed). Captures the number.
 */
export const IMAGE_TOKEN_RE = /\[Image\s+(\d+)\]/gi

/**
 * Parse [Image N] tokens from text, returning ordered unique indices (1-based).
 * Duplicates removed, order preserved by first appearance.
 */
export function parseImageTokens(text: string): number[] {
  const seen = new Set<number>()
  const order: number[] = []
  IMAGE_TOKEN_RE.lastIndex = 0
  let m: RegExpExecArray | null
  while ((m = IMAGE_TOKEN_RE.exec(text)) !== null) {
    const n = parseInt(m[1], 10)
    if (!Number.isNaN(n) && n > 0 && !seen.has(n)) {
      seen.add(n)
      order.push(n)
    }
  }
  return order
}

/**
 * Build the ordered vision image list based on [Image N] tokens in the prompt.
 * - Token indices map to the Nth image attachment (1-based, among image kind only).
 * - Only images referenced by tokens are sent (selective vision).
 * - Images are ordered by token appearance in text (context alignment).
 * - If no tokens present, returns all ready images (legacy behavior).
 *
 * Returns { images, usedAttachmentIds } where usedAttachmentIds tracks which
 * attachments were consumed (for inline text building).
 */
export function buildTokenOrderedImages(
  promptText: string,
  imageAttachments: ChatAttachment[],
): {
  images: VisionImagePart[]
  usedAttachmentNames: Set<string>
} {
  const tokens = parseImageTokens(promptText)

  // No tokens → legacy: send all ready images in attachment order.
  if (tokens.length === 0) {
    const ready = imageAttachments.filter(
      (a) => a.kind === 'image' && !!a.dataUrl && !a.error && !a.loading,
    )
    const images = ready.slice(0, MAX_VISION_IMAGES).map((a) => ({
      name: a.name,
      media_type: a.mime || guessImageMime(a.name),
      data_url: a.dataUrl!,
    }))
    return {
      images,
      usedAttachmentNames: new Set(images.map((i) => i.name)),
    }
  }

  // Token mode: map N → attachment at index N-1 (image-only ordering).
  const ready = imageAttachments.filter(
    (a) => a.kind === 'image' && !!a.dataUrl && !a.error && !a.loading,
  )
  const images: VisionImagePart[] = []
  const usedNames = new Set<string>()
  for (const n of tokens) {
    const idx = n - 1
    if (idx < 0 || idx >= ready.length) continue
    if (idx >= MAX_VISION_IMAGES) break
    const a = ready[idx]
    if (usedNames.has(a.name)) continue
    images.push({
      name: a.name,
      media_type: a.mime || guessImageMime(a.name),
      data_url: a.dataUrl!,
    })
    usedNames.add(a.name)
  }
  return { images, usedAttachmentNames: usedNames }
}

/**
 * Build inline text for image attachments in token mode.
 * Only images NOT referenced by tokens (excess attachments) get a note.
 * Token-referenced images are described inline as part of the prompt text
 * already, so no duplicate [Attached image: ...] prose is added for them.
 */
export function buildTokenModeInlineText(
  attachments: ChatAttachment[],
  usedAttachmentNames: Set<string>,
): string {
  const parts: string[] = []
  for (const a of attachments) {
    if (a.loading) continue
    if (a.kind === 'image') {
      // Skip images that were sent as vision via tokens — no duplicate note.
      if (usedAttachmentNames.has(a.name)) continue
      parts.push(
        `[Attached image: ${a.name} (${formatBytes(a.size)}) — not referenced by [Image N] token in prompt; included as metadata only.]`,
      )
      continue
    }
    if (a.kind === 'pdf') {
      const body = truncateInline(a.textContent || '')
      parts.push(
        `${pdfAnalysisPolicy(a.name)}\n\n### Extracted text from ${a.name}\n\`\`\`\n${body || '(no extractable text)'}\n\`\`\``,
      )
      continue
    }
    if (a.kind === 'text' || a.kind === 'code') {
      const body = truncateInline(a.textContent || '')
      const fence = a.kind === 'code' ? extOf(a.name) || '' : ''
      parts.push(
        `### Attached ${a.kind} file: ${a.name} (${formatBytes(a.size)})\n\`\`\`${fence}\n${body}\n\`\`\``,
      )
      continue
    }
    parts.push(
      `[Attached binary file: ${a.name} (${formatBytes(a.size)}, ${a.mime || 'unknown type'}) — content not inlined.]`,
    )
  }
  return parts.join('\n\n')
}

/**
 * Extract image Files from a clipboard paste event.
 * Returns null if no images found (text paste should proceed normally).
 * Accepts a minimal clipboard-like object so this module stays framework-agnostic.
 */
export function extractClipboardImages(
  clipboardData: {
    items?: DataTransferItemList
    files?: FileList
  },
): File[] | null {
  const files: File[] = []

  if (clipboardData.items && clipboardData.items.length > 0) {
    for (let i = 0; i < clipboardData.items.length; i += 1) {
      const item = clipboardData.items[i]
      if (item.kind === 'file' && item.type.startsWith('image/')) {
        const file = item.getAsFile()
        if (file) files.push(file)
      }
    }
  }

  if (files.length === 0 && clipboardData.files && clipboardData.files.length > 0) {
    for (let i = 0; i < clipboardData.files.length; i += 1) {
      const f = clipboardData.files[i]
      if (f.type.startsWith('image/')) files.push(f)
    }
  }

  return files.length > 0 ? files : null
}

// ---------------------------------------------------------------------------
// Atomic [Image N] token editing
// ---------------------------------------------------------------------------

export type TokenRange = {
  start: number
  end: number
  index: number
  text: string
}

/**
 * Find all complete [Image N] token ranges in text.
 */
export function findImageTokenRanges(text: string): TokenRange[] {
  const ranges: TokenRange[] = []
  IMAGE_TOKEN_RE.lastIndex = 0
  let m: RegExpExecArray | null
  while ((m = IMAGE_TOKEN_RE.exec(text)) !== null) {
    ranges.push({
      start: m.index,
      end: m.index + m[0].length,
      index: parseInt(m[1], 10),
      text: m[0],
    })
  }
  return ranges
}

/**
 * Find a complete [Image N] token that contains or is adjacent to the cursor.
 * Returns the token range if cursor is inside, or immediately before/after it.
 * For Backspace: checks if cursor is right after a token (end === caret).
 * For Delete: checks if cursor is right before a token (start === caret).
 * For inside: start < caret < end.
 */
export function findTokenAtCaret(
  text: string,
  caret: number,
  direction: 'backspace' | 'delete' | 'inside',
): TokenRange | null {
  const ranges = findImageTokenRanges(text)
  for (const r of ranges) {
    if (direction === 'backspace') {
      if (r.end === caret || (caret > r.start && caret <= r.end)) return r
    }
    if (direction === 'delete') {
      if (r.start === caret || (caret >= r.start && caret < r.end)) return r
    }
    if (direction === 'inside') {
      if (caret > r.start && caret < r.end) return r
    }
  }
  return null
}

/**
 * Find broken/partial [Image N] fragments in text.
 * Matches incomplete tokens like "[Image 1", "[Image ", "[Imag", etc.
 * Returns ranges of the broken fragments so they can be stripped.
 */
export function findBrokenTokenFragments(text: string): TokenRange[] {
  const complete = findImageTokenRanges(text)

  const scanRe = /\[Image/gi
  let m: RegExpExecArray | null
  const ranges: TokenRange[] = []
  while ((m = scanRe.exec(text)) !== null) {
    const start = m.index
    const isComplete = complete.some((r) => r.start === start)
    if (isComplete) continue

    let end = start + 1
    const rest = text.slice(start + 1)
    const imageMatch = /^Image\s*/i.exec(rest)
    if (imageMatch) {
      end += imageMatch[0].length
      const digitMatch = /^\d+/.exec(text.slice(end))
      if (digitMatch) end += digitMatch[0].length
      if (text[end] === ']') end += 1
    }
    ranges.push({
      start,
      end,
      index: -1,
      text: text.slice(start, end),
    })
  }
  return ranges
}

/**
 * Given old text and new text, detect which [Image N] tokens were removed.
 * Returns the set of image indices (1-based) that were present in old but
 * missing (or broken) in new.
 */
export function detectRemovedTokens(oldText: string, newText: string): number[] {
  const oldTokens = parseImageTokens(oldText)
  const newTokens = new Set(parseImageTokens(newText))
  return oldTokens.filter((n) => !newTokens.has(n))
}

/**
 * Strip broken [Image fragments from text, returning clean text.
 */
export function stripBrokenTokenFragments(text: string): string {
  const broken = findBrokenTokenFragments(text)
  if (broken.length === 0) return text
  let result = text
  for (let i = broken.length - 1; i >= 0; i -= 1) {
    const r = broken[i]
    result = result.slice(0, r.start) + result.slice(r.end)
  }
  return result
}

/**
 * Renumber [Image N] tokens sequentially (1, 2, 3...) in order of appearance.
 * Used after token deletion to keep indices matching attachment order.
 */
export function renumberImageTokens(text: string): string {
  const ranges = findImageTokenRanges(text)
  if (ranges.length === 0) return text
  const sorted = [...ranges].sort((a, b) => a.start - b.start)
  let result = text
  for (let i = sorted.length - 1; i >= 0; i -= 1) {
    const r = sorted[i]
    const replacement = `[Image ${i + 1}]`
    result = result.slice(0, r.start) + replacement + result.slice(r.end)
  }
  return result
}

export function removeAttachmentById(
  list: ChatAttachment[],
  id: string,
): ChatAttachment[] {
  return list.filter((a) => a.id !== id)
}

/**
 * Compose the final user prompt: free text + attachment inline bodies.
 */
export function composePromptWithAttachments(
  userText: string,
  payload: AttachmentSendPayload,
): string {
  const t = userText.trim()
  if (!payload.inlineText) return t
  if (!t) return payload.inlineText
  return `${t}\n\n${payload.inlineText}`
}

/** Whether the composer can send with current input + attachments. */
export function canSendWithAttachments(
  input: string,
  attachments: ChatAttachment[],
  mentions: string[],
): boolean {
  if (attachments.some((a) => a.loading)) return false
  if (input.trim()) return true
  if (mentions.length > 0) return true
  if (attachments.some((a) => !a.error || a.kind === 'image' || a.textContent)) return true
  return false
}

/** Read a File as a data URL (browser). */
export function readFileAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result || ''))
    reader.onerror = () => reject(reader.error || new Error('read failed'))
    reader.readAsDataURL(file)
  })
}

/** Read a File as UTF-8 text (browser). */
export function readFileAsText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result || ''))
    reader.onerror = () => reject(reader.error || new Error('read failed'))
    reader.readAsText(file)
  })
}

/**
 * Extract text from a PDF ArrayBuffer via pdfjs-dist (dynamic import).
 * Returns empty string on failure (caller can still show chip + error).
 */
export async function extractPdfText(data: ArrayBuffer): Promise<string> {
  // Dynamic import so unit tests without the package still load pure helpers.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let pdfjs: any
  try {
    pdfjs = await import('pdfjs-dist')
  } catch (e) {
    throw new Error(
      `pdf.js not available: ${e instanceof Error ? e.message : String(e)}`,
    )
  }
  // Vite-friendly worker URL; if the import fails we still try getDocument
  // (some builds use a fake worker in the main thread).
  try {
    // @vite-ignore — resolved by Vite for web build; harmless in node tests.
    const workerMod = await import(
      /* @vite-ignore */ 'pdfjs-dist/build/pdf.worker.mjs?url'
    )
    if (workerMod?.default) {
      pdfjs.GlobalWorkerOptions.workerSrc = workerMod.default as string
    }
  } catch {
    try {
      if (pdfjs.GlobalWorkerOptions) {
        pdfjs.GlobalWorkerOptions.workerSrc = new URL(
          'pdfjs-dist/build/pdf.worker.mjs',
          import.meta.url,
        ).toString()
      }
    } catch {
      /* main-thread fallback when worker URL is unavailable */
    }
  }

  const loadingTask = pdfjs.getDocument({ data: new Uint8Array(data) })
  const pdf = await loadingTask.promise
  const pages: string[] = []
  const maxPages = Math.min(pdf.numPages, 40)
  for (let i = 1; i <= maxPages; i += 1) {
    const page = await pdf.getPage(i)
    const content = await page.getTextContent()
    const line = (content.items as Array<{ str?: string }>)
      .map((item) => String(item?.str || ''))
      .join(' ')
    if (line.trim()) pages.push(line)
  }
  if (pdf.numPages > maxPages) {
    pages.push(`…(${pdf.numPages - maxPages} further pages omitted)`)
  }
  return pages.join('\n\n')
}

/**
 * Process a browser File into a ChatAttachment (async I/O).
 * Safe to call from composer paperclip handler.
 */
export async function processAttachmentFile(file: File): Promise<ChatAttachment> {
  const id = nextAttachmentId()
  const kind = classifyAttachment(file)
  const base: ChatAttachment = {
    id,
    name: file.name || 'attachment',
    size: file.size || 0,
    mime: file.type || '',
    kind,
  }

  if (kind === 'image') {
    if (file.size > MAX_IMAGE_BYTES) {
      return {
        ...base,
        error: `Image too large (max ${formatBytes(MAX_IMAGE_BYTES)})`,
      }
    }
    try {
      const dataUrl = await readFileAsDataURL(file)
      return {
        ...base,
        mime: file.type || guessImageMime(file.name),
        dataUrl,
        previewUrl: dataUrl,
      }
    } catch (e) {
      return { ...base, error: e instanceof Error ? e.message : String(e) }
    }
  }

  if (kind === 'pdf') {
    try {
      const buf = await file.arrayBuffer()
      const text = await extractPdfText(buf)
      return {
        ...base,
        mime: file.type || 'application/pdf',
        textContent: truncateInline(text),
        error: text.trim() ? undefined : 'No extractable text in PDF',
      }
    } catch (e) {
      return {
        ...base,
        mime: file.type || 'application/pdf',
        error: e instanceof Error ? e.message : String(e),
      }
    }
  }

  if (kind === 'text' || kind === 'code') {
    try {
      const text = await readFileAsText(file)
      return {
        ...base,
        textContent: truncateInline(text),
      }
    } catch (e) {
      return { ...base, error: e instanceof Error ? e.message : String(e) }
    }
  }

  // Binary: keep chip for user visibility only.
  return {
    ...base,
    error: 'Binary file cannot be inlined; attach text/image/PDF instead',
  }
}
