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

export class FormulaError extends Error {}

interface Tok {
  t: 'num' | 'str' | 'ref' | 'name' | 'op' | '(' | ')' | 'sep'
  v: string
}

function lex(src: string): Tok[] {
  const out: Tok[] = []
  let i = 0
  while (i < src.length) {
    const c = src[i]
    if (/\s/.test(c)) {
      i++
    } else if (/[0-9.]/.test(c)) {
      const m = /^(\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?/.exec(src.slice(i))
      if (!m) throw new FormulaError(`unexpected “${c}”`)
      out.push({ t: 'num', v: m[0] })
      i += m[0].length
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
      out.push({ t: 'str', v: s })
    } else if (c === '[') {
      const end = src.indexOf(']', i)
      if (end < 0) throw new FormulaError('a [name] has no closing ]')
      out.push({ t: 'ref', v: src.slice(i + 1, end) })
      i = end + 1
    } else if (/[\p{L}_]/u.test(c)) {
      const m = /^[\p{L}_][\p{L}\p{N}_.]*/u.exec(src.slice(i))!
      out.push({ t: 'name', v: m[0] })
      i += m[0].length
    } else if (c === '(' || c === ')') {
      out.push({ t: c, v: c })
      i++
    } else if (c === ',' || c === ';') {
      out.push({ t: 'sep', v: c })
      i++
    } else {
      const two = src.slice(i, i + 2)
      if (two === '<>' || two === '<=' || two === '>=') {
        out.push({ t: 'op', v: two })
        i += 2
      } else if ('+-*/^&=<>%'.includes(c)) {
        out.push({ t: 'op', v: c })
        i++
      } else throw new FormulaError(`unexpected “${c}”`)
    }
  }
  return out
}

export function parse(src: string): Node {
  const toks = lex(src.trim().replace(/^=/, ''))
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
// Portuguese Excel's names (and a few common spellings).
const PT: Record<string, string> = {
  soma: 'sum', media: 'average', minimo: 'min', maximo: 'max', contar: 'count', 'contar.val': 'counta', 'contar.vazio': 'countblank',
  'contar.se': 'countif', 'soma.se': 'sumif', 'media.se': 'averageif', arred: 'round', 'arred.para.cima': 'roundup', 'arred.para.baixo': 'rounddown',
  int: 'int', abs: 'abs', resto: 'mod', potencia: 'power', raizq: 'sqrt', se: 'if', 'se.s': 'ifs', 'se.erro': 'iferror', e: 'and', ou: 'or', nao: 'not',
  'e.cel.vazia': 'isblank', 'e.num': 'isnumber', concatenar: 'concat', 'num.carat': 'len', esquerda: 'left', direita: 'right', 'seg.texto': 'mid',
  maiusculas: 'upper', minusculas: 'lower', compactar: 'trim', subst: 'substitute', hoje: 'today', data: 'date', ano: 'year', mes: 'month', dia: 'day',
  'dia.semana': 'weekday', dias: 'days', datadif: 'datedif', datam: 'edate', diatrabalhototal: 'networkdays', concatenate: 'concat',
}
export const FUNCTION_NAMES = Object.keys(FNS).map((k) => k.toUpperCase())

// ---- Evaluating ----

export interface Ctx {
  today: number // a day number
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
