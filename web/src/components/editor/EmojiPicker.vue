<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Shuffle, Trash2 } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import { EMOJIS, randomEmoji } from './emojis'

// An emoji grid in a popover, for a page icon or a callout. "Remove" only
// shows when the caller allows an empty choice.
withDefaults(defineProps<{ anchor: HTMLElement | null; removable?: boolean }>(), { removable: false })
const emit = defineEmits<{ pick: [emoji: string]; close: [] }>()
const { t } = useI18n()

function pick(e: string) {
  emit('pick', e)
  emit('close')
}
</script>

<template>
  <Popover :anchor="anchor" :width="316" placement="bottom-start" @close="emit('close')">
    <div class="flex items-center justify-between px-1.5 pb-1 pt-0.5">
      <span class="cap">{{ t('editor.emoji') }}</span>
      <div class="flex items-center gap-0.5">
        <button class="btn-ghost btn-sm" type="button" @click="pick(randomEmoji())"><Shuffle class="h-3.5 w-3.5" />{{ t('editor.random') }}</button>
        <button v-if="removable" class="btn-ghost btn-sm" type="button" @click="pick('')"><Trash2 class="h-3.5 w-3.5" />{{ t('editor.remove') }}</button>
      </div>
    </div>
    <div class="emoji-grid">
      <button v-for="e in EMOJIS" :key="e" type="button" class="emoji-cell" :aria-label="e" @click="pick(e)">{{ e }}</button>
    </div>
  </Popover>
</template>

<style scoped>
.emoji-grid {
  display: grid;
  grid-template-columns: repeat(9, 1fr);
  gap: 2px;
  max-height: 260px;
  overflow-y: auto;
  padding: 2px;
}
.emoji-cell {
  display: grid;
  place-items: center;
  aspect-ratio: 1;
  font-size: 21px;
  line-height: 1;
  border: 0;
  border-radius: 7px;
  background: transparent;
  cursor: pointer;
  transition: background 0.12s var(--ease), transform 0.12s var(--ease);
}
.emoji-cell:hover {
  background: var(--fill);
}
.emoji-cell:active {
  transform: scale(0.9);
}
</style>
