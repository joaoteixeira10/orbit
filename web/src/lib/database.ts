// Databases: a page whose sub-pages are rows with values for its properties,
// seen through saved views (a table or a board; filtered, sorted, some
// properties hidden). Shapes mirror api/internal/planner/database.go. This
// is the logic: matching filters, sorting, grouping for a board, and how a
// value reads.
import { addDays, dateStr, locale, parseDate } from './dates'
import { FormulaError, dayOf, evaluate, foldName, isDate as isFDate, isErr, isoOf, parse, type Ctx, type FValue } from './formula'

export type PropType = 'text' | 'number' | 'select' | 'multi_select' | 'date' | 'checkbox' | 'person' | 'url' | 'formula'
export const PROP_TYPES: PropType[] = ['text', 'number', 'select', 'multi_select', 'date', 'checkbox', 'person', 'url', 'formula']

export interface DBOption {
  id: string
  name: string
  color?: string
}
export interface DBProp {
  id: string
  name: string
  type: PropType
  options?: DBOption[]
  formula?: string // a formula property's formula (lib/formula.ts)
}
export interface DBFilter {
  prop: string // a property id, or "title"
  op: string
  value?: string | number | boolean
}
export interface DBSort {
  prop: string
  desc?: boolean
}
export interface DBView {
  id: string
  name: string
  type: 'table' | 'board'
  group_by?: string
  filters?: DBFilter[]
  sorts?: DBSort[]
  hidden?: string[]
  totals?: Record<string, Agg> // a total under a column (by property id, or "title")
}
export type Agg = 'sum' | 'avg' | 'min' | 'max' | 'count' | 'empty' | 'checked'
export interface Database {
  props: DBProp[]
  views: DBView[]
}
export type PropValue = string | number | boolean | string[]
export type Props = Record<string, PropValue>

// A row as the views need it.
export interface DBRow {
  id: string
  title: string
  rank: number
  props: Props
}

// Option colours: Apple's system palette.
export const OPTION_COLORS = ['#8e8e93', '#007aff', '#34c759', '#ff9500', '#ff3b30', '#af52de', '#ff2d55', '#5ac8fa', '#ffcc00', '#a2845e']

export const newId = () => Math.random().toString(36).slice(2, 10)

export function newDatabase(names: { status: string; todo: string; doing: string; done: string; table: string }): Database {
  return {
    props: [
      {
        id: newId(), name: names.status, type: 'select',
        options: [
          { id: newId(), name: names.todo, color: OPTION_COLORS[0] },
          { id: newId(), name: names.doing, color: OPTION_COLORS[1] },
          { id: newId(), name: names.done, color: OPTION_COLORS[2] },
        ],
      },
    ],
    views: [{ id: newId(), name: names.table, type: 'table' }],
  }
}

// ---- Filters ----
// The comparisons each type offers ("set" / "empty" for every one).
export const OPS: Record<PropType | 'title', string[]> = {
  title: ['contains', 'not_contains', 'is', 'empty', 'set'],
  text: ['contains', 'not_contains', 'is', 'empty', 'set'],
  url: ['contains', 'empty', 'set'],
  number: ['eq', 'ne', 'gt', 'lt', 'gte', 'lte', 'empty', 'set'],
  select: ['is', 'is_not', 'empty', 'set'],
  multi_select: ['has', 'has_not', 'empty', 'set'],
  person: ['is', 'is_not', 'empty', 'set'], // value: a user id, or "me"
  date: ['on', 'before', 'after', 'past_week', 'next_week', 'empty', 'set'],
  checkbox: ['checked', 'unchecked'],
  formula: ['eq', 'ne', 'gt', 'lt', 'gte', 'lte', 'contains', 'empty', 'set'],
}
// Ops that need no value.
export const NO_VALUE = new Set(['empty', 'set', 'checked', 'unchecked', 'past_week', 'next_week'])

const fold = (s: string) => s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()
const isEmpty = (v: PropValue | undefined) => v === undefined || v === '' || (Array.isArray(v) && !v.length)

export interface MatchCtx {
  me: string
  today: string // YYYY-MM-DD
}

export function valueOf(row: DBRow, prop: string): PropValue | undefined {
  return prop === 'title' ? row.title : row.props[prop]
}

export function passes(row: DBRow, f: DBFilter, type: PropType | 'title', c: MatchCtx): boolean {
  const v = valueOf(row, f.prop)
  if (f.op === 'empty') return type === 'checkbox' ? !v : isEmpty(v)
  if (f.op === 'set') return type === 'checkbox' ? !!v : !isEmpty(v)
  const want = f.value === 'me' && type === 'person' ? c.me : f.value
  switch (type) {
    case 'title':
    case 'text':
    case 'url': {
      const s = fold(String(v ?? ''))
      const w = fold(String(want ?? ''))
      if (f.op === 'contains') return s.includes(w)
      if (f.op === 'not_contains') return !s.includes(w)
      return s === w
    }
    case 'number': {
      if (typeof v !== 'number') return false
      const w = Number(want)
      return { eq: v === w, ne: v !== w, gt: v > w, lt: v < w, gte: v >= w, lte: v <= w }[f.op as 'eq'] ?? true
    }
    case 'select':
    case 'person':
      return f.op === 'is_not' ? v !== want : v === want
    case 'multi_select': {
      const has = Array.isArray(v) && v.includes(String(want))
      return f.op === 'has_not' ? !has : has
    }
    case 'date': {
      if (typeof v !== 'string' || !v) return false
      if (f.op === 'past_week') return v <= c.today && v >= dateStr(addDays(parseDate(c.today), -7))
      if (f.op === 'next_week') return v >= c.today && v <= dateStr(addDays(parseDate(c.today), 7))
      const w = String(want ?? '')
      return f.op === 'before' ? v < w : f.op === 'after' ? v > w : v === w
    }
    case 'checkbox':
      return f.op === 'checked' ? !!v : !v
    case 'formula': {
      // A formula's result: compared as a number when both are numbers.
      if (f.op === 'contains') return fold(String(v ?? '')).includes(fold(String(want ?? '')))
      const n = Number(want)
      if (typeof v === 'number' && want !== '' && !Number.isNaN(n)) {
        return { eq: v === n, ne: v !== n, gt: v > n, lt: v < n, gte: v >= n, lte: v <= n }[f.op as 'eq'] ?? true
      }
      const a = fold(String(v ?? ''))
      const b = fold(String(want ?? ''))
      return { eq: a === b, ne: a !== b, gt: a > b, lt: a < b, gte: a >= b, lte: a <= b }[f.op as 'eq'] ?? true
    }
  }
  return true
}

export function typeOf(db: Database, prop: string): PropType | 'title' {
  return prop === 'title' ? 'title' : (db.props.find((p) => p.id === prop)?.type ?? 'text')
}

// rowsOf is a view's rows: filtered, then sorted (by rank when no sort).
export function rowsOf(db: Database, view: DBView, rows: DBRow[], c: MatchCtx): DBRow[] {
  const fs = (view.filters ?? []).filter((f) => f.prop === 'title' || db.props.some((p) => p.id === f.prop))
  const kept = rows.filter((r) => fs.every((f) => passes(r, f, typeOf(db, f.prop), c)))
  const sorts = (view.sorts ?? []).filter((s) => s.prop === 'title' || db.props.some((p) => p.id === s.prop))
  return kept.sort((a, b) => {
    for (const s of sorts) {
      const d = compare(db, s.prop, a, b)
      if (d) return s.desc ? -d : d
    }
    return a.rank - b.rank
  })
}

// compare orders two rows by a property; empty values go last.
function compare(db: Database, prop: string, a: DBRow, b: DBRow): number {
  const va = valueOf(a, prop)
  const vb = valueOf(b, prop)
  if (isEmpty(va) || isEmpty(vb)) return isEmpty(va) === isEmpty(vb) ? 0 : isEmpty(va) ? 1 : -1
  const p = db.props.find((x) => x.id === prop)
  if (p?.type === 'select') {
    const i = (id: PropValue) => p.options?.findIndex((o) => o.id === id) ?? 0
    return i(va!) - i(vb!)
  }
  if (typeof va === 'number' && typeof vb === 'number') return va - vb
  if (typeof va === 'boolean') return Number(va) - Number(vb)
  return String(Array.isArray(va) ? va.join(',') : va).localeCompare(String(Array.isArray(vb) ? vb.join(',') : vb), locale(), { numeric: true })
}

// A board's columns: one per option of its select property (and one for
// rows with none, first), each with its rows.
export interface BoardColumn {
  key: string // the option id, or "" for none
  name: string
  color?: string
  rows: DBRow[]
}
export function boardOf(db: Database, view: DBView, rows: DBRow[], noneName: string): BoardColumn[] {
  const p = db.props.find((x) => x.id === view.group_by && x.type === 'select')
  if (!p) return [{ key: '', name: noneName, rows }]
  const cols: BoardColumn[] = [{ key: '', name: noneName, rows: [] }, ...(p.options ?? []).map((o) => ({ key: o.id, name: o.name, color: o.color, rows: [] as DBRow[] }))]
  for (const r of rows) {
    const v = r.props[p.id]
    ;(cols.find((c) => c.key === v) ?? cols[0]).rows.push(r)
  }
  return cols.filter((c, i) => i > 0 || c.rows.length)
}

// ---- Reading a value ----
export function shownValue(p: DBProp, v: PropValue | undefined, names: Map<string, string>): string {
  if (isEmpty(v)) return ''
  const opt = (id: string) => p.options?.find((o) => o.id === id)?.name ?? ''
  switch (p.type) {
    case 'select':
      return opt(String(v))
    case 'multi_select':
      return (v as string[]).map(opt).filter(Boolean).join(', ')
    case 'person':
      return names.get(String(v)) ?? ''
    case 'checkbox':
      return v ? '✓' : ''
    case 'date':
      return new Intl.DateTimeFormat(locale(), { day: 'numeric', month: 'short', year: 'numeric' }).format(parseDate(String(v)))
    case 'number':
      return typeof v === 'number' ? new Intl.NumberFormat(locale()).format(v) : ''
    case 'formula':
      if (typeof v === 'number') return new Intl.NumberFormat(locale(), { maximumFractionDigits: 4 }).format(v)
      if (typeof v === 'boolean') return v ? '✓' : '✗'
      if (typeof v === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(v)) return new Intl.DateTimeFormat(locale(), { day: 'numeric', month: 'short', year: 'numeric' }).format(parseDate(v))
      return String(v)
    default:
      return String(v)
  }
}

// ---- Formulas ----
// withFormulas works out each row's formula properties (each formula once
// per row, a formula that comes back to itself is #REF!) and puts the
// results among its values, so filters, sorts and boards see them. names:
// people's names by id; today: YYYY-MM-DD.
export function withFormulas(db: Database, rows: DBRow[], names: Map<string, string>, today: string): DBRow[] {
  const formulas = db.props.filter((p) => p.type === 'formula')
  if (!formulas.length) return rows
  const byName = new Map<string, DBProp | 'title'>()
  for (const n of ['name', 'nome', 'title', 'titulo']) byName.set(n, 'title')
  for (const p of db.props) byName.set(foldName(p.name), p)
  const trees = new Map<string, ReturnType<typeof parse> | FormulaError>()
  for (const p of formulas) {
    try {
      trees.set(p.id, parse(p.formula ?? ''))
    } catch (e) {
      trees.set(p.id, e instanceof FormulaError ? e : new FormulaError(String(e)))
    }
  }
  const opt = (p: DBProp, id: string) => p.options?.find((o) => o.id === id)?.name ?? null
  const raw = (r: DBRow, p: DBProp): FValue => {
    const v = r.props[p.id]
    if (v === undefined || v === '') return p.type === 'checkbox' ? false : null
    switch (p.type) {
      case 'number':
        return typeof v === 'number' ? v : null
      case 'checkbox':
        return !!v
      case 'date':
        return typeof v === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(v) ? { date: dayOf(v) } : null
      case 'select':
        return opt(p, String(v))
      case 'multi_select':
        return (v as string[]).map((id) => opt(p, id)).filter(Boolean).join(', ')
      case 'person':
        return names.get(String(v)) ?? null
      default:
        return String(v)
    }
  }
  const cache = new Map<string, FValue>()
  const busy = new Set<string>()
  const value = (r: DBRow, p: DBProp): FValue => (p.type === 'formula' ? formula(r, p) : raw(r, p))
  const ctxFor = (r: DBRow): Ctx => ({
    today: dayOf(today),
    row: (name) => {
      const p = byName.get(foldName(name))
      return p === undefined ? undefined : p === 'title' ? r.title : value(r, p)
    },
    column: (name) => {
      const p = byName.get(foldName(name))
      return p === undefined ? undefined : rows.map((x) => (p === 'title' ? x.title : value(x, p)))
    },
  })
  function formula(r: DBRow, p: DBProp): FValue {
    const key = `${r.id}|${p.id}`
    if (cache.has(key)) return cache.get(key)!
    if (busy.has(key)) return { error: '#REF!' }
    busy.add(key)
    const tree = trees.get(p.id)!
    const v = tree instanceof FormulaError ? { error: '#ERROR!' } : evaluate(tree, ctxFor(r))
    busy.delete(key)
    cache.set(key, v)
    return v
  }
  return rows.map((r) => {
    const props = { ...r.props }
    for (const p of formulas) {
      const v = formula(r, p)
      if (v === null || v === '') delete props[p.id]
      else props[p.id] = isErr(v) ? v.error : isFDate(v) ? isoOf(v.date) : (v as PropValue)
    }
    return { ...r, props }
  })
}

// formulaProblem is what's wrong with a formula as written ('' when nothing).
export function formulaProblem(src: string): string {
  try {
    parse(src)
    return ''
  } catch (e) {
    return e instanceof FormulaError ? e.message : String(e)
  }
}

// ---- Totals ----
// The totals a column can have, by its type.
export function aggsFor(type: PropType | 'title'): Agg[] {
  if (type === 'title') return ['count']
  if (type === 'number' || type === 'formula') return ['sum', 'avg', 'min', 'max', 'count', 'empty']
  if (type === 'checkbox') return ['checked', 'count']
  return ['count', 'empty']
}
// total works out a column's total over the rows shown (null: none to show).
export function total(rows: DBRow[], prop: string, agg: Agg): number | null {
  const vals = rows.map((r) => valueOf(r, prop))
  const filled = vals.filter((v) => !isEmpty(v))
  const nums = filled.filter((v): v is number => typeof v === 'number')
  switch (agg) {
    case 'sum':
      return nums.reduce((a, b) => a + b, 0)
    case 'avg':
      return nums.length ? nums.reduce((a, b) => a + b, 0) / nums.length : null
    case 'min':
      return nums.length ? Math.min(...nums) : null
    case 'max':
      return nums.length ? Math.max(...nums) : null
    case 'count':
      return prop === 'title' ? rows.length : filled.length
    case 'empty':
      return vals.length - filled.length
    case 'checked':
      return vals.filter((v) => v === true).length
  }
  return null
}

// guessType reads a column of imported values: numbers, yes / no, dates,
// else text.
export function guessType(values: string[]): PropType {
  const vs = values.map((v) => v.trim()).filter(Boolean)
  if (!vs.length) return 'text'
  if (vs.every((v) => /^-?\d+(?:[.,]\d+)?$/.test(v))) return 'number'
  if (vs.every((v) => /^(yes|no|true|false|sim|não|nao)$/i.test(v))) return 'checkbox'
  if (vs.every((v) => /^\d{4}-\d{2}-\d{2}$/.test(v) || (!Number.isNaN(Date.parse(v)) && /\d{4}/.test(v) && /[a-z]/i.test(v)))) return 'date'
  const distinct = new Set(vs)
  if (distinct.size <= Math.max(6, vs.length / 3) && [...distinct].every((v) => v.length <= 40 && !v.includes(','))) return 'select'
  return 'text'
}

// importValue turns imported text into a value of a property (adding the
// option it names, for a select).
export function importValue(p: DBProp, text: string): PropValue | undefined {
  const v = text.trim()
  if (!v) return undefined
  switch (p.type) {
    case 'number':
      return Number(v.replace(',', '.'))
    case 'checkbox':
      return /^(yes|true|sim)$/i.test(v)
    case 'date': {
      if (/^\d{4}-\d{2}-\d{2}$/.test(v)) return v
      const t = Date.parse(v)
      return Number.isNaN(t) ? undefined : dateStr(new Date(t))
    }
    case 'select': {
      p.options ??= []
      let o = p.options.find((x) => x.name === v)
      if (!o) {
        o = { id: newId(), name: v, color: OPTION_COLORS[p.options.length % OPTION_COLORS.length] }
        p.options.push(o)
      }
      return o.id
    }
    default:
      return v
  }
}
