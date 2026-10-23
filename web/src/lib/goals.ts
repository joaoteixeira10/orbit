// Goals: the API, and how one is doing — its progress (from its tasks, or by
// hand when it has none), days left, and a status: achieved, done (100%
// but not marked), overdue, behind (well under where the time says it
// should be), on track, or open (no date).
import { api } from '@/lib/api'
import { parseDate } from '@/lib/dates'

export interface Goal {
  id: string
  workspace_id: string
  title: string
  notes: string
  icon: string
  color: string
  due: string | null
  progress: number // by hand (no tasks)
  achieved_at?: string | null
  created_at: string
  tasks: number
  done: number
}
export type GoalInput = Partial<Pick<Goal, 'title' | 'notes' | 'icon' | 'color' | 'progress'>> & { due?: string; achieved?: boolean }
export type GoalStatus = 'achieved' | 'done' | 'overdue' | 'behind' | 'on-track' | 'open'

const j = (b: unknown) => JSON.stringify(b)
export const goalsApi = {
  list: (ws: string) => api<Goal[]>(`/workspaces/${ws}/goals`),
  create: (ws: string, b: GoalInput) => api<Goal>(`/workspaces/${ws}/goals`, { method: 'POST', body: j(b) }),
  update: (id: string, b: GoalInput) => api<Goal>(`/goals/${id}`, { method: 'PUT', body: j(b) }),
  remove: (id: string) => api<void>(`/goals/${id}`, { method: 'DELETE' }),
}

export const pct = (g: Pick<Goal, 'tasks' | 'done' | 'progress'>) => (g.tasks ? Math.round((g.done / g.tasks) * 100) : g.progress)

// daysLeft until its date (negative: past it); null with no date.
export function daysLeft(g: Pick<Goal, 'due'>, today = new Date()): number | null {
  if (!g.due) return null
  const t = new Date(today.getFullYear(), today.getMonth(), today.getDate())
  return Math.round((parseDate(g.due).getTime() - t.getTime()) / 86400000)
}

export function status(g: Goal, today = new Date()): GoalStatus {
  if (g.achieved_at) return 'achieved'
  const p = pct(g)
  if (p >= 100) return 'done'
  const left = daysLeft(g, today)
  if (left === null) return 'open'
  if (left < 0) return 'overdue'
  // Where the time says it should be: the share of the time from its start
  // to its date that has gone by.
  const start = new Date(g.created_at).getTime()
  const end = parseDate(g.due!).getTime() + 86400000
  const gone = Math.min(1, Math.max(0, (today.getTime() - start) / Math.max(1, end - start)))
  return p + 15 < gone * 100 ? 'behind' : 'on-track'
}
