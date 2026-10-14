<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import ChartTip from './ChartTip.vue'
import { tipAnchor, type BarRow, type Series, type Tip } from './types'

// Horizontal stacked bars, one per row, on one shared scale: the label (a
// slot, for dots or avatars) on the left, the segments split by a 2 px gap,
// the total at the bar's end. Hovering or focusing a segment shows the row's
// every value; clicking one emits `select` (row, series).
const props = withDefaults(defineProps<{ rows: BarRow[]; series: Series[]; caption: string; selectable?: boolean; hint?: string }>(), {
  selectable: false,
  hint: '',
})
const emit = defineEmits<{ select: [row: string, series: string] }>()
defineSlots<{ label(p: { row: BarRow }): unknown }>()
const { t } = useI18n()

const sum = (r: BarRow) => props.series.reduce((a, s) => a + (r.values[s.key] ?? 0), 0)
const max = computed(() => Math.max(1, ...props.rows.map(sum)))

const hover = ref<{ row: string; series: string } | null>(null)
const tip = ref<Tip | null>(null)
function show(r: BarRow, s: Series, at: { x: number; y: number }) {
  hover.value = { row: r.key, series: s.key }
  tip.value = {
    ...at,
    title: r.label,
    rows: [
      ...props.series.map((x) => ({ color: x.color, label: x.label, value: r.values[x.key] ?? 0, on: x.key === s.key })),
      { color: 'transparent', label: t('charts.total'), value: sum(r) },
    ],
    hint: props.selectable ? props.hint : '',
  }
}
function hide() {
  hover.value = null
  tip.value = null
}
const segLabel = (r: BarRow, s: Series) => `${r.label}, ${s.label}: ${r.values[s.key] ?? 0}`
</script>

<template>
  <div class="bars" @pointerleave="hide">
    <div v-for="r in rows" :key="r.key" class="bar-row">
      <div class="bar-label"><slot name="label" :row="r">{{ r.label }}</slot></div>
      <div class="track">
        <div class="bar" :style="{ width: `calc((100% - 3.5em) * ${sum(r) / max})` }">
          <template v-for="s in series" :key="s.key">
            <button
              v-if="r.values[s.key]"
              type="button"
              class="bar-seg"
              :class="{ dim: hover && hover.row === r.key && hover.series !== s.key, click: selectable }"
              :style="{ flexGrow: r.values[s.key], background: s.color }"
              :aria-label="segLabel(r, s)"
              :tabindex="0"
              @pointermove="show(r, s, { x: $event.clientX, y: $event.clientY })"
              @focus="show(r, s, tipAnchor($event.target as Element))"
              @blur="hide"
              @click="selectable && emit('select', r.key, s.key)"
            />
          </template>
        </div>
        <span class="total">{{ sum(r) }}</span>
      </div>
    </div>
    <ChartTip :tip="tip" />

    <table class="sr-only">
      <caption>{{ caption }}</caption>
      <thead>
        <tr>
          <th scope="col" />
          <th v-for="s in series" :key="s.key" scope="col">{{ s.label }}</th>
          <th scope="col">{{ t('charts.total') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="r in rows" :key="r.key">
          <th scope="row">{{ r.label }}</th>
          <td v-for="s in series" :key="s.key">{{ r.values[s.key] ?? 0 }}</td>
          <td>{{ sum(r) }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
.bar-row {
  display: grid;
  grid-template-columns: minmax(0, var(--label-w, 34%)) minmax(0, 1fr);
  align-items: center;
  gap: 12px;
  min-height: 34px;
}
.bar-label {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  font-size: 14px;
  color: var(--text);
}
.track {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.bar {
  display: flex;
  gap: 2px;
  height: 16px;
  min-width: 3px;
  border-radius: 0 4px 4px 0;
  overflow: hidden;
  transition: width 0.55s var(--spring);
}
.bar-seg {
  flex: 1 1 0;
  min-width: 3px;
  height: 100%;
  padding: 0;
  border: 0;
  outline: none;
  transition:
    flex-grow 0.55s var(--spring),
    opacity 0.2s var(--ease),
    filter 0.2s var(--ease);
}
.bar-seg:hover,
.bar-seg:focus-visible {
  filter: brightness(1.12) saturate(1.05);
}
.bar-seg:focus-visible {
  box-shadow: inset 0 0 0 2px var(--ring);
}
.bar-seg.click {
  cursor: pointer;
}
.bar-seg.dim {
  opacity: 0.45;
}
.total {
  flex-shrink: 0;
  font-size: 13px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  color: var(--text-2);
}
</style>
