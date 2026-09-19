import {
  EXCALIDRAW_SOURCE,
  EXCALIDRAW_TYPE,
  EXCALIDRAW_VERSION,
  serializeExcalidraw,
  type ExcalidrawDocumentData,
} from './excalidrawDoc'
import type { CanvasAIResult } from '../../lib/api'

export type CanvasElement = Record<string, unknown> & {
  id: string
  type: string
}

const FENCE_RE = /^\s*```(?:json|excalidraw|jsonc)?\s*\n?([\s\S]*?)\n?```\s*$/

const VALID_ELEMENT_TYPES = new Set([
  'rectangle',
  'ellipse',
  'diamond',
  'arrow',
  'line',
  'text',
  'freedraw',
])

export function stripFences(raw: string): string {
  const trimmed = raw.trim()
  const m = FENCE_RE.exec(trimmed)
  return m ? m[1].trim() : trimmed
}

export function extractFirstJSONBlock(s: string): string {
  const startArr = s.indexOf('[')
  const startObj = s.indexOf('{')
  let start = -1
  let open = ''
  let close = ''
  if (startArr < 0 && startObj < 0) return ''
  if (startArr < 0) {
    start = startObj
    open = '{'
    close = '}'
  } else if (startObj < 0) {
    start = startArr
    open = '['
    close = ']'
  } else if (startArr < startObj) {
    start = startArr
    open = '['
    close = ']'
  } else {
    start = startObj
    open = '{'
    close = '}'
  }
  let depth = 0
  let inStr = false
  let escaped = false
  for (let i = start; i < s.length; i++) {
    const c = s[i]
    if (inStr) {
      if (escaped) {
        escaped = false
        continue
      }
      if (c === '\\') {
        escaped = true
        continue
      }
      if (c === '"') inStr = false
      continue
    }
    if (c === '"') inStr = true
    else if (c === open) depth++
    else if (c === close) {
      depth--
      if (depth === 0) return s.slice(start, i + 1)
    }
  }
  return ''
}

function mintId(): string {
  const hex = Math.random().toString(16).slice(2, 10)
  return `el${hex.padStart(8, '0')}`
}

function isPlainObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

export type ExtractResult =
  | { ok: true; elements: CanvasElement[] }
  | { ok: false; error: string }

export function extractElementsFromModelText(raw: string): ExtractResult {
  const stripped = stripFences(raw)
  if (!stripped) {
    return { ok: false, error: 'canvas AI: empty model output' }
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(stripped)
  } catch {
    const block = extractFirstJSONBlock(stripped)
    if (!block) {
      return { ok: false, error: 'canvas AI: model output is not valid JSON' }
    }
    try {
      parsed = JSON.parse(block)
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      return { ok: false, error: `canvas AI: model output is not valid JSON: ${msg}` }
    }
  }
  let elements: unknown[]
  if (Array.isArray(parsed)) {
    elements = parsed
  } else if (isPlainObject(parsed)) {
    if ('elements' in parsed && Array.isArray(parsed.elements)) {
      elements = parsed.elements
    } else if ('type' in parsed) {
      elements = [parsed]
    } else {
      return { ok: false, error: 'canvas AI: "elements" must be an array' }
    }
  } else {
    return {
      ok: false,
      error: `canvas AI: expected JSON array or object, got ${typeof parsed}`,
    }
  }
  if (elements.length === 0) {
    return { ok: false, error: 'canvas AI: elements array is empty' }
  }
  const out: CanvasElement[] = []
  for (let i = 0; i < elements.length; i++) {
    const el = elements[i]
    if (!isPlainObject(el)) {
      return { ok: false, error: `canvas AI: element ${i} is not an object` }
    }
    if (typeof el.type !== 'string' || !el.type) {
      return { ok: false, error: `canvas AI: element ${i} missing "type"` }
    }
    const id = typeof el.id === 'string' ? el.id.trim() : ''
    const normalized: CanvasElement = {
      ...el,
      type: el.type,
      id: id || mintId(),
    }
    out.push(normalized)
  }
  return { ok: true, elements: out }
}

export function isCanvasElementType(type: unknown): boolean {
  return typeof type === 'string' && VALID_ELEMENT_TYPES.has(type)
}

export function elementsJSONForRequest(
  elements: readonly unknown[],
): string {
  const clean = elements.filter(isPlainObject)
  return JSON.stringify(clean)
}

export type MergeResult = {
  document: ExcalidrawDocumentData
  serialized: string
}

export function mergeCanvasAIResult(
  result: CanvasAIResult,
  base: ExcalidrawDocumentData,
): MergeResult | { error: string } {
  if (!result.ok || !result.elements_json) {
    return { error: result.error || 'canvas AI: no elements returned' }
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(result.elements_json)
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e)
    return { error: `canvas AI: invalid elements JSON: ${msg}` }
  }
  if (!Array.isArray(parsed)) {
    return { error: 'canvas AI: elements_json is not an array' }
  }
  const doc: ExcalidrawDocumentData = {
    type: EXCALIDRAW_TYPE,
    version: EXCALIDRAW_VERSION,
    source: EXCALIDRAW_SOURCE,
    elements: parsed,
    appState: { ...base.appState },
    files: { ...base.files },
  }
  return {
    document: doc,
    serialized: serializeExcalidraw(doc),
  }
}

export function placeholderForMode(
  mode: 'generate' | 'edit',
  elementCount: number,
): string {
  if (mode === 'edit' || elementCount > 0) {
    return 'Describe edit… e.g. Change Database box to blue'
  }
  return 'Describe diagram or edit… e.g. Draw auth flow'
}

export function submitLabel(elementCount: number): string {
  return elementCount > 0 ? 'Apply' : 'Generate'
}
