// The calendar's view model: occurrences read into local days and instants,
// and the two layouts the views share — lanes for bars that span days (month
// rows, the week's all-day strip) and side-by-side columns for overlapping
// timed events (the time grid).
import type { Occurrence, Task } from '@/api/planner'
import { addDays, allDayDate, dateStr, fmtDate, fmtTime, parseDate } from '@/lib/dates'

export type CalView = 'day' | 'week' | 'month'

export interface Entry {
  key: string
  occ: Occurrence
  start: Date
  end: Date // exclusive
  allDay: boolean
  first: string // first local date it shows on (YYYY-MM-DD)
  last: string // last local date, inclusive
  editable: boolean // opens in the event sheet
  movable: boolean // can be dragged to another day (single Orbit events)
}

export interface TaskItem {
  task: Task
  color: string
  editable: boolean
}

export function toEntry(o: Occurrence, editable: boolean): Entry {
  let start: Date, end: Date, first: string, last: string
  if (o.all_day) {
    // All-day instants are UTC midnights: read them as dates, end exclusive.
    first = allDayDate(o.start)
    const endEx = allDayDate(o.end)
    last = endEx > first ? dateStr(addDays(parseDate(endEx), -1)) : first
    start = parseDate(first)
    end = addDays(parseDate(last), 1)
  } else {
    start = new Date(o.start)
    end = new Date(o.end)
    first = dateStr(start)
    // An event ending at midnight doesn't show on the next day.
    last = end > start ? dateStr(new Date(end.getTime() - 1)) : first
  }
  const isOrbit = o.kind === 'event'
  return {
    key: `${o.kind}:${o.id}:${o.start}`,
    occ: o,
    start,
    end,
    allDay: o.all_day,
    first,
    last,
    editable: isOrbit && editable,
    movable: isOrbit && editable && !o.recurring,
  }
}

// Bars: all-day events and timed events that cross midnight.
export const isBar = (e: Entry) => e.allDay || e.first !== e.last

export const onDay = (e: Entry, day: string) => e.first <= day && e.last >= day

// sortEntries: bars first (longer first), then by start.
export function sortEntries(list: Entry[]): Entry[] {
  return [...list].sort((a, b) => {
    const ba = isBar(a) ? 0 : 1
    const bb = isBar(b) ? 0 : 1
    if (ba !== bb) return ba - bb
    if (a.first !== b.first) return a.first < b.first ? -1 : 1
    if (ba === 0) {
      const la = a.end.getTime() - a.start.getTime()
      const lb = b.end.getTime() - b.start.getTime()
      if (la !== lb) return lb - la
      if (a.allDay !== b.allDay) return a.allDay ? -1 : 1
    }
    return a.start.getTime() - b.start.getTime() || a.occ.title.localeCompare(b.occ.title)
  })
}

export interface Bar {
  entry: Entry
  col: number // first column in the row
  span: number
  lane: number
  clipStart: boolean // continues from before the row
  clipEnd: boolean // continues after it
}

// layoutBars places bars on lanes over consecutive days (a week): each bar
// takes the first lane free on all its days.
export function layoutBars(entries: Entry[], days: string[]): { bars: Bar[]; lanes: number[] } {
  const n = days.length
  const lastDay = days[n - 1]
  const laneEnd: number[] = [] // last column taken on each lane
  const perDay = Array.from({ length: n }, () => 0) // lanes used per day
  const bars: Bar[] = []
  for (const e of sortEntries(entries.filter(isBar))) {
    if (e.last < days[0] || e.first > lastDay) continue
    const col = e.first < days[0] ? 0 : days.indexOf(e.first)
    const endCol = e.last > lastDay ? n - 1 : days.indexOf(e.last)
    let lane = laneEnd.findIndex((c) => c < col)
    if (lane < 0) lane = laneEnd.length
    laneEnd[lane] = endCol
    for (let c = col; c <= endCol; c++) perDay[c] = Math.max(perDay[c], lane + 1)
    bars.push({ entry: e, col, span: endCol - col + 1, lane, clipStart: e.first < days[0], clipEnd: e.last > lastDay })
  }
  return { bars, lanes: perDay }
}

export interface Placed {
  entry: Entry
  top: number
  height: number
  col: number
  cols: number
  clipTop: boolean
  clipBottom: boolean
}

// layoutDay positions a day's timed events: overlapping ones share the width
// in columns, as in Calendar. minPx keeps short events clickable.
export function layoutDay(entries: Entry[], day: Date, hourPx: number, minPx = 20): Placed[] {
  const dayStart = day.getTime()
  const dayEnd = addDays(day, 1).getTime()
  const pxPerMin = hourPx / 60
  const items = entries
    .filter((e) => !isBar(e) && e.start.getTime() < dayEnd && (e.end.getTime() > dayStart || (e.end.getTime() === e.start.getTime() && e.start.getTime() >= dayStart)))
    .map((e) => {
      const s = Math.max(e.start.getTime(), dayStart)
      const en = Math.min(Math.max(e.end.getTime(), e.start.getTime()), dayEnd)
      const top = ((s - dayStart) / 60000) * pxPerMin
      const height = Math.max(minPx, ((en - s) / 60000) * pxPerMin)
      return { entry: e, top, height, bottom: top + height, clipTop: e.start.getTime() < dayStart, clipBottom: e.end.getTime() > dayEnd }
    })
    .sort((a, b) => a.top - b.top || b.height - a.height)

  const out: Placed[] = []
  let cluster: (typeof items[number] & { col: number })[] = []
  let colEnds: number[] = []
  let clusterBottom = -1
  const flush = () => {
    for (const c of cluster) out.push({ entry: c.entry, top: c.top, height: c.height, col: c.col, cols: colEnds.length, clipTop: c.clipTop, clipBottom: c.clipBottom })
    cluster = []
    colEnds = []
  }
  for (const it of items) {
    if (it.top >= clusterBottom) flush()
    let col = colEnds.findIndex((b) => b <= it.top)
    if (col < 0) col = colEnds.length
    colEnds[col] = it.bottom
    cluster.push({ ...it, col })
    clusterBottom = Math.max(clusterBottom, it.bottom)
  }
  flush()
  return out
}

// tasksByDay groups tasks by due date, timed ones by time.
export function tasksByDay(items: TaskItem[]): Map<string, TaskItem[]> {
  const m = new Map<string, TaskItem[]>()
  const sorted = [...items].sort((a, b) => (a.task.due_time ?? '').localeCompare(b.task.due_time ?? '') || a.task.rank - b.task.rank)
  for (const it of sorted) {
    if (!it.task.due) continue
    const list = m.get(it.task.due)
    if (list) list.push(it)
    else m.set(it.task.due, [it])
  }
  return m
}

export function daysBetween(a: string, b: string): number {
  return Math.round((parseDate(b).getTime() - parseDate(a).getTime()) / 86400000)
}

// eventColor: the colour an entry wears (its own, else its workspace's).
export const eventColor = (e: Entry) => e.occ.color || 'var(--ws)'

const longDate: Intl.DateTimeFormatOptions = { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' }
const shortDate: Intl.DateTimeFormatOptions = { day: 'numeric', month: 'short' }

// whenLines describes when an entry happens, in one or two lines.
export function whenLines(e: Entry): string[] {
  if (e.allDay) {
    if (e.first === e.last) return [fmtDate(parseDate(e.first), longDate)]
    return [`${fmtDate(parseDate(e.first), shortDate)} – ${fmtDate(parseDate(e.last), { ...shortDate, year: 'numeric' })}`]
  }
  if (e.first === e.last) return [fmtDate(e.start, longDate), `${fmtTime(e.start)} – ${fmtTime(e.end)}`]
  return [`${fmtDate(e.start, shortDate)}, ${fmtTime(e.start)} – ${fmtDate(e.end, shortDate)}, ${fmtTime(e.end)}`]
}
