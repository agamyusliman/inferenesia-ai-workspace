export const EXCALIDRAW_SOURCE = 'inferenesia'
export const EXCALIDRAW_VERSION = 2
export const EXCALIDRAW_TYPE = 'excalidraw'

export type ExcalidrawDocumentData = {
  type: string
  version: number
  source: string
  elements: unknown[]
  appState: Record<string, unknown>
  files: Record<string, unknown>
}

export type ExcalidrawParseResult =
  | { ok: true; data: ExcalidrawDocumentData; warning?: string }
  | { ok: false; error: string; data: ExcalidrawDocumentData }

export function isExcalidrawPath(path: string): boolean {
  const base = (path.split(/[/\\]/).pop() || path).toLowerCase()
  return base.endsWith('.excalidraw')
}

export function emptyExcalidrawDocument(): ExcalidrawDocumentData {
  return {
    type: EXCALIDRAW_TYPE,
    version: EXCALIDRAW_VERSION,
    source: EXCALIDRAW_SOURCE,
    elements: [],
    appState: {},
    files: {},
  }
}

const VOLATILE_APP_STATE_KEYS = new Set([
  'collaborators',
  'currentItemFontFamily',
  'cursorButton',
  'editingElement',
  'editingGroupId',
  'editingLinearElement',
  'isBindingEnabled',
  'isLoading',
  'multiElement',
  'newElement',
  'openDialog',
  'openMenu',
  'openPopup',
  'openSidebar',
  'pendingImageElementId',
  'previousSelectedElementIds',
  'resizingElement',
  'selectedElementIds',
  'selectedGroupIds',
  'selectedLinearElement',
  'selectionElement',
  'shouldCacheIgnoreZoom',
  'suggestedBindings',
  'toast',
  'width',
  'height',
  'offsetLeft',
  'offsetTop',
])

export function stripVolatileAppState(
  appState: Record<string, unknown> | null | undefined,
): Record<string, unknown> {
  if (!appState || typeof appState !== 'object') return {}
  const out: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(appState)) {
    if (VOLATILE_APP_STATE_KEYS.has(k)) continue
    if (v === undefined) continue
    out[k] = v
  }
  return out
}

function asRecord(v: unknown): Record<string, unknown> {
  if (v && typeof v === 'object' && !Array.isArray(v)) {
    return v as Record<string, unknown>
  }
  return {}
}

function normalizeDoc(raw: Record<string, unknown>): ExcalidrawDocumentData {
  const elements = Array.isArray(raw.elements) ? raw.elements : []
  const files = asRecord(raw.files)
  const appState = asRecord(raw.appState)
  const version =
    typeof raw.version === 'number' && Number.isFinite(raw.version)
      ? raw.version
      : EXCALIDRAW_VERSION
  const type =
    typeof raw.type === 'string' && raw.type.trim()
      ? raw.type
      : EXCALIDRAW_TYPE
  const source =
    typeof raw.source === 'string' && raw.source.trim()
      ? raw.source
      : EXCALIDRAW_SOURCE
  return {
    type,
    version,
    source,
    elements,
    appState,
    files,
  }
}

export function parseExcalidrawContent(raw: string): ExcalidrawParseResult {
  const trimmed = (raw ?? '').trim()
  if (!trimmed) {
    return { ok: true, data: emptyExcalidrawDocument() }
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(trimmed)
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e)
    return {
      ok: false,
      error: `invalid JSON: ${msg}`,
      data: emptyExcalidrawDocument(),
    }
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return {
      ok: false,
      error: 'expected Excalidraw document object',
      data: emptyExcalidrawDocument(),
    }
  }
  const obj = parsed as Record<string, unknown>
  if (obj.elements == null) {
    return {
      ok: false,
      error: 'missing `elements` array (expected Excalidraw document)',
      data: emptyExcalidrawDocument(),
    }
  }
  if (!Array.isArray(obj.elements)) {
    return {
      ok: false,
      error: '`elements` must be an array',
      data: emptyExcalidrawDocument(),
    }
  }
  return { ok: true, data: normalizeDoc(obj) }
}

export function serializeExcalidraw(
  data: Partial<ExcalidrawDocumentData> & {
    elements?: unknown[]
    appState?: Record<string, unknown>
    files?: Record<string, unknown>
  },
): string {
  const doc: ExcalidrawDocumentData = {
    type:
      typeof data.type === 'string' && data.type.trim()
        ? data.type
        : EXCALIDRAW_TYPE,
    version:
      typeof data.version === 'number' && Number.isFinite(data.version)
        ? data.version
        : EXCALIDRAW_VERSION,
    source:
      typeof data.source === 'string' && data.source.trim()
        ? data.source
        : EXCALIDRAW_SOURCE,
    elements: Array.isArray(data.elements) ? data.elements : [],
    appState: stripVolatileAppState(data.appState),
    files:
      data.files && typeof data.files === 'object' && !Array.isArray(data.files)
        ? data.files
        : {},
  }
  return `${JSON.stringify(doc, null, 2)}\n`
}

export function toExcalidrawInitialData(data: ExcalidrawDocumentData): {
  type?: string
  version?: number
  source?: string
  elements: unknown[]
  appState: Record<string, unknown>
  files: Record<string, unknown>
} {
  return {
    type: data.type,
    version: data.version,
    source: data.source,
    elements: data.elements,
    appState: {
      ...data.appState,
      theme: data.appState.theme ?? 'dark',
    },
    files: data.files,
  }
}
