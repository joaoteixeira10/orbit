<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { AlarmClock, CalendarClock, ChartPie, CircleCheck, ListTodo, Sheet } from 'lucide-vue-next'
import TaskFilter from '@/components/tasks/TaskFilter.vue'
import Avatar from '@/components/ui/Avatar.vue'
import Donut from '@/components/charts/Donut.vue'
import StackedBars from '@/components/charts/StackedBars.vue'
import WeekColumns, { type Period } from '@/components/charts/WeekColumns.vue'
import Workload, { type WorkRow } from '@/components/charts/Workload.vue'
import ChartExport from '@/components/charts/ChartExport.vue'
import { csvOf, saveBlob, type ChartSpec } from '@/lib/chartExport'
import { fileName } from '@/lib/markdown'
import type { BarRow, Series } from '@/components/charts/types'
import '@/components/charts/tokens.css'
import { tasksApi, type Member, type Task } from '@/api/planner'
import { useTasksStore } from '@/stores/tasks'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { usePrefsStore } from '@/stores/prefs'
import { useToastStore } from '@/stores/toast'
import { useTaskFilter, useTaskFilterStore, dueBucket, dueEdges, DUE_BUCKETS, type DueBucket, type Progress } from '@/lib/taskFilter'
import { STATUSES, countBy, countStatus, statusOf, summary, totalOf, weekly, type Counts, type Status } from '@/lib/charts'
import { todayStr, parseDate, addDays, fmtDate, locale } from '@/lib/dates'

// Charts: the workspace's tasks at a glance, like Planner's Charts. Summary
// tiles, then status, lists, priority, people, tags and a weekly trend,
// all on the shared task filter. Every task counts (open and done), so the
// view loads them all itself; it reloads when the store's tasks change.
// Clicking a mark opens the Tasks view filtered to those tasks.
const route = useRoute()
const router = useRouter()
const tasks = useTasksStore()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const prefs = usePrefsStore()
const toast = useToastStore()
const filters = useTaskFilterStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws ?? ''))
const workspace = computed(() => wss.byId(ws.value))
const lists = computed(() => tasks.listsOf(ws.value))
const listMap = computed(() => new Map(lists.value.map((l) => [l.id, l])))
const me = computed(() => auth.user?.id ?? '')
const { active: filterOn, matches, clear } = useTaskFilter(ws, lists, me)

// ---- Data ----
const all = ref<Task[]>([])
const allWs = ref('') // whose tasks `all` holds
const busy = ref(false)
let seq = 0
async function fetchAll() {
  const w = ws.value
  if (!w) return
  const n = ++seq
  busy.value = true
  try {
    const r = await tasksApi.list(w, { state: 'all' })
    if (n === seq) {
      all.value = r
      allWs.value = w
    }
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    if (n === seq) busy.value = false
  }
}
watch(
  ws,
  (w) => {
    if (!w) return
    tasks.load(w).catch((e) => toast.error(e.message))
    void fetchAll()
  },
  { immediate: true },
)
// The store holds open and recently done tasks; when they change (an edit
// here or elsewhere, a live update), reload everything, a moment later.
const storeSig = computed(() => {
  const ts = tasks.tasks[ws.value]
  if (!ts) return ''
  let last = ''
  for (const x of ts) if (x.updated_at > last) last = x.updated_at
  return `${ts.length}|${last}`
})
let timer: ReturnType<typeof setTimeout> | undefined
watch([ws, storeSig], ([w, sig], [pw, psig]) => {
  if (w !== pw || !psig || !sig) return // a switch or first load: already fetching
  clearTimeout(timer)
  timer = setTimeout(fetchAll, 400)
})
onBeforeUnmount(() => clearTimeout(timer))

// Members, for the people chart (only when shared).
const shared = computed(() => (workspace.value?.member_count ?? 0) > 1)
watch(
  () => [ws.value, shared.value] as const,
  ([w, s]) => {
    if (w && s && !wss.members[w]) void wss.loadMembers(w).catch(() => {})
  },
  { immediate: true },
)
const members = computed(() => new Map<string, Member>((wss.members[ws.value] ?? []).map((m) => [m.user_id, m])))

// ---- Numbers ----
const today = todayStr()
const ready = computed(() => allWs.value === ws.value)
const topLevel = computed(() => (ready.value ? all.value.filter((x) => x.parent_id === null) : []))
const shown = computed(() => topLevel.value.filter(matches))

const COLOR: Record<Status, string> = { done: 'var(--c-done)', inProgress: 'var(--c-progress)', notStarted: 'var(--c-not-started)', late: 'var(--c-late)' }
const series = computed<Series[]>(() => STATUSES.map((s) => ({ key: s, label: t(`charts.status.${s}`), color: COLOR[s] })))
const counts = computed(() => countStatus(shown.value, listMap.value, today))
const total = computed(() => totalOf(counts.value))
const slices = computed(() => series.value.map((s) => ({ ...s, value: counts.value[s.key as Status] })))
const pctDone = computed(() => (total.value ? Math.round((counts.value.done / total.value) * 100) : 0))

const sum = computed(() => summary(shown.value, today, prefs.weekStart))
const tiles = computed<{ key: keyof typeof sum.value; icon: Component; color: string }[]>(() => [
  { key: 'open', icon: ListTodo, color: 'var(--c-progress)' },
  { key: 'late', icon: AlarmClock, color: 'var(--c-late)' },
  { key: 'dueWeek', icon: CalendarClock, color: 'var(--warn)' },
  { key: 'done7', icon: CircleCheck, color: 'var(--c-done)' },
])

const asRows = (m: Map<string, Counts>, keys: string[], label: (k: string) => string): BarRow[] =>
  keys.map((k) => ({ key: k, label: label(k), values: { ...(m.get(k) ?? {}) } }))

const byList = computed(() => {
  const m = countBy(shown.value, listMap.value, today, (x) => x.list_id)
  return asRows(
    m,
    lists.value.filter((l) => m.has(l.id)).map((l) => l.id),
    (k) => listMap.value.get(k)?.name ?? '',
  )
})

const PRIOS = ['3', '2', '1', '0']
const BANGS: Record<string, string> = { '3': '!!!', '2': '!!', '1': '!', '0': '' }
const byPriority = computed(() => asRows(countBy(shown.value, listMap.value, today, (x) => String(x.priority)), PRIOS, (k) => t(`filter.prio.${k}`)))

const memberName = (id: string) => {
  const m = members.value.get(id)
  return id === 'none' ? t('filter.unassigned') : m ? m.display_name || m.username : '?'
}
const byPerson = computed(() => {
  const m = countBy(shown.value, listMap.value, today, (x) => (x.assignee_ids.length ? x.assignee_ids : ['none']))
  const keys = [...m.keys()].sort((a, b) => (a === 'none' ? 1 : b === 'none' ? -1 : totalOf(m.get(b)!) - totalOf(m.get(a)!)))
  return asRows(m, keys, memberName)
})

const TAGS = 8
const byTag = computed(() => {
  const m = countBy(shown.value, listMap.value, today, (x) => x.tags)
  const keys = [...m.keys()].sort((a, b) => totalOf(m.get(b)!) - totalOf(m.get(a)!) || a.localeCompare(b)).slice(0, TAGS)
  return asRows(m, keys, (k) => `#${k}`)
})

const WEEKS = 12
const trendSeries = computed<Series[]>(() => [
  { key: 'created', label: t('charts.created'), color: 'var(--c-created)' },
  { key: 'completed', label: t('charts.completed'), color: 'var(--c-done)' },
])
const weeks = computed(() => weekly(shown.value, WEEKS, today, prefs.weekStart))
const periods = computed<Period[]>(() =>
  weeks.value.map((w) => {
    const d = parseDate(w.start)
    const end = addDays(d, 6)
    return {
      key: w.start,
      label: fmtDate(d),
      title: `${fmtDate(d)} – ${fmtDate(end)}`,
      values: { created: w.created, completed: w.completed },
    }
  }),
)
const trendTotals = computed(() => weeks.value.reduce((a, w) => ({ created: a.created + w.created, done: a.done + w.completed }), { created: 0, done: 0 }))

// ---- Workload: open tasks per person, by when they're due ----
const workCols = computed(() => DUE_BUCKETS.map((b) => ({ key: b, label: t(`filter.dueB.${b}`), hot: b === 'overdue' })))
const workload = computed<WorkRow[]>(() => {
  const e = dueEdges(today)
  const m = new Map<string, number[]>()
  for (const x of shown.value) {
    if (x.done) continue
    const i = DUE_BUCKETS.indexOf(dueBucket(x, e))
    for (const who of x.assignee_ids.length ? x.assignee_ids : ['none']) {
      const row = m.get(who) ?? DUE_BUCKETS.map(() => 0)
      row[i]++
      m.set(who, row)
    }
  }
  const sum = (k: string) => m.get(k)!.reduce((a, b) => a + b, 0)
  // Everyone in the workspace, the busiest first; nobody's tasks last.
  for (const id of members.value.keys()) if (!m.has(id)) m.set(id, DUE_BUCKETS.map(() => 0))
  const keys = [...m.keys()].sort((a, b) => (a === 'none' ? 1 : b === 'none' ? -1 : sum(b) - sum(a) || memberName(a).localeCompare(memberName(b))))
  return keys.map((k) => ({ key: k, label: memberName(k), values: m.get(k)! }))
})

// ---- Downloads: each chart as a picture or numbers; the tasks as CSV ----
const subtitle = () =>
  [workspace.value?.name, new Intl.DateTimeFormat(locale(), { dateStyle: 'long' }).format(new Date()), filterOn.value ? t('charts.filtered') : ''].filter(Boolean).join(' · ')
const barSpec = (title: string, rows: () => BarRow[]) => (): ChartSpec => ({ kind: 'bars', title, subtitle: subtitle(), series: series.value, rows: rows() })
const specs = {
  status: (): ChartSpec => ({ kind: 'donut', title: t('charts.status.title'), subtitle: subtitle(), slices: slices.value, center: `${pctDone.value}%` }),
  list: barSpec(t('charts.byList'), () => byList.value),
  priority: barSpec(t('charts.byPriority'), () => byPriority.value),
  person: barSpec(t('charts.byPerson'), () => byPerson.value),
  tag: barSpec(t('charts.byTag'), () => byTag.value),
  trend: (): ChartSpec => ({ kind: 'columns', title: t('charts.trend'), subtitle: subtitle(), series: trendSeries.value, periods: periods.value }),
  workload: (): ChartSpec => ({
    kind: 'table', title: t('charts.workload'), subtitle: subtitle(), cols: workCols.value.map((c) => c.label), rows: workload.value, hot: [0],
  }),
}
function tasksCsv() {
  const c = (k: string) => t(`charts.cols.${k}`)
  const names = (ids: string[]) => ids.map((id) => memberName(id)).join(', ')
  const rows = [
    [c('title'), c('list'), c('status'), c('priority'), c('assignees'), c('start'), c('due'), c('tags'), c('created'), c('completed')],
    ...shown.value.map((x) => [
      x.title, listMap.value.get(x.list_id)?.name ?? '', t(`charts.status.${statusOf(x, listMap.value.get(x.list_id), today)}`), t(`filter.prio.${x.priority}`),
      names(x.assignee_ids), x.start ?? '', x.due ? `${x.due}${x.due_time ? ' ' + x.due_time : ''}` : '', x.tags.join(', '),
      x.created_at.slice(0, 10), x.completed_at?.slice(0, 10) ?? '',
    ]),
  ]
  saveBlob(`${fileName(workspace.value?.name ?? 'tasks')}-${today}.csv`, new Blob([csvOf(rows)], { type: 'text/csv;charset=utf-8' }))
}
// ---- Drilling into the Tasks view ----
// drill narrows the shared filter (one dimension per argument) and opens
// the Tasks view, which shows the same tasks.
function drill(patch: { lists?: string[]; priority?: number[]; assignee?: string[]; tags?: string[]; progress?: Progress[]; due?: DueBucket[] }) {
  const f = filters.of(ws.value)
  for (const [k, v] of Object.entries(patch) as [keyof typeof patch, (string | number)[]][]) (f[k] as (string | number)[]).splice(0, f[k].length, ...v)
  filters.save()
  void router.push({ name: 'ws-tasks', params: { ws: ws.value, list: 'all' } })
}
const byStatus = (s: string) => (s === 'late' ? { due: ['overdue' as DueBucket] } : { progress: [s as Progress] })
const tileDrill: Record<string, Parameters<typeof drill>[0]> = {
  open: { progress: ['notStarted', 'inProgress'] },
  late: { due: ['overdue'] },
  dueWeek: { due: ['today', 'week'] },
  done7: { progress: ['done'] },
}
</script>

<template>
  <div class="charts">
    <header class="mb-6 flex items-end gap-3">
      <div class="min-w-0 flex-1">
        <h1 class="large-title truncate">{{ t('charts.title') }}</h1>
        <p class="subhead mt-0.5 tabular-nums">{{ ready ? t('charts.count', shown.length) : ' ' }}</p>
      </div>
      <div class="mb-1 flex shrink-0 items-center gap-1">
        <button v-if="ready && shown.length" type="button" class="icon-btn h-8 w-8" :title="t('charts.tasksCsv')" :aria-label="t('charts.tasksCsv')" data-testid="tasks-csv" @click="tasksCsv"><Sheet class="h-4 w-4" /></button>
        <TaskFilter :ws="ws" show-lists />
      </div>
    </header>

    <!-- Loading -->
    <p v-if="!ready" class="py-24 text-center text-[13px] text-text-3" aria-busy="true">{{ t('common.loading') }}</p>

    <!-- No tasks at all / none match -->
    <div v-else-if="!shown.length" class="empty card">
      <ChartPie class="mx-auto h-10 w-10 text-text-3" :stroke-width="1.5" />
      <h2 class="title-3 mt-3">{{ filterOn ? t('charts.noMatch') : t('charts.empty') }}</h2>
      <p v-if="!filterOn" class="subhead mx-auto mt-1 max-w-sm">{{ t('charts.emptyHint') }}</p>
      <button v-else class="btn btn-sm mt-4" @click="clear">{{ t('charts.clearFilter') }}</button>
    </div>

    <div v-else class="body" :class="{ busy }">
      <!-- Summary -->
      <div class="stats">
        <button v-for="x in tiles" :key="x.key" type="button" class="stat card" @click="drill(tileDrill[x.key])">
          <span class="stat-head">
            <span class="stat-icon" :style="{ background: x.color }"><component :is="x.icon" class="h-3.5 w-3.5" :stroke-width="2.4" /></span>
            {{ t(`charts.tiles.${x.key}`) }}
          </span>
          <span class="stat-value">{{ sum[x.key] }}</span>
          <span class="stat-sub">{{ t(`charts.tiles.${x.key}Sub`) }}</span>
        </button>
      </div>

      <div class="chart-grid">
        <!-- Status -->
        <section class="chart-card card">
          <header class="chart-head">
            <h2 class="title-3">{{ t('charts.status.title') }}</h2>
            <span class="footnote tabular-nums">{{ t('charts.count', total) }}</span>
            <ChartExport :spec="specs.status" :name="t('charts.status.title')" />
          </header>
          <div class="chart-body justify-center">
            <Donut
              :slices="slices"
              :center-value="`${pctDone}%`"
              :center-label="t('charts.complete')"
              :caption="t('charts.status.title')"
              selectable
              @select="(s) => drill(byStatus(s))"
            />
          </div>
        </section>

        <!-- By list -->
        <section class="chart-card card">
          <header class="chart-head"><h2 class="title-3">{{ t('charts.byList') }}</h2><ChartExport :spec="specs.list" :name="t('charts.byList')" /></header>
          <div class="chart-body">
            <StackedBars
              :rows="byList"
              :series="series"
              :caption="t('charts.byList')"
              :hint="t('charts.showTasks')"
              selectable
              @select="(r, s) => drill({ lists: [r], ...byStatus(s) })"
            >
              <template #label="{ row }">
                <span class="h-2.5 w-2.5 shrink-0 rounded-full" :style="{ background: listMap.get(row.key)?.color }" />
                <span class="truncate">{{ row.label }}</span>
              </template>
            </StackedBars>
          </div>
        </section>

        <!-- Trend -->
        <section class="chart-card card wide">
          <header class="chart-head">
            <h2 class="title-3">{{ t('charts.trend') }}</h2>
            <span class="footnote tabular-nums">{{ t('charts.trendSub', { done: trendTotals.done, created: trendTotals.created, n: WEEKS }) }}</span>
            <ChartExport :spec="specs.trend" :name="t('charts.trend')" />
          </header>
          <div class="chart-body">
            <WeekColumns v-if="trendTotals.done || trendTotals.created" :periods="periods" :series="trendSeries" :caption="t('charts.trend')" />
            <p v-else class="nothing">{{ t('charts.noActivity') }}</p>
          </div>
        </section>

        <!-- By priority -->
        <section class="chart-card card">
          <header class="chart-head"><h2 class="title-3">{{ t('charts.byPriority') }}</h2><ChartExport :spec="specs.priority" :name="t('charts.byPriority')" /></header>
          <div class="chart-body">
            <StackedBars
              :rows="byPriority"
              :series="series"
              :caption="t('charts.byPriority')"
              :hint="t('charts.showTasks')"
              selectable
              @select="(r, s) => drill({ priority: [Number(r)], ...byStatus(s) })"
            >
              <template #label="{ row }">
                <span class="bangs" aria-hidden="true">{{ BANGS[row.key] }}</span>
                <span class="truncate">{{ row.label }}</span>
              </template>
            </StackedBars>
          </div>
        </section>

        <!-- By person -->
        <section v-if="shared" class="chart-card card">
          <header class="chart-head"><h2 class="title-3">{{ t('charts.byPerson') }}</h2><ChartExport :spec="specs.person" :name="t('charts.byPerson')" /></header>
          <div class="chart-body">
            <StackedBars
              :rows="byPerson"
              :series="series"
              :caption="t('charts.byPerson')"
              :hint="t('charts.showTasks')"
              selectable
              @select="(r, s) => drill({ assignee: [r], ...byStatus(s) })"
            >
              <template #label="{ row }">
                <span v-if="row.key === 'none'" class="nobody" aria-hidden="true" />
                <Avatar v-else :name="row.label" :src="members.get(row.key)?.avatar_url" :size="22" />
                <span class="truncate" :class="{ 'text-text-2': row.key === 'none' }">{{ row.label }}</span>
              </template>
            </StackedBars>
          </div>
        </section>

        <!-- Workload -->
        <section v-if="shared" class="chart-card card wide" data-testid="workload">
          <header class="chart-head">
            <h2 class="title-3">{{ t('charts.workload') }}</h2>
            <span class="footnote">{{ t('charts.workloadSub') }}</span>
            <ChartExport :spec="specs.workload" :name="t('charts.workload')" />
          </header>
          <div class="chart-body">
            <Workload
              :cols="workCols"
              :rows="workload"
              :caption="t('charts.workload')"
              :hint="t('charts.showTasks')"
              @select="(r, c) => drill({ assignee: [r], due: [c as DueBucket], progress: ['notStarted', 'inProgress'] })"
            >
              <template #label="{ row }">
                <span v-if="row.key === 'none'" class="nobody" aria-hidden="true" />
                <Avatar v-else :name="row.label" :src="members.get(row.key)?.avatar_url" :size="22" />
                <span class="truncate" :class="{ 'text-text-2': row.key === 'none' }">{{ row.label }}</span>
              </template>
            </Workload>
          </div>
        </section>

        <!-- By tag -->
        <section v-if="byTag.length" class="chart-card card">
          <header class="chart-head">
            <h2 class="title-3">{{ t('charts.byTag') }}</h2>
            <span v-if="byTag.length === TAGS" class="footnote">{{ t('charts.byTagSub', { n: TAGS }) }}</span>
            <ChartExport :spec="specs.tag" :name="t('charts.byTag')" />
          </header>
          <div class="chart-body">
            <StackedBars
              :rows="byTag"
              :series="series"
              :caption="t('charts.byTag')"
              :hint="t('charts.showTasks')"
              selectable
              @select="(r, s) => drill({ tags: [r], ...byStatus(s) })"
            />
          </div>
        </section>
      </div>
    </div>
  </div>
</template>

<style scoped>
.charts {
  padding: 32px clamp(16px, 4vw, 48px) 64px;
}
.body {
  transition: opacity 0.25s var(--ease);
}
/* Reloading keeps the charts in place, a little faded. */
.body.busy {
  opacity: 0.7;
}
.empty {
  max-width: 520px;
  margin: 48px auto 0;
  padding: 40px 24px;
  text-align: center;
}

/* ---- Summary tiles ---- */
.stats {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 20px;
}
@media (min-width: 720px) {
  .stats {
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 20px;
  }
}
.stat {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  padding: 14px 16px 15px;
  text-align: left;
  transition:
    transform 0.18s var(--spring),
    box-shadow 0.18s;
}
.stat:hover {
  transform: translateY(-1px);
  box-shadow: 0 0 0 0.5px var(--border), var(--shadow);
}
.stat:active {
  transform: scale(0.985);
}
.stat-head {
  display: flex;
  align-items: center;
  gap: 7px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-2);
}
.stat-icon {
  display: grid;
  place-items: center;
  width: 22px;
  height: 22px;
  border-radius: 7px;
  color: #fff;
}
.stat-value {
  margin-top: 10px;
  font: 700 34px/1 var(--font-display);
  letter-spacing: -0.02em;
  color: var(--text);
}
.stat-sub {
  margin-top: 5px;
  font-size: 12px;
  color: var(--text-3);
}

/* ---- Panels: one column on phones, two on laptops, three on wide screens ---- */
.chart-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  grid-auto-flow: row dense;
  gap: 20px;
}
@media (min-width: 1024px) {
  .chart-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .wide {
    grid-column: span 2;
  }
}
@media (min-width: 1800px) {
  .chart-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}
.chart-card {
  display: flex;
  flex-direction: column;
  min-width: 0;
  padding: 18px 20px 20px;
}
.chart-head {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
}
/* The title leads; the count and the download button sit at the right. */
.chart-head > h2 {
  margin-right: auto;
}
.chart-body {
  display: flex;
  flex: 1;
  flex-direction: column;
}
.nothing {
  padding: 32px 0;
  text-align: center;
  font-size: 14px;
  color: var(--text-3);
}
.bangs {
  width: 1.6em;
  flex-shrink: 0;
  font-weight: 700;
  letter-spacing: -0.04em;
  color: var(--text-3);
}
.nobody {
  width: 22px;
  height: 22px;
  flex-shrink: 0;
  border-radius: 999px;
  border: 1.5px dashed var(--border-2);
}
</style>
