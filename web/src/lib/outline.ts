// What the outline panel beside a page shows, read from its Markdown: its
// headings (to jump to), and its callouts and checklist items gathered in
// one list. Indexes count as the page shows them (the n-th heading, the
// n-th callout, the n-th checkbox — as toggleTask counts).

export interface OutlineHeading {
  level: 1 | 2 | 3
  text: string
  index: number
}
export interface OutlineCallout {
  kind: string // NOTE | TIP | IMPORTANT | WARNING | CAUTION
  text: string
  index: number
}
export interface OutlineTask {
  done: boolean
  text: string
  index: number
}
export interface Outline {
  headings: OutlineHeading[]
  callouts: OutlineCallout[]
  tasks: OutlineTask[]
}

const TASK = /^(\s*(?:[-*+]|\d+[.)])\s+\[)([ xX])(\].*)$/
const HEADING = /^(#{1,6})\s+(.*?)\s*#*\s*$/
const ALERT = /^>\s?\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*(.*)$/i

// plain is a line's text without its Markdown marks.
export function plain(s: string): string {
  return s
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/<[^>]+>/g, '')
    .replace(/(\*\*|__|~~|`)/g, '')
    .replace(/(^|\W)[*_](\S[^*_]*)[*_](?=\W|$)/g, '$1$2')
    .trim()
}

export function outlineOf(md: string): Outline {
  const out: Outline = { headings: [], callouts: [], tasks: [] }
  const lines = md.replace(/\r\n/g, '\n').split('\n')
  let fence = ''
  let h = 0
  let c = 0
  let t = 0
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    const f = /^\s*(`{3,}|~{3,})/.exec(line)
    if (fence) {
      if (f && f[1][0] === fence[0]) fence = ''
      continue
    }
    if (f) {
      fence = f[1]
      continue
    }
    const hm = HEADING.exec(line)
    if (hm) {
      // The page shows h1–h3 (h4–h6 read as h3's look but are counted too).
      const level = Math.min(hm[1].length, 3) as 1 | 2 | 3
      out.headings.push({ level, text: plain(hm[2]), index: h++ })
      continue
    }
    const am = ALERT.exec(line)
    if (am) {
      let text = am[2]
      for (let j = i + 1; !text && j < lines.length && /^>/.test(lines[j]); j++) text = lines[j].replace(/^>\s?/, '').trim()
      out.callouts.push({ kind: am[1].toUpperCase(), text: plain(text), index: c++ })
      continue
    }
    const tm = TASK.exec(line)
    if (tm) out.tasks.push({ done: tm[2] !== ' ', text: plain(tm[3].slice(1)), index: t++ })
  }
  return out
}
