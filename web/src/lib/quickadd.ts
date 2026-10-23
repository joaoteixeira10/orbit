// Quick add: "Essay tomorrow 14:30 !! #maths" → a task titled "Essay", due
// tomorrow at 14:30, priority 2, tagged maths. Understood words, in English
// and Portuguese: today/hoje, tomorrow/amanhã, a weekday (next one), "next
// week"/"próxima semana", a time (14:30, 9h, 2pm), !, !!, !!! and #tags.
// Anything else stays in the title.
import { dateStr, addDays } from './dates'

export interface Quick {
  title: string
  due?: string
  due_time?: string
  priority?: number
  tags: string[]
  repeat?: string // an RRULE, e.g. FREQ=WEEKLY;INTERVAL=2;BYDAY=TU
}

const weekdays: Record<string, number> = {
  sunday: 0, sun: 0, domingo: 0, dom: 0,
  monday: 1, mon: 1, segunda: 1, seg: 1,
  tuesday: 2, tue: 2, terça: 2, terca: 2, ter: 2,
  wednesday: 3, wed: 3, quarta: 3, qua: 3,
  thursday: 4, thu: 4, quinta: 4, qui: 4,
  friday: 5, fri: 5, sexta: 5, sex: 5,
  saturday: 6, sat: 6, sábado: 6, sabado: 6, sáb: 6, sab: 6,
}

const BYDAY = ['SU', 'MO', 'TU', 'WE', 'TH', 'FR', 'SA']

// A weekday in a repeat: a name or its short form, plural too ("tuesdays",
// "terças", "terças-feiras").
const WD = '(?:sunday|monday|tuesday|wednesday|thursday|friday|saturday|sun|mon|tue|wed|thu|fri|sat|domingo|segunda|terça|terca|quarta|quinta|sexta|sábado|sabado)s?(?:-feiras?)?'
const DAYS = `${WD}(?:\\s*(?:,|and|&|e)\\s*${WD})*`
const NUM: Record<string, number> = { two: 2, three: 3, four: 4, dois: 2, duas: 2, três: 3, tres: 3, quatro: 4 }
const N = '(\\d{1,3}|two|three|four|dois|duas|três|tres|quatro)'
const UNIT: Record<string, string> = { day: 'DAILY', dia: 'DAILY', week: 'WEEKLY', semana: 'WEEKLY', month: 'MONTHLY', mês: 'MONTHLY', mes: 'MONTHLY', year: 'YEARLY', ano: 'YEARLY' }
const UNITS = '(days?|weeks?|months?|years?|dias?|semanas?|mês|mes(?:es)?|anos?)'

const num = (s: string) => NUM[s] ?? +s
const unit = (s: string) => UNIT[s.replace(/es$|s$/, '')] ?? UNIT[s]
const dayCodes = (s: string) =>
  [...new Set(s.split(/\s*(?:,|\band\b|&|\be\b)\s*/).map((d) => d.replace(/-feiras?$/, '').replace(/s$/, '')).filter((d) => d in weekdays).map((d) => BYDAY[weekdays[d]]))]
const rule = (freq: string, interval = 1, days: string[] = []) =>
  `FREQ=${freq}${interval > 1 ? `;INTERVAL=${interval}` : ''}${days.length ? `;BYDAY=${days.join(',')}` : ''}`

// REPEATS: each phrase and the rule it means. Phrases are whole words (a
// space or the ends around them). "daily", "weekly"… on their own count only
// after the first word, so "Weekly review" stays a title.
const REPEATS: [RegExp, (m: RegExpMatchArray) => string][] = [
  [new RegExp(`every\\s+other\\s+(${DAYS})`), (m) => rule('WEEKLY', 2, dayCodes(m[1]))],
  [new RegExp(`every\\s+other\\s+(day|week|month|year)`), (m) => rule(UNIT[m[1]], 2)],
  [new RegExp(`every\\s+${N}\\s+${UNITS}`), (m) => rule(unit(m[2]), num(m[1]))],
  [/every\s+(?:weekday|workday)s?|(?:todos\s+os\s+)?dias\s+úteis|(?:todos\s+os\s+)?dias\s+uteis/, () => rule('WEEKLY', 1, ['MO', 'TU', 'WE', 'TH', 'FR'])],
  [new RegExp(`every\\s+(${DAYS})`), (m) => rule('WEEKLY', 1, dayCodes(m[1]))],
  [/every\s+(day|week|month|year)/, (m) => rule(UNIT[m[1]])],
  [new RegExp(`(${WD})\\s+sim,?\\s+(${WD})\\s+não`), (m) => rule('WEEKLY', 2, dayCodes(m[1]))],
  [/(dia|semana|mês|mes|ano)\s+sim,?\s+(?:dia|semana|mês|mes|ano)\s+não/, (m) => rule(UNIT[m[1]], 2)],
  [new RegExp(`de\\s+${N}\\s+em\\s+\\S+\\s+${UNITS}`), (m) => rule(unit(m[2]), num(m[1]))],
  [new RegExp(`(?:a\\s+)?cada\\s+${N}\\s+${UNITS}`), (m) => rule(unit(m[2]), num(m[1]))],
  [new RegExp(`tod[oa]s\\s+(?:os|as)\\s+(${DAYS})`), (m) => rule('WEEKLY', 1, dayCodes(m[1]))],
  [/tod[oa]s\s+(?:os|as)\s+(dias|semanas|meses|anos)/, (m) => rule(unit(m[1]))],
  [/(?<=\S\s+)(?:daily|diariamente)/, () => rule('DAILY')],
  [/(?<=\S\s+)(?:weekly|semanalmente)/, () => rule('WEEKLY')],
  [/(?<=\S\s+)(?:biweekly|fortnightly|quinzenalmente)/, () => rule('WEEKLY', 2)],
  [/(?<=\S\s+)(?:monthly|mensalmente)/, () => rule('MONTHLY')],
  [/(?<=\S\s+)(?:yearly|annually|anualmente)/, () => rule('YEARLY')],
]

// takeRepeat finds a repeat phrase in input: the rule, and input without it.
function takeRepeat(input: string): { repeat?: string; rest: string } {
  const low = input.toLowerCase()
  for (const [re, make] of REPEATS) {
    const m = low.match(new RegExp(`(?<=^|\\s)(?:${re.source})(?=$|[\\s,.;!])`))
    if (!m || m.index === undefined) continue
    const r = make(m)
    if (/BYDAY=(?:;|$)/.test(r) || r.includes('undefined')) continue
    return { repeat: r, rest: (input.slice(0, m.index) + ' ' + input.slice(m.index + m[0].length)).replace(/\s+/g, ' ').trim() }
  }
  return { rest: input }
}

// firstDay is the first date a rule falls on, from today: its first weekday
// (today included) or today.
function firstDay(r: string, now: Date): string {
  const days = r.match(/BYDAY=([A-Z,]+)/)?.[1].split(',').map((d) => BYDAY.indexOf(d)) ?? []
  if (!days.length) return dateStr(now)
  const diff = Math.min(...days.map((d) => (d - now.getDay() + 7) % 7))
  return dateStr(addDays(now, diff))
}

const pad = (n: number) => String(n).padStart(2, '0')

// parseTime reads 14:30, 14h30, 9h, 2pm, 2:15pm.
function parseTime(w: string): string | undefined {
  let m = w.match(/^(\d{1,2})(?::|h)(\d{2})$/)
  if (m) {
    const h = +m[1], mi = +m[2]
    if (h < 24 && mi < 60) return `${pad(h)}:${pad(mi)}`
  }
  m = w.match(/^(\d{1,2})h$/)
  if (m && +m[1] < 24) return `${pad(+m[1])}:00`
  m = w.match(/^(\d{1,2})(?::(\d{2}))?(am|pm)$/)
  if (m) {
    let h = +m[1] % 12
    if (m[3] === 'pm') h += 12
    const mi = m[2] ? +m[2] : 0
    if (+m[1] <= 12 && mi < 60) return `${pad(h)}:${pad(mi)}`
  }
  return undefined
}

export function parseQuick(input: string, now = new Date()): Quick {
  const out: Quick = { title: '', tags: [] }
  const { repeat, rest } = takeRepeat(input.trim())
  if (repeat) out.repeat = repeat
  const words = rest ? rest.split(/\s+/) : []
  const keep: string[] = []
  for (let i = 0; i < words.length; i++) {
    const raw = words[i]
    const w = raw.toLowerCase()
    const next = (words[i + 1] ?? '').toLowerCase()
    if (/^!{1,3}$/.test(w)) {
      out.priority = w.length
    } else if (/^#[\p{L}\p{N}_-]+$/u.test(raw)) {
      out.tags.push(raw.slice(1))
    } else if (w === 'today' || w === 'hoje') {
      out.due = dateStr(now)
    } else if (w === 'tomorrow' || w === 'amanhã' || w === 'amanha') {
      out.due = dateStr(addDays(now, 1))
    } else if ((w === 'next' && next === 'week') || (w === 'próxima' && next === 'semana') || (w === 'proxima' && next === 'semana')) {
      out.due = dateStr(addDays(now, 7))
      i++
    } else if (w in weekdays && (i > 0 || words.length > 1)) {
      // A weekday name means the next one (today's weekday = in a week).
      const diff = (weekdays[w] - now.getDay() + 7) % 7 || 7
      out.due = dateStr(addDays(now, diff))
    } else if ((w === 'at' || w === 'às' || w === 'as') && parseTime(next)) {
      out.due_time = parseTime(next)
      i++
    } else if (parseTime(w) && keep.length) {
      out.due_time = parseTime(w)
    } else {
      keep.push(raw)
    }
  }
  if (out.repeat && !out.due) out.due = firstDay(out.repeat, now)
  if (out.due_time && !out.due) out.due = dateStr(now)
  out.title = keep.join(' ')
  return out
}
