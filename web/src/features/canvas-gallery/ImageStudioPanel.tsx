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
import {
  Check,
  ChevronDown,
  ClipboardCopy,
  CornerUpLeft,
  Download,
  Expand,
  History,
  Image as ImageIcon,
  Paperclip,
  RotateCcw,
  Trash2,
  Upload,
  X,
} from 'lucide-react'
import { streamChat, type ActiveSessionView, type ChatEvent } from '../../lib/api'
import {
  MAX_VISION_IMAGES,
  canSendWithAttachments,
  processAttachmentFile,
  type ChatAttachment,
  type VisionImagePart,
} from '../chat/chatAttachments'
import {
  imageGenDegradeContent,
  isImageGenUnsupported,
} from '../chat/chatImageGen'
import {
  modelPickerDisabledWhileStreaming,
  stickyFromActive,
  stickyRequestFields,
  type StickySession,
} from '../chat/chatReliability'
import { GeneratingLabel } from '../chat/GeneratingLabel'
import { ModelPicker } from '../chat/ModelPicker'
import { useLocale } from '../i18n/LocaleProvider'
import { VerticalSplitter } from '../shell/VerticalSplitter'
import {
  clearCanvasAgentJob,
  getCanvasAgentJob,
  setCanvasAgentJob,
  subscribeCanvasAgentJobs,
} from '../web-preview/canvasAgentJobs'
import { extractImageFromMarkdown } from './galleryStore'
import {
  DEFAULT_IMAGE_STUDIO_OPTIONS,
  IMAGE_STUDIO_SIZE_GROUPS,
  IMAGE_STUDIO_SIZE_OPTIONS,
  getImageStudioSizeOption,
  imageStudioSize,
  imageStudioSizeSelectLabel,
  type ImageStudioAspect,
  type ImageStudioHistoryItem,
  type ImageStudioOptions,
  type ImageStudioOutputMode,
  type ImageStudioSizeGroup,
  type ImageStudioSizeOption,
  type ImageStudioThinking,
} from './galleryTypes'
import {
  downloadImageUrl,
  imageFormatLabel,
  imageMimeFromDataUrl,
  imageMimeFromUrl,
} from './imageAsset'
import { resolvePlaygroundScope } from './playgroundScope'

function mediaTypeFromDataUrl(url: string): string {
  return imageMimeFromDataUrl(url) || imageMimeFromUrl(url) || 'image/png'
}

function visionPartsFromAttachments(attachments: ChatAttachment[]): VisionImagePart[] {
  const out: VisionImagePart[] = []
  for (const a of attachments) {
    if (a.kind !== 'image' || a.loading || a.error) continue
    const dataUrl = (a.dataUrl || a.previewUrl || '').trim()
    if (!dataUrl) continue
    if (
      !dataUrl.startsWith('data:image/') &&
      !dataUrl.startsWith('https://') &&
      !dataUrl.startsWith('http://')
    ) {
      continue
    }
    out.push({
      name: a.name || `ref-${out.length + 1}`,
      media_type: a.mime || mediaTypeFromDataUrl(dataUrl),
      data_url: dataUrl,
    })
    if (out.length >= MAX_VISION_IMAGES) break
  }
  return out
}

function sanitizeImageCaption(raw: string): string {
  let c = (raw || '').trim()
  if (!c) return ''
  c = c.replace(/^Generated images?\s+for:\s*/i, '').trim()
  const cut = c.search(
    /\n*##\s*Reference image instructions|\n*##\s*Output geometry/i,
  )
  if (cut >= 0) c = c.slice(0, cut).trim()
  c = c
    .replace(/\s*Use the image_generation tool[\s\S]*$/i, '')
    .replace(/\s*You are given \d+ reference image[\s\S]*$/i, '')
    .trim()
  if (c.length > 280) c = c.slice(0, 277).trimEnd() + '…'
  if (/^#+\s|reference image instructions|output geometry/i.test(c)) {
    return ''
  }
  return c
}

function buildImageStudioPrompt(
  instruction: string,
  refCount: number,
  aspect: ImageStudioAspect,
  size: string,
): string {
  const ask = instruction.trim() || '(see reference images)'
  const lines = [
    ask,
    '',
    '## Output geometry (required)',
    `Target aspect ratio: ${aspect}.`,
    `Target pixel size: ${size}.`,
    `Generate the image in ${aspect} orientation (portrait if taller, landscape if wider). Do not keep the reference image’s original aspect if it differs.`,
  ]
  if (refCount > 0) {
    lines.push(
      '',
      '## Reference image instructions',
      `You are given ${refCount} reference image(s). Treat them as the visual source for style and assets.`,
      '- Preserve identity, style, design system, and objects the user wants kept unless they ask to change them.',
      '- Only edit / replace the parts the user describes.',
      `- Recompose the layout into ${aspect} (${size}) while keeping the same brand assets and visual language.`,
      '- Do not invent a completely unrelated scene unless the user asks for a full rewrite.',
      'Use the image_generation tool with the reference image(s) as image-to-image input.',
    )
  }
  return lines.join('\n')
}

type Props = {
  contextId?: string
  canvasKey?: string
  canvasEntrySessionId?: string
  title: string
  prompt: string
  imageUrl: string
  imageAspect?: ImageStudioAspect
  caption: string
  history?: ImageStudioHistoryItem[]
  options: ImageStudioOptions
  onOptionsChange: (next: ImageStudioOptions) => void
  agentPrompt?: string
  onResult: (
    result: {
      prompt: string
      agentPrompt: string
      imageUrl: string
      caption: string
      options: ImageStudioOptions
      title?: string
    },
    opts?: { final?: boolean; canvasId?: string },
  ) => void
  onRestoreHistory?: (historyId: string) => void
  onDeleteHistory?: (historyId: string) => void
  className?: string
}

type LiveStream = {
  abort: AbortController
  apply: Props['onResult']
}

type ImageStudioUiSession = {
  input: string
  historyTabId: string
  useAsRef: boolean
  historyWidth: number
}

const liveStreams = new Map<string, LiveStream>()
const uiSessions = new Map<string, ImageStudioUiSession>()

function readUiSession(canvasKey: string): ImageStudioUiSession {
  return (
    uiSessions.get(canvasKey) || {
      input: '',
      historyTabId: 'current',
      useAsRef: false,
      historyWidth: 96,
    }
  )
}

function writeUiSession(
  canvasKey: string,
  patch: Partial<ImageStudioUiSession>,
): void {
  if (!canvasKey) return
  const prev = readUiSession(canvasKey)
  uiSessions.set(canvasKey, { ...prev, ...patch })
}

const THINKING: ImageStudioThinking[] = ['low', 'medium', 'high']

const SIZE_GROUP_LABEL_KEY: Record<
  ImageStudioSizeGroup,
  | 'imageStudio.sizeGroupLandscape'
  | 'imageStudio.sizeGroupPortrait'
  | 'imageStudio.sizeGroupSquare'
  | 'imageStudio.sizeGroupPaper'
> = {
  landscape: 'imageStudio.sizeGroupLandscape',
  portrait: 'imageStudio.sizeGroupPortrait',
  square: 'imageStudio.sizeGroupSquare',
  paper: 'imageStudio.sizeGroupPaper',
}

const PROMPT_PLACEHOLDER_EN =
  'Subject, style, lighting, camera angle, mood/atmosphere… e.g. Portrait of a tea merchant in a rainy Jakarta alley, cinematic neo-noir, soft neon rim light, 35mm, shallow depth of field, muted teal–amber grade'

const PROMPT_PLACEHOLDER_ID =
  'Subjek, gaya visual, lighting, sudut pandang, mood/atmosfer… cth. Potret pedagang teh di lorong Jakarta hujan, cinematic neo-noir, rim light neon lembut, 35mm, shallow DoF, grade teal–amber redup'

export function ImageStudioPanel({
  contextId,
  canvasKey = 'image-default',
  canvasEntrySessionId,
  title,
  prompt,
  agentPrompt = '',
  imageUrl,
  imageAspect,
  caption: _caption,
  history = [],
  options,
  onOptionsChange,
  onResult,
  onRestoreHistory,
  onDeleteHistory,
  className = '',
}: Props) {
  const currentImageAspect: ImageStudioAspect = imageAspect || '1:1'
  const { locale, t } = useLocale()
  const playgroundScope = resolvePlaygroundScope('image', canvasEntrySessionId, contextId)
  const [input, setInput] = useState(() => readUiSession(canvasKey).input)
  const [fileAttachments, setFileAttachments] = useState<ChatAttachment[]>([])
  const [historyTabId, setHistoryTabId] = useState(
    () => readUiSession(canvasKey).historyTabId,
  )
  const [useAsRef, setUseAsRef] = useState(
    () => readUiSession(canvasKey).useAsRef,
  )
  const [sizeModalOpen, setSizeModalOpen] = useState(false)
  const [historyWidth, setHistoryWidth] = useState(
    () => readUiSession(canvasKey).historyWidth,
  )
  const [ctxMenu, setCtxMenu] = useState<{ x: number; y: number } | null>(null)
  const [historyCtxMenu, setHistoryCtxMenu] = useState<{
    x: number
    y: number
    historyId: string
  } | null>(null)
  const [fullscreenOpen, setFullscreenOpen] = useState(false)
  const [copyFlash, setCopyFlash] = useState<string | null>(null)
  const [previewDims, setPreviewDims] = useState<{
    url: string
    w: number
    h: number
  } | null>(null)
  const [downloading, setDownloading] = useState(false)
  const stickyRef = useRef<StickySession>({})
  const applyRef = useRef(onResult)
  const optionsRef = useRef(options)
  const canvasKeyRef = useRef(canvasKey)
  const lastUserPromptRef = useRef((prompt || '').trim())
  const lastAgentPromptRef = useRef((agentPrompt || '').trim())
  const attachInputRef = useRef<HTMLInputElement | null>(null)
  const importInputRef = useRef<HTMLInputElement | null>(null)
  const inputRef = useRef<HTMLTextAreaElement | null>(null)
  const prevImageUrlRef = useRef(imageUrl)
  applyRef.current = onResult
  optionsRef.current = options
  canvasKeyRef.current = canvasKey

  useEffect(() => {
    const stream = liveStreams.get(canvasKey)
    if (stream) {
      stream.apply = (result, opts) => applyRef.current(result, opts)
    }
  }, [canvasKey, onResult])

  useEffect(() => {
    const s = readUiSession(canvasKey)
    setInput(s.input)
    setHistoryTabId(s.historyTabId)
    setUseAsRef(s.useAsRef)
    setHistoryWidth(s.historyWidth)
    setFileAttachments([])
    setCtxMenu(null)
    setHistoryCtxMenu(null)
    setFullscreenOpen(false)
    setSizeModalOpen(false)
    setPreviewDims(null)
    if ((prompt || '').trim()) lastUserPromptRef.current = prompt.trim()
    lastAgentPromptRef.current = (agentPrompt || '').trim()
    prevImageUrlRef.current = imageUrl
  }, [canvasKey])

  useEffect(() => {
    writeUiSession(canvasKey, {
      input,
      historyTabId,
      useAsRef,
      historyWidth,
    })
  }, [canvasKey, input, historyTabId, useAsRef, historyWidth])

  useEffect(() => {
    if ((prompt || '').trim()) lastUserPromptRef.current = prompt.trim()
  }, [prompt])

  useEffect(() => {
    lastAgentPromptRef.current = (agentPrompt || '').trim()
  }, [agentPrompt])

  useEffect(() => {
    if (prevImageUrlRef.current !== imageUrl && imageUrl) {
      setHistoryTabId('current')
      writeUiSession(canvasKey, { historyTabId: 'current' })
    }
    prevImageUrlRef.current = imageUrl
  }, [canvasKey, imageUrl])

  useEffect(() => {
    if (historyTabId === 'current') return
    if (!history.some((h) => h.id === historyTabId)) {
      setHistoryTabId('current')
      writeUiSession(canvasKey, { historyTabId: 'current' })
    }
  }, [canvasKey, history, historyTabId])

  const job = useSyncExternalStore(
    subscribeCanvasAgentJobs,
    () => getCanvasAgentJob(canvasKey),
    () => getCanvasAgentJob(canvasKey),
  )
  const busy = job.busy
  const status = job.status
  const error = job.error

  const onActiveChange = useCallback(
    (label: string, view?: ActiveSessionView) => {
      if (view) {
        stickyRef.current = stickyFromActive(view)
      } else if (label) {
        stickyRef.current = { ...stickyRef.current, label }
      }
    },
    [],
  )

  const patchOptions = useCallback(
    (partial: Partial<ImageStudioOptions>) => {
      onOptionsChange({ ...optionsRef.current, ...partial })
    },
    [onOptionsChange],
  )

  const activeHistoryItem = useMemo(() => {
    if (!historyTabId || historyTabId === 'current') return null
    return history.find((h) => h.id === historyTabId) || null
  }, [history, historyTabId])

  const previewUrl = useMemo(() => {
    if (activeHistoryItem?.imageUrl) return activeHistoryItem.imageUrl
    return (imageUrl || '').trim()
  }, [activeHistoryItem, imageUrl])

  const previewIsHistorical = Boolean(activeHistoryItem)

  const historyRefUrl = useMemo(() => {
    if (!useAsRef || !previewUrl) return ''
    return previewUrl
  }, [previewUrl, useAsRef])

  const generateRefs = useMemo(() => {
    const out: VisionImagePart[] = []
    if (historyRefUrl) {
      out.push({
        name: 'history-ref.png',
        media_type: mediaTypeFromDataUrl(historyRefUrl),
        data_url: historyRefUrl,
      })
    }
    for (const part of visionPartsFromAttachments(fileAttachments)) {
      if (out.some((p) => p.data_url === part.data_url)) continue
      out.push(part)
      if (out.length >= MAX_VISION_IMAGES) break
    }
    return out
  }, [fileAttachments, historyRefUrl])

  const refCount = generateRefs.length
  const fileRefSlots = Math.max(0, MAX_VISION_IMAGES - (historyRefUrl ? 1 : 0))
  const fileRefsFull =
    visionPartsFromAttachments(fileAttachments).length >= fileRefSlots

  const selectHistoryTab = useCallback(
    (id: string) => {
      if (busy) return
      setHistoryTabId(id)
    },
    [busy],
  )

  const onPickAttachments = useCallback(
    async (files: FileList | null) => {
      if (!files?.length) return
      const list = Array.from(files).filter(
        (f) =>
          (f.type || '').startsWith('image/') ||
          /\.(png|jpe?g|webp|gif|bmp|svg)$/i.test(f.name || ''),
      )
      if (!list.length) return
      const room = Math.max(
        0,
        fileRefSlots - visionPartsFromAttachments(fileAttachments).length,
      )
      const take = list.slice(0, room)
      if (!take.length) return
      const results = await Promise.all(take.map((f) => processAttachmentFile(f)))
      const imagesOnly = results.filter((r) => r.kind === 'image' && !r.error)
      setFileAttachments((prev) => {
        const merged = [...prev, ...imagesOnly]
        const seen = new Set<string>()
        const capped: ChatAttachment[] = []
        for (const a of merged) {
          if (a.kind !== 'image') continue
          const key = a.dataUrl || a.id
          if (seen.has(key)) continue
          seen.add(key)
          capped.push(a)
          if (capped.length >= fileRefSlots) break
        }
        return capped
      })
    },
    [fileAttachments, fileRefSlots],
  )

  const stop = useCallback(() => {
    const stream = liveStreams.get(canvasKey)
    stream?.abort.abort()
    liveStreams.delete(canvasKey)
    setCanvasAgentJob(canvasKey, { busy: false, status: null, error: null })
    clearCanvasAgentJob(canvasKey)
  }, [canvasKey])

  const send = useCallback(async () => {
    if (busy) return
    const refs = generateRefs
    if (!canSendWithAttachments(input, fileAttachments, []) && refs.length === 0) {
      return
    }
    if (!input.trim() && refs.length === 0) return
    const instruction = input.trim()
    if (instruction) lastUserPromptRef.current = instruction
    const jobKey = canvasKey
    const opts = { ...optionsRef.current }
    const ac = new AbortController()
    liveStreams.set(jobKey, {
      abort: ac,
      apply: (result, applyOpts) => applyRef.current(result, applyOpts),
    })
    setCanvasAgentJob(jobKey, { busy: true, status: null, error: null })
    writeUiSession(jobKey, {
      input: instruction,
      historyTabId: 'current',
    })
    let assembled = ''
    let streamError = ''
    const stickyFields = stickyRequestFields(stickyRef.current)
    const size = imageStudioSize(opts.aspect)
    const agentPrompt = buildImageStudioPrompt(
      instruction,
      refs.length,
      opts.aspect,
      size,
    )
    lastAgentPromptRef.current = agentPrompt
    try {
      await streamChat(
        {
          prompt: agentPrompt,
          generate_image: true,
          no_tools: true,
          agent_kind: 'image',
          playground_id: playgroundScope.playgroundId,
          workspace_id: contextId,
          image_size: size,
          image_temperature: opts.temperature,
          image_reasoning_effort: opts.thinking,
          image_only: opts.outputMode === 'image_only',
          images: refs.length
            ? refs.map((img) => ({
                name: img.name,
                media_type: img.media_type,
                data_url: img.data_url,
              }))
            : undefined,
          ...stickyFields,
        },
        (ev: ChatEvent) => {
          if (liveStreams.get(jobKey)?.abort !== ac) return
          if (ev.type === 'TokenDelta' && ev.delta) {
            assembled += ev.delta
          }
          if (ev.type === 'Done' && ev.final) {
            assembled = ev.final
          }
          if (ev.type === 'Error' && ev.error) {
            streamError = isImageGenUnsupported(ev.error)
              ? imageGenDegradeContent(ev.error)
              : ev.error
            setCanvasAgentJob(jobKey, { error: streamError })
          }
        },
        ac.signal,
      )
      const stream = liveStreams.get(jobKey)
      if (stream?.abort !== ac) return
      const parsed = extractImageFromMarkdown(assembled)
      if (parsed.imageUrl) {
        const shortTitle =
          instruction.length > 2 && instruction.length < 64
            ? instruction.split('\n')[0].trim()
            : undefined
        const cleanCaption =
          opts.outputMode === 'image_only'
            ? ''
            : sanitizeImageCaption(parsed.caption || '')
        stream?.apply(
          {
            prompt: instruction,
            agentPrompt,
            imageUrl: parsed.imageUrl,
            caption: cleanCaption,
            options: opts,
            title: shortTitle,
          },
          { final: true, canvasId: jobKey },
        )
        writeUiSession(jobKey, {
          input: '',
          historyTabId: 'current',
          useAsRef: false,
        })
        if (canvasKeyRef.current === jobKey) {
          setInput('')
          setHistoryTabId('current')
          setUseAsRef(false)
        }
        setCanvasAgentJob(jobKey, {
          busy: false,
          status: t('imageStudio.generated'),
          error: null,
        })
        window.setTimeout(() => {
          const cur = getCanvasAgentJob(jobKey)
          if (cur.status === t('imageStudio.generated')) {
            setCanvasAgentJob(jobKey, { status: null })
            if (!cur.busy && !cur.error) clearCanvasAgentJob(jobKey)
          }
        }, 1400)
      } else if (streamError) {
        setCanvasAgentJob(jobKey, {
          busy: false,
          status: null,
          error: streamError,
        })
      } else if (assembled.trim()) {
        setCanvasAgentJob(jobKey, {
          busy: false,
          status: null,
          error: t('imageStudio.noImageInReply'),
        })
      } else {
        setCanvasAgentJob(jobKey, { busy: false, status: null })
        clearCanvasAgentJob(jobKey)
      }
    } catch (e) {
      if ((e as Error)?.name === 'AbortError') {
        if (liveStreams.get(jobKey)?.abort === ac) {
          setCanvasAgentJob(jobKey, { busy: false, status: null, error: null })
          clearCanvasAgentJob(jobKey)
        }
        return
      }
      if (liveStreams.get(jobKey)?.abort !== ac) return
      const msg = e instanceof Error ? e.message : String(e)
      setCanvasAgentJob(jobKey, {
        busy: false,
        status: null,
        error: isImageGenUnsupported(msg)
          ? imageGenDegradeContent(msg)
          : msg,
      })
    } finally {
      if (liveStreams.get(jobKey)?.abort === ac) liveStreams.delete(jobKey)
    }
  }, [busy, canvasKey, fileAttachments, generateRefs, input, t, contextId])

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    void send()
  }

  const canSend =
    !busy &&
    (Boolean(input.trim()) || refCount > 0) &&
    canSendWithAttachments(input || ' ', fileAttachments, [])
  const chip =
    'rounded-md border px-2 py-1 text-[11px] font-medium leading-none transition disabled:opacity-40'
  const chipActive =
    'border-shell-accent bg-shell-active text-shell-text'
  const chipIdle =
    'border-shell-border bg-shell-bg text-shell-muted hover:bg-shell-hover hover:text-shell-text'
  const labelCls =
    'shrink-0 text-[11px] font-medium text-shell-muted'

  const flashStatus = useCallback(
    (message: string) => {
      setCanvasAgentJob(canvasKey, { status: message, error: null })
      window.setTimeout(() => {
        const cur = getCanvasAgentJob(canvasKey)
        if (cur.status === message) {
          setCanvasAgentJob(canvasKey, { status: null })
          if (!cur.busy && !cur.error) clearCanvasAgentJob(canvasKey)
        }
      }, 1800)
    },
    [canvasKey],
  )

  const flashError = useCallback(
    (message: string) => {
      setCanvasAgentJob(canvasKey, { error: message, status: null })
      window.setTimeout(() => {
        const cur = getCanvasAgentJob(canvasKey)
        if (cur.error === message) setCanvasAgentJob(canvasKey, { error: null })
      }, 4000)
    },
    [canvasKey],
  )

  const download = useCallback(async () => {
    if (!previewUrl || downloading) return
    setCtxMenu(null)
    setDownloading(true)
    try {
      const res = await downloadImageUrl(
        previewUrl,
        title || 'image',
        previewIsHistorical ? t('imageStudio.historyVersionSuffix') : undefined,
      )
      flashStatus(t('imageStudio.downloadSaved', { name: res.filename }))
    } catch (e) {
      flashError(
        t('imageStudio.downloadFailed', {
          reason: e instanceof Error ? e.message : String(e),
        }),
      )
    } finally {
      setDownloading(false)
    }
  }, [
    downloading,
    flashError,
    flashStatus,
    previewIsHistorical,
    previewUrl,
    t,
    title,
  ])

  const onPickImportFile = useCallback(
    async (files: FileList | null) => {
      const file = files?.[0]
      if (!file) return
      if (!/^image\//i.test(file.type)) {
        flashError(t('imageStudio.importNotImage', { name: file.name }))
        return
      }
      if (file.size > 12 * 1024 * 1024) {
        flashError(t('imageStudio.importTooLarge', { name: file.name }))
        return
      }
      try {
        const dataUrl = await new Promise<string>((resolve, reject) => {
          const fr = new FileReader()
          fr.onerror = () => reject(new Error('read failed'))
          fr.onload = () => resolve(String(fr.result || ''))
          fr.readAsDataURL(file)
        })
        if (!dataUrl.startsWith('data:image/')) {
          flashError(t('imageStudio.importNotImage', { name: file.name }))
          return
        }
        onResult(
          {
            prompt: '',
            agentPrompt: '',
            imageUrl: dataUrl,
            caption: file.name,
            options,
            title: file.name.replace(/\.[^.]+$/, ''),
          },
          { final: true },
        )
        flashStatus(t('imageStudio.imported', { name: file.name }))
      } catch (e) {
        flashError(
          t('imageStudio.importFailed', {
            reason: e instanceof Error ? e.message : String(e),
          }),
        )
      }
    },
    [flashError, flashStatus, onResult, options, t],
  )

  const copyTextToClipboard = useCallback(
    async (text: string, okLabel: string) => {
      const value = text.trim()
      if (!value) {
        flashError(t('imageStudio.promptEmpty'))
        setCtxMenu(null)
        return
      }
      let ok = false
      try {
        if (navigator.clipboard?.writeText) {
          await navigator.clipboard.writeText(value)
          ok = true
        }
      } catch {
        ok = false
      }
      if (!ok) {
        try {
          const ta = document.createElement('textarea')
          ta.value = value
          ta.setAttribute('readonly', '')
          ta.style.position = 'fixed'
          ta.style.left = '-9999px'
          document.body.appendChild(ta)
          ta.select()
          ok = document.execCommand('copy')
          document.body.removeChild(ta)
        } catch {
          ok = false
        }
      }
      if (ok) {
        setCopyFlash(okLabel)
        window.setTimeout(() => setCopyFlash(null), 1400)
      } else {
        flashError(t('imageStudio.promptCopyFailed'))
      }
      setCtxMenu(null)
    },
    [flashError, t],
  )

  const copyUserPrompt = useCallback(() => {
    const text = (
      (previewIsHistorical ? activeHistoryItem?.prompt : null) ||
      lastUserPromptRef.current ||
      prompt ||
      input ||
      ''
    ).trim()
    void copyTextToClipboard(text, t('imageStudio.userPromptCopied'))
  }, [
    activeHistoryItem,
    copyTextToClipboard,
    input,
    previewIsHistorical,
    prompt,
    t,
  ])

  const copyAgentPrompt = useCallback(() => {
    const text = (
      (previewIsHistorical ? activeHistoryItem?.agentPrompt : null) ||
      lastAgentPromptRef.current ||
      agentPrompt ||
      ''
    ).trim()
    void copyTextToClipboard(text, t('imageStudio.agentPromptCopied'))
  }, [
    activeHistoryItem,
    agentPrompt,
    copyTextToClipboard,
    previewIsHistorical,
    t,
  ])

  const restoreActiveHistory = useCallback(() => {
    const id = activeHistoryItem?.id
    if (!id || busy || !onRestoreHistory) return
    onRestoreHistory(id)
    setHistoryCtxMenu(null)
    setHistoryTabId('current')
    writeUiSession(canvasKey, { historyTabId: 'current' })
    flashStatus(t('imageStudio.historyRestored'))
  }, [
    activeHistoryItem,
    busy,
    canvasKey,
    flashStatus,
    onRestoreHistory,
    t,
  ])

  const reusePrompt = useCallback(() => {
    if (busy) return
    const text = (
      (previewIsHistorical ? activeHistoryItem?.prompt : null) ||
      lastUserPromptRef.current ||
      prompt ||
      ''
    ).trim()
    if (!text) {
      flashError(t('imageStudio.promptEmpty'))
      return
    }
    setInput(text)
    writeUiSession(canvasKey, { input: text })
    setCtxMenu(null)
    setHistoryCtxMenu(null)
    const el = inputRef.current
    if (el) {
      el.focus()
      el.setSelectionRange(text.length, text.length)
    }
    flashStatus(t('imageStudio.promptReused'))
  }, [
    activeHistoryItem,
    busy,
    canvasKey,
    flashError,
    flashStatus,
    previewIsHistorical,
    prompt,
    t,
  ])

  const previewAspect: ImageStudioAspect = previewIsHistorical
    ? activeHistoryItem?.options?.aspect || '1:1'
    : currentImageAspect

  const previewFormat = useMemo(() => {
    if (!previewUrl) return ''
    const mime = imageMimeFromDataUrl(previewUrl) || imageMimeFromUrl(previewUrl)
    return mime ? imageFormatLabel(mime) : ''
  }, [previewUrl])

  const previewVersionLabel = previewIsHistorical
    ? t('imageStudio.historyVersionOf', {
        n: String(history.findIndex((h) => h.id === activeHistoryItem?.id) + 1),
        total: String(history.length),
      })
    : t('imageStudio.historyCurrent')

  const previewTimestamp = previewIsHistorical
    ? activeHistoryItem?.createdAt
    : 0

  const reusablePrompt = (
    (previewIsHistorical ? activeHistoryItem?.prompt : null) ||
    lastUserPromptRef.current ||
    prompt ||
    ''
  ).trim()

  const previewMeta = useMemo(() => {
    const parts: string[] = [previewVersionLabel]
    const dims =
      previewDims && previewDims.url === previewUrl ? previewDims : null
    if (dims) parts.push(`${dims.w}×${dims.h}`)
    parts.push(previewAspect)
    if (previewFormat) parts.push(previewFormat)
    if (previewTimestamp) {
      parts.push(
        new Date(previewTimestamp).toLocaleString(undefined, {
          month: 'short',
          day: 'numeric',
          hour: '2-digit',
          minute: '2-digit',
        }),
      )
    }
    return parts.filter(Boolean).join(' · ')
  }, [
    previewAspect,
    previewDims,
    previewFormat,
    previewTimestamp,
    previewUrl,
    previewVersionLabel,
  ])

  const openFullscreen = useCallback(() => {
    if (!previewUrl) return
    setFullscreenOpen(true)
    setCtxMenu(null)
  }, [previewUrl])

  const closeFullscreen = useCallback(() => {
    setFullscreenOpen(false)
  }, [])

  const showHistoryRail = history.length > 0

  return (
    <div
      data-testid="image-studio"
      className={`playground-workbench flex-1 bg-shell-bg ${className}`}
    >
      <div
        className="flex h-10 shrink-0 items-center gap-2 border-b border-shell-border bg-shell-panel px-2"
        data-testid="image-studio-header"
      >
        <span
          className="min-w-0 flex-1 truncate text-[11px] font-semibold text-shell-text"
          title={title}
        >
          {title}
        </span>
        {copyFlash ? (
          <span
            data-testid="image-studio-copy-flash"
            className="text-[10px] text-shell-accent"
          >
            {copyFlash}
          </span>
        ) : null}
        {previewIsHistorical ? (
          <span
            data-testid="image-studio-viewing-history"
            className="rounded-full border border-shell-border px-2 py-0.5 text-[10px] text-shell-muted"
          >
            {t('imageStudio.viewingHistory')}
          </span>
        ) : null}
        <input
          ref={importInputRef}
          type="file"
          data-testid="image-studio-import-input"
          className="hidden"
          accept="image/png,image/jpeg,image/webp,image/gif,image/svg+xml"
          onChange={(e) => {
            void onPickImportFile(e.target.files)
            e.target.value = ''
          }}
        />
        <button
          type="button"
          data-testid="image-studio-import"
          onClick={() => importInputRef.current?.click()}
          className="inline-flex h-6 shrink-0 items-center gap-1 rounded-md border border-shell-border px-1.5 text-[10px] text-shell-muted hover:text-shell-text"
          title={t('imageStudio.importTitle')}
          aria-label={t('imageStudio.importTitle')}
        >
          <Upload size={12} strokeWidth={2} aria-hidden />
          <span>{t('imageStudio.import')}</span>
        </button>
        {previewUrl ? (
          <button
            type="button"
            data-testid="image-studio-download"
            onClick={() => void download()}
            disabled={downloading}
            aria-busy={downloading}
            className="inline-flex h-6 shrink-0 items-center gap-1 rounded-md border border-shell-border px-1.5 text-[10px] text-shell-muted hover:text-shell-text disabled:opacity-50"
            title={t('imageStudio.ctxDownload')}
          >
            {downloading ? (
              <span
                className="h-3 w-3 animate-spin rounded-full border border-shell-border border-t-shell-accent"
                aria-hidden
              />
            ) : (
              <Download size={12} strokeWidth={2} aria-hidden />
            )}
            <span>
              {downloading ? t('imageStudio.downloading') : t('action.save')}
            </span>
          </button>
        ) : null}
      </div>

      <div
        data-testid="image-studio-stage"
        className="relative flex min-h-0 flex-1 overflow-hidden"
      >
        {showHistoryRail ? (
          <>
            <aside
              data-testid="image-studio-history"
              style={{ width: `clamp(80px, ${historyWidth}px, 22cqi)` }}
              className="flex min-h-0 shrink-0 flex-col border-r border-shell-border bg-shell-panel"
            >
              <div className="shrink-0 border-b border-shell-border px-1.5 py-1">
                <span className="block truncate text-[9px] font-semibold uppercase tracking-wide text-shell-muted">
                  {t('imageStudio.history')}
                </span>
                <span className="mt-0.5 block truncate text-[9px] text-shell-muted">
                  {t('imageStudio.historySelectHint')}
                </span>
              </div>
              <ul
                className="explorer-scroll flex min-h-0 flex-1 flex-col gap-1.5 overflow-y-auto p-1"
                role="tablist"
                aria-label={t('imageStudio.history')}
              >
                {imageUrl ? (
                  <li>
                    <button
                      type="button"
                      role="tab"
                      aria-selected={historyTabId === 'current'}
                      aria-label={`${t('imageStudio.historyCurrent')} · ${currentImageAspect}`}
                      data-testid="image-studio-history-current"
                      disabled={busy}
                      title={`${t('imageStudio.historyCurrent')} · ${currentImageAspect}`}
                      onClick={() => selectHistoryTab('current')}
                      className={`block w-full min-w-0 overflow-hidden rounded-md bg-shell-bg p-0 transition disabled:opacity-40 ${
                        historyTabId === 'current'
                          ? 'border-2 border-shell-accent bg-shell-active'
                          : 'border border-shell-border opacity-90 hover:bg-shell-hover hover:opacity-100'
                      }`}
                    >
                      <span className="relative block w-full aspect-[4/3]">
                        <img
                          src={imageUrl}
                          alt=""
                          className="pointer-events-none absolute inset-0 h-full w-full object-cover"
                        />
                        <span className="pointer-events-none absolute bottom-0 left-0 right-0 truncate bg-black/55 px-1 py-px text-[8px] font-medium uppercase tracking-wide text-white/90">
                          {t('imageStudio.historyCurrent')}
                        </span>
                        {useAsRef && historyTabId === 'current' ? (
                          <span
                            data-testid="image-studio-history-ref-check"
                            className="pointer-events-none absolute right-0.5 top-0.5 flex h-4 w-4 items-center justify-center rounded-full bg-shell-accent text-white shadow"
                          >
                            <Check size={10} strokeWidth={3} aria-hidden />
                          </span>
                        ) : null}
                      </span>
                    </button>
                  </li>
                ) : null}
                {history.map((h, hIndex) => {
                  const aspect = (h.options?.aspect || '1:1') as ImageStudioAspect
                  const selected = historyTabId === h.id
                  const versionLabel = t('imageStudio.historyVersionOf', {
                    n: String(hIndex + 1),
                    total: String(history.length),
                  })
                  return (
                    <li key={h.id}>
                      <button
                        type="button"
                        onKeyDown={(e) => {
                          if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
                            e.preventDefault()
                            if (!busy) onRestoreHistory?.(h.id)
                          }
                        }}
                        role="tab"
                        aria-selected={selected}
                        aria-label={`${versionLabel} · ${aspect}${h.prompt ? ` · ${h.prompt.slice(0, 80)}` : ''}`}
                        data-testid={`image-studio-history-${h.id}`}
                        disabled={busy}
                        onClick={() => selectHistoryTab(h.id)}
                        onDoubleClick={() => {
                          if (!busy) onRestoreHistory?.(h.id)
                        }}
                        onContextMenu={(e) => {
                          e.preventDefault()
                          e.stopPropagation()
                          if (busy) return
                          selectHistoryTab(h.id)
                          setCtxMenu(null)
                          setHistoryCtxMenu({
                            x: e.clientX,
                            y: e.clientY,
                            historyId: h.id,
                          })
                        }}
                        title={
                          h.prompt
                            ? `${versionLabel} · ${aspect} · ${h.prompt.slice(0, 100)}`
                            : `${versionLabel} · ${aspect}`
                        }
                        className={`block w-full min-w-0 overflow-hidden rounded-md bg-shell-bg p-0 transition disabled:opacity-40 ${
                          selected
                            ? 'border-2 border-shell-accent bg-shell-active'
                            : 'border border-shell-border opacity-90 hover:bg-shell-hover hover:opacity-100'
                        }`}
                      >
                        <span className="relative block w-full aspect-[4/3]">
                          <img
                            src={h.imageUrl}
                            alt=""
                            className="pointer-events-none absolute inset-0 h-full w-full object-cover"
                          />
                          <span className="pointer-events-none absolute bottom-0 left-0 right-0 truncate bg-black/55 px-1 py-px text-[8px] font-medium uppercase tracking-wide text-white/90">
                            {versionLabel}
                          </span>
                          {useAsRef && selected ? (
                            <span
                              data-testid={`image-studio-history-ref-check-${h.id}`}
                              className="pointer-events-none absolute right-0.5 top-0.5 flex h-4 w-4 items-center justify-center rounded-full bg-shell-accent text-white shadow"
                            >
                              <Check size={10} strokeWidth={3} aria-hidden />
                            </span>
                          ) : null}
                        </span>
                      </button>
                    </li>
                  )
                })}
              </ul>
            </aside>
            <VerticalSplitter
              testId="image-studio-history-splitter"
              value={historyWidth}
              onChange={(w) =>
                setHistoryWidth(Math.max(72, Math.min(180, w)))
              }
              growSide="left"
              minOpposite={220}
              aria-label={t('imageStudio.historyResize')}
            />
          </>
        ) : null}

        <div className="relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
          <div
            data-testid="image-studio-preview-stage"
            className="relative flex min-h-0 flex-1 items-center justify-center overflow-hidden bg-shell-bg p-3"
          >
        {previewUrl ? (
              <img
                data-testid="image-studio-preview"
                data-history={previewIsHistorical ? 'true' : 'false'}
                src={previewUrl}
                alt={title || 'generated'}
                role="button"
                tabIndex={0}
                onLoad={(e) => {
                  const img = e.currentTarget
                  setPreviewDims({
                    url: previewUrl,
                    w: img.naturalWidth,
                    h: img.naturalHeight,
                  })
                }}
                onClick={(e) => {
                  if (e.button !== 0) return
                  openFullscreen()
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault()
                    openFullscreen()
                  }
                }}
                onContextMenu={(e) => {
                  e.preventDefault()
                  e.stopPropagation()
                  setCtxMenu({ x: e.clientX, y: e.clientY })
                }}
                className="block h-full w-full cursor-zoom-in rounded-md border border-shell-border object-contain shadow-sm outline-none focus-visible:ring-1 focus-visible:ring-shell-accent"
              />
            ) : busy ? (
              <div
                data-testid="image-studio-loading-empty"
                className="flex flex-col items-center gap-3 px-6 text-center"
                aria-live="polite"
                aria-busy="true"
              >
                <span className="relative flex h-14 w-14 items-center justify-center">
                  <span
                    className="absolute inset-0 animate-ping rounded-full bg-shell-active"
                    aria-hidden
                  />
                  <span
                    className="absolute inset-1 animate-spin rounded-full border-2 border-shell-border border-t-shell-accent"
                    aria-hidden
                  />
                  <ImageIcon
                    size={20}
                    strokeWidth={1.5}
                    className="relative text-shell-accent"
                    aria-hidden
                  />
                </span>
                <p className="text-[12px] font-medium text-shell-text">
                  {t('imageStudio.generatingTitle')}
                </p>
                <p className="max-w-xs text-[11px] leading-relaxed text-shell-muted">
                  {t('imageStudio.generatingBody', {
                    aspect: options.aspect,
                    size: imageStudioSize(options.aspect),
                  })}
                </p>
                <GeneratingLabel
                  variant="composer"
                  imageGen
                  className="!text-[10px]"
                />
              </div>
            ) : (
              <div className="flex flex-col items-center gap-2 px-6 text-center text-shell-muted">
                <span className="flex h-12 w-12 items-center justify-center rounded-lg border border-dashed border-shell-border bg-shell-panel">
                  <ImageIcon size={22} strokeWidth={1.5} aria-hidden />
                </span>
                <p className="text-[12px] font-medium text-shell-text">
                  {t('imageStudio.emptyTitle')}
                </p>
                <p className="max-w-sm text-[11px] leading-relaxed">
                  {t('imageStudio.emptyBody')}
                </p>
              </div>
            )}

            {busy && previewUrl ? (
              <div
                data-testid="image-studio-loading-overlay"
                className="absolute inset-0 z-10 flex flex-col items-center justify-center gap-2 bg-shell-panel backdrop-blur-[2px]"
                aria-live="polite"
                aria-busy="true"
              >
                <span
                  className="h-9 w-9 animate-spin rounded-full border-2 border-shell-border border-t-shell-accent"
                  aria-hidden
                />
                <GeneratingLabel
                  variant="composer"
                  imageGen
                  hideSpinner
                  className="!text-[11px]"
                />
                <p className="text-[10px] text-shell-muted">
                  {options.aspect} · {imageStudioSize(options.aspect)}
                </p>
              </div>
            ) : null}
          </div>
          {previewUrl ? (
            <div
              data-testid="image-studio-preview-details"
              className="flex min-h-9 shrink-0 flex-wrap items-center gap-x-2 gap-y-1 border-t border-shell-border bg-shell-panel px-2 py-1.5"
            >
              <span
                className="min-w-0 flex-1 truncate text-[10px] tabular-nums text-shell-muted"
                title={previewMeta}
              >
                {previewMeta}
              </span>
              <button
                type="button"
                data-testid="image-studio-reuse-prompt"
                disabled={busy || !reusablePrompt}
                onClick={reusePrompt}
                className="inline-flex h-6 shrink-0 items-center gap-1 rounded-md border border-shell-border bg-shell-bg px-1.5 text-[10px] text-shell-muted hover:bg-shell-hover hover:text-shell-text disabled:opacity-40"
                title={t('imageStudio.reusePromptTitle')}
              >
                <RotateCcw size={11} strokeWidth={2} aria-hidden />
                {t('imageStudio.reusePrompt')}
              </button>
              {previewIsHistorical && onRestoreHistory ? (
                <button
                  type="button"
                  data-testid="image-studio-history-restore"
                  disabled={busy}
                  onClick={restoreActiveHistory}
                  className="inline-flex h-6 shrink-0 items-center gap-1 rounded-md border border-shell-accent bg-shell-active px-1.5 text-[10px] font-medium text-shell-text hover:brightness-110 disabled:opacity-40"
                  title={t('imageStudio.historyRestore')}
                >
                  <CornerUpLeft size={11} strokeWidth={2} aria-hidden />
                  {t('imageStudio.historyRestore')}
                </button>
              ) : null}
            </div>
          ) : null}
        </div>
      </div>

      {sizeModalOpen
        ? createPortal(
            <ImageSizePickerModal
              value={options.aspect}
              onSelect={(id) => {
                patchOptions({ aspect: id })
                setSizeModalOpen(false)
              }}
              onClose={() => setSizeModalOpen(false)}
              title={t('imageStudio.sizePickerTitle')}
              groupLabel={(g) => t(SIZE_GROUP_LABEL_KEY[g])}
            />,
            document.body,
          )
        : null}

      {ctxMenu && previewUrl
        ? createPortal(
            <ImageResultContextMenu
              x={ctxMenu.x}
              y={ctxMenu.y}
              onReusePrompt={reusePrompt}
              previewLabel={t('imageStudio.ctxPreview')}
              downloadLabel={t('imageStudio.ctxDownload')}
              reusePromptLabel={t('imageStudio.reusePrompt')}
              copyUserPromptLabel={t('imageStudio.ctxCopyUserPrompt')}
              copyAgentPromptLabel={t('imageStudio.ctxCopyAgentPrompt')}
              onClose={() => setCtxMenu(null)}
              onPreview={openFullscreen}
              onDownload={() => void download()}
              onCopyUserPrompt={copyUserPrompt}
              onCopyAgentPrompt={copyAgentPrompt}
            />,
            document.body,
          )
        : null}

      {historyCtxMenu
        ? createPortal(
            <ImageHistoryContextMenu
              x={historyCtxMenu.x}
              y={historyCtxMenu.y}
              previewLabel={t('imageStudio.historyCtxPreview')}
              restoreLabel={t('imageStudio.historyCtxRestore')}
              deleteLabel={t('imageStudio.historyCtxDelete')}
              canRestore={Boolean(onRestoreHistory)}
              canDelete={Boolean(onDeleteHistory)}
              onClose={() => setHistoryCtxMenu(null)}
              onPreview={() => {
                selectHistoryTab(historyCtxMenu.historyId)
                setHistoryCtxMenu(null)
              }}
              onRestore={() => {
                onRestoreHistory?.(historyCtxMenu.historyId)
                setHistoryCtxMenu(null)
              }}
              onDelete={() => {
                onDeleteHistory?.(historyCtxMenu.historyId)
                setHistoryCtxMenu(null)
              }}
            />,
            document.body,
          )
        : null}

      {fullscreenOpen && previewUrl
        ? createPortal(
            <ImageFullscreenDialog
              src={previewUrl}
              title={title || 'generated'}
              meta={previewMeta}
              closeLabel={t('action.close')}
              downloadLabel={t('imageStudio.ctxDownload')}
              reuseLabel={t('imageStudio.reusePrompt')}
              restoreLabel={t('imageStudio.historyRestore')}
              canRestore={previewIsHistorical && Boolean(onRestoreHistory)}
              downloading={downloading}
              onClose={closeFullscreen}
              onDownload={() => void download()}
              onReusePrompt={() => {
                closeFullscreen()
                window.setTimeout(reusePrompt, 0)
              }}
              onRestore={() => {
                closeFullscreen()
                restoreActiveHistory()
              }}
            />,
            document.body,
          )
        : null}

      <div
        data-testid="image-studio-agent-dock"
        className="playground-agent-dock"
      >
        {(error || (status && !busy)) && (
          <div
            className="flex min-h-[22px] items-start justify-end gap-2 px-3 pt-1.5 text-[11px]"
            role="status"
            aria-live="polite"
          >
            {error ? (
              <span
                data-testid="image-studio-error"
                className="max-w-full break-words text-right text-red-400"
                title={error}
              >
                {error}
              </span>
            ) : status ? (
              <span
                data-testid="image-studio-status"
                className="max-w-full break-words text-right text-shell-accent"
              >
                {status}
              </span>
            ) : null}
          </div>
        )}

        <div
          data-testid="image-studio-options"
          className="flex flex-wrap items-center gap-x-3 gap-y-2 px-3 py-2"
        >
          <div className="flex min-w-0 flex-wrap items-center gap-1.5">
            <span className={labelCls}>{t('imageStudio.output')}</span>
            {(
              [
                ['image_text', t('imageStudio.outputImageText')],
                ['image_only', t('imageStudio.outputImageOnly')],
              ] as Array<[ImageStudioOutputMode, string]>
            ).map(([id, label]) => {
              const active = options.outputMode === id
              return (
                <button
                  key={id}
                  type="button"
                  data-testid={`image-studio-output-${id}`}
                  disabled={busy}
                  aria-pressed={active}
                  onClick={() => patchOptions({ outputMode: id })}
                  className={`${chip} ${active ? chipActive : chipIdle}`}
                >
                  {label}
                </button>
              )
            })}
          </div>

          <div className="flex min-w-0 items-center gap-1.5">
            <label className={labelCls} htmlFor="image-studio-temperature">
              {t('imageStudio.temperature')}
            </label>
            <input
              id="image-studio-temperature"
              data-testid="image-studio-temperature"
              type="range"
              min={0}
              max={2}
              step={0.1}
              disabled={busy}
              value={options.temperature}
              onChange={(e) =>
                patchOptions({ temperature: Number(e.target.value) })
              }
              className="h-1 w-20 accent-[var(--shell-accent,#6d9eff)]"
            />
            <span
              data-testid="image-studio-temperature-value"
              className="min-w-[1.75rem] text-[11px] tabular-nums text-shell-text"
            >
              {options.temperature.toFixed(1)}
            </span>
          </div>

          <div className="flex min-w-0 items-center gap-1.5">
            <span className={labelCls}>{t('imageStudio.size')}</span>
            <button
              type="button"
              data-testid="image-studio-size-select"
              disabled={busy}
              onClick={() => setSizeModalOpen(true)}
              className="inline-flex h-7 max-w-[11rem] items-center gap-1 rounded-md border border-shell-border bg-shell-bg px-2 text-[12px] font-medium text-shell-text hover:bg-shell-hover disabled:opacity-40"
              title={t('imageStudio.sizePickerTitle')}
            >
              <span className="truncate">
                {imageStudioSizeSelectLabel(options.aspect)}
              </span>
              <ChevronDown size={12} className="shrink-0 text-shell-muted" />
            </button>
          </div>

          <div className="flex min-w-0 flex-wrap items-center gap-1.5">
            <span className={labelCls}>{t('imageStudio.thinking')}</span>
            {THINKING.map((level) => {
              const active = options.thinking === level
              return (
                <button
                  key={level}
                  type="button"
                  data-testid={`image-studio-thinking-${level}`}
                  disabled={busy}
                  aria-pressed={active}
                  onClick={() => patchOptions({ thinking: level })}
                  className={`${chip} ${active ? chipActive : chipIdle}`}
                >
                  {t(
                    level === 'low'
                      ? 'imageStudio.thinkingLow'
                      : level === 'medium'
                        ? 'imageStudio.thinkingMedium'
                        : 'imageStudio.thinkingHigh',
                  )}
                </button>
              )
            })}
          </div>

          <label
            data-testid="image-studio-use-as-ref"
            className={`inline-flex cursor-pointer select-none items-center gap-1.5 rounded-md px-1 py-0.5 text-[11px] font-medium transition ${
              useAsRef
                ? 'bg-shell-active text-shell-text'
                : 'text-shell-muted hover:bg-shell-hover hover:text-shell-text'
            } ${busy || !previewUrl ? 'opacity-40' : ''}`}
            title={t('imageStudio.useAsRefTitle')}
          >
            <input
              type="checkbox"
              className="sr-only"
              checked={useAsRef}
              disabled={busy || !previewUrl}
              onChange={(e) => setUseAsRef(e.target.checked)}
            />
            <span
              className={`flex h-4 w-4 shrink-0 items-center justify-center rounded border ${
                useAsRef
                  ? 'border-shell-accent bg-shell-accent text-white'
                  : 'border-shell-border bg-shell-bg'
              }`}
              aria-hidden
            >
              {useAsRef ? <Check size={10} strokeWidth={3} /> : null}
            </span>
            {t('imageStudio.useAsRef')}
          </label>
        </div>

        <form
          data-testid="image-studio-form"
          onSubmit={onSubmit}
          className="playground-agent-form relative border-t border-shell-border"
        >
          <textarea
            ref={inputRef}
            data-testid="image-studio-input"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                void send()
              }
            }}
            rows={2}
            disabled={busy}
            placeholder={
              locale === 'id'
                ? useAsRef
                  ? 'Jelaskan apa yang diubah / dipertahankan dari gambar referensi… cth. Pertahankan wajah & pose; ganti baju jadi jas hitam; latar kota malam hujan; lighting neon lembut'
                  : PROMPT_PLACEHOLDER_ID
                : useAsRef
                  ? 'Describe what to change / keep from the reference image(s)… e.g. Keep face & pose; change jacket to black suit; rainy night city background; soft neon lighting'
                  : PROMPT_PLACEHOLDER_EN
            }
            className="playground-composer-input disabled:opacity-60"
          />
          <div
            data-testid="image-studio-toolbar"
            className="mt-1.5 flex flex-wrap items-center justify-between gap-2"
          >
            <div className="flex min-w-0 flex-wrap items-center gap-1.5">
              <input
                ref={attachInputRef}
                type="file"
                multiple
                data-testid="image-studio-attach-input"
                className="hidden"
                accept="image/png,image/jpeg,image/webp,image/gif,image/*"
                onChange={(e) => {
                  void onPickAttachments(e.target.files)
                  e.target.value = ''
                }}
              />
              <button
                type="button"
                data-testid="image-studio-attach"
                disabled={busy || fileRefsFull}
                title={
                  fileRefsFull
                    ? t('imageStudio.refFull')
                    : t('imageStudio.attachRefs')
                }
                aria-label={t('imageStudio.attachRefs')}
                onClick={() => attachInputRef.current?.click()}
                className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-shell-border bg-shell-bg text-shell-muted hover:bg-shell-hover hover:text-shell-text disabled:opacity-40"
              >
                <Paperclip className="h-3.5 w-3.5" strokeWidth={2} />
              </button>
              {fileAttachments.length > 0 ? (
                <span className="text-[10px] text-shell-muted">
                  +{fileAttachments.length} file
                </span>
              ) : null}
              <ModelPicker
                disabled={modelPickerDisabledWhileStreaming(busy)}
                onActiveChange={onActiveChange}
              />
            </div>
            <div className="flex shrink-0 items-center gap-1.5">
              {busy && (
                <>
                  <GeneratingLabel
                    variant="composer"
                    className="!text-[10px]"
                  />
                  <button
                    type="button"
                    data-testid="image-studio-stop"
                    onClick={stop}
                    className="rounded-md border border-shell-border bg-shell-bg px-2 py-1 text-[11px] text-shell-muted hover:bg-shell-hover hover:text-shell-text"
                    title={t('action.stop')}
                  >
                    {t('action.stop')}
                  </button>
                </>
              )}
              <button
                type="submit"
                data-testid="image-studio-generate"
                disabled={!canSend || busy}
                title={t('action.generate')}
                className="shell-primary-button rounded-md px-3 py-1.5 text-[11px] font-semibold disabled:opacity-40"
              >
                {t('action.generate')}
              </button>
            </div>
          </div>
        </form>
      </div>
    </div>
  )
}

function ImageFullscreenDialog({
  src,
  title,
  meta,
  closeLabel,
  downloadLabel,
  reuseLabel,
  restoreLabel,
  canRestore,
  downloading,
  onClose,
  onDownload,
  onReusePrompt,
  onRestore,
}: {
  src: string
  title: string
  meta: string
  closeLabel: string
  downloadLabel: string
  reuseLabel: string
  restoreLabel: string
  canRestore: boolean
  downloading: boolean
  onClose: () => void
  onDownload: () => void
  onReusePrompt: () => void
  onRestore: () => void
}) {
  const dialogRef = useRef<HTMLDivElement | null>(null)
  const closeRef = useRef<HTMLButtonElement | null>(null)

  useEffect(() => {
    const opener = document.activeElement
    closeRef.current?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        onClose()
        return
      }
      if (e.key !== 'Tab') return
      const controls = dialogRef.current?.querySelectorAll<HTMLElement>(
        'button:not(:disabled), [href], [tabindex]:not([tabindex="-1"])',
      )
      if (!controls?.length) {
        e.preventDefault()
        dialogRef.current?.focus()
        return
      }
      const first = controls[0]
      const last = controls[controls.length - 1]
      if (
        e.shiftKey &&
        (document.activeElement === first || document.activeElement === dialogRef.current)
      ) {
        e.preventDefault()
        last.focus()
      } else if (
        !e.shiftKey &&
        (document.activeElement === last || document.activeElement === dialogRef.current)
      ) {
        e.preventDefault()
        first.focus()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
      if (opener instanceof HTMLElement && opener.isConnected) opener.focus()
    }
  }, [onClose])

  return (
    <div
      ref={dialogRef}
      data-testid="image-studio-fullscreen"
      className="fixed inset-0 z-[230] flex flex-col bg-black/90 outline-none"
      role="dialog"
      aria-modal="true"
      aria-labelledby="image-studio-fullscreen-title"
      aria-describedby="image-studio-fullscreen-meta"
      tabIndex={-1}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div className="flex min-h-11 shrink-0 flex-wrap items-center gap-2 border-b border-white/10 px-3 py-1.5">
        <span
          id="image-studio-fullscreen-title"
          className="min-w-0 flex-1 truncate text-[12px] font-medium text-white/90"
        >
          {title}
        </span>
        <button
          type="button"
          data-testid="image-studio-fullscreen-reuse-prompt"
          onClick={onReusePrompt}
          className="inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[11px] text-white/75 hover:bg-white/10 hover:text-white"
        >
          <RotateCcw size={13} strokeWidth={2} aria-hidden />
          {reuseLabel}
        </button>
        {canRestore ? (
          <button
            type="button"
            data-testid="image-studio-fullscreen-restore"
            onClick={onRestore}
            className="inline-flex h-7 items-center gap-1.5 rounded-md bg-white/15 px-2 text-[11px] font-medium text-white hover:bg-white/20"
          >
            <CornerUpLeft size={13} strokeWidth={2} aria-hidden />
            {restoreLabel}
          </button>
        ) : null}
        <button
          type="button"
          data-testid="image-studio-fullscreen-download"
          disabled={downloading}
          onClick={onDownload}
          className="inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[11px] text-white/75 hover:bg-white/10 hover:text-white disabled:opacity-50"
        >
          <Download size={13} strokeWidth={2} aria-hidden />
          {downloadLabel}
        </button>
        <button
          ref={closeRef}
          type="button"
          data-testid="image-studio-fullscreen-close"
          onClick={onClose}
          className="inline-flex h-7 w-7 items-center justify-center rounded-md text-white/80 hover:bg-white/10 hover:text-white focus-visible:ring-2 focus-visible:ring-white/70"
          title={closeLabel}
          aria-label={closeLabel}
        >
          <X size={16} strokeWidth={2} aria-hidden />
        </button>
      </div>
      <div className="pointer-events-none flex min-h-0 flex-1 items-center justify-center p-4">
        <img
          src={src}
          alt={title}
          className="pointer-events-auto max-h-full max-w-full object-contain"
          onClick={(e) => e.stopPropagation()}
        />
      </div>
      <div
        id="image-studio-fullscreen-meta"
        className="flex min-h-8 shrink-0 items-center justify-center gap-1.5 border-t border-white/10 px-3 text-[10px] tabular-nums text-white/60"
      >
        <History size={11} strokeWidth={1.8} aria-hidden />
        {meta}
      </div>
    </div>
  )
}

function SizeGlyph({ option }: { option: ImageStudioSizeOption }) {
  return (
    <span

      className="rounded-[3px] border-2 border-current opacity-80"
      style={{ width: option.iconW, height: option.iconH }}
      aria-hidden
    />
  )
}

function ImageSizePickerModal({
  value,
  onSelect,
  onClose,
  title,
  groupLabel,
}: {
  value: ImageStudioAspect
  onSelect: (id: ImageStudioAspect) => void
  onClose: () => void
  title: string
  groupLabel: (g: ImageStudioSizeGroup) => string
}) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <div
      data-testid="image-studio-size-modal-backdrop"
      className="fixed inset-0 z-[220] flex items-center justify-center bg-black/50 p-4"
      role="presentation"
      onClick={onClose}
    >
      <div
        data-testid="image-studio-size-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="image-studio-size-modal-title"
        className="flex max-h-[min(32rem,90vh)] w-full max-w-md flex-col overflow-hidden rounded-md border border-shell-border bg-shell-panel shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between gap-2 border-b border-shell-border px-4 py-3">
          <h2
            id="image-studio-size-modal-title"
            className="text-sm font-medium text-shell-text"
          >
            {title}
          </h2>
          <button
            type="button"
            data-testid="image-studio-size-modal-close"
            onClick={onClose}
            className="inline-flex h-7 w-7 items-center justify-center rounded-md text-shell-muted hover:bg-shell-hover hover:text-shell-text"
            aria-label="Close"
          >
            <X size={14} strokeWidth={2} />
          </button>
        </div>
        <div className="shell-scroll space-y-4 overflow-y-auto px-4 py-3">
          {IMAGE_STUDIO_SIZE_GROUPS.map((group) => {
            const opts = IMAGE_STUDIO_SIZE_OPTIONS.filter((o) => o.group === group)
            if (!opts.length) return null
            return (
              <div key={group}>
                <p className="mb-1.5 text-[10px] font-semibold uppercase tracking-wide text-shell-muted">
                  {groupLabel(group)}
                </p>
                <div className="grid grid-cols-4 gap-1.5">
                  {opts.map((opt) => {
                    const active = value === opt.id
                    return (
                      <button
                        key={opt.id}
                        type="button"
                        data-testid={`image-studio-size-${opt.id}`}
                        data-active={active ? 'true' : 'false'}
                        onClick={() => onSelect(opt.id)}
                        className={`flex flex-col items-center gap-1.5 rounded-md border px-1.5 py-2 text-[10px] font-medium transition ${
                          active
                            ? 'border-shell-accent bg-shell-active text-shell-text'
                            : 'border-shell-border text-shell-muted hover:bg-shell-hover hover:text-shell-text'
                        }`}
                        title={`${opt.label} · ${opt.size}`}
                      >
                        <SizeGlyph option={opt} />
                        <span className="truncate">{opt.label}</span>
                      </button>
                    )
                  })}
                </div>
              </div>
            )
          })}
        </div>
        <div className="border-t border-shell-border px-4 py-2 text-[10px] text-shell-muted">
          {getImageStudioSizeOption(value).label} ·{' '}
          {getImageStudioSizeOption(value).size}
        </div>
      </div>
    </div>
  )
}

function ImageHistoryContextMenu({
  x,
  y,
  previewLabel,
  restoreLabel,
  deleteLabel,
  canRestore,
  canDelete,
  onClose,
  onPreview,
  onRestore,
  onDelete,
}: {
  x: number
  y: number
  previewLabel: string
  restoreLabel: string
  deleteLabel: string
  canRestore: boolean
  canDelete: boolean
  onClose: () => void
  onPreview: () => void
  onRestore: () => void
  onDelete: () => void
}) {
  const ref = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose()
    }
    window.addEventListener('keydown', onKey)
    window.addEventListener('mousedown', onDown)
    return () => {
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('mousedown', onDown)
    }
  }, [onClose])

  const left = Math.min(
    x,
    Math.max(8, (typeof window !== 'undefined' ? window.innerWidth : x) - 200),
  )
  const top = Math.min(
    y,
    Math.max(8, (typeof window !== 'undefined' ? window.innerHeight : y) - 140),
  )
  const itemCls =
    'flex w-full items-center gap-2 px-2.5 py-1.5 text-left text-[11px] text-shell-text hover:bg-shell-hover'
  const dangerCls =
    'flex w-full items-center gap-2 px-2.5 py-1.5 text-left text-[11px] text-red-400 hover:bg-shell-hover'

  return (
    <div
      ref={ref}
      data-testid="image-studio-history-context-menu"
      role="menu"
      className="fixed z-[220] min-w-[11rem] rounded-md border border-shell-border bg-shell-panel py-0.5 shadow-lg shadow-black/40"
      style={{ left, top }}
    >
      <button
        type="button"
        role="menuitem"
        data-testid="image-studio-history-ctx-preview"
        className={itemCls}
        onClick={onPreview}
      >
        <Expand size={12} strokeWidth={2} aria-hidden />
        {previewLabel}
      </button>
      {canRestore ? (
        <button
          type="button"
          role="menuitem"
          data-testid="image-studio-history-ctx-restore"
          className={itemCls}
          onClick={onRestore}
        >
          <ImageIcon size={12} strokeWidth={2} aria-hidden />
          {restoreLabel}
        </button>
      ) : null}
      {canDelete ? (
        <>
          <div role="separator" className="my-0.5 border-t border-shell-border" />
          <button
            type="button"
            role="menuitem"
            data-testid="image-studio-history-ctx-delete"
            className={dangerCls}
            onClick={onDelete}
          >
            <Trash2 size={12} strokeWidth={2} aria-hidden />
            {deleteLabel}
          </button>
        </>
      ) : null}
    </div>
  )
}

function ImageResultContextMenu({
  x,
  y,
  previewLabel,
  downloadLabel,
  reusePromptLabel,
  copyUserPromptLabel,
  copyAgentPromptLabel,
  onClose,
  onPreview,
  onDownload,
  onReusePrompt,
  onCopyUserPrompt,
  onCopyAgentPrompt,
}: {
  x: number
  y: number
  previewLabel: string
  downloadLabel: string
  reusePromptLabel: string
  copyUserPromptLabel: string
  copyAgentPromptLabel: string
  onClose: () => void
  onPreview: () => void
  onDownload: () => void
  onReusePrompt: () => void
  onCopyUserPrompt: () => void
  onCopyAgentPrompt: () => void
}) {
  const ref = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    const onDown = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) onClose()
    }
    window.addEventListener('keydown', onKey)
    window.addEventListener('mousedown', onDown)
    return () => {
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('mousedown', onDown)
    }
  }, [onClose])

  const left = Math.min(
    x,
    Math.max(8, (typeof window !== 'undefined' ? window.innerWidth : x) - 200),
  )
  const top = Math.min(
    y,
    Math.max(8, (typeof window !== 'undefined' ? window.innerHeight : y) - 180),
  )
  const itemCls =
    'flex w-full items-center gap-2 px-2.5 py-1.5 text-left text-[11px] text-shell-text hover:bg-shell-hover'

  return (
    <div
      ref={ref}
      data-testid="image-studio-context-menu"
      role="menu"
      className="fixed z-[220] min-w-[12rem] rounded-md border border-shell-border bg-shell-panel py-0.5 shadow-lg shadow-black/40"
      style={{ left, top }}
    >
      <button
        type="button"
        role="menuitem"
        data-testid="image-studio-ctx-preview"
        className={itemCls}
        onClick={onPreview}
      >
        <Expand size={12} strokeWidth={2} aria-hidden />
        {previewLabel}
      </button>
      <button
        type="button"
        role="menuitem"
        data-testid="image-studio-ctx-download"
        className={itemCls}
        onClick={onDownload}
      >
        <Download size={12} strokeWidth={2} aria-hidden />
        {downloadLabel}
      </button>
      <button
        type="button"
        role="menuitem"
        data-testid="image-studio-ctx-reuse-prompt"
        className={itemCls}
        onClick={onReusePrompt}
      >
        <RotateCcw size={12} strokeWidth={2} aria-hidden />
        {reusePromptLabel}
      </button>
      <div role="separator" className="my-0.5 border-t border-shell-border" />
      <button
        type="button"
        role="menuitem"
        data-testid="image-studio-ctx-copy-user-prompt"
        className={itemCls}
        onClick={onCopyUserPrompt}
      >
        <ClipboardCopy size={12} strokeWidth={2} aria-hidden />
        {copyUserPromptLabel}
      </button>
      <button
        type="button"
        role="menuitem"
        data-testid="image-studio-ctx-copy-agent-prompt"
        className={itemCls}
        onClick={onCopyAgentPrompt}
      >
        <ClipboardCopy size={12} strokeWidth={2} aria-hidden />
        {copyAgentPromptLabel}
      </button>
    </div>
  )
}

export { DEFAULT_IMAGE_STUDIO_OPTIONS }
