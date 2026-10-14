// The Charts module's numbers: tasks counted by status (Planner's four:
// not started, in progress, late, completed) per list, priority, person and
// tag, the summary tiles, and created vs completed per week. Pure functions
// of the tasks, so they are easy to test; the view feeds them filtered,
// top-level tasks.
import type { Task, TaskList } from '@/api/planner'
import { progressOf } from '@/lib/taskFilter'
import { addDays, dateStr, parseDate, startOfWeek } from '@/lib/dates'

export type Status = 'done' | 'inProgress' | 'notStarted' | 'late'
// Stack and legend order: work done, work moving, the backlog, then trouble.
// It also keeps red and green apart (they're hard to tell apart for many).
export const STATUSES: Status[] = ['done', 'inProgress', 'notStarted', 'late']

export type Counts = Record<Status, number>
export const zeroCounts = (): Counts => ({ done: 0, inProgress: 0, notStarted: 0, late: 0 })
export const totalOf = (c: Counts) => c.done + c.inProgress + c.notStarted + c.late

// isLate: open and past its due date.
export const isLate = (t: Task, today: string) => !t.done && !!t.due && t.due < today

// statusOf counts a late task as late, whatever its progress.
export function statusOf(t: Task, list: TaskList | undefined, today: string): Status {
  return isLate(t, today) ? 'late' : progressOf(t, list)
}

export interface Row {
  key: string
  counts: Counts
}

// countBy groups tasks by key(s) and counts their statuses. A task with
// several keys (tags) counts in each.
export function countBy(tasks: Task[], lists: Map<string, TaskList>, today: string, key: (t: Task) => string | string[]): Map<string, Counts> {
  const out = new Map<string, Counts>()
  for (const t of tasks) {
    const s = statusOf(t, lists.get(t.list_id), today)
    const keys = key(t)
    for (const k of Array.isArray(keys) ? keys : [keys]) {
      let c = out.get(k)
      if (!c) out.set(k, (c = zeroCounts()))
      c[s]++
    }
  }
  return out
}

export function countStatus(tasks: Task[], lists: Map<string, TaskList>, today: string): Counts {
  return countBy(tasks, lists, today, () => '').get('') ?? zeroCounts()
}

export interface Summary {
  open: number
  late: number
  dueWeek: number // open, due from today to the end of the week
  done7: number // completed in the last 7 days
}

export function summary(tasks: Task[], today: string, weekStart: 0 | 1, now = new Date()): Summary {
  const weekEnd = dateStr(addDays(startOfWeek(parseDate(today), weekStart), 6))
  const since = now.getTime() - 7 * 86_400_000
  const s: Summary = { open: 0, late: 0, dueWeek: 0, done7: 0 }
  for (const t of tasks) {
    if (t.done) {
      if (t.completed_at && Date.parse(t.completed_at) >= since) s.done7++
      continue
    }
    s.open++
    if (isLate(t, today)) s.late++
    else if (t.due && t.due <= weekEnd) s.dueWeek++
  }
  return s
}

export interface Week {
  start: string // YYYY-MM-DD, the week's first day
  created: number
  completed: number
}

// weekly counts the tasks created and completed in each of the last n weeks
// (the current one last). Instants are read as local dates.
export function weekly(tasks: Task[], n: number, today: string, weekStart: 0 | 1): Week[] {
  const first = addDays(startOfWeek(parseDate(today), weekStart), -7 * (n - 1))
  const weeks: Week[] = Array.from({ length: n }, (_, i) => ({ start: dateStr(addDays(first, 7 * i)), created: 0, completed: 0 }))
  const idx = (iso: string | null) => {
    if (!iso) return -1
    const d = new Date(iso)
    if (Number.isNaN(d.getTime())) return -1
    const days = Math.round((parseDate(dateStr(d)).getTime() - first.getTime()) / 86_400_000)
    const i = Math.floor(days / 7)
    return i >= 0 && i < n ? i : -1
  }
  for (const t of tasks) {
    const c = idx(t.created_at)
    if (c >= 0) weeks[c].created++
    if (t.done) {
      const d = idx(t.completed_at)
      if (d >= 0) weeks[d].completed++
    }
  }
  return weeks
}

// niceScale picks a round axis maximum and step (1, 2 or 5 × 10^k) with at
// most `ticks` intervals.
export function niceScale(max: number, ticks = 5): { max: number; step: number } {
  if (max <= 0) return { max: ticks, step: 1 }
  const raw = max / ticks
  const pow = 10 ** Math.floor(Math.log10(raw))
  const step = [1, 2, 5, 10].map((m) => m * pow).find((s) => s >= raw && Number.isInteger(s)) ?? Math.ceil(raw)
  return { max: Math.ceil(max / step) * step, step }
}
