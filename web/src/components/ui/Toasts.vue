<script setup lang="ts">
import { useToastStore } from '@/stores/toast'

const toasts = useToastStore()
</script>

<template>
  <!-- v0's toasts: top right, a coloured left edge, errors with details. -->
  <div class="toasts" aria-live="polite">
    <TransitionGroup name="toast">
      <div v-for="t in toasts.toasts" :key="t.id" class="toast" :class="t.kind" :role="t.kind === 'error' ? 'alert' : undefined">
        <div class="toast-text">
          <b>{{ t.message }}</b>
          <pre v-if="t.detail">{{ t.detail }}</pre>
        </div>
        <button type="button" class="toast-x" aria-label="Dismiss" @click="toasts.dismiss(t.id)">✕</button>
      </div>
    </TransitionGroup>
  </div>
</template>

<style scoped>
.toasts {
  position: fixed;
  top: 60px;
  right: 16px;
  z-index: 300;
  display: grid;
  gap: 8px;
  width: min(400px, calc(100vw - 32px));
}
.toast {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 10px 10px 10px 14px;
  border-radius: 10px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-left: 3px solid var(--ok);
  box-shadow: var(--shadow);
  font-size: 13px;
}
.toast.error {
  border-left-color: var(--crit);
}
.toast.info {
  border-left-color: var(--accent);
}
.toast-text {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}
.toast.error b {
  color: var(--crit);
}
.toast pre {
  margin: 6px 0 0;
  font: 12px/1.5 var(--font-mono);
  white-space: pre-wrap;
  max-height: 200px;
  overflow: auto;
  color: var(--text);
}
.toast-x {
  border: 0;
  background: transparent;
  color: var(--text-3);
  padding: 2px 6px;
  font-size: 12px;
  cursor: pointer;
}
.toast-enter-active,
.toast-leave-active {
  transition: all 0.2s ease;
}
.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translateX(12px);
}
</style>
