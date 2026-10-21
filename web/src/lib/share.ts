// What another app shares into Orbit (the phone's share sheet, through the
// web manifest's share_target, or a shortcut opening /share?…) becomes a
// Quick Note. Apps fill title / text / url unevenly — many put the link in
// text, some repeat the title in it — so this tidies them into one note.
import type { NoteInput } from '@/api/planner'

export interface Shared {
  title?: string
  text?: string
  url?: string
}

const clean = (s?: string) => (s ?? '').replace(/\r\n?/g, '\n').trim()

// shareNote turns what was shared into a note (null when there's nothing).
export function shareNote(s: Shared): NoteInput | null {
  let title = clean(s.title)
  let text = clean(s.text)
  const url = clean(s.url)
  // The title repeated as the text's first line.
  if (title && text.startsWith(title) && /^(\s|$)/.test(text.slice(title.length))) text = text.slice(title.length).trim()
  const lines = text ? [text] : []
  if (url && !text.includes(url)) lines.push(url)
  let body = lines.join('\n\n')
  // Only a title: it's the note.
  if (!body && title) {
    body = title
    title = ''
  }
  if (!body) return null
  if (title.length > 300) title = `${title.slice(0, 299)}…`
  if (body.length > 20000) body = body.slice(0, 20000)
  return title ? { title, body } : { body }
}
