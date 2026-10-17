<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { Calendar, CalendarArrowUp, Clock, Repeat, Flag, Tag, ListTree, Trash2, Plus, Check, X, UserRound, Columns3, List, GitBranch, TriangleAlert, Paperclip } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import Toggle from '@/components/ui/Toggle.vue'
import { useTasksStore } from '@/stores/tasks'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useToastStore } from '@/stores/toast'
import { gitApi, type TaskInput } from '@/api/planner'
import { useAuthStore } from '@/stores/auth'
import GitBadge from './GitBadge.vue'
import Attachments from '@/components/ui/Attachments.vue'
import Avatar from '@/components/ui/Avatar.vue'
import MarkdownView from '@/components/editor/MarkdownView.vue'
import { toggleTask } from '@/lib/markdown'
import { todayStr, dateStr, addDays, REPEAT_RULES, repeatPreset, type RepeatPreset } from '@/lib/dates'

// Everything about one task, Reminders' "Details": notes, start, date and
// time, repeat, priority, tags, list and column, who it's for, sub-tasks.
// Each field saves when it changes.
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
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at)
const members = computed(() => wss.members[props.ws] ?? [])
const auth = useAuthStore()
const assigned = computed(() => (task.value?.assignee_ids ?? []).map((id) => members.value.find((m) => m.user_id === id)).filter((m) => !!m))
const unassigned = computed(() => members.value.filter((m) => !task.value?.assignee_ids.includes(m.user_id)))
function setAssignees(ids: string[]) {
  void save({ assignee_ids: ids.filter(Boolean) })
}

const title = ref('')
const notes = ref('')
// Notes are Markdown (as a linked git issue's body is): read rendered, with
// boxes that tick; a click (not on a link or box) or the pencil edits them.
const editingNotes = ref(false)
const notesEl = ref<HTMLTextAreaElement | null>(null)
const tagDraft = ref('')
const subDraft = ref('')
watch(
  () => [props.open, props.taskId] as const,
  async ([o]) => {
    if (!o) return
    if (!tasks.tasks[props.ws]) await tasks.load(props.ws).catch(() => {})
    if ((workspace.value?.member_count ?? 1) > 1 && !wss.members[props.ws]) void wss.loadMembers(props.ws).catch(() => {})
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
  } catch (e: any) {
    toast.error(e.message)
  }
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
  notesEl.value?.focus()
}
function tickInNotes(i: number) {
  notes.value = toggleTask(notes.value, i)
  void save({ notes: notes.value })
}
function notesClick(e: MouseEvent) {
  if (!(e.target as HTMLElement).closest('a, input, pre, button, .md-remote-img, img')) void editNotes()
}

// Date and time.
const hasDate = computed(() => !!task.value?.due)
const hasTime = computed(() => !!task.value?.due_time)
const quick = computed(() => [
  { label: t('dates.today'), v: todayStr() },
  { label: t('dates.tomorrow'), v: dateStr(addDays(new Date(), 1)) },
  { label: t('tasks.nextWeek'), v: dateStr(addDays(new Date(), 7)) },
])
function setDate(on: boolean) {
  void save(on ? { due: task.value?.due || todayStr() } : { due: '', repeat: '' })
}
// The start (the timeline bar's left end) defaults to the due date, else today.
const hasStart = computed(() => !!task.value?.start)
function setStart(on: boolean) {
  const due = task.value?.due
  void save({ start: on ? (due && due < todayStr() ? due : todayStr()) : '' })
}
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
</script>

<template>
  <Overlay :open="open && !!task" size="md" @close="emit('close')">
    <template v-if="task">
      <div class="flex items-start gap-3 pt-2">
        <button
          class="check mt-1.5 !h-6 !w-6"
          :class="{ on: task.done }"
          :style="{ '--check': list?.color || 'var(--ws)' }"
          :disabled="!editable"
          @click="save({ completed: !task.done })"
        >
          <Check class="h-3.5 w-3.5" :stroke-width="3.4" />
        </button>
        <textarea
          id="task-title"
          v-model="title"
          rows="1"
          class="title-area min-w-0 flex-1 resize-none bg-transparent text-[20px] font-semibold leading-snug outline-none"
          :class="{ 'text-text-3 line-through': task.done }"
          :readonly="!editable"
          maxlength="500"
          @blur="saveTitle"
          @keydown.enter.prevent="($event.target as HTMLTextAreaElement).blur()"
        />
      </div>
      <div v-if="notes.trim() && !editingNotes" class="notes-box mt-2" :class="{ editable }" data-testid="task-notes-view" @click="notesClick">
        <MarkdownView class="notes-md" :content="notes" :editable="editable" @toggle="tickInNotes" @edit="editNotes" />
      </div>
      <textarea
        v-else-if="editable || notes.trim()"
        id="task-notes"
        ref="notesEl"
        v-model="notes"
        rows="3"
        class="notes-area mt-2 w-full resize-none rounded-xl bg-fill-2 px-3 py-2.5 text-[14px] outline-none placeholder:text-text-faint focus:shadow-[0_0_0_3px_var(--ring)]"
        :placeholder="t('tasks.notesMd')"
        :readonly="!editable"
        @blur="saveNotes"
        @keydown.esc.stop="($event.target as HTMLTextAreaElement).blur()"
      />

      <!-- When -->
      <div class="group mt-4">
        <div class="group-row">
          <span class="tile h-7 w-7" style="--tile: #ff9500"><CalendarArrowUp class="h-4 w-4" /></span>
          <span class="min-w-0 flex-1">
            <span class="block text-[14px]">{{ t('tasks.start') }}</span>
            <input v-if="hasStart" type="date" class="mt-0.5 bg-transparent text-[13px] text-accent-text outline-none" :value="task.start ?? ''" :max="task.due ?? undefined" :disabled="!editable" @change="save({ start: ($event.target as HTMLInputElement).value })" />
            <span v-else class="block text-[12px] text-text-3">{{ t('tasks.startHint') }}</span>
          </span>
          <Toggle :model-value="hasStart" :disabled="!editable" @update:model-value="setStart" />
        </div>
        <div class="group-row">
          <span class="tile h-7 w-7" style="--tile: #ff3b30"><Calendar class="h-4 w-4" /></span>
          <span class="min-w-0 flex-1">
            <span class="block text-[14px]">{{ t('tasks.date') }}</span>
            <input v-if="hasDate" type="date" class="mt-0.5 bg-transparent text-[13px] text-accent-text outline-none" :value="task.due ?? ''" :disabled="!editable" @change="save({ due: ($event.target as HTMLInputElement).value })" />
          </span>
          <Toggle :model-value="hasDate" :disabled="!editable" @update:model-value="setDate" />
        </div>
        <div v-if="hasDate && editable" class="flex gap-1.5 px-3.5 py-2">
          <button v-for="q in quick" :key="q.v" class="pill hover:bg-fill-strong" :class="{ '!bg-accent !text-accent-fg': task.due === q.v }" @click="save({ due: q.v })">{{ q.label }}</button>
        </div>
        <div v-if="hasDate" class="group-row">
          <span class="tile h-7 w-7" style="--tile: #007aff"><Clock class="h-4 w-4" /></span>
          <span class="min-w-0 flex-1">
            <span class="block text-[14px]">{{ t('tasks.time') }}</span>
            <input v-if="hasTime" type="time" class="mt-0.5 bg-transparent text-[13px] text-accent-text outline-none" :value="task.due_time ?? ''" :disabled="!editable" @change="save({ due_time: ($event.target as HTMLInputElement).value })" />
          </span>
          <Toggle :model-value="hasTime" :disabled="!editable" @update:model-value="setTime" />
        </div>
        <div class="group-row">
          <span class="tile h-7 w-7" style="--tile: #8e8e93"><Repeat class="h-4 w-4" /></span>
          <span class="flex-1 text-[14px]">{{ t('tasks.repeat') }}</span>
          <select class="input !w-auto !py-1 text-[13px]" :value="repeat" :disabled="!editable" @change="setRepeat(($event.target as HTMLSelectElement).value)">
            <option value="">{{ t('repeat.never') }}</option>
            <option value="daily">{{ t('repeat.daily') }}</option>
            <option value="weekdays">{{ t('repeat.weekdays') }}</option>
            <option value="weekly">{{ t('repeat.weekly') }}</option>
            <option value="biweekly">{{ t('repeat.biweekly') }}</option>
            <option value="monthly">{{ t('repeat.monthly') }}</option>
            <option value="yearly">{{ t('repeat.yearly') }}</option>
            <option v-if="repeat === 'custom'" value="custom">{{ t('repeat.custom') }}</option>
          </select>
        </div>
      </div>

      <!-- What / where -->
      <div class="group mt-4">
        <div class="group-row">
          <span class="tile h-7 w-7" style="--tile: #ff9500"><Flag class="h-4 w-4" /></span>
          <span class="flex-1 text-[14px]">{{ t('tasks.priority') }}</span>
          <div class="seg">
            <button v-for="p in [0, 1, 2, 3]" :key="p" :class="{ on: task.priority === p }" :disabled="!editable" @click="save({ priority: p })">
              {{ p === 0 ? t('tasks.prioNone') : '!'.repeat(p) }}
            </button>
          </div>
        </div>
        <div class="group-row">
          <span class="tile h-7 w-7" style="--tile: #5856d6"><List class="h-4 w-4" /></span>
          <span class="flex-1 text-[14px]">{{ t('tasks.list') }}</span>
          <select class="input !w-auto !py-1 text-[13px]" :value="task.list_id" :disabled="!editable || !!task.parent_id" @change="save({ list_id: ($event.target as HTMLSelectElement).value })">
            <option v-for="l in lists" :key="l.id" :value="l.id">{{ l.name }}</option>
          </select>
        </div>
        <div v-if="list" class="group-row">
          <span class="tile h-7 w-7" style="--tile: #30b0c7"><Columns3 class="h-4 w-4" /></span>
          <span class="flex-1 text-[14px]">{{ t('tasks.status') }}</span>
          <select class="input !w-auto !py-1 text-[13px]" :value="task.column_id" :disabled="!editable || !!task.parent_id" @change="save({ column_id: ($event.target as HTMLSelectElement).value })">
            <option v-for="c in list.columns" :key="c.id" :value="c.id">{{ c.name }}</option>
          </select>
        </div>
        <div v-if="members.length > 1" class="group-row !items-start" data-testid="assignees">
          <span class="tile mt-0.5 h-7 w-7" style="--tile: #34c759"><UserRound class="h-4 w-4" /></span>
          <div class="min-w-0 flex-1">
            <span class="block text-[14px]">{{ t('tasks.assignee') }}</span>
            <div class="mt-1 flex flex-wrap items-center gap-1.5">
              <span v-for="m in assigned" :key="m.user_id" class="pill inline-flex items-center gap-1.5 !pl-0.5">
                <Avatar :name="m.display_name || m.username" :src="m.avatar_url" :size="18" />{{ m.display_name || m.username }}
                <button v-if="editable" type="button" class="-mr-0.5 text-text-3 hover:text-text" :aria-label="t('tasks.removeAssignee', { name: m.display_name || m.username })" @click="setAssignees(task.assignee_ids.filter((x) => x !== m.user_id))"><X class="h-3 w-3" /></button>
              </span>
              <span v-if="!assigned.length && !editable" class="text-[13px] text-text-3">{{ t('tasks.nobody') }}</span>
              <select
                v-if="editable && unassigned.length"
                class="input !w-auto !py-0.5 text-[13px]"
                :aria-label="t('tasks.addAssignee')"
                value=""
                @change="setAssignees([...task.assignee_ids, ($event.target as HTMLSelectElement).value]); ($event.target as HTMLSelectElement).value = ''"
              >
                <option value="" disabled>{{ assigned.length ? '+' : t('tasks.addAssignee') }}</option>
                <option v-for="m in unassigned" :key="m.user_id" :value="m.user_id">{{ m.display_name || m.username }}</option>
              </select>
            </div>
          </div>
        </div>
        <div class="group-row !items-start">
          <span class="tile mt-0.5 h-7 w-7" style="--tile: #8e8e93"><Tag class="h-4 w-4" /></span>
          <div class="flex min-w-0 flex-1 flex-wrap items-center gap-1.5 py-0.5">
            <span v-for="tag in task.tags" :key="tag" class="pill !bg-accent-bg !text-accent-text">
              #{{ tag }}
              <button v-if="editable" class="-mr-1 opacity-60 hover:opacity-100" @click="removeTag(tag)"><X class="h-3 w-3" /></button>
            </span>
            <input v-if="editable" v-model="tagDraft" class="min-w-[90px] flex-1 bg-transparent py-1 text-[14px] outline-none" :placeholder="t('tasks.addTag')" @keydown.enter="addTag" @blur="addTag" />
          </div>
        </div>
      </div>

      <!-- Its issue -->
      <div v-if="gitOn && !task.parent_id && (task.git_number || list?.git || editable)" class="mt-4">
        <h3 class="cap mb-1.5 flex items-center gap-1.5 px-1"><GitBranch class="h-3.5 w-3.5" />{{ t('git.task.title') }}</h3>
        <div class="group">
          <div v-if="task.git_number" class="group-row">
            <div class="min-w-0 flex-1 text-[13.5px]">
              <span class="text-text-2">{{ task.git_repo }}</span>
              <GitBadge :task="task" class="ml-1.5" />
              <span class="ml-1.5 text-[12.5px] text-text-3">{{ t(`git.task.state.${task.git_state || 'open'}`) }}</span>
            </div>
            <button v-if="editable" class="btn-plain shrink-0 !text-danger" @click="unlinkGit">{{ t('git.task.unlink') }}</button>
          </div>
          <form v-else-if="editable && !list?.git" class="group-row" @submit.prevent="linkGit">
            <input v-model="gitDraft" type="url" class="min-w-0 flex-1 bg-transparent text-[14px] outline-none" :placeholder="t('git.task.placeholder')" :aria-label="t('git.task.placeholder')" />
            <button type="submit" class="btn-plain shrink-0" :disabled="!gitDraft.trim() || linking">{{ t('git.task.link') }}</button>
          </form>
          <div v-if="task.git_dirty" class="group-row !min-h-0 !py-2 text-[12.5px] text-warn-text"><TriangleAlert class="h-3.5 w-3.5 shrink-0" />{{ t('git.task.pending') }}</div>
        </div>
        <p class="footnote mt-1 px-1 leading-snug">
          {{ list?.git ? t('git.task.inList', { repo: `${list.git.owner}/${list.git.repo}` }) : t('git.task.hint') }}
        </p>
      </div>

      <!-- Files -->
      <div v-if="!task.parent_id" class="mt-4">
        <h3 class="cap mb-1.5 flex items-center gap-1.5 px-1"><Paperclip class="h-3.5 w-3.5" />{{ t('files.title') }}</h3>
        <div class="group px-3 py-1.5">
          <Attachments target="task" :target-id="task.id" :editable="editable" />
        </div>
      </div>

      <!-- Sub-tasks -->
      <div v-if="!task.parent_id" class="mt-4">
        <h3 class="cap mb-1.5 flex items-center gap-1.5 px-1"><ListTree class="h-3.5 w-3.5" />{{ t('tasks.subtasks') }}</h3>
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
        </div>
      </div>
    </template>
    <template v-if="task && editable" #footer>
      <button class="btn-ghost mr-auto !text-danger" @click="remove"><Trash2 class="h-4 w-4" />{{ t('common.delete') }}</button>
      <button class="btn-primary" @click="emit('close')">{{ t('common.done') }}</button>
    </template>
  </Overlay>
</template>

<style scoped>
.title-area {
  field-sizing: content;
}
.notes-area {
  field-sizing: content;
  min-height: 4.5em;
  max-height: 50vh;
  font-family: var(--font-mono);
  font-size: 13px;
  line-height: 1.55;
}
.notes-box {
  max-height: 50vh;
  overflow-y: auto;
  padding: 10px 12px;
  border-radius: 12px;
  background: var(--fill-2);
}
.notes-box.editable {
  cursor: text;
}
.notes-box .notes-md {
  font-size: 14px;
  line-height: 1.55;
  padding-bottom: 0;
}
</style>
