import { describe, expect, it } from 'vitest'
import { estimateText, freeTime, mergeSpans, planWeek, workingHours } from './weekPlan'

const at = (d: number, h: number, m = 0) => new Date(2026, 10, d, h, m).getTime() // November 2026

describe('weekPlan', () => {
  it('finds the free time in working hours', () => {
    const work = workingHours(new Date(2026, 10, 2), 7, [1, 2, 3, 4, 5], '09:00', '17:00') // Mon 2 … Sun 8
    expect(work).toHaveLength(5)
    expect(work[0]).toEqual({ start: at(2, 9), end: at(2, 17) })
    const free = freeTime(work.slice(0, 1), [{ start: at(2, 10), end: at(2, 11) }, { start: at(2, 10, 30), end: at(2, 12) }, { start: at(2, 16), end: at(2, 18) }], at(2, 9, 30))
    expect(free).toEqual([
      { start: at(2, 9, 30), end: at(2, 10) },
      { start: at(2, 12), end: at(2, 16) },
    ])
    expect(mergeSpans([{ start: 5, end: 9 }, { start: 1, end: 3 }, { start: 2, end: 6 }])).toEqual([{ start: 1, end: 9 }])
  })

  it('fits the earliest due first, splits what needs it, and says what doesn’t fit', () => {
    const free = [
      { start: at(2, 9), end: at(2, 10) },
      { start: at(2, 11), end: at(2, 12, 30) },
      { start: at(3, 9), end: at(3, 12) },
    ]
    const plan = planWeek(
      [
        { id: 'later', minutes: 120, priority: 0 }, // no due date: after the others
        { id: 'soon', minutes: 90, due: at(2, 23, 59), priority: 1 }, // due Monday: split over Monday's gaps
        { id: 'urgent', minutes: 30, due: at(2, 23, 59), priority: 3 },
        { id: 'huge', minutes: 600, priority: 0 },
      ],
      free,
    )
    expect(plan.blocks.map((b) => [b.taskId, new Date(b.start).getHours() + ':' + new Date(b.start).getMinutes(), (b.end - b.start) / 60_000])).toEqual([
      ['urgent', '9:0', 30],
      ['soon', '9:30', 30],
      ['soon', '11:0', 60],
      ['later', '12:0', 30], // the half hour left on Monday, then Tuesday
      ['later', '9:0', 90],
    ])
    // Too big for what's left (210 of its 600 minutes fit): not planned at all.
    expect(plan.unplaced).toEqual([{ id: 'huge', minutes: 600 - 210 }])
    // Nothing is planned after its due day.
    const late = planWeek([{ id: 'x', minutes: 60, due: at(2, 9, 30), priority: 0 }], free)
    expect(late.blocks).toEqual([{ taskId: 'x', start: at(2, 9), end: at(2, 9, 30) }])
    expect(late.unplaced).toEqual([{ id: 'x', minutes: 30 }])
    expect(planWeek([{ id: 'y', minutes: 45, priority: 0 }], [{ start: at(2, 9, 5), end: at(2, 11) }]).blocks[0].start).toBe(at(2, 9, 15))
  })

  it('reads estimates', () => {
    expect(estimateText(45)).toBe('45m')
    expect(estimateText(90)).toBe('1h 30m')
    expect(estimateText(120)).toBe('2h')
    expect(estimateText(0)).toBe('')
  })
})
