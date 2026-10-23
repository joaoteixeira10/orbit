<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { Play, Square, Timer, Check } from 'lucide-vue-next'
import { useFocusStore, clockOf, durationOf } from '@/stores/focus'
import { useTasksStore } from '@/stores/tasks'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useToastStore } from '@/stores/toast'
import { fmtTime } from '@/lib/dates'

// The focus timer: pick a task (or none) and a length, Start; a ring counts
// down. Stopping early counts the time so far; either way it's logged on
// the task. Below: what was focused on today.
const route = useRoute()
const focus = useFocusStore()
const tasks = useTasksStore()
const wss = useWorkspacesStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws))
const workspace = computed(() => wss.byId(ws.value))
const lengths = [15, 25, 50]
const minutes = ref(25)
const taskId = ref(typeof route.query.task === 'string' ? route.query.task : '')

watch(
  ws,
  () => {
    void focus.load(ws.value).catch(() => {})
    void tasks.load(ws.value).catch(() => {})
  },
  { immediate: true },
)
const open = computed(() => tasks.tasksOf(ws.value).filter((x) => !x.done && !x.parent_id))
const here = computed(() => focus.running && focus.running.workspace_id === ws.value)
const elsewhere = computed(() => focus.running && focus.running.workspace_id !== ws.value)

const stopBtn = ref<HTMLButtonElement | null>(null)
async function start() {
  try {
    await focus.start(ws.value, taskId.value, minutes.value)
    // Start is gone: keep the focus on what's there now.
    await nextTick()
    stopBtn.value?.focus({ preventScroll: true })
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function stop() {
  try {
    await focus.stop()
  } catch (e: any) {
    toast.error(e.message)
  }
}
function onDone(e: Event) {
  const s = (e as CustomEvent).detail
  if (s?.workspace_id === ws.value) toast.success(t('focus.done', { t: durationOf(s.minutes) }))
}
onMounted(() => window.addEventListener('orbit:focus-done', onDone))
onUnmounted(() => window.removeEventListener('orbit:focus-done', onDone))

// The dial: a circle's dash, filled as time passes.
const R = 108
const C = 2 * Math.PI * R
const dash = computed(() => `${(here.value ? focus.progress : 0) * C} ${C}`)
const shown = computed(() => (here.value ? clockOf(focus.left) : clockOf(minutes.value * 60)))
</script>

<template>
  <div class="focus-view">
    <header class="mb-8">
      <h1 class="large-title">{{ t('modules.focus') }}</h1>
      <p class="subhead mt-0.5">{{ workspace?.name }} · {{ t('focus.todayTotal', { t: durationOf(focus.minutes) }) }}</p>
    </header>

    <div class="flex flex-col items-center">
      <div class="dial" :style="{ '--c': workspace?.color || 'var(--accent)' }">
        <svg viewBox="0 0 240 240" aria-hidden="true">
          <circle cx="120" cy="120" :r="R" class="track" />
          <circle cx="120" cy="120" :r="R" class="fill" :stroke-dasharray="dash" transform="rotate(-90 120 120)" />
        </svg>
        <div class="face">
          <span class="clock" data-testid="focus-clock">{{ shown }}</span>
          <span v-if="here" class="mt-1 max-w-[170px] truncate text-[13px] text-text-2">{{ focus.running?.task_title || t('focus.noTask') }}</span>
        </div>
      </div>

      <template v-if="!here">
        <p v-if="elsewhere" class="mt-4 text-[13px] text-text-3">{{ t('focus.elsewhere') }}</p>
        <div class="seg mt-6" role="radiogroup">
          <button v-for="m in lengths" :key="m" :class="{ on: minutes === m }" @click="minutes = m">{{ t('focus.minutes', { n: m }) }}</button>
        </div>
        <label class="mt-4 flex w-full max-w-[380px] items-center gap-2">
          <span class="text-[13px] text-text-2">{{ t('focus.task') }}</span>
          <select v-model="taskId" class="input min-w-0 flex-1 !py-1.5 text-[14px]" data-testid="focus-task">
            <option value="">{{ t('focus.noTask') }}</option>
            <option v-for="x in open" :key="x.id" :value="x.id">{{ x.title }}</option>
          </select>
        </label>
        <button class="btn-primary mt-6 !px-8" data-testid="focus-start" @click="start"><Play class="h-4 w-4" />{{ t('focus.start') }}</button>
      </template>
      <template v-else>
        <button ref="stopBtn" class="btn mt-8 !px-8" :title="t('focus.stopHint')" data-testid="focus-stop" @click="stop"><Square class="h-4 w-4" />{{ t('focus.stop') }}</button>
        <p class="mt-2 text-[12px] text-text-3">{{ t('focus.stopHint') }}</p>
      </template>
    </div>

    <section class="mx-auto mt-10 max-w-[520px]">
      <h3 class="cap mb-1.5 px-1">{{ t('focus.today') }}</h3>
      <div v-if="focus.today.length" class="group">
        <div v-for="s in focus.today" :key="s.id" class="group-row">
          <Check v-if="s.spent >= s.minutes" class="h-4 w-4 shrink-0 text-ok" />
          <Timer v-else class="h-4 w-4 shrink-0 text-text-3" />
          <span class="min-w-0 flex-1 truncate text-[14px]">{{ s.task_title || t('focus.noTask') }}</span>
          <span class="text-[12.5px] tabular-nums text-text-3">{{ fmtTime(new Date(s.started_at)) }} · {{ durationOf(s.spent) }}</span>
        </div>
      </div>
      <p v-else class="footnote px-1">{{ t('focus.none') }}</p>
    </section>
  </div>
</template>

<style scoped>
.focus-view {
  padding: 32px clamp(16px, 4vw, 48px) 96px;
}
.dial {
  position: relative;
  width: 260px;
  height: 260px;
}
.dial svg {
  width: 100%;
  height: 100%;
}
.track {
  fill: none;
  stroke: var(--fill);
  stroke-width: 12;
}
.fill {
  fill: none;
  stroke: var(--c);
  stroke-width: 12;
  stroke-linecap: round;
  transition: stroke-dasharray 0.9s linear;
}
.face {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
}
.clock {
  font-size: 56px;
  font-weight: 300;
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.02em;
}
</style>
