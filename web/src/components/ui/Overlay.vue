<script setup lang="ts">
import { watch, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { X } from 'lucide-vue-next'

// Overlay is Orbit's sheet: a centred panel on wide screens, a bottom sheet
// on phones ('centered'), or an inspector sliding in from the right ('side').
// The title row is optional; #actions sits at its right (e.g. Done). bare:
// the content lays itself out (its own scrolling, header and footer).
const props = withDefaults(
  // top: above the other sheets (one asked for from inside them, as the lock
  // PIN — mounted once by the app shell, before them); popovers stay above.
  defineProps<{ open: boolean; title?: string; mode?: 'side' | 'centered'; wide?: boolean; size?: 'sm' | 'md' | 'lg' | 'xl' | 'xxl'; bare?: boolean; top?: boolean }>(),
  { mode: 'centered', wide: false, size: 'md', bare: false },
)
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()

function onKey(e: KeyboardEvent) {
  // An Escape something inside used (an editor's extra cursors, a menu) isn't to close.
  if (e.key === 'Escape' && !e.defaultPrevented) {
    e.stopPropagation()
    emit('close')
  }
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
  { immediate: true },
)

onUnmounted(() => {
  document.removeEventListener('keydown', onKey)
  document.body.style.overflow = ''
})

const widths = { sm: 'sm:max-w-[400px]', md: 'sm:max-w-[520px]', lg: 'sm:max-w-[680px]', xl: 'sm:max-w-[880px]', xxl: 'sm:max-w-[1060px]' }
</script>

<template>
  <Teleport to="body">
    <Transition name="sheet">
      <div v-if="open" class="fixed inset-0 flex items-end justify-center sm:items-center" :class="top ? 'z-[55]' : 'z-50'" style="background: var(--backdrop)" @mousedown.self="emit('close')">
        <!-- Sheet -->
        <div
          v-if="mode === 'centered'"
          role="dialog"
          aria-modal="true"
          :aria-label="title"
          class="sheet-panel sheet flex max-h-[92dvh] w-full flex-col overflow-hidden sm:max-h-[86vh] sm:w-[calc(100%-32px)]"
          :class="wide ? widths.lg : widths[size]"
        >
          <header v-if="title || $slots.actions" class="flex shrink-0 items-center gap-3 px-5 pb-2 pt-4">
            <h2 class="title-3 min-w-0 flex-1 truncate">{{ title }}</h2>
            <slot name="actions" />
            <button class="icon-btn -mr-1.5 h-7 w-7 rounded-full bg-fill" :aria-label="t('common.close')" @click="emit('close')"><X class="h-4 w-4" :stroke-width="2.4" /></button>
          </header>
          <div v-if="bare" class="flex min-h-0 flex-1 flex-col"><slot /></div>
          <div v-else class="min-h-0 flex-1 overflow-y-auto px-5 pb-5 pt-2"><slot /></div>
          <footer v-if="$slots.footer" class="flex shrink-0 items-center justify-end gap-2 border-t-[0.5px] border-border px-5 py-3">
            <slot name="footer" />
          </footer>
        </div>

        <!-- Inspector -->
        <div
          v-else
          role="dialog"
          aria-modal="true"
          :aria-label="title"
          class="sheet-panel absolute bottom-0 right-0 top-0 flex w-full max-w-md flex-col bg-surface shadow-lg2"
        >
          <header v-if="title" class="flex items-center justify-between border-b-[0.5px] border-border px-5 py-3.5">
            <h2 class="title-3">{{ title }}</h2>
            <button class="icon-btn h-7 w-7 rounded-full bg-fill" :aria-label="t('common.close')" @click="emit('close')"><X class="h-4 w-4" :stroke-width="2.4" /></button>
          </header>
          <div class="flex-1 overflow-y-auto p-5"><slot /></div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.sheet {
  background: var(--surface);
  border-radius: 16px 16px 0 0;
  box-shadow: var(--shadow-lg);
  padding-bottom: env(safe-area-inset-bottom);
}
@media (min-width: 640px) {
  .sheet {
    border-radius: 16px;
    padding-bottom: 0;
  }
}
html.dark .sheet {
  background: #2c2c2e;
}
</style>
