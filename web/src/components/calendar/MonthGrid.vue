<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, Repeat } from 'lucide-vue-next'
import { monthGrid, weekdayNames, dateStr, sameDay, parseDate, addDays, fmtDate, fmtTime, fmtClock } from '@/lib/dates'
import { layoutBars, isBar, onDay, sortEntries, tasksByDay, daysBetween, eventColor, type Entry, type TaskItem } from './model'
import { startTaskDrag, isTaskDrag, readTaskDrag, type DraggedTask } from '@/lib/taskDrag'


// The month as Calendar shows it: six weeks, today in a red circle, bars for
// all-day and multi-day events across the days they cover, timed events as
// a dot, a time and a title, due tasks with their round check, and "+N more"
// when a day is full. Single Orbit events drag to another day. compact is
// the phone layout: numbers and coloured dots, the day's list goes below.
const props = defineProps<{
  month: Date
  entries: Entry[]
  tasks: TaskItem[]
  selected: string
  compact?: boolean
  canCreate?: boolean
}>()
const emit = defineEmits<{
  select: [day: string]
  openDay: [day: string]
  open: [entry: Entry]
  openTask: [item: TaskItem]
  toggleTask: [item: TaskItem]
  create: [day: string]
  move: [entry: Entry, days: number]
  dropTask: [task: DraggedTask, day: string]
}>()
const { t } = useI18n()

const LINE = 20 // px per line (18 + 2 gap)
const HEAD = 28 // the day number row

const today = ref(new Date())
let tick: ReturnType<typeof setInterval> | null = null

const names = computed(() => weekdayNames(props.compact ? 'narrow' : 'short'))
const weeks = computed(() => {
  const days = monthGrid(props.month)
  const m = props.month.getMonth()
  return Array.from({ length: 6 }, (_, w) =>
    days.slice(w * 7, w * 7 + 7).map((d) => ({
      date: d,
      key: dateStr(d),
      out: d.getMonth() !== m,
      today: sameDay(d, today.value),
      weekend: d.getDay() === 0 || d.getDay() === 6,
    })),
  )
})
const taskMap = computed(() => tasksByDay(props.tasks))

// ---- How many lines fit in a day (desktop) ----
const body = ref<HTMLElement | null>(null)
const rowH = ref(110)
let ro: ResizeObserver | null = null
onMounted(() => {
  tick = setInterval(() => (today.value = new Date()), 60000)
  if (body.value && typeof ResizeObserver !== 'undefined') {
    ro = new ResizeObserver(([e]) => (rowH.value = e.contentRect.height / 6))
    ro.observe(body.value)
  }
})
onBeforeUnmount(() => {
  if (tick) clearInterval(tick)
  ro?.disconnect()
})
const maxLines = computed(() => Math.max(1, Math.floor((rowH.value - HEAD - 4) / LINE)))

type Slot =
  | { kind: 'bar'; entry: Entry; span: number; clipStart: boolean; clipEnd: boolean; from: string }
  | { kind: 'gap' }
  | { kind: 'event'; entry: Entry }
  | { kind: 'task'; item: TaskItem }

// Each week: bars on lanes, then each day's timed events and tasks in the
// lanes left free; what doesn't fit becomes "+N more". A bar is cut where a
// day hides its lane, so it never runs over a "+N more".
const layout = computed(() => {
  if (props.compact) return []
  const max = maxLines.value
  return weeks.value.map((week) => {
    const keys = week.map((d) => d.key)
    const { bars } = layoutBars(props.entries, keys)
    const taken = keys.map(() => new Set<number>())
    for (const b of bars) for (let c = b.col; c < b.col + b.span; c++) taken[c].add(b.lane)
    const singles = keys.map((k, c) => {
      const list: ({ kind: 'event'; entry: Entry } | { kind: 'task'; item: TaskItem })[] = [
        ...sortEntries(props.entries.filter((e) => !isBar(e) && e.first === k)).map((entry) => ({ kind: 'event' as const, entry })),
        ...(taskMap.value.get(k) ?? []).map((item) => ({ kind: 'task' as const, item })),
      ]
      let lane = 0
      return list.map((s) => {
        while (taken[c].has(lane)) lane++
        return { ...s, lane: lane++ }
      })
    })
    const visible = keys.map((_, c) => {
      const used = Math.max(0, ...[...taken[c]].map((l) => l + 1), ...singles[c].map((s) => s.lane + 1))
      return used > max ? max - 1 : max
    })
    const slots: Slot[][] = keys.map((_, c) => Array.from({ length: visible[c] }, () => ({ kind: 'gap' }) as Slot))
    for (const b of bars) {
      let c = b.col
      const end = b.col + b.span - 1
      while (c <= end) {
        if (b.lane >= visible[c]) {
          c++
          continue
        }
        const from = c
        while (c + 1 <= end && b.lane < visible[c + 1]) c++
        slots[from][b.lane] = {
          kind: 'bar',
          entry: b.entry,
          span: c - from + 1,
          clipStart: from > b.col || b.clipStart,
          clipEnd: c < end || b.clipEnd,
          from: keys[from],
        }
        c++
      }
    }
    const more = keys.map((_, c) => {
      let n = singles[c].filter((s) => s.lane >= visible[c]).length
      for (const l of taken[c]) if (l >= visible[c]) n++
      return n
    })
    singles.forEach((list, c) => {
      for (const s of list) if (s.lane < visible[c]) slots[c][s.lane] = s.kind === 'event' ? { kind: 'event', entry: s.entry } : { kind: 'task', item: s.item }
    })
    // Trailing gaps take no room.
    for (const s of slots) while (s.length && s[s.length - 1].kind === 'gap') s.pop()
    return { slots, more }
  })
})

// ---- Phone: dots under the number ----
function dots(key: string): string[] {
  const colors = props.entries.filter((e) => onDay(e, key)).map(eventColor)
  for (const it of taskMap.value.get(key) ?? []) if (!it.task.done) colors.push(it.color)
  return [...new Set(colors)].slice(0, 3)
}

// ---- Drag to another day ----
const drag = ref<{ entry: Entry; grab: string } | null>(null)
const dropDay = ref('')
function onDragStart(ev: DragEvent, entry: Entry, from: string, span: number) {
  if (!entry.movable || !ev.dataTransfer) return
  // Which day of a long bar was grabbed: the drop moves that day there.
  const r = (ev.currentTarget as HTMLElement).getBoundingClientRect()
  const idx = span > 1 ? Math.min(span - 1, Math.max(0, Math.floor(((ev.clientX - r.left) / r.width) * span))) : 0
  drag.value = { entry, grab: dateStr(addDays(parseDate(from), idx)) }
  ev.dataTransfer.effectAllowed = 'move'
  ev.dataTransfer.setData('text/plain', entry.key)
}
function onDragOver(ev: DragEvent, key: string) {
  if (!drag.value && !isTaskDrag(ev)) return
  ev.preventDefault()
  if (ev.dataTransfer) ev.dataTransfer.dropEffect = 'move'
  dropDay.value = key
}
function onDrop(ev: DragEvent, key: string) {
  if (!drag.value) {
    const task = isTaskDrag(ev) ? readTaskDrag(ev) : null
    if (task) {
      ev.preventDefault()
      emit('dropTask', task, key)
    }
    endDrag()
    return
  }
  ev.preventDefault()
  const n = daysBetween(drag.value.grab, key)
  if (n) emit('move', drag.value.entry, n)
  endDrag()
}
function endDrag() {
  drag.value = null
  dropDay.value = ''
}

function timeOf(e: Entry) {
  return fmtTime(e.start)
}
function barStyle(s: Extract<Slot, { kind: 'bar' }>) {
  const ml = s.clipStart ? 0 : 4
  const mr = s.clipEnd ? 0 : 4
  return { '--c': eventColor(s.entry), marginLeft: `${ml}px`, width: `calc(${s.span} * 100% - ${ml + mr}px)` }
}
</script>

<template>
  <div class="mg" :class="{ compact }">
    <div class="mg-names" aria-hidden="true">
      <div v-for="(n, i) in names" :key="i" :class="{ weekend: weeks[0][i].weekend }">{{ n }}</div>
    </div>

    <!-- Phone: numbers and dots -->
    <div v-if="compact" class="mg-body" role="grid">
      <div v-for="(week, w) in weeks" :key="w" class="mg-week" role="row">
        <button
          v-for="d in week"
          :key="d.key"
          class="mg-cday"
          role="gridcell"
          :class="{ out: d.out, today: d.today, sel: d.key === selected }"
          :aria-selected="d.key === selected"
          :aria-label="fmtDate(d.date, { weekday: 'long', day: 'numeric', month: 'long' })"
          @click="emit('select', d.key)"
        >
          <span class="num">{{ d.date.getDate() }}</span>
          <span class="dots">
            <i v-for="c in dots(d.key)" :key="c" :style="{ background: c }" />
          </span>
        </button>
      </div>
    </div>

    <!-- Desktop -->
    <div v-else ref="body" class="mg-body" role="grid">
      <div v-for="(week, w) in weeks" :key="w" class="mg-week" role="row">
        <div
          v-for="(d, c) in week"
          :key="d.key"
          class="mg-cell"
          role="gridcell"
          :data-day="d.key"
          :class="{ out: d.out, weekend: d.weekend, drop: dropDay === d.key }"
          @click="emit('select', d.key)"
          @dblclick="canCreate && emit('create', d.key)"
          @dragover="onDragOver($event, d.key)"
          @drop="onDrop($event, d.key)"
        >
          <div class="mg-head">
            <button class="mg-num" :class="{ today: d.today, sel: d.key === selected && !d.today }" :title="t('cal.openDay')" @click.stop="emit('openDay', d.key)">
              {{ d.date.getDate() === 1 && !d.today ? fmtDate(d.date, { day: 'numeric', month: 'short' }) : d.date.getDate() }}
            </button>
          </div>
          <div class="mg-lines">
            <template v-for="(s, i) in layout[w]?.slots[c] ?? []" :key="i">
              <div
                v-if="s.kind === 'bar'"
                class="mg-bar"
                :class="{ cs: s.clipStart, ce: s.clipEnd, allday: s.entry.allDay, ext: s.entry.occ.kind === 'external', dragging: drag?.entry.key === s.entry.key }"
                :style="barStyle(s)"
                :draggable="s.entry.movable"
                role="button"
                tabindex="0"
                :title="s.entry.occ.title"
                @click.stop="emit('open', s.entry)"
                @keydown.enter="emit('open', s.entry)"
                @dblclick.stop
                @dragstart="onDragStart($event, s.entry, s.from, s.span)"
                @dragend="endDrag"
              >
                <span v-if="!s.entry.allDay && !s.clipStart" class="mg-time">{{ timeOf(s.entry) }}</span>
                <span class="truncate">{{ s.entry.occ.title || t('cal.untitled') }}</span>
                <Repeat v-if="s.entry.occ.recurring" class="h-2.5 w-2.5 shrink-0 opacity-70" />
              </div>
              <div v-else-if="s.kind === 'gap'" class="mg-gap" />
              <div
                v-else-if="s.kind === 'event'"
                class="mg-ev"
                :class="{ ext: s.entry.occ.kind === 'external', dragging: drag?.entry.key === s.entry.key }"
                :style="{ '--c': eventColor(s.entry) }"
                :draggable="s.entry.movable"
                role="button"
                tabindex="0"
                :title="`${timeOf(s.entry)} ${s.entry.occ.title}`"
                @click.stop="emit('open', s.entry)"
                @keydown.enter="emit('open', s.entry)"
                @dblclick.stop
                @dragstart="onDragStart($event, s.entry, d.key, 1)"
                @dragend="endDrag"
              >
                <i class="dot" />
                <span class="min-w-0 flex-1 truncate">{{ s.entry.occ.title || t('cal.untitled') }}</span>
                <span class="mg-time">{{ timeOf(s.entry) }}</span>
              </div>
              <div
                v-else
                class="mg-task"
                :class="{ done: s.item.task.done }"
                :draggable="s.item.editable && !s.item.task.done"
                @click.stop
                @dblclick.stop
                @dragstart="startTaskDrag($event, s.item.task)"
                @dragend="endDrag"
              >
                <button
                  class="check"
                  :class="{ on: s.item.task.done }"
                  :style="{ '--check': s.item.color }"
                  role="checkbox"
                  :aria-checked="s.item.task.done"
                  :aria-label="s.item.task.title"
                  :disabled="!s.item.editable"
                  @click="emit('toggleTask', s.item)"
                >
                  <Check class="h-2 w-2" :stroke-width="4" />
                </button>
                <button class="min-w-0 flex-1 truncate text-left" :title="s.item.task.title" @click="emit('openTask', s.item)">{{ s.item.task.title }}</button>
                <span v-if="s.item.task.due_time" class="mg-time">{{ fmtClock(s.item.task.due_time) }}</span>
              </div>
            </template>
            <button v-if="layout[w]?.more[c]" class="mg-more" @click.stop="emit('openDay', d.key)">{{ t('cal.more', { n: layout[w].more[c] }) }}</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.mg {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-width: 0;
}
.mg-names {
  display: grid;
  grid-template-columns: repeat(7, minmax(0, 1fr));
  flex: none;
  border-bottom: 0.5px solid var(--border);
}
.mg-names > div {
  padding: 0 8px 6px;
  text-align: right;
  font-size: 12px;
  font-weight: 500;
  color: var(--text-2);
}
.mg-names > .weekend {
  color: var(--text-3);
}
.mg-body {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-rows: repeat(6, minmax(0, 1fr));
}
.mg-week {
  display: grid;
  grid-template-columns: repeat(7, minmax(0, 1fr));
  min-height: 0;
  border-bottom: 0.5px solid var(--border);
}
.mg-week:last-child {
  border-bottom: 0;
}
.mg-cell {
  position: relative;
  min-width: 0;
  min-height: 0;
  overflow: visible;
  box-shadow: inset 0.5px 0 0 var(--border);
  transition: background 0.15s var(--ease);
}
.mg-cell:first-child {
  box-shadow: none;
}
.mg-cell.weekend {
  background: color-mix(in srgb, var(--fill-2) 45%, transparent);
}
.mg-cell.drop {
  background: var(--accent-bg);
}
.mg-head {
  display: flex;
  justify-content: flex-end;
  height: 28px;
  padding: 4px 6px 0;
}
.mg-num {
  min-width: 24px;
  height: 24px;
  padding: 0 6px;
  border-radius: 999px;
  font-size: 13px;
  font-weight: 500;
  font-variant-numeric: tabular-nums;
  color: var(--text);
  transition: background 0.15s var(--ease);
}
.mg-num:hover {
  background: var(--fill);
}
.out .mg-num {
  color: var(--text-faint);
}
.mg-num.sel {
  background: var(--fill-strong);
}
.mg-num.today {
  background: var(--crit);
  color: #fff;
  font-weight: 600;
}
.mg-lines {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding-bottom: 2px;
}
.mg-gap,
.mg-bar,
.mg-ev,
.mg-task,
.mg-more {
  height: 18px;
  flex: none;
}
.mg-bar,
.mg-ev,
.mg-task {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11.5px;
  line-height: 18px;
  cursor: default;
  user-select: none;
}
/* All-day and multi-day: a filled bar in the event's colour. */
.mg-bar {
  position: relative;
  z-index: 1;
  padding: 0 6px;
  border-radius: 5px;
  font-weight: 500;
  background: color-mix(in srgb, var(--c) 22%, var(--bg));
  color: color-mix(in srgb, var(--c) 62%, var(--text));
}
.mg-bar.allday {
  background: var(--c);
  color: #fff;
}
.mg-bar.cs {
  border-top-left-radius: 0;
  border-bottom-left-radius: 0;
}
.mg-bar.ce {
  border-top-right-radius: 0;
  border-bottom-right-radius: 0;
}
.mg-bar.ext.allday {
  background: color-mix(in srgb, var(--c) 78%, var(--bg));
}
.mg-bar:hover {
  filter: brightness(1.06);
}
.mg-bar .mg-time {
  color: inherit;
  opacity: 0.8;
}
.mg-ev,
.mg-task {
  margin: 0 4px;
  padding: 0 4px;
  border-radius: 5px;
  color: var(--text);
}
.mg-ev:hover,
.mg-task:hover {
  background: var(--surface-hover);
}
.mg-ev .dot {
  width: 7px;
  height: 7px;
  flex: none;
  border-radius: 999px;
  background: var(--c);
}
.mg-ev.ext .dot {
  background: transparent;
  box-shadow: inset 0 0 0 1.5px var(--c);
}
.mg-time {
  flex: none;
  font-size: 11px;
  font-variant-numeric: tabular-nums;
  color: var(--text-3);
}
.mg-task .check {
  width: 12px;
  height: 12px;
  border-width: 1.2px;
}
.mg-task.done > button:nth-child(2) {
  color: var(--text-3);
  text-decoration: line-through;
}
.dragging {
  opacity: 0.45;
}
.mg-more {
  margin: 0 4px;
  padding: 0 4px;
  border-radius: 5px;
  text-align: left;
  font-size: 11.5px;
  font-weight: 600;
  color: var(--text-2);
}
.mg-more:hover {
  background: var(--fill);
}

/* ---- Phone ---- */
.compact {
  height: auto;
}
.compact .mg-names {
  border-bottom: 0;
}
.compact .mg-names > div {
  padding: 0 0 4px;
  text-align: center;
  font-size: 11px;
}
.compact .mg-body {
  grid-template-rows: none;
}
.compact .mg-week {
  border-bottom: 0;
  border-top: 0.5px solid var(--line-soft);
}
.mg-cday {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  height: 50px;
  padding-top: 5px;
  min-width: 0;
}
.mg-cday .num {
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border-radius: 999px;
  font-size: 17px;
  font-variant-numeric: tabular-nums;
  transition: background 0.15s var(--ease), color 0.15s;
}
.mg-cday.out .num {
  color: var(--text-faint);
}
.mg-cday.today .num {
  color: var(--crit);
  font-weight: 600;
}
.mg-cday.sel .num {
  background: var(--text);
  color: var(--bg);
  font-weight: 600;
}
.mg-cday.today.sel .num {
  background: var(--crit);
  color: #fff;
}
.dots {
  display: flex;
  gap: 3px;
  height: 6px;
}
.dots i {
  width: 5px;
  height: 5px;
  border-radius: 999px;
}
</style>
