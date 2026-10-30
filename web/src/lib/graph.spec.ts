import { describe, expect, it } from 'vitest'
import { degrees, layout, neighbourhood, only, type Graph } from './graph'

const g: Graph = {
  nodes: ['a', 'b', 'c', 'd', 'e', 'x', 'y'].map((id) => ({ id, kind: id === 'x' ? 'sheet' : 'page', title: id.toUpperCase() })),
  edges: [
    { from: 'a', to: 'b', kind: 'link' },
    { from: 'b', to: 'c', kind: 'link' },
    { from: 'c', to: 'd', kind: 'parent' },
    { from: 'd', to: 'e', kind: 'link' },
    { from: 'a', to: 'x', kind: 'embed' },
  ],
}

describe('graph', () => {
  it('takes a neighbourhood, links either way', () => {
    expect(neighbourhood(g, 'c', 1).nodes.map((n) => n.id)).toEqual(['b', 'c', 'd'])
    expect(neighbourhood(g, 'c', 2).nodes.map((n) => n.id)).toEqual(['a', 'b', 'c', 'd', 'e'])
    expect(neighbourhood(g, 'c', 2).edges).toHaveLength(4)
    expect(neighbourhood(g, 'y', 3).nodes.map((n) => n.id)).toEqual(['y'])
  })

  it('keeps only some kinds, and counts links', () => {
    const pages = only(g, new Set(['page']))
    expect(pages.nodes).toHaveLength(6)
    expect(pages.edges).toHaveLength(4)
    expect(degrees(g).get('a')).toBe(2)
    expect(degrees(g).get('y')).toBeUndefined()
  })

  it('lays linked things out closer than unlinked ones, the same every time', () => {
    const p = layout(g)
    const d = (a: string, b: string) => Math.hypot(p.get(a)!.x - p.get(b)!.x, p.get(a)!.y - p.get(b)!.y)
    expect(d('a', 'b')).toBeLessThan(d('a', 'e'))
    expect(d('c', 'd')).toBeLessThan(d('c', 'y'))
    for (const v of p.values()) expect(Number.isFinite(v.x) && Number.isFinite(v.y)).toBe(true)
    const again = layout(g)
    expect(again.get('e')).toEqual(p.get('e'))
    const pinned = layout(g, { pinned: { c: { x: 0, y: 0 } } })
    expect(pinned.get('c')).toEqual({ x: 0, y: 0 })
  })
})
