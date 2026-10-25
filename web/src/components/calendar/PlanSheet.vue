<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { CalendarClock, TriangleAlert } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import Checkbox from '@/components/ui/Checkbox.vue'
import { blocksApi, type Occurrence, type Task, type TimeBlock } from '@/api/planner'
import { estimateText, freeTime, planWeek, workingHours, type Block, type Span } from '@/lib/weekPlan'
import { addDays, dateStr, fmtDate, fmtTime, parseDate } from '@/lib/dates'
import { useTasksStore } from '@/stores/tasks'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

// Plan these days: my open tasks (for me or no one; due by the end of the
// days, or undated) fitted into the free time of my working hours —
// around timed events, tasks at a set time and my other blocks — the
// earliest due first, then the most important. A preview; Apply saves them
// as my time blocks (they show on my calendar; the tasks don't change).
const props = defineProps<{
  open: boolean
  days: Date[] // the days shown (local midnights)
  workspaceIds: string[] // planned over (one, or all shown)
  events: Occurrence[] // what's on those days already
  dueTasks: Task[] // tasks at a set time those days
  blocks: TimeBlock[] // my blocks now (those days)
}>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()
const tasksStore = useTasksStore()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()

// ---- Working hours (kept in this browser) ----
const KEY = 'orbit_plan_hours'
interface Hours {
  days: number[]
  from: string
  to: string
}
function readHours(): Hours {
  try {
    const v = JSON.parse(localStorage.getItem(KEY) || '')
    if (Array.isArray(v.days) && /^\d\d:\d\d$/.test(v.from) && /^\d\d:\d\d$/.test(v.to)) return v
  } catch {
    /* defaults */
  }
  return { days: [1, 2, 3, 4, 5], from: '09:00', to: '17:00' }
}
const hours = ref<Hours>(readHours())
watch(
  hours,
  (v) => {
    try {
      localStorage.setItem(KEY, JSON.stringify(v))
    } catch {
      /* private window */
    }
  },
  { deep: true },
)
const WEEK = [1, 2, 3, 4, 5, 6, 0]
const dayName = (d: number) => fmtDate(new Date(2026, 10, 1 + d), { weekday: 'short' }) // 1 Nov 2026 is a Sunday
function toggleDay(d: number) {
  const s = new Set(hours.value.days)
  if (!s.delete(d)) s.add(d)
  hours.value = { ...hours.value, days: [...s] }
}

// ---- The tasks ----
const from = computed(() => props.days[0] ?? new Date())
const to = computed(() => addDays(props.days[props.days.length - 1] ?? new Date(), 1))
const lastDay = computed(() => dateStr(addDays(to.value, -1)))
watch(
  () => props.open,
  async (o) => {
    if (!o) return
    await Promise.all(props.workspaceIds.filter((w) => !tasksStore.tasks[w]).map((w) => tasksStore.load(w).catch(() => undefined)))
    picked.value = new Set(candidates.value.filter((x) => x.estimate).map((x) => x.id))
  },
)
const me = computed(() => auth.user?.id ?? '')
const candidates = computed<Task[]>(() =>
  props.workspaceIds
    .flatMap((w) => tasksStore.tasksOf(w))
    .filter((x) => !x.done && (!x.assignee_ids.length || x.assignee_ids.includes(me.value)) && (!x.due || x.due <= lastDay.value) && !x.due_time)
    .sort((a, b) => (a.due ?? '9999').localeCompare(b.due ?? '9999') || b.priority - a.priority || a.title.localeCompare(b.title)),
)
const picked = ref(new Set<string>())
function pick(id: string, on: boolean) {
  const s = new Set(picked.value)
  if (on) s.add(id)
  else s.delete(id)
  picked.value = s
}
const ESTIMATES = [15, 30, 45, 60, 90, 120, 180, 240, 360, 480]
async function estimate(x: Task, minutes: number) {
  try {
    await tasksStore.update(x.workspace_id, x.id, { estimate: minutes })
    if (minutes) pick(x.id, true)
  } catch (e: any) {
    toast.error(e.message)
  }
}
const wsName = (id: string) => wss.byId(id)?.name ?? ''

// ---- The plan ----
const busy = computed<Span[]>(() => {
  const out: Span[] = []
  for (const o of props.events) {
    if (o.kind === 'block' || o.all_day) continue
    out.push({ start: Date.parse(o.start), end: Date.parse(o.end) })
  }
  for (const x of props.dueTasks) {
    if (!x.due || !x.due_time) continue
    const s = new Date(`${x.due}T${x.due_time}`).getTime()
    out.push({ start: s, end: s + Math.max(30, x.estimate || 0) * 60_000 })
  }
  // My blocks in workspaces not being planned stay where they are.
  for (const b of props.blocks) if (!props.workspaceIds.includes(b.workspace_id)) out.push({ start: Date.parse(b.starts_at), end: Date.parse(b.ends_at) })
  return out
})
// A day off: an all-day holiday.
const holidays = computed(() => new Set(props.events.filter((o) => o.kind === 'holiday').map((o) => o.start.slice(0, 10))))
const plan = computed(() => {
  const work = workingHours(from.value, props.days.length, hours.value.days, hours.value.from, hours.value.to).filter((w) => !holidays.value.has(dateStr(new Date(w.start))))
  const free = freeTime(work, busy.value, Date.now())
  const tasks = candidates.value
    .filter((x) => picked.value.has(x.id) && x.estimate)
    .map((x) => ({ id: x.id, minutes: x.estimate!, due: x.due ? addDays(parseDate(x.due), 1).getTime() : undefined, priority: x.priority }))
  return planWeek(tasks, free)
})
const byId = computed(() => new Map(candidates.value.map((x) => [x.id, x])))
const byDay = computed(() => {
  const out: { day: string; blocks: Block[] }[] = []
  for (const b of plan.value.blocks) {
    const d = dateStr(new Date(b.start))
    const last = out[out.length - 1]
    if (last?.day === d) last.blocks.push(b)
    else out.push({ day: d, blocks: [b] })
  }
  return out
})

const saving = ref(false)
async function apply(blocks: Block[]) {
  saving.value = true
  try {
    await blocksApi.plan({
      from: from.value.toISOString(),
      to: to.value.toISOString(),
      workspace_ids: props.workspaceIds,
      blocks: blocks.map((b) => ({ task_id: b.taskId, starts_at: new Date(b.start).toISOString(), ends_at: new Date(b.end).toISOString() })),
    })
    toast.success(blocks.length ? t('plan.saved', { n: blocks.length }, blocks.length) : t('plan.cleared'))
    emit('saved')
    emit('close')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}
const mine = computed(() => props.blocks.filter((b) => props.workspaceIds.includes(b.workspace_id)).length)
</script>

<template>
  <Overlay :open="open" :title="t('plan.title')" size="lg" @close="emit('close')">
    <div class="space-y-4" data-testid="plan-sheet">
      <p class="text-[13px] leading-relaxed text-text-2">{{ t('plan.hint', { from: fmtDate(from, { weekday: 'short', day: 'numeric', month: 'short' }), to: fmtDate(addDays(to, -1), { weekday: 'short', day: 'numeric', month: 'short' }) }) }}</p>

      <div class="group">
        <div class="group-row flex-wrap !gap-y-2">
          <span class="text-[14px]">{{ t('plan.hours') }}</span>
          <div class="ml-auto flex flex-wrap items-center gap-1">
            <button v-for="d in WEEK" :key="d" type="button" class="day" :class="{ on: hours.days.includes(d) }" :aria-pressed="hours.days.includes(d)" @click="toggleDay(d)">{{ dayName(d) }}</button>
          </div>
          <div class="flex items-center gap-1.5">
            <input v-model="hours.from" type="time" class="input !w-auto !py-1 text-[13px]" :aria-label="t('plan.from')" data-testid="plan-from" />
            <span class="text-text-3">–</span>
            <input v-model="hours.to" type="time" class="input !w-auto !py-1 text-[13px]" :aria-label="t('plan.to')" data-testid="plan-to" />
          </div>
        </div>
      </div>

      <div>
        <h3 class="cap mb-1.5">{{ t('plan.tasks') }}</h3>
        <p v-if="!candidates.length" class="text-[13px] text-text-3">{{ t('plan.noTasks') }}</p>
        <div class="group max-h-[260px] overflow-y-auto">
          <div v-for="x in candidates" :key="x.id" class="group-row !py-1.5" data-testid="plan-task">
            <Checkbox :model-value="picked.has(x.id)" :disabled="!x.estimate" :aria-label="x.title" @update:model-value="(v) => pick(x.id, v)" />
            <div class="min-w-0 flex-1">
              <div class="truncate text-[13.5px]"><span v-if="x.priority" class="mr-1 font-semibold text-warn">{{ '!'.repeat(x.priority) }}</span>{{ x.title }}</div>
              <div class="text-[11.5px] text-text-3">
                <template v-if="x.due">{{ t('plan.due', { day: fmtDate(parseDate(x.due), { weekday: 'short', day: 'numeric', month: 'short' }) }) }}</template>
                <template v-if="workspaceIds.length > 1"> · {{ wsName(x.workspace_id) }}</template>
              </div>
            </div>
            <select class="input !w-auto !py-0.5 text-[12.5px]" :value="String(x.estimate || 0)" :aria-label="t('tasks.estimate')" data-testid="plan-estimate" @change="estimate(x, Number(($event.target as HTMLSelectElement).value))">
              <option value="0">{{ t('plan.howLong') }}</option>
              <option v-for="m in ESTIMATES" :key="m" :value="String(m)">{{ estimateText(m) }}</option>
              <option v-if="x.estimate && !ESTIMATES.includes(x.estimate)" :value="String(x.estimate)">{{ estimateText(x.estimate) }}</option>
            </select>
          </div>
        </div>
      </div>

      <div>
        <h3 class="cap mb-1.5">{{ t('plan.preview') }}</h3>
        <p v-if="!plan.blocks.length" class="text-[13px] text-text-3">{{ t('plan.nothing') }}</p>
        <div v-for="d in byDay" :key="d.day" class="mb-2" data-testid="plan-day">
          <div class="text-[12.5px] font-semibold text-text-2">{{ fmtDate(parseDate(d.day), { weekday: 'long', day: 'numeric', month: 'long' }) }}</div>
          <div v-for="(b, i) in d.blocks" :key="i" class="flex items-center gap-2 py-0.5 text-[13px]" data-testid="plan-block">
            <CalendarClock class="h-3.5 w-3.5 shrink-0 text-text-3" />
            <span class="w-[104px] shrink-0 tabular-nums text-text-2">{{ fmtTime(new Date(b.start)) }} – {{ fmtTime(new Date(b.end)) }}</span>
            <span class="truncate">{{ byId.get(b.taskId)?.title }}</span>
          </div>
        </div>
        <p v-for="u in plan.unplaced" :key="u.id" class="flex items-center gap-1.5 text-[12.5px] text-warn" data-testid="plan-unplaced">
          <TriangleAlert class="h-3.5 w-3.5 shrink-0" />{{ t('plan.noRoom', { title: byId.get(u.id)?.title ?? '', left: estimateText(u.minutes) }) }}
        </p>
      </div>
    </div>
    <template #footer>
      <button v-if="mine" class="btn-ghost mr-auto" :disabled="saving" data-testid="plan-clear" @click="apply([])">{{ t('plan.clear') }}</button>
      <button class="btn" @click="emit('close')">{{ t('common.cancel') }}</button>
      <button class="btn-primary" :disabled="saving || !plan.blocks.length" data-testid="plan-apply" @click="apply(plan.blocks)">{{ t('plan.apply') }}</button>
    </template>
  </Overlay>
</template>

<style scoped>
.day {
  min-width: 38px;
  height: 28px;
  padding: 0 6px;
  border-radius: 8px;
  font-size: 12.5px;
  color: var(--text-2);
  background: var(--fill-2);
}
.day.on {
  background: var(--accent);
  color: var(--accent-fg, #fff);
}
</style>
