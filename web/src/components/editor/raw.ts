// Markdown the visual editor doesn't edit, kept exactly as written: HTML
// blocks (<details>…), footnote definitions ([^1]: …), and inline HTML it has
// no mark for (<kbd>, <sup>…) or footnote references ([^1]). The editor shows
// them as their source, greyed; whoever edits in Markdown changes them.
import { Node, mergeAttributes } from '@tiptap/core'

const rawAttr = {
  raw: {
    default: '',
    parseHTML: (el: HTMLElement) => el.getAttribute('data-raw') ?? el.textContent ?? '',
    renderHTML: (a: { raw: string }) => ({ 'data-raw': a.raw }),
  },
}

// An HTML block (a marked "html" token at block level).
export const RawBlock = Node.create({
  name: 'rawBlock',
  group: 'block',
  atom: true,
  selectable: true,
  addAttributes: () => rawAttr,
  parseHTML: () => [{ tag: 'pre[data-type="raw"]' }],
  renderHTML: ({ node, HTMLAttributes }) => ['pre', mergeAttributes(HTMLAttributes, { 'data-type': 'raw', class: 'raw-block' }), node.attrs.raw],
  markdownTokenName: 'html',
  // Inline HTML goes through the marks (mark, u) or RawInline instead.
  parseMarkdown: (token: any, h: any) => (token.block ? h.createNode('rawBlock', { raw: String(token.raw ?? '').replace(/\n+$/, '') }) : null),
  renderMarkdown: (node: any) => node.attrs?.raw ?? '',
})

// [^id]: text — a footnote definition (and its indented continuation lines).
const FOOTNOTE_DEF = /^\[\^[^\]\s]+\]:[^\n]*(?:\n(?: {2,}|\t)[^\n]*)*/
export const FootnoteDef = Node.create({
  name: 'footnoteDef',
  group: 'block',
  atom: true,
  selectable: true,
  addAttributes: () => rawAttr,
  parseHTML: () => [{ tag: 'pre[data-type="footnote"]' }],
  renderHTML: ({ node, HTMLAttributes }) => ['pre', mergeAttributes(HTMLAttributes, { 'data-type': 'footnote', class: 'raw-block' }), node.attrs.raw],
  markdownTokenizer: {
    name: 'footnoteDef',
    level: 'block',
    start: (src: string) => src.search(/^\[\^/m),
    tokenize(src: string) {
      const m = FOOTNOTE_DEF.exec(src)
      return m ? { type: 'footnoteDef', raw: m[0] } : undefined
    },
  },
  parseMarkdown: (token: any, h: any) => h.createNode('footnoteDef', { raw: token.raw }),
  renderMarkdown: (node: any) => node.attrs?.raw ?? '',
})

// The inline HTML elements kept as they are (span is left out: pasted web
// content is full of them).
const INLINE_TAGS = ['kbd', 'sup', 'sub', 'abbr', 'small', 'samp', 'var', 'cite', 'q', 'time', 'ins']

export const RawInline = Node.create({
  name: 'rawInline',
  group: 'inline',
  inline: true,
  atom: true,
  selectable: true,
  addAttributes: () => ({
    raw: {
      default: '',
      parseHTML: (el: HTMLElement) => el.getAttribute('data-raw') ?? el.outerHTML,
      renderHTML: (a: { raw: string }) => ({ 'data-raw': a.raw }),
    },
  }),
  parseHTML: () => [{ tag: 'code[data-type="raw"]' }, ...INLINE_TAGS.map((tag) => ({ tag, priority: 60 }))],
  renderHTML: ({ node, HTMLAttributes }) => ['code', mergeAttributes(HTMLAttributes, { 'data-type': 'raw', class: 'raw-inline' }), node.attrs.raw],
  // [^id] — a footnote reference.
  markdownTokenizer: {
    name: 'rawInline',
    level: 'inline',
    start: (src: string) => src.indexOf('[^'),
    tokenize(src: string) {
      const m = /^\[\^[^\]\s]+\](?!:)/.exec(src)
      return m ? { type: 'rawInline', raw: m[0] } : undefined
    },
  },
  parseMarkdown: (token: any, h: any) => h.createNode('rawInline', { raw: token.raw }),
  renderMarkdown: (node: any) => node.attrs?.raw ?? '',
})
