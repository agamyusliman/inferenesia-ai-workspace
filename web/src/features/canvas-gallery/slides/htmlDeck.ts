export const SLIDE_W = 1280
export const SLIDE_H = 720

export type DeckStyleId =
  | 'dark'
  | 'light'
  | 'brand'
  | 'pitch'
  | 'report'
  | 'minimal'

export type LayoutBiasId =
  | 'title'
  | 'two-column'
  | 'tiled'
  | 'bleed'
  | 'kpi'
  | 'bullets'
  | 'qa'
  | 'auto'

export type ImageSlotId =
  | 'hero'
  | 'left'
  | 'right'
  | 'background'
  | 'tile-1'
  | 'inline'

export type SlideMeta = {
  id: string
  title: string
  layout: string
  index: number
  outerHtml: string
}

export const DECK_STYLES: {
  id: DeckStyleId
  label: string
  prompt: string
}[] = [
  {
    id: 'dark',
    label: 'Dark',
    prompt:
      'Dark editorial theme: deep neutral surfaces, confident light typography, and one restrained accent. Use contrast and whitespace rather than repeated translucent cards.',
  },
  {
    id: 'light',
    label: 'Light',
    prompt:
      'Light editorial theme: warm or cool near-white surfaces, dark typography, and one restrained accent. Use rules, scale, and whitespace before adding containers.',
  },
  {
    id: 'brand',
    label: 'Brand',
    prompt:
      'Brand theme: slate and blue, with a clear editorial hierarchy. Let one brand gesture carry each slide; do not turn every point into a branded card.',
  },
  {
    id: 'pitch',
    label: 'Pitch',
    prompt:
      'Pitch theme: decisive headlines, sparse supporting copy, high contrast, and one main idea per slide. Use only evidence supplied by the user; never invent traction, metrics, customers, or quotes.',
  },
  {
    id: 'report',
    label: 'Report',
    prompt:
      'Report theme: sober hierarchy, precise labels, readable evidence, and restrained rules or tables. Show only sourced numbers and make uncertainty explicit rather than manufacturing KPIs.',
  },
  {
    id: 'minimal',
    label: 'Minimal',
    prompt:
      'Minimal editorial theme: few colors, purposeful negative space, strong type, and asymmetric composition when useful. Minimal means fewer elements, not tiny copy or a forced 56px inset.',
  },
]

export function slidesDesignSystemPrompt(): string {
  return [
    '## SLIDE DESIGN SYSTEM — EDITORIAL, CONTENT-DRIVEN',
    'The canvas is exactly 1280×720. Every slide is <section class="slide" data-slide-id data-layout data-title>. Keep these dimensions, ids, and data-ex export attributes intact.',
    '',
    '### Start with meaning, not a template',
    '- Give each slide one communicative job. Write a concise editorial headline that states the takeaway; avoid generic headings such as “Overview”, “Key Benefits”, or “Our Solution” when a specific claim is available.',
    '- Choose composition from the content: contrast → split; sequence → flow/timeline; evidence → chart/table/stat; hierarchy → diagram; one key idea → type-led hero; nuanced explanation → editorial columns; truly parallel rich items → cards.',
    '- Cards are containers, not a default visual language. Do not repeat the same card grid, badge, icon, or ornamental recipe across the deck. Two adjacent slides should not share the same composition unless the narrative genuinely requires continuity.',
    '- Vary rhythm intentionally: alternate sparse and information-rich slides, shift image/text balance, and use occasional full-field or divider moments. Variation must serve the story, not become random decoration.',
    '',
    '### Grounded content',
    '- Treat user materials and outline as source of truth. Never invent metrics, market sizes, customer names, dates, research findings, testimonials, quotations, or product capabilities.',
    '- When evidence is absent, use qualitative wording or a clearly labeled placeholder such as “Add verified baseline” only if the user asked for a placeholder. Never make a fake KPI to fill a layout.',
    '- Preserve nuance. Do not convert every sentence into a forced bullet list. Prefer short prose, direct labels, or a visual relationship when those communicate better.',
    '- Edit copy for projection: short headline, one useful lede at most, then only the supporting content needed for this slide. Move overflow to another slide or cut repetition; never clip it.',
    '',
    '### Composition and fit',
    '- Keep meaningful content at least 28px from canvas edges (20px absolute minimum for dense slides). Full-bleed backgrounds and media may reach the edge; their text may not.',
    '- Budget vertical space before writing markup. Titles may wrap naturally to two lines when needed; shorten or scale them deliberately instead of line-clamping, cropping, or forcing <br>.',
    '- A headline in a narrow column must still fit within two or three natural lines. Shorten its wording without changing meaning, widen its column, or move it above the columns. Never stack a sentence into four or more display lines just to retain the outline wording.',
    '- Outline titles describe the message, not mandatory verbatim copy. Prefer a sharp 4–8 word headline with the nuance in the supporting sentence. Keep bottom content at least 28px inside the artboard, including card surfaces.',
    '- Use a clear type hierarchy: opening headline roughly 48–64px, content headline 34–48px, body 17–22px, supporting labels 13–16px. These are ranges, not global overrides. Match data-ex-size and data-ex-font to visible CSS.',
    '- Keep line lengths readable, align related edges, and use whitespace as structure. Do not distribute items merely to fill the canvas or stretch short cards into empty towers.',
    '- For 4–6 rich parallel items, use a sensible multi-row grid only if cards are warranted. For a simple list, use an editorial list. For a sequence, use a flow. Never force five tall cards into one row or one column.',
    '- All meaningful content must remain in normal layout flow. Absolute positioning is reserved for backgrounds, edge accents, connectors, and deliberately placed media—not paragraph stacks or cards.',
    '',
    '### Visual restraint',
    '- Build one coherent palette and type pairing across the deck, then vary composition within it. Use common PowerPoint-safe fonts such as Arial, Calibri, Georgia, Times New Roman, Trebuchet MS, or Segoe UI.',
    '- Decoration is optional and subordinate. Prefer one useful field, rule, crop, texture, or geometric gesture over clusters of blobs, stickers, emoji, glass cards, and gradients.',
    '- Images must carry meaning. Use supplied or requested imagery only; do not fabricate stock-photo facts or create image boxes for every bullet.',
    '- Maintain contrast against the immediate surface. If the user asks for a color change, update visible CSS and matching data-ex-color without silently re-theming unrelated surfaces.',
    '',
    '### Export contract',
    '- Every editable title, subtitle, body, list item, stat, label, card, shape, table cell, and image needs its appropriate data-ex role and a stable data-ex-id.',
    '- Roles: title | subtitle | body | label | bullet | number | list-item | stat | caption are text. card | shape are surfaces with no baked-in text. image is a picture. table plus table-cell (with data-ex-row and data-ex-col) is a table.',
    '- Text roles require data-ex-color, data-ex-size, data-ex-weight, data-ex-font, and data-ex-hide-on-capture="1". The attributes must describe the visible styling.',
    '- Optional metadata: data-ex-align (left|center|right), data-ex-valign (top|middle|bottom), data-ex-z (higher is in front), data-ex-opacity, data-ex-index on numbered items.',
    '- Card/shape surfaces carry data-ex-bg, data-ex-border, and data-ex-radius. Their text is a separate data-ex text element above the surface. Images use data-ex="image" and data-image-slot.',
    '- If a surface carries an icon, emoji, or text, mark the surface data-ex="card" or "shape" and mark its content as its own text role above it. Never rely on text painted into a surface that becomes an editable shape.',
    '- Keep decorative background elements free of data-ex. Never bake meaningful labels into decoration that becomes hidden behind editable PowerPoint shapes.',
    '- Gradient, image, or blurred surfaces stay in the captured background. Give them no data-ex role, and never place required reading inside them expecting it to become editable.',
    '- Never put slide numbers in exportable text content.',
    '',
    '### Final fit pass',
    'For every slide, verify: exact 1280×720 section; stable id; headline fully visible; no content beyond the bottom/right edge; no accidental overlap; no unsupported facts; composition differs meaningfully from its neighbors; visible styles and data-ex metadata agree.',
  ].join('\n')
}

export const LAYOUT_BIASES: { id: LayoutBiasId; label: string }[] = [
  { id: 'auto', label: 'Auto' },
  { id: 'title', label: 'Title' },
  { id: 'bullets', label: 'Bullets' },
  { id: 'two-column', label: 'Two-col' },
  { id: 'tiled', label: 'Tiles' },
  { id: 'bleed', label: 'Bleed' },
  { id: 'kpi', label: 'KPI' },
  { id: 'qa', label: 'Q&A' },
]

export const IMAGE_SLOTS: { id: ImageSlotId; label: string }[] = [
  { id: 'right', label: 'Right' },
  { id: 'left', label: 'Left' },
  { id: 'hero', label: 'Hero' },
  { id: 'background', label: 'BG' },
  { id: 'tile-1', label: 'Tile' },
  { id: 'inline', label: 'Inline' },
]

function mintSlideId(): string {
  return `s_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 6)}`
}

export function stylePrompt(styleId: DeckStyleId): string {
  return DECK_STYLES.find((s) => s.id === styleId)?.prompt || DECK_STYLES[0].prompt
}

export function deckBaseStyles(): string {
  return `
html,body{margin:0;padding:0;background:transparent;}
.inferenesia-deck{width:${SLIDE_W}px;margin:0 auto;}
.slide{
  width:${SLIDE_W}px;height:${SLIDE_H}px;
  box-sizing:border-box;overflow:hidden;position:relative;isolation:isolate;
  page-break-after:always;
  font-family:Calibri,Arial,Helvetica,sans-serif;
  -webkit-font-smoothing:antialiased;
}
.slide,.slide *,.slide *::before,.slide *::after{box-sizing:border-box;}
.slide.slide-pad{padding:32px;}
.slide[data-layout="bleed"]{padding:0;}
.slide[data-layout="bleed"] .slide-inset{box-sizing:border-box;height:100%;padding:32px;}
.slide .slide-stack,.slide .content-stack,.slide .slide-body,.slide .slide-header,.slide .header-block{
  display:flex;flex-direction:column;min-width:0;
}
.slide h1,.slide h2,.slide h3,.slide p{max-width:100%;}
.slide h1,.slide h2,.slide h3,.slide p,.slide ul,.slide ol{margin-block-start:0;}
.slide .grid,.slide [class*="grid-cols"]{min-width:0;}
.slide .grid > *,.slide [class*="grid-cols"] > *{min-width:0;min-height:0;}
.slide .flex,.slide .flex > *{min-width:0;}
.slide .split,.slide .two-col,.slide [data-layout-inner="split"]{min-width:0;min-height:0;}
.slide .split > *,.slide .two-col > *,.slide [data-layout-inner="split"] > *{min-width:0;}
.slide img{max-width:100%;max-height:100%;object-fit:cover;display:block;}
.slide img[data-asset-id]{width:100%;height:100%;object-fit:cover;}
.slide [data-image-slot]{min-width:0;min-height:0;overflow:hidden;}
.slide [data-decoration]{pointer-events:none;}
.slide-blank{
  display:flex;align-items:center;justify-content:center;padding:32px;
  background:#f8fafc;color:#64748b;
  font-size:1.05rem;font-weight:500;border:2px dashed #cbd5e1;
}
.slide-blank .hint{text-align:center;max-width:420px;line-height:1.5;}
@media print{.slide{page-break-after:always;}}
`.trim()
}

export function blankSlideHtml(id?: string, title = 'Blank slide'): string {
  const sid = id || mintSlideId()
  return `<section class="slide slide-blank" data-slide-id="${sid}" data-layout="blank" data-title="${escapeAttr(title)}">
  <div class="hint">Blank slide — describe content in the agent bar below</div>
</section>`
}

export function emptyHtmlDeck(title = 'Untitled deck'): string {
  const sid = mintSlideId()
  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>${escapeHtml(title)}</title>
<style>${deckBaseStyles()}</style>
<script src="https://cdn.tailwindcss.com"></script>
<script>tailwind.config={corePlugins:{preflight:false}}</script>
</head>
<body>
<div class="inferenesia-deck" data-deck-title="${escapeAttr(title)}" data-width="${SLIDE_W}" data-height="${SLIDE_H}">
${blankSlideHtml(sid, 'Title')}
</div>
</body>
</html>
`
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function escapeAttr(s: string): string {
  return escapeHtml(s).replace(/'/g, '&#39;')
}

export function listSlidesFromHtml(html: string): SlideMeta[] {
  return listSlidesFromHtmlRegex(html)
}

type SlideRange = {
  id: string
  title: string
  layout: string
  index: number
  outerHtml: string
  start: number
  end: number
}

function listSlideRanges(html: string): SlideRange[] {
  if (!html) return []
  const strictRe =
    /<section\b(?=[^>]*\bclass\s*=\s*["'][^"']*\bslide\b)[^>]*>[\s\S]*?<\/section>/gi
  const looseRe =
    /<section\b(?=[^>]*data-slide-id\s*=)[^>]*>[\s\S]*?<\/section>/gi

  const parseRange = (
    outer: string,
    start: number,
    i: number,
  ): SlideRange => {
    const idM =
      /data-slide-id\s*=\s*"([^"]+)"/i.exec(outer) ||
      /data-slide-id\s*=\s*'([^']+)'/i.exec(outer)
    const titleM =
      /data-title\s*=\s*"([^"]+)"/i.exec(outer) ||
      /data-title\s*=\s*'([^']+)'/i.exec(outer)
    const layoutM =
      /data-layout\s*=\s*"([^"]+)"/i.exec(outer) ||
      /data-layout\s*=\s*'([^']+)'/i.exec(outer)
    const hM = /<h[12][^>]*>([\s\S]*?)<\/h[12]>/i.exec(outer)
    return {
      id: idM?.[1] || `s_auto_${i}`,
      title: (titleM?.[1] || stripTags(hM?.[1] || '') || `Slide ${i + 1}`).slice(
        0,
        80,
      ),
      layout: layoutM?.[1] || 'custom',
      index: i,
      outerHtml: outer,
      start,
      end: start + outer.length,
    }
  }

  const out: SlideRange[] = []
  const seen = new Set<string>()
  let m: RegExpExecArray | null
  let i = 0

  strictRe.lastIndex = 0
  while ((m = strictRe.exec(html)) !== null) {
    if (!seen.has(m[0])) {
      seen.add(m[0])
      out.push(parseRange(m[0], m.index, i))
      i += 1
    }
  }
  if (out.length) return out

  looseRe.lastIndex = 0
  while ((m = looseRe.exec(html)) !== null) {
    if (!seen.has(m[0])) {
      seen.add(m[0])
      out.push(parseRange(m[0], m.index, i))
      i += 1
    }
  }
  return out
}

function listSlidesFromHtmlRegex(html: string): SlideMeta[] {
  return listSlideRanges(html).map(({ id, title, layout, index, outerHtml }) => ({
    id,
    title,
    layout,
    index,
    outerHtml,
  }))
}

function stripTags(s: string): string {
  return s.replace(/<[^>]+>/g, '').trim()
}

export function insertBlankSlide(
  html: string,
  afterSlideId?: string | null,
): { html: string; slideId: string } {
  const slideId = mintSlideId()
  const block = blankSlideHtml(slideId)
  const ranges = listSlideRanges(html)
  if (!ranges.length) {
    return { html: emptyHtmlDeck(), slideId }
  }
  const target =
    (afterSlideId && ranges.find((s) => s.id === afterSlideId)) ||
    ranges[ranges.length - 1]
  const at = target.end
  return {
    html: html.slice(0, at) + '\n' + block + html.slice(at),
    slideId,
  }
}
function remapDuplicateIds(outerHtml: string, nextSlideId: string): string {
  const idMap = new Map<string, string>()
  const withSlideId = ensureSlideIdsStable(outerHtml, nextSlideId)
  withSlideId.replace(/\s(?:id|data-ex-id)\s*=\s*(["'])([^"']+)\1/gi, (_match, _quote, id: string) => {
    if (!idMap.has(id)) idMap.set(id, `${nextSlideId}-${id}`)
    return _match
  })
  let clone = withSlideId
  for (const [sourceId, cloneId] of idMap) {
    const escaped = escapeRegExp(sourceId)
    clone = clone
      .replace(new RegExp(`(\\s(?:id|data-ex-id)\\s*=\\s*["'])${escaped}(["'])`, 'gi'), `$1${cloneId}$2`)
      .replace(new RegExp(`(\\s(?:for|aria-labelledby|aria-describedby)\\s*=\\s*["'])${escaped}(["'])`, 'gi'), `$1${cloneId}$2`)
      .replace(new RegExp(`(["'(])#${escaped}(?=["')])`, 'g'), `$1#${cloneId}`)
  }
  return clone
}

export function duplicateSlideInHtml(
  html: string,
  slideId: string,
): { html: string; slideId: string } | null {
  const source = listSlideRanges(html).find((slide) => slide.id === slideId)
  if (!source) return null
  const nextSlideId = mintSlideId()
  const clone = remapDuplicateIds(source.outerHtml, nextSlideId)
  const nextHtml = insertSlideAfter(html, slideId, clone)
  if (!nextHtml) return null
  const after = listSlidesFromHtml(nextHtml)
  if (after.length !== listSlidesFromHtml(html).length + 1) return null
  if (after.filter((slide) => slide.id === nextSlideId).length !== 1) return null
  return { html: nextHtml, slideId: nextSlideId }
}

export function deleteSlideFromHtml(
  html: string,
  slideId: string,
): string | null {
  const ranges = listSlideRanges(html)
  if (ranges.length <= 1) return null
  const target = ranges.find((s) => s.id === slideId)
  if (!target) return null
  const next = (
    html.slice(0, target.start) + html.slice(target.end)
  ).replace(/\n{3,}/g, '\n\n')
  const after = listSlideRanges(next)
  if (after.length !== ranges.length - 1) return null
  if (after.some((s) => s.id === slideId)) return null
  return next
}

export function reorderSlidesInHtml(
  html: string,
  orderedIds: string[],
): string | null {
  const ranges = listSlideRanges(html)
  if (!ranges.length || orderedIds.length !== ranges.length) return null

  const byId = new Map(ranges.map((s) => [s.id, s]))
  if (byId.size !== ranges.length) return null
  if (orderedIds.some((id) => !byId.has(id))) return null

  const sortedCur = ranges.map((s) => s.id).slice().sort().join('\0')
  const sortedNext = orderedIds.slice().sort().join('\0')
  if (sortedCur !== sortedNext) return null
  if (orderedIds.every((id, i) => id === ranges[i].id)) return html

  const start = ranges[0].start
  const end = ranges[ranges.length - 1].end
  if (start < 0 || end <= start || end > html.length) return null

  const reordered = orderedIds.map((id) => byId.get(id)!.outerHtml).join('\n')
  const nextHtml = html.slice(0, start) + reordered + html.slice(end)

  const after = listSlideRanges(nextHtml)
  if (after.length !== ranges.length) return null
  return nextHtml
}

export function getSlideOuterHtml(
  html: string,
  slideId: string,
): string | null {
  return listSlidesFromHtml(html).find((s) => s.id === slideId)?.outerHtml || null
}

export function isBlankSlideHtml(outerHtml: string): boolean {
  return /\bslide-blank\b/.test(outerHtml) || /data-layout="blank"/i.test(outerHtml)
}

export function replaceSlideOuterHtml(
  html: string,
  slideId: string,
  nextOuter: string,
): string | null {
  const ranges = listSlideRanges(html)
  const target = ranges.find((s) => s.id === slideId)
  if (!target) return null
  return html.slice(0, target.start) + nextOuter + html.slice(target.end)
}

export function insertSlideAfter(
  html: string,
  afterSlideId: string | null | undefined,
  slideOuterHtml: string,
): string | null {
  const ranges = listSlideRanges(html)
  if (!ranges.length) {
    return ensureDeckShell(slideOuterHtml)
  }
  const target =
    (afterSlideId && ranges.find((s) => s.id === afterSlideId)) ||
    ranges[ranges.length - 1]
  const at = target.end
  return html.slice(0, at) + '\n' + slideOuterHtml + html.slice(at)
}

export function extractSlideSections(text: string): string[] {
  if (!text) return []
  const normalized = text.replace(/\r\n/g, '\n')
  const bodies: string[] = []
  const fenceRe = /```(?:html|htm)?\s*\n([\s\S]*?)```/gi
  let fm: RegExpExecArray | null
  while ((fm = fenceRe.exec(normalized)) !== null) {
    if (fm[1]?.trim()) bodies.push(fm[1].trim())
  }
  const openFence = /```(?:html|htm)?\s*\n([\s\S]*)$/i.exec(normalized)
  if (openFence?.[1]?.trim() && !bodies.some((b) => b.includes(openFence[1].slice(0, 40)))) {
    bodies.push(openFence[1].trim())
  }
  if (!bodies.length) bodies.push(normalized)

  const sectionRe =
    /<section\b(?=[^>]*\bclass\s*=\s*["'][^"']*\bslide\b)[^>]*>[\s\S]*?<\/section>/gi
  const looseRe =
    /<section\b(?=[^>]*data-slide-id\s*=)[^>]*>[\s\S]*?<\/section>/gi
  const out: string[] = []
  const seen = new Set<string>()
  for (const body of bodies) {
    let m: RegExpExecArray | null
    sectionRe.lastIndex = 0
    while ((m = sectionRe.exec(body)) !== null) {
      if (!seen.has(m[0])) {
        seen.add(m[0])
        out.push(m[0])
      }
    }
  }
  if (out.length) return out
  for (const body of bodies) {
    let m: RegExpExecArray | null
    looseRe.lastIndex = 0
    while ((m = looseRe.exec(body)) !== null) {
      if (!seen.has(m[0])) {
        seen.add(m[0])
        out.push(m[0])
      }
    }
  }
  return out
}

function pickSectionForActive(
  sections: string[],
  activeSlideId?: string | null,
): string | null {
  if (!sections.length) return null
  if (activeSlideId) {
    const match = sections.find((s) =>
      new RegExp(
        `data-slide-id\\s*=\\s*["']${activeSlideId.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}["']`,
        'i',
      ).test(s),
    )
    if (match) return match
  }
  return sections[0]
}
function validateSurgicalSlideScope(
  before: string,
  after: string,
  targetIds: string[],
): { ok: true; changed: number } | { ok: false; error: string } {
  const targets = new Set(targetIds.filter(Boolean))
  if (!targets.size) return { ok: false, error: 'No target slide was selected.' }
  const beforeSlides = listSlidesFromHtml(before)
  const afterSlides = listSlidesFromHtml(after)
  if (beforeSlides.length !== afterSlides.length) {
    return {
      ok: false,
      error: `Rejected edit: slide count changed (${beforeSlides.length} → ${afterSlides.length}).`,
    }
  }
  if (deckShellWithoutSlides(before) !== deckShellWithoutSlides(after)) {
    return { ok: false, error: 'Rejected edit: deck styles or shell changed outside the selected slides.' }
  }
  const afterById = new Map(afterSlides.map((slide) => [slide.id, slide.outerHtml]))
  let changed = 0
  for (const slide of beforeSlides) {
    const nextOuter = afterById.get(slide.id)
    if (!nextOuter) {
      return { ok: false, error: `Rejected edit: slide ${slide.id} was removed or renamed.` }
    }
    if (targets.has(slide.id)) {
      if (nextOuter !== slide.outerHtml) changed++
    } else if (nextOuter !== slide.outerHtml) {
      return {
        ok: false,
        error: `Rejected edit: unselected slide ${slide.id} would be modified.`,
      }
    }
  }
  const missing = [...targets].filter((id) => !beforeSlides.some((slide) => slide.id === id))
  if (missing.length) {
    return { ok: false, error: `Selected slide not found: ${missing.join(', ')}.` }
  }
  if (changed !== targets.size) {
    return {
      ok: false,
      error: `Edit changed ${changed} of ${targets.size} selected slide(s). No partial edit was applied.`,
    }
  }
  return { ok: true, changed }
}
function deckShellWithoutSlides(html: string): string {
  const ranges = listSlideRanges(html)
  if (!ranges.length) return html
  let cursor = 0
  let shell = ''
  for (const range of ranges) {
    shell += html.slice(cursor, range.start)
    shell += `<!--slide:${range.id}-->`
    cursor = range.end
  }
  return shell + html.slice(cursor)
}

export function ensureSlideIdsStable(
  outer: string,
  preferredId?: string | null,
): string {
  if (!preferredId) return outer
  if (/data-slide-id\s*=\s*"/i.test(outer)) {
    return outer.replace(
      /data-slide-id\s*=\s*"[^"]*"/i,
      `data-slide-id="${preferredId}"`,
    )
  }
  if (/data-slide-id\s*=\s*'/i.test(outer)) {
    return outer.replace(
      /data-slide-id\s*=\s*'[^']*'/i,
      `data-slide-id='${preferredId}'`,
    )
  }
  return outer.replace(/<section\b/i, `<section data-slide-id="${preferredId}"`)
}

function looksLikeMetaOrClarifyReply(text: string): boolean {
  const t = (text || '').trim()
  if (!t) return true
  if (/<\/?(?:section|div|h[1-6]|p)\b/i.test(t) && /class\s*=\s*["'][^"']*\bslide\b/i.test(t)) {
    return false
  }
  if (/<<<<<<<\s*SEARCH/i.test(t)) return false
  if (
    /\b(I still need|Please paste|Paste those|Your message only|restates the|re-paste|Concrete instructions)\b/i.test(
      t,
    )
  ) {
    return true
  }
  if (
    /\bINTENT\b/i.test(t) &&
    /\b(edit|fill|new-after|rebuild-deck)\b/i.test(t) &&
    !/<section\b/i.test(t)
  ) {
    return true
  }
  return false
}

function sectionLooksLikeMeta(outer: string): boolean {
  const body = stripTags(outer).replace(/\s+/g, ' ').trim()
  if (body.length < 40) return false
  return (
    /\b(I still need|Please paste|Paste those|Concrete instructions|SEARCH\/REPLACE patches)\b/i.test(
      body,
    ) ||
    (/\bINTENT\b/i.test(body) &&
      /\b(edit|fill|new-after|rebuild)\b/i.test(body) &&
      !/\b(h1|title|slide)\b/i.test(body.slice(0, 80)))
  )
}

export function resolveSlidesAgentReply(
  baseHtml: string,
  reply: string,
  opts: {
    intent: 'edit-active' | 'fill-blank' | 'new-after' | 'rebuild-deck'
    activeSlideId?: string | null
    targetSlideIds?: string[] | null
  },
):
  | { ok: true; next: string; applied: number; mode: 'patches' | 'full' | 'slide' | 'insert' }
  | { ok: false; error: string; applied: number } {
  const normalizedReply = (reply || '').replace(/\r\n/g, '\n')
  if (looksLikeMetaOrClarifyReply(normalizedReply)) {
    return {
      ok: false,
      error:
        'Agent returned a clarification instead of slide HTML (prompt may have been truncated). Retry the edit — deck HTML and INTENT are re-sent automatically.',
      applied: 0,
    }
  }
  const baseCount = listSlidesFromHtml(baseHtml).length
  const patches = extractLooseSearchReplacePatches(normalizedReply)
  if (patches.length > 0) {
    const applied = applyLoosePatches(baseHtml, patches)
    if (applied.ok) {
      if (opts.intent === 'edit-active' || opts.intent === 'fill-blank') {
        const targetIds =
          (opts.targetSlideIds || []).filter(Boolean).length >= 2
            ? (opts.targetSlideIds || []).filter(Boolean)
            : opts.activeSlideId
              ? [opts.activeSlideId]
              : []
        const scope = validateSurgicalSlideScope(baseHtml, applied.next, targetIds)
        if (!scope.ok) return { ok: false, error: scope.error, applied: 0 }
        return { ok: true, next: applied.next, applied: scope.changed, mode: 'patches' }
      }
      const nextCount = listSlidesFromHtml(applied.next).length
      if (opts.intent !== 'rebuild-deck' && opts.intent !== 'new-after' && nextCount < baseCount) {
        return {
          ok: false,
          error: `Rejected patch: would drop slides (${baseCount} → ${nextCount}).`,
          applied: 0,
        }
      }
      return { ...applied, mode: 'patches' }
    }
  }

  const sections = extractSlideSections(normalizedReply).filter(
    (s) => !sectionLooksLikeMeta(s),
  )
  const fullDoc = extractHtmlDocument(normalizedReply)
  const fromFull = fullDoc
    ? extractSlideSections(fullDoc).filter((s) => !sectionLooksLikeMeta(s))
    : []
  const pool =
    sections.length > 0 ? sections : fromFull.length > 0 ? fromFull : []

  if (opts.intent === 'rebuild-deck') {
    if (fullDoc && fromFull.length > 0) {
      return {
        ok: true,
        next: ensureDeckShell(fromFull.join('\n')),
        applied: fromFull.length,
        mode: 'full',
      }
    }
    if (fullDoc && !fromFull.length) {
      return {
        ok: false,
        error:
          'Rebuild reply had no valid <section class="slide"> blocks — rejected so deck is not wiped.',
        applied: 0,
      }
    }
    if (pool.length > 0) {
      return {
        ok: true,
        next: ensureDeckShell(pool.join('\n')),
        applied: pool.length,
        mode: 'full',
      }
    }
  }

  const multiTargets = (opts.targetSlideIds || []).filter(Boolean)
  if (
    (opts.intent === 'edit-active' || opts.intent === 'fill-blank') &&
    multiTargets.length >= 2
  ) {
    let nextHtml = baseHtml
    let applied = 0
    const remaining = [...pool]
    for (const tid of multiTargets) {
      const byId = remaining.findIndex((s) =>
        new RegExp(
          `data-slide-id\\s*=\\s*["']${tid.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}["']`,
          'i',
        ).test(s),
      )
      let chosen: string | null = null
      if (byId >= 0) {
        chosen = remaining.splice(byId, 1)[0]
      } else if (remaining.length) {
        chosen = remaining.shift() || null
      }
      if (!chosen || sectionLooksLikeMeta(chosen)) continue
      const nextOuter = ensureSlideIdsStable(chosen, tid)
      const replaced = replaceSlideOuterHtml(nextHtml, tid, nextOuter)
      if (replaced) {
        nextHtml = replaced
        applied++
      }
    }
    if (applied > 0) {
      const nextCount = listSlidesFromHtml(nextHtml).length
      if (nextCount === baseCount) {
        return { ok: true, next: nextHtml, applied, mode: 'slide' }
      }
    }
    return {
      ok: false,
      error:
        multiTargets.length > 1
          ? `Edit selected (${multiTargets.length} slides) needs one <section> per selected id (or SEARCH/REPLACE covering each). Got ${pool.length} section(s), applied ${applied}.`
          : 'Edit selected failed',
      applied: 0,
    }
  }

  if (
    (opts.intent === 'edit-active' || opts.intent === 'fill-blank') &&
    opts.activeSlideId
  ) {
    const chosen = pickSectionForActive(pool, opts.activeSlideId)
    if (chosen && !sectionLooksLikeMeta(chosen)) {
      const nextOuter = ensureSlideIdsStable(chosen, opts.activeSlideId)
      const next = replaceSlideOuterHtml(
        baseHtml,
        opts.activeSlideId,
        nextOuter,
      )
      if (next) {
        const nextCount = listSlidesFromHtml(next).length
        if (nextCount === baseCount) {
          return { ok: true, next, applied: 1, mode: 'slide' }
        }
      }
    }
    return {
      ok: false,
      error:
        'Edit active only accepts SEARCH/REPLACE or one <section> for the active slide — full deck rewrite ignored (other slides preserved)',
      applied: 0,
    }
  }

  if (opts.intent === 'new-after') {
    const chosen = pool[0]
    if (chosen) {
      const newId = mintSlideId()
      const nextOuter = ensureSlideIdsStable(chosen, newId)
      const next = insertSlideAfter(baseHtml, opts.activeSlideId, nextOuter)
      if (next) {
        return { ok: true, next, applied: 1, mode: 'insert' }
      }
    }
    return {
      ok: false,
      error:
        'New-after only accepts one new <section> (or SEARCH/REPLACE insert) — multi-slide rebuild ignored',
      applied: 0,
    }
  }

  return {
    ok: false,
    error:
      'No usable slide HTML in reply (need SEARCH/REPLACE, one <section class="slide">, or full deck on rebuild only)',
    applied: 0,
  }
}

export function looksLikeImageGenRequest(text: string): boolean {
  const t = (text || '').toLowerCase()
  if (!t.trim()) return false
  if (
    /\b(generate|buat|bikin|create|draw|render)\b[\s\S]{0,40}\b(image|gambar|illustration|foto|photo|diagram\s*image)\b/i.test(
      t,
    )
  ) {
    return true
  }
  if (
    /\b(image|gambar|illustration)\b[\s\S]{0,30}\b(generate|buat|bikin|generation)\b/i.test(
      t,
    )
  ) {
    return true
  }
  if (
    /\b(sisipkan|embed|place|taruh|masukkan)\b[\s\S]{0,40}\b(gambar|image|diagram)\b/i.test(
      t,
    ) &&
    /\b(generate|ai|buat|bikin)\b/i.test(t)
  ) {
    return true
  }
  if (/\bdiagram\b/i.test(t) && /\b(image|gambar|generate|buat)\b/i.test(t)) {
    return true
  }
  return false
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

export function replaceImageSlotElement(
  slideHtml: string,
  slot: string,
  replacement: string,
): string {
  const openRe = new RegExp(
    `<div\\b[^>]*\\bdata-image-slot=["']${escapeRegExp(slot)}["'][^>]*>`,
    'i',
  )
  const m = openRe.exec(slideHtml)
  if (!m || m.index === undefined) {
    const anySlot = /<div\b[^>]*\bdata-image-slot=["'][^"']*["'][^>]*>/i.exec(
      slideHtml,
    )
    if (!anySlot || anySlot.index === undefined) return slideHtml
    return replaceImageSlotElementAt(slideHtml, anySlot.index, anySlot[0].length, replacement)
  }
  return replaceImageSlotElementAt(slideHtml, m.index, m[0].length, replacement)
}

function replaceImageSlotElementAt(
  slideHtml: string,
  openStart: number,
  openLen: number,
  replacement: string,
): string {
  const lower = slideHtml.toLowerCase()
  let depth = 1
  let i = openStart + openLen
  while (i < slideHtml.length && depth > 0) {
    const nextOpen = lower.indexOf('<div', i)
    const nextClose = lower.indexOf('</div>', i)
    if (nextClose < 0) break
    if (nextOpen >= 0 && nextOpen < nextClose) {
      const after = lower[nextOpen + 4]
      if (after === undefined || /[\s/>]/.test(after)) {
        depth += 1
        i = nextOpen + 4
        continue
      }
      i = nextOpen + 4
      continue
    }
    depth -= 1
    if (depth === 0) {
      let end = nextClose + 6
      const tail = slideHtml.slice(end)
      const styleTail = /^\s*<style>@keyframes\s+ie-spin[\s\S]*?<\/style>/i.exec(tail)
      if (styleTail) end += styleTail[0].length
      return slideHtml.slice(0, openStart) + replacement + slideHtml.slice(end)
    }
    i = nextClose + 6
  }
  return (
    slideHtml.slice(0, openStart) +
    replacement +
    slideHtml.slice(openStart + openLen)
  )
}

/**
 * Geometry utilities that decide where an image slot sits in the composition.
 * The placeholder paint (dashed border, grey fill, centering, label type) is
 * deliberately excluded, because that styling belongs to an empty slot and
 * would look wrong around a real picture.
 */
const SLOT_GEOMETRY_UTIL =
  /^(?:w|h|min-w|min-h|max-w|max-h|shrink|grow|basis|aspect|mx|my|absolute|inset|z|left|right|top|bottom)-|^(?:flex-1|mx-auto|relative)$/

/**
 * Reuses the geometry already authored on a slide's image slot so the element
 * keeps its composition through the pending → loading → image lifecycle. Emitting
 * a fixed w-[380px]/h-[260px] box regardless of what the slide declared made the
 * slot jump size when generation finished.
 */
export function existingSlotLayoutClass(
  slideHtml: string,
  slot: string,
): string | null {
  const openRe = new RegExp(
    `<div\\b[^>]*\\bdata-image-slot=["']${escapeRegExp(slot)}["'][^>]*>`,
    'i',
  )
  const m = openRe.exec(slideHtml)
  if (!m) return null
  const classMatch = /\bclass=["']([^"']*)["']/i.exec(m[0])
  if (!classMatch) return null
  const kept = classMatch[1]
    .split(/\s+/)
    .filter((c) => c && SLOT_GEOMETRY_UTIL.test(c))
  if (!kept.length) return null
  return kept.join(' ')
}

function aiSlotFallbackClass(slot: ImageSlotId): string {
  const fallbacks: Record<ImageSlotId, string> = {
    right: 'w-[380px] max-w-[42%] h-[260px] shrink-0',
    left: 'w-[380px] max-w-[42%] h-[260px] shrink-0',
    hero: 'w-full max-w-lg h-[200px] mx-auto',
    'tile-1': 'w-full h-[170px]',
    inline: 'w-full h-[170px]',
    background: 'absolute inset-0',
  }
  const geometry = fallbacks[slot] || fallbacks.right
  const paint =
    slot === 'background'
      ? 'hidden'
      : 'rounded-2xl border border-dashed border-slate-500/50 bg-slate-900/30 text-slate-400 text-sm font-semibold flex items-center justify-center'
  return `${geometry} ${paint}`
}

function imageSlotContainerClass(slideHtml: string, slot: string): string {
  const authored = existingSlotLayoutClass(slideHtml, slot)
  return (
    authored ||
    {
      right: 'w-[380px] max-w-[42%] h-[260px] shrink-0',
      left: 'w-[380px] max-w-[42%] h-[260px] shrink-0',
      hero: 'w-full max-w-lg h-[200px] mx-auto',
      'tile-1': 'w-full h-[170px]',
      inline: 'w-full h-[170px]',
    }[slot] ||
    'w-[380px] max-w-[42%] h-[260px] shrink-0'
  )
}

export function ensureAiImagePlaceholderOnSlide(
  html: string,
  slideId: string,
  prompt: string,
  slot: ImageSlotId = 'right',
): string {
  const target = listSlidesFromHtml(html).find((s) => s.id === slideId)
  if (!target) return html
  const safePrompt = prompt.replace(/"/g, "'").slice(0, 220)
  const layout = existingSlotLayoutClass(target.outerHtml, slot)
  const cls = layout
    ? `${layout} rounded-2xl border border-dashed border-slate-500/50 bg-slate-900/30 text-slate-400 text-sm font-semibold flex items-center justify-center`
    : aiSlotFallbackClass(slot)
  const box = `<div data-image-slot="${slot}" data-ai-prompt="${safePrompt}" data-ai-status="pending" class="${cls}">Image slot</div>`
  let nextSlide = target.outerHtml
  if (new RegExp(`data-image-slot="${slot}"`, 'i').test(nextSlide)) {
    nextSlide = replaceImageSlotElement(nextSlide, slot, box)
  } else if (/slide-blank/i.test(nextSlide)) {
    // For blank slide: build a split layout with text left + image right (or hero if slot=hero).
    if (slot === 'hero') {
      nextSlide = `<section class="slide flex flex-col p-10" data-slide-id="${slideId}" data-layout="hero" data-title="${target.title.replace(/"/g, '')}">
  <div class="flex flex-col min-w-0">
    <h2 class="text-4xl font-bold tracking-tight" style="margin:0 0 0.75rem 0">${target.title}</h2>
    <p class="text-lg opacity-70" style="margin:0">Illustration generating…</p>
  </div>
  ${box}
</section>`
    } else {
      nextSlide = `<section class="slide flex p-10" data-slide-id="${slideId}" data-layout="two-column" data-title="${target.title.replace(/"/g, '')}">
  <div class="flex-1 flex flex-col justify-center min-w-0">
    <h2 class="text-4xl font-bold tracking-tight" style="margin:0 0 0.75rem 0">${target.title}</h2>
    <p class="text-lg opacity-70" style="margin:0">Illustration generating…</p>
  </div>
  ${box}
</section>`
    }
  } else {
    nextSlide = nextSlide.replace(/<\/section>\s*$/i, `${box}</section>`)
  }
  const idx = html.indexOf(target.outerHtml)
  if (idx < 0) return html
  return html.slice(0, idx) + nextSlide + html.slice(idx + target.outerHtml.length)
}

type LoosePatch = { search: string; replace: string }

function extractLooseSearchReplacePatches(text: string): LoosePatch[] {
  if (!text) return []
  const out: LoosePatch[] = []
  const markL = '<<<<<<<'
  const markM = '======='
  const markR = '>>>>>>>'
  const re = new RegExp(
    `${markL}\\s*SEARCH\\s*\\n([\\s\\S]*?)\\n${markM}\\s*\\n([\\s\\S]*?)\\n${markR}\\s*REPLACE`,
    'gi',
  )
  let m: RegExpExecArray | null
  while ((m = re.exec(text)) !== null) {
    const search = (m[1] ?? '').replace(/\r\n/g, '\n')
    const replace = (m[2] ?? '').replace(/\r\n/g, '\n')
    if (!search && !replace) continue
    out.push({ search, replace })
  }
  if (out.length) return out

  const fenceRe =
    /```(?:diff|patch|html)?\s*\n<<<<<<<\s*SEARCH\s*\n([\s\S]*?)\n=======\s*\n([\s\S]*?)\n>>>>>>>\s*REPLACE\s*```/gi
  while ((m = fenceRe.exec(text)) !== null) {
    out.push({
      search: (m[1] ?? '').replace(/\r\n/g, '\n'),
      replace: (m[2] ?? '').replace(/\r\n/g, '\n'),
    })
  }
  return out
}

function applyLoosePatches(
  base: string,
  patches: LoosePatch[],
):
  | { ok: true; next: string; applied: number }
  | { ok: false; error: string; applied: number } {
  let next = base
  let applied = 0
  for (let i = 0; i < patches.length; i += 1) {
    const p = patches[i]
    let idx = next.indexOf(p.search)
    if (idx < 0) {
      const compactNeedle = p.search.replace(/\s+/g, ' ').trim()
      const compactHay = next.replace(/\s+/g, ' ')
      const cIdx = compactHay.indexOf(compactNeedle)
      if (cIdx < 0 || compactNeedle.length < 8) {
        return {
          ok: false,
          error: `Patch ${i + 1}/${patches.length} SEARCH not found`,
          applied,
        }
      }
      idx = approxIndexFromCollapsed(next, cIdx)
    }
    if (idx < 0) {
      return {
        ok: false,
        error: `Patch ${i + 1}/${patches.length} SEARCH not found`,
        applied,
      }
    }
    next = next.slice(0, idx) + p.replace + next.slice(idx + p.search.length)
    applied += 1
  }
  return { ok: true, next, applied }
}

function approxIndexFromCollapsed(original: string, collapsedIndex: number): number {
  let ci = 0
  let inWs = false
  let started = false
  for (let i = 0; i < original.length; i += 1) {
    const ch = original[i]
    const ws = /\s/.test(ch)
    if (ws) {
      if (started && !inWs) {
        if (ci === collapsedIndex) return i
        ci += 1
        inWs = true
      }
      continue
    }
    if (!started) started = true
    inWs = false
    if (ci === collapsedIndex) return i
    ci += 1
  }
  return ci === collapsedIndex ? original.length : -1
}

export function forcePlaceImageInSlide(
  html: string,
  slideId: string,
  asset: { id: string; src: string; alt?: string },
  slot: ImageSlotId,
): string | null {
  const slides = listSlidesFromHtml(html)
  const target = slides.find((s) => s.id === slideId)
  if (!target) return null
  const imgId = `img-${slideId}-${slot}`
  const img = `<img data-ex="image" data-ex-id="${escapeAttr(imgId)}" data-image-slot="${escapeAttr(slot)}" data-asset-id="${escapeAttr(asset.id)}" src="${escapeAttr(asset.src)}" alt="${escapeAttr(asset.alt || '')}" class="w-full h-full object-cover rounded-xl" />`
  let nextSlide = target.outerHtml

  if (slot === 'background') {
    const bgLayer = `<div data-ex="image" data-ex-id="${escapeAttr(imgId)}" data-image-slot="background" class="absolute inset-0 z-0 overflow-hidden pointer-events-none" aria-hidden="true">${img}</div>`
    if (/data-image-slot=["']background["']/i.test(nextSlide)) {
      nextSlide = replaceImageSlotElement(nextSlide, 'background', bgLayer)
    } else if (/<\/section>/i.test(nextSlide)) {
      nextSlide = nextSlide.replace(/<\/section>/i, `${bgLayer}</section>`)
    } else {
      nextSlide = `${nextSlide}${bgLayer}`
    }
  } else {
    const containerCls = `${imageSlotContainerClass(target.outerHtml, slot)} overflow-hidden rounded-xl border border-slate-700/50`
    const slotHtml = `<div data-ex="image" data-ex-id="${escapeAttr(imgId)}" data-image-slot="${slot}" class="${containerCls}">${img}</div>`
    if (nextSlide.includes('slide-blank')) {
      if (slot === 'hero') {
        nextSlide = `<section class="slide flex flex-col p-10" data-slide-id="${slideId}" data-layout="hero" data-title="Slide">
  <div class="flex flex-col min-w-0">
    <h2 class="text-4xl font-bold tracking-tight" style="margin:0 0 0.75rem 0">Image slide</h2>
    <p class="text-lg opacity-70" style="margin:0">Edit copy with the agent.</p>
  </div>
  ${slotHtml}
</section>`
      } else {
        nextSlide = `<section class="slide flex p-10" data-slide-id="${slideId}" data-layout="two-column" data-title="Slide">
  <div class="flex-1 flex flex-col justify-center min-w-0">
    <h2 class="text-4xl font-bold tracking-tight" style="margin:0 0 0.75rem 0">Image slide</h2>
    <p class="text-lg opacity-70" style="margin:0">Edit copy with the agent.</p>
  </div>
  ${slotHtml}
</section>`
      }
    } else if (/data-image-slot=/i.test(nextSlide)) {
      nextSlide = replaceImageSlotElement(nextSlide, slot, slotHtml)
    } else {
      nextSlide = nextSlide.replace(
        /<\/section>\s*$/i,
        `${slotHtml}</section>`,
      )
    }
  }

  const idx = html.indexOf(target.outerHtml)
  if (idx < 0) return null
  return html.slice(0, idx) + nextSlide + html.slice(idx + target.outerHtml.length)
}

export function extractHtmlDocument(text: string): string | null {
  if (!text) return null
  const normalized = text.replace(/\r\n/g, '\n')

  // Closed fence: ```html ... ``` or ``` ... ```
  const fence =
    /```(?:html|htm)?\s*\r?\n([\s\S]*?)```/i.exec(normalized) ||
    /```(?:html|htm)\s+([\s\S]*?)```/i.exec(normalized)
  if (fence?.[1]?.trim()) {
    const body = fence[1].trim()
    if (looksLikeDeckHtml(body)) return ensureDeckShell(body)
  }

  // Unclosed fence: ```html\n<html>... (no closing ```)
  const openFence = /```(?:html|htm)?\s*\r?\n([\s\S]*)$/i.exec(normalized)
  if (openFence?.[1]) {
    const body = openFence[1].trim()
    if (body.length > 80 && looksLikeDeckHtml(body)) {
      return ensureDeckShell(body)
    }
  }

  // Raw document (maybe with prose before it)
  const idx = normalized.search(/<!DOCTYPE\s+html|<html[\s>]/i)
  if (idx >= 0) {
    let body = normalized.slice(idx).trim()
    // Strip trailing markdown fence if model closed after prose+html
    body = body.replace(/\n```[\s\S]*$/i, '').trim()
    if (looksLikeDeckHtml(body)) return body
  }

  // Sections only
  if (/<section\b[^>]*class=["'][^"']*\bslide\b/i.test(normalized)) {
    return ensureDeckShell(normalized)
  }
  // class='slide' with single quotes
  if (/class=['"][^'"]*\bslide\b/i.test(normalized) && /<section\b/i.test(normalized)) {
    return ensureDeckShell(normalized)
  }
  return null
}

function looksLikeDeckHtml(body: string): boolean {
  if (!body) return false
  if (/<!DOCTYPE\s+html|<html[\s>]/i.test(body)) return true
  if (/class=["'][^"']*\bslide\b/i.test(body)) return true
  if (/<section\b[^>]*data-slide-id=/i.test(body)) return true
  if (/\binferenesia-deck\b/i.test(body)) return true
  return false
}

function ensureDeckShell(body: string): string {
  if (/<!DOCTYPE\s+html|<html[\s>]/i.test(body)) return body
  return emptyHtmlDeck('Deck').replace(
    /<div class="inferenesia-deck"[\s\S]*?<\/div>/,
    `<div class="inferenesia-deck" data-width="${SLIDE_W}" data-height="${SLIDE_H}">\n${body}\n</div>`,
  )
}

function extractDeckHeadAssets(fullHtml: string): {
  styles: string
  hasTailwindCdn: boolean
} {
  const styles: string[] = []
  const re = /<style\b[^>]*>([\s\S]*?)<\/style>/gi
  let m: RegExpExecArray | null
  while ((m = re.exec(fullHtml || '')) !== null) {
    const body = (m[1] || '').trim()
    if (body) styles.push(body)
  }
  const hasTailwindCdn = /cdn\.tailwindcss\.com/i.test(fullHtml || '')
  return { styles: styles.join('\n'), hasTailwindCdn }
}

export function previewSrcDoc(
  fullHtml: string,
  activeSlideId: string | null,
  opts?: { thumb?: boolean },
): string {
  const slides = listSlidesFromHtml(fullHtml)
  if (!slides.length) {
    return fullHtml || emptyHtmlDeck()
  }
  const active =
    slides.find((s) => s.id === activeSlideId) || slides[0]
  const assets = extractDeckHeadAssets(fullHtml)
  const thumb = Boolean(opts?.thumb)
  const extraDeckCss = assets.styles
    ? `\n/* deck styles from source */\n${assets.styles}`
    : ''
  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="color-scheme" content="dark light" />
<style>${deckBaseStyles()}${extraDeckCss}
html,body{margin:0;min-height:100%;}
body{
  display:flex;align-items:center;justify-content:center;
  min-height:100vh;margin:0;
  background:#020617;
}
.slide{
  box-shadow:${thumb ? 'none' : '0 20px 50px rgba(0,0,0,.45)'};
  border-radius:${thumb ? '0' : '12px'};
  flex-shrink:0;
  color:inherit;
}
</style>
<script src="https://cdn.tailwindcss.com"></script>
<script>tailwind.config={corePlugins:{preflight:false}}</script>
<style>
/* Preview keeps the authored 1280×720 composition; source CSS remains authoritative. */
.slide{box-sizing:border-box;}
</style>
</head>
<body>
${active.outerHtml}
</body>
</html>`
}

export type ImageGenSlot = {
  slideId: string
  slot: string
  prompt: string
  status: 'pending' | 'loading' | 'done' | 'error'
}

/**
 * Builds an illustration brief from the slide's actual copy rather than its
 * title alone. A title-only prompt ("Illustration for: Overview") gave the
 * image model nothing to work with, which is what produced interchangeable
 * generic stock art on every slide.
 */
function slotPromptFromSlide(slideHtml: string, title: string): string {
  const body = slideHtml
    .replace(/<(script|style)\b[\s\S]*?<\/\1>/gi, ' ')
    .replace(/<[^>]+>/g, ' ')
    .replace(/&nbsp;/gi, ' ')
    .replace(/\s+/g, ' ')
    .trim()
  const detail = body.slice(0, 260)
  return [
    `Editorial illustration supporting a presentation slide titled "${title}".`,
    detail ? `Slide content: ${detail}` : '',
    'Compose for the described subject specifically; do not produce generic business stock imagery.',
    'No text, letters, numbers, logos, or watermarks in the image.',
  ]
    .filter(Boolean)
    .join(' ')
}

export function listAiImageSlots(html: string): ImageGenSlot[] {
  const slides = listSlidesFromHtml(html)
  const out: ImageGenSlot[] = []
  for (const s of slides) {
    const openRe = /<div\b[^>]*\bdata-image-slot=["']([^"']+)["'][^>]*>/gi
    let m: RegExpExecArray | null
    const outer = s.outerHtml
    while ((m = openRe.exec(outer)) !== null) {
      const slot = m[1] || 'right'
      const openTag = m[0]
      if (/data-ai-status=["'](?:done|error)["']/i.test(openTag)) continue
      const promptMatch = /data-ai-prompt=["']([^"']*)["']/i.exec(openTag)
      const prompt =
        (promptMatch?.[1] || '').trim() || slotPromptFromSlide(outer, s.title)
      const hasImg = new RegExp(
        `data-image-slot=["']${escapeRegExp(slot)}["'][\\s\\S]*?<img\\b`,
        'i',
      ).test(outer)
      if (hasImg) continue
      out.push({
        slideId: s.id,
        slot,
        prompt,
        status: 'pending',
      })
    }
    if (
      /data-ai-image="1"|data-gen-image="1"|class="[^"]*ai-image-slot/i.test(
        outer,
      ) &&
      !out.some((x) => x.slideId === s.id)
    ) {
      out.push({
        slideId: s.id,
        slot: 'right',
        prompt: slotPromptFromSlide(outer, s.title),
        status: 'pending',
      })
    }
  }
  return out
}

export function markImageSlotLoading(
  html: string,
  slideId: string,
  slot: string,
): string {
  const target = listSlidesFromHtml(html).find((s) => s.id === slideId)
  if (!target) return html
  let nextSlide = target.outerHtml
  const cls =
    slot === 'background'
      ? 'hidden'
      : `${imageSlotContainerClass(nextSlide, slot)} rounded-2xl border border-dashed border-slate-500/50 bg-slate-900/40 text-slate-300 flex items-center justify-center`
  const loading = `<div data-image-slot="${slot}" data-ai-status="loading" class="${cls}" style="flex-direction:column;gap:10px;text-align:center;font-size:14px;font-weight:600"><span class="ai-spin" style="display:block;width:28px;height:28px;border:3px solid rgba(148,163,184,.35);border-top-color:#60a5fa;border-radius:50%;animation:ie-spin 0.8s linear infinite"></span>Generating image…</div><style>@keyframes ie-spin{to{transform:rotate(360deg)}}</style>`
  if (/data-image-slot=/i.test(nextSlide)) {
    nextSlide = replaceImageSlotElement(nextSlide, slot, loading)
  } else {
    nextSlide = nextSlide.replace(/<\/section>\s*$/i, `${loading}</section>`)
  }
  const idx = html.indexOf(target.outerHtml)
  if (idx < 0) return html
  return html.slice(0, idx) + nextSlide + html.slice(idx + target.outerHtml.length)
}

export function applyImageToSlot(
  html: string,
  slideId: string,
  slot: string,
  src: string,
  alt = '',
): string | null {
  return forcePlaceImageInSlide(
    html,
    slideId,
    { id: `ai_${slideId}_${slot}`, src, alt },
    (slot as ImageSlotId) || 'right',
  )
}

export function markImageSlotFailed(
  html: string,
  slideId: string,
  slot: string,
  message = 'Image failed',
): string {
  const target = listSlidesFromHtml(html).find((s) => s.id === slideId)
  if (!target) return html
  const cls =
    slot === 'background'
      ? 'hidden'
      : `${imageSlotContainerClass(target.outerHtml, slot)} rounded-2xl border border-dashed border-rose-500/40 bg-slate-900/40 text-rose-300 text-sm font-semibold flex items-center justify-center px-4 text-center`
  const safeMsg = escapeHtml(message).slice(0, 120)
  const failed = `<div data-image-slot="${slot}" data-ai-status="error" class="${cls}">${safeMsg}</div>`
  let nextSlide = target.outerHtml
  if (/data-image-slot=/i.test(nextSlide)) {
    nextSlide = replaceImageSlotElement(nextSlide, slot, failed)
  } else {
    return html
  }
  const idx = html.indexOf(target.outerHtml)
  if (idx < 0) return html
  return html.slice(0, idx) + nextSlide + html.slice(idx + target.outerHtml.length)
}

export function presentSrcDoc(fullHtml: string): string {
  const slides = listSlidesFromHtml(fullHtml)
  if (!slides.length) return emptyHtmlDeck()
  const assets = extractDeckHeadAssets(fullHtml)
  const extraDeckCss = assets.styles
    ? `\n/* deck styles from source */\n${assets.styles}`
    : ''
  const slidesHtml = slides.map((s) => s.outerHtml).join('\n')
  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="color-scheme" content="dark light" />
<style>${deckBaseStyles()}${extraDeckCss}
html,body{margin:0;padding:0;background:#020617;overflow:hidden;}
body{display:flex;align-items:center;justify-content:center;min-height:100vh;}
#ie-stage-wrap{width:100vw;height:100vh;display:flex;align-items:center;justify-content:center;overflow:hidden;background:#020617;}
#ie-stage{width:${SLIDE_W}px;height:${SLIDE_H}px;transform-origin:center center center;position:relative;}
.inferenesia-deck{margin:0 !important;padding:0 !important;background:transparent !important;}
.slide{
  position:absolute !important;
  top:0 !important;left:0 !important;
  width:100% !important;height:100% !important;
  opacity:0 !important;
  pointer-events:none !important;
  visibility:hidden !important;
  transition:opacity 0.15s ease !important;
}
.slide.is-active{
  opacity:1 !important;
  pointer-events:auto !important;
  visibility:visible !important;
}
.inferenesia-deck:not(.ie-ready) .slide:first-of-type{
  opacity:1 !important;
  pointer-events:auto !important;
  visibility:visible !important;
}
</style>
<script src="https://cdn.tailwindcss.com"></script>
<script>tailwind.config={corePlugins:{preflight:false}}</script>
</head>
<body>
<div id="ie-stage-wrap">
  <div id="ie-stage">
    <div class="inferenesia-deck">
${slidesHtml}
    </div>
  </div>
</div>
<script>
(function(){
  function init(){
    var slides=[].slice.call(document.querySelectorAll('.slide, [data-slide-id], section'));
    var i=0;
    function show(n){
      if(!slides.length) return;
      i=(n+slides.length)%slides.length;
      slides.forEach(function(s,idx){
        s.classList.toggle('is-active', idx===i);
      });
    }
    function fit(){
      var el=document.getElementById('ie-stage');
      if(!el) return;
      var sx=window.innerWidth/${SLIDE_W}, sy=window.innerHeight/${SLIDE_H};
      var s=Math.max(0.05, Math.min(sx,sy)*0.96);
      el.style.transform='scale('+s+')';
    }
    document.addEventListener('keydown',function(e){
      if(e.key==='ArrowRight'||e.key===' '){ e.preventDefault(); show(i+1); }
      if(e.key==='ArrowLeft'){ e.preventDefault(); show(i-1); }
      if(e.key==='Escape'){ try{parent.postMessage('inferenesia:present:exit','*')}catch(ex){} }
    });
    window.addEventListener('resize',fit);
    show(0);
    fit();
    var deckEl=document.querySelector('.inferenesia-deck');
    if(deckEl) deckEl.classList.add('ie-ready');
  }
  if(document.readyState==='loading'){
    document.addEventListener('DOMContentLoaded',init);
  } else {
    init();
  }
})();
</script>
</body>
</html>`
}

export function isLegacyExcalidrawSlides(content: string): boolean {
  return content.includes('inferenesia-slides-excalidraw')
}
