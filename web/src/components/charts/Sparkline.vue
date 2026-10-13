<script setup lang="ts">
import { computed } from 'vue'

// Filled mini area sparkline. Normalized over a 200×H viewBox with a
// non-scaling stroke so it stretches to any card width.
const props = withDefaults(
  defineProps<{ data: number[]; color: string; height?: number; fill?: boolean }>(),
  { height: 38, fill: true },
)

const PAD = 2
const W = 200

const geom = computed(() => {
  const d = props.data.length ? props.data : [0, 0]
  const h = props.height
  const max = Math.max(...d)
  const min = Math.min(...d)
  const range = max - min || 1
  const step = (W - PAD * 2) / Math.max(1, d.length - 1)
  const pts = d.map((y, i) => [PAD + i * step, PAD + (h - PAD * 2) * (1 - (y - min) / range)])
  const line = pts.map((p, i) => `${i ? 'L' : 'M'}${p[0].toFixed(1)} ${p[1].toFixed(1)}`).join(' ')
  const area = `${line} L${W - PAD} ${h} L${PAD} ${h} Z`
  return { line, area, h }
})
</script>

<template>
  <svg :viewBox="`0 0 ${W} ${geom.h}`" preserveAspectRatio="none" width="100%" :height="geom.h">
    <path v-if="fill" :d="geom.area" :fill="color" fill-opacity="0.16" />
    <path
      :d="geom.line"
      fill="none"
      :stroke="color"
      stroke-width="1.8"
      stroke-linecap="round"
      stroke-linejoin="round"
      vector-effect="non-scaling-stroke"
    />
  </svg>
</template>
