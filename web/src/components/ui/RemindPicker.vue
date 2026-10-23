<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Clock, Sun, CalendarDays, Hourglass, X } from 'lucide-vue-next'
import { fmtDate, fmtTime, toLocalInput, fromLocalInput } from '@/lib/dates'

// When to be reminded (a note) or to see something again (snooze): a few
// quick choices — in an hour (snooze), later today, this evening, tomorrow
// morning, next Monday — or a date and time. Emits the instant (ISO), or ""
// for "no reminder".
const props = defineProps<{ snooze?: boolean; current?: string | null }>()
const emit = defineEmits<{ pick: [at: string] }>()
const { t } = useI18n()

const at = (d: Date, h: number) => new Date(d.getFullYear(), d.getMonth(), d.getDate(), h, 0)
const options = computed(() => {
  const now = new Date()
  const out: { key: string; label: string; when: Date; icon: unknown }[] = []
  if (props.snooze) out.push({ key: 'hour', label: t('remind.inHour'), when: new Date(now.getTime() + 3600000), icon: Hourglass })
  const later = new Date(now.getTime() + 3 * 3600000)
  later.setMinutes(0, 0, 0)
  if (later.getDate() === now.getDate() && later.getHours() <= 21) out.push({ key: 'later', label: t('remind.later'), when: later, icon: Clock })
  if (now.getHours() < 17) out.push({ key: 'evening', label: t('remind.evening'), when: at(now, 18), icon: Clock })
  const tomorrow = new Date(now)
  tomorrow.setDate(now.getDate() + 1)
  out.push({ key: 'tomorrow', label: t('remind.tomorrow'), when: at(tomorrow, 9), icon: Sun })
  const monday = new Date(now)
  monday.setDate(now.getDate() + (((8 - now.getDay()) % 7) || 7))
  out.push({ key: 'monday', label: t('remind.nextWeek'), when: at(monday, 9), icon: CalendarDays })
  return out
})
const label = (d: Date) => {
  const today = new Date()
  const same = d.toDateString() === today.toDateString()
  return same ? fmtTime(d) : `${fmtDate(d, { weekday: 'short' })} ${fmtTime(d)}`
}

const custom = ref(toLocalInput(new Date(Date.now() + 3600000)))
const customOk = computed(() => !!custom.value && fromLocalInput(custom.value).getTime() > Date.now())
</script>

<template>
  <div class="py-1" data-testid="remind-picker">
    <button v-for="o in options" :key="o.key" class="menu-item" @click="emit('pick', o.when.toISOString())">
      <component :is="o.icon" class="h-4 w-4 text-text-2" />
      <span class="flex-1">{{ o.label }}</span>
      <span class="text-[12px] tabular-nums text-text-3">{{ label(o.when) }}</span>
    </button>
    <div class="menu-sep" />
    <form class="flex items-center gap-1.5 px-3 py-1.5" @submit.prevent="customOk && emit('pick', fromLocalInput(custom).toISOString())">
      <input v-model="custom" type="datetime-local" class="input min-w-0 flex-1 !py-1 text-[13px]" :aria-label="t('remind.pick')" />
      <button class="btn btn-sm" :disabled="!customOk">{{ t('remind.set') }}</button>
    </form>
    <template v-if="current && !snooze">
      <div class="menu-sep" />
      <button class="menu-item danger" @click="emit('pick', '')"><X class="h-4 w-4" />{{ t('remind.remove') }}</button>
    </template>
  </div>
</template>
