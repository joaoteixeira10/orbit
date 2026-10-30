import { Node, mergeAttributes } from '@tiptap/core'
import { VueNodeViewRenderer } from '@tiptap/vue-3'
import EmbedNodeView from './EmbedNodeView.vue'
import { embedHref, embedOf, type EmbedRef } from '@/lib/embeds'

declare module '@tiptap/core' {
  interface Commands<ReturnType> {
    docEmbed: {
      // Puts a sheet, a diagram or a whiteboard in the page at pos (the
      // caret when not given): in place of an empty line, else after its block.
      insertEmbed: (attrs: EmbedRef & { title: string }, pos?: number) => ReturnType
    }
  }
}

// A link to a sheet, a diagram or a whiteboard alone on its line, after a
// blank line (or at the start): what the reader sees there (PageEmbed), as a
// block. In Markdown it stays the link — [Name](/w/{ws}/s/{id}).
const LINE = /^\[((?:\\.|[^\]\\\n])*)\]\((\/w\/[^/\s)]+\/(?:s|d|whiteboards)\/[^/\s)?#]+)\)[ \t]*(?=\n[ \t]*\n|\n?$)\n?/
export const unescapeTitle = (s: string) => s.replace(/\\([\\[\]])/g, '$1')
export const escapeTitle = (s: string) => s.replace(/[\\[\]]/g, '\\$&').replace(/\n/g, ' ')

export const DocEmbed = Node.create({
  name: 'docEmbed',
  group: 'block',
  atom: true,
  selectable: true,
  draggable: true,

  addAttributes() {
    const attr = (name: string) => ({
      default: '',
      parseHTML: (el: HTMLElement) => el.getAttribute(`data-${name}`) ?? '',
      renderHTML: (a: Record<string, string>) => ({ [`data-${name}`]: a[name] }),
    })
    return { kind: attr('kind'), ws: attr('ws'), id: attr('id'), title: attr('title') }
  },
  parseHTML: () => [{ tag: 'div[data-type="embed"]' }],
  renderHTML: ({ HTMLAttributes }) => ['div', mergeAttributes(HTMLAttributes, { 'data-type': 'embed' })],
  addNodeView: () => VueNodeViewRenderer(EmbedNodeView),

  addCommands() {
    return {
      insertEmbed:
        (attrs, pos) =>
        ({ state, commands }) => {
          const $at = state.doc.resolve(Math.min(pos ?? state.selection.from, state.doc.content.size))
          const block = $at.depth ? $at.before(1) : 0
          const top = $at.depth ? $at.node(1) : null
          const content = [{ type: this.name, attrs }, { type: 'paragraph' }]
          if (top?.type.name === 'paragraph' && top.content.size === 0) return commands.insertContentAt({ from: block, to: block + top.nodeSize }, content)
          return commands.insertContentAt(top ? block + top.nodeSize : $at.pos, content)
        },
    }
  },

  markdownTokenizer: {
    name: 'docEmbed',
    level: 'block',
    start: (src: string) => {
      const m = /(^|\n[ \t]*\n)\[[^\]\n]*\]\(\/w\//.exec(src)
      return m ? m.index + m[1].length : -1
    },
    tokenize(src: string) {
      const m = LINE.exec(src)
      const ref = m && embedOf(m[2])
      return ref ? { type: 'docEmbed', raw: m[0], ...ref, title: unescapeTitle(m[1]) } : undefined
    },
  },
  parseMarkdown: (token: any, h: any) => h.createNode('docEmbed', { kind: token.kind, ws: token.ws, id: token.id, title: token.title }),
  renderMarkdown: (node: any) => {
    const a = node.attrs ?? {}
    return `[${escapeTitle(a.title || a.id)}](${embedHref(a)})`
  },
})
