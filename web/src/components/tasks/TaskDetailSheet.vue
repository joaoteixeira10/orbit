<script setup lang="ts">
import { ref, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { GraduationCap, Calendar, CalendarDays, Clock, Hourglass, Repeat, Flag, Hash, Plus, Check, ChevronDown, X, UserRound, ListTodo, GitBranch, TriangleAlert, Milestone, Timer, Target, Link2, ArrowRight, GripVertical, Maximize2, Minimize2, Trash2 } from '@lucide/vue'
import { estimateText } from '@/lib/weekPlan'
import { durationOf, useFocusStore } from '@/stores/focus'
import { goalsApi, type Goal } from '@/lib/goals'
import Overlay from '@/components/ui/Overlay.vue'
import { useSortable } from '@/lib/sortable'
import { Button } from '@/components/shadcn/button'
import { Input } from '@/components/shadcn/input'
import { Checkbox } from '@/components/shadcn/checkbox'
import { Badge } from '@/components/shadcn/badge'
import { Separator } from '@/components/shadcn/separator'
import { Switch } from '@/components/shadcn/switch'
import { Tabs, TabsList, TabsTrigger } from '@/components/shadcn/tabs'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/shadcn/popover'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/shadcn/select'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuTrigger } from '@/components/shadcn/dropdown-menu'
import { useTasksStore } from '@/stores/tasks'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useToastStore } from '@/stores/toast'
import { gitApi, type Column, type TaskInput, type GitMilestone } from '@/api/planner'
import { useAuthStore } from '@/stores/auth'
import GitBadge from './GitBadge.vue'
import Avatar from '@/components/ui/Avatar.vue'
import TaskComments from './TaskComments.vue'
import MarkdownView from '@/components/editor/MarkdownView.vue'
import { toggleTask } from '@/lib/markdown'
import CodeArea from '@/components/editor/CodeArea.vue'
import { todayStr, dateStr, addDays, parseDate, fmtDate, isOverdue, REPEAT_RULES, repeatLabel, repeatPreset, type RepeatPreset } from '@/lib/dates'
import { NativeSelect } from '@/components/shadcn/native-select'

// Everything about one task: its title and notes, how its date stands (with
// Today / Tomorrow / Next week), then Schedule (dates, time, repeat),
// Planning (status, priority, estimate, list…), People & tags, sub-tasks;
// Delete and Start focus below. Beside it (Planner's way) its Activity:
// comments, changes and files. Each field saves when it changes.
const props = defineProps<{ open: boolean; ws: string; taskId: string }>()
const emit = defineEmits<{ close: [] }>()
const tasks = useTasksStore()
const wss = useWorkspacesStore()
const toast = useToastStore()
const { t } = useI18n()

// (A task of an archived list opens too: from search, or the list itself.)
const task = computed(() => tasks.allTasksOf(props.ws).find((x) => x.id === props.taskId))
const lists = computed(() => tasks.listsOf(props.ws))
const list = computed(() => tasks.allListsOf(props.ws).find((l) => l.id === task.value?.list_id))
const subtasks = computed(() => (task.value ? tasks.subtasksOf(props.ws, task.value.id) : []))
const workspace = computed(() => wss.byId(props.ws))
// A task of a git-linked list is an issue there: changed only by someone with
// their own account on the server, or through the list's bot (never as
// someone else). Sub-tasks stay Orbit's.
const gitHost = computed(() => {
  try {
    return new URL(list.value?.git?.host ?? '').host
  } catch {
    return list.value?.git?.host ?? ''
  }
})
const gitReadOnly = computed(() => !!list.value?.git && list.value.git.write === 'none' && !task.value?.parent_id)
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !gitReadOnly.value)
const members = computed(() => wss.members[props.ws] ?? [])
const auth = useAuthStore()
const focusWs = computed(() => !!workspace.value?.modules.includes('focus'))
// Goals of the workspace, for the Goal row (loaded when it has the module).
const goalsWs = computed(() => !!workspace.value?.modules.includes('goals'))
const goalList = ref<Goal[]>([])
watch(
  () => [props.ws, goalsWs.value, props.open] as const,
  async ([ws, on, o]) => {
    if (!on || !o) return
    try {
      goalList.value = (await goalsApi.list(ws)).filter((g) => !g.achieved_at)
    } catch {
      goalList.value = []
    }
  },
  { immediate: true },
)
const schoolWs = computed(() => !!workspace.value && (workspace.value.modules.includes('courses') || workspace.value.modules.includes('grades')))
const assigned = computed(() => (task.value?.assignee_ids ?? []).map((id) => members.value.find((m) => m.user_id === id)).filter((m) => !!m))
const unassigned = computed(() => members.value.filter((m) => !task.value?.assignee_ids.includes(m.user_id)))
function invitePeople() {
  emit('close')
  window.dispatchEvent(new CustomEvent('orbit:settings', { detail: { ws: props.ws, tab: 'members' } }))
}
function setAssignees(ids: string[]) {
  void save({ assignee_ids: ids.filter(Boolean) })
}

const title = ref('')
// Comments go beside the details (a sub-task has none).
const sideComments = computed(() => !!task.value && !task.value.parent_id)
const notes = ref('')
// Notes are Markdown (as a linked git issue's body is): read rendered, with
// boxes that tick; a click (not on a link or box) or the pencil edits them.
const editingNotes = ref(false)
const notesEl = ref<InstanceType<typeof CodeArea> | null>(null)
// The notes box keeps one height; when they're longer it fades at the
// bottom (until scrolled to the end) and "Show all" lets it grow to fit.
const notesBox = ref<HTMLElement | null>(null)
const notesAll = ref(false)
const notesMore = ref(false)
const notesAtEnd = ref(true)
function measureNotes() {
  const el = notesBox.value
  notesMore.value = !!el && el.scrollHeight > el.clientHeight + 1
  notesAtEnd.value = !el || el.scrollTop + el.clientHeight >= el.scrollHeight - 2
}
const notesSizes = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(measureNotes)
watch(notesBox, (el) => {
  notesSizes?.disconnect()
  if (el) {
    notesSizes?.observe(el)
    if (el.firstElementChild) notesSizes?.observe(el.firstElementChild)
  }
  measureNotes()
})
watch([notes, notesAll], () => void nextTick(measureNotes))
onBeforeUnmount(() => notesSizes?.disconnect())
const tagDraft = ref('')
const subDraft = ref('')
watch(
  () => [props.open, props.taskId] as const,
  async ([o]) => {
    if (!o) return
    if (!tasks.tasks[props.ws]) await tasks.load(props.ws).catch(() => {})
    if (!wss.members[props.ws]) void wss.loadMembers(props.ws).catch(() => {})
    title.value = task.value?.title ?? ''
    notes.value = task.value?.notes ?? ''
    editingNotes.value = false
    notesAll.value = false
  },
  { immediate: true },
)
watch(task, (tk, prev) => {
  // Someone else changed it: take their text unless we're mid-edit.
  if (tk && prev && tk.id === prev.id) {
    if (document.activeElement?.id !== 'task-title') title.value = tk.title
    if (document.activeElement?.id !== 'task-notes') notes.value = tk.notes
  }
})

async function save(patch: TaskInput) {
  if (!task.value) return
  try {
    const r = await tasks.update(props.ws, task.value.id, patch)
    if (r.repeated) toast.success(t('tasks.repeated', { date: r.task.due }))
    if (r.shifted) toast.info(t('tasks.shifted', { n: r.shifted }, r.shifted))
  } catch (e: any) {
    toast.error(e.message)
  }
}
// ---- What it waits on (starts after) ----
const byId = computed(() => new Map(tasks.tasksOf(props.ws).map((x) => [x.id, x])))
const waits = computed(() => (task.value?.waits_on ?? []).map((id) => byId.value.get(id)).filter((x): x is NonNullable<typeof x> => !!x))
// Candidates: the workspace's other top-level tasks, not those that (in the
// end) wait on this one — that would be a loop.
const waitChoices = computed(() => {
  const me = task.value
  if (!me) return []
  const after = new Set<string>([me.id])
  let grew = true
  while (grew) {
    grew = false
    for (const x of tasks.tasksOf(props.ws)) {
      if (!after.has(x.id) && (x.waits_on ?? []).some((id) => after.has(id))) {
        after.add(x.id)
        grew = true
      }
    }
  }
  const have = new Set(me.waits_on ?? [])
  return tasks
    .tasksOf(props.ws)
    .filter((x) => !x.parent_id && !after.has(x.id) && !have.has(x.id))
    .sort((a, b) => Number(a.done) - Number(b.done) || (a.due ?? '9').localeCompare(b.due ?? '9') || a.title.localeCompare(b.title))
})
function addWait(id: string) {
  if (id && task.value) void save({ waits_on: [...(task.value.waits_on ?? []), id] })
}
function removeWait(id: string) {
  if (task.value) void save({ waits_on: (task.value.waits_on ?? []).filter((x) => x !== id) })
}

function saveTitle() {
  const v = title.value.trim()
  if (v && v !== task.value?.title) void save({ title: v })
  else title.value = task.value?.title ?? ''
}
function saveNotes() {
  editingNotes.value = false
  if (notes.value !== task.value?.notes) void save({ notes: notes.value })
}
async function editNotes() {
  if (!editable.value) return
  editingNotes.value = true
  await nextTick()
  notesEl.value?.focus('end')
}
// Escape the editor didn't use (it first goes back to one cursor) ends the
// editing, not the sheet.
function notesEsc(e: KeyboardEvent) {
  if (e.defaultPrevented) return
  e.stopPropagation()
  ;(document.activeElement as HTMLElement | null)?.blur()
}
function tickInNotes(i: number) {
  notes.value = toggleTask(notes.value, i)
  void save({ notes: notes.value })
}
// A click in the box's empty space below the text writes at its end.
function notesBoxDown(e: MouseEvent) {
  if (!editable.value || (e.target as HTMLElement).closest('.cm-editor')) return
  e.preventDefault()
  notesEl.value?.focus('end')
}
function notesClick(e: MouseEvent) {
  if (!(e.target as HTMLElement).closest('a, input, pre, button, .md-remote-img, img')) void editNotes()
}

// Date and time.
const quick = computed(() => [
  { label: t('dates.today'), v: todayStr() },
  { label: t('dates.tomorrow'), v: dateStr(addDays(new Date(), 1)) },
  { label: t('tasks.nextWeek'), v: dateStr(addDays(new Date(), 7)) },
])
function setTime(on: boolean) {
  void save({ due_time: on ? task.value?.due_time || '09:00' : '' })
}
const repeat = computed<RepeatPreset>(() => repeatPreset(task.value?.repeat ?? ''))
function setRepeat(p: string) {
  if (p === 'custom') return
  const rule = p ? REPEAT_RULES[p as keyof typeof REPEAT_RULES] : ''
  void save({ repeat: rule, ...(rule && !task.value?.due ? { due: todayStr() } : {}) })
}

// Tags.
function addTag() {
  const v = tagDraft.value.trim().replace(/^#/, '')
  if (!v || !task.value) return
  tagDraft.value = ''
  void save({ tags: [...task.value.tags, v] })
}
function removeTag(tag: string) {
  if (task.value) void save({ tags: task.value.tags.filter((x) => x !== tag) })
}

// Sub-tasks.
async function addSub() {
  const v = subDraft.value.trim()
  if (!v || !task.value) return
  subDraft.value = ''
  try {
    await tasks.create(props.ws, { title: v, parent_id: task.value.id })
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function toggleSub(id: string, done: boolean) {
  try {
    await tasks.update(props.ws, id, { completed: done })
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function removeSub(id: string) {
  await tasks.remove(props.ws, id).catch((e) => toast.error(e.message))
}
// A sub-task's name is changed in place (empty puts it back).
async function renameSub(id: string, was: string, el: HTMLInputElement) {
  const v = el.value.trim()
  if (!v || v === was) {
    el.value = was
    return
  }
  try {
    await tasks.update(props.ws, id, { title: v })
  } catch (e: any) {
    el.value = was
    toast.error(e.message)
  }
}
// Dragged by their grip into another order.
const subsEl = ref<HTMLElement | null>(null)
useSortable(
  subsEl,
  (m) => {
    const s = subtasks.value.find((x) => x.id === m.id)
    if (s) void tasks.move(props.ws, s.id, s.column_id, m.beforeId || '^').catch((e) => toast.error(e.message))
  },
  { handle: '.sub-grip', draggable: '[data-id]' },
)

// Git: the task's issue, or linking one by its address.
const gitOn = computed(() => auth.features.git_integration !== false)
const gitDraft = ref('')
const linking = ref(false)
async function linkGit() {
  const url = gitDraft.value.trim()
  if (!url || !task.value) return
  linking.value = true
  try {
    tasks.put(props.ws, await gitApi.linkTask(task.value.id, url))
    gitDraft.value = ''
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    linking.value = false
  }
}
// A linked list's task: its issue's milestone, from the repository's open ones.
const milestones = ref<GitMilestone[]>([])
watch(
  () => [task.value?.id, list.value?.git ? list.value.id : ''] as const,
  async ([id, listId]) => {
    milestones.value = []
    if (!id || !listId || !gitOn.value) return
    try {
      milestones.value = await gitApi.milestones(listId)
    } catch {
      /* the repository can't be read now: the current one still shows */
    }
  },
  { immediate: true },
)
function setMilestone(ref: number) {
  const m = milestones.value.find((x) => x.ref === ref)
  void save({ git_milestone_id: ref, git_milestone: m?.title ?? '' })
}
async function unlinkGit() {
  if (!task.value) return
  try {
    tasks.put(props.ws, await gitApi.unlinkTask(task.value.id))
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function remove() {
  if (!task.value) return
  try {
    await tasks.remove(props.ws, task.value.id)
    emit('close')
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ---- How its date stands, and the date chips ----
const router = useRouter()
const focus = useFocusStore()
const dayText = (s: string) => fmtDate(parseDate(s), { weekday: 'short', day: 'numeric', month: 'short' })
const late = computed(() => !!task.value && isOverdue(task.value.due, task.value.due_time, task.value.done))
const dueState = computed(() => {
  const tk = task.value
  if (!tk || tk.done) return null
  if (!tk.due) return { tone: 'none', text: t('tasks.noDate') }
  const days = Math.round((parseDate(tk.due).getTime() - parseDate(todayStr()).getTime()) / 86400000)
  if (late.value) return days < 0 ? { tone: 'late', text: t('tasks.overdueBy', { n: -days }, -days) } : { tone: 'late', text: t('tasks.overdue') }
  if (days === 0) return { tone: 'today', text: t('tasks.dueToday') }
  if (days === 1) return { tone: 'soon', text: t('tasks.dueTomorrow') }
  return { tone: 'soon', text: t('tasks.dueIn', { n: days }, days) }
})
const datePop = ref<'start' | 'due' | ''>('')
function setDay(which: 'start' | 'due', v: string) {
  if (which === 'due') void save(v ? { due: v } : { due: '', repeat: '' })
  else void save({ start: v })
  if (!v) datePop.value = ''
}

// Status: the list's columns, each with its dot, picked from a menu.
function colDot(c: Column, i: number): string {
  return c.color || (c.done ? 'var(--ok)' : i === 0 ? 'var(--text-3)' : 'var(--warn)')
}
const column = computed(() => {
  const cols = list.value?.columns ?? []
  const i = cols.findIndex((c) => c.id === task.value?.column_id)
  return i < 0 ? null : { c: cols[i], i }
})
function setStatus(id: string) {
  if (id !== task.value?.column_id) void save({ column_id: id })
}

// Estimate: any length, in hours and minutes (a week at most).
const estPop = ref(false)
const estH = ref<number | ''>('')
const estM = ref<number | ''>('')
// Opened: the current estimate in its boxes (the popover focuses the hours).
watch(estPop, (o) => {
  if (!o) return
  const e = task.value?.estimate ?? 0
  estH.value = e ? Math.floor(e / 60) : ''
  estM.value = e ? e % 60 : ''
})
function setEstimate(clear = false) {
  const min = clear ? 0 : Math.round(Number(estH.value || 0) * 60 + Number(estM.value || 0))
  if (min < 0 || min > 10080) return toast.error(t('tasks.estRange'))
  estPop.value = false
  if (min !== (task.value?.estimate ?? 0)) void save({ estimate: min })
}

// Repeat and goal in shadcn's Select (which takes no empty value).
const repeatSel = computed(() => repeat.value || 'never')
const goalSel = computed(() => task.value?.goal_id || 'none')

// Start focus: a session on it at once (the length picked last), shown on
// the focus timer.
const canFocus = computed(() => focusWs.value && !!task.value && !task.value.parent_id && !task.value.done)
async function startFocus() {
  if (!task.value) return
  let m = 25
  try {
    const x = Number(localStorage.getItem('orbit_focus_minutes'))
    if (x >= 1 && x <= 180) m = x
  } catch {
    /* private window */
  }
  try {
    await focus.start(props.ws, task.value.id, m)
    emit('close')
    void router.push({ name: 'ws-focus', params: { ws: props.ws }, query: { task: task.value.id } })
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <!-- shadcn's design (an icon in its colour on each row, shadcn's
       controls in bordered boxes), with everything the sheet does. -->
  <Overlay :open="open && !!task" :size="sideComments ? 'xxl' : 'lg'" bare @close="emit('close')">
    <div v-if="task" class="tsheet" :class="{ split: sideComments }" data-testid="task-sheet">
      <div class="t-scroll">
        <header class="t-head">
          <div class="flex items-start gap-3">
            <Checkbox
              :model-value="task.done"
              class="mt-1.5 size-5 rounded-full"
              :disabled="!editable"
              :aria-label="task.done ? t('tasks.reopen') : t('tasks.complete')"
              @update:model-value="(v) => save({ completed: v === true })"
            />
            <div class="min-w-0 flex-1">
              <textarea
                id="task-title"
                v-model="title"
                rows="1"
                class="title-area w-full resize-none bg-transparent text-[20px] font-semibold leading-snug outline-hidden"
                :class="{ 'text-text-3 line-through': task.done }"
                :readonly="!editable"
                maxlength="500"
                @blur="saveTitle"
                @keydown.enter.prevent="($event.target as HTMLTextAreaElement).blur()"
              />
              <p class="mt-0.5 flex min-w-0 items-center gap-1.5 text-[13px] text-text-2">
                <span v-if="list" class="size-2 shrink-0 rounded-full" :style="{ background: list.color || 'var(--ws)' }" />
                <span class="truncate">{{ list?.name }}<template v-if="workspace"> · {{ workspace.name }}</template></span>
              </p>
            </div>
            <Button variant="ghost" size="icon-sm" class="head-x -mr-1" :aria-label="t('common.close')" :title="t('common.close')" @click="emit('close')"><X /></Button>
          </div>
        </header>

        <div class="grid gap-4 px-5 pb-5">
          <!-- Notes: Markdown, read rendered; a click writes in them. -->
          <div>
            <div v-if="notes.trim() && !editingNotes" ref="notesBox" class="notes-box" :class="{ editable, all: notesAll, fade: notesMore && !notesAtEnd }" data-testid="task-notes-view" @click="notesClick" @scroll="measureNotes">
              <MarkdownView class="notes-md" :content="notes" :editable="editable" math breaks @toggle="tickInNotes" @edit="editNotes" />
            </div>
            <div v-else-if="editable || notes.trim()" ref="notesBox" class="math-wrap notes-area" :class="{ all: notesAll, fade: notesMore && !notesAtEnd }" data-testid="task-notes-box" @scroll="measureNotes" @keydown.esc="notesEsc" @mousedown="notesBoxDown" @focusin="editingNotes = editable">
              <CodeArea id="task-notes" ref="notesEl" v-model="notes" class="w-full" sums :placeholder="t('tasks.addNotes')" :readonly="!editable" :label="t('tasks.notesMd')" testid="task-notes" @blur="saveNotes" />
            </div>
            <Button v-if="notesMore || notesAll" variant="link" size="sm" class="mt-1 h-auto gap-1.5 px-1 text-text-2" data-testid="task-notes-more" @mousedown.prevent @click="notesAll = !notesAll">
              <Minimize2 v-if="notesAll" /><Maximize2 v-else />{{ notesAll ? t('tasks.notesLess') : t('tasks.notesAll') }}
            </Button>
          </div>

          <p v-if="gitReadOnly" class="rounded-lg border border-border px-3 py-2 text-[12.5px] text-warn-text" data-testid="git-read-only">{{ t('git.bot.readOnlyTask', { host: gitHost }) }}</p>

          <!-- How its date stands -->
          <div v-if="dueState" class="due-bar" :class="dueState.tone" data-testid="task-due-bar">
            <Calendar class="size-4 shrink-0" />
            <span class="due-text">{{ dueState.text }}</span>
            <span v-if="editable" class="flex flex-none gap-1.5">
              <Button v-for="q in quick" :key="q.v" size="xs" :variant="task.due === q.v ? 'default' : 'outline'" @click="save({ due: q.v })">{{ q.label }}</Button>
            </span>
          </div>

          <!-- When -->
          <div class="box">
            <div class="row">
              <CalendarDays class="ico text-crit" />
              <span class="flex-1">{{ t('tasks.dates') }}</span>
              <Popover :open="datePop === 'start'" @update:open="(o) => (datePop = o ? 'start' : '')">
                <PopoverTrigger as-child>
                  <Button variant="outline" size="sm" :class="{ 'text-text-3': !task.start }" :disabled="!editable" :title="t('tasks.startHint')" data-testid="task-start">{{ task.start ? dayText(task.start) : t('tasks.startsOn') }}</Button>
                </PopoverTrigger>
                <PopoverContent align="end" class="w-60" data-testid="task-date-pop">
                  <p class="text-[12px] font-medium text-text-3">{{ t('tasks.start') }}</p>
                  <Input type="date" :model-value="task.start ?? ''" :max="task.due ?? undefined" :aria-label="t('tasks.start')" @change="setDay('start', ($event.target as HTMLInputElement).value)" />
                  <p class="text-[12px] leading-snug text-text-3">{{ t('tasks.startHint') }}</p>
                  <Button v-if="task.start" variant="ghost" size="sm" class="w-full text-destructive" @click="setDay('start', '')">{{ t('tasks.removeDate') }}</Button>
                </PopoverContent>
              </Popover>
              <ArrowRight class="size-3.5 text-text-3" aria-hidden="true" />
              <Popover :open="datePop === 'due'" @update:open="(o) => (datePop = o ? 'due' : '')">
                <PopoverTrigger as-child>
                  <Button variant="outline" size="sm" :class="{ 'text-text-3': !task.due, 'border-destructive/40 bg-destructive/10 text-destructive hover:bg-destructive/15 hover:text-destructive': late }" :disabled="!editable" data-testid="task-due">{{ task.due ? dayText(task.due) : t('tasks.addDate') }}</Button>
                </PopoverTrigger>
                <PopoverContent align="end" class="w-60" data-testid="task-date-pop">
                  <p class="text-[12px] font-medium text-text-3">{{ t('tasks.date') }}</p>
                  <Input type="date" :model-value="task.due ?? ''" :aria-label="t('tasks.date')" @change="setDay('due', ($event.target as HTMLInputElement).value)" />
                  <Button v-if="task.due" variant="ghost" size="sm" class="w-full text-destructive" @click="setDay('due', '')">{{ t('tasks.removeDate') }}</Button>
                </PopoverContent>
              </Popover>
            </div>
            <template v-if="task.due">
              <Separator />
              <div class="row">
                <Clock class="ico text-accent" />
                <span class="flex-1">{{ t('tasks.time') }}</span>
                <template v-if="task.due_time">
                  <Input type="time" class="h-7 w-32" :model-value="task.due_time" :disabled="!editable" :aria-label="t('tasks.time')" @change="save({ due_time: ($event.target as HTMLInputElement).value })" />
                  <Button v-if="editable" variant="ghost" size="icon-sm" :aria-label="t('tasks.noTime')" :title="t('tasks.noTime')" @click="setTime(false)"><X /></Button>
                </template>
                <Button v-else-if="editable" variant="link" size="sm" class="px-0" @click="setTime(true)">{{ t('tasks.addTime') }}</Button>
              </div>
            </template>
            <Separator />
            <div class="row">
              <Repeat class="ico text-text-3" />
              <span class="flex-1">{{ t('tasks.repeat') }}</span>
              <Select :model-value="repeatSel" :disabled="!editable" @update:model-value="(v) => setRepeat(v === 'never' ? '' : String(v))">
                <SelectTrigger size="sm" class="w-40" :aria-label="t('tasks.repeat')"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="never">{{ t('repeat.never') }}</SelectItem>
                  <SelectItem v-for="r in ['daily', 'weekdays', 'weekly', 'biweekly', 'monthly', 'yearly']" :key="r" :value="r">{{ t(`repeat.${r}`) }}</SelectItem>
                  <SelectItem v-if="repeat === 'custom'" value="custom">{{ repeatLabel(task.repeat ?? '') }}</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <!-- Details -->
          <div class="box">
            <div class="row">
              <ListTodo class="ico text-accent" />
              <span class="flex-1">{{ t('tasks.list') }}</span>
              <Select :model-value="task.list_id" :disabled="!editable || !!task.parent_id" @update:model-value="(v) => save({ list_id: String(v) })">
                <SelectTrigger size="sm" class="w-40" :aria-label="t('tasks.list')"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem v-if="list?.archived_at" :value="list.id">{{ list.name }} · {{ t('lists.archivedTag') }}</SelectItem>
                  <SelectItem v-for="l in lists" :key="l.id" :value="l.id">{{ l.name }}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <template v-if="list">
              <Separator />
              <div class="row">
                <span class="ico grid place-items-center"><span class="size-2.5 rounded-full" :style="{ background: column ? colDot(column.c, column.i) : 'var(--text-3)' }" /></span>
                <span class="flex-1">{{ t('tasks.status') }}</span>
                <DropdownMenu :modal="false">
                  <DropdownMenuTrigger as-child>
                    <Button variant="outline" size="sm" class="w-40 justify-between font-normal" :disabled="!editable || !!task.parent_id" :aria-label="t('tasks.status')" data-testid="task-status"><span class="min-w-0 truncate">{{ column?.c.name ?? '–' }}</span><ChevronDown class="text-text-3" /></Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" class="w-52" data-testid="task-status-menu">
                    <DropdownMenuRadioGroup :model-value="task.column_id" @update:model-value="(v) => setStatus(String(v))">
                      <DropdownMenuRadioItem v-for="(c, i) in list.columns" :key="c.id" :value="c.id">
                        <span class="size-2 rounded-full" :style="{ background: colDot(c, i) }" />{{ c.name }}
                      </DropdownMenuRadioItem>
                    </DropdownMenuRadioGroup>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </template>
            <Separator />
            <div class="row">
              <Flag class="ico text-warn" />
              <span class="flex-1">{{ t('tasks.priority') }}</span>
              <Tabs :model-value="String(task.priority)" @update:model-value="(v) => editable && save({ priority: Number(v) })">
                <TabsList>
                  <TabsTrigger v-for="p in [0, 1, 2, 3]" :key="p" :value="String(p)" :disabled="!editable" :aria-label="t(`tasks.prio.${p}`)" :title="t(`tasks.prio.${p}`)">{{ p === 0 ? '–' : '!'.repeat(p) }}</TabsTrigger>
                </TabsList>
              </Tabs>
            </div>
            <Separator />
            <div class="row">
              <Hourglass class="ico text-accent" />
              <span class="flex-1">{{ t('tasks.estimate') }}</span>
              <Popover v-model:open="estPop">
                <PopoverTrigger as-child>
                  <Button variant="outline" size="sm" :class="{ 'text-text-3': !task.estimate }" :disabled="!editable" :aria-label="t('tasks.estimate')" data-testid="task-estimate">{{ task.estimate ? estimateText(task.estimate) : t('tasks.noEstimate') }}</Button>
                </PopoverTrigger>
                <PopoverContent align="end" class="w-64" data-testid="task-estimate-pop">
                  <form class="grid gap-2.5" @submit.prevent="setEstimate()">
                    <p class="text-[12px] font-medium text-text-3">{{ t('tasks.estimateHint') }}</p>
                    <div class="flex items-center gap-2">
                      <Input v-model.number="estH" type="number" min="0" max="168" step="1" inputmode="numeric" class="w-16 text-center" placeholder="0" :aria-label="t('tasks.estHours')" />
                      <span class="text-[13px] text-text-2">h</span>
                      <Input v-model.number="estM" type="number" min="0" max="59" step="5" inputmode="numeric" class="w-16 text-center" placeholder="0" :aria-label="t('tasks.estMinutes')" />
                      <span class="text-[13px] text-text-2">min</span>
                    </div>
                    <div class="flex gap-2">
                      <Button v-if="task.estimate" type="button" variant="ghost" size="sm" class="flex-1 text-destructive" @click="setEstimate(true)">{{ t('tasks.estClear') }}</Button>
                      <Button type="submit" size="sm" class="flex-1">{{ t('tasks.estSet') }}</Button>
                    </div>
                  </form>
                </PopoverContent>
              </Popover>
            </div>
            <template v-if="schoolWs && !task.parent_id">
              <Separator />
              <div class="row">
                <GraduationCap class="ico text-crit" />
                <span class="min-w-0 flex-1">
                  <span class="block">{{ t('school.exam') }}</span>
                  <span class="block text-[12px] text-text-3">{{ t('school.examHint') }}</span>
                </span>
                <Switch :model-value="task.kind === 'exam'" :disabled="!editable" :aria-label="t('school.exam')" @update:model-value="(v) => save({ kind: v ? 'exam' : 'task' })" />
              </div>
            </template>
            <template v-if="goalsWs && !task.parent_id && (goalList.length || task.goal_id)">
              <Separator />
              <div class="row" data-testid="task-goal">
                <Target class="ico text-[#ff2d55]" />
                <span class="flex-1">{{ t('goals.field') }}</span>
                <Select :model-value="goalSel" :disabled="!editable" @update:model-value="(v) => save({ goal_id: v === 'none' ? '' : String(v) })">
                  <SelectTrigger size="sm" class="w-40" :aria-label="t('goals.field')"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">{{ t('goals.noGoal') }}</SelectItem>
                    <SelectItem v-for="g in goalList" :key="g.id" :value="g.id">{{ g.icon ? `${g.icon} ` : '' }}{{ g.title }}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </template>
            <template v-if="!task.parent_id && (waits.length || (editable && waitChoices.length))">
              <Separator />
              <div class="row items-start!" data-testid="task-waits">
                <Link2 class="ico mt-1.5 text-text-3" />
                <div class="min-w-0 flex-1">
                  <div class="flex min-h-8 items-center gap-2">
                    <span class="flex-1">{{ t('tasks.waitsOn') }}</span>
                    <DropdownMenu v-if="editable && waitChoices.length" :modal="false">
                      <DropdownMenuTrigger as-child><Button variant="outline" size="sm" data-testid="waits-add">{{ t('tasks.waitsAddShort') }}<ChevronDown class="text-text-3" /></Button></DropdownMenuTrigger>
                      <DropdownMenuContent align="end" class="max-h-72 w-64">
                        <DropdownMenuLabel>{{ t('tasks.waitsAdd') }}</DropdownMenuLabel>
                        <DropdownMenuItem v-for="x in waitChoices" :key="x.id" @select="addWait(x.id)"><Check v-if="x.done" class="text-ok" />{{ x.title }}</DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                  <div v-if="waits.length" class="mt-1.5 flex flex-wrap gap-1.5">
                    <Badge v-for="w in waits" :key="w.id" variant="secondary" class="gap-1" :class="{ 'line-through opacity-60': w.done }">
                      {{ w.title }}
                      <button v-if="editable" type="button" class="opacity-60 hover:opacity-100" :aria-label="t('tasks.waitsRemove', { title: w.title })" @click="removeWait(w.id)"><X class="size-3" /></button>
                    </Badge>
                  </div>
                  <p v-if="waits.length" class="mt-1 text-[12px] text-text-3">{{ t('tasks.waitsHint') }}</p>
                </div>
              </div>
            </template>
            <Separator />
            <div class="row" data-testid="assignees">
              <UserRound class="ico text-ok" />
              <span class="flex-1">{{ t('tasks.assignee') }}</span>
              <div class="flex flex-wrap items-center justify-end gap-1.5">
                <Badge v-for="m in assigned" :key="m.user_id" variant="secondary" class="person h-6 gap-1.5 pl-0.5">
                  <Avatar :name="m.display_name || m.username" :src="m.avatar_url" :size="20" />{{ (m.display_name || m.username).split(' ')[0] }}
                  <button v-if="editable" type="button" class="opacity-60 hover:opacity-100" :aria-label="t('tasks.removeAssignee', { name: m.display_name || m.username })" @click="setAssignees(task.assignee_ids.filter((x) => x !== m.user_id))"><X class="size-3" /></button>
                </Badge>
                <span v-if="!assigned.length && !editable" class="text-[13px] text-text-3">{{ t('tasks.nobody') }}</span>
                <DropdownMenu v-if="editable && unassigned.length" :modal="false">
                  <DropdownMenuTrigger as-child><Button variant="outline" size="icon-xs" class="rounded-full" :aria-label="t('tasks.addAssignee')" :title="t('tasks.addAssignee')" data-testid="assign-add"><Plus /></Button></DropdownMenuTrigger>
                  <DropdownMenuContent align="end" class="max-h-72 w-60">
                    <DropdownMenuItem v-for="m in unassigned" :key="m.user_id" @select="setAssignees([...task.assignee_ids, m.user_id])">
                      <Avatar :name="m.display_name || m.username" :src="m.avatar_url" :size="20" />{{ m.display_name || m.username }}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
                <Button v-if="members.length <= 1 && workspace?.role === 'owner'" variant="link" size="sm" class="px-0" @click="invitePeople">{{ t('tasks.invitePeople') }}</Button>
              </div>
            </div>
            <Separator />
            <div class="row flex-wrap">
              <Hash class="ico text-text-3" />
              <span class="mr-auto">{{ t('tasks.tags') }}</span>
              <Badge v-for="tag in task.tags" :key="tag" variant="secondary" class="gap-1">
                #{{ tag }}
                <button v-if="editable" type="button" class="opacity-60 hover:opacity-100" :aria-label="t('tasks.removeTag', { tag })" @click="removeTag(tag)"><X class="size-3" /></button>
              </Badge>
              <Input v-if="editable" v-model="tagDraft" :placeholder="t('tasks.addTag')" :aria-label="t('tasks.addTag')" class="h-7 w-28" data-testid="tag-add" @keydown.enter.prevent="addTag" @blur="addTag" />
            </div>
          </div>

          <!-- Its issue -->
          <div v-if="gitOn && !task.parent_id && (task.git_number || list?.git || editable)">
            <p class="mb-2 flex items-center gap-1.5 text-[14px] font-medium text-text-2"><GitBranch class="size-3.5" />{{ t('git.task.title') }}</p>
            <div class="box">
              <div v-if="task.git_number" class="row">
                <div class="min-w-0 flex-1 text-[13.5px]">
                  <span class="text-text-2">{{ task.git_repo }}</span>
                  <GitBadge :task="task" class="ml-1.5" />
                  <span class="ml-1.5 text-[12.5px] text-text-3">{{ t(`git.task.state.${task.git_state || 'open'}`) }}</span>
                </div>
                <Button v-if="editable" variant="link" size="sm" class="shrink-0 text-destructive" @click="unlinkGit">{{ t('git.task.unlink') }}</Button>
              </div>
              <template v-if="task.git_number && list?.git">
                <Separator />
                <label class="row">
                  <Milestone class="ico text-text-3" />
                  <span class="flex-1">{{ t('git.task.milestone') }}</span>
                  <NativeSelect size="sm" :model-value="task.git_milestone_id || 0" class="w-auto max-w-[60%]" :disabled="!editable" data-testid="milestone" @change="setMilestone(Number(($event.target as HTMLSelectElement).value))">
                    <option :value="0">{{ t('git.task.noMilestone') }}</option>
                    <option v-if="task.git_milestone_id && !milestones.some((m) => m.ref === task!.git_milestone_id)" :value="task.git_milestone_id">{{ task.git_milestone }}</option>
                    <option v-for="m in milestones" :key="m.ref" :value="m.ref">{{ m.title }}</option>
                  </NativeSelect>
                </label>
              </template>
              <form v-else-if="editable && !list?.git" class="row" @submit.prevent="linkGit">
                <Input v-model="gitDraft" type="url" class="h-8 flex-1" :placeholder="t('git.task.placeholder')" :aria-label="t('git.task.placeholder')" />
                <Button type="submit" variant="outline" size="sm" class="shrink-0" :disabled="!gitDraft.trim() || linking">{{ t('git.task.link') }}</Button>
              </form>
              <div v-if="task.git_dirty" class="row text-[12.5px] text-warn-text"><TriangleAlert class="size-3.5 shrink-0" />{{ t('git.task.pending') }}</div>
            </div>
            <p class="mt-1.5 px-1 text-[12px] leading-snug text-text-3">
              {{ list?.git ? t('git.task.inList', { repo: `${list.git.owner}/${list.git.repo}` }) : t('git.task.hint') }}
            </p>
          </div>

          <!-- Sub-tasks -->
          <div v-if="!task.parent_id">
            <p class="mb-2 text-[14px] font-medium text-text-2">{{ t('tasks.subtasks') }}</p>
            <div class="box">
              <div v-if="subtasks.length" ref="subsEl" class="subs">
                <div v-for="s in subtasks" :key="s.id" :data-id="s.id" class="group/sub" data-testid="task-sub">
                  <div class="row">
                    <GripVertical v-if="editable && subtasks.length > 1" data-testid="task-sub-grip" class="sub-grip size-4 shrink-0 cursor-grab text-text-3" :aria-label="t('tasks.dragSub')" />
                    <Checkbox :model-value="s.done" class="rounded-full" :disabled="!editable" :aria-label="s.title" @update:model-value="(v) => toggleSub(s.id, v === true)" />
                    <input
                      v-if="editable"
                      :value="s.title"
                      class="min-w-0 flex-1 bg-transparent outline-hidden"
                      :class="{ 'text-text-3 line-through': s.done }"
                      :aria-label="t('tasks.renameSub')"
                      maxlength="500"
                      data-testid="task-sub-title"
                      @keydown.enter="($event.target as HTMLInputElement).blur()"
                      @keydown.esc="(($event.target as HTMLInputElement).value = s.title), ($event.target as HTMLInputElement).blur()"
                      @blur="renameSub(s.id, s.title, $event.target as HTMLInputElement)"
                    />
                    <span v-else class="flex-1" :class="{ 'text-text-3 line-through': s.done }">{{ s.title }}</span>
                    <Button v-if="editable" variant="ghost" size="icon-xs" class="opacity-0 group-hover/sub:opacity-100 focus-visible:opacity-100 pointer-coarse:opacity-100" :aria-label="t('common.delete')" @click="removeSub(s.id)"><X /></Button>
                  </div>
                </div>
              </div>
              <div v-if="editable" class="row">
                <Plus class="ico text-text-3" />
                <input v-model="subDraft" class="flex-1 bg-transparent outline-hidden" :placeholder="t('tasks.addSubtask')" @keydown.enter="addSub" />
              </div>
              <p v-if="!subtasks.length && !editable" class="row text-text-3">–</p>
            </div>
          </div>
        </div>
      </div>

      <!-- Activity: any member, viewers too; beside the details when wide -->
      <aside v-if="sideComments" class="t-side" data-testid="task-side">
        <TaskComments
          :ws="ws"
          :task-id="task.id"
          :can-write="!!workspace && !workspace.archived_at && !auth.privileges.read_only"
          :can-attach="editable"
          :is-owner="workspace?.role === 'owner'"
          :members="members"
          side
          @close="emit('close')"
        />
      </aside>

      <footer v-if="editable || canFocus" class="t-foot">
        <Button v-if="editable" variant="link" class="px-1 text-destructive" @click="remove"><Trash2 />{{ t('common.delete') }}</Button>
        <span v-if="focusWs && task.focus_minutes" class="ml-auto flex items-center gap-1.5 text-[12.5px] text-text-3" data-testid="task-focus"><Timer class="size-3.5" />{{ t('focus.spent', { t: durationOf(task.focus_minutes) }) }}</span>
        <Button v-if="canFocus" :class="{ 'ml-auto': !task.focus_minutes }" data-testid="task-start-focus" @click="startFocus"><Timer />{{ t('focus.startFocus') }}</Button>
        <Button v-else class="ml-auto" @click="emit('close')">{{ t('common.done') }}</Button>
      </footer>
    </div>
  </Overlay>
</template>

<style scoped>
/* One column: everything scrolls, Delete / Start focus stay at the bottom.
   Wide (a task with its activity): the details on the left — header, the
   fields scrolling, the buttons — and the activity on the right. */
.tsheet {
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 1;
  overflow-y: auto;
  background: var(--sheet-bg);
}
.t-head {
  padding: 20px 20px 12px;
}
.t-foot {
  position: sticky;
  bottom: 0;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 20px;
  border-top: 1px solid var(--border);
  background: inherit;
}
.t-side {
  border-top: 1px solid var(--border);
}
@media (min-width: 900px) {
  .tsheet.split {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 380px;
    grid-template-rows: minmax(0, 1fr) auto;
    grid-template-areas: 'main side' 'foot side';
    height: min(88vh, 880px);
    overflow: hidden;
  }
  .split .t-scroll {
    grid-area: main;
    min-height: 0;
    overflow-y: auto;
  }
  .split .t-foot {
    grid-area: foot;
    position: static;
  }
  .split .t-side {
    grid-area: side;
    min-height: 0;
    border-top: 0;
    border-left: 1px solid var(--border);
  }
  .split .head-x {
    display: none;
  }
}
.title-area {
  field-sizing: content;
}
/* Grouped boxes: bordered, a row per field, an icon in its colour. */
.box {
  border: 1px solid var(--border);
  border-radius: 12px;
}
.row {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 44px;
  padding: 6px 12px;
  font-size: 14px;
}
.ico {
  width: 18px;
  height: 18px;
  flex-shrink: 0;
}
/* The sub-tasks (to be dragged), lined like the rows. */
.subs > * {
  border-bottom: 1px solid var(--border);
}
/* Notes: in a box of their own (as shadcn's textarea), always the same height
   (about six lines), empty or long, scrolling inside, so the fields below
   never move; "Show all" lets them grow. */
.notes-area,
.notes-box {
  --notes-h: 136px;
  height: var(--notes-h);
  overflow-y: auto;
  padding: 8px 10px;
  border: 1px solid var(--border-2);
  border-radius: 10px;
  font-size: 14px;
  line-height: 1.55;
}
.notes-area {
  cursor: text;
  color: var(--text);
}
.notes-area:focus-within {
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--ring);
  font-family: var(--font-mono);
  font-size: 13px;
}
.notes-area :deep(.cm-content) {
  min-height: 1.6em;
}
.notes-box.editable {
  cursor: text;
}
.notes-area.all,
.notes-box.all {
  height: auto;
  min-height: var(--notes-h);
}
/* More below: the last lines fade out until scrolled to (not the border). */
.notes-area.fade,
.notes-box.fade {
  mask-image: linear-gradient(to bottom, #000 calc(100% - 36px), rgb(0 0 0 / 0.15) calc(100% - 1px), #000 calc(100% - 1px));
}
.notes-box .notes-md {
  font-size: 14px;
  line-height: 1.55;
  padding-bottom: 0;
}
/* How its date stands. */
.due-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 6px 6px 6px 12px;
  border: 1px solid var(--border);
  border-radius: 12px;
  font-size: 13.5px;
  font-weight: 500;
  color: var(--text-2);
}
.due-bar.late {
  border-color: var(--crit-border);
  background: var(--crit-bg);
  color: var(--crit-text);
}
.due-bar.today {
  border-color: var(--warn-border);
  background: var(--warn-bg);
  color: var(--warn-text);
}
/* The words first; on a narrow sheet the buttons go under them. */
.due-text {
  flex: 1 0 auto;
}
</style>
