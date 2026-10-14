<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ChevronLeft, ChevronRight, Plus, CalendarDays } from 'lucide-vue-next'
import MonthGrid from '@/components/calendar/MonthGrid.vue'
import TimeGrid from '@/components/calendar/TimeGrid.vue'
import DayAgenda from '@/components/calendar/DayAgenda.vue'
import EventSheet from '@/components/calendar/EventSheet.vue'
import EventDetail from '@/components/calendar/EventDetail.vue'
import SubscriptionsSheet from '@/components/calendar/SubscriptionsSheet.vue'
import TaskDetailSheet from '@/components/tasks/TaskDetailSheet.vue'
import { toEntry, type CalView, type Entry, type TaskItem } from '@/components/calendar/model'
import { useFillHeight } from '@/components/calendar/useFillHeight'
import { calendarApi, tasksApi, type CalendarData, type EventInput, type Occurrence, type Workspace } from '@/api/planner'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useTasksStore } from '@/stores/tasks'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { onLiveEvent } from '@/lib/live'
import { CLIENT_ID } from '@/lib/api'
import {
  monthGrid, monthTitle, startOfWeek, startOfDay, addDays, addMonths, dateStr, parseDate, fmtDate, relDay, utcMidnight,
  allDayDate, deviceTZ,
} from '@/lib/dates'

// Calendar: one workspace (/w/:ws/calendar) or every workspace merged
// (/calendar, events in their workspace's colour). Month, Week and Day like
// macOS Calendar; on phones the month shows dots with the chosen day's list
// below, like iOS. Events open in the event sheet (or read-only when they
// can't be changed), due tasks can be ticked off where they are.
const route = useRoute()
const wss = useWorkspacesStore()
const tasksStore = useTasksStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

// ---- Where, and what may be changed ----
const wsId = computed(() => (typeof route.params.ws === 'string' ? route.params.ws : ''))
const workspace = computed(() => (wsId.value ? wss.byId(wsId.value) : undefined))
const canEditWs = (w?: Workspace) => !!w && w.role !== 'viewer' && !w.archived_at && !auth.privileges.read_only
// The workspaces a new event can go to from the merged calendar.
const choices = computed(() => (wsId.value ? [] : wss.active.filter((w) => canEditWs(w) && w.modules.includes('calendar'))))
const canCreate = computed(() => (wsId.value ? canEditWs(workspace.value) : choices.value.length > 0))
const readOnly = computed(() => !!wsId.value && !!workspace.value && !canEditWs(workspace.value))
const showSubs = computed(() => !!wsId.value && canEditWs(workspace.value) && auth.features.calendar_subscriptions !== false)
function defaultWs(): string {
  if (wsId.value) return wsId.value
  let saved = ''
  try {
    saved = localStorage.getItem('orbit_cal_ws') || ''
  } catch {
    /* private mode */
  }
  for (const id of [saved, wss.lastId]) if (id && choices.value.some((w) => w.id === id)) return id
  return choices.value[0]?.id ?? ''
}
// The merged calendar's key: which colour is which workspace.
const legend = computed(() => (wsId.value ? [] : wss.active.filter((w) => w.modules.includes('calendar'))))

// ---- Phone or not ----
const phoneMq = window.matchMedia('(max-width: 639px)')
const phone = ref(phoneMq.matches)
const onPhone = (e: MediaQueryListEvent) => (phone.value = e.matches)

// ---- The view, remembered (separately on phones, where Month is the list) ----
const viewKey = computed(() => (phone.value ? 'orbit_cal_view_phone' : 'orbit_cal_view'))
function readView(): CalView {
  try {
    const v = localStorage.getItem(viewKey.value)
    if (v === 'day' || v === 'week' || v === 'month') return v
  } catch {
    /* private mode */
  }
  return 'month'
}
const view = ref<CalView>(readView())
watch(phone, () => (view.value = readView()))
function setView(v: CalView) {
  view.value = v
  try {
    localStorage.setItem(viewKey.value, v)
  } catch {
    /* private mode */
  }
}
const agenda = computed(() => phone.value && view.value === 'month')

// ---- The day in focus ----
const cursor = ref(startOfDay(new Date()))
const selected = computed(() => dateStr(cursor.value))
watch(
  () => route.query.date,
  (d) => {
    if (typeof d === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(d)) cursor.value = parseDate(d)
  },
  { immediate: true },
)
function shift(n: number) {
  if (view.value === 'month') {
    // A month opens on its 1st, or on today in the current month.
    const m = addMonths(cursor.value, n)
    const now = new Date()
    cursor.value = m.getMonth() === now.getMonth() && m.getFullYear() === now.getFullYear() ? startOfDay(now) : m
  } else cursor.value = addDays(cursor.value, view.value === 'week' ? 7 * n : n)
}
function goToday() {
  cursor.value = startOfDay(new Date())
}
function select(day: string) {
  cursor.value = parseDate(day)
}
function openDay(day: string) {
  cursor.value = parseDate(day)
  setView('day')
}

const range = computed(() => {
  if (view.value === 'month') {
    const g = monthGrid(cursor.value)
    return { start: g[0], end: addDays(g[41], 1) }
  }
  const start = view.value === 'week' ? startOfWeek(cursor.value) : cursor.value
  return { start, end: addDays(start, view.value === 'week' ? 7 : 1) }
})
const gridDays = computed(() => Array.from({ length: view.value === 'week' ? 7 : 1 }, (_, i) => addDays(range.value.start, i)))

const title = computed(() => monthTitle(cursor.value))
const subtitle = computed(() => {
  if (view.value === 'week') {
    const last = addDays(range.value.end, -1)
    return `${fmtDate(range.value.start, { day: 'numeric', month: 'short' })} – ${fmtDate(last, { day: 'numeric', month: 'short', year: 'numeric' })}`
  }
  if (view.value === 'day') {
    const rel = relDay(selected.value)
    const full = fmtDate(cursor.value, { weekday: 'long', day: 'numeric', month: 'long' })
    return rel.length < full.length && !full.toLowerCase().startsWith(rel.toLowerCase()) ? `${rel} · ${full}` : full
  }
  return ''
})

// ---- Data ----
const data = ref<CalendarData>({ events: [], tasks: [] })
let seq = 0
async function load() {
  const { start, end } = range.value
  const mine = ++seq
  // A day either side: all-day events are UTC dates, which may fall a day
  // off in the local zone at the edges.
  const p = {
    from: addDays(start, -1).toISOString(),
    to: addDays(end, 1).toISOString(),
    from_date: dateStr(start),
    to_date: dateStr(addDays(end, -1)),
  }
  try {
    const d = wsId.value ? await calendarApi.workspace(wsId.value, p) : await calendarApi.mine(p)
    if (mine === seq) data.value = d
  } catch (e: any) {
    if (mine === seq) toast.error(e.message)
  }
}
watch(wsId, () => (data.value = { events: [], tasks: [] }))
watch(() => `${wsId.value}|${dateStr(range.value.start)}|${dateStr(range.value.end)}`, load, { immediate: true })

const entries = computed(() => data.value.events.map((o) => toEntry(o, canEditWs(wss.byId(o.workspace_id)))))
const taskItems = computed<TaskItem[]>(() =>
  data.value.tasks.map((task) => {
    const w = wss.byId(task.workspace_id)
    return { task, color: w?.color ?? 'var(--ws)', editable: canEditWs(w) }
  }),
)

// Live: someone changed events, tasks or subscriptions of a workspace shown.
let liveTimer: ReturnType<typeof setTimeout> | null = null
const offLive = onLiveEvent('ws', (e) => {
  if (e.origin === CLIENT_ID || !['events', 'tasks', 'subscriptions'].includes(String(e.what))) return
  const ws = String(e.ws)
  if (wsId.value ? ws !== wsId.value : !wss.byId(ws)) return
  if (e.what === 'subscriptions') subsLoaded.clear()
  if (liveTimer) clearTimeout(liveTimer)
  liveTimer = setTimeout(load, 250)
})

// ---- Events: create, open, move ----
const sheetOpen = ref(false)
const sheetOcc = ref<Occurrence | null>(null)
const sheetDraft = ref<{ start: Date; end: Date; allDay: boolean } | null>(null)
const sheetWs = ref('')
function create(r: { start: Date; end: Date; allDay: boolean }) {
  if (!canCreate.value) return
  sheetOcc.value = null
  sheetDraft.value = r
  sheetWs.value = defaultWs()
  sheetOpen.value = true
}
// createOn: a new event on a day, at the next hour today, else at 9:00.
function createOn(day: string) {
  const d = parseDate(day)
  const now = new Date()
  const h = day === dateStr(now) ? Math.min(23, now.getHours() + 1) : 9
  const start = new Date(d.getFullYear(), d.getMonth(), d.getDate(), h)
  create({ start, end: new Date(start.getTime() + 3600000), allDay: false })
}

const detailOpen = ref(false)
const detail = ref<Entry | null>(null)
const subNames = ref<Record<string, string>>({})
const subsLoaded = new Set<string>()
async function loadSubNames(ws: string) {
  if (subsLoaded.has(ws)) return
  subsLoaded.add(ws)
  try {
    for (const s of await calendarApi.subscriptions(ws)) subNames.value[s.id] = s.name
  } catch {
    subsLoaded.delete(ws)
  }
}
const detailName = computed(() => {
  const e = detail.value
  if (!e) return ''
  const wsName = wss.byId(e.occ.workspace_id)?.name ?? ''
  const sub = e.occ.subscription_id ? subNames.value[e.occ.subscription_id] : ''
  if (!sub) return wsName
  return wsId.value ? sub : `${sub} · ${wsName}`
})
function openEntry(e: Entry) {
  if (e.editable) {
    sheetOcc.value = e.occ
    sheetDraft.value = null
    sheetWs.value = e.occ.workspace_id
    sheetOpen.value = true
    return
  }
  detail.value = e
  detailOpen.value = true
  if (e.occ.kind === 'external') void loadSubNames(e.occ.workspace_id)
}

// Dragging a single event to another day keeps its time and length.
async function moveEntry(e: Entry, days: number) {
  const o = e.occ
  const body: EventInput = o.all_day
    ? {
        starts_at: utcMidnight(dateStr(addDays(parseDate(allDayDate(o.start)), days))),
        ends_at: utcMidnight(dateStr(addDays(parseDate(allDayDate(o.end)), days))),
      }
    : { starts_at: addDays(e.start, days).toISOString(), ends_at: addDays(e.end, days).toISOString(), tz: deviceTZ() }
  const prev = { start: o.start, end: o.end }
  o.start = body.starts_at!
  o.end = body.ends_at!
  try {
    await calendarApi.updateEvent(o.id, body)
  } catch (err: any) {
    o.start = prev.start
    o.end = prev.end
    toast.error(err.message)
  }
  void load()
}

// ---- Tasks ----
const taskOpen = ref<{ ws: string; id: string } | null>(null)
async function openTask(x: TaskItem) {
  const ws = x.task.workspace_id
  try {
    if (!tasksStore.tasks[ws]) await tasksStore.load(ws)
  } catch {
    /* the sheet loads what it can */
  }
  // Long-done tasks aren't in the store's list: give the sheet this one.
  if (!tasksStore.tasksOf(ws).some((tk) => tk.id === x.task.id)) tasksStore.put(ws, { ...x.task })
  taskOpen.value = { ws, id: x.task.id }
}
function closeTask() {
  taskOpen.value = null
  void load()
}
async function toggleTask(x: TaskItem) {
  const tk = x.task
  const done = !tk.done
  tk.done = done
  try {
    // Through the store when it holds this workspace (Tasks and Board then
    // update too); a partial list must not be seeded into it otherwise.
    const ws = tk.workspace_id
    const r = tasksStore.tasks[ws] ? await tasksStore.update(ws, tk.id, { completed: done }) : await tasksApi.update(tk.id, { completed: done })
    if (r.repeated && r.task.due) toast.success(t('cal.taskRepeated', { date: relDay(r.task.due) }))
  } catch (e: any) {
    tk.done = !done
    toast.error(e.message)
  }
  void load()
}

// ---- Subscribed calendars ----
const subsOpen = ref(false)
function onSubsChanged() {
  subsLoaded.clear()
  void load()
}

// ---- Keyboard: ← → to move, T today, D / W / M for the view ----
function onKey(e: KeyboardEvent) {
  if (e.metaKey || e.ctrlKey || e.altKey || e.defaultPrevented) return
  const el = e.target as HTMLElement | null
  if (el?.closest('input, textarea, select, [contenteditable]') || document.querySelector('[role="dialog"]')) return
  const k = e.key.toLowerCase()
  if (k === 'arrowleft') shift(-1)
  else if (k === 'arrowright') shift(1)
  else if (k === 't') goToday()
  else if (k === 'd') setView('day')
  else if (k === 'w') setView('week')
  else if (k === 'm') setView('month')
  else return
  e.preventDefault()
}

const fill = ref<HTMLElement | null>(null)
useFillHeight(fill)

onMounted(() => {
  phoneMq.addEventListener('change', onPhone)
  window.addEventListener('keydown', onKey)
})
onUnmounted(() => {
  phoneMq.removeEventListener('change', onPhone)
  window.removeEventListener('keydown', onKey)
  offLive()
  if (liveTimer) clearTimeout(liveTimer)
})

const views: CalView[] = ['day', 'week', 'month']
</script>

<template>
  <div class="cal-page px-4 pt-6 sm:px-8 sm:pt-8" :class="{ 'pb-6': agenda }">
    <header class="flex flex-wrap items-center gap-x-4 gap-y-3">
      <div class="min-w-0 flex-1">
        <div class="flex min-w-0 items-center gap-2">
          <h1 class="large-title cal-title truncate">{{ title }}</h1>
          <span v-if="readOnly" class="pill shrink-0">{{ t('cal.viewOnly') }}</span>
        </div>
        <p v-if="subtitle" class="subhead mt-0.5 truncate">{{ subtitle }}</p>
      </div>

      <div class="flex shrink-0 items-center gap-1.5">
        <button v-if="showSubs" class="btn btn-sm" :title="t('cal.calendars')" @click="subsOpen = true">
          <CalendarDays class="h-4 w-4" /><span class="hidden sm:inline">{{ t('cal.calendars') }}</span>
        </button>
        <button
          v-if="!readOnly"
          class="icon-btn !text-accent-text"
          :disabled="!canCreate"
          :title="canCreate ? t('cal.newEvent') : t('cal.noWritable')"
          :aria-label="t('cal.newEvent')"
          @click="createOn(selected)"
        >
          <Plus class="h-5 w-5" :stroke-width="2.2" />
        </button>
      </div>

      <div class="flex w-full items-center justify-between gap-2 sm:w-auto">
        <div class="seg views" role="tablist" :aria-label="t('nav.calendar')">
          <button v-for="v in views" :key="v" role="tab" :aria-selected="view === v" :class="{ on: view === v }" @click="setView(v)">{{ t(`cal.${v}`) }}</button>
        </div>
        <div class="seg nav" role="group">
          <button :aria-label="t('cal.prev')" :title="t('cal.prev')" @click="shift(-1)"><ChevronLeft class="h-4 w-4" :stroke-width="2.4" /></button>
          <button @click="goToday">{{ t('dates.today') }}</button>
          <button :aria-label="t('cal.next')" :title="t('cal.next')" @click="shift(1)"><ChevronRight class="h-4 w-4" :stroke-width="2.4" /></button>
        </div>
      </div>
    </header>

    <div v-if="legend.length > 1" class="mt-2 flex flex-wrap gap-x-3.5 gap-y-1 text-[12.5px] text-text-2">
      <span v-for="w in legend" :key="w.id" class="inline-flex min-w-0 items-center gap-1.5">
        <i class="h-2 w-2 shrink-0 rounded-full" :style="{ background: w.color }" />
        <span class="truncate">{{ w.name }}</span>
      </span>
    </div>

    <!-- Phone: the month with dots, the chosen day's list below -->
    <template v-if="agenda">
      <MonthGrid class="mt-4" :month="cursor" :entries="entries" :tasks="taskItems" :selected="selected" compact @select="select" />
      <DayAgenda
        class="mt-4"
        :day="selected"
        :entries="entries"
        :tasks="taskItems"
        :can-create="canCreate"
        @open="openEntry"
        @open-task="openTask"
        @toggle-task="toggleTask"
        @create="createOn"
      />
    </template>

    <div v-else ref="fill" class="cal-fill mt-4">
      <MonthGrid
        v-if="view === 'month'"
        :month="cursor"
        :entries="entries"
        :tasks="taskItems"
        :selected="selected"
        :can-create="canCreate"
        @select="select"
        @open-day="openDay"
        @open="openEntry"
        @open-task="openTask"
        @toggle-task="toggleTask"
        @create="createOn"
        @move="moveEntry"
      />
      <TimeGrid
        v-else
        :key="view"
        :days="gridDays"
        :entries="entries"
        :tasks="taskItems"
        :can-create="canCreate"
        @open="openEntry"
        @open-task="openTask"
        @toggle-task="toggleTask"
        @create="create"
        @open-day="openDay"
      />
    </div>

    <EventSheet :open="sheetOpen" :occurrence="sheetOcc" :draft="sheetDraft" :ws="sheetWs" :choices="choices" @close="sheetOpen = false" @saved="load" />
    <EventDetail :open="detailOpen" :entry="detail" :calendar-name="detailName" @close="detailOpen = false" />
    <TaskDetailSheet :open="!!taskOpen" :ws="taskOpen?.ws ?? ''" :task-id="taskOpen?.id ?? ''" @close="closeTask" />
    <SubscriptionsSheet v-if="showSubs" :open="subsOpen" :ws="wsId" @close="subsOpen = false" @changed="onSubsChanged" />
  </div>
</template>

<style scoped>
.cal-page {
  --cal-bottom: 20px;
  max-width: 100%;
  overflow-x: clip;
}
/* Until it's fitted to the window (and as a fallback). */
.cal-fill {
  height: calc(100dvh - 220px);
  min-height: 360px;
}
.seg.nav button {
  display: inline-flex;
  align-items: center;
  padding-left: 9px;
  padding-right: 9px;
}
.seg.nav button:hover,
.seg button:not(.on):hover {
  background: var(--surface-hover);
}
@media (max-width: 639px) {
  .cal-page {
    /* The tab bar. */
    --cal-bottom: calc(66px + env(safe-area-inset-bottom));
  }
  .cal-title {
    font-size: 26px;
  }
  /* The view switch takes the row; the arrows and Today keep their size. */
  .seg.views {
    flex: 1;
    min-width: 0;
  }
  .seg.views button {
    flex: 1;
    min-width: 0;
    padding-left: 6px;
    padding-right: 6px;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .seg.nav button {
    padding-left: 7px;
    padding-right: 7px;
  }
}
</style>
