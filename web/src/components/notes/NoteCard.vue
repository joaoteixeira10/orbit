<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, Pin, Palette, SquareCheck, Type, MoreHorizontal, Trash2, Copy, FileText, ListChecks, FolderInput, Paperclip, BellRing } from 'lucide-vue-next'
import { fmtDate, fmtTime } from '@/lib/dates'
import { uploadBlobUrl } from '@/lib/uploads'
import Popover from '@/components/ui/Popover.vue'
import NoteColorPicker from './NoteColorPicker.vue'
import { orderedItems } from './noteModel'
import type { Note, NoteColor } from '@/api/planner'
import { evaluate } from '@/lib/mathnotes'

// One sticky note on the wall: its tint, title, text (clamped with a fade)
// or checklist (tick right here; open items first, then the checked ones).
// Editors get a toolbar on hover (always shown on touch): pin, colour,
// text ↔ checklist and a ⋯ menu. A click anywhere else opens the editor.
const props = withDefaults(defineProps<{ note: Note; editable?: boolean; canTasks?: boolean }>(), { editable: false, canTasks: false })
const emit = defineEmits<{
  open: []
  toggleItem: [id: string]
  pin: []
  color: [c: NoteColor]
  convert: []
  page: []
  tasks: []
  copy: []
  move: []
  remove: []
}>()
const { t } = useI18n()

const MAX_ROWS = 8
const items = computed(() => orderedItems(props.note.items))
const shown = computed(() => items.value.slice(0, MAX_ROWS))
const hidden = computed(() => items.value.length - shown.value.length)
const empty = computed(() => !props.note.title && !props.note.body && !props.note.items.length && !props.note.attachments?.length)

// The note's first image, on top of the card (fetched with the sign-in
// token); other files are counted at the bottom.
const cover = ref('')
const firstImage = computed(() => props.note.attachments?.find((a) => a.image))
const otherFiles = computed(() => props.note.attachments?.filter((a) => !a.image).length ?? 0)
watch(
  firstImage,
  (f) => {
    cover.value = ''
    if (f) void uploadBlobUrl(f.url).then((u) => (cover.value = u)).catch(() => {})
  },
  { immediate: true },
)

// The text is clamped; the fade only shows when something is cut off.
const bodyEl = ref<HTMLElement | null>(null)
const bodyLines = computed(() => props.note.body.split('\n'))
// Sums ending in "=" show their answer, as in the editor.
const answers = computed(() => new Map(props.note.checklist ? [] : evaluate(props.note.body).map((a) => [a.line, a.result])))
const clamped = ref(false)
const measure = () => nextTick(() => (clamped.value = !!bodyEl.value && bodyEl.value.scrollHeight > bodyEl.value.clientHeight + 2))
onMounted(measure)
watch(() => props.note.body, measure)

const colorBtn = ref<HTMLElement | null>(null)
const moreBtn = ref<HTMLElement | null>(null)
const menu = ref<'' | 'color' | 'more'>('')
const color = computed({
  get: () => props.note.color,
  set: (c: NoteColor) => emit('color', c),
})
function act(e: 'page' | 'tasks' | 'copy' | 'move' | 'remove') {
  menu.value = ''
  if (e === 'page') emit('page')
  else if (e === 'tasks') emit('tasks')
  else if (e === 'copy') emit('copy')
  else if (e === 'move') emit('move')
  else emit('remove')
}
</script>

<template>
  <article
    class="note-card group/card"
    :class="{ active: !!menu, editable }"
    :data-note="note.color"
    tabindex="0"
    role="button"
    :aria-label="note.title || note.body.slice(0, 60) || t('notes.title')"
    @click="emit('open')"
    @keydown.enter.self="emit('open')"
  >
    <button
      v-if="editable"
      class="pin tb"
      :class="{ on: note.pinned }"
      :aria-label="note.pinned ? t('notes.unpin') : t('notes.pin')"
      :title="note.pinned ? t('notes.unpin') : t('notes.pin')"
      @click.stop="emit('pin')"
    >
      <Pin class="h-4 w-4" :fill="note.pinned ? 'currentColor' : 'none'" />
    </button>

    <img v-if="cover" :src="cover" alt="" class="n-cover" />
    <h3 v-if="note.title" class="n-title" :class="{ 'pr-7': editable }">{{ note.title }}</h3>

    <div v-if="!note.checklist && note.body" ref="bodyEl" class="n-body" :class="{ clamped, big: !note.title && note.body.length < 80 }"><template v-if="answers.size"><template v-for="(l, i) in bodyLines" :key="i">{{ l }}<b v-if="answers.get(i)" class="n-calc">{{ /\s$/.test(l) ? '' : ' ' }}{{ answers.get(i) }}</b>{{ i < bodyLines.length - 1 ? '\n' : '' }}</template></template><template v-else>{{ note.body }}</template></div>

    <ul v-else-if="note.checklist && items.length" class="n-list">
      <li v-for="it in shown" :key="it.id" class="n-item" :class="{ done: it.done }">
        <button
          class="check"
          :class="{ on: it.done }"
          :style="{ '--check': 'var(--note-ink)' }"
          role="checkbox"
          :aria-checked="it.done"
          :aria-label="it.text"
          :disabled="!editable"
          @click.stop="emit('toggleItem', it.id)"
        >
          <Check class="h-2.5 w-2.5" :stroke-width="3.6" />
        </button>
        <span class="min-w-0 flex-1">{{ it.text }}</span>
      </li>
      <li v-if="hidden" class="n-more">{{ t('notes.more', { n: hidden }) }}</li>
    </ul>

    <p v-else-if="empty" class="n-body text-text-3">…</p>
    <p v-if="otherFiles" class="n-files"><Paperclip class="h-3 w-3" />{{ otherFiles }}</p>
    <p v-if="note.remind_at" class="n-files"><BellRing class="h-3 w-3" />{{ fmtDate(new Date(note.remind_at), { weekday: 'short', day: 'numeric', month: 'short' }) }}, {{ fmtTime(new Date(note.remind_at)) }}</p>

    <footer v-if="editable" class="bar" @click.stop>
      <button ref="colorBtn" class="tb" :class="{ on: menu === 'color' }" :aria-label="t('notes.color')" :title="t('notes.color')" @click="menu = menu === 'color' ? '' : 'color'">
        <Palette class="h-4 w-4" />
      </button>
      <button class="tb" :aria-label="note.checklist ? t('notes.hideChecks') : t('notes.showChecks')" :title="note.checklist ? t('notes.hideChecks') : t('notes.showChecks')" @click="emit('convert')">
        <Type v-if="note.checklist" class="h-4 w-4" />
        <SquareCheck v-else class="h-4 w-4" />
      </button>
      <button ref="moreBtn" class="tb" :class="{ on: menu === 'more' }" :aria-label="t('notes.options')" :title="t('notes.options')" @click="menu = menu === 'more' ? '' : 'more'">
        <MoreHorizontal class="h-4 w-4" />
      </button>
    </footer>

    <Popover v-if="menu === 'color'" :anchor="colorBtn" placement="bottom-start" @close="menu = ''">
      <NoteColorPicker v-model="color" />
    </Popover>
    <Popover v-if="menu === 'more'" :anchor="moreBtn" placement="bottom-start" :width="210" @close="menu = ''">
      <button class="menu-item" @click="act('move')"><FolderInput class="h-4 w-4 text-text-2" />{{ t('notes.mine.moveTo') }}</button>
      <button class="menu-item" @click="act('page')"><FileText class="h-4 w-4 text-text-2" />{{ t('notes.toPage') }}</button>
      <button v-if="canTasks" class="menu-item" @click="act('tasks')"><ListChecks class="h-4 w-4 text-text-2" />{{ t('notes.toTasks') }}</button>
      <button class="menu-item" @click="act('copy')"><Copy class="h-4 w-4 text-text-2" />{{ t('notes.copy') }}</button>
      <div class="menu-sep" />
      <button class="menu-item danger" @click="act('remove')"><Trash2 class="h-4 w-4" />{{ t('common.delete') }}</button>
    </Popover>
  </article>
</template>

<style scoped>
.n-cover {
  display: block;
  width: calc(100% + 32px);
  max-width: none;
  max-height: 180px;
  margin: -14px -16px 4px;
  object-fit: cover;
  border-radius: 14px 14px 0 0;
}
.n-files {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  margin-top: 6px;
  font-size: 12px;
  color: var(--text-3);
}
.note-card {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 14px 16px 14px;
  border-radius: 14px;
  background: var(--note-bg);
  box-shadow: 0 0 0 0.5px var(--border), var(--shadow-sm);
  cursor: default;
  user-select: none;
  transition: box-shadow 0.2s var(--ease), transform 0.2s var(--spring);
}
.note-card:not([data-note='']) {
  box-shadow: 0 0 0 0.5px var(--note-line), var(--shadow-sm);
}
.note-card:hover {
  box-shadow: 0 0 0 0.5px var(--border-2), var(--shadow);
}
.note-card:focus-visible {
  box-shadow: 0 0 0 3px var(--ring);
  border-radius: 14px;
}
.n-title {
  font: 600 15px/1.35 var(--font-display);
  letter-spacing: -0.012em;
  color: var(--text);
  overflow-wrap: anywhere;
}
.n-body {
  font-size: 14px;
  line-height: 1.45;
  color: var(--text);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  max-height: calc(14 * 1.45em);
  overflow: hidden;
}
.n-body.big {
  font-size: 17px;
  line-height: 1.4;
  letter-spacing: -0.012em;
  max-height: calc(14 * 1.4em);
}
.n-body.clamped {
  -webkit-mask-image: linear-gradient(to bottom, #000 72%, transparent);
  mask-image: linear-gradient(to bottom, #000 72%, transparent);
}
.n-title + .n-body,
.n-title + .n-list {
  color: var(--text);
}
.n-list {
  display: flex;
  flex-direction: column;
  gap: 5px;
  margin-top: 1px;
}
.n-item {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  font-size: 14px;
  line-height: 1.4;
  overflow-wrap: anywhere;
}
.n-item .check {
  width: 16px;
  height: 16px;
  margin-top: 1.5px;
  border-width: 1.3px;
}
.n-item.done {
  color: var(--text-3);
  text-decoration: line-through;
}
.n-item.done .check {
  opacity: 0.7;
}
.n-more {
  padding-left: 25px;
  font-size: 12.5px;
  font-weight: 500;
  color: var(--text-2);
}
.n-body.text-text-3 {
  color: var(--text-3);
}

/* Toolbar: fades in on hover (and stays while a popover is open). */
.tb {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: 999px;
  color: var(--text-2);
  transition: background 0.15s var(--ease), color 0.15s, opacity 0.18s var(--ease);
}
.tb:hover,
.tb.on {
  background: var(--fill);
  color: var(--text);
}
.pin {
  position: absolute;
  top: 8px;
  right: 8px;
  opacity: 0;
}
.pin.on {
  color: var(--text);
}
.pin.on:not(:hover) {
  background: transparent;
}
.bar {
  display: flex;
  gap: 2px;
  margin: -2px -9px -9px;
  opacity: 0;
  transition: opacity 0.18s var(--ease);
}
.note-card:hover .bar,
.note-card:hover .pin,
.note-card:focus-within .bar,
.note-card:focus-within .pin,
.note-card.active .bar,
.note-card.active .pin {
  opacity: 1;
}
@media (hover: none) {
  .bar,
  .pin {
    opacity: 1;
  }
}
.n-calc {
  color: var(--accent-text);
  font-weight: 600;
}
</style>
