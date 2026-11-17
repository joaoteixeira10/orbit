// Editing a page together: its starting document, built from the page's
// Markdown into the shared (Yjs) document before any editor shows it — with
// the same extensions as the editor, so every tab builds the same.
import { Editor, type JSONContent } from '@tiptap/core'
import { prosemirrorJSONToYXmlFragment } from '@tiptap/y-tiptap'
import type { CollabLink } from '@/lib/collab'
import { pageExtensions } from './extensions'

export const FIELD = 'default'

export function seedPage(link: CollabLink, markdown: string) {
  const ed = new Editor({ extensions: pageExtensions(), content: markdown, contentType: 'markdown' })
  try {
    const json = docOf(ed)
    link.seed((doc) => prosemirrorJSONToYXmlFragment(ed.schema, json, doc.getXmlFragment(FIELD)))
  } finally {
    ed.destroy()
  }
}

// docOf: the editor's document, made valid. An empty list item ("-", "1.",
// "- [ ]" with nothing after it, as templates leave for you to fill) reads
// as an item with no paragraph, which the schema doesn't allow (and a shared
// editor checks).
function docOf(ed: Editor): JSONContent {
  const json = ed.getJSON()
  const fix = (n: JSONContent) => {
    if ((n.type === 'listItem' || n.type === 'taskItem') && !n.content?.length) n.content = [{ type: 'paragraph' }]
    n.content?.forEach(fix)
  }
  fix(json)
  if (!json.content?.length) json.content = [{ type: 'paragraph', attrs: {} }]
  return json
}

// joinPage connects a tab to a page's shared document: from its log, or
// started from the Markdown (this tab, or another one that got there first).
export async function joinPage(link: CollabLink, markdown: string): Promise<void> {
  const s = await link.start()
  if (!s.fresh) return
  if (s.seed || !(await link.waitSeeded())) seedPage(link, markdown)
  await link.flush()
}
