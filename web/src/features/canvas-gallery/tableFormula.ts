// Formula evaluation for the table playground. Recursive-descent parser over a
// hand-written tokenizer — no eval, no dependency, so a sheet can never execute
// arbitrary JavaScript.
//
// Cell text starting with '=' is a formula. Everything else is a literal.
// Excel semantics kept deliberately: 1-based rows, A1 refs, ranges expand to
// flat value lists, and text in a numeric position is skipped by aggregates
// (SUM) rather than poisoning the result.

import type { TableDoc } from './tableDoc'

export type FormulaError =
  | '#DIV/0!'
  | '#VALUE!'
  | '#REF!'
  | '#NAME?'
  | '#CYCLE!'
  | '#ERROR!'

export type CellValue = number | string | boolean | FormulaError

const ERRORS: FormulaError[] = [
  '#DIV/0!',
  '#VALUE!',
  '#REF!',
  '#NAME?',
  '#CYCLE!',
  '#ERROR!',
]

export function isFormulaError(value: CellValue): value is FormulaError {
  return typeof value === 'string' && (ERRORS as string[]).includes(value)
}

export function isFormula(text: string): boolean {
  return typeof text === 'string' && text.trimStart().startsWith('=')
}

/** 0 -> A, 25 -> Z, 26 -> AA */
export function columnLetter(index: number): string {
  let n = index + 1
  let out = ''
  while (n > 0) {
    const rem = (n - 1) % 26
    out = String.fromCharCode(65 + rem) + out
    n = Math.floor((n - 1) / 26)
  }
  return out
}
/**
 * "A2" -> {row: 0, col: 0}. Spreadsheet row 1 is the header, so the first data
 * row is row 2 — the same convention the XLSX writer and the agent prompt use.
 * A reference to row 1 addresses the header and has no data row.
 */
export function parseRef(text: string): { row: number; col: number } | null {
  const m = /^\$?([A-Z]{1,3})\$?(\d{1,7})$/i.exec(text.trim())
  if (!m) return null
  let col = 0
  for (const ch of m[1].toUpperCase()) {
    col = col * 26 + (ch.charCodeAt(0) - 64)
  }
  const row = Number(m[2])
  if (row < 1) return null
  return { row: row - 2, col: col - 1 }
}

type Token =
  | { kind: 'number'; value: number }
  | { kind: 'string'; value: string }
  | { kind: 'ref'; value: string }
  | { kind: 'name'; value: string }
  | { kind: 'op'; value: string }

function tokenize(src: string): Token[] | null {
  const tokens: Token[] = []
  let i = 0
  while (i < src.length) {
    const ch = src[i]
    if (ch === ' ' || ch === '\t' || ch === '\n' || ch === '\r') {
      i += 1
      continue
    }
    if (ch === '"') {
      let value = ''
      i += 1
      while (i < src.length) {
        if (src[i] === '"') {
          if (src[i + 1] === '"') {
            value += '"'
            i += 2
            continue
          }
          i += 1
          break
        }
        value += src[i]
        i += 1
      }
      tokens.push({ kind: 'string', value })
      continue
    }
    if (/[0-9]/.test(ch) || (ch === '.' && /[0-9]/.test(src[i + 1] || ''))) {
      const m = /^[0-9]*\.?[0-9]+(?:[eE][+-]?[0-9]+)?/.exec(src.slice(i))
      if (!m) return null
      tokens.push({ kind: 'number', value: Number(m[0]) })
      i += m[0].length
      continue
    }
    if (/[A-Z_]/i.test(ch) || ch === '$') {
      const m = /^\$?[A-Z]{1,3}\$?[0-9]{1,7}(?![A-Z0-9_.])/i.exec(src.slice(i))
      if (m) {
        tokens.push({ kind: 'ref', value: m[0] })
        i += m[0].length
        continue
      }
      const name = /^[A-Z_][A-Z0-9_.]*/i.exec(src.slice(i))
      if (!name) return null
      tokens.push({ kind: 'name', value: name[0].toUpperCase() })
      i += name[0].length
      continue
    }
    const two = src.slice(i, i + 2)
    if (two === '<=' || two === '>=' || two === '<>') {
      tokens.push({ kind: 'op', value: two })
      i += 2
      continue
    }
    if ('+-*/^%()<>=,:&'.includes(ch)) {
      tokens.push({ kind: 'op', value: ch })
      i += 1
      continue
    }
    return null
  }
  return tokens
}

type Node =
  | { type: 'num'; value: number }
  | { type: 'str'; value: string }
  | { type: 'ref'; ref: string }
  | { type: 'range'; from: string; to: string }
  | { type: 'call'; name: string; args: Node[] }
  | { type: 'unary'; op: string; arg: Node }
  | { type: 'binary'; op: string; left: Node; right: Node }

class Parser {
  private pos = 0
  constructor(private readonly tokens: Token[]) {}

  parse(): Node | null {
    const node = this.comparison()
    if (!node || this.pos !== this.tokens.length) return null
    return node
  }

  private peek(): Token | undefined {
    return this.tokens[this.pos]
  }

  private eatOp(...values: string[]): string | null {
    const t = this.peek()
    if (t?.kind === 'op' && values.includes(t.value)) {
      this.pos += 1
      return t.value
    }
    return null
  }

  private comparison(): Node | null {
    let left = this.concat()
    if (!left) return null
    for (;;) {
      const op = this.eatOp('=', '<>', '<', '>', '<=', '>=')
      if (!op) return left
      const right = this.concat()
      if (!right) return null
      left = { type: 'binary', op, left, right }
    }
  }

  private concat(): Node | null {
    let left = this.additive()
    if (!left) return null
    for (;;) {
      const op = this.eatOp('&')
      if (!op) return left
      const right = this.additive()
      if (!right) return null
      left = { type: 'binary', op, left, right }
    }
  }

  private additive(): Node | null {
    let left = this.multiplicative()
    if (!left) return null
    for (;;) {
      const op = this.eatOp('+', '-')
      if (!op) return left
      const right = this.multiplicative()
      if (!right) return null
      left = { type: 'binary', op, left, right }
    }
  }

  private multiplicative(): Node | null {
    let left = this.power()
    if (!left) return null
    for (;;) {
      const op = this.eatOp('*', '/')
      if (!op) return left
      const right = this.power()
      if (!right) return null
      left = { type: 'binary', op, left, right }
    }
  }

  private power(): Node | null {
    const base = this.unary()
    if (!base) return null
    if (this.eatOp('^')) {
      const exp = this.power()
      if (!exp) return null
      return { type: 'binary', op: '^', left: base, right: exp }
    }
    return base
  }

  private unary(): Node | null {
    const op = this.eatOp('-', '+')
    if (op) {
      const arg = this.unary()
      if (!arg) return null
      return { type: 'unary', op, arg }
    }
    return this.postfix()
  }

  private postfix(): Node | null {
    const node = this.primary()
    if (!node) return null
    if (this.eatOp('%')) {
      return { type: 'binary', op: '/', left: node, right: { type: 'num', value: 100 } }
    }
    return node
  }

  private primary(): Node | null {
    const t = this.peek()
    if (!t) return null

    if (t.kind === 'number') {
      this.pos += 1
      return { type: 'num', value: t.value }
    }
    if (t.kind === 'string') {
      this.pos += 1
      return { type: 'str', value: t.value }
    }
    if (t.kind === 'ref') {
      this.pos += 1
      if (this.eatOp(':')) {
        const end = this.peek()
        if (end?.kind !== 'ref') return null
        this.pos += 1
        return { type: 'range', from: t.value, to: end.value }
      }
      return { type: 'ref', ref: t.value }
    }
    if (t.kind === 'name') {
      this.pos += 1
      if (t.value === 'TRUE') return { type: 'num', value: 1 }
      if (t.value === 'FALSE') return { type: 'num', value: 0 }
      if (!this.eatOp('(')) return null
      const args: Node[] = []
      if (!this.eatOp(')')) {
        for (;;) {
          const arg = this.comparison()
          if (!arg) return null
          args.push(arg)
          if (this.eatOp(',')) continue
          if (this.eatOp(')')) break
          return null
        }
      }
      return { type: 'call', name: t.value, args }
    }
    if (t.kind === 'op' && t.value === '(') {
      this.pos += 1
      const inner = this.comparison()
      if (!inner) return null
      if (!this.eatOp(')')) return null
      return inner
    }
    return null
  }
}

function toNumber(value: CellValue): number | FormulaError {
  if (typeof value === 'number') return value
  if (typeof value === 'boolean') return value ? 1 : 0
  if (isFormulaError(value)) return value
  const text = String(value).trim()
  if (!text) return 0
  const cleaned = text.replace(/,/g, '')
  const n = Number(cleaned)
  return Number.isFinite(n) ? n : '#VALUE!'
}

function toText(value: CellValue): string {
  if (typeof value === 'boolean') return value ? 'TRUE' : 'FALSE'
  return String(value)
}

type Ctx = {
  doc: TableDoc
  /** Cells currently being evaluated, used to detect self-reference. */
  visiting: Set<string>
  cache: Map<string, CellValue>
}

function rawCell(doc: TableDoc, row: number, col: number): string {
  return doc.rows[row]?.[col] ?? ''
}

function evalCell(ctx: Ctx, row: number, col: number): CellValue {
  if (row < 0 || col < 0) return '#REF!'
  if (row >= ctx.doc.rows.length || col >= ctx.doc.columns.length) return ''
  const key = `${row}:${col}`
  const cached = ctx.cache.get(key)
  if (cached !== undefined) return cached
  if (ctx.visiting.has(key)) return '#CYCLE!'

  const raw = rawCell(ctx.doc, row, col)
  if (!isFormula(raw)) {
    const value = literalValue(raw)
    ctx.cache.set(key, value)
    return value
  }

  ctx.visiting.add(key)
  const value = evaluateFormula(raw, ctx)
  ctx.visiting.delete(key)
  ctx.cache.set(key, value)
  return value
}

function literalValue(raw: string): CellValue {
  const text = raw.trim()
  if (!text) return ''
  const cleaned = text.replace(/,/g, '')
  if (/^-?\d*\.?\d+(?:[eE][+-]?\d+)?$/.test(cleaned)) return Number(cleaned)
  if (/^-?\d*\.?\d+%$/.test(cleaned)) return Number(cleaned.slice(0, -1)) / 100
  return text
}

function flatten(ctx: Ctx, node: Node): CellValue[] {
  if (node.type === 'range') {
    const from = parseRef(node.from)
    const to = parseRef(node.to)
    if (!from || !to) return ['#REF!']
    const out: CellValue[] = []
    const r1 = Math.min(from.row, to.row)
    const r2 = Math.max(from.row, to.row)
    const c1 = Math.min(from.col, to.col)
    const c2 = Math.max(from.col, to.col)
    for (let r = r1; r <= r2; r += 1) {
      for (let c = c1; c <= c2; c += 1) out.push(evalCell(ctx, r, c))
    }
    return out
  }
  return [evalNode(ctx, node)]
}

function numbersOf(values: CellValue[]): number[] | FormulaError {
  const out: number[] = []
  for (const v of values) {
    if (isFormulaError(v)) return v
    if (typeof v === 'number') out.push(v)
    else if (typeof v === 'boolean') out.push(v ? 1 : 0)
    else if (String(v).trim()) {
      const n = Number(String(v).replace(/,/g, ''))
      if (Number.isFinite(n)) out.push(n)
    }
  }
  return out
}

function callFunction(ctx: Ctx, name: string, args: Node[]): CellValue {
  const collect = () => args.flatMap((a) => flatten(ctx, a))

  switch (name) {
    case 'SUM':
    case 'AVERAGE':
    case 'MIN':
    case 'MAX':
    case 'PRODUCT':
    case 'MEDIAN': {
      const nums = numbersOf(collect())
      if (!Array.isArray(nums)) return nums
      if (name === 'SUM') return nums.reduce((a, b) => a + b, 0)
      if (name === 'PRODUCT') return nums.reduce((a, b) => a * b, 1)
      if (nums.length === 0) return '#DIV/0!'
      if (name === 'AVERAGE') return nums.reduce((a, b) => a + b, 0) / nums.length
      if (name === 'MIN') return Math.min(...nums)
      if (name === 'MAX') return Math.max(...nums)
      const sorted = nums.slice().sort((a, b) => a - b)
      const mid = Math.floor(sorted.length / 2)
      return sorted.length % 2 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2
    }
    case 'COUNT': {
      const nums = numbersOf(collect())
      return Array.isArray(nums) ? nums.length : nums
    }
    case 'COUNTA':
      return collect().filter((v) => !isFormulaError(v) && String(v).trim() !== '').length
    case 'ROUND':
    case 'ROUNDUP':
    case 'ROUNDDOWN': {
      const value = toNumber(evalNode(ctx, args[0]))
      if (typeof value !== 'number') return value
      const digitsRaw = args[1] ? toNumber(evalNode(ctx, args[1])) : 0
      if (typeof digitsRaw !== 'number') return digitsRaw
      const f = 10 ** Math.trunc(digitsRaw)
      if (name === 'ROUNDUP') return Math.ceil(value * f) / f
      if (name === 'ROUNDDOWN') return Math.trunc(value * f) / f
      return Math.round(value * f) / f
    }
    case 'ABS': {
      const v = toNumber(evalNode(ctx, args[0]))
      return typeof v === 'number' ? Math.abs(v) : v
    }
    case 'INT': {
      const v = toNumber(evalNode(ctx, args[0]))
      return typeof v === 'number' ? Math.floor(v) : v
    }
    case 'SQRT': {
      const v = toNumber(evalNode(ctx, args[0]))
      if (typeof v !== 'number') return v
      return v < 0 ? '#VALUE!' : Math.sqrt(v)
    }
    case 'POWER': {
      const base = toNumber(evalNode(ctx, args[0]))
      const exp = toNumber(evalNode(ctx, args[1]))
      if (typeof base !== 'number') return base
      if (typeof exp !== 'number') return exp
      return base ** exp
    }
    case 'IF': {
      if (args.length < 2) return '#VALUE!'
      const cond = evalNode(ctx, args[0])
      if (isFormulaError(cond)) return cond
      const truthy =
        typeof cond === 'number' ? cond !== 0 : typeof cond === 'boolean' ? cond : Boolean(String(cond).trim())
      if (truthy) return evalNode(ctx, args[1])
      return args[2] ? evalNode(ctx, args[2]) : 0
    }
    case 'IFERROR': {
      const value = evalNode(ctx, args[0])
      return isFormulaError(value) ? evalNode(ctx, args[1]) : value
    }
    case 'AND':
    case 'OR': {
      const values = collect()
      const flags: boolean[] = []
      for (const v of values) {
        if (isFormulaError(v)) return v
        flags.push(typeof v === 'number' ? v !== 0 : Boolean(String(v).trim()))
      }
      const result = name === 'AND' ? flags.every(Boolean) : flags.some(Boolean)
      return result ? 1 : 0
    }
    case 'NOT': {
      const v = evalNode(ctx, args[0])
      if (isFormulaError(v)) return v
      const truthy = typeof v === 'number' ? v !== 0 : Boolean(String(v).trim())
      return truthy ? 0 : 1
    }
    case 'CONCAT':
    case 'CONCATENATE':
      return collect()
        .map((v) => (isFormulaError(v) ? '' : toText(v)))
        .join('')
    case 'LEN':
      return toText(evalNode(ctx, args[0])).length
    case 'UPPER':
      return toText(evalNode(ctx, args[0])).toUpperCase()
    case 'LOWER':
      return toText(evalNode(ctx, args[0])).toLowerCase()
    case 'TRIM':
      return toText(evalNode(ctx, args[0])).trim()
    default:
      return '#NAME?'
  }
}

function evalNode(ctx: Ctx, node: Node | undefined): CellValue {
  if (!node) return '#VALUE!'
  switch (node.type) {
    case 'num':
      return node.value
    case 'str':
      return node.value
    case 'ref': {
      const ref = parseRef(node.ref)
      if (!ref) return '#REF!'
      return evalCell(ctx, ref.row, ref.col)
    }
    case 'range': {
      const values = flatten(ctx, node)
      return values[0] ?? ''
    }
    case 'call':
      return callFunction(ctx, node.name, node.args)
    case 'unary': {
      const v = toNumber(evalNode(ctx, node.arg))
      if (typeof v !== 'number') return v
      return node.op === '-' ? -v : v
    }
    case 'binary': {
      if (node.op === '&') {
        const l = evalNode(ctx, node.left)
        const r = evalNode(ctx, node.right)
        if (isFormulaError(l)) return l
        if (isFormulaError(r)) return r
        return toText(l) + toText(r)
      }
      if (['=', '<>', '<', '>', '<=', '>='].includes(node.op)) {
        const l = evalNode(ctx, node.left)
        const r = evalNode(ctx, node.right)
        if (isFormulaError(l)) return l
        if (isFormulaError(r)) return r
        const bothNumeric = typeof l === 'number' && typeof r === 'number'
        const a: number | string = bothNumeric ? (l as number) : toText(l)
        const b: number | string = bothNumeric ? (r as number) : toText(r)
        let result: boolean
        switch (node.op) {
          case '=':
            result = a === b
            break
          case '<>':
            result = a !== b
            break
          case '<':
            result = a < b
            break
          case '>':
            result = a > b
            break
          case '<=':
            result = a <= b
            break
          default:
            result = a >= b
        }
        return result ? 1 : 0
      }
      const l = toNumber(evalNode(ctx, node.left))
      if (typeof l !== 'number') return l
      const r = toNumber(evalNode(ctx, node.right))
      if (typeof r !== 'number') return r
      switch (node.op) {
        case '+':
          return l + r
        case '-':
          return l - r
        case '*':
          return l * r
        case '/':
          return r === 0 ? '#DIV/0!' : l / r
        case '^':
          return l ** r
        default:
          return '#ERROR!'
      }
    }
    default:
      return '#ERROR!'
  }
}

function evaluateFormula(raw: string, ctx: Ctx): CellValue {
  const src = raw.trimStart().slice(1)
  if (!src.trim()) return ''
  const tokens = tokenize(src)
  if (!tokens) return '#ERROR!'
  const ast = new Parser(tokens).parse()
  if (!ast) return '#ERROR!'
  return evalNode(ctx, ast)
}

function makeCtx(doc: TableDoc): Ctx {
  return { doc, visiting: new Set(), cache: new Map() }
}

/** Evaluated value for one cell. Literals pass through typed. */
export function evaluateCell(
  doc: TableDoc,
  row: number,
  col: number,
): CellValue {
  return evalCell(makeCtx(doc), row, col)
}

/** Whole grid evaluated in one pass so shared references are computed once. */
export function evaluateGrid(doc: TableDoc): CellValue[][] {
  const ctx = makeCtx(doc)
  return doc.rows.map((row, r) => row.map((_, c) => evalCell(ctx, r, c)))
}

export function formatCellValue(value: CellValue): string {
  if (value === '') return ''
  if (typeof value === 'boolean') return value ? 'TRUE' : 'FALSE'
  if (typeof value !== 'number') return String(value)
  if (!Number.isFinite(value)) return '#NUM!'
  if (Number.isInteger(value)) return String(value)
  return String(Math.round(value * 1e10) / 1e10)
}
