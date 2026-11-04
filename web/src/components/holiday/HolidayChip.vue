<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useHolidayStore } from '@/stores/holiday'
import { countdown } from '@/lib/holidayThemes'

// The countdown by the greeting: days to the holiday's day (New Year's:
// hours on Dec 31), or its greeting on the day itself.
const holiday = useHolidayStore()
const { t } = useI18n()

const text = computed(() => {
  const h = holiday.active
  if (!h) return ''
  const c = countdown(h, holiday.now)
  if (c.unit === 'today') return t(`holiday.today.${h.id}`)
  return t(c.unit === 'hours' ? 'holiday.chip.nyeHours' : `holiday.chip.${h.id}`, { n: c.n, year: c.year }, c.n)
})
</script>

<template>
  <span v-if="holiday.active" class="h-chip" data-testid="holiday-chip">
    <span class="grid place-items-center" aria-hidden="true" v-html="holiday.active.art.chipIcon" />{{ text }}
  </span>
</template>

<style scoped>
.h-chip {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 11px 0 9px;
  border-radius: 14px;
  background: var(--h-chipBg);
  color: var(--h-chipText);
  box-shadow: inset 0 0 0 1px var(--h-chipLine);
  font-size: 13px;
  font-weight: 600;
  white-space: nowrap;
}
</style>
