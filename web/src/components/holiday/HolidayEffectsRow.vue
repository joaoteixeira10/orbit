<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Flower2, Leaf, Snowflake, Sun } from '@lucide/vue'
import Toggle from '@/components/ui/Toggle.vue'
import { useHolidayStore } from '@/stores/holiday'
import { usePrefsStore } from '@/stores/prefs'

// The sidebar's switch for a holiday or season theme's falling snow, bats,
// confetti, petals or leaves ("Let it snow"): the same as Settings →
// Appearance → Theme effects. Only while such a theme shows. A holiday shows
// its own drawn icon; a season, the icon set's.
const { t } = useI18n()
const holiday = useHolidayStore()
const prefs = usePrefsStore()
const seasonIcons = { spring: Flower2, summer: Sun, autumn: Leaf, winter: Snowflake }
const label = computed(() => (holiday.active ? t(`holiday.effects.${holiday.active.id}`) : holiday.season ? t(`holiday.seasons.effects.${holiday.season.id}`) : ''))
</script>

<template>
  <div v-if="holiday.look" class="h-fx-row" data-testid="holiday-effects-row">
    <span v-if="holiday.active" class="grid h-4 w-4 place-items-center" aria-hidden="true" v-html="holiday.active.art.toggleIcon" />
    <component :is="seasonIcons[holiday.season.id]" v-else-if="holiday.season" class="h-4 w-4 text-(--h-icon)" aria-hidden="true" />
    <span class="min-w-0 flex-1 truncate">{{ label }}</span>
    <Toggle :model-value="prefs.holidayEffects" :aria-label="label" data-testid="holiday-effects-toggle" @update:model-value="prefs.setHolidayEffects" />
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
