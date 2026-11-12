<script setup lang="ts">
import Attachments from '@/components/ui/Attachments.vue'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Pin, Palette, SquareCheck, Type, MoreHorizontal, Trash2, Copy, FileText, ListChecks, FolderInput, Paperclip, BellPlus, BellRing, X, Brush, Maximize2, Minimize2, LockKeyhole, LockKeyholeOpen } from 'lucide-vue-next'
import { seal, unseal, unlocked } from '@/lib/lock'
import { askUnlock } from '@/lib/lockPrompt'
import { useAuthStore } from '@/stores/auth'
import { noteWide } from './noteWide'
import DrawingSheet from '@/components/draw/DrawingSheet.vue'
import { drawingsApi, previewSrc, type Drawing } from '@/components/draw/drawing'
import RemindPicker from '@/components/ui/RemindPicker.vue'
import Overlay from '@/components/ui/Overlay.vue'
import Popover from '@/components/ui/Popover.vue'
import NoteFields from './NoteFields.vue'
import NoteColorPicker from './NoteColorPicker.vue'
import { useNotesStore } from '@/stores/notes'
import { useToastStore } from '@/stores/toast'
import { locale, fmtDate, fmtTime } from '@/lib/dates'
import { ago, draftOf, inputOf, isEmptyDraft, sameContent, toChecklist, toText, type Draft } from './noteModel'
import type { Note } from '@/api/planner'

// One note, opened from the wall: the same editing as the capture box in a
// sheet tinted with the note's colour. Changes save while typing (debounced)
// and on close; closing an emptied note deletes it. The ⋯ menu's actions are
// the view's (it saves first, then asks for them).
const props = withDefaults(defineProps<{ open: boolean; ws: string; noteId: string; editable?: boolean; canTasks?: boolean }>(), {
  editable: false,
  canTasks: false,
})
const emit = defineEmits<{ close: []; page: [n: Note]; tasks: [n: Note]; copy: [n: Note]; move: [n: Note]; remove: [n: Note] }>()
const notes = useNotesStore()
const toast = useToastStore()
const { t } = useI18n()

const note = computed(() => notes.notesOf(props.ws).find((n) => n.id === props.noteId))
const auth = useAuthStore()

// A locked note (lib/lock): its text only sealed. Opened with the PIN, its
// text lives here (opened) and saves sealed; someone else's stays shut.
type Plain = { body: string; checklist: boolean; items: Note['items'] }
const opened = ref<Note | null>(null)
const sealed = computed(() => !!note.value?.sealed)
const sealedMine = computed(() => sealed.value && note.value?.sealed_by === auth.user?.id)
const shut = computed(() => sealed.value && !opened.value)
// What the draft is compared with: the note, or its opened text.
const src = computed<Note | undefined>(() => (sealed.value ? (opened.value ?? undefined) : note.value))
const canEdit = computed(() => props.editable && (!sealed.value || (sealedMine.value && !!opened.value)))
async function openSealed() {
  const n = note.value
  if (!n?.cipher || !sealedMine.value || !(await askUnlock())) return
  try {
    const p = await unseal<Plain>(n.cipher)
    opened.value = { ...n, body: p.body, checklist: p.checklist, items: p.items }
    draft.value = draftOf(opened.value)
  } catch (e: any) {
    toast.error(e.message)
  }
}
// Locked again (by hand, or after a while): shut it.
watch(unlocked, (u) => {
  if (!u && opened.value) {
    void save().then(() => {
      opened.value = null
      draft.value = draftOf({ ...note.value!, body: '', items: [] })
    })
  }
})
const withIds = (items: Note['items']) => items.map((i) => ({ ...i, id: i.id || Math.random().toString(36).slice(2, 10) }))
const files = ref<InstanceType<typeof Attachments> | null>(null)
const draft = ref<Draft>(draftOf())
const fields = ref<InstanceType<typeof NoteFields> | null>(null)
const root = ref<HTMLElement | null>(null)
let loadedId = ''
let timer: ReturnType<typeof setTimeout> | undefined
let saving: Promise<unknown> = Promise.resolve()

// Load the note when the editor opens (or another one is shown).
watch(
  () => [props.open, note.value?.id] as const,
  ([o, id]) => {
    if (!o || !id || id === loadedId) return
    loadedId = id
    opened.value = null
    draft.value = draftOf(note.value)
    if (note.value?.sealed && unlocked.value) void openSealed()
  },
  { immediate: true },
)
watch(
  () => props.open,
  (o) => {
    if (!o) loadedId = ''
  },
)
// Someone else changed it: take their version unless we're typing in it.
watch(note, (n, prev) => {
  if (!n || !prev || n.id !== prev.id || n.updated_at === prev.updated_at || timer) return
  if (n.sealed) return // its text is ours to open
  if (root.value?.contains(document.activeElement)) return
  if (!sameContent(draft.value, n)) draft.value = draftOf(n)
})

watch(
  draft,
  () => {
    if (!canEdit.value || !src.value || sameContent(draft.value, src.value)) return
    clearTimeout(timer)
    timer = setTimeout(() => void save(), 600)
  },
  { deep: true },
)

async function save() {
  clearTimeout(timer)
  timer = undefined
  const n = note.value
  const d = draft.value
  const base = src.value
  if (!canEdit.value || !n || !base || isEmptyDraft(d) || sameContent(d, base)) return
  const body = inputOf(d)
  const run = saving.then(async () => {
    try {
      if (n.sealed) {
        // Locked: the text goes sealed; the rest (title, colour…) as it is.
        const plain: Plain = { body: body.body ?? '', checklist: !!body.checklist, items: withIds(body.items ?? []) }
        const { body: _b, items: _i, ...rest } = body
        await notes.update(props.ws, n.id, { ...rest, cipher: await seal(plain) })
        opened.value = { ...n, ...rest, ...plain } as Note
        d.items.forEach((i, x) => (i.id ||= plain.items[x]?.id ?? ''))
        return
      }
      const saved = await notes.update(props.ws, n.id, body)
      // New rows get their server ids (by position among the non-empty ones).
      const kept = d.items.filter((i) => i.text.trim())
      if (saved.checklist && saved.items.length === kept.length) kept.forEach((i, x) => (i.id ||= saved.items[x].id))
    } catch (e: any) {
      toast.error(e.message)
    }
  })
  saving = run
  await run
}

async function close() {
  menu.value = ''
  const n = note.value
  if (props.editable && n && !sealed.value && isEmptyDraft(draft.value)) {
    clearTimeout(timer)
    timer = undefined
    emit('close')
    emit('remove', n)
    return
  }
  await save()
  emit('close')
}
onBeforeUnmount(() => void save())

// The toolbar.
const colorBtn = ref<HTMLElement | null>(null)
const moreBtn = ref<HTMLElement | null>(null)
const menu = ref<'' | 'color' | 'more' | 'remind'>('')
const remindBtn = ref<HTMLElement | null>(null)

// A one-off reminder (to me): set, changed or taken off at once.
const remindLabel = computed(() => {
  const at = note.value?.remind_at
  if (!at) return ''
  const d = new Date(at)
  return `${fmtDate(d, { weekday: 'short', day: 'numeric', month: 'short' })}, ${fmtTime(d)}`
})
// Its drawing: made the first time it's opened; its picture shows here and
// on the card.
const drawingId = ref('')
async function openDrawing() {
  const n = note.value
  if (!n) return
  if (n.drawing) {
    drawingId.value = n.drawing.id
    return
  }
  if (!props.editable) return
  try {
    const d = await drawingsApi.ofNote(n.id)
    n.drawing = { id: d.id, preview: d.preview ?? '' }
    drawingId.value = d.id
  } catch (e: any) {
    toast.error(e.message)
  }
}
function drawn(d: Drawing) {
  const n = note.value
  if (n) n.drawing = { id: d.id, preview: d.preview ?? '' }
}
async function removeDrawing() {
  const n = note.value
  const id = drawingId.value
  drawingId.value = ''
  if (!n || !id) return
  try {
    await drawingsApi.remove(id)
    n.drawing = undefined
    toast.info(t('draw.removed'))
  } catch (e: any) {
    toast.error(e.message)
  }
}
// Closing an empty drawing leaves no picture behind.
function drawingClosed() {
  const n = note.value
  const id = drawingId.value
  drawingId.value = ''
  if (n?.drawing && n.drawing.id === id && !n.drawing.preview && props.editable) {
    void drawingsApi.remove(id).catch(() => {})
    n.drawing = undefined
  }
}

async function remind(at: string) {
  menu.value = ''
  const n = note.value
  if (!n) return
  try {
    await notes.update(props.ws, n.id, { remind_at: at })
    if (at) {
      const d = new Date(at)
      toast.success(t('remind.setTo', { when: `${fmtDate(d, { weekday: 'short' })} ${fmtTime(d)}` }))
    } else toast.info(t('remind.removed'))
  } catch (e: any) {
    toast.error(e.message)
  }
}
function toggleChecklist() {
  if (draft.value.checklist) toText(draft.value)
  else toChecklist(draft.value)
  void fields.value?.focus('end')
}
// Lock it with my PIN (set one first, if there's none), or take the lock off.
async function lockNote() {
  menu.value = ''
  const n = note.value
  if (!n || !(await askUnlock())) return
  await save()
  const d = inputOf(draft.value)
  const plain: Plain = { body: d.body ?? '', checklist: !!d.checklist, items: withIds(d.items ?? []) }
  try {
    await notes.update(props.ws, n.id, { lock: true, cipher: await seal(plain) })
    opened.value = { ...n, ...plain, sealed: true, sealed_by: auth.user?.id }
    toast.success(t('lock.lockedDone'))
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function unlockNote() {
  menu.value = ''
  const n = note.value
  const o = opened.value
  if (!n || !o) return
  await save()
  try {
    await notes.update(props.ws, n.id, { lock: false, body: o.body, checklist: o.checklist, items: o.items })
    opened.value = null
    toast.info(t('lock.unlockedDone'))
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function act(kind: 'page' | 'tasks' | 'copy' | 'move' | 'remove') {
  menu.value = ''
  if (kind !== 'remove') await save()
  const n = note.value
  if (!n) return
  if (kind === 'remove') {
    clearTimeout(timer)
    timer = undefined
    emit('close')
  }
  if (kind === 'page') emit('page', n)
  else if (kind === 'tasks') emit('tasks', n)
  else if (kind === 'copy') emit('copy', n)
  else if (kind === 'move') emit('move', n)
  else emit('remove', n)
}

// "Edited 2 hours ago", kept fresh while open.
const now = ref(Date.now())
const tick = setInterval(() => (now.value = Date.now()), 30000)
onBeforeUnmount(() => clearInterval(tick))
const edited = computed(() => (note.value ? t('notes.edited', { when: ago(note.value.updated_at, locale(), now.value) }) : ''))
</script>

<template>
  <Overlay :open="open && !!note" :size="noteWide ? 'xl' : 'md'" @close="close">
    <div v-if="note" ref="root" class="note-sheet" :class="{ wide: noteWide }" :data-note="draft.color">
      <div class="relative">
        <div v-if="shut" class="sealed-note" data-testid="note-locked">
          <input v-if="draft.title" class="sealed-title" :value="draft.title" readonly :aria-label="t('lock.locked')" />
          <LockKeyhole class="h-7 w-7 text-text-3" />
          <p class="text-[13px] text-text-2">{{ sealedMine ? t('lock.lockedHint') : t('lock.lockedOther') }}</p>
          <button v-if="sealedMine" class="btn btn-sm" data-testid="note-unlock" @click="openSealed">{{ t('lock.openIt') }}</button>
        </div>
        <NoteFields v-else ref="fields" v-model="draft" big :readonly="!canEdit" @submit="close" />
        <Attachments
          v-if="editable || note.attachments?.length"
          ref="files"
          class="mt-2"
          target="note"
          :target-id="note.id"
          :editable="editable"
          :initial="note.attachments ?? []"
          compact
          @change="(f) => (note!.attachments = f)"
        />
        <button v-if="note.drawing?.preview" class="drawing-pic" data-testid="note-drawing" :aria-label="t('draw.noteTitle')" @click="openDrawing">
          <img :src="previewSrc(note.drawing.preview)" alt="" draggable="false" />
        </button>
        <p v-if="note.remind_at" class="remind-chip" data-testid="note-reminder">
          <BellRing class="h-3.5 w-3.5" />{{ t('remind.at', { when: remindLabel }) }}
          <button v-if="editable" class="ml-0.5 opacity-70 hover:opacity-100" :aria-label="t('remind.remove')" :title="t('remind.remove')" @click="remind('')"><X class="h-3.5 w-3.5" /></button>
        </p>
        <button
          v-if="editable"
          class="tb pin absolute -right-2 -top-1"
          :class="{ on: draft.pinned }"
          :aria-label="draft.pinned ? t('notes.unpin') : t('notes.pin')"
          :title="draft.pinned ? t('notes.unpin') : t('notes.pin')"
          @click="draft.pinned = !draft.pinned"
        >
          <Pin class="h-[18px] w-[18px]" :fill="draft.pinned ? 'currentColor' : 'none'" />
        </button>
      </div>
    </div>
    <template v-if="note" #footer>
      <div class="note-sheet flex min-w-0 flex-1 items-center gap-1" :data-note="draft.color">
        <template v-if="canEdit">
          <button ref="colorBtn" class="tb" :class="{ on: menu === 'color' }" :aria-label="t('notes.color')" :title="t('notes.color')" @click="menu = menu === 'color' ? '' : 'color'">
            <Palette class="h-[18px] w-[18px]" />
          </button>
          <button class="tb" :aria-label="draft.checklist ? t('notes.hideChecks') : t('notes.showChecks')" :title="draft.checklist ? t('notes.hideChecks') : t('notes.showChecks')" @click="toggleChecklist">
            <Type v-if="draft.checklist" class="h-[18px] w-[18px]" />
            <SquareCheck v-else class="h-[18px] w-[18px]" />
          </button>
          <button class="tb" :class="{ on: !!note.drawing }" :aria-label="t('draw.onNote')" :title="t('draw.onNote')" data-testid="note-draw" @click="openDrawing">
            <Brush class="h-[18px] w-[18px]" />
          </button>
          <button ref="remindBtn" class="tb" :class="{ on: menu === 'remind' || !!note.remind_at }" :aria-label="t('remind.title')" :title="t('remind.title')" data-testid="note-remind" @click="menu = menu === 'remind' ? '' : 'remind'">
            <BellPlus class="h-[18px] w-[18px]" />
          </button>
          <button class="tb" :aria-label="t('files.attach')" :title="t('files.attach')" @click="files?.pick()">
            <Paperclip class="h-[18px] w-[18px]" />
          </button>
          <button ref="moreBtn" class="tb" :class="{ on: menu === 'more' }" :aria-label="t('notes.options')" :title="t('notes.options')" @click="menu = menu === 'more' ? '' : 'more'">
            <MoreHorizontal class="h-[18px] w-[18px]" />
          </button>
        </template>
        <button class="tb" :class="{ on: noteWide }" :aria-label="noteWide ? t('notes.smaller') : t('notes.bigger')" :title="noteWide ? t('notes.smaller') : t('notes.bigger')" :aria-pressed="noteWide" data-testid="note-size" @click="noteWide = !noteWide">
          <Minimize2 v-if="noteWide" class="h-[18px] w-[18px]" />
          <Maximize2 v-else class="h-[18px] w-[18px]" />
        </button>
        <span class="ml-2 min-w-0 flex-1 truncate text-[12px] text-text-2">{{ edited }}</span>
        <button class="btn btn-sm close-btn" @click="close">{{ t('common.close') }}</button>
      </div>
      <Popover v-if="menu === 'color'" :anchor="colorBtn" placement="top-start" @close="menu = ''">
        <NoteColorPicker v-model="draft.color" />
      </Popover>
      <Popover v-if="menu === 'remind'" :anchor="remindBtn" placement="top-start" :width="270" @close="menu = ''">
        <RemindPicker :current="note.remind_at" @pick="remind" />
      </Popover>
      <DrawingSheet :open="!!drawingId" :id="drawingId" :title="draft.title" :readonly="!editable" @close="drawingClosed" @saved="drawn" @remove="removeDrawing" />
      <Popover v-if="menu === 'more'" :anchor="moreBtn" placement="top-start" :width="210" @close="menu = ''">
        <button class="menu-item" @click="act('move')"><FolderInput class="h-4 w-4 text-text-2" />{{ t('notes.mine.moveTo') }}</button>
        <template v-if="!sealed">
          <button class="menu-item" @click="act('page')"><FileText class="h-4 w-4 text-text-2" />{{ t('notes.toPage') }}</button>
          <button v-if="canTasks" class="menu-item" @click="act('tasks')"><ListChecks class="h-4 w-4 text-text-2" />{{ t('notes.toTasks') }}</button>
          <button class="menu-item" @click="act('copy')"><Copy class="h-4 w-4 text-text-2" />{{ t('notes.copy') }}</button>
          <button class="menu-item" data-testid="note-lock" @click="lockNote"><LockKeyhole class="h-4 w-4 text-text-2" />{{ t('lock.lock') }}</button>
        </template>
        <button v-else class="menu-item" data-testid="note-unlock-for-good" @click="unlockNote"><LockKeyholeOpen class="h-4 w-4 text-text-2" />{{ t('lock.unlockFor') }}</button>
        <div class="menu-sep" />
        <button class="menu-item danger" @click="act('remove')"><Trash2 class="h-4 w-4" />{{ t('common.delete') }}</button>
      </Popover>
    </template>
  </Overlay>
</template>

<style scoped>
.sealed-note {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 18px 8px 22px;
  text-align: center;
}
.sealed-title {
  align-self: stretch;
  background: transparent;
  font-size: 17px;
  font-weight: 600;
  text-align: left;
  outline: none;
}
.note-sheet.wide :deep(.note-fields .cm-content) {
  min-height: 50vh;
}
.drawing-pic {
  display: block;
  width: 100%;
  margin-top: 10px;
  padding: 8px;
  border-radius: 12px;
  background: #fff;
  box-shadow: 0 0 0 0.5px rgb(0 0 0 / 0.08);
}
.drawing-pic img {
  display: block;
  max-width: 100%;
  max-height: 260px;
  margin: 0 auto;
}
.remind-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  margin-top: 8px;
  padding: 3px 9px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--text) 8%, transparent);
  font-size: 12.5px;
  font-weight: 500;
}
.note-sheet {
  min-height: 0;
  padding-top: 10px;
}
footer .note-sheet,
.note-sheet + .note-sheet {
  padding-top: 0;
}
.note-sheet :deep(.f-title) {
  padding-right: 34px;
}
.tb.pin.on {
  background: transparent;
}
.tb.pin.on:hover {
  background: var(--fill);
}
.tb {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  border-radius: 999px;
  color: var(--text-2);
  transition: background 0.15s var(--ease), color 0.15s;
}
.tb:hover,
.tb.on {
  background: var(--fill);
  color: var(--text);
}
.close-btn {
  background: var(--note-line);
  font-weight: 600;
}
:global(.sheet-panel:has(.note-sheet) footer) {
  border-top-color: var(--note-line) !important;
  padding-top: 8px !important;
  padding-bottom: 10px !important;
}
</style>
