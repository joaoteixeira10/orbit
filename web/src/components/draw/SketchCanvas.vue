<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, shallowRef, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Brush, Copy, Eraser, Eye, EyeOff, FileImage, Hand, Highlighter, Lasso, Layers, NotebookText, PaintBucket, Pen, PenLine, Pencil, Pipette, Plus,
  Redo2, Trash2, Undo2, ChevronUp, ChevronDown, ZoomIn, ZoomOut,
} from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import ColorPicker from './ColorPicker.vue'
import {
  DEFAULT_SIZE, PAPER_COLORS, PAPERS, TOOLS, floodFill, hits, isSketch, lassoed, looks, moved, newId, newLayer, outline, readSketch,
  recognize, regionSvg, rub, toSvg, unionBounds, wipe, type Item, type Layer, type Paper, type Picture, type Sketch, type Stroke, type Tool,
} from '@/lib/sketch'
import { useToastStore } from '@/stores/toast'
import type { Snapshot } from './drawing'

// Quick Draw's own canvas: pens, a pencil, a marker, a highlighter and a
// brush that answer a stylus's pressure; any colour; an eraser that takes
// out just what it rubs (or whole strokes); a lasso to move, recolour,
// copy or delete; a bucket that fills what the lines close off; hold at the end of a stroke and it snaps into a clean
// line, circle, ellipse, triangle or rectangle; layers, with a photo or a
// PDF page under the ink; plain, lined, squared or dotted paper. With a
// stylus, fingers only move and zoom (the palm can rest); two fingers
// tapped undo, three redo. It hands over the same snapshot as Excalidraw's
// canvas (the scene, an SVG picture), so it saves the same way.
const props = defineProps<{ scene: string; readonly?: boolean; name?: string }>()
const emit = defineEmits<{ change: [] }>()
const { t } = useI18n()
const toast = useToastStore()

const layerName = (n: number) => t('sketch.layerN', { n })
const sketch = shallowRef<Sketch>(readSketch(props.scene, layerName(1)))
const active = ref(sketch.value.layers[sketch.value.layers.length - 1].id)

// ---- History (each step a version of the layers; strokes are never changed in place) ----
const past: Layer[][] = []
const future: Layer[][] = []
const canUndo = ref(false)
const canRedo = ref(false)
function commit(layers: Layer[], keep = true) {
  if (keep) {
    past.push(sketch.value.layers)
    if (past.length > 200) past.shift()
    future.length = 0
  }
  sketch.value = { ...sketch.value, layers }
  canUndo.value = past.length > 0
  canRedo.value = future.length > 0
  emit('change')
}
function undo() {
  const prev = past.pop()
  if (!prev) return
  future.push(sketch.value.layers)
  clearSelection()
  sketch.value = { ...sketch.value, layers: prev }
  if (!prev.some((l) => l.id === active.value)) active.value = prev[prev.length - 1].id
  canUndo.value = past.length > 0
  canRedo.value = true
  emit('change')
}
function redo() {
  const next = future.pop()
  if (!next) return
  past.push(sketch.value.layers)
  clearSelection()
  sketch.value = { ...sketch.value, layers: next }
  if (!next.some((l) => l.id === active.value)) active.value = next[next.length - 1].id
  canUndo.value = true
  canRedo.value = future.length > 0
  emit('change')
}
const layerOf = (id = active.value) => sketch.value.layers.find((l) => l.id === id) ?? sketch.value.layers[0]
// withItems: the layers, one layer's items changed.
const withItems = (id: string, items: Item[]) => sketch.value.layers.map((l) => (l.id === id ? { ...l, items } : l))

// ---- Tools ----
type Mode = Tool | 'eraser' | 'lasso' | 'dropper' | 'hand' | 'fill'
const mode = ref<Mode>('pen')
const lastPen = ref<Tool>('pen')
const RECENT_KEY = 'orbit_sketch_recent'
const color = ref('#1c1c1e')
const opacity = ref(1)
const sizes = ref<Record<Tool, number>>({ ...DEFAULT_SIZE })
const eraserSize = ref(12)
const eraseWhole = ref(false)
const recent = ref<string[]>(readRecent())
function readRecent(): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]')
    return Array.isArray(v) ? v.filter((c) => typeof c === 'string' && /^#[0-9a-f]{6}$/i.test(c)).slice(0, 10) : []
  } catch {
    return []
  }
}
function used(c: string) {
  recent.value = [c, ...recent.value.filter((x) => x !== c)].slice(0, 10)
  try {
    localStorage.setItem(RECENT_KEY, JSON.stringify(recent.value))
  } catch {
    /* private window */
  }
}
function pick(m: Mode) {
  if (m !== 'lasso') clearSelection()
  mode.value = m
  if ((TOOLS as string[]).includes(m)) lastPen.value = m as Tool
}
const isPen = computed(() => (TOOLS as string[]).includes(mode.value))
const size = computed({
  get: () => (mode.value === 'eraser' ? eraserSize.value : sizes.value[lastPen.value]),
  set: (v: number) => {
    if (mode.value === 'eraser') eraserSize.value = v
    else sizes.value = { ...sizes.value, [lastPen.value]: v }
  },
})
const PEN_ICONS: Record<Tool, Component> = { pen: Pen, pencil: Pencil, marker: PenLine, highlighter: Highlighter, brush: Brush }

// ---- The view: panned and zoomed ----
const view = ref({ x: 0, y: 0, z: 1 })
const svg = ref<SVGSVGElement | null>(null)
const wrap = ref<HTMLElement | null>(null)
function toWorld(cx: number, cy: number): [number, number] {
  const r = svg.value!.getBoundingClientRect()
  return [(cx - r.left - view.value.x) / view.value.z, (cy - r.top - view.value.y) / view.value.z]
}
function zoomAt(cx: number, cy: number, factor: number) {
  const r = svg.value!.getBoundingClientRect()
  const z = Math.min(8, Math.max(0.1, view.value.z * factor))
  const px = cx - r.left
  const py = cy - r.top
  view.value = { x: px - ((px - view.value.x) * z) / view.value.z, y: py - ((py - view.value.y) * z) / view.value.z, z }
}
function zoomBy(factor: number) {
  const r = svg.value!.getBoundingClientRect()
  zoomAt(r.left + r.width / 2, r.top + r.height / 2, factor)
}
// fit shows everything (or the paper's start at 100%).
function fit() {
  const all = sketch.value.layers.filter((l) => l.visible).flatMap((l) => l.items)
  const b = unionBounds(all)
  const r = svg.value?.getBoundingClientRect()
  if (!r || !b) {
    view.value = { x: 0, y: 0, z: 1 }
    return
  }
  const z = Math.min(1.5, Math.max(0.1, Math.min((r.width - 80) / Math.max(b.w, 1), (r.height - 140) / Math.max(b.h, 1))))
  view.value = { z, x: r.width / 2 - (b.x + b.w / 2) * z, y: r.height / 2 + 30 - (b.y + b.h / 2) * z }
}
function onWheel(e: WheelEvent) {
  if (e.ctrlKey || e.metaKey) zoomAt(e.clientX, e.clientY, Math.exp(-e.deltaY * 0.01))
  else view.value = { ...view.value, x: view.value.x - e.deltaX, y: view.value.y - e.deltaY }
}

// ---- Drawing, rubbing, choosing: the pointers ----
const live = shallowRef<Stroke | null>(null)
let snapTimer: ReturnType<typeof setTimeout> | undefined
let held = false
let still: [number, number] = [0, 0]
let penSeen = false
const pointers = new Map<number, { x: number; y: number; type: string; t: number; moved: boolean }>()
let gesture: { kind: 'pan'; x: number; y: number } | { kind: 'pinch'; d: number; mx: number; my: number; z: number; vx: number; vy: number; t: number; n: number; moved: boolean } | null = null
let rubbing: { layer: string; before: Item[]; items: Item[]; last: [number, number] } | null = null
const lasso = ref<number[][] | null>(null)
const selected = ref<Set<string>>(new Set())
const selLayer = ref('')
const drag = ref<{ start: [number, number]; dx: number; dy: number } | null>(null)
const spaceDown = ref(false)
const cursor = ref<[number, number] | null>(null) // the eraser's ring

function clearSelection() {
  selected.value = new Set()
  lasso.value = null
  drag.value = null
}
const selItems = computed(() => (selected.value.size ? layerOf(selLayer.value).items.filter((i) => selected.value.has(i.id)) : []))
const selBox = computed(() => unionBounds(selItems.value))

function down(e: PointerEvent) {
  if (e.pointerType === 'pen') penSeen = true
  svg.value!.setPointerCapture(e.pointerId)
  pointers.set(e.pointerId, { x: e.clientX, y: e.clientY, type: e.pointerType, t: Date.now(), moved: false })
  const touches = [...pointers.values()].filter((p) => p.type === 'touch')
  // Two or three fingers: move and zoom (whatever was being drawn is dropped).
  if (e.pointerType === 'touch' && touches.length >= 2) {
    live.value = null
    clearTimeout(snapTimer)
    rubbing = null
    const [a, b] = touches
    gesture = { kind: 'pinch', d: Math.hypot(a.x - b.x, a.y - b.y), mx: (a.x + b.x) / 2, my: (a.y + b.y) / 2, z: view.value.z, vx: view.value.x, vy: view.value.y, t: touches[0].t, n: touches.length, moved: false }
    return
  }
  // A finger once a stylus has been seen: it moves the paper (palm rejection).
  if (props.readonly || mode.value === 'hand' || spaceDown.value || e.button === 1 || (e.pointerType === 'touch' && penSeen)) {
    gesture = { kind: 'pan', x: e.clientX, y: e.clientY }
    return
  }
  if (e.pointerType === 'mouse' && e.button !== 0) return
  const [x, y] = toWorld(e.clientX, e.clientY)
  const layer = layerOf()
  if (isPen.value) {
    if (!layer.visible) commit(sketch.value.layers.map((l) => (l.id === layer.id ? { ...l, visible: true } : l)), false)
    held = false
    still = [e.clientX, e.clientY]
    live.value = { id: newId(), kind: 'stroke', tool: lastPen.value, color: color.value, size: sizes.value[lastPen.value], opacity: opacity.value, points: [[r2(x), r2(y), pressureOf(e)]], pen: e.pointerType === 'pen' }
    armSnap()
  } else if (mode.value === 'eraser') {
    rubbing = { layer: layer.id, before: layer.items, items: layer.items, last: [x, y] }
    rubAt(x, y)
  } else if (mode.value === 'lasso') {
    const b = selBox.value
    if (b && x >= b.x && x <= b.x + b.w && y >= b.y && y <= b.y + b.h) drag.value = { start: [x, y], dx: 0, dy: 0 }
    else {
      clearSelection()
      selLayer.value = layer.id
      lasso.value = [[x, y]]
    }
  } else if (mode.value === 'dropper') void dropAt(x, y)
  else if (mode.value === 'fill') void fillAt(x, y)
}

function move(e: PointerEvent) {
  const p = pointers.get(e.pointerId)
  if (p) {
    if (Math.hypot(e.clientX - p.x, e.clientY - p.y) > 10) p.moved = true
  }
  if (mode.value === 'eraser' && e.pointerType !== 'touch') cursor.value = toWorld(e.clientX, e.clientY)
  if (gesture?.kind === 'pinch') {
    const touches = [...pointers.entries()].filter(([, q]) => q.type === 'touch')
    if (p) Object.assign(p, { x: e.clientX, y: e.clientY })
    if (touches.length < 2) return
    const [a, b] = touches.map(([, q]) => q)
    const d = Math.hypot(a.x - b.x, a.y - b.y)
    const mx = (a.x + b.x) / 2
    const my = (a.y + b.y) / 2
    if (Math.abs(d - gesture.d) > 12 || Math.hypot(mx - gesture.mx, my - gesture.my) > 12) gesture.moved = true
    const z = Math.min(8, Math.max(0.1, (gesture.z * d) / gesture.d))
    const r = svg.value!.getBoundingClientRect()
    // The point under the fingers' middle stays under it.
    const wx = (gesture.mx - r.left - gesture.vx) / gesture.z
    const wy = (gesture.my - r.top - gesture.vy) / gesture.z
    view.value = { z, x: mx - r.left - wx * z, y: my - r.top - wy * z }
    return
  }
  if (gesture?.kind === 'pan') {
    view.value = { ...view.value, x: view.value.x + e.clientX - gesture.x, y: view.value.y + e.clientY - gesture.y }
    gesture = { kind: 'pan', x: e.clientX, y: e.clientY }
    return
  }
  if (!p) return
  const evs = typeof e.getCoalescedEvents === 'function' ? e.getCoalescedEvents() : []
  const all = evs.length ? evs : [e]
  if (live.value && !held) {
    const pts = live.value.points.slice()
    for (const ev of all) {
      const [x, y] = toWorld(ev.clientX, ev.clientY)
      const last = pts[pts.length - 1]
      if (Math.hypot(x - last[0], y - last[1]) * view.value.z < 0.8) continue
      pts.push([r2(x), r2(y), pressureOf(ev)])
    }
    live.value = { ...live.value, points: pts }
    if (Math.hypot(e.clientX - still[0], e.clientY - still[1]) > 5) {
      still = [e.clientX, e.clientY]
      armSnap()
    }
  } else if (rubbing) {
    for (const ev of all) {
      const [x, y] = toWorld(ev.clientX, ev.clientY)
      // Every step along the way, so a quick rub misses nothing.
      const [lx, ly] = rubbing.last
      const n = Math.max(1, Math.ceil(Math.hypot(x - lx, y - ly) / (eraserSize.value / 2 / view.value.z)))
      for (let k = 1; k <= n; k++) rubAt(lx + ((x - lx) * k) / n, ly + ((y - ly) * k) / n)
      rubbing.last = [x, y]
    }
  } else if (lasso.value) {
    const [x, y] = toWorld(e.clientX, e.clientY)
    lasso.value = [...lasso.value, [r2(x), r2(y)]]
  } else if (drag.value) {
    const [x, y] = toWorld(e.clientX, e.clientY)
    drag.value = { ...drag.value, dx: x - drag.value.start[0], dy: y - drag.value.start[1] }
  }
}

function up(e: PointerEvent) {
  const p = pointers.get(e.pointerId)
  pointers.delete(e.pointerId)
  if (gesture?.kind === 'pinch') {
    // Fingers tapped (not moved): two undo, three redo.
    if (!pointers.size) {
      if (!gesture.moved && Date.now() - gesture.t < 350) {
        if (gesture.n >= 3) redo()
        else undo()
      }
      gesture = null
    } else if (p?.type === 'touch') gesture.n = Math.max(gesture.n, pointers.size + 1)
    return
  }
  if (gesture?.kind === 'pan') {
    if (!pointers.size) gesture = null
    return
  }
  clearTimeout(snapTimer)
  if (live.value) {
    const s = live.value
    live.value = null
    commit(withItems(active.value, [...layerOf().items, s]))
    used(s.color)
  } else if (rubbing) {
    const r = rubbing
    rubbing = null
    if (r.items !== r.before) {
      // The rub showed as it went; it's one step to undo.
      past.push(sketch.value.layers.map((l) => (l.id === r.layer ? { ...l, items: r.before } : l)))
      future.length = 0
      canUndo.value = true
      canRedo.value = false
      emit('change')
    }
  } else if (lasso.value) {
    const poly = lasso.value
    lasso.value = null
    selected.value = lassoed(layerOf(selLayer.value).items, poly)
  } else if (drag.value) {
    const { dx, dy } = drag.value
    drag.value = null
    if (dx || dy) commit(withItems(selLayer.value, layerOf(selLayer.value).items.map((i) => (selected.value.has(i.id) ? moved(i, dx, dy) : i))))
  }
}
function leave() {
  cursor.value = null
}

const r2 = (n: number) => Math.round(n * 100) / 100
// A stylus's pressure; a mouse or a finger: even (the brush makes its own).
const pressureOf = (e: PointerEvent) => (e.pointerType === 'pen' ? Math.max(0.05, Math.round(e.pressure * 100) / 100) : 0.5)

// Hold still at the end of a stroke: it snaps into what it was meant to be.
function armSnap() {
  clearTimeout(snapTimer)
  snapTimer = setTimeout(() => {
    const s = live.value
    if (!s || held || s.points.length < 3) return
    const r = recognize(s.points)
    if (!r) return
    held = true
    live.value = { ...s, points: r.points, snapped: true }
    snapped.value = r.shape
    setTimeout(() => (snapped.value = ''), 1200)
  }, 550)
}
const snapped = ref('')

function rubAt(x: number, y: number) {
  if (!rubbing) return
  const r = eraserSize.value / 2 / view.value.z
  const next = eraseWhole.value ? wipe(rubbing.items, x, y, r) : rub(rubbing.items, x, y, r)
  if (next === rubbing.items) return
  rubbing.items = next
  sketch.value = { ...sketch.value, layers: withItems(rubbing.layer, next) }
}

// The eyedropper (where the browser has none of its own): the colour of the
// stroke under the click, or of the picture there.
async function dropAt(x: number, y: number) {
  for (const l of [...sketch.value.layers].reverse()) {
    if (!l.visible) continue
    for (const it of [...l.items].reverse()) {
      if (it.kind === 'stroke' && hits(it, x, y, 2 / view.value.z)) {
        setColor(it.color)
        pick(lastPen.value)
        return
      }
      if (it.kind === 'image' && x >= it.x && x <= it.x + it.w && y >= it.y && y <= it.y + it.h) {
        const c = await pixel(it, x, y)
        if (c) setColor(c)
        pick(lastPen.value)
        return
      }
    }
  }
  setColor(sketch.value.paperColor)
  pick(lastPen.value)
}
async function pixel(p: Picture, x: number, y: number): Promise<string> {
  const img = new Image()
  img.src = p.src
  await img.decode().catch(() => undefined)
  const c = document.createElement('canvas')
  c.width = c.height = 1
  const g = c.getContext('2d')
  if (!g || !img.naturalWidth) return ''
  g.drawImage(img, ((x - p.x) / p.w) * img.naturalWidth, ((y - p.y) / p.h) * img.naturalHeight, 1, 1, 0, 0, 1, 1)
  const [r, gr, b] = g.getImageData(0, 0, 1, 1).data
  return `#${[r, gr, b].map((v) => v.toString(16).padStart(2, '0')).join('')}`
}
function setColor(c: string) {
  color.value = c
}

// The bucket: what's around the click that the lines close off (of what's
// on screen), painted in the colour as a picture under the lines.
async function fillAt(x: number, y: number) {
  const r = svg.value?.getBoundingClientRect()
  if (!r) return
  const z = view.value.z
  const box = { x: -view.value.x / z, y: -view.value.y / z, w: r.width / z, h: r.height / z }
  let scale = Math.min(2, window.devicePixelRatio || 1) * z
  if (box.w * box.h * scale * scale > 2_500_000) scale = Math.sqrt(2_500_000 / (box.w * box.h))
  const W = Math.max(1, Math.round(box.w * scale))
  const H = Math.max(1, Math.round(box.h * scale))
  try {
    const img = new Image()
    img.src = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(regionSvg(sketch.value, box, scale))}`
    await img.decode()
    const c = document.createElement('canvas')
    c.width = W
    c.height = H
    const g = c.getContext('2d', { willReadFrequently: true })!
    g.drawImage(img, 0, 0, W, H)
    const px = g.getImageData(0, 0, W, H).data
    const alpha = new Uint8Array(W * H)
    for (let i = 0; i < alpha.length; i++) alpha[i] = px[i * 4 + 3]
    const got = floodFill(alpha, W, H, Math.floor((x - box.x) * scale), Math.floor((y - box.y) * scale), Math.max(1, Math.round(scale)))
    if (!got) return
    const { x0, y0, x1, y1 } = got.box
    const cw = x1 - x0 + 1
    const ch = y1 - y0 + 1
    const out = document.createElement('canvas')
    out.width = cw
    out.height = ch
    const o = out.getContext('2d')!
    const paint = o.createImageData(cw, ch)
    const [cr, cg, cb] = [1, 3, 5].map((i) => parseInt(color.value.slice(i, i + 2), 16))
    const a = Math.round(opacity.value * 255)
    for (let yy = 0; yy < ch; yy++) {
      for (let xx = 0; xx < cw; xx++) {
        if (!got.mask[(y0 + yy) * W + x0 + xx]) continue
        const j = (yy * cw + xx) * 4
        paint.data[j] = cr
        paint.data[j + 1] = cg
        paint.data[j + 2] = cb
        paint.data[j + 3] = a
      }
    }
    o.putImageData(paint, 0, 0)
    const pic: Picture = { id: newId(), kind: 'image', src: out.toDataURL('image/png'), x: box.x + x0 / scale, y: box.y + y0 / scale, w: cw / scale, h: ch / scale, fill: true }
    // Over earlier fills, under the lines.
    const items = layerOf().items
    let at = 0
    while (at < items.length && items[at].kind === 'image' && (items[at] as Picture).fill) at++
    commit(withItems(layerOf().id, [...items.slice(0, at), pic, ...items.slice(at)]))
    used(color.value)
  } catch {
    toast.error(t('sketch.fillFailed'))
  }
}

// ---- What's chosen ----
function recolor() {
  commit(withItems(selLayer.value, layerOf(selLayer.value).items.map((i) => (selected.value.has(i.id) && i.kind === 'stroke' ? { ...i, color: color.value, opacity: opacity.value } : i))))
  used(color.value)
}
function duplicate() {
  const copies = selItems.value.map((i) => ({ ...moved(i, 24, 24), id: newId() }))
  commit(withItems(selLayer.value, [...layerOf(selLayer.value).items, ...copies]))
  selected.value = new Set(copies.map((c) => c.id))
}
function removeSelected() {
  commit(withItems(selLayer.value, layerOf(selLayer.value).items.filter((i) => !selected.value.has(i.id))))
  clearSelection()
}

// ---- Layers ----
const layersOpen = ref(false)
const layersEl = ref<HTMLElement | null>(null)
const renaming = ref('')
function addLayer() {
  const l = newLayer(layerName(sketch.value.layers.length + 1))
  const i = sketch.value.layers.findIndex((x) => x.id === active.value)
  const layers = sketch.value.layers.slice()
  layers.splice(i + 1, 0, l)
  commit(layers)
  active.value = l.id
}
function setLayer(id: string, patch: Partial<Layer>, keep = true) {
  commit(
    sketch.value.layers.map((l) => (l.id === id ? { ...l, ...patch } : l)),
    keep,
  )
}
function shiftLayer(id: string, by: 1 | -1) {
  const layers = sketch.value.layers.slice()
  const i = layers.findIndex((l) => l.id === id)
  const j = i + by
  if (j < 0 || j >= layers.length) return
  ;[layers[i], layers[j]] = [layers[j], layers[i]]
  commit(layers)
}
function removeLayer(id: string) {
  if (sketch.value.layers.length < 2) return
  const layers = sketch.value.layers.filter((l) => l.id !== id)
  commit(layers)
  if (active.value === id) active.value = layers[layers.length - 1].id
  if (selLayer.value === id) clearSelection()
}
// Shown top first.
const layersTopFirst = computed(() => [...sketch.value.layers].reverse())

// ---- Paper ----
const paperOpen = ref(false)
const paperEl = ref<HTMLElement | null>(null)
function setPaper(patch: Partial<Pick<Sketch, 'paper' | 'paperColor'>>) {
  sketch.value = { ...sketch.value, ...patch }
  emit('change')
}
const night = computed(() => sketch.value.paperColor === PAPER_COLORS[2])
const ruling = computed(() => (night.value ? 'rgba(255,255,255,0.16)' : sketch.value.paper === 'lined' ? 'rgba(0,122,255,0.22)' : 'rgba(0,0,0,0.13)'))
const GAP = 32
const patternTransform = computed(() => `translate(${view.value.x} ${view.value.y}) scale(${view.value.z})`)

// ---- A picture or a PDF page, on a layer of its own under the one drawn on ----
const fileEl = ref<HTMLInputElement | null>(null)
const busy = ref(false)
// (Not deeply reactive: pdf.js's objects keep private fields.)
const pdf = shallowRef<{ doc: { numPages: number; getPage: (n: number) => Promise<any> }; page: number; name: string } | null>(null)
const insertEl = ref<HTMLElement | null>(null)
async function chosen(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  ;(e.target as HTMLInputElement).value = ''
  if (!f) return
  busy.value = true
  try {
    if (f.type === 'application/pdf' || /\.pdf$/i.test(f.name)) {
      const pdfjs = await import('pdfjs-dist')
      const worker = (await import('pdfjs-dist/build/pdf.worker.min.mjs?url')).default
      pdfjs.GlobalWorkerOptions.workerSrc = worker
      const doc = await pdfjs.getDocument({ data: new Uint8Array(await f.arrayBuffer()) }).promise
      if (doc.numPages > 1) pdf.value = { doc, page: 1, name: f.name }
      else await placePdf(doc, 1, f.name)
    } else {
      const src = await fitImage(await readAsUrl(f), f.type === 'image/png' || f.type === 'image/gif' || f.type === 'image/svg+xml')
      place(src.url, src.w, src.h, f.name)
    }
  } catch (err: any) {
    alertText.value = t('sketch.cantOpen', { error: err?.message ?? String(err) })
  } finally {
    busy.value = false
  }
}
const alertText = ref('')
async function placePdf(doc: { getPage: (n: number) => Promise<any> }, n: number, name: string) {
  busy.value = true
  try {
    const page = await doc.getPage(n)
    const base = page.getViewport({ scale: 1 })
    const scale = Math.min(3, 1600 / base.width)
    const vp = page.getViewport({ scale })
    const c = document.createElement('canvas')
    c.width = Math.round(vp.width)
    c.height = Math.round(vp.height)
    const g = c.getContext('2d')!
    g.fillStyle = '#fff'
    g.fillRect(0, 0, c.width, c.height)
    await page.render({ canvasContext: g, canvas: c, viewport: vp }).promise
    place(c.toDataURL('image/jpeg', 0.9), c.width / 2, c.height / 2, `${name.replace(/\.pdf$/i, '')} · ${t('sketch.pageN', { n })}`)
  } finally {
    busy.value = false
    pdf.value = null
  }
}
const readAsUrl = (f: File) =>
  new Promise<string>((ok, no) => {
    const r = new FileReader()
    r.onload = () => ok(String(r.result))
    r.onerror = () => no(r.error)
    r.readAsDataURL(f)
  })
// fitImage keeps a picture at most 2000 px a side (so sketches stay light).
async function fitImage(url: string, keepAlpha: boolean): Promise<{ url: string; w: number; h: number }> {
  const img = new Image()
  img.src = url
  await img.decode()
  const k = Math.min(1, 2000 / Math.max(img.naturalWidth, img.naturalHeight))
  const w = Math.round(img.naturalWidth * k)
  const h = Math.round(img.naturalHeight * k)
  if (k === 1 && url.length < 3_000_000) return { url, w, h }
  const c = document.createElement('canvas')
  c.width = w
  c.height = h
  c.getContext('2d')!.drawImage(img, 0, 0, w, h)
  return { url: keepAlpha ? c.toDataURL('image/png') : c.toDataURL('image/jpeg', 0.88), w, h }
}
// place puts a picture in the middle of the view, on a new layer under the
// one being drawn on.
function place(src: string, w: number, h: number, name: string) {
  const r = svg.value!.getBoundingClientRect()
  const k = Math.min(1, (r.width * 0.8) / view.value.z / w, (r.height * 0.8) / view.value.z / h)
  const [cx, cy] = toWorld(r.left + r.width / 2, r.top + r.height / 2)
  const pic: Picture = { id: newId(), kind: 'image', src, x: r2(cx - (w * k) / 2), y: r2(cy - (h * k) / 2), w: r2(w * k), h: r2(h * k) }
  const layer: Layer = { ...newLayer(name.slice(0, 60) || t('sketch.picture')), items: [pic] }
  const layers = sketch.value.layers.slice()
  const i = layers.findIndex((l) => l.id === active.value)
  layers.splice(Math.max(0, i), 0, layer)
  commit(layers)
}

// ---- Keys ----
function onKey(e: KeyboardEvent) {
  const el = e.target as HTMLElement
  if (el.closest('input, textarea, [contenteditable="true"]') || props.readonly) return
  const mod = e.metaKey || e.ctrlKey
  if (mod && e.key.toLowerCase() === 'z') {
    e.preventDefault()
    if (e.shiftKey) redo()
    else undo()
  } else if (mod && e.key.toLowerCase() === 'y') {
    e.preventDefault()
    redo()
  } else if ((e.key === 'Delete' || e.key === 'Backspace') && selected.value.size) {
    e.preventDefault()
    removeSelected()
  } else if (e.key === 'Escape') clearSelection()
  else if (e.key === ' ' && !e.repeat) {
    spaceDown.value = true
    e.preventDefault()
  } else if (!mod && !e.altKey) {
    const keys: Record<string, Mode> = { p: 'pen', n: 'pencil', m: 'marker', h: 'highlighter', b: 'brush', e: 'eraser', l: 'lasso', i: 'dropper', g: 'fill' }
    const m = keys[e.key.toLowerCase()]
    if (m) pick(m)
    else if (e.key === '[') size.value = Math.max(1, size.value - 1)
    else if (e.key === ']') size.value = Math.min(60, size.value + 1)
  }
}
function onKeyUp(e: KeyboardEvent) {
  if (e.key === ' ') spaceDown.value = false
}
onMounted(() => {
  window.addEventListener('keydown', onKey)
  window.addEventListener('keyup', onKeyUp)
  void nextTick(fit)
})
onUnmounted(() => {
  window.removeEventListener('keydown', onKey)
  window.removeEventListener('keyup', onKeyUp)
  clearTimeout(snapTimer)
})

// ---- For the editor ----
// snapshot is what to save: the scene and an SVG picture (under the
// server's 1 MB for pictures: without photos, if they'd make it bigger).
async function snapshot(): Promise<Snapshot | null> {
  let preview = toSvg(sketch.value)
  if (preview.length > 950_000) preview = toSvg(sketch.value, 0)
  if (preview.length > 950_000) preview = ''
  return { scene: JSON.stringify(sketch.value), preview, text: sketch.value.layers.map((l) => l.name).join(' ') }
}
// replace shows a newer version saved elsewhere.
function replace(scene: string) {
  if (!isSketch(scene)) return
  sketch.value = readSketch(scene, layerName(1))
  past.length = 0
  future.length = 0
  canUndo.value = canRedo.value = false
  clearSelection()
  if (!sketch.value.layers.some((l) => l.id === active.value)) active.value = sketch.value.layers[sketch.value.layers.length - 1].id
}
defineExpose({ snapshot, replace })

// Paths are worked out once per stroke (strokes never change in place).
const paths = new WeakMap<Stroke, string>()
function pathOfItem(s: Stroke): string {
  let d = paths.get(s)
  if (d === undefined) {
    d = outline(s)
    paths.set(s, d)
  }
  return d
}
const liveD = computed(() => (live.value ? outline(live.value, false) : ''))
const dragT = computed(() => (drag.value ? `translate(${drag.value.dx} ${drag.value.dy})` : undefined))
const boardCursor = computed(() => {
  if (props.readonly || mode.value === 'hand' || spaceDown.value) return 'grab'
  if (mode.value === 'eraser') return 'none'
  if (mode.value === 'lasso') return 'default'
  return 'crosshair'
})
const colorOpen = ref(false)
const colorEl = ref<HTMLElement | null>(null)
const sizeLabel = computed(() => t('sketch.size', { n: size.value }))
</script>

<template>
  <div ref="wrap" class="sketch" :style="{ background: sketch.paperColor }" data-testid="sketch">
    <svg
      ref="svg"
      class="board"
      :style="{ cursor: boardCursor }"
      data-testid="sketch-board"
      @pointerdown="down"
      @pointermove="move"
      @pointerup="up"
      @pointercancel="up"
      @pointerleave="leave"
      @wheel.prevent="onWheel"
      @contextmenu.prevent
    >
      <defs>
        <filter id="grain" x="-5%" y="-5%" width="110%" height="110%">
          <feTurbulence type="fractalNoise" baseFrequency="1.2" numOctaves="1" result="n" />
          <feDisplacementMap in="SourceGraphic" in2="n" scale="1.6" />
        </filter>
        <pattern id="paper-lined" :width="GAP" :height="GAP" patternUnits="userSpaceOnUse" :patternTransform="patternTransform">
          <line x1="0" :y1="GAP - 0.5" :x2="GAP" :y2="GAP - 0.5" :stroke="ruling" :stroke-width="1 / view.z" />
        </pattern>
        <pattern id="paper-grid" :width="GAP" :height="GAP" patternUnits="userSpaceOnUse" :patternTransform="patternTransform">
          <path :d="`M ${GAP} 0 L 0 0 0 ${GAP}`" fill="none" :stroke="ruling" :stroke-width="1 / view.z" />
        </pattern>
        <pattern id="paper-dots" :width="GAP" :height="GAP" patternUnits="userSpaceOnUse" :patternTransform="patternTransform">
          <circle :cx="GAP / 2" :cy="GAP / 2" :r="1.4 / Math.max(view.z, 0.5)" :fill="ruling" />
        </pattern>
      </defs>
      <rect v-if="sketch.paper !== 'plain'" width="100%" height="100%" :fill="`url(#paper-${sketch.paper})`" />
      <g :transform="`translate(${view.x} ${view.y}) scale(${view.z})`">
        <g v-for="l in sketch.layers" v-show="l.visible" :key="l.id" :opacity="l.opacity">
          <template v-for="it in l.items" :key="it.id">
            <image
              v-if="it.kind === 'image'"
              :href="it.src"
              :x="it.x"
              :y="it.y"
              :width="it.w"
              :height="it.h"
              preserveAspectRatio="none"
              :transform="l.id === selLayer && selected.has(it.id) ? dragT : undefined"
            />
            <path
              v-else
              :d="pathOfItem(it)"
              :fill="looks(it).fill"
              :fill-opacity="looks(it).opacity"
              :filter="looks(it).grain ? 'url(#grain)' : undefined"
              :style="looks(it).blend ? { mixBlendMode: looks(it).blend as any } : undefined"
              :transform="l.id === selLayer && selected.has(it.id) ? dragT : undefined"
            />
          </template>
          <path
            v-if="live && l.id === active"
            :d="liveD"
            :fill="looks(live).fill"
            :fill-opacity="looks(live).opacity"
            :filter="looks(live).grain ? 'url(#grain)' : undefined"
            :style="looks(live).blend ? { mixBlendMode: looks(live).blend as any } : undefined"
            data-testid="sketch-live"
          />
        </g>
        <polyline v-if="lasso" :points="lasso.map((p) => p.join(',')).join(' ')" fill="rgba(0,122,255,0.06)" stroke="#007aff" :stroke-width="1.5 / view.z" :stroke-dasharray="`${5 / view.z} ${4 / view.z}`" />
        <rect
          v-if="selBox"
          :x="selBox.x - 6 / view.z"
          :y="selBox.y - 6 / view.z"
          :width="selBox.w + 12 / view.z"
          :height="selBox.h + 12 / view.z"
          :transform="dragT"
          fill="rgba(0,122,255,0.04)"
          stroke="#007aff"
          :stroke-width="1.5 / view.z"
          :stroke-dasharray="`${5 / view.z} ${4 / view.z}`"
          rx="4"
          data-testid="sketch-selection"
        />
        <circle v-if="cursor && mode === 'eraser'" :cx="cursor[0]" :cy="cursor[1]" :r="eraserSize / 2 / view.z" fill="rgba(255,255,255,0.4)" stroke="#8e8e93" :stroke-width="1 / view.z" pointer-events="none" />
      </g>
    </svg>

    <!-- Tools -->
    <div v-if="!readonly" class="bar tools" role="toolbar" :aria-label="t('sketch.tools')">
      <button
        v-for="tool in TOOLS"
        :key="tool"
        type="button"
        class="tb"
        :class="{ on: mode === tool }"
        :title="t(`sketch.tool.${tool}`)"
        :aria-label="t(`sketch.tool.${tool}`)"
        :aria-pressed="mode === tool"
        :data-testid="`sketch-${tool}`"
        @click="pick(tool)"
      >
        <component :is="PEN_ICONS[tool]" class="h-[18px] w-[18px]" />
        <span v-if="mode === tool || lastPen === tool" class="ink" :style="{ background: color, opacity }" />
      </button>
      <span class="sep" />
      <button type="button" class="tb" :class="{ on: mode === 'eraser' }" :title="t('sketch.tool.eraser')" :aria-label="t('sketch.tool.eraser')" :aria-pressed="mode === 'eraser'" data-testid="sketch-eraser" @click="pick('eraser')"><Eraser class="h-[18px] w-[18px]" /></button>
      <button type="button" class="tb" :class="{ on: mode === 'fill' }" :title="t('sketch.tool.fill')" :aria-label="t('sketch.tool.fill')" :aria-pressed="mode === 'fill'" data-testid="sketch-fill" @click="pick('fill')"><PaintBucket class="h-[18px] w-[18px]" /></button>
      <button type="button" class="tb" :class="{ on: mode === 'lasso' }" :title="t('sketch.tool.lasso')" :aria-label="t('sketch.tool.lasso')" :aria-pressed="mode === 'lasso'" data-testid="sketch-lasso" @click="pick('lasso')"><Lasso class="h-[18px] w-[18px]" /></button>
      <button type="button" class="tb" :class="{ on: mode === 'hand' }" :title="t('sketch.tool.hand')" :aria-label="t('sketch.tool.hand')" :aria-pressed="mode === 'hand'" @click="pick('hand')"><Hand class="h-[18px] w-[18px]" /></button>
      <span class="sep" />
      <button ref="colorEl" type="button" class="swatch-btn" :title="t('sketch.color')" :aria-label="t('sketch.color')" data-testid="sketch-color" @click="colorOpen = !colorOpen">
        <span :style="{ background: color, opacity }" />
      </button>
      <label class="size" :title="sizeLabel">
        <span class="sr-only">{{ sizeLabel }}</span>
        <input v-model.number="size" type="range" min="1" max="60" :aria-label="sizeLabel" data-testid="sketch-size" />
      </label>
      <button v-if="mode === 'eraser'" type="button" class="chip" :aria-pressed="eraseWhole" data-testid="sketch-erase-whole" @click="eraseWhole = !eraseWhole">{{ eraseWhole ? t('sketch.eraseWhole') : t('sketch.erasePart') }}</button>
      <span class="sep" />
      <button type="button" class="tb" :disabled="!canUndo" :title="t('sketch.undo')" :aria-label="t('sketch.undo')" data-testid="sketch-undo" @click="undo"><Undo2 class="h-[18px] w-[18px]" /></button>
      <button type="button" class="tb" :disabled="!canRedo" :title="t('sketch.redo')" :aria-label="t('sketch.redo')" data-testid="sketch-redo" @click="redo"><Redo2 class="h-[18px] w-[18px]" /></button>
    </div>

    <!-- Paper, layers, a picture; zoom -->
    <div class="bar side">
      <template v-if="!readonly">
        <button ref="paperEl" type="button" class="tb" :title="t('sketch.paper')" :aria-label="t('sketch.paper')" data-testid="sketch-paper" @click="paperOpen = !paperOpen"><NotebookText class="h-[18px] w-[18px]" /></button>
        <button ref="layersEl" type="button" class="tb" :title="t('sketch.layers')" :aria-label="t('sketch.layers')" data-testid="sketch-layers" @click="layersOpen = !layersOpen"><Layers class="h-[18px] w-[18px]" /></button>
        <input ref="fileEl" type="file" class="hidden" accept="image/*,application/pdf,.pdf" data-testid="sketch-file" @change="chosen" />
        <button ref="insertEl" type="button" class="tb" :disabled="busy" :title="t('sketch.insert')" :aria-label="t('sketch.insert')" @click="fileEl?.click()"><FileImage class="h-[18px] w-[18px]" /></button>
        <span class="sep" />
      </template>
      <button type="button" class="tb" :title="t('sketch.zoomOut')" :aria-label="t('sketch.zoomOut')" @click="zoomBy(1 / 1.25)"><ZoomOut class="h-[18px] w-[18px]" /></button>
      <button type="button" class="zoom" :title="t('sketch.fit')" data-testid="sketch-fit" @click="fit">{{ Math.round(view.z * 100) }}%</button>
      <button type="button" class="tb" :title="t('sketch.zoomIn')" :aria-label="t('sketch.zoomIn')" @click="zoomBy(1.25)"><ZoomIn class="h-[18px] w-[18px]" /></button>
    </div>

    <!-- What's chosen -->
    <div v-if="selected.size && !drag" class="bar picked" data-testid="sketch-picked">
      <span class="px-2 text-[12.5px] text-text-2">{{ t('sketch.chosen', { n: selected.size }, selected.size) }}</span>
      <button type="button" class="chip" :title="t('sketch.recolor')" data-testid="sketch-recolor" @click="recolor"><PaintBucket class="h-3.5 w-3.5" /><span class="dot" :style="{ background: color }" />{{ t('sketch.recolor') }}</button>
      <button type="button" class="chip" data-testid="sketch-duplicate" @click="duplicate"><Copy class="h-3.5 w-3.5" />{{ t('sketch.duplicate') }}</button>
      <button type="button" class="chip danger" data-testid="sketch-delete" @click="removeSelected"><Trash2 class="h-3.5 w-3.5" />{{ t('sketch.delete') }}</button>
    </div>

    <Transition name="fade">
      <div v-if="snapped" class="snapped" aria-live="polite">{{ t(`sketch.shape.${snapped}`) }}</div>
    </Transition>
    <div v-if="mode === 'dropper'" class="snapped" aria-live="polite"><Pipette class="h-3.5 w-3.5" />{{ t('sketch.dropperHint') }}</div>

    <Popover v-if="colorOpen" :anchor="colorEl" :width="280" placement="bottom" @close="colorOpen = false">
      <ColorPicker
        :color="color"
        :opacity="opacity"
        :recent="recent"
        @update:color="(c) => (color = c)"
        @update:opacity="(o) => (opacity = o)"
        @dropper="(colorOpen = false), pick('dropper')"
      />
    </Popover>

    <Popover v-if="paperOpen" :anchor="paperEl" :width="250" placement="bottom-end" @close="paperOpen = false">
      <div class="space-y-3 p-3" data-testid="sketch-paper-menu">
        <div class="grid grid-cols-4 gap-2">
          <button v-for="p in PAPERS" :key="p" type="button" class="paper-pick" :class="{ on: sketch.paper === p }" :aria-pressed="sketch.paper === p" :data-testid="`paper-${p}`" @click="setPaper({ paper: p as Paper })">
            <span class="paper-sample" :class="p" :style="{ background: sketch.paperColor }" />
            <span>{{ t(`sketch.papers.${p}`) }}</span>
          </button>
        </div>
        <div class="flex items-center gap-2">
          <span class="text-[12px] text-text-3">{{ t('sketch.paperColor') }}</span>
          <button
            v-for="c in PAPER_COLORS"
            :key="c"
            type="button"
            class="cp-dot"
            :class="{ on: sketch.paperColor === c }"
            :style="{ background: c }"
            :aria-label="c"
            @click="setPaper({ paperColor: c })"
          />
        </div>
      </div>
    </Popover>

    <Popover v-if="layersOpen" :anchor="layersEl" :width="290" placement="bottom-end" @close="(layersOpen = false), (renaming = '')">
      <div class="p-2" data-testid="sketch-layers-menu">
        <div v-for="l in layersTopFirst" :key="l.id" class="layer" :class="{ on: l.id === active }" data-testid="sketch-layer" @click="active = l.id">
          <button type="button" class="icon-btn h-7 w-7 shrink-0" :aria-label="l.visible ? t('sketch.hide') : t('sketch.show')" @click.stop="setLayer(l.id, { visible: !l.visible })">
            <Eye v-if="l.visible" class="h-4 w-4" /><EyeOff v-else class="h-4 w-4 text-text-3" />
          </button>
          <input
            v-if="renaming === l.id"
            class="input min-w-0 flex-1 !py-0.5 text-[13px]"
            :value="l.name"
            maxlength="60"
            :aria-label="t('sketch.layerName')"
            @click.stop
            @keydown.enter="($event.target as HTMLInputElement).blur()"
            @blur="(setLayer(l.id, { name: ($event.target as HTMLInputElement).value.trim() || l.name }), (renaming = ''))"
          />
          <span v-else class="min-w-0 flex-1 truncate text-[13px]" @dblclick.stop="renaming = l.id">{{ l.name }}</span>
          <span class="text-[11px] tabular-nums text-text-3">{{ l.items.length }}</span>
          <button type="button" class="icon-btn h-6 w-6 shrink-0" :aria-label="t('sketch.up')" @click.stop="shiftLayer(l.id, 1)"><ChevronUp class="h-3.5 w-3.5" /></button>
          <button type="button" class="icon-btn h-6 w-6 shrink-0" :aria-label="t('sketch.down')" @click.stop="shiftLayer(l.id, -1)"><ChevronDown class="h-3.5 w-3.5" /></button>
          <button v-if="sketch.layers.length > 1" type="button" class="icon-btn h-6 w-6 shrink-0" :aria-label="t('sketch.deleteLayer')" @click.stop="removeLayer(l.id)"><Trash2 class="h-3.5 w-3.5" /></button>
        </div>
        <label v-if="layerOf()" class="mt-2 flex items-center gap-2 px-2 text-[12px] text-text-3">
          {{ t('sketch.layerOpacity') }}
          <input class="min-w-0 flex-1" type="range" min="5" max="100" :value="Math.round(layerOf().opacity * 100)" :aria-label="t('sketch.layerOpacity')" @change="setLayer(active, { opacity: Number(($event.target as HTMLInputElement).value) / 100 })" />
        </label>
        <button type="button" class="menu-item mt-1 w-full" data-testid="sketch-add-layer" @click="addLayer"><Plus class="h-4 w-4 text-text-2" />{{ t('sketch.addLayer') }}</button>
        <p class="px-2 pt-1 text-[11.5px] leading-snug text-text-3">{{ t('sketch.layersHint') }}</p>
      </div>
    </Popover>

    <Popover v-if="pdf" :anchor="insertEl" :width="240" placement="bottom-end" @close="pdf = null">
      <form class="space-y-2 p-3" @submit.prevent="pdf && placePdf(pdf.doc, pdf.page, pdf.name)">
        <label class="flex items-center gap-2 text-[13px]">
          {{ t('sketch.whichPage', { n: pdf.doc.numPages }) }}
          <input class="input !w-20 !py-1" type="number" min="1" :max="pdf.doc.numPages" :value="pdf.page" data-testid="sketch-pdf-page" @input="pdf = { ...pdf!, page: Math.min(pdf!.doc.numPages, Math.max(1, Number(($event.target as HTMLInputElement).value) || 1)) }" />
        </label>
        <button type="submit" class="btn-primary btn-sm w-full justify-center">{{ t('sketch.putIn') }}</button>
      </form>
    </Popover>
    <div v-if="alertText" class="snapped error" role="alert" @click="alertText = ''">{{ alertText }}</div>
  </div>
</template>

<style scoped>
.sketch {
  position: relative;
  width: 100%;
  height: 100%;
  overflow: hidden;
  user-select: none;
  -webkit-user-select: none;
}
.board {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  touch-action: none;
}
.bar {
  position: absolute;
  z-index: 3;
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 4px;
  border-radius: 14px;
  background: color-mix(in srgb, var(--surface) 92%, transparent);
  box-shadow: 0 0 0 0.5px var(--border), 0 4px 16px rgba(0, 0, 0, 0.12);
  backdrop-filter: blur(12px);
}
.tools {
  top: 12px;
  left: 50%;
  max-width: calc(100% - 24px);
  overflow-x: auto;
  transform: translateX(-50%);
  scrollbar-width: none;
}
.side {
  right: 12px;
  bottom: 12px;
}
.picked {
  bottom: 56px;
  left: 50%;
  transform: translateX(-50%);
}
.tb {
  position: relative;
  display: inline-flex;
  width: 34px;
  height: 34px;
  flex: none;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  color: var(--text-2);
}
.tb:hover:not(:disabled) {
  background: var(--fill-2);
  color: var(--text);
}
.tb.on {
  background: var(--accent-bg, var(--fill));
  color: var(--accent-text, var(--accent));
}
.tb:disabled {
  opacity: 0.35;
}
.tb .ink {
  position: absolute;
  bottom: 3px;
  left: 50%;
  width: 14px;
  height: 3px;
  border-radius: 2px;
  transform: translateX(-50%);
}
.sep {
  width: 0.5px;
  height: 22px;
  margin: 0 4px;
  flex: none;
  background: var(--border);
}
.swatch-btn {
  display: inline-flex;
  width: 34px;
  height: 34px;
  flex: none;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
}
.swatch-btn span {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  box-shadow: 0 0 0 2px var(--surface), 0 0 0 2.5px var(--border);
}
.size input {
  width: 84px;
  accent-color: var(--accent);
}
.size {
  display: inline-flex;
  flex: none;
  align-items: center;
  padding: 0 6px;
}
.chip {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: 5px;
  height: 30px;
  padding: 0 10px;
  border-radius: 9px;
  font-size: 12.5px;
  color: var(--text-2);
  background: var(--fill-2);
}
.chip:hover {
  color: var(--text);
}
.chip.danger {
  color: var(--crit-text);
}
.chip .dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
}
.zoom {
  min-width: 48px;
  height: 34px;
  border-radius: 10px;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  color: var(--text-2);
}
.zoom:hover {
  background: var(--fill-2);
}
.snapped {
  position: absolute;
  top: 64px;
  left: 50%;
  z-index: 3;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 5px 12px;
  border-radius: 999px;
  transform: translateX(-50%);
  background: color-mix(in srgb, var(--surface) 92%, transparent);
  box-shadow: 0 0 0 0.5px var(--border);
  font-size: 12.5px;
  color: var(--text-2);
  pointer-events: none;
}
.snapped.error {
  color: var(--crit-text);
  pointer-events: auto;
  cursor: pointer;
}
.paper-pick {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 4px;
  border-radius: 10px;
  font-size: 11.5px;
  color: var(--text-2);
}
.paper-pick.on {
  background: var(--accent-bg, var(--fill));
  color: var(--accent-text, var(--accent));
}
.paper-sample {
  width: 40px;
  height: 40px;
  border-radius: 6px;
  box-shadow: inset 0 0 0 0.5px var(--border);
}
.paper-sample.lined {
  background-image: repeating-linear-gradient(to bottom, transparent 0 7px, rgba(0, 122, 255, 0.35) 7px 8px) !important;
}
.paper-sample.grid {
  background-image:
    repeating-linear-gradient(to bottom, transparent 0 7px, rgba(0, 0, 0, 0.18) 7px 8px),
    repeating-linear-gradient(to right, transparent 0 7px, rgba(0, 0, 0, 0.18) 7px 8px) !important;
}
.paper-sample.dots {
  background-image: radial-gradient(rgba(0, 0, 0, 0.35) 1px, transparent 1.5px) !important;
  background-size: 8px 8px !important;
}
.cp-dot {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  box-shadow: inset 0 0 0 0.5px rgba(0, 0, 0, 0.3);
}
.cp-dot.on {
  box-shadow: 0 0 0 2px var(--surface), 0 0 0 3.5px var(--accent);
}
.layer {
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 2px 4px;
  border-radius: 8px;
  cursor: pointer;
}
.layer.on {
  background: var(--accent-bg, var(--fill));
}
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
