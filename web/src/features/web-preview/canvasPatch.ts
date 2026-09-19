export type SearchReplacePatch = {
  search: string
  replace: string
}

export type CanvasPatchResult =
  | { ok: true; next: string; applied: number; mode: 'patches' | 'full' }
  | { ok: false; error: string; applied: number }

const MARK_L = '<' + '<<<<<<'
const MARK_M = '=' + '======'
const MARK_R = '>' + '>>>>>>'
const SEARCH_HDR = MARK_L + ' SEARCH'
const REPLACE_FTR = MARK_R + ' REPLACE'

const PATCH_BLOCK_RE = new RegExp(
  `${SEARCH_HDR}\\s*\\n([\\s\\S]*?)\\n${MARK_M}\\s*\\n([\\s\\S]*?)\\n${REPLACE_FTR}`,
  'g',
)

export function extractSearchReplacePatches(
  text: string,
): SearchReplacePatch[] {
  if (!text) return []
  const out: SearchReplacePatch[] = []
  PATCH_BLOCK_RE.lastIndex = 0
  let m: RegExpExecArray | null
  while ((m = PATCH_BLOCK_RE.exec(text)) !== null) {
    const search = m[1] ?? ''
    const replace = m[2] ?? ''
    if (search.length === 0 && replace.length === 0) continue
    out.push({ search, replace })
  }
  return out
}

export function looksLikePatchResponse(text: string): boolean {
  return text.includes(SEARCH_HDR) && text.includes(REPLACE_FTR)
}

function indexOfFlexible(haystack: string, needle: string): number {
  if (!needle) return -1
  const direct = haystack.indexOf(needle)
  if (direct >= 0) return direct

  const normNeedle = needle.replace(/\r\n/g, '\n')
  const normHay = haystack.replace(/\r\n/g, '\n')
  const n1 = normHay.indexOf(normNeedle)
  if (n1 >= 0) {
    return mapNormIndexToOriginal(haystack, n1)
  }

  const compactNeedle = collapseWs(normNeedle)
  if (compactNeedle.length < 8) return -1
  const compactHay = collapseWs(normHay)
  const cIdx = compactHay.indexOf(compactNeedle)
  if (cIdx < 0) return -1

  return mapCollapsedIndexToOriginal(haystack, cIdx)
}

function collapseWs(s: string): string {
  return s.replace(/\s+/g, ' ').trim()
}

function mapNormIndexToOriginal(original: string, normIndex: number): number {
  let ni = 0
  for (let i = 0; i < original.length; i += 1) {
    if (ni === normIndex) return i
    const ch = original[i]
    if (ch === '\r' && original[i + 1] === '\n') {
      ni += 1
      i += 1
    } else {
      ni += 1
    }
  }
  return ni === normIndex ? original.length : -1
}

function mapCollapsedIndexToOriginal(
  original: string,
  collapsedIndex: number,
): number {
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

export function applySearchReplacePatches(
  base: string,
  patches: SearchReplacePatch[],
): CanvasPatchResult {
  if (!patches.length) {
    return { ok: false, error: 'No patches in reply', applied: 0 }
  }
  let next = base
  let applied = 0
  for (let i = 0; i < patches.length; i += 1) {
    const p = patches[i]
    const idx = indexOfFlexible(next, p.search)
    if (idx < 0) {
      return {
        ok: false,
        error: `Patch ${i + 1}/${patches.length} SEARCH not found in current canvas`,
        applied,
      }
    }
    next = next.slice(0, idx) + p.replace + next.slice(idx + p.search.length)
    applied += 1
  }
  return { ok: true, next, applied, mode: 'patches' }
}

export function resolveCanvasAgentReply(
  base: string,
  reply: string,
  fullDocument: string | null,
): CanvasPatchResult {
  const patches = extractSearchReplacePatches(reply)
  if (patches.length > 0) {
    return applySearchReplacePatches(base, patches)
  }
  if (fullDocument && fullDocument.trim()) {
    return {
      ok: true,
      next: fullDocument,
      applied: 1,
      mode: 'full',
    }
  }
  return {
    ok: false,
    error: 'No patches or full document in reply',
    applied: 0,
  }
}
