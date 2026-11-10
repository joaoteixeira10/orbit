<script setup lang="ts">
import { ref, computed, watch, onMounted, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Plus, Eye, EyeOff, Pencil, CalendarClock, Inbox, CheckCircle2, CalendarDays, ChevronLeft, ListPlus, Rows3, Check, ListFilter, FolderOpen, ChevronRight, Archive, ArchiveRestore } from 'lucide-vue-next'
import { countParts, filesApi, type Folder } from '@/lib/filesApi'
import TaskRow from '@/components/tasks/TaskRow.vue'
import TaskDetailSheet from '@/components/tasks/TaskDetailSheet.vue'
import ListSheet from '@/components/tasks/ListSheet.vue'
import TaskFilter from '@/components/tasks/TaskFilter.vue'
import SmartListSheet from '@/components/tasks/SmartListSheet.vue'
import Popover from '@/components/ui/Popover.vue'
import WsIcon from '@/components/ui/WsIcon.vue'
import { useTasksStore } from '@/stores/tasks'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { useSortable } from '@/lib/sortable'
import { parseQuick } from '@/lib/quickadd'
import { todayStr, relDay } from '@/lib/dates'
import { tasksApi, type Member, type Task, type TaskList, type TaskInput } from '@/api/planner'
import { useTaskFilter, useTaskFilterStore, dueBucket, dueEdges, emptyFilter, matchContext, matchTask, DUE_BUCKETS, type DueBucket, type TaskFilter as Filter } from '@/lib/taskFilter'
import type { SmartList } from '@/api/planner'

// Tasks, Reminders-style: smart lists (Today, Scheduled, All, Completed, and
// saved filters) and the workspace's own lists on the left; the chosen one on the right, its open
// tasks grouped by status column (or, Planner-style, by due date, priority or
// person), completed ones on demand, narrowed by the workspace's filter. On
// phones the two panes are two screens.
const route = useRoute()
const router = useRouter()
const tasks = useTasksStore()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t, locale } = useI18n()

const ws = computed(() => String(route.params.ws))
const workspace = computed(() => wss.byId(ws.value))
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only)
const lists = computed(() => tasks.listsOf(ws.value))
// Archived lists are put away under My Lists; one opened shows its tasks.
const archivedLists = computed(() => tasks.archivedListsOf(ws.value))
const showArchived = ref(false)
const active = computed(() => tasks.tasksOf(ws.value))
const members = computed(() => wss.members[ws.value] ?? [])

type Smart = 'today' | 'scheduled' | 'all' | 'completed'
const SMART: Smart[] = ['today', 'scheduled', 'all', 'completed']
const sel = computed(() => (typeof route.params.list === 'string' ? route.params.list : ''))
const list = computed(() => tasks.allListsOf(ws.value).find((l) => l.id === sel.value))
const all = computed(() => (list.value?.archived_at ? tasks.allTasksOf(ws.value) : active.value))
// Opening an archived list unfolds them (once); the toggle folds them again.
watch(
  () => list.value?.archived_at && list.value.id,
  (id) => {
    if (id) showArchived.value = true
  },
  { immediate: true },
)
async function restoreList() {
  if (!list.value) return
  try {
    const l = await tasks.archiveList(ws.value, list.value.id, false)
    toast.success(t('lists.restored', { name: l.name }))
  } catch (e: any) {
    toast.error(e.message)
  }
}

// A list that's a folder's (a project): the folder, what it holds, a way there.
const folders = ref<Folder[]>([])
watch(
  () => list.value?.folder_id,
  async (id) => {
    if (id) folders.value = await filesApi.folders(ws.value).catch(() => [])
  },
  { immediate: true },
)
const listFolder = computed(() => {
  const f = folders.value.find((x) => x.id === list.value?.folder_id)
  const mods = workspace.value?.modules ?? []
  const view = ['files', 'sheets', 'diagrams', 'whiteboards'].find((m) => mods.includes(m))
  if (!f || !view) return null
  return {
    name: f.name,
    color: f.color || '#5ac8fa',
    what: countParts(f.counts).map((p) => t(`project.count.${p.key}`, { n: p.n }, p.n)).join(' · '),
    to: { name: `ws-${view}`, params: { ws: ws.value }, query: { folder: f.id } },
  }
})
// A saved smart list (a filter with a name), when one is open.
const smartLists = computed(() => tasks.smartOf(ws.value))
const saved = computed(() => smartLists.value.find((l) => l.id === sel.value))
const narrowMq = window.matchMedia('(max-width: 767px)')
const narrow = ref(narrowMq.matches)
narrowMq.addEventListener('change', (e) => (narrow.value = e.matches))

// ---- Loading ----
const doneTasks = ref<Task[]>([]) // every completed task, for "Completed"
async function load() {
  try {
    await Promise.all([tasks.load(ws.value), tasks.loadSmart(ws.value).catch(() => {})])
    if ((workspace.value?.member_count ?? 1) > 1) void wss.loadMembers(ws.value)
  } catch (e: any) {
    toast.error(e.message)
  }
}
watch(ws, load)
onMounted(load)
watch(
  () => [lists.value.length, sel.value, narrow.value] as const,
  () => {
    // Wide screens always show a list: the first one by default.
    if (!sel.value && !narrow.value && lists.value.length) router.replace({ name: 'ws-tasks', params: { ws: ws.value, list: lists.value[0].id }, query: route.query })
  },
  { immediate: true },
)
watch([sel, () => all.value.length], async () => {
  if (sel.value === 'completed') doneTasks.value = await tasksApi.list(ws.value, { state: 'done' }).catch(() => [])
}, { immediate: true })

// ---- Counts and selections ----
const today = todayStr()
const open = computed(() => active.value.filter((x) => !x.done && !x.parent_id))
const counts = computed(() => ({
  today: open.value.filter((x) => x.due && x.due <= today).length,
  scheduled: open.value.filter((x) => x.due).length,
  all: open.value.length,
  completed: active.value.filter((x) => x.done && !x.parent_id).length,
}))
const listById = computed(() => new Map(tasks.allListsOf(ws.value).map((l) => [l.id, l])))
const showDone = ref(localStorage.getItem('orbit_show_done') === '1')
watch(showDone, (v) => localStorage.setItem('orbit_show_done', v ? '1' : '0'))
// Tasks ticked a moment ago stay visible briefly, so ticking feels done.
const justDone = ref(new Set<string>())

// The workspace's filter (shared with the Board, Timeline and Charts) — or,
// in a smart list, the smart list's own, saved as it changes.
const me = computed(() => auth.user?.id ?? '')
const filterKey = computed(() => (saved.value ? `${ws.value}#${saved.value.id}` : ws.value))
const { active: filterOn, matches, clear: clearFilter } = useTaskFilter(filterKey, lists, me)
const filters = useTaskFilterStore()
const canon = (f: Partial<Filter>) => JSON.stringify({ ...emptyFilter(), ...f })
// Per smart list: a change waiting to be saved (or being saved), and
// whether its server copy has been taken in yet.
const pending = new Map<string, ReturnType<typeof setTimeout> | 'saving'>()
const seeded = new Set<string>()
watch(
  () => saved.value && canon(saved.value.filter),
  () => {
    // The server's copy, unless a change of ours is on its way.
    const l = saved.value
    if (!l || pending.has(filterKey.value)) return
    filters.byWs[filterKey.value] = JSON.parse(canon(l.filter))
    seeded.add(filterKey.value)
  },
  { immediate: true },
)
watch(
  () => saved.value && canon(filters.byWs[filterKey.value] ?? {}),
  (now) => {
    const l = saved.value
    const key = filterKey.value
    if (!l || !now || !l.can_edit || !seeded.has(key) || now === canon(l.filter)) return
    const was = pending.get(key)
    if (was && was !== 'saving') clearTimeout(was)
    pending.set(key, setTimeout(async () => {
      pending.set(key, 'saving')
      try {
        await tasks.saveSmart(ws.value, l.id, { filter: JSON.parse(now) })
      } catch (e: any) {
        toast.error(e.message)
      } finally {
        if (pending.get(key) === 'saving') pending.delete(key)
      }
    }, 600))
  },
)
const ctxAll = computed(() => matchContext(me.value, lists.value))
const smartCount = (l: SmartList) => open.value.filter((x) => matchTask(x, { ...emptyFilter(), ...l.filter }, ctxAll.value)).length
const visible = (x: Task) => !x.parent_id && (!x.done || showDone.value || justDone.value.has(x.id)) && matches(x)

// Grouping of a list (and of All): its natural one (status columns, or
// lists) or by due date, priority or person. Remembered per browser.
type GroupBy = 'default' | 'due' | 'priority' | 'assignee'
const groupBy = ref<GroupBy>((localStorage.getItem('orbit_tasks_group') as GroupBy) || 'default')
watch(groupBy, (v) => localStorage.setItem('orbit_tasks_group', v))
const canGroup = computed(() => !!list.value || sel.value === 'all' || !!saved.value)
const grouping = computed<GroupBy>(() => (!canGroup.value || (groupBy.value === 'assignee' && members.value.length < 2) ? 'default' : groupBy.value))
const groupOptions = computed<GroupBy[]>(() => ['default', 'due', 'priority', ...(members.value.length > 1 ? (['assignee'] as GroupBy[]) : [])])
const groupLabel = (g: GroupBy) => t(`tasks.group.${g === 'default' ? (list.value ? 'status' : 'list') : g}`)
const groupAnchor = ref<HTMLElement | null>(null)
const groupOpen = ref(false)

interface Section {
  key: string
  title: string
  color?: string
  columnId?: string
  listId?: string
  tasks: Task[]
  overdue?: boolean
  preset?: TaskInput // what a task added here gets (its group's priority, person, date)
  noAdd?: boolean
}
const grouped = computed<Section[]>(() => {
  const byRank = (a: Task, b: Task) => a.rank - b.rank
  const byDue = (a: Task, b: Task) => (a.due ?? '').localeCompare(b.due ?? '') || (a.due_time ?? '').localeCompare(b.due_time ?? '') || a.rank - b.rank
  if (grouping.value !== 'default') {
    const pool = all.value.filter((x) => visible(x) && (!list.value || x.list_id === list.value.id))
    const base = { listId: list.value?.id }
    switch (grouping.value) {
      case 'due': {
        const e = dueEdges(today)
        const dueOf: Partial<Record<DueBucket, string>> = { today, none: '' }
        return DUE_BUCKETS.map((b) => ({
          ...base, key: 'due-' + b, title: t(`filter.dueB.${b}`), overdue: b === 'overdue',
          tasks: pool.filter((x) => dueBucket(x, e) === b).sort(byDue),
          preset: b in dueOf ? { due: dueOf[b] || undefined } : undefined, noAdd: !(b in dueOf),
        })).filter((s) => s.tasks.length)
      }
      case 'priority':
        return [3, 2, 1, 0].map((p) => ({
          ...base, key: 'prio-' + p, title: t(`tasks.prio.${p}`), color: ['', '#34C759', '#FF9500', '#FF3B30'][p] || undefined,
          tasks: pool.filter((x) => x.priority === p).sort(byDue), preset: { priority: p },
        })).filter((s) => s.tasks.length)
      case 'assignee':
        return [...members.value.map((m) => ({ id: m.user_id, name: m.display_name || m.username })), { id: '', name: t('tasks.unassigned') }].map((m) => ({
          ...base, key: 'who-' + (m.id || 'none'), title: m.name,
          // A task for several people shows under each of them.
          tasks: pool.filter((x) => (m.id ? x.assignee_ids.includes(m.id) : !x.assignee_ids.length)).sort(byDue), preset: m.id ? { assignee_ids: [m.id] } : undefined,
        })).filter((s) => s.tasks.length)
    }
  }
  const byList = (preset?: TaskInput) =>
    lists.value.map((l) => ({ key: l.id, title: l.name, color: l.color, listId: l.id, columnId: l.columns.find((c) => !c.done)?.id, preset, tasks: all.value.filter((x) => x.list_id === l.id && visible(x)).sort(byRank) })).filter((s) => s.tasks.length)
  if (saved.value) {
    const sl = saved.value
    const s = byList(smartPreset(sl.filter))
    // An empty smart list still offers a place to add a task.
    if (s.length || !lists.value.length) return s
    const l = (sl.filter.lists?.length ? lists.value.find((x) => sl.filter.lists.includes(x.id)) : undefined) ?? lists.value[0]
    return [{ key: l.id, title: l.name, color: l.color, listId: l.id, columnId: l.columns.find((c) => !c.done)?.id, preset: smartPreset(sl.filter), tasks: [] }]
  }
  if (list.value) {
    const l = list.value
    const mine = all.value.filter((x) => x.list_id === l.id && visible(x))
    const cols = l.columns.filter((c) => !c.done || showDone.value || mine.some((x) => x.column_id === c.id))
    return cols.map((c) => ({ key: c.id, title: c.name, color: c.color, columnId: c.id, listId: l.id, tasks: mine.filter((x) => x.column_id === c.id).sort(byRank) }))
      .filter((s, i) => s.tasks.length || i === 0 || l.columns.filter((c) => !c.done).length > 1)
  }
  switch (sel.value) {
    case 'today': {
      const due = all.value.filter((x) => visible(x) && x.due && x.due <= today).sort(byDue)
      return [
        { key: 'overdue', title: t('tasks.overdue'), overdue: true, tasks: due.filter((x) => x.due! < today) },
        { key: 'today', title: t('dates.today'), tasks: due.filter((x) => x.due === today) },
      ].filter((s) => s.tasks.length)
    }
    case 'scheduled': {
      const dated = all.value.filter((x) => visible(x) && x.due).sort(byDue)
      const groups = new Map<string, Task[]>()
      for (const x of dated) groups.set(x.due!, [...(groups.get(x.due!) ?? []), x])
      return [...groups].map(([d, ts]) => ({ key: d, title: relDay(d), overdue: d < today, tasks: ts }))
    }
    case 'all':
      return byList()
    case 'completed': {
      const groups = new Map<string, Task[]>()
      for (const x of doneTasks.value.filter((x) => !x.parent_id)) {
        const d = (x.completed_at ?? '').slice(0, 10)
        groups.set(d, [...(groups.get(d) ?? []), x])
      }
      return [...groups].map(([d, ts]) => ({ key: d, title: relDay(d), tasks: ts }))
    }
  }
  return []
})

// Order within each section: the view's own (a list's drag order, dates),
// or by name, newest or oldest first. Remembered per browser.
type SortBy = 'manual' | 'name' | 'newest' | 'oldest'
const SORTS: SortBy[] = ['manual', 'name', 'newest', 'oldest']
const sortBy = ref<SortBy>(SORTS.find((x) => x === localStorage.getItem('orbit_tasks_sort')) ?? 'manual')
watch(sortBy, (v) => localStorage.setItem('orbit_tasks_sort', v))
const sorting = computed<SortBy>(() => (canGroup.value ? sortBy.value : 'manual'))
const sections = computed<Section[]>(() => {
  const s = sorting.value
  if (s === 'manual') return grouped.value
  const names = new Intl.Collator(locale.value, { numeric: true, sensitivity: 'base' })
  const order = {
    name: (a: Task, b: Task) => names.compare(a.title, b.title),
    newest: (a: Task, b: Task) => b.created_at.localeCompare(a.created_at),
    oldest: (a: Task, b: Task) => a.created_at.localeCompare(b.created_at),
  }[s]
  return grouped.value.map((x) => ({ ...x, tasks: [...x.tasks].sort(order) }))
})

// smartPreset is what a task added in a smart list gets, so it shows there:
// its one priority, its tags, "me" when it's for me.
function smartPreset(f: Filter): TaskInput | undefined {
  const p: TaskInput = {}
  if (f.priority?.length === 1) p.priority = f.priority[0]
  if (f.tags?.length) p.tags = [f.tags[0]]
  const who = (f.assignee ?? []).filter((a) => a !== 'none')
  if (who.length === 1) p.assignee_ids = [who[0] === 'me' ? me.value : who[0]]
  return Object.keys(p).length ? p : undefined
}

const heading = computed(() => {
  if (saved.value) return { title: saved.value.name, color: saved.value.color || '#FF9500', icon: saved.value.icon || 'star' }
  if (list.value) return { title: list.value.name, color: list.value.color, icon: list.value.icon }
  const meta: Record<Smart, { color: string; icon: string }> = {
    today: { color: '#007AFF', icon: 'calendar' }, scheduled: { color: '#FF3B30', icon: 'calendar' },
    all: { color: '#5856D6', icon: 'list-checks' }, completed: { color: '#8E8E93', icon: 'list-checks' },
  }
  const m = meta[sel.value as Smart] ?? meta.all
  return { title: t(`tasks.smart.${sel.value || 'all'}`), color: m.color, icon: m.icon }
})
// The filter hides everything here (rather than there being nothing).
const filteredOut = computed(() => filterOn.value && !sections.value.some((x) => x.tasks.length))
const total = computed(() => sections.value.reduce((n, s) => n + s.tasks.filter((x) => !x.done).length, 0))

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
async function change(x: Task, patch: TaskInput) {
  try {
    const r = await tasks.update(ws.value, x.id, patch)
    if (r.repeated) toast.success(t('tasks.repeated', { date: r.task.due }))
  } catch (e: any) {
    toast.error(e.message)
  }
}
const peopleOf = (x: Task) => x.assignee_ids.map((id) => memberById.value.get(id)).filter((m): m is Member => !!m)
// Sub-tasks start folded under their task; the ones opened stay open
// (in this browser).
const openSubs = ref(new Set<string>(readOpenSubs()))
function readOpenSubs(): string[] {
  try {
    const v = JSON.parse(localStorage.getItem('orbit_open_subtasks') || '[]')
    return Array.isArray(v) ? v.filter((x) => typeof x === 'string') : []
  } catch {
    return []
  }
}
function foldSubs(id: string) {
  const s = new Set(openSubs.value)
  if (!s.delete(id)) s.add(id)
  openSubs.value = s
  try {
    localStorage.setItem('orbit_open_subtasks', JSON.stringify([...s].slice(-300)))
  } catch {
    /* private window: open for now only */
  }
}
function subCount(x: Task) {
  const subs = all.value.filter((s) => s.parent_id === x.id)
  return { done: subs.filter((s) => s.done).length, total: subs.length }
}
const memberById = computed(() => new Map(members.value.map((m) => [m.user_id, m])))

// Detail sheet (?task= opens one, e.g. from search).
const openTask = ref('')
watch(() => route.query.task, (id) => { if (typeof id === 'string') openTask.value = id }, { immediate: true })
function closeTask() {
  openTask.value = ''
  if (route.query.task) router.replace({ query: { ...route.query, task: undefined } })
}

// Quick add at the bottom of a section.
const adding = ref('')
const draft = ref('')
const addInput = ref<HTMLInputElement[] | HTMLInputElement | null>(null)
async function startAdd(key: string) {
  adding.value = key
  draft.value = ''
  await nextTick()
  const el = Array.isArray(addInput.value) ? addInput.value[0] : addInput.value
  el?.focus()
}
async function commitAdd(s: Section) {
  const text = draft.value
  const q = parseQuick(text)
  if (!q.title) {
    adding.value = ''
    return
  }
  // Cleared at once: a blur while this saves mustn't add it again.
  draft.value = ''
  const target = s.listId ? listById.value.get(s.listId) : list.value ?? lists.value[0]
  try {
    await tasks.create(ws.value, {
      title: q.title, list_id: target?.id, column_id: s.columnId,
      // In Today and Scheduled a new task needs a date to show up there.
      due: q.due ?? (sel.value === 'today' ? today : sel.value === 'scheduled' ? (/^\d{4}-\d{2}-\d{2}$/.test(s.key) && s.key >= today ? s.key : today) : undefined),
      due_time: q.due_time, priority: q.priority, tags: q.tags.length ? q.tags : undefined, repeat: q.repeat, estimate: q.estimate,
      ...s.preset,
      // What was typed wins over the group's preset (tags add up).
      ...(q.due ? { due: q.due } : {}), ...(q.priority ? { priority: q.priority } : {}),
      ...(q.tags.length && s.preset?.tags ? { tags: [...new Set([...q.tags, ...s.preset.tags])] } : q.tags.length ? { tags: q.tags } : {}),
    })
  } catch (e: any) {
    if (!draft.value) draft.value = text
    toast.error(e.message)
  }
}
watch(() => route.query.new, async (v) => {
  if (v === '1' && sections.value.length) await startAdd(sections.value[0].key)
}, { immediate: true })

// Drag to reorder within a list (sections are columns there).
const sectionEls = ref<HTMLElement[]>([])
function onDrop(m: { id: string; toGroup: string; beforeId: string }) {
  const s = sections.value.find((x) => x.key === m.toGroup)
  if (!s?.columnId) return
  void tasks.move(ws.value, m.id, s.columnId, m.beforeId || '^', s.listId).catch((e) => toast.error(e.message))
}
const container = ref<HTMLElement | null>(null)
// One Sortable per section (a list has at most 12 columns); each follows
// its element as sections come and go. Only a real list in its own order
// is reorderable.
const sortEls = computed(() => (list.value && editable.value && grouping.value === 'default' && sorting.value === 'manual' ? sectionEls.value : []))
for (let i = 0; i < 12; i++) {
  const r = computed(() => sortEls.value[i] ?? null)
  useSortable(r, onDrop, { group: 'task-rows', handle: '.task-row', filter: 'input,button.check,button.info', preventOnFilter: false, draggable: '[data-id]' })
}

// Lists.
const listSheet = ref<{ open: boolean; id: string }>({ open: false, id: '' })
function onListSaved(l: TaskList) {
  void tasks.loadLists(ws.value)
  router.push({ name: 'ws-tasks', params: { ws: ws.value, list: l.id } })
}
function pick(id: string) {
  router.push({ name: 'ws-tasks', params: { ws: ws.value, list: id } })
}
// Smart lists (saved filters).
const smartSheet = ref<{ open: boolean; id: string; filter?: Filter }>({ open: false, id: '' })
const canSmart = computed(() => !auth.privileges.read_only)
function newSmart() {
  smartSheet.value = { open: true, id: '', filter: JSON.parse(canon(filters.of(ws.value))) }
}
function onSmartSaved(l: SmartList) {
  if (smartSheet.value.id) return
  // Made from the filter on now: that filter is the smart list's from here.
  filters.clear(ws.value)
  pick(l.id)
}
function onSmartDeleted() {
  router.replace({ name: 'ws-tasks', params: { ws: ws.value } })
}
const smartIcons = { today: CalendarDays, scheduled: CalendarClock, all: Inbox, completed: CheckCircle2 }
const smartColors = { today: '#007AFF', scheduled: '#FF3B30', all: '#5856D6', completed: '#8E8E93' }
</script>

<template>
  <div class="tasks-view" :class="{ 'one-pane': narrow }">
    <!-- Lists pane -->
    <aside v-if="!narrow || !sel" class="lists-pane">
      <h1 v-if="narrow" class="large-title mb-4">{{ t('modules.tasks') }}</h1>
      <div class="grid grid-cols-2 gap-2.5">
        <button v-for="k in SMART" :key="k" class="smart" :class="{ on: sel === k }" @click="pick(k)">
          <span class="flex items-start justify-between">
            <span class="grid h-[30px] w-[30px] place-items-center rounded-full text-white" :style="{ background: smartColors[k] }"><component :is="smartIcons[k]" class="h-4 w-4" :stroke-width="2.4" /></span>
            <span class="text-[22px] font-bold leading-none">{{ k === 'completed' ? '' : counts[k] }}</span>
          </span>
          <span class="mt-2 text-[14px] font-semibold text-text-2">{{ t(`tasks.smart.${k}`) }}</span>
        </button>
      </div>
      <template v-if="smartLists.length">
        <h2 class="title-3 mb-2 mt-6 px-1">{{ t('smartList.section') }}</h2>
        <div class="group" data-testid="smart-lists">
          <button v-for="l in smartLists" :key="l.id" class="group-row w-full text-left" :class="{ 'list-on': sel === l.id }" @click="pick(l.id)">
            <WsIcon :icon="l.icon || 'star'" :color="l.color || '#FF9500'" :size="28" />
            <span class="min-w-0 flex-1 truncate text-[15px]">{{ l.name }}</span>
            <span v-if="l.shared" class="text-[11.5px] text-text-3">{{ t('smartList.sharedBadge') }}</span>
            <span class="text-[15px] text-text-3">{{ smartCount(l) }}</span>
          </button>
        </div>
      </template>
      <h2 class="title-3 mb-2 mt-6 px-1">{{ t('tasks.myLists') }}</h2>
      <div class="group">
        <button v-for="l in lists" :key="l.id" class="group-row w-full text-left" :class="{ 'list-on': sel === l.id }" @click="pick(l.id)">
          <WsIcon :icon="l.icon || 'list-checks'" :color="l.color" :size="28" />
          <span class="min-w-0 flex-1 truncate text-[15px]">{{ l.name }}</span>
          <span class="text-[15px] text-text-3">{{ l.open_count }}</span>
        </button>
        <div v-if="!lists.length" class="group-row text-[14px] text-text-3">{{ t('tasks.noLists') }}</div>
      </div>
      <template v-if="archivedLists.length">
        <button type="button" class="arch-toggle" :aria-expanded="showArchived" data-testid="archived-lists" @click="showArchived = !showArchived">
          <ChevronRight class="h-3.5 w-3.5 transition-transform" :class="{ 'rotate-90': showArchived }" />
          <Archive class="h-3.5 w-3.5" />{{ t('lists.archived') }}<span class="text-text-3">{{ archivedLists.length }}</span>
        </button>
        <div v-if="showArchived" class="group archived">
          <button v-for="l in archivedLists" :key="l.id" class="group-row w-full text-left" :class="{ 'list-on': sel === l.id }" @click="pick(l.id)">
            <WsIcon :icon="l.icon || 'list-checks'" :color="l.color" :size="28" />
            <span class="min-w-0 flex-1 truncate text-[15px]">{{ l.name }}</span>
            <span class="text-[13px] text-text-3">{{ l.open_count || '' }}</span>
          </button>
        </div>
      </template>
      <div class="mt-2 flex flex-wrap gap-x-3">
        <button v-if="editable" class="btn-plain" @click="listSheet = { open: true, id: '' }"><ListPlus class="h-4 w-4" />{{ t('tasks.addList') }}</button>
        <button v-if="canSmart" class="btn-plain" data-testid="smart-add" @click="newSmart"><ListFilter class="h-4 w-4" />{{ t('smartList.add') }}</button>
      </div>
    </aside>

    <!-- Tasks pane -->
    <section v-if="!narrow || sel" ref="container" class="tasks-pane">
      <header class="mb-1 flex items-center gap-2">
        <button v-if="narrow" class="btn-plain -ml-2" @click="router.push({ name: 'ws-tasks', params: { ws } })"><ChevronLeft class="h-5 w-5" />{{ t('tasks.lists') }}</button>
        <div class="flex-1" />
        <template v-if="sel !== 'completed'">
          <button v-if="canGroup" ref="groupAnchor" class="btn btn-sm shrink-0" data-testid="tasks-view-menu" :aria-expanded="groupOpen" @click="groupOpen = !groupOpen">
            <Rows3 class="h-3.5 w-3.5" /><span class="hidden sm:inline">{{ t('tasks.groupBy') }}:</span> {{ groupLabel(grouping) }}<template v-if="sorting !== 'manual'"> · {{ t(`tasks.sortShort.${sorting}`) }}</template>
          </button>
          <TaskFilter v-if="!saved || saved.can_edit" :ws="ws" :fkey="filterKey" :show-lists="!list" :can-save="!saved && canSmart" @save="newSmart" />
        </template>
        <button v-if="sel !== 'completed'" class="icon-btn" :title="showDone ? t('tasks.hideDone') : t('tasks.showDone')" @click="showDone = !showDone">
          <component :is="showDone ? EyeOff : Eye" class="h-[18px] w-[18px]" />
        </button>
        <button v-if="list && editable" class="icon-btn" :title="t('tasks.editList')" @click="listSheet = { open: true, id: list.id }"><Pencil class="h-[17px] w-[17px]" /></button>
        <button v-if="saved" class="icon-btn" :title="t('smartList.edit')" data-testid="smart-edit" @click="smartSheet = { open: true, id: saved.id }"><Pencil class="h-[17px] w-[17px]" /></button>
      </header>
      <div class="mb-5 flex items-baseline gap-3">
        <h1 class="large-title min-w-0 flex-1 truncate" :style="{ color: heading.color }">{{ heading.title }}</h1>
        <span class="text-[28px] font-semibold tabular-nums" :style="{ color: heading.color }">{{ total }}</span>
      </div>
      <RouterLink v-if="listFolder" :to="listFolder.to" class="list-folder" data-testid="list-folder">
        <FolderOpen class="h-4 w-4 shrink-0" :style="{ color: listFolder.color }" />
        <span class="font-medium">{{ listFolder.name }}</span>
        <span v-if="listFolder.what" class="truncate text-text-3">· {{ listFolder.what }}</span>
      </RouterLink>

      <Popover v-if="groupOpen" :anchor="groupAnchor" placement="bottom-end" :width="220" @close="groupOpen = false">
        <div class="p-1.5">
          <div class="cap px-2.5 pb-1 pt-1.5">{{ t('tasks.groupBy') }}</div>
          <button v-for="g in groupOptions" :key="g" class="menu-item w-full" @click="(groupBy = g), (groupOpen = false)">
            <Check class="h-4 w-4" :class="grouping === g ? 'text-accent' : 'opacity-0'" />{{ groupLabel(g) }}
          </button>
          <div class="cap px-2.5 pb-1 pt-2.5">{{ t('tasks.sortBy') }}</div>
          <button v-for="o in SORTS" :key="o" class="menu-item w-full" :data-testid="`tasks-sort-${o}`" @click="(sortBy = o), (groupOpen = false)">
            <Check class="h-4 w-4" :class="sorting === o ? 'text-accent' : 'opacity-0'" />{{ t(`tasks.sort.${o}`) }}
          </button>
        </div>
      </Popover>

      <p v-if="workspace?.archived_at" class="mb-4 rounded-xl bg-fill-2 px-4 py-2.5 text-[13px] text-text-2">{{ t('ws.archivedBanner') }}</p>
      <div v-else-if="list?.archived_at" class="mb-4 flex items-center gap-3 rounded-xl bg-fill-2 px-4 py-2.5 text-[13px] text-text-2" data-testid="list-archived">
        <Archive class="h-4 w-4 shrink-0" />
        <span class="min-w-0 flex-1">{{ t('lists.archivedBanner') }}</span>
        <button v-if="editable" type="button" class="btn btn-sm shrink-0" data-testid="list-restore" @click="restoreList"><ArchiveRestore class="h-3.5 w-3.5" />{{ t('lists.restore') }}</button>
      </div>

      <div v-for="(s, i) in filteredOut ? [] : sections" :key="s.key" class="mb-6">
        <h3 v-if="sections.length > 1 || !list || grouping !== 'default'" class="sec-head" :class="{ 'text-crit-text': s.overdue }">
          <span v-if="s.color" class="h-2.5 w-2.5 rounded-full" :style="{ background: s.color }" />
          {{ s.title }}
          <span class="font-normal text-text-3">{{ s.tasks.filter((x) => !x.done).length || '' }}</span>
        </h3>
        <div :ref="(el) => { if (el) sectionEls[i] = el as HTMLElement }" class="rows" :data-group="s.key">
          <div v-for="x in s.tasks" :key="x.id" :data-id="x.id" class="row-wrap" :class="{ leaving: x.done && justDone.has(x.id) && !showDone }">
            <TaskRow
              :task="x"
              :color="listById.get(x.list_id)?.color"
              :list-name="!list && sel !== 'all' ? listById.get(x.list_id)?.name : ''"
              :subtasks="subCount(x)"
              :expanded="openSubs.has(x.id)"
              :assignees="peopleOf(x)"
              :members="members.length > 1 ? members : []"
              :editable="editable && listById.get(x.list_id)?.git?.write !== 'none'"
              @toggle="toggle(x)"
              @open="openTask = x.id"
              @update="(p) => change(x, p)"
              @expand="foldSubs(x.id)"
            />
            <div v-for="sub in openSubs.has(x.id) ? all.filter((y) => y.parent_id === x.id && (!y.done || showDone)).sort((a, b) => a.rank - b.rank) : []" :key="sub.id" class="pl-8">
              <TaskRow :task="sub" :color="listById.get(x.list_id)?.color" compact :editable="editable" @toggle="toggle(sub)" @open="openTask = sub.id" />
            </div>
          </div>
        </div>
        <div v-if="editable && sel !== 'completed' && !s.noAdd && !list?.archived_at" class="add-row">
          <template v-if="adding === s.key">
            <span class="check pointer-events-none" :style="{ '--check': s.color || heading.color }" />
            <input
              ref="addInput"
              v-model="draft"
              class="min-w-0 flex-1 bg-transparent text-[14.5px] outline-none"
              :placeholder="t('tasks.quickPlaceholder')"
              maxlength="500"
              @keydown.enter="commitAdd(s)"
              @keydown.esc="adding = ''"
              @blur="draft.trim() ? commitAdd(s) : (adding = '')"
            />
          </template>
          <button v-else class="flex items-center gap-2.5 text-[14.5px] text-text-3 hover:text-text-2" @click="startAdd(s.key)">
            <Plus class="h-5 w-5" :style="{ color: s.color || heading.color }" />{{ t('tasks.newTask') }}
          </button>
        </div>
      </div>

      <div v-if="filteredOut" class="empty">
        <ListFilter class="mx-auto mb-3 h-10 w-10 text-text-faint" :stroke-width="1.5" />
        <p class="title-3">{{ t('tasks.noMatch') }}</p>
        <p class="subhead mt-1">{{ t('tasks.noMatchHint') }}</p>
        <button class="btn mt-4" @click="clearFilter">{{ t('filter.clear') }}</button>
      </div>
      <div v-else-if="!sections.length" class="empty">
        <CheckCircle2 class="mx-auto mb-3 h-10 w-10 text-text-faint" :stroke-width="1.5" />
        <p class="title-3">{{ lists.length ? t('tasks.allDone') : t('tasks.noLists') }}</p>
        <p class="subhead mt-1">{{ lists.length ? t('tasks.allDoneHint') : t('tasks.noListsHint') }}</p>
        <button v-if="editable && !lists.length" class="btn-primary mt-4" @click="listSheet = { open: true, id: '' }"><ListPlus class="h-4 w-4" />{{ t('tasks.addList') }}</button>
        <div v-else-if="editable && sel !== 'completed'" class="mx-auto mt-5 max-w-sm">
          <input
            v-model="draft"
            class="field text-center"
            :placeholder="t('tasks.quickPlaceholder')"
            maxlength="500"
            @keydown.enter="commitAdd({ key: 'empty', title: '', tasks: [], listId: lists[0].id })"
          />
        </div>
      </div>
    </section>

    <TaskDetailSheet :open="!!openTask" :ws="ws" :task-id="openTask" @close="closeTask" />
    <ListSheet :open="listSheet.open" :ws="ws" :list-id="listSheet.id" @close="listSheet.open = false" @saved="onListSaved" />
    <SmartListSheet :open="smartSheet.open" :ws="ws" :id="smartSheet.id || undefined" :filter="smartSheet.filter" @close="smartSheet.open = false" @saved="onSmartSaved" @deleted="onSmartDeleted" />
  </div>
</template>

<style scoped>
.list-folder {
  display: inline-flex;
  max-width: 100%;
  align-items: center;
  gap: 6px;
  margin: -10px 0 18px;
  padding: 4px 10px;
  border-radius: 999px;
  background: var(--fill-2);
  font-size: 12.5px;
  color: var(--text);
}
.list-folder:hover {
  background: var(--fill);
}
.tasks-view {
  display: grid;
  grid-template-columns: 300px minmax(0, 1fr);
  min-height: 100%;
}
.tasks-view.one-pane {
  grid-template-columns: minmax(0, 1fr);
}
.lists-pane {
  padding: 28px 18px 32px;
  border-right: 0.5px solid var(--border);
  background: var(--grouped);
}
html.dark .lists-pane {
  background: #161618;
}
/* A holiday theme: the lists' column in its sidebar colour. */
html.holiday .lists-pane {
  background: var(--h-side);
}
.one-pane .lists-pane {
  border: 0;
  padding: 24px 16px;
}
.smart {
  display: flex;
  flex-direction: column;
  padding: 10px 12px;
  border-radius: 12px;
  background: var(--surface);
  text-align: left;
  box-shadow: 0 0 0 0.5px var(--border);
  transition: transform 0.15s var(--ease);
}
.smart:active {
  transform: scale(0.97);
}
.smart.on {
  box-shadow: 0 0 0 2px var(--accent);
}
.list-on {
  background: var(--accent-bg);
}
/* Archived lists: folded away under My Lists, quieter when open. */
.arch-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 14px 4px 6px;
  font-size: 13px;
  color: var(--text-2);
}
.arch-toggle:hover {
  color: var(--text);
}
.group.archived .group-row:not(.list-on) {
  opacity: 0.75;
}
.tasks-pane {
  padding: 22px clamp(16px, 4vw, 48px) 80px;
  width: 100%;
}
.sec-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 4px 4px;
  font: 600 17px var(--font-display);
  border-bottom: 0.5px solid var(--border);
}
.rows {
  min-height: 4px;
}
.row-wrap {
  border-bottom: 0.5px solid var(--line-soft);
  margin-left: 32px;
  padding-left: 0;
}
.row-wrap :deep(.task-row) {
  margin-left: -32px;
  padding-left: 4px;
}
.row-wrap.leaving {
  animation: leave 1.4s var(--ease) forwards;
}
@keyframes leave {
  0%,
  70% {
    opacity: 1;
  }
  100% {
    opacity: 0.35;
  }
}
.add-row {
  display: flex;
  align-items: center;
  gap: 11px;
  min-height: 42px;
  padding: 6px 4px;
}
.empty {
  padding: 64px 16px;
  text-align: center;
}
:deep(.sortable-ghost) {
  opacity: 0.35;
}
:deep(.sortable-drag) {
  background: var(--surface);
  box-shadow: var(--shadow);
  border-radius: 10px;
}
</style>
