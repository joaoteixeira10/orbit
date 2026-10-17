import { Extension } from '@tiptap/core'
import { Plugin, PluginKey, type EditorState } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'
import type { Node as PMNode } from '@tiptap/pm/model'
import { evaluate, type MathLine } from '@/lib/mathnotes'

// Sums in the visual editor, as in Quick Notes: a paragraph (or list item,
// heading, cell) whose text ends with "=" shows its answer after it, in
// grey; Tab writes it in. Each text block is a line, top to bottom, so names
// defined above ("rent = 650") work below. Code blocks are left alone.

interface Block {
  end: number // the position at the end of its text
  text: string
}

function blocksOf(doc: PMNode): Block[] {
  const out: Block[] = []
  doc.descendants((node, pos) => {
    if (!node.isTextblock) return true
    if (node.type.name !== 'codeBlock') out.push({ end: pos + 1 + node.content.size, text: node.textContent.replace(/\n/g, ' ') })
    return false
  })
  return out
}

function answersOf(doc: PMNode): { blocks: Block[]; answers: MathLine[] } {
  const blocks = blocksOf(doc)
  return { blocks, answers: evaluate(blocks.map((b) => b.text).join('\n')) }
}

const key = new PluginKey<DecorationSet>('mathAnswers')

function decorate(doc: PMNode): DecorationSet {
  const { blocks, answers } = answersOf(doc)
  return DecorationSet.create(
    doc,
    answers.map((a) => {
      const b = blocks[a.line]
      return Decoration.widget(
        b.end,
        () => {
          const s = document.createElement('span')
          s.className = 'md-calc'
          s.contentEditable = 'false'
          s.textContent = (/\s$/.test(b.text) ? '' : ' ') + a.result
          return s
        },
        { side: 1, ignoreSelection: true, key: `${a.line}:${a.result}` },
      )
    }),
  )
}

// answerAtCaret: the answer of the block the caret is in, and where it goes.
function answerAtCaret(state: EditorState): { at: number; text: string } | null {
  const { $from, empty } = state.selection
  if (!empty || !$from.parent.isTextblock) return null
  const { blocks, answers } = answersOf(state.doc)
  const end = $from.end()
  const i = blocks.findIndex((b) => b.end === end)
  const a = answers.find((x) => x.line === i)
  if (i < 0 || !a) return null
  return { at: end, text: (/\s$/.test(blocks[i].text) ? '' : ' ') + a.result }
}

export const MathAnswers = Extension.create({
  name: 'mathAnswers',
  priority: 1000, // Tab here first, before lists indent
  addProseMirrorPlugins() {
    return [
      new Plugin<DecorationSet>({
        key,
        state: {
          init: (_, state) => decorate(state.doc),
          apply: (tr, old) => (tr.docChanged ? decorate(tr.doc) : old),
        },
        props: {
          decorations: (state) => key.getState(state),
        },
      }),
    ]
  },
  addKeyboardShortcuts() {
    return {
      Tab: ({ editor }) => {
        const a = answerAtCaret(editor.state)
        if (!a) return false
        // Plain text, as typed (insertContent would read it as HTML).
        return editor.commands.command(({ tr }) => {
          tr.insertText(a.text, a.at)
          return true
        })
      },
    }
  },
})
