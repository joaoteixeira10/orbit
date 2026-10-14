import { defineStore } from 'pinia'
import { workspacesApi, plannerApi, pagesApi, type Workspace, type Member, type CatalogModule, type Template, type PageSummary } from '@/api/planner'
import { onLiveEvent } from '@/lib/live'
import { CLIENT_ID } from '@/lib/api'

// The signed-in user's workspaces (sidebar order), the module catalog, the
// members of the workspaces looked at, and their favourite pages. Live
// updates keep it in sync: "workspaces" (my list changed), "ws" with what
// "workspace" / "members" (one changed), "favorites".
export const useWorkspacesStore = defineStore('workspaces', {
  state: () => ({
    list: [] as Workspace[],
    loaded: false,
    modules: [] as CatalogModule[],
    templates: [] as Template[],
    members: {} as Record<string, Member[]>,
    favorites: [] as PageSummary[],
    // The last workspace opened, so "/" returns to it.
    lastId: localStorage.getItem('orbit_last_ws') || '',
  }),
  getters: {
    byId: (s) => (id: string) => s.list.find((w) => w.id === id),
    active: (s) => s.list.filter((w) => !w.archived_at),
    archived: (s) => s.list.filter((w) => !!w.archived_at),
  },
  actions: {
    async load() {
      this.list = await workspacesApi.list()
      this.loaded = true
    },
    async loadCatalog() {
      if (this.modules.length) return
      const c = await plannerApi.catalog()
      this.modules = c.modules
      this.templates = c.templates
    },
    async loadMembers(id: string) {
      const r = await workspacesApi.get(id)
      this.members[id] = r.members
      this.upsert(r.workspace)
      return r.members
    },
    async loadFavorites() {
      this.favorites = await pagesApi.favorites()
    },
    upsert(ws: Workspace) {
      const i = this.list.findIndex((w) => w.id === ws.id)
      if (i >= 0) this.list[i] = ws
      else this.list.push(ws)
    },
    setLast(id: string) {
      this.lastId = id
      localStorage.setItem('orbit_last_ws', id)
    },
    async create(body: Parameters<typeof workspacesApi.create>[0]) {
      const ws = await workspacesApi.create(body)
      this.upsert(ws)
      return ws
    },
    async update(id: string, body: Parameters<typeof workspacesApi.update>[1]) {
      const ws = await workspacesApi.update(id, body)
      this.upsert(ws)
      return ws
    },
    async remove(id: string) {
      await workspacesApi.remove(id)
      this.list = this.list.filter((w) => w.id !== id)
      if (this.lastId === id) this.setLast('')
    },
    async reorder(ids: string[]) {
      const pos = new Map(ids.map((id, i) => [id, i]))
      this.list.sort((a, b) => (pos.get(a.id) ?? 0) - (pos.get(b.id) ?? 0))
      await workspacesApi.order(ids)
    },
  },
})

let wired = false
// wireWorkspaceLive subscribes the store to live updates (once).
export function wireWorkspaceLive() {
  if (wired) return
  wired = true
  const s = useWorkspacesStore()
  onLiveEvent('workspaces', () => void s.load())
  onLiveEvent('favorites', () => void s.loadFavorites())
  onLiveEvent('ws', (e) => {
    if (e.origin === CLIENT_ID) return
    const ws = String(e.ws)
    if (e.what === 'workspace' || e.what === 'members') {
      void s.load()
      if (s.members[ws]) void s.loadMembers(ws)
    }
    if (e.what === 'pages') void s.loadFavorites()
  })
}
