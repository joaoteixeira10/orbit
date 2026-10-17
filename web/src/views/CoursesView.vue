<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Plus, GraduationCap, MapPin, User, Circle } from 'lucide-vue-next'
import CourseSheet from '@/components/school/CourseSheet.vue'
import { coursesApi, tasksApi, type Course, type CourseSlot, type Task } from '@/api/planner'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { onLiveEvent } from '@/lib/live'
import { CLIENT_ID } from '@/lib/api'
import { dueLabel, fmtClock, locale, weekdayNames, weekStart } from '@/lib/dates'
import { fmtMark } from '@/lib/grades'

// Courses: the week's timetable (every course's classes), then a card per
// course — teacher, room, its classes, its open tasks (tagged #tag) and,
// with Grades on, the person's average.
const route = useRoute()
const router = useRouter()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws))
const workspace = computed(() => wss.byId(ws.value))
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only)
const withGrades = computed(() => !!workspace.value?.modules.includes('grades'))
const courses = ref<Course[]>([])
const tasks = ref<Task[]>([])
const loaded = ref(false)

async function load() {
  try {
    const [c, ts] = await Promise.all([coursesApi.list(ws.value), tasksApi.list(ws.value, { state: 'active' }).catch(() => [] as Task[])])
    courses.value = c
    tasks.value = ts
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loaded.value = true
  }
}
watch(ws, load)
onMounted(load)
const off = onLiveEvent('ws', (e) => {
  if (e.origin === CLIENT_ID || String(e.ws) !== ws.value || !['courses', 'tasks', 'grades'].includes(String(e.what))) return
  void load()
})
onUnmounted(off)

// The sheet: ?course=<id> opens one (the calendar links here), ?new=1 a new one.
const sheetFor = ref<Course | null>(null)
const sheetOpen = ref(false)
function open(c: Course | null) {
  sheetFor.value = c
  sheetOpen.value = true
}
watch(
  () => [route.query.course, loaded.value] as const,
  ([id, ok]) => {
    if (!ok || typeof id !== 'string') return
    const c = courses.value.find((x) => x.id === id)
    if (c) open(c)
    router.replace({ query: { ...route.query, course: undefined } })
  },
  { immediate: true },
)
function onSaved(c: Course) {
  const i = courses.value.findIndex((x) => x.id === c.id)
  if (i >= 0) courses.value[i] = c
  else courses.value.push(c)
  sheetOpen.value = false
}
function onDeleted() {
  courses.value = courses.value.filter((x) => x.id !== sheetFor.value?.id)
  sheetOpen.value = false
}

// ---- The timetable ----
// Columns: Monday–Friday, plus the weekend days that have classes, in the
// person's week order. Rows: the hours from the first class to the last.
const HOUR = 52
const days = computed(() => {
  const ws = weekStart()
  const names = weekdayNames('short', ws)
  const used = new Set(courses.value.flatMap((c) => c.slots.map((s) => s.weekday)))
  return names
    .map((label, i) => ({ value: ((i + ws + 6) % 7) + 1, label }))
    .filter((d) => d.value <= 5 || used.has(d.value))
})
const minutes = (hhmm: string) => Number(hhmm.slice(0, 2)) * 60 + Number(hhmm.slice(3, 5))
interface Block {
  key: string
  course: Course
  slot: CourseSlot
  top: number
  height: number
}
const allSlots = computed(() => courses.value.flatMap((c) => c.slots.map((s) => ({ c, s }))))
const range = computed(() => {
  if (!allSlots.value.length) return { from: 8, to: 17 }
  const from = Math.floor(Math.min(...allSlots.value.map((x) => minutes(x.s.start))) / 60)
  const to = Math.ceil(Math.max(...allSlots.value.map((x) => minutes(x.s.end))) / 60)
  return { from, to: Math.max(to, from + 1) }
})
const hours = computed(() => Array.from({ length: range.value.to - range.value.from }, (_, i) => range.value.from + i))
function blocksOn(day: number): Block[] {
  return allSlots.value
    .filter((x) => x.s.weekday === day)
    .map(({ c, s }) => ({
      key: s.id,
      course: c,
      slot: s,
      top: ((minutes(s.start) - range.value.from * 60) / 60) * HOUR,
      height: Math.max(((minutes(s.end) - minutes(s.start)) / 60) * HOUR, 22),
    }))
}
const today = computed(() => ((new Date().getDay() + 6) % 7) + 1)

// ---- Cards ----
const tasksOf = (c: Course) =>
  c.tag ? tasks.value.filter((x) => x.tags.includes(c.tag)).sort((a, b) => (a.due ?? '9999').localeCompare(b.due ?? '9999')) : []
function openTask(x: Task) {
  router.push({ name: 'ws-tasks', params: { ws: ws.value, list: x.list_id }, query: { task: x.id } })
}
const scale = computed(() => workspace.value?.grade_scale ?? 20)
const mark = (v: number) => fmtMark(v, locale())
</script>

<template>
  <div class="courses">
    <header class="mb-6 flex items-end gap-4">
      <div class="min-w-0 flex-1">
        <h1 class="large-title">{{ t('school.courses') }}</h1>
        <p class="subhead mt-1">{{ t('school.coursesLead') }}</p>
      </div>
      <button v-if="editable" class="btn-primary shrink-0" @click="open(null)"><Plus class="h-4 w-4" />{{ t('school.newCourse') }}</button>
    </header>

    <template v-if="courses.length">
      <!-- The week -->
      <section v-if="allSlots.length" class="tt card mb-8" :aria-label="t('school.timetable')">
        <div class="tt-grid" :style="{ '--cols': days.length }">
          <div />
          <div v-for="d in days" :key="d.value" class="tt-day" :class="{ today: d.value === today }">{{ d.label }}</div>
          <div class="tt-hours" :style="{ height: hours.length * HOUR + 'px' }">
            <span v-for="(h, i) in hours" :key="h" :style="{ top: i * HOUR + 'px' }">{{ fmtClock(String(h).padStart(2, '0') + ':00') }}</span>
          </div>
          <div v-for="d in days" :key="d.value" class="tt-col" :class="{ today: d.value === today }" :style="{ height: hours.length * HOUR + 'px', '--hour': HOUR + 'px' }">
            <button
              v-for="b in blocksOn(d.value)"
              :key="b.key"
              class="tt-block"
              :style="{ top: b.top + 'px', height: b.height + 'px', '--c': b.course.color || 'var(--accent)' }"
              :title="`${b.course.name} · ${fmtClock(b.slot.start)}–${fmtClock(b.slot.end)}`"
              @click="open(b.course)"
            >
              <b>{{ b.course.code || b.course.name }}</b>
              <span v-if="b.height >= 40">{{ fmtClock(b.slot.start) }}–{{ fmtClock(b.slot.end) }}<template v-if="b.slot.room || b.course.room"> · {{ b.slot.room || b.course.room }}</template></span>
              <span v-if="b.height >= 58 && b.slot.kind" class="kind">{{ b.slot.kind }}</span>
            </button>
          </div>
        </div>
      </section>

      <!-- The courses -->
      <div class="cards">
        <article v-for="c in courses" :key="c.id" class="course card" :style="{ '--c': c.color || 'var(--accent)' }">
          <button class="course-head" @click="open(c)">
            <span class="dot" />
            <span class="min-w-0 flex-1 text-left">
              <span class="block truncate text-[16px] font-semibold">{{ c.name }}</span>
              <span class="flex flex-wrap gap-x-3 text-[12.5px] text-text-2">
                <span v-if="c.teacher" class="inline-flex items-center gap-1"><User class="h-3 w-3" />{{ c.teacher }}</span>
                <span v-if="c.room" class="inline-flex items-center gap-1"><MapPin class="h-3 w-3" />{{ c.room }}</span>
                <span>{{ t('school.classesN', { n: c.slots.length }, c.slots.length) }}</span>
              </span>
            </span>
            <span v-if="withGrades && c.average != null" class="avg" :class="{ low: c.average < scale / 2 }" :title="t('school.average')">{{ mark(c.average) }}</span>
          </button>
          <ul v-if="tasksOf(c).length" class="course-tasks">
            <li v-for="x in tasksOf(c).slice(0, 4)" :key="x.id">
              <button class="flex w-full items-center gap-2 text-left" @click="openTask(x)">
                <Circle class="h-3.5 w-3.5 shrink-0 text-text-3" />
                <span class="min-w-0 flex-1 truncate text-[13.5px]">{{ x.title }}</span>
                <span v-if="x.due" class="shrink-0 text-[12px] text-text-3">{{ dueLabel(x.due, x.due_time) }}</span>
              </button>
            </li>
          </ul>
          <p class="course-foot">#{{ c.tag }} · {{ t('school.openTasks', { n: c.open_tasks }, c.open_tasks) }}</p>
        </article>
      </div>
    </template>

    <div v-else-if="loaded" class="py-20 text-center">
      <GraduationCap class="mx-auto mb-3 h-10 w-10 text-text-faint" :stroke-width="1.5" />
      <p class="title-3">{{ t('school.noCourses') }}</p>
      <p class="subhead mx-auto mt-1 max-w-sm">{{ t('school.noCoursesHint') }}</p>
      <button v-if="editable" class="btn-primary mt-5" @click="open(null)"><Plus class="h-4 w-4" />{{ t('school.newCourse') }}</button>
    </div>

    <CourseSheet :open="sheetOpen" :ws="ws" :course="sheetFor" :editable="editable" @close="sheetOpen = false" @saved="onSaved" @deleted="onDeleted" />
  </div>
</template>

<style scoped>
.courses {
  padding: 32px clamp(16px, 4vw, 48px) 64px;
  max-width: 1100px;
}
.card {
  background: var(--surface);
  border-radius: 14px;
  box-shadow: var(--shadow-sm), 0 0 0 0.5px var(--border);
}
/* The timetable */
.tt {
  padding: 12px 14px 14px 6px;
  overflow-x: auto;
}
.tt-grid {
  display: grid;
  grid-template-columns: 44px repeat(var(--cols), minmax(92px, 1fr));
  column-gap: 6px;
  min-width: calc(44px + var(--cols) * 98px);
}
.tt-day {
  padding: 0 4px 8px;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-2);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.tt-day.today {
  color: var(--accent-text);
}
.tt-hours {
  position: relative;
}
.tt-hours span {
  position: absolute;
  right: 4px;
  transform: translateY(-50%);
  font-size: 11px;
  color: var(--text-3);
  font-variant-numeric: tabular-nums;
}
.tt-hours span:first-child {
  transform: none;
}
.tt-col {
  position: relative;
  border-radius: 8px;
  background: repeating-linear-gradient(to bottom, color-mix(in srgb, var(--border) 70%, transparent) 0 1px, transparent 1px var(--hour));
}
.tt-col.today {
  background-color: color-mix(in srgb, var(--accent) 5%, transparent);
}
.tt-block {
  position: absolute;
  left: 2px;
  right: 2px;
  display: flex;
  flex-direction: column;
  gap: 1px;
  overflow: hidden;
  padding: 4px 7px;
  border-radius: 7px;
  border-left: 3px solid var(--c);
  background: color-mix(in srgb, var(--c) 16%, var(--surface));
  text-align: left;
  font-size: 11.5px;
  line-height: 1.25;
  color: var(--text);
  transition: filter var(--speed);
}
.tt-block:hover {
  filter: brightness(0.97);
}
.tt-block b {
  font-size: 12.5px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.tt-block span {
  color: var(--text-2);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.tt-block .kind {
  color: var(--text-3);
}
/* The cards */
.cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 14px;
}
.course {
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.course-head {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px 10px;
}
.dot {
  width: 10px;
  height: 36px;
  flex: none;
  border-radius: 999px;
  background: var(--c);
}
.avg {
  flex: none;
  min-width: 44px;
  padding: 4px 8px;
  border-radius: 10px;
  background: var(--fill-2);
  font: 700 18px var(--font-display);
  font-variant-numeric: tabular-nums;
  text-align: center;
}
.avg.low {
  color: var(--crit-text);
}
.course-tasks {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 2px 16px 8px 38px;
}
.course-foot {
  margin-top: auto;
  padding: 8px 16px 12px 38px;
  font-size: 12px;
  color: var(--text-3);
}
</style>
