<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import FileViewer from './FileViewer.vue'
import { closeFile, fileViewer } from '@/lib/fileView'

// The file viewer over whatever is open (openFile) — above a task's or a
// note's sheet it was opened from: Esc or ✕ closes it (and only it).
function onKey(e: KeyboardEvent) {
  if (e.key !== 'Escape' || !fileViewer.open) return
  e.preventDefault()
  e.stopImmediatePropagation()
  closeFile()
}
onMounted(() => document.addEventListener('keydown', onKey, true))
onUnmounted(() => document.removeEventListener('keydown', onKey, true))
</script>

<template>
  <Teleport to="body">
    <Transition name="sheet">
      <div v-if="fileViewer.open" class="viewer-backdrop" @click.self="closeFile">
        <div class="viewer-panel" role="dialog" aria-modal="true">
          <FileViewer :id="fileViewer.id" closable @close="closeFile" />
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.viewer-backdrop {
  position: fixed;
  inset: 0;
  z-index: 60; /* over the sheet it was opened from (sheets and dialogs are 50) */
  display: flex;
  padding: clamp(0px, 2.5vw, 28px);
  background: var(--backdrop);
}
.viewer-panel {
  display: flex;
  flex: 1;
  min-width: 0;
  flex-direction: column;
  overflow: hidden;
  border-radius: 16px;
  background: var(--bg);
  box-shadow: 0 12px 48px rgba(0, 0, 0, 0.25);
}
.viewer-panel > :deep(*) {
  flex: 1;
  min-height: 0;
}
@media (max-width: 640px) {
  .viewer-panel {
    border-radius: 0;
  }
}
</style>
