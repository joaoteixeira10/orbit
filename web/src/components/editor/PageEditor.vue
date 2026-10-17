<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useEditor, EditorContent } from '@tiptap/vue-3'
import type { Editor } from '@tiptap/core'
import { TextSelection } from '@tiptap/pm/state'
import type { Node as PMNode } from '@tiptap/pm/model'
import { Placeholder } from '@tiptap/extensions'
import { pageExtensions } from './extensions'
import { normalizeMarkdown } from '@/lib/markdown'
import { SlashCommand, slashItems, filterItems, type SlashState } from './slash'
import { FileUpload } from './upload'
import { downloadUpload, isUploadSrc, uploadFile } from '@/lib/uploads'
import { useToastStore } from '@/stores/toast'
import SlashMenu from './SlashMenu.vue'
import BubbleToolbar from './BubbleToolbar.vue'
import './editor.css'
import { MathAnswers } from './math'

// The page body, edited visually: a TipTap editor that reads and writes the
// page's Markdown (extensions.ts). The parent owns saving; this reports
// changes and hands out the Markdown on request ("" = empty page).
const props = defineProps<{ content: string; editable: boolean; pageId?: string }>()
const toast = useToastStore()
const emit = defineEmits<{ change: []; invalid: [] }>()
const { t } = useI18n()

const root = ref<HTMLElement | null>(null)
const bubble = ref<InstanceType<typeof BubbleToolbar> | null>(null)
const invalid = ref(false)
const slash = reactive<SlashState>({ open: false, items: [], index: 0, rect: null, pick: () => {} })

// valid reports whether the document the Markdown became fits the schema;
// one that doesn't must never be saved over.
function valid(ed: Editor): boolean {
  try {
    ed.state.doc.check()
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

const editor = useEditor({
  content: props.content,
  contentType: 'markdown',
  editable: props.editable,
  extensions: pageExtensions(
    [
      Placeholder.configure({ placeholder, includeChildren: true }),
      MathAnswers,
      SlashCommand.configure({ state: slash, items: (q: string) => filterItems(slashItems(t), q) }),
      FileUpload.configure({
        upload: props.pageId ? (f: File) => (toast.info(t('editor.uploading', { name: f.name })), uploadFile('page', props.pageId!, f)) : null,
        onError: (e: any) => toast.error(e?.message ?? String(e)),
      }),
    ],
    // ⌘/Ctrl-click opens a link (handleClick below); a plain click edits.
    { HTMLAttributes: { rel: 'noopener noreferrer nofollow', target: '_blank' } },
  ),
  editorProps: {
    attributes: { class: 'orbit-prose', spellcheck: 'true' },
    handleClick: (_view, _pos, event) => {
      if (!(event.metaKey || event.ctrlKey)) return false
      const a = (event.target as HTMLElement | null)?.closest('a')
      if (!a?.href) return false
      if (isUploadSrc(a.href)) void downloadUpload(a.href, a.textContent || 'file') // needs the sign-in token
      else window.open(a.href, '_blank', 'noopener,noreferrer')
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
    if (!valid(ed)) markInvalid(ed)
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

// getContent is the page's Markdown ("" when it's blank).
function getContent(): string {
  const ed = editor.value
  if (!ed) return props.content
  return blank(ed) ? '' : normalizeMarkdown(ed.getMarkdown())
}

// setContent replaces the document (someone else's save) without an update
// event or an undo step, keeping the caret near where it was.
function setContent(content: string) {
  const ed = editor.value
  if (!ed) return
  const { from, to } = ed.state.selection
  ed.chain()
    .setMeta('addToHistory', false)
    .setContent(content, { emitUpdate: false, contentType: 'markdown' })
    .command(({ tr }) => {
      const max = tr.doc.content.size
      tr.setSelection(TextSelection.between(tr.doc.resolve(Math.min(from, max)), tr.doc.resolve(Math.min(to, max))))
      return true
    })
    .run()
  if (!valid(ed)) markInvalid(ed)
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
