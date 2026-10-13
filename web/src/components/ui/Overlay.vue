<script setup lang="ts">
import { watch, onUnmounted } from 'vue'
import { X } from 'lucide-vue-next'

// Overlay renders its content either as a right-hand drawer ('side') or a
// centered modal ('centered'). The cert-detail layout honors the per-user pref.
const props = withDefaults(
  defineProps<{ open: boolean; title?: string; mode?: 'side' | 'centered'; wide?: boolean }>(),
  { mode: 'centered', wide: false },
)
const emit = defineEmits<{ close: [] }>()

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}

// Escape-to-close via a document listener (the overlay element isn't reliably
// focused), plus body scroll lock while open.
watch(
  () => props.open,
  (o) => {
    document.body.style.overflow = o ? 'hidden' : ''
    if (o) document.addEventListener('keydown', onKey)
    else document.removeEventListener('keydown', onKey)
  },
)

onUnmounted(() => {
  document.removeEventListener('keydown', onKey)
  document.body.style.overflow = ''
})
</script>

<template>
  <Teleport to="body">
    <Transition name="fade">
      <div
        v-if="open"
        class="fixed inset-0 z-50 bg-[rgba(15,23,42,.45)]"
        tabindex="-1"
        @click.self="emit('close')"
        @keydown="onKey"
      >
        <!-- Centered modal -->
        <div
          v-if="mode === 'centered'"
          role="dialog"
          aria-modal="true"
          :aria-label="title"
          class="absolute left-1/2 top-1/2 max-h-[88vh] w-full -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-2xl border border-border bg-surface shadow-soft"
          :class="wide ? 'max-w-3xl' : 'max-w-lg'"
        >
          <header v-if="title" class="flex items-center justify-between border-b border-border px-5 py-3.5">
            <h2 class="text-[15px] font-semibold">{{ title }}</h2>
            <button class="text-text-3 hover:text-text" aria-label="Close" @click="emit('close')"><X class="h-5 w-5" /></button>
          </header>
          <div class="p-5"><slot /></div>
        </div>

        <!-- Right drawer -->
        <div
          v-else
          role="dialog"
          aria-modal="true"
          :aria-label="title"
          class="absolute right-0 top-0 flex h-full w-full max-w-md flex-col border-l border-border bg-surface shadow-soft"
        >
          <header v-if="title" class="flex items-center justify-between border-b border-border px-5 py-3.5">
            <h2 class="text-[15px] font-semibold">{{ title }}</h2>
            <button class="text-text-3 hover:text-text" aria-label="Close" @click="emit('close')"><X class="h-5 w-5" /></button>
          </header>
          <div class="flex-1 overflow-y-auto p-5"><slot /></div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
