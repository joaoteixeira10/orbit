<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/ui/Toggle.vue'
import { useHolidayStore } from '@/stores/holiday'
import { usePrefsStore } from '@/stores/prefs'

// The sidebar's switch for a holiday theme's falling snow, bats, confetti…
// ("Let it snow"): the same as Settings → Appearance → Holiday effects.
// Only while a holiday theme shows.
const { t } = useI18n()
const holiday = useHolidayStore()
const prefs = usePrefsStore()
</script>

<template>
  <div v-if="holiday.active" class="h-fx-row" data-testid="holiday-effects-row">
    <span class="grid h-4 w-4 place-items-center" aria-hidden="true" v-html="holiday.active.art.toggleIcon" />
    <span class="min-w-0 flex-1 truncate">{{ t(`holiday.effects.${holiday.active.id}`) }}</span>
    <Toggle :model-value="prefs.holidayEffects" :aria-label="t(`holiday.effects.${holiday.active.id}`)" data-testid="holiday-effects-toggle" @update:model-value="prefs.setHolidayEffects" />
  </div>
</template>

<style scoped>
.h-fx-row {
  --ok: var(--h-box-calendar);
  --fill-strong: var(--h-switchOff);
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
  padding: 7px 8px;
  border-radius: 9px;
  background: var(--h-field);
  color: var(--text);
  font-size: 13px;
}
</style>
