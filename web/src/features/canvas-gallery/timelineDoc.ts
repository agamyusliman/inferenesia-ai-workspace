// Timeline playground document: ordered milestones with dates + status.
// Persists as JSON, imports/exports CSV and Markdown checklists. No new deps.
//
// Dates are validated, never repaired: an impossible date (2026-02-30) or an
// end before its start is reported as an import error rather than silently
// rewritten, so an imported plan always matches its source file.

export type TimelineStatus = 'planned' | 'in_progress' | 'done' | 'blocked'

export const TIMELINE_STATUSES: TimelineStatus[] = [
  'planned',
  'in_progress',
  'done',
  'blocked',
]

export type TimelineMilestone = {
  id: string
  title: string
  /** ISO date (YYYY-MM-DD); empty when undated. */
  start: string
  /** ISO date (YYYY-MM-DD); empty for point-in-time milestones. */
  end: string
  status: TimelineStatus
  owner: string
  notes: string
}

export type TimelineDoc = {
  version: 1
  title: string
  milestones: TimelineMilestone[]
}

export const TIMELINE_DOC_VERSION = 1 as const

export const TIMELINE_MAX_MILESTONES = 400

let milestoneSeq = 0

export function newMilestoneId(): string {
  milestoneSeq += 1
  return `m${Date.now().toString(36)}${milestoneSeq.toString(36)}`
}

function isoDay(d: Date): string {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

export function todayIso(): string {
  return isoDay(new Date())
}

export function shiftIso(iso: string, days: number): string {
  const base = parseIsoDate(iso)
  if (!base) return ''
  base.setDate(base.getDate() + days)
  return isoDay(base)
}

/**
 * Parses YYYY-MM-DD as a local date. Returns null for any other format and for
 * calendar-impossible days, which `Date` would otherwise roll into next month.
 */
export function parseIsoDate(iso: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec((iso || '').trim())
  if (!m) return null
  const y = Number(m[1])
  const mo = Number(m[2])
  const d = Number(m[3])
  if (mo < 1 || mo > 12 || d < 1 || d > 31) return null
  const dt = new Date(y, mo - 1, d)
  if (dt.getFullYear() !== y || dt.getMonth() !== mo - 1 || dt.getDate() !== d) {
    return null
  }
  return dt
}

/**
 * Converts an unambiguous written date to ISO for import. Returns null when the
 * value is not a date the app can read without guessing — the caller reports it
 * instead of inventing a day.
 */
export function coerceIsoDate(value: string): string | null {
  const raw = (value || '').trim()
  if (!raw) return ''
  if (parseIsoDate(raw)) return raw
  const slash = /^(\d{1,2})[/.-](\d{1,2})[/.-](\d{4})$/.exec(raw)
  if (slash) {
    const a = Number(slash[1])
    const b = Number(slash[2])
    const year = slash[3]
    // Day-first only when the first field cannot be a month; a genuinely
    // ambiguous value like 03/04/2026 is rejected rather than guessed.
    if (a <= 12 && b <= 12 && a !== b) return null
    const month = a > 12 ? b : a
    const day = a > 12 ? a : b
    const iso = `${year}-${String(month).padStart(2, '0')}-${String(day).padStart(2, '0')}`
    return parseIsoDate(iso) ? iso : null
  }
  return null
}

export function coerceTimelineStatus(value: string): TimelineStatus | null {
  const v = (value || '').trim().toLowerCase().replace(/[\s-]+/g, '_')
  if (!v) return 'planned'
  if (v === 'done' || v === 'complete' || v === 'completed' || v === 'shipped') {
    return 'done'
  }
  if (
    v === 'in_progress' ||
    v === 'progress' ||
    v === 'doing' ||
    v === 'active' ||
    v === 'wip'
  ) {
    return 'in_progress'
  }
  if (v === 'blocked' || v === 'risk' || v === 'at_risk' || v === 'stuck') {
    return 'blocked'
  }
  if (v === 'planned' || v === 'todo' || v === 'backlog' || v === 'not_started') {
    return 'planned'
  }
  return null
}

/** A new timeline is empty — the user's first milestone is their own. */
export function emptyTimelineDoc(title?: string): TimelineDoc {
  return {
    version: TIMELINE_DOC_VERSION,
    title: title || 'Timeline',
    milestones: [],
  }
}

/**
 * Fills in structural defaults for a document the app itself produced (editor
 * state, agent result already validated). It does not reinterpret dates.
 */
export function normalizeTimelineDoc(doc: TimelineDoc): TimelineDoc {
  const milestones = doc.milestones.map((m, i) => ({
    id: m.id || newMilestoneId(),
    title: typeof m.title === 'string' ? m.title : `Milestone ${i + 1}`,
    start: parseIsoDate(m.start) ? m.start.trim() : '',
    end: parseIsoDate(m.end) ? m.end.trim() : '',
    status: coerceTimelineStatus(m.status) || 'planned',
    owner: typeof m.owner === 'string' ? m.owner : '',
    notes: typeof m.notes === 'string' ? m.notes : '',
  }))
  return {
    version: TIMELINE_DOC_VERSION,
    title: typeof doc.title === 'string' && doc.title ? doc.title : 'Timeline',
    milestones,
  }
}

/** First ordering/consistency complaint, or null when the plan is coherent. */
export function timelineIntegrityError(doc: TimelineDoc): string | null {
  if (doc.milestones.length > TIMELINE_MAX_MILESTONES) {
    return `Too many milestones: ${doc.milestones.length} (max ${TIMELINE_MAX_MILESTONES})`
  }
  for (const m of doc.milestones) {
    const label = m.title || 'Untitled milestone'
    if (m.end && !m.start) {
      return `“${label}” has an end date but no start date`
    }
    if (m.start && m.end && m.end < m.start) {
      return `“${label}” ends (${m.end}) before it starts (${m.start})`
    }
  }
  return null
}

/** Undated milestones sink to the bottom, keeping their relative order. */
export function sortTimelineMilestones(
  milestones: TimelineMilestone[],
): TimelineMilestone[] {
  return milestones
    .map((m, i) => ({ m, i }))
    .sort((a, b) => {
      const as = a.m.start
      const bs = b.m.start
      if (!as && !bs) return a.i - b.i
      if (!as) return 1
      if (!bs) return -1
      if (as === bs) return a.i - b.i
      return as < bs ? -1 : 1
    })
    .map((x) => x.m)
}

export function serializeTimelineDoc(doc: TimelineDoc): string {
  return `${JSON.stringify(normalizeTimelineDoc(doc), null, 2)}\n`
}

export type TimelineParseResult = {
  doc: TimelineDoc
  error?: string
}

export function parseTimelineDoc(raw: string): TimelineParseResult {
  const text = (raw || '').trim()
  if (!text) return { doc: emptyTimelineDoc() }
  if (text.startsWith('{') || text.startsWith('[')) {
    let parsed: unknown
    try {
      parsed = JSON.parse(text)
    } catch (e) {
      return {
        doc: emptyTimelineDoc(),
        error: e instanceof Error ? e.message : 'Invalid timeline JSON',
      }
    }
    const result = timelineFromUnknown(parsed)
    if (result.error) return { doc: emptyTimelineDoc(), error: result.error }
    const integrity = timelineIntegrityError(result.doc)
    if (integrity) return { doc: emptyTimelineDoc(), error: integrity }
    return { doc: normalizeTimelineDoc(result.doc) }
  }
  return timelineFromCsv(text)
}

type FieldReader = (...keys: string[]) => string

function readMilestone(
  pick: FieldReader,
  index: number,
): { milestone?: TimelineMilestone; error?: string } {
  const title =
    pick('title', 'name', 'label', 'milestone') || `Milestone ${index + 1}`
  const rawStart = pick('start', 'date', 'begin', 'from', 'startdate')
  const rawEnd = pick('end', 'due', 'finish', 'to', 'enddate')
  const start = coerceIsoDate(rawStart)
  if (start === null) {
    return { error: `“${title}”: unreadable start date “${rawStart}” (use YYYY-MM-DD)` }
  }
  const end = coerceIsoDate(rawEnd)
  if (end === null) {
    return { error: `“${title}”: unreadable end date “${rawEnd}” (use YYYY-MM-DD)` }
  }
  const rawStatus = pick('status', 'state', 'progress')
  const status = coerceTimelineStatus(rawStatus)
  if (status === null) {
    return {
      error: `“${title}”: unknown status “${rawStatus}” (planned, in_progress, done, blocked)`,
    }
  }
  return {
    milestone: {
      id: pick('id') || newMilestoneId(),
      title,
      start,
      end,
      status,
      owner: pick('owner', 'assignee', 'who', 'team'),
      notes: pick('notes', 'note', 'detail', 'description'),
    },
  }
}

function timelineFromUnknown(value: unknown): TimelineParseResult {
  const list = Array.isArray(value)
    ? value
    : value && typeof value === 'object'
      ? ((): unknown[] | null => {
          const o = value as Record<string, unknown>
          for (const key of ['milestones', 'items', 'events']) {
            if (Array.isArray(o[key])) return o[key] as unknown[]
          }
          return null
        })()
      : null
  if (!list) {
    return {
      doc: emptyTimelineDoc(),
      error: 'Unrecognized timeline JSON shape',
    }
  }
  const title =
    !Array.isArray(value) &&
    value &&
    typeof (value as Record<string, unknown>).title === 'string'
      ? String((value as Record<string, unknown>).title)
      : 'Timeline'
  const milestones: TimelineMilestone[] = []
  for (let i = 0; i < list.length; i += 1) {
    const raw = list[i]
    if (!raw || typeof raw !== 'object') continue
    const o = raw as Record<string, unknown>
    const read = readMilestone((...keys) => {
      for (const k of keys) {
        const direct = o[k]
        if (typeof direct === 'string' && direct.trim()) return direct.trim()
        if (typeof direct === 'number') return String(direct)
        // Tolerate camelCase/spacing variants of the same field name.
        for (const actual of Object.keys(o)) {
          if (actual.toLowerCase().replace(/[\s_-]+/g, '') !== k) continue
          const v = o[actual]
          if (typeof v === 'string' && v.trim()) return v.trim()
          if (typeof v === 'number') return String(v)
        }
      }
      return ''
    }, i)
    if (read.error) return { doc: emptyTimelineDoc(), error: read.error }
    if (read.milestone) milestones.push(read.milestone)
  }
  return { doc: { version: TIMELINE_DOC_VERSION, title, milestones } }
}

export const TIMELINE_CSV_HEADER = 'title,start,end,status,owner,notes'

export function timelineToCsv(doc: TimelineDoc): string {
  const cell = (v: string) =>
    /[",\r\n]/.test(v || '') ? `"${(v || '').replace(/"/g, '""')}"` : v || ''
  const lines = [TIMELINE_CSV_HEADER]
  for (const m of normalizeTimelineDoc(doc).milestones) {
    lines.push(
      [m.title, m.start, m.end, m.status, m.owner, m.notes].map(cell).join(','),
    )
  }
  return `${lines.join('\n')}\n`
}

export function timelineToMarkdown(doc: TimelineDoc): string {
  const d = normalizeTimelineDoc(doc)
  const mark: Record<TimelineStatus, string> = {
    planned: ' ',
    in_progress: '~',
    done: 'x',
    blocked: '!',
  }
  const lines = [`# ${d.title}`, '']
  for (const m of sortTimelineMilestones(d.milestones)) {
    const range = m.end && m.end !== m.start ? `${m.start} → ${m.end}` : m.start
    const meta = [range, m.owner].filter(Boolean).join(' · ')
    lines.push(`- [${mark[m.status]}] ${m.title}${meta ? ` — ${meta}` : ''}`)
    if (m.notes.trim()) lines.push(`  - ${m.notes.trim().replace(/\n+/g, ' ')}`)
  }
  return `${lines.join('\n')}\n`
}

/** Mermaid gantt export — reuses the app's existing Mermaid rendering path. */
export function timelineToMermaidGantt(doc: TimelineDoc): string {
  const d = normalizeTimelineDoc(doc)
  const lines = [
    'gantt',
    `  title ${d.title.replace(/\n+/g, ' ')}`,
    '  dateFormat YYYY-MM-DD',
    '  axisFormat %d %b',
    '  section Milestones',
  ]
  for (const m of sortTimelineMilestones(d.milestones)) {
    if (!m.start) continue
    const label = m.title.replace(/[:\n]+/g, ' ').trim() || 'Milestone'
    const tag =
      m.status === 'done' ? 'done, ' : m.status === 'in_progress' ? 'active, ' : ''
    // Mermaid needs a non-zero span; a point milestone renders as one day.
    const end = m.end && m.end !== m.start ? m.end : shiftIso(m.start, 1)
    lines.push(`  ${label} :${tag}${m.start}, ${end}`)
  }
  return `${lines.join('\n')}\n`
}

const CSV_COLUMN_ALIASES: Record<string, string[]> = {
  title: ['title', 'name', 'milestone', 'label'],
  start: ['start', 'date', 'from', 'begin'],
  end: ['end', 'due', 'to', 'finish'],
  status: ['status', 'state'],
  owner: ['owner', 'assignee', 'team'],
  notes: ['notes', 'note', 'description'],
}

export function timelineFromCsv(text: string): TimelineParseResult {
  const parsed = parseTimelineCsvGrid(text)
  if (parsed.error) return { doc: emptyTimelineDoc(), error: parsed.error }
  const rows = parsed.rows
  if (rows.length === 0) {
    return { doc: emptyTimelineDoc(), error: 'No milestones found' }
  }
  const header = rows[0].map((h) => h.trim().toLowerCase())
  const looksLikeHeader = CSV_COLUMN_ALIASES.title.some((a) =>
    header.includes(a),
  )
  if (!looksLikeHeader) {
    return {
      doc: emptyTimelineDoc(),
      error: `Missing a header row — expected: ${TIMELINE_CSV_HEADER}`,
    }
  }
  const indexOf: Record<string, number> = {}
  for (const [field, aliases] of Object.entries(CSV_COLUMN_ALIASES)) {
    indexOf[field] = aliases.reduce(
      (found, a) => (found >= 0 ? found : header.indexOf(a)),
      -1,
    )
  }
  const body = rows.slice(1).filter((r) => r.some((c) => c.trim()))
  if (body.length > TIMELINE_MAX_MILESTONES) {
    return {
      doc: emptyTimelineDoc(),
      error: `Too many milestones: ${body.length} (max ${TIMELINE_MAX_MILESTONES})`,
    }
  }
  const milestones: TimelineMilestone[] = []
  for (let i = 0; i < body.length; i += 1) {
    const row = body[i]
    const read = readMilestone((...keys) => {
      for (const k of keys) {
        const idx = indexOf[k]
        if (idx != null && idx >= 0) {
          const v = (row[idx] || '').trim()
          if (v) return v
        }
      }
      return ''
    }, i)
    if (read.error) {
      return { doc: emptyTimelineDoc(), error: `Row ${i + 2}: ${read.error}` }
    }
    if (read.milestone) milestones.push(read.milestone)
  }
  const doc: TimelineDoc = {
    version: TIMELINE_DOC_VERSION,
    title: 'Timeline',
    milestones,
  }
  const integrity = timelineIntegrityError(doc)
  if (integrity) return { doc: emptyTimelineDoc(), error: integrity }
  return { doc: normalizeTimelineDoc(doc) }
}

type CsvGrid = { rows: string[][]; error?: string }

function parseTimelineCsvGrid(text: string): CsvGrid {
  // Local reader keeps timelineDoc independent of tableDoc; same strictness:
  // an unterminated quote is an error, not a silently accepted field.
  const src = (text || '').replace(/^\uFEFF/, '')
  const rows: string[][] = []
  let row: string[] = []
  let field = ''
  let inQuotes = false
  let quoted = false
  let started = false
  let line = 1
  let i = 0
  const endField = () => {
    row.push(field)
    field = ''
    quoted = false
    started = false
  }
  const endRow = () => {
    endField()
    rows.push(row)
    row = []
  }
  while (i < src.length) {
    const ch = src[i]
    if (inQuotes) {
      if (ch === '"') {
        if (src[i + 1] === '"') {
          field += '"'
          i += 2
          continue
        }
        inQuotes = false
        i += 1
        continue
      }
      if (ch === '\n') line += 1
      field += ch
      i += 1
      continue
    }
    if (ch === '"') {
      if (started) {
        return {
          rows: [],
          error: `Unescaped quote in an unquoted field on line ${line}`,
        }
      }
      inQuotes = true
      quoted = true
      started = true
      i += 1
      continue
    }
    if (quoted && ch !== ',' && ch !== '\r' && ch !== '\n') {
      return {
        rows: [],
        error: `Unexpected text after a closing quote on line ${line}`,
      }
    }
    if (ch === ',') {
      endField()
      i += 1
      continue
    }
    if (ch === '\r' || ch === '\n') {
      if (ch === '\r' && src[i + 1] === '\n') i += 1
      endRow()
      line += 1
      i += 1
      continue
    }
    field += ch
    started = true
    i += 1
  }
  if (inQuotes) {
    return {
      rows: [],
      error: `Unterminated quoted field starting before line ${line}`,
    }
  }
  if (field.length > 0 || row.length > 0) endRow()
  while (rows.length > 0) {
    const last = rows[rows.length - 1]
    if (last.length === 1 && last[0] === '') rows.pop()
    else break
  }
  return { rows }
}

export type TimelineBounds = {
  min: string
  max: string
  days: number
}

/** Date span used to lay out the visual track; null when nothing is dated. */
export function timelineBounds(doc: TimelineDoc): TimelineBounds | null {
  let min = ''
  let max = ''
  for (const m of doc.milestones) {
    const s = m.start
    if (!s) continue
    const e = m.end || s
    if (!min || s < min) min = s
    if (!max || e > max) max = e
  }
  if (!min || !max) return null
  const a = parseIsoDate(min)
  const b = parseIsoDate(max)
  if (!a || !b) return null
  const days = Math.max(1, Math.round((b.getTime() - a.getTime()) / 86_400_000))
  return { min, max, days }
}

/** 0..1 offset+width for a milestone bar inside `bounds`. */
export function timelineSpan(
  m: TimelineMilestone,
  bounds: TimelineBounds,
): { offset: number; width: number } | null {
  const start = parseIsoDate(m.start)
  if (!start) return null
  const base = parseIsoDate(bounds.min)
  if (!base) return null
  const end = parseIsoDate(m.end || m.start) || start
  const total = bounds.days * 86_400_000
  if (total <= 0) return { offset: 0, width: 1 }
  const offset = Math.max(
    0,
    Math.min(1, (start.getTime() - base.getTime()) / total),
  )
  const width = Math.max(end.getTime() - start.getTime(), 0) / total
  return {
    offset,
    // Zero-length milestones still need a visible tick.
    width: Math.max(0.012, Math.min(1 - offset, width)),
  }
}
