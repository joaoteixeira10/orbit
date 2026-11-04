<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Check, Droplet } from 'lucide-vue-next'
import type { NoteColor } from '@/api/planner'
import { NOTE_COLORS } from './noteColors'

// A row of note-colour swatches (the tints themselves), for a popover. The
// current one carries a tick; "Default" is the plain surface.
const color = defineModel<NoteColor>({ default: '' })
const { t } = useI18n()
const label = (c: NoteColor) => t(`notes.colors.${c || 'plain'}`)
</script>

<template>
  <div class="swatches" role="radiogroup" :aria-label="t('notes.color')">
    <button
      v-for="c in NOTE_COLORS"
      :key="c || 'plain'"
      type="button"
      role="radio"
      class="swatch"
      :class="{ on: color === c }"
      :data-note="c"
      :aria-checked="color === c"
      :aria-label="label(c)"
      :title="label(c)"
      @click="color = c"
    >
      <Check v-if="color === c" class="h-3.5 w-3.5" :stroke-width="3" />
      <Droplet v-else-if="!c" class="h-3.5 w-3.5 opacity-50" />
    </button>
  </div>
</template>

<style scoped>
.swatches {
  display: grid;
  grid-template-columns: repeat(6, 30px);
  gap: 8px;
  padding: 6px;
}
.swatch {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  border-radius: 999px;
  background: var(--note-bg);
  color: var(--text-2);
  box-shadow: inset 0 0 0 0.5px var(--border-2);
  transition: transform 0.15s var(--spring), box-shadow 0.15s var(--ease);
  cursor: pointer;
}
.swatch:hover {
  transform: scale(1.08);
}
.swatch.on {
  color: var(--text);
  box-shadow: inset 0 0 0 0.5px var(--border-2), 0 0 0 2px var(--surface), 0 0 0 3.5px var(--accent);
}
html.dark .swatch.on {
  box-shadow: inset 0 0 0 0.5px var(--border-2), 0 0 0 2px #2c2c2e, 0 0 0 3.5px var(--accent);
}
</style>
