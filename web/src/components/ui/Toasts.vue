<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useToastStore } from '@/stores/toast'

const toasts = useToastStore()
const { t: tr } = useI18n()
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
        <button type="button" class="toast-x" :aria-label="tr('acct.dismiss')" @click="toasts.dismiss(t.id)">✕</button>
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
/* A toast: shadcn-vue's (Sonner's) — a card, the text, a close. */
.toast {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 14px 10px 14px 16px;
  border-radius: 10px;
  background: var(--surface);
  border: 1px solid var(--border);
  box-shadow: 0 10px 15px -3px rgba(0, 0, 0, 0.1), 0 4px 6px -4px rgba(0, 0, 0, 0.1);
  font-size: 13.5px;
}
.toast.error {
  border-color: var(--crit-border);
  color: var(--crit-text);
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
