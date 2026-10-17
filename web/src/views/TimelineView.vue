<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Eye, EyeOff, CalendarRange, ListFilter } from 'lucide-vue-next'
import TimelineChart from '@/components/timeline/TimelineChart.vue'
import TaskFilter from '@/components/tasks/TaskFilter.vue'
import TaskDetailSheet from '@/components/tasks/TaskDetailSheet.vue'
import { useFillHeight } from '@/components/calendar/useFillHeight'
import { useTasksStore } from '@/stores/tasks'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { useTaskFilter } from '@/lib/taskFilter'
import { todayStr, weekStart, isOverdue, relDay } from '@/lib/dates'
import { ZOOMS, ZOOM_PX, dayNum, rangeFor, spanOf, patchFor, type Span, type TlGroup, type TlRow, type Zoom } from '@/lib/timeline'
import type { Member, Task } from '@/api/planner'

// Timeline: the workspace's tasks as bars on a Gantt chart, grouped by list,
// from their start to their due date. Zoom by weeks, months or quarters;
// editors drag bars to reschedule, click an undated row to schedule it and
// add tasks at the end of each list. The shared task filter applies.
const route = useRoute()
const tasks = useTasksStore()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws ?? ''))
const workspace = computed(() => wss.byId(ws.value))
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only)

// ---- Data ----
const loaded = computed(() => !!tasks.lists[ws.value])
const lists = computed(() => tasks.listsOf(ws.value))
const all = computed(() => tasks.tasksOf(ws.value))
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
const members = computed(() => new Map<string, Member>((wss.members[ws.value] ?? []).map((m) => [m.user_id, m])))
const shared = computed(() => (workspace.value?.member_count ?? 1) > 1)

const me = computed(() => auth.user?.id ?? '')
const { active, matches, clear } = useTaskFilter(ws, lists, me)

// ---- Preferences ----
const phone = window.matchMedia('(max-width: 767px)').matches
function stored<T extends string>(key: string, ok: readonly T[]): T | null {
  try {
    const v = localStorage.getItem(key) as T | null
    return v && ok.includes(v) ? v : null
  } catch {
    return null
  }
}
function store(key: string, v: string) {
  try {
    localStorage.setItem(key, v)
  } catch {
    /* private mode */
  }
}
const zoom = ref<Zoom>(stored('orbit_tl_zoom', ZOOMS) ?? (phone ? 'weeks' : 'months'))
watch(zoom, (z) => store('orbit_tl_zoom', z))
const px = computed(() => ZOOM_PX[zoom.value])
// Shared with the Tasks view.
const showDone = ref(stored('orbit_show_done', ['1']) === '1')
watch(showDone, (v) => store('orbit_show_done', v ? '1' : '0'))
// Collapsed lists (by id, across workspaces).
function storedIds(key: string): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(key) || '[]')
    return Array.isArray(v) ? v.filter((x) => typeof x === 'string') : []
  } catch {
    return []
  }
}
const collapsed = ref(new Set(storedIds('orbit_tl_collapsed')))
function collapse(id: string) {
  const next = new Set(collapsed.value)
  if (!next.delete(id)) next.add(id)
  collapsed.value = next
  store('orbit_tl_collapsed', JSON.stringify([...next]))
}
// Tasks ticked a moment ago stay in view briefly, so ticking feels done.
const justDone = ref(new Set<string>())

// ---- Rows ----
const todayS = todayStr()
const today = dayNum(todayS)
const subs = computed(() => {
  const m = new Map<string, { done: number; total: number }>()
  for (const x of all.value) {
    if (!x.parent_id) continue
    const s = m.get(x.parent_id) ?? { done: 0, total: 0 }
    s.total++
    if (x.done) s.done++
    m.set(x.parent_id, s)
  }
  return m
})
const NONE = { done: 0, total: 0 }
const groups = computed<TlGroup[]>(() => {
  const byList = new Map<string, TlRow[]>()
  for (const x of all.value) {
    if (x.parent_id || (x.done && !showDone.value && !justDone.value.has(x.id)) || !matches(x)) continue
    const row: TlRow = {
      task: x,
      span: spanOf(x),
      subs: subs.value.get(x.id) ?? NONE,
      overdue: isOverdue(x.due, x.due_time, x.done),
      assignees: shared.value ? x.assignee_ids.map((id) => members.value.get(id)).filter((m): m is Member => !!m) : [],
    }
    const arr = byList.get(x.list_id) ?? []
    arr.push(row)
    byList.set(x.list_id, arr)
  }
  // Dated tasks in order of their bars (a cascade), then the undated ones.
  const order = (a: TlRow, b: TlRow) =>
    !a.span || !b.span ? (a.span ? -1 : b.span ? 1 : a.task.rank - b.task.rank) : a.span.from - b.span.from || a.span.to - b.span.to || a.task.rank - b.task.rank
  return lists.value
    .map((l) => {
      const rows = (byList.get(l.id) ?? []).sort(order)
      let span: Span | null = null
      for (const r of rows) if (r.span) span = span ? { from: Math.min(span.from, r.span.from), to: Math.max(span.to, r.span.to) } : { ...r.span }
      return { list: l, rows, span }
    })
    .filter((g) => g.rows.length || !active.value)
})
const rows = computed(() => groups.value.flatMap((g) => g.rows))
const counts = computed(() => ({
  all: rows.value.length,
  undated: rows.value.filter((r) => !r.span).length,
  overdue: rows.value.filter((r) => r.overdue).length,
}))

// ---- The range of days ----
// Generous around today and the bars; it grows as you scroll near an end.
const extra = ref({ before: 0, after: 0 })
watch(ws, () => (extra.value = { before: 0, after: 0 }))
const range = computed<Span>(() => {
  const spans = rows.value.flatMap((r) => (r.span ? [r.span] : []))
  const r = rangeFor(today, spans, weekStart())
  return { from: r.from - extra.value.before, to: r.to + extra.value.after }
})
function extend(side: 'before' | 'after') {
  const days = Math.ceil(Math.max(56, 1600 / px.value) / 7) * 7
  extra.value = { ...extra.value, [side]: extra.value[side] + days }
}

const chart = ref<InstanceType<typeof TimelineChart> | null>(null)
function goToday() {
  chart.value?.scrollToDay(today, 0.25, true)
}

// ---- Actions ----
async function toggle(x: Task) {
  if (!editable.value) return
  const done = !x.done
  if (done) {
    justDone.value.add(x.id)
    setTimeout(() => justDone.value.delete(x.id), 1400)
  }
  try {
    const r = await tasks.update(ws.value, x.id, { completed: done })
    if (r.repeated) toast.success(t('tasks.repeated', { date: relDay(r.task.due!) }))
  } catch (e: any) {
    toast.error(e.message)
  }
}

// schedule gives a task new dates: shown at once, undone if the server says no.
const flash = ref('')
let flashTimer = 0
async function schedule(x: Task, span: Span) {
  if (!editable.value) return
  const patch = patchFor(x, span)
  const before = { ...x }
  tasks.put(ws.value, { ...x, start: patch.start ?? x.start, due: patch.due ?? x.due })
  flash.value = x.id
  clearTimeout(flashTimer)
  flashTimer = window.setTimeout(() => (flash.value = ''), 900)
  try {
    await tasks.update(ws.value, x.id, patch)
  } catch (e: any) {
    tasks.put(ws.value, before)
    toast.error(e.message)
  }
}
onBeforeUnmount(() => clearTimeout(flashTimer))

async function create(listId: string, title: string) {
  try {
    await tasks.create(ws.value, { title, list_id: listId, start: todayS, due: todayS })
  } catch (e: any) {
    toast.error(e.message)
  }
}

const openTask = ref('')

// ---- Layout: the chart fills the window below the header ----
const frame = ref<HTMLElement | null>(null)
useFillHeight(frame, 320)
</script>

<template>
  <div class="tl-page flex min-w-0 flex-col">
    <header class="flex flex-wrap items-end gap-x-4 gap-y-3 px-4 pt-6 sm:px-8 sm:pt-8">
      <div class="min-w-0 flex-1">
        <div class="flex min-w-0 items-center gap-2">
          <h1 class="large-title tl-title truncate">{{ t('timeline.title') }}</h1>
          <span v-if="loaded && !editable" class="pill mt-1 shrink-0"><Eye class="h-3 w-3" />{{ t('timeline.readOnly') }}</span>
        </div>
        <p class="subhead mt-0.5 truncate tabular-nums">
          <template v-if="loaded">
            {{ t('timeline.count', { n: counts.all }, counts.all) }}
            <template v-if="counts.overdue"> · <span class="text-crit-text">{{ t('timeline.overdue', { n: counts.overdue }) }}</span></template>
            <template v-if="counts.undated"> · {{ t('timeline.unscheduled', { n: counts.undated }) }}</template>
          </template>
          <template v-else>&nbsp;</template>
        </p>
      </div>

      <!-- Filter and completed (beside the title on phones too) -->
      <div class="order-1 mb-0.5 flex items-center gap-1.5 md:order-2">
        <TaskFilter :ws="ws" show-lists />
        <button class="icon-btn shrink-0" :class="{ on: showDone }" :title="showDone ? t('tasks.hideDone') : t('tasks.showDone')" :aria-label="showDone ? t('tasks.hideDone') : t('tasks.showDone')" :aria-pressed="showDone" @click="showDone = !showDone">
          <component :is="showDone ? EyeOff : Eye" class="h-[18px] w-[18px]" />
        </button>
      </div>
      <!-- Zoom and Today (their own row on phones) -->
      <div class="zoom-row order-2 flex w-full items-center gap-1.5 md:order-1 md:mb-0.5 md:w-auto">
        <div class="seg zoom" role="tablist" :aria-label="t('timeline.zoom')">
          <button v-for="z in ZOOMS" :key="z" role="tab" :aria-selected="zoom === z" :class="{ on: zoom === z }" @click="zoom = z">{{ t(`timeline.zooms.${z}`) }}</button>
        </div>
        <button class="btn btn-sm shrink-0" @click="goToday">{{ t('dates.today') }}</button>
      </div>
    </header>

    <div ref="frame" class="frame mx-4 mt-4 sm:mx-8 sm:mt-5">
      <!-- Loading -->
      <div v-if="!loaded" class="h-full p-4" aria-busy="true">
        <div v-for="n in 6" :key="n" class="skeleton" :style="{ width: `${30 + ((n * 37) % 50)}%`, marginLeft: `${(n * 13) % 40}%` }" />
      </div>

      <!-- No lists -->
      <div v-else-if="!lists.length" class="empty">
        <span class="tile mx-auto h-16 w-16 !rounded-2xl" style="--tile: var(--ws)"><CalendarRange class="h-8 w-8" :stroke-width="2" /></span>
        <h2 class="title-2 mt-5">{{ t('timeline.noLists') }}</h2>
        <p class="subhead mt-1.5 leading-relaxed">{{ t('timeline.noListsHint') }}</p>
      </div>

      <!-- The filter hides everything -->
      <div v-else-if="active && !rows.length" class="empty">
        <ListFilter class="mx-auto mb-3 h-10 w-10 text-text-faint" :stroke-width="1.5" />
        <p class="title-3">{{ t('tasks.noMatch') }}</p>
        <p class="subhead mt-1">{{ t('tasks.noMatchHint') }}</p>
        <button class="btn btn-sm mt-4" @click="clear">{{ t('filter.clear') }}</button>
      </div>

      <TimelineChart
        v-else
        ref="chart"
        class="h-full"
        :groups="groups"
        :range="range"
        :px="px"
        :zoom="zoom"
        :today="today"
        :week-start="weekStart()"
        :editable="editable"
        :collapsed="collapsed"
        :flash="flash"
        @toggle="toggle"
        @open="openTask = $event"
        @schedule="schedule"
        @create="create"
        @collapse="collapse"
        @extend="extend"
      />
    </div>

    <TaskDetailSheet :open="!!openTask" :ws="ws" :task-id="openTask" @close="openTask = ''" />
  </div>
</template>

<style scoped>
.tl-page {
  --cal-bottom: 20px;
}
@media (min-width: 640px) {
  .tl-page {
    --cal-bottom: 28px;
  }
}
@media (max-width: 639px) {
  .tl-page {
    /* Above the tab bar. */
    --cal-bottom: calc(58px + env(safe-area-inset-bottom) + 12px);
  }
  .tl-title {
    font-size: 26px;
  }
}
@media (max-width: 767px) {
  .seg.zoom {
    flex: 1;
    min-width: 0;
  }
  .seg.zoom button {
    flex: 1;
    padding-left: 6px;
    padding-right: 6px;
  }
}
.seg.zoom button:not(.on):hover {
  background: var(--surface-hover);
}
.frame {
  height: calc(100dvh - 200px);
  min-height: 320px;
  overflow: hidden;
  border-radius: var(--r-lg);
  background: var(--bg);
  box-shadow: 0 0 0 0.5px var(--border), var(--shadow-sm);
}
html.dark .frame {
  box-shadow: 0 0 0 0.5px var(--border);
}
.empty {
  max-width: 24rem;
  margin: 0 auto;
  padding: 72px 24px;
  text-align: center;
}
.skeleton {
  height: 22px;
  margin: 14px 0;
  border-radius: 7px;
  background: var(--fill-2);
  animation: pulse 1.4s var(--ease) infinite alternate;
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
