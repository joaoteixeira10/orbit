<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Trash2 } from 'lucide-vue-next'
import DrawingEditor from './DrawingEditor.vue'
import type { Drawing } from './drawing'

// A note's drawing, nearly the whole window: Excalidraw with Done (which
// saves first) and, for editors, Remove.
const props = defineProps<{ open: boolean; id: string; title?: string; readonly?: boolean }>()
const emit = defineEmits<{ close: []; saved: [d: Drawing]; remove: [] }>()
const { t } = useI18n()

const editor = ref<InstanceType<typeof DrawingEditor> | null>(null)
async function close() {
  await editor.value?.flush()
  emit('close')
}
function onKey(e: KeyboardEvent) {
  // Excalidraw uses Escape itself (deselect); only a bare one with nothing
  // being edited closes.
  if (e.key === 'Escape' && document.activeElement === document.body) void close()
}
watch(
  () => props.open,
  (o) => (o ? window.addEventListener('keydown', onKey) : window.removeEventListener('keydown', onKey)),
  { immediate: true },
)
</script>

<template>
  <Teleport to="body">
    <!-- No leave transition: the note's sheet may close with it, which
         would strand a leaving element in <body>. -->
    <div v-if="open && id" class="draw-sheet" role="dialog" aria-modal="true" :aria-label="title || t('draw.noteTitle')" data-testid="drawing-sheet">
        <header class="flex items-center gap-2 px-4 py-2.5">
          <h2 class="min-w-0 flex-1 truncate text-[15px] font-semibold">{{ title || t('draw.noteTitle') }}</h2>
          <button v-if="!readonly" class="btn-plain !text-danger" @click="emit('remove')"><Trash2 class="h-4 w-4" />{{ t('draw.removeNote') }}</button>
          <button class="btn-primary btn-sm" @click="close">{{ t('common.done') }}</button>
        </header>
        <div class="min-h-0 flex-1">
          <DrawingEditor ref="editor" :id="id" :readonly="readonly" @saved="(d) => emit('saved', d)" />
        </div>
    </div>
  </Teleport>
</template>

<style scoped>
.draw-sheet {
  position: fixed;
  inset: max(12px, env(safe-area-inset-top)) 12px 12px;
  z-index: 70;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-radius: 16px;
  background: var(--surface);
  box-shadow: 0 0 0 100vmax rgb(0 0 0 / 0.35), 0 20px 60px rgb(0 0 0 / 0.3);
}
@media (max-width: 639px) {
  .draw-sheet {
    inset: 0;
    border-radius: 0;
  }
}
</style>
