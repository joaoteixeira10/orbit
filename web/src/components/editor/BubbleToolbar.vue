<script setup lang="ts">
import { ref, computed, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Editor } from '@tiptap/vue-3'
import { BubbleMenu } from '@tiptap/vue-3/menus'
import type { Editor as CoreEditor } from '@tiptap/core'
import { NodeSelection, type EditorState } from '@tiptap/pm/state'
import type { EditorView } from '@tiptap/pm/view'
import { Bold, Italic, Underline, Strikethrough, Code, Highlighter, Link2, ChevronDown, Check, Unlink, ExternalLink } from 'lucide-vue-next'

// The formatting toolbar over a text selection: turn into (text / headings),
// marks, and a link field.
const props = defineProps<{ editor: Editor }>()
const { t } = useI18n()

const linking = ref(false)
const href = ref('')
const turning = ref(false)
const input = ref<HTMLInputElement | null>(null)

const blocks = computed(() => [
  { key: 'text', label: t('editor.block.text.title'), on: () => props.editor.isActive('paragraph'), run: () => props.editor.chain().focus().setParagraph().run() },
  ...([1, 2, 3] as const).map((level) => ({
    key: `h${level}`,
    label: t(`editor.block.h${level}.title`),
    on: () => props.editor.isActive('heading', { level }),
    run: () => props.editor.chain().focus().setHeading({ level }).run(),
  })),
])
const current = computed(() => blocks.value.find((b) => b.on())?.label ?? t('editor.tb.turnInto'))

const marks = computed(() => [
  { key: 'bold', icon: Bold, label: `${t('editor.tb.bold')} (⌘B)`, run: () => props.editor.chain().focus().toggleBold().run() },
  { key: 'italic', icon: Italic, label: `${t('editor.tb.italic')} (⌘I)`, run: () => props.editor.chain().focus().toggleItalic().run() },
  { key: 'underline', icon: Underline, label: `${t('editor.tb.underline')} (⌘U)`, run: () => props.editor.chain().focus().toggleUnderline().run() },
  { key: 'strike', icon: Strikethrough, label: `${t('editor.tb.strike')} (⌘⇧S)`, run: () => props.editor.chain().focus().toggleStrike().run() },
  { key: 'code', icon: Code, label: `${t('editor.tb.code')} (⌘E)`, run: () => props.editor.chain().focus().toggleCode().run() },
  { key: 'highlight', icon: Highlighter, label: `${t('editor.tb.highlight')} (⌘⇧H)`, run: () => props.editor.chain().focus().toggleHighlight().run() },
])

// Text selections only: not in code blocks, not a selected node (divider…).
function shouldShow({ editor, state, from, to, element, view }: { editor: CoreEditor; state: EditorState; from: number; to: number; element: HTMLElement; view: EditorView }) {
  if (!editor.isEditable) return false
  if (element.contains(document.activeElement)) return true
  if (!view.hasFocus() || state.selection.empty || state.selection instanceof NodeSelection) return false
  if (editor.isActive('codeBlock')) return false
  return state.doc.textBetween(from, to).length > 0
}

async function openLink() {
  href.value = props.editor.getAttributes('link').href ?? ''
  turning.value = false
  linking.value = true
  await nextTick()
  input.value?.focus()
  input.value?.select()
}
function applyLink() {
  const url = href.value.trim()
  const chain = props.editor.chain().focus().extendMarkRange('link')
  if (!url) chain.unsetLink().run()
  else chain.setLink({ href: /^[a-z][a-z0-9+.-]*:|^\/|^#/i.test(url) ? url : `https://${url}` }).run()
  linking.value = false
}
function removeLink() {
  props.editor.chain().focus().extendMarkRange('link').unsetLink().run()
  linking.value = false
}
function openCurrent() {
  const url = props.editor.getAttributes('link').href
  if (url) window.open(url, '_blank', 'noopener')
}
defineExpose({ openLink })

function onHide() {
  linking.value = false
  turning.value = false
}
</script>

<template>
  <BubbleMenu :editor="editor" :should-show="shouldShow" :options="{ placement: 'top', offset: 8, onHide }">
    <div class="bubble" @mousedown="($event.target as HTMLElement).tagName !== 'INPUT' && $event.preventDefault()">
      <form v-if="linking" class="flex items-center gap-1" @submit.prevent="applyLink">
        <input
          ref="input"
          v-model="href"
          class="bubble-input"
          type="text"
          inputmode="url"
          autocomplete="off"
          spellcheck="false"
          :placeholder="t('editor.tb.linkPlaceholder')"
          @keydown.esc.prevent="linking = false; editor.commands.focus()"
        />
        <button class="bubble-btn" type="submit" :title="t('editor.tb.apply')" :aria-label="t('editor.tb.apply')"><Check class="h-4 w-4" /></button>
        <button v-if="editor.isActive('link')" class="bubble-btn" type="button" :title="t('editor.tb.removeLink')" :aria-label="t('editor.tb.removeLink')" @click="removeLink">
          <Unlink class="h-4 w-4" />
        </button>
      </form>
      <template v-else>
        <div class="relative">
          <button class="bubble-btn bubble-turn" type="button" :title="t('editor.tb.turnInto')" :aria-expanded="turning" @click="turning = !turning">
            <span class="truncate">{{ current }}</span><ChevronDown class="h-3.5 w-3.5 shrink-0 opacity-60" />
          </button>
          <div v-if="turning" class="bubble-drop popover" role="menu">
            <button v-for="b in blocks" :key="b.key" type="button" class="menu-item" @click="b.run(); turning = false">
              <span class="flex-1">{{ b.label }}</span><Check v-if="b.on()" class="h-3.5 w-3.5" />
            </button>
          </div>
        </div>
        <span class="bubble-sep" />
        <button
          v-for="m in marks"
          :key="m.key"
          type="button"
          class="bubble-btn"
          :class="{ on: editor.isActive(m.key) }"
          :title="m.label"
          :aria-label="m.label"
          :aria-pressed="editor.isActive(m.key)"
          @click="m.run"
        >
          <component :is="m.icon" class="h-4 w-4" :stroke-width="2.2" />
        </button>
        <span class="bubble-sep" />
        <button type="button" class="bubble-btn" :class="{ on: editor.isActive('link') }" :title="t('editor.tb.link') + ' (⌘K)'" :aria-label="t('editor.tb.link')" @click="openLink">
          <Link2 class="h-4 w-4" :stroke-width="2.2" />
        </button>
        <button v-if="editor.isActive('link')" type="button" class="bubble-btn" :title="t('editor.tb.openLink')" :aria-label="t('editor.tb.openLink')" @click="openCurrent">
          <ExternalLink class="h-4 w-4" :stroke-width="2.2" />
        </button>
      </template>
    </div>
  </BubbleMenu>
</template>
