import { defineStore } from 'pinia'
import { notesApi, type Note, type NoteInput } from '@/api/planner'
import { onLiveEvent } from '@/lib/live'
import { CLIENT_ID } from '@/lib/api'

// Quick notes per workspace — and under "me" the user's own Quick Notes —
// pinned first, then by rank (the server's order). Changes show at once and
// are reconciled with the server's answer.
const order = (a: Note, b: Note) => Number(b.pinned) - Number(a.pinned) || a.rank - b.rank

export const useNotesStore = defineStore('notes', {
  state: () => ({
    byWs: {} as Record<string, Note[]>,
  }),
  getters: {
    notesOf: (s) => (ws: string) => s.byWs[ws] ?? [],
  },
  actions: {
    async load(ws: string) {
      this.byWs[ws] = await notesApi.list(ws)
    },
    put(ws: string, n: Note) {
      const list = (this.byWs[ws] ??= [])
      const i = list.findIndex((x) => x.id === n.id)
      if (i >= 0) list[i] = n
      else list.push(n)
      list.sort(order)
    },
    async create(ws: string, body: NoteInput) {
      const n = await notesApi.create(ws, body)
      this.put(ws, n)
      return n
    },
    async update(ws: string, id: string, body: NoteInput) {
      const cur = this.byWs[ws]?.find((x) => x.id === id)
      if (cur) this.put(ws, { ...cur, ...body, items: body.items ?? cur.items } as Note)
      try {
        const n = await notesApi.update(id, body)
        this.put(ws, n)
        return n
      } catch (e) {
        if (cur) this.put(ws, cur)
        throw e
      }
    },
    // remove deletes a note and returns it, so it can be put back (Undo).
    async remove(ws: string, id: string) {
      const cur = this.byWs[ws]?.find((x) => x.id === id)
      this.byWs[ws] = (this.byWs[ws] ?? []).filter((x) => x.id !== id)
      try {
        await notesApi.remove(id)
      } catch (e) {
        if (cur) this.put(ws, cur)
        throw e
      }
      return cur
    },
    // move puts a note in another workspace ("me" = my Quick Notes); it
    // leaves this wall and lands on top of the other one, if loaded.
    async move(from: string, id: string, to: string) {
      const n = await notesApi.move(id, to === 'me' ? '' : to)
      this.byWs[from] = (this.byWs[from] ?? []).filter((x) => x.id !== id)
      if (this.byWs[to]) this.put(to, n)
      return n
    },
    // reorder saves a new order of the given notes (every note of the wall).
    async reorder(ws: string, ids: string[]) {
      const list = this.byWs[ws] ?? []
      const pos = new Map(ids.map((id, i) => [id, (i + 1) * 1024]))
      this.byWs[ws] = list.map((n) => ({ ...n, rank: pos.get(n.id) ?? n.rank })).sort(order)
      try {
        await notesApi.order(ws, ids)
      } catch (e) {
        await this.load(ws)
        throw e
      }
    },
  },
})

let wired = false
export function wireNotesLive() {
  if (wired) return
  wired = true
  const s = useNotesStore()
  onLiveEvent('ws', (e) => {
    if (e.what !== 'notes' || e.origin === CLIENT_ID) return
    const ws = String(e.ws)
    if (s.byWs[ws]) void s.load(ws)
  })
}
