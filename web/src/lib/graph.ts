// The graph of a workspace (api/internal/api/handlers_graph.go): its things
// and how they connect, a neighbourhood of one of them, and a force layout
// to draw it (linked things pull together, everything pushes apart).
import { api } from './api'
import type { RouteLocationRaw } from 'vue-router'

export type NodeKind = 'page' | 'sheet' | 'diagram' | 'board' | 'list' | 'folder' | 'person'
export type EdgeKind = 'link' | 'embed' | 'mention' | 'parent' | 'folder' | 'list'
export const NODE_KINDS: NodeKind[] = ['page', 'sheet', 'diagram', 'board', 'list', 'folder', 'person']

export interface GraphNode {
  id: string
  kind: NodeKind
  title: string
  icon?: string
  color?: string
}
export interface GraphEdge {
  from: string
  to: string
  kind: EdgeKind
}
export interface Graph {
  nodes: GraphNode[]
  edges: GraphEdge[]
}

export const KIND_COLOR: Record<NodeKind, string> = {
  page: '#007aff',
  sheet: '#34c759',
  diagram: '#ff9500',
  board: '#af52de',
  list: '#ff2d55',
  folder: '#5ac8fa',
  person: '#8e8e93',
}

// A workspace's graph, kept for a little while (several small graphs on one
// screen ask once).
const cache = new Map<string, { at: number; graph: Promise<Graph> }>()
export function loadGraph(ws: string, fresh = false): Promise<Graph> {
  const hit = cache.get(ws)
  if (!fresh && hit && Date.now() - hit.at < 15_000) return hit.graph
  const graph = api<Graph>(`/workspaces/${ws}/graph`)
  cache.set(ws, { at: Date.now(), graph })
  graph.catch(() => cache.delete(ws))
  return graph
}
export const forgetGraph = (ws: string) => cache.delete(ws)

// neighbourhood is the part of the graph within depth links of id (either
// way along a link).
export function neighbourhood(g: Graph, id: string, depth: number): Graph {
  const near = new Map<string, string[]>()
  for (const e of g.edges) {
    near.set(e.from, [...(near.get(e.from) ?? []), e.to])
    near.set(e.to, [...(near.get(e.to) ?? []), e.from])
  }
  const keep = new Set([id])
  let ring = [id]
  for (let d = 0; d < depth && ring.length; d++) {
    const next: string[] = []
    for (const x of ring) {
      for (const y of near.get(x) ?? []) {
        if (keep.has(y)) continue
        keep.add(y)
        next.push(y)
      }
    }
    ring = next
  }
  return { nodes: g.nodes.filter((n) => keep.has(n.id)), edges: g.edges.filter((e) => keep.has(e.from) && keep.has(e.to)) }
}

// only keeps the kinds asked for (and the links between what's kept).
export function only(g: Graph, kinds: Set<NodeKind>): Graph {
  const nodes = g.nodes.filter((n) => kinds.has(n.kind))
  const ids = new Set(nodes.map((n) => n.id))
  return { nodes, edges: g.edges.filter((e) => ids.has(e.from) && ids.has(e.to)) }
}

// degrees counts each one's links.
export function degrees(g: Graph): Map<string, number> {
  const out = new Map<string, number>()
  for (const e of g.edges) {
    out.set(e.from, (out.get(e.from) ?? 0) + 1)
    out.set(e.to, (out.get(e.to) ?? 0) + 1)
  }
  return out
}

export interface Point {
  x: number
  y: number
}

// layout places the nodes (Fruchterman–Reingold): every pair pushes apart,
// each link pulls its ends together, a pull to the middle keeps loose ones
// near; it cools as it goes. The same graph gives the same picture.
// pinned: where some start and stay (the one a small graph is about, in the
// middle).
export function layout(g: Graph, opts: { size?: number; iterations?: number; pinned?: Record<string, Point> } = {}): Map<string, Point> {
  const n = g.nodes.length
  const size = opts.size ?? Math.max(400, Math.sqrt(n) * 110)
  const k = (size / Math.sqrt(Math.max(n, 1))) * 0.75
  const pos = new Map<string, Point>()
  // Start on a spiral, in order: deterministic.
  g.nodes.forEach((v, i) => {
    const pin = opts.pinned?.[v.id]
    const a = i * 2.39996 // the golden angle
    const r = (size / 2) * Math.sqrt((i + 0.5) / Math.max(n, 1))
    pos.set(v.id, pin ? { ...pin } : { x: Math.cos(a) * r, y: Math.sin(a) * r })
  })
  const edges = g.edges.filter((e) => pos.has(e.from) && pos.has(e.to) && e.from !== e.to)
  const iterations = opts.iterations ?? (n > 400 ? 120 : 260)
  let temp = size / 8
  const gravity = 0.9
  const ids = g.nodes.map((v) => v.id)
  for (let it = 0; it < iterations; it++) {
    const disp = new Map<string, Point>(ids.map((id) => [id, { x: 0, y: 0 }]))
    for (let i = 0; i < n; i++) {
      const a = pos.get(ids[i])!
      const da = disp.get(ids[i])!
      for (let j = i + 1; j < n; j++) {
        const b = pos.get(ids[j])!
        let dx = a.x - b.x
        let dy = a.y - b.y
        let d2 = dx * dx + dy * dy
        if (d2 < 0.01) {
          // On top of each other: nudge apart, the same way every time.
          dx = ((i % 7) - 3) * 0.1 + 0.05
          dy = ((j % 5) - 2) * 0.1 + 0.05
          d2 = dx * dx + dy * dy
        }
        const f = (k * k) / d2
        da.x += dx * f
        da.y += dy * f
        const db = disp.get(ids[j])!
        db.x -= dx * f
        db.y -= dy * f
      }
    }
    for (const e of edges) {
      const a = pos.get(e.from)!
      const b = pos.get(e.to)!
      const dx = a.x - b.x
      const dy = a.y - b.y
      const d = Math.sqrt(dx * dx + dy * dy) || 0.01
      const f = d / k
      const da = disp.get(e.from)!
      const db = disp.get(e.to)!
      da.x -= dx * f
      da.y -= dy * f
      db.x += dx * f
      db.y += dy * f
    }
    for (const id of ids) {
      if (opts.pinned?.[id]) continue
      const p = pos.get(id)!
      const dv = disp.get(id)!
      // The pull to the middle: as strong as the push of the others at
      // about the picture's radius, so loose ones stay in sight.
      dv.x -= p.x * gravity
      dv.y -= p.y * gravity
      const len = Math.sqrt(dv.x * dv.x + dv.y * dv.y) || 1
      const step = Math.min(len, temp)
      p.x += (dv.x / len) * step
      p.y += (dv.y / len) * step
    }
    temp = Math.max(temp * 0.97, 0.5)
  }
  return pos
}

// routeOf is where a thing opens (a person: nowhere).
export function routeOf(ws: string, n: GraphNode): RouteLocationRaw | null {
  switch (n.kind) {
    case 'page':
      return { name: 'ws-page', params: { ws, page: n.id } }
    case 'sheet':
      return { name: 'ws-sheet', params: { ws, id: n.id } }
    case 'diagram':
      return { name: 'ws-diagram', params: { ws, id: n.id } }
    case 'board':
      return { name: 'ws-whiteboard', params: { ws, id: n.id } }
    case 'list':
      return { name: 'ws-tasks', params: { ws, list: n.id } }
    case 'folder':
      return { name: 'ws-files', params: { ws }, query: { folder: n.id } }
    default:
      return null
  }
}
