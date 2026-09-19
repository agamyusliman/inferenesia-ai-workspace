// Minimal XLSX writer. A workbook is a ZIP of XML parts, so we build the parts
// and store them uncompressed — no dependency, and Excel/Sheets/Numbers accept
// stored entries.
//
// Cells carry either a formula (<f>), a number (<v>), or an inline string
// (t="inlineStr"). Inline strings avoid a shared-string table that must stay
// index-consistent with every cell. Styles are deduplicated into styles.xml
// because Excel addresses formatting by index, not by value.

import { columnLetter, isFormula } from './tableFormula'
import {
  getCellStyle,
  normalizeTableDoc,
  type TableCellStyle,
  type TableSheet,
} from './tableDoc'

export { columnLetter }

const CRC_TABLE = (() => {
  const table = new Uint32Array(256)
  for (let i = 0; i < 256; i += 1) {
    let c = i
    for (let k = 0; k < 8; k += 1) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1
    table[i] = c >>> 0
  }
  return table
})()

function crc32(bytes: Uint8Array): number {
  let c = 0xffffffff
  for (let i = 0; i < bytes.length; i += 1) {
    c = CRC_TABLE[(c ^ bytes[i]) & 0xff] ^ (c >>> 8)
  }
  return (c ^ 0xffffffff) >>> 0
}

function xmlEscape(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;')
    // Excel rejects control characters other than tab, newline and carriage return.
    .replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F]/g, '')
}

/** Excel forbids : \ / ? * [ ] in sheet names and caps them at 31 characters. */
export function safeSheetName(name: string, fallback: string): string {
  const cleaned = (name || '').replace(/[:\\/?*[\]]/g, ' ').trim().slice(0, 31)
  return cleaned || fallback
}

/** Column width is measured in characters of the default font, not pixels. */
function pxToCharWidth(px: number): number {
  return Math.round(((px - 5) / 7) * 100) / 100
}

/** Row height is measured in points. */
function pxToPoints(px: number): number {
  return Math.round(px * 0.75 * 100) / 100
}

function isNumericLiteral(text: string): boolean {
  const t = text.trim()
  if (!t) return false
  return /^-?\d*\.?\d+(?:[eE][+-]?\d+)?$/.test(t.replace(/,/g, ''))
}

type StyleTable = {
  index: Map<string, number>
  fonts: string[]
  fills: string[]
  numFmts: string[]
  xfs: string[]
}

function newStyleTable(): StyleTable {
  return {
    index: new Map(),
    // Index 0 must be the default for fonts and fills; Excel additionally
    // reserves fill index 1 for gray125.
    fonts: ['<font><sz val="11"/><name val="Calibri"/></font>'],
    fills: [
      '<fill><patternFill patternType="none"/></fill>',
      '<fill><patternFill patternType="gray125"/></fill>',
    ],
    numFmts: [],
    xfs: ['<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>'],
  }
}

function styleIndex(
  table: StyleTable,
  style: TableCellStyle | undefined,
): number {
  if (!style) return 0
  const key = JSON.stringify([
    style.bold ?? false,
    style.italic ?? false,
    style.color ?? '',
    style.fill ?? '',
    style.align ?? '',
    style.numFmt ?? '',
  ])
  const existing = table.index.get(key)
  if (existing !== undefined) return existing

  let fontId = 0
  if (style.bold || style.italic || style.color) {
    const parts: string[] = []
    if (style.bold) parts.push('<b/>')
    if (style.italic) parts.push('<i/>')
    parts.push('<sz val="11"/>')
    if (style.color) parts.push(`<color rgb="FF${style.color}"/>`)
    parts.push('<name val="Calibri"/>')
    table.fonts.push(`<font>${parts.join('')}</font>`)
    fontId = table.fonts.length - 1
  }

  let fillId = 0
  if (style.fill) {
    table.fills.push(
      `<fill><patternFill patternType="solid"><fgColor rgb="FF${style.fill}"/><bgColor indexed="64"/></patternFill></fill>`,
    )
    fillId = table.fills.length - 1
  }

  let numFmtId = 0
  if (style.numFmt) {
    // Custom format ids must start at 164; lower ids are reserved by Excel.
    numFmtId = 164 + table.numFmts.length
    table.numFmts.push(
      `<numFmt numFmtId="${numFmtId}" formatCode="${xmlEscape(style.numFmt)}"/>`,
    )
  }

  const applies = [
    fontId ? 'applyFont="1"' : '',
    fillId ? 'applyFill="1"' : '',
    numFmtId ? 'applyNumberFormat="1"' : '',
    style.align ? 'applyAlignment="1"' : '',
  ]
    .filter(Boolean)
    .join(' ')
  const alignment = style.align ? `<alignment horizontal="${style.align}"/>` : ''
  table.xfs.push(
    `<xf numFmtId="${numFmtId}" fontId="${fontId}" fillId="${fillId}" borderId="0" xfId="0"${
      applies ? ` ${applies}` : ''
    }>${alignment}</xf>`,
  )
  const id = table.xfs.length - 1
  table.index.set(key, id)
  return id
}

function stylesXml(table: StyleTable): string {
  const numFmts = table.numFmts.length
    ? `<numFmts count="${table.numFmts.length}">${table.numFmts.join('')}</numFmts>`
    : ''
  return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">${numFmts}<fonts count="${table.fonts.length}">${table.fonts.join(
    '',
  )}</fonts><fills count="${table.fills.length}">${table.fills.join(
    '',
  )}</fills><borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="${table.xfs.length}">${table.xfs.join(
    '',
  )}</cellXfs><cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles></styleSheet>`
}

function sheetXml(sheet: TableSheet, styles: StyleTable): string {
  const doc = normalizeTableDoc(sheet.doc)
  const hiddenCols = new Set(doc.hiddenColumns || [])
  const hiddenRows = new Set(doc.hiddenRows || [])

  const colDefs: string[] = []
  doc.columns.forEach((_, i) => {
    const width = doc.widths?.[i]
    const hidden = hiddenCols.has(i)
    if (!width && !hidden) return
    const attrs = [`min="${i + 1}"`, `max="${i + 1}"`]
    if (width) attrs.push(`width="${pxToCharWidth(width)}"`, 'customWidth="1"')
    if (hidden) attrs.push('hidden="1"')
    colDefs.push(`<col ${attrs.join(' ')}/>`)
  })
  const cols = colDefs.length ? `<cols>${colDefs.join('')}</cols>` : ''

  const rowsXml: string[] = []
  const headerStyle = styleIndex(styles, { bold: true })
  const headerCells = doc.columns
    .map(
      (c, i) =>
        `<c r="${columnLetter(i)}1" s="${headerStyle}" t="inlineStr"><is><t xml:space="preserve">${xmlEscape(
          c.name,
        )}</t></is></c>`,
    )
    .join('')
  rowsXml.push(`<row r="1">${headerCells}</row>`)

  doc.rows.forEach((row, r) => {
    const excelRow = r + 2
    const cells = row
      .map((value, c) => {
        const s = styleIndex(styles, getCellStyle(doc, r, c))
        const ref = `${columnLetter(c)}${excelRow}`
        const attrs = s ? ` s="${s}"` : ''
        if (!value) return s ? `<c r="${ref}"${attrs}/>` : ''
        if (isFormula(value)) {
          return `<c r="${ref}"${attrs}><f>${xmlEscape(
            value.trimStart().slice(1),
          )}</f></c>`
        }
        if (isNumericLiteral(value)) {
          return `<c r="${ref}"${attrs}><v>${value
            .trim()
            .replace(/,/g, '')}</v></c>`
        }
        return `<c r="${ref}"${attrs} t="inlineStr"><is><t xml:space="preserve">${xmlEscape(
          value,
        )}</t></is></c>`
      })
      .join('')

    const height = doc.heights?.[r]
    const rowAttrs = [`r="${excelRow}"`]
    if (height) rowAttrs.push(`ht="${pxToPoints(height)}"`, 'customHeight="1"')
    if (hiddenRows.has(r)) rowAttrs.push('hidden="1"')
    rowsXml.push(`<row ${rowAttrs.join(' ')}>${cells}</row>`)
  })

  // Row 1 holds the header, so a document row r maps to spreadsheet row r + 2.
  const merges = (doc.merges || []).map((m) => {
    const from = `${columnLetter(m.col)}${m.row + 2}`
    const to = `${columnLetter(m.col + m.colSpan - 1)}${m.row + m.rowSpan + 1}`
    return `<mergeCell ref="${from}:${to}"/>`
  })
  const mergeXml = merges.length
    ? `<mergeCells count="${merges.length}">${merges.join('')}</mergeCells>`
    : ''

  return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">${cols}<sheetData>${rowsXml.join(
    '',
  )}</sheetData>${mergeXml}</worksheet>`
}

type ZipEntry = { name: string; data: Uint8Array }

function zipEntries(entries: ZipEntry[]): Blob {
  const chunks: Uint8Array[] = []
  const central: Uint8Array[] = []
  let offset = 0

  const u16 = (n: number) => [n & 0xff, (n >>> 8) & 0xff]
  const u32 = (n: number) => [
    n & 0xff,
    (n >>> 8) & 0xff,
    (n >>> 16) & 0xff,
    (n >>> 24) & 0xff,
  ]

  for (const entry of entries) {
    const nameBytes = new TextEncoder().encode(entry.name)
    const crc = crc32(entry.data)
    const size = entry.data.length

    const local = new Uint8Array([
      0x50, 0x4b, 0x03, 0x04,
      ...u16(20), ...u16(0), ...u16(0),
      ...u16(0), ...u16(0),
      ...u32(crc), ...u32(size), ...u32(size),
      ...u16(nameBytes.length), ...u16(0),
    ])
    chunks.push(local, nameBytes, entry.data)

    central.push(
      new Uint8Array([
        0x50, 0x4b, 0x01, 0x02,
        ...u16(20), ...u16(20), ...u16(0), ...u16(0),
        ...u16(0), ...u16(0),
        ...u32(crc), ...u32(size), ...u32(size),
        ...u16(nameBytes.length), ...u16(0), ...u16(0),
        ...u16(0), ...u16(0), ...u32(0),
        ...u32(offset),
      ]),
      nameBytes,
    )
    offset += local.length + nameBytes.length + entry.data.length
  }

  const centralSize = central.reduce((sum, c) => sum + c.length, 0)
  const end = new Uint8Array([
    0x50, 0x4b, 0x05, 0x06,
    ...u16(0), ...u16(0),
    ...u16(entries.length), ...u16(entries.length),
    ...u32(centralSize), ...u32(offset),
    ...u16(0),
  ])

  const parts = [...chunks, ...central, end]
  const total = parts.reduce((sum, p) => sum + p.length, 0)
  const out = new Uint8Array(new ArrayBuffer(total))
  let cursor = 0
  for (const part of parts) {
    out.set(part, cursor)
    cursor += part.length
  }

  return new Blob([out], {
    type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  })
}

export function buildXlsx(sheets: TableSheet[]): Blob {
  const used: string[] = []
  const named = sheets.map((sheet, i) => {
    let name = safeSheetName(sheet.name, `Sheet${i + 1}`)
    let n = 2
    while (used.includes(name.toLowerCase())) {
      name = safeSheetName(`${name.slice(0, 28)} ${n}`, `Sheet${i + 1}`)
      n += 1
    }
    used.push(name.toLowerCase())
    return { sheet, name }
  })

  const styles = newStyleTable()
  const sheetParts = named.map(({ sheet }) => sheetXml(sheet, styles))

  const enc = (s: string) => new TextEncoder().encode(s)

  const contentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>${named
    .map(
      (_, i) =>
        `<Override PartName="/xl/worksheets/sheet${
          i + 1
        }.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`,
    )
    .join('')}</Types>`

  const rootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`

  // fullCalcOnLoad makes Excel evaluate our formulas; we ship no cached values.
  const workbook = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>${named
    .map(
      ({ name }, i) =>
        `<sheet name="${xmlEscape(name)}" sheetId="${i + 1}" r:id="rId${i + 1}"/>`,
    )
    .join('')}</sheets><calcPr calcId="0" fullCalcOnLoad="1"/></workbook>`

  const styleRelId = named.length + 1
  const workbookRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">${named
    .map(
      (_, i) =>
        `<Relationship Id="rId${i + 1}" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet${
          i + 1
        }.xml"/>`,
    )
    .join(
      '',
    )}<Relationship Id="rId${styleRelId}" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`

  return zipEntries([
    { name: '[Content_Types].xml', data: enc(contentTypes) },
    { name: '_rels/.rels', data: enc(rootRels) },
    { name: 'xl/workbook.xml', data: enc(workbook) },
    { name: 'xl/_rels/workbook.xml.rels', data: enc(workbookRels) },
    { name: 'xl/styles.xml', data: enc(stylesXml(styles)) },
    ...sheetParts.map((xml, i) => ({
      name: `xl/worksheets/sheet${i + 1}.xml`,
      data: enc(xml),
    })),
  ])
}

export function downloadXlsx(sheets: TableSheet[], filename: string): void {
  const safe = filename.replace(/[/\\:*?"<>|]+/g, '_')
  const url = URL.createObjectURL(buildXlsx(sheets))
  const a = document.createElement('a')
  a.href = url
  a.download = safe.endsWith('.xlsx') ? safe : `${safe}.xlsx`
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}
