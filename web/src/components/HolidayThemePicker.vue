<script setup lang="ts">
// Settings → Appearance → Holiday theme / Season theme, when the owner lets
// each person choose: as for everyone (the default), off, automatic (each in
// its days) or one always. Tiles like the developer theme's picker
// (DevThemePicker.vue): the same look, side by side. The value is the
// person's preference: '' = as for everyone, 'off', 'auto' or an id. The
// view stores it; this only picks.
import { computed, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { Ban, Building2, CalendarClock, Check, Flower2, Leaf, Snowflake, Sun } from '@lucide/vue'
import { HOLIDAYS } from '@/lib/holidayThemes'
import { SEASONS } from '@/lib/seasonThemes'

const choice = defineModel<string>({ required: true })
const props = defineProps<{ kind: 'holiday' | 'season'; everyone: string }>()
const { t } = useI18n()

const seasonIcons: Record<string, Component> = { spring: Flower2, summer: Sun, autumn: Leaf, winter: Snowflake }
type Opt = { id: string; name: string; sub: string; color: string; logo?: string; icon?: Component; tile?: string }
const opts = computed<Opt[]>(() =>
  props.kind === 'holiday'
    ? HOLIDAYS.map((h) => ({ id: h.id, name: t(`holiday.name.${h.id}`), sub: t(`holiday.admin.season.${h.id}`), color: h.art.iconBoxes.calendar, logo: h.art.logo }))
    : SEASONS.map((s) => ({ id: s.id, name: t(`holiday.seasons.name.${s.id}`), sub: t(`holiday.seasons.admin.window.${s.id}`), color: s.art.iconBoxes.calendar, icon: seasonIcons[s.id], tile: s.art.tile.background })),
)
// "As for everyone", and under it what that is now.
const everyoneSub = computed(() => {
  const v = props.everyone
  if (v === 'off') return t('holiday.mine.off')
  if (v === 'auto') return t('holiday.mine.auto')
  return props.kind === 'holiday' ? t(`holiday.name.${v}`) : t(`holiday.seasons.name.${v}`)
})
</script>

<template>
  <div class="dt-picker" role="radiogroup">
    <button type="button" class="dt-opt" :class="{ on: choice === '' }" role="radio" :aria-checked="choice === ''" @click="choice = ''">
      <span class="dt-ico"><Building2 class="h-4 w-4" /></span>
      <span class="dt-name">{{ t('holiday.mine.asInstance') }}<small>{{ everyoneSub }}</small></span>
      <Check v-if="choice === ''" class="dt-check h-4 w-4" />
    </button>
    <button type="button" class="dt-opt" :class="{ on: choice === 'off' }" role="radio" :aria-checked="choice === 'off'" @click="choice = 'off'">
      <span class="dt-ico"><Ban class="h-4 w-4" /></span>
      <span class="dt-name">{{ t('holiday.mine.off') }}</span>
      <Check v-if="choice === 'off'" class="dt-check h-4 w-4" />
    </button>
    <button type="button" class="dt-opt" :class="{ on: choice === 'auto' }" role="radio" :aria-checked="choice === 'auto'" @click="choice = 'auto'">
      <span class="dt-ico"><CalendarClock class="h-4 w-4" /></span>
      <span class="dt-name">{{ t('holiday.mine.auto') }}<small>{{ t(kind === 'holiday' ? 'holiday.admin.autoHint' : 'holiday.seasons.admin.autoHint') }}</small></span>
      <Check v-if="choice === 'auto'" class="dt-check h-4 w-4" />
    </button>
    <button
      v-for="o in opts"
      :key="o.id"
      type="button"
      class="dt-opt"
      :class="{ on: choice === o.id }"
      role="radio"
      :aria-checked="choice === o.id"
      :style="{ '--tc': o.color }"
      :data-testid="`my-${kind}-${o.id}`"
      @click="choice = o.id"
    >
      <span v-if="o.logo" class="dt-crest" aria-hidden="true" v-html="o.logo" />
      <span v-else class="dt-ico text-white" :style="{ background: o.tile }" aria-hidden="true"><component :is="o.icon" class="h-4 w-4" :stroke-width="2.2" /></span>
      <span class="dt-name">{{ o.name }}<small>{{ o.sub }}</small></span>
      <Check v-if="choice === o.id" class="dt-check h-4 w-4" />
    </button>
  </div>
</template>

<style scoped>
/* As DevThemePicker.vue's tiles (keep the two alike). */
.dt-picker {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(170px, 1fr));
  gap: 8px;
}
.dt-opt {
  display: flex;
  align-items: center;
  gap: 9px;
  min-height: 44px;
  padding: 6px 10px;
  border-radius: 10px;
  border: 1px solid var(--border);
  background: var(--surface);
  color: var(--text);
  font-size: 13.5px;
  text-align: left;
  cursor: pointer;
  transition: border-color var(--speed), box-shadow var(--speed);
}
.dt-opt:hover {
  border-color: var(--border-2);
}
.dt-opt.on {
  border-color: var(--tc, var(--accent));
  box-shadow: 0 0 0 1px var(--tc, var(--accent));
}
.dt-ico {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  flex-shrink: 0;
  border-radius: 7px;
  background: var(--surface-2);
  color: var(--text-2);
}
.dt-crest {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  flex-shrink: 0;
}
.dt-crest :deep(svg) {
  width: 28px;
  height: 28px;
}
.dt-name {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dt-name small {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 11.5px;
  color: var(--text-3);
}
.dt-check {
  flex-shrink: 0;
  color: var(--tc, var(--accent));
}
</style>
