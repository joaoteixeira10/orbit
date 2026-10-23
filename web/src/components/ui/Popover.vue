<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch, nextTick } from 'vue'
import { computePosition, autoUpdate, flip, shift, offset, type Placement } from '@floating-ui/dom'

// A popover anchored to an element (menus, pickers). It closes on a click
// outside, on Escape and when the anchor scrolls away.
const props = withDefaults(defineProps<{ anchor: HTMLElement | null; placement?: Placement; width?: number | string }>(), {
  placement: 'bottom-start',
  width: 'auto',
})
const emit = defineEmits<{ close: [] }>()
const el = ref<HTMLElement | null>(null)
let cleanup: (() => void) | null = null

async function place() {
  if (!props.anchor || !el.value) return
  const { x, y } = await computePosition(props.anchor, el.value, {
    placement: props.placement,
    strategy: 'fixed',
    middleware: [offset(6), flip({ padding: 8 }), shift({ padding: 8 })],
  })
  Object.assign(el.value.style, { left: `${x}px`, top: `${y}px` })
}

function onDown(e: MouseEvent) {
  const t = e.target as Node
  if (el.value?.contains(t) || props.anchor?.contains(t)) return
  // A popover opened from this one (later in the page): still "inside".
  const other = (t instanceof Element ? t : t.parentElement)?.closest('.popover')
  if (other && el.value && other !== el.value && el.value.compareDocumentPosition(other) & Node.DOCUMENT_POSITION_FOLLOWING) return
  emit('close')
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.stopPropagation()
    emit('close')
  }
}

onMounted(async () => {
  await nextTick()
  if (props.anchor && el.value) cleanup = autoUpdate(props.anchor, el.value, place)
  document.addEventListener('mousedown', onDown, true)
  document.addEventListener('keydown', onKey, true)
})
watch(() => props.anchor, place)
onBeforeUnmount(() => {
  cleanup?.()
  document.removeEventListener('mousedown', onDown, true)
  document.removeEventListener('keydown', onKey, true)
})
</script>

<template>
  <Teleport to="body">
    <Transition name="pop" appear>
      <div ref="el" class="popover fixed left-0 top-0 z-[60]" :style="{ width: typeof width === 'number' ? width + 'px' : width }" role="menu">
        <slot />
      </div>
    </Transition>
  </Teleport>
</template>
