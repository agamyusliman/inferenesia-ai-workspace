import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import { buildXlsx, columnLetter, safeSheetName } from './xlsx'
import { evaluateCell } from './tableFormula'
import {
  activeSheet,
  addSheet,
  addTableRow,
  emptyTableWorkbook,
  makeSheet,
  mergeAt,
  mergeCells,
  moveColumn,
  moveRow,
  normalizeTableDoc,
  parseTableWorkbook,
  removeSheet,
  removeTableColumn,
  removeTableRow,
  serializeTableDoc,
  serializeTableWorkbook,
  setCellStyles,
  setColumnWidth,
  setRowHeight,
  tableFromCsv,
  toggleColumnHidden,
  unmergeCells,
  updateSheetDoc,
} from './tableDoc'

describe('columnLetter', () => {
  it('maps indices to spreadsheet column names', () => {
    assert.equal(columnLetter(0), 'A')
    assert.equal(columnLetter(25), 'Z')
    assert.equal(columnLetter(26), 'AA')
    assert.equal(columnLetter(51), 'AZ')
    assert.equal(columnLetter(701), 'ZZ')
  })
})

describe('safeSheetName', () => {
  it('strips the characters Excel rejects and caps length', () => {
    assert.equal(safeSheetName('Q1/Q2: sales', 'Sheet1'), 'Q1 Q2  sales')
    assert.equal(safeSheetName('   ', 'Sheet1'), 'Sheet1')
    assert.equal(safeSheetName('x'.repeat(50), 'Sheet1').length, 31)
  })
})

describe('table workbook', () => {
  it('loads a pre-multi-sheet document as one sheet instead of discarding it', () => {
    const legacy = serializeTableDoc(tableFromCsv('a,b\n1,2\n').doc)
    const { workbook, error } = parseTableWorkbook(legacy)
    assert.equal(error, undefined)
    assert.equal(workbook.sheets.length, 1)
    assert.deepEqual(activeSheet(workbook).doc.rows, [['1', '2']])
  })

  it('round-trips through serialize and parse', () => {
    const wb = addSheet(emptyTableWorkbook(), 'Totals')
    const text = serializeTableWorkbook(wb)
    const back = parseTableWorkbook(text).workbook
    assert.equal(back.sheets.length, 2)
    assert.equal(back.sheets[1].name, 'Totals')
    assert.equal(back.activeSheetId, wb.activeSheetId)
  })

  it('never leaves the workbook without a sheet or a valid active id', () => {
    const wb = emptyTableWorkbook()
    assert.equal(removeSheet(wb, wb.sheets[0].id).sheets.length, 1)

    const two = addSheet(wb, 'Second')
    const afterRemove = removeSheet(two, two.activeSheetId)
    assert.equal(afterRemove.sheets.length, 1)
    assert.ok(
      afterRemove.sheets.some((s) => s.id === afterRemove.activeSheetId),
      'active id must address a surviving sheet',
    )
  })

  it('gives duplicate sheet names distinct labels', () => {
    const wb = addSheet(addSheet(emptyTableWorkbook(), 'Data'), 'Data')
    const names = wb.sheets.map((s) => s.name)
    assert.equal(new Set(names).size, names.length)
  })

  it('edits only the addressed sheet', () => {
    const wb = addSheet(emptyTableWorkbook(), 'Second')
    const target = wb.sheets[0]
    const next = updateSheetDoc(wb, target.id, tableFromCsv('x\n9\n').doc)
    assert.deepEqual(next.sheets[0].doc.rows, [['9']])
    assert.deepEqual(next.sheets[1].doc, wb.sheets[1].doc)
  })
})

describe('structural edits keep metadata on the right cell', () => {
  const styled = () =>
    normalizeTableDoc({
      version: 1,
      columns: [
        { id: 'a', name: 'A' },
        { id: 'b', name: 'B' },
        { id: 'c', name: 'C' },
      ],
      rows: [
        ['a0', 'b0', 'c0'],
        ['a1', 'b1', 'c1'],
        ['a2', 'b2', 'c2'],
      ],
      styles: { '2:2': { bold: true } },
      widths: [100, 200, 300],
      heights: [10, 20, 30],
    })

  it('follows the cell when a column moves', () => {
    const moved = moveColumn(styled(), 2, 0)
    assert.deepEqual(
      moved.columns.map((c) => c.name),
      ['C', 'A', 'B'],
    )
    assert.equal(moved.styles?.['2:0']?.bold, true, 'style follows column C')
    assert.equal(moved.styles?.['2:2'], undefined)
    assert.deepEqual(moved.widths, [300, 100, 200])
  })

  it('follows the cell when a row moves', () => {
    const moved = moveRow(styled(), 2, 0)
    assert.deepEqual(moved.rows[0], ['a2', 'b2', 'c2'])
    assert.equal(moved.styles?.['0:2']?.bold, true, 'style follows the moved row')
    assert.deepEqual(moved.heights, [30, 10, 20])
  })

  it('shifts styles when a row is inserted above', () => {
    const next = addTableRow(styled(), 0)
    assert.equal(next.styles?.['3:2']?.bold, true)
    assert.equal(next.styles?.['2:2'], undefined)
  })

  it('shifts styles when a column is removed', () => {
    const next = removeTableColumn(styled(), 0)
    assert.equal(next.styles?.['2:1']?.bold, true)
    assert.deepEqual(next.widths, [200, 300])
  })

  it('drops styles belonging to a deleted row', () => {
    const next = removeTableRow(styled(), 2)
    assert.equal(next.styles, undefined)
  })
})

describe('merges', () => {
  const base = () =>
    normalizeTableDoc({
      version: 1,
      columns: [
        { id: 'a', name: 'A' },
        { id: 'b', name: 'B' },
        { id: 'c', name: 'C' },
      ],
      rows: [
        ['keep', 'drop', 'drop'],
        ['x', 'y', 'z'],
      ],
    })

  it('keeps only the anchor value', () => {
    const merged = mergeCells(base(), 0, 0, 1, 3)
    assert.deepEqual(merged.rows[0], ['keep', '', ''])
    assert.deepEqual(merged.merges, [{ row: 0, col: 0, rowSpan: 1, colSpan: 3 }])
  })

  it('refuses an overlapping merge', () => {
    let doc = mergeCells(base(), 0, 0, 1, 2)
    doc = mergeCells(doc, 0, 1, 1, 2)
    assert.equal(doc.merges?.length, 1)
  })

  it('refuses a merge past the grid', () => {
    const doc = mergeCells(base(), 1, 2, 3, 3)
    assert.equal(doc.merges, undefined)
  })

  it('unmerges by anchor', () => {
    const merged = mergeCells(base(), 0, 0, 1, 3)
    assert.equal(unmergeCells(merged, 0, 0).merges, undefined)
  })

  it('reports the merge covering any cell in its span', () => {
    const merged = mergeCells(base(), 0, 0, 1, 3)
    assert.deepEqual(mergeAt(merged, 0, 2)?.col, 0)
    assert.equal(mergeAt(merged, 1, 0), undefined)
  })
})

describe('hide and show', () => {
  const base = () =>
    normalizeTableDoc({
      version: 1,
      columns: [
        { id: 'a', name: 'A' },
        { id: 'b', name: 'B' },
      ],
      rows: [['1', '2'], ['3', '4']],
    })

  it('hides and reveals a column', () => {
    const hidden = toggleColumnHidden(base(), 1)
    assert.deepEqual(hidden.hiddenColumns, [1])
    assert.equal(toggleColumnHidden(hidden, 1).hiddenColumns, undefined)
  })

  it('never hides the last visible column', () => {
    let doc = toggleColumnHidden(base(), 0)
    doc = toggleColumnHidden(doc, 1)
    assert.deepEqual(doc.hiddenColumns, [0], 'one column must stay visible')
  })
})

describe('sizes', () => {
  it('clamps width and height to sane bounds', () => {
    const doc = normalizeTableDoc({
      version: 1,
      columns: [{ id: 'a', name: 'A' }],
      rows: [['1']],
    })
    assert.equal(setColumnWidth(doc, 0, 10).widths?.[0], 48)
    assert.equal(setColumnWidth(doc, 0, 9999).widths?.[0], 2000)
    assert.equal(setRowHeight(doc, 0, 1).heights?.[0], 20)
    assert.equal(setRowHeight(doc, 0, 9999).heights?.[0], 800)
  })
})

describe('cell styles', () => {
  const doc = () =>
    normalizeTableDoc({
      version: 1,
      columns: [{ id: 'a', name: 'A' }],
      rows: [['1'], ['2']],
    })

  it('normalises colour input to bare uppercase hex', () => {
    const styled = setCellStyles(doc(), [{ row: 0, col: 0 }], { fill: '#abc' })
    assert.equal(styled.styles?.['0:0']?.fill, 'AABBCC')
  })

  it('merges patches instead of replacing the style', () => {
    let styled = setCellStyles(doc(), [{ row: 0, col: 0 }], { bold: true })
    styled = setCellStyles(styled, [{ row: 0, col: 0 }], { italic: true })
    assert.equal(styled.styles?.['0:0']?.bold, true)
    assert.equal(styled.styles?.['0:0']?.italic, true)
  })

  it('drops the entry when every attribute is cleared', () => {
    let styled = setCellStyles(doc(), [{ row: 0, col: 0 }], { bold: true })
    styled = setCellStyles(styled, [{ row: 0, col: 0 }], { bold: false })
    assert.equal(styled.styles, undefined)
  })

  it('applies to a whole selection', () => {
    const styled = setCellStyles(
      doc(),
      [
        { row: 0, col: 0 },
        { row: 1, col: 0 },
      ],
      { bold: true },
    )
    assert.equal(Object.keys(styled.styles || {}).length, 2)
  })
})

describe('formulas survive structural edits', () => {
  const withTotal = () =>
    normalizeTableDoc({
      version: 1,
      columns: [{ id: 'a', name: 'A' }],
      rows: [['100'], ['250'], ['=SUM(A2:A3)']],
    })

  it('follows its own row when that row moves', () => {
    const moved = moveRow(withTotal(), 0, 2)
    assert.deepEqual(
      moved.rows.map((r) => r[0]),
      ['250', '=SUM(A2:A4)', '100'],
      'endpoints track their rows and stay in ascending order',
    )
    assert.equal(
      evaluateCell(moved, 1, 0),
      '#CYCLE!',
      'the moved total now spans its own row, which a spreadsheet reports rather than guessing',
    )
  })

  it('keeps summing the same values when the total itself moves', () => {
    const doc = normalizeTableDoc({
      version: 1,
      columns: [{ id: 'a', name: 'A' }],
      rows: [['100'], ['250'], ['=SUM(A2:A3)']],
    })
    const moved = moveRow(doc, 2, 0)
    assert.equal(moved.rows[0][0], '=SUM(A3:A4)')
    assert.equal(evaluateCell(moved, 0, 0), 350)
  })

  it('keeps pointing at the same values when a row is inserted above', () => {
    const next = addTableRow(withTotal(), 0)
    assert.equal(evaluateCell(next, 3, 0), 350)
  })

  it('marks a reference to a deleted row as #REF!', () => {
    const next = removeTableRow(withTotal(), 0)
    assert.match(next.rows[1][0], /#REF!/)
  })

  it('leaves quoted text untouched while remapping', () => {
    const doc = normalizeTableDoc({
      version: 1,
      columns: [{ id: 'a', name: 'A' }],
      rows: [['1'], ['=IF(A2>0,"A2 ok","A2 bad")']],
    })
    const moved = moveRow(doc, 0, 1)
    assert.match(moved.rows[0][0], /"A2 ok"/)
    assert.match(moved.rows[0][0], /"A2 bad"/)
  })
})

describe('buildXlsx', () => {
  const read = async (blob: Blob) => new Uint8Array(await blob.arrayBuffer())

  it('produces a zip container', async () => {
    const bytes = await read(buildXlsx([makeSheet('Sheet 1')]))
    assert.deepEqual([...bytes.slice(0, 4)], [0x50, 0x4b, 0x03, 0x04])
  })

  it('declares one worksheet part per sheet', async () => {
    const sheets = [makeSheet('One'), makeSheet('Two'), makeSheet('Three')]
    const text = new TextDecoder().decode(await read(buildXlsx(sheets)))
    for (let i = 1; i <= sheets.length; i += 1) {
      assert.ok(
        text.includes(`xl/worksheets/sheet${i}.xml`),
        `sheet${i}.xml must be present`,
      )
    }
    assert.ok(text.includes('<sheet name="One"'))
    assert.ok(text.includes('<sheet name="Three"'))
  })

  it('writes cell values and escapes XML', async () => {
    const sheet = makeSheet('Data', tableFromCsv('name\nA & <B>\n').doc)
    const text = new TextDecoder().decode(await read(buildXlsx([sheet])))
    assert.ok(text.includes('A &amp; &lt;B&gt;'))
    assert.ok(!text.includes('A & <B>'))
  })

  it('keeps sheet names unique after Excel sanitising', async () => {
    const sheets = [makeSheet('Q1/Q2'), makeSheet('Q1:Q2')]
    const text = new TextDecoder().decode(await read(buildXlsx(sheets)))
    const names = [...text.matchAll(/<sheet name="([^"]+)"/g)].map((m) => m[1])
    assert.equal(names.length, 2)
    assert.equal(new Set(names).size, 2)
  })

  it('writes formulas as formulas, not as text', async () => {
    const doc = normalizeTableDoc({
      version: 1,
      columns: [{ id: 'a', name: 'n' }],
      rows: [['10'], ['20'], ['=SUM(A2:A3)']],
    })
    const text = new TextDecoder().decode(await read(buildXlsx([makeSheet('S', doc)])))
    assert.ok(text.includes('<f>SUM(A2:A3)</f>'), 'formula is written verbatim')
    assert.ok(!text.includes('=SUM(A2:A3)</t>'), 'formula must not be inline text')
    assert.ok(text.includes('fullCalcOnLoad="1"'), 'Excel must recalculate on open')
  })

  it('writes numeric literals as numbers', async () => {
    const doc = normalizeTableDoc({
      version: 1,
      columns: [{ id: 'a', name: 'n' }],
      rows: [['1500']],
    })
    const text = new TextDecoder().decode(await read(buildXlsx([makeSheet('S', doc)])))
    assert.ok(text.includes('<v>1500</v>'))
  })

  it('emits a style part carrying colour, bold and number format', async () => {
    const doc = normalizeTableDoc({
      version: 1,
      columns: [{ id: 'a', name: 'n' }],
      rows: [['1']],
      styles: {
        '0:0': { bold: true, fill: 'FFE599', color: '1F2937', numFmt: '#,##0' },
      },
    })
    const text = new TextDecoder().decode(await read(buildXlsx([makeSheet('S', doc)])))
    assert.ok(text.includes('xl/styles.xml'))
    assert.ok(text.includes('<b/>'))
    assert.ok(text.includes('FFFFE599'), 'fill must be ARGB prefixed')
    assert.ok(text.includes('FF1F2937'), 'font colour must be ARGB prefixed')
    assert.ok(text.includes('#,##0'))
    assert.ok(/numFmtId="16[4-9]"/.test(text), 'custom formats start at 164')
  })

  it('writes merges offset past the header row', async () => {
    const doc = normalizeTableDoc({
      version: 1,
      columns: [
        { id: 'a', name: 'a' },
        { id: 'b', name: 'b' },
      ],
      rows: [
        ['Title', ''],
        ['x', 'y'],
      ],
      merges: [{ row: 0, col: 0, rowSpan: 1, colSpan: 2 }],
    })
    const text = new TextDecoder().decode(await read(buildXlsx([makeSheet('S', doc)])))
    assert.ok(text.includes('<mergeCell ref="A2:B2"/>'), 'document row 0 is sheet row 2')
  })

  it('writes column widths, row heights and hidden flags', async () => {
    const doc = normalizeTableDoc({
      version: 1,
      columns: [
        { id: 'a', name: 'a' },
        { id: 'b', name: 'b' },
      ],
      rows: [
        ['1', '2'],
        ['3', '4'],
      ],
      widths: [200, 0],
      heights: [40, 0],
      hiddenColumns: [1],
      hiddenRows: [1],
    })
    const text = new TextDecoder().decode(await read(buildXlsx([makeSheet('S', doc)])))
    assert.ok(text.includes('customWidth="1"'))
    assert.ok(text.includes('customHeight="1"'))
    assert.match(text, /<col [^>]*min="2"[^>]*hidden="1"/)
    assert.match(text, /<row r="3"[^>]*hidden="1"/)
  })

  it('reuses one style entry for repeated formatting', async () => {
    const doc = normalizeTableDoc({
      version: 1,
      columns: [{ id: 'a', name: 'n' }],
      rows: [['1'], ['2'], ['3']],
      styles: {
        '0:0': { bold: true },
        '1:0': { bold: true },
        '2:0': { bold: true },
      },
    })
    const text = new TextDecoder().decode(await read(buildXlsx([makeSheet('S', doc)])))
    const count = /<cellXfs count="(\d+)"/.exec(text)?.[1]
    assert.equal(count, '2', 'default + one shared bold style')
  })
})
