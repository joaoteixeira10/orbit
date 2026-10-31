// Drawing together on a whiteboard (a sketch, or an Excalidraw one): the
// drawing as a set of keyed pieces — each stroke or picture, each layer, the
// paper; or each element and image, the background — in a shared map (Yjs,
// through lib/collab). A change here sends the pieces it changed; a change
// from elsewhere is drawn after this tab's own waiting ones are sent, so
// nothing drawn here is lost. Each piece keeps its place (its order), so
// strokes added at once by two people both stay.
import * as Y from 'yjs'
import type { CollabLink } from './collab'

export type BoardKind = 'sketch' | 'excalidraw'
type Pieces = Map<string, string> // key → JSON

const byOrder = (a: [string, any], b: [string, any]) => (a[1].o - b[1].o) || (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0)

export function piecesOf(kind: BoardKind, scene: string): Pieces {
  const out: Pieces = new Map()
  let s: any
  try {
    s = JSON.parse(scene || '{}')
  } catch {
    return out
  }
  if (kind === 'sketch') {
    out.set('M', JSON.stringify({ paper: s.paper, paperColor: s.paperColor }))
    ;(s.layers ?? []).forEach((l: any, i: number) => {
      out.set(`L:${l.id}`, JSON.stringify({ o: i, name: l.name, visible: l.visible, opacity: l.opacity }))
      ;(l.items ?? []).forEach((it: any, j: number) => out.set(`I:${it.id}`, JSON.stringify({ o: j, l: l.id, d: it })))
    })
  } else {
    out.set('M', JSON.stringify({ bg: s.appState?.viewBackgroundColor ?? null }))
    ;(s.elements ?? []).forEach((el: any, i: number) => out.set(`E:${el.id}`, JSON.stringify({ o: i, d: el })))
    for (const [id, f] of Object.entries(s.files ?? {})) out.set(`F:${id}`, JSON.stringify(f))
  }
  return out
}

export function sceneOf(kind: BoardKind, pieces: Pieces): string {
  const parsed = [...pieces].map(([k, v]) => [k, JSON.parse(v)] as [string, any])
  const meta = parsed.find(([k]) => k === 'M')?.[1] ?? {}
  if (kind === 'sketch') {
    const layers = parsed.filter(([k]) => k.startsWith('L:')).sort(byOrder)
    const items = parsed.filter(([k]) => k.startsWith('I:')).sort(byOrder)
    return JSON.stringify({
      type: 'orbit-sketch',
      version: 1,
      paper: meta.paper,
      paperColor: meta.paperColor,
      layers: layers.map(([k, l]) => {
        const id = k.slice(2)
        return { id, name: l.name, visible: l.visible, opacity: l.opacity, items: items.filter(([, it]) => it.l === id).map(([, it]) => it.d) }
      }),
    })
  }
  const elements = parsed.filter(([k]) => k.startsWith('E:')).sort(byOrder).map(([, e]) => e.d)
  const files = Object.fromEntries(parsed.filter(([k]) => k.startsWith('F:')).map(([k, f]) => [k.slice(2), f]))
  return JSON.stringify({ type: 'excalidraw', version: 2, source: 'orbit', elements, appState: meta.bg ? { viewBackgroundColor: meta.bg } : {}, files })
}

export class BoardTogether {
  private map: Y.Map<string>
  private last: Pieces = new Map()
  private timer: ReturnType<typeof setTimeout> | undefined
  private off: () => void

  constructor(
    readonly link: CollabLink,
    readonly kind: BoardKind,
    // The drawing as it is here now (its scene), and drawing another's in.
    private scene: () => string,
    private draw: (scene: string) => void,
  ) {
    this.map = link.doc.getMap<string>('pieces')
    const observer = (_e: Y.YMapEvent<string>, tx: Y.Transaction) => {
      if (tx.origin !== link) return // ours
      this.pushNow() // what's waiting here goes first, then theirs is drawn
      const pieces = new Map(this.map.entries())
      this.last = pieces
      this.draw(sceneOf(this.kind, pieces))
    }
    this.map.observe(observer)
    this.off = () => this.map.unobserve(observer)
  }

  // seed fills an empty shared drawing from what's saved (in a fixed
  // order: any tab doing it makes the same changes).
  seed(saved: string) {
    const pieces = piecesOf(this.kind, saved)
    this.link.seed((doc) => {
      const m = doc.getMap<string>('pieces')
      for (const k of [...pieces.keys()].sort()) m.set(k, pieces.get(k)!)
    })
    this.last = new Map(this.map.entries())
  }

  // The shared drawing as it is (after joining), to draw here.
  current(): string {
    this.last = new Map(this.map.entries())
    return sceneOf(this.kind, this.last)
  }

  // changed: the drawing changed here; its pieces go out shortly.
  changed() {
    clearTimeout(this.timer)
    this.timer = setTimeout(() => this.pushNow(), 100)
  }

  pushNow() {
    clearTimeout(this.timer)
    const now = piecesOf(this.kind, this.scene())
    const was = this.last
    const set: [string, string][] = []
    const gone: string[] = []
    for (const [k, v] of now) if (was.get(k) !== v) set.push([k, v])
    for (const k of was.keys()) if (!now.has(k)) gone.push(k)
    if (!set.length && !gone.length) return
    this.last = now
    Y.transact(this.link.doc, () => {
      for (const [k, v] of set) this.map.set(k, v)
      for (const k of gone) this.map.delete(k)
    })
  }

  stop() {
    this.pushNow()
    this.off()
  }
}
