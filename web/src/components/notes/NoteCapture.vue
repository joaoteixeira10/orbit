<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Pin, Palette, SquareCheck, Type } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import NoteFields from './NoteFields.vue'
import NoteColorPicker from './NoteColorPicker.vue'
import { draftOf, inputOf, isEmptyDraft, toChecklist, toText, type Draft } from './noteModel'
import type { NoteInput } from '@/api/planner'

// The "Take a note…" box above the wall. It opens into a title and a text
// (or checklist) with a colour, checklist and pin toolbar; a click outside,
// Close, Escape or ⌘↵ adds the note (when there is something in it) and
// folds the box again.
const props = defineProps<{ placeholder?: string }>()
const emit = defineEmits<{ create: [body: NoteInput] }>()
const { t } = useI18n()

const open = ref(false)
const draft = ref<Draft>(draftOf())
const root = ref<HTMLElement | null>(null)
const fields = ref<InstanceType<typeof NoteFields> | null>(null)
const colorBtn = ref<HTMLElement | null>(null)
const picking = ref(false)

async function start(checklist = false) {
  if (!open.value) draft.value = draftOf(null, { checklist })
  else if (checklist) toChecklist(draft.value)
  open.value = true
  await nextTick()
  void fields.value?.focus(checklist ? 'end' : 'body')
}

function close() {
  picking.value = false
  if (!open.value) return
  if (!isEmptyDraft(draft.value)) emit('create', inputOf(draft.value))
  draft.value = draftOf()
  open.value = false
  ;(document.activeElement as HTMLElement | null)?.blur?.()
}

function toggleChecklist() {
  if (draft.value.checklist) toText(draft.value)
  else toChecklist(draft.value)
  void fields.value?.focus('end')
}

// A press outside the box (and outside its colour popover) closes it.
function onDown(e: MouseEvent) {
  if (!open.value) return
  const el = e.target as HTMLElement
  if (root.value?.contains(el) || el.closest?.('.popover')) return
  close()
}
// ⌘↵ saves from anywhere while the box is open (e.g. after picking a colour).
function onDocKey(e: KeyboardEvent) {
  if (open.value && e.key === 'Enter' && (e.metaKey || e.ctrlKey) && !root.value?.contains(e.target as Node)) {
    e.preventDefault()
    close()
  }
}
onMounted(() => {
  document.addEventListener('mousedown', onDown, true)
  document.addEventListener('keydown', onDocKey)
})
onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onDown, true)
  document.removeEventListener('keydown', onDocKey)
})

function onKey(e: KeyboardEvent) {
  // (An Escape the editor used — back to one cursor, the find bar — isn't for us.)
  if (e.key === 'Escape' && open.value && !e.defaultPrevented) {
    e.preventDefault()
    close()
  }
}
defineExpose({ start })
</script>

<template>
  <div ref="root" class="capture" :class="{ open }" :data-note="open ? draft.color : ''" @keydown="onKey">
    <div v-if="!open" class="flex items-center">
      <button class="cap-prompt" @click="start(false)">{{ props.placeholder || t('notes.takeNote') }}</button>
      <button class="tb mr-2" :aria-label="t('notes.newList')" :title="t('notes.newList')" @click="start(true)">
        <SquareCheck class="h-[19px] w-[19px]" />
      </button>
    </div>

    <template v-else>
      <div class="relative px-4 pt-3">
        <NoteFields ref="fields" v-model="draft" @submit="close" />
        <button
          class="tb pin absolute right-2 top-2"
          :class="{ on: draft.pinned }"
          :aria-label="draft.pinned ? t('notes.unpin') : t('notes.pin')"
          :title="draft.pinned ? t('notes.unpin') : t('notes.pin')"
          @click="draft.pinned = !draft.pinned"
        >
          <Pin class="h-4 w-4" :fill="draft.pinned ? 'currentColor' : 'none'" />
        </button>
      </div>
      <div class="flex items-center gap-0.5 px-2 pb-2 pt-2">
        <button ref="colorBtn" class="tb" :class="{ on: picking }" :aria-label="t('notes.color')" :title="t('notes.color')" @click="picking = !picking">
          <Palette class="h-[18px] w-[18px]" />
        </button>
        <button class="tb" :aria-label="draft.checklist ? t('notes.hideChecks') : t('notes.showChecks')" :title="draft.checklist ? t('notes.hideChecks') : t('notes.showChecks')" @click="toggleChecklist">
          <Type v-if="draft.checklist" class="h-[18px] w-[18px]" />
          <SquareCheck v-else class="h-[18px] w-[18px]" />
        </button>
        <span class="ml-auto mr-3 hidden text-[12px] text-text-3 sm:inline">{{ t('notes.shortcut') }}</span>
        <button class="btn btn-sm close-btn" @click="close">{{ t('common.close') }}</button>
      </div>
      <Popover v-if="picking" :anchor="colorBtn" placement="bottom-start" @close="picking = false">
        <NoteColorPicker v-model="draft.color" />
      </Popover>
    </template>
  </div>
</template>

<style scoped>
.capture {
  width: 100%;
  max-width: 600px;
  margin-left: auto;
  margin-right: auto;
  border-radius: 14px;
  background: var(--note-bg, var(--surface));
  box-shadow: 0 0 0 0.5px var(--border), 0 1px 2px rgba(0, 0, 0, 0.04), 0 4px 16px rgba(0, 0, 0, 0.06);
  transition: background 0.25s var(--ease), box-shadow 0.25s var(--ease);
}
:global(html.dark) .capture {
  box-shadow: 0 0 0 0.5px var(--border), 0 4px 16px rgba(0, 0, 0, 0.3);
}
.capture.open {
  box-shadow: 0 0 0 0.5px var(--border), 0 2px 6px rgba(0, 0, 0, 0.05), 0 12px 36px rgba(0, 0, 0, 0.12);
}
:global(html.dark) .capture.open {
  box-shadow: 0 0 0 0.5px var(--border-2), 0 12px 36px rgba(0, 0, 0, 0.45);
}
.cap-prompt {
  flex: 1;
  min-width: 0;
  padding: 14px 18px;
  text-align: left;
  font-size: 15px;
  font-weight: 500;
  color: var(--text-3);
  cursor: text;
}
.tb {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  border-radius: 999px;
  color: var(--text-2);
  transition: background 0.15s var(--ease), color 0.15s;
}
.tb:hover,
.tb.on {
  background: var(--fill);
  color: var(--text);
}
.tb.pin.on {
  background: transparent;
}
.tb.pin.on:hover {
  background: var(--fill);
}
.capture :deep(.f-title) {
  padding-right: 30px;
}
.close-btn {
  background: transparent;
  font-weight: 600;
}
.close-btn:hover {
  background: var(--fill) !important;
}
</style>
