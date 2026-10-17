<script setup lang="ts">
import { ref, computed, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { MoreHorizontal, Plus, X, Pencil, CircleCheck, Circle, ArrowLeft, ArrowRight, Trash2 } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import BoardCard from './BoardCard.vue'
import ColorDots from './ColorDots.vue'
import { useSortable, type SortMove } from '@/lib/sortable'
import { useTasksStore } from '@/stores/tasks'
import { useToastStore } from '@/stores/toast'
import type { Column, Member, Task } from '@/api/planner'

// One column of the board: its header (colour, name, count and a ⋯ menu to
// change the column), its cards — a drop target shared with every other
// column (group "board-cards") — and an inline composer to add cards.
// Changes to the column itself are emitted; the board saves the list.
const props = defineProps<{
  ws: string
  listId: string
  column: Column
  index: number
  count: number // columns in the list
  cards: Task[]
  color: string // the list's colour
  members: Record<string, Member>
  readonly: boolean
}>()
const emit = defineEmits<{
  open: [id: string]
  move: [m: SortMove]
  dragging: [on: boolean]
  rename: [name: string]
  recolor: [color: string]
  toggleDone: []
  shift: [by: -1 | 1]
  remove: []
}>()
const tasks = useTasksStore()
const toast = useToastStore()
const { t } = useI18n()

const dot = computed(() => props.column.color || (props.column.done ? 'var(--ok)' : 'var(--text-faint)'))
function progress(id: string) {
  const subs = tasks.subtasksOf(props.ws, id)
  return { done: subs.filter((s) => s.done).length, total: subs.length }
}

// ---- Drag and drop ----
const cardsEl = ref<HTMLElement | null>(null)
let lastDrag = 0
let dragged = false
useSortable(cardsEl, (m) => emit('move', m), {
  group: 'board-cards',
  draggable: '.board-card',
  disabled: props.readonly,
  // The floating copy (not the browser's drag image) so it can tilt, and
  // so mouse and touch behave the same.
  forceFallback: true,
  fallbackOnBody: true,
  fallbackTolerance: 4,
  // Touch: a long press picks a card up; a swipe still scrolls the board.
  delay: 220,
  delayOnTouchOnly: true,
  touchStartThreshold: 6,
  scrollSensitivity: 70,
  scrollSpeed: 16,
  bubbleScroll: true,
  onStart: () => {
    dragged = true
    emit('dragging', true)
  },
  // Also fires after a plain click (the card is "chosen" on mouse down).
  onUnchoose: () => {
    if (!dragged) return
    dragged = false
    lastDrag = Date.now()
    emit('dragging', false)
  },
})
function open(id: string) {
  // The click that ends a drag isn't a tap on the card.
  if (Date.now() - lastDrag < 250) return
  emit('open', id)
}

// ---- Header: rename in place and the ⋯ menu ----
const menuBtn = ref<HTMLElement | null>(null)
const menu = ref(false)
const renaming = ref(false)
const nameDraft = ref('')
const nameInput = ref<HTMLInputElement | null>(null)
async function startRename() {
  if (props.readonly) return
  menu.value = false
  nameDraft.value = props.column.name
  renaming.value = true
  await nextTick()
  nameInput.value?.select()
}
function commitRename() {
  if (!renaming.value) return
  renaming.value = false
  const v = nameDraft.value.trim()
  if (v && v !== props.column.name) emit('rename', v)
}
function act(fn: () => void) {
  menu.value = false
  fn()
}

// ---- Add a card ----
const adding = ref(false)
const draft = ref('')
const busy = ref(false)
const composer = ref<HTMLTextAreaElement | null>(null)
const scroller = ref<HTMLElement | null>(null)
async function startAdd() {
  adding.value = true
  await nextTick()
  composer.value?.focus()
  scroller.value?.scrollTo({ top: scroller.value.scrollHeight, behavior: 'smooth' })
}
function stopAdd() {
  adding.value = false
  draft.value = ''
}
async function addCard() {
  const title = draft.value.trim()
  if (!title || busy.value) return
  busy.value = true
  try {
    await tasks.create(props.ws, { title, list_id: props.listId, column_id: props.column.id })
    draft.value = ''
    await nextTick()
    scroller.value?.scrollTo({ top: scroller.value.scrollHeight, behavior: 'smooth' })
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    busy.value = false
    composer.value?.focus()
  }
}
function onComposerBlur() {
  if (!draft.value.trim()) stopAdd()
}
</script>

<template>
  <section class="col" :aria-label="column.name">
    <header class="col-head">
      <span class="h-2.5 w-2.5 shrink-0 rounded-full" :style="{ background: dot }" />
      <input
        v-if="renaming"
        ref="nameInput"
        v-model="nameDraft"
        class="min-w-0 flex-1 rounded-md bg-surface px-1.5 py-0.5 text-[14px] font-semibold outline-none shadow-[0_0_0_3px_var(--ring)]"
        maxlength="60"
        :aria-label="t('board.columnName')"
        @blur="commitRename"
        @keydown.enter.prevent="commitRename"
        @keydown.esc.stop="renaming = false"
      />
      <h2 v-else class="col-name" :title="column.name" @dblclick="startRename">{{ column.name }}</h2>
      <span v-if="column.done && !renaming" class="grid shrink-0 place-items-center text-ok" :title="t('board.doneColumn')"><CircleCheck class="h-3.5 w-3.5" /></span>
      <span v-if="!renaming" class="count">{{ cards.length }}</span>
      <button
        v-if="!readonly"
        ref="menuBtn"
        class="icon-btn ml-auto h-7 w-7"
        :class="{ on: menu }"
        :aria-label="t('board.columnMenu')"
        :title="t('board.columnMenu')"
        aria-haspopup="menu"
        :aria-expanded="menu"
        @click="menu = !menu"
      >
        <MoreHorizontal class="h-[18px] w-[18px]" />
      </button>
    </header>

    <div ref="scroller" class="col-body" :class="{ composing: adding }">
      <div class="relative">
        <div ref="cardsEl" class="cards" :data-group="column.id">
          <BoardCard
            v-for="c in cards"
            :key="c.id"
            :task="c"
            :color="color"
            :done="column.done || c.done"
            :subtasks="progress(c.id)"
            :assignees="c.assignee_ids.map((id) => members[id]).filter(Boolean)"
            @click="open(c.id)"
            @keydown.enter.prevent="emit('open', c.id)"
            @keydown.space.prevent="emit('open', c.id)"
          />
        </div>
        <p v-if="!cards.length && !adding" class="empty">{{ readonly ? t('board.noCards') : t('board.dropHere') }}</p>
      </div>

      <div v-if="adding" class="composer card">
        <textarea
          ref="composer"
          v-model="draft"
          rows="2"
          class="w-full resize-none bg-transparent text-[14px] leading-snug outline-none placeholder:text-text-faint"
          :placeholder="t('board.cardPlaceholder')"
          :aria-label="t('board.addCard')"
          maxlength="500"
          @keydown.enter.exact.prevent="addCard"
          @keydown.esc.stop.prevent="stopAdd"
          @blur="onComposerBlur"
        />
        <div class="mt-1.5 flex items-center gap-1.5">
          <button class="btn-primary btn-sm" :disabled="!draft.trim() || busy" @mousedown.prevent @click="addCard">{{ t('common.add') }}</button>
          <button class="icon-btn h-7 w-7" :aria-label="t('common.close')" @mousedown.prevent @click="stopAdd"><X class="h-4 w-4" /></button>
          <span class="footnote ml-auto hidden truncate sm:inline">{{ t('board.cardHint') }}</span>
        </div>
      </div>
    </div>

    <button v-if="!readonly && !adding" class="add-card" @click="startAdd"><Plus class="h-4 w-4" />{{ t('board.addCard') }}</button>

    <Popover v-if="menu" :anchor="menuBtn" placement="bottom-end" :width="236" @close="menu = false">
      <button class="menu-item" @click="startRename"><Pencil class="h-4 w-4 text-text-2" />{{ t('common.rename') }}</button>
      <button class="menu-item" @click="act(() => emit('toggleDone'))">
        <component :is="column.done ? Circle : CircleCheck" class="h-4 w-4 text-text-2" />{{ column.done ? t('board.markOpen') : t('board.markDone') }}
      </button>
      <div class="menu-sep" />
      <p class="cap px-2.5 pb-0.5 pt-1">{{ t('board.colour') }}</p>
      <ColorDots :model-value="column.color ?? ''" @update:model-value="(v) => act(() => emit('recolor', v))" />
      <div class="menu-sep" />
      <button class="menu-item disabled:pointer-events-none disabled:opacity-40" :disabled="index === 0" @click="act(() => emit('shift', -1))">
        <ArrowLeft class="h-4 w-4 text-text-2" />{{ t('board.moveLeft') }}
      </button>
      <button class="menu-item disabled:pointer-events-none disabled:opacity-40" :disabled="index >= count - 1" @click="act(() => emit('shift', 1))">
        <ArrowRight class="h-4 w-4 text-text-2" />{{ t('board.moveRight') }}
      </button>
      <div class="menu-sep" />
      <button class="menu-item danger disabled:pointer-events-none disabled:opacity-40" :disabled="count <= 1" @click="act(() => emit('remove'))">
        <Trash2 class="h-4 w-4" />{{ t('board.deleteColumn') }}
      </button>
    </Popover>
  </section>
</template>

<style scoped>
.col {
  display: flex;
  flex: none;
  flex-direction: column;
  width: 288px;
  max-height: 100%;
  border-radius: 14px;
  background: var(--fill-2);
  scroll-snap-align: start;
}
:global(html.dark) .col {
  background: rgba(118, 118, 128, 0.12);
}
@media (max-width: 639px) {
  .col {
    width: 85vw;
    max-width: 360px;
  }
}
.col-head {
  display: flex;
  align-items: center;
  gap: 7px;
  min-height: 44px;
  padding: 8px 8px 4px 13px;
}
.col-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 14px;
  font-weight: 600;
  letter-spacing: -0.01em;
}
.count {
  font-size: 12.5px;
  font-variant-numeric: tabular-nums;
  color: var(--text-3);
}
.col-body {
  min-height: 0;
  flex: 1;
  overflow-y: auto;
  overscroll-behavior: contain;
  padding: 4px 8px 4px;
}
.col-body.composing {
  padding-bottom: 10px;
}
.cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
  /* Room to drop into an empty column. */
  min-height: 52px;
  padding-bottom: 2px;
}
.empty {
  pointer-events: none;
  position: absolute;
  inset: 0 0 2px;
  display: grid;
  place-items: center;
  border-radius: 12px;
  border: 1.5px dashed var(--border);
  font-size: 12.5px;
  color: var(--text-3);
}
.cards:has(.sortable-ghost) + .empty {
  display: none;
}
.composer {
  margin-top: 8px;
  padding: 9px 10px 8px 12px;
}
.composer textarea {
  field-sizing: content;
  min-height: 2.8em;
  max-height: 10em;
}
.add-card {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 2px 8px 8px;
  padding: 7px 6px;
  border-radius: 9px;
  font-size: 13.5px;
  font-weight: 500;
  color: var(--text-2);
  transition: background var(--speed) var(--ease), color var(--speed) var(--ease);
}
.add-card:hover {
  background: var(--surface-hover);
  color: var(--text);
}
</style>
