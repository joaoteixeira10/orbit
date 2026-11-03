import { defineStore } from 'pinia'
import { tasksApi, smartListsApi, type SmartList, type SmartListInput, type Task, type TaskList, type TaskInput } from '@/api/planner'
import { onLiveEvent } from '@/lib/live'
import { CLIENT_ID } from '@/lib/api'
import { i18n } from '@/i18n'
import { useToastStore } from './toast'

// gitWarn tells that a change was saved but didn't reach the linked issue
// (the background sync sends it again).
function gitWarn(t: Task) {
  if (t.git_error) useToastStore().error(i18n.global.t('git.task.failed', { error: t.git_error }))
}

// Task lists and tasks per workspace. The Tasks and Board views share it, so
// a change on one shows on the other at once. Updates are applied locally
// first and reconciled with the server's answer. An archived list and its
// tasks are put away: listsOf / tasksOf leave them out (Today, the board,
// the calendar, the timeline… don't see them); the all… getters keep them,
// for the archived list itself and a task opened from search.
export const useTasksStore = defineStore('tasks', {
  state: () => ({
    lists: {} as Record<string, TaskList[]>,
    tasks: {} as Record<string, Task[]>,
    loading: {} as Record<string, boolean>,
    smart: {} as Record<string, SmartList[]>, // saved filters, per workspace
  }),
  getters: {
    listsOf: (s) => (ws: string) => (s.lists[ws] ?? []).filter((l) => !l.archived_at),
    archivedListsOf: (s) => (ws: string) => (s.lists[ws] ?? []).filter((l) => !!l.archived_at),
    allListsOf: (s) => (ws: string) => s.lists[ws] ?? [],
    smartOf: (s) => (ws: string) => s.smart[ws] ?? [],
    tasksOf: (s) => (ws: string) => {
      const away = new Set((s.lists[ws] ?? []).filter((l) => l.archived_at).map((l) => l.id))
      const all = s.tasks[ws] ?? []
      return away.size ? all.filter((t) => !away.has(t.list_id)) : all
    },
    allTasksOf: (s) => (ws: string) => s.tasks[ws] ?? [],
    subtasksOf: (s) => (ws: string, parent: string) =>
      (s.tasks[ws] ?? []).filter((t) => t.parent_id === parent).sort((a, b) => a.rank - b.rank),
  },
  actions: {
    async load(ws: string) {
      this.loading[ws] = true
      try {
        const [lists, tasks] = await Promise.all([tasksApi.lists(ws), tasksApi.list(ws)])
        this.lists[ws] = lists
        this.tasks[ws] = tasks
      } finally {
        this.loading[ws] = false
      }
    },
    async loadLists(ws: string) {
      this.lists[ws] = await tasksApi.lists(ws)
    },
    put(ws: string, t: Task) {
      const list = (this.tasks[ws] ??= [])
      const i = list.findIndex((x) => x.id === t.id)
      if (i >= 0) list[i] = t
      else list.push(t)
    },
    async create(ws: string, body: TaskInput) {
      const t = await tasksApi.create(ws, body)
      gitWarn(t)
      this.put(ws, t)
      void this.loadLists(ws) // counts
      return t
    },
    // update applies the patch locally, then the server's answer. It returns
    // whether a repeating task moved to its next date.
    async update(ws: string, id: string, body: TaskInput): Promise<{ task: Task; repeated: boolean; shifted?: number }> {
      const cur = this.tasks[ws]?.find((t) => t.id === id)
      if (cur) {
        const optimistic = { ...cur } as Task
        if (body.title !== undefined) optimistic.title = body.title
        if (body.completed !== undefined) optimistic.done = body.completed
        if (body.priority !== undefined) optimistic.priority = body.priority as Task['priority']
        this.put(ws, optimistic)
      }
      try {
        const r = await tasksApi.update(id, body)
        gitWarn(r.task)
        this.put(ws, r.task)
        if (body.completed !== undefined || body.list_id !== undefined || body.column_id !== undefined) void this.loadLists(ws)
        // Tasks that wait on it moved along: fetch them.
        if (r.shifted) void this.load(ws)
        return r
      } catch (e) {
        if (cur) this.put(ws, cur)
        throw e
      }
    },
    async move(ws: string, id: string, column_id: string, after_id: string, list_id?: string) {
      const t = await tasksApi.move(id, { column_id, after_id, list_id })
      gitWarn(t)
      this.put(ws, t)
      // Sub-tasks follow; refresh them from the server.
      void this.load(ws)
      return t
    },
    async loadSmart(ws: string) {
      this.smart[ws] = await smartListsApi.list(ws)
    },
    // saveSmart creates (no id) or changes a smart list.
    async saveSmart(ws: string, id: string, body: SmartListInput) {
      const l = id ? await smartListsApi.update(id, body) : await smartListsApi.create(ws, body)
      const arr = (this.smart[ws] ??= [])
      const i = arr.findIndex((x) => x.id === l.id)
      if (i >= 0) arr[i] = l
      else arr.push(l)
      return l
    },
    async removeSmart(ws: string, id: string) {
      await smartListsApi.remove(id)
      this.smart[ws] = (this.smart[ws] ?? []).filter((x) => x.id !== id)
    },
    // archiveList puts a list away (or brings it back), its tasks kept.
    async archiveList(ws: string, id: string, archived: boolean) {
      const l = await tasksApi.updateList(id, { archived })
      const arr = (this.lists[ws] ??= [])
      const i = arr.findIndex((x) => x.id === id)
      if (i >= 0) arr[i] = l
      return l
    },
    async remove(ws: string, id: string) {
      await tasksApi.remove(id)
      this.tasks[ws] = (this.tasks[ws] ?? []).filter((t) => t.id !== id && t.parent_id !== id)
      void this.loadLists(ws)
    },
  },
})

let wired = false
export function wireTasksLive() {
  if (wired) return
  wired = true
  const s = useTasksStore()
  onLiveEvent('ws', (e) => {
    if (e.what === 'smart-lists' && e.origin !== CLIENT_ID && s.smart[String(e.ws)]) void s.loadSmart(String(e.ws)).catch(() => {})
    if ((e.what !== 'tasks' && e.what !== 'lists' && e.what !== 'comments') || e.origin === CLIENT_ID) return
    const ws = String(e.ws)
    if (s.tasks[ws]) void s.load(ws)
  })
}
