import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  evaluateCell,
  evaluateGrid,
  formatCellValue,
  isFormula,
  parseRef,
} from './tableFormula'
import { normalizeTableDoc, tableFromCsv, type TableDoc } from './tableDoc'

function sheet(csv: string): TableDoc {
  return normalizeTableDoc(tableFromCsv(csv, { headerRow: false }).doc)
}

/** Grid built from explicit cells, so a formula may contain commas. */
function grid(rows: string[][]): TableDoc {
  return normalizeTableDoc({
    version: 1,
    columns: rows[0].map((_, i) => ({ id: `c${i}`, name: `C${i}` })),
    rows,
  })
}

function valueAt(csv: string, row: number, col: number) {
  return evaluateCell(sheet(csv), row, col)
}

/** Evaluates a single formula that may contain commas. */
function formula(expr: string, extra: string[][] = []) {
  return evaluateCell(grid([[expr, ...(extra[0] || [])], ...extra.slice(1)]), 0, 0)
}

describe('parseRef', () => {
  it('treats row 1 as the header so row 2 is the first data row', () => {
    assert.deepEqual(parseRef('A2'), { row: 0, col: 0 })
    assert.deepEqual(parseRef('B3'), { row: 1, col: 1 })
    assert.deepEqual(parseRef('AA10'), { row: 8, col: 26 })
    assert.deepEqual(parseRef('$C$2'), { row: 0, col: 2 })
    assert.deepEqual(parseRef('A1'), { row: -1, col: 0 })
  })

  it('rejects text that is not a reference', () => {
    assert.equal(parseRef('SUM'), null)
    assert.equal(parseRef('A0'), null)
    assert.equal(parseRef('1A'), null)
  })
})

describe('isFormula', () => {
  it('only treats a leading = as a formula', () => {
    assert.equal(isFormula('=1+1'), true)
    assert.equal(isFormula('  =A1'), true)
    assert.equal(isFormula('1+1'), false)
    assert.equal(isFormula('a = b'), false)
  })
})

describe('arithmetic', () => {
  it('applies operator precedence', () => {
    assert.equal(valueAt('=2+3*4', 0, 0), 14)
    assert.equal(valueAt('=(2+3)*4', 0, 0), 20)
    assert.equal(valueAt('=2^3^2', 0, 0), 512)
    assert.equal(valueAt('=-3+5', 0, 0), 2)
  })

  it('reads percentages as fractions', () => {
    assert.equal(valueAt('=50%', 0, 0), 0.5)
    assert.equal(valueAt('=200*11%', 0, 0), 22)
  })

  it('returns #DIV/0! rather than Infinity', () => {
    assert.equal(valueAt('=1/0', 0, 0), '#DIV/0!')
  })
})

describe('references and ranges', () => {
  it('reads other cells', () => {
    assert.equal(valueAt('10,20,=A2+B2', 0, 2), 30)
  })

  it('aggregates a range', () => {
    const csv = '1,2,3\n4,5,6\n=SUM(A2:C3),=AVERAGE(A2:C3),=MAX(A2:C3)'
    assert.equal(valueAt(csv, 2, 0), 21)
    assert.equal(valueAt(csv, 2, 1), 3.5)
    assert.equal(valueAt(csv, 2, 2), 6)
  })

  it('skips text inside a numeric aggregate instead of failing', () => {
    assert.equal(valueAt('10,abc,20,=SUM(A2:C2)', 0, 3), 30)
    assert.equal(valueAt('10,abc,20,=COUNT(A2:C2)', 0, 3), 2)
    assert.equal(valueAt('10,abc,20,=COUNTA(A2:C2)', 0, 3), 3)
  })

  it('resolves a chain of dependent formulas', () => {
    const csv = '5,=A2*2,=B2*2,=C2+A2'
    assert.equal(valueAt(csv, 0, 3), 25)
  })

  it('reports a cycle instead of hanging', () => {
    assert.equal(valueAt('=B2,=A2', 0, 0), '#CYCLE!')
    assert.equal(valueAt('=A2', 0, 0), '#CYCLE!')
  })

  it('treats an out-of-range reference as empty, not as a crash', () => {
    assert.equal(valueAt('=Z99', 0, 0), '')
  })
})

describe('functions', () => {
  it('branches with IF', () => {
    assert.equal(formula('=IF(1>0,"yes","no")'), 'yes')
    assert.equal(formula('=IF(0,"yes","no")'), 'no')
  })

  it('rounds', () => {
    assert.equal(formula('=ROUND(2.345,2)'), 2.35)
    assert.equal(formula('=ROUNDUP(2.341,2)'), 2.35)
    assert.equal(formula('=ROUNDDOWN(2.349,2)'), 2.34)
  })

  it('handles text helpers and concatenation', () => {
    assert.equal(formula('=UPPER("abc")'), 'ABC')
    assert.equal(formula('=LEN("hello")'), 5)
    assert.equal(formula('="a"&"b"'), 'ab')
    assert.equal(formula('=CONCAT("x",1,"y")'), 'x1y')
  })

  it('swallows an error with IFERROR', () => {
    assert.equal(formula('=IFERROR(1/0,"n/a")'), 'n/a')
  })

  it('names an unknown function', () => {
    assert.equal(formula('=NOPE(1)'), '#NAME?')
  })

  it('reports malformed input instead of throwing', () => {
    assert.equal(formula('=1+'), '#ERROR!')
    assert.equal(formula('=((1+2)'), '#ERROR!')
  })
})

describe('security', () => {
  it('does not execute JavaScript', () => {
    const marker = `__formula_escape_${Date.now()}__`
    const globalThisAny = globalThis as Record<string, unknown>
    assert.equal(formula(`=globalThis["${marker}"]=1`), '#ERROR!')
    assert.equal(globalThisAny[marker], undefined)
    assert.equal(formula('=constructor.constructor("return 1")()'), '#ERROR!')
  })
})

describe('evaluateGrid', () => {
  it('evaluates every cell in one pass', () => {
    const grid = evaluateGrid(sheet('1,2\n=A2+B2,=A3*10'))
    assert.deepEqual(grid, [
      [1, 2],
      [3, 30],
    ])
  })
})

describe('formatCellValue', () => {
  it('renders values the way a sheet displays them', () => {
    assert.equal(formatCellValue(3), '3')
    assert.equal(formatCellValue(3.5), '3.5')
    assert.equal(formatCellValue(''), '')
    assert.equal(formatCellValue('#DIV/0!'), '#DIV/0!')
    assert.equal(formatCellValue(0.1 + 0.2), '0.3')
  })
})
