// Timeline (Gantt) math. Dates are day numbers — days since 1970-01-01,
// counted in UTC so a daylight-saving change never skews a day — which makes
// a bar's position a multiplication. A task's bar runs from its start (else
// its due date) to its due date (else its start), both days included.
import type { Member, Task, TaskList } from '@/api/planner'

export type Zoom = 'weeks' | 'months' | 'quarters'
export const ZOOMS: Zoom[] = ['weeks', 'months', 'quarters']
// Pixels per day at each zoom.
export const ZOOM_PX: Record<Zoom, number> = { weeks: 44, months: 16, quarters: 6 }

const DAY = 86400000
const pad = (n: number) => String(n).padStart(2, '0')

// dayNum reads YYYY-MM-DD as a day number.
export function dayNum(s: string): number {
  const [y, m, d] = s.split('-').map(Number)
  return Math.round(Date.UTC(y, m - 1, d) / DAY)
}
// numDate is the YYYY-MM-DD of a day number.
export function numDate(n: number): string {
  const d = new Date(n * DAY)
  return `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())}`
}
// numLocal is a day number as local midnight, for Intl formatting.
export function numLocal(n: number): Date {
  const d = new Date(n * DAY)
  return new Date(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate())
}
// weekday of a day number: 0 Sunday … 6 Saturday (1970-01-01 was a Thursday).
export function weekday(n: number): number {
  return (((n + 4) % 7) + 7) % 7
}
// weekStartOf is the first day of n's week (ws: 0 Sunday, 1 Monday).
export function weekStartOf(n: number, ws: number): number {
  return n - ((weekday(n) - ws + 7) % 7)
}

export interface Span {
  from: number
  to: number
}
// spanOf is a task's bar, or null when it has no dates at all.
export function spanOf(t: { start: string | null; due: string | null }): Span | null {
  const a = t.start ?? t.due
  const b = t.due ?? t.start
  if (!a || !b) return null
  const from = dayNum(a)
  const to = dayNum(b)
  return from <= to ? { from, to } : { from: to, to: from }
}

// rangeFor is the stretch of days shown: at least `around` days either side
// of today, and a margin before the earliest bar and after the latest, from
// the start of a week to the end of one.
export function rangeFor(today: number, spans: Span[], ws: number, opts = { before: 14, after: 42, around: 92 }): Span {
  let from = today - opts.around
  let to = today + opts.around
  for (const s of spans) {
    from = Math.min(from, s.from - opts.before)
    to = Math.max(to, s.to + opts.after)
  }
  from = weekStartOf(from, ws)
  to = weekStartOf(to, ws) + 6
  return { from, to }
}

export type DragMode = 'move' | 'start' | 'end'
// dragSpan is a bar after dragging it (or one of its edges) by delta days;
// an edge never passes the other one.
export function dragSpan(s: Span, mode: DragMode, delta: number): Span {
  if (mode === 'move') return { from: s.from + delta, to: s.to + delta }
  if (mode === 'start') return { from: Math.min(s.from + delta, s.to), to: s.to }
  return { from: s.from, to: Math.max(s.to + delta, s.from) }
}

// patchFor is the update that gives a task the bar `next`. A one-day bar of a
// task that had only a due date (or only a start) keeps it that way.
export function patchFor(t: { start: string | null; due: string | null }, next: Span): { start?: string; due?: string } {
  const start = numDate(next.from)
  const due = numDate(next.to)
  if (next.from === next.to) {
    if (t.due && !t.start) return { due }
    if (t.start && !t.due) return { start }
  }
  return { start, due }
}

// ---- The date header ----
// A cell of the header: from day `at`, `days` long.
export interface Cell {
  at: number
  days: number
}
// cellsBy splits [from, to] at every day for which `isStart` holds (and at from).
function cellsBy(r: Span, isStart: (n: number) => boolean): Cell[] {
  const out: Cell[] = []
  for (let n = r.from; n <= r.to; n++) {
    if (n === r.from || isStart(n)) out.push({ at: n, days: 0 })
    out[out.length - 1].days++
  }
  return out
}
const dateOf = (n: number) => new Date(n * DAY)
export const isMonthStart = (n: number) => dateOf(n).getUTCDate() === 1
export const isQuarterStart = (n: number) => isMonthStart(n) && dateOf(n).getUTCMonth() % 3 === 0
export const monthCells = (r: Span) => cellsBy(r, isMonthStart)
export const quarterCells = (r: Span) => cellsBy(r, isQuarterStart)
export const weekCells = (r: Span, ws: number) => cellsBy(r, (n) => weekday(n) === ws)
// quarterOf is 1–4.
export const quarterOf = (n: number) => Math.floor(dateOf(n).getUTCMonth() / 3) + 1
export const yearOf = (n: number) => dateOf(n).getUTCFullYear()

// weekendStops is a CSS gradient that shades the weekends of a 7-day period
// starting at `from` (repeat it every 7 days), or '' when none fall in it.
export function weekendStops(from: number, px: number, shade: string): string {
  const stops: string[] = []
  for (let i = 0; i < 7; i++) {
    const wd = weekday(from + i)
    if (wd !== 0 && wd !== 6) continue
    stops.push(`transparent ${i * px}px`, `${shade} ${i * px}px`, `${shade} ${(i + 1) * px}px`, `transparent ${(i + 1) * px}px`)
  }
  return stops.length ? `linear-gradient(to right, ${stops.join(', ')})` : ''
}

// ---- What the chart shows ----

export interface TlRow {
  task: Task
  span: Span | null
  subs: { done: number; total: number }
  overdue: boolean
  assignee?: Member
}
export interface TlGroup {
  list: TaskList
  rows: TlRow[]
  span: Span | null // from its first bar to its last
}
