import { streamChat, type ChatEvent } from '../../../lib/api'
import { extractImageFromMarkdown } from '../galleryStore'
import {
  applyImageToSlot,
  listAiImageSlots,
  markImageSlotFailed,
  markImageSlotLoading,
  type ImageGenSlot,
} from './htmlDeck'

export type SlideImageJobState = {
  busy: boolean
  slots: ImageGenSlot[]
  error: string | null
}

type Listener = () => void

const states = new Map<string, SlideImageJobState>()
const idleByKey = new Map<string, SlideImageJobState>()
const listeners = new Map<string, Set<Listener>>()
const aborts = new Map<string, AbortController>()

const IMAGE_GEN_TIMEOUT_MS = 120_000

function idleState(canvasKey: string): SlideImageJobState {
  let idle = idleByKey.get(canvasKey)
  if (!idle) {
    idle = { busy: false, slots: [], error: null }
    idleByKey.set(canvasKey, idle)
  }
  return idle
}

export function getSlideImageJob(canvasKey: string): SlideImageJobState {
  return states.get(canvasKey) || idleState(canvasKey)
}

export function subscribeSlideImageJobs(
  canvasKey: string,
  fn: Listener,
): () => void {
  let set = listeners.get(canvasKey)
  if (!set) {
    set = new Set()
    listeners.set(canvasKey, set)
  }
  set.add(fn)
  return () => {
    set?.delete(fn)
  }
}

function emit(canvasKey: string) {
  listeners.get(canvasKey)?.forEach((fn) => fn())
}

function setState(canvasKey: string, patch: Partial<SlideImageJobState>) {
  const prev = getSlideImageJob(canvasKey)
  states.set(canvasKey, { ...prev, ...patch })
  emit(canvasKey)
}

export function stopSlideImageJobs(canvasKey: string) {
  const current = aborts.get(canvasKey)
  if (!current) return
  current.abort()
  setState(canvasKey, { busy: false, error: null })
}

async function streamChatWithTimeout(
  req: Parameters<typeof streamChat>[0],
  onEvent: (ev: ChatEvent) => void,
  signal: AbortSignal,
  timeoutMs: number,
): Promise<void> {
  const timeoutCtrl = new AbortController()
  const onAbort = () => timeoutCtrl.abort()
  signal.addEventListener('abort', onAbort)
  const timer = setTimeout(() => timeoutCtrl.abort(), timeoutMs)
  try {
    await streamChat(req, onEvent, timeoutCtrl.signal)
  } catch (e) {
    if (timeoutCtrl.signal.aborted && !signal.aborted) {
      throw new Error(`Image generation timed out after ${Math.round(timeoutMs / 1000)}s`)
    }
    throw e
  } finally {
    clearTimeout(timer)
    signal.removeEventListener('abort', onAbort)
  }
}

export async function runSlideImageJobs(opts: {
  canvasKey: string
  html: string
  contextId?: string
  playgroundId?: string
  onHtml: (html: string) => void
  sticky?: Record<string, unknown>
}): Promise<string> {
  const { canvasKey, contextId, playgroundId, onHtml } = opts
  let html = opts.html
  const found = listAiImageSlots(html)
  if (!found.length) {
    setState(canvasKey, { busy: false, slots: [], error: null })
    return html
  }

  const previous = aborts.get(canvasKey)
  previous?.abort()
  const ac = new AbortController()
  aborts.set(canvasKey, ac)

  const slots: ImageGenSlot[] = found.map((s) => ({
    ...s,
    status: 'pending',
  }))
  setState(canvasKey, { busy: true, slots: [...slots], error: null })

  for (let i = 0; i < slots.length; i += 1) {
    if (ac.signal.aborted || aborts.get(canvasKey) !== ac) break
    const slot = slots[i]
    slots[i] = { ...slot, status: 'loading' }
    setState(canvasKey, { slots: [...slots], busy: true })
    html = markImageSlotLoading(html, slot.slideId, slot.slot)
    onHtml(html)

    try {
      let assembled = ''
      let streamError = ''
      await streamChatWithTimeout(
        {
          prompt: [
            'Generate one presentation illustration image.',
            'No text, logos, or watermarks in the image.',
            'Subject:',
            slot.prompt,
          ].join('\n'),
          generate_image: true,
          agent_kind: 'slides',
          playground_id: playgroundId,
          no_tools: true,
          workspace_id: contextId,
          image_size: '1536x1024',
          image_only: true,
          ...(opts.sticky || {}),
        },
        (ev: ChatEvent) => {
          if (aborts.get(canvasKey) !== ac || ac.signal.aborted) return
          if (ev.type === 'TokenDelta' && ev.delta) assembled += ev.delta
          if (ev.type === 'Done' && ev.final) assembled = ev.final
          if (ev.type === 'Error' && ev.error) streamError = ev.error
        },
        ac.signal,
        IMAGE_GEN_TIMEOUT_MS,
      )
      if (aborts.get(canvasKey) !== ac) break
      if (ac.signal.aborted) throw new DOMException('Generation cancelled', 'AbortError')
      if (streamError) {
        throw new Error(streamError)
      }
      const parsed = extractImageFromMarkdown(assembled)
      if (parsed.imageUrl) {
        const next = applyImageToSlot(
          html,
          slot.slideId,
          slot.slot,
          parsed.imageUrl,
          slot.prompt.slice(0, 80),
        )
        if (next) {
          html = next
          onHtml(html)
          slots[i] = { ...slots[i], status: 'done' }
        } else {
          html = markImageSlotFailed(
            html,
            slot.slideId,
            slot.slot,
            'Could not place image',
          )
          onHtml(html)
          slots[i] = { ...slots[i], status: 'error' }
        }
      } else {
        html = markImageSlotFailed(
          html,
          slot.slideId,
          slot.slot,
          'No image in response',
        )
        onHtml(html)
        slots[i] = { ...slots[i], status: 'error' }
        setState(canvasKey, {
          error: 'Image generation returned no image URL',
          slots: [...slots],
        })
      }
    } catch (e) {
      if (aborts.get(canvasKey) !== ac) break
      if ((e as Error)?.name === 'AbortError' || ac.signal.aborted) {
        html = markImageSlotFailed(html, slot.slideId, slot.slot, 'Generation cancelled')
        onHtml(html)
        slots[i] = { ...slots[i], status: 'error' }
        break
      }
      const msg = e instanceof Error ? e.message : String(e)
      html = markImageSlotFailed(html, slot.slideId, slot.slot, msg)
      onHtml(html)
      slots[i] = { ...slots[i], status: 'error' }
      setState(canvasKey, { error: msg, slots: [...slots] })
    }
    if (aborts.get(canvasKey) === ac) setState(canvasKey, { slots: [...slots] })
  }

  if (aborts.get(canvasKey) === ac) {
    setState(canvasKey, { busy: false, slots: [...slots] })
    aborts.delete(canvasKey)
  }
  return html
}
