<script setup lang="ts">
import { ref, watch, nextTick, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { computePosition, autoUpdate, flip, shift, offset, size, type VirtualElement } from '@floating-ui/dom'
import type { SlashState } from './slash'

// The "/" block menu, anchored to the typed slash. The SlashCommand extension
// fills `state`; this only renders and positions it.
// label / empty: the heading and the no-match text (the blocks' by default).
const props = defineProps<{ state: SlashState; context: HTMLElement | null; label?: string; empty?: string }>()
const emit = defineEmits<{ hover: [index: number] }>()
const { t } = useI18n()
const el = ref<HTMLElement | null>(null)
const list = ref<HTMLElement | null>(null)
let stop: (() => void) | null = null

const virtual = (): VirtualElement => ({
  getBoundingClientRect: () => props.state.rect?.() ?? new DOMRect(),
  contextElement: props.context ?? undefined,
})

async function place() {
  if (!el.value || !props.state.rect) return
  const { x, y } = await computePosition(virtual(), el.value, {
    placement: 'bottom-start',
    strategy: 'fixed',
    middleware: [
      offset(8),
      flip({ padding: 8 }),
      shift({ padding: 8 }),
      // Never taller than the room left on screen.
      size({
        padding: 8,
        apply: ({ availableHeight, elements }) => {
          elements.floating.style.maxHeight = `${Math.max(140, Math.min(360, availableHeight))}px`
        },
      }),
    ],
  })
  if (el.value) Object.assign(el.value.style, { left: `${x}px`, top: `${y}px` })
}

watch(
  () => props.state.open,
  async (open) => {
    stop?.()
    stop = null
    if (!open) return
    await nextTick()
    if (el.value) stop = autoUpdate(virtual(), el.value, place)
  },
)
watch(() => [props.state.items, props.state.rect], () => void place())

// Keep the highlighted row in view while arrowing.
watch(
  () => props.state.index,
  async (i) => {
    await nextTick()
    list.value?.querySelectorAll<HTMLElement>('.slash-item')[i]?.scrollIntoView({ block: 'nearest' })
  },
)

onBeforeUnmount(() => stop?.())
</script>

<template>
  <Teleport to="body">
    <Transition name="pop">
      <div v-if="state.open" ref="el" class="slash-menu popover" role="listbox" :aria-label="label || t('editor.blocks')" @mousedown.prevent>
        <div ref="list" class="slash-scroll">
          <div class="cap px-2.5 pb-1 pt-1.5">{{ label || t('editor.blocks') }}</div>
          <button
            v-for="(it, i) in state.items"
            :key="it.key"
            type="button"
            role="option"
            class="slash-item"
            :class="{ on: i === state.index }"
            :aria-selected="i === state.index"
            @mouseenter="emit('hover', i)"
            @click="state.pick(it)"
          >
            <span class="slash-icon"><component :is="it.icon" class="h-[18px] w-[18px]" :stroke-width="1.8" /></span>
            <span class="min-w-0">
              <span class="block truncate text-[13.5px] font-medium">{{ it.title }}</span>
              <span class="block truncate text-[12px] text-text-3">{{ it.desc }}</span>
            </span>
          </button>
          <div v-if="!state.items.length" class="px-2.5 py-2 text-[13px] text-text-3">{{ empty || t('editor.noResults') }}</div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
