import { Node, mergeAttributes } from '@tiptap/core'
import { VueNodeViewRenderer } from '@tiptap/vue-3'
import CalloutView from './CalloutView.vue'

declare module '@tiptap/core' {
  interface Commands<ReturnType> {
    callout: {
      // Wraps the current block(s) in a callout.
      setCallout: (attrs?: { emoji?: string }) => ReturnType
    }
  }
}

// A callout: an emoji next to a tinted, rounded box of blocks. Stored as
// {"type":"callout","attrs":{"emoji":"💡"},"content":[…blocks]}; the server
// only needs it to be part of a "doc" and walks its text for search.
export const Callout = Node.create({
  name: 'callout',
  group: 'block',
  content: 'block+',
  defining: true,

  addAttributes() {
    return {
      emoji: {
        default: '💡',
        parseHTML: (el) => el.getAttribute('data-emoji') || '💡',
        renderHTML: (attrs) => ({ 'data-emoji': attrs.emoji }),
      },
    }
  },

  parseHTML() {
    return [{ tag: 'div[data-type="callout"]' }]
  },

  renderHTML({ HTMLAttributes }) {
    return ['div', mergeAttributes(HTMLAttributes, { 'data-type': 'callout', class: 'callout' }), 0]
  },

  addNodeView() {
    return VueNodeViewRenderer(CalloutView)
  },

  addCommands() {
    return {
      setCallout:
        (attrs) =>
        ({ commands }) =>
          commands.wrapIn(this.name, attrs),
    }
  },

  addKeyboardShortcuts() {
    return {
      // Backspace in an empty callout's only block removes the callout.
      Backspace: ({ editor }) => {
        const { $from, empty } = editor.state.selection
        if (!empty || $from.parentOffset !== 0) return false
        const callout = $from.node(-1)
        if (callout?.type.name !== this.name || callout.childCount !== 1 || $from.parent.content.size) return false
        return editor.commands.lift(this.name)
      },
    }
  },
})
