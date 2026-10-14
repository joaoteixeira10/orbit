<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, Plus, X, ChevronRight } from 'lucide-vue-next'
import { newItem, MAX_BODY, MAX_ITEMS, MAX_ITEM_TEXT, MAX_TITLE, type Draft, type DraftItem } from './noteModel'

// The editable inside of a note, shared by the capture box and the editor:
// a title, then either the text (growing with it) or checklist rows. In a
// checklist, Enter adds a row below (splitting at the caret), Backspace on an
// empty row removes it; ticked rows move under "N checked items".
const draft = defineModel<Draft>({ required: true })
const props = withDefaults(defineProps<{ readonly?: boolean; big?: boolean }>(), { readonly: false, big: false })
const emit = defineEmits<{ submit: [] }>()
const { t } = useI18n()

const titleEl = ref<HTMLTextAreaElement | null>(null)
const bodyEl = ref<HTMLTextAreaElement | null>(null)
const addEl = ref<HTMLInputElement | null>(null)
const rows = new Map<number, HTMLInputElement>()
const setRow = (k: number, el: unknown) => {
  if (el) rows.set(k, el as HTMLInputElement)
  else rows.delete(k)
}

const open = computed(() => draft.value.items.filter((i) => !i.done))
const checked = computed(() => draft.value.items.filter((i) => i.done))
const showChecked = ref(true)

// Grow a textarea with its text (field-sizing where supported, else by hand).
function grow(el: HTMLTextAreaElement | null) {
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${el.scrollHeight}px`
}
watch(
  () => [draft.value.title, draft.value.body, draft.value.checklist],
  () => nextTick(() => (grow(titleEl.value), grow(bodyEl.value))),
)
onMounted(() => (grow(titleEl.value), grow(bodyEl.value)))

function onKey(e: KeyboardEvent) {
  if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
    e.preventDefault()
    emit('submit')
    return true
  }
  return false
}
function onTitleKey(e: KeyboardEvent) {
  if (onKey(e)) return
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
    e.preventDefault()
    focus(draft.value.checklist ? 'end' : 'body')
  }
}

async function focusRow(k: number, at: 'start' | 'end' = 'end') {
  await nextTick()
  const el = rows.get(k)
  if (!el) return
  el.focus()
  const p = at === 'start' ? 0 : el.value.length
  el.setSelectionRange(p, p)
}

function onRowKey(e: KeyboardEvent, it: DraftItem) {
  if (onKey(e)) return
  const el = e.target as HTMLInputElement
  const list = draft.value.items
  const i = list.indexOf(it)
  if (e.key === 'Enter' && !e.isComposing) {
    e.preventDefault()
    if (list.length >= MAX_ITEMS) return
    const at = el.selectionStart ?? it.text.length
    const rest = it.text.slice(at)
    it.text = it.text.slice(0, at)
    const n = newItem(rest, false)
    list.splice(i + 1, 0, n)
    void focusRow(n.k, 'start')
  } else if (e.key === 'Backspace' && !it.text && el.selectionStart === 0) {
    e.preventDefault()
    const visible = it.done ? checked.value : open.value
    const prev = visible[visible.indexOf(it) - 1]
    list.splice(i, 1)
    if (prev) void focusRow(prev.k)
    else if (!it.done) focus('title')
  } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    const visible = [...open.value, ...checked.value]
    const next = visible[visible.indexOf(it) + (e.key === 'ArrowDown' ? 1 : -1)]
    if (next) {
      e.preventDefault()
      void focusRow(next.k)
    } else if (e.key === 'ArrowDown' && !it.done) addEl.value?.focus()
  }
}

// Typing in the "List item" row adds a real row and moves there.
function onAdd(e: Event) {
  const el = e.target as HTMLInputElement
  if (!el.value || draft.value.items.length >= MAX_ITEMS) return (el.value = '')
  const n = newItem(el.value)
  el.value = ''
  // Right after the last open one.
  const list = draft.value.items
  const last = list.map((i) => i.done).lastIndexOf(false)
  list.splice(last + 1, 0, n)
  void focusRow(n.k)
}
function onAddKey(e: KeyboardEvent) {
  if (onKey(e)) return
  if (e.key === 'Backspace' && open.value.length) {
    e.preventDefault()
    void focusRow(open.value[open.value.length - 1].k)
  } else if (e.key === 'ArrowUp' && open.value.length) {
    e.preventDefault()
    void focusRow(open.value[open.value.length - 1].k)
  }
}

function remove(it: DraftItem) {
  draft.value.items.splice(draft.value.items.indexOf(it), 1)
}

// focus puts the caret in the title, the body (or the add row), or at the end.
async function focus(where: 'title' | 'body' | 'end' = 'end') {
  await nextTick()
  if (where === 'title') return titleEl.value?.focus()
  if (!draft.value.checklist) {
    const el = bodyEl.value
    if (!el) return
    el.focus()
    el.setSelectionRange(el.value.length, el.value.length)
    return
  }
  addEl.value?.focus()
}
defineExpose({ focus })
</script>

<template>
  <div class="note-fields" :class="{ big }">
    <textarea
      v-if="!readonly || draft.title"
      ref="titleEl"
      v-model="draft.title"
      rows="1"
      class="f-title"
      :placeholder="t('notes.titlePlaceholder')"
      :readonly="props.readonly"
      :maxlength="MAX_TITLE"
      @keydown="onTitleKey"
      @input="grow(titleEl)"
    />

    <textarea
      v-if="!draft.checklist"
      ref="bodyEl"
      v-model="draft.body"
      rows="1"
      class="f-body"
      :placeholder="t('notes.takeNote')"
      :readonly="props.readonly"
      :maxlength="MAX_BODY"
      @keydown="onKey"
      @input="grow(bodyEl)"
    />

    <div v-else class="f-list">
      <div v-for="it in open" :key="it.k" class="f-row group/row">
        <button type="button" class="check" :style="{ '--check': 'var(--note-ink)' }" role="checkbox" :aria-checked="false" :aria-label="it.text" :disabled="readonly" @click="it.done = true">
          <Check class="h-3 w-3" :stroke-width="3.4" />
        </button>
        <input
          :ref="(el) => setRow(it.k, el)"
          v-model="it.text"
          class="f-item"
          :readonly="readonly"
          :maxlength="MAX_ITEM_TEXT"
          :aria-label="t('notes.listItem')"
          @keydown="onRowKey($event, it)"
        />
        <button v-if="!readonly" type="button" class="f-x" :aria-label="t('notes.removeItem')" tabindex="-1" @click="remove(it)"><X class="h-3.5 w-3.5" /></button>
      </div>
      <label v-if="!readonly && draft.items.length < 200" class="f-row f-add">
        <Plus class="mx-[2px] h-4 w-4 shrink-0 text-text-3" />
        <input ref="addEl" class="f-item" :placeholder="t('notes.listItem')" @input="onAdd" @keydown="onAddKey" />
      </label>

      <template v-if="checked.length">
        <button type="button" class="f-checked" @click="showChecked = !showChecked">
          <ChevronRight class="h-3.5 w-3.5 transition-transform" :class="{ 'rotate-90': showChecked }" />
          {{ t('notes.checkedItems', checked.length) }}
        </button>
        <template v-if="showChecked">
          <div v-for="it in checked" :key="it.k" class="f-row done group/row">
            <button type="button" class="check on" :style="{ '--check': 'var(--note-ink)' }" role="checkbox" :aria-checked="true" :aria-label="it.text" :disabled="readonly" @click="it.done = false">
              <Check class="h-3 w-3" :stroke-width="3.4" />
            </button>
            <input
              :ref="(el) => setRow(it.k, el)"
              v-model="it.text"
              class="f-item"
              :readonly="readonly"
              :maxlength="MAX_ITEM_TEXT"
              :aria-label="t('notes.listItem')"
              @keydown="onRowKey($event, it)"
            />
            <button v-if="!readonly" type="button" class="f-x" :aria-label="t('notes.removeItem')" tabindex="-1" @click="remove(it)"><X class="h-3.5 w-3.5" /></button>
          </div>
        </template>
      </template>
    </div>
  </div>
</template>

<style scoped>
.note-fields {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
textarea {
  display: block;
  width: 100%;
  resize: none;
  overflow: hidden;
  background: transparent;
  outline: none;
  color: var(--text);
  field-sizing: content;
}
textarea:focus-visible,
.f-item:focus-visible {
  box-shadow: none;
}
textarea::placeholder,
.f-item::placeholder {
  color: var(--text-3);
}
.f-title {
  font: 600 16px/1.35 var(--font-display);
  letter-spacing: -0.012em;
}
.f-body {
  font-size: 14.5px;
  line-height: 1.5;
  min-height: 1.5em;
}
.big .f-title {
  font-size: 20px;
  letter-spacing: -0.018em;
}
.big .f-body {
  font-size: 15px;
  min-height: 7.5em;
}
.f-list {
  display: flex;
  flex-direction: column;
  margin: 0 -4px;
}
.f-row {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 32px;
  padding: 0 4px;
  border-radius: 8px;
}
.f-row:focus-within {
  background: var(--note-line);
}
.f-item {
  flex: 1;
  min-width: 0;
  background: transparent;
  outline: none;
  font-size: 14.5px;
  color: var(--text);
}
.f-row :focus-visible {
  box-shadow: none;
}
.done .f-item {
  color: var(--text-3);
  text-decoration: line-through;
}
.check {
  width: 18px;
  height: 18px;
}
.f-x {
  display: grid;
  place-items: center;
  width: 22px;
  height: 22px;
  border-radius: 999px;
  color: var(--text-3);
  opacity: 0;
  transition: opacity 0.15s;
}
.f-row:hover .f-x,
.f-row:focus-within .f-x {
  opacity: 1;
}
.f-x:hover {
  background: var(--fill);
  color: var(--text);
}
@media (hover: none) {
  .f-x {
    opacity: 1;
  }
}
.f-add {
  cursor: text;
}
.f-checked {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 8px 0 2px;
  padding: 6px 4px 4px;
  border-top: 0.5px solid var(--note-line);
  font-size: 12.5px;
  font-weight: 500;
  color: var(--text-2);
  text-align: left;
}
</style>
