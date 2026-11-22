<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Check } from '@lucide/vue'
import type { NoteColor } from '@/api/planner'
import { NOTE_COLORS, noteTint } from './noteColors'

// The note colours, shadcn style: one row of solid
// dots (each colour's ink), a fine ring, a tick on the current one; "Plain"
// is the surface.
const color = defineModel<NoteColor>({ default: '' })
const { t } = useI18n()
const label = (c: NoteColor) => t(`notes.colors.${c || 'plain'}`)
</script>

<template>
  <div class="flex items-center gap-1.5 p-1" role="radiogroup" :aria-label="t('notes.color')">
    <button
      v-for="c in NOTE_COLORS"
      :key="c || 'plain'"
      type="button"
      role="radio"
      class="grid size-7 place-items-center rounded-full ring-1 ring-black/10 transition-transform hover:scale-110 dark:ring-white/15"
      :style="{ background: c ? noteTint(c).ink : 'var(--surface)' }"
      :aria-checked="color === c"
      :aria-label="label(c)"
      :title="label(c)"
      @click="color = c"
    >
      <Check v-if="color === c" class="size-3.5" :class="c ? 'text-white' : 'text-text'" :stroke-width="3" />
    </button>
  </div>
</template>
