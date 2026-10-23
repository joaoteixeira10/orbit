// @-mentions in pages, as plain Markdown so they read anywhere: a person is
// @username (they're told), a date @2026-10-23 (shown as a date, opening the
// calendar on it), a page a link to it, [Title](/w/<ws>/p/<id>) — which is
// also how a page knows what links to it (backlinks).
import { addDays, dateStr, locale, parseDate } from './dates'

export interface MentionPerson {
  user_id: string
  username: string
  display_name: string
}
export interface MentionPage {
  id: string
  title: string
  icon: string
  trashed_at?: string | null
}
export interface MentionContext {
  ws: string
  members: MentionPerson[]
  pages: MentionPage[]
  self?: string // the page being written (not offered)
  now?: Date
}

export type MentionKind = 'person' | 'page' | 'date'
export interface MentionItem {
  key: string
  kind: MentionKind
  title: string
  desc: string
  // What goes in: text, linked to href when there's one.
  text: string
  href?: string
}

type T = (key: string, p?: Record<string, unknown>) => string

// Accent-insensitive lower case ("sabado" finds "Sábado").
const fold = (s: string) => s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()

// The date words understood after "@", in English and Portuguese, besides
// the dates' own names.
const DAY_WORDS: [string[], number][] = [
  [['today', 'hoje'], 0],
  [['tomorrow', 'amanha'], 1],
  [['yesterday', 'ontem'], -1],
]

// dateLabel names a date for a chip: "Fri 23 Oct" (with the year when not
// this one).
export function dateLabel(iso: string, now = new Date()): string {
  const d = parseDate(iso)
  return new Intl.DateTimeFormat(locale(), {
    weekday: 'short', day: 'numeric', month: 'short', year: d.getFullYear() === now.getFullYear() ? undefined : 'numeric',
  }).format(d)
}

function dateItems(q: string, now: Date, t: T): MentionItem[] {
  const item = (d: Date, title: string): MentionItem => {
    const iso = dateStr(d)
    return { key: 'd' + iso, kind: 'date', title, desc: dateLabel(iso, now), text: '@' + iso }
  }
  const out: MentionItem[] = []
  const f = fold(q)
  // A date typed out.
  if (/^\d{4}-\d{2}-\d{2}$/.test(q) && dateStr(parseDate(q)) === q) out.push(item(parseDate(q), q))
  for (const [words, n] of DAY_WORDS) {
    const title = t(n === 0 ? 'dates.today' : n === 1 ? 'dates.tomorrow' : 'dates.yesterday')
    if (!f || words.some((w) => w.startsWith(f)) || fold(title).startsWith(f)) out.push(item(addDays(now, n), title))
  }
  // The next seven days by name ("friday", "sexta").
  if (f) {
    const long = new Intl.DateTimeFormat(locale(), { weekday: 'long' })
    const en = new Intl.DateTimeFormat('en', { weekday: 'long' })
    for (let n = 2; n <= 7; n++) {
      const d = addDays(now, n)
      const name = long.format(d)
      if (fold(name).startsWith(f) || fold(en.format(d)).startsWith(f)) out.push(item(d, name.charAt(0).toUpperCase() + name.slice(1)))
    }
  }
  return out
}

// mentionItems is what "@" + q offers: people, then pages, then dates —
// those whose names start with q first.
export function mentionItems(q: string, c: MentionContext, t: T): MentionItem[] {
  const f = fold(q.trim())
  const rank = (names: string[]) => {
    const n = names.map(fold)
    return !f ? 1 : n.some((x) => x.startsWith(f)) ? 2 : n.some((x) => x.includes(f)) ? 1 : 0
  }
  const best = <X>(xs: X[], names: (x: X) => string[], max: number) =>
    xs.map((x) => ({ x, r: rank(names(x)) })).filter((e) => e.r > 0).sort((a, b) => b.r - a.r).slice(0, max).map((e) => e.x)

  const people = best(c.members, (m) => [m.username, ...m.display_name.split(/\s+/), m.display_name], 5).map<MentionItem>((m) => ({
    key: 'u' + m.user_id, kind: 'person', title: m.display_name || m.username, desc: '@' + m.username, text: '@' + m.username,
  }))
  const pages = best(c.pages.filter((p) => p.id !== c.self && !p.trashed_at), (p) => [p.title || t('pages.untitled')], 5).map<MentionItem>((p) => ({
    key: 'p' + p.id, kind: 'page', title: `${p.icon ? p.icon + ' ' : ''}${p.title || t('pages.untitled')}`, desc: t('mention.page'),
    text: p.title || t('pages.untitled'), href: `/w/${c.ws}/p/${p.id}`,
  }))
  return [...people, ...pages, ...dateItems(q.trim(), c.now ?? new Date(), t)]
}

// markdownOf writes an item as Markdown (the source editor's insert).
export function markdownOf(it: MentionItem): string {
  if (!it.href) return it.text
  const label = it.text.replace(/([[\]\\])/g, '\\$1')
  return `[${label}](${it.href})`
}

// MENTION finds @username and @YYYY-MM-DD in text (not inside a word or an
// address: "ana@example.org" is no mention).
export const MENTION = /(^|[^\w@./-])@(\d{4}-\d{2}-\d{2}|[a-z0-9][a-z0-9._-]{0,30}[a-z0-9_])/gi
export const isDate = (s: string) => /^\d{4}-\d{2}-\d{2}$/.test(s) && dateStr(parseDate(s)) === s
