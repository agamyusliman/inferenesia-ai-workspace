import {
  FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from 'react'
import { createPortal } from 'react-dom'
import { Image as ImageIcon, X } from 'lucide-react'
import { streamChat, type ChatEvent } from '../../../lib/api'
import {
  PaperclipButton,
  PreSendAttachmentChips,
} from '../../chat/AttachmentChips'
import {
  buildAttachmentPayload,
  canSendWithAttachments,
  composePromptWithAttachments,
  nextAttachmentId,
  processAttachmentFile,
  removeAttachmentById,
  type ChatAttachment,
} from '../../chat/chatAttachments'
import {
  imageGenDegradeContent,
  isImageGenUnsupported,
} from '../../chat/chatImageGen'
import {
  modelPickerDisabledWhileStreaming,
  stickyRequestFields,
  type StickySession,
} from '../../chat/chatReliability'
import { GeneratingLabel } from '../../chat/GeneratingLabel'
import { ModelPicker, type ModelPickerLocalValue } from '../../chat/ModelPicker'
import {
  clearCanvasAgentJob,
  getCanvasAgentJob,
  setCanvasAgentJob,
  subscribeCanvasAgentJobs,
} from '../../web-preview/canvasAgentJobs'
import { useLocale } from '../../i18n/LocaleProvider'
import { looksLikePatchResponse } from '../../web-preview/canvasPatch'
import {
  clearSlideDispatcher,
  registerSlideDispatcher,
  type SlideEditInstruction,
} from './slidesAgentApi'
import {
  extractImageFromMarkdown,
  GALLERY_GLOBAL_SCOPE,
  loadImages,
  type ImageEntry,
} from '../galleryStore'
import {
  DECK_STYLES,
  type DeckStyleId,
  ensureAiImagePlaceholderOnSlide,
  extractHtmlDocument,
  forcePlaceImageInSlide,
  getSlideOuterHtml,
  IMAGE_SLOTS,
  type ImageSlotId,
  isBlankSlideHtml,
  LAYOUT_BIASES,
  type LayoutBiasId,
  listSlidesFromHtml,
  looksLikeImageGenRequest,
  resolveSlidesAgentReply,
  slidesDesignSystemPrompt,
  stylePrompt,
} from './htmlDeck'
import { runSlideImageJobs } from './slideImageJobs'
import { resolvePlaygroundScope } from '../playgroundScope'

type Props = {
  contextId?: string
  canvasKey?: string
  canvasEntrySessionId?: string
  html: string
  activeSlideId?: string | null
  refSlideIds?: string[]
  onApplyHtml: (html: string) => void
  className?: string
}

export type SlideIntent =
  | 'edit-active'
  | 'fill-blank'
  | 'new-after'
  | 'rebuild-deck'

type ToolbarDialog = 'style' | 'layout' | 'intent' | 'image' | null

const liveStreams = new Map<string, {
  abort: AbortController
  apply?: (html: string) => void
}>()

export function abortSlidesAgent(canvasKey: string) {
  const stream = liveStreams.get(canvasKey)
  if (stream) {
    stream.abort.abort()
    liveStreams.delete(canvasKey)
  }
  setCanvasAgentJob(canvasKey, { busy: false, status: null, error: null })
  clearCanvasAgentJob(canvasKey)
}

export function isSlidesAgentBusy(canvasKey: string): boolean {
  return liveStreams.has(canvasKey)
}

function useCanvasAgentJob(canvasKey: string) {
  return useSyncExternalStore(
    subscribeCanvasAgentJobs,
    () => getCanvasAgentJob(canvasKey),
    () => getCanvasAgentJob(canvasKey),
  )
}

function intentLabel(intent: SlideIntent): string {
  switch (intent) {
    case 'edit-active':
      return 'Edit active'
    case 'fill-blank':
      return 'Fill blank'
    case 'new-after':
      return 'New after'
    case 'rebuild-deck':
      return 'Rebuild deck'
  }
}

function listStudioImages(contextId?: string): ImageEntry[] {
  const scopes = new Set<string>([GALLERY_GLOBAL_SCOPE])
  if (contextId) scopes.add(contextId)
  const byUrl = new Map<string, ImageEntry>()
  for (const sid of scopes) {
    for (const img of loadImages(sid)) {
      if (!img.imageUrl) continue
      const key = img.imageUrl
      const prev = byUrl.get(key)
      if (!prev || img.updatedAt > prev.updatedAt) byUrl.set(key, img)
      for (const h of img.history || []) {
        if (!h.imageUrl) continue
        const hk = h.imageUrl
        if (!byUrl.has(hk)) {
          byUrl.set(hk, {
            ...img,
            id: h.id || `${img.id}_h`,
            imageUrl: h.imageUrl,
            title: img.title,
            updatedAt: h.createdAt || img.updatedAt,
          })
        }
      }
    }
  }
  return [...byUrl.values()]
    .sort((a, b) => b.updatedAt - a.updatedAt)
    .slice(0, 48)
}

/** Infer rebuild vs edit from natural language when toolbar intent is still default. */
export function inferSlideIntentFromText(
  text: string,
  current: SlideIntent,
  activeIsBlank: boolean,
): SlideIntent {
  const t = (text || '').toLowerCase()
  if (!t.trim()) return current

  const wantsRebuild =
    /\b(rebuild|regenerate|ulang\s*dari\s*awal|generate\s*ulang|buat\s*ulang|mulai\s*dari\s*nol|from\s*scratch|entire\s*deck|seluruh\s*deck|semua\s*slide|whole\s*deck|full\s*deck)\b/i.test(
      t,
    ) ||
    /\b(rewrite|tulis\s*ulang)\b[\s\S]{0,40}\b(deck|all\s*slides|semua)\b/i.test(
      t,
    )

  const wantsNewAfter =
    /\b(slide\s*baru|new\s*slide|tambah\s*slide|add\s*(a\s*)?slide|insert\s*(a\s*)?slide)\b/i.test(
      t,
    )

  const wantsEditOnly =
    /\b(edit|ubah|ganti|perbaiki|update|revise|fix|replace|ganti\s*teks|ubah\s*judul)\b/i.test(
      t,
    ) &&
    !wantsRebuild &&
    !wantsNewAfter

  if (wantsRebuild) return 'rebuild-deck'
  if (wantsNewAfter) return 'new-after'
  if (wantsEditOnly) return activeIsBlank ? 'fill-blank' : 'edit-active'
  if (activeIsBlank && current === 'edit-active') return 'fill-blank'
  return current
}

function looksLikeTextColorInstruction(text: string): boolean {
  const t = text.toLowerCase()
  return (
    /\b(warna|color|font|text|teks)\b/.test(t) &&
    /\b(gelap|dark|hitam|black|putih|white|terang|light|merah|red|biru|blue|hijau|green|kuning|yellow|abu|gray|grey|kontras|contrast)\b/.test(
      t,
    )
  )
}

function wantsDarkText(text: string): boolean {
  const t = text.toLowerCase()
  return (
    /\b(warna|color|font|text|teks)\b/.test(t) &&
    /\b(gelap|dark|hitam|black)\b/.test(t) &&
    !/\b(bg|background|latar)\b.*\b(gelap|dark)\b/.test(t)
  )
}

function wantsLightText(text: string): boolean {
  const t = text.toLowerCase()
  return (
    /\b(warna|color|font|text|teks)\b/.test(t) &&
    /\b(putih|white|terang|light)\b/.test(t) &&
    !/\b(bg|background|latar)\b.*\b(putih|white|terang|light)\b/.test(t)
  )
}

function slidesSurgicalEditRules(instruction: string): string {
  const lines = [
    '## Surgical edit rules (compact)',
    '- Keep layout, structure, content text, and data-slide-id unchanged unless the user asks otherwise.',
    '- Prefer SEARCH/REPLACE patches over rewriting full <section> blocks.',
    '- COLOR SYNC (critical): HTML preview and PPT export must match. For every text color change you MUST update ALL of: (1) visible CSS — inline style color and/or Tailwind text-* class, (2) data-ex-color="#RRGGBB". Updating only data-ex-color leaves HTML white while PPT is dark — that is a FAILURE.',
    '- BACKGROUND LOCK: preserve each slide\'s existing background, gradients, ornaments, and card fills. Do NOT flip the deck to dark/light theme. Do NOT rewrite section style="background:…".',
    '- USER COLOR OVERRIDE (absolute): text/font color requests change TEXT only. Never invert backgrounds to "fix" contrast by flipping the theme.',
    '- Do not invent new slides, drop slides, or rebuild the deck.',
  ]
  if (wantsDarkText(instruction)) {
    lines.push(
      '- USER ASKED FOR DARK TEXT: set titles/body/labels to dark readable colors (#0f172a / #064e3b / #1e293b / #334155). Apply on style="color:…" OR text-slate-900 / text-emerald-950 classes AND matching data-ex-color. KEEP existing light/mint/cream backgrounds. FORBIDDEN: dark BG + white text.',
    )
  } else if (wantsLightText(instruction)) {
    lines.push(
      '- USER ASKED FOR LIGHT TEXT: set titles/body/labels to light readable colors with matching style/class + data-ex-color. Keep existing dark backgrounds if present.',
    )
  } else if (looksLikeTextColorInstruction(instruction)) {
    lines.push(
      '- USER ASKED FOR A TEXT COLOR CHANGE: apply requested text colors via CSS + data-ex-color; leave backgrounds unchanged.',
    )
  }
  return lines.join('\n')
}

function buildSlidesAgentPrompt(opts: {
  instruction: string
  html: string
  activeSlideId?: string | null
  refSlideIds?: string[]
  styleId: DeckStyleId
  layoutBias: LayoutBiasId
  intent: SlideIntent
  placeImage?: { assetId: string; slot: ImageSlotId; name: string } | null
  visionOnly: boolean
}): string {
  const slides = listSlidesFromHtml(opts.html)
  const active =
    slides.find((s) => s.id === opts.activeSlideId) || slides[0] || null
  const refs = (opts.refSlideIds || [])
    .map((id) => slides.find((s) => s.id === id))
    .filter(Boolean)
  const activeIsBlank = active ? isBlankSlideHtml(active.outerHtml) : false

  const multiEditIds =
    opts.intent === 'edit-active' || opts.intent === 'fill-blank'
      ? (opts.refSlideIds || []).filter(Boolean)
      : []
  const multiEdit = multiEditIds.length >= 2
  // Surgical = edit existing slides (not fill/new/rebuild). Slim payload.
  const surgical =
    opts.intent === 'edit-active' ||
    (opts.intent === 'fill-blank' && multiEdit)
  // Full design system only when generating structure from scratch.
  const needsFullDesign =
    opts.intent === 'rebuild-deck' ||
    opts.intent === 'new-after' ||
    (opts.intent === 'fill-blank' && !multiEdit)

  // Prefer active + nearby slides when deck is huge; always keep active intact.
  // For surgical multi-edit we NEVER send full deck — targets only (below).
  const maxDeckChars = 100_000
  let clipped = opts.html
  if (opts.html.length > maxDeckChars && active) {
    const others = slides
      .filter((s) => s.id !== active.id)
      .slice(0, 6)
      .map((s) => s.outerHtml)
      .join('\n')
    clipped = [
      '<!-- deck truncated for size; ACTIVE slide is authoritative -->',
      active.outerHtml,
      others,
      `<!-- … ${Math.max(0, slides.length - 7)} more slides omitted; preserve all ids unless rebuild-deck -->`,
    ].join('\n')
  } else if (opts.html.length > maxDeckChars) {
    clipped = opts.html.slice(0, maxDeckChars) + '\n<!-- truncated -->'
  }

  // Multi-edit: only target slides once. Skip active if it is already a target.
  const targetIds = new Set(multiEditIds)
  const activeInTargets = active ? targetIds.has(active.id) : false
  const refBlock =
    refs.length > 0
      ? refs
          .map(
            (s) =>
              `### Target slide ${s!.id} (${s!.title})\n\`\`\`html\n${s!.outerHtml}\n\`\`\``,
          )
          .join('\n\n')
      : '(none)'
  // Inspiration refs for single-edit: omit slides already shown as active.
  const inspirationRefs = refs.filter(
    (s) => !active || s!.id !== active.id,
  )
  const inspirationBlock =
    inspirationRefs.length > 0
      ? inspirationRefs
          .map(
            (s) =>
              `### Ref slide ${s!.id} (${s!.title})\n\`\`\`html\n${s!.outerHtml}\n\`\`\``,
          )
          .join('\n\n')
      : '(none)'

  const placeBlock = opts.placeImage
    ? [
        '## Image placement (REQUIRED for this turn only)',
        `Embed image asset id="${opts.placeImage.assetId}" name="${opts.placeImage.name}" into slot="${opts.placeImage.slot}" on the target slide.`,
        'Use real <img data-asset-id="…" src="…" class="w-full h-full object-cover rounded-xl" alt="…" /> inside layout.',
        'Do not invent extra image cards for every bullet — only place this asset where it belongs.',
      ].join('\n')
    : opts.visionOnly
      ? '## Images: vision / style reference only — do NOT embed.'
      : surgical
        ? '## Images: keep existing images; do not add new ones unless the user asked.'
        : '## Images: only embed if user attached a place-image or explicitly asked for photos. Default = no images.'

  const layoutLine =
    opts.layoutBias === 'auto'
      ? 'Layout bias: keep existing layout unless the user asks to change it.'
      : `Layout bias for this turn: data-layout="${opts.layoutBias}".`

  let intentBlock = ''
  switch (opts.intent) {
    case 'edit-active':
      if (multiEdit) {
        intentBlock = [
          `## INTENT: edit-selected (${multiEditIds.length} slides — apply SAME instruction to EACH selected slide)`,
          `Target slide ids (ALL of them): ${multiEditIds.join(', ')}`,
          'CRITICAL: You MUST update EVERY listed id.',
          'PREFERRED: SEARCH/REPLACE patches that touch each selected slide (smallest possible snippets: color attrs, style attrs, class tokens).',
          'ALTERNATE: one full <section class="slide" data-slide-id="…"> per target id (same count as targets).',
          'Do NOT rebuild the full deck. Do NOT emit unselected slides.',
          'Never ask the user to re-paste deck HTML or INTENT.',
        ].join('\n')
      } else {
        intentBlock = [
          '## INTENT: edit-active (surgical change on ONE slide — do NOT rebuild deck)',
          active
            ? `Modify ONLY data-slide-id="${active.id}". Keep that id.`
            : 'Modify the active slide only.',
          'PREFERRED: SEARCH/REPLACE on the active section only (smallest snippets).',
          'ALTERNATE: exactly ONE <section class="slide" data-slide-id="…"> for the active id.',
          'Do NOT delete, recreate, or rewrite other slides. Never ask for re-paste.',
        ].join('\n')
      }
      break
    case 'fill-blank':
      if (multiEdit) {
        intentBlock = [
          `## INTENT: fill-selected blanks (${multiEditIds.length} slides)`,
          `Fill EACH blank target id: ${multiEditIds.join(', ')}`,
          'Return one <section class="slide"> per id with matching data-slide-id. Remove slide-blank on each.',
          'Do NOT rebuild the full deck. Never ask for re-paste.',
        ].join('\n')
      } else {
        intentBlock = [
          '## INTENT: fill-blank (fill ONE blank slide — do NOT rebuild deck)',
          active
            ? `Replace blank data-slide-id="${active.id}" only. Keep id. Remove slide-blank.`
            : 'Fill the blank active slide.',
          'Do NOT touch other slides. Do NOT return a multi-slide deck.',
          'Return SEARCH/REPLACE or exactly ONE <section class="slide">.',
          'Never ask the user to re-paste deck HTML or INTENT.',
        ].join('\n')
      }
      break
    case 'new-after':
      intentBlock = [
        '## INTENT: new-after (ONE new slide after active — do NOT rebuild deck)',
        active
          ? `Insert ONE new slide after data-slide-id="${active.id}". Do not overwrite or delete existing slides.`
          : 'Append ONE new slide.',
        'Return exactly ONE new <section class="slide" data-slide-id="NEW_UNIQUE_ID">.',
        'Never return a full rebuilt deck.',
        'Never ask the user to re-paste deck HTML or INTENT.',
      ].join('\n')
      break
    case 'rebuild-deck':
      intentBlock = [
        '## INTENT: rebuild-deck (full multi-slide rewrite — explicit)',
        'User wants the entire deck regenerated. Return complete multi-slide HTML in one ```html fence.',
        'Every slide must be a proper <section class="slide" data-slide-id data-layout data-title>…</section>.',
        'Never ask the user to re-paste deck HTML or INTENT.',
      ].join('\n')
      break
  }

  const patchFormat = [
    '<<<<<<< SEARCH',
    '[exact snippet from target HTML]',
    '=======',
    '[replacement]',
    '>>>>>>> REPLACE',
  ].join('\n')

  // CRITICAL ORDER: user request + intent + target HTML FIRST.
  const parts: string[] = [
    '## User request (PRIMARY — follow this)',
    opts.instruction.trim() || '(see attachments / place image)',
    '',
    intentBlock,
    '',
  ]

  // Active slide: only when NOT already listed as multi-edit target.
  if (!multiEdit || !activeInTargets) {
    parts.push(
      `## Active slide: ${
        active
          ? `id=${active.id} blank=${activeIsBlank ? 'yes' : 'no'} layout=${active.layout} title=${active.title}`
          : '(none)'
      }`,
    )
    if (active) {
      parts.push(`\`\`\`html\n${active.outerHtml}\n\`\`\``)
    }
    parts.push('')
  } else if (active) {
    parts.push(
      `## Active slide (also a target): id=${active.id} title=${active.title}`,
      '',
    )
  }

  parts.push('## Output formats (pick ONE matching INTENT)')
  if (multiEdit) {
    parts.push(
      `1) SEARCH/REPLACE patches (PREFERRED) — cover EVERY target id (${multiEditIds.join(', ')})`,
      patchFormat,
      `2) ${multiEditIds.length} separate <section class="slide" data-slide-id="…"> — one per selected id`,
    )
  } else if (opts.intent === 'rebuild-deck') {
    parts.push('1) Full multi-slide HTML in one ```html fence')
  } else {
    parts.push(
      '1) SEARCH/REPLACE (PREFERRED for edit-active)',
      patchFormat,
      '2) Exactly ONE <section class="slide"> when INTENT is edit-active / fill-blank / new-after',
      '3) Full multi-slide HTML ONLY when INTENT is rebuild-deck',
    )
  }
  parts.push(
    'NEVER reply with questions asking for deck HTML, INTENT, or instructions — they are already provided.',
    'NEVER wrap meta-instructions inside a <section>. Output only slide HTML or patches.',
    '',
    placeBlock,
    '',
  )

  if (multiEdit) {
    parts.push(
      '## Selected slides (TARGETS — apply instruction to each; each appears ONCE)',
      refBlock,
      '',
    )
  } else if (inspirationRefs.length > 0) {
    parts.push(
      '## Use-as-ref slides (inspiration only)',
      inspirationBlock,
      '',
    )
  }

  const colorInstr = looksLikeTextColorInstruction(opts.instruction)
  if (needsFullDesign) {
    parts.push('## Style', stylePrompt(opts.styleId), layoutLine, '')
  } else if (colorInstr) {
    parts.push(
      '## Style context (surgical color edit)',
      'IGNORE the toolbar Style (Dark/Light/etc.) for this turn. Do NOT re-theme the deck.',
      'Preserve each slide\'s existing background, card fills, borders, and layout exactly.',
      'Only change text/font colors as the user requested.',
      'HTML visible color (style/class) and data-ex-color MUST stay in sync.',
      layoutLine,
      '',
    )
  } else {
    parts.push(
      '## Style context',
      'Toolbar style is a weak hint only. Prefer preserving the existing slide palette.',
      stylePrompt(opts.styleId),
      layoutLine,
      'User color/style instructions override theme defaults. Do not flip light↔dark unless the user asked.',
      '',
    )
  }

  parts.push(
    '## Rules',
    'You are the Inferenesia slides HTML editor. Each slide is <section class="slide" data-slide-id data-layout data-title> 1280×720, Tailwind only.',
    `- Deck has ${slides.length} slide(s). Unless INTENT is rebuild-deck, preserve every existing slide id and count.`,
    '- edit/fill/new-after = surgical change only. rebuild-deck = full rewrite only when INTENT says so.',
  )
  if (needsFullDesign) {
    parts.push(slidesDesignSystemPrompt())
  } else {
    parts.push(slidesSurgicalEditRules(opts.instruction))
  }

  // Full deck HTML: only when rebuild needs context, or single fill/new needs neighbors.
  // Surgical multi-edit: SKIP (targets already provided once).
  // Surgical single edit: SKIP full deck (active section is enough).
  if (opts.intent === 'rebuild-deck') {
    parts.push(
      '',
      '## Current deck HTML (reference for rebuild)',
      '```html',
      clipped,
      '```',
    )
  } else if (opts.intent === 'new-after' || (opts.intent === 'fill-blank' && !multiEdit)) {
    parts.push(
      '',
      '## Deck context (preserve all existing slides)',
      '```html',
      clipped,
      '```',
    )
  } else if (surgical && !multiEdit) {
    // Single edit: list other slide ids only (no HTML) so model knows what not to touch.
    const otherIds = slides
      .filter((s) => !active || s.id !== active.id)
      .map((s) => s.id)
    if (otherIds.length) {
      parts.push(
        '',
        `## Other slide ids (do not modify): ${otherIds.join(', ')}`,
      )
    }
  } else if (multiEdit) {
    const nonTarget = slides
      .filter((s) => !targetIds.has(s.id))
      .map((s) => s.id)
    if (nonTarget.length) {
      parts.push(
        '',
        `## Unselected slide ids (do not modify): ${nonTarget.join(', ')}`,
      )
    }
  }

  return parts.join('\n')
}

function extractStreamingHtml(text: string): string | null {
  const full = extractHtmlDocument(text)
  if (full) return full
  const openFence = /```(?:html|htm)?\s*\n([\s\S]*)$/i.exec(text)
  if (openFence?.[1] && openFence[1].trim().length > 80) {
    return extractHtmlDocument('```html\n' + openFence[1]) || openFence[1]
  }
  const idx = text.search(/<!DOCTYPE\s+html|<html[\s>]/i)
  if (idx >= 0 && text.length - idx > 80) return text.slice(idx)
  return null
}

export function SlidesAgentPanel({
  contextId,
  canvasKey = 'slides-default',
  canvasEntrySessionId,
  html,
  activeSlideId,
  refSlideIds = [],
  onApplyHtml,
  className = '',
}: Props) {
  const { t } = useLocale()
  const playgroundScope = resolvePlaygroundScope(
    'slides',
    canvasEntrySessionId,
    contextId,
  )
  const job = useCanvasAgentJob(canvasKey)
  const busy = job.busy
  const status = job.status
  const error = job.error

  const [input, setInput] = useState('')
  const [attachments, setAttachments] = useState<ChatAttachment[]>([])
  const [styleId, setStyleId] = useState<DeckStyleId>('dark')
  const [layoutBias, setLayoutBias] = useState<LayoutBiasId>('auto')
  const [intent, setIntent] = useState<SlideIntent>('edit-active')
  const [imageSlot, setImageSlot] = useState<ImageSlotId>('right')
  const [visionOnly, setVisionOnly] = useState(false)
  const [placeOnSend, setPlaceOnSend] = useState(false)
  const [dialog, setDialog] = useState<ToolbarDialog>(null)
  const [dragOver, setDragOver] = useState(false)
  const [genPrompt, setGenPrompt] = useState('')
  const [genBusy, setGenBusy] = useState(false)
  const [studioTick, setStudioTick] = useState(0)
  const intentTouched = useRef(false)

  const [textModel, setTextModel] = useState<ModelPickerLocalValue | null>(null)
  const [visionModel, setVisionModel] = useState<ModelPickerLocalValue | null>(
    null,
  )
  const attachInputRef = useRef<HTMLInputElement | null>(null)
  const htmlRef = useRef(html)
  htmlRef.current = html
  const applyRef = useRef(onApplyHtml)
  applyRef.current = onApplyHtml
  const lastLiveRef = useRef('')

  useEffect(() => {
    const stream = liveStreams.get(canvasKey)
    if (stream) {
      stream.apply = (h: string) => applyRef.current(h)
    }
  }, [canvasKey, onApplyHtml])

  const stickyFromLocal = useCallback(
    (slot: ModelPickerLocalValue | null): StickySession => {
      if (!slot?.profile && !slot?.model) return {}
      return {
        profile: slot.profile,
        model: slot.model,
        label: slot.label,
      }
    },
    [],
  )

  const onTextModelChange = useCallback((next: ModelPickerLocalValue) => {
    setTextModel(next)
  }, [])

  const onVisionModelChange = useCallback((next: ModelPickerLocalValue) => {
    setVisionModel(next)
  }, [])

  const activeOuter = activeSlideId
    ? getSlideOuterHtml(html, activeSlideId)
    : null
  const activeIsBlank = activeOuter ? isBlankSlideHtml(activeOuter) : false
  const scopedSlideIds =
    refSlideIds.length > 0 ? refSlideIds : activeSlideId ? [activeSlideId] : []

  useEffect(() => {
    if (intentTouched.current) return
    if (activeIsBlank) setIntent('fill-blank')
    else setIntent('edit-active')
  }, [activeIsBlank, activeSlideId])

  useEffect(() => {
    if (dialog !== 'image') return
    setStudioTick((n) => n + 1)
  }, [dialog])

  const studioImages = useMemo(
    () => listStudioImages(contextId),
    [contextId, studioTick],
  )

  const removeAttachment = (id: string) => {
    setAttachments((prev) => removeAttachmentById(prev, id))
  }

  const ingestFiles = useCallback(async (files: FileList | File[] | null) => {
    if (!files) return
    const list = Array.from(files).filter((f) => f.type.startsWith('image/'))
    if (!list.length) return
    const placeholders: ChatAttachment[] = list.map((f, i) => ({
      id: `pending-${Date.now()}-${i}`,
      name: f.name || 'image',
      size: f.size || 0,
      mime: f.type || 'image/*',
      kind: 'image',
      loading: true,
    }))
    setAttachments((prev) => [...prev, ...placeholders])
    const results = await Promise.all(list.map((f) => processAttachmentFile(f)))
    setAttachments((prev) => {
      const withoutPending = prev.filter(
        (a) => !a.loading || !a.id.startsWith('pending-'),
      )
      const cleaned = withoutPending.filter(
        (a) => !placeholders.some((p) => p.id === a.id),
      )
      return [...cleaned, ...results]
    })
  }, [])

  const onPickAttachments = useCallback(
    async (files: FileList | null) => {
      await ingestFiles(files)
    },
    [ingestFiles],
  )

  const addStudioImage = useCallback((img: ImageEntry) => {
    const url = img.imageUrl
    if (!url) return
    const att: ChatAttachment = {
      id: nextAttachmentId(),
      name: img.title || 'studio-image',
      size: 0,
      mime: url.startsWith('data:image/')
        ? url.slice(5, url.indexOf(';')) || 'image/png'
        : 'image/png',
      kind: 'image',
      dataUrl: url.startsWith('data:') ? url : undefined,
      previewUrl: url,
    }
    setAttachments((prev) => [...prev, att])
    setPlaceOnSend(true)
    setVisionOnly(false)
  }, [])

  const stop = useCallback(() => {
    const stream = liveStreams.get(canvasKey)
    stream?.abort.abort()
    liveStreams.delete(canvasKey)
    setCanvasAgentJob(canvasKey, { busy: false, status: null, error: null })
    clearCanvasAgentJob(canvasKey)
    setGenBusy(false)
  }, [canvasKey])

  const firstImageAttach = attachments.find(
    (a) => a.kind === 'image' && (a.dataUrl || a.previewUrl),
  )

  const placeAssetOnSlide = useCallback(
    (asset: { id: string; src: string; alt?: string }, slot: ImageSlotId) => {
      if (!activeSlideId) return false
      const next = forcePlaceImageInSlide(
        htmlRef.current,
        activeSlideId,
        asset,
        slot,
      )
      if (!next) return false
      applyRef.current(next)
      setCanvasAgentJob(canvasKey, {
        busy: false,
        status: 'Image placed on slide',
        error: null,
      })
      window.setTimeout(() => {
        const cur = getCanvasAgentJob(canvasKey)
        if (cur.status === 'Image placed on slide') {
          setCanvasAgentJob(canvasKey, { status: null })
          if (!cur.busy && !cur.error) clearCanvasAgentJob(canvasKey)
        }
      }, 1200)
      return true
    },
    [activeSlideId, canvasKey],
  )

  const forcePlace = useCallback(() => {
    if (!firstImageAttach) {
      setCanvasAgentJob(canvasKey, {
        busy: false,
        status: null,
        error: 'Attach or drop an image first',
      })
      return
    }
    const src = firstImageAttach.dataUrl || firstImageAttach.previewUrl || ''
    if (!src) return
    const ok = placeAssetOnSlide(
      {
        id: firstImageAttach.id,
        src,
        alt: firstImageAttach.name,
      },
      imageSlot,
    )
    if (!ok) {
      setCanvasAgentJob(canvasKey, {
        busy: false,
        status: null,
        error: 'Could not place image on active slide',
      })
    }
  }, [canvasKey, firstImageAttach, imageSlot, placeAssetOnSlide])

  const generateAndPlace = useCallback(async () => {
    const prompt = genPrompt.trim()
    if (!prompt || genBusy || busy) return
    setGenBusy(true)
    setCanvasAgentJob(canvasKey, {
      busy: true,
      status: 'Generating image…',
      error: null,
    })
    const ac = new AbortController()
    liveStreams.set(canvasKey, { abort: ac, apply: (h: string) => applyRef.current(h) })
    let assembled = ''
    const stickyFields = stickyRequestFields(stickyFromLocal(visionModel))
    try {
      await streamChat(
        {
          prompt: [
            'Generate one image for a presentation slide.',
            'Subject and style:',
            prompt,
            'Use the image_generation tool. Return the image in the response.',
          ].join('\n'),
          generate_image: true,
          agent_kind: 'slides',
          playground_id: playgroundScope.playgroundId,
          no_tools: true,
          workspace_id: contextId,
          image_size: '1536x1024',
          image_only: true,
          ...stickyFields,
        },
        (ev: ChatEvent) => {
          if (liveStreams.get(canvasKey)?.abort !== ac) return
          if (ev.type === 'TokenDelta' && ev.delta) assembled += ev.delta
          if (ev.type === 'Done' && ev.final) assembled = ev.final
          if (ev.type === 'Error' && ev.error) {
            setCanvasAgentJob(canvasKey, {
              error: isImageGenUnsupported(ev.error)
                ? imageGenDegradeContent(ev.error)
                : ev.error,
            })
          }
        },
        ac.signal,
      )
      if (liveStreams.get(canvasKey)?.abort !== ac) return
      const parsed = extractImageFromMarkdown(assembled)
      if (parsed.imageUrl) {
        const id = nextAttachmentId()
        const att: ChatAttachment = {
          id,
          name: prompt.slice(0, 40) || 'generated',
          size: 0,
          mime: 'image/png',
          kind: 'image',
          dataUrl: parsed.imageUrl.startsWith('data:')
            ? parsed.imageUrl
            : undefined,
          previewUrl: parsed.imageUrl,
        }
        setAttachments((prev) => [...prev, att])
        setPlaceOnSend(true)
        setVisionOnly(false)
        placeAssetOnSlide(
          { id, src: parsed.imageUrl, alt: prompt.slice(0, 80) },
          imageSlot,
        )
        setGenPrompt('')
        setCanvasAgentJob(canvasKey, {
          busy: false,
          status: 'Generated + placed',
          error: null,
        })
      } else {
        setCanvasAgentJob(canvasKey, {
          busy: false,
          status: null,
          error: 'Image generation returned no image URL',
        })
      }
    } catch (e) {
      if (liveStreams.get(canvasKey)?.abort !== ac) return
      if ((e as Error)?.name === 'AbortError') {
        setCanvasAgentJob(canvasKey, { busy: false, status: null, error: null })
      } else {
        setCanvasAgentJob(canvasKey, {
          busy: false,
          status: null,
          error: e instanceof Error ? e.message : String(e),
        })
      }
    } finally {
      if (liveStreams.get(canvasKey)?.abort === ac) liveStreams.delete(canvasKey)
      setGenBusy(false)
    }
  }, [
    busy,
    canvasKey,
    genBusy,
    genPrompt,
    imageSlot,
    placeAssetOnSlide,
    playgroundScope,
    visionModel,
    contextId,
  ])

  const send = useCallback(async (overrideText?: string, forceActive = false) => {
    if (busy || genBusy) return
    if (!forceActive && !scopedSlideIds.length) return
    const effectiveInput = (overrideText ?? input).trim()
    if (!canSendWithAttachments(effectiveInput, attachments, []) && !placeOnSend) return
    const instruction = effectiveInput
    const attachPayload = buildAttachmentPayload(attachments)
    const userFacing = composePromptWithAttachments(instruction, attachPayload)
    // Smart intent: natural language can override toolbar when user asks rebuild vs edit.
    const effectiveIntent = inferSlideIntentFromText(
      userFacing,
      intent,
      activeIsBlank,
    )
    if (effectiveIntent !== intent) {
      setIntent(effectiveIntent)
      intentTouched.current = true
    }
    const jobKey = canvasKey
    const applyAtStart = applyRef.current
    setInput('')
    lastLiveRef.current = ''
    const ac = new AbortController()
    liveStreams.set(jobKey, { abort: ac, apply: (h: string) => applyRef.current(h) })
    setCanvasAgentJob(jobKey, { busy: true, status: null, error: null })
    let assembled = ''
    const baseHtml = htmlRef.current
    const textSticky = stickyRequestFields(stickyFromLocal(textModel))
    const visionSticky = stickyRequestFields(stickyFromLocal(visionModel))
    const wantImageGen =
      looksLikeImageGenRequest(userFacing) &&
      effectiveIntent !== 'rebuild-deck' &&
      Boolean(activeSlideId)

    const placeImage =
      placeOnSend && firstImageAttach && !visionOnly
        ? {
            assetId: firstImageAttach.id,
            slot: imageSlot,
            name: firstImageAttach.name,
          }
        : null

    if (wantImageGen && activeSlideId) {
      try {
        const withSlot = ensureAiImagePlaceholderOnSlide(
          baseHtml,
          activeSlideId,
          userFacing || 'Presentation diagram illustration, no text',
          imageSlot,
        )
        applyAtStart(withSlot)
        setCanvasAgentJob(jobKey, {
          busy: true,
          status: 'Generating image into slide…',
          error: null,
        })
        await runSlideImageJobs({
          canvasKey: `${jobKey}::img`,
          html: withSlot,
          contextId,
          playgroundId: playgroundScope.playgroundId,
          sticky: visionSticky,
          onHtml: (h) => applyAtStart(h),
        })
        setAttachments([])
        setPlaceOnSend(false)
        setCanvasAgentJob(jobKey, {
          busy: false,
          status: 'Image generated',
          error: null,
        })
        window.setTimeout(() => {
          const cur = getCanvasAgentJob(jobKey)
          if (cur.status === 'Image generated') {
            setCanvasAgentJob(jobKey, { status: null })
            if (!cur.busy && !cur.error) clearCanvasAgentJob(jobKey)
          }
        }, 1400)
      } catch (e) {
        if ((e as Error)?.name === 'AbortError') {
          setCanvasAgentJob(jobKey, { busy: false, status: null, error: null })
        } else {
          setCanvasAgentJob(jobKey, {
            busy: false,
            status: null,
            error: e instanceof Error ? e.message : String(e),
          })
        }
      } finally {
        liveStreams.delete(jobKey)
      }
      return
    }

    try {
      await streamChat(
        {
          prompt: buildSlidesAgentPrompt({
            instruction: userFacing,
            html: baseHtml,
            activeSlideId,
            refSlideIds: scopedSlideIds,
            styleId,
            layoutBias,
            intent: effectiveIntent,
            placeImage,
            visionOnly,
          }),
          no_tools: true,
          agent_kind: 'slides',
          playground_id: playgroundScope.playgroundId,
          session_id:
            playgroundScope.mode === 'session'
              ? playgroundScope.sessionId
              : undefined,
          workspace_id: contextId,
          images: attachPayload.images.length
            ? attachPayload.images.map((img) => ({
                name: img.name,
                media_type: img.media_type,
                data_url: img.data_url,
              }))
            : undefined,
          ...textSticky,
        },
        (ev: ChatEvent) => {
          const stream = liveStreams.get(jobKey)
          if (!stream || stream.abort !== ac) return
          if (ev.type === 'TokenDelta' && ev.delta) {
            assembled += ev.delta
            if (
              effectiveIntent === 'rebuild-deck' &&
              !looksLikePatchResponse(assembled)
            ) {
              const live = extractStreamingHtml(assembled)
              if (live && live !== lastLiveRef.current) {
                lastLiveRef.current = live
                stream.apply?.(live)
              }
            }
          }
          if (ev.type === 'Done' && ev.final) assembled = ev.final
          if (ev.type === 'Error' && ev.error) {
            setCanvasAgentJob(jobKey, { error: ev.error })
          }
        },
        ac.signal,
      )

      if (liveStreams.get(jobKey)?.abort !== ac) return
      const multiTargets =
        (effectiveIntent === 'edit-active' || effectiveIntent === 'fill-blank') &&
        scopedSlideIds.length >= 2
          ? scopedSlideIds
          : undefined
      const resolved = resolveSlidesAgentReply(baseHtml, assembled, {
        intent: effectiveIntent,
        activeSlideId,
        targetSlideIds: multiTargets,
      })
      if (resolved.ok) {
        if (liveStreams.get(jobKey)?.abort !== ac) return
        liveStreams.get(jobKey)?.apply?.(resolved.next)
        setAttachments([])
        setPlaceOnSend(false)
        const statusMsg =
          resolved.mode === 'patches'
            ? `Updated (${resolved.applied} patch${resolved.applied === 1 ? '' : 'es'})`
            : resolved.mode === 'insert'
              ? 'Slide added'
              : resolved.mode === 'slide'
                ? resolved.applied > 1
                  ? `${resolved.applied} slides updated`
                  : 'Slide updated'
                : 'Deck updated'
        setCanvasAgentJob(jobKey, {
          busy: false,
          status: statusMsg,
          error: null,
        })
        window.setTimeout(() => {
          const cur = getCanvasAgentJob(jobKey)
          if (
            cur.status?.startsWith('Updated') ||
            cur.status === 'Slide added' ||
            cur.status === 'Slide updated' ||
            cur.status?.endsWith('slides updated') ||
            cur.status === 'Deck updated'
          ) {
            setCanvasAgentJob(jobKey, { status: null })
            if (!cur.busy && !cur.error) clearCanvasAgentJob(jobKey)
          }
        }, 1400)
      } else if (assembled.trim()) {
        const preview = assembled.trim().slice(0, 160).replace(/\s+/g, ' ')
        setCanvasAgentJob(jobKey, {
          busy: false,
          status: null,
          error: `${resolved.error || 'No usable slide HTML'} · ${preview}`,
        })
      } else if (!getCanvasAgentJob(jobKey).error) {
        setCanvasAgentJob(jobKey, { busy: false, status: null })
        clearCanvasAgentJob(jobKey)
      }
    } catch (e) {
      if (liveStreams.get(jobKey)?.abort !== ac) return
      if ((e as Error)?.name === 'AbortError') {
        setCanvasAgentJob(jobKey, { busy: false, status: null, error: null })
        clearCanvasAgentJob(jobKey)
        return
      }
      setCanvasAgentJob(jobKey, {
        busy: false,
        status: null,
        error: e instanceof Error ? e.message : String(e),
      })
    } finally {
      if (liveStreams.get(jobKey)?.abort === ac) liveStreams.delete(jobKey)
    }
  }, [
    activeSlideId,
    attachments,
    busy,
    canvasKey,
    firstImageAttach,
    genBusy,
    imageSlot,
    input,
    intent,
    layoutBias,
    placeOnSend,
    playgroundScope,
    scopedSlideIds,
    styleId,
    textModel,
    visionModel,
    visionOnly,
    contextId,
  ])

  useEffect(() => {
    const dispatch = (instruction: SlideEditInstruction) => {
      if (instruction.intent) {
        setIntent(instruction.intent)
        intentTouched.current = true
      }
      void send(instruction.text, true)
    }
    registerSlideDispatcher(canvasKey, dispatch)
    return () => clearSlideDispatcher(canvasKey)
  }, [canvasKey, send, activeSlideId])

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    void send()
  }

  const hasScope = scopedSlideIds.length > 0
  const canSend =
    hasScope &&
    !busy &&
    !genBusy &&
    (canSendWithAttachments(input, attachments, []) ||
      (placeOnSend && Boolean(firstImageAttach)))

  const styleLabel =
    DECK_STYLES.find((s) => s.id === styleId)?.label || 'Style'
  const layoutLabel =
    LAYOUT_BIASES.find((l) => l.id === layoutBias)?.label || 'Layout'
  const slotLabel =
    IMAGE_SLOTS.find((s) => s.id === imageSlot)?.label || imageSlot

  const selectBtn = (active: boolean) =>
    `inline-flex items-center gap-1 rounded-md border px-2 py-1 text-[11px] font-medium transition ${
      active
        ? 'border-shell-accent bg-shell-active text-shell-text'
        : 'border-shell-border text-shell-muted hover:bg-shell-hover hover:text-shell-text'
    }`

  const optionBtn = (active: boolean) =>
    `w-full rounded-md border px-2 py-1.5 text-left text-[11px] transition ${
      active
        ? 'border-shell-accent bg-shell-active text-shell-text'
        : 'border-shell-border text-shell-text hover:bg-shell-hover'
    }`

  const openDialog = (id: ToolbarDialog) => {
    setDialog((cur) => (cur === id ? null : id))
  }

  const onDrop = (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setDragOver(false)
    void ingestFiles(e.dataTransfer.files)
    setDialog('image')
    setPlaceOnSend(true)
    setVisionOnly(false)
  }

  const dialogTitle =
    dialog === 'style'
      ? 'Style'
      : dialog === 'layout'
        ? 'Layout bias'
        : dialog === 'intent'
          ? 'Intent'
          : dialog === 'image'
            ? 'Image on slide'
            : ''

  return (
    <div
      data-testid="slides-agent"
      className={`playground-agent-dock relative ${className}`}
      onDragEnter={(e) => {
        if (e.dataTransfer.types.includes('Files')) {
          e.preventDefault()
          setDragOver(true)
        }
      }}
      onDragOver={(e) => {
        if (e.dataTransfer.types.includes('Files')) {
          e.preventDefault()
          setDragOver(true)
        }
      }}
      onDragLeave={() => setDragOver(false)}
      onDrop={onDrop}
    >
      {dragOver && (
        <div className="pointer-events-none absolute inset-0 z-20 flex items-center justify-center border-2 border-dashed border-shell-accent bg-shell-active text-xs font-medium text-shell-accent">
          Drop image onto slide agent
        </div>
      )}

      {(error || (status && !busy && !genBusy)) && (
        <div
          role={error ? 'alert' : 'status'}
          className={`mx-2 mt-1.5 rounded-md border px-2 py-1.5 text-[11px] leading-relaxed ${
            error
              ? 'border-red-500/30 bg-red-500/10 text-red-300'
              : 'border-shell-accent/30 bg-shell-active text-shell-accent'
          }`}
          data-testid={error ? 'slides-agent-error' : 'slides-agent-status'}
        >
          {error || status}
        </div>
      )}

      <div
        data-testid="slides-agent-toolbar"
        className="flex flex-wrap items-center gap-1 px-2 py-1.5"
      >
        <button
          type="button"
          data-testid="slides-toolbar-style"
          disabled={busy || genBusy}
          onClick={() => openDialog('style')}
          className={selectBtn(dialog === 'style')}
          title="Deck style"
        >
          Style · {styleLabel}
        </button>
        <button
          type="button"
          data-testid="slides-toolbar-layout"
          disabled={busy || genBusy}
          onClick={() => openDialog('layout')}
          className={selectBtn(dialog === 'layout')}
          title="Layout bias for agent"
        >
          Layout · {layoutLabel}
        </button>
        <button
          type="button"
          data-testid="slides-toolbar-intent"
          disabled={busy || genBusy}
          onClick={() => openDialog('intent')}
          className={selectBtn(dialog === 'intent')}
          title="Edit existing vs create new"
        >
          Intent · {intentLabel(intent)}
          {activeIsBlank ? ' · blank' : ''}
        </button>
        <button
          type="button"
          data-testid="slides-toolbar-image"
          disabled={busy || genBusy}
          onClick={() => openDialog('image')}
          className={selectBtn(dialog === 'image' || Boolean(firstImageAttach))}
          title="Insert image: attach, drop, Image Studio, or generate"
        >
          <ImageIcon className="h-3 w-3" />
          Image
          {firstImageAttach ? ` · ${slotLabel}` : ''}
        </button>
        <span className="rounded-full bg-shell-active px-1.5 py-0.5 text-[10px] font-medium text-shell-accent">
          {refSlideIds.length > 0
            ? t('slides.agent.selectedCount', { count: refSlideIds.length })
            : activeSlideId
              ? t('slides.agent.activeScope')
              : t('slides.agent.noScope')}
        </span>
      </div>

      <PreSendAttachmentChips
        attachments={attachments}
        onRemove={removeAttachment}
        disabled={busy || genBusy}
      />

      <form
        data-testid="slides-agent-form"
        onSubmit={onSubmit}
        className="playground-agent-form"
      >
        <textarea
          data-testid="slides-agent-input"
          value={hasScope ? input : ''}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault()
              void send()
            }
          }}
          rows={2}
          disabled={busy || genBusy || !hasScope}
          placeholder={
            !hasScope
              ? t('slides.agent.selectSlideHint')
              : refSlideIds.length > 1
                ? t('slides.agent.multiScopeHint', { count: refSlideIds.length })
                : intent === 'fill-blank' || activeIsBlank
                  ? t('slides.agent.fillBlankHint')
                  : intent === 'new-after'
                    ? t('slides.agent.newAfterHint')
                    : intent === 'rebuild-deck'
                      ? t('slides.agent.rebuildHint')
                      : t('slides.agent.editActiveHint')
          }
          className="playground-composer-input disabled:opacity-60"
        />
        <div
          data-testid="slides-agent-runbar"
          className="mt-1.5 flex flex-wrap items-center justify-between gap-2"
        >
          <div className="flex min-w-0 flex-wrap items-center gap-1.5">
            <PaperclipButton
              inputRef={attachInputRef}
              disabled={busy || genBusy}
              onPick={(files) => void onPickAttachments(files)}
            />
            <div className="flex min-w-0 flex-wrap items-center gap-1">
              <div className="min-w-0">
                <div className="mb-0.5 text-[9px] font-medium uppercase tracking-wide text-shell-muted">
                  Teks
                </div>
                <ModelPicker
                  localOnly
                  slotLabel="Teks"
                  testId="slides-agent-model-text"
                  value={textModel}
                  onLocalChange={onTextModelChange}
                  disabled={modelPickerDisabledWhileStreaming(busy || genBusy)}
                />
              </div>
              <div className="min-w-0">
                <div className="mb-0.5 text-[9px] font-medium uppercase tracking-wide text-shell-muted">
                  Vision
                </div>
                <ModelPicker
                  localOnly
                  slotLabel="Vision"
                  testId="slides-agent-model-vision"
                  value={visionModel}
                  onLocalChange={onVisionModelChange}
                  disabled={modelPickerDisabledWhileStreaming(busy || genBusy)}
                />
              </div>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            {(busy || genBusy) && (
              <>
                <GeneratingLabel
                  variant="composer"
                  className="!text-[10px]"
                />
                <button
                  type="button"
                  data-testid="slides-agent-stop"
                  onClick={stop}
                  className="rounded-lg border border-shell-border px-2 py-1 text-[11px] text-shell-muted hover:bg-shell-hover"
                  title="Stop"
                >
                  Stop
                </button>
              </>
            )}
            <button
              type="submit"
              data-testid="slides-agent-send"
              disabled={!canSend || busy || genBusy}
              title="Update slides"
              className="shell-primary-button rounded-lg px-3 py-1 text-[11px] font-semibold disabled:opacity-40"
            >
              Send
            </button>
          </div>
        </div>
      </form>

      {dialog &&
        createPortal(
          <div
            className="fixed inset-0 z-[85] flex justify-end bg-black/40"
            data-testid="slides-toolbar-dialog-backdrop"
            onClick={() => setDialog(null)}
          >
            <div
              role="dialog"
              aria-modal="true"
              data-testid={`slides-dialog-${dialog}`}
              className="flex h-full w-full max-w-sm flex-col border-l border-shell-border bg-shell-panel shadow-2xl"
              onClick={(e) => e.stopPropagation()}
            >
              <div className="flex h-10 shrink-0 items-center justify-between border-b border-shell-border px-3">
                <span className="text-[11px] font-semibold uppercase tracking-wide text-shell-muted">
                  {dialogTitle}
                </span>
                <button
                  type="button"
                  className="rounded p-1 text-shell-muted hover:bg-shell-hover"
                >
                  <X className="h-4 w-4" />
                </button>
              </div>

              <div className="shell-scroll min-h-0 flex-1 space-y-3 overflow-y-auto p-3">
                {dialog === 'style' && (
                  <div className="space-y-1.5">
                    {DECK_STYLES.map((s) => (
                      <button
                        key={s.id}
                        type="button"
                        data-testid={`slides-style-${s.id}`}
                        className={optionBtn(styleId === s.id)}
                        onClick={() => {
                          setStyleId(s.id)
                          setDialog(null)
                        }}
                      >
                        <div className="font-medium">{s.label}</div>
                        <div className="mt-0.5 text-[10px] text-shell-muted line-clamp-2">
                          {s.prompt}
                        </div>
                      </button>
                    ))}
                  </div>
                )}

                {dialog === 'layout' && (
                  <div className="space-y-1.5">
                    {LAYOUT_BIASES.map((l) => (
                      <button
                        key={l.id}
                        type="button"
                        data-testid={`slides-layout-${l.id}`}
                        className={optionBtn(layoutBias === l.id)}
                        onClick={() => {
                          setLayoutBias(l.id)
                          setDialog(null)
                        }}
                      >
                        {l.label}
                      </button>
                    ))}
                  </div>
                )}

                {dialog === 'intent' && (
                  <div className="space-y-1.5">
                    {(
                      [
                        [
                          'edit-active',
                          'Edit active',
                          'Change only the selected existing slide',
                        ],
                        [
                          'fill-blank',
                          'Fill blank',
                          'Fill the blank selected slide with content',
                        ],
                        [
                          'new-after',
                          'New after',
                          'Create one new slide after the selected one',
                        ],
                        [
                          'rebuild-deck',
                          'Rebuild deck',
                          'Rewrite the entire multi-slide deck',
                        ],
                      ] as const
                    ).map(([id, label, desc]) => (
                      <button
                        key={id}
                        type="button"
                        data-testid={`slides-intent-${id}`}
                        className={optionBtn(intent === id)}
                        onClick={() => {
                          intentTouched.current = true
                          setIntent(id)
                          setDialog(null)
                        }}
                      >
                        <div className="font-medium">{label}</div>
                        <div className="mt-0.5 text-[10px] text-shell-muted">
                          {desc}
                        </div>
                      </button>
                    ))}
                  </div>
                )}

                {dialog === 'image' && (
                  <div className="space-y-4">
                    <section className="space-y-2">
                      <div className="text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
                        Add image
                      </div>
                      <p className="text-[11px] text-shell-muted">
                        Attach, drag & drop onto the agent bar, pick from Image
                        Studio, or generate — then place into the active slide
                        (like Excalidraw insert image).
                      </p>
                      <div className="flex flex-wrap gap-1.5">
                        <button
                          type="button"
                          data-testid="slides-image-attach"
                          className="rounded-md border border-shell-border px-2 py-1 text-[11px] text-shell-text hover:border-shell-accent"
                          onClick={() => attachInputRef.current?.click()}
                        >
                          Attach file
                        </button>
                        <button
                          type="button"
                          data-testid="slides-image-force"
                          disabled={!firstImageAttach || !activeSlideId}
                          className="rounded-md border border-shell-border px-2 py-1 text-[11px] text-shell-text hover:border-shell-accent disabled:opacity-40"
                          onClick={forcePlace}
                        >
                          Place now
                        </button>
                      </div>
                      <div
                        className="rounded-md border border-dashed border-shell-border px-3 py-6 text-center text-[11px] text-shell-muted"
                        onDragOver={(e) => {
                          e.preventDefault()
                          setDragOver(true)
                        }}
                        onDrop={onDrop}
                      >
                        Drop image here
                      </div>
                    </section>

                    <section className="space-y-2">
                      <div className="text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
                        Slot on slide
                      </div>
                      <div className="grid grid-cols-2 gap-1.5">
                        {IMAGE_SLOTS.map((s) => (
                          <button
                            key={s.id}
                            type="button"
                            data-testid={`slides-slot-${s.id}`}
                            className={optionBtn(imageSlot === s.id)}
                            onClick={() => setImageSlot(s.id)}
                          >
                            {s.label}
                          </button>
                        ))}
                      </div>
                    </section>

                    <section className="space-y-2">
                      <div className="text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
                        On agent send
                      </div>
                      <label className="flex cursor-pointer items-center gap-2 text-[11px] text-shell-text">
                        <input
                          type="checkbox"
                          checked={placeOnSend}
                          onChange={(e) => {
                            setPlaceOnSend(e.target.checked)
                            if (e.target.checked) setVisionOnly(false)
                          }}
                        />
                        Place attached image into slide (slot above)
                      </label>
                      <label className="flex cursor-pointer items-center gap-2 text-[11px] text-shell-text">
                        <input
                          type="checkbox"
                          checked={visionOnly}
                          onChange={(e) => {
                            setVisionOnly(e.target.checked)
                            if (e.target.checked) setPlaceOnSend(false)
                          }}
                        />
                        Vision / style ref only (do not embed)
                      </label>
                    </section>

                    <section className="space-y-2">
                      <div className="text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
                        Image Studio library
                      </div>
                      {studioImages.length === 0 ? (
                        <p className="text-[11px] text-shell-muted">
                          No studio images yet. Generate in Image Studio or
                          below.
                        </p>
                      ) : (
                        <div className="grid grid-cols-3 gap-1.5">
                          {studioImages.map((img) => (
                            <button
                              key={`${img.id}-${img.imageUrl.slice(0, 24)}`}
                              type="button"
                              title={img.title}
                              className="overflow-hidden rounded-md border border-shell-border hover:border-shell-accent"
                              onClick={() => {
                                addStudioImage(img)
                              }}
                            >
                              <img
                                src={img.imageUrl}
                                alt={img.title}
                                className="aspect-square w-full object-cover"
                              />
                            </button>
                          ))}
                        </div>
                      )}
                    </section>

                    <section className="space-y-2">
                      <div className="text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
                        Generate image
                      </div>
                      <textarea
                        data-testid="slides-image-gen-prompt"
                        value={genPrompt}
                        onChange={(e) => setGenPrompt(e.target.value)}
                        rows={3}
                        placeholder="Describe image for this slide…"
                        className="w-full resize-none rounded-md border border-shell-border bg-shell-bg px-2 py-1.5 text-[11px] text-shell-text outline-none focus:border-shell-accent"
                      />
                      <button
                        type="button"
                        data-testid="slides-image-generate"
                        disabled={!genPrompt.trim() || genBusy || busy}
                        onClick={() => void generateAndPlace()}
                        className="w-full rounded-md bg-shell-accent px-2 py-1.5 text-[11px] font-medium text-white disabled:opacity-40"
                      >
                        {genBusy ? 'Generating…' : 'Generate & place'}
                      </button>
                    </section>
                  </div>
                )}
              </div>
            </div>
          </div>,
          document.body,
        )}
    </div>
  )
}
