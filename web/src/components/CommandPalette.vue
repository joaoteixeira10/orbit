<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Search, FileText, CheckCircle2, Circle, CalendarDays, Plus, Settings, Moon, Sun, House, CornerDownLeft, StickyNote, NotebookPen, Brush, Bookmark } from 'lucide-vue-next'
import WsIcon from './ui/WsIcon.vue'
import { plannerApi, pagesApi, type SearchHit } from '@/api/planner'
import { useWorkspacesStore } from '@/stores/workspaces'
import { usePagesStore } from '@/stores/pages'
import { usePrefsStore } from '@/stores/prefs'
import { useToastStore } from '@/stores/toast'
import { relDay, fmtDate, parseDate } from '@/lib/dates'
import { drawingsApi } from '@/components/draw/drawing'

// ⌘K / Ctrl+K: search pages, tasks, notes and events in every workspace, jump to a
// workspace, or run a quick action. Arrows move, Enter opens, Esc closes.
const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: []; newWorkspace: []; newTask: [ws: string]; quickNote: [] }>()
const router = useRouter()
const route = useRoute()
const wss = useWorkspacesStore()
const pages = usePagesStore()
const prefs = usePrefsStore()
const toast = useToastStore()
const { t } = useI18n()

const q = ref('')
const hits = ref<SearchHit[]>([])
const sel = ref(0)
const input = ref<HTMLInputElement | null>(null)
let seq = 0

watch(
  () => props.open,
  async (o) => {
    if (!o) return
    q.value = ''
    hits.value = []
    sel.value = 0
    await nextTick()
    input.value?.focus()
  },
)
let timer: ReturnType<typeof setTimeout> | null = null
watch(q, (v) => {
  sel.value = 0
  if (timer) clearTimeout(timer)
  if (v.trim().length < 2) {
    hits.value = []
    return
  }
  const my = ++seq
  timer = setTimeout(async () => {
    try {
      const r = await plannerApi.search(v.trim())
      if (my === seq) hits.value = r
    } catch {
      /* keep the last results */
    }
  }, 160)
})

const currentWs = computed(() => (typeof route.params.ws === 'string' ? route.params.ws : ''))

interface Item {
  key: string
  label: string
  sub?: string
  icon?: unknown
  ws?: string
  done?: boolean
  run: () => void
}
const actions = computed<Item[]>(() => {
  const ql = q.value.trim().toLowerCase()
  const out: Item[] = [
    { key: 'a-home', label: t('nav.today'), icon: House, run: () => router.push({ name: 'home' }) },
    { key: 'a-cal', label: t('nav.calendar'), icon: CalendarDays, run: () => router.push({ name: 'calendar' }) },
  ]
  if (currentWs.value) {
    out.push({ key: 'a-page', label: t('cmd.newPage'), icon: Plus, run: newPage })
    out.push({ key: 'a-task', label: t('cmd.newTask'), icon: Plus, run: () => emit('newTask', currentWs.value) })
    if (wss.byId(currentWs.value)?.modules.includes('notes'))
      out.push({ key: 'a-note', label: t('cmd.newNote'), icon: Plus, run: () => router.push({ name: 'ws-notes', params: { ws: currentWs.value }, query: { new: '1' } }) })
  }
  out.push({ key: 'a-quick', label: t('notes.mine.action'), sub: '⌥N', icon: NotebookPen, run: () => emit('quickNote') })
  out.push({ key: 'a-draw', label: t('draw.newSketch'), sub: t('draw.quick'), icon: Brush, run: () => void newSketch() })
  out.push({ key: 'a-ws', label: t('ws.new'), icon: Plus, run: () => emit('newWorkspace') })
  out.push({ key: 'a-theme', label: prefs.isDark() ? t('cmd.lightMode') : t('cmd.darkMode'), icon: prefs.isDark() ? Sun : Moon, run: () => prefs.setTheme(prefs.isDark() ? 'light' : 'dark') })
  out.push({ key: 'a-settings', label: t('nav.settings'), icon: Settings, run: () => router.push({ name: 'settings' }) })
  for (const w of wss.active) out.push({ key: 'w-' + w.id, label: w.name, sub: t('cmd.workspace'), ws: w.id, run: () => router.push({ name: 'ws', params: { ws: w.id } }) })
  return ql ? out.filter((i) => i.label.toLowerCase().includes(ql)) : out
})
const results = computed<Item[]>(() =>
  hits.value.map((h) => {
    const w = wss.byId(h.workspace_id)
    const where = w?.name ?? ''
    if (h.kind === 'page')
      return { key: 'p-' + h.id, label: h.title || t('pages.untitled'), sub: h.snippet ? `${where} · ${h.snippet}` : where, icon: h.icon || FileText, run: () => router.push({ name: 'ws-page', params: { ws: h.workspace_id, page: h.id } }) }
    if (h.kind === 'task')
      return { key: 't-' + h.id, label: h.title, sub: [where, h.due ? relDay(h.due) : ''].filter(Boolean).join(' · '), icon: h.done ? CheckCircle2 : Circle, done: h.done, run: () => router.push({ name: 'ws-tasks', params: { ws: h.workspace_id, list: h.list_id }, query: { task: h.id } }) }
    if (h.kind === 'note') {
      // A personal note (no workspace) is in Quick Notes.
      const at = h.workspace_id ? where : t('notes.mine.title')
      const to = h.workspace_id ? { name: 'ws-notes', params: { ws: h.workspace_id }, query: { note: h.id } } : { name: 'my-notes', query: { note: h.id } }
      return { key: 'n-' + h.id, label: h.title || h.snippet || '', sub: h.title && h.snippet ? `${at} · ${h.snippet}` : at, icon: StickyNote, run: () => router.push(to) }
    }
    if (h.kind === 'bookmark')
      return { key: 'b-' + h.id, label: h.title, sub: h.snippet ? `${where} · ${h.snippet}` : where, icon: Bookmark, run: () => window.open(h.snippet || '', '_blank', 'noopener,noreferrer') }
    if (h.kind === 'journal')
      return { key: 'j-' + h.id, label: fmtDate(parseDate(h.id), { weekday: 'short', day: 'numeric', month: 'long', year: 'numeric' }), sub: h.snippet ? `${where} · ${h.snippet}` : where, icon: NotebookPen, run: () => router.push({ name: 'ws-journal', params: { ws: h.workspace_id }, query: { date: h.id } }) }
    if (h.kind === 'drawing') {
      // No workspace: a Quick Draw sketch.
      const at = h.workspace_id ? where : t('draw.quick')
      const to = h.workspace_id ? { name: 'ws-whiteboard', params: { ws: h.workspace_id, id: h.id } } : { name: 'my-drawing', params: { id: h.id } }
      return { key: 'd-' + h.id, label: h.title || t('draw.untitled'), sub: h.snippet ? `${at} · ${h.snippet}` : at, icon: Brush, run: () => router.push(to) }
    }
    return { key: 'e-' + h.id, label: h.title, sub: [where, h.start ? fmtDate(new Date(h.start), { day: 'numeric', month: 'short', year: 'numeric' }) : ''].filter(Boolean).join(' · '), icon: CalendarDays, run: () => router.push({ name: 'ws-calendar', params: { ws: h.workspace_id }, query: { date: h.start?.slice(0, 10) } }) }
  }),
)
const all = computed(() => [...results.value, ...actions.value])

// A new Quick Draw sketch, open.
async function newSketch() {
  try {
    const d = await drawingsApi.create('me', '')
    router.push({ name: 'my-drawing', params: { id: d.id } })
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function newPage() {
  try {
    const p = await pagesApi.create(currentWs.value, { title: '' })
    await pages.load(currentWs.value)
    router.push({ name: 'ws-page', params: { ws: currentWs.value, page: p.id } })
  } catch (e: any) {
    toast.error(e.message)
  }
}

function choose(i: Item) {
  emit('close')
  i.run()
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    sel.value = Math.min(all.value.length - 1, sel.value + 1)
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    sel.value = Math.max(0, sel.value - 1)
  } else if (e.key === 'Enter') {
    e.preventDefault()
    const i = all.value[sel.value]
    if (i) choose(i)
  } else if (e.key === 'Escape') {
    emit('close')
  }
}
watch(sel, async () => {
  await nextTick()
  document.querySelector('.cmd-item.on')?.scrollIntoView({ block: 'nearest' })
})
</script>

<template>
  <Teleport to="body">
    <Transition name="sheet">
      <div v-if="open" class="fixed inset-0 z-50 flex items-start justify-center px-4 pt-[12vh]" style="background: var(--backdrop)" @mousedown.self="emit('close')">
        <div class="sheet-panel cmd w-full max-w-[600px] overflow-hidden rounded-2xl shadow-lg2" role="dialog" aria-modal="true" :aria-label="t('cmd.title')">
          <div class="flex items-center gap-3 px-4 py-3.5">
            <Search class="h-5 w-5 text-text-3" />
            <input ref="input" v-model="q" class="w-full bg-transparent text-[18px] outline-none placeholder:text-text-faint" :placeholder="t('cmd.placeholder')" @keydown="onKey" />
            <kbd class="rounded-md bg-fill px-1.5 py-0.5 text-[11px] text-text-3">esc</kbd>
          </div>
          <div class="max-h-[50vh] overflow-y-auto border-t-[0.5px] border-border p-2">
            <div v-if="results.length" class="cap px-2.5 pb-1 pt-1.5">{{ t('cmd.results') }}</div>
            <button
              v-for="(i, n) in all"
              :key="i.key"
              class="cmd-item flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left"
              :class="{ on: n === sel }"
              @mouseenter="sel = n"
              @click="choose(i)"
            >
              <WsIcon v-if="i.ws" :icon="wss.byId(i.ws)?.icon" :color="wss.byId(i.ws)?.color" :size="22" :name="i.label" />
              <span v-else-if="typeof i.icon === 'string'" class="grid h-[22px] w-[22px] place-items-center text-[15px]">{{ i.icon }}</span>
              <component :is="i.icon" v-else class="h-[18px] w-[22px] shrink-0 text-text-2" :class="{ 'text-ok': i.done }" />
              <span class="min-w-0 flex-1">
                <span class="block truncate text-[14px]" :class="{ 'text-text-3 line-through': i.done }">{{ i.label }}</span>
                <span v-if="i.sub" class="block truncate text-[12px] text-text-3">{{ i.sub }}</span>
              </span>
              <CornerDownLeft v-if="n === sel" class="h-4 w-4 shrink-0 text-text-3" />
            </button>
            <div v-if="q.trim().length >= 2 && !all.length" class="py-8 text-center text-[13px] text-text-3">{{ t('cmd.nothing') }}</div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.cmd {
  background: color-mix(in srgb, var(--surface) 88%, transparent);
  backdrop-filter: saturate(180%) blur(30px);
  -webkit-backdrop-filter: saturate(180%) blur(30px);
}
.cmd-item.on {
  background: var(--accent);
  color: #fff;
}
.cmd-item.on span,
.cmd-item.on svg {
  color: #fff !important;
}
</style>
