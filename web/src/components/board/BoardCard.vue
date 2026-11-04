<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Calendar, Check, ListTree, MessageSquare, Repeat } from 'lucide-vue-next'
import AvatarStack from '@/components/ui/AvatarStack.vue'
import GitBadge from '@/components/tasks/GitBadge.vue'
import type { Member, Task } from '@/api/planner'
import { dueLabel, isOverdue, repeatLabel } from '@/lib/dates'
import { firstLine } from '@/lib/markdown'

// One card on the board: a raised tile with the title (and its priority),
// the first line of the notes, then a line of details — due (red when
// late), repeat, tags, sub-task progress — and who it's for. Cards in a done
// column read as completed. Dragging is wired by the column (data-id).
const props = defineProps<{
  task: Task
  color: string // the list's colour
  done: boolean // in a done column
  subtasks: { done: number; total: number }
  assignees?: Member[]
}>()
const { t } = useI18n()

const due = computed(() => dueLabel(props.task.due, props.task.due_time))
const overdue = computed(() => isOverdue(props.task.due, props.task.due_time, props.done || props.task.done))
const prio = computed(() => '!'.repeat(props.task.priority))
const note = computed(() => firstLine(props.task.notes))
const who = computed(() => (props.assignees ?? []).map((m) => m.display_name || m.username).join(', '))
const hasMeta = computed(() => !!props.task.git_number || !!due.value || !!props.task.repeat || props.task.tags.length > 0 || props.subtasks.total > 0 || !!who.value || props.task.comment_count > 0)
</script>

<template>
  <div class="board-card card" :class="{ done }" :data-id="task.id" :style="{ '--list': color }" role="button" tabindex="0" :aria-label="task.title">
    <div class="flex items-start gap-2">
      <span v-if="done" class="done-mark" :title="t('board.completed')"><Check class="h-2.5 w-2.5" :stroke-width="3.6" /></span>
      <p class="title">
        <span v-if="prio" class="prio">{{ prio }}</span>{{ task.title }}
      </p>
    </div>
    <p v-if="note" class="note">{{ note }}</p>
    <div v-if="hasMeta" class="meta">
      <span v-if="due" class="chip" :class="{ late: overdue }"><Calendar class="h-3 w-3" />{{ due }}</span>
      <span v-if="task.repeat" class="inline-flex items-center" :title="repeatLabel(task.repeat)" :aria-label="repeatLabel(task.repeat)"><Repeat class="h-3.5 w-3.5" /></span>
      <span
        v-if="subtasks.total"
        class="inline-flex items-center gap-1 tabular-nums"
        :class="{ 'text-ok-text': subtasks.done === subtasks.total }"
        :title="t('board.subtasks', subtasks)"
      >
        <ListTree class="h-3.5 w-3.5" />{{ subtasks.done }}/{{ subtasks.total }}
      </span>
      <span v-if="task.comment_count" class="inline-flex items-center gap-1 tabular-nums" :title="t('comments.count', { n: task.comment_count }, task.comment_count)"><MessageSquare class="h-3.5 w-3.5" />{{ task.comment_count }}</span>
      <GitBadge :task="task" />
      <span v-for="tag in task.tags" :key="tag" class="tag">#{{ tag }}</span>
      <AvatarStack v-if="assignees?.length" :people="assignees" :size="20" class="avatar" :title="t('board.assignedTo', { name: who })" />
    </div>
  </div>
</template>

<style scoped>
.board-card {
  display: block;
  padding: 10px 12px;
  border-radius: 12px;
  cursor: pointer;
  user-select: none;
  -webkit-user-select: none;
  -webkit-touch-callout: none;
  transition: transform 0.22s var(--spring), box-shadow 0.22s var(--ease);
}
@media (hover: hover) {
  .board-card:hover {
    transform: translateY(-1px);
    box-shadow: 0 0 0 0.5px var(--border), 0 4px 14px rgba(0, 0, 0, 0.08), 0 1px 3px rgba(0, 0, 0, 0.06);
  }
  html.dark .board-card:hover {
    box-shadow: 0 0 0 0.5px var(--border-2), 0 6px 18px rgba(0, 0, 0, 0.4);
  }
}
.board-card:active {
  transform: scale(0.985);
}
.title {
  min-width: 0;
  flex: 1;
  font-size: 14px;
  font-weight: 500;
  line-height: 1.35;
  word-break: break-word;
  color: var(--text);
}
.prio {
  margin-right: 4px;
  font-weight: 700;
  color: var(--list);
}
.done .title {
  color: var(--text-3);
}
.done-mark {
  display: grid;
  place-items: center;
  flex: none;
  width: 16px;
  height: 16px;
  margin-top: 1.5px;
  border-radius: 999px;
  background: var(--list);
  color: #fff;
}
.note {
  margin-top: 3px;
  font-size: 12.5px;
  line-height: 1.4;
  color: var(--text-2);
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}
.done .note {
  color: var(--text-3);
}
.meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 9px;
  margin-top: 8px;
  font-size: 12px;
  color: var(--text-3);
}
.chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 1px 7px;
  border-radius: 6px;
  background: var(--fill-2);
  color: var(--text-2);
  font-weight: 500;
}
.chip.late {
  background: var(--crit-bg);
  color: var(--crit-text);
}
.tag {
  color: var(--accent-text);
}
.done .tag {
  opacity: 0.7;
}
.avatar {
  margin-left: auto;
}

/* Drag states (SortableJS). The placeholder left in the list is a dashed
   slot; the card under the finger tilts a little and floats. */
.board-card.sortable-ghost {
  opacity: 1;
  background: var(--fill-2);
  box-shadow: inset 0 0 0 1.5px var(--border-2);
  outline: 1.5px dashed var(--border-2);
  outline-offset: -1.5px;
  transform: none;
}
.board-card.sortable-ghost > * {
  visibility: hidden;
}
.board-card.sortable-chosen:not(.sortable-ghost) {
  box-shadow: 0 0 0 0.5px var(--border), var(--shadow);
}
.board-card.sortable-drag,
.board-card.sortable-fallback {
  opacity: 1 !important;
  /* Sortable moves the floating copy with an inline transform, so tilt it
     with the individual rotate / scale properties, which compose with it. */
  rotate: 2.5deg;
  scale: 1.02;
  box-shadow: 0 0 0 0.5px var(--border), 0 18px 40px rgba(0, 0, 0, 0.18), 0 4px 10px rgba(0, 0, 0, 0.1);
  cursor: grabbing;
  transition: none;
}
</style>
