// Table playground document: a boring column/row grid that round-trips through
// JSON (storage) and CSV (import/export/agent replies). No external deps.
//
// Import/parse is strict on purpose: malformed CSV, oversize payloads and
// unknown shapes are reported as errors instead of being silently repaired,
// so the user never sees quietly truncated or invented data.

export type TableColumn = {
  id: string
  name: string
}

export type TableCellStyle = {
  bold?: boolean
  italic?: boolean
  /** #RRGGBB, no leading '#' */
  color?: string
  fill?: string
  align?: 'left' | 'center' | 'right'
  /** Excel format code, e.g. "#,##0" or "0.00%" */
  numFmt?: string
}

export type TableMerge = {
  /** Inclusive zero-based bounds. */
  row: number
  col: number
  rowSpan: number
  colSpan: number
}

export type TableDoc = {
  version: 1
  columns: TableColumn[]
  rows: string[][]
  /** Sparse: only styled cells appear, keyed `${row}:${col}`. */
  styles?: Record<string, TableCellStyle>
  /** Column width in px, index-aligned. */
  widths?: number[]
  /** Row height in px, index-aligned. */
  heights?: number[]
  hiddenColumns?: number[]
  hiddenRows?: number[]
  merges?: TableMerge[]
}

export const TABLE_DOC_VERSION = 1 as const

export const TABLE_MAX_COLUMNS = 64
export const TABLE_MAX_ROWS = 2000

let columnSeq = 0

export function makeColumn(name: string): TableColumn {
  columnSeq += 1
  return { id: `c${Date.now().toString(36)}${columnSeq.toString(36)}`, name }
}

/** New tables start with named columns and blank rows — never sample data. */
export function emptyTableDoc(): TableDoc {
  return {
    version: TABLE_DOC_VERSION,
    columns: [makeColumn('Column 1'), makeColumn('Column 2'), makeColumn('Column 3')],
    rows: [
      ['', '', ''],
      ['', '', ''],
      ['', '', ''],
    ],
  }
}

/** Size complaint for a candidate document, or null when it fits. */
export function tableSizeError(
  columns: number,
  rows: number,
): string | null {
  if (columns > TABLE_MAX_COLUMNS) {
    return `Too many columns: ${columns} (max ${TABLE_MAX_COLUMNS})`
  }
  if (rows > TABLE_MAX_ROWS) {
    return `Too many rows: ${rows} (max ${TABLE_MAX_ROWS})`
  }
  return null
}

/**
 * Squares off an already size-checked document: every row gets exactly one
 * cell per column. Never drops columns or rows — callers validate size first.
 */
export function normalizeTableDoc(doc: TableDoc): TableDoc {
  const columns = doc.columns.map((c, i) => ({
    id: c.id || makeColumn('').id,
    name: typeof c.name === 'string' ? c.name : `Column ${i + 1}`,
  }))
  if (columns.length === 0) columns.push(makeColumn('Column 1'))
  const rows = doc.rows.map((row) => {
    const out = new Array<string>(columns.length)
    for (let i = 0; i < columns.length; i += 1) {
      const v = row[i]
      out[i] = typeof v === 'string' ? v : v == null ? '' : String(v)
    }
    return out
  })

  const next: TableDoc = { version: TABLE_DOC_VERSION, columns, rows }

  const styles = normalizeStyles(doc.styles, rows.length, columns.length)
  if (styles) next.styles = styles
  const widths = normalizeSizes(doc.widths, columns.length)
  if (widths) next.widths = widths
  const heights = normalizeSizes(doc.heights, rows.length)
  if (heights) next.heights = heights
  const hiddenColumns = normalizeHidden(doc.hiddenColumns, columns.length)
  if (hiddenColumns.length && columns.length - hiddenColumns.length >= 1) {
    next.hiddenColumns = hiddenColumns
  }
  const hiddenRows = normalizeHidden(doc.hiddenRows, rows.length)
  if (hiddenRows.length && hiddenRows.length < rows.length) {
    next.hiddenRows = hiddenRows
  }
  const merges = normalizeMerges(doc.merges, rows.length, columns.length)
  if (merges.length) next.merges = merges

  return next
}

function normalizeStyles(
  styles: TableDoc['styles'],
  rowCount: number,
  colCount: number,
): Record<string, TableCellStyle> | undefined {
  if (!styles) return undefined
  const out: Record<string, TableCellStyle> = {}
  for (const [key, value] of Object.entries(styles)) {
    const [r, c] = key.split(':').map((n) => Number(n))
    if (!Number.isInteger(r) || !Number.isInteger(c)) continue
    if (r < 0 || r >= rowCount || c < 0 || c >= colCount) continue
    const clean = cleanCellStyle(value)
    if (clean) out[`${r}:${c}`] = clean
  }
  return Object.keys(out).length ? out : undefined
}

function cleanCellStyle(style: TableCellStyle | undefined): TableCellStyle | null {
  if (!style || typeof style !== 'object') return null
  const out: TableCellStyle = {}
  if (style.bold) out.bold = true
  if (style.italic) out.italic = true
  const color = normalizeHexColor(style.color)
  if (color) out.color = color
  const fill = normalizeHexColor(style.fill)
  if (fill) out.fill = fill
  if (style.align === 'left' || style.align === 'center' || style.align === 'right') {
    out.align = style.align
  }
  if (typeof style.numFmt === 'string' && style.numFmt.trim()) {
    out.numFmt = style.numFmt.trim().slice(0, 64)
  }
  return Object.keys(out).length ? out : null
}

/** Accepts `#rgb`, `#rrggbb` or bare `rrggbb` and stores bare uppercase `RRGGBB`. */
export function normalizeHexColor(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const raw = value.trim().replace(/^#/, '')
  if (/^[0-9a-f]{3}$/i.test(raw)) {
    return raw
      .split('')
      .map((ch) => ch + ch)
      .join('')
      .toUpperCase()
  }
  if (/^[0-9a-f]{6}$/i.test(raw)) return raw.toUpperCase()
  return undefined
}

function normalizeSizes(
  sizes: number[] | undefined,
  count: number,
): number[] | undefined {
  if (!Array.isArray(sizes)) return undefined
  const out = new Array<number>(count).fill(0)
  let any = false
  for (let i = 0; i < count; i += 1) {
    const v = sizes[i]
    if (typeof v === 'number' && Number.isFinite(v) && v > 0) {
      out[i] = Math.round(Math.min(v, 2000))
      any = true
    }
  }
  return any ? out : undefined
}

function normalizeHidden(
  hidden: number[] | undefined,
  count: number,
): number[] {
  if (!Array.isArray(hidden)) return []
  const set = new Set<number>()
  for (const v of hidden) {
    if (Number.isInteger(v) && v >= 0 && v < count) set.add(v)
  }
  return [...set].sort((a, b) => a - b)
}

function normalizeMerges(
  merges: TableMerge[] | undefined,
  rowCount: number,
  colCount: number,
): TableMerge[] {
  if (!Array.isArray(merges)) return []
  const accepted: TableMerge[] = []
  for (const m of merges) {
    if (!m || typeof m !== 'object') continue
    const row = Number(m.row)
    const col = Number(m.col)
    const rowSpan = Number(m.rowSpan)
    const colSpan = Number(m.colSpan)
    if (![row, col, rowSpan, colSpan].every(Number.isInteger)) continue
    if (row < 0 || col < 0 || rowSpan < 1 || colSpan < 1) continue
    if (row + rowSpan > rowCount || col + colSpan > colCount) continue
    if (rowSpan === 1 && colSpan === 1) continue
    const overlaps = accepted.some(
      (o) =>
        row < o.row + o.rowSpan &&
        o.row < row + rowSpan &&
        col < o.col + o.colSpan &&
        o.col < col + colSpan,
    )
    if (overlaps) continue
    accepted.push({ row, col, rowSpan, colSpan })
  }
  return accepted
}

export function cellStyleKey(row: number, col: number): string {
  return `${row}:${col}`
}

export function getCellStyle(
  doc: TableDoc,
  row: number,
  col: number,
): TableCellStyle | undefined {
  return doc.styles?.[cellStyleKey(row, col)]
}

export function isHiddenColumn(doc: TableDoc, col: number): boolean {
  return Boolean(doc.hiddenColumns?.includes(col))
}

export function mergeAt(
  doc: TableDoc,
  row: number,
  col: number,
): TableMerge | undefined {
  return doc.merges?.find(
    (m) =>
      row >= m.row &&
      row < m.row + m.rowSpan &&
      col >= m.col &&
      col < m.col + m.colSpan,
  )
}

export function serializeTableDoc(doc: TableDoc): string {
  return `${JSON.stringify(normalizeTableDoc(doc), null, 2)}\n`
}

export type TableParseResult = {
  doc: TableDoc
  error?: string
}

/**
 * Reads a stored document. JSON is the storage format; CSV text is accepted so
 * pasted or agent-produced content opens. A parse failure keeps the raw bytes
 * untouched on disk and surfaces the reason — it never overwrites with a blank.
 */
export function parseTableDoc(raw: string): TableParseResult {
  const text = (raw || '').trim()
  if (!text) return { doc: emptyTableDoc() }
  if (text.startsWith('{') || text.startsWith('[')) {
    let parsed: unknown
    try {
      parsed = JSON.parse(text)
    } catch (e) {
      return {
        doc: emptyTableDoc(),
        error: e instanceof Error ? e.message : 'Invalid table JSON',
      }
    }
    const doc = tableDocFromUnknown(parsed)
    if (!doc) {
      return { doc: emptyTableDoc(), error: 'Unrecognized table JSON shape' }
    }
    const sizeError = tableSizeError(doc.columns.length, doc.rows.length)
    if (sizeError) return { doc: emptyTableDoc(), error: sizeError }
    return { doc: normalizeTableDoc({ ...doc, ...presentationFromUnknown(parsed) }) }
  }
  return tableFromCsv(text)
}

function tableDocFromUnknown(value: unknown): TableDoc | null {
  if (Array.isArray(value)) {
    // [["h1","h2"],["a","b"]] or [{col: val}, …]
    if (value.length === 0) return emptyTableDoc()
    if (Array.isArray(value[0])) {
      const grid = value as unknown[][]
      const header = grid[0].map((c) => String(c ?? ''))
      return {
        version: TABLE_DOC_VERSION,
        columns: header.map((h, i) => makeColumn(h || `Column ${i + 1}`)),
        rows: grid.slice(1).map((r) => r.map((c) => String(c ?? ''))),
      }
    }
    if (typeof value[0] === 'object' && value[0] !== null) {
      const objects = value as Array<Record<string, unknown>>
      const names: string[] = []
      for (const o of objects) {
        for (const k of Object.keys(o)) if (!names.includes(k)) names.push(k)
      }
      return {
        version: TABLE_DOC_VERSION,
        columns: names.map((n) => makeColumn(n)),
        rows: objects.map((o) =>
          names.map((n) => (o[n] == null ? '' : String(o[n]))),
        ),
      }
    }
    return null
  }
  if (!value || typeof value !== 'object') return null
  const o = value as Record<string, unknown>
  const rawColumns = o.columns
  const rawRows = o.rows
  if (!Array.isArray(rawColumns)) return null
  const columns = rawColumns.map((c, i) => {
    if (typeof c === 'string') return makeColumn(c)
    if (c && typeof c === 'object') {
      const cc = c as Record<string, unknown>
      return {
        id: typeof cc.id === 'string' && cc.id ? cc.id : makeColumn('').id,
        name: String(cc.name ?? cc.title ?? `Column ${i + 1}`),
      }
    }
    return makeColumn(`Column ${i + 1}`)
  })
  const rows: string[][] = []
  if (Array.isArray(rawRows)) {
    for (const r of rawRows) {
      if (Array.isArray(r)) {
        rows.push(r.map((c) => (c == null ? '' : String(c))))
      } else if (r && typeof r === 'object') {
        const rr = r as Record<string, unknown>
        rows.push(
          columns.map((col) => {
            const v = rr[col.name] ?? rr[col.id]
            return v == null ? '' : String(v)
          }),
        )
      }
    }
  }
  return { version: TABLE_DOC_VERSION, columns, rows }
}

/**
 * Pulls the presentation fields off a raw sheet object. They are read here
 * rather than in normalizeTableDoc because normalizeTableDoc is what validates
 * them, and by then they must already be attached to the document.
 */
function presentationFromUnknown(value: unknown): Partial<TableDoc> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  const o = value as Record<string, unknown>
  const out: Partial<TableDoc> = {}
  if (o.styles && typeof o.styles === 'object' && !Array.isArray(o.styles)) {
    out.styles = o.styles as Record<string, TableCellStyle>
  }
  if (Array.isArray(o.widths)) out.widths = o.widths as number[]
  if (Array.isArray(o.heights)) out.heights = o.heights as number[]
  if (Array.isArray(o.hiddenColumns)) out.hiddenColumns = o.hiddenColumns as number[]
  if (Array.isArray(o.hiddenRows)) out.hiddenRows = o.hiddenRows as number[]
  if (Array.isArray(o.merges)) out.merges = o.merges as TableMerge[]
  return out
}

export type CsvParseResult = {
  rows: string[][]
  error?: string
}

/**
 * RFC4180 reader: quoted fields, doubled-quote escapes, CRLF, embedded
 * newlines. An unterminated quoted field or stray quote inside an unquoted
 * field is a hard error — silently accepting it corrupts every later column.
 */
export function parseCsv(text: string): CsvParseResult {
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
    return { rows: [], error: `Unterminated quoted field starting before line ${line}` }
  }
  if (field.length > 0 || row.length > 0) endRow()
  // A file ending in a newline leaves one empty trailing record; drop it.
  while (rows.length > 0) {
    const last = rows[rows.length - 1]
    if (last.length === 1 && last[0] === '') rows.pop()
    else break
  }
  return { rows }
}

function csvCell(value: string): string {
  const v = value ?? ''
  return /[",\r\n]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v
}

export function toCsv(doc: TableDoc): string {
  const d = normalizeTableDoc(doc)
  const lines = [d.columns.map((c) => csvCell(c.name)).join(',')]
  for (const row of d.rows) lines.push(row.map(csvCell).join(','))
  return `${lines.join('\n')}\n`
}

/** Strict CSV → document. Reports malformed and oversize input as errors. */
export function tableFromCsv(
  text: string,
  opts?: { headerRow?: boolean },
): TableParseResult {
  const parsed = parseCsv(text)
  if (parsed.error) return { doc: emptyTableDoc(), error: parsed.error }
  const grid = parsed.rows
  if (grid.length === 0) {
    return { doc: emptyTableDoc(), error: 'No rows found' }
  }
  const headerRow = opts?.headerRow !== false
  const width = grid.reduce((max, r) => Math.max(max, r.length), 0) || 1
  const body = headerRow ? grid.slice(1) : grid
  const sizeError = tableSizeError(width, body.length)
  if (sizeError) return { doc: emptyTableDoc(), error: sizeError }
  const header = headerRow ? grid[0] : []
  const columns: TableColumn[] = []
  for (let i = 0; i < width; i += 1) {
    const name = (header[i] || '').trim()
    columns.push(makeColumn(name || `Column ${i + 1}`))
  }
  return {
    doc: normalizeTableDoc({
      version: TABLE_DOC_VERSION,
      columns,
      rows: body,
    }),
  }
}

export function tableToMarkdown(doc: TableDoc): string {
  const d = normalizeTableDoc(doc)
  const esc = (v: string) => (v || '').replace(/\|/g, '\\|').replace(/\n/g, ' ')
  const head = `| ${d.columns.map((c) => esc(c.name)).join(' | ')} |`
  const sep = `| ${d.columns.map(() => '---').join(' | ')} |`
  const body = d.rows.map((r) => `| ${r.map(esc).join(' | ')} |`)
  return [head, sep, ...body].join('\n') + '\n'
}

/**
 * Column order expressed as old indices: `order[newIndex] = oldIndex`.
 * A negative entry means a column was inserted there and has no source.
 * Rewrites every index-addressed field (styles, widths, hidden, merges) so
 * structural edits cannot silently shift metadata onto the wrong cell.
 */
function applyColumnOrder(
  d: TableDoc,
  order: number[],
  columns: TableColumn[],
  rows: string[][],
): TableDoc {
  const position = new Map<number, number>()
  order.forEach((old, next) => {
    if (old >= 0) position.set(old, next)
  })

  const widths = d.widths
    ? order.map((old) => (old >= 0 ? d.widths![old] || 0 : 0))
    : undefined
  const hiddenColumns = (d.hiddenColumns || [])
    .map((old) => position.get(old))
    .filter((v): v is number => v !== undefined)

  const styles: Record<string, TableCellStyle> = {}
  for (const [key, value] of Object.entries(d.styles || {})) {
    const [r, c] = key.split(':').map(Number)
    const nextCol = position.get(c)
    if (nextCol !== undefined) styles[`${r}:${nextCol}`] = value
  }

  const merges = (d.merges || [])
    .map((m) => ({ ...m, col: position.get(m.col) ?? -1 }))
    .filter((m) => m.col >= 0)

  return normalizeTableDoc({
    ...d,
    columns,
    rows,
    widths,
    hiddenColumns,
    styles,
    merges,
  })
}

/**
 * Rewrites the row numbers inside a formula after rows are reordered.
 * `position[oldRow] = newRow`; a row that no longer exists becomes #REF!.
 * Without this a moved total keeps pointing at whatever now sits in the old
 * range, which is how a sum silently starts including itself.
 *
 * Range endpoints are re-sorted afterwards: remapping can invert `A2:A3` into
 * `A4:A2`, and a spreadsheet reads that as the whole band between them — which
 * would swallow rows the author never referenced.
 */
function remapFormulaRows(
  expr: string,
  position: Map<number, number>,
): string {
  const rewritten = rewriteRefs(expr, position)
  return sortRangeEndpoints(rewritten)
}

function rewriteRefs(expr: string, position: Map<number, number>): string {
  let out = ''
  let i = 0
  while (i < expr.length) {
    const ch = expr[i]
    if (ch === '"') {
      out += ch
      i += 1
      while (i < expr.length) {
        out += expr[i]
        if (expr[i] === '"' && expr[i + 1] === '"') {
          out += expr[i + 1]
          i += 2
          continue
        }
        if (expr[i] === '"') {
          i += 1
          break
        }
        i += 1
      }
      continue
    }
    const ref = /^(\$?)([A-Z]{1,3})(\$?)(\d{1,7})/i.exec(expr.slice(i))
    if (ref && expr[i + ref[0].length] !== '(') {
      const [match, colAbs, col, rowAbs, rowText] = ref
      const dataRow = Number(rowText) - 2
      const nextRow = position.get(dataRow)
      if (nextRow === undefined) {
        out += dataRow < 0 ? match : '#REF!'
      } else {
        out += `${colAbs}${col}${rowAbs}${nextRow + 2}`
      }
      i += match.length
      continue
    }
    const word = /^[A-Z_][A-Z0-9_.]*/i.exec(expr.slice(i))
    if (word) {
      out += word[0]
      i += word[0].length
      continue
    }
    out += ch
    i += 1
  }
  return out
}

function sortRangeEndpoints(expr: string): string {
  return expr.replace(
    /(\$?)([A-Z]{1,3})(\$?)(\d{1,7}):(\$?)([A-Z]{1,3})(\$?)(\d{1,7})/gi,
    (match, c1Abs, c1, r1Abs, r1, c2Abs, c2, r2Abs, r2) => {
      const a = Number(r1)
      const b = Number(r2)
      if (a <= b) return match
      return `${c1Abs}${c1}${r1Abs}${b}:${c2Abs}${c2}${r2Abs}${a}`
    },
  )
}

/** Row counterpart of applyColumnOrder. */
function applyRowOrder(
  d: TableDoc,
  order: number[],
  rows: string[][],
): TableDoc {
  const position = new Map<number, number>()
  order.forEach((old, next) => {
    if (old >= 0) position.set(old, next)
  })

  const remapped = rows.map((row) =>
    row.map((cell) =>
      cell.trimStart().startsWith('=')
        ? `=${remapFormulaRows(cell.trimStart().slice(1), position)}`
        : cell,
    ),
  )

  const heights = d.heights
    ? order.map((old) => (old >= 0 ? d.heights![old] || 0 : 0))
    : undefined
  const hiddenRows = (d.hiddenRows || [])
    .map((old) => position.get(old))
    .filter((v): v is number => v !== undefined)

  const styles: Record<string, TableCellStyle> = {}
  for (const [key, value] of Object.entries(d.styles || {})) {
    const [r, c] = key.split(':').map(Number)
    const nextRow = position.get(r)
    if (nextRow !== undefined) styles[`${nextRow}:${c}`] = value
  }

  const merges = (d.merges || [])
    .map((m) => ({ ...m, row: position.get(m.row) ?? -1 }))
    .filter((m) => m.row >= 0)

  return normalizeTableDoc({
    ...d,
    rows: remapped,
    heights,
    hiddenRows,
    styles,
    merges,
  })
}

export function addTableRow(doc: TableDoc, at?: number): TableDoc {
  const d = normalizeTableDoc(doc)
  if (d.rows.length >= TABLE_MAX_ROWS) return d
  const index =
    at == null ? d.rows.length : Math.max(0, Math.min(at, d.rows.length))
  const order = d.rows.map((_, i) => i)
  order.splice(index, 0, -1)
  const rows = d.rows.slice()
  rows.splice(index, 0, d.columns.map(() => ''))
  return applyRowOrder(d, order, rows)
}

export function removeTableRow(doc: TableDoc, index: number): TableDoc {
  const d = normalizeTableDoc(doc)
  if (index < 0 || index >= d.rows.length) return d
  if (d.rows.length <= 1) return d
  const rows = d.rows.slice()
  rows.splice(index, 1)
  const order = d.rows.map((_, i) => i).filter((i) => i !== index)
  return applyRowOrder(d, order, rows)
}

export function addTableColumn(
  doc: TableDoc,
  at?: number,
  name?: string,
): TableDoc {
  const d = normalizeTableDoc(doc)
  if (d.columns.length >= TABLE_MAX_COLUMNS) return d
  const index =
    at == null ? d.columns.length : Math.max(0, Math.min(at, d.columns.length))
  const order = d.columns.map((_, i) => i)
  order.splice(index, 0, -1)
  const columns = d.columns.slice()
  columns.splice(index, 0, makeColumn(name || `Column ${d.columns.length + 1}`))
  const rows = d.rows.map((r) => {
    const next = r.slice()
    next.splice(index, 0, '')
    return next
  })
  return applyColumnOrder(d, order, columns, rows)
}

export function removeTableColumn(doc: TableDoc, index: number): TableDoc {
  const d = normalizeTableDoc(doc)
  if (d.columns.length <= 1) return d
  if (index < 0 || index >= d.columns.length) return d
  const columns = d.columns.slice()
  columns.splice(index, 1)
  const rows = d.rows.map((r) => {
    const next = r.slice()
    next.splice(index, 1)
    return next
  })
  const order = d.columns.map((_, i) => i).filter((i) => i !== index)
  return applyColumnOrder(d, order, columns, rows)
}

export function renameTableColumn(
  doc: TableDoc,
  index: number,
  name: string,
): TableDoc {
  const d = normalizeTableDoc(doc)
  if (index < 0 || index >= d.columns.length) return d
  const columns = d.columns.slice()
  columns[index] = { ...columns[index], name }
  return { ...d, columns }
}

export function setTableCell(
  doc: TableDoc,
  rowIndex: number,
  colIndex: number,
  value: string,
): TableDoc {
  const d = normalizeTableDoc(doc)
  if (rowIndex < 0 || rowIndex >= d.rows.length) return d
  if (colIndex < 0 || colIndex >= d.columns.length) return d
  const rows = d.rows.slice()
  const row = rows[rowIndex].slice()
  row[colIndex] = value
  rows[rowIndex] = row
  return { ...d, rows }
}

export function setCellStyles(
  doc: TableDoc,
  targets: Array<{ row: number; col: number }>,
  patch: TableCellStyle | null,
): TableDoc {
  const d = normalizeTableDoc(doc)
  const styles: Record<string, TableCellStyle> = { ...(d.styles || {}) }
  for (const { row, col } of targets) {
    if (row < 0 || row >= d.rows.length) continue
    if (col < 0 || col >= d.columns.length) continue
    const key = cellStyleKey(row, col)
    if (patch === null) {
      delete styles[key]
      continue
    }
    const merged = cleanCellStyle({ ...styles[key], ...patch })
    if (merged) styles[key] = merged
    else delete styles[key]
  }
  const next: TableDoc = { ...d }
  if (Object.keys(styles).length) next.styles = styles
  else delete next.styles
  return next
}

export function setColumnWidth(
  doc: TableDoc,
  col: number,
  width: number,
): TableDoc {
  const d = normalizeTableDoc(doc)
  if (col < 0 || col >= d.columns.length) return d
  const widths = d.widths ? d.widths.slice() : new Array<number>(d.columns.length).fill(0)
  widths[col] = Math.round(Math.max(48, Math.min(width, 2000)))
  return { ...d, widths }
}

export function setRowHeight(doc: TableDoc, row: number, height: number): TableDoc {
  const d = normalizeTableDoc(doc)
  if (row < 0 || row >= d.rows.length) return d
  const heights = d.heights ? d.heights.slice() : new Array<number>(d.rows.length).fill(0)
  heights[row] = Math.round(Math.max(20, Math.min(height, 800)))
  return { ...d, heights }
}

export function toggleColumnHidden(doc: TableDoc, col: number): TableDoc {
  const d = normalizeTableDoc(doc)
  if (col < 0 || col >= d.columns.length) return d
  const hidden = new Set(d.hiddenColumns || [])
  if (hidden.has(col)) hidden.delete(col)
  else if (d.columns.length - hidden.size > 1) hidden.add(col)
  return normalizeTableDoc({ ...d, hiddenColumns: [...hidden] })
}

export function toggleRowHidden(doc: TableDoc, row: number): TableDoc {
  const d = normalizeTableDoc(doc)
  if (row < 0 || row >= d.rows.length) return d
  const hidden = new Set(d.hiddenRows || [])
  if (hidden.has(row)) hidden.delete(row)
  else if (d.rows.length - hidden.size > 1) hidden.add(row)
  return normalizeTableDoc({ ...d, hiddenRows: [...hidden] })
}

export function showAllColumns(doc: TableDoc): TableDoc {
  const d = normalizeTableDoc(doc)
  const next = { ...d }
  delete next.hiddenColumns
  return next
}

export function moveRow(doc: TableDoc, from: number, to: number): TableDoc {
  const d = normalizeTableDoc(doc)
  if (from === to) return d
  if (from < 0 || from >= d.rows.length) return d
  const target = Math.max(0, Math.min(to, d.rows.length - 1))

  const order = d.rows.map((_, i) => i)
  const [movedIndex] = order.splice(from, 1)
  order.splice(target, 0, movedIndex)

  const rows = order.map((i) => d.rows[i])
  return applyRowOrder(d, order, rows)
}

export function moveColumn(doc: TableDoc, from: number, to: number): TableDoc {
  const d = normalizeTableDoc(doc)
  if (from === to) return d
  if (from < 0 || from >= d.columns.length) return d
  const target = Math.max(0, Math.min(to, d.columns.length - 1))

  const order = d.columns.map((_, i) => i)
  const [movedIndex] = order.splice(from, 1)
  order.splice(target, 0, movedIndex)

  const columns = order.map((i) => d.columns[i])
  const rows = d.rows.map((row) => order.map((i) => row[i]))
  return applyColumnOrder(d, order, columns, rows)
}

export function mergeCells(
  doc: TableDoc,
  row: number,
  col: number,
  rowSpan: number,
  colSpan: number,
): TableDoc {
  const d = normalizeTableDoc(doc)
  if (rowSpan < 1 || colSpan < 1) return d
  if (rowSpan === 1 && colSpan === 1) return unmergeCells(d, row, col)

  const first = d.rows[row]?.[col] ?? ''
  const rows = d.rows.map((r) => r.slice())
  for (let r = row; r < row + rowSpan && r < rows.length; r += 1) {
    for (let c = col; c < col + colSpan && c < rows[r].length; c += 1) {
      if (r === row && c === col) continue
      rows[r][c] = ''
    }
  }
  rows[row][col] = first

  return normalizeTableDoc({
    ...d,
    rows,
    merges: [...(d.merges || []), { row, col, rowSpan, colSpan }],
  })
}

export function unmergeCells(doc: TableDoc, row: number, col: number): TableDoc {
  const d = normalizeTableDoc(doc)
  const merges = (d.merges || []).filter(
    (m) => !(m.row === row && m.col === col),
  )
  return normalizeTableDoc({ ...d, merges })
}

export type TableSheet = {  id: string
  name: string
  doc: TableDoc
}

export type TableWorkbook = {
  version: 1
  sheets: TableSheet[]
  activeSheetId: string
}

export const TABLE_MAX_SHEETS = 24

let sheetSeq = 0

export function makeSheet(name: string, doc?: TableDoc): TableSheet {
  sheetSeq += 1
  return {
    id: `s${Date.now().toString(36)}${sheetSeq.toString(36)}`,
    name,
    doc: doc ? normalizeTableDoc(doc) : emptyTableDoc(),
  }
}

export function emptyTableWorkbook(): TableWorkbook {
  const sheet = makeSheet('Sheet 1')
  return { version: TABLE_DOC_VERSION, sheets: [sheet], activeSheetId: sheet.id }
}

function uniqueSheetName(taken: string[], desired: string): string {
  const base = (desired || '').trim() || 'Sheet'
  if (!taken.includes(base)) return base
  for (let n = 2; n < 1000; n += 1) {
    const candidate = `${base} ${n}`
    if (!taken.includes(candidate)) return candidate
  }
  return `${base} ${Date.now().toString(36)}`
}

export function normalizeTableWorkbook(wb: TableWorkbook): TableWorkbook {
  const taken: string[] = []
  const sheets = wb.sheets.slice(0, TABLE_MAX_SHEETS).map((s, i) => {
    const name = uniqueSheetName(taken, s.name || `Sheet ${i + 1}`)
    taken.push(name)
    return {
      id: s.id || makeSheet(name).id,
      name,
      doc: normalizeTableDoc(s.doc),
    }
  })
  if (sheets.length === 0) sheets.push(makeSheet('Sheet 1'))
  const active = sheets.some((s) => s.id === wb.activeSheetId)
    ? wb.activeSheetId
    : sheets[0].id
  return { version: TABLE_DOC_VERSION, sheets, activeSheetId: active }
}

export function serializeTableWorkbook(wb: TableWorkbook): string {
  return `${JSON.stringify(normalizeTableWorkbook(wb), null, 2)}\n`
}

export type TableWorkbookParseResult = {
  workbook: TableWorkbook
  error?: string
}

/** Also accepts a pre-multi-sheet TableDoc, loading it as a one-sheet workbook. */
export function parseTableWorkbook(raw: string): TableWorkbookParseResult {
  const text = (raw || '').trim()
  if (!text) return { workbook: emptyTableWorkbook() }
  if (text.startsWith('{')) {
    let parsed: unknown
    try {
      parsed = JSON.parse(text)
    } catch {
      const legacy = parseTableDoc(text)
      return legacy.error
        ? { workbook: emptyTableWorkbook(), error: legacy.error }
        : { workbook: workbookFromDoc(legacy.doc) }
    }
    const o = parsed as Record<string, unknown>
    if (Array.isArray(o?.sheets)) {
      const sheets: TableSheet[] = []
      for (const [i, raw] of (o.sheets as unknown[]).entries()) {
        if (!raw || typeof raw !== 'object') continue
        const s = raw as Record<string, unknown>
        const doc = tableDocFromUnknown(s.doc ?? s)
        if (!doc) continue
        const sizeError = tableSizeError(doc.columns.length, doc.rows.length)
        if (sizeError) return { workbook: emptyTableWorkbook(), error: sizeError }
        sheets.push({
          id: typeof s.id === 'string' && s.id ? s.id : makeSheet('').id,
          name: String(s.name ?? `Sheet ${i + 1}`),
          doc: normalizeTableDoc({ ...doc, ...presentationFromUnknown(s.doc ?? s) }),
        })
      }
      if (sheets.length === 0) {
        return { workbook: emptyTableWorkbook(), error: 'Workbook has no readable sheet' }
      }
      return {
        workbook: normalizeTableWorkbook({
          version: TABLE_DOC_VERSION,
          sheets,
          activeSheetId:
            typeof o.activeSheetId === 'string' ? o.activeSheetId : sheets[0].id,
        }),
      }
    }
  }
  const legacy = parseTableDoc(text)
  if (legacy.error) return { workbook: emptyTableWorkbook(), error: legacy.error }
  return { workbook: workbookFromDoc(legacy.doc) }
}

export function workbookFromDoc(doc: TableDoc, name = 'Sheet 1'): TableWorkbook {
  const sheet = makeSheet(name, doc)
  return { version: TABLE_DOC_VERSION, sheets: [sheet], activeSheetId: sheet.id }
}

export function activeSheet(wb: TableWorkbook): TableSheet {
  return wb.sheets.find((s) => s.id === wb.activeSheetId) || wb.sheets[0]
}

export function updateSheetDoc(
  wb: TableWorkbook,
  sheetId: string,
  next: TableDoc,
): TableWorkbook {
  return normalizeTableWorkbook({
    ...wb,
    sheets: wb.sheets.map((s) =>
      s.id === sheetId ? { ...s, doc: normalizeTableDoc(next) } : s,
    ),
  })
}

export function addSheet(wb: TableWorkbook, name?: string): TableWorkbook {
  if (wb.sheets.length >= TABLE_MAX_SHEETS) return wb
  const sheet = makeSheet(
    uniqueSheetName(wb.sheets.map((s) => s.name), name || `Sheet ${wb.sheets.length + 1}`),
  )
  return normalizeTableWorkbook({
    ...wb,
    sheets: [...wb.sheets, sheet],
    activeSheetId: sheet.id,
  })
}

export function removeSheet(wb: TableWorkbook, sheetId: string): TableWorkbook {
  if (wb.sheets.length <= 1) return wb
  const index = wb.sheets.findIndex((s) => s.id === sheetId)
  if (index < 0) return wb
  const sheets = wb.sheets.filter((s) => s.id !== sheetId)
  const nextActive =
    wb.activeSheetId === sheetId
      ? sheets[Math.min(index, sheets.length - 1)].id
      : wb.activeSheetId
  return normalizeTableWorkbook({ ...wb, sheets, activeSheetId: nextActive })
}

export function renameSheet(
  wb: TableWorkbook,
  sheetId: string,
  name: string,
): TableWorkbook {
  return normalizeTableWorkbook({
    ...wb,
    sheets: wb.sheets.map((s) => (s.id === sheetId ? { ...s, name } : s)),
  })
}

export function setActiveSheet(wb: TableWorkbook, sheetId: string): TableWorkbook {
  if (!wb.sheets.some((s) => s.id === sheetId)) return wb
  return { ...wb, activeSheetId: sheetId }
}
