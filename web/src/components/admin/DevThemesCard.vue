<script setup lang="ts">
// SHARED — from ../blueprint/shared/dev-themes (copied by blueprint/sync-themes.sh).
// Change it there, not here: the same file is in every project.
//
// Owner → Branding → Developer themes: the master switch, the four theme
// cards, and per asset the theme's or the owner's own (the accent always
// follows the theme). The view owns the form and saves it; this card only
// edits its fields. Its words are its own (English and Portuguese, below),
// so every project shows the same card without keys in its message files.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, Code2, Image as ImageIcon, ShieldCheck, SlidersHorizontal, Sparkles, Type } from 'lucide-vue-next'
import Toggle from '@/components/ui/Toggle.vue'
import { DEV_THEME_LIST, DEV_THEME_NAMES, type DevThemeId } from '@/lib/devThemes'

type Source = 'theme' | 'keep'
const enabled = defineModel<boolean>('enabled', { required: true })
const theme = defineModel<DevThemeId>('theme', { required: true })
const logo = defineModel<Source>('logo', { required: true })
const wordmark = defineModel<Source>('wordmark', { required: true })
const favicon = defineModel<Source>('favicon', { required: true })
// Optional: when bound, a switch lets each person pick their own theme in
// Settings (a project without per-person themes just doesn't bind it).
const personal = defineModel<boolean | undefined>('personal')
defineProps<{ hasLogo: boolean; hasFavicon: boolean }>()

const en = {
  title: 'Developer themes',
  on: 'On',
  off: 'Off',
  enable: 'Enable developer themes',
  enableHint: 'Locks the accent to a curated theme and swaps in the matching UBonito crest, wordmark and favicon for everyone.',
  pick: 'Theme — sets the accent colour',
  assets: 'Apply the theme’s assets — or keep your own',
  mark: 'Logo / mark',
  hasLogo: 'You have an uploaded logo',
  noLogo: 'No custom logo uploaded',
  themeCrest: 'Theme crest',
  myLogo: 'My logo',
  default: 'Default',
  wordmark: 'Wordmark',
  wordmarkHint: 'Blackletter “UBonito” or your application name',
  themeWordmark: 'Theme wordmark',
  appName: 'App name',
  favicon: 'Favicon',
  hasFavicon: 'You have an uploaded favicon',
  noFavicon: 'No custom favicon uploaded',
  themeFavicon: 'Theme favicon',
  myFavicon: 'My favicon',
  accent: 'Accent colour',
  accentHint: 'Set by the theme selected above',
  alwaysTheme: 'Always theme',
  personal: 'Let each person choose',
  personalHint: 'Everyone can pick another theme, or none, for themselves in Settings → Appearance. The one above stays the default and the sign-in page’s.',
}
const pt: typeof en = {
  title: 'Temas de programador',
  on: 'Ligados',
  off: 'Desligados',
  enable: 'Ativar os temas de programador',
  enableHint: 'Fixa a cor de destaque num tema escolhido e troca o brasão, o logótipo escrito e o favicon UBonito correspondentes, para todos.',
  pick: 'Tema — define a cor de destaque',
  assets: 'Usar os elementos do tema — ou manter os seus',
  mark: 'Logótipo / símbolo',
  hasLogo: 'Tem um logótipo carregado',
  noLogo: 'Nenhum logótipo próprio carregado',
  themeCrest: 'Brasão do tema',
  myLogo: 'O meu logótipo',
  default: 'Predefinido',
  wordmark: 'Logótipo escrito',
  wordmarkHint: '“UBonito” em letra gótica ou o nome da aplicação',
  themeWordmark: 'Do tema',
  appName: 'Nome da aplicação',
  favicon: 'Favicon',
  hasFavicon: 'Tem um favicon carregado',
  noFavicon: 'Nenhum favicon próprio carregado',
  themeFavicon: 'Favicon do tema',
  myFavicon: 'O meu favicon',
  accent: 'Cor de destaque',
  accentHint: 'Definida pelo tema escolhido acima',
  alwaysTheme: 'Sempre do tema',
  personal: 'Cada pessoa pode escolher',
  personalHint: 'Cada um pode escolher outro tema, ou nenhum, para si em Definições → Aspeto. O de cima continua a ser o predefinido e o da página de entrada.',
}
const { locale } = useI18n()
const lang = computed(() => (locale.value.startsWith('pt') ? 'pt' : 'en'))
const L = computed(() => (lang.value === 'pt' ? pt : en))
const names = computed(() => DEV_THEME_NAMES[lang.value])
</script>

<template>
  <div class="card overflow-hidden">
    <div class="flex items-center gap-2 border-b border-border px-5 py-3">
      <Code2 class="h-[18px] w-[18px] text-text-2" /><h3 class="text-[14px] font-semibold">{{ L.title }}</h3>
      <span class="ml-1 rounded-full px-2 py-0.5 text-[11px] font-semibold" :class="enabled ? 'bg-accent-bg text-accent-text' : 'bg-surface-3 text-text-3'">{{ enabled ? L.on : L.off }}</span>
    </div>
    <div class="p-5">
      <div class="dev-master">
        <div class="flex items-center gap-[13px]">
          <div class="dm-ico"><Sparkles class="h-[19px] w-[19px]" /></div>
          <div>
            <div class="text-[15px] font-semibold">{{ L.enable }}</div>
            <p class="mt-[3px] max-w-[420px] text-[12.5px] leading-snug text-text-3">{{ L.enableHint }}</p>
          </div>
        </div>
        <Toggle v-model="enabled" />
      </div>
      <div v-if="personal !== undefined" class="src-row mt-2">
        <div class="min-w-0 flex-1 pl-1">
          <b class="text-[14px] font-semibold">{{ L.personal }}</b>
          <span class="mt-0.5 block max-w-[460px] text-[12px] leading-snug text-text-3">{{ L.personalHint }}</span>
        </div>
        <Toggle v-model="personal" />
      </div>

      <div v-if="enabled" class="mt-[22px]">
        <div class="mb-[13px] text-[11px] font-semibold uppercase tracking-wider text-text-faint">{{ L.pick }}</div>
        <div class="theme-grid">
          <button
            v-for="th in DEV_THEME_LIST"
            :key="th.id"
            type="button"
            class="theme-card"
            :class="{ on: theme === th.id }"
            :aria-pressed="theme === th.id"
            :style="{ '--tc': th.accent, '--tc-ground': th.ground, '--tc-badge': th.badge, '--tc-mark': th.mark, '--tc-halo': th.halo }"
            @click="theme = th.id"
          >
            <span class="tc-check"><Check class="h-[13px] w-[13px]" /></span>
            <div class="theme-prev">
              <img :src="th.crest" alt="" class="h-[50px] w-[50px]" />
              <div class="tp-word blackletter">UBonito</div>
            </div>
            <div class="theme-foot">
              <span class="tf-dot" /><b class="text-[13.5px] font-semibold">{{ names[th.id] }}</b><span class="tf-hex">{{ th.accent }}</span>
            </div>
          </button>
        </div>

        <div class="mt-6">
          <div class="mb-[13px] text-[11px] font-semibold uppercase tracking-wider text-text-faint">{{ L.assets }}</div>
          <div class="src-row">
            <div class="src-ico"><ImageIcon class="h-4 w-4" /></div>
            <div class="min-w-0 flex-1"><b class="text-[14px] font-semibold">{{ L.mark }}</b><span class="mt-0.5 block text-[12px] text-text-3">{{ hasLogo ? L.hasLogo : L.noLogo }}</span></div>
            <div class="seg">
              <button type="button" :class="{ on: logo === 'theme' }" @click="logo = 'theme'">{{ L.themeCrest }}</button>
              <button type="button" class="keep" :class="{ on: logo === 'keep' }" @click="logo = 'keep'">{{ hasLogo ? L.myLogo : L.default }}</button>
            </div>
          </div>
          <div class="src-row">
            <div class="src-ico"><Type class="h-4 w-4" /></div>
            <div class="min-w-0 flex-1"><b class="text-[14px] font-semibold">{{ L.wordmark }}</b><span class="mt-0.5 block text-[12px] text-text-3">{{ L.wordmarkHint }}</span></div>
            <div class="seg">
              <button type="button" :class="{ on: wordmark === 'theme' }" @click="wordmark = 'theme'">{{ L.themeWordmark }}</button>
              <button type="button" class="keep" :class="{ on: wordmark === 'keep' }" @click="wordmark = 'keep'">{{ L.appName }}</button>
            </div>
          </div>
          <div class="src-row">
            <div class="src-ico"><ShieldCheck class="h-4 w-4" /></div>
            <div class="min-w-0 flex-1"><b class="text-[14px] font-semibold">{{ L.favicon }}</b><span class="mt-0.5 block text-[12px] text-text-3">{{ hasFavicon ? L.hasFavicon : L.noFavicon }}</span></div>
            <div class="seg">
              <button type="button" :class="{ on: favicon === 'theme' }" @click="favicon = 'theme'">{{ L.themeFavicon }}</button>
              <button type="button" class="keep" :class="{ on: favicon === 'keep' }" @click="favicon = 'keep'">{{ hasFavicon ? L.myFavicon : L.default }}</button>
            </div>
          </div>
          <div class="src-row forced">
            <div class="src-ico"><SlidersHorizontal class="h-4 w-4" /></div>
            <div class="min-w-0 flex-1"><b class="text-[14px] font-semibold">{{ L.accent }}</b><span class="mt-0.5 block text-[12px] text-text-3">{{ L.accentHint }}</span></div>
            <span class="src-badge">{{ L.alwaysTheme }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
