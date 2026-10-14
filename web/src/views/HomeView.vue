<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Plus, Settings, UserPlus, FileText, ChevronRight, Sparkles, MapPin } from 'lucide-vue-next'
import TaskRow from '@/components/tasks/TaskRow.vue'
import TaskDetailSheet from '@/components/tasks/TaskDetailSheet.vue'
import WsIcon from '@/components/ui/WsIcon.vue'
import Avatar from '@/components/ui/Avatar.vue'
import { plannerApi, tasksApi, type Home, type Task, type Occurrence } from '@/api/planner'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useTasksStore } from '@/stores/tasks'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { onLiveEvent } from '@/lib/live'
import { CLIENT_ID } from '@/lib/api'
import { ICONS } from '@/lib/icons'
import { todayStr, dateStr, startOfDay, addDays, fmtDate, fmtTime, relDay, sameDay, allDayDate, parseDate } from '@/lib/dates'

// Home. At / it is Today across every workspace: what's late and due, the
// week's events and recently edited pages. At /w/:ws it is that workspace's
// overview, with its modules and members.
const route = useRoute()
const wss = useWorkspacesStore()
const tasks = useTasksStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const wsId = computed(() => (typeof route.params.ws === 'string' ? route.params.ws : ''))
const workspace = computed(() => wss.byId(wsId.value))
const data = ref<Home | null>(null)
const loading = ref(true)

async function load() {
  const today = todayStr()
  const from = startOfDay(new Date())
  try {
    data.value = await plannerApi.home({ today, from: from.toISOString(), to: addDays(from, 7).toISOString(), ws: wsId.value || undefined })
  } catch (e: any) {
    if (e.status !== 404) toast.error(e.message)
  } finally {
    loading.value = false
  }
}
watch(wsId, () => {
  loading.value = true
  data.value = null
  void load()
  if (wsId.value && (workspace.value?.member_count ?? 1) > 1) void wss.loadMembers(wsId.value).catch(() => {})
})
let timer: ReturnType<typeof setTimeout> | null = null
let off: (() => void) | null = null
onMounted(() => {
  void load()
  if (wsId.value && (workspace.value?.member_count ?? 1) > 1) void wss.loadMembers(wsId.value).catch(() => {})
  off = onLiveEvent('ws', (e) => {
    if (e.origin === CLIENT_ID || (wsId.value && e.ws !== wsId.value)) return
    if (timer) clearTimeout(timer)
    timer = setTimeout(load, 300)
  })
})
onUnmounted(() => off?.())

// ---- Greeting ----
const name = computed(() => (auth.user?.display_name || auth.user?.username || '').split(' ')[0])
const greeting = computed(() => {
  const h = new Date().getHours()
  return t(h < 5 ? 'home.evening' : h < 12 ? 'home.morning' : h < 19 ? 'home.afternoon' : 'home.evening', { name: name.value })
})
const dateLine = computed(() => {
  const s = fmtDate(new Date(), { weekday: 'long', day: 'numeric', month: 'long' })
  return s.charAt(0).toUpperCase() + s.slice(1)
})

// ---- Tasks ----
const listColor = (x: Task) => wss.byId(x.workspace_id)?.color
const wsName = (x: Task) => (wsId.value ? '' : wss.byId(x.workspace_id)?.name ?? '')
const justDone = ref(new Set<string>())
async function toggle(x: Task) {
  const w = wss.byId(x.workspace_id)
  if (!w || w.role === 'viewer') return
  justDone.value.add(x.id)
  x.done = !x.done
  try {
    const r = await tasksApi.update(x.id, { completed: x.done })
    if (r.repeated) toast.success(t('tasks.repeated', { date: relDay(r.task.due!) }))
    if (tasks.tasks[x.workspace_id]) tasks.put(x.workspace_id, r.task)
  } catch (e: any) {
    x.done = !x.done
    toast.error(e.message)
  }
  setTimeout(() => {
    justDone.value.delete(x.id)
    void load()
  }, 1200)
}
const openTask = ref<{ ws: string; id: string } | null>(null)
async function open(x: Task) {
  if (!tasks.tasks[x.workspace_id]) await tasks.load(x.workspace_id).catch(() => {})
  openTask.value = { ws: x.workspace_id, id: x.id }
}
function closeTask() {
  openTask.value = null
  void load()
}

// Upcoming tasks grouped by day.
const upcomingByDay = computed(() => {
  const groups = new Map<string, Task[]>()
  for (const x of data.value?.upcoming ?? []) groups.set(x.due!, [...(groups.get(x.due!) ?? []), x])
  return [...groups]
})

// ---- Events, grouped by day ----
const eventsByDay = computed(() => {
  const days: { day: Date; items: Occurrence[] }[] = []
  const today = startOfDay(new Date())
  for (let i = 0; i < 7; i++) {
    const d = addDays(today, i)
    const items = (data.value?.events ?? []).filter((e) => {
      if (e.all_day) {
        const s = parseDate(allDayDate(e.start))
        const end = parseDate(allDayDate(e.end))
        return d >= s && d < end
      }
      const s = new Date(e.start)
      const end = new Date(e.end)
      return sameDay(s, d) || (s < d && end > d)
    })
    if (items.length) days.push({ day: d, items })
  }
  return days
})
const now = ref(Date.now())
const tick = setInterval(() => (now.value = Date.now()), 60000)
onUnmounted(() => clearInterval(tick))
const isNow = (e: Occurrence) => !e.all_day && new Date(e.start).getTime() <= now.value && new Date(e.end).getTime() > now.value

// ---- Workspace overview ----
const moduleRoute: Record<string, string> = { pages: 'ws-pages', tasks: 'ws-tasks', board: 'ws-board', calendar: 'ws-calendar' }
const modules = computed(() => (workspace.value?.modules ?? []).filter((m) => m !== 'home'))
const moduleIcon = (key: string) => ICONS[wss.modules.find((m) => m.key === key)?.icon ?? ''] ?? FileText
const members = computed(() => wss.members[wsId.value] ?? [])
function newWorkspace() {
  window.dispatchEvent(new CustomEvent('orbit:new-workspace'))
}
function settings(tab: 'general' | 'members' = 'general') {
  window.dispatchEvent(new CustomEvent('orbit:settings', { detail: { ws: wsId.value, tab } }))
}
const nothing = computed(() => !!data.value && !data.value.overdue.length && !data.value.today.length && !data.value.upcoming.length && !data.value.events.length)
</script>

<template>
  <div class="home">
    <!-- Header -->
    <header v-if="workspace" class="mb-8 flex flex-wrap items-center gap-4">
      <WsIcon :icon="workspace.icon" :color="workspace.color" :size="56" :name="workspace.name" />
      <div class="min-w-0 flex-1">
        <h1 class="large-title truncate">{{ workspace.name }}</h1>
        <p class="subhead mt-0.5 truncate">{{ workspace.description || t(`templates.${workspace.kind}.name`) }}</p>
      </div>
      <div class="flex items-center gap-2">
        <button v-if="members.length > 1" class="flex -space-x-2" :title="t('ws.members')" @click="settings('members')">
          <Avatar v-for="m in members.slice(0, 4)" :key="m.user_id" :name="m.display_name || m.username" :src="m.avatar_url" :size="30" class="ring-2 ring-[var(--bg)]" />
        </button>
        <button v-if="workspace.role === 'owner'" class="btn btn-sm" @click="settings('members')"><UserPlus class="h-4 w-4" />{{ t('ws.invite') }}</button>
        <button class="icon-btn" :title="t('ws.settings')" @click="settings()"><Settings class="h-[18px] w-[18px]" /></button>
      </div>
    </header>
    <header v-else class="mb-8">
      <p class="text-[13px] font-semibold uppercase tracking-wide text-text-3">{{ dateLine }}</p>
      <h1 class="large-title mt-1">{{ greeting }}</h1>
    </header>

    <p v-if="workspace?.archived_at" class="mb-6 rounded-xl bg-fill-2 px-4 py-2.5 text-[13px] text-text-2">{{ t('ws.archivedBanner') }}</p>

    <!-- Module shortcuts (workspace overview) -->
    <div v-if="workspace && modules.length" class="mb-8 grid grid-cols-2 gap-3 sm:grid-cols-4">
      <RouterLink v-for="m in modules" :key="m" :to="{ name: moduleRoute[m] ?? 'ws', params: { ws: workspace.id } }" class="mod card">
        <span class="tile h-9 w-9 rounded-[10px]" :style="{ '--tile': workspace.color }"><component :is="moduleIcon(m)" class="h-5 w-5" :stroke-width="2.2" /></span>
        <span class="mt-3 block text-[15px] font-semibold">{{ t(`modules.${m}`) }}</span>
        <span class="block text-[12px] leading-snug text-text-3">{{ t(`modules.${m}Desc`) }}</span>
      </RouterLink>
    </div>

    <!-- First run: no workspaces -->
    <div v-if="!workspace && wss.loaded && !wss.active.length" class="hero card">
      <Sparkles class="mx-auto h-10 w-10 text-accent" :stroke-width="1.6" />
      <h2 class="title-2 mt-4">{{ t('home.welcome') }}</h2>
      <p class="subhead mx-auto mt-2 max-w-md">{{ t('home.welcomeHint') }}</p>
      <button v-if="auth.canCreateWorkspaces" class="btn-primary btn-lg mt-6" @click="newWorkspace"><Plus class="h-5 w-5" />{{ t('ws.new') }}</button>
      <p v-else class="footnote mt-6">{{ t('home.askInvite') }}</p>
    </div>

    <div v-else-if="data" class="grid items-start gap-6 lg:grid-cols-2">
      <div class="flex min-w-0 flex-col gap-6">
      <!-- Today -->
      <section class="card p-5">
        <h2 class="widget-title" style="color: var(--accent-text)">{{ t('home.today') }}<span class="count">{{ data.overdue.length + data.today.length || '' }}</span></h2>
        <template v-if="data.overdue.length">
          <h3 class="sub text-crit-text">{{ t('tasks.overdue') }}</h3>
          <TaskRow
            v-for="x in data.overdue"
            :key="x.id"
            :task="x"
            :color="listColor(x)"
            :workspace-name="wsName(x)"
            compact
            :editable="wss.byId(x.workspace_id)?.role !== 'viewer'"
            class="border-b-[0.5px] border-[var(--line-soft)]"
            @toggle="toggle(x)"
            @open="open(x)"
          />
        </template>
        <TaskRow
          v-for="x in data.today"
          :key="x.id"
          :task="x"
          :color="listColor(x)"
          :workspace-name="wsName(x)"
          compact
          :editable="wss.byId(x.workspace_id)?.role !== 'viewer'"
          class="border-b-[0.5px] border-[var(--line-soft)]"
          @toggle="toggle(x)"
          @open="open(x)"
        />
        <p v-if="!data.overdue.length && !data.today.length" class="empty-line">{{ t('home.nothingToday') }}</p>
      </section>

      <!-- This week -->
      <section v-if="upcomingByDay.length" class="card p-5">
        <h2 class="widget-title" style="color: #ff9500">{{ t('home.thisWeek') }}</h2>
        <div v-for="[day, list] in upcomingByDay" :key="day" class="mb-2">
          <h3 class="sub">{{ relDay(day) }}</h3>
          <TaskRow
            v-for="x in list"
            :key="x.id"
            :task="x"
            :color="listColor(x)"
            :workspace-name="wsName(x)"
            compact
            :editable="wss.byId(x.workspace_id)?.role !== 'viewer'"
            @toggle="toggle(x)"
            @open="open(x)"
          />
        </div>
      </section>
      </div>

      <div class="flex min-w-0 flex-col gap-6">
      <!-- Up next: events -->
      <section class="card p-5">
        <h2 class="widget-title" style="color: #ff3b30">{{ t('home.upNext') }}</h2>
        <div v-for="g in eventsByDay" :key="dateStr(g.day)" class="mb-3">
          <h3 class="sub">{{ relDay(dateStr(g.day)) }}</h3>
          <RouterLink
            v-for="e in g.items"
            :key="e.id + e.start"
            :to="{ name: 'ws-calendar', params: { ws: e.workspace_id }, query: { date: dateStr(g.day) } }"
            class="event"
            :class="{ now: isNow(e) }"
          >
            <span class="bar" :style="{ background: e.color }" />
            <span class="min-w-0 flex-1">
              <span class="block truncate text-[14px] font-medium">{{ e.title }}</span>
              <span class="flex items-center gap-2 text-[12px] text-text-3">
                <span>{{ e.all_day ? t('home.allDay') : `${fmtTime(new Date(e.start))} – ${fmtTime(new Date(e.end))}` }}</span>
                <span v-if="e.location" class="inline-flex min-w-0 items-center gap-0.5 truncate"><MapPin class="h-3 w-3 shrink-0" />{{ e.location }}</span>
                <span v-if="!wsId">{{ wss.byId(e.workspace_id)?.name }}</span>
              </span>
            </span>
            <span v-if="isNow(e)" class="pill !bg-crit-bg !text-crit-text">{{ t('home.now') }}</span>
          </RouterLink>
        </div>
        <p v-if="!eventsByDay.length" class="empty-line">{{ t('home.noEvents') }}</p>
      </section>

      <!-- Recent pages -->
      <section v-if="data.pages.length" class="card p-5">
        <h2 class="widget-title" style="color: #5856d6">{{ t('home.recentPages') }}</h2>
        <div class="grid grid-cols-1 gap-1">
          <RouterLink v-for="p in data.pages" :key="p.id" :to="{ name: 'ws-page', params: { ws: p.workspace_id, page: p.id } }" class="page-link">
            <span class="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-fill-2 text-[16px]">
              <template v-if="p.icon">{{ p.icon }}</template><FileText v-else class="h-4 w-4 text-text-3" />
            </span>
            <span class="min-w-0 flex-1">
              <span class="block truncate text-[14px] font-medium">{{ p.title || t('pages.untitled') }}</span>
              <span class="block truncate text-[12px] text-text-3">{{ wsId ? '' : wss.byId(p.workspace_id)?.name + ' · ' }}{{ relDay(p.updated_at.slice(0, 10)) }}</span>
            </span>
            <ChevronRight class="h-4 w-4 text-text-faint" />
          </RouterLink>
        </div>
      </section>
      </div>

      <p v-if="nothing" class="footnote lg:col-span-2">{{ t('home.allClear') }}</p>
    </div>

    <div v-else-if="loading" class="grid gap-6 lg:grid-cols-2">
      <div v-for="i in 2" :key="i" class="card h-48 animate-pulse" />
    </div>

    <TaskDetailSheet v-if="openTask" :open="true" :ws="openTask.ws" :task-id="openTask.id" @close="closeTask" />
  </div>
</template>

<style scoped>
.home {
  padding: 32px clamp(16px, 4vw, 48px) 64px;
  max-width: 1180px;
}
.widget-title {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 8px;
  font: 700 20px var(--font-display);
  letter-spacing: -0.015em;
}
.widget-title .count {
  font-weight: 600;
  color: var(--text-3);
  font-size: 15px;
}
.sub {
  margin: 10px 0 2px;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-3);
}
.empty-line {
  padding: 10px 0;
  font-size: 14px;
  color: var(--text-3);
}
.event {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 7px 8px;
  margin: 0 -8px;
  border-radius: 9px;
}
.event:hover {
  background: var(--surface-hover);
}
.event .bar {
  width: 4px;
  align-self: stretch;
  border-radius: 4px;
}
.event.now {
  background: var(--crit-bg);
}
.page-link {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px;
  border-radius: 10px;
}
.page-link:hover {
  background: var(--surface-hover);
}
.mod {
  padding: 14px;
  transition: transform 0.18s var(--spring), box-shadow 0.18s;
}
.mod:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow);
}
.hero {
  padding: 48px 24px;
  text-align: center;
}
</style>
