<script setup lang="ts">
import { computed } from 'vue'
import { locale, monthTitle } from '@/lib/dates'
import { monthCells, quarterCells, weekCells, weekday, numLocal, quarterOf, yearOf, type Span, type Zoom } from '@/lib/timeline'

// The timeline's date header: months (quarters when zoomed out) on top;
// days, weeks or months below. Weekends are dimmed, today is red (as in
// Calendar) and the days a dragged bar covers are highlighted.
const props = defineProps<{ range: Span; px: number; zoom: Zoom; today: number; weekStart: number; sel: Span | null }>()

const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)
const x = (n: number) => (n - props.range.from) * props.px

const top = computed(() => {
  if (props.zoom === 'quarters') return quarterCells(props.range).map((c) => ({ ...c, label: `Q${quarterOf(c.at)} ${yearOf(c.at)}` }))
  return monthCells(props.range).map((c) => ({ ...c, label: monthTitle(numLocal(c.at)) }))
})

interface Low {
  at: number
  days: number
  label: string
  wd?: string
  weekend?: boolean
  today?: boolean
}
const low = computed<Low[]>(() => {
  const loc = locale()
  if (props.zoom === 'weeks') {
    const wd = new Intl.DateTimeFormat(loc, { weekday: 'narrow' })
    const out: Low[] = []
    for (let n = props.range.from; n <= props.range.to; n++) {
      const d = numLocal(n)
      const w = weekday(n)
      out.push({ at: n, days: 1, label: String(d.getDate()), wd: wd.format(d), weekend: w === 0 || w === 6, today: n === props.today })
    }
    return out
  }
  if (props.zoom === 'months') {
    return weekCells(props.range, props.weekStart).map((c) => ({ ...c, label: `${numLocal(c.at).getDate()}–${numLocal(c.at + c.days - 1).getDate()}` }))
  }
  const mf = new Intl.DateTimeFormat(loc, { month: 'short' })
  return monthCells(props.range).map((c) => ({ ...c, label: cap(mf.format(numLocal(c.at)).replace('.', '')) }))
})

const todayLabel = computed(() => new Intl.DateTimeFormat(locale(), { day: 'numeric', month: 'short' }).format(numLocal(props.today)).replace('.', ''))
const inRange = computed(() => props.today >= props.range.from && props.today <= props.range.to)
// Zoomed out, today's red label sits on the scale; the label it would cover steps aside.
const covered = computed(() => {
  if (props.zoom === 'weeks' || !inRange.value) return -1
  const mid = x(props.today) + props.px / 2
  const half = todayLabel.value.length * 3.6 + 10
  const c = low.value.find((c) => {
    const cm = x(c.at) + (c.days * props.px) / 2
    return Math.abs(cm - mid) < half + c.label.length * 3.6
  })
  return c ? c.at : -1
})
</script>

<template>
  <div class="scale" :style="{ width: `${(range.to - range.from + 1) * px}px` }" aria-hidden="true">
    <div class="row top">
      <div v-for="c in top" :key="c.at" class="cell" :style="{ left: `${x(c.at)}px`, width: `${c.days * px}px` }">
        <span class="lbl">{{ c.label }}</span>
      </div>
    </div>
    <div class="row low" :class="zoom">
      <div v-if="sel" class="sel" :style="{ left: `${x(sel.from)}px`, width: `${(sel.to - sel.from + 1) * px}px` }" />
      <div
        v-for="c in low"
        :key="c.at"
        class="cell"
        :class="{ weekend: c.weekend, today: c.today }"
        :style="{ left: `${x(c.at)}px`, width: `${c.days * px}px` }"
      >
        <template v-if="zoom === 'weeks'">
          <span class="wd">{{ c.wd }}</span>
          <span class="dn">{{ c.label }}</span>
        </template>
        <span v-else-if="c.at !== covered" class="lbl">{{ c.label }}</span>
      </div>
      <span v-if="zoom !== 'weeks' && inRange" class="today-pill" :style="{ left: `${x(today) + px / 2}px` }">{{ todayLabel }}</span>
    </div>
  </div>
</template>

<style scoped>
.scale {
  position: relative;
  height: 100%;
  flex: none;
  user-select: none;
}
.row {
  position: relative;
}
.row.top {
  height: 26px;
}
.row.low {
  height: 34px;
}
.cell {
  position: absolute;
  top: 0;
  bottom: 0;
  display: flex;
  align-items: center;
}
.top .cell {
  border-left: 0.5px solid var(--border);
}
.top .cell .lbl {
  /* The month's name stays in view while its days scroll by. */
  position: sticky;
  left: calc(var(--name-w) + 10px);
  padding: 0 10px;
  font-size: 12.5px;
  font-weight: 600;
  white-space: nowrap;
  color: var(--text);
}
.low .cell {
  justify-content: center;
  font-size: 12px;
  color: var(--text-2);
  font-variant-numeric: tabular-nums;
}
.low.months .cell,
.low.quarters .cell {
  border-left: 0.5px solid var(--line-soft);
}
.low.weeks .cell {
  flex-direction: column;
  gap: 1px;
  line-height: 1;
}
.wd {
  font-size: 10px;
  font-weight: 500;
  color: var(--text-3);
  text-transform: uppercase;
}
.dn {
  display: grid;
  place-items: center;
  min-width: 22px;
  height: 20px;
  padding: 0 4px;
  border-radius: 999px;
  font-size: 13px;
  font-weight: 500;
  color: var(--text);
}
.weekend .dn,
.weekend .wd {
  color: var(--text-3);
}
.today .wd {
  color: var(--crit);
}
.today .dn {
  background: var(--crit);
  color: #fff;
  font-weight: 600;
}
.today-pill {
  position: absolute;
  top: 50%;
  z-index: 2;
  transform: translate(-50%, -50%);
  padding: 1px 7px;
  border-radius: 999px;
  background: var(--crit);
  color: #fff;
  font-size: 11px;
  font-weight: 600;
  white-space: nowrap;
}
.sel {
  position: absolute;
  top: 4px;
  bottom: 4px;
  border-radius: 7px;
  background: var(--accent-soft);
  box-shadow: inset 0 0 0 1px var(--accent);
}
</style>
