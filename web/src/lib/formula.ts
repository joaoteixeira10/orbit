// Formulas, the way Excel writes them, for database formula properties:
// `=[@Price]*[@Qty]`, `=IF([@Done], 0, [@Hours])`, `=SUM([Hours])`.
//
// - [@Name] is this row's value of a property; [Name] is the whole column
//   (for SUM, AVERAGE, COUNTIF…). A plain name works too when it has no
//   spaces: `Price * Qty`. Names are matched without minding case or accents.
// - Excel's operators: + - * / ^ %, & joins text, = <> < > <= >= compare.
// - Excel's function names in English or Portuguese (SUM / SOMA, IF / SE…),
//   arguments separated by , or ; (as Portuguese Excel writes them).
// - Excel's errors: #DIV/0!, #VALUE!, #NAME?, #REF!, #N/A.
//
// In a sheet (parse with { cells: true }) it also reads Excel's cell
// references: A1, $A$1, B2:B20, A:A, Tab2!A1, 'My tab'!A1:B5.
//
// It's parsed into a tree and walked — nothing is ever run as code.

export type FDate = { date: number } // a day number (days since 1970-01-01, UTC)
export type FError = { error: string }
export type FValue = number | string | boolean | FDate | FError | null | FValue[]

export const isErr = (v: unknown): v is FError => typeof v === 'object' && v !== null && 'error' in v
export const isDate = (v: unknown): v is FDate => typeof v === 'object' && v !== null && 'date' in v
const err = (e: string): FError => ({ error: e })

// ---- Parsing ----

type Node =
  | { k: 'num'; v: number }
  | { k: 'str'; v: string }
  | { k: 'bool'; v: boolean }
  | { k: 'ref'; name: string; col: boolean } // [@x] (this row) or [x] (the column)
  | { k: 'un'; op: string; a: Node }
  | { k: 'pct'; a: Node }
  | { k: 'bin'; op: string; a: Node; b: Node }
  | { k: 'call'; fn: string; args: Node[] }
  | { k: 'cells'; ref: CellRef }

export class FormulaError extends Error {}

// A cell (0-based; row -1: the whole column) and whether each part is
// absolute ($).
export interface CellPos {
  col: number
  row: number
  absCol: boolean
  absRow: boolean
}
// A reference: one cell, or a range from a to b; on another tab when sheet.
export interface CellRef {
  sheet?: string
  a: CellPos
  b?: CellPos
}

interface Tok {
  t: 'num' | 'str' | 'ref' | 'name' | 'op' | '(' | ')' | 'sep' | 'cells'
  v: string
  s: number // where it starts in the source
  e: number // and ends
  cell?: CellRef
}

export const colIndex = (letters: string) => [...letters.toUpperCase()].reduce((n, ch) => n * 26 + ch.charCodeAt(0) - 64, 0) - 1
export const colName = (i: number): string => (i < 26 ? String.fromCharCode(65 + i) : colName(Math.floor(i / 26) - 1) + String.fromCharCode(65 + (i % 26)))

const SHEET = /^(?:'((?:[^']|'')+)'|([\p{L}_][\p{L}\p{N}_.]*))!/u
const PART = /^(\$?)([A-Za-z]{1,3})(?:(\$?)(\d{1,7}))?/
const goesOn = (ch: string | undefined) => !!ch && /[\p{L}\p{N}_(.$]/u.test(ch)

// cellAt reads a reference at src[i] ('' when there's none there).
function cellAt(src: string, i: number): { ref: CellRef; end: number } | null {
  let j = i
  let sheet: string | undefined
  const sm = SHEET.exec(src.slice(j))
  if (sm) {
    sheet = sm[1] !== undefined ? sm[1].replace(/''/g, "'") : sm[2]
    j += sm[0].length
  }
  const p1 = PART.exec(src.slice(j))
  if (!p1) {
    if (sheet) throw new FormulaError(`${sheet}! needs a cell after it`)
    return null
  }
  const pos = (m: RegExpExecArray): CellPos => ({ col: colIndex(m[2]), row: m[4] !== undefined ? Number(m[4]) - 1 : -1, absCol: m[1] === '$', absRow: m[3] === '$' })
  const end = j + p1[0].length
  const a = pos(p1)
  if (src[end] === ':') {
    const p2 = PART.exec(src.slice(end + 1))
    if (p2 && !goesOn(src[end + 1 + p2[0].length])) {
      const b = pos(p2)
      if ((a.row < 0) === (b.row < 0)) return { ref: { sheet, a, b }, end: end + 1 + p2[0].length }
    }
  }
  if (a.row < 0 || goesOn(src[end])) {
    if (sheet) throw new FormulaError(`${sheet}! needs a cell after it`)
    return null
  }
  return { ref: { sheet, a }, end }
}

function lex(src: string, cells = false): Tok[] {
  const out: Tok[] = []
  let i = 0
  const push = (t: Tok['t'], v: string, start: number) => out.push({ t, v, s: start, e: i })
  while (i < src.length) {
    const c = src[i]
    const start = i
    if (cells && !/\s/.test(c) && (out.length === 0 || !['num', 'str', 'ref', 'cells', ')'].includes(out[out.length - 1].t) || out[out.length - 1].t === 'op')) {
      const hit = /[\p{L}_$']/u.test(c) ? cellAt(src, i) : null
      if (hit) {
        i = hit.end
        out.push({ t: 'cells', v: src.slice(start, i), s: start, e: i, cell: hit.ref })
        continue
      }
    }
    if (/\s/.test(c)) {
      i++
    } else if (/[0-9.]/.test(c)) {
      const m = /^(\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?/.exec(src.slice(i))
      if (!m) throw new FormulaError(`unexpected “${c}”`)
      i += m[0].length
      push('num', m[0], start)
    } else if (c === '"') {
      let s = ''
      i++
      for (;;) {
        if (i >= src.length) throw new FormulaError('a text has no closing "')
        if (src[i] === '"') {
          if (src[i + 1] === '"') {
            s += '"'
            i += 2
            continue
          }
          i++
          break
        }
        s += src[i++]
      }
      push('str', s, start)
    } else if (c === '[') {
      const end = src.indexOf(']', i)
      if (end < 0) throw new FormulaError('a [name] has no closing ]')
      i = end + 1
      push('ref', src.slice(start + 1, end), start)
    } else if (/[\p{L}_]/u.test(c)) {
      const m = /^[\p{L}_][\p{L}\p{N}_.]*/u.exec(src.slice(i))!
      i += m[0].length
      push('name', m[0], start)
    } else if (c === '(' || c === ')') {
      i++
      push(c, c, start)
    } else if (c === ',' || c === ';') {
      i++
      push('sep', c, start)
    } else {
      const two = src.slice(i, i + 2)
      if (two === '<>' || two === '<=' || two === '>=') {
        i += 2
        push('op', two, start)
      } else if ('+-*/^&=<>%'.includes(c)) {
        i++
        push('op', c, start)
      } else throw new FormulaError(`unexpected “${c}”`)
    }
  }
  return out
}

export function parse(src: string, opts: { cells?: boolean } = {}): Node {
  const toks = lex(src.trim().replace(/^=/, ''), !!opts.cells)
  if (!toks.length) throw new FormulaError('the formula is empty')
  let p = 0
  const peek = () => toks[p]
  const next = () => toks[p++]
  const isOp = (...ops: string[]) => peek()?.t === 'op' && ops.includes(peek().v)

  // Excel's precedence, lowest first: comparison, &, + -, * /, ^, unary -, %.
  const cmp = (): Node => {
    let a = cat()
    while (isOp('=', '<>', '<', '>', '<=', '>=')) a = { k: 'bin', op: next().v, a, b: cat() }
    return a
  }
  const cat = (): Node => {
    let a = add()
    while (isOp('&')) a = { k: 'bin', op: next().v, a, b: add() }
    return a
  }
  const add = (): Node => {
    let a = mul()
    while (isOp('+', '-')) a = { k: 'bin', op: next().v, a, b: mul() }
    return a
  }
  const mul = (): Node => {
    let a = pow()
    while (isOp('*', '/')) a = { k: 'bin', op: next().v, a, b: pow() }
    return a
  }
  const pow = (): Node => {
    let a = unary()
    while (isOp('^')) a = { k: 'bin', op: next().v, a, b: unary() }
    return a
  }
  const unary = (): Node => {
    if (isOp('-', '+')) {
      const op = next().v
      return { k: 'un', op, a: unary() }
    }
    let a = atom()
    while (isOp('%')) {
      next()
      a = { k: 'pct', a }
    }
    return a
  }
  const atom = (): Node => {
    const t = next()
    if (!t) throw new FormulaError('the formula ends too soon')
    if (t.t === 'num') return { k: 'num', v: Number(t.v) }
    if (t.t === 'cells') return { k: 'cells', ref: t.cell! }
    if (t.t === 'str') return { k: 'str', v: t.v }
    if (t.t === 'ref') return t.v.startsWith('@') ? { k: 'ref', name: t.v.slice(1).trim(), col: false } : { k: 'ref', name: t.v.trim(), col: true }
    if (t.t === '(') {
      const e = cmp()
      if (next()?.t !== ')') throw new FormulaError('a ( has no closing )')
      return e
    }
    if (t.t === 'name') {
      if (peek()?.t === '(') {
        next()
        const args: Node[] = []
        if (peek()?.t !== ')') {
          for (;;) {
            args.push(cmp())
            if (peek()?.t === 'sep') {
              next()
              continue
            }
            break
          }
        }
        if (next()?.t !== ')') throw new FormulaError(`${t.v}( has no closing )`)
        return { k: 'call', fn: t.v, args }
      }
      const u = fold(t.v)
      if (u === 'true' || u === 'verdadeiro') return { k: 'bool', v: true }
      if (u === 'false' || u === 'falso') return { k: 'bool', v: false }
      return { k: 'ref', name: t.v, col: false } // a plain name: this row's
    }
    throw new FormulaError(`unexpected “${t.v}”`)
  }
  const tree = cmp()
  if (p < toks.length) throw new FormulaError(`unexpected “${toks[p].v}”`)
  return tree
}

// ---- Values ----

const fold = (s: string) => s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase().trim()
const DAY = 86400000
export const dayOf = (iso: string) => {
  const [y, m, d] = iso.split('-').map(Number)
  return Math.round(Date.UTC(y, m - 1, d) / DAY)
}
export const isoOf = (n: number) => {
  const d = new Date(n * DAY)
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, '0')}-${String(d.getUTCDate()).padStart(2, '0')}`
}
const parts = (n: number) => {
  const d = new Date(n * DAY)
  return { y: d.getUTCFullYear(), m: d.getUTCMonth() + 1, d: d.getUTCDate(), wd: d.getUTCDay() }
}

function num(v: FValue): number | FError {
  if (isErr(v)) return v
  if (v === null || v === '') return 0
  if (typeof v === 'number') return v
  if (typeof v === 'boolean') return v ? 1 : 0
  if (isDate(v)) return v.date
  if (Array.isArray(v)) return v.length === 1 ? num(v[0]) : err('#VALUE!')
  const n = Number(String(v).replace(',', '.'))
  return Number.isFinite(n) && String(v).trim() !== '' ? n : err('#VALUE!')
}
function text(v: FValue): string | FError {
  if (isErr(v)) return v
  if (v === null) return ''
  if (typeof v === 'boolean') return v ? 'TRUE' : 'FALSE'
  if (isDate(v)) return isoOf(v.date)
  if (Array.isArray(v)) return v.map((x) => text(x)).filter((x) => typeof x === 'string').join(', ')
  if (typeof v === 'number') return String(Math.round(v * 1e10) / 1e10)
  return v
}
function truthy(v: FValue): boolean | FError {
  if (isErr(v)) return v
  if (typeof v === 'string') {
    const f = fold(v)
    if (f === 'true' || f === 'verdadeiro') return true
    if (f === 'false' || f === 'falso' || f === '') return false
    return err('#VALUE!')
  }
  const n = num(v)
  return isErr(n) ? n : n !== 0
}
const flat = (vs: FValue[]): FValue[] => vs.flatMap((v) => (Array.isArray(v) ? flat(v) : [v]))
const blank = (v: FValue) => v === null || v === ''

function compare(a: FValue, b: FValue): number | FError {
  if (isErr(a)) return a
  if (isErr(b)) return b
  const an = typeof a === 'number' || isDate(a) || typeof a === 'boolean' || a === null
  const bn = typeof b === 'number' || isDate(b) || typeof b === 'boolean' || b === null
  if (an && bn) {
    const x = num(a) as number
    const y = num(b) as number
    return x < y ? -1 : x > y ? 1 : 0
  }
  const x = fold(text(a) as string)
  const y = fold(text(b) as string)
  return x < y ? -1 : x > y ? 1 : 0
}

// criteria as COUNTIF reads it: ">5", "<>done", "done", 3.
function meets(v: FValue, crit: FValue): boolean {
  if (typeof crit === 'string') {
    const m = /^(<>|<=|>=|=|<|>)?(.*)$/.exec(crit)!
    const op = m[1] ?? '='
    const raw = m[2]
    const target: FValue = raw !== '' && Number.isFinite(Number(raw)) ? Number(raw) : raw
    const c = compare(v ?? '', target)
    if (isErr(c)) return false
    return { '=': c === 0, '<>': c !== 0, '<': c < 0, '>': c > 0, '<=': c <= 0, '>=': c >= 0 }[op] ?? false
  }
  const c = compare(v, crit)
  return !isErr(c) && c === 0
}

// ---- Functions ----

type Fn = (args: FValue[], ctx: Ctx) => FValue
const nums = (args: FValue[]): number[] | FError => {
  const out: number[] = []
  for (const v of flat(args)) {
    if (isErr(v)) return v
    if (typeof v === 'number') out.push(v)
    else if (isDate(v)) out.push(v.date)
  }
  return out
}
const n1 = (f: (x: number, y: number) => FValue, def = 0): Fn => (a) => {
  const x = num(a[0] ?? null)
  const y = num(a[1] ?? def)
  return isErr(x) ? x : isErr(y) ? y : f(x, y)
}
const t1 = (f: (s: string, n: number, m: number) => FValue): Fn => (a) => {
  const s = text(a[0] ?? null)
  const n = a.length > 1 ? num(a[1]) : 1
  const m = a.length > 2 ? num(a[2]) : 0
  return isErr(s) ? s : isErr(n) ? n : isErr(m) ? m : f(s, n, m)
}
const date = (a: FValue): number | FError => {
  if (isDate(a)) return a.date
  if (typeof a === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(a)) return dayOf(a)
  const n = num(a)
  return isErr(n) ? n : n
}
const round = (x: number, d: number, how: 'near' | 'up' | 'down') => {
  const f = 10 ** d
  const v = Math.abs(x) * f
  const r = how === 'near' ? Math.round(v + 1e-9) : how === 'up' ? Math.ceil(v - 1e-9) : Math.floor(v + 1e-9)
  return (Math.sign(x) * r) / f
}

const FNS: Record<string, Fn> = {
  sum: (a) => {
    const n = nums(a)
    return isErr(n) ? n : n.reduce((s, x) => s + x, 0)
  },
  average: (a) => {
    const n = nums(a)
    return isErr(n) ? n : n.length ? n.reduce((s, x) => s + x, 0) / n.length : err('#DIV/0!')
  },
  min: (a) => {
    const n = nums(a)
    return isErr(n) ? n : n.length ? Math.min(...n) : 0
  },
  max: (a) => {
    const n = nums(a)
    return isErr(n) ? n : n.length ? Math.max(...n) : 0
  },
  count: (a) => flat(a).filter((v) => typeof v === 'number' || isDate(v)).length,
  counta: (a) => flat(a).filter((v) => !blank(v)).length,
  countblank: (a) => flat(a).filter(blank).length,
  countif: (a) => flat([a[0] ?? null]).filter((v) => meets(v, a[1] ?? null)).length,
  sumif: (a) => {
    const range = flat([a[0] ?? null])
    const sums = a.length > 2 ? flat([a[2]]) : range
    let s = 0
    range.forEach((v, i) => {
      const x = num(sums[i] ?? null)
      if (meets(v, a[1] ?? null) && !isErr(x)) s += x
    })
    return s
  },
  averageif: (a) => {
    const range = flat([a[0] ?? null])
    const vals = a.length > 2 ? flat([a[2]]) : range
    const xs: number[] = []
    range.forEach((v, i) => {
      const x = num(vals[i] ?? null)
      if (meets(v, a[1] ?? null) && !isErr(x)) xs.push(x)
    })
    return xs.length ? xs.reduce((s, x) => s + x, 0) / xs.length : err('#DIV/0!')
  },
  round: n1((x, d) => round(x, d, 'near')),
  roundup: n1((x, d) => round(x, d, 'up')),
  rounddown: n1((x, d) => round(x, d, 'down')),
  int: n1((x) => Math.floor(x)),
  abs: n1((x) => Math.abs(x)),
  mod: n1((x, y) => (y === 0 ? err('#DIV/0!') : x - y * Math.floor(x / y))),
  power: n1((x, y) => x ** y),
  sqrt: n1((x) => (x < 0 ? err('#NUM!') : Math.sqrt(x))),
  if: (a) => {
    const c = truthy(a[0] ?? false)
    return isErr(c) ? c : c ? (a[1] ?? true) : (a[2] ?? false)
  },
  ifs: (a) => {
    for (let i = 0; i + 1 < a.length; i += 2) {
      const c = truthy(a[i])
      if (isErr(c)) return c
      if (c) return a[i + 1]
    }
    return err('#N/A')
  },
  iferror: (a) => (isErr(a[0]) ? (a[1] ?? '') : a[0]),
  and: (a) => {
    for (const v of flat(a)) {
      const c = truthy(v)
      if (isErr(c)) return c
      if (!c) return false
    }
    return true
  },
  or: (a) => {
    for (const v of flat(a)) {
      const c = truthy(v)
      if (isErr(c)) return c
      if (c) return true
    }
    return false
  },
  not: (a) => {
    const c = truthy(a[0] ?? false)
    return isErr(c) ? c : !c
  },
  isblank: (a) => blank(a[0] ?? null),
  isnumber: (a) => typeof a[0] === 'number',
  concat: (a) => {
    let s = ''
    for (const v of flat(a)) {
      const t = text(v)
      if (isErr(t)) return t
      s += t
    }
    return s
  },
  len: t1((s) => [...s].length),
  left: t1((s, n) => [...s].slice(0, n).join('')),
  right: t1((s, n) => (n <= 0 ? '' : [...s].slice(-n).join(''))),
  mid: t1((s, n, m) => [...s].slice(n - 1, n - 1 + m).join('')),
  upper: t1((s) => s.toLocaleUpperCase()),
  lower: t1((s) => s.toLocaleLowerCase()),
  trim: t1((s) => s.trim().replace(/\s+/g, ' ')),
  substitute: (a) => {
    const s = text(a[0] ?? null)
    const o = text(a[1] ?? null)
    const n = text(a[2] ?? null)
    return isErr(s) ? s : isErr(o) ? o : isErr(n) ? n : o ? s.split(o).join(n) : s
  },
  today: (_a, ctx) => ({ date: ctx.today }),
  date: (a) => {
    const [y, m, d] = [num(a[0] ?? null), num(a[1] ?? null), num(a[2] ?? null)]
    for (const x of [y, m, d]) if (isErr(x)) return x
    return { date: Math.round(Date.UTC(y as number, (m as number) - 1, d as number) / DAY) }
  },
  year: (a) => {
    const d = date(a[0] ?? null)
    return isErr(d) ? d : parts(d).y
  },
  month: (a) => {
    const d = date(a[0] ?? null)
    return isErr(d) ? d : parts(d).m
  },
  day: (a) => {
    const d = date(a[0] ?? null)
    return isErr(d) ? d : parts(d).d
  },
  weekday: (a) => {
    const d = date(a[0] ?? null)
    return isErr(d) ? d : parts(d).wd + 1 // Excel: Sunday is 1
  },
  days: (a) => {
    const e = date(a[0] ?? null)
    const s = date(a[1] ?? null)
    return isErr(e) ? e : isErr(s) ? s : e - s
  },
  datedif: (a) => {
    const s = date(a[0] ?? null)
    const e = date(a[1] ?? null)
    const u = text(a[2] ?? 'D')
    if (isErr(s)) return s
    if (isErr(e)) return e
    if (isErr(u)) return u
    if (e < s) return err('#NUM!')
    const p = parts(s)
    const q = parts(e)
    const months = (q.y - p.y) * 12 + (q.m - p.m) - (q.d < p.d ? 1 : 0)
    return { D: e - s, M: months, Y: Math.floor(months / 12) }[u.toUpperCase() as 'D'] ?? err('#NUM!')
  },
  edate: (a) => {
    const s = date(a[0] ?? null)
    const m = num(a[1] ?? 0)
    if (isErr(s)) return s
    if (isErr(m)) return m
    const p = parts(s)
    const last = new Date(Date.UTC(p.y, p.m - 1 + m + 1, 0)).getUTCDate()
    return { date: Math.round(Date.UTC(p.y, p.m - 1 + m, Math.min(p.d, last)) / DAY) }
  },
  networkdays: (a) => {
    const s = date(a[0] ?? null)
    const e = date(a[1] ?? null)
    if (isErr(s)) return s
    if (isErr(e)) return e
    let n = 0
    const [from, to, sign] = s <= e ? [s, e, 1] : [e, s, -1]
    for (let d = from; d <= to; d++) if (parts(d).wd % 6 !== 0) n++
    return n * sign
  },
}
// Lookups and more, mostly for sheets (ranges are rows of values).
const grid = (v: FValue): FValue[][] => (Array.isArray(v) ? (v.every(Array.isArray) ? (v as FValue[][]) : v.map((x) => [x])) : [[v]])
const same = (a: FValue, b: FValue) => {
  const c = compare(a, b)
  return !isErr(c) && c === 0
}
Object.assign(FNS, {
  vlookup: ((a) => {
    const rows = grid(a[1] ?? null)
    const col = num(a[2] ?? 1)
    if (isErr(col)) return col
    const approx = a.length > 3 ? truthy(a[3]) : true
    if (isErr(approx)) return approx
    let hit = -1
    for (let i = 0; i < rows.length; i++) {
      if (same(rows[i][0], a[0] ?? null)) {
        hit = i
        break
      }
      // Approximate (Excel's default, a sorted first column): the last one not above it.
      if (approx) {
        const c = compare(rows[i][0], a[0] ?? null)
        if (!isErr(c) && c <= 0 && !blank(rows[i][0])) hit = i
      }
    }
    if (hit < 0) return err('#N/A')
    const v = rows[hit][col - 1]
    return v === undefined ? err('#REF!') : v
  }) as Fn,
  hlookup: ((a) => {
    const rows = grid(a[1] ?? null)
    const row = num(a[2] ?? 1)
    if (isErr(row)) return row
    const i = (rows[0] ?? []).findIndex((v) => same(v, a[0] ?? null))
    if (i < 0) return err('#N/A')
    const v = rows[row - 1]?.[i]
    return v === undefined ? err('#REF!') : v
  }) as Fn,
  index: ((a) => {
    const rows = grid(a[0] ?? null)
    const r = num(a[1] ?? 1)
    const c = num(a[2] ?? 1)
    if (isErr(r)) return r
    if (isErr(c)) return c
    const one = rows.length === 1 && a.length < 3 // a single row: INDEX(row, n)
    const v = one ? rows[0][r - 1] : rows[r - 1]?.[(c || 1) - 1]
    return v === undefined ? err('#REF!') : v
  }) as Fn,
  match: ((a) => {
    const list = flat([a[1] ?? null])
    const type = a.length > 2 ? num(a[2]) : 1
    if (isErr(type)) return type
    if (type === 0) {
      const i = list.findIndex((v) => same(v, a[0] ?? null))
      return i < 0 ? err('#N/A') : i + 1
    }
    let hit = -1
    list.forEach((v, i) => {
      const c = compare(v, a[0] ?? null)
      if (!isErr(c) && !blank(v) && (type > 0 ? c <= 0 : c >= 0)) hit = i
    })
    return hit < 0 ? err('#N/A') : hit + 1
  }) as Fn,
  xlookup: ((a) => {
    const keys = flat([a[1] ?? null])
    const vals = flat([a[2] ?? null])
    const i = keys.findIndex((v) => same(v, a[0] ?? null))
    if (i < 0) return a.length > 3 ? a[3] : err('#N/A')
    return vals[i] ?? err('#REF!')
  }) as Fn,
  choose: ((a) => {
    const i = num(a[0] ?? null)
    if (isErr(i)) return i
    return a[i] ?? err('#VALUE!')
  }) as Fn,
  product: ((a) => {
    const n = nums(a)
    return isErr(n) ? n : n.reduce((p, x) => p * x, 1)
  }) as Fn,
  median: ((a) => {
    const n = nums(a)
    if (isErr(n)) return n
    if (!n.length) return err('#NUM!')
    const s = [...n].sort((x, y) => x - y)
    const m = s.length >> 1
    return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2
  }) as Fn,
  sumifs: ((a) => ifs(a, 1, 'sum')) as Fn,
  countifs: ((a) => ifs(a, 0, 'count')) as Fn,
  averageifs: ((a) => ifs(a, 1, 'avg')) as Fn,
  pi: (() => Math.PI) as Fn,
  ln: n1((x) => (x <= 0 ? err('#NUM!') : Math.log(x))),
  log: n1((x, b) => (x <= 0 || b <= 0 || b === 1 ? err('#NUM!') : Math.log(x) / Math.log(b)), 10),
  log10: n1((x) => (x <= 0 ? err('#NUM!') : Math.log10(x))),
  exp: n1((x) => Math.exp(x)),
  ceiling: n1((x, f) => (f === 0 ? 0 : Math.ceil(x / f - 1e-9) * f), 1),
  floor: n1((x, f) => (f === 0 ? 0 : Math.floor(x / f + 1e-9) * f), 1),
  trunc: n1((x, d) => round(x, d, 'down')),
  sign: n1((x) => Math.sign(x)),
  value: ((a) => num(typeof a[0] === 'string' ? a[0].replace(/\s/g, '') : (a[0] ?? null))) as Fn,
  istext: ((a) => typeof a[0] === 'string') as Fn,
  iserror: ((a) => isErr(a[0] ?? null)) as Fn,
  textjoin: ((a) => {
    const sep = text(a[0] ?? '')
    const skip = truthy(a[1] ?? true)
    if (isErr(sep)) return sep
    if (isErr(skip)) return skip
    const parts: string[] = []
    for (const v of flat(a.slice(2))) {
      if (skip && blank(v)) continue
      const t = text(v)
      if (isErr(t)) return t
      parts.push(t)
    }
    return parts.join(sep)
  }) as Fn,
  rept: t1((s, n) => (n < 0 ? err('#VALUE!') : s.repeat(Math.floor(n)))),
  proper: t1((s) => s.toLocaleLowerCase().replace(/(^|[^\p{L}])(\p{L})/gu, (_m, p, ch: string) => p + ch.toLocaleUpperCase())),
  find: ((a) => {
    const what = text(a[0] ?? '')
    const within = text(a[1] ?? '')
    const from = num(a[2] ?? 1)
    if (isErr(what)) return what
    if (isErr(within)) return within
    if (isErr(from)) return from
    const i = within.indexOf(what, from - 1)
    return i < 0 ? err('#VALUE!') : i + 1
  }) as Fn,
  search: ((a) => {
    const what = text(a[0] ?? '')
    const within = text(a[1] ?? '')
    if (isErr(what)) return what
    if (isErr(within)) return within
    const i = fold(within).indexOf(fold(what))
    return i < 0 ? err('#VALUE!') : i + 1
  }) as Fn,
  eomonth: ((a) => {
    const s = date(a[0] ?? null)
    const m = num(a[1] ?? 0)
    if (isErr(s)) return s
    if (isErr(m)) return m
    const p = parts(s)
    return { date: Math.round(Date.UTC(p.y, p.m + m, 0) / DAY) }
  }) as Fn,
})

// ifs: SUMIFS / COUNTIFS / AVERAGEIFS — the ranges and criteria in pairs
// (after the values to add up, when there are).
function ifs(a: FValue[], from: number, how: 'sum' | 'count' | 'avg'): FValue {
  const vals = from ? flat([a[0] ?? null]) : []
  const pairs: [FValue[], FValue][] = []
  for (let i = from; i + 1 < a.length; i += 2) pairs.push([flat([a[i]]), a[i + 1]])
  if (!pairs.length) return err('#VALUE!')
  const n = pairs[0][0].length
  let sum = 0
  let count = 0
  for (let i = 0; i < n; i++) {
    if (!pairs.every(([range, crit]) => meets(range[i] ?? null, crit))) continue
    count++
    if (from) {
      const x = num(vals[i] ?? null)
      if (!isErr(x)) sum += x
    }
  }
  if (how === 'count') return count
  if (how === 'sum') return sum
  return count ? sum / count : err('#DIV/0!')
}

// Portuguese Excel's names (and a few common spellings).
const PT: Record<string, string> = {
  soma: 'sum', media: 'average', minimo: 'min', maximo: 'max', contar: 'count', 'contar.val': 'counta', 'contar.vazio': 'countblank',
  'contar.se': 'countif', 'soma.se': 'sumif', 'media.se': 'averageif', arred: 'round', 'arred.para.cima': 'roundup', 'arred.para.baixo': 'rounddown',
  int: 'int', abs: 'abs', resto: 'mod', potencia: 'power', raizq: 'sqrt', se: 'if', 'se.s': 'ifs', 'se.erro': 'iferror', e: 'and', ou: 'or', nao: 'not',
  'e.cel.vazia': 'isblank', 'e.num': 'isnumber', concatenar: 'concat', 'num.carat': 'len', esquerda: 'left', direita: 'right', 'seg.texto': 'mid',
  maiusculas: 'upper', minusculas: 'lower', compactar: 'trim', subst: 'substitute', hoje: 'today', data: 'date', ano: 'year', mes: 'month', dia: 'day',
  'dia.semana': 'weekday', dias: 'days', datadif: 'datedif', datam: 'edate', diatrabalhototal: 'networkdays', concatenate: 'concat',
  procv: 'vlookup', proch: 'hlookup', indice: 'index', corresp: 'match', procx: 'xlookup', seleccionar: 'choose', escolher: 'choose',
  produto: 'product', med: 'median', 'soma.se.s': 'sumifs', 'contar.se.s': 'countifs', 'media.se.s': 'averageifs', ln: 'ln', log: 'log',
  log10: 'log10', exp: 'exp', 'arred.excesso': 'ceiling', 'arred.defeito': 'floor', truncar: 'trunc', sinal: 'sign', valor: 'value',
  'e.texto': 'istext', 'e.erro': 'iserror', 'unir.texto': 'textjoin', repetir: 'rept', 'inicial.maiuscula': 'proper', localizar: 'search',
  procurar: 'find', 'fim.mes': 'eomonth',
}
export const FUNCTION_NAMES = Object.keys(FNS).map((k) => k.toUpperCase())

// ---- Evaluating ----

export interface Ctx {
  today: number // a day number
  // cells: a sheet's cell (one value) or range (rows of values); none: #REF!
  cells?: (ref: CellRef) => FValue | FValue[][]
  // row: this row's value of a property by name ('' unknown → #NAME?)
  row: (name: string) => FValue | undefined
  // column: every row's values of a property by name
  column: (name: string) => FValue[] | undefined
}

function evalNode(n: Node, c: Ctx): FValue {
  switch (n.k) {
    case 'num':
    case 'str':
    case 'bool':
      return n.v
    case 'ref': {
      const v = n.col ? c.column(n.name) : c.row(n.name)
      return v === undefined ? err('#NAME?') : v
    }
    case 'cells':
      return c.cells ? (c.cells(n.ref) as FValue) : err('#REF!')
    case 'un': {
      const x = num(evalNode(n.a, c))
      return isErr(x) ? x : n.op === '-' ? -x : x
    }
    case 'pct': {
      const x = num(evalNode(n.a, c))
      return isErr(x) ? x : x / 100
    }
    case 'bin': {
      const a = evalNode(n.a, c)
      const b = evalNode(n.b, c)
      if (n.op === '&') {
        const x = text(a)
        const y = text(b)
        return isErr(x) ? x : isErr(y) ? y : x + y
      }
      if (['=', '<>', '<', '>', '<=', '>='].includes(n.op)) {
        const r = compare(a, b)
        if (isErr(r)) return r
        return { '=': r === 0, '<>': r !== 0, '<': r < 0, '>': r > 0, '<=': r <= 0, '>=': r >= 0 }[n.op]!
      }
      const x = num(a)
      const y = num(b)
      if (isErr(x)) return x
      if (isErr(y)) return y
      switch (n.op) {
        case '+':
          // A date and a number of days is a date.
          return isDate(a) !== isDate(b) ? { date: x + y } : x + y
        case '-':
          return isDate(a) && !isDate(b) ? { date: x - y } : x - y
        case '*':
          return x * y
        case '/':
          return y === 0 ? err('#DIV/0!') : x / y
        case '^':
          return x ** y
      }
      return err('#VALUE!')
    }
    case 'call': {
      const name = fold(n.fn)
      const fn = FNS[name] ?? FNS[PT[name] ?? '']
      if (!fn) return err('#NAME?')
      // IF and IFERROR only look at the side they take (an error on the
      // other side doesn't matter).
      if (fn === FNS.if) {
        const cond = truthy(evalNode(n.args[0] ?? { k: 'bool', v: false }, c))
        if (isErr(cond)) return cond
        const pick = cond ? n.args[1] : n.args[2]
        return pick ? evalNode(pick, c) : cond
      }
      if (fn === FNS.iferror) {
        const v = evalNode(n.args[0] ?? { k: 'str', v: '' }, c)
        return isErr(v) ? (n.args[1] ? evalNode(n.args[1], c) : '') : v
      }
      return fn(
        n.args.map((a) => evalNode(a, c)),
        c,
      )
    }
  }
}

// evaluate runs a parsed formula (a single value: a column alone is #VALUE!).
export function evaluate(tree: Node, c: Ctx): FValue {
  const v = evalNode(tree, c)
  if (Array.isArray(v)) return err('#VALUE!')
  if (typeof v === 'number' && !Number.isFinite(v)) return err('#NUM!')
  return v
}

// names a formula refers to (to check them, or to order formulas).
export function refsOf(tree: Node): string[] {
  const out: string[] = []
  const walk = (n: Node) => {
    if (n.k === 'ref') out.push(n.name)
    else if (n.k === 'cells') out.push(formatRef(n.ref))
    else if (n.k === 'un' || n.k === 'pct') walk(n.a)
    else if (n.k === 'bin') {
      walk(n.a)
      walk(n.b)
    } else if (n.k === 'call') n.args.forEach(walk)
  }
  walk(tree)
  return out
}
export { fold as foldName }

// ---- References in a sheet's formulas ----

const plainSheet = /^[\p{L}_][\p{L}\p{N}_.]*$/u
export function formatPos(p: CellPos): string {
  return `${p.absCol ? '$' : ''}${colName(p.col)}${p.row >= 0 ? `${p.absRow ? '$' : ''}${p.row + 1}` : ''}`
}
export function formatRef(r: CellRef): string {
  const sheet = r.sheet === undefined ? '' : plainSheet.test(r.sheet) ? `${r.sheet}!` : `'${r.sheet.replace(/'/g, "''")}'!`
  return sheet + formatPos(r.a) + (r.b ? `:${formatPos(r.b)}` : '')
}

// rewriteRefs gives a formula back with each of its references changed by
// fn (null: the reference is gone — #REF!, as in Excel). Text in quotes and
// everything else stays as written. A formula that can't be read stays too.
export function rewriteRefs(src: string, fn: (r: CellRef) => CellRef | null): string {
  const lead = /^\s*=/.exec(src)
  if (!lead) return src
  const body = src.slice(lead[0].length)
  let toks: Tok[]
  try {
    toks = lex(body, true)
  } catch {
    return src
  }
  let out = ''
  let last = 0
  for (const t of toks) {
    if (t.t !== 'cells') continue
    const next = fn(t.cell!)
    out += body.slice(last, t.s) + (next ? formatRef(next) : '#REF!')
    last = t.e
  }
  return lead[0] + out + body.slice(last)
}

// shiftFormula moves a formula's relative references by rows and columns
// (copying it elsewhere, the fill handle). One that falls off the sheet
// becomes #REF!.
export function shiftFormula(src: string, dRow: number, dCol: number): string {
  if (!dRow && !dCol) return src
  const move = (p: CellPos): CellPos | null => {
    const q = { ...p, col: p.absCol ? p.col : p.col + dCol, row: p.absRow || p.row < 0 ? p.row : p.row + dRow }
    return q.col < 0 || (q.row < 0 && p.row >= 0) ? null : q
  }
  return rewriteRefs(src, (r) => {
    const a = move(r.a)
    const b = r.b ? move(r.b) : undefined
    return a && b !== null ? { ...r, a, b } : null
  })
}

// toExcel writes a formula the way Excel files keep it: English function
// names, "," between arguments, TRUE / FALSE (no leading "=").
export function toExcel(src: string): string {
  const body = src.replace(/^\s*=/, '')
  let toks: Tok[]
  try {
    toks = lex(body, true)
  } catch {
    return body
  }
  let out = ''
  let last = 0
  toks.forEach((t, i) => {
    let rep: string | null = null
    if (t.t === 'sep') rep = ','
    else if (t.t === 'name') {
      const f = fold(t.v)
      if (toks[i + 1]?.t === '(') rep = (FNS[f] ? f : (PT[f] ?? f)).toUpperCase()
      else if (f === 'verdadeiro' || f === 'true') rep = 'TRUE'
      else if (f === 'falso' || f === 'false') rep = 'FALSE'
    }
    if (rep === null) return
    out += body.slice(last, t.s) + rep
    last = t.e
  })
  return out + body.slice(last)
}
