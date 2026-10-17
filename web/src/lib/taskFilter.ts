// Task filters, Microsoft Planner-style: by keyword, due date, priority,
// progress, who it's for, tags and list. One filter per workspace, shared by
// the Tasks, Board, Timeline and Charts views (and remembered per browser),
// so narrowing it on one shows the same tasks on the others.
import { computed, type Ref } from 'vue'
import { defineStore } from 'pinia'
import type { Task, TaskList } from '@/api/planner'
import { dateStr, addDays, startOfWeek, parseDate, todayStr, weekStart } from '@/lib/dates'

export type DueBucket = 'overdue' | 'today' | 'week' | 'next' | 'later' | 'none'
export type Progress = 'notStarted' | 'inProgress' | 'done'
export const DUE_BUCKETS: DueBucket[] = ['overdue', 'today', 'week', 'next', 'later', 'none']
export const PROGRESS: Progress[] = ['notStarted', 'inProgress', 'done']

export interface TaskFilter {
  q: string
  due: DueBucket[]
  priority: number[] // 0 none … 3 high
  progress: Progress[]
  assignee: string[] // user ids, 'me' or 'none'
  tags: string[]
  lists: string[]
}

export const emptyFilter = (): TaskFilter => ({ q: '', due: [], priority: [], progress: [], assignee: [], tags: [], lists: [] })

// activeCount is how many kinds of filter are on (the badge on the button).
export function activeCount(f: TaskFilter): number {
  return (f.q.trim() ? 1 : 0) + [f.due, f.priority, f.progress, f.assignee, f.tags, f.lists].filter((a) => a.length).length
}

// progressOf: done tasks are completed; open ones in their list's first open
// column haven't started; open ones further along are in progress.
export function progressOf(t: Task, list?: TaskList): Progress {
  if (t.done) return 'done'
  const firstOpen = list?.columns.find((c) => !c.done)
  return !firstOpen || t.column_id === firstOpen.id ? 'notStarted' : 'inProgress'
}

// The edges of the due buckets for a given today: the rest of this week,
// then next week, then later.
export interface DueEdges {
  today: string
  weekEnd: string
  nextEnd: string
}
export function dueEdges(today = todayStr(), ws: number = weekStart()): DueEdges {
  const wEnd = addDays(startOfWeek(parseDate(today), ws), 6)
  return { today, weekEnd: dateStr(wEnd), nextEnd: dateStr(addDays(wEnd, 7)) }
}
export function dueBucket(t: Task, e: DueEdges): DueBucket {
  if (!t.due) return 'none'
  if (t.due < e.today) return t.done ? 'week' : 'overdue'
  if (t.due === e.today) return 'today'
  if (t.due <= e.weekEnd) return 'week'
  if (t.due <= e.nextEnd) return 'next'
  return 'later'
}

export interface MatchContext {
  me: string
  lists: Map<string, TaskList>
  edges: DueEdges
}

export function matchTask(t: Task, f: TaskFilter, ctx: MatchContext): boolean {
  const q = f.q.trim().toLowerCase()
  if (q && !t.title.toLowerCase().includes(q) && !t.notes.toLowerCase().includes(q) && !t.tags.some((x) => x.toLowerCase().includes(q))) return false
  if (f.due.length && !f.due.includes(dueBucket(t, ctx.edges))) return false
  if (f.priority.length && !f.priority.includes(t.priority)) return false
  if (f.progress.length && !f.progress.includes(progressOf(t, ctx.lists.get(t.list_id)))) return false
  if (f.assignee.length) {
    const who = t.assignee_ids.length ? t.assignee_ids : ['none']
    if (!who.some((w) => f.assignee.includes(w) || (w === ctx.me && f.assignee.includes('me')))) return false
  }
  if (f.tags.length && !t.tags.some((x) => f.tags.includes(x))) return false
  if (f.lists.length && !f.lists.includes(t.list_id)) return false
  return true
}

const KEY = 'orbit_task_filters'
function loadAll(): Record<string, TaskFilter> {
  try {
    const raw = JSON.parse(localStorage.getItem(KEY) || '{}') as Record<string, Partial<TaskFilter>>
    return Object.fromEntries(Object.entries(raw).map(([ws, f]) => [ws, { ...emptyFilter(), ...f }]))
  } catch {
    return {}
  }
}

export const useTaskFilterStore = defineStore('taskFilter', {
  state: () => ({ byWs: loadAll() }),
  actions: {
    // of is the workspace's filter, created empty the first time. It returns
    // the stored (reactive) object, not the one just assigned.
    of(ws: string): TaskFilter {
      if (!this.byWs[ws]) this.byWs[ws] = emptyFilter()
      return this.byWs[ws]
    },
    save() {
      try {
        // Only filters that do something are worth keeping.
        localStorage.setItem(KEY, JSON.stringify(Object.fromEntries(Object.entries(this.byWs).filter(([, f]) => activeCount(f)))))
      } catch {
        /* private mode: the filter just isn't remembered */
      }
    },
    clear(ws: string) {
      this.byWs[ws] = emptyFilter()
      this.save()
    },
  },
})

// useTaskFilter gives a view the workspace's filter and a matcher for it.
export function useTaskFilter(ws: Ref<string>, lists: Ref<TaskList[]>, me: Ref<string>) {
  const store = useTaskFilterStore()
  const filter = computed(() => store.of(ws.value))
  const active = computed(() => activeCount(filter.value))
  const ctx = computed<MatchContext>(() => ({ me: me.value, lists: new Map(lists.value.map((l) => [l.id, l])), edges: dueEdges() }))
  const matches = (t: Task) => !active.value || matchTask(t, filter.value, ctx.value)
  return { filter, active, matches, clear: () => store.clear(ws.value) }
}
