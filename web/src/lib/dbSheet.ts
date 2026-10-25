// Databases and spreadsheets: a view's rows as a sheet (to save as an Excel
// file or a CSV), and a spreadsheet's rows into a database — its columns
// matched to the properties by name, the others made new properties, a
// "Name" column (or else the first) the rows' names.
import { dayOf, foldName, isDate, isErr, isoOf } from './formula'
import { Book, addr, literal, newTab, parseAddr, type SheetData } from './sheet'
import { OPTION_COLORS, guessType, importValue, newId, shownValue, type Database, type DBProp, type DBRow, type PropValue, type Props } from './database'

// ---- Out ----

// As typed into a cell: text that would read as a number, a date… keeps
// its "'" (so it stays text).
const asText = (s: string) => (s.startsWith('=') || s.startsWith("'") || typeof literal(s) !== 'string' ? `'${s}` : s)

// typed is a value as it goes into a cell: numbers in figures, dates
// YYYY-MM-DD, yes / no as TRUE / FALSE, options and people by name.
export function typed(p: DBProp, v: PropValue | undefined, names: Map<string, string>): string {
  if (v === undefined || v === null || v === '' || (Array.isArray(v) && !v.length)) return p.type === 'checkbox' ? 'FALSE' : ''
  if (typeof v === 'number') return Number.isFinite(v) ? String(v) : ''
  if (typeof v === 'boolean') return v ? 'TRUE' : 'FALSE'
  if (p.type === 'date' || (p.type === 'formula' && typeof v === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(v))) return String(v)
  if (p.type === 'select' || p.type === 'multi_select' || p.type === 'person') return asText(shownValue(p, v, names))
  return asText(String(v))
}

// sheetOf makes a sheet of rows: a bold top row of names (kept in view),
// then a row each.
export function sheetOf(rows: DBRow[], columns: DBProp[], names: Map<string, string>, words: { name: string; tab: string }): SheetData {
  const tab = newTab(words.tab.slice(0, 31))
  const head = [words.name, ...columns.map((p) => p.name)]
  head.forEach((h, c) => (tab.cells[addr(0, c)] = { v: asText(h), b: true }))
  rows.forEach((r, i) => {
    const line = [asText(r.title), ...columns.map((p) => typed(p, r.props[p.id], names))]
    line.forEach((v, c) => {
      if (v !== '') tab.cells[addr(i + 1, c)] = p0(columns[c - 1], v)
    })
  })
  tab.rows = Math.max(tab.rows, rows.length + 21)
  tab.cols = Math.max(tab.cols, head.length + 2)
  tab.freeze = 1
  tab.widths = { A: 220 }
  return { tabs: [tab] }
}
// A date column shows as a date.
const p0 = (p: DBProp | undefined, v: string) => (p?.type === 'date' ? { v, nf: 'date' as const } : { v })

// ---- In ----

export interface Grid {
  headers: string[]
  rows: string[][]
}

// gridOf reads a spreadsheet's first tab as text, each value as Orbit
// reads it back: numbers in figures, dates YYYY-MM-DD, true / false. The
// first row with anything in it holds the names; empty rows are left out.
export function gridOf(data: SheetData, comma: boolean, today: string): Grid {
  const tab = data.tabs[0]
  if (!tab) return { headers: [], rows: [] }
  const book = new Book(data, dayOf(today), comma)
  let maxR = -1
  let maxC = -1
  for (const a of Object.keys(tab.cells)) {
    const p = parseAddr(a)
    if (p && tab.cells[a].v !== '') {
      maxR = Math.max(maxR, p.r)
      maxC = Math.max(maxC, p.c)
    }
  }
  const lines: string[][] = []
  for (let r = 0; r <= maxR; r++) {
    const line: string[] = []
    for (let c = 0; c <= maxC; c++) {
      const v = book.value(tab.id, r, c)
      line.push(v === null || isErr(v) ? '' : isDate(v) ? isoOf(v.date) : typeof v === 'boolean' ? String(v) : String(v).trim())
    }
    if (line.some((x) => x !== '')) lines.push(line)
  }
  return { headers: lines[0] ?? [], rows: lines.slice(1) }
}

export interface Person {
  id: string
  names: string[] // their name and user name
}
export interface ImportPlan {
  db: Database // with the properties (and options) added
  rows: { title: string; props: Props }[]
  added: string[] // the new properties' names
}
export const MAX_IMPORT_ROWS = 2000

// planImport works out what a grid adds to a database. titleNames: the
// names a "name" column goes by.
export function planImport(db: Database, grid: Grid, people: Person[], titleNames: string[]): ImportPlan {
  const next: Database = JSON.parse(JSON.stringify(db))
  const titles = new Set(titleNames.map(foldName))
  const ti = Math.max(0, grid.headers.findIndex((h) => titles.has(foldName(h.trim()))))
  const added: string[] = []
  const used = new Set<string>()
  // The property of each column (none: left out — the name's, a formula's, a repeat).
  const cols: (DBProp | null)[] = grid.headers.map((h, i) => {
    if (i === ti) return null
    const name = h.trim() || `#${i + 1}`
    let p = next.props.find((x) => foldName(x.name) === foldName(name))
    if (!p) {
      p = { id: newId(), name: name.slice(0, 100), type: guessType(grid.rows.map((r) => r[i] ?? '')) }
      next.props.push(p)
      added.push(p.name)
    }
    if (p.type === 'formula' || used.has(p.id)) return null
    used.add(p.id)
    return p
  })
  const who = new Map<string, string>()
  for (const m of people) for (const n of m.names) if (n) who.set(foldName(n), m.id)
  const rows = grid.rows.slice(0, MAX_IMPORT_ROWS).map((r) => {
    const props: Props = {}
    cols.forEach((p, i) => {
      if (!p) return
      const v = valueOf(p, r[i] ?? '', who)
      if (v !== undefined) props[p.id] = v
    })
    return { title: (r[ti] ?? '').slice(0, 500), props }
  })
  return { db: next, rows, added }
}

function valueOf(p: DBProp, text: string, who: Map<string, string>): PropValue | undefined {
  const v = text.trim()
  if (!v) return undefined
  switch (p.type) {
    case 'person':
      return who.get(foldName(v))
    case 'multi_select': {
      p.options ??= []
      const ids = v
        .split(/[,;]/)
        .map((x) => x.trim())
        .filter(Boolean)
        .map((name) => {
          let o = p.options!.find((x) => x.name === name)
          if (!o) {
            o = { id: newId(), name, color: OPTION_COLORS[p.options!.length % OPTION_COLORS.length] }
            p.options!.push(o)
          }
          return o.id
        })
      return ids.length ? [...new Set(ids)] : undefined
    }
    default:
      return importValue(p, v)
  }
}
