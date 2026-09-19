import { fetchPlaygroundImage } from '../../lib/api'

/** True-format image download helpers used by Image Studio. */

const EXT_BY_MIME: Record<string, string> = {
  'image/png': 'png',
  'image/apng': 'apng',
  'image/jpeg': 'jpg',
  'image/jpg': 'jpg',
  'image/webp': 'webp',
  'image/gif': 'gif',
  'image/svg+xml': 'svg',
  'image/bmp': 'bmp',
  'image/x-ms-bmp': 'bmp',
  'image/avif': 'avif',
  'image/heic': 'heic',
  'image/heif': 'heif',
  'image/tiff': 'tiff',
  'image/x-icon': 'ico',
  'image/vnd.microsoft.icon': 'ico',
}

const MIME_BY_EXT: Record<string, string> = {
  png: 'image/png',
  apng: 'image/apng',
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  webp: 'image/webp',
  gif: 'image/gif',
  svg: 'image/svg+xml',
  bmp: 'image/bmp',
  avif: 'image/avif',
  heic: 'image/heic',
  heif: 'image/heif',
  tif: 'image/tiff',
  tiff: 'image/tiff',
  ico: 'image/x-icon',
}

/** Normalize `Image/PNG; charset=x` → `image/png`. */
export function normalizeImageMime(raw: string): string {
  return (raw || '').split(';')[0].trim().toLowerCase()
}

/** `data:image/webp;base64,…` → `image/webp` (empty when not a data URL). */
export function imageMimeFromDataUrl(url: string): string {
  const m = /^data:([a-zA-Z0-9.+-]+\/[a-zA-Z0-9.+-]+)\s*[;,]/.exec((url || '').trim())
  return m ? normalizeImageMime(m[1]) : ''
}

/** File extension for a media type. Non-image types fall back to `bin`. */
export function imageExtFromMime(mime: string): string {
  const m = normalizeImageMime(mime)
  const known = EXT_BY_MIME[m]
  if (known) return known
  if (!m.startsWith('image/')) return 'bin'
  const sub = m.slice('image/'.length).split('+')[0].replace(/[^a-z0-9]/g, '')
  return sub || 'bin'
}

/** Media type guessed from a URL path extension (empty when unknown). */
export function imageMimeFromUrl(url: string): string {
  const raw = (url || '').trim()
  if (!raw || raw.startsWith('data:')) return ''
  let path = raw
  try {
    path = new URL(raw, 'http://local.invalid').pathname
  } catch {
    path = raw.split('?')[0].split('#')[0]
  }
  const m = /\.([a-z0-9]{2,5})$/i.exec(path)
  if (!m) return ''
  return MIME_BY_EXT[m[1].toLowerCase()] || ''
}

function startsWith(bytes: Uint8Array, sig: number[], offset = 0): boolean {
  if (bytes.length < offset + sig.length) return false
  for (let i = 0; i < sig.length; i += 1) {
    if (bytes[offset + i] !== sig[i]) return false
  }
  return true
}

function ascii(bytes: Uint8Array, offset: number, length: number): string {
  let out = ''
  const end = Math.min(bytes.length, offset + length)
  for (let i = offset; i < end; i += 1) out += String.fromCharCode(bytes[i])
  return out
}

/**
 * Media type sniffed from the leading bytes. This is the only source that
 * cannot lie about the payload, so callers should prefer it.
 */
export function imageMimeFromBytes(bytes: Uint8Array): string {
  if (!bytes || bytes.length < 4) return ''
  if (startsWith(bytes, [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])) {
    return 'image/png'
  }
  if (startsWith(bytes, [0xff, 0xd8, 0xff])) return 'image/jpeg'
  if (startsWith(bytes, [0x47, 0x49, 0x46, 0x38])) return 'image/gif'
  if (startsWith(bytes, [0x42, 0x4d])) return 'image/bmp'
  if (startsWith(bytes, [0x00, 0x00, 0x01, 0x00])) return 'image/x-icon'
  if (startsWith(bytes, [0x49, 0x49, 0x2a, 0x00])) return 'image/tiff'
  if (startsWith(bytes, [0x4d, 0x4d, 0x00, 0x2a])) return 'image/tiff'
  if (startsWith(bytes, [0x52, 0x49, 0x46, 0x46]) && ascii(bytes, 8, 4) === 'WEBP') {
    return 'image/webp'
  }
  if (ascii(bytes, 4, 4) === 'ftyp') {
    const brand = ascii(bytes, 8, 4).toLowerCase()
    if (brand.startsWith('avif') || brand.startsWith('avis')) return 'image/avif'
    if (brand.startsWith('heic') || brand.startsWith('heix')) return 'image/heic'
    if (brand.startsWith('mif1') || brand.startsWith('msf1')) return 'image/heif'
  }
  const head = ascii(bytes, 0, 300).trimStart().toLowerCase()
  if (head.startsWith('<svg') || (head.startsWith('<?xml') && head.includes('<svg'))) {
    return 'image/svg+xml'
  }
  return ''
}

/** Human label for the details strip: `image/jpeg` → `JPG`. */
export function imageFormatLabel(mime: string): string {
  const ext = imageExtFromMime(mime)
  return ext === 'bin' ? '' : ext.toUpperCase()
}

function sanitizeStem(raw: string, fallback: string): string {
  const s = (raw || '')
    .replace(/[<>:"/\\|?*\u0000-\u001f]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
  if (!s) return fallback
  const stem = s.slice(0, 80).replace(/[. ]+$/g, '')
  return stem || fallback
}

/** `Rainy alley` + `image/webp` → `Rainy alley.webp`. */
export function imageDownloadName(
  title: string,
  mime: string,
  suffix?: string,
): string {
  const stem = sanitizeStem(title, 'image')
  const tail = suffix ? ` ${sanitizeStem(suffix, '')}`.trimEnd() : ''
  return `${stem}${tail}.${imageExtFromMime(mime)}`
}

function decodeDataUrl(url: string): { bytes: Uint8Array; mime: string } {
  const comma = url.indexOf(',')
  if (comma < 0) throw new Error('malformed data URL')
  const head = url.slice(5, comma)
  const body = url.slice(comma + 1)
  const mime = normalizeImageMime(head.split(';')[0]) || 'application/octet-stream'
  if (/;base64/i.test(head)) {
    const clean = body.replace(/\s+/g, '')
    const bin = atob(clean)
    const bytes = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; i += 1) bytes[i] = bin.charCodeAt(i)
    return { bytes, mime }
  }
  const text = decodeURIComponent(body)
  const bytes = new TextEncoder().encode(text)
  return { bytes, mime }
}

export type ImageDownloadResult = {
  filename: string
  mime: string
  bytes: number
}

function triggerAnchor(href: string, filename: string): void {
  const a = document.createElement('a')
  a.href = href
  a.download = filename
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  a.remove()
}

/**
 * Download `url` with its true extension. Same-origin generated-asset routes
 * remain direct; external HTTP(S) bytes go through the guarded hub proxy so
 * image hosts without CORS support still download reliably.
 */
export async function downloadImageUrl(
  url: string,
  title: string,
  suffix?: string,
): Promise<ImageDownloadResult> {
  const src = (url || '').trim()
  if (!src) throw new Error('no image to download')

  let bytes: Uint8Array
  let declaredMime = ''
  if (src.startsWith('data:')) {
    const decoded = decodeDataUrl(src)
    bytes = decoded.bytes
    declaredMime = decoded.mime
  } else {
    const resolved = new URL(src, window.location.href)
    const externalHttp =
      (resolved.protocol === 'https:' || resolved.protocol === 'http:') &&
      resolved.origin !== window.location.origin
    let blob: Blob
    if (externalHttp) {
      blob = await fetchPlaygroundImage(resolved.href)
    } else {
      const res = await fetch(src, {
        credentials: 'same-origin',
        cache: 'no-store',
      })
      if (!res.ok) {
        throw new Error(
          `HTTP ${res.status}${res.statusText ? ` ${res.statusText}` : ''}`,
        )
      }
      blob = await res.blob()
    }
    if (!blob.size) throw new Error('empty image response')
    declaredMime = normalizeImageMime(blob.type)
    bytes = new Uint8Array(await blob.arrayBuffer())
  }

  const sniffedMime = imageMimeFromBytes(bytes)
  const mime =
    sniffedMime ||
    (declaredMime.startsWith('image/') ? declaredMime : '') ||
    imageMimeFromUrl(src)
  if (!mime) throw new Error('response was not a recognized image')
  const filename = imageDownloadName(title, mime, suffix)
  const payload = bytes.buffer.slice(
    bytes.byteOffset,
    bytes.byteOffset + bytes.byteLength,
  ) as ArrayBuffer
  const objectUrl = URL.createObjectURL(new Blob([payload], { type: mime }))
  try {
    triggerAnchor(objectUrl, filename)
  } finally {
    window.setTimeout(() => URL.revokeObjectURL(objectUrl), 10_000)
  }
  return { filename, mime, bytes: bytes.length }
}
