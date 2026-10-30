// Smaller page pieces, each written in Markdown the way GitHub shows it (or
// at least shows its text): collapsible sections (<details>), layouts of 2–5
// columns (<div data-columns>), a live list of the page's sub-pages
// (::pagetree) and coloured status badges (<span data-status>).
import { Node, mergeAttributes } from '@tiptap/core'
import { VueNodeViewRenderer } from '@tiptap/vue-3'
import PageTreeView from './PageTreeView.vue'
import StatusView from './StatusView.vue'
import { STATUS_COLORS } from './statusColors'

declare module '@tiptap/core' {
  interface Commands<ReturnType> {
    pageBlocks: {
      insertDetails: () => ReturnType
      insertColumns: (n: number) => ReturnType
      insertPageTree: () => ReturnType
      insertStatus: (attrs?: { text?: string; color?: string }) => ReturnType
    }
  }
}

// Each block's Markdown, rendered: a child at a time, a blank line between.
const blocksOf = (node: any, h: any) =>
  (node.content ?? []).map((child: any, i: number) => (h.renderChild?.(child, i) ?? h.renderChildren([child])) as string).join('\n\n')
const parseBlocks = (h: any, tokens: any[]) => (h.parseBlockChildren ?? h.parseChildren)(tokens)

// ---- Collapsible sections ----
// <details>
// <summary>Title</summary>
//
// body…
//
// </details>
const DETAILS = /^<details( open)?>[ \t]*\n<summary>(.*?)<\/summary>[ \t]*\n([\s\S]*?)\n<\/details>[ \t]*(?:\n|$)/

export const DetailsSummary = Node.create({
  name: 'detailsSummary',
  content: 'inline*',
  defining: true,
  parseHTML: () => [{ tag: 'div[data-type="details-summary"]' }, { tag: 'summary' }],
  renderHTML: ({ HTMLAttributes }) => ['div', mergeAttributes(HTMLAttributes, { 'data-type': 'details-summary', class: 'details-summary' }), 0],
})

export const DetailsContent = Node.create({
  name: 'detailsContent',
  content: 'block+',
  defining: true,
  parseHTML: () => [{ tag: 'div[data-type="details-content"]' }],
  renderHTML: ({ HTMLAttributes }) => ['div', mergeAttributes(HTMLAttributes, { 'data-type': 'details-content', class: 'details-content' }), 0],
})

export const Details = Node.create({
  name: 'details',
  group: 'block',
  content: 'detailsSummary detailsContent',
  defining: true,
  addAttributes: () => ({ open: { default: false, parseHTML: (el) => el.hasAttribute('open') || el.getAttribute('data-open') === 'true', renderHTML: (a) => ({ 'data-open': String(!!a.open) }) } }),
  // In the editor it's always open (to write in); reading, it folds.
  parseHTML: () => [{ tag: 'div[data-type="details"]' }, { tag: 'details' }],
  renderHTML: ({ HTMLAttributes }) => ['div', mergeAttributes(HTMLAttributes, { 'data-type': 'details', class: 'details-block' }), 0],
  addCommands() {
    return {
      insertDetails:
        () =>
        ({ commands }: any) =>
          commands.insertContent({ type: 'details', content: [{ type: 'detailsSummary' }, { type: 'detailsContent', content: [{ type: 'paragraph' }] }] }),
    } as any
  },
  markdownTokenizer: {
    name: 'details',
    level: 'block',
    start: (src: string) => src.search(/^<details[ >]/m),
    tokenize(src: string, _t: unknown, lexer: any) {
      const m = DETAILS.exec(src)
      if (!m) return undefined
      return { type: 'details', raw: m[0], open: !!m[1], summary: lexer.inlineTokens(m[2].trim()), tokens: lexer.blockTokens(m[3].trim() || '') }
    },
  },
  parseMarkdown: (token: any, h: any) => {
    const body = parseBlocks(h, token.tokens || [])
    return h.createNode('details', { open: token.open }, [
      h.createNode('detailsSummary', null, h.parseInline(token.summary || [])),
      h.createNode('detailsContent', null, body?.length ? body : [h.createNode('paragraph')]),
    ])
  },
  renderMarkdown: (node: any, h: any) => {
    const [summary, content] = node.content ?? []
    const title = summary ? (h.renderChildren(summary.content ?? []) as string).replace(/\n/g, ' ') : ''
    const body = content ? blocksOf(content, h) : ''
    return `<details${node.attrs?.open ? ' open' : ''}>\n<summary>${title}</summary>\n\n${body}\n\n</details>`
  },
})

// ---- Columns ----
// <div data-columns="2">
// <div data-column>
//
// first column…
//
// </div>
// <div data-column>
//
// second…
//
// </div>
// </div>

// readColumns finds a columns block at the start of src: its raw text and
// each column's Markdown (null: not one).
export function readColumns(src: string): { raw: string; n: number; bodies: string[] } | null {
  const head = /^<div data-columns="([2-5])">[ \t]*\n/.exec(src)
  if (!head) return null
  const lines = src.slice(head[0].length).split('\n')
  const bodies: string[] = []
  let used = head[0].length
  let i = 0
  for (;;) {
    if (i >= lines.length) return null
    const line = lines[i].trim()
    used += lines[i].length + 1
    i++
    if (line === '') continue
    if (line === '</div>') break // the block's end
    if (line !== '<div data-column>') return null
    const body: string[] = []
    let depth = 0
    for (;;) {
      if (i >= lines.length) return null
      const l = lines[i]
      used += l.length + 1
      i++
      if (/^\s*<div\b/.test(l)) depth++
      if (l.trim() === '</div>') {
        if (depth === 0) break
        depth--
      }
      body.push(l)
    }
    bodies.push(body.join('\n').trim())
  }
  if (bodies.length < 2) return null
  return { raw: src.slice(0, Math.min(used, src.length)), n: bodies.length, bodies }
}

export const Column = Node.create({
  name: 'column',
  content: 'block+',
  isolating: true,
  parseHTML: () => [{ tag: 'div[data-column]' }],
  renderHTML: ({ HTMLAttributes }) => ['div', mergeAttributes(HTMLAttributes, { 'data-column': '', class: 'column' }), 0],
})

export const Columns = Node.create({
  name: 'columns',
  group: 'block',
  content: 'column{2,5}',
  defining: true,
  parseHTML: () => [{ tag: 'div[data-columns]' }],
  renderHTML: ({ node, HTMLAttributes }) => ['div', mergeAttributes(HTMLAttributes, { 'data-columns': String(node.childCount), class: 'columns-block' }), 0],
  addCommands() {
    return {
      insertColumns:
        (n: number) =>
        ({ commands }: any) =>
          commands.insertContent({ type: 'columns', content: Array.from({ length: Math.min(5, Math.max(2, n)) }, () => ({ type: 'column', content: [{ type: 'paragraph' }] })) }),
    } as any
  },
  markdownTokenizer: {
    name: 'columns',
    level: 'block',
    start: (src: string) => src.search(/^<div data-columns="/m),
    tokenize(src: string, _t: unknown, lexer: any) {
      const c = readColumns(src)
      if (!c) return undefined
      return { type: 'columns', raw: c.raw, columns: c.bodies.map((b) => lexer.blockTokens(b)) }
    },
  },
  parseMarkdown: (token: any, h: any) =>
    h.createNode(
      'columns',
      null,
      (token.columns as any[][]).map((toks) => {
        const body = parseBlocks(h, toks)
        return h.createNode('column', null, body?.length ? body : [h.createNode('paragraph')])
      }),
    ),
  renderMarkdown: (node: any, h: any) => {
    const cols = (node.content ?? []).map((col: any) => `<div data-column>\n\n${blocksOf(col, h)}\n\n</div>`)
    return `<div data-columns="${cols.length}">\n${cols.join('\n')}\n</div>`
  },
})

// ---- The page's sub-pages, as a live list ----
export const PageTree = Node.create({
  name: 'pageTree',
  group: 'block',
  atom: true,
  selectable: true,
  draggable: true,
  parseHTML: () => [{ tag: 'div[data-type="page-tree"]' }],
  renderHTML: ({ HTMLAttributes }) => ['div', mergeAttributes(HTMLAttributes, { 'data-type': 'page-tree' })],
  addNodeView: () => VueNodeViewRenderer(PageTreeView),
  addCommands() {
    return {
      insertPageTree:
        () =>
        ({ commands }: any) =>
          commands.insertContent([{ type: 'pageTree' }, { type: 'paragraph' }]),
    } as any
  },
  markdownTokenizer: {
    name: 'pageTree',
    level: 'block',
    start: (src: string) => src.search(/^::pagetree[ \t]*$/m),
    tokenize(src: string) {
      const m = /^::pagetree[ \t]*(?:\n|$)/.exec(src)
      return m ? { type: 'pageTree', raw: m[0] } : undefined
    },
  },
  parseMarkdown: (_token: any, h: any) => h.createNode('pageTree'),
  renderMarkdown: () => '::pagetree',
})

// ---- Status badges ----
export { STATUS_COLORS }
const STATUS = /^<span data-status="(gray|red|orange|yellow|green|blue|purple)">([^<>\n]{1,40})<\/span>/

export const Status = Node.create({
  name: 'status',
  group: 'inline',
  inline: true,
  atom: true,
  selectable: true,
  addAttributes: () => ({
    text: { default: 'Status', parseHTML: (el) => el.textContent ?? '' },
    color: { default: 'gray', parseHTML: (el) => el.getAttribute('data-status') || 'gray' },
  }),
  parseHTML: () => [{ tag: 'span[data-status]' }],
  renderHTML: ({ node }) => ['span', { 'data-status': node.attrs.color, class: 'status-badge' }, node.attrs.text],
  addNodeView: () => VueNodeViewRenderer(StatusView),
  addCommands() {
    return {
      insertStatus:
        (attrs: { text?: string; color?: string } = {}) =>
        ({ commands }: any) =>
          commands.insertContent([{ type: 'status', attrs: { text: attrs.text ?? 'To do', color: attrs.color ?? 'gray' } }, { type: 'text', text: ' ' }]),
    } as any
  },
  markdownTokenizer: {
    name: 'status',
    level: 'inline',
    start: (src: string) => src.indexOf('<span data-status='),
    tokenize(src: string) {
      const m = STATUS.exec(src)
      return m ? { type: 'status', raw: m[0], color: m[1], text: m[2] } : undefined
    },
  },
  parseMarkdown: (token: any, h: any) => h.createNode('status', { text: token.text, color: token.color }),
  renderMarkdown: (node: any) => {
    const text = String(node.attrs?.text ?? '').replace(/[<>\n]/g, '').slice(0, 40) || 'Status'
    return `<span data-status="${node.attrs?.color ?? 'gray'}">${text}</span>`
  },
})

export const pageBlocks = [DetailsSummary, DetailsContent, Details, Column, Columns, PageTree, Status]
