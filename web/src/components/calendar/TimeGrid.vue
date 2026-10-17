<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, Repeat, MapPin } from 'lucide-vue-next'
import { dateStr, sameDay, fmtDate, fmtTime, fmtClock } from '@/lib/dates'
import { layoutBars, layoutDay, tasksByDay, eventColor, type Entry, type TaskItem } from './model'
import { startTaskDrag, isTaskDrag, readTaskDrag, type DraggedTask } from '@/lib/taskDrag'

// Week and Day: hours down the side, all-day events and due tasks in a
// strip on top, timed events placed by start and end (side by side when they
// overlap), and the red line at the current time. Clicking an empty slot
// creates an event there; dragging down with the mouse picks its length.
const props = defineProps<{
  days: Date[]
  entries: Entry[]
  tasks: TaskItem[]
  canCreate?: boolean
}>()
const emit = defineEmits<{
  open: [entry: Entry]
  openTask: [item: TaskItem]
  toggleTask: [item: TaskItem]
  create: [range: { start: Date; end: Date; allDay: boolean }]
  openDay: [day: string]
  dropTask: [task: DraggedTask, day: string, time: string | null]
}>()
const { t } = useI18n()

const HOUR = 48 // px per hour
const LANE = 22 // px per all-day line

const now = ref(new Date())
let tick: ReturnType<typeof setInterval> | null = null
const scroller = ref<HTMLElement | null>(null)
onMounted(() => {
  tick = setInterval(() => (now.value = new Date()), 30000)
  // Open on the working day, as Calendar does (once laid out).
  requestAnimationFrame(() => {
    if (scroller.value) scroller.value.scrollTop = 8 * HOUR - 8
  })
})
onBeforeUnmount(() => {
  if (tick) clearInterval(tick)
})

const keys = computed(() => props.days.map(dateStr))
const cols = computed(() => props.days.length)
const todayIdx = computed(() => props.days.findIndex((d) => sameDay(d, now.value)))
const nowMin = computed(() => now.value.getHours() * 60 + now.value.getMinutes())

const hours = computed(() =>
  Array.from({ length: 23 }, (_, i) => {
    const h = i + 1
    // The current time's label replaces the hour next to it.
    const hidden = todayIdx.value >= 0 && Math.abs(nowMin.value - h * 60) < 20
    return { h, label: fmtTime(new Date(2000, 0, 1, h)), hidden }
  }),
)

// ---- All-day strip: bars on lanes, then each day's tasks below them ----
const strip = computed(() => {
  const { bars, lanes } = layoutBars(props.entries, keys.value)
  const byDay = tasksByDay(props.tasks)
  const tasks = keys.value.flatMap((k, c) => (byDay.get(k) ?? []).map((item, i) => ({ item, col: c, lane: lanes[c] + i })))
  const height = Math.max(1, ...lanes, ...tasks.map((x) => x.lane + 1))
  return { bars, tasks, height }
})
const stripPx = computed(() => strip.value.height * LANE + 6)
const span = (col: number, n: number) => ({
  left: `calc(${col} * 100% / ${cols.value} + 2px)`,
  width: `calc(${n} * 100% / ${cols.value} - 4px)`,
})

// ---- Tasks dropped here: on the strip a date, on an hour a date and time ----
const dropAt = ref<{ col: number; min: number | null } | null>(null)
function colAt(ev: DragEvent, el: HTMLElement) {
  const r = el.getBoundingClientRect()
  return Math.min(cols.value - 1, Math.max(0, Math.floor(((ev.clientX - r.left) / r.width) * cols.value)))
}
function overStrip(ev: DragEvent) {
  if (!isTaskDrag(ev)) return
  ev.preventDefault()
  dropAt.value = { col: colAt(ev, ev.currentTarget as HTMLElement), min: null }
}
function overCol(ev: DragEvent, i: number) {
  if (!isTaskDrag(ev)) return
  ev.preventDefault()
  const r = (ev.currentTarget as HTMLElement).getBoundingClientRect()
  dropAt.value = { col: i, min: Math.min(1425, Math.floor((((ev.clientY - r.top) / HOUR) * 60) / 15) * 15) }
}
function dropTask(ev: DragEvent) {
  const at = dropAt.value
  dropAt.value = null
  const task = isTaskDrag(ev) ? readTaskDrag(ev) : null
  if (!task || !at) return
  ev.preventDefault()
  const time = at.min == null ? null : `${String(Math.floor(at.min / 60)).padStart(2, '0')}:${String(at.min % 60).padStart(2, '0')}`
  emit('dropTask', task, keys.value[at.col], time)
}

function onStripClick(ev: MouseEvent) {
  if (!props.canCreate || ev.target !== ev.currentTarget) return
  const r = (ev.currentTarget as HTMLElement).getBoundingClientRect()
  const c = Math.min(cols.value - 1, Math.max(0, Math.floor(((ev.clientX - r.left) / r.width) * cols.value)))
  const d = props.days[c]
  emit('create', { start: d, end: d, allDay: true })
}

// ---- Timed events ----
const placed = computed(() => props.days.map((d) => layoutDay(props.entries, d, HOUR)))
function blockStyle(p: ReturnType<typeof layoutDay>[number]) {
  return {
    '--c': eventColor(p.entry),
    top: `${p.top}px`,
    height: `${p.height - 1}px`,
    left: `calc(${p.col} * 100% / ${p.cols})`,
    width: `calc(100% / ${p.cols} - 3px)`,
  }
}

// ---- Click or drag on an empty slot to create ----
const sel = ref<{ col: number; a: number; b: number } | null>(null)
let press: { col: number; el: HTMLElement; y: number; at: number; moved: boolean; mouse: boolean } | null = null
const floorTo = (m: number, step: number) => Math.floor(m / step) * step
const ceilTo = (m: number, step: number) => Math.ceil(m / step) * step
function minutesAt(ev: PointerEvent, el: HTMLElement) {
  const r = el.getBoundingClientRect()
  return Math.min(1440, Math.max(0, ((ev.clientY - r.top) / HOUR) * 60))
}
function atMinutes(day: Date, m: number) {
  return new Date(day.getFullYear(), day.getMonth(), day.getDate(), 0, m)
}
function onDown(ev: PointerEvent, col: number) {
  if (!props.canCreate || ev.button !== 0 || ev.target !== ev.currentTarget) return
  const el = ev.currentTarget as HTMLElement
  const mouse = ev.pointerType === 'mouse'
  press = { col, el, y: ev.clientY, at: minutesAt(ev, el), moved: false, mouse }
  if (mouse) {
    el.setPointerCapture(ev.pointerId)
    ev.preventDefault()
  }
}
function onMove(ev: PointerEvent) {
  if (!press || !press.mouse) return
  if (!press.moved && Math.abs(ev.clientY - press.y) < 6) return
  press.moved = true
  const m = minutesAt(ev, press.el)
  const a = floorTo(Math.min(press.at, m), 15)
  const b = Math.max(a + 15, ceilTo(Math.max(press.at, m), 15))
  sel.value = { col: press.col, a, b: Math.min(1440, b) }
}
function onUp() {
  if (!press) return
  const day = props.days[press.col]
  let a: number, b: number
  if (press.moved && sel.value) {
    a = sel.value.a
    b = sel.value.b
  } else {
    a = Math.min(floorTo(press.at, 30), 1380)
    b = a + 60
  }
  press = null
  sel.value = null
  emit('create', { start: atMinutes(day, a), end: atMinutes(day, b), allDay: false })
}
function onCancel() {
  press = null
  sel.value = null
}
const selLabel = computed(() => {
  if (!sel.value) return ''
  const d = props.days[sel.value.col]
  return `${fmtTime(atMinutes(d, sel.value.a))} – ${fmtTime(atMinutes(d, sel.value.b))}`
})
</script>

<template>
  <div class="tg" :class="{ single: cols === 1 }">
    <!-- Day headers -->
    <div class="tg-row tg-head">
      <div class="tg-gutter" />
      <div class="tg-grid" :style="{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` }">
        <button v-for="(d, i) in days" :key="keys[i]" class="tg-dh" :class="{ today: i === todayIdx }" :disabled="cols === 1" @click="emit('openDay', keys[i])">
          <span class="wd">{{ fmtDate(d, { weekday: cols === 1 ? 'long' : 'short' }) }}</span>
          <span class="dn">{{ d.getDate() }}</span>
        </button>
      </div>
    </div>

    <!-- All-day strip -->
    <div class="tg-row tg-allday">
      <div class="tg-gutter lbl">{{ t('cal.allDayShort') }}</div>
      <div
        class="tg-strip"
        :class="{ 'drop-on': dropAt && dropAt.min == null }"
        :style="{ height: `${stripPx}px`, '--cols': cols, '--drop-col': dropAt?.col ?? 0 }"
        @click="onStripClick"
        @dragover="overStrip"
        @dragleave="dropAt = null"
        @drop="dropTask"
      >
        <div
          v-for="b in strip.bars"
          :key="b.entry.key"
          class="tg-bar"
          :class="{ allday: b.entry.allDay, cs: b.clipStart, ce: b.clipEnd }"
          :style="{ ...span(b.col, b.span), top: `${b.lane * LANE + 3}px`, '--c': eventColor(b.entry) }"
          role="button"
          tabindex="0"
          :title="b.entry.occ.title"
          @click.stop="emit('open', b.entry)"
          @keydown.enter="emit('open', b.entry)"
        >
          <span v-if="!b.entry.allDay && !b.clipStart" class="opacity-80">{{ fmtTime(b.entry.start) }}</span>
          <span class="truncate">{{ b.entry.occ.title || t('cal.untitled') }}</span>
          <Repeat v-if="b.entry.occ.recurring" class="h-2.5 w-2.5 shrink-0 opacity-70" />
        </div>
        <div
          v-for="x in strip.tasks"
          :key="x.item.task.id"
          class="tg-task"
          :class="{ done: x.item.task.done }"
          :style="{ ...span(x.col, 1), top: `${x.lane * LANE + 3}px` }"
          :draggable="x.item.editable && !x.item.task.done"
          @click.stop
          @dragstart="startTaskDrag($event, x.item.task)"
        >
          <button
            class="check"
            :class="{ on: x.item.task.done }"
            :style="{ '--check': x.item.color }"
            role="checkbox"
            :aria-checked="x.item.task.done"
            :aria-label="x.item.task.title"
            :disabled="!x.item.editable"
            @click="emit('toggleTask', x.item)"
          >
            <Check class="h-2 w-2" :stroke-width="4" />
          </button>
          <button class="min-w-0 truncate text-left" :title="x.item.task.title" @click="emit('openTask', x.item)">{{ x.item.task.title }}</button>
          <span v-if="x.item.task.due_time" class="shrink-0 text-[10.5px] tabular-nums text-text-3">{{ fmtClock(x.item.task.due_time) }}</span>
        </div>
      </div>
    </div>

    <!-- Hours -->
    <div ref="scroller" class="tg-scroll">
      <div class="tg-row tg-body" :style="{ height: `${24 * HOUR}px` }">
        <div class="tg-gutter hours">
          <span v-for="h in hours" v-show="!h.hidden" :key="h.h" :style="{ top: `${h.h * HOUR}px` }">{{ h.label }}</span>
          <span v-if="todayIdx >= 0" class="now-label" :style="{ top: `${(nowMin / 60) * HOUR}px` }">{{ fmtTime(now) }}</span>
        </div>
        <div class="tg-grid tg-cols" :style="{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`, backgroundSize: `100% ${HOUR}px` }">
          <div
            v-for="(_d, i) in days"
            :key="keys[i]"
            class="tg-col"
            :class="{ creatable: canCreate }"
            @pointerdown="onDown($event, i)"
            @pointermove="onMove"
            @pointerup="onUp"
            @pointercancel="onCancel"
            @dragover="overCol($event, i)"
            @dragleave="dropAt = null"
            @drop="dropTask"
          >
            <div v-if="dropAt && dropAt.col === i && dropAt.min != null" class="tg-drop" :style="{ top: `${(dropAt.min / 60) * HOUR}px`, height: `${HOUR / 2}px` }" />
            <div
              v-for="p in placed[i]"
              :key="p.entry.key"
              class="tg-ev"
              :class="{ ext: p.entry.occ.kind === 'external', short: p.height < 34 }"
              :style="blockStyle(p)"
              role="button"
              tabindex="0"
              @click.stop="emit('open', p.entry)"
              @keydown.enter="emit('open', p.entry)"
              @pointerdown.stop
            >
              <span class="ttl">{{ p.entry.occ.title || t('cal.untitled') }}<Repeat v-if="p.entry.occ.recurring" class="ml-1 inline h-2.5 w-2.5 opacity-70" /></span>
              <span class="tm">{{ fmtTime(p.entry.start) }}<template v-if="p.height >= 34"> – {{ fmtTime(p.entry.end) }}</template></span>
              <span v-if="p.height >= 56 && p.entry.occ.location" class="loc"><MapPin class="mr-0.5 inline h-2.5 w-2.5" />{{ p.entry.occ.location }}</span>
            </div>
            <div v-if="sel && sel.col === i" class="tg-sel" :style="{ top: `${(sel.a / 60) * HOUR}px`, height: `${((sel.b - sel.a) / 60) * HOUR}px` }">
              {{ selLabel }}
            </div>
            <div v-if="i === todayIdx" class="tg-now" :style="{ top: `${(nowMin / 60) * HOUR}px` }"><i /></div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tg {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-width: 0;
  --gutter: 56px;
}
.tg-row {
  display: flex;
  min-width: 0;
}
.tg-head,
.tg-allday {
  flex: none;
  /* Keep the columns above in line with the scrolling ones below. */
  overflow-y: hidden;
  scrollbar-gutter: stable;
}
.tg-gutter {
  flex: none;
  width: var(--gutter);
}
.tg-grid {
  display: grid;
  flex: 1;
  min-width: 0;
}
.tg-dh {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  min-width: 0;
  padding: 2px 2px 8px;
  font-size: 13px;
  color: var(--text-2);
  cursor: pointer;
}
.tg-dh:disabled {
  cursor: default;
}
.single .tg-dh {
  justify-content: flex-start;
  padding-left: 8px;
}
.tg-dh .wd {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tg-dh .dn {
  display: grid;
  place-items: center;
  min-width: 26px;
  height: 26px;
  padding: 0 4px;
  border-radius: 999px;
  font-size: 15px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  color: var(--text);
}
.tg-dh:not(:disabled):hover .dn {
  background: var(--fill);
}
.tg-dh.today .wd {
  color: var(--crit);
}
.tg-dh.today .dn,
.tg-dh.today:not(:disabled):hover .dn {
  background: var(--crit);
  color: #fff;
}
.tg-allday {
  border-top: 0.5px solid var(--border);
  border-bottom: 0.5px solid var(--border);
}
.tg-allday .lbl {
  padding: 6px 8px 0 0;
  text-align: right;
  font-size: 11px;
  color: var(--text-3);
}
.tg-strip {
  position: relative;
  flex: 1;
  min-width: 0;
  max-height: 128px;
  overflow-y: auto;
  background-image: linear-gradient(to right, var(--border) 0.5px, transparent 0.5px);
  background-size: calc(100% / var(--cols)) 100%;
}
.tg-bar,
.tg-task {
  position: absolute;
  display: flex;
  align-items: center;
  gap: 4px;
  height: 20px;
  padding: 0 6px;
  border-radius: 5px;
  font-size: 11.5px;
  font-weight: 500;
  cursor: default;
  user-select: none;
}
.tg-bar {
  background: color-mix(in srgb, var(--c) 22%, var(--bg));
  color: color-mix(in srgb, var(--c) 62%, var(--text));
}
.tg-bar.allday {
  background: var(--c);
  color: #fff;
}
.tg-bar.cs {
  border-top-left-radius: 0;
  border-bottom-left-radius: 0;
}
.tg-bar.ce {
  border-top-right-radius: 0;
  border-bottom-right-radius: 0;
}
.tg-bar:hover {
  filter: brightness(1.06);
}
.tg-task {
  padding-left: 4px;
  font-weight: 400;
  color: var(--text);
}
.tg-task:hover {
  background: var(--surface-hover);
}
.tg-task .check {
  width: 13px;
  height: 13px;
  border-width: 1.2px;
}
.tg-task.done > button:nth-child(2) {
  color: var(--text-3);
  text-decoration: line-through;
}
.tg-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-gutter: stable;
}
.tg-body {
  position: relative;
}
.hours {
  position: relative;
}
.hours span {
  position: absolute;
  right: 8px;
  transform: translateY(-50%);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
  color: var(--text-3);
  white-space: nowrap;
}
.hours .now-label {
  color: var(--crit);
  font-weight: 600;
}
.tg-cols {
  position: relative;
  background-image: linear-gradient(to bottom, var(--border) 0.5px, transparent 0.5px);
  border-top: 0;
}
.tg-col {
  position: relative;
  min-width: 0;
  box-shadow: inset 0.5px 0 0 var(--border);
  touch-action: pan-y;
}
.tg-col.creatable {
  cursor: cell;
}
.tg-ev {
  position: absolute;
  z-index: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  padding: 2px 5px 2px 7px;
  border-radius: 5px;
  font-size: 11.5px;
  line-height: 1.3;
  background: color-mix(in srgb, var(--c) 18%, var(--bg));
  color: color-mix(in srgb, var(--c) 58%, var(--text));
  box-shadow: inset 3px 0 0 var(--c), 0 0 0 0.5px var(--bg);
  cursor: default;
  user-select: none;
  transition: filter 0.15s;
}
.tg-ev:hover {
  filter: brightness(0.97);
  z-index: 2;
}
:global(html.dark) .tg-ev:hover {
  filter: brightness(1.15);
}
.tg-ev.ext {
  background: color-mix(in srgb, var(--c) 11%, var(--bg));
}
.tg-ev .ttl {
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
}
.tg-ev .tm,
.tg-ev .loc {
  opacity: 0.85;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-variant-numeric: tabular-nums;
}
.tg-ev.short {
  flex-direction: row;
  align-items: center;
  gap: 5px;
  padding-top: 0;
  padding-bottom: 0;
}
.tg-ev.short .ttl {
  white-space: nowrap;
  flex: 0 1 auto;
  min-width: 0;
}
.tg-sel {
  position: absolute;
  left: 0;
  right: 3px;
  z-index: 3;
  padding: 2px 6px;
  border-radius: 5px;
  font-size: 11.5px;
  font-weight: 600;
  background: var(--accent-soft);
  box-shadow: inset 3px 0 0 var(--accent);
  color: var(--accent-text);
  pointer-events: none;
}
.tg-now {
  position: absolute;
  left: 0;
  right: 0;
  z-index: 4;
  height: 0;
  border-top: 1.5px solid var(--crit);
  pointer-events: none;
}
.tg-now i {
  position: absolute;
  left: -5px;
  top: -5px;
  width: 9px;
  height: 9px;
  border-radius: 999px;
  background: var(--crit);
}
@media (max-width: 639px) {
  .tg {
    --gutter: 44px;
  }
  .tg-dh {
    flex-direction: column;
    gap: 1px;
    font-size: 11px;
  }
  .single .tg-dh {
    flex-direction: row;
  }
}
/* Where a dragged task will land. */
.tg-drop {
  position: absolute;
  left: 2px;
  right: 2px;
  border-radius: 6px;
  border: 1.5px dashed var(--accent);
  background: color-mix(in srgb, var(--accent) 10%, transparent);
  pointer-events: none;
}
.tg-strip.drop-on {
  background-image: linear-gradient(to right, transparent calc(var(--drop-col) * 100% / var(--cols)), color-mix(in srgb, var(--accent) 12%, transparent) calc(var(--drop-col) * 100% / var(--cols)), color-mix(in srgb, var(--accent) 12%, transparent) calc((var(--drop-col) + 1) * 100% / var(--cols)), transparent calc((var(--drop-col) + 1) * 100% / var(--cols)));
}
</style>
