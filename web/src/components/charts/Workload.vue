<script setup lang="ts">
import { computed } from 'vue'

// A heat table: one row per person, one column per due bucket, each cell
// how many open tasks — darker with more, red where they're late. A cell
// selects (to see those tasks).
export interface WorkRow {
  key: string
  label: string
  values: number[]
}
const props = defineProps<{ cols: { key: string; label: string; hot?: boolean }[]; rows: WorkRow[]; caption: string; hint?: string }>()
const emit = defineEmits<{ select: [row: string, col: string] }>()
const max = computed(() => Math.max(1, ...props.rows.flatMap((r) => r.values)))
const shade = (v: number, hot?: boolean) =>
  v ? `color-mix(in srgb, ${hot ? 'var(--c-late)' : 'var(--c-progress)'} ${Math.round(14 + 52 * (v / max.value))}%, transparent)` : undefined
</script>

<template>
  <div class="work" role="table" :aria-label="caption">
    <div class="work-row head" role="row">
      <span role="columnheader" />
      <span v-for="c in cols" :key="c.key" role="columnheader" class="work-col">{{ c.label }}</span>
    </div>
    <div v-for="r in rows" :key="r.key" class="work-row" role="row" data-testid="workload-row">
      <span class="work-label" role="rowheader"><slot name="label" :row="r">{{ r.label }}</slot></span>
      <button
        v-for="(v, i) in r.values"
        :key="cols[i].key"
        type="button"
        role="cell"
        class="work-cell"
        :class="{ empty: !v }"
        :style="{ background: shade(v, cols[i].hot) }"
        :disabled="!v"
        :title="v ? `${r.label} · ${cols[i].label}: ${v}${hint ? ' — ' + hint : ''}` : undefined"
        @click="emit('select', r.key, cols[i].key)"
      >
        {{ v || '' }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.work {
  display: grid;
  gap: 4px;
  overflow-x: auto;
}
.work-row {
  display: grid;
  grid-template-columns: minmax(120px, 1.6fr) repeat(v-bind('cols.length'), minmax(52px, 1fr));
  gap: 4px;
  align-items: center;
}
.work-row.head {
  font-size: 11.5px;
  font-weight: 600;
  color: var(--text-3);
}
.work-col {
  text-align: center;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.work-label {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
  font-size: 13.5px;
}
.work-cell {
  height: 32px;
  border-radius: 8px;
  background: var(--c-track);
  font: 600 13.5px var(--font-text, inherit);
  font-variant-numeric: tabular-nums;
  color: var(--text);
  transition: transform 0.15s var(--ease);
}
.work-cell:not(.empty):hover {
  transform: scale(1.04);
}
.work-cell.empty {
  cursor: default;
}
</style>
