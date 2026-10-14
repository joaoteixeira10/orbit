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
  const words = input.trim().split(/\s+/)
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
  if (out.due_time && !out.due) out.due = dateStr(now)
  out.title = keep.join(' ')
  return out
}
