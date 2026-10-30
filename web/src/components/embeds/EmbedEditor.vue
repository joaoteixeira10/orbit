<script setup lang="ts">
import { computed, defineAsyncComponent, onUnmounted, watch } from 'vue'
import { closeEmbed, embedEditor } from '@/lib/embeds'

// The big editor over a page: a sheet, a diagram or a whiteboard of a page
// opened in place (its own view, embedded), Done putting it back. One for
// the app (App.vue); lib/embeds opens it.
const SheetView = defineAsyncComponent(() => import('@/views/SheetView.vue'))
const DiagramView = defineAsyncComponent(() => import('@/views/DiagramView.vue'))
const WhiteboardView = defineAsyncComponent(() => import('@/views/WhiteboardView.vue'))
const view = computed(() => (embedEditor.kind === 'sheet' ? SheetView : embedEditor.kind === 'diagram' ? DiagramView : WhiteboardView))

watch(
  () => embedEditor.open,
  (o) => (document.body.style.overflow = o ? 'hidden' : ''),
)
onUnmounted(() => (document.body.style.overflow = ''))
</script>

<template>
  <Teleport to="body">
    <Transition name="sheet">
      <div v-if="embedEditor.open" class="embed-backdrop" data-testid="embed-editor">
        <div class="embed-panel" role="dialog" aria-modal="true">
          <component :is="view" :key="embedEditor.id" :ws="embedEditor.ws" :id="embedEditor.id" embedded @close="closeEmbed" />
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.embed-backdrop {
  position: fixed;
  inset: 0;
  z-index: 50; /* as dialogs: one opened from here comes later, on top */
  display: flex;
  padding: clamp(0px, 2.5vw, 28px);
  background: var(--backdrop);
}
.embed-panel {
  display: flex;
  flex: 1;
  min-width: 0;
  flex-direction: column;
  overflow: hidden;
  border-radius: 16px;
  background: var(--bg);
  box-shadow: 0 12px 48px rgba(0, 0, 0, 0.25);
}
.embed-panel > :deep(*) {
  flex: 1;
  min-height: 0;
}
@media (max-width: 640px) {
  .embed-panel {
    border-radius: 0;
  }
}
</style>
