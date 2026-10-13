<script setup lang="ts">
import { ref, computed } from 'vue'

export interface DonutSegment {
  label: string
  value: number
  color: string
}

const props = withDefaults(
  defineProps<{ segments: DonutSegment[]; track: string; size?: number; thickness?: number }>(),
  { size: 168, thickness: 22 },
)

const hover = ref<number | null>(null)

const total = computed(() => props.segments.reduce((s, x) => s + x.value, 0) || 1)
const r = computed(() => (props.size - props.thickness) / 2)
const center = computed(() => props.size / 2)
const circ = computed(() => 2 * Math.PI * r.value)

const arcs = computed(() => {
  let offset = 0
  return props.segments.map((s) => {
    const len = (s.value / total.value) * circ.value
    const arc = { ...s, dash: len, gap: circ.value - len, off: offset }
    offset -= len
    return arc
  })
})

const active = computed(() => (hover.value != null ? props.segments[hover.value] : null))
</script>

<template>
  <div class="donut" :style="{ width: size + 'px', height: size + 'px' }">
    <svg :width="size" :height="size" style="transform: rotate(-90deg)">
      <circle :cx="center" :cy="center" :r="r" fill="none" :stroke="track" :stroke-width="thickness" />
      <template v-for="(a, i) in arcs" :key="i">
        <circle
          v-if="a.value > 0"
          :cx="center"
          :cy="center"
          :r="r"
          fill="none"
          :stroke="a.color"
          :stroke-width="hover === i ? thickness + 4 : thickness"
          :stroke-dasharray="`${a.dash} ${a.gap}`"
          :stroke-dashoffset="a.off"
          :style="{ opacity: hover != null && hover !== i ? 0.35 : 1 }"
          class="seg"
          @mouseenter="hover = i"
          @mouseleave="hover = null"
        />
      </template>
    </svg>
    <div class="readout">
      <div>
        <div class="rv">{{ active ? active.value : total }}</div>
        <div class="rl">{{ active ? active.label : 'Total certs' }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.donut {
  position: relative;
  flex-shrink: 0;
}
.seg {
  transition:
    stroke-width 0.15s,
    opacity 0.15s;
  cursor: pointer;
}
.readout {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  text-align: center;
  pointer-events: none;
}
.rv {
  font-size: 30px;
  font-weight: 600;
  letter-spacing: -0.03em;
  font-feature-settings: 'tnum';
  color: var(--text);
}
.rl {
  font-size: 11px;
  color: var(--text-3);
  text-transform: uppercase;
  letter-spacing: 0.06em;
  font-weight: 600;
  margin-top: 2px;
}
</style>
