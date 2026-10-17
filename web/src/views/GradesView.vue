<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Award, Plus, X, Lock, Check } from 'lucide-vue-next'
import { coursesApi, type Course, type Grade } from '@/api/planner'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { onLiveEvent } from '@/lib/live'
import { CLIENT_ID } from '@/lib/api'
import { fmtDate, locale, parseDate, todayStr } from '@/lib/dates'
import { average, fmtMark, needed, passes } from '@/lib/grades'

// Grades: the person's own marks in each course (nobody else sees them),
// each course's weighted average, the overall one (courses weighed by their
// weight), a goal per course and what the next mark must be to reach it.
const route = useRoute()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws))
const workspace = computed(() => wss.byId(ws.value))
const isOwner = computed(() => workspace.value?.role === 'owner')
const canEditCourses = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only)
const canWrite = computed(() => !!workspace.value && !workspace.value.archived_at && !auth.privileges.read_only) // my own grades: viewers too
const scale = computed(() => workspace.value?.grade_scale ?? 20)

const courses = ref<Course[]>([])
const grades = ref<Grade[]>([])
const loaded = ref(false)

async function load() {
  try {
    const [c, g] = await Promise.all([coursesApi.list(ws.value), coursesApi.grades(ws.value)])
    courses.value = c
    grades.value = g
    for (const co of c) {
      goals[co.id] = co.target == null ? '' : String(co.target)
      if (!(co.id in needW)) needW[co.id] = 1
    }
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loaded.value = true
  }
}
watch(ws, load)
onMounted(load)
const off = onLiveEvent('ws', (e) => {
  if (e.origin === CLIENT_ID || String(e.ws) !== ws.value || !['courses', 'grades', 'workspace'].includes(String(e.what))) return
  void load()
})
onUnmounted(off)

const gradesOf = (id: string) => grades.value.filter((g) => g.course_id === id)
const avgOf = (id: string) => average(gradesOf(id))
const mark = (fraction: number) => fmtMark(fraction * scale.value, locale())
const overall = computed(() => {
  let sum = 0
  let w = 0
  for (const c of courses.value) {
    const a = avgOf(c.id)
    if (a == null || c.weight <= 0) continue
    sum += a * c.weight
    w += c.weight
  }
  return w ? sum / w : null
})

async function setScale(n: number) {
  try {
    await wss.update(ws.value, { grade_scale: n })
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ---- Adding and changing grades ----
const draft = reactive<Record<string, { title: string; score: string; out_of: string; weight: string; date: string }>>({})
const blank = () => ({ title: '', score: '', out_of: String(scale.value), weight: '1', date: todayStr() })
const draftOf = (id: string) => (draft[id] ??= blank())
const editing = ref('') // a grade being changed
const edit = reactive({ title: '', score: '', out_of: '', weight: '', date: '' })

const num = (s: string) => Number(String(s).replace(',', '.'))
const validDraft = (d: { title: string; score: string; out_of: string; weight: string }) =>
  !!d.title.trim() && d.score !== '' && num(d.score) >= 0 && num(d.out_of) > 0 && num(d.weight) >= 0

async function addGrade(c: Course) {
  const d = draftOf(c.id)
  if (!validDraft(d)) return
  try {
    const g = await coursesApi.addGrade(c.id, { title: d.title.trim(), score: num(d.score), out_of: num(d.out_of), weight: num(d.weight), date: d.date })
    grades.value.push(g)
    draft[c.id] = { ...blank(), out_of: d.out_of, weight: d.weight }
  } catch (e: any) {
    toast.error(e.message)
  }
}
function startEdit(g: Grade) {
  if (!canWrite.value) return
  editing.value = g.id
  Object.assign(edit, { title: g.title, score: String(g.score), out_of: String(g.out_of), weight: String(g.weight), date: g.date ?? '' })
}
async function saveEdit() {
  if (!editing.value || !validDraft(edit)) return
  try {
    const g = await coursesApi.updateGrade(editing.value, { title: edit.title.trim(), score: num(edit.score), out_of: num(edit.out_of), weight: num(edit.weight), date: edit.date })
    grades.value = grades.value.map((x) => (x.id === g.id ? g : x))
    editing.value = ''
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function removeGrade(g: Grade) {
  try {
    await coursesApi.removeGrade(g.id)
    grades.value = grades.value.filter((x) => x.id !== g.id)
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ---- Goals and what's needed ----
const goals = reactive<Record<string, string>>({})
const needW = reactive<Record<string, number>>({})
async function saveGoal(c: Course) {
  const v = goals[c.id].trim()
  const target = v === '' ? null : num(v)
  if (target != null && (Number.isNaN(target) || target < 0 || target > scale.value)) return
  if (target === c.target) return
  try {
    await coursesApi.setGoal(c.id, target)
    c.target = target
  } catch (e: any) {
    toast.error(e.message)
  }
}
function need(c: Course): string {
  if (c.target == null) return t('school.needGoal')
  const target = c.target / scale.value
  const n = needed(gradesOf(c.id), target, needW[c.id] || 0)
  const goal = fmtMark(c.target, locale())
  if (n <= 0) return t('school.needReached', { target: goal })
  if (n > 1) {
    const best = average([...gradesOf(c.id), { score: 1, out_of: 1, weight: needW[c.id] || 0 } as Grade]) ?? 1
    return t('school.needOut', { best: mark(best) })
  }
  return t('school.needResult', { mark: `${mark(n)}/${scale.value}`, target: goal })
}

// ---- A first course, from here ----
const newCourse = ref('')
async function addCourse() {
  if (!newCourse.value.trim()) return
  try {
    courses.value.push(await coursesApi.create(ws.value, { name: newCourse.value.trim() }))
    newCourse.value = ''
  } catch (e: any) {
    toast.error(e.message)
  }
}
const dateLabel = (d: string | null) => (d ? fmtDate(parseDate(d)) : '')
</script>

<template>
  <div class="grades">
    <header class="mb-2 flex flex-wrap items-end gap-4">
      <div class="min-w-0 flex-1">
        <h1 class="large-title">{{ t('school.grades') }}</h1>
        <p class="subhead mt-1">{{ t('school.gradesLead') }}</p>
      </div>
      <label class="inline-flex shrink-0 items-center gap-2 text-[13px] text-text-2">
        {{ t('school.scale') }}
        <select v-if="isOwner && !workspace?.archived_at" class="input !w-auto !py-1 text-[13.5px]" :value="scale" @change="setScale(Number(($event.target as HTMLSelectElement).value))">
          <option v-for="n in [20, 100, 10, 5]" :key="n" :value="n">{{ n }}</option>
        </select>
        <b v-else class="text-text">{{ scale }}</b>
      </label>
    </header>
    <p class="footnote mb-6 flex items-center gap-1.5"><Lock class="h-3 w-3" />{{ t('school.private') }}</p>

    <template v-if="courses.length">
      <div v-if="overall != null" class="overall card mb-6">
        <span class="text-[14px] text-text-2">{{ t('school.overall') }}</span>
        <span class="big" :class="{ low: !passes(overall) }" data-testid="overall">{{ mark(overall) }}</span>
        <span class="text-[14px] text-text-3">/ {{ scale }}</span>
      </div>

      <section v-for="c in courses" :key="c.id" class="course card mb-4" :style="{ '--c': c.color || 'var(--accent)' }" :data-testid="`grades-${c.name}`">
        <header class="course-head">
          <span class="dot" />
          <h2 class="min-w-0 flex-1 truncate text-[16px] font-semibold">{{ c.name }}<span v-if="c.weight !== 1" class="ml-1.5 text-[12px] font-normal text-text-3">{{ t('school.weightOf', { w: c.weight }) }}</span></h2>
          <label class="inline-flex items-center gap-1.5 text-[12.5px] text-text-2">
            {{ t('school.goal') }}
            <input
              v-model="goals[c.id]"
              class="input !w-16 !py-0.5 text-right text-[13.5px]"
              inputmode="decimal"
              :placeholder="'–'"
              :disabled="!canWrite"
              @blur="saveGoal(c)"
              @keydown.enter="($event.target as HTMLInputElement).blur()"
            />
          </label>
          <span class="avg" :class="{ low: avgOf(c.id) != null && !passes(avgOf(c.id)!) }" :title="t('school.average')">{{ avgOf(c.id) == null ? '–' : mark(avgOf(c.id)!) }}</span>
        </header>

        <div class="rows">
          <template v-for="g in gradesOf(c.id)" :key="g.id">
            <form v-if="editing === g.id" class="row edit" @submit.prevent="saveEdit" @keydown.esc="editing = ''">
              <input v-model="edit.title" class="input min-w-0 flex-1 !py-1 text-[13.5px]" maxlength="200" :aria-label="t('school.gradeTitle')" />
              <input v-model="edit.date" type="date" class="input !w-auto !py-1 text-[13px]" :aria-label="t('school.date')" />
              <input v-model="edit.score" class="input !w-16 !py-1 text-right text-[13.5px]" inputmode="decimal" :aria-label="t('school.score')" />
              <span class="text-text-3">/</span>
              <input v-model="edit.out_of" class="input !w-16 !py-1 text-right text-[13.5px]" inputmode="decimal" :aria-label="t('school.outOf')" />
              <span class="text-text-3">×</span>
              <input v-model="edit.weight" class="input !w-14 !py-1 text-right text-[13.5px]" inputmode="decimal" :aria-label="t('school.gradeWeight')" :title="t('school.gradeWeight')" />
              <button type="submit" class="icon-btn h-7 w-7" :aria-label="t('common.save')"><Check class="h-4 w-4" /></button>
            </form>
            <div v-else class="row" :class="{ clickable: canWrite }" @click="startEdit(g)">
              <span class="min-w-0 flex-1 truncate text-[14px]">{{ g.title }}</span>
              <span class="text-[12.5px] text-text-3">{{ dateLabel(g.date) }}</span>
              <span v-if="g.weight !== 1" class="text-[12px] text-text-3">{{ t('school.weightOf', { w: g.weight }) }}</span>
              <span class="score" :class="{ low: g.score / g.out_of < 0.5 }">{{ fmtMark(g.score, locale()) }}<small>/{{ fmtMark(g.out_of, locale()) }}</small></span>
              <button v-if="canWrite" class="icon-btn h-6 w-6" :title="t('school.removeGrade')" :aria-label="t('school.removeGrade')" @click.stop="removeGrade(g)"><X class="h-3.5 w-3.5" /></button>
            </div>
          </template>
          <p v-if="!gradesOf(c.id).length" class="row text-[13.5px] text-text-3">{{ t('school.noGrades') }}</p>

          <form v-if="canWrite" class="row add" @submit.prevent="addGrade(c)">
            <input v-model="draftOf(c.id).title" class="input min-w-0 flex-1 !py-1 text-[13.5px]" :placeholder="t('school.gradeTitlePh')" maxlength="200" :aria-label="t('school.gradeTitle')" />
            <input v-model="draftOf(c.id).date" type="date" class="input !w-auto !py-1 text-[13px]" :aria-label="t('school.date')" />
            <input v-model="draftOf(c.id).score" class="input !w-16 !py-1 text-right text-[13.5px]" inputmode="decimal" :placeholder="t('school.score')" :aria-label="t('school.score')" />
            <span class="text-text-3">/</span>
            <input v-model="draftOf(c.id).out_of" class="input !w-16 !py-1 text-right text-[13.5px]" inputmode="decimal" :aria-label="t('school.outOf')" />
            <span class="text-text-3">×</span>
              <input v-model="draftOf(c.id).weight" class="input !w-14 !py-1 text-right text-[13.5px]" inputmode="decimal" :aria-label="t('school.gradeWeight')" :title="t('school.gradeWeight')" />
            <button type="submit" class="btn btn-sm" :disabled="!validDraft(draftOf(c.id))"><Plus class="h-3.5 w-3.5" />{{ t('school.addGrade') }}</button>
          </form>
        </div>

        <footer class="need">
          <b class="text-[13px]">{{ t('school.need') }}</b>
          <label class="inline-flex items-center gap-1.5 text-[12.5px] text-text-2">
            {{ t('school.needWeight') }}
            <input v-model.number="needW[c.id]" type="number" min="0.1" step="0.5" class="input !w-16 !py-0.5 text-right text-[13px]" />
          </label>
          <span class="text-[13px] text-text-2" data-testid="need">{{ need(c) }}</span>
        </footer>
      </section>
    </template>

    <div v-else-if="loaded" class="py-16 text-center">
      <Award class="mx-auto mb-3 h-10 w-10 text-text-faint" :stroke-width="1.5" />
      <p class="title-3">{{ t('school.noGrades') }}</p>
      <template v-if="canEditCourses">
        <p class="subhead mx-auto mt-1 max-w-sm">{{ t('school.noCourseYet') }}</p>
        <form class="mx-auto mt-4 flex max-w-xs gap-2" @submit.prevent="addCourse">
          <input v-model="newCourse" class="input flex-1" :placeholder="t('school.namePh')" maxlength="120" />
          <button class="btn-primary" :disabled="!newCourse.trim()"><Plus class="h-4 w-4" /></button>
        </form>
      </template>
    </div>
  </div>
</template>

<style scoped>
.grades {
  padding: 32px clamp(16px, 4vw, 48px) 64px;
  max-width: 880px;
}
.card {
  background: var(--surface);
  border-radius: 14px;
  box-shadow: var(--shadow-sm), 0 0 0 0.5px var(--border);
}
.overall {
  display: flex;
  align-items: baseline;
  gap: 10px;
  padding: 16px 20px;
}
.big {
  font: 700 34px var(--font-display);
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.02em;
}
.low {
  color: var(--crit-text);
}
.course {
  overflow: hidden;
}
.course-head {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  border-bottom: 0.5px solid var(--border);
}
.dot {
  width: 10px;
  height: 26px;
  flex: none;
  border-radius: 999px;
  background: var(--c);
}
.avg {
  flex: none;
  min-width: 52px;
  padding: 3px 8px;
  border-radius: 10px;
  background: var(--fill-2);
  font: 700 18px var(--font-display);
  font-variant-numeric: tabular-nums;
  text-align: center;
}
.rows {
  padding: 4px 8px;
}
.row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 38px;
  padding: 4px 8px;
  border-radius: 9px;
}
.row.clickable {
  cursor: pointer;
}
.row.clickable:hover {
  background: var(--fill-2);
}
.row.add,
.row.edit {
  flex-wrap: wrap;
}
.score {
  min-width: 56px;
  text-align: right;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.score small {
  font-weight: 400;
  color: var(--text-3);
}
.need {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 14px;
  padding: 10px 16px 12px;
  border-top: 0.5px solid var(--border);
  background: var(--fill-2);
}
</style>
