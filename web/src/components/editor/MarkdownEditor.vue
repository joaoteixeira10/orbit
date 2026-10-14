<script setup lang="ts">
import { ref, watch, nextTick, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { normalizeMarkdown } from '@/lib/markdown'

// The page body, edited as Markdown source: a plain text area that grows with
// the page, with the small helps of a Markdown editor — Enter continues a
// list, task or quote (and ends it on an empty item), Tab / ⇧Tab indent,
// ⌘B ⌘I ⌘E ⌘K wrap the selection in bold, italic, code, a link. Same
// contract as PageEditor: the parent saves; this hands out the Markdown.
const props = defineProps<{ content: string; editable: boolean }>()
const emit = defineEmits<{ change: [] }>()
const { t } = useI18n()

const text = ref(props.content)
const area = ref<HTMLTextAreaElement | null>(null)

function grow() {
  const el = area.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${el.scrollHeight}px`
}
onMounted(grow)
watch(text, () => nextTick(grow))

function onInput() {
  emit('change')
}

// replace swaps [from, to) for s and puts the caret (or selection) at
// [selFrom, selTo), as an edit the browser can undo.
function replace(from: number, to: number, s: string, selFrom: number, selTo = selFrom) {
  const el = area.value!
  el.focus()
  el.setSelectionRange(from, to)
  if (!document.execCommand?.('insertText', false, s)) {
    text.value = text.value.slice(0, from) + s + text.value.slice(to)
  }
  text.value = el.value
  void nextTick(() => el.setSelectionRange(selFrom, selTo))
  emit('change')
}

const LIST = /^(\s*)(?:([-*+])\s+(\[[ xX]\]\s+)?|(\d+)([.)])\s+|(>\s?))/

function onEnter(e: KeyboardEvent) {
  const el = area.value!
  const pos = el.selectionStart
  if (pos !== el.selectionEnd) return
  const start = text.value.lastIndexOf('\n', pos - 1) + 1
  const line = text.value.slice(start, pos)
  const m = LIST.exec(line)
  if (!m) return
  e.preventDefault()
  if (line.slice(m[0].length).trim() === '' && text.value.slice(pos, text.value.indexOf('\n', pos) >>> 0).trim() === '') {
    // An empty item ends the list.
    return replace(start, pos, '', start)
  }
  const [, ind, bullet, box, num, delim, quote] = m
  const next = quote ? `${ind}> ` : num ? `${ind}${Number(num) + 1}${delim} ` : `${ind}${bullet} ${box ? '[ ] ' : ''}`
  replace(pos, pos, `\n${next}`, pos + 1 + next.length)
}

function onTab(e: KeyboardEvent) {
  e.preventDefault()
  const el = area.value!
  const { selectionStart: a, selectionEnd: b } = el
  const start = text.value.lastIndexOf('\n', a - 1) + 1
  const end = b
  const block = text.value.slice(start, end)
  const lines = block.split('\n')
  const out = e.shiftKey ? lines.map((l) => l.replace(/^ {1,2}/, '')) : lines.map((l) => '  ' + l)
  const joined = out.join('\n')
  const shift = joined.length - block.length
  const first = e.shiftKey ? out[0].length - lines[0].length : 2
  replace(start, end, joined, Math.max(start, a + first), b + shift)
}

function wrap(before: string, after = before) {
  const el = area.value!
  const { selectionStart: a, selectionEnd: b } = el
  const sel = text.value.slice(a, b)
  replace(a, b, before + sel + after, a + before.length, b + before.length)
}

function onKey(e: KeyboardEvent) {
  if (!props.editable) return
  const mod = e.metaKey || e.ctrlKey
  if (e.key === 'Enter' && !mod && !e.shiftKey && !e.isComposing) onEnter(e)
  else if (e.key === 'Tab' && !mod) onTab(e)
  else if (mod && !e.altKey) {
    const k = e.key.toLowerCase()
    const forms: Record<string, [string, string?]> = { b: ['**'], i: ['*'], e: ['`'], k: ['[', '](https://)'] }
    if (forms[k] && !(k === 'k' && area.value!.selectionStart === area.value!.selectionEnd)) {
      e.preventDefault()
      e.stopPropagation()
      wrap(...forms[k])
    }
  }
}

// getContent is the page's Markdown, tidied the way every save is.
function getContent(): string {
  return normalizeMarkdown(text.value)
}
// setContent replaces the text (someone else's save), keeping the caret.
function setContent(md: string) {
  const el = area.value
  const [a, b] = el ? [el.selectionStart, el.selectionEnd] : [0, 0]
  text.value = md
  void nextTick(() => el?.setSelectionRange(Math.min(a, md.length), Math.min(b, md.length)))
}
function focusStart() {
  area.value?.focus()
  area.value?.setSelectionRange(0, 0)
}
defineExpose({ getContent, setContent, focusStart })
</script>

<template>
  <textarea
    ref="area"
    v-model="text"
    class="md-source"
    :readonly="!editable"
    :placeholder="t('editor.mdPlaceholder')"
    spellcheck="true"
    :aria-label="t('editor.mdLabel')"
    @input="onInput"
    @keydown="onKey"
  />
</template>

<style scoped>
.md-source {
  display: block;
  width: 100%;
  min-height: 40vh;
  padding: 2px 0 24px;
  resize: none;
  overflow: hidden;
  border: 0;
  outline: none;
  background: transparent;
  color: var(--text);
  font: 14px/1.7 var(--font-mono);
  tab-size: 2;
  field-sizing: content;
}
.md-source::placeholder {
  color: var(--text-faint);
}
.md-source:focus-visible {
  box-shadow: none;
}
</style>
