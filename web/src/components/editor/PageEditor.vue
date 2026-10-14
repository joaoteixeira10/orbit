<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useEditor, EditorContent } from '@tiptap/vue-3'
import type { Editor, JSONContent } from '@tiptap/core'
import { TextSelection } from '@tiptap/pm/state'
import type { Node as PMNode } from '@tiptap/pm/model'
import StarterKit from '@tiptap/starter-kit'
import { TaskList, TaskItem } from '@tiptap/extension-list'
import { Placeholder } from '@tiptap/extensions'
import { TableKit } from '@tiptap/extension-table'
import { Highlight } from '@tiptap/extension-highlight'
import { Callout } from './callout'
import { SlashCommand, slashItems, filterItems, type SlashState } from './slash'
import SlashMenu from './SlashMenu.vue'
import BubbleToolbar from './BubbleToolbar.vue'
import './editor.css'

// The page body: a TipTap editor over ProseMirror JSON. The parent owns
// saving; this reports changes and hands out the document on request.
// Content is a JSON string ("" = empty document).
const props = defineProps<{ content: string; editable: boolean }>()
const emit = defineEmits<{ change: []; invalid: [] }>()
const { t } = useI18n()

const root = ref<HTMLElement | null>(null)
const bubble = ref<InstanceType<typeof BubbleToolbar> | null>(null)
const invalid = ref(false)
const slash = reactive<SlashState>({ open: false, items: [], index: 0, rect: null, pick: () => {} })

function parse(content: string): JSONContent | null {
  if (!content) return null
  try {
    const doc = JSON.parse(content) as JSONContent
    return doc?.type === 'doc' ? doc : null
  } catch {
    return null
  }
}

// valid reports whether this editor's schema can represent the document; one
// it can't (say, a block from a newer version) must never be saved over.
function valid(ed: Editor, doc: JSONContent | null, raw: string): boolean {
  if (!raw) return true
  if (!doc) return false
  try {
    ed.schema.nodeFromJSON(doc).check()
    return true
  } catch {
    return false
  }
}
function markInvalid(ed: Editor) {
  invalid.value = true
  ed.setEditable(false, false)
  emit('invalid')
}

function placeholder({ editor, node, pos }: { editor: Editor; node: PMNode; pos: number }): string {
  if (node.type.name === 'heading') return t('editor.placeholderHeading', { level: node.attrs.level })
  if (blank(editor)) return t('editor.placeholderEmpty')
  if (node.type.name !== 'paragraph') return ''
  const parent = editor.state.doc.resolve(pos).parent.type.name
  if (parent === 'taskItem') return t('editor.placeholderTodo')
  if (parent === 'listItem') return t('editor.placeholderList')
  if (parent === 'blockquote') return t('editor.placeholderQuote')
  if (parent === 'tableCell' || parent === 'tableHeader') return ''
  return t('editor.placeholder')
}

const initial = parse(props.content)

const editor = useEditor({
  content: initial,
  editable: props.editable,
  extensions: [
    StarterKit.configure({
      heading: { levels: [1, 2, 3] },
      dropcursor: { color: 'var(--ws)', width: 2 },
      link: {
        openOnClick: false, // ⌘/Ctrl-click opens (handleClick below); a plain click edits
        autolink: true,
        linkOnPaste: true,
        defaultProtocol: 'https',
        HTMLAttributes: { rel: 'noopener noreferrer nofollow', target: '_blank' },
      },
    }),
    TaskList,
    TaskItem.configure({ nested: true }),
    TableKit.configure({ table: { resizable: false, renderWrapper: true } }),
    Highlight,
    Callout,
    Placeholder.configure({ placeholder, includeChildren: true }),
    SlashCommand.configure({ state: slash, items: (q: string) => filterItems(slashItems(t), q) }),
  ],
  editorProps: {
    attributes: { class: 'orbit-prose', spellcheck: 'true' },
    handleClick: (_view, _pos, event) => {
      if (!(event.metaKey || event.ctrlKey)) return false
      const a = (event.target as HTMLElement | null)?.closest('a')
      if (!a?.href) return false
      window.open(a.href, '_blank', 'noopener,noreferrer')
      return true
    },
    handleKeyDown: (view, event) => {
      // ⌘K / Ctrl+K: link the selected text.
      if ((event.metaKey || event.ctrlKey) && !event.shiftKey && event.key.toLowerCase() === 'k' && !view.state.selection.empty) {
        event.preventDefault()
        void bubble.value?.openLink()
        return true
      }
      return false
    },
  },
  onCreate: ({ editor: ed }) => {
    if (!valid(ed, initial, props.content)) markInvalid(ed)
  },
  onUpdate: () => {
    if (!invalid.value) emit('change')
  },
})

watch(
  () => props.editable,
  (on) => editor.value?.setEditable(on && !invalid.value, false),
)

// blank: only one empty paragraph. (editor.isEmpty also counts an empty
// table or callout as empty, which must not save as "".)
function blank(ed: Editor): boolean {
  const { doc } = ed.state
  const first = doc.firstChild
  return doc.childCount === 1 && !!first && first.type.name === 'paragraph' && first.content.size === 0
}

// getContent serialises the document ("" when it's blank).
function getContent(): string {
  const ed = editor.value
  if (!ed) return props.content
  return blank(ed) ? '' : JSON.stringify(ed.getJSON())
}

// setContent replaces the document (someone else's save) without an update
// event or an undo step, keeping the caret near where it was.
function setContent(content: string) {
  const ed = editor.value
  if (!ed) return
  const doc = parse(content)
  if (!valid(ed, doc, content)) return markInvalid(ed)
  const { from, to } = ed.state.selection
  ed.chain()
    .setMeta('addToHistory', false)
    .setContent(doc ?? '', { emitUpdate: false })
    .command(({ tr }) => {
      const max = tr.doc.content.size
      tr.setSelection(TextSelection.between(tr.doc.resolve(Math.min(from, max)), tr.doc.resolve(Math.min(to, max))))
      return true
    })
    .run()
}

function focusStart() {
  const ed = editor.value
  if (!ed) return
  // The command sets the selection now but focuses on the next frame; focus
  // the view at once too, so keys typed right after Enter land in the page.
  ed.commands.focus('start')
  ed.view.focus()
}

defineExpose({ getContent, setContent, focusStart })
</script>

<template>
  <div ref="root" class="page-editor" :class="{ 'is-readonly': !editable || invalid }">
    <EditorContent :editor="editor" />
    <BubbleToolbar v-if="editor" ref="bubble" :editor="editor" />
    <SlashMenu :state="slash" :context="root" @hover="(i) => (slash.index = i)" />
  </div>
</template>
