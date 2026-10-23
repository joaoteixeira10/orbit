// Habits: the API, and what the page shows of one — whether a day counts,
// the current and best streak, and how often it was done lately. A daily
// habit's streak is in days (the days it counts, in a row, done); a weekly
// one's in weeks (weeks, Monday first, that reached its target). Today not
// done yet doesn't break a streak: the day isn't over.
import { api } from '@/lib/api'
import { addDays, dateStr, parseDate } from '@/lib/dates'

export interface Habit {
  id: string
  workspace_id: string
  title: string
  icon: string
  color: string
  schedule: 'daily' | 'weekly'
  days: string // Monday…Sunday as 0/1
  target: number // weekly: times a week
  remind_at: string // HH:MM or ""
  archived_at?: string | null
  checks: string[] // YYYY-MM-DD, oldest first
}
export type HabitInput = Partial<Pick<Habit, 'title' | 'icon' | 'color' | 'schedule' | 'days' | 'target' | 'remind_at'>> & { archived?: boolean }

const j = (b: unknown) => JSON.stringify(b)
export const habitsApi = {
  list: (ws: string) => api<Habit[]>(`/workspaces/${ws}/habits`),
  create: (ws: string, b: HabitInput) => api<Habit>(`/workspaces/${ws}/habits`, { method: 'POST', body: j(b) }),
  update: (id: string, b: HabitInput) => api<Habit>(`/habits/${id}`, { method: 'PUT', body: j(b) }),
  remove: (id: string) => api<void>(`/habits/${id}`, { method: 'DELETE' }),
  check: (id: string, date: string, done: boolean) => api<Habit>(`/habits/${id}/checks/${date}`, { method: 'PUT', body: j({ done }) }),
}

// mondayIndex: Monday 0 … Sunday 6.
export const mondayIndex = (d: Date) => (d.getDay() + 6) % 7
export const mondayOf = (d: Date) => addDays(new Date(d.getFullYear(), d.getMonth(), d.getDate()), -mondayIndex(d))

// counts reports whether a habit asks for anything on a day (a weekly one:
// any day).
export function counts(h: Pick<Habit, 'schedule' | 'days'>, d: Date): boolean {
  return h.schedule === 'weekly' || h.days[mondayIndex(d)] === '1'
}

function doneInWeek(done: Set<string>, monday: Date): number {
  let n = 0
  for (let i = 0; i < 7; i++) if (done.has(dateStr(addDays(monday, i)))) n++
  return n
}

// streak is the current run (days, or weeks for a weekly habit).
export function streak(h: Habit, today = new Date()): number {
  const done = new Set(h.checks)
  if (h.schedule === 'weekly') {
    let m = mondayOf(today)
    let n = doneInWeek(done, m) >= h.target ? 1 : 0
    for (let i = 0; i < 60; i++) {
      m = addDays(m, -7)
      if (doneInWeek(done, m) < h.target) break
      n++
    }
    return n
  }
  let n = done.has(dateStr(today)) && counts(h, today) ? 1 : 0
  let d = addDays(today, -1)
  for (let i = 0; i < 400; i++, d = addDays(d, -1)) {
    if (!counts(h, d)) continue
    if (!done.has(dateStr(d))) break
    n++
  }
  return n
}

// best is the longest run in the history kept.
export function best(h: Habit, today = new Date()): number {
  if (!h.checks.length) return 0
  const done = new Set(h.checks)
  const first = parseDate(h.checks[0])
  let run = 0
  let top = 0
  if (h.schedule === 'weekly') {
    for (let m = mondayOf(first); m <= today; m = addDays(m, 7)) {
      run = doneInWeek(done, m) >= h.target ? run + 1 : 0
      top = Math.max(top, run)
    }
    return top
  }
  for (let d = first; d <= today; d = addDays(d, 1)) {
    if (!counts(h, d)) continue
    if (done.has(dateStr(d))) top = Math.max(top, ++run)
    else if (dateStr(d) !== dateStr(today)) run = 0
  }
  return top
}

// rate is how often it was done in the last 30 days (0–1): of the days it
// counted, or for a weekly habit of its target over the last four weeks.
export function rate(h: Habit, today = new Date()): number {
  const done = new Set(h.checks)
  if (h.schedule === 'weekly') {
    let got = 0
    let m = mondayOf(today)
    for (let i = 0; i < 4; i++, m = addDays(m, -7)) got += Math.min(h.target, doneInWeek(done, m))
    return got / (4 * h.target)
  }
  let due = 0
  let got = 0
  for (let i = 0; i < 30; i++) {
    const d = addDays(today, -i)
    if (!counts(h, d)) continue
    due++
    if (done.has(dateStr(d))) got++
  }
  return due ? got / due : 0
}

// dueToday: it counts today and isn't done (a weekly one: its week isn't met).
export function dueToday(h: Habit, today = new Date()): boolean {
  const done = new Set(h.checks)
  if (done.has(dateStr(today))) return false
  if (h.schedule === 'weekly') return doneInWeek(done, mondayOf(today)) < h.target
  return counts(h, today)
}
