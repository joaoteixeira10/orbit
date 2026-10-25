<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Plus, SlidersHorizontal, Columns3, X, Eye, Rows3 } from 'lucide-vue-next'
import BoardColumn from '@/components/board/BoardColumn.vue'
import BoardLanes, { type Lane } from '@/components/board/BoardLanes.vue'
import ListSheet from '@/components/tasks/ListSheet.vue'
import TaskDetailSheet from '@/components/tasks/TaskDetailSheet.vue'
import TaskFilter from '@/components/tasks/TaskFilter.vue'
import Confirm from '@/components/ui/Confirm.vue'
import { useTasksStore } from '@/stores/tasks'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { tasksApi, type Column, type Member, type Task, type TaskList } from '@/api/planner'
import type { SortMove } from '@/lib/sortable'
import { useTaskFilter } from '@/lib/taskFilter'

// The Board: one list's tasks as kanban columns (its statuses). The lists
// are a segmented strip at the top; the active one is in the URL. Cards are
// dragged within and across columns (the server ranks them, and completes
// or reopens them when they enter or leave a done column); columns are
// changed from their ⋯ menu or the list sheet.
const route = useRoute()
const router = useRouter()
const tasks = useTasksStore()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws ?? ''))
const routeList = computed(() => String(route.params.list ?? ''))
const workspace = computed(() => wss.byId(ws.value))
const readonly = computed(
  () =>
    !workspace.value ||
    workspace.value.role === 'viewer' ||
    !!workspace.value.archived_at ||
    !!auth.privileges.read_only ||
    list.value?.git?.write === 'none', // a linked list I can't write to git
)

// ---- Data ----
const loaded = computed(() => !!tasks.lists[ws.value])
const lists = computed(() => tasks.listsOf(ws.value))
const list = computed<TaskList | undefined>(() => lists.value.find((l) => l.id === routeList.value) ?? lists.value[0])

watch(
  ws,
  (w) => {
    if (w) tasks.load(w).catch((e) => toast.error(e.message))
  },
  { immediate: true },
)
// Assignees' avatars need the members (only worth it when shared).
watch(
  () => [ws.value, workspace.value?.member_count ?? 0] as const,
  ([w, n]) => {
    if (w && n > 1 && !wss.members[w]) void wss.loadMembers(w).catch(() => {})
  },
  { immediate: true },
)
const members = computed<Record<string, Member>>(() => Object.fromEntries((wss.members[ws.value] ?? []).map((m) => [m.user_id, m])))

// Keep the URL on the list shown (the first one when none or a gone one is asked for).
watch(
  [loaded, () => list.value?.id, routeList],
  ([ok, id, asked]) => {
    if (!ok || route.name !== 'ws-board' || (id ?? '') === asked) return
    void router.replace({ name: 'ws-board', params: { ws: ws.value, list: id ?? '' } })
  },
  { immediate: true },
)
function switchTo(id: string) {
  if (id !== list.value?.id) void router.replace({ name: 'ws-board', params: { ws: ws.value, list: id } })
}

// The workspace's filter (shared with Tasks, Timeline and Charts) hides
// cards that don't match.
const me = computed(() => auth.user?.id ?? '')
const { matches } = useTaskFilter(ws, lists, me)

// The cards of each column: top-level tasks by rank. A task whose column
// is gone (mid-update) shows in the first one.
const byColumn = computed<Record<string, Task[]>>(() => {
  const l = list.value
  if (!l) return {}
  const out: Record<string, Task[]> = Object.fromEntries(l.columns.map((c) => [c.id, [] as Task[]]))
  const first = l.columns[0]?.id
  for (const tk of tasks.tasksOf(ws.value)) {
    if (tk.list_id !== l.id || tk.parent_id !== null || !matches(tk)) continue
    const col = out[tk.column_id] ? tk.column_id : first
    if (col) out[col].push(tk)
  }
  for (const arr of Object.values(out)) arr.sort((a, b) => a.rank - b.rank)
  return out
})

// ---- Swimlanes: by person or by priority (chosen per browser) ----
type LaneBy = '' | 'person' | 'priority'
const laneKey = 'orbit_board_lanes'
const laneBy = ref<LaneBy>(readLanes())
function readLanes(): LaneBy {
  try {
    const v = localStorage.getItem(laneKey)
    return v === 'person' || v === 'priority' ? v : ''
  } catch {
    return ''
  }
}
watch(laneBy, (v) => {
  try {
    localStorage.setItem(laneKey, v)
  } catch {
    /* private window */
  }
})
// Lanes by person need the members (even alone in a workspace: "me").
watch(
  () => [laneBy.value, ws.value] as const,
  ([by, w]) => {
    if (by === 'person' && w && !wss.members[w]) void wss.loadMembers(w).catch(() => {})
  },
  { immediate: true },
)
const lanes = computed<Lane[]>(() => {
  if (laneBy.value === 'priority') return ['3', '2', '1', '0'].map((k) => ({ key: k, label: t(`board.lanes.priority${k}`) }))
  if (laneBy.value !== 'person') return []
  // The members with cards here first, then the others; "No one" last.
  const all = Object.values(byColumn.value).flat()
  const ids = new Set(all.flatMap((x) => x.assignee_ids))
  const people = Object.values(members.value)
  const ordered = [...people.filter((m) => ids.has(m.user_id)), ...people.filter((m) => !ids.has(m.user_id))]
  return [...ordered.map((m) => ({ key: m.user_id, label: m.display_name || m.username, member: m })), { key: '', label: t('board.lanes.noOne') }]
})
// laneCards: a cell's cards (a card shows in each of its people's lanes).
function laneCards(col: string, lane: string): Task[] {
  const cards = byColumn.value[col] ?? []
  if (laneBy.value === 'priority') return cards.filter((x) => String(x.priority) === lane)
  return cards.filter((x) => (lane ? x.assignee_ids.includes(lane) : !x.assignee_ids.length))
}
const totals = computed(() => Object.fromEntries(Object.entries(byColumn.value).map(([k, v]) => [k, v.length])))

// ---- Moving cards ----
const dragging = ref(false)
async function onMove(m: SortMove) {
  // In lanes, a cell is "column|lane": a new lane is a new person or priority.
  const [toCol, toLane] = m.toGroup.split('|')
  const [fromCol, fromLane] = m.fromGroup.split('|')
  if (laneBy.value && toLane !== undefined && toLane !== fromLane) await changeLane(m.id, fromLane ?? '', toLane)
  if (toCol === fromCol && toLane !== undefined && toLane !== fromLane) return
  m = { ...m, toGroup: toCol }
  const l = list.value
  const task = tasks.tasksOf(ws.value).find((x) => x.id === m.id)
  const col = l?.columns.find((c) => c.id === m.toGroup)
  if (readonly.value || !l || !task || !col) return
  // Show it in place at once, with a rank between its new neighbours; the
  // server's answer (its real rank) replaces it.
  const sibs = (byColumn.value[col.id] ?? []).filter((x) => x.id !== task.id)
  const at = m.beforeId ? sibs.findIndex((x) => x.id === m.beforeId) + 1 : 0
  const prev = sibs[at - 1]?.rank
  const next = sibs[at]?.rank
  const rank = prev === undefined ? (next === undefined ? 0 : next - 1) : next === undefined ? prev + 1 : (prev + next) / 2
  const before = { ...task }
  tasks.put(ws.value, {
    ...task,
    column_id: col.id,
    rank,
    done: col.done,
    completed_at: col.done ? (task.completed_at ?? new Date().toISOString()) : null,
  })
  try {
    await tasks.move(ws.value, task.id, col.id, m.beforeId || '^')
  } catch (e: any) {
    tasks.put(ws.value, before)
    toast.error(e.message)
  }
}

async function changeLane(id: string, from: string, to: string) {
  const task = tasks.tasksOf(ws.value).find((x) => x.id === id)
  if (!task || readonly.value) return
  const patch =
    laneBy.value === 'priority'
      ? { priority: Number(to) }
      : { assignee_ids: [...task.assignee_ids.filter((x) => x !== from && x !== to), ...(to ? [to] : [])] }
  try {
    await tasks.update(ws.value, id, patch)
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ---- Columns ----
function upsertList(l: TaskList) {
  const arr = (tasks.lists[ws.value] ??= [])
  const i = arr.findIndex((x) => x.id === l.id)
  if (i >= 0) arr[i] = l
  else arr.push(l)
}
// saveColumns shows the new columns at once and saves them; on an error
// (e.g. no done column left) the old ones come back. reload refetches the
// tasks the server moved or completed.
async function saveColumns(next: Column[], reload = false) {
  const l = list.value
  if (!l) return
  const prev = l.columns
  l.columns = next
  try {
    upsertList(await tasksApi.updateList(l.id, { columns: next }))
    if (reload) await tasks.load(ws.value)
  } catch (e: any) {
    l.columns = prev
    toast.error(e.message)
  }
}
const cols = computed(() => list.value?.columns ?? [])
function patchColumn(i: number, patch: Partial<Column>, reload = false) {
  void saveColumns(
    cols.value.map((c, j) => (j === i ? { ...c, ...patch } : c)),
    reload,
  )
}
function shiftColumn(i: number, by: -1 | 1) {
  const next = [...cols.value]
  const j = i + by
  if (j < 0 || j >= next.length) return
  ;[next[i], next[j]] = [next[j], next[i]]
  void saveColumns(next)
}

// Deleting a column: its cards go to the first open column left (else the
// first one), as the server does; ask first when it has any.
const toDelete = ref<Column | null>(null)
const deleteTarget = computed(() => {
  const rest = cols.value.filter((c) => c.id !== toDelete.value?.id)
  return rest.find((c) => !c.done) ?? rest[0]
})
function askRemove(c: Column) {
  if ((byColumn.value[c.id] ?? []).length) toDelete.value = c
  else void removeColumn(c)
}
async function removeColumn(c: Column) {
  toDelete.value = null
  await saveColumns(
    cols.value.filter((x) => x.id !== c.id),
    true,
  )
}

// "+ Add column" at the end of the board.
const addingCol = ref(false)
const colDraft = ref('')
const colBusy = ref(false)
const colInput = ref<HTMLInputElement | null>(null)
async function startAddColumn() {
  addingCol.value = true
  colDraft.value = ''
  await nextTick()
  colInput.value?.focus()
  trackEl.value?.scrollTo({ left: trackEl.value.scrollWidth, behavior: 'smooth' })
}
async function addColumn() {
  const l = list.value
  const name = colDraft.value.trim()
  if (!l || !name || colBusy.value) return
  colBusy.value = true
  try {
    upsertList(await tasksApi.updateList(l.id, { columns: [...l.columns, { id: '', name, done: false }] }))
    colDraft.value = ''
    await nextTick()
    trackEl.value?.scrollTo({ left: trackEl.value.scrollWidth, behavior: 'smooth' })
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    colBusy.value = false
    colInput.value?.focus()
  }
}
function onColBlur() {
  if (!colDraft.value.trim()) addingCol.value = false
}

// ---- Sheets ----
const sheetOpen = ref(false)
const sheetList = ref('')
function openSheet(id: string) {
  sheetList.value = id
  sheetOpen.value = true
}
function onSaved(l: TaskList) {
  if (!sheetList.value) switchTo(l.id)
}
const openTask = ref('')

// ---- Layout ----
// The board fills the window below the header; its columns scroll inside
// it (sideways for the board, down for a long column), so the page doesn't.
const headEl = ref<HTMLElement | null>(null)
const trackEl = ref<HTMLElement | null>(null)
const top = ref(0)
function measure() {
  if (trackEl.value) top.value = Math.round(trackEl.value.getBoundingClientRect().top + window.scrollY)
}
let ro: ResizeObserver | null = null
onMounted(() => {
  measure()
  window.addEventListener('resize', measure)
  ro = new ResizeObserver(measure)
  if (headEl.value) ro.observe(headEl.value)
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', measure)
  ro?.disconnect()
})
watch(trackEl, () => nextTick(measure))

// Keep the active list's tab in view in the strip.
const stripEl = ref<HTMLElement | null>(null)
watch(
  () => list.value?.id,
  async () => {
    await nextTick()
    stripEl.value?.querySelector('.on')?.scrollIntoView({ block: 'nearest', inline: 'nearest', behavior: 'smooth' })
  },
)
</script>

<template>
  <div class="flex min-w-0 flex-col">
    <!-- Header -->
    <header ref="headEl" class="px-4 pt-6 sm:px-8 sm:pt-8">
      <div class="flex items-end gap-3">
        <div class="min-w-0 flex-1">
          <h1 class="large-title truncate" :style="list ? { color: list.color } : {}">{{ list?.name ?? t('modules.board') }}</h1>
          <p v-if="list" class="subhead mt-0.5 tabular-nums">{{ t('board.openCount', { n: list.open_count }) }}</p>
        </div>
        <div v-if="loaded && list" class="mb-1 flex shrink-0 items-center gap-1">
          <label class="lanes-pick" :title="t('board.lanes.title')">
            <Rows3 class="h-3.5 w-3.5" />
            <select v-model="laneBy" :aria-label="t('board.lanes.title')" data-testid="board-lanes-by">
              <option value="">{{ t('board.lanes.none') }}</option>
              <option value="person">{{ t('board.lanes.person') }}</option>
              <option value="priority">{{ t('board.lanes.priority') }}</option>
            </select>
          </label>
          <TaskFilter :ws="ws" />
        </div>
        <span v-if="readonly && loaded" class="pill mb-1 shrink-0"><Eye class="h-3 w-3" />{{ t('board.readOnly') }}</span>
        <template v-else-if="loaded">
          <button v-if="list" class="btn btn-sm mb-1 shrink-0 !px-2 sm:!px-2.5" :title="t('board.editList')" :aria-label="t('board.editList')" @click="openSheet(list.id)">
            <SlidersHorizontal class="h-3.5 w-3.5" /><span class="hidden sm:inline">{{ t('board.editList') }}</span>
          </button>
          <button class="btn-tinted btn-sm mb-1 shrink-0 !px-2 sm:!px-2.5" :title="t('board.newList')" :aria-label="t('board.newList')" @click="openSheet('')">
            <Plus class="h-4 w-4" /><span class="hidden sm:inline">{{ t('board.newList') }}</span>
          </button>
        </template>
      </div>

      <nav v-if="lists.length > 1" ref="stripEl" class="strip -mx-4 mt-4 overflow-x-auto px-4 sm:-mx-8 sm:px-8" :aria-label="t('board.lists')">
        <div class="seg" role="tablist">
          <button
            v-for="l in lists"
            :key="l.id"
            role="tab"
            :aria-selected="l.id === list?.id"
            class="inline-flex items-center gap-1.5"
            :class="{ on: l.id === list?.id }"
            @click="switchTo(l.id)"
          >
            <span class="h-2 w-2 shrink-0 rounded-full" :style="{ background: l.color }" />
            <span class="max-w-[180px] truncate">{{ l.name }}</span>
            <span v-if="l.open_count" class="text-[12px] tabular-nums text-text-3">{{ l.open_count }}</span>
          </button>
        </div>
      </nav>
    </header>

    <!-- Loading -->
    <div v-if="!loaded" class="track mt-5" aria-busy="true">
      <div v-for="n in 3" :key="n" class="skeleton" :style="{ height: `${220 - n * 40}px` }" />
    </div>

    <!-- No lists -->
    <div v-else-if="!list" class="mx-auto mt-16 max-w-sm px-6 text-center sm:mt-24">
      <span class="tile mx-auto h-16 w-16 !rounded-2xl" style="--tile: var(--ws)"><Columns3 class="h-8 w-8" :stroke-width="2" /></span>
      <h2 class="title-2 mt-5">{{ t('board.emptyTitle') }}</h2>
      <p class="subhead mt-1.5 leading-relaxed">{{ readonly ? t('board.emptyReadOnly') : t('board.emptyHint') }}</p>
      <button v-if="!readonly" class="btn-primary mt-6" @click="openSheet('')"><Plus class="h-4 w-4" />{{ t('board.createList') }}</button>
    </div>

    <!-- Swimlanes -->
    <div v-else-if="laneBy" ref="trackEl" class="track lanes-track mt-5" :class="{ dragging }" :style="{ height: `calc(100dvh - ${top}px - var(--board-bottom))` }">
      <BoardLanes
        :ws="ws"
        :list-id="list.id"
        :columns="list.columns"
        :lanes="lanes"
        :cards="laneCards"
        :totals="totals"
        :color="list.color"
        :members="members"
        :readonly="readonly"
        :by="laneBy"
        @open="openTask = $event"
        @move="onMove"
        @dragging="dragging = $event"
      />
    </div>

    <!-- Columns -->
    <div v-else ref="trackEl" class="track board mt-5" :class="{ dragging }" :style="{ height: `calc(100dvh - ${top}px - var(--board-bottom))` }">
      <BoardColumn
        v-for="(c, i) in list.columns"
        :key="`${list.id}:${c.id}:${readonly ? 'ro' : 'rw'}`"
        :ws="ws"
        :list-id="list.id"
        :column="c"
        :index="i"
        :count="list.columns.length"
        :cards="byColumn[c.id] ?? []"
        :color="list.color"
        :members="members"
        :readonly="readonly"
        @open="openTask = $event"
        @move="onMove"
        @dragging="dragging = $event"
        @rename="patchColumn(i, { name: $event })"
        @recolor="patchColumn(i, { color: $event })"
        @toggle-done="patchColumn(i, { done: !c.done }, true)"
        @shift="shiftColumn(i, $event)"
        @limit="patchColumn(i, { limit: $event })"
        @remove="askRemove(c)"
      />

      <div v-if="!readonly && list.columns.length < 12" class="add-col">
        <div v-if="addingCol" class="card p-2">
          <input
            ref="colInput"
            v-model="colDraft"
            class="field"
            maxlength="60"
            :placeholder="t('board.columnName')"
            :aria-label="t('board.columnName')"
            @keydown.enter.prevent="addColumn"
            @keydown.esc.stop.prevent="addingCol = false"
            @blur="onColBlur"
          />
          <div class="mt-2 flex items-center gap-1.5">
            <button class="btn-primary btn-sm" :disabled="!colDraft.trim() || colBusy" @mousedown.prevent @click="addColumn">{{ t('board.addColumn') }}</button>
            <button class="icon-btn h-7 w-7" :aria-label="t('common.close')" @mousedown.prevent @click="addingCol = false"><X class="h-4 w-4" /></button>
          </div>
        </div>
        <button v-else class="add-col-btn" @click="startAddColumn"><Plus class="h-4 w-4" />{{ t('board.addColumn') }}</button>
      </div>
      <!-- Trailing room so the last column doesn't touch the edge. -->
      <span class="end" aria-hidden="true" />
    </div>

    <ListSheet :open="sheetOpen" :ws="ws" :list-id="sheetList" @close="sheetOpen = false" @saved="onSaved" />
    <TaskDetailSheet :open="!!openTask" :ws="ws" :task-id="openTask" @close="openTask = ''" />
    <Confirm
      :open="!!toDelete"
      :title="t('board.deleteColumnTitle', { name: toDelete?.name ?? '' })"
      :message="t('board.deleteColumnBody', { to: deleteTarget?.name ?? '' })"
      :confirm-label="t('common.delete')"
      danger
      @confirm="toDelete && removeColumn(toDelete)"
      @close="toDelete = null"
    />
  </div>
</template>

<style scoped>
.strip {
  scrollbar-width: none;
}
.lanes-pick {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 30px;
  padding: 0 4px 0 9px;
  border-radius: 9px;
  background: var(--fill-2);
  color: var(--text-2);
  font-size: 13px;
}
.lanes-pick select {
  background: transparent;
  outline: none;
  color: var(--text);
  cursor: pointer;
}
/* Lanes scroll both ways inside the page. */
.lanes-track {
  overflow: auto;
  align-items: stretch;
}
.strip::-webkit-scrollbar {
  display: none;
}
.strip .seg button {
  padding-left: 11px;
  padding-right: 11px;
}

/* The columns' track: side by side, scrolling sideways inside the page. */
.track {
  --board-bottom: 16px;
  display: flex;
  align-items: flex-start;
  gap: 14px;
  min-height: 320px;
  padding: 0 16px 4px;
  overflow-x: auto;
  overflow-y: hidden;
  overscroll-behavior-x: contain;
}
@media (min-width: 640px) {
  .track {
    --board-bottom: 24px;
    padding: 0 32px 6px;
  }
}
@media (max-width: 639px) {
  .track {
    /* Above the tab bar. */
    --board-bottom: calc(58px + env(safe-area-inset-bottom) + 12px);
    gap: 12px;
    scroll-snap-type: x mandatory;
    scroll-padding: 0 16px;
  }
  .track.dragging {
    scroll-snap-type: none;
  }
}
.end {
  flex: none;
  width: 2px;
  height: 1px;
}
@media (min-width: 640px) {
  .end {
    width: 18px;
  }
}

.add-col {
  flex: none;
  width: 288px;
  scroll-snap-align: start;
}
@media (max-width: 639px) {
  .add-col {
    width: 85vw;
    max-width: 360px;
  }
}
.add-col-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  min-height: 44px;
  padding: 0 14px;
  border-radius: 14px;
  border: 1.5px dashed var(--border-2);
  font-size: 14px;
  font-weight: 500;
  color: var(--text-2);
  transition: background var(--speed) var(--ease), color var(--speed) var(--ease), border-color var(--speed) var(--ease);
}
.add-col-btn:hover {
  background: var(--fill-2);
  border-color: transparent;
  color: var(--text);
}

.skeleton {
  flex: none;
  width: 288px;
  border-radius: 14px;
  background: var(--fill-2);
  animation: pulse 1.4s var(--ease) infinite alternate;
}
@media (max-width: 639px) {
  .skeleton {
    width: 85vw;
  }
}
@keyframes pulse {
  from {
    opacity: 1;
  }
  to {
    opacity: 0.45;
  }
}
</style>
