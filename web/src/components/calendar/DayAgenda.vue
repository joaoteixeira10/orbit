<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, MapPin, Plus, Repeat } from 'lucide-vue-next'
import { fmtDate, fmtTime, fmtClock, parseDate, relDay } from '@/lib/dates'
import { onDay, sortEntries, eventColor, type Entry, type TaskItem } from './model'

// One day as a list, under the phone's month (as in iOS Calendar): all-day
// events, then timed ones with their times on the left, then due tasks.
const props = defineProps<{ day: string; entries: Entry[]; tasks: TaskItem[]; canCreate?: boolean }>()
const emit = defineEmits<{
  open: [entry: Entry]
  openTask: [item: TaskItem]
  toggleTask: [item: TaskItem]
  create: [day: string]
}>()
const { t } = useI18n()

const list = computed(() => sortEntries(props.entries.filter((e) => onDay(e, props.day))))
const dayTasks = computed(() =>
  props.tasks.filter((x) => x.task.due === props.day).sort((a, b) => (a.task.due_time ?? '').localeCompare(b.task.due_time ?? '')),
)
const heading = computed(() => relDay(props.day))
const sub = computed(() => fmtDate(parseDate(props.day), { weekday: 'long', day: 'numeric', month: 'long' }))

// times: what the left column says for an entry on this day.
function times(e: Entry): [string, string] {
  if (e.allDay) return [t('cal.allDay'), '']
  const starts = e.first === props.day
  const ends = e.last === props.day
  if (starts && ends) return [fmtTime(e.start), fmtTime(e.end)]
  if (starts) return [fmtTime(e.start), '']
  if (ends) return [t('cal.until', { time: fmtTime(e.end) }), '']
  return [t('cal.allDay'), '']
}
</script>

<template>
  <section class="ag">
    <header class="mb-2 flex items-end gap-2 px-1">
      <div class="min-w-0 flex-1">
        <h3 class="title-3 truncate">{{ heading }}</h3>
        <p v-if="heading !== sub" class="subhead truncate">{{ sub }}</p>
      </div>
      <button v-if="canCreate" class="icon-btn" :aria-label="t('cal.newEvent')" @click="emit('create', day)"><Plus class="h-5 w-5" /></button>
    </header>

    <div v-if="list.length || dayTasks.length" class="group">
      <button v-for="e in list" :key="e.key" class="group-row ag-row w-full text-left" @click="emit('open', e)">
        <span class="ag-time">
          <span>{{ times(e)[0] }}</span>
          <span v-if="times(e)[1]" class="text-text-3">{{ times(e)[1] }}</span>
        </span>
        <span class="ag-bar" :class="{ ext: e.occ.kind === 'external' }" :style="{ '--c': eventColor(e) }" />
        <span class="min-w-0 flex-1">
          <span class="flex items-center gap-1 text-[15px] font-medium">
            <span class="truncate">{{ e.occ.title || t('cal.untitled') }}</span>
            <Repeat v-if="e.occ.recurring" class="h-3 w-3 shrink-0 text-text-3" />
          </span>
          <span v-if="e.occ.location" class="flex items-center gap-1 truncate text-[12.5px] text-text-3"><MapPin class="h-3 w-3 shrink-0" />{{ e.occ.location }}</span>
        </span>
      </button>
      <div v-for="x in dayTasks" :key="x.task.id" class="group-row ag-row">
        <span class="ag-time">
          <span>{{ x.task.due_time ? fmtClock(x.task.due_time) : '' }}</span>
        </span>
        <button
          class="check"
          :class="{ on: x.task.done }"
          :style="{ '--check': x.color }"
          role="checkbox"
          :aria-checked="x.task.done"
          :aria-label="x.task.title"
          :disabled="!x.editable"
          @click="emit('toggleTask', x)"
        >
          <Check class="h-3 w-3" :stroke-width="3.4" />
        </button>
        <button class="min-w-0 flex-1 truncate text-left text-[15px]" :class="{ 'text-text-3 line-through': x.task.done }" @click="emit('openTask', x)">
          {{ x.task.title }}
        </button>
      </div>
    </div>
    <p v-else class="footnote px-1 py-6 text-center">{{ t('cal.noEvents') }}</p>
  </section>
</template>

<style scoped>
.ag-row {
  align-items: center;
  gap: 10px;
}
.ag-time {
  display: flex;
  flex: none;
  flex-direction: column;
  width: 58px;
  font-size: 12.5px;
  line-height: 1.3;
  font-variant-numeric: tabular-nums;
  text-align: right;
}
.ag-bar {
  align-self: stretch;
  flex: none;
  width: 4px;
  margin: 2px 0;
  border-radius: 2px;
  background: var(--c);
}
.ag-bar.ext {
  background: transparent;
  box-shadow: inset 0 0 0 1.5px var(--c);
}
</style>
