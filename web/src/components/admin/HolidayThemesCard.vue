<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { CalendarClock, CircleOff, PartyPopper } from 'lucide-vue-next'
import { settingsApi } from '@/api'
import { useBrandingStore } from '@/stores/branding'
import { useHolidayStore } from '@/stores/holiday'
import { useToastStore } from '@/stores/toast'
import { HOLIDAYS, type HolidaySetting } from '@/lib/holidayThemes'

// Owner → Branding: holiday themes for everyone — off, each in its season,
// or one now (to see it, or to have it). Saved at once.
const { t } = useI18n()
const branding = useBrandingStore()
const holiday = useHolidayStore()
const toast = useToastStore()
const saving = ref(false)

const choice = computed(() => branding.holidayTheme)
async function pick(v: HolidaySetting) {
  if (saving.value || v === choice.value) return
  saving.value = true
  try {
    branding.apply(await settingsApi.update({ holiday_theme: v }))
    toast.success(t('holiday.admin.saved'))
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="card overflow-hidden" data-testid="holiday-admin">
    <div class="flex items-center gap-2 border-b border-border px-5 py-3">
      <PartyPopper class="h-[18px] w-[18px] text-text-2" /><h3 class="text-[14px] font-semibold">{{ t('holiday.admin.title') }}</h3>
    </div>
    <div class="flex flex-col gap-3 p-5">
      <p class="text-[12px] text-text-3">{{ t('holiday.admin.hint') }}</p>
      <div class="h-grid">
        <button type="button" class="h-opt" :class="{ on: choice === 'off' }" :aria-pressed="choice === 'off'" data-testid="holiday-off" @click="pick('off')">
          <CircleOff class="h-7 w-7 text-text-3" :stroke-width="1.6" />
          <span><b>{{ t('holiday.admin.off') }}</b><small>{{ t('holiday.admin.offHint') }}</small></span>
        </button>
        <button type="button" class="h-opt" :class="{ on: choice === 'auto' }" :aria-pressed="choice === 'auto'" data-testid="holiday-auto" @click="pick('auto')">
          <CalendarClock class="h-7 w-7 text-text-3" :stroke-width="1.6" />
          <span>
            <b>{{ t('holiday.admin.auto') }}</b>
            <small>{{ choice === 'auto' ? (holiday.active ? t('holiday.admin.nowShowing', { name: t(`holiday.name.${holiday.active.id}`) }) : t('holiday.admin.noneNow')) : t('holiday.admin.autoHint') }}</small>
          </span>
        </button>
        <button
          v-for="h in HOLIDAYS"
          :key="h.id"
          type="button"
          class="h-opt"
          :class="{ on: choice === h.id }"
          :aria-pressed="choice === h.id"
          :title="t('holiday.admin.pickHint')"
          :data-testid="`holiday-${h.id}`"
          @click="pick(h.id)"
        >
          <span class="grid h-7 w-7 shrink-0 place-items-center" aria-hidden="true" v-html="h.art.logo" />
          <span><b>{{ t(`holiday.name.${h.id}`) }}</b><small>{{ t(`holiday.admin.season.${h.id}`) }}</small></span>
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.h-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(190px, 1fr));
  gap: 8px;
}
.h-opt {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 12px;
  border: 1.5px solid var(--border);
  background: var(--surface-2);
  text-align: left;
  transition: border-color var(--speed);
}
.h-opt:hover {
  border-color: var(--border-2);
}
.h-opt.on {
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
}
.h-opt b {
  display: block;
  font-size: 13px;
}
.h-opt small {
  display: block;
  font-size: 11.5px;
  color: var(--text-3);
}
</style>
