<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { CalendarClock, CircleOff, Flower2, Leaf, Snowflake, Sun, TreeDeciduous } from '@lucide/vue'
import { settingsApi } from '@/api'
import { useBrandingStore } from '@/stores/branding'
import { useToastStore } from '@/stores/toast'
import { SEASONS, resolveSeason, type SeasonSetting } from '@/lib/seasonThemes'
import Toggle from '@/components/ui/Toggle.vue'

// Owner → Branding: season themes for everyone — off, the season today is
// in, or one now — and whether each person may pick their own (Settings →
// Appearance). Saved at once. A holiday theme in its days shows instead.
const { t } = useI18n()
const branding = useBrandingStore()
const toast = useToastStore()
const saving = ref(false)
const icons = { spring: Flower2, summer: Sun, autumn: Leaf, winter: Snowflake }

const choice = computed(() => branding.seasonTheme)
// What auto shows today (whether or not a holiday covers it now).
const today = computed(() => resolveSeason('auto'))
async function setPersonal(v: boolean) {
  try {
    branding.apply(await settingsApi.update({ season_themes_personal: v }))
    toast.success(t('holiday.seasons.admin.saved'))
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function pick(v: SeasonSetting) {
  if (saving.value || v === choice.value) return
  saving.value = true
  try {
    branding.apply(await settingsApi.update({ season_theme: v }))
    toast.success(t('holiday.seasons.admin.saved'))
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="card overflow-hidden" data-testid="season-admin">
    <div class="flex items-center gap-2 border-b border-border px-5 py-3">
      <TreeDeciduous class="h-[18px] w-[18px] text-text-2" /><h3 class="text-[14px] font-semibold">{{ t('holiday.seasons.admin.title') }}</h3>
    </div>
    <div class="flex flex-col gap-3 p-5">
      <p class="text-[12px] text-text-3">{{ t('holiday.seasons.admin.hint') }}</p>
      <div class="h-grid">
        <button type="button" class="h-opt" :class="{ on: choice === 'off' }" :aria-pressed="choice === 'off'" data-testid="season-off" @click="pick('off')">
          <CircleOff class="h-7 w-7 text-text-3" :stroke-width="1.6" />
          <span><b>{{ t('holiday.admin.off') }}</b><small>{{ t('holiday.seasons.admin.offHint') }}</small></span>
        </button>
        <button type="button" class="h-opt" :class="{ on: choice === 'auto' }" :aria-pressed="choice === 'auto'" data-testid="season-auto" @click="pick('auto')">
          <CalendarClock class="h-7 w-7 text-text-3" :stroke-width="1.6" />
          <span>
            <b>{{ t('holiday.admin.auto') }}</b>
            <small>{{ choice === 'auto' && today ? t('holiday.seasons.admin.nowShowing', { name: t(`holiday.seasons.name.${today.id}`) }) : t('holiday.seasons.admin.autoHint') }}</small>
          </span>
        </button>
        <button
          v-for="s in SEASONS"
          :key="s.id"
          type="button"
          class="h-opt"
          :class="{ on: choice === s.id }"
          :aria-pressed="choice === s.id"
          :title="t('holiday.seasons.admin.pickHint')"
          :data-testid="`season-${s.id}`"
          @click="pick(s.id)"
        >
          <span class="grid h-7 w-7 shrink-0 place-items-center rounded-md text-white" :style="{ background: s.art.tile.background }" aria-hidden="true">
            <component :is="icons[s.id]" class="h-4 w-4" :stroke-width="2.2" />
          </span>
          <span><b>{{ t(`holiday.seasons.name.${s.id}`) }}</b><small>{{ t(`holiday.seasons.admin.window.${s.id}`) }}</small></span>
        </button>
      </div>
      <div class="mt-1 flex items-center gap-3 border-t border-border pt-4">
        <div class="min-w-0 flex-1">
          <b class="text-[14px] font-semibold">{{ t('holiday.admin.personal') }}</b>
          <span class="mt-0.5 block text-[12px] leading-snug text-text-3">{{ t('holiday.seasons.admin.personalHint') }}</span>
        </div>
        <Toggle :model-value="branding.seasonsPersonal" data-testid="season-personal" @update:model-value="setPersonal" />
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
