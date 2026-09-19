import {
  classifyDiffLine,
  countDiffStats,
  parseDiffLines,
  type DiffLine,
} from '../git/gitDiffLines'

export const CODE_PREVIEW_LINE_LIMIT = 24

const EXT_BY_LANG: Record<string, string> = {
  typescript: 'ts',
  ts: 'ts',
  tsx: 'tsx',
  javascript: 'js',
  js: 'js',
  jsx: 'jsx',
  go: 'go',
  python: 'py',
  py: 'py',
  rust: 'rs',
  rs: 'rs',
  json: 'json',
  yaml: 'yml',
  yml: 'yml',
  markdown: 'md',
  md: 'md',
  css: 'css',
  scss: 'scss',
  html: 'html',
  sql: 'sql',
  shell: 'sh',
  bash: 'sh',
  sh: 'sh',
  zsh: 'sh',
  diff: 'diff',
  patch: 'diff',
  text: 'txt',
  txt: 'txt',
}

export function parseFenceMeta(langRaw: string): {
  language: string
  path?: string
} {
  const raw = (langRaw || '').trim()
  if (!raw) return { language: '' }
  const colon = raw.indexOf(':')
  if (colon > 0) {
    return {
      language: raw.slice(0, colon).trim().toLowerCase(),
      path: raw.slice(colon + 1).trim() || undefined,
    }
  }
  const space = raw.indexOf(' ')
  if (space > 0) {
    const left = raw.slice(0, space).trim().toLowerCase()
    const right = raw.slice(space + 1).trim()
    if (right.includes('/') || right.includes('.')) {
      return { language: left, path: right }
    }
  }
  return { language: raw.toLowerCase() }
}

export function looksLikeUnifiedDiff(text: string): boolean {
  const t = text.trimStart()
  if (!t) return false
  if (t.startsWith('diff --git ') || t.startsWith('--- ') || t.startsWith('+++ ')) {
    return true
  }
  if (t.includes('\n@@ ') || t.startsWith('@@ ')) return true
  const lines = t.split('\n').slice(0, 40)
  let add = 0
  let del = 0
  for (const line of lines) {
    if (line.startsWith('+') && !line.startsWith('+++')) add++
    if (line.startsWith('-') && !line.startsWith('---')) del++
  }
  return add + del >= 3 && (add > 0 || del > 0)
}

export function shouldRenderAsDiff(language: string, text: string): boolean {
  const l = (language || '').toLowerCase()
  if (l === 'diff' || l === 'patch' || l === 'udiff') return true
  return looksLikeUnifiedDiff(text)
}

export function extensionForLanguage(language: string, path?: string): string {
  if (path) {
    const base = path.split(/[/\\]/).pop() || ''
    const dot = base.lastIndexOf('.')
    if (dot > 0) return base.slice(dot + 1)
  }
  return EXT_BY_LANG[(language || '').toLowerCase()] || 'txt'
}

export function downloadFilename(
  language: string,
  path: string | undefined,
  isDiff: boolean,
  opts?: {
    sessionName?: string | null
    blockIndex?: number | null
  },
): string {
  if (path) {
    const base = path.split(/[/\\]/).pop() || path
    if (isDiff && !base.endsWith('.diff') && !base.endsWith('.patch')) {
      return `${base}.diff`
    }
    return base
  }
  const ext = isDiff ? 'diff' : extensionForLanguage(language)
  if (opts?.sessionName || opts?.blockIndex != null) {
    return inferenesiaSnippetDownloadName({
      sessionName: opts.sessionName,
      canvasIndex: opts.blockIndex ?? 1,
      ext,
    })
  }
  return isDiff ? `change.${ext}` : `snippet.${ext}`
}

export function sanitizeDownloadSegment(raw: string, fallback = 'session'): string {
  const s = (raw || '')
    .replace(/[<>:"/\\|?*\u0000-\u001f]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
  if (!s) return fallback
  return s.slice(0, 80).replace(/[. ]+$/g, '') || fallback
}

export function inferenesiaSnippetDownloadName(opts: {
  sessionName?: string | null
  canvasId?: string | null
  canvasIndex?: number | null
  ext?: string
}): string {
  const session = sanitizeDownloadSegment(
    (opts.sessionName || '')
      .replace(/^Session\s*[·•\-–—]\s*/i, '')
      .replace(/^Sesi\s*[·•\-–—]\s*/i, '')
      .trim() || 'New chat',
    'New chat',
  )
  let idPart = ''
  if (opts.canvasIndex != null && Number.isFinite(opts.canvasIndex) && opts.canvasIndex > 0) {
    idPart = String(Math.floor(opts.canvasIndex))
  } else if (opts.canvasId) {
    const m = /([a-z0-9]{4,})$/i.exec(opts.canvasId.replace(/^cv_/, ''))
    idPart = m?.[1] || opts.canvasId.slice(-8)
  } else {
    idPart = '1'
  }
  const ext = (opts.ext || 'html').replace(/^\./, '') || 'html'
  return `Inferenesia - ${session} - ${idPart}.${ext}`
}

export function truncateCodeLines(
  text: string,
  limit = CODE_PREVIEW_LINE_LIMIT,
): { preview: string; totalLines: number; truncated: boolean } {
  const lines = text.replace(/\r\n/g, '\n').split('\n')
  const totalLines = lines.length === 1 && lines[0] === '' ? 0 : lines.length
  if (totalLines <= limit) {
    return { preview: text, totalLines, truncated: false }
  }
  return {
    preview: lines.slice(0, limit).join('\n') + '\n',
    totalLines,
    truncated: true,
  }
}

export function buildDiffPreview(
  text: string,
  limit = CODE_PREVIEW_LINE_LIMIT,
): {
  lines: DiffLine[]
  stats: { additions: number; deletions: number }
  totalLines: number
  truncated: boolean
} {
  const all = parseDiffLines(text)
  const stats = countDiffStats(all)
  if (all.length <= limit) {
    return { lines: all, stats, totalLines: all.length, truncated: false }
  }
  return {
    lines: all.slice(0, limit),
    stats,
    totalLines: all.length,
    truncated: true,
  }
}

export function diffLineClass(kind: string): string {
  switch (kind) {
    case 'add':
      return 'bg-emerald-950/45 text-emerald-200'
    case 'del':
      return 'bg-red-950/45 text-red-200'
    case 'hunk':
      return 'bg-sky-950/40 text-sky-300'
    case 'meta':
      return 'text-shell-muted'
    default:
      return 'text-shell-text/90'
  }
}

export { classifyDiffLine, parseDiffLines, countDiffStats }
