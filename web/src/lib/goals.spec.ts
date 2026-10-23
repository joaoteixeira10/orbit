import { describe, expect, it } from 'vitest'
import { daysLeft, pct, status, type Goal } from './goals'

const goal = (over: Partial<Goal>): Goal => ({
  id: 'g', workspace_id: 'w', title: 'Ship', notes: '', icon: '', color: '', due: null, progress: 0, created_at: '2026-12-01T09:00:00Z', tasks: 0, done: 0, ...over,
})
const today = new Date(2026, 11, 16, 12) // 16 December

describe('goals', () => {
  it('progress from tasks, else by hand', () => {
    expect(pct(goal({ tasks: 4, done: 1 }))).toBe(25)
    expect(pct(goal({ progress: 60 }))).toBe(60)
  })
  it('days left', () => {
    expect(daysLeft(goal({ due: '2026-12-31' }), today)).toBe(15)
    expect(daysLeft(goal({ due: '2026-12-10' }), today)).toBe(-6)
    expect(daysLeft(goal({}), today)).toBeNull()
  })
  it('status', () => {
    // From 1 to 31 December, half the time gone on the 16th.
    expect(status(goal({ due: '2026-12-31', tasks: 10, done: 5 }), today)).toBe('on-track')
    expect(status(goal({ due: '2026-12-31', tasks: 10, done: 2 }), today)).toBe('behind')
    expect(status(goal({ due: '2026-12-10', tasks: 10, done: 9 }), today)).toBe('overdue')
    expect(status(goal({ due: '2026-12-10', tasks: 2, done: 2 }), today)).toBe('done')
    expect(status(goal({ achieved_at: '2026-12-12T10:00:00Z' }), today)).toBe('achieved')
    expect(status(goal({}), today)).toBe('open')
  })
})
