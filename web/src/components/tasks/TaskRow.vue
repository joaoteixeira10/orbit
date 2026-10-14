<script setup lang="ts">
import { computed, ref, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, Info, Repeat, Flag, ListTree } from 'lucide-vue-next'
import Avatar from '@/components/ui/Avatar.vue'
import type { Task, Member } from '@/api/planner'
import { dueLabel, isOverdue, repeatLabel, relDay } from '@/lib/dates'

// One task, Reminders-style: a round check in its list's colour, the title
// (click to edit in place), and a line of details — when it's due (red when
// late), repeat, priority, tags, sub-task progress, who it's for. The ⓘ
// button opens the details sheet.
const props = withDefaults(
  defineProps<{
    task: Task
    color?: string
    listName?: string
    workspaceName?: string
    subtasks?: { done: number; total: number }
    assignee?: Member
    editable?: boolean
    compact?: boolean
  }>(),
  { color: 'var(--ws)', listName: '', workspaceName: '', editable: true, compact: false },
)
const emit = defineEmits<{ toggle: []; open: []; rename: [title: string] }>()
const { t } = useI18n()

const overdue = computed(() => isOverdue(props.task.due, props.task.due_time, props.task.done))
// With a start before the due date it reads as a span: "Mon – Friday, 9:00".
const due = computed(() => {
  const d = dueLabel(props.task.due, props.task.due_time)
  const s = props.task.start
  if (!s || s === props.task.due) return d
  return d ? `${relDay(s)} – ${d}` : `${t('tasks.startsOn')} ${relDay(s)}`
})
const prio = computed(() => '!'.repeat(props.task.priority))

const editing = ref(false)
const draft = ref('')
const input = ref<HTMLInputElement | null>(null)
async function startEdit() {
  if (!props.editable) return emit('open')
  draft.value = props.task.title
  editing.value = true
  await nextTick()
  input.value?.focus()
}
function commit() {
  editing.value = false
  const v = draft.value.trim()
  if (v && v !== props.task.title) emit('rename', v)
}
</script>

<template>
  <div class="task-row group/task" :class="{ done: task.done, compact }">
    <button
      class="check mt-[1px]"
      :class="{ on: task.done }"
      :style="{ '--check': color }"
      role="checkbox"
      :aria-checked="task.done"
      :aria-label="task.title"
      :disabled="!editable"
      @click="emit('toggle')"
    >
      <Check class="h-3 w-3" :stroke-width="3.4" />
    </button>
    <div class="min-w-0 flex-1">
      <input
        v-if="editing"
        ref="input"
        v-model="draft"
        class="w-full bg-transparent text-[14.5px] outline-none"
        maxlength="500"
        @blur="commit"
        @keydown.enter="commit"
        @keydown.esc="editing = false"
      />
      <div v-else class="title" @click="startEdit">
        <span v-if="prio" class="mr-1 font-semibold" :style="{ color }">{{ prio }}</span>{{ task.title }}
      </div>
      <div v-if="!compact || due" class="meta">
        <span v-if="due" :class="{ 'text-crit-text': overdue }">{{ due }}</span>
        <span v-if="task.repeat" class="inline-flex items-center gap-0.5" :title="repeatLabel(task.repeat)"><Repeat class="h-3 w-3" />{{ repeatLabel(task.repeat) }}</span>
        <span v-if="listName">{{ listName }}</span>
        <span v-if="workspaceName">{{ workspaceName }}</span>
        <span v-if="subtasks && subtasks.total" class="inline-flex items-center gap-0.5"><ListTree class="h-3 w-3" />{{ subtasks.done }}/{{ subtasks.total }}</span>
        <span v-for="tag in task.tags" :key="tag" class="text-accent-text">#{{ tag }}</span>
        <span v-if="task.notes && !compact" class="truncate">{{ task.notes.split('\n')[0] }}</span>
      </div>
    </div>
    <Flag v-if="task.priority === 3 && !task.done" class="mt-0.5 h-3.5 w-3.5 shrink-0 text-warn" fill="currentColor" />
    <Avatar v-if="assignee" :name="assignee.display_name || assignee.username" :src="assignee.avatar_url" :size="20" class="mt-0.5" />
    <button class="info" :title="t('tasks.details')" @click="emit('open')"><Info class="h-[18px] w-[18px]" /></button>
  </div>
</template>

<style scoped>
.task-row {
  display: flex;
  align-items: flex-start;
  gap: 11px;
  padding: 9px 12px 9px 4px;
  min-height: 44px;
}
.task-row.compact {
  padding-top: 6px;
  padding-bottom: 6px;
  min-height: 34px;
}
.title {
  font-size: 14.5px;
  line-height: 1.35;
  word-break: break-word;
  cursor: text;
}
.done .title {
  color: var(--text-3);
}
.meta {
  display: flex;
  flex-wrap: wrap;
  gap: 2px 10px;
  margin-top: 1px;
  font-size: 12.5px;
  color: var(--text-3);
}
.meta:empty {
  display: none;
}
.info {
  opacity: 0;
  color: var(--accent);
  transition: opacity 0.15s;
}
.task-row:hover .info,
.info:focus-visible {
  opacity: 1;
}
@media (hover: none) {
  .info {
    opacity: 0.6;
  }
}
</style>
