// Sheets, diagrams and whiteboards inside a page: in its Markdown a link to
// one alone on its line ([Name](/w/{ws}/s/{id}) — d for a diagram,
// whiteboards for a whiteboard), shown in place. A double click opens it in
// the big editor over the page (EmbedEditor, one for the app); when that
// closes, the pictures of it in pages are drawn again.
import { reactive } from 'vue'
import { sheetsApi } from './sheetsApi'
import { diagramsApi } from './diagramsApi'
import { drawingsApi } from '@/components/draw/drawing'
import { MERMAID_TEMPLATES } from './mermaid'

export type EmbedKind = 'sheet' | 'diagram' | 'board'
// What "/" makes: a draw.io and a Mermaid diagram are both diagrams.
export type EmbedNew = 'sheet' | 'drawio' | 'mermaid' | 'board'

const SEG: Record<EmbedKind, string> = { sheet: 's', diagram: 'd', board: 'whiteboards' }
const KIND: Record<string, EmbedKind> = { s: 'sheet', d: 'diagram', whiteboards: 'board' }
// The module each kind needs in the workspace.
export const EMBED_MODULE: Record<EmbedKind, string> = { sheet: 'sheets', diagram: 'diagrams', board: 'whiteboards' }

export interface EmbedRef {
  kind: EmbedKind
  ws: string
  id: string
}

export const embedHref = (r: EmbedRef) => `/w/${r.ws}/${SEG[r.kind]}/${r.id}`

// embedOf reads a link to a sheet, a diagram or a whiteboard (null: it isn't one).
export function embedOf(href: string): EmbedRef | null {
  const m = /^\/w\/([^/?#\s]+)\/(s|d|whiteboards)\/([^/?#\s]+)$/.exec(href)
  return m ? { kind: KIND[m[2]], ws: m[1], id: m[3] } : null
}

// The big editor (one for the app) and how many times each one was edited
// in it (an embed redraws when its count changes).
export const embedEditor = reactive<{ open: boolean } & EmbedRef>({ open: false, kind: 'sheet', ws: '', id: '' })
export const embedEdits = reactive<Record<string, number>>({})

export function openEmbed(r: EmbedRef) {
  Object.assign(embedEditor, r, { open: true })
}
export function closeEmbed() {
  embedEdits[embedEditor.id] = (embedEdits[embedEditor.id] ?? 0) + 1
  embedEditor.open = false
}

// createEmbed makes a new one, named after the page it goes in.
export async function createEmbed(what: EmbedNew, ws: string, title: string): Promise<EmbedRef> {
  if (what === 'sheet') return { kind: 'sheet', ws, id: (await sheetsApi.create(ws, { title })).id }
  if (what === 'board') return { kind: 'board', ws, id: (await drawingsApi.make(ws, { title })).id }
  const data = what === 'mermaid' ? MERMAID_TEMPLATES[0].code : undefined
  return { kind: 'diagram', ws, id: (await diagramsApi.create(ws, { title, kind: what, data })).id }
}
