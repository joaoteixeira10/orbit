<script setup lang="ts">
import { ref, computed, useId, onMounted, onBeforeUnmount } from 'vue'
import ChartTip from './ChartTip.vue'
import { tipAnchor, type Series, type Tip } from './types'
import { niceScale } from '@/lib/charts'

// Grouped columns over time (e.g. tasks created vs completed per week), like
// Health's: thin columns with rounded tops on a hairline grid, one y axis.
// Each period is a hover / focus band whose readout lists every series.
export interface Period {
  key: string
  label: string // the x-axis tick
  title: string // the readout's heading
  values: Record<string, number>
}
const props = withDefaults(defineProps<{ periods: Period[]; series: Series[]; caption: string; height?: number }>(), { height: 210 })

// The SVG is drawn at the card's width (in px, so strokes stay crisp).
const box = ref<HTMLElement | null>(null)
const width = ref(0)
let ro: ResizeObserver | undefined
onMounted(() => {
  ro = new ResizeObserver(([e]) => (width.value = Math.floor(e.contentRect.width)))
  if (box.value) ro.observe(box.value)
})
onBeforeUnmount(() => ro?.disconnect())

const pad = { l: 30, r: 4, t: 10, b: 26 }
const clip = `wk-${useId()}`
const base = computed(() => props.height - pad.b)
const plotW = computed(() => Math.max(0, width.value - pad.l - pad.r))
const band = computed(() => (props.periods.length ? plotW.value / props.periods.length : 0))
const scale = computed(() => niceScale(Math.max(0, ...props.periods.flatMap((p) => props.series.map((s) => p.values[s.key] ?? 0)))))
const ticks = computed(() => Array.from({ length: Math.round(scale.value.max / scale.value.step) + 1 }, (_, i) => i * scale.value.step))
const y = (v: number) => base.value - (v / scale.value.max) * (base.value - pad.t)

// Columns: up to 14 px wide, 2 px apart, centred in their period.
const colW = computed(() => {
  const k = props.series.length
  return Math.max(3, Math.min(14, (band.value * 0.6 - (k - 1) * 2) / k))
})
const columns = computed(() => {
  const k = props.series.length
  const gw = k * colW.value + (k - 1) * 2
  return props.periods.flatMap((p, i) =>
    props.series.map((s, j) => {
      const v = p.values[s.key] ?? 0
      return { key: `${p.key}-${s.key}`, v, color: s.color, x: pad.l + i * band.value + (band.value - gw) / 2 + j * (colW.value + 2), y: y(v), h: base.value - y(v) }
    }),
  )
})
// Every k-th tick label, counted back from the latest so it is always shown.
const every = computed(() => Math.max(1, Math.ceil(52 / (band.value || 1))))
const showTick = (i: number) => (props.periods.length - 1 - i) % every.value === 0

const hover = ref<string | null>(null)
const tip = ref<Tip | null>(null)
function show(p: Period, at: { x: number; y: number }) {
  hover.value = p.key
  tip.value = { ...at, title: p.title, rows: props.series.map((s) => ({ color: s.color, label: s.label, value: p.values[s.key] ?? 0 })) }
}
function hide() {
  hover.value = null
  tip.value = null
}
const bandLabel = (p: Period) => `${p.title}: ${props.series.map((s) => `${s.label} ${p.values[s.key] ?? 0}`).join(', ')}`
</script>

<template>
  <div>
    <ul class="legend" aria-hidden="true">
      <li v-for="s in series" :key="s.key"><span class="swatch" :style="{ background: s.color }" />{{ s.label }}</li>
    </ul>
    <div ref="box" class="plot" :style="{ height: height + 'px' }">
      <svg v-if="width" :width="width" :height="height" role="group" :aria-label="caption" @pointerleave="hide">
        <defs>
          <clipPath :id="clip"><rect :x="0" :y="0" :width="width" :height="base" /></clipPath>
        </defs>
        <!-- Grid and y axis -->
        <g>
          <template v-for="v in ticks" :key="v">
            <line :x1="pad.l" :x2="width - pad.r" :y1="Math.round(y(v)) + 0.5" :y2="Math.round(y(v)) + 0.5" :stroke="v === 0 ? 'var(--c-axis)' : 'var(--c-grid)'" stroke-width="1" />
            <text class="tick" :x="pad.l - 8" :y="y(v) + 4" text-anchor="end">{{ v }}</text>
          </template>
        </g>
        <!-- The hovered period -->
        <template v-for="(p, i) in periods" :key="p.key">
          <rect v-if="hover === p.key" class="band-on" :x="pad.l + i * band + 1" :y="pad.t - 4" :width="Math.max(0, band - 2)" :height="base - pad.t + 4" rx="6" />
        </template>
        <!-- Columns: rounded tops; the square foot is clipped at the baseline -->
        <g :clip-path="`url(#${clip})`">
          <template v-for="c in columns" :key="c.key">
            <rect v-if="c.v" class="col" :x="c.x" :y="c.y" :width="colW" :height="c.h + 6" :rx="Math.min(4, colW / 2)" :fill="c.color" />
          </template>
        </g>
        <!-- X ticks -->
        <template v-for="(p, i) in periods" :key="p.key">
          <text v-if="showTick(i)" class="tick" :x="pad.l + i * band + band / 2" :y="height - 8" text-anchor="middle">{{ p.label }}</text>
        </template>
        <!-- Hit bands (on top, the full height of each period) -->
        <rect
          v-for="(p, i) in periods"
          :key="p.key"
          class="hit"
          :x="pad.l + i * band"
          :y="0"
          :width="band"
          :height="base"
          tabindex="0"
          role="img"
          :aria-label="bandLabel(p)"
          @pointermove="show(p, { x: $event.clientX, y: $event.clientY })"
          @focus="show(p, tipAnchor($event.target as Element))"
          @blur="hide"
        />
      </svg>
    </div>
    <ChartTip :tip="tip" />

    <table class="sr-only">
      <caption>{{ caption }}</caption>
      <thead>
        <tr>
          <th scope="col" />
          <th v-for="s in series" :key="s.key" scope="col">{{ s.label }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="p in periods" :key="p.key">
          <th scope="row">{{ p.title }}</th>
          <td v-for="s in series" :key="s.key">{{ p.values[s.key] ?? 0 }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
.legend {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 16px;
  margin-bottom: 10px;
  font-size: 12px;
  color: var(--text-2);
}
.legend li {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.swatch {
  width: 10px;
  height: 10px;
  border-radius: 3px;
}
.plot {
  position: relative;
  width: 100%;
}
.tick {
  font-size: 11px;
  fill: var(--text-3);
  font-variant-numeric: tabular-nums;
}
.band-on {
  fill: var(--fill-2);
}
.col {
  transform-box: fill-box;
  transform-origin: bottom;
  animation: grow 0.6s var(--spring) both;
}
@keyframes grow {
  from {
    transform: scaleY(0);
  }
}
.hit {
  fill: transparent;
  outline: none;
}
.hit:focus-visible {
  fill: var(--accent-bg);
}
@media (prefers-reduced-motion: reduce) {
  .col {
    animation: none;
  }
}
</style>
