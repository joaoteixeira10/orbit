<script setup lang="ts">
import { computed } from 'vue'
import { renderMarkdown } from '@/lib/markdown'

// A page as it reads: its Markdown rendered like a README on GitHub, with
// the app's type. Task boxes can be ticked without editing (when allowed);
// a double click asks to edit.
const props = defineProps<{ content: string; editable: boolean }>()
const emit = defineEmits<{ toggle: [index: number]; edit: [] }>()

const html = computed(() => renderMarkdown(props.content))

function onClick(e: MouseEvent) {
  const box = (e.target as HTMLElement).closest('input[data-task]') as HTMLInputElement | null
  if (!box) return
  if (!props.editable) {
    e.preventDefault()
    return
  }
  emit('toggle', Number(box.dataset.task))
}
function onDblClick(e: MouseEvent) {
  if (props.editable && !(e.target as HTMLElement).closest('a, input')) emit('edit')
}
</script>

<template>
  <!-- eslint-disable-next-line vue/no-v-html -- sanitised by renderMarkdown -->
  <article class="md-view" :class="{ editable }" @click="onClick" @dblclick="onDblClick" v-html="html" />
</template>

<style scoped>
.md-view {
  font-size: 16px;
  line-height: 1.65;
  color: var(--text);
  overflow-wrap: anywhere;
  padding-bottom: 24px;
}
.md-view :deep(> :first-child) {
  margin-top: 0;
}
.md-view :deep(h1),
.md-view :deep(h2),
.md-view :deep(h3),
.md-view :deep(h4) {
  margin: 1.6em 0 0.5em;
  font-family: var(--font-display);
  font-weight: 700;
  letter-spacing: -0.018em;
  line-height: 1.25;
}
.md-view :deep(h1) {
  font-size: 1.75em;
  padding-bottom: 0.3em;
  border-bottom: 0.5px solid var(--border);
}
.md-view :deep(h2) {
  font-size: 1.4em;
  padding-bottom: 0.25em;
  border-bottom: 0.5px solid var(--border);
}
.md-view :deep(h3) {
  font-size: 1.15em;
}
.md-view :deep(p),
.md-view :deep(ul),
.md-view :deep(ol),
.md-view :deep(blockquote),
.md-view :deep(pre),
.md-view :deep(table),
.md-view :deep(.md-alert) {
  margin: 0 0 1em;
}
.md-view :deep(ul),
.md-view :deep(ol) {
  padding-left: 1.6em;
}
.md-view :deep(ul) {
  list-style: disc;
}
.md-view :deep(ol) {
  list-style: decimal;
}
.md-view :deep(li + li) {
  margin-top: 0.2em;
}
.md-view :deep(li > ul),
.md-view :deep(li > ol) {
  margin: 0.2em 0 0;
}
/* Task lists: the app's round checkboxes. */
.md-view :deep(li:has(> input[type='checkbox'])),
.md-view :deep(li:has(> p > input[type='checkbox'])) {
  list-style: none;
  margin-left: -1.4em;
}
.md-view :deep(input[type='checkbox']) {
  appearance: none;
  width: 17px;
  height: 17px;
  margin: 0 8px 0 0;
  vertical-align: -3px;
  border-radius: 999px;
  border: 1.5px solid var(--text-faint);
  cursor: default;
  transition: background 0.15s, border-color 0.15s;
}
.md-view.editable :deep(input[type='checkbox']) {
  cursor: pointer;
}
.md-view :deep(input[type='checkbox']:checked) {
  border-color: var(--ws, var(--accent));
  background: var(--ws, var(--accent)) url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='white' stroke-width='3.5' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='M20 6 9 17l-5-5'/%3E%3C/svg%3E") center / 11px no-repeat;
}
.md-view :deep(li:has(> input:checked)),
.md-view :deep(li:has(> p > input:checked)) {
  color: var(--text-3);
  text-decoration: line-through;
}
.md-view :deep(a) {
  color: var(--accent-text);
  text-decoration: underline;
  text-decoration-color: color-mix(in srgb, var(--accent-text) 35%, transparent);
  text-underline-offset: 2px;
}
.md-view :deep(blockquote) {
  padding: 0 1em;
  border-left: 3px solid var(--border-2);
  color: var(--text-2);
}
.md-view :deep(code) {
  padding: 0.15em 0.4em;
  border-radius: 6px;
  background: var(--fill-2);
  font: 0.86em var(--font-mono);
}
.md-view :deep(pre) {
  padding: 14px 16px;
  border-radius: 12px;
  background: var(--fill-2);
  overflow-x: auto;
  line-height: 1.5;
}
.md-view :deep(pre code) {
  padding: 0;
  background: none;
  font-size: 13.5px;
}
.md-view :deep(hr) {
  height: 0;
  margin: 2em 0;
  border: 0;
  border-top: 0.5px solid var(--border-2);
}
.md-view :deep(table) {
  display: block;
  max-width: 100%;
  overflow-x: auto;
  border-collapse: collapse;
  font-size: 14.5px;
}
.md-view :deep(th),
.md-view :deep(td) {
  padding: 7px 13px;
  border: 0.5px solid var(--border-2);
}
.md-view :deep(th) {
  font-weight: 600;
  background: var(--fill-2);
}
.md-view :deep(tr:nth-child(2n) td) {
  background: color-mix(in srgb, var(--fill-2) 50%, transparent);
}
.md-view :deep(img) {
  max-width: 100%;
  border-radius: 10px;
}
.md-view :deep(mark) {
  padding: 0 0.15em;
  border-radius: 3px;
  background: color-mix(in srgb, #ffcc00 45%, transparent);
  color: inherit;
}
.md-view :deep(kbd) {
  padding: 0.1em 0.45em;
  border-radius: 5px;
  border: 0.5px solid var(--border-2);
  box-shadow: inset 0 -1px 0 var(--border-2);
  font: 0.85em var(--font-mono);
}
/* GitHub alerts, in the app's colours. */
.md-view :deep(.md-alert) {
  padding: 10px 16px;
  border-left: 3px solid var(--c);
  border-radius: 4px 12px 12px 4px;
  background: color-mix(in srgb, var(--c) 9%, transparent);
}
.md-view :deep(.md-alert > :last-child) {
  margin-bottom: 0;
}
.md-view :deep(.md-alert-title) {
  margin-bottom: 4px;
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.02em;
  color: var(--c);
}
.md-view :deep(.md-alert-note) {
  --c: #007aff;
}
.md-view :deep(.md-alert-tip) {
  --c: #34c759;
}
.md-view :deep(.md-alert-important) {
  --c: #af52de;
}
.md-view :deep(.md-alert-warning) {
  --c: #ff9500;
}
.md-view :deep(.md-alert-caution) {
  --c: #ff3b30;
}
.md-view :deep(.footnotes) {
  margin-top: 2em;
  padding-top: 0.8em;
  border-top: 0.5px solid var(--border);
  font-size: 13.5px;
  color: var(--text-2);
}
.md-view :deep(sup a) {
  text-decoration: none;
}
</style>
