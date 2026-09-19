import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  applyDocAgentReply,
  buildDocAgentPrompt,
  docAgentSource,
  workbookFromAgentJson,
  workbookToAgentJson,
} from './docAgent'
import {
  emptyTableWorkbook,
  makeSheet,
  normalizeTableDoc,
  normalizeTableWorkbook,
  parseTableWorkbook,
  serializeTableWorkbook,
} from './tableDoc'

const reply = (body: unknown) =>
  ['Here you go:', '```json', JSON.stringify(body), '```'].join('\n')

describe('table agent prompt', () => {
  const prompt = buildDocAgentPrompt('table', 'buat laporan pajak 3 sheet', '{"sheets":[]}')

  it('asks for one entry per requested sheet', () => {
    assert.match(prompt, /ONE entry in `sheets` per requested sheet/)
    assert.match(prompt, /Do not collapse them into one/)
  })

  it('documents formulas with the header offset', () => {
    assert.match(prompt, /FIRST DATA ROW IS ROW 2/)
    assert.match(prompt, /never hard-code a computed number/)
    assert.match(prompt, /SUM, AVERAGE/)
  })

  it('documents styling so a report can be formatted', () => {
    assert.match(prompt, /numFmt/)
    assert.match(prompt, /RRGGBB/)
    assert.match(prompt, /widths.+heights.+pixel/s)
  })

  it('permits illustrative data when asked to draft a report', () => {
    assert.match(prompt, /Illustrative values are expected/)
  })

  it('carries the user instruction', () => {
    assert.match(prompt, /buat laporan pajak 3 sheet/)
  })
})

describe('workbookToAgentJson', () => {
  it('sends headers separately from data rows', () => {
    const wb = normalizeTableWorkbook({
      version: 1,
      sheets: [
        makeSheet(
          'PPN',
          normalizeTableDoc({
            version: 1,
            columns: [
              { id: 'a', name: 'Masa' },
              { id: 'b', name: 'DPP' },
            ],
            rows: [['Januari', '1000']],
          }),
        ),
      ],
      activeSheetId: 'x',
    })
    const json = JSON.parse(workbookToAgentJson(wb))
    assert.deepEqual(json.sheets[0].columns, ['Masa', 'DPP'])
    assert.deepEqual(json.sheets[0].rows, [['Januari', '1000']])
    assert.equal(json.sheets[0].name, 'PPN')
  })

  it('omits empty presentation fields', () => {
    const json = JSON.parse(workbookToAgentJson(emptyTableWorkbook()))
    assert.equal('styles' in json.sheets[0], false)
    assert.equal('merges' in json.sheets[0], false)
  })
})

describe('applyDocAgentReply for table', () => {
  const before = serializeTableWorkbook(emptyTableWorkbook())

  it('creates every sheet the model returned', () => {
    const out = applyDocAgentReply(
      'table',
      before,
      reply({
        sheets: [
          { name: 'PPN', columns: ['Masa', 'DPP'], rows: [['Januari', '100']] },
          { name: 'PPh 21', columns: ['Nama'], rows: [['Budi']] },
          { name: 'Rekap', columns: ['Total'], rows: [['=SUM(B2:B2)']] },
        ],
      }),
    )
    assert.equal(out.ok, true)
    if (!out.ok) return
    const wb = parseTableWorkbook(out.content).workbook
    assert.deepEqual(
      wb.sheets.map((s) => s.name),
      ['PPN', 'PPh 21', 'Rekap'],
    )
  })

  it('keeps formulas as text so they stay live', () => {
    const out = applyDocAgentReply(
      'table',
      before,
      reply({
        sheets: [{ name: 'S', columns: ['a', 'b'], rows: [['1', '=A2*2']] }],
      }),
    )
    assert.equal(out.ok, true)
    if (!out.ok) return
    const doc = parseTableWorkbook(out.content).workbook.sheets[0].doc
    assert.equal(doc.rows[0][1], '=A2*2')
  })

  it('round-trips styles, widths and merges', () => {
    const out = applyDocAgentReply(
      'table',
      before,
      reply({
        sheets: [
          {
            name: 'S',
            columns: ['a', 'b'],
            rows: [
              ['Title', ''],
              ['1', '2'],
            ],
            styles: { '0:0': { bold: true, fill: 'FFE599', numFmt: '#,##0' } },
            widths: [200, 120],
            heights: [40, 0],
            merges: [{ row: 0, col: 0, rowSpan: 1, colSpan: 2 }],
          },
        ],
      }),
    )
    assert.equal(out.ok, true)
    if (!out.ok) return
    const doc = parseTableWorkbook(out.content).workbook.sheets[0].doc
    assert.equal(doc.styles?.['0:0']?.bold, true)
    assert.equal(doc.styles?.['0:0']?.fill, 'FFE599')
    assert.equal(doc.styles?.['0:0']?.numFmt, '#,##0')
    assert.equal(doc.widths?.[0], 200)
    assert.equal(doc.heights?.[0], 40)
    assert.deepEqual(doc.merges, [{ row: 0, col: 0, rowSpan: 1, colSpan: 2 }])
  })

  it('rejects a reply with no sheets instead of blanking the document', () => {
    const out = applyDocAgentReply('table', before, reply({ sheets: [] }))
    assert.equal(out.ok, false)
  })

  it('rejects malformed JSON', () => {
    const out = applyDocAgentReply('table', before, '```json\n{oops\n```')
    assert.equal(out.ok, false)
  })

  it('still accepts a CSV reply, updating the active sheet', () => {
    const out = applyDocAgentReply('table', before, '```csv\nname,qty\nBudi,2\n```')
    assert.equal(out.ok, true)
    if (!out.ok) return
    const wb = parseTableWorkbook(out.content).workbook
    assert.equal(wb.sheets.length, 1)
    assert.deepEqual(wb.sheets[0].doc.rows, [['Budi', '2']])
  })
})

describe('workbookFromAgentJson', () => {
  it('pads short rows to the header width', () => {
    const { workbook } = workbookFromAgentJson({
      sheets: [{ name: 'S', columns: ['a', 'b', 'c'], rows: [['1']] }],
    })
    assert.deepEqual(workbook.sheets[0].doc.rows, [['1', '', '']])
  })

  it('refuses an oversize sheet', () => {
    const { error } = workbookFromAgentJson({
      sheets: [{ name: 'S', columns: ['a'], rows: Array.from({ length: 5000 }, () => ['x']) }],
    })
    assert.ok(error, 'oversize input must be reported')
  })
})

describe('docAgentSource for table', () => {
  it('hands the model the workbook, not one CSV sheet', () => {
    const wb = normalizeTableWorkbook({
      version: 1,
      sheets: [makeSheet('One'), makeSheet('Two')],
      activeSheetId: 'x',
    })
    const source = docAgentSource('table', serializeTableWorkbook(wb))
    const json = JSON.parse(source)
    assert.equal(json.sheets.length, 2)
  })
})
