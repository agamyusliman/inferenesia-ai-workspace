import {
  FormEvent,
  useCallback,
  useRef,
  useState,
  useSyncExternalStore,
} from 'react'
import {
  Check,
  ChevronDown,
  ChevronUp,
  Image as ImageIcon,
  Loader2,
  Minus,
  Paperclip,
  Plus,
  Sparkles,
  Trash2,
  Type,
  X,
} from 'lucide-react'
import { streamChat, type ChatEvent } from '../../../lib/api'
import {
  buildAttachmentPayload,
  canSendWithAttachments,
  composePromptWithAttachments,
  processAttachmentFile,
  removeAttachmentById,
  type ChatAttachment,
} from '../../chat/chatAttachments'
import {
  modelPickerDisabledWhileStreaming,
  stickyRequestFields,
  type StickySession,
} from '../../chat/chatReliability'
import {
  ModelPicker,
  type ModelPickerLocalValue,
} from '../../chat/ModelPicker'
import {
  clearCanvasAgentJob,
  getCanvasAgentJob,
  setCanvasAgentJob,
  subscribeCanvasAgentJobs,
} from '../../web-preview/canvasAgentJobs'
import { useLocale } from '../../i18n/LocaleProvider'
import { extractHtmlDocument } from './htmlDeck'
import { resolvePlaygroundScope } from '../playgroundScope'
import {
  buildDeckFromPlanPrompt,
  buildOutlineAgentPrompt,
  DECOR_STYLES,
  type DecorStyleId,
  densityById,
  emptyOutlineCard,
  IMAGE_SOURCES,
  type ImageSourceId,
  markWizardDone,
  mintOutlineId,
  parseOutlineFromReply,
  TEXT_DENSITY,
  type TextDensityId,
  type OutlineCard,
  VISUAL_THEMES,
  type VisualThemeId,
} from './slidesWizard'

export { deckNeedsWizard } from './slidesWizard'

type Props = {
  contextId?: string
  canvasKey: string
  canvasEntrySessionId?: string
  title: string
  onComplete: (
    html: string,
    opts?: {
      runAiImages?: boolean
      /** Sticky for image-gen jobs (vision model). */
      imageSticky?: { profile?: string; model?: string }
    },
  ) => void
  onSkip: () => void
  className?: string
}

function useJob(key: string) {
  return useSyncExternalStore(
    subscribeCanvasAgentJobs,
    () => getCanvasAgentJob(key),
    () => getCanvasAgentJob(key),
  )
}

export function SlidesSetupWizard({
  contextId,
  canvasKey,
  canvasEntrySessionId,
  title,
  onComplete,
  onSkip,
  className = '',
}: Props) {
  const { t } = useLocale()
  const playgroundScope = resolvePlaygroundScope(
    'slides',
    canvasEntrySessionId,
    contextId,
  )
  const jobKey = `${canvasKey}::wizard`
  const job = useJob(jobKey)
  const busy = job.busy

  const [topic, setTopic] = useState('')
  const [materials, setMaterials] = useState('')
  const [attachments, setAttachments] = useState<ChatAttachment[]>([])
  const [outline, setOutline] = useState<OutlineCard[]>([])
  const [density, setDensity] = useState<TextDensityId>('ringkas')
  const [theme, setTheme] = useState<VisualThemeId>('tranquil')
  const [customThemeInstruction, setCustomThemeInstruction] = useState('')
  const [decor, setDecor] = useState<DecorStyleId>('decorative')
  const [imageSource, setImageSource] = useState<ImageSourceId>('none')
  const [imageMenuOpen, setImageMenuOpen] = useState(false)
  const [slideCount, setSlideCount] = useState(5)
  const [language, setLanguage] = useState<'id' | 'en'>('id')
  /** Text model: outline + deck redaction / HTML generation. */
  const [textModel, setTextModel] = useState<ModelPickerLocalValue | null>(null)
  /** Vision / image model: read attachments + generate images later. */
  const [visionModel, setVisionModel] = useState<ModelPickerLocalValue | null>(
    null,
  )
  const stickyRef = useRef<StickySession>({})
  const attachRef = useRef<HTMLInputElement | null>(null)
  const abortRef = useRef<AbortController | null>(null)
  const runRef = useRef(0)

  const stickyFromLocal = useCallback(
    (slot: ModelPickerLocalValue | null): StickySession => {
      if (!slot?.profile && !slot?.model) return stickyRef.current
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
    stickyRef.current = {
      profile: next.profile,
      model: next.model,
      label: next.label,
    }
  }, [])

  const onVisionModelChange = useCallback((next: ModelPickerLocalValue) => {
    setVisionModel(next)
  }, [])

  const stop = () => {
    runRef.current += 1
    abortRef.current?.abort()
    abortRef.current = null
    setCanvasAgentJob(jobKey, { busy: false, status: null, error: null })
    clearCanvasAgentJob(jobKey)
  }

  const onPickFiles = async (files: FileList | null) => {
    if (!files?.length) return
    const list = Array.from(files)
    const results = await Promise.all(list.map((f) => processAttachmentFile(f)))
    setAttachments((prev) => [...prev, ...results])
  }

  const generateOutline = async () => {
    if (busy) return
    if (!canSendWithAttachments(topic, attachments, []) && !materials.trim()) {
      setCanvasAgentJob(jobKey, {
        busy: false,
        status: null,
        error: 'Isi topik atau tempel bahan dulu',
      })
      return
    }
    const runId = ++runRef.current
    const attachPayload = buildAttachmentPayload(attachments)
    const userFacing = composePromptWithAttachments(
      [topic.trim(), materials.trim()].filter(Boolean).join('\n\n'),
      attachPayload,
    )
    const ac = new AbortController()
    abortRef.current = ac
    setCanvasAgentJob(jobKey, {
      busy: true,
      status: 'Menyusun garis besar…',
      error: null,
    })
    let assembled = ''
    // Use vision model when attachments include images and vision is set.
    const outlineSticky =
      attachPayload.images.length > 0 &&
      (visionModel?.profile || visionModel?.model)
        ? stickyFromLocal(visionModel)
        : stickyFromLocal(textModel)
    try {
      await streamChat(
        {
          prompt: buildOutlineAgentPrompt({
            topic: userFacing || topic,
            materials,
            slideCount,
            language,
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
          ...stickyRequestFields(outlineSticky),
        },
        (ev: ChatEvent) => {
          if (runRef.current !== runId || abortRef.current !== ac) return
          if (ev.type === 'TokenDelta' && ev.delta) assembled += ev.delta
          if (ev.type === 'Done' && ev.final) {
            if (
              !assembled ||
              (ev.final.length >= assembled.length &&
                (ev.final.includes('<') || ev.final.includes('{')))
            ) {
              assembled = ev.final
            }
          }
          if (ev.type === 'Error' && ev.error) {
            setCanvasAgentJob(jobKey, { error: ev.error })
          }
        },
        ac.signal,
      )
      if (runRef.current !== runId || abortRef.current !== ac) return
      const cards = parseOutlineFromReply(assembled, slideCount)
      setOutline(cards.length ? cards : [emptyOutlineCard(topic || title)])
      setCanvasAgentJob(jobKey, {
        busy: false,
        status: `${cards.length || 1} slide direncanakan`,
        error: null,
      })
      window.setTimeout(() => {
        const cur = getCanvasAgentJob(jobKey)
        if (cur.status?.includes('direncanakan')) {
          setCanvasAgentJob(jobKey, { status: null })
          if (!cur.busy && !cur.error) clearCanvasAgentJob(jobKey)
        }
      }, 1200)
    } catch (e) {
      if (runRef.current !== runId || abortRef.current !== ac) return
      if ((e as Error)?.name === 'AbortError') {
        setCanvasAgentJob(jobKey, { busy: false, status: null, error: null })
        return
      }
      setCanvasAgentJob(jobKey, {
        busy: false,
        status: null,
        error: e instanceof Error ? e.message : String(e),
      })
    } finally {
      if (runRef.current === runId && abortRef.current === ac) abortRef.current = null
    }
  }

  const buildDeck = async () => {
    if (busy) return
    const cards =
      outline.length > 0
        ? outline
        : [emptyOutlineCard(topic.trim() || title || 'Slide 1')]
    const runId = ++runRef.current
    const ac = new AbortController()
    abortRef.current = ac
    setCanvasAgentJob(jobKey, {
      busy: true,
      status: 'Membuat deck…',
      error: null,
    })
    let assembled = ''
    const deckSticky = stickyFromLocal(textModel)
    try {
      await streamChat(
        {
          prompt: buildDeckFromPlanPrompt({
            topic,
            materials,
            density,
            theme,
            customThemeInstruction,
            decor,
            imageSource,
            outline: cards,
            language,
          }),
          no_tools: true,
          agent_kind: 'slides',
          playground_id: playgroundScope.playgroundId,
          session_id:
            playgroundScope.mode === 'session'
              ? playgroundScope.sessionId
              : undefined,
          workspace_id: contextId,
          ...stickyRequestFields(deckSticky),
        },
        (ev: ChatEvent) => {
          if (runRef.current !== runId || abortRef.current !== ac) return
          if (ev.type === 'TokenDelta' && ev.delta) assembled += ev.delta
          if (ev.type === 'Done' && ev.final) {
            const fin = ev.final
            const hasHtml =
              /<!DOCTYPE\s+html|<html[\s>]|```(?:html|htm)|class=["'][^"']*\bslide\b/i.test(
                fin,
              )
            if (
              !assembled ||
              (hasHtml && fin.length >= Math.min(assembled.length, 200)) ||
              fin.length > assembled.length + 40
            ) {
              assembled = fin
            }
          }
          if (ev.type === 'Error' && ev.error) {
            setCanvasAgentJob(jobKey, { error: ev.error })
          }
        },
        ac.signal,
      )
      if (runRef.current !== runId || abortRef.current !== ac) return
      const html = extractHtmlDocument(assembled)
      if (!html) {
        const preview = assembled.trim().slice(0, 200).replace(/\s+/g, ' ')
        setCanvasAgentJob(jobKey, {
          busy: false,
          status: null,
          error:
            getCanvasAgentJob(jobKey).error ||
            `Tidak ada HTML deck di balasan model · ${preview || '(kosong)'}`,
        })
        return
      }
      const finalHtml = markWizardDone(html)
      setCanvasAgentJob(jobKey, { busy: false, status: 'Selesai', error: null })
      clearCanvasAgentJob(jobKey)
      const visionSticky = stickyRequestFields(stickyFromLocal(visionModel))
      onComplete(finalHtml, {
        runAiImages: imageSource === 'ai' || imageSource === 'placeholder',
        imageSticky:
          visionSticky.profile || visionSticky.model
            ? visionSticky
            : undefined,
      })
    } catch (e) {
      if (runRef.current !== runId || abortRef.current !== ac) return
      if ((e as Error)?.name === 'AbortError') {
        setCanvasAgentJob(jobKey, { busy: false, status: null, error: null })
        return
      }
      setCanvasAgentJob(jobKey, {
        busy: false,
        status: null,
        error: e instanceof Error ? e.message : String(e),
      })
    } finally {
      if (runRef.current === runId && abortRef.current === ac) abortRef.current = null
    }
  }

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (outline.length === 0) {
      void generateOutline()
    }
  }

  const updateCard = (id: string, patch: Partial<OutlineCard>) => {
    setOutline((prev) =>
      prev.map((c) => (c.id === id ? { ...c, ...patch } : c)),
    )
  }

  const removeCard = (id: string) => {
    setOutline((prev) => (prev.length <= 1 ? prev : prev.filter((c) => c.id !== id)))
  }

  const addCard = () => {
    setOutline((prev) => [
      ...prev,
      { id: mintOutlineId(), title: `Slide ${prev.length + 1}`, bullets: [] },
    ])
  }
  const moveCard = (id: string, direction: -1 | 1) => {
    setOutline((prev) => {
      const from = prev.findIndex((card) => card.id === id)
      const to = from + direction
      if (from < 0 || to < 0 || to >= prev.length) return prev
      const next = [...prev]
      const [moved] = next.splice(from, 1)
      next.splice(to, 0, moved)
      return next
    })
  }

  const outlineReady =
    outline.length > 0 && outline.every((card) => card.title.trim().length > 0)

  const imageLabel =
    IMAGE_SOURCES.find((s) => s.id === imageSource)?.label || 'Sumber gambar'

  return (
    <div
      data-testid="slides-setup-wizard"
      className={`playground-workbench bg-shell-bg ${className}`}
    >
      <div className="flex h-10 shrink-0 items-center justify-between overflow-hidden border-b border-shell-border bg-shell-panel px-3">
        <div className="flex min-w-0 items-center gap-2">
          <span className="text-[11px] font-semibold uppercase tracking-wide text-shell-muted">
            Setup slides
          </span>
          <span className="hidden truncate text-[11px] font-medium text-shell-text sm:inline">
            {title || 'Deck baru'}
          </span>
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          {busy && (
            <button
              type="button"
              onClick={stop}
              className="rounded-md border border-shell-border px-2 py-1 text-[11px] text-shell-muted"
            >
              Stop
            </button>
          )}
          <button
            type="button"
            data-testid="slides-wizard-skip"
            disabled={busy}
            onClick={onSkip}
            className="rounded-md border border-shell-border px-2 py-1 text-[11px] text-shell-muted hover:text-shell-text disabled:opacity-40"
            title="Lewati wizard, buka editor kosong"
          >
            Skip
          </button>
        </div>
      </div>

      {(job.error || (job.status && !busy)) && (
        <div
          role={job.error ? 'alert' : 'status'}
          className={`rounded-md border px-2 py-1.5 text-[11px] leading-relaxed ${
            job.error
              ? 'border-red-500/30 bg-red-500/10 text-red-300'
              : 'border-shell-accent/30 bg-shell-active text-shell-accent'
          }`}
        >
          {job.error || job.status}
        </div>
      )}

      <div className="shell-scroll min-h-0 flex-1 overflow-y-auto px-3 py-2">
        <form
          data-testid="slides-wizard-form"
          onSubmit={onSubmit}
          className="flex w-full flex-col gap-3"
        >
          <section className="space-y-2">
            <div className="flex flex-wrap items-center gap-2 text-[11px]">
              <span className="text-shell-muted">Jumlah slide</span>
              <div className="flex items-center gap-0.5 rounded-full border border-shell-border bg-shell-panel p-0.5">
                <button
                  type="button"
                  disabled={busy || slideCount <= 1}
                  data-testid="slides-wizard-count-minus"
                  onClick={() => setSlideCount((n) => Math.max(1, n - 1))}
                  className="flex h-5 w-5 items-center justify-center rounded-full text-shell-muted hover:bg-shell-hover hover:text-shell-text disabled:opacity-30"
                  title="Kurangi"
                >
                  <Minus className="h-3 w-3" />
                </button>
                <input
                  type="number"
                  min={1}
                  max={30}
                  value={slideCount}
                  disabled={busy}
                  data-testid="slides-wizard-count-input"
                  onChange={(e) => {
                    const n = Number(e.target.value)
                    if (!Number.isNaN(n) && n >= 1) setSlideCount(Math.min(30, Math.floor(n)))
                  }}
                  onBlur={(e) => {
                    const n = Number(e.target.value)
                    setSlideCount(!Number.isNaN(n) && n >= 1 ? Math.min(30, Math.floor(n)) : 5)
                  }}
                  className="w-7 border-0 bg-transparent text-center text-[11px] font-medium text-shell-text outline-none [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
                />
                <button
                  type="button"
                  disabled={busy || slideCount >= 30}
                  data-testid="slides-wizard-count-plus"
                  onClick={() => setSlideCount((n) => Math.min(30, n + 1))}
                  className="flex h-5 w-5 items-center justify-center rounded-full text-shell-muted hover:bg-shell-hover hover:text-shell-text disabled:opacity-30"
                  title="Tambah"
                >
                  <Plus className="h-3 w-3" />
                </button>
              </div>
              <div className="mx-1 h-3 w-px bg-shell-border" />
              <select
                value={language}
                disabled={busy}
                onChange={(e) =>
                  setLanguage(e.target.value === 'en' ? 'en' : 'id')
                }
                className="rounded-full border border-shell-border bg-shell-panel px-2 py-0.5 text-[11px] text-shell-text"
              >
                <option value="id">Bahasa Indonesia</option>
                <option value="en">English</option>
              </select>
            </div>
            <div
              data-testid="slides-wizard-models"
              className="flex flex-wrap items-start gap-2 rounded-lg border border-shell-border bg-shell-panel px-2 py-2"
            >
              <div className="min-w-0 flex-1 space-y-1">
                <div className="text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
                  Model teks
                </div>
                <p className="text-[10px] leading-snug text-shell-muted">
                  Garis besar + redaksi + generate HTML deck
                </p>
                <ModelPicker
                  localOnly
                  slotLabel="Teks"
                  testId="slides-wizard-model-text"
                  value={textModel}
                  onLocalChange={onTextModelChange}
                  disabled={modelPickerDisabledWhileStreaming(busy)}
                />
              </div>
              <div className="hidden h-16 w-px bg-shell-border sm:block" />
              <div className="min-w-0 flex-1 space-y-1">
                <div className="text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
                  Model vision
                </div>
                <p className="text-[10px] leading-snug text-shell-muted">
                  Baca file/gambar lampiran + generate gambar AI
                </p>
                <ModelPicker
                  localOnly
                  slotLabel="Vision"
                  testId="slides-wizard-model-vision"
                  value={visionModel}
                  onLocalChange={onVisionModelChange}
                  disabled={modelPickerDisabledWhileStreaming(busy)}
                />
              </div>
            </div>
            <input
              data-testid="slides-wizard-topic"
              value={topic}
              disabled={busy}
              onChange={(e) => setTopic(e.target.value)}
              placeholder="Contoh: ppt tentang pancasila dasar / pitch produk kasir F&B"
              className="w-full rounded-lg border border-shell-border bg-shell-panel px-3 py-1.5 text-[13px] text-shell-text outline-none placeholder:text-shell-muted focus:border-shell-accent"
            />
          </section>

          <section className="space-y-2">
            <div className="flex items-center justify-between">
              <label className="text-[11px] font-medium text-shell-muted">
                Bahan / catatan (opsional)
              </label>
              <button
                type="button"
                disabled={busy}
                onClick={() => attachRef.current?.click()}
                className="inline-flex items-center gap-1 rounded-md border border-shell-border px-2 py-0.5 text-[10px] text-shell-muted hover:text-shell-text"
              >
                <Paperclip className="h-3 w-3" />
                Attach
              </button>
              <input
                ref={attachRef}
                type="file"
                className="hidden"
                multiple
                onChange={(e) => void onPickFiles(e.target.files)}
              />
            </div>
            <textarea
              data-testid="slides-wizard-materials"
              value={materials}
              disabled={busy}
              onChange={(e) => setMaterials(e.target.value)}
              rows={4}
              placeholder="Tempel outline, poin, kutipan, data… atau lampirkan dokumen."
              className="w-full resize-y rounded-lg border border-shell-border bg-shell-panel px-3 py-1.5 text-xs text-shell-text outline-none placeholder:text-shell-muted focus:border-shell-accent"
            />
            {attachments.length > 0 && (
              <div className="flex flex-wrap gap-1">
                {attachments.map((a) => (
                  <span
                    key={a.id}
                    className="inline-flex items-center gap-1 rounded-full border border-shell-border px-2 py-0.5 text-[10px] text-shell-muted"
                  >
                    {a.name}
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() =>
                        setAttachments((prev) => removeAttachmentById(prev, a.id))
                      }
                      className="hover:text-shell-text"
                    >
                      <X className="h-3 w-3" />
                    </button>
                  </span>
                ))}
              </div>
            )}
          </section>

          {outline.length > 0 && (
            <section className="space-y-2" data-testid="slides-wizard-outline">
              <div className="flex items-center justify-between">
                <div className="text-[11px] font-semibold uppercase tracking-wide text-shell-muted">
                  Garis besar
                </div>
                <span className="text-[10px] text-shell-muted">
                  {outline.length} slide
                </span>
              </div>
              <div className="space-y-2">
                {outline.map((card, i) => (
                  <div
                    key={card.id}
                    className="rounded-lg border border-shell-border bg-shell-panel p-2"
                  >
                    <div className="mb-1.5 flex items-start gap-2">
                      <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded bg-shell-active text-[10px] font-bold text-shell-accent">
                        {i + 1}
                      </span>
                      <input
                        value={card.title}
                        disabled={busy}
                        onChange={(e) =>
                          updateCard(card.id, { title: e.target.value })
                        }
                        className={`min-w-0 flex-1 border-b bg-transparent text-xs font-semibold text-shell-text outline-none ${
                          card.title.trim() ? 'border-transparent' : 'border-amber-400'
                        }`}
                        aria-label={t('slides.wizard.titleLabel', { slide: i + 1 })}
                      />
                      <button
                        type="button"
                        disabled={busy || i === 0}
                        onClick={() => moveCard(card.id, -1)}
                        className="rounded p-0.5 text-shell-muted hover:bg-shell-hover hover:text-shell-text disabled:opacity-25"
                        title={t('slides.wizard.moveUp')}
                        aria-label={t('slides.wizard.moveUpLabel', { slide: i + 1 })}
                      >
                        <ChevronUp className="h-3.5 w-3.5" />
                      </button>
                      <button
                        type="button"
                        disabled={busy || i === outline.length - 1}
                        onClick={() => moveCard(card.id, 1)}
                        className="rounded p-0.5 text-shell-muted hover:bg-shell-hover hover:text-shell-text disabled:opacity-25"
                        title={t('slides.wizard.moveDown')}
                        aria-label={t('slides.wizard.moveDownLabel', { slide: i + 1 })}
                      >
                        <ChevronDown className="h-3.5 w-3.5" />
                      </button>
                      <button
                        type="button"
                        disabled={busy || outline.length <= 1}
                        onClick={() => removeCard(card.id)}
                        className="rounded p-0.5 text-shell-muted hover:bg-shell-hover hover:text-shell-text disabled:opacity-30"
                        title={t('slides.wizard.deleteSlide')}
                        aria-label={t('slides.wizard.deleteSlideLabel', { slide: i + 1 })}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                    <textarea
                      value={card.bullets.join('\n')}
                      disabled={busy}
                      onChange={(e) =>
                        updateCard(card.id, {
                          bullets: e.target.value
                            .split('\n')
                            .map((l) => l.replace(/^[-•*]\s*/, '').trim())
                            .filter(Boolean),
                        })
                      }
                      rows={Math.min(5, Math.max(2, card.bullets.length || 2))}
                      placeholder="Poin (satu baris = satu bullet)"
                      className="w-full resize-none rounded-md border border-shell-border bg-shell-bg px-2 py-1.5 text-[11px] text-shell-text outline-none placeholder:text-shell-muted"
                    />
                  </div>
                ))}
              </div>
              <button
                type="button"
                disabled={busy}
                onClick={addCard}
                className="flex w-full items-center justify-center gap-1 rounded-lg border border-dashed border-shell-border py-2 text-[11px] text-shell-muted hover:bg-shell-hover hover:text-shell-text"
              >
                <Plus className="h-3.5 w-3.5" />
                Tambah slide
              </button>
            </section>
          )}

          <section className="space-y-3 rounded-lg border border-shell-border bg-shell-panel p-3">
            <div className="flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wide text-shell-muted">
              <Type className="h-3.5 w-3.5" />
              Konten teks
            </div>
            <div className="grid grid-cols-2 gap-1.5 sm:grid-cols-4">
              {TEXT_DENSITY.map((d) => {
                const active = density === d.id
                return (
                  <button
                    key={d.id}
                    type="button"
                    disabled={busy}
                    data-testid={`slides-density-${d.id}`}
                    onClick={() => setDensity(d.id)}
                    className={`rounded-lg border px-2 py-1.5 text-left transition ${
                      active
                        ? 'border-shell-accent bg-shell-active text-shell-text'
                        : 'border-shell-border text-shell-text hover:bg-shell-hover'
                    }`}
                  >
                    <div className="text-[11px] font-semibold">{d.label}</div>
                    <div className="mt-0.5 text-[10px] text-shell-muted">
                      {d.hint}
                    </div>
                  </button>
                )
              })}
            </div>

            <div className="border-t border-shell-border pt-3">
              <div className="mb-2 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wide text-shell-muted">
                <Sparkles className="h-3.5 w-3.5" />
                Tema visual
              </div>
              <div className="grid grid-cols-2 gap-1.5 sm:grid-cols-4">
                {VISUAL_THEMES.map((t) => {
                  const active = theme === t.id
                  return (
                    <button
                      key={t.id}
                      type="button"
                      disabled={busy}
                      data-testid={`slides-theme-${t.id}`}
                      onClick={() => setTheme(t.id)}
                      className={`overflow-hidden rounded-lg border text-left transition ${
                        active
                          ? 'border-shell-accent ring-1 ring-shell-accent'
                          : 'border-shell-border hover:border-shell-accent'
                      }`}
                    >
                      <div
                        className="flex h-12 flex-col justify-end p-1.5"
                        style={{ background: t.swatch }}
                      >
                        <div
                          className="rounded-md px-1.5 py-1 shadow-sm"
                          style={{ background: t.card, color: t.fg }}
                        >
                          <div className="text-[10px] font-bold leading-tight">
                            Judul
                          </div>
                          <div className="text-[8px] opacity-70">Isi & tautan</div>
                        </div>
                      </div>
                      <div className="flex items-center gap-1 px-1.5 py-1 text-[10px] text-shell-muted">
                        {active ? (
                          <Check className="h-3 w-3 text-shell-accent" />
                        ) : null}
                        {t.label}
                      </div>
                    </button>
                  )
                })}
              </div>
              {theme === 'custom' && (
                <div className="mt-2 space-y-1.5" data-testid="slides-custom-theme">
                  <label className="text-[10px] font-medium text-shell-muted">
                    Instruksi tema custom
                  </label>
                  <textarea
                    value={customThemeInstruction}
                    disabled={busy}
                    onChange={(e) => setCustomThemeInstruction(e.target.value)}
                    rows={4}
                    placeholder={
                      'Contoh: Tema gelap elegant dengan aksen emas/copper, background gradient deep charcoal ke navy, judul font besar bold putih, kartu translucent dengan border tipis emas, sedikit glow halus di card. Atau: Light minimal warm, background krem ivory, accent terracotta, judul serif, body sans-serif, banyak white space.'
                    }
                    className="w-full resize-y rounded-lg border border-shell-border bg-shell-bg px-3 py-1.5 text-xs text-shell-text outline-none placeholder:text-shell-muted focus:border-shell-accent"
                  />
                  <p className="text-[10px] text-shell-muted">
                    Jelaskan warna, mood, background, aksen, gaya kartu, dan
                    tipografi. Agent akan mengikuti instruksi ini.
                  </p>
                </div>
              )}

              <div className="mt-3 space-y-1.5" data-testid="slides-decor-style">
                <div className="text-[10px] font-medium text-shell-muted">
                  Gaya dekorasi
                </div>
                <div className="grid grid-cols-2 gap-1.5">
                  {DECOR_STYLES.map((d) => {
                    const active = decor === d.id
                    return (
                      <button
                        key={d.id}
                        type="button"
                        disabled={busy}
                        data-testid={`slides-decor-${d.id}`}
                        onClick={() => setDecor(d.id)}
                        className={`rounded-lg border px-2 py-1.5 text-left transition ${
                          active
                            ? 'border-shell-accent bg-shell-active text-shell-text'
                            : 'border-shell-border text-shell-text hover:bg-shell-hover'
                        }`}
                      >
                        <div className="flex items-center gap-1 text-[11px] font-semibold">
                          {active ? (
                            <Check className="h-3 w-3 shrink-0" />
                          ) : null}
                          {d.label}
                        </div>
                        <div className="mt-0.5 text-[10px] text-shell-muted">
                          {d.hint}
                        </div>
                      </button>
                    )
                  })}
                </div>
                <p className="text-[10px] text-shell-muted">
                  Biasa = layout bersih ber-depth tanpa ornament. Dekoratif =
                  shape / blob / icon / emoji soft agar slide tidak kosong.
                </p>
              </div>
            </div>

            <div className="relative border-t border-shell-border pt-3">
              <div className="mb-1.5 flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wide text-shell-muted">
                <ImageIcon className="h-3.5 w-3.5" />
                Sumber gambar
              </div>
              <button
                type="button"
                disabled={busy}
                data-testid="slides-image-source"
                onClick={() => setImageMenuOpen((v) => !v)}
                className="flex w-full items-center justify-between rounded-lg border border-shell-border bg-shell-bg px-3 py-2 text-left text-[12px] text-shell-text"
              >
                <span>{imageLabel}</span>
                <span className="text-[10px] text-shell-muted">
                  {imageMenuOpen ? 'Tutup' : 'Pilih'}
                </span>
              </button>
              {imageMenuOpen && (
                <div
                  role="menu"
                  className="absolute left-0 right-0 z-20 mt-1 max-h-64 overflow-y-auto rounded-lg border border-shell-border bg-shell-panel py-1 shadow-xl"
                >
                  {IMAGE_SOURCES.map((s) => (
                    <button
                      key={s.id}
                      type="button"
                      role="menuitem"
                      data-testid={`slides-img-src-${s.id}`}
                      onClick={() => {
                        setImageSource(s.id)
                        setImageMenuOpen(false)
                      }}
                      className={`flex w-full flex-col gap-0.5 px-3 py-1.5 text-left hover:bg-shell-hover ${
                        imageSource === s.id ? 'bg-shell-active' : ''
                      }`}
                    >
                      <span className="text-[12px] font-medium text-shell-text">
                        {s.label}
                      </span>
                      <span className="text-[10px] text-shell-muted">
                        {s.desc}
                      </span>
                    </button>
                  ))}
                </div>
              )}
              <p className="mt-1.5 text-[10px] text-shell-muted">
                Default cerdas: tidak memaksa gambar di setiap slide. AI generate /
                stok = slot terpilih saja; placeholder = kotak kosong; tanpa gambar
                = murni layout teks.
              </p>
            </div>
          </section>

          <div className="sticky bottom-0 flex flex-col gap-1.5 border-t border-shell-border bg-shell-bg py-2 backdrop-blur">
            <div className="flex flex-wrap items-center justify-between gap-2 text-[10px] text-shell-muted">
              <span>
                {outline.length > 0
                  ? `${outline.length} slide · ${densityById(density).label} · ${VISUAL_THEMES.find((t) => t.id === theme)?.label} · ${DECOR_STYLES.find((d) => d.id === decor)?.label}`
                  : 'Buat garis besar dulu, atau langsung generate dari topik'}
              </span>
              {busy && (
                <span className="inline-flex items-center gap-1 text-shell-accent">
                  <Loader2 className="h-3 w-3 animate-spin" />
                  {job.status || 'Working…'}
                </span>
              )}
            </div>
            <div className="flex flex-wrap gap-2">
              {outline.length === 0 ? (
                <button
                  type="submit"
                  data-testid="slides-wizard-outline-btn"
                  disabled={busy}
                  className="inline-flex flex-1 items-center justify-center gap-1.5 rounded-lg bg-shell-accent px-4 py-2 text-[13px] font-semibold text-white disabled:opacity-40"
                >
                  <Sparkles className="h-4 w-4" />
                  Susun garis besar
                </button>
              ) : (
                <>
                  <button
                    type="button"
                    data-testid="slides-wizard-reoutline"
                    disabled={busy}
                    onClick={() => void generateOutline()}
                    className="rounded-lg border border-shell-border px-3 py-2 text-[12px] font-medium text-shell-text disabled:opacity-40"
                  >
                    Ulangi outline
                  </button>
                  <button
                    type="button"
                    data-testid="slides-wizard-build"
                    disabled={busy || !outlineReady || (theme === 'custom' && !customThemeInstruction.trim())}
                    onClick={() => void buildDeck()}
                    className="inline-flex flex-1 items-center justify-center gap-1.5 rounded-lg bg-shell-accent px-4 py-2 text-[13px] font-semibold text-white disabled:opacity-40"
                  >
                    <Sparkles className="h-4 w-4" />
                    Buat {outline.length} slide
                  </button>
                </>
              )}
            </div>
            {!outlineReady && outline.length > 0 && (
              <p role="alert" className="text-[10px] text-amber-400">
                {t('slides.wizard.titleRequired')}
              </p>
            )}
            {theme === 'custom' && !customThemeInstruction.trim() && (
              <p role="alert" className="text-[10px] text-amber-400">
                {t('slides.wizard.customThemeRequired')}
              </p>
            )}
          </div>
        </form>
      </div>
    </div>
  )
}

