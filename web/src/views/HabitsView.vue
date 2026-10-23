<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { Plus, Check, Flame, Repeat, ChevronDown, Pencil } from 'lucide-vue-next'
import HabitSheet from '@/components/habits/HabitSheet.vue'
import YearGrid from '@/components/habits/YearGrid.vue'
import { habitsApi, streak, best, rate, counts, dueToday, type Habit } from '@/lib/habits'
import { addDays, dateStr, fmtDate, startOfWeek, sameDay, weekdayNames } from '@/lib/dates'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

// Habits: yours alone, a row each — how often, the streak, this week's days
// to tick, and (opened) the year and how it's going.
const route = useRoute()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws))
const workspace = computed(() => wss.byId(ws.value))
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only)

const habits = ref<Habit[]>([])
const loaded = ref(false)
watch(
  ws,
  async () => {
    loaded.value = false
    try {
      habits.value = await habitsApi.list(ws.value)
    } catch (e: any) {
      toast.error(e.message)
    } finally {
      loaded.value = true
    }
  },
  { immediate: true },
)
const active = computed(() => habits.value.filter((h) => !h.archived_at))
const archived = computed(() => habits.value.filter((h) => h.archived_at))
const showArchived = ref(false)

const today = new Date()
const week = computed(() => {
  const start = startOfWeek(today)
  return Array.from({ length: 7 }, (_, i) => addDays(start, i))
})
const names = computed(() => weekdayNames('narrow'))
const dueNow = computed(() => active.value.filter((h) => counts(h, today) || h.schedule === 'weekly'))
const doneNow = computed(() => dueNow.value.filter((h) => h.checks.includes(dateStr(today))).length)

function put(h: Habit) {
  const i = habits.value.findIndex((x) => x.id === h.id)
  if (i >= 0) habits.value[i] = h
  else habits.value.push(h)
}
async function toggle(h: Habit, d: Date) {
  if (!editable.value || d > today) return
  const day = dateStr(d)
  const done = !h.checks.includes(day)
  // At once on screen; the server's answer replaces it.
  h.checks = done ? [...h.checks, day].sort() : h.checks.filter((x) => x !== day)
  try {
    put(await habitsApi.check(h.id, day, done))
  } catch (e: any) {
    toast.error(e.message)
  }
}

const describe = (h: Habit) =>
  h.schedule === 'weekly'
    ? t('habits.timesWeek', { n: h.target }, h.target)
    : h.days === '1111111'
      ? t('habits.daily')
      : h.days === '1111100'
        ? t('habits.weekdays')
        : h.days === '0000011'
          ? t('habits.weekends')
          : weekdayNames('short', 1).filter((_, i) => h.days[i] === '1').join(', ')
const streakText = (h: Habit) => {
  const n = streak(h, today)
  return h.schedule === 'weekly' ? t('habits.streakWeeks', { n }, n) : t('habits.streakDays', { n }, n)
}

const opened = ref<string | null>(null)
const sheet = ref<{ open: boolean; habit: Habit | null }>({ open: false, habit: null })
</script>

<template>
  <div class="habits-view">
    <header class="mb-6 flex flex-wrap items-end gap-x-4 gap-y-3">
      <div class="min-w-0 flex-1">
        <h1 class="large-title">{{ t('modules.habits') }}</h1>
        <p class="subhead mt-0.5">
          {{ workspace?.name }}<template v-if="dueNow.length"> · {{ doneNow >= dueNow.length ? t('habits.allDone') : t('habits.todayCount', { done: doneNow, due: dueNow.length }) }}</template>
        </p>
      </div>
      <button v-if="editable" class="btn-primary" data-testid="new-habit" @click="sheet = { open: true, habit: null }"><Plus class="h-4 w-4" />{{ t('habits.new') }}</button>
    </header>

    <div v-if="loaded && !active.length && !archived.length" class="flex flex-col items-center py-16 text-center">
      <span class="tile h-14 w-14" style="--tile: #34c759"><Repeat class="h-7 w-7" /></span>
      <p class="mt-4 text-[15px] font-semibold">{{ t('habits.none') }}</p>
      <p class="mt-1 max-w-[360px] text-[13px] text-text-2">{{ t('habits.noneHint') }}</p>
      <button v-if="editable" class="btn-primary mt-5" @click="sheet = { open: true, habit: null }"><Plus class="h-4 w-4" />{{ t('habits.new') }}</button>
    </div>

    <div v-if="active.length" class="mb-2 hidden items-center gap-3 px-4 sm:flex">
      <span class="flex-1" />
      <span v-for="(d, i) in week" :key="i" class="w-9 text-center text-[11px] font-semibold" :class="sameDay(d, today) ? 'text-accent-text' : 'text-text-3'">{{ names[i] }}<br /><span class="font-normal">{{ d.getDate() }}</span></span>
    </div>
    <div class="space-y-2.5">
      <div v-for="h in active" :key="h.id" class="card habit" :style="{ '--c': h.color || 'var(--accent)' }" data-testid="habit">
        <div class="flex flex-wrap items-center gap-3 px-4 py-3">
          <button class="flex min-w-0 flex-1 items-center gap-3 text-left" :aria-expanded="opened === h.id" @click="opened = opened === h.id ? null : h.id">
            <span class="emoji">{{ h.icon || '•' }}</span>
            <span class="min-w-0">
              <span class="block truncate text-[15px] font-semibold">{{ h.title }}</span>
              <span class="flex flex-wrap items-center gap-x-2 text-[12.5px] text-text-3">
                <span>{{ describe(h) }}</span>
                <span v-if="streak(h, today)" class="inline-flex items-center gap-0.5 font-medium text-[color:var(--c)]"><Flame class="h-3.5 w-3.5" />{{ streakText(h) }}</span>
              </span>
            </span>
            <ChevronDown class="ml-auto h-4 w-4 shrink-0 text-text-3 transition" :class="{ 'rotate-180': opened === h.id }" />
          </button>
          <div class="flex gap-3">
            <button
              v-for="d in week"
              :key="dateStr(d)"
              class="tick"
              :class="{ done: h.checks.includes(dateStr(d)), off: !counts(h, d), today: sameDay(d, today), due: sameDay(d, today) && dueToday(h, today) }"
              :disabled="!editable || d > today"
              :aria-label="h.checks.includes(dateStr(d)) ? t('habits.markUndone', { day: fmtDate(d, { weekday: 'long' }) }) : t('habits.markDone', { day: fmtDate(d, { weekday: 'long' }) })"
              :aria-pressed="h.checks.includes(dateStr(d))"
              data-testid="habit-day"
              @click="toggle(h, d)"
            >
              <Check v-if="h.checks.includes(dateStr(d))" class="h-4 w-4" :stroke-width="3" />
            </button>
          </div>
        </div>
        <div v-if="opened === h.id" class="border-t border-line-soft px-4 py-3">
          <div class="mb-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-[12.5px] text-text-2">
            <span>{{ t('habits.best', { n: best(h, today) }) }}</span>
            <span>{{ t('habits.month', { p: Math.round(rate(h, today) * 100) }) }}</span>
            <button v-if="editable" class="btn-plain ml-auto !text-[13px]" @click="sheet = { open: true, habit: h }"><Pencil class="h-3.5 w-3.5" />{{ t('common.edit') }}</button>
          </div>
          <YearGrid :habit="h" :color="h.color || 'var(--accent)'" />
        </div>
      </div>
    </div>

    <div v-if="archived.length" class="mt-6">
      <button class="cap flex items-center gap-1 px-1" @click="showArchived = !showArchived">
        <ChevronDown class="h-3.5 w-3.5 transition" :class="{ '-rotate-90': !showArchived }" />{{ t('habits.archived') }} · {{ archived.length }}
      </button>
      <div v-if="showArchived" class="group mt-2">
        <button v-for="h in archived" :key="h.id" class="group-row w-full text-left" @click="editable && (sheet = { open: true, habit: h })">
          <span>{{ h.icon || '•' }}</span><span class="flex-1 text-text-2">{{ h.title }}</span>
        </button>
      </div>
    </div>

    <HabitSheet :open="sheet.open" :ws="ws" :habit="sheet.habit" @close="sheet.open = false" @saved="put" @removed="(id) => (habits = habits.filter((x) => x.id !== id))" />
  </div>
</template>

<style scoped>
.habits-view {
  padding: 32px clamp(16px, 4vw, 48px) 96px;
  max-width: 920px;
}
.emoji {
  display: grid;
  flex: none;
  place-items: center;
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: color-mix(in srgb, var(--c) 16%, transparent);
  font-size: 18px;
}
.tick {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  border-radius: 999px;
  box-shadow: inset 0 0 0 1.5px color-mix(in srgb, var(--c) 45%, transparent);
  color: #fff;
  transition: transform 0.12s var(--spring), background 0.12s;
}
.tick:not(:disabled):active {
  transform: scale(0.9);
}
.tick.done {
  background: var(--c);
  box-shadow: none;
}
.tick.off:not(.done) {
  box-shadow: inset 0 0 0 1px var(--border);
  opacity: 0.55;
}
.tick.due {
  box-shadow: inset 0 0 0 2px var(--c);
}
.tick:disabled {
  cursor: default;
}
@media (max-width: 639px) {
  .tick {
    width: 30px;
    height: 30px;
  }
  .habit .flex.gap-3 {
    gap: 6px;
    width: 100%;
    justify-content: space-between;
  }
}
</style>
