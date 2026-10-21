import { describe, it, expect } from 'vitest'
import type { Task, TaskList } from '@/api/planner'
import { activeCount, dueBucket, dueEdges, emptyFilter, matchTask, progressOf, type MatchContext } from './taskFilter'

const list: TaskList = {
  id: 'l1', workspace_id: 'w', name: 'Projects', color: '#5856D6', icon: '', rank: 1, open_count: 0,
  columns: [{ id: 'backlog', name: 'Backlog', done: false }, { id: 'doing', name: 'Doing', done: false }, { id: 'done', name: 'Done', done: true }],
}
const task = (p: Partial<Task>): Task => ({
  id: 't', workspace_id: 'w', list_id: 'l1', column_id: 'backlog', parent_id: null, title: 'Report', notes: '', priority: 0, tags: [],
  start: null, due: null, due_time: null, repeat: '', rank: 1, completed_at: null, done: false, assignee_ids: [], assignee_id: null, comment_count: 0, kind: '',
  created_by: 'me', created_at: '', updated_at: '', ...p,
})
// Tuesday 13 October 2026; weeks start on Monday.
const edges = dueEdges('2026-10-13', 1)
const ctx: MatchContext = { me: 'me', lists: new Map([[list.id, list]]), edges }

describe('task filter', () => {
  it('buckets due dates: overdue, today, the rest of this week, next week, later', () => {
    expect(edges).toEqual({ today: '2026-10-13', weekEnd: '2026-10-18', nextEnd: '2026-10-25' })
    expect(dueBucket(task({ due: '2026-10-12' }), edges)).toBe('overdue')
    expect(dueBucket(task({ due: '2026-10-13' }), edges)).toBe('today')
    expect(dueBucket(task({ due: '2026-10-18' }), edges)).toBe('week')
    expect(dueBucket(task({ due: '2026-10-19' }), edges)).toBe('next')
    expect(dueBucket(task({ due: '2026-10-26' }), edges)).toBe('later')
    expect(dueBucket(task({}), edges)).toBe('none')
  })
  it('reads progress from the column', () => {
    expect(progressOf(task({}), list)).toBe('notStarted')
    expect(progressOf(task({ column_id: 'doing' }), list)).toBe('inProgress')
    expect(progressOf(task({ column_id: 'done', done: true }), list)).toBe('done')
  })
  it('matches every kind of filter together', () => {
    const f = emptyFilter()
    expect(activeCount(f)).toBe(0)
    f.q = 'rep'
    f.priority = [2]
    f.assignee = ['me']
    expect(activeCount(f)).toBe(3)
    expect(matchTask(task({ priority: 2, assignee_ids: ['me'] }), f, ctx)).toBe(true)
    expect(matchTask(task({ priority: 2, assignee_ids: ['other'] }), f, ctx)).toBe(false)
    expect(matchTask(task({ priority: 1, assignee_ids: ['me'] }), f, ctx)).toBe(false)
    expect(matchTask(task({ priority: 2, assignee_ids: ['other', 'me'] }), f, ctx)).toBe(true) // one of several
    expect(matchTask(task({ title: 'Slides', priority: 2, assignee_ids: ['me'], tags: ['report'] }), f, ctx)).toBe(true)
  })
  it('"unassigned" and tags', () => {
    const f = { ...emptyFilter(), assignee: ['none'], tags: ['q4'] }
    expect(matchTask(task({ tags: ['q4'] }), f, ctx)).toBe(true)
    expect(matchTask(task({ tags: ['q3'] }), f, ctx)).toBe(false)
    expect(matchTask(task({ tags: ['q4'], assignee_ids: ['me'] }), f, ctx)).toBe(false)
  })
})

describe('the filter store', () => {
  it('a change made through the store shows in a view matcher', async () => {
    const { createPinia, setActivePinia } = await import('pinia')
    const { ref, nextTick } = await import('vue')
    const { useTaskFilter, useTaskFilterStore } = await import('./taskFilter')
    setActivePinia(createPinia())
    const f = useTaskFilter(ref('ws-new'), ref([list]), ref('me'))
    expect(f.active.value).toBe(0) // the first read creates it
    useTaskFilterStore().of('ws-new').priority.push(3)
    await nextTick()
    expect(f.active.value).toBe(1)
    expect(f.matches(task({ priority: 2 }))).toBe(false)
  })
})
