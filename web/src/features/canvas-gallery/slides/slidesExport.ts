import type { SlideMeta } from './htmlDeck'
import { SLIDE_H, SLIDE_W, previewSrcDoc } from './htmlDeck'

const PPTX_W = 13.333
const PPTX_H = 7.5

export type PptxExportMode = 'hybrid' | 'visual'

export type ExportOverlayRole =
  | 'title'
  | 'subtitle'
  | 'body'
  | 'label'
  | 'bullet'
  | 'number'
  | 'list-item'
  | 'stat'
  | 'caption'
  | 'card'
  | 'shape'
  | 'image'
  | 'table'
  | 'table-cell'

type MeasuredBox = {
  role: ExportOverlayRole
  id: string
  text: string
  x: number
  y: number
  w: number
  h: number
  color: string
  bg: string
  border: string
  weight: number
  sizePx: number
  lineHeightPx: number
  align: 'left' | 'center' | 'right'
  valign: 'top' | 'middle' | 'bottom'
  font: string
  radius: number
  opacity: number
  hideOnCapture: boolean
  /**
   * Surfaces whose paint a PowerPoint shape cannot reproduce (gradients,
   * background images). Neither reconstructed nor hidden, so the plate keeps
   * them exactly as the preview shows.
   */
  keepInPlate?: boolean
  zIndex: number
  /**
   * The element this box was measured from. The plate capture hides exactly
   * these nodes — nothing else — so anything we failed to reconstruct stays
   * visible in the background image instead of vanishing from the deck.
   */
  el: HTMLElement
  parentId?: string
  src?: string
  objectFit?: 'contain' | 'cover' | 'fill'
  index?: number
  row?: number
  col?: number
}

type SlideCapture = {
  plateDataUrl: string
  overlays: MeasuredBox[]
}

const TEXT_ROLES: Partial<Record<ExportOverlayRole, true>> = {
  title: true,
  subtitle: true,
  body: true,
  label: true,
  bullet: true,
  number: true,
  'list-item': true,
  stat: true,
  caption: true,
}

function layerRank(role: ExportOverlayRole): number {
  switch (role) {
    case 'image':
      return 10
    case 'card':
    case 'shape':
      return 20
    case 'table':
    case 'table-cell':
      return 30
    case 'title':
    case 'subtitle':
    case 'body':
    case 'label':
    case 'bullet':
    case 'number':
    case 'list-item':
    case 'stat':
    case 'caption':
      return 50
    default:
      return 40
  }
}

function safeFileName(name: string): string {
  return (name || 'slides')
    .replace(/[\\/:*?"<>|]+/g, '-')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, 80)
}

function sanitizeDownloadSegment(raw: string, fallback = 'Playground'): string {
  const s = (raw || '')
    .replace(/[<>:"/\\|?*\u0000-\u001f]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
  if (!s) return fallback
  return s.slice(0, 80).replace(/[. ]+$/g, '') || fallback
}

function wait(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms))
}

function pxToInX(px: number): number {
  return (px / SLIDE_W) * PPTX_W
}

function pxToInY(px: number): number {
  return (px / SLIDE_H) * PPTX_H
}

/**
 * The 1280×720 artboard maps onto a 13.333×7.5in slide, i.e. exactly 96 px/in
 * on both axes. A point is 1/72in, so one artboard pixel is 72/96 = 0.75pt.
 * The previous 0.72 factor rendered every string ~4% small, which is what made
 * exported text visibly disagree with the preview.
 */
const PX_PER_INCH = SLIDE_W / PPTX_W
const PT_PER_PX = 72 / PX_PER_INCH

function pxToPt(px: number): number {
  const n = px * PT_PER_PX
  if (!Number.isFinite(n) || n <= 0) return 12
  return Math.max(6, Math.round(n * 10) / 10)
}

const PPT_SAFE_FONTS = new Set([
  'arial',
  'helvetica',
  'calibri',
  'georgia',
  'verdana',
  'trebuchet ms',
  'segoe ui',
  'times new roman',
  'times',
  'courier new',
  'comic sans ms',
])

function mapPptFont(font: string): string {
  const raw = (font || '').trim()
  if (!raw) return 'Arial'
  const first = raw.split(',')[0]?.replace(/['"]/g, '').trim() || 'Arial'
  const lower = first.toLowerCase()
  if (PPT_SAFE_FONTS.has(lower)) {
    if (lower === 'times') return 'Times New Roman'
    if (lower === 'helvetica') return 'Arial'
    return first
  }
  return 'Arial'
}

function normalizeHex(input: string, fallback = 'FFFFFF'): string {
  const s = (input || '').trim()
  if (!s || s === 'none' || s === 'transparent') return fallback
  const hex = s.match(/#([0-9a-fA-F]{3,8})/)
  if (hex) {
    let h = hex[1]
    if (h.length === 3) {
      h = h
        .split('')
        .map((c) => c + c)
        .join('')
    }
    return h.slice(0, 6).toUpperCase()
  }
  const rgb = s.match(/rgba?\(\s*([\d.]+)\s*,\s*([\d.]+)\s*,\s*([\d.]+)/i)
  if (rgb) {
    const to = (n: string) =>
      Math.max(0, Math.min(255, Math.round(Number(n))))
        .toString(16)
        .padStart(2, '0')
    return `${to(rgb[1])}${to(rgb[2])}${to(rgb[3])}`.toUpperCase()
  }
  return fallback
}

function parseOpacity(input: string, fallback = 1): number {
  const n = Number(input)
  if (Number.isFinite(n)) return Math.max(0, Math.min(1, n))
  return fallback
}

function cssColorToHex(
  color: string,
  fallback = 'FFFFFF',
): { hex: string; alpha: number } {
  const s = (color || '').trim()
  if (!s || s === 'transparent') return { hex: fallback, alpha: 0 }
  const hex = s.match(/#([0-9a-fA-F]{3,8})/)
  if (hex) {
    let h = hex[1]
    if (h.length === 3) {
      h = h
        .split('')
        .map((c) => c + c)
        .join('')
    }
    if (h.length === 8) {
      const a = parseInt(h.slice(6, 8), 16) / 255
      return { hex: h.slice(0, 6).toUpperCase(), alpha: a }
    }
    return { hex: h.slice(0, 6).toUpperCase(), alpha: 1 }
  }
  const rgba = s.match(
    /rgba?\(\s*([\d.]+)\s*,\s*([\d.]+)\s*,\s*([\d.]+)(?:\s*,\s*([\d.]+))?\s*\)/i,
  )
  if (rgba) {
    const to = (n: string) =>
      Math.max(0, Math.min(255, Math.round(Number(n))))
        .toString(16)
        .padStart(2, '0')
    const alpha =
      rgba[4] !== undefined ? Math.max(0, Math.min(1, Number(rgba[4]))) : 1
    return {
      hex: `${to(rgba[1])}${to(rgba[2])}${to(rgba[3])}`.toUpperCase(),
      alpha,
    }
  }
  return { hex: fallback, alpha: 1 }
}

async function waitForRender(
  doc: Document,
  root: HTMLElement,
  view: Window,
  timeoutMs = 8000,
): Promise<void> {
  const deadline = Date.now() + timeoutMs

  // 1) Tailwind Play CDN applies utilities asynchronously by injecting a
  //    <style>. Probe with a sentinel that only resolves once `.hidden`
  //    (a core utility) is generated — otherwise we capture unstyled boxes.
  const probe = doc.createElement('div')
  probe.className = 'hidden'
  probe.setAttribute('aria-hidden', 'true')
  probe.style.position = 'absolute'
  probe.style.left = '-9999px'
  root.appendChild(probe)
  try {
    let tailwindReady = false
    while (Date.now() < deadline) {
      if (view.getComputedStyle(probe).display === 'none') {
        tailwindReady = true
        break
      }
      await wait(50)
    }
    if (!tailwindReady) {
      throw new Error(
        'Slide styles did not finish loading (Tailwind runtime timed out). Check network access for the CDN, then retry export.',
      )
    }
  } finally {
    probe.remove()
  }

  // 2) Fonts: pt sizing and line wrapping depend on the real metrics.
  try {
    const fonts = (doc as Document & { fonts?: FontFaceSet }).fonts
    if (fonts?.ready) {
      await Promise.race([fonts.ready, wait(Math.max(0, deadline - Date.now()))])
    }
  } catch {
    /* fonts API unavailable — proceed */
  }

  // 3) Pictures. Two failure modes must be reported, not swallowed: the
  //    gateway's image host sends no CORS headers (so html-to-image's own
  //    fetch is blocked and the picture silently disappears from the plate),
  //    and a proxied blob can still fail to decode. Neither is visible in the
  //    capture's dimensions — a slide missing its photo is still 2560×1440 —
  //    so each one raises an actionable error naming the image.
  //
  //    Inlining replaces the src only; width/height and object-fit are
  //    untouched, so a CSS `cover` crop still renders exactly as authored.
  //    Only the offscreen capture DOM is rewritten; the on-screen preview
  //    keeps using the original <img src>.
  const imgs = Array.from(root.querySelectorAll('img')) as HTMLImageElement[]
  const shortSrc = (s: string) => (s.length > 80 ? `${s.slice(0, 77)}…` : s)

  await Promise.all(
    imgs.map(async (img) => {
      const src = img.currentSrc || img.src
      if (!src || src.startsWith('data:') || src.startsWith('blob:')) return
      let sameOrigin = false
      try {
        sameOrigin = new URL(src, doc.baseURI).origin === window.location.origin
      } catch {
        sameOrigin = false
      }
      if (sameOrigin) return
      try {
        const { fetchPlaygroundImage } = await import('../../../lib/api')
        const blob = await fetchPlaygroundImage(src)
        img.src = await new Promise<string>((resolve, reject) => {
          const fr = new FileReader()
          fr.onload = () => resolve(String(fr.result))
          fr.onerror = () => reject(new Error('the image bytes were unreadable'))
          fr.readAsDataURL(blob)
        })
      } catch (err) {
        const why = err instanceof Error ? err.message : String(err)
        throw new Error(
          `the image ${shortSrc(src)} could not be loaded for export (${why}). It is on another origin, so exporting it needs the image proxy to reach that host.`,
        )
      }
    }),
  )

  // 4) Decode every <img> so pictures export at full resolution and the plate
  //    is not captured mid-load. A picture that never decodes would vanish
  //    from the exported slide, so time-outs and errors both fail the export.
  await Promise.all(
    imgs.map(async (img) => {
      if (img.complete && img.naturalWidth > 0) return
      const budget = Math.max(0, deadline - Date.now())
      const outcome = await Promise.race([
        new Promise<'ok' | 'error'>((resolve) => {
          img.addEventListener('load', () => resolve('ok'), { once: true })
          img.addEventListener('error', () => resolve('error'), { once: true })
          if (typeof img.decode === 'function') {
            img.decode().then(
              () => resolve('ok'),
              () => resolve('error'),
            )
          }
        }),
        wait(budget).then(() => 'timeout' as const),
      ])
      if (outcome === 'ok' || (img.complete && img.naturalWidth > 0)) return
      const src = shortSrc(img.currentSrc || img.src || '(no src)')
      throw new Error(
        outcome === 'timeout'
          ? `the image ${src} did not finish loading in time`
          : `the image ${src} failed to decode`,
      )
    }),
  )

  // 5) Two rAF ticks so any final layout/paint settles before measuring.
  await new Promise<void>((resolve) => {
    const raf =
      view.requestAnimationFrame?.bind(view) ||
      ((cb: FrameRequestCallback) => view.setTimeout(() => cb(0), 16))
    raf(() => raf(() => resolve()))
  })
}

async function openSlideDoc(
  fullHtml: string,
  slideId: string,
): Promise<{ host: HTMLDivElement; doc: Document; root: HTMLElement }> {
  const srcDoc = previewSrcDoc(fullHtml, slideId, { thumb: true })
  const host = document.createElement('div')
  host.setAttribute('aria-hidden', 'true')
  host.style.cssText =
    'position:fixed;left:-10000px;top:0;width:1280px;height:720px;opacity:0;pointer-events:none;z-index:-1;overflow:hidden;'
  const iframe = document.createElement('iframe')
  iframe.setAttribute('sandbox', 'allow-scripts allow-same-origin')
  iframe.style.cssText = `width:${SLIDE_W}px;height:${SLIDE_H}px;border:0;background:transparent;`
  host.appendChild(iframe)
  document.body.appendChild(host)

  try {
    const doc = iframe.contentDocument
    if (!doc) {
      throw new Error('the export frame could not be created')
    }
    doc.open()
    doc.write(srcDoc)
    doc.close()

    // document.write is synchronous, but the Tailwind CDN <script> in the
    // preview head blocks the parser, so <body> — and therefore .slide — does
    // not exist yet at this point. Poll until the parser reaches the artboard
    // rather than querying immediately (which reported a bogus "no .slide
    // element" failure) or sleeping blindly.
    const deadline = Date.now() + 15000
    let root: HTMLElement | null = null
    for (;;) {
      root =
        (doc.querySelector('.slide') as HTMLElement | null) ||
        (doc.querySelector('section[data-slide-id]') as HTMLElement | null) ||
        (doc.querySelector('section') as HTMLElement | null)
      if (root) break
      if (doc.readyState === 'complete') break
      if (Date.now() > deadline) {
        throw new Error(
          'the slide did not finish loading in time (the Tailwind runtime may be unreachable)',
        )
      }
      await wait(25)
    }
    if (!root) {
      throw new Error('no .slide element was found in the deck HTML')
    }

    await waitForRender(doc, root, doc.defaultView || window)
    return { host, doc, root }
  } catch (err) {
    host.remove()
    throw err
  }
}

/**
 * Rasterizes the artboard and verifies the result actually covers it.
 *
 * Byte length is not a validity signal — a legitimately flat slide compresses
 * to very little. What does signal failure is a decoded bitmap that is not the
 * artboard's size, which is how html-to-image reports a collapsed or
 * placeholder render.
 */
async function toPngOf(root: HTMLElement): Promise<string> {
  // Dynamic: html-to-image is a large rasterizer only needed when the user
  // actually exports, so it must stay out of the main Playground bundle.
  const { toPng } = await import('html-to-image')
  const pixelRatio = 2
  const png = await toPng(root, {
    width: SLIDE_W,
    height: SLIDE_H,
    // 2x keeps text in the plate crisp on a 1920-wide projector without
    // ballooning the .pptx; the deck is 1280 CSS px wide.
    pixelRatio,
    cacheBust: true,
    style: {
      transform: 'none',
      margin: '0',
      // The offscreen capture frame renders the artboard flush, so the
      // preview's presentation chrome must not bleed into the picture.
      boxShadow: 'none',
      borderRadius: '0',
    },
  })
  if (!png || !png.startsWith('data:image/')) {
    throw new Error('the slide renderer returned no image data')
  }

  const probe = new Image()
  probe.src = png
  await new Promise<void>((resolve, reject) => {
    probe.onload = () => resolve()
    probe.onerror = () => reject(new Error('the captured image failed to decode'))
  })
  const expectedW = SLIDE_W * pixelRatio
  const expectedH = SLIDE_H * pixelRatio
  // Allow a pixel of rounding slack, reject a collapsed or partial capture.
  if (
    Math.abs(probe.naturalWidth - expectedW) > 2 ||
    Math.abs(probe.naturalHeight - expectedH) > 2
  ) {
    throw new Error(
      `the capture came out ${probe.naturalWidth}×${probe.naturalHeight} instead of ${expectedW}×${expectedH}`,
    )
  }
  return png
}

function attr(el: Element, name: string): string {
  return (el.getAttribute(name) || '').trim()
}

/**
 * `innerText` reports `<br>` and block boundaries as `\n`. Collapsing with
 * `\s+` destroyed those breaks, so PowerPoint re-flowed authored multi-line
 * text into one wrapping run.
 */
function textOf(el: HTMLElement): string {
  return (el.innerText || el.textContent || '')
    .replace(/\r\n?/g, '\n')
    .split('\n')
    .map((line) => line.replace(/[^\S\n]+/g, ' ').trim())
    .filter(Boolean)
    .join('\n')
}

function flatText(s: string): string {
  return s.replace(/\s+/g, ' ').trim()
}

function roleOf(el: Element): ExportOverlayRole | null {
  const raw = attr(el, 'data-ex').toLowerCase()
  const allowed: ExportOverlayRole[] = [
    'title',
    'subtitle',
    'body',
    'label',
    'bullet',
    'number',
    'list-item',
    'stat',
    'caption',
    'card',
    'shape',
    'image',
    'table',
    'table-cell',
  ]
  if ((allowed as string[]).includes(raw)) return raw as ExportOverlayRole
  return null
}

function measureElement(
  el: HTMLElement,
  root: HTMLElement,
  role: ExportOverlayRole,
  idFallback: string,
  view: Window,
): MeasuredBox | null {
  const rootRect = root.getBoundingClientRect()
  const r = el.getBoundingClientRect()
  const x = r.left - rootRect.left
  const y = r.top - rootRect.top
  const w = r.width
  const h = r.height
  if (w < 2 || h < 2) return null
  if (x + w < 0 || y + h < 0 || x > SLIDE_W || y > SLIDE_H) return null

  const cs = view.getComputedStyle(el)
  const colorAttr = attr(el, 'data-ex-color')
  const bgAttr = attr(el, 'data-ex-bg')
  const borderAttr = attr(el, 'data-ex-border')
  const weightAttr = attr(el, 'data-ex-weight')
  const sizeAttr = attr(el, 'data-ex-size')
  const alignAttr = attr(el, 'data-ex-align').toLowerCase()
  const valignAttr = attr(el, 'data-ex-valign').toLowerCase()
  const fontAttr = attr(el, 'data-ex-font')
  const radiusAttr = attr(el, 'data-ex-radius')
  const opacityAttr = attr(el, 'data-ex-opacity')
  const hideAttr = attr(el, 'data-ex-hide-on-capture')

  const colorCss = cssColorToHex(colorAttr || cs.color, '1F2937')
  const bgCss = cssColorToHex(
    bgAttr || (role === 'card' || role === 'shape' ? cs.backgroundColor : 'transparent'),
    role === 'card' || role === 'shape' ? '1E293B' : '000000',
  )
  const borderCss = cssColorToHex(
    borderAttr || cs.borderColor,
    '334155',
  )

  let text = ''
  if (
    role !== 'card' &&
    role !== 'shape' &&
    role !== 'image' &&
    role !== 'table'
  ) {
    text = textOf(el)
  }

  // A display:none / aria-hidden subtree has no box to measure; measuring it
  // anyway produced a zero-size or stale overlay, and hiding it for the plate
  // is a no-op. Skip rather than emit a phantom PPTX object.
  if (role !== 'card' && role !== 'shape' && role !== 'image') {
    if (cs.display === 'none' || cs.visibility === 'hidden') return null
  }

  let src: string | undefined
  let objectFit: MeasuredBox['objectFit']
  if (role === 'image') {
    const img =
      el.tagName.toLowerCase() === 'img'
        ? (el as HTMLImageElement)
        : (el.querySelector('img') as HTMLImageElement | null)
    src = img?.currentSrc || img?.src || attr(el, 'src') || undefined
    if (src && src.startsWith('about:')) src = undefined
    // The picture's own object-fit decides how PowerPoint must size it:
    // `cover` means the browser cropped to fill, and exporting that as
    // `contain` would letterbox an image the preview shows edge-to-edge.
    const fit = img ? view.getComputedStyle(img).objectFit : cs.objectFit
    objectFit = fit === 'cover' || fit === 'fill' ? fit : 'contain'
  }

  const align: MeasuredBox['align'] =
    alignAttr === 'center' || alignAttr === 'right'
      ? (alignAttr as 'center' | 'right')
      : cs.textAlign === 'center' || cs.textAlign === 'right'
        ? (cs.textAlign as 'center' | 'right')
        : 'left'

  const valign: MeasuredBox['valign'] = (() => {
    if (valignAttr === 'top' || valignAttr === 'middle' || valignAttr === 'bottom') {
      return valignAttr
    }
    const ai = cs.alignItems
    const ji = cs.justifyContent
    const va = cs.verticalAlign
    if (ai === 'center' || ji === 'center' || va === 'middle') return 'middle'
    if (ai === 'flex-end' || ji === 'flex-end' || va === 'bottom') return 'bottom'
    // `stretch`/`flex-start`/`normal` and the block default all place text at
    // the top of the measured frame. Defaulting to 'middle' re-centred every
    // ordinary heading and paragraph vertically inside its own box, which the
    // preview never showed.
    return 'top'
  })()

  const sizePx = sizeAttr
    ? Number(sizeAttr)
    : parseFloat(cs.fontSize) || 16
  // `normal` resolves to roughly 1.2em for the fonts we ship.
  const lineHeightPx = parseFloat(cs.lineHeight) || sizePx * 1.2
  const weight = weightAttr
    ? Number(weightAttr)
    : parseInt(cs.fontWeight, 10) || 400
  const radius = radiusAttr
    ? Number(radiusAttr)
    : parseFloat(cs.borderRadius) || 0
  const opacity = opacityAttr
    ? parseOpacity(opacityAttr, 1)
    : bgCss.alpha < 1
      ? bgCss.alpha
      : 1

  // A gradient / background-image / backdrop-filter cannot be expressed as a
  // pptxgenjs solid fill. Reconstructing the surface would silently drop the
  // paint, and hiding it would delete it from the plate too — the export then
  // shows a flat card where the preview showed a gradient. Keep the whole
  // element in the captured plate instead and emit no native shape.
  const surfacePaint =
    role === 'card' || role === 'shape'
      ? cs.backgroundImage !== 'none' ||
        (cs.backdropFilter && cs.backdropFilter !== 'none') ||
        cs.backgroundBlendMode !== 'normal'
      : false

  const hideDefault =
    role !== 'card' && role !== 'shape' && role !== 'image' && role !== 'table'
  const hideOnCapture =
    surfacePaint
      ? false
      : hideAttr === '0' || hideAttr === 'false'
        ? false
        : hideAttr === '1' || hideAttr === 'true' || hideDefault

  const zAttr = attr(el, 'data-ex-z')
  const zCss = parseInt(cs.zIndex, 10)
  const zIndex = zAttr
    ? Number(zAttr)
    : Number.isFinite(zCss)
      ? zCss
      : 0

  // Clip against the artboard instead of independently clamping origin and
  // size: the old `x=max(0,x)` + `w=min(SLIDE_W,w)` pair shifted and stretched
  // any element that started off-canvas.
  const clipX = Math.max(0, x)
  const clipY = Math.max(0, y)
  const clipR = Math.min(SLIDE_W, x + w)
  const clipB = Math.min(SLIDE_H, y + h)

  return {
    role,
    id: attr(el, 'data-ex-id') || idFallback,
    text,
    x: clipX,
    y: clipY,
    w: Math.max(1, clipR - clipX),
    h: Math.max(1, clipB - clipY),
    color: colorCss.hex,
    bg: bgAttr === 'none' ? 'none' : bgCss.hex,
    border: borderAttr === 'none' ? 'none' : borderCss.hex,
    weight: Number.isFinite(weight) ? weight : 400,
    sizePx: Number.isFinite(sizePx) ? sizePx : 16,
    lineHeightPx: Number.isFinite(lineHeightPx) ? lineHeightPx : 19,
    align,
    valign,
    font: mapPptFont(fontAttr || cs.fontFamily || 'Arial'),
    radius: Number.isFinite(radius) ? radius : 0,
    opacity,
    hideOnCapture,
    keepInPlate: surfacePaint || undefined,
    zIndex: Number.isFinite(zIndex) ? zIndex : 0,
    el,
    src,
    objectFit,
    index: attr(el, 'data-ex-index')
      ? Number(attr(el, 'data-ex-index'))
      : undefined,
    row: attr(el, 'data-ex-row') ? Number(attr(el, 'data-ex-row')) : undefined,
    col: attr(el, 'data-ex-col') ? Number(attr(el, 'data-ex-col')) : undefined,
  }
}

function looksLikeCardSurface(el: HTMLElement, view: Window): boolean {
  const cs = view.getComputedStyle(el)
  const tag = el.tagName.toLowerCase()
  if (tag === 'section' || tag === 'html' || tag === 'body') return false
  if (el.getAttribute('data-ex') === 'image') return false
  if (el.querySelector('img') && el.children.length === 1) return false
  const bg = cs.backgroundColor || ''
  if (!bg || bg === 'transparent' || bg === 'rgba(0, 0, 0, 0)') return false
  const radius = parseFloat(cs.borderRadius) || 0
  const rect = el.getBoundingClientRect()
  if (rect.width < 28 || rect.height < 28) return false
  if (rect.width > SLIDE_W * 0.95 && rect.height > SLIDE_H * 0.9) return false
  const cls = el.className?.toString?.() || ''
  const isCircle =
    /\brounded-full\b/i.test(cls) ||
    (radius >= Math.min(rect.width, rect.height) / 2 - 1 &&
      Math.abs(rect.width - rect.height) < 8)
  if (isCircle && rect.width >= 28 && rect.height >= 28) return true
  if (
    /\b(card|rounded|panel|tile|chip-box)\b/i.test(cls) ||
    el.hasAttribute('data-card') ||
    radius >= 8
  ) {
    return true
  }
  return radius >= 6 && rect.width >= 80 && rect.height >= 48
}

function hasMarkedTextDescendant(el: HTMLElement): boolean {
  return !!el.querySelector(
    '[data-ex="title"],[data-ex="subtitle"],[data-ex="body"],[data-ex="label"],[data-ex="bullet"],[data-ex="number"],[data-ex="list-item"],[data-ex="stat"],[data-ex="caption"]',
  )
}

function liftSurfaceTextOverlays(
  surfaceEl: HTMLElement,
  surfaceBox: MeasuredBox,
  root: HTMLElement,
  view: Window,
  idPrefix: string,
): MeasuredBox[] {
  if (hasMarkedTextDescendant(surfaceEl)) return []
  const raw = (surfaceEl.innerText || surfaceEl.textContent || '')
    .replace(/\s+/g, ' ')
    .trim()
  if (!raw) return []

  const markedTextEls = Array.from(
    surfaceEl.querySelectorAll('[data-ex]'),
  ) as HTMLElement[]
  if (markedTextEls.some((e) => roleOf(e) && roleOf(e) !== 'card' && roleOf(e) !== 'shape')) {
    return []
  }

  const children = Array.from(surfaceEl.children) as HTMLElement[]
  const textChildren = children.filter((c) => {
    const t = (c.innerText || c.textContent || '').replace(/\s+/g, ' ').trim()
    return t.length > 0 && t.length <= 80
  })

  if (textChildren.length >= 1) {
    return textChildren
      .map((c, i) => {
        const box = measureElement(
          c,
          root,
          'label',
          `${idPrefix}-txt-${i}`,
          view,
        )
        if (!box) return null
        // Keep the measured alignment. Forcing center here re-centred every
        // left-aligned card line and was a constant preview/export mismatch.
        box.hideOnCapture = true
        box.zIndex = Math.max(box.zIndex, surfaceBox.zIndex + 10)
        return box
      })
      .filter((b): b is MeasuredBox => !!b)
  }

  // Whole-surface fallback: the surface's own computed alignment already
  // reflects how the browser laid the text out, so reuse it.
  const box: MeasuredBox = {
    ...surfaceBox,
    role: 'label',
    id: `${idPrefix}-txt`,
    text: raw,
    bg: 'none',
    border: 'none',
    hideOnCapture: true,
    zIndex: surfaceBox.zIndex + 10,
    src: undefined,
  }
  return [box]
}

function collectOverlays(root: HTMLElement, doc: Document): MeasuredBox[] {
  const view = doc.defaultView || window
  const raw: MeasuredBox[] = []
  const marked = Array.from(root.querySelectorAll('[data-ex]')) as HTMLElement[]
  let n = 0

  const push = (el: Element | null, role: ExportOverlayRole) => {
    if (!el) return
    const box = measureElement(
      el as HTMLElement,
      root,
      role,
      role === 'card' || role === 'shape'
        ? `auto-card-${n++}`
        : `auto-${n++}`,
      view,
    )
    if (!box) return
    if (
      role === 'title' ||
      role === 'body' ||
      role === 'bullet' ||
      role === 'subtitle' ||
      role === 'card' ||
      role === 'shape' ||
      role === 'image' ||
      role === 'table' ||
      role === 'table-cell'
    ) {
      box.hideOnCapture = true
    }
    if (role === 'card' || role === 'shape') {
      box.hideOnCapture = true
      if (!box.bg || box.bg === 'none' || box.bg === '000000') {
        const bgCss = cssColorToHex(
          view.getComputedStyle(el as HTMLElement).backgroundColor,
          '1E293B',
        )
        box.bg = bgCss.hex
        box.opacity = bgCss.alpha < 1 ? bgCss.alpha : box.opacity
      }
    }
    raw.push(box)
  }

  if (marked.length > 0) {
    marked.forEach((el, i) => {
      const role = roleOf(el)
      if (!role) return
      const box = measureElement(el, root, role, `ex-${i}`, view)
      if (box) {
        if (role === 'card' || role === 'shape' || role === 'image') {
          box.hideOnCapture = true
        }
        raw.push(box)
        if (role === 'card' || role === 'shape') {
          raw.push(...liftSurfaceTextOverlays(el, box, root, view, `ex-${i}`))
        }
      }
    })
  }

  const hasCardRole = raw.some((o) => o.role === 'card' || o.role === 'shape')
  if (!hasCardRole) {
    const cardSel =
      '[data-card], .card, .slide-card, [class*="rounded-2xl"], [class*="rounded-xl"], [class*="rounded-lg"], [class*="rounded-full"]'
    root.querySelectorAll(cardSel).forEach((el) => {
      if (looksLikeCardSurface(el as HTMLElement, view)) {
        const before = raw.length
        push(el, 'card')
        if (raw.length > before) {
          const surface = raw[raw.length - 1]
          raw.push(
            ...liftSurfaceTextOverlays(
              el as HTMLElement,
              surface,
              root,
              view,
              surface.id,
            ),
          )
        }
      }
    })
    if (!raw.some((o) => o.role === 'card' || o.role === 'shape')) {
      root.querySelectorAll('div').forEach((el) => {
        if (looksLikeCardSurface(el as HTMLElement, view)) {
          const before = raw.length
          push(el, 'card')
          if (raw.length > before) {
            const surface = raw[raw.length - 1]
            raw.push(
              ...liftSurfaceTextOverlays(
                el as HTMLElement,
                surface,
                root,
                view,
                surface.id,
              ),
            )
          }
        }
      })
    }
  }

  if (!marked.length) {
    push(root.querySelector('h1, h2, .slide-title'), 'title')
    push(root.querySelector('.lede, .subtitle, .subtitle-text'), 'subtitle')

    root.querySelectorAll('ul li, ol li').forEach((el, i) => {
      const parent = el.parentElement
      const role: ExportOverlayRole =
        parent && parent.tagName.toLowerCase() === 'ol' ? 'number' : 'bullet'
      const box = measureElement(
        el as HTMLElement,
        root,
        role,
        `auto-li-${i}`,
        view,
      )
      if (box) {
        box.hideOnCapture = true
        if (role === 'number') box.index = i + 1
        raw.push(box)
      }
    })

    root.querySelectorAll('img').forEach((el, i) => {
      const box = measureElement(
        el as HTMLElement,
        root,
        'image',
        `auto-img-${i}`,
        view,
      )
      if (box?.src) {
        box.hideOnCapture = true
        raw.push(box)
      }
    })

    root.querySelectorAll('table').forEach((el, i) => {
      const box = measureElement(
        el as HTMLElement,
        root,
        'table',
        `auto-tbl-${i}`,
        view,
      )
      if (box) {
        box.hideOnCapture = true
        raw.push(box)
      }
      el.querySelectorAll('th, td').forEach((cell, j) => {
        const tr = cell.parentElement
        const row = tr
          ? Array.from(tr.parentElement?.children || []).indexOf(tr)
          : 0
        const col = tr ? Array.from(tr.children).indexOf(cell) : j
        const cbox = measureElement(
          cell as HTMLElement,
          root,
          'table-cell',
          `auto-cell-${i}-${j}`,
          view,
        )
        if (cbox) {
          cbox.hideOnCapture = true
          cbox.row = row
          cbox.col = col
          raw.push(cbox)
        }
      })
    })
  }

  return assignParents(deduplicateOverlays(raw))
}

function deduplicateOverlays(overlays: MeasuredBox[]): MeasuredBox[] {
  const seen = new Map<string, MeasuredBox>()
  const result: MeasuredBox[] = []
  for (const box of overlays) {
    if (!box.text && box.role !== 'card' && box.role !== 'shape' && box.role !== 'image' && box.role !== 'table') {
      continue
    }
    const key = `${box.role}:${box.x.toFixed(1)},${box.y.toFixed(1)},${box.w.toFixed(1)},${box.h.toFixed(1)}:${box.text}`
    const existing = seen.get(key)
    if (existing) {
      if (box.zIndex > existing.zIndex) {
        const idx = result.findIndex((r) => r === existing)
        if (idx >= 0) result[idx] = box
        seen.set(key, box)
      }
      continue
    }
    seen.set(key, box)
    result.push(box)
  }

  // Where a marked container and its marked children both survive, keep the
  // children: they carry the real per-element size, weight and colour. Keeping
  // the container instead re-rendered the whole group at one font size, which
  // is a visible mismatch against the preview.
  const texts = result.filter((o) => TEXT_ROLES[o.role] && o.text)
  const norm = flatText
  return result.filter((o) => {
    if (!TEXT_ROLES[o.role] || !o.text) return true
    const parentText = norm(o.text)
    let covered = 0
    for (const c of texts) {
      if (c === o) continue
      const inside =
        c.x >= o.x - 4 &&
        c.y >= o.y - 4 &&
        c.x + c.w <= o.x + o.w + 4 &&
        c.y + c.h <= o.y + o.h + 4
      if (!inside) continue
      if (c.w * c.h >= o.w * o.h) continue
      const childText = norm(c.text)
      if (!childText || !parentText.includes(childText)) continue
      covered += childText.length
    }
    return covered < parentText.length * 0.9
  })
}

/**
 * Records the card/shape each text box sits on and trims only its right edge
 * so PowerPoint's wider text metrics cannot push a caption outside its card.
 * Positions are never moved: the browser layout is the source of truth, and
 * the old "snap a column to a shared x" pass was what pulled exported text
 * out of alignment with the preview.
 */
function assignParents(overlays: MeasuredBox[]): MeasuredBox[] {
  const surfaces = overlays.filter(
    (o) => o.role === 'card' || o.role === 'shape',
  )
  if (surfaces.length === 0) return overlays

  return overlays.map((o) => {
    if (!TEXT_ROLES[o.role]) return o
    let parent: MeasuredBox | undefined
    for (const s of surfaces) {
      if (
        o.x >= s.x - 2 &&
        o.y >= s.y - 2 &&
        o.x + o.w <= s.x + s.w + 2 &&
        o.y + o.h <= s.y + s.h + 2
      ) {
        // Prefer the tightest enclosing surface when cards are nested.
        if (!parent || s.w * s.h < parent.w * parent.h) parent = s
      }
    }
    if (!parent) return o
    const maxRight = parent.x + parent.w - 2
    return {
      ...o,
      w: Math.max(8, Math.min(o.x + o.w, maxRight) - o.x),
      parentId: parent.id,
    }
  })
}

/**
 * Hides exactly the elements that became native PowerPoint objects, so the
 * background plate keeps every ornament we did not reconstruct.
 *
 * The previous implementation additionally blanket-hid `h1,h2,p,li,img,
 * [class*="rounded-xl"], …` and then geometry-matched every `div`. Any
 * ornament sharing those selectors was erased from the plate without a
 * replacement object, which is why exported slides lost backgrounds and
 * decorations that are clearly visible in the preview.
 *
 * Text is hidden with `color: transparent` rather than `visibility: hidden`
 * so the element keeps occupying space — hiding it outright let sibling
 * content reflow, shifting the ornaments we still capture.
 */
function hideForPlate(overlays: MeasuredBox[]): void {
  for (const box of overlays) {
    if (box.keepInPlate) continue
    if (box.hideOnCapture === false) continue
    const el = box.el
    if (!el?.style) continue
    if (TEXT_ROLES[box.role]) {
      el.style.setProperty('color', 'transparent', 'important')
      el.style.setProperty('text-shadow', 'none', 'important')
      el.style.setProperty('-webkit-text-fill-color', 'transparent', 'important')
      for (const child of Array.from(el.querySelectorAll('*'))) {
        const c = child as HTMLElement
        c.style.setProperty('color', 'transparent', 'important')
        c.style.setProperty('-webkit-text-fill-color', 'transparent', 'important')
      }
      continue
    }
    if (box.role === 'image') {
      el.style.setProperty('visibility', 'hidden', 'important')
      continue
    }
    if (box.role === 'card' || box.role === 'shape') {
      // The surface becomes a real shape; strip only its own paint so any
      // child we did not reconstruct still renders onto the plate.
      el.style.setProperty('background', 'none', 'important')
      el.style.setProperty('background-color', 'transparent', 'important')
      el.style.setProperty('border-color', 'transparent', 'important')
      el.style.setProperty('box-shadow', 'none', 'important')
      continue
    }
    if (box.role === 'table' || box.role === 'table-cell') {
      el.style.setProperty('visibility', 'hidden', 'important')
    }
  }
}

async function captureSlideHybrid(
  fullHtml: string,
  slideId: string,
): Promise<SlideCapture> {
  const { host, doc, root } = await openSlideDoc(fullHtml, slideId)
  try {
    const overlays = collectOverlays(root, doc)
    hideForPlate(overlays)
    // Let the style mutations paint before capturing the plate.
    await new Promise<void>((resolve) => {
      const view = doc.defaultView || window
      const raf =
        view.requestAnimationFrame?.bind(view) ||
        ((cb: FrameRequestCallback) => view.setTimeout(() => cb(0), 16))
      raf(() => raf(() => resolve()))
    })
    return { plateDataUrl: await toPngOf(root), overlays }
  } finally {
    host.remove()
  }
}

async function captureSlideVisual(
  fullHtml: string,
  slideId: string,
): Promise<string> {
  const { host, root } = await openSlideDoc(fullHtml, slideId)
  try {
    return await toPngOf(root)
  } finally {
    host.remove()
  }
}

type PptxSlideApi = {
  addImage: (opts: object) => unknown
  addText: (text: string | object[], opts: object) => unknown
  addShape: (shape: unknown, opts: object) => unknown
  addTable: (rows: object[][], opts: object) => unknown
  addNotes: (notes: string) => unknown
}

function textPrefix(box: MeasuredBox): string {
  if (box.role === 'bullet') return box.text.startsWith('•') ? box.text : `• ${box.text}`
  if (box.role === 'number') {
    const n = box.index && box.index > 0 ? box.index : 1
    return /^\d+[\.\)]\s/.test(box.text) ? box.text : `${n}. ${box.text}`
  }
  return box.text
}

function addOverlayToSlide(
  s: PptxSlideApi,
  pptx: { ShapeType: { roundRect: unknown } },
  box: MeasuredBox,
): void {
  if (box.keepInPlate) return
  const x = pxToInX(box.x)
  const y = pxToInY(box.y)
  const w = Math.max(0.05, pxToInX(box.w))
  const h = Math.max(0.05, pxToInY(box.h))

  if (box.role === 'image' && box.src) {
    // Match the browser's object-fit. `fill` is the one case where stretching
    // to the frame is correct; otherwise pptxgenjs `sizing` reproduces the
    // letterbox (contain) or the crop (cover) the preview shows.
    s.addImage(
      box.objectFit === 'fill'
        ? { data: box.src, x, y, w, h }
        : {
            data: box.src,
            x,
            y,
            w,
            h,
            sizing: { type: box.objectFit === 'cover' ? 'cover' : 'contain', w, h },
          },
    )
    return
  }

  if (box.role === 'card' || box.role === 'shape') {
    const fill =
      box.bg && box.bg !== 'none'
        ? {
            type: 'solid',
            color: box.bg,
            transparency: Math.round((1 - box.opacity) * 100),
          }
        : { type: 'none' }
    const line =
      box.border && box.border !== 'none'
        ? { color: box.border, width: 1 }
        : { type: 'none' }
    // rectRadius is a 0–1 fraction of the shape's shorter side, not inches.
    // Dividing the CSS radius by a fixed 96 made every card's corner rounding
    // depend on its size instead of matching the authored radius.
    const shortSideIn = Math.max(0.01, Math.min(w, h))
    s.addShape(pptx.ShapeType.roundRect, {
      x,
      y,
      w,
      h,
      fill,
      line,
      rectRadius: Math.max(
        0,
        Math.min(0.5, pxToInX(box.radius || 0) / shortSideIn),
      ),
    })
    return
  }

  // Tables are emitted by addTables(); cells are consumed there.
  if (box.role === 'table' || box.role === 'table-cell') return
  if (!box.text) return

  // Keep the measured frame. PowerPoint's text metrics are slightly wider than
  // the browser's, so allow only a small horizontal bleed (never past the
  // artboard or the parent card) and let the height grow downward rather than
  // re-centering the block.
  const bleed = box.parentId ? 0 : Math.min(0.12, w * 0.06)
  const boxX = Math.max(0, x - bleed)
  const boxW = Math.max(0.05, Math.min(PPTX_W - boxX, w + bleed * 2))
  const boxY = Math.max(0, y)
  const boxH = Math.max(0.05, Math.min(PPTX_H - boxY, h))

  s.addText(textPrefix(box), {
    x: boxX,
    y: boxY,
    w: boxW,
    h: boxH,
    fontSize: pxToPt(box.sizePx),
    fontFace: mapPptFont(box.font),
    color: normalizeHex(box.color, '1F2937'),
    bold: box.weight >= 600,
    align: box.align,
    valign: box.valign,
    // The browser box already includes padding; PowerPoint's default 0.1"
    // inset would shift every string right and down relative to the preview.
    margin: 0,
    // Line height comes from the measured element so multi-line blocks land on
    // the same baselines the preview shows.
    lineSpacing: pxToPt(box.lineHeightPx),
    wrap: true,
    // 'shrink' asks PowerPoint to scale text down if it overflows on open,
    // which protects long strings without us silently capping font sizes.
    fit: 'shrink',
    isTextBox: true,
  })
}

function addTables(
  s: PptxSlideApi,
  overlays: MeasuredBox[],
): void {
  const tables = overlays.filter((o) => o.role === 'table')
  for (const tbl of tables) {
    const cells = overlays.filter(
      (o) =>
        o.role === 'table-cell' &&
        o.x >= tbl.x - 2 &&
        o.y >= tbl.y - 2 &&
        o.x + o.w <= tbl.x + tbl.w + 4 &&
        o.y + o.h <= tbl.y + tbl.h + 4,
    )
    if (cells.length === 0) continue
    const maxRow = Math.max(...cells.map((c) => c.row ?? 0), 0)
    const maxCol = Math.max(...cells.map((c) => c.col ?? 0), 0)
    const rows: object[][] = []
    for (let r = 0; r <= maxRow; r++) {
      const row: object[] = []
      for (let c = 0; c <= maxCol; c++) {
        const cell = cells.find((x) => (x.row ?? 0) === r && (x.col ?? 0) === c)
        row.push({
          text: cell?.text || '',
          options: {
            color: normalizeHex(cell?.color || 'E2E8F0', 'E2E8F0'),
            bold: (cell?.weight || 400) >= 600,
            fontSize: pxToPt(cell?.sizePx || 14),
            fontFace: mapPptFont(cell?.font || 'Arial'),
            align: cell?.align || 'left',
            valign: 'middle',
          },
        })
      }
      rows.push(row)
    }
    try {
      s.addTable(rows, {
        x: pxToInX(tbl.x),
        y: pxToInY(tbl.y),
        w: Math.max(0.5, pxToInX(tbl.w)),
        h: Math.max(0.3, pxToInY(tbl.h)),
        border: [
          {
            type: 'solid',
            pt: 0.5,
            color: normalizeHex(tbl.border || '334155', '334155'),
          },
        ],
        fontFace: 'Arial',
        color: 'E2E8F0',
      })
    } catch {
      for (const cell of cells) {
        if (!cell.text) continue
        s.addText(cell.text, {
          x: pxToInX(cell.x),
          y: pxToInY(cell.y),
          w: Math.max(0.2, pxToInX(cell.w)),
          h: Math.max(0.15, pxToInY(cell.h)),
          fontSize: pxToPt(cell.sizePx),
          fontFace: mapPptFont(cell.font),
          color: normalizeHex(cell.color, 'E2E8F0'),
          bold: cell.weight >= 600,
          margin: 0,
          wrap: true,
        })
      }
    }
  }
}

/** Author-provided speaker notes, if the slide carries any. */
function slideNotes(slide: SlideMeta): string {
  const m =
    /<section\b[^>]*\bdata-notes\s*=\s*"([^"]*)"/i.exec(slide.outerHtml) ||
    /<section\b[^>]*\bdata-notes\s*=\s*'([^']*)'/i.exec(slide.outerHtml)
  if (!m?.[1]) return ''
  const doc = new DOMParser().parseFromString(m[1], 'text/html')
  return (doc.documentElement.textContent || '').trim()
}

/**
 * `visual` is the default because it is the only mode that reproduces the
 * preview exactly: one rasterized image per slide. `hybrid` rebuilds elements
 * as native PowerPoint objects, which is genuinely editable but cannot promise
 * pixel parity — PowerPoint's own text metrics, line breaking and font
 * substitution differ from the browser's.
 */
export async function exportToPptx(
  slides: SlideMeta[],
  deckTitle: string,
  fullHtml: string,
  mode: PptxExportMode = 'visual',
  contextLabel?: string,
): Promise<void> {
  if (!slides.length) {
    throw new Error('This deck has no slides to export.')
  }

  // Dynamic: pptxgenjs is a multi-hundred-kB writer only needed on export.
  const pptxgen = (await import('pptxgenjs')).default
  const pptx = new pptxgen()

  pptx.defineLayout({ name: 'SLIDE_16x9', width: PPTX_W, height: PPTX_H })
  pptx.layout = 'SLIDE_16x9'
  pptx.title = deckTitle
  pptx.company = 'Inferenesia'
  pptx.author = 'Inferenesia App'

  for (const slide of slides) {
    const s = pptx.addSlide() as unknown as PptxSlideApi
    const label = slide.title || `slide ${slide.index + 1}`
    const notes = slideNotes(slide)

    try {
      if (mode === 'visual') {
        const imgData = await captureSlideVisual(fullHtml, slide.id)
        s.addImage({ data: imgData, x: 0, y: 0, w: PPTX_W, h: PPTX_H })
        if (notes) s.addNotes(notes)
        continue
      }

      const cap = await captureSlideHybrid(fullHtml, slide.id)
      s.addImage({ data: cap.plateDataUrl, x: 0, y: 0, w: PPTX_W, h: PPTX_H })

      const byLayer = (a: MeasuredBox, b: MeasuredBox) => {
        const lr = layerRank(a.role) - layerRank(b.role)
        if (lr !== 0) return lr
        if (a.zIndex !== b.zIndex) return a.zIndex - b.zIndex
        return a.y - b.y || a.x - b.x
      }

      // Full-bleed pictures go first so smaller pictures stack on top of them.
      const images = cap.overlays
        .filter((o) => o.role === 'image')
        .sort((a, b) => {
          const aFull = a.w >= SLIDE_W * 0.9 && a.h >= SLIDE_H * 0.9 ? 0 : 1
          const bFull = b.w >= SLIDE_W * 0.9 && b.h >= SLIDE_H * 0.9 ? 0 : 1
          return aFull !== bFull ? aFull - bFull : byLayer(a, b)
        })
      // Larger surfaces before smaller ones so nested cards are not buried.
      // Surfaces kept in the plate are excluded: they are already painted in
      // the background image, and emitting a flat-fill shape over them would
      // cover the very gradient we preserved.
      const cards = cap.overlays
        .filter((o) => (o.role === 'card' || o.role === 'shape') && !o.keepInPlate)
        .sort((a, b) => b.w * b.h - a.w * a.h || byLayer(a, b))
      const texts = cap.overlays
        .filter((o) => TEXT_ROLES[o.role])
        .sort(byLayer)

      // Emission order is PowerPoint's z-order: pictures, surfaces, tables,
      // then text on top — matching the data-ex contract the agent writes to.
      for (const img of images) addOverlayToSlide(s, pptx, img)
      for (const card of cards) addOverlayToSlide(s, pptx, card)
      addTables(s, cap.overlays)
      for (const t of texts) addOverlayToSlide(s, pptx, t)

      if (notes) s.addNotes(notes)
    } catch (err) {
      const reason = err instanceof Error ? err.message : String(err)
      throw new Error(`Export stopped on "${label}": ${reason}`)
    }
  }

  const suffix = mode === 'hybrid' ? '-editable' : ''
  const ctx = sanitizeDownloadSegment(
    (contextLabel || '')
      .replace(/^Session\s*[·•\-–—]\s*/i, '')
      .replace(/^Sesi\s*[·•\-–—]\s*/i, '')
      .replace(/^Workspace\s*[·•\-–—]\s*/i, '')
      .replace(/^playground:\s*/i, '')
      .trim(),
    'Playground',
  )
  const stem = safeFileName(deckTitle) || 'Untitled deck'
  await pptx.writeFile({
    fileName: `Inferenesia - ${ctx} - ${stem}${suffix}.pptx`,
  })
}

/**
 * Print stylesheet for a 16:9 deck.
 *
 * Each slide is authored at exactly 1280×720 CSS px, so A4 landscape
 * (297×210mm) was the wrong page: 297/210 is 1.414, not 1.778, so every slide
 * was letterboxed with white bands or clipped, and the PDF stopped matching the
 * preview. The page is sized to the artboard's own 16:9 ratio, and the slide
 * keeps its authored padding instead of being forced to `display:block`, which
 * had discarded `.slide-pad`'s 32px inset.
 */
export function buildPrintHtml(html: string): string {
  const printStyles = `
    @page { size: ${SLIDE_W}px ${SLIDE_H}px; margin: 0; }
    html, body {
      margin: 0 !important; padding: 0 !important;
      background: #fff !important;
      width: ${SLIDE_W}px; height: ${SLIDE_H}px;
    }
    .inferenesia-deck { width: ${SLIDE_W}px !important; margin: 0 !important; }
    .slide {
      page-break-after: always;
      break-after: page;
      page-break-inside: avoid;
      break-inside: avoid;
      width: ${SLIDE_W}px !important;
      height: ${SLIDE_H}px !important;
      max-width: ${SLIDE_W}px !important;
      max-height: ${SLIDE_H}px !important;
      overflow: hidden !important;
      margin: 0 !important;
      box-shadow: none !important;
      border-radius: 0 !important;
      border: none !important;
      position: relative !important;
    }
    .slide:last-child { page-break-after: auto; break-after: auto; }
    .slide-blank { display: none !important; }
    * { -webkit-print-color-adjust: exact !important; print-color-adjust: exact !important; }
  `
  if (/<\/head>/i.test(html)) {
    return html.replace(/<\/head>/i, `<style>${printStyles}</style></head>`)
  }
  return `<style>${printStyles}</style>${html}`
}

/**
 * Waits for the print window's pictures and fonts before calling print().
 * Printing on a fixed 800ms timer fired while remote images were still
 * downloading, so the PDF came out with blank or half-painted slides.
 */
async function waitForPrintAssets(w: Window, timeoutMs = 10000): Promise<void> {
  const doc = w.document
  const deadline = Date.now() + timeoutMs

  const pending = Array.from(doc.images || []).filter((img) => !img.complete)
  if (pending.length) {
    await Promise.race([
      Promise.all(
        pending.map(
          (img) =>
            new Promise<void>((resolve) => {
              img.addEventListener('load', () => resolve(), { once: true })
              img.addEventListener('error', () => resolve(), { once: true })
            }),
        ),
      ),
      wait(Math.max(0, deadline - Date.now())),
    ])
  }

  try {
    const fonts = (doc as Document & { fonts?: FontFaceSet }).fonts
    if (fonts?.ready) {
      await Promise.race([fonts.ready, wait(Math.max(0, deadline - Date.now()))])
    }
  } catch {
    /* fonts API unavailable — print anyway */
  }

  await new Promise<void>((resolve) => {
    const raf =
      w.requestAnimationFrame?.bind(w) ||
      ((cb: FrameRequestCallback) => w.setTimeout(() => cb(0), 16))
    raf(() => raf(() => resolve()))
  })
}

export function exportToPdf(html: string): void {
  const printHtml = buildPrintHtml(html)
  const w = window.open('', '_blank', 'width=1280,height=720')
  if (!w) {
    window.alert('Pop-up blocked. Allow pop-ups to export PDF.')
    return
  }
  const doc = w.document
  doc.open()
  doc.write(printHtml)
  doc.close()

  void (async () => {
    try {
      await waitForPrintAssets(w)
      w.focus()
      w.print()
    } catch {
      try {
        w.focus()
        w.print()
      } catch {
        /* the window was closed before printing */
      }
    }
  })()
}
