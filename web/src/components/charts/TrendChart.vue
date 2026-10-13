<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import type { Palette } from '@/lib/palette'

export interface TrendSeries {
  label: string
  color: string
  area: boolean
  data: number[]
}

const props = withDefaults(
  defineProps<{ series: TrendSeries[]; labels: string[]; pal: Palette; height?: number; year?: string }>(),
  { height: 220, year: '' },
)

const wrap = ref<HTMLElement | null>(null)
const w = ref(560)
const hi = ref<number | null>(null)
let ro: ResizeObserver | null = null

onMounted(() => {
  ro = new ResizeObserver((e) => (w.value = e[0].contentRect.width))
  if (wrap.value) ro.observe(wrap.value)
})
onUnmounted(() => ro?.disconnect())

const padL = 30
const padR = 10
const padT = 14
const padB = 26

const n = computed(() => Math.max(1, props.labels.length))
const max = computed(() => {
  const all = props.series.flatMap((s) => s.data)
  return Math.ceil(Math.max(...all, 1) / 5) * 5
})
const xOf = (i: number) => padL + (i * (w.value - padL - padR)) / Math.max(1, n.value - 1)
const yOf = (v: number) => padT + (props.height - padT - padB) * (1 - v / max.value)
const gridY = computed(() => [0, 0.25, 0.5, 0.75, 1].map((f) => max.value * f))

const pathFor = (data: number[]) =>
  data.map((d, i) => `${i ? 'L' : 'M'}${xOf(i).toFixed(1)} ${yOf(d).toFixed(1)}`).join(' ')
const areaFor = (data: number[]) =>
  `${pathFor(data)} L${xOf(n.value - 1)} ${props.height - padB} L${xOf(0)} ${props.height - padB} Z`

function onMove(e: MouseEvent) {
  if (!wrap.value) return
  const rect = wrap.value.getBoundingClientRect()
  const idx = Math.round(((e.clientX - rect.left - padL) / (w.value - padL - padR)) * (n.value - 1))
  hi.value = Math.max(0, Math.min(n.value - 1, idx))
}

const tipLeft = computed(() =>
  hi.value == null ? 0 : Math.min(Math.max(xOf(hi.value) - 66, 0), w.value - 134),
)
</script>

<template>
  <div ref="wrap" class="trend" @mousemove="onMove" @mouseleave="hi = null">
    <svg :width="w" :height="height" style="display: block; max-width: 100%">
      <g v-for="(g, i) in gridY" :key="'g' + i">
        <line :x1="padL" :x2="w - padR" :y1="yOf(g)" :y2="yOf(g)" :stroke="pal.grid" stroke-width="1" />
        <text
          :x="padL - 7"
          :y="yOf(g) + 3"
          text-anchor="end"
          font-size="10"
          :fill="pal.textFaint"
          font-family="ui-monospace, SFMono-Regular, Menlo, monospace"
        >
          {{ g }}
        </text>
      </g>
      <g v-for="(s, i) in series" :key="'s' + i">
        <path v-if="s.area" :d="areaFor(s.data)" :fill="s.color" fill-opacity="0.13" />
        <path
          :d="pathFor(s.data)"
          fill="none"
          :stroke="s.color"
          stroke-width="2.2"
          stroke-linecap="round"
          stroke-linejoin="round"
        />
      </g>
      <g v-if="hi != null">
        <line
          :x1="xOf(hi)"
          :x2="xOf(hi)"
          :y1="padT"
          :y2="height - padB"
          :stroke="pal.borderStrong"
          stroke-width="1"
          stroke-dasharray="3 3"
        />
        <circle
          v-for="(s, i) in series"
          :key="'d' + i"
          :cx="xOf(hi)"
          :cy="yOf(s.data[hi])"
          r="4"
          :fill="pal.surface"
          :stroke="s.color"
          stroke-width="2.2"
        />
      </g>
      <text
        v-for="(l, i) in labels"
        :key="'l' + i"
        :x="xOf(i)"
        :y="height - padB + 16"
        text-anchor="middle"
        font-size="10.5"
        :fill="pal.textFaint"
        font-family="ui-monospace, SFMono-Regular, Menlo, monospace"
      >
        {{ l }}
      </text>
    </svg>
    <div v-if="hi != null" class="tip" :style="{ left: tipLeft + 'px' }">
      <div class="tip-m">{{ labels[hi] }} {{ year }}</div>
      <div v-for="(s, i) in series" :key="'t' + i" class="tip-row">
        <span class="sw" :style="{ background: s.color }" />
        <span class="nm">{{ s.label }}</span>
        <b>{{ s.data[hi] }}</b>
      </div>
    </div>
  </div>
</template>

<style scoped>
.trend {
  width: 100%;
  min-width: 0;
  overflow: hidden;
  position: relative;
}
.tip {
  position: absolute;
  top: 6px;
  background: var(--surface-3);
  border: 1px solid var(--border-strong);
  border-radius: 9px;
  padding: 8px 11px;
  box-shadow: var(--shadow-lg);
  pointer-events: none;
  min-width: 118px;
}
.tip-m {
  font-size: 11px;
  color: var(--text-3);
  margin-bottom: 6px;
  font-family: var(--font-mono);
}
.tip-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  margin-top: 3px;
}
.tip-row .sw {
  width: 8px;
  height: 8px;
  border-radius: 3px;
}
.tip-row .nm {
  color: var(--text-2);
  flex: 1;
}
.tip-row b {
  font-feature-settings: 'tnum';
  color: var(--text);
}
</style>
