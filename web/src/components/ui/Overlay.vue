<script setup lang="ts">
import { watch, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { X } from 'lucide-vue-next'
import { Button } from '@/components/shadcn/button'

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
  // Nor while a shadcn popover, select or menu (reka-ui) is open: that one takes it.
  if (e.key === 'Escape' && !e.defaultPrevented && !document.querySelector('[data-dismissable-layer]')) {
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
      <!-- The backdrop: shadcn's dialog overlay (a light wash, the page blurred). -->
      <div v-if="open" class="sheet-backdrop fixed inset-0 flex items-end justify-center bg-black/10 supports-backdrop-filter:backdrop-blur-xs dark:bg-black/40 sm:items-center" :class="top ? 'z-55' : 'z-50'" @mousedown.self="emit('close')">
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
            <Button variant="ghost" size="icon-sm" class="-mr-1.5 text-text-2" :aria-label="t('common.close')" @click="emit('close')"><X /></Button>
          </header>
          <div v-if="bare" class="flex min-h-0 flex-1 flex-col"><slot /></div>
          <div v-else class="min-h-0 flex-1 overflow-y-auto px-5 pb-5 pt-2"><slot /></div>
          <footer v-if="$slots.footer" class="flex shrink-0 items-center justify-end gap-2 border-t border-border px-5 py-3">
            <slot name="footer" />
          </footer>
        </div>

        <!-- Inspector -->
        <div
          v-else
          role="dialog"
          aria-modal="true"
          :aria-label="title"
          class="sheet-panel absolute bottom-0 right-0 top-0 flex w-full max-w-md flex-col bg-(--sheet-bg) shadow-lg2"
        >
          <header v-if="title" class="flex items-center justify-between border-b border-border px-5 py-3.5">
            <h2 class="title-3">{{ title }}</h2>
            <Button variant="ghost" size="icon-sm" class="text-text-2" :aria-label="t('common.close')" @click="emit('close')"><X /></Button>
          </header>
          <div class="flex-1 overflow-y-auto p-5"><slot /></div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
/* shadcn-vue's dialog: rounded, a fine ring. */
.sheet {
  background: var(--sheet-bg);
  border-radius: 12px 12px 0 0;
  box-shadow: 0 0 0 1px var(--sheet-ring), var(--shadow-lg);
  padding-bottom: env(safe-area-inset-bottom);
}
@media (min-width: 640px) {
  .sheet {
    border-radius: 12px;
    padding-bottom: 0;
  }
}
</style>
