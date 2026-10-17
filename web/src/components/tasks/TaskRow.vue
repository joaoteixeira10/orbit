<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, Repeat, Flag, ListTree, MessageSquare } from 'lucide-vue-next'
import AvatarStack from '@/components/ui/AvatarStack.vue'
import GitBadge from './GitBadge.vue'
import type { Task, Member } from '@/api/planner'
import { dueLabel, isOverdue, repeatLabel, relDay } from '@/lib/dates'
import { firstLine } from '@/lib/markdown'

// One task, Reminders-style: a round check in its list's colour, the title
// and a line of details — when it's due (red when late), repeat, priority,
// tags, sub-task progress, who it's for (one or several). A click anywhere but the check
// opens the details (as in Plane); the title is renamed there.
const props = withDefaults(
  defineProps<{
    task: Task
    color?: string
    listName?: string
    workspaceName?: string
    subtasks?: { done: number; total: number }
    assignees?: Member[]
    editable?: boolean
    compact?: boolean
  }>(),
  { color: 'var(--ws)', listName: '', workspaceName: '', editable: true, compact: false },
)
const emit = defineEmits<{ toggle: []; open: [] }>()
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
</script>

<template>
  <div class="task-row group/task" :class="{ done: task.done, compact }" @click="emit('open')">
    <button
      class="check mt-[1px]"
      :class="{ on: task.done }"
      :style="{ '--check': color }"
      role="checkbox"
      :aria-checked="task.done"
      :aria-label="task.title"
      :disabled="!editable"
      @click.stop="emit('toggle')"
    >
      <Check class="h-3 w-3" :stroke-width="3.4" />
    </button>
    <button type="button" class="main min-w-0 flex-1 text-left" :title="t('tasks.details')" @click.stop="emit('open')">
      <div class="title">
        <span v-if="prio" class="mr-1 font-semibold" :style="{ color }">{{ prio }}</span>{{ task.title }}
      </div>
      <div v-if="!compact || due || task.git_number" class="meta">
        <GitBadge :task="task" />
        <span v-if="due" :class="{ 'text-crit-text': overdue }">{{ due }}</span>
        <span v-if="task.repeat" class="inline-flex items-center gap-0.5" :title="repeatLabel(task.repeat)"><Repeat class="h-3 w-3" />{{ repeatLabel(task.repeat) }}</span>
        <span v-if="listName">{{ listName }}</span>
        <span v-if="workspaceName">{{ workspaceName }}</span>
        <span v-if="subtasks && subtasks.total" class="inline-flex items-center gap-0.5"><ListTree class="h-3 w-3" />{{ subtasks.done }}/{{ subtasks.total }}</span>
        <span v-if="task.comment_count" class="inline-flex items-center gap-0.5" :title="t('comments.count', { n: task.comment_count }, task.comment_count)"><MessageSquare class="h-3 w-3" />{{ task.comment_count }}</span>
        <span v-for="tag in task.tags" :key="tag" class="text-accent-text">#{{ tag }}</span>
        <span v-if="task.notes && !compact" class="truncate">{{ firstLine(task.notes) }}</span>
      </div>
    </button>
    <Flag v-if="task.priority === 3 && !task.done" class="mt-0.5 h-3.5 w-3.5 shrink-0 text-warn" fill="currentColor" />
    <AvatarStack v-if="assignees?.length" :people="assignees" :size="20" class="mt-0.5" />
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
.task-row {
  cursor: pointer;
  border-radius: 10px;
  transition: background var(--speed);
}
.task-row:hover {
  background: var(--fill-2);
}
.main:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--ring);
  border-radius: 6px;
}
</style>
