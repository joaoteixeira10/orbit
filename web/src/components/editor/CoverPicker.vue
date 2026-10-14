<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Check, Trash2 } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import { GRADIENTS, SOLIDS } from './covers'

// The cover choices: gradient presets and solid system colours.
defineProps<{ anchor: HTMLElement | null; current: string }>()
const emit = defineEmits<{ pick: [key: string]; close: [] }>()
const { t } = useI18n()

function pick(k: string) {
  emit('pick', k)
  emit('close')
}
</script>

<template>
  <Popover :anchor="anchor" :width="300" placement="bottom-end" @close="emit('close')">
    <div class="px-1.5 pb-1.5 pt-0.5 cap">{{ t('editor.gradients') }}</div>
    <div class="grid grid-cols-3 gap-1.5 px-1">
      <button
        v-for="(bg, name) in GRADIENTS"
        :key="name"
        type="button"
        class="swatch h-11"
        :style="{ background: bg }"
        :aria-label="name"
        @click="pick(`g:${name}`)"
      >
        <Check v-if="current === `g:${name}`" class="h-4 w-4" />
      </button>
    </div>
    <div class="px-1.5 pb-1.5 pt-3 cap">{{ t('editor.colors') }}</div>
    <div class="grid grid-cols-5 gap-1.5 px-1 pb-1">
      <button v-for="c in SOLIDS" :key="c" type="button" class="swatch h-8" :style="{ background: c }" :aria-label="c" @click="pick(c)">
        <Check v-if="current === c" class="h-4 w-4" />
      </button>
    </div>
    <template v-if="current">
      <div class="menu-sep" />
      <button class="menu-item" type="button" @click="pick('')"><Trash2 class="h-4 w-4 text-text-2" />{{ t('editor.remove') }}</button>
    </template>
  </Popover>
</template>

<style scoped>
.swatch {
  display: grid;
  place-items: center;
  width: 100%;
  border: 0;
  border-radius: 8px;
  color: #fff;
  cursor: pointer;
  box-shadow: inset 0 0 0 0.5px rgba(0, 0, 0, 0.1);
  transition: transform 0.12s var(--ease), box-shadow 0.12s var(--ease);
}
.swatch:hover {
  transform: scale(1.04);
}
.swatch svg {
  filter: drop-shadow(0 1px 1px rgba(0, 0, 0, 0.35));
}
</style>
