// Pure helpers for quick notes: the editable draft (capture box and editor),
// converting text ↔ checklist, search, copy text, and turning a note into a
// page document or tasks.
import type { Note, NoteColor, NoteInput } from '@/api/planner'

export const MAX_TITLE = 300
export const MAX_BODY = 20000
export const MAX_ITEMS = 200
export const MAX_ITEM_TEXT = 500
export const MAX_TASKS = 50

// A checklist row being edited; k is a stable local key (new rows have no id).
export interface DraftItem {
  k: number
  id: string
  text: string
  done: boolean
}
export interface Draft {
  title: string
  body: string
  checklist: boolean
  items: DraftItem[]
  color: NoteColor
  pinned: boolean
}

let seq = 0
export function newItem(text = '', done = false, id = ''): DraftItem {
  return { k: ++seq, id, text, done }
}

export function draftOf(n?: Note | null, over: Partial<Draft> = {}): Draft {
  return {
    title: n?.title ?? '',
    body: n?.body ?? '',
    checklist: n?.checklist ?? false,
    items: (n?.items ?? []).map((i) => newItem(i.text, i.done, i.id)),
    color: n?.color ?? '',
    pinned: n?.pinned ?? false,
    ...over,
  }
}

// inputOf is what gets sent for a draft (empty checklist rows dropped).
export function inputOf(d: Draft): NoteInput {
  return {
    title: d.title.trim().slice(0, MAX_TITLE),
    body: d.checklist ? '' : d.body.slice(0, MAX_BODY),
    checklist: d.checklist,
    items: d.checklist
      ? d.items.filter((i) => i.text.trim()).slice(0, MAX_ITEMS).map((i) => ({ id: i.id, text: i.text.trim().slice(0, MAX_ITEM_TEXT), done: i.done }))
      : [],
    color: d.color,
    pinned: d.pinned,
  }
}

export function isEmptyDraft(d: Draft): boolean {
  return !d.title.trim() && (d.checklist ? !d.items.some((i) => i.text.trim()) : !d.body.trim())
}

// sameContent tells whether a draft would change nothing on a note.
export function sameContent(d: Draft, n: Note): boolean {
  const a = inputOf(d)
  const items = (x: { text: string; done: boolean }[]) => JSON.stringify(x.map((i) => [i.text, i.done]))
  return (
    a.title === n.title &&
    a.checklist === n.checklist &&
    a.color === n.color &&
    a.pinned === n.pinned &&
    (n.checklist ? items(a.items ?? []) === items(n.items) : (a.body ?? '').trimEnd() === n.body)
  )
}

// linesOf returns a text's non-empty lines without list bullets.
export function linesOf(body: string): string[] {
  return body
    .split('\n')
    .map((l) => l.trim().replace(/^(?:[-•*]|\[[ xX]?\])\s*/, '').trim())
    .filter(Boolean)
}

// toChecklist / toText switch a draft between the two forms, keeping lines.
export function toChecklist(d: Draft): void {
  if (d.checklist) return
  d.items = linesOf(d.body).slice(0, MAX_ITEMS).map((l) => newItem(l.slice(0, MAX_ITEM_TEXT)))
  d.body = ''
  d.checklist = true
}
export function toText(d: Draft): void {
  if (!d.checklist) return
  d.body = d.items
    .map((i) => i.text.trim())
    .filter(Boolean)
    .join('\n')
  d.items = []
  d.checklist = false
}

// convertedInput is the update that flips a stored note's form.
export function convertedInput(n: Note): NoteInput {
  const d = draftOf(n)
  if (n.checklist) toText(d)
  else toChecklist(d)
  return { checklist: d.checklist, body: d.body, items: inputOf(d).items }
}

// orderedItems puts unchecked items first, then the checked ones.
export function orderedItems<T extends { done: boolean }>(items: T[]): T[] {
  return [...items.filter((i) => !i.done), ...items.filter((i) => i.done)]
}

// noteText is a note as plain text (copy, search).
export function noteText(n: Pick<Note, 'title' | 'body' | 'checklist' | 'items'>): string {
  const body = n.checklist ? orderedItems(n.items).map((i) => `${i.done ? '☑' : '☐'} ${i.text}`).join('\n') : n.body
  return [n.title, body].filter((s) => s.trim()).join('\n\n')
}

export function matches(n: Note, q: string): boolean {
  const ql = q.trim().toLowerCase()
  if (!ql) return true
  return n.title.toLowerCase().includes(ql) || n.body.toLowerCase().includes(ql) || n.items.some((i) => i.text.toLowerCase().includes(ql))
}

// pageDoc is the page (Markdown) for a note: its text as a page would show
// what was typed (noteMarkdown), or a task list for a checklist.
export function pageDoc(n: Pick<Note, 'body' | 'checklist' | 'items'>): string {
  if (n.checklist) return n.items.map((i) => `- [${i.done ? 'x' : ' '}] ${inline(i.text)}`).join('\n')
  return noteMarkdown(n.body)
}

// A note is plain text, a page Markdown. What people type in a note as
// structure becomes that structure on the page: "- " / "* " / "• " and "1. "
// lines lists, "[ ] " / "[x] " lines tasks, "# " lines headings, "> " lines
// quotes, and links stay links. Everything else is escaped (a "*" stays a
// star), and line breaks stay where they were.
const LIST = /^(\s*)([-*•+])\s+(.*)$/
const NUMBERED = /^(\s*)(\d{1,9})([.)])\s+(.*)$/
const TASK = /^(\s*)(?:[-*•+]\s+)?\[([ xX])\]\s+(.*)$/
const HEADING = /^(#{1,6})\s+(.*)$/
const QUOTE = /^>\s?(.*)$/
const URL_RE = /(https?:\/\/[^\s<>]+[^\s<>.,;:!?)\]'"])/

// inline escapes a line's text for Markdown, leaving web addresses whole.
function inline(text: string): string {
  return text
    .split(URL_RE)
    .map((part, i) => (i % 2 ? part : part.replace(/([\\`*_[\]~<#>|])/g, '\\$1')))
    .join('')
}

type Block = 'text' | 'list' | 'quote' | 'heading'

export function noteMarkdown(body: string): string {
  const out: string[] = []
  let prev: Block | '' = '' // the line before ('' after a blank line)
  for (const raw of body.replace(/\r\n?/g, '\n').split('\n')) {
    const line = raw.trimEnd()
    if (!line.trim()) {
      if (prev) out.push('')
      prev = ''
      continue
    }
    let md: string
    let kind: Block
    let m: RegExpMatchArray | null
    if ((m = line.match(TASK))) [md, kind] = [`${m[1]}- [${m[2] === ' ' ? ' ' : 'x'}] ${inline(m[3])}`, 'list']
    else if ((m = line.match(LIST))) [md, kind] = [`${m[1]}- ${inline(m[3])}`, 'list']
    else if ((m = line.match(NUMBERED))) [md, kind] = [`${m[1]}${m[2]}${m[3]} ${inline(m[4])}`, 'list']
    else if ((m = line.match(HEADING))) [md, kind] = [`${m[1]} ${inline(m[2])}`, 'heading']
    else if ((m = line.match(QUOTE))) [md, kind] = [`> ${inline(m[1])}`, 'quote']
    else {
      // A line of only "-" or "=" would make a rule or a heading of the one above.
      md = inline(line.trimStart()).replace(/^([-=])/, '\\$1')
      kind = 'text'
    }
    if (prev && (prev !== kind || kind === 'heading')) out.push('') // a new block
    else if (prev === kind && kind !== 'list') out[out.length - 1] += '  ' // a line break within it
    out.push(md)
    prev = kind
  }
  return out.join('\n').trim()
}

// taskTitles are the tasks a note turns into: its unchecked items, or its
// lines (the title when there is nothing else).
export function taskTitles(n: Pick<Note, 'title' | 'body' | 'checklist' | 'items'>): string[] {
  let out = n.checklist ? n.items.filter((i) => !i.done).map((i) => i.text.trim()) : linesOf(n.body)
  out = out.filter(Boolean)
  if (!out.length && n.title.trim() && (!n.checklist || !n.items.length)) out = [n.title.trim()]
  return out.slice(0, MAX_TASKS).map((s) => s.slice(0, 500))
}

// ago says how long ago an instant was ("2 hours ago", "yesterday").
export function ago(iso: string, locale: string, now = Date.now()): string {
  const secs = (new Date(iso).getTime() - now) / 1000
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })
  const steps: [Intl.RelativeTimeFormatUnit, number][] = [
    ['second', 60],
    ['minute', 60],
    ['hour', 24],
    ['day', 7],
    ['week', 4.35],
    ['month', 12],
    ['year', Infinity],
  ]
  let v = secs
  for (const [unit, size] of steps) {
    if (Math.abs(v) < size) return unit === 'second' ? rtf.format(0, 'second') : rtf.format(Math.round(v), unit)
    v /= size
  }
  return ''
}
