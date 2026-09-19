// Prompt + reply handling for the Markdown / Table / Timeline playgrounds.
// Everything streams through the same core `streamChat` ephemeral path the
// other gallery panels use — no separate LLM client.

import { resolveCanvasAgentReply } from '../web-preview/canvasPatch'
import { extractMarkdownFromReply } from './markdownDoc'
import {
  activeSheet,
  emptyTableWorkbook,
  makeColumn,
  makeSheet,
  normalizeTableDoc,
  normalizeTableWorkbook,
  parseTableWorkbook,
  serializeTableWorkbook,
  tableFromCsv,
  updateSheetDoc,
  TABLE_MAX_COLUMNS,
  TABLE_MAX_ROWS,
  type TableCellStyle,
  type TableMerge,
  type TableSheet,
  type TableWorkbook,
  type TableWorkbookParseResult,
} from './tableDoc'
import {
  parseTimelineDoc,
  serializeTimelineDoc,
  timelineFromCsv,
  timelineToCsv,
  TIMELINE_CSV_HEADER,
  type TimelineDoc,
} from './timelineDoc'

export type DocAgentKind = 'markdown' | 'table' | 'timeline'

export type DocAgentHint = {
  id: string
  label: string
  instruction: string
  note?: string
}

export const DOC_AGENT_HINTS: Record<DocAgentKind, DocAgentHint[]> = {
  markdown: [
    {
      id: 'outline',
      label: 'Outline',
      instruction: 'Restructure into a clear heading outline with short sections.',
      note: 'Headings + sections',
    },
    {
      id: 'expand',
      label: 'Expand',
      instruction: 'Expand each section with concrete detail; keep the structure.',
      note: 'More detail',
    },
    {
      id: 'tighten',
      label: 'Tighten',
      instruction: 'Cut filler and tighten the wording without losing facts.',
      note: 'Shorter',
    },
    {
      id: 'summary',
      label: 'Summary',
      instruction: 'Add a short TL;DR summary section at the top.',
      note: 'TL;DR',
    },
    {
      id: 'checklist',
      label: 'Checklist',
      instruction: 'Turn the action items into a Markdown task checklist.',
      note: 'Tasks',
    },
  ],
  table: [
    {
      id: 'fill',
      label: 'Fill from source',
      instruction:
        'Fill empty cells ONLY from values stated in my message or the attachments. Leave a cell blank when the source does not give it — never invent a value.',
      note: 'From attachments',
    },
    {
      id: 'derive',
      label: 'Derive column',
      instruction:
        'Add one column computed from existing columns (for example a total or a difference). Leave it blank on rows where the inputs are missing.',
      note: 'Computed field',
    },
    {
      id: 'clean',
      label: 'Clean up',
      instruction:
        'Normalize casing, date format and units of existing values. Do not add, remove or invent rows, columns or values.',
      note: 'Normalize',
    },
    {
      id: 'sort',
      label: 'Sort',
      instruction:
        'Sort the rows by the most meaningful column. Keep the header and every row exactly as-is otherwise.',
      note: 'Reorder',
    },
    {
      id: 'dedupe',
      label: 'Deduplicate',
      instruction:
        'Remove rows that duplicate another row. Keep the first occurrence and change nothing else.',
      note: 'Drop repeats',
    },
  ],
  timeline: [
    {
      id: 'plan',
      label: 'Draft plan',
      instruction:
        'Draft milestones for the project I describe below. Use only scope I state; if a date is not implied by my message, leave it blank rather than inventing one.',
      note: 'From my brief',
    },
    {
      id: 'split',
      label: 'Break down',
      instruction:
        'Split milestones longer than two weeks into smaller ones. Keep the overall start and end dates unchanged.',
      note: 'Smaller steps',
    },
    {
      id: 'sequence',
      label: 'Sequence',
      instruction:
        'Reorder milestones into a workable dependency order. Keep every milestone and its duration; only the dates may move.',
      note: 'Fix order',
    },
    {
      id: 'risk',
      label: 'Risks',
      instruction:
        'Mark milestones that depend on something unresolved as blocked and name the blocker in notes. Change nothing else.',
      note: 'Flag blockers',
    },
    {
      id: 'shift',
      label: 'Shift',
      instruction:
        'Shift every milestone that is not done two weeks later. Keep each duration and status exactly as it is.',
      note: 'Re-plan',
    },
  ],
}

const PATCH_BLOCK = [
  '<<<<<<< SEARCH',
  '[exact contiguous snippet from the current document]',
  '=======',
  '[replacement snippet]',
  '>>>>>>> REPLACE',
].join('\n')

function clip(source: string, limit: number): string {
  return source.length > limit
    ? `${source.slice(0, limit)}\n… (truncated)`
    : source
}

export function buildDocAgentPrompt(
  kind: DocAgentKind,
  instruction: string,
  currentSource: string,
  hint?: DocAgentHint,
): string {
  const ask = instruction.trim() || '(see attachments)'
  const extra = hint ? `\nPreset: ${hint.instruction}` : ''
  if (kind === 'markdown') {
    return [
      'You are a surgical editor for ONE Markdown document.',
      '',
      '## Output format (required for normal edits)',
      'Return one or more patches using EXACTLY this syntax, with no markdown fences around them:',
      '',
      PATCH_BLOCK,
      '',
      '- SEARCH must be copied exactly from the current document.',
      '- Keep each SEARCH as small as it can be while staying unique.',
      '- Multiple blocks are fine for disjoint edits.',
      '',
      '## Full rewrite exception',
      'Only when the user asks to rewrite / restructure / start over, return the COMPLETE document in a single ```markdown fence instead of patches.',
      '',
      '## Forbidden',
      '- Commentary outside the patches or the fence',
      '- Dropping sections the user did not mention',
      '',
      `## User request${extra}`,
      ask,
      '',
      '## Current document (source of truth)',
      '```markdown',
      clip(currentSource, 60_000) || '# Untitled',
      '```',
    ].join('\n')
  }
  if (kind === 'table') {
    return [
      'You are a spreadsheet editor. You receive the current workbook as JSON and an instruction.',
      '',
      '## Output format (required)',
      'Return the COMPLETE updated workbook as JSON inside a single ```json fence:',
      '',
      '{"sheets":[{"name":"Sheet name","columns":["Header 1","Header 2"],"rows":[["cell","cell"]],',
      '  "styles":{"0:1":{"bold":true,"fill":"FFE599","color":"1F2937","align":"center","numFmt":"#,##0"}},',
      '  "widths":[180,140],"heights":[36],"hiddenColumns":[],"hiddenRows":[]}]}',
      '',
      '- `columns` is the header row. `rows` holds data only — do not repeat the header.',
      '- `styles` keys are `"row:col"` using ZERO-BASED data-row and column indexes.',
      '- Colours are `RRGGBB` without `#`. `align` is left | center | right.',
      '- `numFmt` is an Excel number format: `#,##0` for thousands, `0.00%`, `"Rp"#,##0`.',
      '- `widths` and `heights` are pixel sizes, index-aligned with columns/rows; 0 means default.',
      '- Omit `styles`, `widths`, `heights`, `hiddenColumns`, `hiddenRows` when unused.',
      '',
      '## Formulas',
      'A cell whose text starts with `=` is a live spreadsheet formula, not text.',
      'Row numbers are 1-based spreadsheet rows where the FIRST DATA ROW IS ROW 2 (row 1 is the header).',
      'Use them for every total, percentage, or derived value — never hard-code a computed number.',
      'Supported: SUM, AVERAGE, MIN, MAX, COUNT, COUNTA, PRODUCT, MEDIAN, ROUND, ROUNDUP, ROUNDDOWN,',
      'ABS, INT, SQRT, POWER, IF, IFERROR, AND, OR, NOT, CONCAT, LEN, UPPER, LOWER, TRIM, %, &, and',
      'the operators + - * / ^ = <> < > <= >=. Ranges look like A2:A9.',
      '',
      '## Multiple sheets',
      'When the user asks for several sheets, reports, or gives you a breakdown by category, produce',
      'ONE entry in `sheets` per requested sheet with a meaningful name. Do not collapse them into one.',
      '',
      '## Building a new workbook',
      'When asked to create, draft, or demonstrate a report, build it properly: clear headers, enough',
      'plausible rows to be useful, a total row using formulas, and formatting that makes it readable',
      '(bold header, filled total row, thousands separators, sensible column widths).',
      'Illustrative values are expected here.',
      '',
      '## Preserving real data',
      'When the user supplies real figures, keep them exactly. Never drop a row or column that was not',
      'asked to be removed. Leave a genuinely unknown value empty rather than inventing it.',
      '',
      `## User request${extra}`,
      ask,
      '',
      '## Current workbook (JSON)',
      '```json',
      clip(currentSource, 60_000) || '{"sheets":[]}',
      '```',
    ].join('\n')
  }
  return [
    'You are a project schedule editor. You receive milestones as CSV and an instruction.',
    '',
    '## Output format (required)',
    'Return the COMPLETE updated schedule as CSV inside a single ```csv fence.',
    `- Header must be exactly: ${TIMELINE_CSV_HEADER}`,
    '- Dates are ISO `YYYY-MM-DD` and must be real calendar dates. Leave `end` empty for a point-in-time milestone.',
    '- `end` must never be earlier than `start`, and a row with an `end` must have a `start`.',
    '- `status` is exactly one of: planned, in_progress, done, blocked',
    '- Quote any field containing a comma, quote or newline.',
    '- No commentary before or after the fence.',
    '',
    '## Data integrity',
    '- When the user gives you real dates or commitments, use exactly those.',
    '- When you are asked to complete a real schedule and a date is genuinely unknown, leave it empty rather than inventing one.',
    '',
    '## Building a new schedule',
    'When the user asks you to draft, plan, or demonstrate a schedule, produce the milestones they described with realistic sequencing and durations. Proposed dates are expected here; the empty-date rule above applies only to schedules the user is relying on.',
    '',
    `## User request${extra}`,
    ask,
    '',
    '## Current schedule (CSV)',
    '```csv',
    clip(currentSource, 60_000),
    '```',
  ].join('\n')
}

/** The document text handed to the model (and diffed against its reply). */
export function docAgentSource(kind: DocAgentKind, stored: string): string {
  if (kind === 'markdown') return stored
  if (kind === 'table') return workbookToAgentJson(parseTableWorkbook(stored).workbook)
  return timelineToCsv(parseTimelineDoc(stored).doc)
}

/** Compact workbook shape shown to the model: data rows only, styles inline. */
export function workbookToAgentJson(wb: TableWorkbook): string {
  const sheets = wb.sheets.map((s) => {
    const doc = s.doc
    const out: Record<string, unknown> = {
      name: s.name,
      columns: doc.columns.map((c) => c.name),
      rows: doc.rows,
    }
    if (doc.styles && Object.keys(doc.styles).length) out.styles = doc.styles
    if (doc.widths) out.widths = doc.widths
    if (doc.heights) out.heights = doc.heights
    if (doc.hiddenColumns?.length) out.hiddenColumns = doc.hiddenColumns
    if (doc.hiddenRows?.length) out.hiddenRows = doc.hiddenRows
    if (doc.merges?.length) out.merges = doc.merges
    return out
  })
  return `${JSON.stringify({ sheets }, null, 1)}\n`
}

function readAgentStyles(
  value: unknown,
  rowCount: number,
  colCount: number,
): Record<string, TableCellStyle> | undefined {
  if (!value || typeof value !== 'object') return undefined
  const out: Record<string, TableCellStyle> = {}
  for (const [key, style] of Object.entries(value as Record<string, unknown>)) {
    const [r, c] = key.split(':').map((n) => Number(n))
    if (!Number.isInteger(r) || !Number.isInteger(c)) continue
    if (r < 0 || r >= rowCount || c < 0 || c >= colCount) continue
    if (!style || typeof style !== 'object') continue
    out[`${r}:${c}`] = style as TableCellStyle
  }
  return Object.keys(out).length ? out : undefined
}

function readAgentSizes(value: unknown, count: number): number[] | undefined {
  if (!Array.isArray(value)) return undefined
  const out = Array.from({ length: count }, (_, i) => {
    const v = Number(value[i])
    return Number.isFinite(v) && v > 0 ? v : 0
  })
  return out.some((v) => v > 0) ? out : undefined
}

/** Reads the model's workbook JSON back into a stored workbook. */
export function workbookFromAgentJson(value: unknown): TableWorkbookParseResult {
  if (!value || typeof value !== 'object') {
    return { workbook: emptyTableWorkbook(), error: 'Reply is not a workbook object' }
  }
  const raw = (value as { sheets?: unknown }).sheets
  if (!Array.isArray(raw) || raw.length === 0) {
    return { workbook: emptyTableWorkbook(), error: 'Reply has no sheets' }
  }

  const sheets: TableSheet[] = []
  for (const [i, entry] of raw.entries()) {
    if (!entry || typeof entry !== 'object') continue
    const s = entry as Record<string, unknown>
    if (!Array.isArray(s.rows)) continue
    const names = Array.isArray(s.columns)
      ? (s.columns as unknown[]).map((c) => String(c ?? ''))
      : []
    const grid = s.rows as unknown[][]
    const width = Math.max(
      names.length,
      ...grid.map((r) => (Array.isArray(r) ? r.length : 0)),
      1,
    )
    if (width > TABLE_MAX_COLUMNS || grid.length > TABLE_MAX_ROWS) {
      return {
        workbook: emptyTableWorkbook(),
        error: `Sheet ${i + 1} exceeds the ${TABLE_MAX_COLUMNS}-column or ${TABLE_MAX_ROWS}-row limit`,
      }
    }
    const columns = Array.from({ length: width }, (_, c) =>
      makeColumn(names[c] || `Column ${c + 1}`),
    )
    const rows = grid.map((r) =>
      Array.from({ length: width }, (_, c) =>
        Array.isArray(r) && r[c] != null ? String(r[c]) : '',
      ),
    )
    const doc = normalizeTableDoc({
      version: 1,
      columns,
      rows,
      styles: readAgentStyles(s.styles, rows.length, width),
      widths: readAgentSizes(s.widths, width),
      heights: readAgentSizes(s.heights, rows.length),
      hiddenColumns: Array.isArray(s.hiddenColumns)
        ? (s.hiddenColumns as number[])
        : undefined,
      hiddenRows: Array.isArray(s.hiddenRows) ? (s.hiddenRows as number[]) : undefined,
      merges: Array.isArray(s.merges) ? (s.merges as TableMerge[]) : undefined,
    })
    sheets.push({ id: makeSheet('').id, name: String(s.name ?? `Sheet ${i + 1}`), doc })
  }

  if (sheets.length === 0) {
    return { workbook: emptyTableWorkbook(), error: 'Reply contained no readable sheet' }
  }
  return {
    workbook: normalizeTableWorkbook({
      version: 1,
      sheets,
      activeSheetId: sheets[0].id,
    }),
  }
}

function extractFence(text: string, langs: string[]): string | null {
  for (const lang of langs) {
    const closed = new RegExp(`\`\`\`${lang}\\s*\\n([\\s\\S]*?)\`\`\``, 'i').exec(
      text,
    )
    if (closed?.[1]?.trim()) return closed[1].trim()
  }
  const generic = /```[a-z]*\s*\n([\s\S]*?)```/i.exec(text)
  if (generic?.[1]?.trim()) return generic[1].trim()
  return null
}

/** Only a fence explicitly tagged with one of `langs` — no generic fallback. */
function extractTaggedFence(text: string, langs: string[]): string | null {
  for (const lang of langs) {
    const closed = new RegExp(`\`\`\`${lang}\\s*\\n([\\s\\S]*?)\`\`\``, 'i').exec(
      text,
    )
    if (closed?.[1]?.trim()) return closed[1].trim()
  }
  return null
}

export type DocAgentApply =
  | { ok: true; content: string; mode: 'patches' | 'full'; applied: number }
  | { ok: false; error: string }

/**
 * Turns a model reply into the next stored document. Markdown supports
 * SEARCH/REPLACE patches; table and timeline take a full CSV payload because
 * partial grid patches are unreliable. A reply that fails validation is
 * rejected outright — the stored document is never partially overwritten.
 */
export function applyDocAgentReply(
  kind: DocAgentKind,
  storedBefore: string,
  reply: string,
): DocAgentApply {
  const text = (reply || '').trim()
  if (!text) return { ok: false, error: 'Empty reply' }
  if (kind === 'markdown') {
    const resolved = resolveCanvasAgentReply(
      storedBefore,
      text,
      extractMarkdownFromReply(text),
    )
    if (!resolved.ok) return { ok: false, error: resolved.error }
    return {
      ok: true,
      content: resolved.next,
      mode: resolved.mode,
      applied: resolved.applied,
    }
  }
  const csv =
    extractFence(text, ['csv', 'tsv']) || (text.includes(',') ? text : null)
  if (!csv) return { ok: false, error: 'No CSV block in reply' }
  if (kind === 'table') {
    const json = extractTaggedFence(text, ['json'])
    if (json) {
      let parsed: unknown
      try {
        parsed = JSON.parse(json)
      } catch (e) {
        return {
          ok: false,
          error: `Reply rejected — ${e instanceof Error ? e.message : 'invalid JSON'}`,
        }
      }
      const result = workbookFromAgentJson(parsed)
      if (result.error) return { ok: false, error: `Reply rejected — ${result.error}` }
      return {
        ok: true,
        content: serializeTableWorkbook(result.workbook),
        mode: 'full',
        applied: result.workbook.sheets.reduce((n, s) => n + s.doc.rows.length, 0),
      }
    }

    // Fallback: a model that answered with CSV still updates the active sheet
    // instead of the whole turn being thrown away.
    const csv = extractFence(text, ['csv', 'tsv']) || (text.includes(',') ? text : null)
    if (!csv) return { ok: false, error: 'No JSON workbook block in reply' }
    const parsedCsv = tableFromCsv(csv)
    if (parsedCsv.error) {
      return { ok: false, error: `Reply rejected — ${parsedCsv.error}` }
    }
    const wb = parseTableWorkbook(storedBefore).workbook
    const next = updateSheetDoc(wb, activeSheet(wb).id, parsedCsv.doc)
    return {
      ok: true,
      content: serializeTableWorkbook(next),
      mode: 'full',
      applied: parsedCsv.doc.rows.length,
    }
  }
  const parsed = timelineFromCsv(csv)
  if (parsed.error) {
    return { ok: false, error: `Reply rejected — ${parsed.error}` }
  }
  if (parsed.doc.milestones.length === 0) {
    return { ok: false, error: 'Reply did not contain any milestone' }
  }
  // The document title belongs to the playground, not to the model reply.
  const next: TimelineDoc = {
    ...parsed.doc,
    title: parseTimelineDoc(storedBefore).doc.title,
  }
  return {
    ok: true,
    content: serializeTimelineDoc(next),
    mode: 'full',
    applied: next.milestones.length,
  }
}
