import { markRaw } from 'vue'
import { Extension } from '@tiptap/core'
import { Plugin, PluginKey } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'
import type { Node as PMNode } from '@tiptap/pm/model'
import Suggestion from '@tiptap/suggestion'
import { AtSign, CalendarDays, FileText } from 'lucide-vue-next'
import { MENTION, isDate, type MentionItem } from '@/lib/mentions'
import type { SlashItem, SlashState } from './slash'

export const mentionKey = new PluginKey('mention')
const chipsKey = new PluginKey('mentionChips')

const ICONS = { person: markRaw(AtSign), page: markRaw(FileText), date: markRaw(CalendarDays) }

// asSlashItem lets the "/" menu's list show a mention (same look and keys).
function asSlashItem(it: MentionItem): SlashItem {
  return {
    key: it.key, title: it.title, desc: it.desc, icon: ICONS[it.kind], aliases: [],
    run: (e, r) => {
      const c = e.chain().focus().deleteRange(r)
      if (it.href) c.insertContent([{ type: 'text', text: it.text, marks: [{ type: 'link', attrs: { href: it.href } }] }, { type: 'text', text: ' ' }])
      else c.insertContent(it.text + ' ')
      c.run()
    },
  }
}

// chips marks @username and @YYYY-MM-DD in the text (outside code and
// links), so they look like mentions while the Markdown stays plain.
function chips(doc: PMNode): DecorationSet {
  const decos: Decoration[] = []
  doc.descendants((node, pos, parent) => {
    if (!node.isText || !node.text?.includes('@') || parent?.type.spec.code || node.marks.some((m) => m.type.name === 'link' || m.type.name === 'code')) return
    for (const m of node.text.matchAll(MENTION)) {
      const from = pos + m.index! + m[1].length
      const word = m[2]
      if (/^\d{4}-/.test(word) && !isDate(word)) continue
      decos.push(Decoration.inline(from, from + 1 + word.length, { class: /^\d{4}-/.test(word) ? 'md-date' : 'md-mention' }))
    }
  })
  return DecorationSet.create(doc, decos)
}

// MentionCommand offers people, pages and dates on "@" (shown by a
// <SlashMenu> with its own state).
export const MentionCommand = Extension.create<{ state: SlashState | null; items: (query: string) => MentionItem[] }>({
  name: 'mentionCommand',

  addOptions() {
    return { state: null, items: () => [] }
  },

  addProseMirrorPlugins() {
    const state = this.options.state
    const items = this.options.items
    const decorate = new Plugin({
      key: chipsKey,
      state: {
        init: (_, s) => chips(s.doc),
        apply: (tr, old) => (tr.docChanged ? chips(tr.doc) : old),
      },
      props: { decorations: (s) => chipsKey.getState(s) },
    })
    if (!state) return [decorate]
    const close = () => {
      state.open = false
      state.items = []
      state.rect = null
    }
    return [
      decorate,
      Suggestion<SlashItem, SlashItem>({
        pluginKey: mentionKey,
        editor: this.editor,
        char: '@',
        // Not in code, and not inside a word (an e-mail address).
        allowedPrefixes: [' ', '(', '[', '"', '“'],
        allow: ({ state: s, range }) => !s.doc.resolve(range.from).parent.type.spec.code,
        items: ({ query }) => items(query).map(asSlashItem),
        command: ({ editor, range, props }) => props.run(editor, range),
        render: () => ({
          onStart: (p) => {
            state.items = p.items
            state.index = 0
            state.rect = p.clientRect ?? null
            state.pick = (it) => p.command(it)
            state.open = true
          },
          onUpdate: (p) => {
            state.index = 0
            state.items = p.items
            state.rect = p.clientRect ?? null
            state.pick = (it) => p.command(it)
          },
          onExit: close,
          onKeyDown: ({ event }) => {
            if (!state.open) return false
            const n = state.items.length
            if (event.key === 'ArrowDown') {
              if (n) state.index = (state.index + 1) % n
              return true
            }
            if (event.key === 'ArrowUp') {
              if (n) state.index = (state.index - 1 + n) % n
              return true
            }
            if (event.key === 'Enter' || event.key === 'Tab') {
              const it = state.items[state.index]
              if (!it) return false
              state.pick(it)
              return true
            }
            if (event.key === 'Escape') {
              close()
              return true
            }
            return false
          },
        }),
      }),
    ]
  },
})
