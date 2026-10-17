import { markRaw, type Component } from 'vue'
import { Extension, type Editor, type Range } from '@tiptap/core'
import { PluginKey } from '@tiptap/pm/state'
import Suggestion from '@tiptap/suggestion'
import {
  Type, Heading1, Heading2, Heading3, ListTodo, List, ListOrdered, Quote, SquareCode, Minus, Table, MessageSquareQuote, Paperclip,
} from 'lucide-vue-next'

export interface SlashItem {
  key: string
  title: string
  desc: string
  icon: Component
  // Extra words to match (English names, Markdown-ish shorthands).
  aliases: string[]
  run: (editor: Editor, range: Range) => void
}

// The menu's state, shared with <SlashMenu> (the extension drives it, the
// component renders it inside the editor's Vue tree so it has i18n).
export interface SlashState {
  open: boolean
  items: SlashItem[]
  index: number
  rect: (() => DOMRect | null) | null
  pick: (item: SlashItem) => void
}

type T = (key: string) => string

// slashItems lists the blocks "/" can insert, titled in the current language.
export function slashItems(t: T): SlashItem[] {
  const b = (key: string) => ({ title: t(`editor.block.${key}.title`), desc: t(`editor.block.${key}.desc`) })
  const chain = (e: Editor, r: Range) => e.chain().focus().deleteRange(r)
  const list: SlashItem[] = [
    { key: 'text', ...b('text'), icon: Type, aliases: ['text', 'paragraph', 'p'], run: (e, r) => chain(e, r).setParagraph().run() },
    { key: 'h1', ...b('h1'), icon: Heading1, aliases: ['heading1', 'h1', 'title', '#'], run: (e, r) => chain(e, r).setHeading({ level: 1 }).run() },
    { key: 'h2', ...b('h2'), icon: Heading2, aliases: ['heading2', 'h2', 'subtitle', '##'], run: (e, r) => chain(e, r).setHeading({ level: 2 }).run() },
    { key: 'h3', ...b('h3'), icon: Heading3, aliases: ['heading3', 'h3', '###'], run: (e, r) => chain(e, r).setHeading({ level: 3 }).run() },
    { key: 'todo', ...b('todo'), icon: ListTodo, aliases: ['todo', 'task', 'checkbox', 'check', '[]'], run: (e, r) => chain(e, r).toggleTaskList().run() },
    { key: 'bullet', ...b('bullet'), icon: List, aliases: ['bulleted', 'bullet', 'unordered', 'ul', '-'], run: (e, r) => chain(e, r).toggleBulletList().run() },
    { key: 'numbered', ...b('numbered'), icon: ListOrdered, aliases: ['numbered', 'ordered', 'ol', '1.'], run: (e, r) => chain(e, r).toggleOrderedList().run() },
    { key: 'quote', ...b('quote'), icon: Quote, aliases: ['quote', 'blockquote', '>'], run: (e, r) => chain(e, r).toggleBlockquote().run() },
    { key: 'code', ...b('code'), icon: SquareCode, aliases: ['code', 'snippet', '```'], run: (e, r) => chain(e, r).toggleCodeBlock().run() },
    { key: 'divider', ...b('divider'), icon: Minus, aliases: ['divider', 'hr', 'line', 'separator', '---'], run: (e, r) => chain(e, r).setHorizontalRule().run() },
    { key: 'table', ...b('table'), icon: Table, aliases: ['table', 'grid'], run: (e, r) => chain(e, r).insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run() },
    { key: 'callout', ...b('callout'), icon: MessageSquareQuote, aliases: ['callout', 'note', 'info', 'tip'], run: (e, r) => chain(e, r).setCallout().run() },
    {
      key: 'file', ...b('file'), icon: Paperclip, aliases: ['image', 'picture', 'photo', 'file', 'attachment', 'upload'],
      run: (e, r) => {
        chain(e, r).run()
        ;(e.storage as any).fileUpload?.pick()
      },
    },
  ]
  // Icons are components: keep them out of the reactive menu state.
  return list.map((it) => ({ ...it, icon: markRaw(it.icon) }))
}

// Accent-insensitive lower-casing, so "titulo" finds "Título".
const fold = (s: string) => s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()

export function filterItems(items: SlashItem[], query: string): SlashItem[] {
  const q = fold(query.trim())
  if (!q) return items
  const starts: SlashItem[] = []
  const has: SlashItem[] = []
  for (const it of items) {
    const words = [it.title, ...it.aliases].map(fold)
    if (words.some((w) => w.startsWith(q))) starts.push(it)
    else if (words.some((w) => w.includes(q))) has.push(it)
  }
  return [...starts, ...has]
}

export const slashKey = new PluginKey('slash')

// SlashCommand opens the block menu on "/" (at the start of a line or after a
// space), outside code blocks.
export const SlashCommand = Extension.create<{ state: SlashState | null; items: (query: string) => SlashItem[] }>({
  name: 'slashCommand',

  addOptions() {
    return { state: null, items: () => [] }
  },

  addProseMirrorPlugins() {
    const state = this.options.state
    const items = this.options.items
    if (!state) return []
    const close = () => {
      state.open = false
      state.items = []
      state.rect = null
    }
    return [
      Suggestion<SlashItem, SlashItem>({
        pluginKey: slashKey,
        editor: this.editor,
        char: '/',
        allow: ({ state: s, range }) => !s.doc.resolve(range.from).parent.type.spec.code,
        items: ({ query }) => items(query),
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
