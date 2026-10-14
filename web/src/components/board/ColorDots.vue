<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Check, Ban } from 'lucide-vue-next'
import { COLORS } from '@/lib/icons'

// A compact row of Apple system colours plus "none", for a board column's
// optional colour (columns fall back to a neutral dot).
const color = defineModel<string>({ default: '' })
const { t } = useI18n()
const same = (a: string, b: string) => a.toLowerCase() === b.toLowerCase()
</script>

<template>
  <div class="grid grid-cols-7 gap-1.5 p-1.5">
    <button
      type="button"
      class="dot bg-fill text-text-3"
      :class="{ on: !color }"
      :title="t('board.noColour')"
      :aria-label="t('board.noColour')"
      :aria-pressed="!color"
      @click="color = ''"
    >
      <Ban class="h-3.5 w-3.5" />
    </button>
    <button
      v-for="c in COLORS"
      :key="c"
      type="button"
      class="dot"
      :class="{ on: same(color, c) }"
      :style="{ background: c, '--c': c }"
      :aria-label="c"
      :aria-pressed="same(color, c)"
      @click="color = c"
    >
      <Check v-if="same(color, c)" class="h-3 w-3 text-white" :stroke-width="3.2" />
    </button>
  </div>
</template>

<style scoped>
.dot {
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  border-radius: 999px;
  transition: transform 0.15s var(--spring), box-shadow 0.15s var(--ease);
}
.dot:hover {
  transform: scale(1.12);
}
.dot.on {
  box-shadow: 0 0 0 2px var(--surface), 0 0 0 3.5px var(--c, var(--text-3));
}
</style>
