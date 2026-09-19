import { slidesDesignSystemPrompt } from './htmlDeck'

export type TextDensityId = 'minimal' | 'ringkas' | 'detail' | 'luas'

export type VisualThemeId =
  | 'tranquil'
  | 'slate'
  | 'malibu'
  | 'dialogue'
  | 'petrol'
  | 'stardust'
  | 'coal'
  | 'icebreaker'
  | 'custom'

export type ImageSourceId =
  | 'none'
  | 'placeholder'
  | 'ai'
  | 'stock'
  | 'accent'

/** Visual ornament level — not the same as color theme. */
export type DecorStyleId = 'plain' | 'decorative'

export type OutlineCard = {
  id: string
  title: string
  bullets: string[]
}

export type SlidesWizardPlan = {
  topic: string
  materials: string
  density: TextDensityId
  theme: VisualThemeId
  customThemeInstruction: string
  decor: DecorStyleId
  imageSource: ImageSourceId
  outline: OutlineCard[]
  language: 'id' | 'en'
}

export const TEXT_DENSITY: {
  id: TextDensityId
  label: string
  hint: string
  prompt: string
}[] = [
  {
    id: 'minimal',
    label: 'Minimal',
    hint: 'Satu ide utama',
    prompt: [
      'MINIMAL density: one clear idea per slide.',
      'Use an editorial headline plus, only when needed, one short supporting line or one meaningful visual.',
      'Do not add bullets, cards, icons, or filler simply to occupy space.',
    ].join('\n'),
  },
  {
    id: 'ringkas',
    label: 'Ringkas',
    hint: '2–3 bukti pendukung',
    prompt: [
      'RINGKAS density: a concise headline with 2–3 supporting ideas.',
      'Supporting ideas may be a short editorial list, comparison, flow, or evidence—not automatically bullets or cards.',
      'Prefer one short sentence per idea. Remove repetition.',
    ].join('\n'),
  },
  {
    id: 'detail',
    label: 'Detail',
    hint: '3–5 poin terjelaskan',
    prompt: [
      'DETAIL density: enough context to explain 3–5 distinct ideas.',
      'Use a concise lede only when it adds context, then organize the detail in the form that matches its meaning.',
      'Keep each supporting explanation to one useful sentence. Split the narrative rather than clipping it.',
    ].join('\n'),
  },
  {
    id: 'luas',
    label: 'Luas',
    hint: 'Lengkap, tetap terbaca',
    prompt: [
      'LUAS density: comprehensive but still presentation-readable.',
      'Cover nuance through the full deck, not by packing every detail onto every slide. Use up to 5 compact supporting ideas on a slide when needed.',
      'If the outline contains more material than one 1280×720 slide can hold, synthesize or distribute it across the planned slides. Never use tiny type, walls of text, or clipped containers.',
    ].join('\n'),
  },
]

export const VISUAL_THEMES: {
  id: VisualThemeId
  label: string
  swatch: string
  fg: string
  card: string
  prompt: string
}[] = [
  {
    id: 'tranquil',
    label: 'Tranquil',
    swatch: 'linear-gradient(145deg,#e8f1ff 0%,#f7fbff 55%,#dfe9ff 100%)',
    fg: '#0f172a',
    card: '#ffffff',
    prompt:
      'Theme Tranquil: soft ice-blue gradient background. Light mood → choose text that contrasts against the light surface. White elevated cards with readable text inside. Calm blue accent. Airy and professional. Pick a common PPT-safe font pairing that fits the mood.',
  },
  {
    id: 'slate',
    label: 'Slate',
    swatch: 'linear-gradient(145deg,#0f172a 0%,#1e293b 100%)',
    fg: '#f8fafc',
    card: '#1e293b',
    prompt:
      'Theme Slate: dark slate surfaces. Dark mood → choose text that contrasts against the dark surface. Subtle blue accent, dark cards with readable text inside. Pick a common PPT-safe font pairing.',
  },
  {
    id: 'malibu',
    label: 'Malibu',
    swatch: 'linear-gradient(145deg,#fce7f3 0%,#fbcfe8 50%,#e9d5ff 100%)',
    fg: '#1e1b4b',
    card: '#fff7fb',
    prompt:
      'Theme Malibu: soft pink–lavender gradient. Light mood → choose text that contrasts against the light surface. Light cards with readable text inside. Playful but clean accent. Pick a common PPT-safe font pairing.',
  },
  {
    id: 'dialogue',
    label: 'Dialogue',
    swatch: 'linear-gradient(145deg,#f8fafc 0%,#e2e8f0 100%)',
    fg: '#0f172a',
    card: '#ffffff',
    prompt:
      'Theme Dialogue: clean light gray/white. Light mood → choose strong titles that contrast against the light surface. Minimal borders, editorial corporate look. Pick a common PPT-safe font pairing.',
  },
  {
    id: 'petrol',
    label: 'Petrol',
    swatch: 'linear-gradient(145deg,#ecfdf5 0%,#d1fae5 40%,#a7f3d0 100%)',
    fg: '#064e3b',
    card: '#ffffff',
    prompt:
      'Theme Petrol: soft mint/teal field. Light mood → choose text that contrasts against the light surface. White cards with readable text inside, teal accent. Pick a common PPT-safe font pairing.',
  },
  {
    id: 'stardust',
    label: 'Stardust',
    swatch: 'linear-gradient(145deg,#020617 0%,#111827 60%,#1e1b4b 100%)',
    fg: '#f8fafc',
    card: '#111827',
    prompt:
      'Theme Stardust: near-black cosmic background. Dark mood → choose text that contrasts against the dark surface. Subtle indigo glow, dark cards with readable text inside. Pick a common PPT-safe font pairing.',
  },
  {
    id: 'coal',
    label: 'Coal',
    swatch: 'linear-gradient(145deg,#09090b 0%,#18181b 100%)',
    fg: '#fafafa',
    card: '#27272a',
    prompt:
      'Theme Coal: pure black/zinc. Dark mood → choose text that contrasts against the dark surface. High contrast, minimal accent. Pick a common PPT-safe font pairing.',
  },
  {
    id: 'icebreaker',
    label: 'Icebreaker',
    swatch: 'linear-gradient(145deg,#f0f9ff 0%,#e0f2fe 50%,#bae6fd 100%)',
    fg: '#0c4a6e',
    card: '#ffffff',
    prompt:
      'Theme Icebreaker: bright sky-blue wash. Light mood → choose text that contrasts against the light surface. White cards with readable text inside, crisp cyan accent. Pick a common PPT-safe font pairing.',
  },
  {
    id: 'custom',
    label: 'Custom',
    swatch:
      'repeating-linear-gradient(45deg,#1e293b 0 6px,#0f172a 6px 12px)',
    fg: '#f8fafc',
    card: '#1e293b',
    prompt:
      'Theme Custom: follow the user-provided custom theme instruction below. CONTRAST principle always applies: light BG → darker text; dark BG → lighter text. You choose actual colors from the palette. If empty, fall back to a clean neutral theme. Pick a common PPT-safe font pairing.',
  },
]

export const DECOR_STYLES: {
  id: DecorStyleId
  label: string
  hint: string
  prompt: string
}[] = [
  {
    id: 'plain',
    label: 'Biasa',
    hint: 'Bersih, ber-depth — tanpa ornament',
    prompt: [
      '## Decor style: PLAIN',
      '- Use typography, alignment, whitespace, restrained rules, and subtle surface contrast.',
      '- Do not add floating blobs, stickers, emoji, glass panels, or decorative card grids.',
      '- A plain slide may still use one purposeful color field, crop, or border when it clarifies hierarchy.',
    ].join('\n'),
  },
  {
    id: 'decorative',
    label: 'Dekoratif',
    hint: 'Aksen visual yang terukur',
    prompt: [
      '## Decor style: DECORATIVE',
      '- Add at most one or two coherent visual gestures when they support the composition: a field, rule, texture, crop, connector, or geometric accent.',
      '- Vary those gestures with the narrative; do not stamp identical blobs, gradient meshes, emoji, or floating shapes on every slide.',
      '- Decoration stays behind content, carries no unsupported meaning, and never competes with the headline.',
    ].join('\n'),
  },
]

export const IMAGE_SOURCES: {
  id: ImageSourceId
  label: string
  desc: string
  prompt: string
}[] = [
  {
    id: 'none',
    label: 'Tanpa gambar',
    desc: 'Hanya tipografi, kartu, dan layout',
    prompt:
      'IMAGE POLICY: no photos/illustrations. Typography + cards + shapes only. Do not invent image tiles for bullets.',
  },
  {
    id: 'placeholder',
    label: 'Penampung gambar',
    desc: 'Kotak placeholder kosong untuk diganti nanti',
    prompt:
      'IMAGE POLICY: use empty dashed placeholder boxes (data-image-slot) only on 0–2 slides where a visual would help. Never replace every bullet with an image card. No fake stock photos.',
  },
  {
    id: 'ai',
    label: 'Gambar AI',
    desc: 'Placeholder dulu; generate di background setelah deck siap',
    prompt:
      'IMAGE POLICY: On 1–2 slides only, include a dashed placeholder box with BOTH attributes: data-image-slot="right" (or left/hero/inline) AND data-ai-prompt="specific English scene describing THIS slide\'s subject, no text in image". Write the prompt from the slide\'s actual content — never a generic business-stock brief reused across slides.\nSize each placeholder from the composition it sits in: give it an explicit width/height (or aspect ratio) that suits that layout, and make different slides use different image proportions rather than repeating one fixed box. Do NOT use flex-1 min-h, which creates full-width grey bars.\nMost slides stay text-only. Never image every bullet.',
  },
  {
    id: 'stock',
    label: 'Stok foto (gaya)',
    desc: 'Layout siap foto stok; gunakan placeholder berlabel',
    prompt:
      'IMAGE POLICY: design layouts ready for stock photos on 1–2 slides only (hero/split). Use labeled placeholders with data-image-slot, not random images on every point.',
  },
  {
    id: 'accent',
    label: 'Gambar aksen tema',
    desc: 'Aksen dekoratif dari tema (bentuk/gradient), bukan foto',
    prompt:
      'IMAGE POLICY: decorative theme shapes/gradients only (no stock photos). Prefer pure UI. Optional soft accent panels on split layouts.',
  },
]

export function mintOutlineId(): string {
  return `oc_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 5)}`
}

export function emptyOutlineCard(title = 'Slide baru'): OutlineCard {
  return { id: mintOutlineId(), title, bullets: [] }
}

export function themeById(id: VisualThemeId) {
  return VISUAL_THEMES.find((t) => t.id === id) || VISUAL_THEMES[0]
}

export function densityById(id: TextDensityId) {
  return TEXT_DENSITY.find((d) => d.id === id) || TEXT_DENSITY[1]
}

export function decorById(id: DecorStyleId) {
  return DECOR_STYLES.find((d) => d.id === id) || DECOR_STYLES[1]
}

export function imageSourceById(id: ImageSourceId) {
  return IMAGE_SOURCES.find((s) => s.id === id) || IMAGE_SOURCES[0]
}

export function buildOutlineAgentPrompt(opts: {
  topic: string
  materials: string
  slideCount: number
  language: 'id' | 'en'
}): string {
  const lang =
    opts.language === 'id'
      ? 'Write all visible copy in natural Bahasa Indonesia.'
      : 'Write all visible copy in natural English.'
  return [
    'Plan an editorial presentation outline for Inferenesia Slides.',
    `Return exactly ${opts.slideCount} slides as JSON only (no markdown fence or prose).`,
    lang,
    'Schema: {"slides":[{"title":"string","bullets":["string",...]}]}',
    'Rules:',
    '- Treat the topic and supplied materials as the only factual source. Do not invent numbers, quotations, research, customers, dates, or capabilities.',
    '- Each title is a concise takeaway or useful question, not a generic section label.',
    '- Build a narrative arc rather than a list of categories. Opening and closing slides are optional and must suit the request.',
    '- bullets are content notes for the slide designer, not a demand to render bullets. Use 0–5 short, non-overlapping notes per slide.',
    '- Do not repeat the title in its notes. Preserve uncertainty or source limitations.',
    '',
    '## Topic / user request',
    opts.topic.trim() || '(see materials)',
    '',
    '## Materials / notes',
    (opts.materials || '').trim() || '(none)',
  ].join('\n')
}

export function parseOutlineFromReply(
  text: string,
  fallbackCount: number,
): OutlineCard[] {
  const raw = (text || '').trim()
  if (!raw) return []
  let jsonStr = raw
  const fence = /```(?:json)?\s*([\s\S]*?)```/i.exec(raw)
  if (fence?.[1]) jsonStr = fence[1].trim()
  const start = jsonStr.indexOf('{')
  const end = jsonStr.lastIndexOf('}')
  if (start >= 0 && end > start) jsonStr = jsonStr.slice(start, end + 1)
  try {
    const data = JSON.parse(jsonStr) as {
      slides?: Array<{ title?: string; bullets?: string[] }>
    }
    const slides = Array.isArray(data.slides) ? data.slides : []
    const out = slides
      .map((s) => ({
        id: mintOutlineId(),
        title: String(s.title || 'Slide').trim() || 'Slide',
        bullets: Array.isArray(s.bullets)
          ? s.bullets.map((b) => String(b || '').trim()).filter(Boolean)
          : [],
      }))
      .filter((s) => s.title)
    if (out.length) return out
  } catch {
    void 0
  }
  return Array.from({ length: Math.max(1, fallbackCount) }, (_, i) =>
    emptyOutlineCard(`Slide ${i + 1}`),
  )
}

export function buildDeckFromPlanPrompt(plan: {
  topic: string
  materials: string
  density: TextDensityId
  theme: VisualThemeId
  customThemeInstruction?: string
  decor?: DecorStyleId
  imageSource: ImageSourceId
  outline: OutlineCard[]
  language: 'id' | 'en'
}): string {
  const theme = themeById(plan.theme)
  const density = densityById(plan.density)
  const decor = decorById(plan.decor || 'decorative')
  const images = imageSourceById(plan.imageSource)
  const lang =
    plan.language === 'id'
      ? 'All visible slide copy must be natural Bahasa Indonesia.'
      : 'All visible slide copy must be natural English.'
  const outlineBlock = plan.outline
    .map((s, i) => {
      const notes =
        s.bullets.length > 0
          ? s.bullets.map((b) => `  - ${b}`).join('\n')
          : '  - (derive only from the title and source materials)'
      return `${i + 1}. ${s.title}\n${notes}`
    })
    .join('\n')

  const themeLines =
    plan.theme === 'custom'
      ? [
          theme.prompt,
          'Custom direction from the user:',
          (plan.customThemeInstruction || '').trim() ||
            '(empty — use a restrained neutral editorial theme)',
        ]
      : [theme.prompt]

  return [
    'Generate one complete Inferenesia HTML presentation.',
    '',
    '## Output contract',
    '- Reply with exactly one fenced HTML document and no other prose.',
    '- Include <!DOCTYPE html>, head, Tailwind CDN, body, .inferenesia-deck, and exactly one <section class="slide"> for every outline item in the same order.',
    '- Every section is exactly 1280×720 and has data-slide-id, data-layout, and data-title.',
    lang,
    '',
    '## User choices — preserve them',
    'Theme:',
    ...themeLines,
    'Density:',
    density.prompt,
    'Decoration:',
    decor.prompt,
    'Images:',
    images.prompt,
    '',
    '## Source integrity',
    '- Topic, materials, and outline are the factual source of truth.',
    '- Never invent numbers, market claims, customer names, dates, quotations, testimonials, research, or product capabilities.',
    '- If an outline note is vague, use careful qualitative language; do not fabricate a metric or quote to make the slide look complete.',
    '',
    slidesDesignSystemPrompt(),
    '',
    '## Narrative and composition',
    '- First assign each outline item the form that best communicates its meaning. Do not default to card grids or render the outline notes as forced bullets.',
    '- Keep a coherent palette and typography across the deck while varying composition and density deliberately. Do not repeat an identical layout on adjacent slides.',
    '- Titles should read as concise editorial headlines. Use the selected density as the content budget; never exceed the canvas to satisfy it.',
    '- Do not show deck-position numbers (“Slide 2”, “2/5”). Semantic step numbers are allowed only for genuine sequences.',
    '',
    '## Topic',
    plan.topic.trim() || '(from outline)',
    '',
    '## Materials',
    (plan.materials || '').trim() || '(none)',
    '',
    '## Approved outline',
    outlineBlock,
    '',
    'Return only ```html followed by the complete document and the closing fence.',
  ].join('\n')
}

export function isBlankDeckContent(html: string): boolean {
  const slides = (html || '').match(/class="[^"]*\bslide\b/gi) || []
  if (slides.length === 0) return true
  if (slides.length === 1 && /slide-blank|data-layout="blank"/i.test(html)) {
    return true
  }
  return false
}

export function deckNeedsWizard(html: string): boolean {
  if (!html || !html.trim()) return true
  if (/data-wizard-done="1"/i.test(html)) return false
  return isBlankDeckContent(html)
}

export function markWizardDone(html: string): string {
  if (/data-wizard-done=/i.test(html)) {
    return html.replace(/data-wizard-done="[^"]*"/i, 'data-wizard-done="1"')
  }
  if (/class="inferenesia-deck"/i.test(html)) {
    return html.replace(
      /class="inferenesia-deck"/i,
      'class="inferenesia-deck" data-wizard-done="1"',
    )
  }
  if (/class='inferenesia-deck'/i.test(html)) {
    return html.replace(
      /class='inferenesia-deck'/i,
      "class='inferenesia-deck' data-wizard-done='1'",
    )
  }
  return html
}
