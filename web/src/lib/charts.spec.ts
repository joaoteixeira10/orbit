import { describe, it, expect } from 'vitest'
import type { Task, TaskList } from '@/api/planner'
import { statusOf, countStatus, countBy, summary, weekly, niceScale, totalOf } from './charts'

const list: TaskList = {
  id: 'L',
  workspace_id: 'w',
  name: 'Work',
  color: '#007aff',
  icon: '',
  rank: 0,
  open_count: 0,
  columns: [
    { id: 'todo', name: 'To do', done: false },
    { id: 'doing', name: 'Doing', done: false },
    { id: 'done', name: 'Done', done: true },
  ],
}
const lists = new Map([[list.id, list]])
let n = 0
function task(p: Partial<Task> = {}): Task {
  return {
    id: String(++n),
    workspace_id: 'w',
    list_id: 'L',
    column_id: 'todo',
    parent_id: null,
    title: 't',
    notes: '',
    priority: 0,
    tags: [],
    start: null,
    due: null,
    due_time: null,
    repeat: '',
    rank: 0,
    completed_at: null,
    done: false,
    assignee_ids: [],
    assignee_id: null,
    comment_count: 0, kind: '',
    created_by: 'u',
    created_at: '2026-10-01T10:00:00Z',
    updated_at: '2026-10-01T10:00:00Z',
    ...p,
  }
}
const today = '2026-10-14' // a Wednesday

describe('statusOf', () => {
  it('follows the column, with overdue open tasks as late', () => {
    expect(statusOf(task(), list, today)).toBe('notStarted')
    expect(statusOf(task({ column_id: 'doing' }), list, today)).toBe('inProgress')
    expect(statusOf(task({ column_id: 'doing', due: '2026-10-13' }), list, today)).toBe('late')
    expect(statusOf(task({ due: today }), list, today)).toBe('notStarted')
    expect(statusOf(task({ done: true, column_id: 'done', due: '2026-10-01' }), list, today)).toBe('done')
  })
})

describe('counts', () => {
  const ts = [task(), task({ column_id: 'doing' }), task({ due: '2026-10-01' }), task({ done: true, tags: ['a', 'b'] }), task({ tags: ['a'] })]
  it('counts every status', () => {
    const c = countStatus(ts, lists, today)
    expect(c).toEqual({ done: 1, inProgress: 1, notStarted: 2, late: 1 })
    expect(totalOf(c)).toBe(5)
  })
  it('counts a task in each of its keys', () => {
    const m = countBy(ts, lists, today, (t) => t.tags)
    expect(totalOf(m.get('a')!)).toBe(2)
    expect(m.get('b')!.done).toBe(1)
  })
})

describe('summary', () => {
  it('splits open tasks into late and due this week', () => {
    const now = new Date('2026-10-14T12:00:00')
    const s = summary(
      [
        task({ due: '2026-10-10' }),
        task({ due: '2026-10-18' }), // Sunday, same week (Monday start)
        task({ due: '2026-10-19' }),
        task({ done: true, completed_at: '2026-10-12T09:00:00' }),
        task({ done: true, completed_at: '2026-09-01T09:00:00' }),
      ],
      today,
      1,
      now,
    )
    expect(s).toEqual({ open: 3, late: 1, dueWeek: 1, done7: 1 })
  })
})

describe('weekly', () => {
  it('buckets created and completed tasks into weeks, the current one last', () => {
    const w = weekly(
      [
        task({ created_at: '2026-10-12T08:00:00' }),
        task({ created_at: '2026-10-05T08:00:00', done: true, completed_at: '2026-10-14T08:00:00' }),
        task({ created_at: '2025-01-01T08:00:00' }),
      ],
      3,
      today,
      1,
    )
    expect(w.map((x) => x.start)).toEqual(['2026-09-28', '2026-10-05', '2026-10-12'])
    expect(w.map((x) => [x.created, x.completed])).toEqual([
      [0, 0],
      [1, 0],
      [1, 1],
    ])
  })
})

describe('niceScale', () => {
  it('rounds the axis up to clean steps', () => {
    expect(niceScale(0)).toEqual({ max: 5, step: 1 })
    expect(niceScale(3)).toEqual({ max: 3, step: 1 })
    expect(niceScale(7)).toEqual({ max: 8, step: 2 })
    expect(niceScale(45)).toEqual({ max: 50, step: 10 })
  })
})
