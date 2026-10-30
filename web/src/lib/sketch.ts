// Sketches: Quick Draw's own drawing — strokes of pens, pencils, markers,
// highlighters and brushes (outlined with perfect-freehand, so they stay
// sharp at any size and thicken with a stylus's pressure), pictures, all on
// layers, over a paper. This is the model and its geometry: drawing a
// stroke, snapping a held one into a clean line or shape, rubbing parts of
// strokes out, choosing with a lasso, and the SVG picture of it all.
import { getStroke } from 'perfect-freehand'

export type Tool = 'pen' | 'pencil' | 'marker' | 'highlighter' | 'brush'
export const TOOLS: Tool[] = ['pen', 'pencil', 'marker', 'highlighter', 'brush']
export type Paper = 'plain' | 'lined' | 'grid' | 'dots'
export const PAPERS: Paper[] = ['plain', 'lined', 'grid', 'dots']
export const PAPER_COLORS = ['#ffffff', '#fbf6e9', '#1c1c1e'] // white, cream, night

export type Point = [x: number, y: number, pressure: number]
export interface Stroke {
  id: string
  kind: 'stroke'
  tool: Tool
  color: string // #rrggbb
  size: number
  opacity: number // 0–1, on top of the tool's own
  points: Point[]
  pen?: boolean // drawn with a stylus: its pressure counts
  snapped?: boolean // a clean line or shape (held to snap)
}
export interface Picture {
  id: string
  kind: 'image'
  src: string // a data: URL
  x: number
  y: number
  w: number
  h: number
  fill?: boolean // a bucket fill (not a wall for the next one)
}
export type Item = Stroke | Picture
export interface Layer {
  id: string
  name: string
  visible: boolean
  opacity: number
  items: Item[]
}
export interface Sketch {
  type: 'orbit-sketch'
  version: 1
  paper: Paper
  paperColor: string
  layers: Layer[]
}

export const newId = () => Math.random().toString(36).slice(2, 10)

export function emptySketch(layerName: string): Sketch {
  return { type: 'orbit-sketch', version: 1, paper: 'plain', paperColor: PAPER_COLORS[0], layers: [newLayer(layerName)] }
}
export const newLayer = (name: string): Layer => ({ id: newId(), name, visible: true, opacity: 1, items: [] })

// isSketch: a drawing's scene that is a sketch (not Excalidraw's).
export const isSketch = (scene: string | undefined) => !!scene && /^\s*\{\s*"type"\s*:\s*"orbit-sketch"/.test(scene)

// readSketch reads a scene, made safe (a bad one: an empty sketch).
export function readSketch(scene: string, layerName: string): Sketch {
  try {
    const s = JSON.parse(scene) as Partial<Sketch>
    if (s.type !== 'orbit-sketch' || !Array.isArray(s.layers)) throw new Error('not a sketch')
    const layers = s.layers
      .filter((l) => l && Array.isArray(l.items))
      .map((l) => ({
        id: String(l.id || newId()),
        name: String(l.name ?? layerName),
        visible: l.visible !== false,
        opacity: clamp(Number(l.opacity ?? 1), 0, 1),
        items: l.items.filter((x) => (x?.kind === 'stroke' && Array.isArray(x.points)) || (x?.kind === 'image' && /^data:image\//.test(x.src))),
      }))
    return {
      type: 'orbit-sketch',
      version: 1,
      paper: PAPERS.includes(s.paper as Paper) ? (s.paper as Paper) : 'plain',
      paperColor: /^#[0-9a-f]{6}$/i.test(s.paperColor ?? '') ? s.paperColor! : PAPER_COLORS[0],
      layers: layers.length ? layers : [newLayer(layerName)],
    }
  } catch {
    return emptySketch(layerName)
  }
}

const clamp = (v: number, lo: number, hi: number) => (Number.isFinite(v) ? Math.min(hi, Math.max(lo, v)) : lo)

// ---- Brushes ----

interface Brush {
  thinning: number
  smoothing: number
  streamline: number
  alpha: number // how see-through the tool is
  taper?: boolean // thin at both ends
  scale: number // its width, for a given size
  flat?: boolean // no round ends
  blend?: 'multiply'
  grain?: boolean // a pencil's rough edge
}
export const BRUSHES: Record<Tool, Brush> = {
  pen: { thinning: 0.5, smoothing: 0.5, streamline: 0.5, alpha: 1, scale: 1 },
  pencil: { thinning: 0.6, smoothing: 0.35, streamline: 0.3, alpha: 0.8, scale: 0.7, grain: true },
  marker: { thinning: 0, smoothing: 0.6, streamline: 0.6, alpha: 1, scale: 1.6 },
  highlighter: { thinning: 0, smoothing: 0.7, streamline: 0.7, alpha: 0.35, scale: 4, flat: true, blend: 'multiply' },
  brush: { thinning: 0.8, smoothing: 0.6, streamline: 0.45, alpha: 1, scale: 2.2, taper: true },
}
export const DEFAULT_SIZE: Record<Tool, number> = { pen: 4, pencil: 3, marker: 6, highlighter: 6, brush: 8 }

// outline is a stroke's shape (perfect-freehand), as an SVG path.
export function outline(s: Stroke, done = true): string {
  const b = BRUSHES[s.tool] ?? BRUSHES.pen
  const size = s.size * b.scale
  const pts = getStroke(s.points, {
    size,
    thinning: s.snapped ? Math.min(b.thinning, 0.2) : b.thinning,
    smoothing: s.snapped ? 0.2 : b.smoothing,
    streamline: s.snapped ? 0 : b.streamline,
    simulatePressure: !s.pen && !s.snapped,
    start: { cap: !b.flat, taper: b.taper && !s.snapped ? size * 3 : 0 },
    end: { cap: !b.flat, taper: b.taper && !s.snapped ? size * 3 : 0 },
    last: done,
  })
  return pathOf(pts)
}

const avg = (a: number, b: number) => (a + b) / 2
const f = (n: number) => n.toFixed(2)
// pathOf: an outline's points as a closed, smoothed path.
export function pathOf(points: number[][]): string {
  const n = points.length
  if (n < 4) return ''
  let a = points[0]
  let b = points[1]
  const c = points[2]
  let d = `M${f(a[0])},${f(a[1])} Q${f(b[0])},${f(b[1])} ${f(avg(b[0], c[0]))},${f(avg(b[1], c[1]))} T`
  for (let i = 2; i < n - 1; i++) {
    a = points[i]
    b = points[i + 1]
    d += `${f(avg(a[0], b[0]))},${f(avg(a[1], b[1]))} `
  }
  return d + 'Z'
}

// The look of a stroke: its fill, how see-through, how it mixes.
export function looks(s: Stroke): { fill: string; opacity: number; blend?: string; grain: boolean } {
  const b = BRUSHES[s.tool] ?? BRUSHES.pen
  return { fill: s.color, opacity: clamp(s.opacity, 0.05, 1) * b.alpha, blend: b.blend, grain: !!b.grain }
}

// ---- Geometry ----

type XY = [number, number]
const dist = (a: number[], b: number[]) => Math.hypot(a[0] - b[0], a[1] - b[1])

function segDist(p: number[], a: number[], b: number[]): number {
  const dx = b[0] - a[0]
  const dy = b[1] - a[1]
  const l2 = dx * dx + dy * dy
  if (!l2) return dist(p, a)
  const t = clamp(((p[0] - a[0]) * dx + (p[1] - a[1]) * dy) / l2, 0, 1)
  return Math.hypot(p[0] - (a[0] + t * dx), p[1] - (a[1] + t * dy))
}

export function boundsOf(points: number[][]): { x: number; y: number; w: number; h: number } {
  let x0 = Infinity
  let y0 = Infinity
  let x1 = -Infinity
  let y1 = -Infinity
  for (const p of points) {
    x0 = Math.min(x0, p[0])
    y0 = Math.min(y0, p[1])
    x1 = Math.max(x1, p[0])
    y1 = Math.max(y1, p[1])
  }
  return x0 === Infinity ? { x: 0, y: 0, w: 0, h: 0 } : { x: x0, y: y0, w: x1 - x0, h: y1 - y0 }
}

// simplify keeps a line's corners (Ramer–Douglas–Peucker).
export function simplify(points: number[][], eps: number): number[][] {
  if (points.length < 3) return points.slice()
  let worst = 0
  let at = 0
  const a = points[0]
  const b = points[points.length - 1]
  for (let i = 1; i < points.length - 1; i++) {
    const d = segDist(points[i], a, b)
    if (d > worst) {
      worst = d
      at = i
    }
  }
  if (worst <= eps) return [a, b]
  return [...simplify(points.slice(0, at + 1), eps).slice(0, -1), ...simplify(points.slice(at), eps)]
}

// along walks a polyline, a point every `step`.
function along(corners: XY[], step: number): Point[] {
  const out: Point[] = []
  for (let i = 0; i < corners.length - 1; i++) {
    const a = corners[i]
    const b = corners[i + 1]
    const n = Math.max(1, Math.ceil(dist(a, b) / step))
    for (let k = 0; k < n; k++) out.push([round(a[0] + ((b[0] - a[0]) * k) / n), round(a[1] + ((b[1] - a[1]) * k) / n), 0.5])
  }
  const last = corners[corners.length - 1]
  out.push([round(last[0]), round(last[1]), 0.5])
  return out
}
const round = (n: number) => Math.round(n * 100) / 100

export type Shape = 'line' | 'circle' | 'ellipse' | 'triangle' | 'rectangle' | 'polygon'

// recognize snaps a held stroke into the clean shape it was meant to be:
// a straight line, a circle or ellipse, a triangle, a rectangle (or a
// four-sided shape). null: it's none of them (it stays as drawn).
export function recognize(points: number[][]): { shape: Shape; points: Point[] } | null {
  if (points.length < 3) return null
  const box = boundsOf(points)
  const size = Math.hypot(box.w, box.h)
  if (size < 16) return null
  const first = points[0]
  const last = points[points.length - 1]
  const closed = dist(first, last) < Math.max(18, size * 0.22)

  if (!closed) {
    // A line: every point near the one from start to end.
    const len = dist(first, last)
    const off = Math.max(...points.map((p) => segDist(p, first, last)))
    if (len > size * 0.8 && off < Math.max(6, len * 0.08)) return { shape: 'line', points: along([xy(first), xy(last)], 4) }
    return null
  }

  // Closed: its corners, or an ellipse.
  const ring = [...points, points[0]]
  let corners = simplify(ring, size * 0.08)
  if (corners.length > 2 && dist(corners[0], corners[corners.length - 1]) < size * 0.15) corners = corners.slice(0, -1)
  const cx = box.x + box.w / 2
  const cy = box.y + box.h / 2
  const rx = box.w / 2
  const ry = box.h / 2
  const misfit = rx && ry ? points.reduce((s, p) => s + Math.abs(Math.hypot((p[0] - cx) / rx, (p[1] - cy) / ry) - 1), 0) / points.length : 1
  const ellipse = (): { shape: Shape; points: Point[] } => {
    const circle = Math.abs(rx - ry) < Math.max(rx, ry) * 0.18
    const r = (rx + ry) / 2
    const [ax, ay] = circle ? [r, r] : [rx, ry]
    const n = Math.max(24, Math.ceil((Math.PI * (ax + ay)) / 4))
    const ring: XY[] = Array.from({ length: n + 1 }, (_, i) => [cx + ax * Math.cos((2 * Math.PI * i) / n), cy + ay * Math.sin((2 * Math.PI * i) / n)])
    return { shape: circle ? 'circle' : 'ellipse', points: along(ring, Infinity) }
  }
  // A close fit to an ellipse is one (its corners are only wobbles).
  if (misfit < 0.1) return ellipse()
  if (corners.length === 3) return { shape: 'triangle', points: along([...corners.map(xy), xy(corners[0])], 4) }
  if (corners.length === 4) {
    // Sides near level and upright: a rectangle, square to the page.
    const square = corners.every((p, i) => {
      const q = corners[(i + 1) % 4]
      const a = Math.abs(Math.atan2(q[1] - p[1], q[0] - p[0])) % (Math.PI / 2)
      return Math.min(a, Math.PI / 2 - a) < (16 * Math.PI) / 180
    })
    if (square) {
      const { x, y, w, h } = boundsOf(corners)
      return { shape: 'rectangle', points: along([[x, y], [x + w, y], [x + w, y + h], [x, y + h], [x, y]], 4) }
    }
    return { shape: 'polygon', points: along([...corners.map(xy), xy(corners[0])], 4) }
  }
  if (misfit < 0.22) return ellipse()
  return null
}
const xy = (p: number[]): XY => [p[0], p[1]]

// ---- The eraser: rubbing parts of strokes out ----

// densify puts points along a stroke no further apart than `step` (so a
// rub takes out only what it touches).
function densify(points: Point[], step: number): Point[] {
  const out: Point[] = []
  for (let i = 0; i < points.length; i++) {
    const p = points[i]
    if (i) {
      const q = points[i - 1]
      const n = Math.floor(dist(p, q) / step)
      for (let k = 1; k <= n; k++) {
        const t = k / (n + 1)
        out.push([round(q[0] + (p[0] - q[0]) * t), round(q[1] + (p[1] - q[1]) * t), q[2] + (p[2] - q[2]) * t])
      }
    }
    out.push(p)
  }
  return out
}

// rub takes out of a layer's strokes the parts within r of (x, y): a stroke
// cut through becomes two. Pictures stay. The same array back: nothing hit.
export function rub(items: Item[], x: number, y: number, r: number): Item[] {
  let changed = false
  const out: Item[] = []
  for (const it of items) {
    if (it.kind !== 'stroke') {
      out.push(it)
      continue
    }
    const reach = r + (it.size * (BRUSHES[it.tool]?.scale ?? 1)) / 2
    const box = boundsOf(it.points)
    if (x < box.x - reach || x > box.x + box.w + reach || y < box.y - reach || y > box.y + box.h + reach) {
      out.push(it)
      continue
    }
    const pts = densify(it.points, Math.max(1, r / 2))
    const keep = pts.map((p) => Math.hypot(p[0] - x, p[1] - y) > reach)
    if (keep.every(Boolean)) {
      out.push(it)
      continue
    }
    changed = true
    let run: Point[] = []
    const flush = () => {
      if (run.length > 1) out.push({ ...it, id: newId(), points: run, snapped: false })
      run = []
    }
    pts.forEach((p, i) => (keep[i] ? run.push(p) : flush()))
    flush()
  }
  return changed ? out : items
}

// wipe takes out whole strokes that (x, y) touches.
export function wipe(items: Item[], x: number, y: number, r: number): Item[] {
  const out = items.filter((it) => it.kind !== 'stroke' || !hits(it, x, y, r))
  return out.length === items.length ? items : out
}

// hits: (x, y) is on the stroke (within r).
export function hits(s: Stroke, x: number, y: number, r: number): boolean {
  const reach = r + (s.size * (BRUSHES[s.tool]?.scale ?? 1)) / 2
  if (s.points.length === 1) return Math.hypot(s.points[0][0] - x, s.points[0][1] - y) <= reach
  for (let i = 1; i < s.points.length; i++) if (segDist([x, y], s.points[i - 1], s.points[i]) <= reach) return true
  return false
}

// ---- The lasso ----

export function inside(p: number[], poly: number[][]): boolean {
  let yes = false
  for (let i = 0, j = poly.length - 1; i < poly.length; j = i++) {
    const [xi, yi] = poly[i]
    const [xj, yj] = poly[j]
    if (yi > p[1] !== yj > p[1] && p[0] < ((xj - xi) * (p[1] - yi)) / (yj - yi) + xi) yes = !yes
  }
  return yes
}

// lassoed: the items a loop goes round (most of a stroke; a picture's middle).
export function lassoed(items: Item[], poly: number[][]): Set<string> {
  const out = new Set<string>()
  if (poly.length < 3) return out
  for (const it of items) {
    if (it.kind === 'stroke') {
      const n = it.points.filter((p) => inside(p, poly)).length
      if (n && n >= it.points.length * 0.6) out.add(it.id)
    } else if (inside([it.x + it.w / 2, it.y + it.h / 2], poly)) out.add(it.id)
  }
  return out
}

// moved: an item shifted by (dx, dy).
export function moved(it: Item, dx: number, dy: number): Item {
  if (it.kind === 'image') return { ...it, x: round(it.x + dx), y: round(it.y + dy) }
  return { ...it, points: it.points.map((p) => [round(p[0] + dx), round(p[1] + dy), p[2]] as Point) }
}

// itemBounds: where an item is (a stroke with its width).
export function itemBounds(it: Item): { x: number; y: number; w: number; h: number } {
  if (it.kind === 'image') return { x: it.x, y: it.y, w: it.w, h: it.h }
  const b = boundsOf(it.points)
  const pad = (it.size * (BRUSHES[it.tool]?.scale ?? 1)) / 2
  return { x: b.x - pad, y: b.y - pad, w: b.w + 2 * pad, h: b.h + 2 * pad }
}

export function unionBounds(items: Item[]): { x: number; y: number; w: number; h: number } | null {
  if (!items.length) return null
  const bs = items.map(itemBounds)
  const x = Math.min(...bs.map((b) => b.x))
  const y = Math.min(...bs.map((b) => b.y))
  return { x, y, w: Math.max(...bs.map((b) => b.x + b.w)) - x, h: Math.max(...bs.map((b) => b.y + b.h)) - y }
}

// ---- The picture ----

const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;')
export const GRAIN_FILTER =
  '<filter id="grain" x="-5%" y="-5%" width="110%" height="110%"><feTurbulence type="fractalNoise" baseFrequency="1.2" numOctaves="1" result="n"/><feDisplacementMap in="SourceGraphic" in2="n" scale="1.6"/></filter>'

// toSvg is the sketch as an SVG picture (its visible layers, on its paper),
// for lists and previews. Pictures are left out past `maxImages` bytes.
export function toSvg(s: Sketch, maxImages = 600_000): string {
  const items = s.layers.filter((l) => l.visible).flatMap((l) => l.items)
  const box = unionBounds(items) ?? { x: 0, y: 0, w: 200, h: 120 }
  const pad = 16
  const vb = { x: box.x - pad, y: box.y - pad, w: Math.max(box.w + 2 * pad, 40), h: Math.max(box.h + 2 * pad, 40) }
  let budget = maxImages
  const layers = s.layers
    .filter((l) => l.visible)
    .map((l) => {
      const body = l.items
        .map((it) => {
          if (it.kind === 'image') {
            if (it.src.length > budget) return ''
            budget -= it.src.length
            return `<image href="${esc(it.src)}" x="${it.x}" y="${it.y}" width="${it.w}" height="${it.h}" preserveAspectRatio="none"/>`
          }
          const d = outline(it)
          if (!d) return ''
          const lk = looks(it)
          const style = lk.blend ? ` style="mix-blend-mode:${lk.blend}"` : ''
          const filter = lk.grain ? ' filter="url(#grain)"' : ''
          return `<path d="${d}" fill="${esc(lk.fill)}" fill-opacity="${lk.opacity.toFixed(2)}"${filter}${style}/>`
        })
        .join('')
      return `<g opacity="${l.opacity}">${body}</g>`
    })
    .join('')
  return (
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${f(vb.x)} ${f(vb.y)} ${f(vb.w)} ${f(vb.h)}" width="${Math.round(vb.w)}" height="${Math.round(vb.h)}">` +
    `<defs>${GRAIN_FILTER}</defs><rect x="${f(vb.x)}" y="${f(vb.y)}" width="${f(vb.w)}" height="${f(vb.h)}" fill="${esc(s.paperColor)}"/>${layers}</svg>`
  )
}

// ---- The bucket: fill what's enclosed ----

// regionSvg draws the visible layers' items (no paper, no earlier fills) of
// the world box at `scale` pixels a unit — what a fill is bounded by.
export function regionSvg(s: Sketch, box: { x: number; y: number; w: number; h: number }, scale: number): string {
  const body = s.layers
    .filter((l) => l.visible)
    .map((l) =>
      l.items
        .map((it) => {
          if (it.kind === 'image') return it.fill ? '' : `<image href="${esc(it.src)}" x="${it.x}" y="${it.y}" width="${it.w}" height="${it.h}" preserveAspectRatio="none"/>`
          const d = outline(it)
          return d ? `<path d="${d}" fill="#000"/>` : ''
        })
        .join(''),
    )
    .join('')
  const W = Math.max(1, Math.round(box.w * scale))
  const H = Math.max(1, Math.round(box.h * scale))
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${f(box.x)} ${f(box.y)} ${f(box.w)} ${f(box.h)}" width="${W}" height="${H}">${body}</svg>`
}

// floodFill finds the area around (sx, sy) that nothing drawn closes off:
// pixels whose alpha is under `edge` are open. Gaps up to `gap` pixels in
// the lines still close it (the lines are thickened by that much first),
// and the area then grows back under the lines (no light seam along them).
// The answer: 1 for filled pixels, and their box (null: the start is on a line).
export function floodFill(
  alpha: Uint8ClampedArray | Uint8Array,
  w: number,
  h: number,
  sx: number,
  sy: number,
  gap = 2,
  edge = 60,
): { mask: Uint8Array; box: { x0: number; y0: number; x1: number; y1: number } } | null {
  const n = w * h
  // Walls: the lines, thickened by `gap`.
  let wall = new Uint8Array(n)
  for (let i = 0; i < n; i++) wall[i] = alpha[i] >= edge ? 1 : 0
  for (let k = 0; k < gap; k++) wall = grow(wall, w)
  const start = sy * w + sx
  if (sx < 0 || sy < 0 || sx >= w || sy >= h || wall[start]) return null
  let mask = new Uint8Array(n)
  const stack = [start]
  mask[start] = 1
  const open = (j: number) => {
    if (mask[j] || wall[j]) return
    mask[j] = 1
    stack.push(j)
  }
  while (stack.length) {
    const i = stack.pop()!
    const x = i % w
    const y = (i - x) / w
    if (x > 0) open(i - 1)
    if (x < w - 1) open(i + 1)
    if (y > 0) open(i - w)
    if (y < h - 1) open(i + w)
  }
  // Back under the lines: as far as they were thickened, and a pixel more.
  for (let k = 0; k <= gap; k++) mask = grow(mask, w)
  let x0 = w
  let y0 = h
  let x1 = -1
  let y1 = -1
  for (let i = 0; i < n; i++) {
    if (!mask[i]) continue
    const x = i % w
    const y = (i - x) / w
    if (x < x0) x0 = x
    if (x > x1) x1 = x
    if (y < y0) y0 = y
    if (y > y1) y1 = y
  }
  return x1 < 0 ? null : { mask, box: { x0, y0, x1, y1 } }
}

// grow sets each pixel next to a set one (4 ways).
function grow(m: Uint8Array, w: number): Uint8Array {
  const out = m.slice()
  for (let i = 0; i < m.length; i++) {
    if (!m[i]) continue
    const x = i % w
    if (x > 0) out[i - 1] = 1
    if (x < w - 1) out[i + 1] = 1
    if (i >= w) out[i - w] = 1
    if (i + w < m.length) out[i + w] = 1
  }
  return out
}
