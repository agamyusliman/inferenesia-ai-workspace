export type GalleryCanvasKind =
  | 'html'
  | 'diagram'
  | 'image'
  | 'excalidraw'
  | 'slides'
  | 'markdown'
  | 'table'
  | 'timeline'

export type GalleryCanvasItem = {
  id: string
  kind: GalleryCanvasKind
  title: string
  sessionId: string
  sessionName: string
  updatedAt: number
  createdAt: number
  html?: string
  source?: string
  path?: string
  chatMessageId?: string
  prompt?: string
  agentPrompt?: string
  imageUrl?: string
  caption?: string
  imageOptions?: ImageStudioOptions
  imageHistory?: ImageStudioHistoryItem[]
}

export const GALLERY_KIND_LABEL: Record<GalleryCanvasKind, string> = {
  html: 'HTML',
  diagram: 'Diagram',
  image: 'Image',
  excalidraw: 'Excalidraw',
  slides: 'Slides',
  markdown: 'Markdown',
  table: 'Table',
  timeline: 'Timeline',
}

export const GALLERY_KIND_FILTERS: Array<GalleryCanvasKind | 'all'> = [
  'all',
  'html',
  'diagram',
  'image',
  'excalidraw',
  'slides',
  'markdown',
  'table',
  'timeline',
]

export type ImageStudioOutputMode = 'image_text' | 'image_only'
export type ImageStudioThinking = 'low' | 'medium' | 'high'

export type ImageStudioAspect =
  | '1:1'
  | '5:4'
  | '4:3'
  | '3:2'
  | '16:9'
  | '21:9'
  | '4:5'
  | '3:4'
  | '2:3'
  | '9:16'
  | 'a4-portrait'
  | 'a4-landscape'
  | 'a3-portrait'
  | 'a3-landscape'
  | 'letter-portrait'
  | 'letter-landscape'
  | 'legal-portrait'
  | 'legal-landscape'
  | 'tabloid-portrait'
  | 'tabloid-landscape'

export type ImageStudioSizeGroup =
  | 'landscape'
  | 'portrait'
  | 'square'
  | 'paper'

export type ImageStudioSizeOption = {
  id: ImageStudioAspect
  group: ImageStudioSizeGroup
  label: string
  css: string
  size: string
  iconW: number
  iconH: number
}

export const IMAGE_STUDIO_SIZE_OPTIONS: ImageStudioSizeOption[] = [
  { id: '4:3', group: 'landscape', label: '4:3', css: '4 / 3', size: '2048x1536', iconW: 20, iconH: 15 },
  { id: '3:2', group: 'landscape', label: '3:2', css: '3 / 2', size: '2160x1440', iconW: 21, iconH: 14 },
  { id: '16:9', group: 'landscape', label: '16:9', css: '16 / 9', size: '2560x1440', iconW: 24, iconH: 13.5 },
  { id: '21:9', group: 'landscape', label: '21:9', css: '21 / 9', size: '2520x1080', iconW: 26, iconH: 11 },
  { id: '3:4', group: 'portrait', label: '3:4', css: '3 / 4', size: '1536x2048', iconW: 15, iconH: 20 },
  { id: '2:3', group: 'portrait', label: '2:3', css: '2 / 3', size: '1440x2160', iconW: 14, iconH: 21 },
  { id: '9:16', group: 'portrait', label: '9:16', css: '9 / 16', size: '1440x2560', iconW: 13.5, iconH: 24 },
  { id: '4:5', group: 'portrait', label: '4:5', css: '4 / 5', size: '1638x2048', iconW: 16, iconH: 20 },
  { id: '1:1', group: 'square', label: '1:1', css: '1 / 1', size: '2048x2048', iconW: 18, iconH: 18 },
  { id: '5:4', group: 'square', label: '5:4', css: '5 / 4', size: '2048x1638', iconW: 20, iconH: 16 },
  { id: 'a4-portrait', group: 'paper', label: 'A4', css: '210 / 297', size: '1748x2480', iconW: 14, iconH: 20 },
  { id: 'a4-landscape', group: 'paper', label: 'A4 ↔', css: '297 / 210', size: '2480x1748', iconW: 20, iconH: 14 },
  { id: 'a3-portrait', group: 'paper', label: 'A3', css: '297 / 420', size: '1754x2480', iconW: 14, iconH: 20 },
  { id: 'a3-landscape', group: 'paper', label: 'A3 ↔', css: '420 / 297', size: '2480x1754', iconW: 20, iconH: 14 },
  { id: 'letter-portrait', group: 'paper', label: 'Letter', css: '8.5 / 11', size: '1700x2200', iconW: 15, iconH: 19 },
  { id: 'letter-landscape', group: 'paper', label: 'Letter ↔', css: '11 / 8.5', size: '2200x1700', iconW: 19, iconH: 15 },
  { id: 'legal-portrait', group: 'paper', label: 'Legal', css: '8.5 / 14', size: '1400x2310', iconW: 14, iconH: 23 },
  { id: 'legal-landscape', group: 'paper', label: 'Legal ↔', css: '14 / 8.5', size: '2310x1400', iconW: 23, iconH: 14 },
  { id: 'tabloid-portrait', group: 'paper', label: 'Tabloid', css: '11 / 17', size: '1600x2473', iconW: 14, iconH: 22 },
  { id: 'tabloid-landscape', group: 'paper', label: 'Tabloid ↔', css: '17 / 11', size: '2473x1600', iconW: 22, iconH: 14 },
]

export const IMAGE_STUDIO_SIZE_GROUPS: ImageStudioSizeGroup[] = [
  'landscape',
  'portrait',
  'square',
  'paper',
]

const SIZE_BY_ID = new Map(
  IMAGE_STUDIO_SIZE_OPTIONS.map((o) => [o.id, o] as const),
)

export type ImageStudioOptions = {
  outputMode: ImageStudioOutputMode
  temperature: number
  aspect: ImageStudioAspect
  thinking: ImageStudioThinking
}

export type ImageStudioHistoryItem = {
  id: string
  imageUrl: string
  prompt: string
  agentPrompt?: string
  caption: string
  createdAt: number
  options?: ImageStudioOptions
}

export const DEFAULT_IMAGE_STUDIO_OPTIONS: ImageStudioOptions = {
  outputMode: 'image_text',
  temperature: 0.8,
  aspect: '1:1',
  thinking: 'high',
}

export const IMAGE_STUDIO_HISTORY_MAX = 12

export function isImageStudioAspect(v: unknown): v is ImageStudioAspect {
  return typeof v === 'string' && SIZE_BY_ID.has(v as ImageStudioAspect)
}

export function getImageStudioSizeOption(
  aspect?: ImageStudioAspect | string,
): ImageStudioSizeOption {
  if (aspect && SIZE_BY_ID.has(aspect as ImageStudioAspect)) {
    return SIZE_BY_ID.get(aspect as ImageStudioAspect)!
  }
  return SIZE_BY_ID.get('1:1')!
}

export function imageStudioAspectCss(
  aspect?: ImageStudioAspect | string,
): string {
  return getImageStudioSizeOption(aspect).css
}

export function imageStudioSize(aspect: ImageStudioAspect | string): string {
  return getImageStudioSizeOption(aspect).size
}

export function imageStudioSizeGroupLabel(
  group: ImageStudioSizeGroup,
): string {
  switch (group) {
    case 'landscape':
      return 'Landscape'
    case 'portrait':
      return 'Portrait'
    case 'square':
      return 'Square'
    case 'paper':
      return 'Paper'
    default:
      return 'Size'
  }
}

export function imageStudioSizeLabel(
  aspect?: ImageStudioAspect | string,
): string {
  return getImageStudioSizeOption(aspect).label
}

export function imageStudioSizeSelectLabel(
  aspect?: ImageStudioAspect | string,
): string {
  const opt = getImageStudioSizeOption(aspect)
  return `${imageStudioSizeGroupLabel(opt.group)} - ${opt.label}`
}
