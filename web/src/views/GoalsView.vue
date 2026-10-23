<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { Plus, Target, ChevronDown } from 'lucide-vue-next'
import GoalSheet from '@/components/goals/GoalSheet.vue'
import { goalsApi, pct, status, daysLeft, type Goal } from '@/lib/goals'
import { fmtDate, parseDate } from '@/lib/dates'
import { onLiveEvent } from '@/lib/live'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

// Goals: a card each — how far along (its tasks, or by hand), by when, and
// whether it's on track. Achieved ones fold away below.
const route = useRoute()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws))
const workspace = computed(() => wss.byId(ws.value))
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only)

const goals = ref<Goal[]>([])
const loaded = ref(false)
async function load() {
  try {
    goals.value = await goalsApi.list(ws.value)
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loaded.value = true
  }
}
watch(ws, () => {
  loaded.value = false
  void load()
}, { immediate: true })
const off = onLiveEvent('ws', (e) => {
  if (String(e.ws) === ws.value && (e.what === 'goals' || e.what === 'tasks')) void load()
})
onUnmounted(off)

const open = computed(() => goals.value.filter((g) => !g.achieved_at))
const achieved = computed(() => goals.value.filter((g) => g.achieved_at))
const showAchieved = ref(false)
const sheet = ref<{ open: boolean; goal: Goal | null }>({ open: false, goal: null })
function put(g: Goal) {
  const i = goals.value.findIndex((x) => x.id === g.id)
  if (i >= 0) goals.value[i] = g
  else goals.value.push(g)
}
const when = (g: Goal) => {
  const n = daysLeft(g)
  if (n === null) return t('goals.noDue')
  const date = fmtDate(parseDate(g.due!), { day: 'numeric', month: 'short', year: 'numeric' })
  return n >= 0 ? `${date} · ${t('goals.left', { n }, n)}` : `${date} · ${t('goals.past', { n: -n }, -n)}`
}
</script>

<template>
  <div class="goals-view">
    <header class="mb-6 flex flex-wrap items-end gap-x-4 gap-y-3">
      <div class="min-w-0 flex-1">
        <h1 class="large-title">{{ t('modules.goals') }}</h1>
        <p class="subhead mt-0.5">{{ workspace?.name }} · {{ open.length }}</p>
      </div>
      <button v-if="editable" class="btn-primary" data-testid="new-goal" @click="sheet = { open: true, goal: null }"><Plus class="h-4 w-4" />{{ t('goals.new') }}</button>
    </header>

    <div v-if="loaded && !goals.length" class="flex flex-col items-center py-16 text-center">
      <span class="tile h-14 w-14" style="--tile: #ff3b30"><Target class="h-7 w-7" /></span>
      <p class="mt-4 text-[15px] font-semibold">{{ t('goals.none') }}</p>
      <p class="mt-1 max-w-[360px] text-[13px] text-text-2">{{ t('goals.noneHint') }}</p>
      <button v-if="editable" class="btn-primary mt-5" @click="sheet = { open: true, goal: null }"><Plus class="h-4 w-4" />{{ t('goals.new') }}</button>
    </div>

    <div class="grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-4">
      <button v-for="g in open" :key="g.id" class="card goal text-left" :style="{ '--c': g.color || 'var(--accent)' }" data-testid="goal" @click="sheet = { open: true, goal: g }">
        <div class="flex items-start gap-3">
          <span class="emoji">{{ g.icon || '🎯' }}</span>
          <div class="min-w-0 flex-1">
            <div class="truncate text-[15px] font-semibold">{{ g.title }}</div>
            <div class="text-[12.5px] text-text-3">{{ when(g) }}</div>
          </div>
          <span class="status" :class="status(g)">{{ t(`goals.status.${status(g)}`) }}</span>
        </div>
        <div class="bar mt-4"><span :style="{ width: `${pct(g)}%` }" /></div>
        <div class="mt-1.5 flex justify-between text-[12.5px] text-text-2">
          <span>{{ g.tasks ? t('goals.tasksOf', { done: g.done, n: g.tasks }) : t('goals.byHand') }}</span>
          <span class="font-semibold tabular-nums">{{ pct(g) }}%</span>
        </div>
      </button>
    </div>

    <div v-if="achieved.length" class="mt-8">
      <button class="cap flex items-center gap-1 px-1" @click="showAchieved = !showAchieved">
        <ChevronDown class="h-3.5 w-3.5 transition" :class="{ '-rotate-90': !showAchieved }" />{{ t('goals.achieved') }} · {{ achieved.length }}
      </button>
      <div v-if="showAchieved" class="group mt-2">
        <button v-for="g in achieved" :key="g.id" class="group-row w-full text-left" @click="sheet = { open: true, goal: g }">
          <span>{{ g.icon || '🏆' }}</span><span class="flex-1">{{ g.title }}</span>
          <span class="text-[12px] text-text-3">{{ fmtDate(new Date(g.achieved_at!), { day: 'numeric', month: 'short', year: 'numeric' }) }}</span>
        </button>
      </div>
    </div>

    <GoalSheet :open="sheet.open" :ws="ws" :goal="sheet.goal" :editable="editable" @close="sheet.open = false" @saved="put" @removed="(id) => (goals = goals.filter((x) => x.id !== id))" />
  </div>
</template>

<style scoped>
.goals-view {
  padding: 32px clamp(16px, 4vw, 48px) 96px;
}
.goal {
  padding: 16px;
  transition: transform 0.15s var(--spring);
}
.goal:hover {
  transform: translateY(-2px);
}
.emoji {
  display: grid;
  flex: none;
  place-items: center;
  width: 38px;
  height: 38px;
  border-radius: 11px;
  background: color-mix(in srgb, var(--c) 16%, transparent);
  font-size: 19px;
}
.bar {
  height: 8px;
  overflow: hidden;
  border-radius: 999px;
  background: var(--fill);
}
.bar span {
  display: block;
  height: 100%;
  border-radius: 999px;
  background: var(--c);
  transition: width 0.3s;
}
.status {
  flex: none;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 11.5px;
  font-weight: 600;
  background: var(--fill);
  color: var(--text-2);
}
.status.on-track,
.status.done {
  background: color-mix(in srgb, #34c759 16%, transparent);
  color: color-mix(in srgb, #34c759 70%, var(--text));
}
.status.behind {
  background: color-mix(in srgb, #ff9500 18%, transparent);
  color: color-mix(in srgb, #ff9500 70%, var(--text));
}
.status.overdue {
  background: color-mix(in srgb, #ff3b30 16%, transparent);
  color: var(--crit-text);
}
</style>
