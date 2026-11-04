<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronDown, CircleCheck, Plus } from 'lucide-vue-next'
import BoardCard from './BoardCard.vue'
import Avatar from '@/components/ui/Avatar.vue'
import { useSortables, type SortMove } from '@/lib/sortable'
import { useTasksStore } from '@/stores/tasks'
import { useToastStore } from '@/stores/toast'
import type { Column, Member, Task } from '@/api/planner'

// The board in swimlanes: the columns across the top (each with its count,
// and its limit when it has one), and a lane down the page for each person
// (a card shows in each of its people's lanes; "No one" for none) or each
// priority. Dragging a card to another lane gives it that person (in place
// of the lane's it left) or that priority; to another column, that status.
export type LaneKey = string // a person's id, "" (no one); or "3".."0" for priority
export interface Lane {
  key: LaneKey
  label: string
  member?: Member
}
const props = defineProps<{
  ws: string
  listId: string
  columns: Column[]
  lanes: Lane[]
  cards: (col: string, lane: LaneKey) => Task[]
  totals: Record<string, number> // cards per column, all lanes
  color: string
  members: Record<string, Member>
  readonly: boolean
  by: 'person' | 'priority'
}>()
const emit = defineEmits<{ open: [id: string]; move: [m: SortMove]; dragging: [on: boolean] }>()
const { t } = useI18n()
const tasks = useTasksStore()
const toast = useToastStore()

const folded = ref(new Set<string>())
const fold = (k: string) => {
  const s = new Set(folded.value)
  if (!s.delete(k)) s.add(k)
  folded.value = s
}
const laneCount = (k: LaneKey) => props.columns.reduce((n, c) => n + props.cards(c.id, k).length, 0)
function progress(id: string) {
  const subs = tasks.subtasksOf(props.ws, id)
  return { done: subs.filter((s) => s.done).length, total: subs.length }
}

// One sortable per cell; all share the group, so cards go anywhere.
let lastDrag = 0
let dragged = false
const cellEls = ref<Record<string, HTMLElement | null>>({})
function setCell(key: string, el: unknown) {
  const node = el instanceof HTMLElement ? el : null
  if (cellEls.value[key] !== node) cellEls.value = { ...cellEls.value, [key]: node }
}
useSortables(cellEls, (m) => emit('move', m), {
  group: 'board-cards',
  draggable: '.board-card',
  disabled: props.readonly,
  forceFallback: true,
  fallbackOnBody: true,
  fallbackTolerance: 4,
  delay: 220,
  delayOnTouchOnly: true,
  touchStartThreshold: 6,
  onStart: () => {
    dragged = true
    emit('dragging', true)
  },
  onUnchoose: () => {
    if (!dragged) return
    dragged = false
    lastDrag = Date.now()
    emit('dragging', false)
  },
})
function open(id: string) {
  if (Date.now() - lastDrag < 250) return
  emit('open', id)
}

// A card added in a cell gets its column, and its lane's person or priority.
const adding = ref('')
const draft = ref('')
const addEl = ref<HTMLInputElement | HTMLInputElement[] | null>(null)
async function startAdd(key: string) {
  adding.value = key
  draft.value = ''
  await nextTick()
  ;(Array.isArray(addEl.value) ? addEl.value[0] : addEl.value)?.focus()
}
async function add(col: Column, lane: Lane) {
  const title = draft.value.trim()
  if (!title) {
    adding.value = ''
    return
  }
  draft.value = ''
  try {
    await tasks.create(props.ws, {
      title,
      list_id: props.listId,
      column_id: col.id,
      ...(props.by === 'person' ? (lane.key ? { assignee_ids: [lane.key] } : {}) : { priority: Number(lane.key) }),
    })
  } catch (e: any) {
    toast.error(e.message)
  }
}
const over = (c: Column) => !!c.limit && (props.totals[c.id] ?? 0) > c.limit
</script>

<template>
  <div class="lanes" :style="{ '--cols': columns.length }" data-testid="board-lanes">
    <!-- The columns, across the top -->
    <div class="lane-heads">
      <div v-for="c in columns" :key="c.id" class="lane-col-head" :class="{ over: over(c) }" data-testid="lane-col-head">
        <span class="h-2.5 w-2.5 shrink-0 rounded-full" :style="{ background: c.color || (c.done ? 'var(--ok)' : 'var(--text-faint)') }" />
        <span class="truncate font-semibold">{{ c.name }}</span>
        <CircleCheck v-if="c.done" class="h-3.5 w-3.5 shrink-0 text-ok" />
        <span class="count" :title="c.limit ? t('board.limitOf', { n: c.limit }) : ''">{{ totals[c.id] ?? 0 }}<template v-if="c.limit">/{{ c.limit }}</template></span>
      </div>
    </div>

    <section v-for="l in lanes" :key="l.key" class="lane" :data-lane="l.key" data-testid="lane">
      <button type="button" class="lane-head" :aria-expanded="!folded.has(l.key)" @click="fold(l.key)">
        <ChevronDown class="chev h-4 w-4" :class="{ shut: folded.has(l.key) }" />
        <Avatar v-if="l.member" :name="l.member.display_name || l.member.username" :src="l.member.avatar_url" :size="20" />
        <span class="font-semibold">{{ l.label }}</span>
        <span class="count">{{ laneCount(l.key) }}</span>
      </button>
      <div v-show="!folded.has(l.key)" class="lane-row">
        <div v-for="c in columns" :key="c.id" class="cell">
          <div :ref="(el) => setCell(`${c.id}|${l.key}`, el)" class="cell-cards" :data-group="`${c.id}|${l.key}`">
            <BoardCard
              v-for="x in cards(c.id, l.key)"
              :key="x.id"
              :task="x"
              :color="color"
              :done="c.done || x.done"
              :subtasks="progress(x.id)"
              :assignees="x.assignee_ids.map((id) => members[id]).filter(Boolean)"
              @click="open(x.id)"
              @keydown.enter.prevent="emit('open', x.id)"
            />
          </div>
          <template v-if="!readonly">
            <form v-if="adding === `${c.id}|${l.key}`" class="mt-1.5" @submit.prevent="add(c, l)">
              <input ref="addEl" v-model="draft" class="field !py-1.5 text-[13.5px]" maxlength="500" :placeholder="t('board.cardPlaceholder')" :aria-label="t('board.addCard')" @keydown.esc="adding = ''" @blur="add(c, l)" />
            </form>
            <button v-else type="button" class="cell-add" :aria-label="t('board.addCard')" @click="startAdd(`${c.id}|${l.key}`)"><Plus class="h-3.5 w-3.5" /></button>
          </template>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.lanes {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: max-content;
}
.lane-heads,
.lane-row {
  display: grid;
  grid-template-columns: repeat(var(--cols), 272px);
  gap: 12px;
}
.lane-heads {
  position: sticky;
  top: 0;
  z-index: 2;
  padding-bottom: 4px;
  background: var(--bg);
}
.lane-col-head {
  display: flex;
  align-items: center;
  gap: 7px;
  min-height: 36px;
  padding: 0 12px;
  border-radius: 12px;
  background: var(--fill-2);
  font-size: 13.5px;
}
.lane-col-head.over {
  background: color-mix(in srgb, var(--crit, #ff3b30) 14%, transparent);
  color: var(--crit-text);
}
.count {
  margin-left: auto;
  font-size: 12.5px;
  font-variant-numeric: tabular-nums;
  color: var(--text-3);
}
.over .count {
  color: var(--crit-text);
  font-weight: 600;
}
.lane-head {
  position: sticky;
  left: 0;
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 4px 8px 6px 2px;
  font-size: 14px;
}
.lane-head .chev {
  color: var(--text-3);
  transition: transform var(--speed);
}
.lane-head .chev.shut {
  transform: rotate(-90deg);
}
.lane {
  padding-bottom: 6px;
  border-bottom: 0.5px solid var(--border);
}
.cell {
  min-height: 64px;
  padding: 6px;
  border-radius: 12px;
  background: var(--fill-2);
}
html.dark .cell,
html.dark .lane-col-head {
  background: rgba(118, 118, 128, 0.12);
}
.cell-cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-height: 40px;
}
.cell-add {
  display: flex;
  width: 100%;
  justify-content: center;
  margin-top: 4px;
  padding: 3px;
  border-radius: 8px;
  color: var(--text-3);
  opacity: 0;
  transition: opacity var(--speed), background var(--speed);
}
.cell:hover .cell-add,
.cell-add:focus-visible {
  opacity: 1;
}
.cell-add:hover {
  background: var(--surface-hover);
  color: var(--text);
}
</style>
