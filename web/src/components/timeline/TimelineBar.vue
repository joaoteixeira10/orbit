<script setup lang="ts">
import { computed } from 'vue'
import { Check } from 'lucide-vue-next'
import type { DragMode } from '@/lib/timeline'

// One task's bar on the timeline, in its list's colour like Calendar's
// events: a tint with a solid leading edge and the title inside when it fits
// (else just after the bar). Done tasks are striped and muted, late ones
// ringed in red, and sub-task progress is a thin fill along the bottom.
// Editors drag the bar (or an edge); the timeline does the dragging.
const props = defineProps<{
  title: string
  label: string // for screen readers: the title and the dates
  color: string
  left: number
  width: number
  done: boolean
  overdue: boolean
  subs?: { done: number; total: number }
  editable: boolean
  dragging: boolean
  linkTitle?: string // the handle that links another task to wait on it
}>()
const emit = defineEmits<{ grab: [mode: DragMode, e: PointerEvent]; open: []; nudge: [mode: DragMode, delta: number]; link: [e: PointerEvent] }>()

// Text widths, measured once per title.
let ctx: CanvasRenderingContext2D | null = null
const widths = new Map<string, number>()
function textWidth(s: string): number {
  let w = widths.get(s)
  if (w !== undefined) return w
  ctx ??= document.createElement('canvas').getContext('2d')
  if (!ctx) return s.length * 6.5
  ctx.font = `500 12px ${getComputedStyle(document.body).fontFamily}`
  w = ctx.measureText(s).width
  widths.set(s, w)
  return w
}
const inside = computed(() => props.width >= textWidth(props.title) + (props.done ? 34 : 20))
const narrow = computed(() => props.width < 26)
const pct = computed(() => (props.subs && props.subs.total ? (props.subs.done / props.subs.total) * 100 : 0))

function onKey(e: KeyboardEvent) {
  if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    emit('open')
  } else if (props.editable && (e.key === 'ArrowLeft' || e.key === 'ArrowRight')) {
    e.preventDefault()
    // ← → move the bar a day; with ⇧ they move its end (its length).
    emit('nudge', e.shiftKey ? 'end' : 'move', e.key === 'ArrowLeft' ? -1 : 1)
  }
}
</script>

<template>
  <div
    class="bar"
    :class="{ done, overdue, narrow, dragging, editable }"
    :style="{ left: `${left}px`, width: `${width}px`, '--c': color }"
    role="button"
    tabindex="0"
    :aria-label="label"
    :title="inside ? '' : title"
    @pointerdown="emit('grab', 'move', $event)"
    @keydown="onKey"
  >
    <span class="body">
      <Check v-if="done" class="ml-1.5 h-3 w-3 shrink-0" :stroke-width="3.2" />
      <span v-if="inside" class="t">{{ title }}</span>
      <i v-if="pct" class="prog" :style="{ width: `${pct}%` }" />
    </span>
    <template v-if="editable">
      <span class="edge l" @pointerdown.stop="emit('grab', 'start', $event)"><i /></span>
      <span class="edge r" @pointerdown.stop="emit('grab', 'end', $event)"><i /></span>
      <span v-if="linkTitle" class="link" :title="linkTitle" :aria-label="linkTitle" data-testid="tl-link" @pointerdown.stop.prevent="emit('link', $event)" />
    </template>
  </div>
  <span v-if="!inside" class="out" :class="{ done, overdue }" :style="{ left: `${left + width + 7}px` }">{{ title }}</span>
</template>

<style scoped>
.bar {
  position: absolute;
  top: 7px;
  height: 22px;
  z-index: 1;
  border-radius: 7px;
  outline: none;
  touch-action: pan-x pan-y;
  -webkit-touch-callout: none;
  user-select: none;
  cursor: pointer;
}
.bar.editable {
  cursor: grab;
}
.body {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  /* clip, not hidden: the title can stick to the left as the bar scrolls by. */
  overflow: clip;
  border-radius: inherit;
  background: color-mix(in srgb, var(--c) 22%, var(--bg));
  box-shadow: inset 3px 0 0 var(--c), inset 0 0 0 0.5px color-mix(in srgb, var(--c) 30%, transparent);
  color: color-mix(in srgb, var(--c) 62%, var(--text));
  transition: box-shadow var(--speed) var(--ease), filter var(--speed) var(--ease);
}
html.dark .body {
  background: color-mix(in srgb, var(--c) 40%, var(--bg));
  color: color-mix(in srgb, var(--c) 30%, var(--text));
}
.bar:hover .body {
  filter: brightness(0.97) saturate(1.1);
}
html.dark .bar:hover .body {
  filter: brightness(1.12);
}
.bar:focus-visible .body {
  box-shadow: inset 3px 0 0 var(--c), 0 0 0 3px var(--ring);
}
.t {
  position: sticky;
  left: calc(var(--name-w, 0px) + 4px);
  min-width: 0;
  padding: 0 8px 0 10px;
  overflow: hidden;
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
  text-overflow: ellipsis;
}
.done .t {
  padding-left: 5px;
}
.prog {
  position: absolute;
  left: 0;
  bottom: 0;
  height: 3px;
  background: var(--c);
  opacity: 0.75;
}

/* Late: a red ring. */
.overdue .body {
  box-shadow: inset 3px 0 0 var(--c), inset 0 0 0 1.5px var(--crit);
}
/* Done: muted and striped, with a check. */
.done .body {
  background: repeating-linear-gradient(-45deg, color-mix(in srgb, var(--c) 13%, transparent) 0 4px, transparent 4px 8px), color-mix(in srgb, var(--c) 7%, var(--bg));
  box-shadow: inset 3px 0 0 color-mix(in srgb, var(--c) 45%, transparent);
  color: var(--text-3);
}
.done .body svg {
  color: var(--c);
}

/* A day or two when zoomed out: a solid pill. */
.narrow .body {
  background: color-mix(in srgb, var(--c) 85%, var(--bg));
  box-shadow: inset 0 0 0 0.5px color-mix(in srgb, var(--c) 60%, #000);
}
.narrow.overdue .body {
  box-shadow: 0 0 0 1.5px var(--bg), 0 0 0 3px var(--crit);
}
.narrow.done .body {
  background: color-mix(in srgb, var(--c) 30%, var(--bg));
  box-shadow: none;
}
.narrow.done .body svg {
  display: none;
}

/* While dragged: lifted. */
.dragging {
  z-index: 3;
  cursor: grabbing;
}
.dragging .body {
  filter: none;
  box-shadow: inset 3px 0 0 var(--c), 0 0 0 0.5px color-mix(in srgb, var(--c) 50%, transparent), 0 6px 18px rgba(0, 0, 0, 0.18);
}

/* The edges that change the start and due dates: grips on hover. */
.edge {
  position: absolute;
  top: -3px;
  bottom: -3px;
  width: 9px;
  display: grid;
  place-items: center;
  cursor: ew-resize;
  touch-action: none;
}
.edge.l {
  left: -2px;
}
.edge.r {
  right: -2px;
}
.narrow .edge.l {
  left: -10px;
}
.narrow .edge.r {
  right: -10px;
}
.edge i {
  width: 3px;
  height: 12px;
  border-radius: 2px;
  background: var(--c);
  box-shadow: 0 0 0 1.5px var(--bg);
  opacity: 0;
  transition: opacity 0.12s var(--ease);
}
.bar:hover .edge i,
.dragging .edge i {
  opacity: 1;
}
@media (hover: none) {
  .edge {
    width: 14px;
  }
  .narrow .edge.l {
    left: -14px;
  }
  .narrow .edge.r {
    right: -14px;
  }
}

/* Drag from here onto another bar: that one waits on this one. */
.link {
  position: absolute;
  top: 50%;
  right: -17px;
  width: 11px;
  height: 11px;
  margin-top: -5.5px;
  border-radius: 999px;
  background: var(--bg);
  box-shadow: 0 0 0 1.5px var(--c);
  cursor: crosshair;
  touch-action: none;
  opacity: 0;
  transition: opacity 0.12s var(--ease);
}
.narrow .link {
  right: -25px;
}
.bar:hover .link,
.link:focus-visible {
  opacity: 1;
}
@media (hover: none) {
  .link {
    opacity: 0.8;
  }
}

/* The title beside a bar too short for it. */
.out {
  position: absolute;
  top: 0;
  height: 36px;
  display: flex;
  align-items: center;
  max-width: 320px;
  overflow: hidden;
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
  text-overflow: ellipsis;
  color: var(--text-2);
  pointer-events: none;
}
.out.overdue {
  color: var(--crit-text);
}
.out.done {
  color: var(--text-3);
}
</style>
