<script setup lang="ts">
import { computed } from 'vue'
import { addDays, dateStr, fmtDate } from '@/lib/dates'
import { counts, mondayOf, type Habit } from '@/lib/habits'

// The last year of a habit, a square a day (weeks across, Monday on top):
// filled when done, faint on the days it didn't ask for.
const props = defineProps<{ habit: Habit; color: string }>()

const weeks = computed(() => {
  const today = new Date()
  const start = addDays(mondayOf(today), -52 * 7)
  const done = new Set(props.habit.checks)
  const out: { key: string; cells: { key: string; state: 'done' | 'due' | 'off' | 'future'; title: string }[]; month: string }[] = []
  for (let w = 0; w < 53; w++) {
    const monday = addDays(start, w * 7)
    const cells = []
    for (let i = 0; i < 7; i++) {
      const d = addDays(monday, i)
      const key = dateStr(d)
      const state = d > today ? 'future' : done.has(key) ? 'done' : counts(props.habit, d) ? 'due' : 'off'
      cells.push({ key, state: state as 'done' | 'due' | 'off' | 'future', title: fmtDate(d, { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' }) })
    }
    // A month's name over the week its first day falls in.
    const first = Array.from({ length: 7 }, (_, i) => addDays(monday, i)).find((d) => d.getDate() === 1)
    out.push({ key: dateStr(monday), cells, month: first ? fmtDate(first, { month: 'short' }) : '' })
  }
  return out
})
</script>

<template>
  <div class="year" :style="{ '--c': color }" role="img">
    <div v-for="w in weeks" :key="w.key" class="wk">
      <span class="mo">{{ w.month }}</span>
      <span v-for="c in w.cells" :key="c.key" class="cell" :class="c.state" :title="c.title" />
    </div>
  </div>
</template>

<style scoped>
.year {
  display: flex;
  gap: 3px;
  overflow-x: auto;
  padding-bottom: 2px;
}
.wk {
  display: grid;
  grid-template-rows: 14px repeat(7, 10px);
  gap: 3px;
}
.mo {
  font-size: 10px;
  color: var(--text-3);
  white-space: nowrap;
  width: 10px;
  overflow: visible;
}
.cell {
  width: 10px;
  height: 10px;
  border-radius: 3px;
  background: var(--fill);
}
.cell.done {
  background: var(--c);
}
.cell.off {
  background: color-mix(in srgb, var(--fill) 40%, transparent);
}
.cell.future {
  background: transparent;
}
</style>
