<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Calendar, Flag, UserRound, Check } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import AvatarStack from '@/components/ui/AvatarStack.vue'
import Avatar from '@/components/ui/Avatar.vue'
import type { Member, Task, TaskInput } from '@/api/planner'
import { addDays, dateStr, dueLabel, isOverdue, todayStr } from '@/lib/dates'

// A task's date, priority and assignees as chips at the end of its row, each
// changed right there from a small menu (as in Plane). Empty ones show only
// on hover, as a faint icon.
const props = defineProps<{ task: Task; members: Member[]; editable: boolean }>()
const emit = defineEmits<{ update: [patch: TaskInput] }>()
const { t } = useI18n()

const open = ref<'' | 'date' | 'prio' | 'who'>('')
const anchor = ref<HTMLElement | null>(null)
function show(which: 'date' | 'prio' | 'who', e: MouseEvent) {
  if (!props.editable) return
  anchor.value = e.currentTarget as HTMLElement
  open.value = open.value === which ? '' : which
}

const overdue = computed(() => isOverdue(props.task.due, props.task.due_time, props.task.done))
const people = computed(() => props.task.assignee_ids.map((id) => props.members.find((m) => m.user_id === id)).filter((m): m is Member => !!m))
const quick = computed(() => [
  { label: t('dates.today'), v: todayStr() },
  { label: t('dates.tomorrow'), v: dateStr(addDays(new Date(), 1)) },
  { label: t('tasks.nextWeek'), v: dateStr(addDays(new Date(), 7)) },
])
function setDue(v: string) {
  emit('update', { due: v })
  open.value = ''
}
function setPrio(p: number) {
  emit('update', { priority: p })
  open.value = ''
}
function toggleWho(id: string) {
  const ids = props.task.assignee_ids.includes(id) ? props.task.assignee_ids.filter((x) => x !== id) : [...props.task.assignee_ids, id]
  emit('update', { assignee_ids: ids })
}
const PRIO = ['', '!', '!!', '!!!']
</script>

<template>
  <span class="chips" @click.stop>
    <span class="slot w-date"><button type="button" class="chip" :class="{ empty: !task.due, late: overdue }" :disabled="!editable" :title="t('tasks.date')" :aria-label="t('tasks.date')" @click="show('date', $event)">
      <Calendar class="h-3.5 w-3.5 shrink-0" /><span v-if="task.due" class="truncate">{{ dueLabel(task.due, task.due_time) }}</span>
    </button></span>
    <span class="slot w-prio"><button type="button" class="chip" :class="{ empty: !task.priority }" :disabled="!editable" :title="t('tasks.priority')" :aria-label="t('tasks.priority')" @click="show('prio', $event)">
      <Flag class="h-3.5 w-3.5" :class="{ 'text-warn': task.priority === 3 }" /><span v-if="task.priority">{{ PRIO[task.priority] }}</span>
    </button></span>
    <span v-if="members.length" class="slot w-who"><button type="button" class="chip" :class="{ empty: !people.length }" :disabled="!editable" :title="t('tasks.assignee')" :aria-label="t('tasks.assignee')" @click="show('who', $event)">
      <AvatarStack v-if="people.length" :people="people" :size="18" />
      <UserRound v-else class="h-3.5 w-3.5" />
    </button></span>

    <Popover v-if="open === 'date'" :anchor="anchor" placement="bottom-end" :width="220" @close="open = ''">
      <div class="p-1.5">
        <button v-for="q in quick" :key="q.v" class="menu-item" @click="setDue(q.v)">{{ q.label }}</button>
        <label class="menu-item"><input type="date" class="w-full bg-transparent text-[13.5px] outline-none" :value="task.due ?? ''" @change="setDue(($event.target as HTMLInputElement).value)" /></label>
        <button v-if="task.due" class="menu-item !text-danger" @click="setDue('')">{{ t('dates.noDate') }}</button>
      </div>
    </Popover>
    <Popover v-if="open === 'prio'" :anchor="anchor" placement="bottom-end" :width="180" @close="open = ''">
      <div class="p-1.5">
        <button v-for="p in [3, 2, 1, 0]" :key="p" class="menu-item" @click="setPrio(p)">
          <Check class="h-3.5 w-3.5" :class="task.priority === p ? '' : 'invisible'" />{{ t(`tasks.prio.${p}`) }}
        </button>
      </div>
    </Popover>
    <Popover v-if="open === 'who'" :anchor="anchor" placement="bottom-end" :width="230" @close="open = ''">
      <div class="p-1.5">
        <button v-for="m in members" :key="m.user_id" class="menu-item" role="menuitemcheckbox" :aria-checked="task.assignee_ids.includes(m.user_id)" @click="toggleWho(m.user_id)">
          <Check class="h-3.5 w-3.5" :class="task.assignee_ids.includes(m.user_id) ? '' : 'invisible'" />
          <Avatar :name="m.display_name || m.username" :src="m.avatar_url" :size="20" />
          <span class="truncate">{{ m.display_name || m.username }}</span>
        </button>
      </div>
    </Popover>
  </span>
</template>

<style scoped>
.chips {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: 4px;
}
/* Fixed slots, so the chips line up from row to row. */
.slot {
  display: inline-flex;
  justify-content: flex-start;
}
.w-date {
  width: 112px;
}
.w-prio {
  width: 48px;
}
.w-who {
  width: 64px;
}
.chip {
  max-width: 100%;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 24px;
  padding: 0 7px;
  border-radius: 7px;
  font-size: 12px;
  color: var(--text-2);
  box-shadow: inset 0 0 0 0.5px var(--border);
  white-space: nowrap;
  transition: background var(--speed), opacity var(--speed);
}
.chip:not(:disabled):hover {
  background: var(--fill);
}
.chip.late {
  color: var(--crit-text);
}
.chip.empty {
  box-shadow: none;
  color: var(--text-3);
  opacity: 0;
}
.chip.empty:focus-visible {
  opacity: 1;
}
</style>
