import { describe, expect, it } from 'vitest'
import { best, counts, dueToday, rate, streak, type Habit } from './habits'

const habit = (over: Partial<Habit>): Habit => ({
  id: 'h', workspace_id: 'w', title: 'Read', icon: '', color: '', schedule: 'daily', days: '1111111', target: 1, remind_at: '', checks: [], ...over,
})
// Wednesday 9 December 2026.
const wed = new Date(2026, 11, 9)

describe('habits', () => {
  it('counts the days it asks for', () => {
    const weekdays = habit({ days: '1111100' })
    expect(counts(weekdays, new Date(2026, 11, 12))).toBe(false) // Saturday
    expect(counts(weekdays, wed)).toBe(true)
  })

  it('a daily streak: today not done yet doesn’t break it; a missed day does', () => {
    const h = habit({ checks: ['2026-12-05', '2026-12-07', '2026-12-08'] })
    expect(streak(h, wed)).toBe(2) // Mon, Tue (Sun 6 missed)
    expect(streak({ ...h, checks: [...h.checks, '2026-12-09'] }, wed)).toBe(3)
    expect(dueToday(h, wed)).toBe(true)
  })

  it('skips the days it doesn’t ask for', () => {
    // Weekdays: Fri 4, Mon 7, Tue 8 — the weekend in between doesn't count.
    const h = habit({ days: '1111100', checks: ['2026-12-04', '2026-12-07', '2026-12-08'] })
    expect(streak(h, wed)).toBe(3)
    expect(best(h, wed)).toBe(3)
  })

  it('a weekly streak counts weeks that reached the target', () => {
    // Twice a week: the week of 30 Nov (2), of 23 Nov (2), of 16 Nov (1).
    const h = habit({ schedule: 'weekly', target: 2, checks: ['2026-11-17', '2026-11-23', '2026-11-26', '2026-11-30', '2026-12-02'] })
    expect(streak(h, wed)).toBe(2) // this week (Dec 7) has none yet
    expect(dueToday(h, wed)).toBe(true)
    expect(best(h, wed)).toBe(2)
  })

  it('the rate over the last 30 days', () => {
    const all = Array.from({ length: 30 }, (_, i) => {
      const d = new Date(2026, 11, 9 - i)
      return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
    })
    expect(rate(habit({ checks: all.sort() }), wed)).toBe(1)
    expect(rate(habit({ checks: [] }), wed)).toBe(0)
  })
})
