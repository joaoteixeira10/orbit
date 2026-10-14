import { onBeforeUnmount, nextTick, watch, type Ref } from 'vue'

// useFillHeight sizes an element to reach the bottom of the viewport (less
// --cal-bottom, which leaves room for the phone tab bar), like Calendar's
// grids. The page still scrolls when the window is too short (min). The
// element may come and go (v-if); it is fitted whenever it appears.
export function useFillHeight(el: Ref<HTMLElement | null>, min = 360) {
  function fit() {
    const node = el.value
    if (!node) return
    const top = node.getBoundingClientRect().top + window.scrollY
    node.style.height = `max(${min}px, calc(100dvh - ${Math.round(top)}px - var(--cal-bottom, 16px)))`
  }
  const ro = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(() => fit()) : null
  watch(
    el,
    (node) => {
      ro?.disconnect()
      if (!node) return
      // The view grows when something above appears (a legend, a banner);
      // refitting to the same height doesn't loop.
      if (node.parentElement) ro?.observe(node.parentElement)
      void nextTick(fit)
    },
    { flush: 'post' },
  )
  window.addEventListener('resize', fit)
  onBeforeUnmount(() => {
    window.removeEventListener('resize', fit)
    ro?.disconnect()
  })
  return fit
}
