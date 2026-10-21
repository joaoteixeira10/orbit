// Date helpers for the planner. Tasks carry floating dates ("2026-10-20",
// "14:30"), read in the browser's zone; events are instants shown in it too.
// Names come from Intl in the UI's language, so they follow en / pt.
import { i18n } from '@/i18n'
import { usePrefsStore } from '@/stores/prefs'

const pad = (n: number) => String(n).padStart(2, '0')

export function locale(): string {
  const l = String(i18n.global.locale.value)
  return l === 'pt' ? 'pt-PT' : 'en-GB'
}

// dateStr formats a Date as its local calendar date (YYYY-MM-DD).
export function dateStr(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}
export function todayStr(): string {
  return dateStr(new Date())
}
// parseDate reads YYYY-MM-DD as local midnight.
export function parseDate(s: string): Date {
  const [y, m, d] = s.split('-').map(Number)
  return new Date(y, m - 1, d)
}
export function addDays(d: Date, n: number): Date {
  const x = new Date(d)
  x.setDate(x.getDate() + n)
  return x
}
export function addMonths(d: Date, n: number): Date {
  const x = new Date(d.getFullYear(), d.getMonth() + n, 1)
  return x
}
export function sameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate()
}
export function startOfDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate())
}
export function weekStart(): 0 | 1 {
  return usePrefsStore().weekStart
}
export function startOfWeek(d: Date, ws: number = weekStart()): Date {
  const x = startOfDay(d)
  const diff = (x.getDay() - ws + 7) % 7
  return addDays(x, -diff)
}
// monthGrid returns the 6 weeks shown for a month.
export function monthGrid(month: Date, ws: number = weekStart()): Date[] {
  const first = new Date(month.getFullYear(), month.getMonth(), 1)
  const start = startOfWeek(first, ws)
  return Array.from({ length: 42 }, (_, i) => addDays(start, i))
}
export function deviceTZ(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  } catch {
    return 'UTC'
  }
}

// ---- Names ----
export function weekdayNames(style: 'short' | 'narrow' | 'long' = 'short', ws: number = weekStart()): string[] {
  const f = new Intl.DateTimeFormat(locale(), { weekday: style })
  // 2023-01-01 was a Sunday.
  return Array.from({ length: 7 }, (_, i) => f.format(new Date(2023, 0, 1 + ((i + ws) % 7))))
}
export function monthTitle(d: Date): string {
  const s = new Intl.DateTimeFormat(locale(), { month: 'long', year: 'numeric' }).format(d)
  return s.charAt(0).toUpperCase() + s.slice(1)
}
export function fmtDate(d: Date, opts: Intl.DateTimeFormatOptions = { day: 'numeric', month: 'short' }): string {
  return new Intl.DateTimeFormat(locale(), opts).format(d)
}
export function fmtTime(d: Date): string {
  const h12 = usePrefsStore().timeFormat === '12h'
  return new Intl.DateTimeFormat(locale(), { hour: 'numeric', minute: '2-digit', hour12: h12 }).format(d)
}
// fmtClock formats a floating "HH:MM" in the user's time format.
export function fmtClock(hhmm: string): string {
  const [h, m] = hhmm.split(':').map(Number)
  return fmtTime(new Date(2000, 0, 1, h, m))
}

// relDay names a date relative to today: Today, Tomorrow, Yesterday, a
// weekday within the week ahead, else a short date (with the year if not this year).
export function relDay(s: string): string {
  const t = i18n.global.t
  const d = parseDate(s)
  const today = startOfDay(new Date())
  const diff = Math.round((d.getTime() - today.getTime()) / 86400000)
  if (diff === 0) return t('dates.today')
  if (diff === 1) return t('dates.tomorrow')
  if (diff === -1) return t('dates.yesterday')
  if (diff > 1 && diff < 7) {
    const w = new Intl.DateTimeFormat(locale(), { weekday: 'long' }).format(d)
    return w.charAt(0).toUpperCase() + w.slice(1)
  }
  return fmtDate(d, d.getFullYear() === today.getFullYear() ? { day: 'numeric', month: 'short' } : { day: 'numeric', month: 'short', year: 'numeric' })
}

// dueLabel is a task's due date (and time) for lists.
export function dueLabel(due: string | null, time: string | null): string {
  if (!due) return ''
  return time ? `${relDay(due)}, ${fmtClock(time)}` : relDay(due)
}
export function isOverdue(due: string | null, time: string | null, done = false): boolean {
  if (!due || done) return false
  const today = todayStr()
  if (due < today) return true
  if (due > today || !time) return false
  const now = new Date()
  return time < `${pad(now.getHours())}:${pad(now.getMinutes())}`
}

// ---- Repeat rules (the subset the UI offers) ----
export type RepeatPreset = '' | 'daily' | 'weekdays' | 'weekly' | 'biweekly' | 'monthly' | 'yearly' | 'custom'
export const REPEAT_RULES: Record<Exclude<RepeatPreset, '' | 'custom'>, string> = {
  daily: 'FREQ=DAILY',
  weekdays: 'FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR',
  weekly: 'FREQ=WEEKLY',
  biweekly: 'FREQ=WEEKLY;INTERVAL=2',
  monthly: 'FREQ=MONTHLY',
  yearly: 'FREQ=YEARLY',
}
export function repeatPreset(rule: string): RepeatPreset {
  if (!rule) return ''
  for (const [k, v] of Object.entries(REPEAT_RULES)) if (v === rule) return k as RepeatPreset
  return 'custom'
}
export function repeatLabel(rule: string): string {
  const p = repeatPreset(rule)
  return p ? i18n.global.t(`repeat.${p}`) : ''
}

// toLocalInput / fromLocalInput convert between a Date and the value of an
// <input type="datetime-local"> (and "date").
export function toLocalInput(d: Date, withTime = true): string {
  const date = dateStr(d)
  return withTime ? `${date}T${pad(d.getHours())}:${pad(d.getMinutes())}` : date
}
export function fromLocalInput(v: string): Date {
  if (!v.includes('T')) return parseDate(v)
  const [date, time] = v.split('T')
  const d = parseDate(date)
  const [h, m] = time.split(':').map(Number)
  d.setHours(h, m, 0, 0)
  return d
}
// ---- Time zones: an event's times typed in its own zone ----

// zoneOffset is how far tz's wall clock is ahead of UTC at instant ms.
export function zoneOffset(ms: number, tz: string): number {
  const p: Record<string, number> = {}
  for (const x of new Intl.DateTimeFormat('en-US', { timeZone: tz, hourCycle: 'h23', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).formatToParts(new Date(ms))) {
    if (x.type !== 'literal') p[x.type] = Number(x.value)
  }
  return Date.UTC(p.year, p.month - 1, p.day, p.hour % 24, p.minute, p.second) - Math.floor(ms / 1000) * 1000
}
// toZoneInput: an instant as tz's wall clock, for <input type="datetime-local">.
export function toZoneInput(d: Date, tz: string): string {
  const w = new Date(d.getTime() + zoneOffset(d.getTime(), tz))
  return `${w.getUTCFullYear()}-${pad(w.getUTCMonth() + 1)}-${pad(w.getUTCDate())}T${pad(w.getUTCHours())}:${pad(w.getUTCMinutes())}`
}
// fromZoneInput: tz's wall clock ("YYYY-MM-DDTHH:MM") as an instant. A time
// that doesn't exist (the hour skipped in spring) lands just after the gap.
export function fromZoneInput(v: string, tz: string): Date {
  const [date, time = '00:00'] = v.split('T')
  const [y, m, d] = date.split('-').map(Number)
  const [h, mi] = time.split(':').map(Number)
  const wall = Date.UTC(y, m - 1, d, h, mi)
  let t = wall - zoneOffset(wall, tz)
  t = wall - zoneOffset(t, tz)
  return new Date(t)
}
// zoneName: "America/New_York" → "New York"; with the offset when asked.
export function zoneName(tz: string, at?: Date): string {
  const city = (tz.split('/').pop() ?? tz).replace(/_/g, ' ')
  if (!at) return city
  try {
    const off = new Intl.DateTimeFormat('en-US', { timeZone: tz, timeZoneName: 'shortOffset' }).formatToParts(at).find((x) => x.type === 'timeZoneName')?.value
    return off ? `${city} (${off})` : city
  } catch {
    return city
  }
}
// allZones: every zone the browser knows (a short list where it can't say).
export function allZones(): string[] {
  try {
    const list = (Intl as unknown as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf?.('timeZone')
    if (list?.length) return list
  } catch {
    /* older browsers */
  }
  return ['UTC', 'Europe/Lisbon', 'Europe/London', 'Europe/Madrid', 'Europe/Paris', 'Europe/Berlin', 'America/New_York', 'America/Chicago', 'America/Los_Angeles', 'America/Sao_Paulo', 'Asia/Tokyo', 'Australia/Sydney']
}
// sameZoneTime: whether two zones show the same clock at an instant.
export const sameZoneTime = (a: string, b: string, at: Date) => zoneOffset(at.getTime(), a) === zoneOffset(at.getTime(), b)

// utcMidnight is the instant the server uses for an all-day date.
export function utcMidnight(s: string): string {
  return `${s}T00:00:00Z`
}
// allDayDate reads an all-day instant back as its date.
export function allDayDate(iso: string): string {
  return iso.slice(0, 10)
}

// countdown is how far away a date is: Today, Tomorrow, In 5 days.
export function countdown(s: string): string {
  const days = Math.round((parseDate(s).getTime() - startOfDay(new Date()).getTime()) / 86400000)
  const rtf = new Intl.RelativeTimeFormat(locale(), { numeric: 'auto' })
  const out = rtf.format(days, 'day')
  return out.charAt(0).toUpperCase() + out.slice(1)
}
