// Sheets: a spreadsheet's data and its arithmetic. A sheet is tabs of
// cells; a cell keeps what was typed (a value, or an "=" formula — Excel's,
// lib/formula.ts) and its look. Values are worked out here, on demand
// (Book), never stored. Inserting or deleting rows and columns moves the
// cells and every reference to them, the way Excel does.
import { colIndex, colName, dayOf, evaluate, isDate, isErr, isoOf, parse, rewriteRefs, FormulaError, type CellPos, type CellRef, type FValue } from './formula'

export type NumFmt = 'num' | 'int' | 'cur' | 'pct' | 'date'
export interface Cell {
  v: string // what was typed
  b?: boolean // bold
  i?: boolean // italic
  al?: 'l' | 'c' | 'r'
  nf?: NumFmt
  fg?: string // text colour
  bg?: string // fill
}
export interface Tab {
  id: string
  name: string
  cells: Record<string, Cell> // by address, "B3"
  rows: number
  cols: number
  widths?: Record<string, number> // by column letter
  freeze?: number // rows kept at the top
}
export interface SheetData {
  tabs: Tab[]
}

export const ROWS = 100
export const COLS = 26
export const MAX_ROWS = 20000
export const MAX_COLS = 702 // A … ZZ

export const newId = () => Math.random().toString(36).slice(2, 10)
export const addr = (r: number, c: number) => `${colName(c)}${r + 1}`
export function parseAddr(a: string): { r: number; c: number } | null {
  const m = /^([A-Z]{1,3})(\d+)$/.exec(a)
  return m ? { r: Number(m[2]) - 1, c: colIndex(m[1]) } : null
}
export function newTab(name: string): Tab {
  return { id: newId(), name, cells: {}, rows: ROWS, cols: COLS }
}
export function emptySheet(name: string): SheetData {
  return { tabs: [newTab(name)] }
}
// readData takes a sheet's stored data, whatever shape, as a SheetData.
export function readData(raw: unknown, name: string): SheetData {
  const d = raw as Partial<SheetData> | null
  if (!d || !Array.isArray(d.tabs) || !d.tabs.length) return emptySheet(name)
  return {
    tabs: d.tabs.map((t) => ({
      id: t.id || newId(), name: t.name || name, cells: t.cells ?? {}, rows: Math.max(t.rows || ROWS, 1), cols: Math.max(t.cols || COLS, 1),
      ...(t.widths ? { widths: t.widths } : {}), ...(t.freeze ? { freeze: t.freeze } : {}),
    })),
  }
}

// ---- What a typed value is ----
// literal reads what was typed (not a formula): a number as written in
// English or Portuguese (1,234.5 · 1234,5 · 12% · €12 · 12 €), TRUE / FALSE,
// a date (2026-10-23, 23/10/2026), else text. A leading ' keeps it text.
// comma: the comma is the decimal mark (Portuguese), so 1,234 is 1.234.
export function literal(input: string, comma = false): FValue {
  const s = input.trim()
  if (s === '') return null
  if (input.startsWith("'")) return input.slice(1)
  const up = s.toUpperCase()
  if (up === 'TRUE' || up === 'VERDADEIRO') return true
  if (up === 'FALSE' || up === 'FALSO') return false
  if (/^\d{4}-\d{2}-\d{2}$/.test(s)) return { date: dayOf(s) }
  const dm = /^(\d{1,2})\/(\d{1,2})\/(\d{4})$/.exec(s)
  if (dm) return { date: dayOf(`${dm[3]}-${dm[2].padStart(2, '0')}-${dm[1].padStart(2, '0')}`) }
  const n = number(s, comma)
  return n ?? input
}
function number(s: string, comma: boolean): number | null {
  let t = s.replace(/^[€$£]\s*|\s*[€$£]$/g, '')
  const pct = t.endsWith('%')
  if (pct) t = t.slice(0, -1).trim()
  if (!/^[-+]?[\d.,\s]+$/.test(t) || !/\d/.test(t)) return null
  t = t.replace(/\s/g, '')
  const lastDot = t.lastIndexOf('.')
  const lastComma = t.lastIndexOf(',')
  if (lastDot >= 0 && lastComma >= 0) {
    // Both: the last one is the decimal mark.
    t = lastComma > lastDot ? t.replace(/\./g, '').replace(',', '.') : t.replace(/,/g, '')
  } else if (lastComma >= 0) {
    // 12,5 is a decimal; 1,234 too in Portuguese, a thousand and more in English.
    t = !comma && /^\d{1,3}(,\d{3})+$/.test(t.replace(/^[-+]/, '')) ? t.replace(/,/g, '') : t.replace(',', '.')
  } else if (comma && /^[-+]?\d{1,3}(\.\d{3})+$/.test(t)) {
    t = t.replace(/\./g, '') // 1.234 in Portuguese: a thousand and more
  }
  const n = Number(t)
  if (!Number.isFinite(n)) return null
  return pct ? n / 100 : n
}

// ---- Working values out ----
export class Book {
  private memo = new Map<string, FValue>()
  private busy = new Set<string>()
  private trees = new Map<string, ReturnType<typeof parse> | FormulaError>()
  constructor(
    readonly data: SheetData,
    readonly today: number = dayOf(new Date().toISOString().slice(0, 10)),
    readonly comma = false, // the comma is the decimal mark (Portuguese)
  ) {}

  tab(id: string) {
    return this.data.tabs.find((t) => t.id === id)
  }
  private tabNamed(name: string | undefined, from: Tab): Tab | undefined {
    if (name === undefined) return from
    const n = name.toLowerCase()
    return this.data.tabs.find((t) => t.name.toLowerCase() === n)
  }
  // used is how far a tab's cells go (for A:A, a whole column).
  private used(t: Tab): number {
    let max = -1
    for (const a of Object.keys(t.cells)) {
      const p = parseAddr(a)
      if (p && p.r > max && t.cells[a].v !== '') max = p.r
    }
    return max
  }

  value(tabId: string, r: number, c: number): FValue {
    const tab = this.tab(tabId)
    if (!tab) return { error: '#REF!' }
    const key = `${tabId}!${r}:${c}`
    if (this.memo.has(key)) return this.memo.get(key)!
    const cell = tab.cells[addr(r, c)]
    if (!cell || cell.v === '') return null
    if (!cell.v.startsWith('=') || cell.v.length < 2) {
      const v = literal(cell.v, this.comma)
      this.memo.set(key, v)
      return v
    }
    if (this.busy.has(key)) return { error: '#REF!' } // a formula that needs itself
    this.busy.add(key)
    let tree = this.trees.get(cell.v)
    if (!tree) {
      try {
        tree = parse(cell.v, { cells: true })
      } catch (e) {
        tree = e instanceof FormulaError ? e : new FormulaError(String(e))
      }
      this.trees.set(cell.v, tree)
    }
    const v =
      tree instanceof FormulaError
        ? { error: '#ERROR!' }
        : evaluate(tree, {
            today: this.today,
            row: () => undefined,
            column: () => undefined,
            cells: (ref) => this.refValue(tab, ref),
          })
    this.busy.delete(key)
    this.memo.set(key, v)
    return v
  }

  private refValue(from: Tab, ref: CellRef): FValue | FValue[][] {
    const tab = this.tabNamed(ref.sheet, from)
    if (!tab) return { error: '#REF!' }
    if (!ref.b) return ref.a.row < 0 ? { error: '#REF!' } : this.value(tab.id, ref.a.row, ref.a.col)
    const whole = ref.a.row < 0
    const r1 = whole ? 0 : Math.min(ref.a.row, ref.b.row)
    const r2 = whole ? this.used(tab) : Math.max(ref.a.row, ref.b.row)
    const c1 = Math.min(ref.a.col, ref.b.col)
    const c2 = Math.max(ref.a.col, ref.b.col)
    if ((r2 - r1 + 1) * (c2 - c1 + 1) > 1_000_000) return { error: '#NUM!' }
    const out: FValue[][] = []
    for (let r = r1; r <= r2; r++) {
      const row: FValue[] = []
      for (let c = c1; c <= c2; c++) row.push(this.value(tab.id, r, c))
      out.push(row)
    }
    return out
  }
}

// ---- Showing a value ----
export interface Shown {
  text: string
  align: 'l' | 'c' | 'r'
  error?: boolean
}
const trimNum = (n: number) => String(Math.round(n * 1e10) / 1e10)
export function show(v: FValue, cell: Cell | undefined, locale: string, words: { yes: string; no: string }): Shown {
  const al = cell?.al
  if (v === null || v === '') return { text: '', align: al ?? 'l' }
  if (isErr(v)) return { text: v.error, align: al ?? 'c', error: true }
  if (typeof v === 'boolean') return { text: v ? words.yes : words.no, align: al ?? 'c' }
  if (Array.isArray(v)) return { text: '#VALUE!', align: 'c', error: true }
  const nf = cell?.nf
  if (isDate(v) || (typeof v === 'number' && nf === 'date')) {
    const n = isDate(v) ? v.date : v
    const [y, m, d] = isoOf(n).split('-').map(Number)
    return { text: new Intl.DateTimeFormat(locale, { day: '2-digit', month: '2-digit', year: 'numeric' }).format(new Date(y, m - 1, d)), align: al ?? 'r' }
  }
  if (typeof v === 'number') {
    let text: string
    switch (nf) {
      case 'num':
        text = new Intl.NumberFormat(locale, { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(v)
        break
      case 'int':
        text = new Intl.NumberFormat(locale, { maximumFractionDigits: 0 }).format(v)
        break
      case 'cur':
        text = new Intl.NumberFormat(locale, { style: 'currency', currency: 'EUR' }).format(v)
        break
      case 'pct':
        text = new Intl.NumberFormat(locale, { style: 'percent', maximumFractionDigits: 2 }).format(v)
        break
      default:
        text = trimNum(v)
        if (locale.startsWith('pt')) text = text.replace('.', ',')
    }
    return { text, align: al ?? 'r' }
  }
  return { text: String(v), align: al ?? 'l' }
}

// ---- Inserting and deleting rows and columns ----
type Axis = 'row' | 'col'

// moveRef is a reference after rows / columns [at, at+n) were inserted (n
// > 0) or deleted (n < 0) on its tab: positions after them move; one in
// what was deleted is gone (null); a range keeps what's left of it.
function moveRef(ref: CellRef, axis: Axis, at: number, n: number): CellRef | null {
  const k = axis === 'row' ? 'row' : 'col'
  const whole = axis === 'row' && ref.a.row < 0 // A:A: rows don't touch it
  if (whole) return ref
  const pos = (p: CellPos, end: boolean): CellPos | null => {
    const v = p[k]
    if (n > 0) return v >= at ? { ...p, [k]: v + n } : p
    const del = -n
    if (v < at) return p
    if (v >= at + del) return { ...p, [k]: v - del }
    // In the deleted ones: a range's end comes back to the edge, a single cell is gone.
    return end === undefined ? null : end ? { ...p, [k]: at - 1 } : { ...p, [k]: at }
  }
  if (!ref.b) {
    const a = n < 0 && ref.a[k] >= at && ref.a[k] < at - n ? null : pos(ref.a, false)
    return a ? { ...ref, a } : null
  }
  const lo = ref.a[k] <= ref.b[k] ? 'a' : 'b'
  const hi = lo === 'a' ? 'b' : 'a'
  const start = ref[lo]!
  const end = ref[hi]!
  if (n < 0 && start[k] >= at && end[k] < at - n) return null // all of it deleted
  const s = pos(start, false)!
  const e = pos(end, true)!
  if (e[k] < s[k]) return null
  return { ...ref, [lo]: s, [hi]: e }
}

// reshape inserts (n > 0) or deletes (n < 0) rows or columns at `at` of a
// tab: its cells move, and every formula of the sheet that points at that
// tab follows. Returns the new data.
export function reshape(data: SheetData, tabId: string, axis: Axis, at: number, n: number): SheetData {
  const target = data.tabs.find((t) => t.id === tabId)
  if (!target || !n) return data
  const tabs = data.tabs.map((t) => {
    const cells: Record<string, Cell> = {}
    for (const [a, cell] of Object.entries(t.cells)) {
      let c = cell
      if (cell.v.startsWith('=')) {
        const v = rewriteRefs(cell.v, (ref) => {
          const on = ref.sheet === undefined ? t.id === tabId : ref.sheet.toLowerCase() === target.name.toLowerCase()
          return on ? moveRef(ref, axis, at, n) : ref
        })
        if (v !== cell.v) c = { ...cell, v }
      }
      if (t.id !== tabId) {
        cells[a] = c
        continue
      }
      const p = parseAddr(a)!
      const v = axis === 'row' ? p.r : p.c
      if (n < 0 && v >= at && v < at - n) continue // deleted
      const moved = v >= at ? v + n : v
      cells[axis === 'row' ? addr(moved, p.c) : addr(p.r, moved)] = c
    }
    if (t.id !== tabId) return { ...t, cells }
    const out: Tab = { ...t, cells }
    if (axis === 'row') out.rows = Math.min(MAX_ROWS, Math.max(1, t.rows + n))
    else {
      out.cols = Math.min(MAX_COLS, Math.max(1, t.cols + n))
      if (t.widths) {
        const w: Record<string, number> = {}
        for (const [l, px] of Object.entries(t.widths)) {
          const i = colIndex(l)
          if (n < 0 && i >= at && i < at - n) continue
          w[colName(i >= at ? i + n : i)] = px
        }
        out.widths = w
      }
    }
    return out
  })
  return { tabs }
}

// ---- Text in and out ----
// tsv: rows of cells as text separated by tabs (Excel's clipboard).
export function toTSV(rows: string[][]): string {
  return rows.map((r) => r.map((s) => (/[\t\n"]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s)).join('\t')).join('\n')
}
export function fromTSV(text: string): string[][] {
  const rows: string[][] = []
  let row: string[] = []
  let cell = ''
  let quoted = false
  const s = text.replace(/\r\n?/g, '\n').replace(/\n$/, '')
  for (let i = 0; i < s.length; i++) {
    const c = s[i]
    if (quoted) {
      if (c === '"' && s[i + 1] === '"') {
        cell += '"'
        i++
      } else if (c === '"') quoted = false
      else cell += c
    } else if (c === '"' && cell === '') quoted = true
    else if (c === '\t') {
      row.push(cell)
      cell = ''
    } else if (c === '\n') {
      row.push(cell)
      rows.push(row)
      row = []
      cell = ''
    } else cell += c
  }
  row.push(cell)
  rows.push(row)
  return rows
}

// textOf is a sheet's words and numbers (for search).
export function textOf(data: SheetData): string {
  const out: string[] = []
  for (const t of data.tabs) {
    out.push(t.name)
    for (const c of Object.values(t.cells)) if (c.v && !c.v.startsWith('=')) out.push(c.v)
  }
  return out.join(' ').slice(0, 250_000)
}

// series continues what a fill handle drags from: numbers going up by the
// same step go on; anything else repeats (formulas move their references —
// the caller does that).
export function series(inputs: string[], count: number): string[] {
  const nums = inputs.map((s) => (s.startsWith('=') ? null : literal(s)))
  if (inputs.length >= 2 && nums.every((n) => typeof n === 'number')) {
    const ns = nums as number[]
    const step = ns[1] - ns[0]
    if (ns.every((n, i) => i === 0 || Math.abs(n - ns[i - 1] - step) < 1e-9)) {
      return Array.from({ length: count }, (_, i) => trimNum(ns[ns.length - 1] + step * (i + 1)))
    }
  }
  return Array.from({ length: count }, (_, i) => inputs[i % inputs.length])
}

export { colName, colIndex }
