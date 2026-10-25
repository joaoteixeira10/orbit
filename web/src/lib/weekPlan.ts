// The week planner's arithmetic: the free time in working hours (minus
// what's already there — events, tasks at a set time, other blocks), and
// tasks fitted into it: the earliest due first (then the most important),
// each before the end of its due day, split across gaps when it doesn't fit
// in one (pieces of at least a minimum, unless that's all that's left).

export interface Span {
  start: number // ms since 1970
  end: number
}
export interface PlanTask {
  id: string
  minutes: number
  due?: number // the end of its due day (ms); none: any time
  priority: number // 0–3
}
export interface Block extends Span {
  taskId: string
}
export interface Plan {
  blocks: Block[]
  unplaced: { id: string; minutes: number }[] // what didn't fit (minutes left)
}

const MIN = 60_000

// freeTime is the working hours minus what's busy, from now on, as sorted,
// non-overlapping spans.
export function freeTime(work: Span[], busy: Span[], now: number): Span[] {
  const taken = mergeSpans(busy)
  const out: Span[] = []
  for (const w of [...work].sort((a, b) => a.start - b.start)) {
    let start = Math.max(w.start, now)
    for (const b of taken) {
      if (b.end <= start || b.start >= w.end) continue
      if (b.start > start) out.push({ start, end: Math.min(b.start, w.end) })
      start = Math.max(start, b.end)
      if (start >= w.end) break
    }
    if (start < w.end) out.push({ start, end: w.end })
  }
  return out
}

export function mergeSpans(spans: Span[]): Span[] {
  const s = [...spans].filter((x) => x.end > x.start).sort((a, b) => a.start - b.start)
  const out: Span[] = []
  for (const x of s) {
    const last = out[out.length - 1]
    if (last && x.start <= last.end) last.end = Math.max(last.end, x.end)
    else out.push({ ...x })
  }
  return out
}

// planWeek fits tasks into free time. step rounds starts up (minutes);
// minPiece is the shortest piece a task is split into.
export function planWeek(tasks: PlanTask[], free: Span[], opts: { step?: number; minPiece?: number } = {}): Plan {
  const step = (opts.step ?? 15) * MIN
  const minPiece = (opts.minPiece ?? 30) * MIN
  const gaps = free.map((f) => ({ ...f }))
  const order = [...tasks].sort((a, b) => (a.due ?? Infinity) - (b.due ?? Infinity) || b.priority - a.priority || b.minutes - a.minutes)
  const blocks: Block[] = []
  const unplaced: Plan['unplaced'] = []
  for (const t of order) {
    let left = t.minutes * MIN
    const pieces: Block[] = []
    for (const g of gaps) {
      if (left <= 0) break
      const start = Math.ceil(g.start / step) * step
      const end = Math.min(g.end, t.due ?? Infinity)
      const room = end - start
      if (room <= 0) continue
      const take = Math.min(left, room)
      // Too small a piece (unless it finishes the task): skip this gap.
      if (take < Math.min(minPiece, left)) continue
      pieces.push({ taskId: t.id, start, end: start + take })
      left -= take
    }
    if (left > 0) {
      unplaced.push({ id: t.id, minutes: Math.round(left / MIN) })
      // What fitted stays planned only when most of it did.
      if (left > (t.minutes * MIN) / 2) continue
    }
    // Take the pieces out of the gaps.
    for (const p of pieces) {
      blocks.push(p)
      for (let i = 0; i < gaps.length; i++) {
        const g = gaps[i]
        if (p.end <= g.start || p.start >= g.end) continue
        const rest: Span[] = []
        if (g.start < p.start) rest.push({ start: g.start, end: p.start })
        if (p.end < g.end) rest.push({ start: p.end, end: g.end })
        gaps.splice(i, 1, ...rest)
        break
      }
    }
  }
  blocks.sort((a, b) => a.start - b.start)
  return { blocks, unplaced }
}

// workingHours are the spans of the working days from `from` (a local
// midnight) for `days` days: days of the week (0 Sunday … 6 Saturday) and
// hours "HH:MM", in the browser's time.
export function workingHours(from: Date, days: number, weekdays: number[], startHM: string, endHM: string): Span[] {
  const [sh, sm] = startHM.split(':').map(Number)
  const [eh, em] = endHM.split(':').map(Number)
  const out: Span[] = []
  for (let i = 0; i < days; i++) {
    const d = new Date(from.getFullYear(), from.getMonth(), from.getDate() + i)
    if (!weekdays.includes(d.getDay())) continue
    const s = new Date(d.getFullYear(), d.getMonth(), d.getDate(), sh, sm).getTime()
    const e = new Date(d.getFullYear(), d.getMonth(), d.getDate(), eh, em).getTime()
    if (e > s) out.push({ start: s, end: e })
  }
  return out
}

// estimateText: minutes as people say them ("45m", "1h 30m", "2h").
export function estimateText(min: number): string {
  if (!min) return ''
  const h = Math.floor(min / 60)
  const m = min % 60
  return h ? (m ? `${h}h ${m}m` : `${h}h`) : `${m}m`
}
