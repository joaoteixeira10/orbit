// Math in notes, like iPhone's Math Notes: a line that ends with "=" shows
// its answer ("120 + 35 =" → 155); "rent = 650" names a value for the lines
// below ("rent * 12 =" → 7800). + − × ÷ * / ^, brackets, percentages
// ("80 + 10%", "20% of 80"), decimals with "." or ",", and a currency sign
// (€ $ £) carried to the answer. Words before the sum are left alone
// ("March: 650 * 3 ="). Its own little parser — nothing is ever eval'd.

type Tok =
  | { k: 'num'; v: number; comma: boolean }
  | { k: 'op'; v: string }
  | { k: '(' }
  | { k: ')' }
  | { k: '%' }
  | { k: 'cur'; v: string }
  | { k: 'id'; v: string }

const CURRENCY = /[€$£]/

function tokenize(s: string): Tok[] | null {
  const out: Tok[] = []
  let i = 0
  while (i < s.length) {
    const c = s[i]
    if (/\s/.test(c)) {
      i++
      continue
    }
    const num = /^\d+(?:[.,]\d+)?/.exec(s.slice(i))
    if (num) {
      out.push({ k: 'num', v: Number(num[0].replace(',', '.')), comma: num[0].includes(',') })
      i += num[0].length
      continue
    }
    const id = /^[\p{L}_][\p{L}\p{N}_]*/u.exec(s.slice(i))
    if (id) {
      out.push({ k: 'id', v: id[0] })
      i += id[0].length
      continue
    }
    if ('+-−*/×÷^'.includes(c)) out.push({ k: 'op', v: c === '−' ? '-' : c === '×' ? '*' : c === '÷' ? '/' : c })
    else if (c === '(') out.push({ k: '(' })
    else if (c === ')') out.push({ k: ')' })
    else if (c === '%') out.push({ k: '%' })
    else if (CURRENCY.test(c)) out.push({ k: 'cur', v: c })
    else return null
    i++
  }
  return out
}

interface Val {
  v: number
  pct: boolean // a bare percentage (10% = 0.1), for "80 + 10%"
}

// A recursive-descent parser over tokens[from..]; it must use them all.
function parse(toks: Tok[], vars: Map<string, number>): { v: number; uses: boolean } | null {
  let i = 0
  let uses = false // an operator or a name: more than a lone number
  const peek = () => toks[i]
  const isOp = (v: string) => {
    const t = toks[i]
    return t?.k === 'op' && t.v === v
  }
  function primary(): Val | null {
    const t = peek()
    if (!t) return null
    if (t.k === 'op' && (t.v === '-' || t.v === '+')) {
      i++
      const p = power()
      return p && { v: t.v === '-' ? -p.v : p.v, pct: p.pct }
    }
    if (t.k === 'cur') {
      i++
      return primary()
    }
    let val: Val | null = null
    if (t.k === 'num') {
      i++
      val = { v: t.v, pct: false }
    } else if (t.k === 'id') {
      const key = t.v.toLowerCase()
      if (!vars.has(key)) return null
      i++
      uses = true
      val = { v: vars.get(key)!, pct: false }
    } else if (t.k === '(') {
      i++
      const inner = sum()
      if (!inner || peek()?.k !== ')') return null
      i++
      val = { v: inner.v, pct: false }
    } else return null
    if (peek()?.k === 'cur') i++
    if (peek()?.k === '%') {
      i++
      uses = true
      val = { v: val.v / 100, pct: true }
      // "20% of 80" / "20% de 80"
      const w = peek()
      if (w?.k === 'id' && ['of', 'de', 'do', 'da'].includes(w.v.toLowerCase())) {
        i++
        const base = power()
        if (!base) return null
        val = { v: val.v * base.v, pct: false }
      }
    }
    return val
  }
  function power(): Val | null {
    const b = primary()
    if (!b) return null
    if (isOp('^')) {
      i++
      uses = true
      const e = power()
      if (!e) return null
      return { v: Math.pow(b.v, e.v), pct: false }
    }
    return b
  }
  function product(): Val | null {
    let l = power()
    while (l && (isOp('*') || isOp('/'))) {
      const op = (toks[i] as { v: string }).v
      i++
      uses = true
      const r = power()
      if (!r) return null
      l = { v: op === '*' ? l.v * r.v : l.v / r.v, pct: false }
    }
    return l
  }
  function sum(): Val | null {
    let l = product()
    while (l && (isOp('+') || isOp('-'))) {
      const op = (toks[i] as { v: string }).v
      i++
      uses = true
      const r = product()
      if (!r) return null
      // "80 + 10%" is 88, as on a calculator.
      const rv = r.pct && !l.pct ? l.v * r.v : r.v
      l = { v: op === '+' ? l.v + rv : l.v - rv, pct: false }
    }
    return l
  }
  const v = sum()
  if (!v || i !== toks.length || !Number.isFinite(v.v)) return null
  return { v: v.v, uses }
}

// Names of several words ("per person") become one ("per_person").
const keyOf = (name: string) => name.trim().toLowerCase().replace(/\s+/g, '_')
function joinNames(s: string, vars: Map<string, number>): string {
  const spaced = [...vars.keys()].filter((k) => k.includes('_')).sort((a, b) => b.length - a.length)
  for (const k of spaced) {
    const words = k.split('_').map((w) => w.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('\\s+')
    s = s.replace(new RegExp(`(?<![\\p{L}\\p{N}_])${words}(?![\\p{L}\\p{N}_])`, 'giu'), k)
  }
  return s
}

// The value of the longest calculation that ends the text, if any.
function tail(s: string, vars: Map<string, number>): { v: number; comma: boolean; cur: string } | null {
  s = joinNames(s, vars)
  const toks = tokenize(s)
  if (toks) return finish(toks, vars)
  // Words or signs we don't read before the sum: try ever shorter endings.
  for (let i = 1; i < s.length; i++) {
    if (!/\s|:|\(/.test(s[i - 1])) continue
    const t = tokenize(s.slice(i))
    if (t) {
      const r = finish(t, vars)
      if (r) return r
    }
  }
  return null
}
function finish(toks: Tok[], vars: Map<string, number>) {
  for (let from = 0; from < toks.length; from++) {
    const part = toks.slice(from)
    const r = parse(part, vars)
    if (r && r.uses) {
      const cur = part.find((t) => t.k === 'cur') as { v: string } | undefined
      return { v: r.v, comma: part.some((t) => t.k === 'num' && t.comma), cur: cur?.v ?? '' }
    }
    // Only skip leading words (a label), never part of a sum.
    if (toks[from].k !== 'id') break
  }
  return null
}

export function formatResult(v: number, comma: boolean, cur = '', curFirst = false): string {
  const r = Math.round(v * 1e9) / 1e9
  let s = String(Math.abs(r) >= 1e15 ? r.toExponential(6) : Number(r.toFixed(Math.abs(r) < 1 ? 8 : 6)))
  if (comma) s = s.replace('.', ',')
  if (!cur) return s
  return curFirst ? `${cur}${s}` : `${s} ${cur}`
}

export interface MathLine {
  line: number
  result: string // what to show (and write) after the "="
}

// evaluate reads a note line by line: names defined so far, and the answer
// of each line that ends with "=" (nothing when it can't be worked out, or
// when the answer is already written after it).
export function evaluate(text: string): MathLine[] {
  const vars = new Map<string, number>()
  const out: MathLine[] = []
  // The note's way of writing decimals, for answers of lines without any.
  const noteComma = (text.match(/\d,\d/g)?.length ?? 0) > (text.match(/\d\.\d/g)?.length ?? 0)
  const commaFor = (s: string, used: boolean) => used || (!/\d\.\d/.test(s) && noteComma)
  text.split('\n').forEach((raw, line) => {
    const s = raw.replace(/\s+$/, '')
    const asks = s.endsWith('=') && !s.endsWith('==')
    const body = asks ? s.slice(0, -1) : s
    // "name = sum" defines name (and, with a closing "=", shows it too).
    const def = /^\s*([\p{L}_][\p{L}\p{N}_]*(?:[ \t]+[\p{L}_][\p{L}\p{N}_]*){0,3})\s*=\s*([^=]+)$/u.exec(body)
    if (def) {
      const r = tail(def[2], vars)
      if (r) {
        vars.set(keyOf(def[1]), r.v)
        if (asks) out.push({ line, result: formatResult(r.v, commaFor(def[2], r.comma), r.cur, def[2].trim().startsWith(r.cur) && !!r.cur) })
      } else {
        const n = tokenize(def[2].trim())
        if (n && n.length >= 1 && n.every((t) => t.k === 'num' || t.k === 'cur')) {
          const num = n.find((t) => t.k === 'num') as { v: number } | undefined
          if (num) vars.set(keyOf(def[1]), num.v)
        }
      }
      return
    }
    if (!asks || body.includes('=')) return
    const r = tail(body, vars)
    if (r) {
      const curFirst = !!r.cur && new RegExp(`\\${r.cur}\\s*\\d`).test(body)
      out.push({ line, result: formatResult(r.v, commaFor(body, r.comma), r.cur, curFirst) })
    }
  })
  return out
}
