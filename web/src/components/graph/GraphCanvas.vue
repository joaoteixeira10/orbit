<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Maximize, Minus, Plus } from 'lucide-vue-next'
import { KIND_COLOR, degrees, layout, type Graph, type GraphNode } from '@/lib/graph'

// A graph drawn: each thing a dot (its kind's colour, bigger with more
// links), each link a line (embeds thicker, sub-pages and folders dashed,
// mentions dotted). Drag to move around, the wheel (or the buttons) to zoom
// — in a small one, with ⌘/Ctrl so the page still scrolls. Pointing at a dot
// lights it and its neighbours; a click opens it. focus: the one it's about
// (in the middle, ringed).
const props = defineProps<{ graph: Graph; focus?: string; compact?: boolean; search?: string }>()
const emit = defineEmits<{ open: [node: GraphNode] }>()
const { t } = useI18n()

const box = ref<HTMLElement | null>(null)
const size = ref({ w: 600, h: 400 })
let ro: ResizeObserver | null = null
onMounted(() => {
  ro = new ResizeObserver(() => {
    const r = box.value?.getBoundingClientRect()
    if (r && r.width && r.height) size.value = { w: r.width, h: r.height }
  })
  if (box.value) ro.observe(box.value)
})
onBeforeUnmount(() => ro?.disconnect())

const pos = computed(() => layout(props.graph, props.focus ? { pinned: { [props.focus]: { x: 0, y: 0 } } } : {}))
const deg = computed(() => degrees(props.graph))
// Small dots in a small graph (as Folio's), bigger with more links.
const radius = (n: GraphNode) =>
  props.compact ? Math.min(7, 2.6 + Math.sqrt(deg.value.get(n.id) ?? 0) * 1.1) + (n.id === props.focus ? 2.5 : 0) : Math.min(16, 4.5 + Math.sqrt(deg.value.get(n.id) ?? 0) * 2.2) + (n.id === props.focus ? 3 : 0)
const colorOf = (n: GraphNode) => (n.kind === 'list' || n.kind === 'folder' ? n.color || KIND_COLOR[n.kind] : KIND_COLOR[n.kind])

// ---- The view: scale and offset; fitted to the graph ----
const scale = ref(1)
const tx = ref(0)
const ty = ref(0)
function fit() {
  const ps = [...pos.value.values()]
  if (!ps.length) return
  const xs = ps.map((p) => p.x)
  const ys = ps.map((p) => p.y)
  const minX = Math.min(...xs)
  const maxX = Math.max(...xs)
  const minY = Math.min(...ys)
  const maxY = Math.max(...ys)
  const pad = 48
  const s = Math.min((size.value.w - pad * 2) / Math.max(maxX - minX, 1), (size.value.h - pad * 2) / Math.max(maxY - minY, 1), 1.6)
  scale.value = Math.max(0.05, s)
  tx.value = size.value.w / 2 - ((minX + maxX) / 2) * scale.value
  ty.value = size.value.h / 2 - ((minY + maxY) / 2) * scale.value
}
watch([pos, size], fit, { immediate: true })

function zoomAt(f: number, cx = size.value.w / 2, cy = size.value.h / 2) {
  const s = Math.min(6, Math.max(0.05, scale.value * f))
  const k = s / scale.value
  tx.value = cx - (cx - tx.value) * k
  ty.value = cy - (cy - ty.value) * k
  scale.value = s
}
function onWheel(e: WheelEvent) {
  if (props.compact && !(e.metaKey || e.ctrlKey)) return
  e.preventDefault()
  const r = box.value!.getBoundingClientRect()
  zoomAt(Math.exp(-e.deltaY * 0.0015), e.clientX - r.left, e.clientY - r.top)
}

// Drag the background to move around.
let drag: { x: number; y: number; tx: number; ty: number; moved: boolean } | null = null
function down(e: PointerEvent) {
  if ((e.target as Element).closest('.g-node')) return
  drag = { x: e.clientX, y: e.clientY, tx: tx.value, ty: ty.value, moved: false }
  ;(e.currentTarget as Element).setPointerCapture(e.pointerId)
}
function move(e: PointerEvent) {
  if (!drag) return
  const dx = e.clientX - drag.x
  const dy = e.clientY - drag.y
  if (Math.abs(dx) + Math.abs(dy) > 3) drag.moved = true
  tx.value = drag.tx + dx
  ty.value = drag.ty + dy
}
function up() {
  drag = null
}

// ---- Pointing: it and its neighbours light up ----
const hover = ref<string | null>(null)
const near = computed(() => {
  const id = hover.value
  if (!id) return null
  const s = new Set([id])
  for (const e of props.graph.edges) {
    if (e.from === id) s.add(e.to)
    if (e.to === id) s.add(e.from)
  }
  return s
})
const q = computed(() => (props.search ?? '').trim().toLowerCase())
const hits = computed(() => (q.value ? new Set(props.graph.nodes.filter((n) => n.title.toLowerCase().includes(q.value)).map((n) => n.id)) : null))
const dim = (id: string) => (near.value ? !near.value.has(id) : hits.value ? !hits.value.has(id) : false)
// Labels: the one it's about, the one pointed at and its neighbours, the
// ones found; all of them when there are few (or when zoomed in).
const labelled = (n: GraphNode) =>
  n.id === props.focus ||
  near.value?.has(n.id) ||
  hits.value?.has(n.id) ||
  props.graph.nodes.length <= (props.compact ? 6 : 40) ||
  (!props.compact && scale.value * radius(n) >= 9)

const EDGE_DASH: Record<string, string> = { parent: '5 4', folder: '5 4', mention: '1.5 4' }
const label = (n: GraphNode) => {
  const s = (n.icon ? `${n.icon} ` : '') + (n.title || t('graph.untitled'))
  return s.length > 34 ? `${s.slice(0, 33)}…` : s
}
function click(n: GraphNode) {
  emit('open', n)
}
</script>

<template>
  <div ref="box" class="graph-canvas" :class="{ compact }" data-testid="graph-canvas">
    <svg :width="size.w" :height="size.h" @wheel="onWheel" @pointerdown="down" @pointermove="move" @pointerup="up" @pointercancel="up">
      <g :transform="`translate(${tx} ${ty}) scale(${scale})`">
        <line
          v-for="(e, i) in graph.edges"
          :key="i"
          :x1="pos.get(e.from)?.x"
          :y1="pos.get(e.from)?.y"
          :x2="pos.get(e.to)?.x"
          :y2="pos.get(e.to)?.y"
          class="g-edge"
          :class="{ dim: near ? !(near.has(e.from) && near.has(e.to) && (e.from === hover || e.to === hover)) : !!hits }"
          :stroke-width="(e.kind === 'embed' ? 2.2 : 1.2) / Math.max(scale, 0.4)"
          :stroke-dasharray="EDGE_DASH[e.kind]"
        />
        <g
          v-for="n in graph.nodes"
          :key="n.id"
          class="g-node"
          :class="{ dim: dim(n.id), focus: n.id === focus }"
          :transform="`translate(${pos.get(n.id)?.x ?? 0} ${pos.get(n.id)?.y ?? 0})`"
          :data-kind="n.kind"
          :data-id="n.id"
          data-testid="graph-node"
          role="button"
          :aria-label="label(n)"
          @pointerenter="hover = n.id"
          @pointerleave="hover = null"
          @click="click(n)"
        >
          <circle v-if="n.id === focus" :r="(radius(n) + 4) / Math.sqrt(Math.max(scale, 0.3))" class="g-ring" :stroke="colorOf(n)" />
          <circle :r="(radius(n) + 8) / Math.sqrt(Math.max(scale, 0.3))" class="g-hit" />
          <circle :r="radius(n) / Math.sqrt(Math.max(scale, 0.3))" :fill="colorOf(n)" class="g-dot" />
          <text v-if="labelled(n)" :y="(radius(n) + (compact ? 10 : 13)) / Math.max(scale, 0.3)" :font-size="(compact ? 10 : 12) / Math.max(scale, 0.3)" text-anchor="middle">{{ label(n) }}</text>
        </g>
      </g>
    </svg>
    <div v-if="!compact" class="g-zoom">
      <button type="button" class="icon-btn h-8 w-8" :aria-label="t('graph.zoomIn')" @click="zoomAt(1.3)"><Plus class="h-4 w-4" /></button>
      <button type="button" class="icon-btn h-8 w-8" :aria-label="t('graph.zoomOut')" @click="zoomAt(1 / 1.3)"><Minus class="h-4 w-4" /></button>
      <button type="button" class="icon-btn h-8 w-8" :aria-label="t('graph.fit')" @click="fit"><Maximize class="h-4 w-4" /></button>
    </div>
  </div>
</template>

<style scoped>
.graph-canvas {
  position: relative;
  height: 100%;
  min-height: 160px;
  overflow: hidden;
  touch-action: none;
  user-select: none;
  cursor: grab;
}
.graph-canvas:active {
  cursor: grabbing;
}
svg {
  display: block;
}
.g-edge {
  stroke: var(--text-3);
  stroke-opacity: 0.45;
}
.compact .g-edge {
  stroke: color-mix(in srgb, var(--accent) 55%, var(--text-3));
  stroke-opacity: 0.35;
  transition: stroke-opacity 0.15s;
}
.g-edge.dim {
  stroke-opacity: 0.08;
}
.g-node {
  cursor: pointer;
  transition: opacity 0.15s;
}
.g-node.dim {
  opacity: 0.18;
}
.g-node text {
  fill: var(--text);
  paint-order: stroke;
  stroke: var(--bg);
  stroke-width: 3px;
  stroke-linejoin: round;
}
.g-hit {
  fill: transparent;
}
.g-node:hover .g-dot {
  stroke: var(--bg);
  stroke-width: 2px;
}
.g-ring {
  fill: none;
  stroke-width: 2px;
}
.g-zoom {
  position: absolute;
  right: 10px;
  bottom: 10px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 3px;
  border-radius: 11px;
  background: color-mix(in srgb, var(--surface) 92%, transparent);
  box-shadow: 0 0 0 0.5px var(--border), 0 2px 8px rgba(0, 0, 0, 0.08);
}
</style>
