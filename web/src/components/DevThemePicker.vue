<script setup lang="ts">
// SHARED — from ../blueprint/shared/dev-themes (copied by blueprint/sync-themes.sh).
// Change it there, not here: the same file is in every project.
//
// Settings → Appearance → Developer theme, when the owner lets each person
// choose (Owner → Branding): the instance's theme (the default), none, or one
// of the five. The value is the person's preference: '' = as the instance,
// 'none', or a theme id. The view stores it; this only picks.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, Ban, Building2 } from 'lucide-vue-next'
import { DEV_THEME_LIST, DEV_THEME_NAMES, resolveCrest, type DevThemeId } from '@/lib/devThemes'

const choice = defineModel<string>({ required: true })
const props = defineProps<{ instanceTheme: DevThemeId | null; dark: boolean }>()

const en = {
  instance: 'As the instance',
  none: 'None',
}
const pt: typeof en = {
  instance: 'Como a instância',
  none: 'Nenhum',
}
const { locale } = useI18n()
const lang = computed(() => (locale.value.startsWith('pt') ? 'pt' : 'en'))
const L = computed(() => (lang.value === 'pt' ? pt : en))
const names = computed(() => DEV_THEME_NAMES[lang.value])
// "As the instance", and under it which theme that is (or none).
const instanceSub = computed(() => (props.instanceTheme ? names.value[props.instanceTheme] : L.value.none))
</script>

<template>
  <div class="dt-picker" role="radiogroup">
    <button type="button" class="dt-opt" :class="{ on: choice === '' }" role="radio" :aria-checked="choice === ''" @click="choice = ''">
      <span class="dt-ico"><Building2 class="h-4 w-4" /></span>
      <span class="dt-name">{{ L.instance }}<small>{{ instanceSub }}</small></span>
      <Check v-if="choice === ''" class="dt-check h-4 w-4" />
    </button>
    <button type="button" class="dt-opt" :class="{ on: choice === 'none' }" role="radio" :aria-checked="choice === 'none'" @click="choice = 'none'">
      <span class="dt-ico"><Ban class="h-4 w-4" /></span>
      <span class="dt-name">{{ L.none }}</span>
      <Check v-if="choice === 'none'" class="dt-check h-4 w-4" />
    </button>
    <button
      v-for="th in DEV_THEME_LIST"
      :key="th.id"
      type="button"
      class="dt-opt"
      :class="{ on: choice === th.id }"
      role="radio"
      :aria-checked="choice === th.id"
      :style="{ '--tc': th.accent }"
      @click="choice = th.id"
    >
      <img :src="resolveCrest(th, dark)" alt="" class="dt-crest" />
      <span class="dt-name">{{ names[th.id] }}</span>
      <Check v-if="choice === th.id" class="dt-check h-4 w-4" />
    </button>
  </div>
</template>

<style scoped>
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
  width: 28px;
  height: 28px;
  flex-shrink: 0;
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
  font-size: 11.5px;
  color: var(--text-3);
}
.dt-check {
  flex-shrink: 0;
  color: var(--tc, var(--accent));
}
</style>
