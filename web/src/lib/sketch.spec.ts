import { describe, expect, it } from 'vitest'
import { emptySketch, hits, isSketch, lassoed, moved, outline, readSketch, recognize, rub, simplify, toSvg, wipe, type Item, type Point, type Stroke } from './sketch'

const stroke = (points: number[][], over: Partial<Stroke> = {}): Stroke => ({ id: 's', kind: 'stroke', tool: 'pen', color: '#000000', size: 4, opacity: 1, points: points.map(([x, y]) => [x, y, 0.5] as Point), ...over })
const wobble = (fn: (t: number) => number[], n = 60) => Array.from({ length: n + 1 }, (_, i) => fn(i / n))

describe('sketch', () => {
  it('knows its scene and reads it back safely', () => {
    const s = emptySketch('Layer 1')
    const scene = JSON.stringify(s)
    expect(isSketch(scene)).toBe(true)
    expect(isSketch('{"type":"excalidraw","elements":[]}')).toBe(false)
    expect(isSketch('')).toBe(false)
    expect(readSketch(scene, 'L').layers[0].name).toBe('Layer 1')
    expect(readSketch('nonsense', 'L').layers).toHaveLength(1)
    const odd = readSketch(JSON.stringify({ type: 'orbit-sketch', paper: 'weird', paperColor: 'red', layers: [{ items: [{ kind: 'image', src: 'javascript:x' }, { kind: 'stroke', points: [[0, 0, 0.5]] }] }] }), 'L')
    expect(odd.paper).toBe('plain')
    expect(odd.paperColor).toBe('#ffffff')
    expect(odd.layers[0].items).toHaveLength(1)
  })

  it('outlines strokes as paths', () => {
    expect(outline(stroke([[0, 0], [10, 10], [20, 5]]))).toMatch(/^M[\d.,-]+ Q.+Z$/)
    expect(outline(stroke([[5, 5]]))).not.toBe('')
  })

  it('snaps a held line, circle, triangle and rectangle', () => {
    const line = recognize(wobble((t) => [t * 200, t * 100 + Math.sin(t * 20) * 2]))
    expect(line?.shape).toBe('line')
    expect(line?.points[0]).toEqual([0, 0, 0.5])
    const circle = recognize(wobble((t) => [100 + 50 * Math.cos(t * 2 * Math.PI) + Math.sin(t * 40) * 2, 100 + 50 * Math.sin(t * 2 * Math.PI)]))
    expect(circle?.shape).toBe('circle')
    const ellipse = recognize(wobble((t) => [100 + 90 * Math.cos(t * 2 * Math.PI), 100 + 40 * Math.sin(t * 2 * Math.PI)]))
    expect(ellipse?.shape).toBe('ellipse')
    const tri = recognize([...wobble((t) => [t * 100, 0], 10), ...wobble((t) => [100 - t * 50, t * 90], 10), ...wobble((t) => [50 - t * 50, 90 - t * 90], 10)])
    expect(tri?.shape).toBe('triangle')
    const rect = recognize([...wobble((t) => [t * 120, 1], 10), ...wobble((t) => [121, t * 80], 10), ...wobble((t) => [120 - t * 120, 80], 10), ...wobble((t) => [0, 80 - t * 80], 10)])
    expect(rect?.shape).toBe('rectangle')
    // A scribble stays a scribble.
    expect(recognize(wobble((t) => [t * 100, Math.sin(t * 12) * 40]))).toBeNull()
    expect(recognize([[0, 0], [3, 3], [5, 5]])).toBeNull()
  })

  it('rubs out only the part it touches', () => {
    const items: Item[] = [stroke([[0, 0], [100, 0]], { size: 2 }), { id: 'p', kind: 'image', src: 'data:image/png;base64,x', x: 40, y: -10, w: 20, h: 20 }]
    const out = rub(items, 50, 0, 5)
    const strokes = out.filter((x): x is Stroke => x.kind === 'stroke')
    expect(strokes).toHaveLength(2)
    expect(Math.max(...strokes[0].points.map((p) => p[0]))).toBeLessThan(45)
    expect(Math.min(...strokes[1].points.map((p) => p[0]))).toBeGreaterThan(55)
    expect(out.some((x) => x.kind === 'image')).toBe(true)
    // Far away: the same array back.
    expect(rub(items, 50, 60, 5)).toBe(items)
    expect(wipe(items, 10, 1, 3)).toHaveLength(1)
    expect(hits(strokes[0], 20, 2, 2)).toBe(true)
  })

  it('lassos most of a stroke, and moves it', () => {
    const a = stroke([[10, 10], [20, 20]], { id: 'a' })
    const b = stroke([[200, 200], [210, 210]], { id: 'b' })
    const sel = lassoed([a, b], [[0, 0], [50, 0], [50, 50], [0, 50]])
    expect([...sel]).toEqual(['a'])
    expect((moved(a, 5, -5) as Stroke).points[0]).toEqual([15, 5, 0.5])
  })

  it('simplifies a line to its corners', () => {
    expect(simplify([[0, 0], [5, 0.1], [10, 0], [10, 10]], 0.5)).toEqual([[0, 0], [10, 0], [10, 10]])
  })

  it('draws an SVG picture on its paper', () => {
    const s = emptySketch('L')
    s.paperColor = '#fbf6e9'
    s.layers[0].items.push(stroke([[0, 0], [50, 50]], { tool: 'highlighter', color: '#ffcc00' }), stroke([[0, 50], [50, 0]], { tool: 'pencil' }))
    const svg = toSvg(s)
    expect(svg).toContain('fill="#fbf6e9"')
    expect(svg).toContain('mix-blend-mode:multiply')
    expect(svg).toContain('filter="url(#grain)"')
    s.layers[0].visible = false
    expect(toSvg(s)).not.toContain('<path')
  })
})
