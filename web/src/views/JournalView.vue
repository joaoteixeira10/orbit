<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { ChevronLeft, ChevronRight, Check, LoaderCircle, Pencil } from 'lucide-vue-next'
import MarkdownEditor from '@/components/editor/MarkdownEditor.vue'
import MarkdownView from '@/components/editor/MarkdownView.vue'
import { journalApi, MOODS, moodFace, type JournalDay } from '@/lib/journal'
import { addMonths, dateStr, fmtDate, monthGrid, monthTitle, parseDate, todayStr, weekdayNames } from '@/lib/dates'
import { toggleTask } from '@/lib/markdown'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

// The journal: a month with each day's mood on the left; the chosen day on
// the right — how it went, what happened (Markdown, saved as you write) —
// and the same day in earlier years. Yours alone.
const route = useRoute()
const router = useRouter()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws))
const workspace = computed(() => wss.byId(ws.value))
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only)

const date = computed(() => (typeof route.query.date === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(route.query.date) ? route.query.date : todayStr()))
const month = ref(parseDate(date.value))
const days = ref<Record<string, JournalDay>>({})
async function loadMonth() {
  const m = dateStr(month.value).slice(0, 7)
  try {
    const list = await journalApi.month(ws.value, m)
    days.value = Object.fromEntries(list.map((d) => [d.date, d]))
  } catch (e: any) {
    toast.error(e.message)
  }
}
watch([ws, month], loadMonth, { immediate: true })
const grid = computed(() => monthGrid(month.value))
const names = computed(() => weekdayNames('narrow'))
function pick(d: Date) {
  void flush()
  void router.replace({ query: { ...route.query, date: dateStr(d) } })
}

// The day.
const content = ref('')
const mood = ref(0)
const past = ref<JournalDay[]>([])
const editing = ref(false)
const state = ref<'' | 'saving' | 'saved'>('')
const editor = ref<InstanceType<typeof MarkdownEditor> | null>(null)
let timer: ReturnType<typeof setTimeout> | undefined
let loadedFor = ''
async function loadDay() {
  const d = date.value
  loadedFor = ''
  try {
    const r = await journalApi.day(ws.value, d)
    if (d !== date.value) return
    content.value = r.entry?.content ?? ''
    mood.value = r.entry?.mood ?? 0
    past.value = r.on_this_day
    editing.value = false
    state.value = ''
    loadedFor = d
    if (dateStr(month.value).slice(0, 7) !== d.slice(0, 7)) month.value = parseDate(d)
  } catch (e: any) {
    toast.error(e.message)
  }
}
watch([ws, date], loadDay, { immediate: true })

function changed() {
  if (!editable.value || !loadedFor) return
  editing.value = true // stays open while writing (the first save fills it)
  state.value = 'saving'
  clearTimeout(timer)
  timer = setTimeout(() => void save(), 800)
}
async function save() {
  clearTimeout(timer)
  timer = undefined
  const d = loadedFor
  if (!d || !editable.value) return
  if (editor.value) content.value = editor.value.getContent()
  try {
    const r = await journalApi.put(ws.value, d, { content: content.value, mood: mood.value })
    state.value = 'saved'
    const text = r.entry?.content ?? ''
    if (r.entry) days.value[d] = { date: d, mood: r.entry.mood, excerpt: text.slice(0, 90) }
    else delete days.value[d]
  } catch (e: any) {
    state.value = ''
    toast.error(e.message)
  }
}
async function flush() {
  if (timer) await save()
}
onBeforeUnmount(() => void flush())
function setMood(m: number) {
  mood.value = mood.value === m ? 0 : m
  changed()
  void save()
}
function tick(i: number) {
  content.value = toggleTask(content.value, i)
  changed()
}

// An empty day opens ready to write (once it's known the person may);
// a written one to read, until Write.
const writing = computed(() => editable.value && (editing.value || !content.value.trim()))

const title = computed(() => {
  const d = parseDate(date.value)
  const s = fmtDate(d, { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' })
  return s.charAt(0).toUpperCase() + s.slice(1)
})
const yearsAgo = (d: string) => Number(date.value.slice(0, 4)) - Number(d.slice(0, 4))
</script>

<template>
  <div class="journal-view">
    <header class="mb-6">
      <h1 class="large-title">{{ t('modules.journal') }}</h1>
      <p class="subhead mt-0.5">{{ workspace?.name }}</p>
    </header>

    <div class="grid gap-6 lg:grid-cols-[300px_minmax(0,1fr)]">
      <!-- The month -->
      <aside class="card self-start p-3">
        <div class="mb-2 flex items-center">
          <span class="flex-1 pl-1 text-[14px] font-semibold">{{ monthTitle(month) }}</span>
          <button class="icon-btn" :aria-label="t('journal.prev')" @click="month = addMonths(month, -1)"><ChevronLeft class="h-4 w-4" /></button>
          <button class="icon-btn" :aria-label="t('journal.next')" @click="month = addMonths(month, 1)"><ChevronRight class="h-4 w-4" /></button>
        </div>
        <div class="days">
          <span v-for="(n, i) in names" :key="'h' + i" class="wd">{{ n }}</span>
          <button
            v-for="d in grid"
            :key="dateStr(d)"
            class="day"
            :class="{ out: d.getMonth() !== month.getMonth(), sel: dateStr(d) === date, today: dateStr(d) === todayStr(), has: !!days[dateStr(d)] }"
            :title="days[dateStr(d)]?.excerpt"
            :data-day="dateStr(d)"
            @click="pick(d)"
          >
            <span>{{ d.getDate() }}</span>
            <i>{{ moodFace(days[dateStr(d)]?.mood ?? 0) || (days[dateStr(d)] ? '•' : '') }}</i>
          </button>
        </div>
      </aside>

      <!-- The day -->
      <section class="min-w-0">
        <div class="flex flex-wrap items-center gap-3">
          <h2 class="title-2 flex-1">{{ title }}</h2>
          <span v-if="state" class="flex items-center gap-1 text-[12.5px] text-text-3" data-testid="journal-state">
            <LoaderCircle v-if="state === 'saving'" class="h-3.5 w-3.5 animate-spin" /><Check v-else class="h-3.5 w-3.5" />
            {{ state === 'saving' ? t('journal.saving') : t('journal.saved') }}
          </span>
        </div>
        <div class="mt-3 flex items-center gap-1.5" role="radiogroup" :aria-label="t('journal.mood')">
          <button
            v-for="(f, i) in MOODS"
            :key="i"
            class="mood"
            :class="{ on: mood === i + 1 }"
            role="radio"
            :aria-checked="mood === i + 1"
            :aria-label="t(`journal.moods.m${i + 1}`)"
            :title="t(`journal.moods.m${i + 1}`)"
            :disabled="!editable"
            @click="setMood(i + 1)"
          >{{ f }}</button>
        </div>

        <div class="card mt-4 min-h-[240px] p-4" data-testid="journal-entry">
          <MarkdownEditor v-if="writing" :key="date" ref="editor" :content="content" :editable="editable" @change="changed" />
          <template v-else>
            <MarkdownView v-if="content.trim()" :content="content" :editable="editable" @toggle="tick" @edit="editing = true" />
            <p v-else class="text-[14px] text-text-3">{{ t('journal.empty') }}</p>
            <button v-if="editable" class="btn-plain mt-3 !text-[13px]" @click="editing = true"><Pencil class="h-3.5 w-3.5" />{{ t('journal.edit') }}</button>
          </template>
        </div>
        <p v-if="writing && !content" class="mt-2 px-1 text-[12.5px] text-text-3">{{ t('journal.write') }}</p>

        <div v-if="past.length" class="mt-8">
          <h3 class="cap mb-2 px-1">{{ t('journal.onThisDay') }}</h3>
          <div class="space-y-2">
            <button v-for="p in past" :key="p.date" class="card w-full p-3.5 text-left" @click="router.replace({ query: { ...route.query, date: p.date } })">
              <div class="flex items-center gap-2 text-[12.5px] text-text-3">
                <span class="text-[16px]">{{ moodFace(p.mood) }}</span>
                {{ t('journal.yearsAgo', { n: yearsAgo(p.date) }, yearsAgo(p.date)) }} · {{ fmtDate(parseDate(p.date), { year: 'numeric' }) }}
              </div>
              <p class="mt-1 line-clamp-3 text-[14px]">{{ p.excerpt }}</p>
            </button>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.journal-view {
  padding: 32px clamp(16px, 4vw, 48px) 96px;
  max-width: 1100px;
}
.days {
  display: grid;
  grid-template-columns: repeat(7, minmax(0, 1fr));
  gap: 2px;
}
.wd {
  padding-bottom: 4px;
  text-align: center;
  font-size: 11px;
  font-weight: 600;
  color: var(--text-3);
}
.day {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1px;
  height: 42px;
  padding-top: 4px;
  border-radius: 9px;
  font-size: 13px;
  font-variant-numeric: tabular-nums;
}
.day i {
  font-style: normal;
  font-size: 13px;
  line-height: 1;
  color: var(--accent);
}
.day:hover {
  background: var(--fill);
}
.day.out {
  color: var(--text-faint);
}
.day.today span {
  color: var(--crit);
  font-weight: 700;
}
.day.sel {
  background: var(--accent-soft);
  font-weight: 600;
}
.mood {
  display: grid;
  place-items: center;
  width: 42px;
  height: 42px;
  border-radius: 12px;
  background: var(--fill);
  font-size: 22px;
  filter: grayscale(0.85);
  opacity: 0.7;
  transition: transform 0.12s var(--spring), filter 0.12s, opacity 0.12s;
}
.mood:hover:not(:disabled) {
  opacity: 1;
  filter: none;
}
.mood.on {
  background: var(--accent-soft);
  filter: none;
  opacity: 1;
  transform: scale(1.08);
}
</style>
