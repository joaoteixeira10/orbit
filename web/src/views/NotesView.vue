<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Search, StickyNote, X } from 'lucide-vue-next'
import NoteCapture from '@/components/notes/NoteCapture.vue'
import NoteCard from '@/components/notes/NoteCard.vue'
import NoteEditor from '@/components/notes/NoteEditor.vue'
import { ensureNoteColorStyles, NOTE_COLORS } from '@/components/notes/noteColors'
import { convertedInput, matches, noteText, pageDoc, taskTitles } from '@/components/notes/noteModel'
import { useNotesStore } from '@/stores/notes'
import { usePagesStore } from '@/stores/pages'
import { useTasksStore } from '@/stores/tasks'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { useSortable, type SortMove } from '@/lib/sortable'
import { pagesApi, type Note, type NoteColor, type NoteInput } from '@/api/planner'

// Quick notes: a wall of sticky notes (Keep-like), pinned ones first. The
// box on top takes a new note; a card opens in an editor (?note=<id> deep
// links there, ?new=1 opens the box). The wall is a masonry grid that reads
// left to right — each card spans as many 8px rows as it is tall — so drag
// and drop (editors) reorders it in reading order.
ensureNoteColorStyles()
const route = useRoute()
const router = useRouter()
const notes = useNotesStore()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws))
const workspace = computed(() => wss.byId(ws.value))
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only)
const hasModule = (m: string) => !workspace.value?.modules?.length || workspace.value.modules.includes(m)
const canTasks = computed(() => editable.value && hasModule('tasks'))

const loaded = ref(false)
async function load() {
  loaded.value = false
  try {
    await notes.load(ws.value)
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loaded.value = true
  }
}
watch(ws, load)
onMounted(load)

// ---- Filters ----
const q = ref('')
const colorFilter = ref<NoteColor | null>(null)
const all = computed(() => notes.notesOf(ws.value))
const shown = computed(() => all.value.filter((n) => matches(n, q.value) && (colorFilter.value === null || n.color === colorFilter.value)))
const pinned = computed(() => shown.value.filter((n) => n.pinned))
const others = computed(() => shown.value.filter((n) => !n.pinned))
const usedColors = computed(() => NOTE_COLORS.filter((c) => all.value.some((n) => n.color === c)))
const filtering = computed(() => !!q.value.trim() || colorFilter.value !== null)
watch(ws, () => ((q.value = ''), (colorFilter.value = null)))
const colorName = (c: NoteColor) => t(`notes.colors.${c || 'plain'}`)

// ---- Masonry: each cell spans ceil((height + gap) / row) grid rows ----
const ROW = 8
const GAP = 14
const spans = reactive<Record<string, number>>({})
const cells = new Map<string, Element>()
const ro =
  typeof ResizeObserver === 'undefined'
    ? null
    : new ResizeObserver((entries) => {
        for (const e of entries) {
          const id = (e.target as HTMLElement).dataset.cell ?? ''
          const h = e.borderBoxSize?.[0]?.blockSize ?? (e.target as HTMLElement).offsetHeight
          const span = Math.ceil((h + GAP) / ROW)
          if (id && spans[id] !== span) spans[id] = span
        }
      })
function observe(id: string, el: unknown) {
  const prev = cells.get(id)
  if (prev === el) return
  if (prev) ro?.unobserve(prev)
  if (el instanceof Element) {
    cells.set(id, el)
    ro?.observe(el)
  } else cells.delete(id)
}
onBeforeUnmount(() => ro?.disconnect())

// ---- Drag to reorder (not while filtering) ----
const pinnedEl = ref<HTMLElement | null>(null)
const othersEl = ref<HTMLElement | null>(null)
const canDrag = computed(() => editable.value && !filtering.value)
const sortOpts = {
  group: 'notes',
  draggable: '.cell',
  filter: 'button, input, textarea',
  preventOnFilter: false,
  forceFallback: true,
  fallbackOnBody: true,
  fallbackTolerance: 4,
  delay: 220,
  delayOnTouchOnly: true,
  touchStartThreshold: 6,
}
useSortable(computed(() => (canDrag.value ? pinnedEl.value : null)), onDrop, sortOpts)
useSortable(computed(() => (canDrag.value ? othersEl.value : null)), onDrop, sortOpts)
async function onDrop(m: SortMove) {
  const lists: Record<string, string[]> = {
    pinned: pinned.value.map((n) => n.id).filter((id) => id !== m.id),
    others: others.value.map((n) => n.id).filter((id) => id !== m.id),
  }
  const target = lists[m.toGroup]
  if (!target) return
  target.splice(m.beforeId ? target.indexOf(m.beforeId) + 1 : 0, 0, m.id)
  try {
    if (m.fromGroup !== m.toGroup) await notes.update(ws.value, m.id, { pinned: m.toGroup === 'pinned' })
    await notes.reorder(ws.value, [...lists.pinned, ...lists.others])
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ---- Actions ----
const capture = ref<InstanceType<typeof NoteCapture> | null>(null)
async function create(body: NoteInput) {
  try {
    await notes.create(ws.value, body)
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function save(n: Note, body: NoteInput) {
  try {
    await notes.update(ws.value, n.id, body)
  } catch (e: any) {
    toast.error(e.message)
  }
}
const togglePin = (n: Note) => save(n, { pinned: !n.pinned })
const setColor = (n: Note, color: NoteColor) => save(n, { color })
const convert = (n: Note) => save(n, convertedInput(n))
const toggleItem = (n: Note, id: string) => save(n, { items: n.items.map((i) => (i.id === id ? { ...i, done: !i.done } : i)) })

async function copy(n: Note) {
  try {
    await navigator.clipboard.writeText(noteText(n))
    toast.success(t('notes.copied'))
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function toPage(n: Note) {
  try {
    const p = await pagesApi.create(ws.value, { title: n.title, content: pageDoc(n) })
    await usePagesStore().load(ws.value)
    router.push({ name: 'ws-page', params: { ws: ws.value, page: p.id } })
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function toTasks(n: Note) {
  const titles = taskTitles(n)
  if (!titles.length) return toast.info(t('notes.noTasks'))
  const tasks = useTasksStore()
  let done = 0
  try {
    for (const title of titles) {
      await tasks.create(ws.value, { title })
      done++
    }
  } catch (e: any) {
    toast.error(e.message)
  }
  if (done) toast.success(t('notes.tasksAdded', done))
}

// Delete at once, with an Undo snackbar that puts it back where it was.
const undo = ref<{ note: Note; order: string[] } | null>(null)
let undoTimer: ReturnType<typeof setTimeout> | undefined
async function remove(n: Note) {
  const order = all.value.map((x) => x.id)
  try {
    const gone = await notes.remove(ws.value, n.id)
    if (!gone) return
    undo.value = { note: gone, order }
    clearTimeout(undoTimer)
    undoTimer = setTimeout(() => (undo.value = null), 6000)
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function restore() {
  const u = undo.value
  if (!u) return
  undo.value = null
  clearTimeout(undoTimer)
  const { title, body, checklist, items, color, pinned: pin } = u.note
  try {
    const back = await notes.create(ws.value, { title, body, checklist, items, color, pinned: pin })
    const have = new Set(all.value.map((x) => x.id))
    const ids = u.order.map((id) => (id === u.note.id ? back.id : id)).filter((id) => have.has(id))
    await notes.reorder(ws.value, ids)
  } catch (e: any) {
    toast.error(e.message)
  }
}
onBeforeUnmount(() => clearTimeout(undoTimer))

// ---- The editor follows ?note=<id>; ?new=1 opens the capture box ----
const openId = computed(() => (typeof route.query.note === 'string' ? route.query.note : ''))
function openNote(n: Note) {
  router.replace({ query: { ...route.query, note: n.id } })
}
function closeEditor() {
  const { note: _note, ...rest } = route.query
  router.replace({ query: rest })
}
watch(
  () => [route.query.new, editable.value] as const,
  async ([v, ok]) => {
    if (v !== '1' || !ok) return
    const { new: _new, ...rest } = route.query
    await router.replace({ query: rest })
    await nextTick()
    void capture.value?.start()
  },
  { immediate: true },
)
</script>

<template>
  <div class="notes-view">
    <header class="mb-6 flex flex-wrap items-end gap-x-4 gap-y-3">
      <div class="min-w-0 flex-1">
        <h1 class="large-title">{{ t('modules.notes') }}</h1>
        <p class="subhead mt-0.5">{{ workspace?.name }} · {{ all.length }}</p>
      </div>
      <div v-if="all.length" class="flex w-full flex-wrap items-center gap-3 sm:w-auto">
        <div v-if="usedColors.length > 1" class="dots" role="radiogroup" :aria-label="t('notes.filterColor')">
          <button
            v-for="c in usedColors"
            :key="c || 'plain'"
            class="dot"
            :class="{ on: colorFilter === c }"
            :data-note="c"
            role="radio"
            :aria-checked="colorFilter === c"
            :aria-label="colorName(c)"
            :title="colorName(c)"
            @click="colorFilter = colorFilter === c ? null : c"
          />
          <button v-if="colorFilter !== null" class="dot-clear" :aria-label="t('notes.allColors')" :title="t('notes.allColors')" @click="colorFilter = null">
            <X class="h-3 w-3" :stroke-width="2.6" />
          </button>
        </div>
        <label class="search h-9 min-w-[200px] flex-1 sm:w-64 sm:flex-none">
          <Search class="h-4 w-4 shrink-0" />
          <input v-model="q" type="search" :placeholder="t('notes.search')" />
        </label>
      </div>
    </header>

    <NoteCapture v-if="editable" ref="capture" class="mb-9" @create="create" />

    <template v-if="shown.length">
      <section v-if="pinned.length" class="mb-4">
        <h2 class="cap section-label">{{ t('notes.pinned') }}</h2>
        <div ref="pinnedEl" class="wall" data-group="pinned">
          <div v-for="n in pinned" :key="n.id" class="cell" :data-id="n.id" :style="{ gridRowEnd: `span ${spans[n.id] ?? 18}` }">
            <div :ref="(el) => observe(n.id, el)" :data-cell="n.id">
              <NoteCard
                :note="n"
                :editable="editable"
                :can-tasks="canTasks"
                @open="openNote(n)"
                @toggle-item="(id) => toggleItem(n, id)"
                @pin="togglePin(n)"
                @color="(c) => setColor(n, c)"
                @convert="convert(n)"
                @page="toPage(n)"
                @tasks="toTasks(n)"
                @copy="copy(n)"
                @remove="remove(n)"
              />
            </div>
          </div>
        </div>
      </section>
      <section v-if="others.length">
        <h2 v-if="pinned.length" class="cap section-label">{{ t('notes.others') }}</h2>
        <div ref="othersEl" class="wall" data-group="others">
          <div v-for="n in others" :key="n.id" class="cell" :data-id="n.id" :style="{ gridRowEnd: `span ${spans[n.id] ?? 18}` }">
            <div :ref="(el) => observe(n.id, el)" :data-cell="n.id">
              <NoteCard
                :note="n"
                :editable="editable"
                :can-tasks="canTasks"
                @open="openNote(n)"
                @toggle-item="(id) => toggleItem(n, id)"
                @pin="togglePin(n)"
                @color="(c) => setColor(n, c)"
                @convert="convert(n)"
                @page="toPage(n)"
                @tasks="toTasks(n)"
                @copy="copy(n)"
                @remove="remove(n)"
              />
            </div>
          </div>
        </div>
      </section>
    </template>

    <div v-else-if="loaded" class="empty">
      <template v-if="filtering">
        <Search class="mx-auto mb-3 h-10 w-10 text-text-faint" :stroke-width="1.5" />
        <p class="title-3">{{ t('notes.noMatch') }}</p>
      </template>
      <template v-else>
        <div class="empty-icon"><StickyNote class="h-9 w-9" :stroke-width="1.6" /></div>
        <p class="title-3">{{ t('notes.empty') }}</p>
        <p class="subhead mx-auto mt-1 max-w-sm">{{ editable ? t('notes.emptyHint') : t('notes.emptyReadOnly') }}</p>
      </template>
    </div>

    <NoteEditor
      :open="!!openId"
      :ws="ws"
      :note-id="openId"
      :editable="editable"
      :can-tasks="canTasks"
      @close="closeEditor"
      @page="toPage"
      @tasks="toTasks"
      @copy="copy"
      @remove="remove"
    />

    <Teleport to="body">
      <Transition name="snack">
        <div v-if="undo" class="snack" role="status">
          <span>{{ t('notes.deleted') }}</span>
          <button class="snack-undo" @click="restore">{{ t('notes.undo') }}</button>
          <button class="snack-x" :aria-label="t('common.close')" @click="undo = null"><X class="h-3.5 w-3.5" :stroke-width="2.6" /></button>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.notes-view {
  padding: 32px clamp(16px, 4vw, 48px) 96px;
}
.section-label {
  margin: 0 0 10px 4px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
}

/* The wall: up to six columns of at least 240px, rows of 8px. */
.wall {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(max(240px, calc((100% - 5 * 14px) / 6)), 1fr));
  grid-auto-rows: 8px;
  column-gap: 14px;
  align-items: start;
}
@media (max-width: 539px) {
  .wall {
    grid-template-columns: 1fr;
  }
}
.cell {
  min-width: 0;
}
.cell.sortable-ghost :deep(.note-card) {
  background: var(--fill-2);
  box-shadow: inset 0 0 0 1.5px var(--border-2);
  outline: 1.5px dashed var(--border-2);
  outline-offset: -1.5px;
}
.cell.sortable-ghost :deep(.note-card > *) {
  visibility: hidden;
}
.cell.sortable-drag,
.cell.sortable-fallback {
  opacity: 1 !important;
  rotate: 2deg;
  scale: 1.02;
  transition: none;
  cursor: grabbing;
}
.cell.sortable-drag :deep(.note-card),
.cell.sortable-fallback :deep(.note-card) {
  box-shadow: 0 0 0 0.5px var(--border), 0 18px 40px rgba(0, 0, 0, 0.18), 0 4px 10px rgba(0, 0, 0, 0.1);
}

/* Colour filter. */
.dots {
  display: flex;
  align-items: center;
  gap: 7px;
}
.dot {
  width: 20px;
  height: 20px;
  border-radius: 999px;
  background: var(--note-bg);
  box-shadow: inset 0 0 0 0.5px var(--border-2);
  transition: transform 0.15s var(--spring), box-shadow 0.15s;
}
.dot:hover {
  transform: scale(1.1);
}
.dot.on {
  box-shadow: inset 0 0 0 0.5px var(--border-2), 0 0 0 2px var(--bg), 0 0 0 3.5px var(--accent);
}
.dot-clear {
  display: grid;
  place-items: center;
  width: 20px;
  height: 20px;
  border-radius: 999px;
  background: var(--fill);
  color: var(--text-2);
}

.empty {
  padding: 64px 16px;
  text-align: center;
}
.empty-icon {
  display: grid;
  place-items: center;
  width: 72px;
  height: 72px;
  margin: 0 auto 16px;
  border-radius: 20px;
  color: #b07d00;
  background: linear-gradient(160deg, #fff7c7, #ffe98a);
  box-shadow: 0 0 0 0.5px rgba(0, 0, 0, 0.06), 0 6px 18px rgba(255, 204, 0, 0.25);
}
:global(html.dark) .empty-icon {
  color: #ffd60a;
  background: linear-gradient(160deg, #4f4622, #3d3518);
  box-shadow: 0 0 0 0.5px rgba(255, 255, 255, 0.08);
}

/* Undo snackbar (an Apple-style HUD pill). */
.snack {
  position: fixed;
  left: 50%;
  bottom: calc(24px + env(safe-area-inset-bottom));
  z-index: 70;
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 8px 8px 8px 18px;
  border-radius: 999px;
  translate: -50% 0;
  font-size: 13.5px;
  font-weight: 500;
  color: #fff;
  background: rgba(36, 36, 38, 0.9);
  backdrop-filter: saturate(180%) blur(20px);
  -webkit-backdrop-filter: saturate(180%) blur(20px);
  box-shadow: 0 0 0 0.5px rgba(255, 255, 255, 0.08), 0 10px 30px rgba(0, 0, 0, 0.25);
}
.snack-undo {
  padding: 4px 10px;
  border-radius: 999px;
  font-weight: 600;
  color: #64b5ff;
}
.snack-undo:hover {
  background: rgba(255, 255, 255, 0.1);
}
.snack-x {
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  border-radius: 999px;
  color: rgba(255, 255, 255, 0.6);
}
.snack-x:hover {
  background: rgba(255, 255, 255, 0.12);
  color: #fff;
}
.snack-enter-active,
.snack-leave-active {
  transition: opacity 0.2s var(--ease), transform 0.3s var(--spring);
}
.snack-enter-from,
.snack-leave-to {
  opacity: 0;
  transform: translateY(12px) scale(0.97);
}
</style>
