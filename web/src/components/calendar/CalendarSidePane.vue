<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, MapPin, GripVertical } from 'lucide-vue-next'
import { calendarApi, tasksApi, type Occurrence, type Task } from '@/api/planner'
import { toEntry, eventColor, type Entry, type TaskItem } from './model'
import { addDays, dateStr, dueLabel, fmtDate, fmtTime, isOverdue, relDay, startOfDay, todayStr } from '@/lib/dates'
import { startTaskDrag } from '@/lib/taskDrag'

// The calendar's right pane, as in Outlook: Calendar — what's coming, day by
// day for two weeks — and Tasks — the open ones, late first, then by date,
// those without one last. Drag a task onto the calendar to give it a date.
const props = defineProps<{
  wsIds: string[] // the workspaces shown
  ws: string // one workspace's calendar ('' = all)
  editable: (ws: string) => boolean
  color: (ws: string) => string
  visible: (o: { occ?: Occurrence; task?: Task }) => boolean
  version: number // bumped when the calendar reloads
}>()
const emit = defineEmits<{ open: [entry: Entry]; openTask: [item: TaskItem]; toggleTask: [item: TaskItem] }>()
const { t } = useI18n()

const tab = ref<'calendar' | 'tasks'>('calendar')
const events = ref<Occurrence[]>([])
const tasks = ref<Task[]>([])

async function loadAgenda() {
  const start = startOfDay(new Date())
  const end = addDays(start, 14)
  const p = { from: start.toISOString(), to: end.toISOString(), from_date: dateStr(start), to_date: dateStr(addDays(end, -1)) }
  try {
    const d = props.ws ? await calendarApi.workspace(props.ws, p) : await calendarApi.mine(p)
    events.value = d.events
  } catch {
    events.value = []
  }
}
async function loadTasks() {
  const all = await Promise.all(props.wsIds.map((w) => tasksApi.list(w, { state: 'active' }).catch(() => [] as Task[])))
  tasks.value = all.flat().filter((x) => !x.done && !x.parent_id)
}
function load() {
  void loadAgenda()
  if (tab.value === 'tasks') void loadTasks()
}
watch(() => [props.version, props.ws, props.wsIds.join()], load, { immediate: true })
watch(tab, (v) => v === 'tasks' && loadTasks())

// ---- Agenda: today and tomorrow always, then the days with something ----
const days = computed(() => {
  const today = startOfDay(new Date())
  const out: { key: string; label: string; entries: Entry[] }[] = []
  const entries = events.value.filter((o) => props.visible({ occ: o })).map((o) => toEntry(o, props.editable(o.workspace_id)))
  for (let i = 0; i < 14; i++) {
    const d = addDays(today, i)
    const key = dateStr(d)
    const on = entries.filter((e) => e.first <= key && e.last >= key).sort((a, b) => Number(b.allDay) - Number(a.allDay) || a.start.getTime() - b.start.getTime())
    if (i > 1 && !on.length) continue
    const full = fmtDate(d, { weekday: 'long', day: 'numeric', month: 'long' })
    out.push({ key, label: i < 2 ? `${relDay(key)} · ${full}` : full, entries: on })
  }
  return out
})
function time(e: Entry) {
  return e.allDay ? t('cal.allDayShort') : fmtTime(e.start)
}
function length(e: Entry) {
  if (e.allDay) return ''
  const m = Math.round((e.end.getTime() - e.start.getTime()) / 60000)
  return m < 60 ? `${m}m` : m % 60 ? `${Math.floor(m / 60)}h ${m % 60}m` : `${m / 60}h`
}

// ---- Tasks ----
const groups = computed(() => {
  const today = todayStr()
  const items = tasks.value.filter((x) => props.visible({ task: x })).map<TaskItem>((task) => ({ task, color: props.color(task.workspace_id), editable: props.editable(task.workspace_id) }))
  const byDue = (a: TaskItem, b: TaskItem) => `${a.task.due}${a.task.due_time ?? ''}`.localeCompare(`${b.task.due}${b.task.due_time ?? ''}`)
  return [
    { key: 'late', title: t('cal.pane.late'), items: items.filter((x) => isOverdue(x.task.due, x.task.due_time)).sort(byDue) },
    { key: 'today', title: t('dates.today'), items: items.filter((x) => x.task.due === today && !isOverdue(x.task.due, x.task.due_time)).sort(byDue) },
    { key: 'next', title: t('cal.pane.upcoming'), items: items.filter((x) => x.task.due && x.task.due > today).sort(byDue) },
    { key: 'none', title: t('cal.pane.noDate'), items: items.filter((x) => !x.task.due) },
  ].filter((g) => g.items.length)
})
function tick(x: TaskItem) {
  tasks.value = tasks.value.filter((y) => y.id !== x.task.id)
  emit('toggleTask', x)
}
</script>

<template>
  <aside class="pane" :aria-label="t('cal.pane.label')">
    <div class="seg mb-3 w-full" role="tablist">
      <button class="flex-1" role="tab" :aria-selected="tab === 'calendar'" :class="{ on: tab === 'calendar' }" @click="tab = 'calendar'">{{ t('cal.pane.calendar') }}</button>
      <button class="flex-1" role="tab" :aria-selected="tab === 'tasks'" :class="{ on: tab === 'tasks' }" @click="tab = 'tasks'">{{ t('cal.pane.tasks') }}</button>
    </div>

    <div v-if="tab === 'calendar'" class="scroll" data-testid="pane-agenda">
      <section v-for="d in days" :key="d.key" class="mb-3">
        <h3 class="day-head">{{ d.label }}</h3>
        <p v-if="!d.entries.length" class="px-2 py-1.5 text-[13px] text-text-3">{{ t('cal.pane.nothing') }}</p>
        <button v-for="e in d.entries" :key="e.key" class="ev" @click="emit('open', e)">
          <span class="when">
            <span>{{ time(e) }}</span>
            <small>{{ length(e) }}</small>
          </span>
          <span class="bar" :style="{ background: eventColor(e) }" />
          <span class="min-w-0 flex-1">
            <span class="block truncate text-[13.5px] font-medium">{{ e.occ.title || t('cal.untitled') }}</span>
            <span v-if="e.occ.location" class="flex items-center gap-1 truncate text-[12px] text-text-3"><MapPin class="h-3 w-3 shrink-0" />{{ e.occ.location }}</span>
          </span>
        </button>
      </section>
    </div>

    <div v-else class="scroll" data-testid="pane-tasks">
      <p class="mb-2 px-2 text-[12px] text-text-3">{{ t('cal.pane.dragHint') }}</p>
      <section v-for="g in groups" :key="g.key" class="mb-3">
        <h3 class="day-head" :class="{ 'text-crit-text': g.key === 'late' }">{{ g.title }} <span class="font-normal text-text-3">{{ g.items.length }}</span></h3>
        <div
          v-for="x in g.items"
          :key="x.task.id"
          class="task"
          :draggable="x.editable"
          :data-task="x.task.id"
          @dragstart="startTaskDrag($event, x.task)"
        >
          <GripVertical v-if="x.editable" class="grip h-3.5 w-3.5 shrink-0" />
          <button
            class="check"
            role="checkbox"
            :aria-checked="false"
            :aria-label="x.task.title"
            :style="{ '--check': x.color }"
            :disabled="!x.editable"
            @click="tick(x)"
          >
            <Check class="h-2.5 w-2.5" :stroke-width="3.4" />
          </button>
          <button class="min-w-0 flex-1 text-left" @click="emit('openTask', x)">
            <span class="block truncate text-[13.5px]">{{ x.task.title }}</span>
            <span v-if="x.task.due" class="block text-[12px]" :class="g.key === 'late' ? 'text-crit-text' : 'text-text-3'">{{ dueLabel(x.task.due, x.task.due_time) }}</span>
          </button>
          <i class="h-2 w-2 shrink-0 rounded-full" :style="{ background: x.color }" />
        </div>
      </section>
      <p v-if="!groups.length" class="px-2 py-6 text-center text-[13px] text-text-3">{{ t('cal.pane.noTasks') }}</p>
    </div>
  </aside>
</template>

<style scoped>
.pane {
  display: flex;
  flex-direction: column;
  width: 300px;
  flex: none;
  min-height: 0;
}
.scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}
.day-head {
  position: sticky;
  top: 0;
  z-index: 1;
  padding: 6px 8px;
  background: var(--bg, var(--surface));
  font-size: 12.5px;
  font-weight: 600;
  color: var(--accent-text);
}
.ev {
  display: flex;
  width: 100%;
  align-items: stretch;
  gap: 10px;
  padding: 7px 8px;
  border-radius: 10px;
  text-align: left;
}
.ev:hover,
.task:hover {
  background: var(--fill-2);
}
.when {
  display: flex;
  flex-direction: column;
  width: 52px;
  flex: none;
  font-size: 12.5px;
  font-variant-numeric: tabular-nums;
}
.when small {
  font-size: 11.5px;
  color: var(--text-3);
}
.bar {
  width: 3px;
  flex: none;
  border-radius: 2px;
}
.task {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px 6px 2px;
  border-radius: 10px;
}
.task[draggable='true'] {
  cursor: grab;
}
.grip {
  color: var(--text-faint);
  opacity: 0;
}
.task:hover .grip {
  opacity: 1;
}
</style>
