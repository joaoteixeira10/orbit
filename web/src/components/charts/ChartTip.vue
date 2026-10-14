<script setup lang="ts">
import { ref, watch } from 'vue'
import type { Tip } from './types'

// The charts' hover / focus readout: a small popover above the pointer,
// kept inside the window. Values lead (strong), labels follow; each row is
// keyed by a short stroke of its series colour.
const props = defineProps<{ tip: Tip | null }>()
const el = ref<HTMLElement | null>(null)
const pos = ref({ left: -9999, top: -9999 })

watch(
  () => props.tip,
  (t) => {
    const e = el.value
    if (!t || !e) return
    const w = e.offsetWidth
    const h = e.offsetHeight
    const left = Math.min(Math.max(8, t.x - w / 2), window.innerWidth - w - 8)
    const above = t.y - h - 12
    pos.value = { left, top: above >= 8 ? above : t.y + 20 }
  },
  { flush: 'post' },
)
</script>

<template>
  <Teleport to="body">
    <div v-if="tip" ref="el" class="chart-tip" role="tooltip" :style="{ left: pos.left + 'px', top: pos.top + 'px' }">
      <p class="title">{{ tip.title }}</p>
      <p v-for="r in tip.rows" :key="r.label" class="row" :class="{ on: r.on, zero: r.value === 0 }">
        <span class="key" :style="{ background: r.color }" />
        <span class="value">{{ r.value }}</span>
        <span class="tip-label">{{ r.label }}</span>
      </p>
      <p v-if="tip.hint" class="hint">{{ tip.hint }}</p>
    </div>
  </Teleport>
</template>

<style scoped>
.chart-tip {
  position: fixed;
  z-index: 60;
  min-width: 132px;
  max-width: 260px;
  padding: 8px 11px 9px;
  border-radius: 10px;
  background: color-mix(in srgb, var(--surface) 88%, transparent);
  -webkit-backdrop-filter: blur(20px) saturate(1.6);
  backdrop-filter: blur(20px) saturate(1.6);
  box-shadow: var(--shadow-lg);
  pointer-events: none;
  font-size: 12px;
  line-height: 1.35;
}
.title {
  margin-bottom: 4px;
  font-weight: 600;
  color: var(--text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.row {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 1px 0;
  color: var(--text-2);
}
.key {
  width: 10px;
  height: 3px;
  border-radius: 2px;
  flex-shrink: 0;
}
.value {
  min-width: 2ch;
  font-weight: 600;
  color: var(--text);
  font-variant-numeric: tabular-nums;
  text-align: right;
}
.row.zero {
  opacity: 0.5;
}
.row.on .tip-label {
  color: var(--text);
  font-weight: 500;
}
.hint {
  margin-top: 5px;
  padding-top: 5px;
  border-top: 0.5px solid var(--line-soft);
  color: var(--text-3);
  font-size: 11px;
}
</style>
