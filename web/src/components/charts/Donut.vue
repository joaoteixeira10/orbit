<script setup lang="ts">
import { ref, computed } from 'vue'
import type { Slice } from './types'

// A donut with its legend, like Screen Time's: slices start at 12 o'clock,
// split by a hairline of the card's colour. The centre shows the headline
// (e.g. % complete) and, while a slice or legend row is hovered or focused,
// that slice's count and share. Slices and legend rows emit `select`.
const props = withDefaults(
  defineProps<{
    slices: Slice[]
    centerValue: string
    centerLabel: string
    caption: string // names the chart for screen readers
    size?: number
    thickness?: number
    selectable?: boolean
  }>(),
  { size: 188, thickness: 22, selectable: false },
)
const emit = defineEmits<{ select: [key: string] }>()

const total = computed(() => props.slices.reduce((a, s) => a + s.value, 0))
const r = computed(() => (props.size - props.thickness) / 2 - 3)
const C = computed(() => 2 * Math.PI * r.value)
const pct = (v: number) => (total.value ? Math.round((v / total.value) * 100) : 0)

// Each drawn slice: its dash (length on the circle) and where it starts.
const arcs = computed(() => {
  const shown = props.slices.filter((s) => s.value > 0)
  const gap = shown.length > 1 ? 2.5 : 0
  let at = 0
  return shown.map((s) => {
    const len = (s.value / total.value) * C.value
    const arc = { ...s, dash: Math.max(len - gap, 0.75), start: at }
    at += len
    return arc
  })
})

const active = ref<string | null>(null)
const activeSlice = computed(() => props.slices.find((s) => s.key === active.value))
const label = (s: Slice) => `${s.label}: ${s.value} (${pct(s.value)}%)`
</script>

<template>
  <div class="donut-wrap">
    <div class="donut" :style="{ width: size + 'px', height: size + 'px' }">
      <svg :width="size" :height="size" :viewBox="`0 0 ${size} ${size}`" role="group" :aria-label="caption">
        <circle :cx="size / 2" :cy="size / 2" :r="r" fill="none" stroke="var(--c-track)" :stroke-width="thickness" />
        <g :transform="`rotate(-90 ${size / 2} ${size / 2})`">
          <circle
            v-for="a in arcs"
            :key="a.key"
            class="arc"
            :class="{ dim: active && active !== a.key, on: active === a.key, click: selectable }"
            :cx="size / 2"
            :cy="size / 2"
            :r="r"
            fill="none"
            :stroke="a.color"
            :stroke-width="active === a.key ? thickness + 5 : thickness"
            :stroke-dasharray="`${a.dash} ${C}`"
            :stroke-dashoffset="-a.start"
            :tabindex="0"
            :role="selectable ? 'button' : 'img'"
            :aria-label="label(a)"
            @pointerenter="active = a.key"
            @pointerleave="active = null"
            @focus="active = a.key"
            @blur="active = null"
            @click="selectable && emit('select', a.key)"
            @keydown.enter.prevent="selectable && emit('select', a.key)"
          />
        </g>
      </svg>
      <div class="center" aria-hidden="true">
        <template v-if="activeSlice">
          <span class="big">{{ activeSlice.value }}</span>
          <span class="center-sub">{{ activeSlice.label }} · {{ pct(activeSlice.value) }}%</span>
        </template>
        <template v-else>
          <span class="big">{{ centerValue }}</span>
          <span class="center-sub">{{ centerLabel }}</span>
        </template>
      </div>
    </div>

    <ul class="legend">
      <li v-for="s in slices" :key="s.key">
        <component
          :is="selectable ? 'button' : 'div'"
          class="legend-row"
          :class="{ dim: active && active !== s.key, click: selectable }"
          :tabindex="selectable ? undefined : -1"
          @pointerenter="active = s.key"
          @pointerleave="active = null"
          @click="selectable && emit('select', s.key)"
        >
          <span class="swatch" :style="{ background: s.color }" />
          <span class="name">{{ s.label }}</span>
          <span class="count">{{ s.value }}</span>
          <span class="share">{{ pct(s.value) }}%</span>
        </component>
      </li>
    </ul>

    <table class="sr-only">
      <caption>{{ caption }}</caption>
      <tbody>
        <tr v-for="s in slices" :key="s.key">
          <th scope="row">{{ s.label }}</th>
          <td>{{ s.value }}</td>
          <td>{{ pct(s.value) }}%</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
.donut-wrap {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 20px 36px;
}
.donut {
  position: relative;
  flex-shrink: 0;
}
.arc {
  outline: none;
  transition:
    stroke-dasharray 0.6s var(--spring),
    stroke-dashoffset 0.6s var(--spring),
    stroke-width 0.2s var(--ease),
    opacity 0.2s var(--ease);
}
.arc.click {
  cursor: pointer;
}
.arc.dim {
  opacity: 0.35;
}
.center {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  pointer-events: none;
  text-align: center;
}
.big {
  font: 700 34px/1.05 var(--font-display);
  letter-spacing: -0.02em;
  color: var(--text);
}
.center-sub {
  max-width: 120px;
  margin-top: 3px;
  font-size: 12px;
  color: var(--text-2);
}
.legend {
  flex: 1 1 200px;
  max-width: 300px;
  min-width: 0;
}
.legend-row {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 10px;
  padding: 7px 8px;
  border-radius: 8px;
  text-align: left;
  font-size: 14px;
  transition:
    opacity 0.2s var(--ease),
    background 0.15s;
}
.legend li + li .legend-row {
  box-shadow: 0 -0.5px 0 var(--line-soft);
}
.legend-row.click:hover {
  background: var(--surface-hover);
}
.legend-row.dim {
  opacity: 0.45;
}
.swatch {
  width: 10px;
  height: 10px;
  border-radius: 3px;
  flex-shrink: 0;
}
.name {
  flex: 1;
  min-width: 0;
  color: var(--text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.count {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  color: var(--text);
}
.share {
  width: 3.2em;
  text-align: right;
  font-variant-numeric: tabular-nums;
  color: var(--text-3);
  font-size: 13px;
}
</style>
