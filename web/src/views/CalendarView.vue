<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ChevronLeft, ChevronRight, Plus, CalendarDays, CalendarClock, PanelLeft, PanelRight } from 'lucide-vue-next'
import MonthGrid from '@/components/calendar/MonthGrid.vue'
import TimeGrid from '@/components/calendar/TimeGrid.vue'
import DayAgenda from '@/components/calendar/DayAgenda.vue'
import EventSheet from '@/components/calendar/EventSheet.vue'
import PlanSheet from '@/components/calendar/PlanSheet.vue'
import EventDetail from '@/components/calendar/EventDetail.vue'
import SubscriptionsSheet from '@/components/calendar/SubscriptionsSheet.vue'
import CalendarSidebar, { type CalSource } from '@/components/calendar/CalendarSidebar.vue'
import CalendarSidePane from '@/components/calendar/CalendarSidePane.vue'
import type { DraggedTask } from '@/lib/taskDrag'
import TaskDetailSheet from '@/components/tasks/TaskDetailSheet.vue'
import { toEntry, type CalView, type Entry, type TaskItem } from '@/components/calendar/model'
import { useFillHeight } from '@/components/calendar/useFillHeight'
import { blocksApi, calendarApi, tasksApi, type TimeBlock, type CalendarData, type EventInput, type Occurrence, type Subscription, type Task, type Workspace } from '@/api/planner'
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
const showSubs = computed(() => !!wsId.value && canEditWs(workspace.value) && (auth.features.calendar_subscriptions !== false || auth.features.caldav_sync !== false))
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

// ---- Phone or not; wide enough for the side panes beside the grid ----
const phoneMq = window.matchMedia('(max-width: 639px)')
const phone = ref(phoneMq.matches)
const onPhone = (e: MediaQueryListEvent) => (phone.value = e.matches)
const wideMq = window.matchMedia('(min-width: 1180px)')
const wide = ref(wideMq.matches)
const onWide = (e: MediaQueryListEvent) => (wide.value = e.matches)

// ---- The side panes, as in Outlook (each remembered; on narrower windows
// they open over the grid) ----
function readFlag(key: string, def: boolean) {
  try {
    const v = localStorage.getItem(key)
    return v == null ? def : v === '1'
  } catch {
    return def
  }
}
function writeFlag(key: string, v: boolean) {
  try {
    localStorage.setItem(key, v ? '1' : '0')
  } catch {
    /* private mode */
  }
}
const leftOpen = ref(wide.value && readFlag('orbit_cal_left', true))
const rightOpen = ref(wide.value && readFlag('orbit_cal_right', false))
function toggleLeft() {
  leftOpen.value = !leftOpen.value
  if (wide.value) writeFlag('orbit_cal_left', leftOpen.value)
}
function toggleRight() {
  rightOpen.value = !rightOpen.value
  if (wide.value) writeFlag('orbit_cal_right', rightOpen.value)
}
watch(wide, (w) => {
  leftOpen.value = w && readFlag('orbit_cal_left', true)
  rightOpen.value = w && readFlag('orbit_cal_right', false)
})

// ---- Which calendars show: per workspace on the merged calendar; in one
// workspace its own events, its tasks and each subscribed calendar ----
const scope = computed(() => wsId.value || 'all')
function readHidden(): Set<string> {
  try {
    const all = JSON.parse(localStorage.getItem('orbit_cal_hidden') || '{}')
    return new Set(Array.isArray(all[scope.value]) ? all[scope.value] : [])
  } catch {
    return new Set()
  }
}
const hidden = ref<Set<string>>(readHidden())
watch(scope, () => (hidden.value = readHidden()))
function toggleSource(id: string) {
  const next = new Set(hidden.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  hidden.value = next
  try {
    const all = JSON.parse(localStorage.getItem('orbit_cal_hidden') || '{}')
    all[scope.value] = [...next]
    localStorage.setItem('orbit_cal_hidden', JSON.stringify(all))
  } catch {
    /* private mode */
  }
}
const subs = ref<Subscription[]>([])
async function loadSubs() {
  subs.value = []
  if (!wsId.value || auth.features.calendar_subscriptions === false) return
  try {
    subs.value = await calendarApi.subscriptions(wsId.value)
  } catch {
    /* none to show */
  }
}
watch(wsId, loadSubs, { immediate: true })
const holidaySource = { id: 'holidays', name: '', color: '#8E8E93' }
const sources = computed<CalSource[]>(() => {
  const hol = { ...holidaySource, name: t('cal.holidays') }
  if (!wsId.value) {
    const list = wss.active.filter((w) => w.modules.includes('calendar'))
    return [...list.map((w) => ({ id: w.id, name: w.name, color: w.color })), ...(list.some((w) => w.holidays) ? [hol] : [])]
  }
  const w = workspace.value
  return [
    { id: 'events', name: w?.name ?? t('cal.events'), color: w?.color ?? '' },
    { id: 'tasks', name: t('cal.pane.tasks'), color: w?.color ?? '' },
    ...subs.value.map((s) => ({ id: s.id, name: s.name, color: s.color || w?.color || '' })),
    ...(w?.holidays ? [hol] : []),
  ]
})
// Public holidays: the workspace's country (its owners choose it).
const countries = ref<{ code: string; name: string }[]>([])
void calendarApi.holidayCountries().then((c) => (countries.value = c)).catch(() => {})
const holidaysCtl = computed(() =>
  wsId.value && workspace.value ? { country: workspace.value.holidays, countries: countries.value, editable: workspace.value.role === 'owner' && !workspace.value.archived_at } : undefined,
)
async function setHolidays(code: string) {
  try {
    await wss.update(wsId.value, { holidays: code })
    void load()
  } catch (e: any) {
    toast.error(e.message)
  }
}
function visible(o: { occ?: Occurrence; task?: Task }): boolean {
  if (o.occ?.kind === 'holiday') return !hidden.value.has('holidays')
  if (o.occ) return !hidden.value.has(wsId.value ? (o.occ.kind === 'external' ? o.occ.subscription_id ?? '' : 'events') : o.occ.workspace_id)
  if (o.task) return !hidden.value.has(wsId.value ? 'tasks' : o.task.workspace_id)
  return true
}
const paneWs = computed(() => (wsId.value ? [wsId.value] : wss.active.filter((w) => w.modules.includes('tasks')).map((w) => w.id)))
const version = ref(0)

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
// My time blocks (the week planner's), shown in Week and Day.
const blocks = ref<TimeBlock[]>([])
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
    const [d, b] = await Promise.all([wsId.value ? calendarApi.workspace(wsId.value, p) : calendarApi.mine(p), blocksApi.list(p.from, p.to).catch(() => [] as TimeBlock[])])
    if (mine === seq) {
      data.value = d
      blocks.value = b
      version.value++
    }
  } catch (e: any) {
    if (mine === seq) toast.error(e.message)
  }
}
watch(wsId, () => (data.value = { events: [], tasks: [] }))
watch(() => `${wsId.value}|${dateStr(range.value.start)}|${dateStr(range.value.end)}`, load, { immediate: true })

const entries = computed(() => data.value.events.filter((occ) => visible({ occ })).map((o) => toEntry(o, canEditWs(wss.byId(o.workspace_id)))))
// The time grid's: the events and my time blocks (of the workspace shown).
const blockOccs = computed<Occurrence[]>(() =>
  blocks.value
    .filter((b) => (wsId.value ? b.workspace_id === wsId.value : !!wss.byId(b.workspace_id)))
    .map((b) => ({ kind: 'block', id: b.id, task_id: b.task_id, workspace_id: b.workspace_id, title: b.done ? `✓ ${b.title}` : b.title, start: b.starts_at, end: b.ends_at, all_day: false, color: b.color || wss.byId(b.workspace_id)?.color || 'var(--ws)', recurring: false })),
)
const gridEntries = computed(() => [...entries.value, ...blockOccs.value.map((o) => toEntry(o, false))])

// ---- Plan the days shown ----
const planOpen = ref(false)
const planWorkspaces = computed(() => (wsId.value ? [wsId.value] : wss.list.filter((w) => !w.archived_at && w.modules.includes('tasks')).map((w) => w.id)))
const taskItems = computed<TaskItem[]>(() =>
  data.value.tasks.filter((task) => visible({ task })).map((task) => {
    const w = wss.byId(task.workspace_id)
    return { task, color: w?.color ?? 'var(--ws)', editable: canEditWs(w) }
  }),
)

// Live: someone changed events, tasks or subscriptions of a workspace shown.
let liveTimer: ReturnType<typeof setTimeout> | null = null
const offLive = onLiveEvent('ws', (e) => {
  if (e.what === 'blocks' && e.origin !== CLIENT_ID) {
    if (liveTimer) clearTimeout(liveTimer)
    liveTimer = setTimeout(load, 250)
    return
  }
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
  // A time block opens its task.
  if (e.occ.kind === 'block' && e.occ.task_id) {
    void openTaskById(e.occ.workspace_id, e.occ.task_id)
    return
  }
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
function moveEntry(e: Entry, days: number) {
  const o = e.occ
  if (!o.all_day) return reschedule(e, addDays(e.start, days), addDays(e.end, days))
  return saveTiming(e, {
    starts_at: utcMidnight(dateStr(addDays(parseDate(allDayDate(o.start)), days))),
    ends_at: utcMidnight(dateStr(addDays(parseDate(allDayDate(o.end)), days))),
  })
}
// Dragged in Week / Day view to another time, or made longer or shorter. It
// keeps its own time zone.
function reschedule(e: Entry, start: Date, end: Date) {
  return saveTiming(e, { starts_at: start.toISOString(), ends_at: end.toISOString(), tz: e.occ.tz || deviceTZ() })
}
async function saveTiming(e: Entry, body: EventInput) {
  const o = e.occ
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
async function openTaskById(ws: string, id: string) {
  try {
    if (!tasksStore.tasks[ws]) await tasksStore.load(ws)
  } catch {
    /* the sheet loads what it can */
  }
  taskOpen.value = { ws, id }
}
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

// A task dragged onto a day (or an hour): that date (and time).
async function dropTask(d: DraggedTask, day: string, time: string | null = null) {
  if (!canEditWs(wss.byId(d.ws))) return
  const body = { due: day, due_time: time ?? '' }
  try {
    if (tasksStore.tasks[d.ws]) await tasksStore.update(d.ws, d.id, body)
    else await tasksApi.update(d.id, body)
    toast.success(t('cal.pane.scheduled', { when: time ? `${relDay(day)}, ${time}` : relDay(day) }))
  } catch (e: any) {
    toast.error(e.message)
  }
  void load()
}

// ---- Subscribed calendars ----
const subsOpen = ref(false)
function onSubsChanged() {
  subsLoaded.clear()
  void loadSubs()
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
const leftEl = ref<HTMLElement | null>(null)
const rightEl = ref<HTMLElement | null>(null)
useFillHeight(leftEl)
useFillHeight(rightEl)

onMounted(() => {
  phoneMq.addEventListener('change', onPhone)
  wideMq.addEventListener('change', onWide)
  window.addEventListener('keydown', onKey)
})
onUnmounted(() => {
  phoneMq.removeEventListener('change', onPhone)
  wideMq.removeEventListener('change', onWide)
  window.removeEventListener('keydown', onKey)
  offLive()
  if (liveTimer) clearTimeout(liveTimer)
})

const views: CalView[] = ['day', 'week', 'month']
</script>

<template>
  <div class="cal-page px-4 pt-6 sm:px-8 sm:pt-8" :class="{ 'pb-6': agenda }">
   <div class="cal-shell">
    <!-- Left: the small month and the calendars -->
    <div v-if="leftOpen && !phone" ref="leftEl" class="side left" :class="{ floating: !wide }">
      <CalendarSidebar
        :cursor="cursor"
        :view="view"
        :range-start="range.start"
        :range-end="range.end"
        :sources="sources"
        :hidden="hidden"
        :heading="wsId ? t('cal.calendars') : t('cal.myCalendars')"
        :can-add="showSubs"
        @go="(d) => ((cursor = d), !wide && (leftOpen = false))"
        :holidays="holidaysCtl"
        @toggle="toggleSource"
        @add="subsOpen = true"
        @holidays="setHolidays"
      />
    </div>
    <div v-if="!wide && (leftOpen || rightOpen) && !phone" class="side-scrim" @click="(leftOpen = false), (rightOpen = false)" />

   <div class="cal-main min-w-0 flex-1">
    <header class="flex flex-wrap items-center gap-x-4 gap-y-3">
      <div class="min-w-0 flex-1">
        <div class="flex min-w-0 items-center gap-2">
          <button v-if="!phone" class="icon-btn shrink-0" :class="{ on: leftOpen }" :title="t('cal.toggleLeft')" :aria-label="t('cal.toggleLeft')" :aria-pressed="leftOpen" @click="toggleLeft"><PanelLeft class="h-[18px] w-[18px]" /></button>
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
        <button v-if="view !== 'month' && !agenda && planWorkspaces.length" class="btn btn-sm shrink-0" :title="t('plan.button')" data-testid="plan-open" @click="planOpen = true"><CalendarClock class="h-3.5 w-3.5" /><span class="hidden lg:inline">{{ t('plan.button') }}</span></button>
        <button v-if="!phone" class="icon-btn shrink-0" :class="{ on: rightOpen }" :title="t('cal.toggleRight')" :aria-label="t('cal.toggleRight')" :aria-pressed="rightOpen" data-testid="toggle-pane" @click="toggleRight"><PanelRight class="h-[18px] w-[18px]" /></button>
      </div>
    </header>

    <div v-if="legend.length > 1 && !leftOpen" class="mt-2 flex flex-wrap gap-x-3.5 gap-y-1 text-[12.5px] text-text-2">
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
        @drop-task="dropTask"
      />
      <TimeGrid
        v-else
        :key="view"
        :days="gridDays"
        :entries="gridEntries"
        :tasks="taskItems"
        :can-create="canCreate"
        @open="openEntry"
        @open-task="openTask"
        @toggle-task="toggleTask"
        @create="create"
        @open-day="openDay"
        @drop-task="dropTask"
        @move="moveEntry"
        @reschedule="reschedule"
      />
    </div>
   </div>

    <!-- Right: what's coming, and the tasks to drag onto the calendar -->
    <div v-if="rightOpen && !phone" ref="rightEl" class="side right" :class="{ floating: !wide }">
      <CalendarSidePane
        :ws-ids="paneWs"
        :ws="wsId"
        :editable="(w) => canEditWs(wss.byId(w))"
        :color="(w) => wss.byId(w)?.color ?? 'var(--ws)'"
        :visible="visible"
        :version="version"
        @open="openEntry"
        @open-task="openTask"
        @toggle-task="toggleTask"
      />
    </div>
   </div>

    <EventSheet :open="sheetOpen" :occurrence="sheetOcc" :draft="sheetDraft" :ws="sheetWs" :choices="choices" @close="sheetOpen = false" @saved="load" />
    <PlanSheet
      :open="planOpen"
      :days="gridDays"
      :workspace-ids="planWorkspaces"
      :events="data.events"
      :due-tasks="data.tasks"
      :blocks="blocks"
      @close="planOpen = false"
      @saved="load"
    />
    <EventDetail
      :open="detailOpen"
      :entry="detail"
      :calendar-name="detailName"
      :can-cancel="!!detail?.occ.course_id && canEditWs(wss.byId(detail.occ.workspace_id))"
      @close="detailOpen = false"
      @cancelled="(detailOpen = false), load()"
    />
    <TaskDetailSheet :open="!!taskOpen" :ws="taskOpen?.ws ?? ''" :task-id="taskOpen?.id ?? ''" @close="closeTask" />
    <SubscriptionsSheet
      v-if="showSubs"
      :open="subsOpen"
      :ws="wsId"
      :owner="workspace?.role === 'owner'"
      :subscriptions="auth.features.calendar_subscriptions !== false"
      :caldav="auth.features.caldav_sync !== false"
      @close="subsOpen = false"
      @changed="onSubsChanged"
    />
  </div>
</template>

<style scoped>
.cal-shell {
  display: flex;
  gap: 24px;
  align-items: flex-start;
}
.side {
  position: sticky;
  top: 24px;
  display: flex;
  flex: none;
  min-height: 0;
}
.side.right :deep(.pane) {
  height: 100%;
}
/* Narrower windows: the panes open over the grid. */
.side.floating {
  position: fixed;
  top: 0;
  bottom: 0;
  z-index: 40;
  height: 100dvh !important;
  padding: 20px 16px;
  background: var(--surface);
  box-shadow: var(--shadow-lg);
}
.side.floating.left {
  left: 0;
}
.side.floating.right {
  right: 0;
}
.side-scrim {
  position: fixed;
  inset: 0;
  z-index: 39;
  background: rgba(0, 0, 0, 0.18);
}
.icon-btn.on {
  color: var(--accent-text);
  background: var(--fill-2);
}
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
