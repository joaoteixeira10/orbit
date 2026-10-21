<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronLeft, ChevronRight, Check, Plus } from 'lucide-vue-next'
import { addMonths, dateStr, monthGrid, monthTitle, startOfDay, weekdayNames } from '@/lib/dates'
import type { CalView } from './model'

// The calendar's left pane, as in Outlook: a small month to move around
// (click a day to go there, in the view you're in; the days on screen are
// shaded) and the calendars, each ticked to show or hide it.
export interface CalSource {
  id: string
  name: string
  color: string
}
const props = defineProps<{
  cursor: Date
  view: CalView
  rangeStart: Date
  rangeEnd: Date // exclusive
  sources: CalSource[]
  hidden: Set<string>
  heading: string
  canAdd?: boolean
  // A workspace's public holidays: its country, the choices, and whether
  // the viewer may change it (owners).
  holidays?: { country: string; countries: { code: string; name: string }[]; editable: boolean }
}>()
const emit = defineEmits<{ go: [day: Date]; toggle: [id: string]; add: []; holidays: [code: string] }>()
const { t } = useI18n()

const month = ref(new Date(props.cursor.getFullYear(), props.cursor.getMonth(), 1))
watch(
  () => props.cursor,
  (c) => {
    if (c.getMonth() !== month.value.getMonth() || c.getFullYear() !== month.value.getFullYear()) month.value = new Date(c.getFullYear(), c.getMonth(), 1)
  },
)
const names = computed(() => weekdayNames('narrow'))
const days = computed(() => monthGrid(month.value))
const today = dateStr(startOfDay(new Date()))
const from = computed(() => dateStr(props.rangeStart))
const to = computed(() => dateStr(props.rangeEnd))
const inRange = (d: Date) => props.view !== 'month' && dateStr(d) >= from.value && dateStr(d) < to.value
</script>

<template>
  <aside class="cal-side" :aria-label="t('cal.navigate')">
    <div class="mini">
      <div class="mini-head">
        <b>{{ monthTitle(month) }}</b>
        <span class="ml-auto flex">
          <button class="icon-btn h-7 w-7" :aria-label="t('cal.prev')" @click="month = addMonths(month, -1)"><ChevronLeft class="h-4 w-4" /></button>
          <button class="icon-btn h-7 w-7" :aria-label="t('cal.next')" @click="month = addMonths(month, 1)"><ChevronRight class="h-4 w-4" /></button>
        </span>
      </div>
      <div class="mini-grid" role="grid">
        <span v-for="(n, i) in names" :key="'n' + i" class="dow">{{ n }}</span>
        <button
          v-for="d in days"
          :key="dateStr(d)"
          class="day"
          :class="{ out: d.getMonth() !== month.getMonth(), today: dateStr(d) === today, sel: dateStr(d) === dateStr(cursor), range: inRange(d) }"
          :aria-label="d.toDateString()"
          @click="emit('go', d)"
        >
          {{ d.getDate() }}
        </button>
      </div>
    </div>

    <div class="mt-5">
      <div class="flex items-center px-2">
        <h3 class="cap flex-1">{{ heading }}</h3>
        <button v-if="canAdd" class="icon-btn h-6 w-6" :title="t('cal.addCalendar')" :aria-label="t('cal.addCalendar')" @click="emit('add')"><Plus class="h-4 w-4" /></button>
      </div>
      <ul class="mt-1">
        <li v-for="s in sources" :key="s.id">
          <button class="src" role="checkbox" :aria-checked="!hidden.has(s.id)" @click="emit('toggle', s.id)">
            <span class="box" :class="{ on: !hidden.has(s.id) }" :style="{ '--c': s.color || 'var(--accent)' }"><Check v-if="!hidden.has(s.id)" class="h-3 w-3" :stroke-width="3.4" /></span>
            <span class="truncate">{{ s.name }}</span>
          </button>
        </li>
      </ul>
      <label v-if="holidays && (holidays.editable || holidays.country)" class="mt-3 block px-2" data-testid="holidays">
        <span class="cap">{{ t('cal.holidays') }}</span>
        <select class="input mt-1 !py-1 text-[13px]" :value="holidays.country" :disabled="!holidays.editable" @change="emit('holidays', ($event.target as HTMLSelectElement).value)">
          <option value="">{{ t('cal.noHolidays') }}</option>
          <option v-for="c in holidays.countries" :key="c.code" :value="c.code">{{ c.name }}</option>
        </select>
      </label>
    </div>
  </aside>
</template>

<style scoped>
.cal-side {
  width: 236px;
  flex: none;
}
.mini-head {
  display: flex;
  align-items: center;
  padding: 0 2px 6px 8px;
  font-size: 14px;
}
.mini-grid {
  display: grid;
  grid-template-columns: repeat(7, 1fr);
  row-gap: 2px;
  text-align: center;
}
.dow {
  padding: 4px 0;
  font-size: 11px;
  font-weight: 600;
  color: var(--text-3);
}
.day {
  height: 30px;
  margin: 0 auto;
  width: 30px;
  border-radius: 999px;
  font-size: 12.5px;
  font-variant-numeric: tabular-nums;
}
.day:hover {
  background: var(--fill-2);
}
.day.out {
  color: var(--text-3);
}
.day.range {
  width: 100%;
  border-radius: 0;
  background: color-mix(in srgb, var(--accent) 12%, transparent);
}
.day.sel {
  box-shadow: inset 0 0 0 1.5px var(--accent);
}
.day.today {
  background: var(--accent);
  color: var(--accent-fg);
  font-weight: 700;
  width: 30px;
  border-radius: 999px;
}
.src {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 10px;
  padding: 6px 8px;
  border-radius: 8px;
  font-size: 13.5px;
  text-align: left;
}
.src:hover {
  background: var(--fill-2);
}
.box {
  display: grid;
  flex: none;
  place-items: center;
  width: 18px;
  height: 18px;
  border-radius: 5px;
  box-shadow: inset 0 0 0 1.5px var(--c);
  color: #fff;
}
.box.on {
  background: var(--c);
  box-shadow: none;
}
</style>
