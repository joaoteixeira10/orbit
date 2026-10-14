import { defineStore } from 'pinia'
import { pagesApi, type PageSummary } from '@/api/planner'
import { onLiveEvent } from '@/lib/live'
import { CLIENT_ID } from '@/lib/api'

export interface PageNode extends PageSummary {
  children: PageNode[]
}

// Page trees per workspace (titles and icons only; documents load per page).
export const usePagesStore = defineStore('pages', {
  state: () => ({
    byWs: {} as Record<string, PageSummary[]>,
    // Expanded tree rows, remembered per browser.
    open: new Set<string>(JSON.parse(localStorage.getItem('orbit_tree_open') || '[]') as string[]),
  }),
  getters: {
    tree: (s) => (ws: string): PageNode[] => {
      const pages = s.byWs[ws] ?? []
      const nodes = new Map<string, PageNode>(pages.map((p) => [p.id, { ...p, children: [] }]))
      const roots: PageNode[] = []
      for (const n of nodes.values()) {
        const parent = n.parent_id ? nodes.get(n.parent_id) : undefined
        if (parent) parent.children.push(n)
        else roots.push(n)
      }
      const sort = (list: PageNode[]) => {
        list.sort((a, b) => a.rank - b.rank)
        list.forEach((c) => sort(c.children))
      }
      sort(roots)
      return roots
    },
    find: (s) => (id: string): PageSummary | undefined => {
      for (const list of Object.values(s.byWs)) {
        const p = list.find((x) => x.id === id)
        if (p) return p
      }
      return undefined
    },
    // The chain of ancestors of a page (root first), for breadcrumbs.
    path: (s) => (ws: string, id: string): PageSummary[] => {
      const list = s.byWs[ws] ?? []
      const out: PageSummary[] = []
      let cur = list.find((p) => p.id === id)
      for (let guard = 0; cur && guard < 50; guard++) {
        out.unshift(cur)
        cur = cur.parent_id ? list.find((p) => p.id === cur!.parent_id) : undefined
      }
      return out
    },
  },
  actions: {
    async load(ws: string) {
      this.byWs[ws] = await pagesApi.list(ws)
    },
    patch(p: Partial<PageSummary> & { id: string }) {
      for (const list of Object.values(this.byWs)) {
        const i = list.findIndex((x) => x.id === p.id)
        if (i >= 0) list[i] = { ...list[i], ...p }
      }
    },
    toggle(id: string, on?: boolean) {
      const next = on ?? !this.open.has(id)
      if (next) this.open.add(id)
      else this.open.delete(id)
      localStorage.setItem('orbit_tree_open', JSON.stringify([...this.open]))
    },
  },
})

let wired = false
export function wirePagesLive() {
  if (wired) return
  wired = true
  const s = usePagesStore()
  onLiveEvent('ws', (e) => {
    if (e.what !== 'pages' || e.origin === CLIENT_ID) return
    const ws = String(e.ws)
    if (s.byWs[ws]) void s.load(ws)
  })
}
