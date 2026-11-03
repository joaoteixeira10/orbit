<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { GraduationCap, Calendar, ChevronRight, Clock, Hourglass, Repeat, Flag, Tag, ListTree, Plus, Check, ChevronDown, X, UserRound, Columns3, List, GitBranch, TriangleAlert, Milestone, Timer, Target, Link2, ArrowRight } from 'lucide-vue-next'
import { estimateText } from '@/lib/weekPlan'
import { durationOf, useFocusStore } from '@/stores/focus'
import { goalsApi, type Goal } from '@/lib/goals'
import Overlay from '@/components/ui/Overlay.vue'
import Popover from '@/components/ui/Popover.vue'
import Toggle from '@/components/ui/Toggle.vue'
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

const task = computed(() => tasks.tasksOf(props.ws).find((x) => x.id === props.taskId))
const lists = computed(() => tasks.listsOf(props.ws))
const list = computed(() => lists.value.find((l) => l.id === task.value?.list_id))
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
const startPill = ref<HTMLElement | null>(null)
const duePill = ref<HTMLElement | null>(null)
function setDay(which: 'start' | 'due', v: string) {
  if (which === 'due') void save(v ? { due: v } : { due: '', repeat: '' })
  else void save({ start: v })
  if (!v) datePop.value = ''
}

// Status: the list's columns, each with its dot, picked from a menu.
function colDot(c: Column, i: number): string {
  return c.color || (c.done ? 'var(--ok)' : i === 0 ? 'var(--text-3)' : 'var(--warn)')
}
const statusPop = ref(false)
const statusBtn = ref<HTMLElement | null>(null)
const column = computed(() => {
  const cols = list.value?.columns ?? []
  const i = cols.findIndex((c) => c.id === task.value?.column_id)
  return i < 0 ? null : { c: cols[i], i }
})
function setStatus(id: string) {
  statusPop.value = false
  if (id !== task.value?.column_id) void save({ column_id: id })
}

// Estimate: any length, in hours and minutes (a week at most).
const estPop = ref(false)
const estBtn = ref<HTMLElement | null>(null)
const estH = ref<number | ''>('')
const estM = ref<number | ''>('')
const estHEl = ref<HTMLInputElement | null>(null)
async function openEstimate() {
  const e = task.value?.estimate ?? 0
  estH.value = e ? Math.floor(e / 60) : ''
  estM.value = e ? e % 60 : ''
  estPop.value = true
  await nextTick()
  estHEl.value?.select()
}
function setEstimate(clear = false) {
  const min = clear ? 0 : Math.round(Number(estH.value || 0) * 60 + Number(estM.value || 0))
  if (min < 0 || min > 10080) return toast.error(t('tasks.estRange'))
  estPop.value = false
  if (min !== (task.value?.estimate ?? 0)) void save({ estimate: min })
}

// People and tags.
const assignPop = ref(false)
const assignBtn = ref<HTMLElement | null>(null)
const tagging = ref(false)
const tagEl = ref<HTMLInputElement | null>(null)
async function openTag() {
  tagging.value = true
  await nextTick()
  tagEl.value?.focus()
}
function tagBlur() {
  addTag()
  tagging.value = false
}

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
  <Overlay :open="open && !!task" :size="sideComments ? 'xxl' : 'lg'" bare @close="emit('close')">
    <div v-if="task" class="tsheet" :class="{ split: sideComments }" data-testid="task-sheet">
      <!-- The task -->
      <header class="t-head">
        <div class="flex items-center gap-2">
          <span v-if="list" class="crumb"><span class="crumb-dot" :style="{ background: list.color || 'var(--ws)' }" />{{ list.name }}</span>
          <button type="button" class="icon-btn head-x ml-auto h-7 w-7" :aria-label="t('common.close')" :title="t('common.close')" @click="emit('close')"><X class="h-4 w-4" /></button>
        </div>
        <div class="mt-1.5 flex items-start gap-3">
          <button
            class="check mt-1 !h-6 !w-6"
            :class="{ on: task.done }"
            :style="{ '--check': list?.color || 'var(--ws)' }"
            :disabled="!editable"
            :aria-label="task.done ? t('tasks.reopen') : t('tasks.complete')"
            @click="save({ completed: !task.done })"
          >
            <Check class="h-3.5 w-3.5" :stroke-width="3.4" />
          </button>
          <div class="min-w-0 flex-1">
            <textarea
              id="task-title"
              v-model="title"
              rows="1"
              class="title-area w-full resize-none bg-transparent text-[22px] font-bold leading-snug outline-none"
              :class="{ 'text-text-3 line-through': task.done }"
              :readonly="!editable"
              maxlength="500"
              @blur="saveTitle"
              @keydown.enter.prevent="($event.target as HTMLTextAreaElement).blur()"
            />
            <div v-if="notes.trim() && !editingNotes" class="notes-box" :class="{ editable }" data-testid="task-notes-view" @click="notesClick">
              <MarkdownView class="notes-md" :content="notes" :editable="editable" math @toggle="tickInNotes" @edit="editNotes" />
            </div>
            <div v-else-if="editable || notes.trim()" class="math-wrap" @keydown.esc="notesEsc">
              <CodeArea
                id="task-notes"
                ref="notesEl"
                v-model="notes"
                class="notes-area w-full"
                sums
                :placeholder="t('tasks.addNotes')"
                :readonly="!editable"
                :label="t('tasks.notesMd')"
                testid="task-notes"
                @blur="saveNotes"
              />
            </div>
          </div>
        </div>
        <p v-if="gitReadOnly" class="mt-2 rounded-xl bg-fill-2 px-3 py-2 text-[12.5px] text-warn-text" data-testid="git-read-only">{{ t('git.bot.readOnlyTask', { host: gitHost }) }}</p>
        <div v-if="dueState" class="due-bar" :class="dueState.tone" data-testid="task-due-bar">
          <Calendar class="h-4 w-4 shrink-0" />
          <span class="due-text">{{ dueState.text }}</span>
          <span v-if="editable" class="flex flex-none gap-1.5">
            <button v-for="q in quick" :key="q.v" type="button" class="due-chip" :class="{ on: task.due === q.v }" @click="save({ due: q.v })">{{ q.label }}</button>
          </span>
        </div>
      </header>

      <div class="t-body">
        <!-- Schedule -->
        <h3 class="sec">{{ t('tasks.schedule') }}</h3>
        <div class="group">
          <div class="group-row">
            <span class="tile h-7 w-7" style="--tile: #ff3b30"><Calendar class="h-4 w-4" /></span>
            <span class="flex-1 text-[14px]">{{ t('tasks.dates') }}</span>
            <div class="flex flex-wrap items-center justify-end gap-1.5">
              <button ref="startPill" type="button" class="day-pill" :class="{ ghost: !task.start }" :disabled="!editable" :title="t('tasks.startHint')" data-testid="task-start" @click="datePop = 'start'">{{ task.start ? dayText(task.start) : t('tasks.startsOn') }}</button>
              <ArrowRight class="h-3.5 w-3.5 text-text-3" aria-hidden="true" />
              <button ref="duePill" type="button" class="day-pill" :class="{ ghost: !task.due, late }" :disabled="!editable" data-testid="task-due" @click="datePop = 'due'">{{ task.due ? dayText(task.due) : t('tasks.addDate') }}</button>
            </div>
          </div>
          <div v-if="task.due" class="group-row">
            <span class="tile h-7 w-7" style="--tile: #007aff"><Clock class="h-4 w-4" /></span>
            <span class="flex-1 text-[14px]">{{ t('tasks.time') }}</span>
            <template v-if="task.due_time">
              <input type="time" class="day-pill" :value="task.due_time" :disabled="!editable" :aria-label="t('tasks.time')" @change="save({ due_time: ($event.target as HTMLInputElement).value })" />
              <button v-if="editable" type="button" class="icon-btn h-6 w-6" :aria-label="t('tasks.noTime')" :title="t('tasks.noTime')" @click="setTime(false)"><X class="h-3.5 w-3.5" /></button>
            </template>
            <button v-else-if="editable" type="button" class="text-[14px] text-accent-text hover:underline" @click="setTime(true)">{{ t('tasks.addTime') }}</button>
          </div>
          <div class="group-row">
            <span class="tile h-7 w-7" style="--tile: #8e8e93"><Repeat class="h-4 w-4" /></span>
            <span class="flex-1 text-[14px]">{{ t('tasks.repeat') }}</span>
            <label class="plain-select">
              <select :value="repeat" :disabled="!editable" :aria-label="t('tasks.repeat')" @change="setRepeat(($event.target as HTMLSelectElement).value)">
                <option value="">{{ t('repeat.never') }}</option>
                <option value="daily">{{ t('repeat.daily') }}</option>
                <option value="weekdays">{{ t('repeat.weekdays') }}</option>
                <option value="weekly">{{ t('repeat.weekly') }}</option>
                <option value="biweekly">{{ t('repeat.biweekly') }}</option>
                <option value="monthly">{{ t('repeat.monthly') }}</option>
                <option value="yearly">{{ t('repeat.yearly') }}</option>
                <option v-if="repeat === 'custom'" value="custom">{{ repeatLabel(task?.repeat ?? '') }}</option>
              </select>
              <ChevronRight class="h-3.5 w-3.5" aria-hidden="true" />
            </label>
          </div>
        </div>

        <!-- Planning -->
        <h3 class="sec">{{ t('tasks.planning') }}</h3>
        <div class="group">
          <div v-if="list" class="group-row">
            <span class="tile h-7 w-7" style="--tile: #30b0c7"><Columns3 class="h-4 w-4" /></span>
            <span class="flex-1 text-[14px]">{{ t('tasks.status') }}</span>
            <button ref="statusBtn" type="button" class="pick" :disabled="!editable || !!task.parent_id" :aria-label="t('tasks.status')" aria-haspopup="menu" data-testid="task-status" @click="statusPop = !statusPop">
              <span v-if="column" class="dot" :style="{ background: colDot(column.c, column.i) }" />{{ column?.c.name ?? '–' }}
              <ChevronDown v-if="editable && !task.parent_id" class="h-3.5 w-3.5 text-text-3" aria-hidden="true" />
            </button>
          </div>
          <div class="group-row">
            <span class="tile h-7 w-7" style="--tile: #ff9500"><Flag class="h-4 w-4" /></span>
            <span class="flex-1 text-[14px]">{{ t('tasks.priority') }}</span>
            <div class="seg">
              <button v-for="p in [0, 1, 2, 3]" :key="p" :class="{ on: task.priority === p }" :disabled="!editable" :aria-label="t(`tasks.prio.${p}`)" :title="t(`tasks.prio.${p}`)" @click="save({ priority: p })">
                {{ p === 0 ? '–' : '!'.repeat(p) }}
              </button>
            </div>
          </div>
          <div class="group-row">
            <span class="tile h-7 w-7" style="--tile: #30b0c7"><Hourglass class="h-4 w-4" /></span>
            <span class="flex-1 text-[14px]">{{ t('tasks.estimate') }}</span>
            <button ref="estBtn" type="button" class="pick" :class="{ empty: !task.estimate }" :disabled="!editable" :aria-label="t('tasks.estimate')" aria-haspopup="dialog" data-testid="task-estimate" @click="estPop ? (estPop = false) : openEstimate()">
              {{ task.estimate ? estimateText(task.estimate) : t('tasks.noEstimate') }}
              <ChevronDown v-if="editable" class="h-3.5 w-3.5 text-text-3" aria-hidden="true" />
            </button>
          </div>
          <div v-if="schoolWs && !task.parent_id" class="group-row">
            <span class="tile h-7 w-7" style="--tile: #ff3b30"><GraduationCap class="h-4 w-4" /></span>
            <span class="min-w-0 flex-1">
              <span class="block text-[14px]">{{ t('school.exam') }}</span>
              <span class="block text-[12px] text-text-3">{{ t('school.examHint') }}</span>
            </span>
            <Toggle :model-value="task.kind === 'exam'" :disabled="!editable" :aria-label="t('school.exam')" @update:model-value="(v) => save({ kind: v ? 'exam' : 'task' })" />
          </div>
          <div class="group-row">
            <span class="tile h-7 w-7" style="--tile: #5856d6"><List class="h-4 w-4" /></span>
            <span class="flex-1 text-[14px]">{{ t('tasks.list') }}</span>
            <label class="plain-select">
              <select :value="task.list_id" :disabled="!editable || !!task.parent_id" :aria-label="t('tasks.list')" @change="save({ list_id: ($event.target as HTMLSelectElement).value })">
                <option v-for="l in lists" :key="l.id" :value="l.id">{{ l.name }}</option>
              </select>
              <ChevronRight class="h-3.5 w-3.5" aria-hidden="true" />
            </label>
          </div>
          <div v-if="goalsWs && !task.parent_id && (goalList.length || task.goal_id)" class="group-row" data-testid="task-goal">
            <span class="tile h-7 w-7" style="--tile: #ff2d55"><Target class="h-4 w-4" /></span>
            <span class="flex-1 text-[14px]">{{ t('goals.field') }}</span>
            <label class="plain-select">
              <select :value="task.goal_id || ''" :disabled="!editable" :aria-label="t('goals.field')" @change="save({ goal_id: ($event.target as HTMLSelectElement).value })">
                <option value="">{{ t('goals.noGoal') }}</option>
                <option v-for="g in goalList" :key="g.id" :value="g.id">{{ g.icon ? `${g.icon} ` : '' }}{{ g.title }}</option>
              </select>
              <ChevronRight class="h-3.5 w-3.5" aria-hidden="true" />
            </label>
          </div>
          <div v-if="!task.parent_id && (waits.length || (editable && waitChoices.length))" class="group-row !items-start" data-testid="task-waits">
            <span class="tile mt-0.5 h-7 w-7" style="--tile: #8e8e93"><Link2 class="h-4 w-4" /></span>
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span class="flex-1 text-[14px]">{{ t('tasks.waitsOn') }}</span>
                <label v-if="editable && waitChoices.length" class="plain-select">
                  <select value="" :aria-label="t('tasks.waitsAdd')" data-testid="waits-add" @change="addWait(($event.target as HTMLSelectElement).value)">
                    <option value="" disabled>{{ t('tasks.waitsAddShort') }}</option>
                    <option v-for="x in waitChoices" :key="x.id" :value="x.id">{{ x.done ? '✓ ' : '' }}{{ x.title }}</option>
                  </select>
                  <ChevronRight class="h-3.5 w-3.5" aria-hidden="true" />
                </label>
              </div>
              <div v-if="waits.length" class="mt-1.5 flex flex-wrap gap-1.5">
                <span v-for="w in waits" :key="w.id" class="pill" :class="{ 'line-through opacity-60': w.done }">
                  {{ w.title }}
                  <button v-if="editable" class="-mr-1 opacity-60 hover:opacity-100" :aria-label="t('tasks.waitsRemove', { title: w.title })" @click="removeWait(w.id)"><X class="h-3 w-3" /></button>
                </span>
              </div>
              <p v-if="waits.length" class="mt-1 text-[12px] text-text-3">{{ t('tasks.waitsHint') }}</p>
            </div>
          </div>
        </div>

        <!-- People & tags -->
        <h3 class="sec">{{ t('tasks.peopleTags') }}</h3>
        <div class="group">
          <div class="group-row" data-testid="assignees">
            <span class="tile h-7 w-7" style="--tile: #34c759"><UserRound class="h-4 w-4" /></span>
            <span class="flex-1 text-[14px]">{{ t('tasks.assignee') }}</span>
            <div class="flex flex-wrap items-center justify-end gap-1.5">
              <span v-for="m in assigned" :key="m.user_id" class="person">
                <Avatar :name="m.display_name || m.username" :src="m.avatar_url" :size="20" />{{ (m.display_name || m.username).split(' ')[0] }}
                <button v-if="editable" type="button" class="person-x" :aria-label="t('tasks.removeAssignee', { name: m.display_name || m.username })" @click="setAssignees(task.assignee_ids.filter((x) => x !== m.user_id))"><X class="h-3 w-3" /></button>
              </span>
              <span v-if="!assigned.length && !editable" class="text-[13px] text-text-3">{{ t('tasks.nobody') }}</span>
              <button v-if="editable && unassigned.length" ref="assignBtn" type="button" class="add-person" :aria-label="t('tasks.addAssignee')" :title="t('tasks.addAssignee')" data-testid="assign-add" @click="assignPop = !assignPop"><Plus class="h-3.5 w-3.5" /></button>
              <!-- Alone in the workspace: others are invited from its Members. -->
              <button v-if="members.length <= 1 && workspace?.role === 'owner'" type="button" class="text-[13px] text-accent-text hover:underline" @click="invitePeople">{{ t('tasks.invitePeople') }}</button>
            </div>
          </div>
          <div class="group-row">
            <span class="tile h-7 w-7" style="--tile: #8e8e93"><Tag class="h-4 w-4" /></span>
            <span class="text-[14px]">{{ t('tasks.tags') }}</span>
            <div class="flex min-w-0 flex-1 flex-wrap items-center justify-end gap-1.5">
              <span v-for="tag in task.tags" :key="tag" class="pill !bg-accent-bg !text-accent-text">
                #{{ tag }}
                <button v-if="editable" class="-mr-1 opacity-60 hover:opacity-100" :aria-label="t('tasks.removeTag', { tag })" @click="removeTag(tag)"><X class="h-3 w-3" /></button>
              </span>
              <input v-if="editable && tagging" ref="tagEl" v-model="tagDraft" class="w-[110px] bg-transparent py-0.5 text-right text-[14px] outline-none" :placeholder="t('tasks.addTag')" :aria-label="t('tasks.addTag')" @keydown.enter="addTag" @keydown.esc.stop="tagging = false" @blur="tagBlur" />
              <button v-else-if="editable" type="button" class="text-[14px] text-text-3 hover:text-text-2" data-testid="tag-add" @click="openTag">{{ t('tasks.addTagShort') }}</button>
            </div>
          </div>
        </div>

        <!-- Its issue -->
        <template v-if="gitOn && !task.parent_id && (task.git_number || list?.git || editable)">
          <h3 class="sec flex items-center gap-1.5"><GitBranch class="h-3.5 w-3.5" />{{ t('git.task.title') }}</h3>
          <div class="group">
            <div v-if="task.git_number" class="group-row">
              <div class="min-w-0 flex-1 text-[13.5px]">
                <span class="text-text-2">{{ task.git_repo }}</span>
                <GitBadge :task="task" class="ml-1.5" />
                <span class="ml-1.5 text-[12.5px] text-text-3">{{ t(`git.task.state.${task.git_state || 'open'}`) }}</span>
              </div>
              <button v-if="editable" class="btn-plain shrink-0 !text-danger" @click="unlinkGit">{{ t('git.task.unlink') }}</button>
            </div>
            <label v-if="task.git_number && list?.git" class="group-row">
              <Milestone class="h-4 w-4 shrink-0 text-text-3" />
              <span class="flex-1 text-[14px]">{{ t('git.task.milestone') }}</span>
              <select
                :value="task.git_milestone_id || 0"
                class="input !w-auto max-w-[60%] !py-1 text-[13.5px]"
                :disabled="!editable"
                data-testid="milestone"
                @change="setMilestone(Number(($event.target as HTMLSelectElement).value))"
              >
                <option :value="0">{{ t('git.task.noMilestone') }}</option>
                <option v-if="task.git_milestone_id && !milestones.some((m) => m.ref === task!.git_milestone_id)" :value="task.git_milestone_id">{{ task.git_milestone }}</option>
                <option v-for="m in milestones" :key="m.ref" :value="m.ref">{{ m.title }}</option>
              </select>
            </label>
            <form v-else-if="editable && !list?.git" class="group-row" @submit.prevent="linkGit">
              <input v-model="gitDraft" type="url" class="min-w-0 flex-1 bg-transparent text-[14px] outline-none" :placeholder="t('git.task.placeholder')" :aria-label="t('git.task.placeholder')" />
              <button type="submit" class="btn-plain shrink-0" :disabled="!gitDraft.trim() || linking">{{ t('git.task.link') }}</button>
            </form>
            <div v-if="task.git_dirty" class="group-row !min-h-0 !py-2 text-[12.5px] text-warn-text"><TriangleAlert class="h-3.5 w-3.5 shrink-0" />{{ t('git.task.pending') }}</div>
          </div>
          <p class="footnote mt-1 px-1 leading-snug">
            {{ list?.git ? t('git.task.inList', { repo: `${list.git.owner}/${list.git.repo}` }) : t('git.task.hint') }}
          </p>
        </template>

        <!-- Sub-tasks -->
        <template v-if="!task.parent_id">
          <h3 class="sec flex items-center gap-1.5"><ListTree class="h-3.5 w-3.5" />{{ t('tasks.subtasks') }}</h3>
          <div class="group">
            <div v-for="s in subtasks" :key="s.id" class="group-row !min-h-[40px]">
              <button class="check !h-[18px] !w-[18px]" :class="{ on: s.done }" :style="{ '--check': list?.color || 'var(--ws)' }" :disabled="!editable" @click="toggleSub(s.id, !s.done)">
                <Check class="h-2.5 w-2.5" :stroke-width="3.4" />
              </button>
              <span class="flex-1 text-[14px]" :class="{ 'text-text-3 line-through': s.done }">{{ s.title }}</span>
              <button v-if="editable" class="icon-btn h-6 w-6" @click="removeSub(s.id)"><X class="h-3.5 w-3.5" /></button>
            </div>
            <div v-if="editable" class="group-row !min-h-[40px]">
              <Plus class="h-4 w-4 text-text-3" />
              <input v-model="subDraft" class="flex-1 bg-transparent text-[14px] outline-none" :placeholder="t('tasks.addSubtask')" @keydown.enter="addSub" />
            </div>
            <p v-if="!subtasks.length && !editable" class="group-row text-[13px] text-text-3">–</p>
          </div>
        </template>
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
        <button v-if="editable" type="button" class="btn-plain !text-danger" @click="remove">{{ t('common.delete') }}</button>
        <span v-if="focusWs && task.focus_minutes" class="ml-auto flex items-center gap-1.5 text-[12.5px] text-text-3" data-testid="task-focus"><Timer class="h-3.5 w-3.5" />{{ t('focus.spent', { t: durationOf(task.focus_minutes) }) }}</span>
        <button v-if="canFocus" type="button" class="btn-primary" :class="{ 'ml-auto': !task.focus_minutes }" data-testid="task-start-focus" @click="startFocus"><Timer class="h-4 w-4" />{{ t('focus.startFocus') }}</button>
        <button v-else type="button" class="btn-primary ml-auto" @click="emit('close')">{{ t('common.done') }}</button>
      </footer>

      <Popover v-if="datePop" :anchor="datePop === 'start' ? startPill : duePill" placement="bottom-end" :width="240" @close="datePop = ''">
        <div class="space-y-2 p-3" data-testid="task-date-pop">
          <p class="text-[12px] font-medium text-text-3">{{ datePop === 'start' ? t('tasks.start') : t('tasks.date') }}</p>
          <input
            type="date"
            class="input w-full"
            :value="(datePop === 'start' ? task.start : task.due) ?? ''"
            :max="datePop === 'start' ? (task.due ?? undefined) : undefined"
            :aria-label="datePop === 'start' ? t('tasks.start') : t('tasks.date')"
            @change="setDay(datePop as 'start' | 'due', ($event.target as HTMLInputElement).value)"
          />
          <p v-if="datePop === 'start'" class="text-[12px] leading-snug text-text-3">{{ t('tasks.startHint') }}</p>
          <button v-if="datePop === 'start' ? task.start : task.due" type="button" class="btn-plain w-full justify-center !text-danger" @click="setDay(datePop as 'start' | 'due', '')">{{ t('tasks.removeDate') }}</button>
        </div>
      </Popover>
      <Popover v-if="statusPop && list" :anchor="statusBtn" placement="bottom-end" :width="220" @close="statusPop = false">
        <div class="p-1" role="menu" data-testid="task-status-menu">
          <button
            v-for="(c, i) in list.columns"
            :key="c.id"
            type="button"
            role="menuitemradio"
            :aria-checked="task.column_id === c.id"
            class="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-left text-[13.5px] hover:bg-fill-2"
            @click="setStatus(c.id)"
          >
            <span class="dot" :style="{ background: colDot(c, i) }" />
            <span class="min-w-0 flex-1 truncate">{{ c.name }}</span>
            <Check v-if="task.column_id === c.id" class="h-4 w-4 text-accent-text" />
          </button>
        </div>
      </Popover>
      <Popover v-if="estPop" :anchor="estBtn" placement="bottom-end" :width="250" @close="estPop = false">
        <form class="space-y-2.5 p-3" data-testid="task-estimate-pop" @submit.prevent="setEstimate()">
          <p class="text-[12px] font-medium text-text-3">{{ t('tasks.estimateHint') }}</p>
          <div class="flex items-center gap-2">
            <input ref="estHEl" v-model.number="estH" type="number" min="0" max="168" step="1" inputmode="numeric" class="input !w-16 !py-1.5 text-center" :aria-label="t('tasks.estHours')" placeholder="0" />
            <span class="text-[13px] text-text-2">h</span>
            <input v-model.number="estM" type="number" min="0" max="59" step="5" inputmode="numeric" class="input !w-16 !py-1.5 text-center" :aria-label="t('tasks.estMinutes')" placeholder="0" />
            <span class="text-[13px] text-text-2">min</span>
          </div>
          <div class="flex gap-2">
            <button v-if="task.estimate" type="button" class="btn-plain flex-1 justify-center !text-danger" @click="setEstimate(true)">{{ t('tasks.estClear') }}</button>
            <button type="submit" class="btn-primary btn-sm flex-1 justify-center">{{ t('tasks.estSet') }}</button>
          </div>
        </form>
      </Popover>
      <Popover v-if="assignPop" :anchor="assignBtn" placement="bottom-end" :width="240" @close="assignPop = false">
        <div class="max-h-[280px] overflow-y-auto p-1" role="menu">
          <button
            v-for="m in unassigned"
            :key="m.user_id"
            type="button"
            role="menuitem"
            class="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-left text-[13.5px] hover:bg-fill-2"
            @click="setAssignees([...task.assignee_ids, m.user_id]); assignPop = unassigned.length > 1"
          >
            <Avatar :name="m.display_name || m.username" :src="m.avatar_url" :size="22" />
            <span class="min-w-0 flex-1 truncate">{{ m.display_name || m.username }}</span>
          </button>
        </div>
      </Popover>
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
}
.t-head {
  padding: 18px 22px 14px;
}
.t-body {
  padding: 2px 18px 22px;
  background: var(--grouped);
}
.t-foot {
  position: sticky;
  bottom: 0;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 18px;
  border-top: 0.5px solid var(--border);
  background: var(--surface);
}
:global(html.dark) .t-foot {
  background: #2c2c2e;
}
.t-side {
  border-top: 0.5px solid var(--border);
}
@media (min-width: 900px) {
  .tsheet.split {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(340px, 46%);
    grid-template-rows: auto minmax(0, 1fr) auto;
    grid-template-areas: 'head side' 'body side' 'foot side';
    height: min(86vh, 860px);
    overflow: hidden;
  }
  .split .t-head {
    grid-area: head;
  }
  .split .t-body {
    grid-area: body;
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
    border-left: 0.5px solid var(--border);
  }
  .split .head-x {
    display: none;
  }
}
.crumb {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  font-size: 12.5px;
  color: var(--text-2);
}
.crumb-dot {
  width: 8px;
  height: 8px;
  border-radius: 2px;
}
.sec {
  margin: 18px 4px 6px;
  font-size: 13px;
  color: var(--text-2);
}
.title-area {
  field-sizing: content;
}
.notes-area {
  field-sizing: content;
  min-height: 1.6em;
  max-height: 40vh;
  margin-top: 2px;
  padding: 2px 0;
  border-radius: 8px;
  font-size: 14px;
  line-height: 1.55;
  color: var(--text-2);
}
.notes-area:focus-within {
  padding: 6px 10px;
  background: var(--fill-2);
  font-family: var(--font-mono);
  font-size: 13px;
  color: var(--text);
}
.notes-box {
  max-height: 40vh;
  overflow-y: auto;
  margin-top: 2px;
}
.notes-box.editable {
  cursor: text;
}
.notes-box .notes-md {
  font-size: 14px;
  line-height: 1.55;
  padding-bottom: 0;
  color: var(--text-2);
}
/* How its date stands. */
.due-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-top: 14px;
  padding: 8px 8px 8px 12px;
  border-radius: 10px;
  font-size: 13.5px;
  font-weight: 600;
  background: var(--fill-2);
  color: var(--text-2);
}
.due-bar.late {
  background: var(--crit-bg);
  color: var(--crit-text);
}
.due-bar.today {
  background: var(--warn-bg);
  color: var(--warn-text);
}
/* The words first; on a narrow sheet the buttons go under them. */
.due-text {
  flex: 1 0 auto;
}
.due-chip {
  flex: none;
  padding: 4px 10px;
  border-radius: 7px;
  background: var(--surface);
  color: var(--text);
  font-size: 12.5px;
  font-weight: 500;
  box-shadow: 0 0 0 0.5px var(--border);
}
.due-chip:hover {
  background: var(--surface-2);
}
.due-chip.on {
  background: var(--accent);
  color: var(--accent-fg);
}
.day-pill {
  padding: 3px 9px;
  border-radius: 7px;
  background: var(--fill);
  color: var(--text);
  font-size: 13px;
  font-weight: 500;
}
.day-pill:hover:not(:disabled) {
  background: var(--fill-strong);
}
.day-pill.ghost {
  color: var(--text-3);
}
.day-pill.late {
  background: var(--crit-bg);
  color: var(--crit-text);
  font-weight: 600;
}
.plain-select {
  position: relative;
  display: inline-flex;
  max-width: 60%;
  align-items: center;
  gap: 4px;
  color: var(--text-3);
}
.plain-select select {
  min-width: 0;
  appearance: none;
  -webkit-appearance: none;
  padding: 2px 0;
  background: transparent;
  color: var(--text-3);
  font-size: 14px;
  text-align: right;
  text-overflow: ellipsis;
  cursor: pointer;
  outline: none;
}
.plain-select select:focus-visible {
  border-radius: 6px;
  box-shadow: 0 0 0 3px var(--ring);
}
.pick {
  display: inline-flex;
  max-width: 60%;
  align-items: center;
  gap: 6px;
  padding: 3px 8px 3px 10px;
  border-radius: 999px;
  font-size: 13px;
  font-weight: 500;
  color: var(--text);
  box-shadow: 0 0 0 0.5px var(--border-2);
}
.pick:hover:not(:disabled) {
  background: var(--fill-2);
}
.pick:disabled {
  padding-right: 10px;
}
.pick.empty {
  color: var(--text-3);
}
.dot {
  flex: none;
  width: 7px;
  height: 7px;
  border-radius: 50%;
}
.person {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 9px 2px 2px;
  border-radius: 999px;
  background: var(--fill);
  font-size: 13px;
  font-weight: 500;
}
.person-x {
  margin-right: -3px;
  opacity: 0.5;
}
.person-x:hover {
  opacity: 1;
}
.add-person {
  display: inline-flex;
  width: 26px;
  height: 26px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  border: 1px dashed var(--border-strong);
  color: var(--text-3);
}
.add-person:hover {
  color: var(--text);
  border-color: var(--text-3);
}
</style>
