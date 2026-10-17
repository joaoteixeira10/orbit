<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, ChevronRight, Plus } from 'lucide-vue-next'
import AvatarStack from '@/components/ui/AvatarStack.vue'
import TimelineScale from './TimelineScale.vue'
import TimelineBar from './TimelineBar.vue'
import { locale } from '@/lib/dates'
import { dragSpan, monthCells, numLocal, weekendStops, type DragMode, type Span, type TlGroup, type TlRow, type Zoom } from '@/lib/timeline'
import type { Task } from '@/api/planner'

// The Gantt chart itself: one scroll area (both ways) with a sticky header
// of dates and a sticky column of task names, grouped by list. Bars are
// placed by day number × pixels per day. Editors drag bars (or their edges)
// to reschedule, with Pointer Events so touch works too (there a short hold
// picks a bar up, so a swipe still scrolls); a tap opens the task.
const props = defineProps<{
  groups: TlGroup[]
  range: Span
  px: number
  zoom: Zoom
  today: number
  weekStart: number
  editable: boolean
  collapsed: Set<string>
  flash: string
}>()
const emit = defineEmits<{
  toggle: [task: Task]
  open: [id: string]
  schedule: [task: Task, span: Span]
  create: [listId: string, title: string]
  collapse: [listId: string]
  extend: [side: 'before' | 'after']
}>()
const { t } = useI18n()

const phoneMq = window.matchMedia('(max-width: 767px)')
const phone = ref(phoneMq.matches)
const onMq = (e: MediaQueryListEvent) => (phone.value = e.matches)
phoneMq.addEventListener('change', onMq)
const nameW = computed(() => (phone.value ? 150 : 280))
const W = computed(() => (props.range.to - props.range.from + 1) * props.px)
const x = (n: number) => (n - props.range.from) * props.px

// ---- Geometry ----
// A bar is inset a little within its days, and never thinner than 10px.
const inset = computed(() => (props.zoom === 'weeks' ? 3 : props.zoom === 'months' ? 2 : 0.5))
function geom(s: Span) {
  let width = (s.to - s.from + 1) * props.px - 2 * inset.value
  let left = x(s.from) + inset.value
  if (width < 10) {
    left -= (10 - width) / 2
    width = 10
  }
  return { left, width }
}

// The grid behind the rows: weekends, day or week lines, month lines, today.
const bgStyle = computed(() => {
  const px = props.px
  const layers: string[] = []
  const sizes: string[] = []
  if (props.zoom !== 'quarters') {
    const wk = weekendStops(props.range.from, px, 'var(--tl-weekend)')
    if (wk) {
      layers.push(wk)
      sizes.push(`${7 * px}px 100%`)
    }
  }
  if (props.zoom === 'weeks') {
    layers.push('linear-gradient(to right, var(--tl-line) 1px, transparent 1px)')
    sizes.push(`${px}px 100%`)
  } else if (props.zoom === 'months') {
    layers.push('linear-gradient(to right, var(--tl-line) 1px, transparent 1px)')
    sizes.push(`${7 * px}px 100%`)
  }
  return { left: `${nameW.value}px`, width: `${W.value}px`, backgroundImage: layers.join(', '), backgroundSize: sizes.join(', ') }
})
const monthLines = computed(() => monthCells(props.range).slice(1).map((c) => x(c.at)))
const todayIn = computed(() => props.today >= props.range.from && props.today <= props.range.to)

// ---- Scrolling ----
const scroller = ref<HTMLElement | null>(null)
const scrolled = ref(false)
const gridW = () => (scroller.value ? scroller.value.clientWidth - nameW.value : 0)
// scrollToDay puts day n at `at` (a fraction) of the visible grid.
function scrollToDay(n: number, at = 0.25, smooth = false) {
  const el = scroller.value
  if (!el) return
  el.scrollTo({ left: Math.max(0, x(n) + props.px / 2 - gridW() * at), behavior: smooth ? 'smooth' : 'auto' })
}
defineExpose({ scrollToDay })

// When the range grows or the zoom changes, keep today where it was (when
// it's in view), else the day in the middle.
let asked = false // more days were asked for and haven't come yet
let keep: { day: number; at: number } | null = null
watch(
  () => [props.range.from, props.px] as const,
  (_, [from, px]) => {
    const el = scroller.value
    if (el && !keep) {
      const w = gridW()
      const at = ((props.today - from + 0.5) * px - el.scrollLeft) / w
      keep = at > 0.03 && at < 0.97 ? { day: props.today + 0.5, at } : { day: from + (el.scrollLeft + w / 2) / px, at: 0.5 }
    }
    void nextTick(() => {
      if (keep && scroller.value) scroller.value.scrollLeft = (keep.day - props.range.from) * props.px - gridW() * keep.at
      keep = null
      asked = false
    })
  },
  { flush: 'pre' },
)
watch(
  () => props.range.to,
  () => void nextTick(() => (asked = false)),
)
// Near either end, ask for more days.
function onScroll() {
  const el = scroller.value
  if (!el) return
  scrolled.value = el.scrollLeft > 2
  if (asked) return
  const margin = Math.max(400, gridW())
  if (el.scrollLeft < margin) {
    asked = true
    emit('extend', 'before')
  } else if (el.scrollLeft + el.clientWidth > el.scrollWidth - margin) {
    asked = true
    emit('extend', 'after')
  }
}
onMounted(() => {
  void nextTick(() => {
    scrollToDay(props.today)
    asked = false
  })
})

// ---- Dragging bars ----
interface Drag {
  row: TlRow
  span: Span
  mode: DragMode
  pointerId: number
  x0: number
  y0: number
  sl0: number
  cx: number
  cy: number
  delta: number
  armed: boolean // the bar follows the pointer (at once with a mouse, after a hold on touch)
  moved: boolean
  touch: boolean
  timer: number
}
const drag = ref<Drag | null>(null)
const preview = computed(() => (drag.value?.armed ? dragSpan(drag.value.span, drag.value.mode, drag.value.delta) : null))
const spanFor = (r: TlRow) => (preview.value && drag.value?.row.task.id === r.task.id ? preview.value : r.span)

function grab(r: TlRow, mode: DragMode, e: PointerEvent) {
  if (e.pointerType === 'mouse' && e.button !== 0) return
  if (!props.editable || !r.span) {
    // Viewers: a click opens it.
    if (e.pointerType === 'mouse') e.preventDefault()
    drag.value = null
    pending = r
    return
  }
  e.preventDefault()
  const touch = e.pointerType !== 'mouse'
  const d: Drag = {
    row: r,
    span: r.span,
    mode,
    pointerId: e.pointerId,
    x0: e.clientX,
    y0: e.clientY,
    sl0: scroller.value?.scrollLeft ?? 0,
    cx: e.clientX,
    cy: e.clientY,
    delta: 0,
    // An edge has touch-action: none, so it can be dragged at once.
    armed: !touch || mode !== 'move',
    moved: false,
    touch,
    timer: 0,
  }
  drag.value = d
  const cur = drag.value
  if (!cur.armed) {
    cur.timer = window.setTimeout(() => {
      if (drag.value === cur) {
        cur.armed = true
        navigator.vibrate?.(8)
      }
    }, 320)
  }
}
// A viewer's click on a bar.
let pending: TlRow | null = null

function onMove(e: PointerEvent) {
  const d = drag.value
  if (!d || e.pointerId !== d.pointerId) return
  d.cx = e.clientX
  d.cy = e.clientY
  if (!d.armed) {
    // A swipe before the hold: let it scroll.
    if (Math.hypot(e.clientX - d.x0, e.clientY - d.y0) > 8) cancel()
    return
  }
  update()
  autoScroll()
}
function update() {
  const d = drag.value
  if (!d) return
  const dx = d.cx - d.x0 + (scroller.value?.scrollLeft ?? 0) - d.sl0
  if (!d.moved && Math.abs(dx) > 3) d.moved = true
  if (d.moved) d.delta = Math.round(dx / props.px)
}
function onUp(e: PointerEvent) {
  if (pending) {
    const r = pending
    pending = null
    emit('open', r.task.id)
    return
  }
  const d = drag.value
  if (!d || e.pointerId !== d.pointerId) return
  const next = preview.value
  cancel()
  if (!d.moved) emit('open', d.row.task.id)
  else if (next && d.span && (next.from !== d.span.from || next.to !== d.span.to)) emit('schedule', d.row.task, next)
}
function cancel() {
  if (drag.value) clearTimeout(drag.value.timer)
  drag.value = null
  pending = null
  stopAuto()
}
// Once a bar is picked up on touch, the finger drags it instead of scrolling.
function onTouchMove(e: TouchEvent) {
  if (drag.value?.armed) e.preventDefault()
}

// Near the left or right of the grid while dragging, it scrolls along.
let raf = 0
function autoScroll() {
  const el = scroller.value
  const d = drag.value
  if (!el || !d) return
  const r = el.getBoundingClientRect()
  const left = r.left + nameW.value
  const speed = d.cx < left + 40 ? -Math.ceil((left + 40 - d.cx) / 4) : d.cx > r.right - 40 ? Math.ceil((d.cx - (r.right - 40)) / 4) : 0
  if (!speed) return stopAuto()
  if (raf) return
  const step = () => {
    if (!drag.value || !scroller.value) return stopAuto()
    const rr = scroller.value.getBoundingClientRect()
    const dd = drag.value
    const s = dd.cx < rr.left + nameW.value + 40 ? -Math.ceil((rr.left + nameW.value + 40 - dd.cx) / 4) : dd.cx > rr.right - 40 ? Math.ceil((dd.cx - (rr.right - 40)) / 4) : 0
    if (!s) return stopAuto()
    scroller.value.scrollLeft += Math.min(24, Math.max(-24, s))
    update()
    raf = requestAnimationFrame(step)
  }
  raf = requestAnimationFrame(step)
}
function stopAuto() {
  if (raf) cancelAnimationFrame(raf)
  raf = 0
}

onMounted(() => {
  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
  window.addEventListener('pointercancel', cancel)
  window.addEventListener('touchmove', onTouchMove, { passive: false })
})
onBeforeUnmount(() => {
  window.removeEventListener('pointermove', onMove)
  window.removeEventListener('pointerup', onUp)
  window.removeEventListener('pointercancel', cancel)
  window.removeEventListener('touchmove', onTouchMove)
  phoneMq.removeEventListener('change', onMq)
  cancel()
})

// The dates while dragging (and on a bar's label).
const fmt = computed(() => new Intl.DateTimeFormat(locale(), { weekday: 'short', day: 'numeric', month: 'short' }))
function spanText(s: Span) {
  const a = fmt.value.format(numLocal(s.from))
  return s.from === s.to ? a : `${a} – ${fmt.value.format(numLocal(s.to))}`
}
const tip = computed(() => {
  const d = drag.value
  const s = preview.value
  if (!d || !s || !d.moved) return null
  // Kept inside the window.
  return { x: Math.min(Math.max(d.cx, 110), window.innerWidth - 110), y: Math.max(d.cy, 70), dates: spanText(s), days: t('timeline.days', { n: s.to - s.from + 1 }, s.to - s.from + 1), mode: d.mode }
})
const cursor = computed(() => (tip.value ? (tip.value.mode === 'move' ? 'grabbing' : 'ew-resize') : ''))
watch(cursor, (c) => (document.documentElement.style.cursor = c))
onBeforeUnmount(() => (document.documentElement.style.cursor = ''))

function nudge(r: TlRow, mode: DragMode, delta: number) {
  if (r.span) emit('schedule', r.task, dragSpan(r.span, mode, delta))
}

// ---- Unscheduled tasks: a ghost day under the pointer; a click schedules it there ----
const ghost = ref<{ id: string; day: number } | null>(null)
const ghostBox = computed(() => (ghost.value ? geom({ from: ghost.value.day, to: ghost.value.day }) : null))
function dayAt(e: MouseEvent, el: HTMLElement) {
  return props.range.from + Math.floor((e.clientX - el.getBoundingClientRect().left) / props.px)
}
function hover(r: TlRow, e: PointerEvent) {
  if (!props.editable || r.span || e.pointerType !== 'mouse' || drag.value) return
  ghost.value = { id: r.task.id, day: dayAt(e, e.currentTarget as HTMLElement) }
}
function place(r: TlRow, e: MouseEvent) {
  if (!props.editable || r.span) return
  const day = dayAt(e, e.currentTarget as HTMLElement)
  ghost.value = null
  emit('schedule', r.task, { from: day, to: day })
}

// ---- Adding a task at the end of a list ----
const adding = ref('')
const draft = ref('')
const addInput = ref<HTMLInputElement[] | HTMLInputElement | null>(null)
async function startAdd(listId: string) {
  adding.value = listId
  draft.value = ''
  await nextTick()
  const el = Array.isArray(addInput.value) ? addInput.value[0] : addInput.value
  el?.focus()
}
function commitAdd(listId: string) {
  const title = draft.value.trim()
  if (!title) {
    adding.value = ''
    return
  }
  emit('create', listId, title)
  draft.value = ''
}
</script>

<template>
  <div
    ref="scroller"
    class="tl"
    :class="{ scrolled, phone, 'is-dragging': !!tip }"
    :style="{ '--name-w': `${nameW}px` }"
    @scroll.passive="onScroll"
  >
    <div class="tl-inner" :style="{ width: `${nameW + W}px` }">
      <!-- Dates -->
      <div class="tl-head">
        <div class="tl-corner">
          <span class="cap">{{ t('timeline.tasksCol') }}</span>
        </div>
        <TimelineScale :range="range" :px="px" :zoom="zoom" :today="today" :week-start="weekStart" :sel="tip ? preview : null" />
      </div>

      <div class="tl-body">
        <!-- The grid -->
        <div class="tl-bg" :style="bgStyle" aria-hidden="true">
          <i v-for="l in monthLines" :key="l" class="month-line" :style="{ left: `${l}px` }" />
          <i v-if="todayIn && zoom === 'weeks'" class="today-col" :style="{ left: `${x(today)}px`, width: `${px}px` }" />
          <i v-if="drag?.armed && preview" class="drag-col" :style="{ left: `${x(preview.from)}px`, width: `${(preview.to - preview.from + 1) * px}px` }" />
          <i v-if="todayIn" class="today-line" :style="{ left: `${x(today) + px / 2}px` }" />
        </div>

        <template v-for="g in groups" :key="g.list.id">
          <!-- The list -->
          <div class="tl-row grp">
            <div class="nm">
              <button
                class="grp-btn"
                :aria-expanded="!collapsed.has(g.list.id)"
                :aria-label="t(collapsed.has(g.list.id) ? 'timeline.expand' : 'timeline.collapse', { name: g.list.name })"
                @click="emit('collapse', g.list.id)"
              >
                <ChevronRight class="chev h-3.5 w-3.5 shrink-0" :class="{ open: !collapsed.has(g.list.id) }" :stroke-width="2.6" />
                <span class="h-2.5 w-2.5 shrink-0 rounded-full" :style="{ background: g.list.color }" />
                <span class="min-w-0 truncate">{{ g.list.name }}</span>
                <span class="count">{{ g.rows.length }}</span>
              </button>
            </div>
            <div class="tr">
              <i v-if="g.span" class="summary" :style="{ left: `${x(g.span.from)}px`, width: `${(g.span.to - g.span.from + 1) * px}px`, '--c': g.list.color }" />
            </div>
          </div>

          <template v-if="!collapsed.has(g.list.id)">
            <!-- Its tasks -->
            <div v-for="r in g.rows" :key="r.task.id" class="tl-row task" :class="{ 'row-flash': flash === r.task.id, lifted: drag?.row.task.id === r.task.id }">
              <div class="nm">
                <button
                  class="check"
                  :class="{ on: r.task.done }"
                  :style="{ '--check': g.list.color }"
                  role="checkbox"
                  :aria-checked="r.task.done"
                  :aria-label="r.task.title"
                  :disabled="!editable"
                  @click="emit('toggle', r.task)"
                >
                  <Check class="h-3 w-3" :stroke-width="3.4" />
                </button>
                <button class="nm-title" :class="{ done: r.task.done }" :title="r.task.title" @click="emit('open', r.task.id)">{{ r.task.title }}</button>
                <span v-if="!r.span && !phone" class="nodate">{{ t('timeline.noDate') }}</span>
                <AvatarStack v-if="r.assignees.length" :people="r.assignees" :size="18" />
              </div>
              <div
                class="tr"
                :class="{ placeable: editable && !r.span }"
                @pointermove="hover(r, $event)"
                @pointerleave="ghost = null"
                @click.self="place(r, $event)"
              >
                <TimelineBar
                  v-if="spanFor(r)"
                  v-bind="geom(spanFor(r)!)"
                  :title="r.task.title"
                  :label="`${r.task.title}: ${spanText(spanFor(r)!)}`"
                  :color="g.list.color"
                  :done="r.task.done"
                  :overdue="r.overdue"
                  :subs="r.subs"
                  :editable="editable"
                  :dragging="!!drag?.armed && drag.row.task.id === r.task.id"
                  @grab="(m, e) => grab(r, m, e)"
                  @open="emit('open', r.task.id)"
                  @nudge="(m, d) => nudge(r, m, d)"
                />
                <span
                  v-else-if="ghost?.id === r.task.id"
                  class="ghost"
                  :style="{ left: `${ghostBox!.left}px`, width: `${ghostBox!.width}px`, '--c': g.list.color }"
                  :title="t('timeline.scheduleOn', { date: spanText({ from: ghost.day, to: ghost.day }) })"
                >
                  <Plus v-if="px >= 16" class="h-3.5 w-3.5" :stroke-width="2.4" />
                </span>
              </div>
            </div>

            <!-- New task -->
            <div v-if="editable" class="tl-row add">
              <div class="nm">
                <template v-if="adding === g.list.id">
                  <span class="check pointer-events-none" :style="{ '--check': g.list.color }" />
                  <input
                    ref="addInput"
                    v-model="draft"
                    class="min-w-0 flex-1 bg-transparent text-[13.5px] outline-none"
                    :placeholder="t('tasks.newTask')"
                    maxlength="500"
                    @keydown.enter="commitAdd(g.list.id)"
                    @keydown.esc="adding = ''"
                    @blur="draft.trim() ? commitAdd(g.list.id) : (adding = '')"
                  />
                </template>
                <button v-else class="add-btn" @click="startAdd(g.list.id)">
                  <Plus class="h-[18px] w-[18px] shrink-0" :style="{ color: g.list.color }" />{{ t('tasks.newTask') }}
                </button>
              </div>
              <div class="tr" />
            </div>
          </template>
        </template>

        <!-- The rest of the height -->
        <div class="tl-row filler"><div class="nm" /><div class="tr" /></div>
      </div>
    </div>

    <Teleport to="body">
      <div v-if="tip" class="tl-tip" :style="{ left: `${tip.x}px`, top: `${tip.y}px` }">
        <span class="font-semibold">{{ tip.dates }}</span>
        <span class="text-text-3">{{ tip.days }}</span>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.tl {
  --tl-weekend: rgba(120, 120, 128, 0.055);
  --tl-line: rgba(60, 60, 67, 0.07);
  --tl-group: color-mix(in srgb, var(--text) 3%, var(--bg));
  --tl-hover: color-mix(in srgb, var(--text) 2.5%, var(--bg));
  --row: 36px;
  position: relative;
  overflow: auto;
  overscroll-behavior: contain;
  background: var(--bg);
}
html.dark .tl {
  --tl-weekend: rgba(0, 0, 0, 0.16);
  --tl-line: rgba(84, 84, 88, 0.22);
  --tl-group: color-mix(in srgb, var(--text) 4%, var(--bg));
}
.tl-inner {
  position: relative;
  display: flex;
  flex-direction: column;
  min-height: 100%;
}

/* ---- Header ---- */
.tl-head {
  position: sticky;
  top: 0;
  z-index: 6;
  display: flex;
  height: 60px;
  flex: none;
  background: color-mix(in srgb, var(--bg) 86%, transparent);
  backdrop-filter: saturate(180%) blur(20px);
  -webkit-backdrop-filter: saturate(180%) blur(20px);
  box-shadow: inset 0 -0.5px 0 var(--border);
}
.tl-corner {
  position: sticky;
  left: 0;
  z-index: 2;
  flex: none;
  display: flex;
  align-items: flex-end;
  width: var(--name-w);
  padding: 0 14px 9px;
  background: var(--bg);
  box-shadow: inset -0.5px -0.5px 0 var(--border);
}

/* ---- Body ---- */
.tl-body {
  position: relative;
  flex: 1;
  display: flex;
  flex-direction: column;
}
.tl-bg {
  position: absolute;
  top: 0;
  bottom: 0;
  background-repeat: repeat-x;
  pointer-events: none;
}
.tl-bg i {
  position: absolute;
  top: 0;
  bottom: 0;
}
.month-line {
  width: 0;
  border-left: 0.5px solid var(--border);
}
.today-col {
  background: color-mix(in srgb, var(--crit) 5%, transparent);
}
.drag-col {
  background: var(--accent-bg);
  opacity: 0.35;
}
.today-line {
  z-index: 2;
  width: 0;
  border-left: 1.5px solid var(--crit);
  margin-left: -0.75px;
}
.today-line::before {
  content: '';
  position: absolute;
  top: -4px;
  left: -5.25px;
  width: 9px;
  height: 9px;
  border-radius: 999px;
  background: var(--crit);
}

.tl-row {
  position: relative;
  display: flex;
  flex: none;
  height: var(--row);
}
.nm {
  position: sticky;
  left: 0;
  z-index: 4;
  flex: none;
  display: flex;
  align-items: center;
  gap: 9px;
  width: var(--name-w);
  padding: 0 12px 0 14px;
  background: var(--bg);
  box-shadow: inset -0.5px 0 0 var(--border), inset 0 -0.5px 0 var(--line-soft);
  transition: box-shadow var(--speed) var(--ease);
}
/* Lifted over the grid once it scrolls beneath. */
.scrolled .nm::after,
.scrolled .tl-corner::after {
  content: '';
  position: absolute;
  top: 0;
  right: -8px;
  bottom: 0;
  width: 8px;
  background: linear-gradient(to right, rgba(0, 0, 0, 0.05), transparent);
  pointer-events: none;
}
html.dark .scrolled .nm::after,
html.dark .scrolled .tl-corner::after {
  background: linear-gradient(to right, rgba(0, 0, 0, 0.35), transparent);
}
.tr {
  position: relative;
  flex: 1;
  box-shadow: inset 0 -0.5px 0 var(--line-soft);
}

/* A list's row. */
.grp .nm,
.grp .tr {
  background: var(--tl-group);
}
.grp-btn {
  display: flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
  width: 100%;
  height: 100%;
  margin-left: -4px;
  font-size: 13px;
  font-weight: 600;
  text-align: left;
  color: var(--text);
}
.chev {
  color: var(--text-3);
  transition: transform 0.2s var(--spring);
}
.chev.open {
  transform: rotate(90deg);
}
.count {
  margin-left: auto;
  padding-left: 6px;
  font-weight: 500;
  font-size: 12px;
  color: var(--text-3);
  font-variant-numeric: tabular-nums;
}
.summary {
  position: absolute;
  top: 50%;
  height: 6px;
  margin-top: -3px;
  border-radius: 3px;
  background: color-mix(in srgb, var(--c) 45%, transparent);
}

/* A task's row. */
.task:hover .nm,
.task.lifted .nm {
  background: var(--tl-hover);
}
.task:hover .tr,
.task.lifted .tr {
  background: color-mix(in srgb, var(--text) 2.5%, transparent);
}
.nm-title {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  font-size: 13.5px;
  text-align: left;
  white-space: nowrap;
  text-overflow: ellipsis;
  color: var(--text);
  cursor: default;
}
.nm-title.done {
  color: var(--text-3);
}
.nodate {
  flex: none;
  font-size: 11.5px;
  color: var(--text-faint);
}
.placeable {
  cursor: copy;
}
.ghost {
  position: absolute;
  top: 7px;
  height: 22px;
  display: grid;
  place-items: center;
  border-radius: 7px;
  border: 1px dashed color-mix(in srgb, var(--c) 70%, transparent);
  background: color-mix(in srgb, var(--c) 10%, transparent);
  color: var(--c);
  pointer-events: none;
}

/* New task. */
.add .nm,
.filler .nm {
  box-shadow: inset -0.5px 0 0 var(--border);
}
.add .tr,
.filler .tr {
  box-shadow: none;
}
.add-btn {
  display: flex;
  align-items: center;
  gap: 9px;
  margin-left: 1px;
  font-size: 13.5px;
  color: var(--text-3);
}
.add-btn:hover {
  color: var(--text-2);
}
.filler {
  flex: 1;
  min-height: 24px;
  height: auto;
}

.phone .nm {
  gap: 8px;
  padding: 0 8px 0 12px;
}
.phone .nm-title {
  font-size: 13px;
}

/* The dates while dragging. */
.tl-tip {
  position: fixed;
  z-index: 100;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 5px 10px;
  border-radius: 9px;
  transform: translate(-50%, calc(-100% - 18px));
  background: var(--surface);
  box-shadow: var(--shadow-lg);
  font-size: 12px;
  line-height: 1.35;
  white-space: nowrap;
  pointer-events: none;
  color: var(--text);
}
html.dark .tl-tip {
  background: #3a3a3c;
}
.is-dragging {
  user-select: none;
}
</style>
