// useSortable wires SortableJS (drag and drop that also works with touch) to
// a container while Vue keeps owning the DOM: when a drag ends, the dragged
// element is put back where Vue rendered it and onMove is told what moved
// where, so the caller updates its data and Vue re-renders the new order.
import { onBeforeUnmount, watch, type Ref } from 'vue'
import Sortable, { type Options } from 'sortablejs'

export interface SortMove {
  id: string // data-id of the dragged element
  fromGroup: string // data-group of the container it left
  toGroup: string // data-group of the container it entered
  index: number // its new index among the target's items
  beforeId: string // data-id of the item now before it ("" = first)
}

// sortableOptions are the options for one container: ours, the caller's,
// and the end of a drag told to onMove (the element put back first).
function sortableOptions(onMove: (m: SortMove) => void, options: Options): Options {
  return {
    animation: 180,
    easing: 'cubic-bezier(0.25, 0.1, 0.25, 1)',
    ghostClass: 'sortable-ghost',
    chosenClass: 'sortable-chosen',
    dragClass: 'sortable-drag',
    delay: 120, // touch: a short press starts a drag, a swipe scrolls
    delayOnTouchOnly: true,
    forceFallback: false,
    ...options,
    onEnd: (e) => {
      const item = e.item as HTMLElement
      const from = e.from as HTMLElement
      const to = e.to as HTMLElement
      const newIndex = e.newDraggableIndex ?? e.newIndex ?? 0
      // Siblings in the target, after the drop, without the dragged one.
      const siblings = Array.from(to.children).filter((c) => c !== item && (c as HTMLElement).dataset.id) as HTMLElement[]
      const before = siblings[newIndex - 1]
      // Put the element back: Vue renders the new order from the data.
      const oldIndex = e.oldIndex ?? 0
      if (from !== to || oldIndex !== e.newIndex) {
        item.remove()
        from.insertBefore(item, from.children[oldIndex] ?? null)
      }
      if (from === to && oldIndex === e.newIndex) return
      onMove({
        id: item.dataset.id ?? '',
        fromGroup: from.dataset.group ?? '',
        toGroup: to.dataset.group ?? '',
        index: newIndex,
        beforeId: before?.dataset.id ?? '',
      })
    },
  }
}

export function useSortable(
  el: Ref<HTMLElement | null | undefined>,
  onMove: (m: SortMove) => void,
  options: Options = {},
) {
  let sortable: Sortable | null = null
  const stop = watch(
    el,
    (node) => {
      sortable?.destroy()
      sortable = null
      if (!node) return
      sortable = Sortable.create(node, sortableOptions(onMove, options))
    },
    { immediate: true, flush: 'post' },
  )
  onBeforeUnmount(() => {
    stop()
    sortable?.destroy()
  })
}

// useSortables is useSortable for a changing set of containers (by key):
// each one gets its own, and loses it when it goes.
export function useSortables(
  els: Ref<Record<string, HTMLElement | null>>,
  onMove: (m: SortMove) => void,
  options: Options = {},
) {
  const made = new Map<string, { node: HTMLElement; sortable: Sortable }>()
  const stop = watch(
    els,
    (now) => {
      for (const [k, v] of made) {
        if (now[k] !== v.node) {
          v.sortable.destroy()
          made.delete(k)
        }
      }
      for (const [k, node] of Object.entries(now)) {
        if (node && !made.has(k)) made.set(k, { node, sortable: Sortable.create(node, sortableOptions(onMove, options)) })
      }
    },
    { immediate: true, flush: 'post' },
  )
  onBeforeUnmount(() => {
    stop()
    for (const v of made.values()) v.sortable.destroy()
    made.clear()
  })
}
