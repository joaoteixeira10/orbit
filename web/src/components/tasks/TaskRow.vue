<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, ChevronRight, Hourglass, Repeat, Flag, MessageSquare, Milestone } from 'lucide-vue-next'
import AvatarStack from '@/components/ui/AvatarStack.vue'
import TaskChips from './TaskChips.vue'
import GitBadge from './GitBadge.vue'
import type { Task, Member, TaskInput } from '@/api/planner'
import { countdown, dueLabel, isOverdue, repeatLabel, relDay } from '@/lib/dates'
import { firstLine } from '@/lib/markdown'
import { estimateText } from '@/lib/weekPlan'

// One task, Reminders-style: a round check in its list's colour, the title
// and a line of details — when it's due (red when late), repeat, priority,
// tags, who it's for (one or several). A click anywhere but the check
// opens the details (as in Plane); the title is renamed there. With
// sub-tasks, a bar of how many are done that folds them open and shut.
const props = withDefaults(
  defineProps<{
    task: Task
    color?: string
    listName?: string
    workspaceName?: string
    subtasks?: { done: number; total: number }
    // The sub-tasks are showing (with subtasks: the bar's arrow points down).
    expanded?: boolean
    assignees?: Member[]
    // With the workspace's members: date, priority and assignees as chips
    // at the end, changed in place (wide screens).
    members?: Member[]
    editable?: boolean
    compact?: boolean
  }>(),
  { color: 'var(--ws)', listName: '', workspaceName: '', editable: true, compact: false },
)
const emit = defineEmits<{ toggle: []; open: []; update: [patch: TaskInput]; expand: [] }>()
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
  <div class="task-row group/task" :class="{ done: task.done, compact, 'has-chips': !!members && !compact }" @click="emit('open')">
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
        <span v-if="prio" class="prio-mark mr-1 font-semibold" :style="{ color }">{{ prio }}</span>{{ task.title }}
      </div>
      <div v-if="!compact || due || task.git_number" class="meta">
        <GitBadge :task="task" />
        <span v-if="task.git_milestone" class="milestone" :title="t('git.task.milestone')"><Milestone class="h-3 w-3" />{{ task.git_milestone }}</span>
        <span v-if="task.kind === 'exam'" class="exam">{{ t('school.exam') }}<template v-if="task.due && !task.done"> · {{ countdown(task.due) }}</template></span>
        <span v-if="due" class="due-meta" :class="{ 'text-crit-text': overdue }">{{ due }}</span>
        <span v-if="task.repeat" class="inline-flex items-center gap-0.5" :title="repeatLabel(task.repeat)"><Repeat class="h-3 w-3" />{{ repeatLabel(task.repeat) }}</span>
        <span v-if="task.estimate" class="inline-flex items-center gap-0.5" :title="t('tasks.estimate')"><Hourglass class="h-3 w-3" />{{ estimateText(task.estimate) }}</span>
        <span v-if="listName">{{ listName }}</span>
        <span v-if="workspaceName">{{ workspaceName }}</span>
        <span v-if="task.comment_count" class="inline-flex items-center gap-0.5" :title="t('comments.count', { n: task.comment_count }, task.comment_count)"><MessageSquare class="h-3 w-3" />{{ task.comment_count }}</span>
        <span v-for="tag in task.tags" :key="tag" class="text-accent-text">#{{ tag }}</span>
        <span v-if="task.notes && !compact" class="truncate">{{ firstLine(task.notes) }}</span>
      </div>
    </button>
    <button
      v-if="subtasks && subtasks.total"
      type="button"
      class="subs"
      :class="{ open: expanded, full: subtasks.done === subtasks.total }"
      :aria-expanded="!!expanded"
      :title="t(expanded ? 'tasks.subsHide' : 'tasks.subsShow')"
      data-testid="subtasks-toggle"
      @click.stop="emit('expand')"
    >
      <span class="bar" role="progressbar" :aria-valuenow="subtasks.done" aria-valuemin="0" :aria-valuemax="subtasks.total"><span :style="{ width: `${(100 * subtasks.done) / subtasks.total}%`, background: color }" /></span>
      <span class="tabular-nums">{{ subtasks.done }}/{{ subtasks.total }}</span>
      <ChevronRight class="chev h-3.5 w-3.5" />
    </button>
    <Flag v-if="task.priority === 3 && !task.done" class="row-flag mt-0.5 h-3.5 w-3.5 shrink-0 text-warn" fill="currentColor" />
    <AvatarStack v-if="assignees?.length" :people="assignees" :size="20" class="row-people mt-0.5" />
    <TaskChips v-if="members && !compact" class="row-chips" :task="task" :members="members" :editable="editable" @update="(p) => emit('update', p)" />
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
.row-chips {
  display: none;
}
.task-row:hover .row-chips :deep(.chip.empty) {
  opacity: 1;
}
@media (min-width: 640px) {
  .has-chips .row-chips {
    display: inline-flex;
  }
  .has-chips .due-meta,
  .has-chips .row-people,
  .has-chips .row-flag,
  .has-chips .prio-mark {
    display: none;
  }
}
.milestone {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  max-width: 14em;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-2);
}
.subs {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: 6px;
  margin-top: 1px;
  padding: 2px 4px 2px 8px;
  border-radius: 7px;
  font-size: 12px;
  color: var(--text-3);
  transition: background var(--speed);
}
.subs:hover {
  background: var(--fill);
  color: var(--text-2);
}
.subs:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--ring);
}
.subs .bar {
  position: relative;
  width: 56px;
  height: 5px;
  overflow: hidden;
  border-radius: 3px;
  background: var(--fill);
}
.subs .bar > span {
  position: absolute;
  inset: 0 auto 0 0;
  border-radius: inherit;
  transition: width 0.3s;
}
.subs.full {
  color: var(--text-2);
}
.subs .chev {
  transition: transform var(--speed);
}
.subs.open .chev {
  transform: rotate(90deg);
}
.exam {
  padding: 0 6px;
  border-radius: 6px;
  background: color-mix(in srgb, #ff3b30 14%, transparent);
  color: var(--crit-text);
  font-weight: 600;
}
</style>
