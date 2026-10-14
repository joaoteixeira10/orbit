import { describe, it, expect } from 'vitest'
import { dayNum, numDate, weekday, weekStartOf, spanOf, rangeFor, dragSpan, patchFor, monthCells, quarterCells, weekCells, quarterOf, weekendStops } from './timeline'

describe('day numbers', () => {
  it('round-trips dates, across DST and leap days', () => {
    for (const s of ['1970-01-01', '2024-02-29', '2026-03-29', '2026-10-25', '2026-12-31']) expect(numDate(dayNum(s))).toBe(s)
    expect(dayNum('1970-01-02')).toBe(1)
    expect(dayNum('2026-10-26') - dayNum('2026-10-24')).toBe(2)
  })
  it('knows the weekday and the start of the week', () => {
    expect(weekday(dayNum('2026-10-14'))).toBe(3) // a Wednesday
    expect(numDate(weekStartOf(dayNum('2026-10-14'), 1))).toBe('2026-10-12')
    expect(numDate(weekStartOf(dayNum('2026-10-14'), 0))).toBe('2026-10-11')
    expect(numDate(weekStartOf(dayNum('2026-10-12'), 1))).toBe('2026-10-12')
  })
})

describe('bars', () => {
  it('spans start to due, or one day for a single date', () => {
    expect(spanOf({ start: '2026-10-10', due: '2026-10-14' })).toEqual({ from: dayNum('2026-10-10'), to: dayNum('2026-10-14') })
    expect(spanOf({ start: null, due: '2026-10-14' })).toEqual({ from: dayNum('2026-10-14'), to: dayNum('2026-10-14') })
    expect(spanOf({ start: '2026-10-14', due: null })).toEqual({ from: dayNum('2026-10-14'), to: dayNum('2026-10-14') })
    expect(spanOf({ start: null, due: null })).toBeNull()
  })
  it('moves and resizes without crossing edges', () => {
    const s = { from: 10, to: 14 }
    expect(dragSpan(s, 'move', 3)).toEqual({ from: 13, to: 17 })
    expect(dragSpan(s, 'start', -2)).toEqual({ from: 8, to: 14 })
    expect(dragSpan(s, 'start', 9)).toEqual({ from: 14, to: 14 })
    expect(dragSpan(s, 'end', -9)).toEqual({ from: 10, to: 10 })
  })
  it('keeps a due-only task due-only when it stays one day', () => {
    const d = dayNum('2026-10-20')
    expect(patchFor({ start: null, due: '2026-10-14' }, { from: d, to: d })).toEqual({ due: '2026-10-20' })
    expect(patchFor({ start: '2026-10-14', due: null }, { from: d, to: d })).toEqual({ start: '2026-10-20' })
    expect(patchFor({ start: null, due: '2026-10-14' }, { from: d - 2, to: d })).toEqual({ start: '2026-10-18', due: '2026-10-20' })
    expect(patchFor({ start: null, due: null }, { from: d, to: d })).toEqual({ start: '2026-10-20', due: '2026-10-20' })
  })
})

describe('range', () => {
  it('covers today and every bar, in whole weeks', () => {
    const today = dayNum('2026-10-14')
    const r = rangeFor(today, [{ from: today - 200, to: today - 199 }], 1)
    expect(r.from).toBeLessThanOrEqual(today - 214)
    expect(weekday(r.from)).toBe(1)
    expect(weekday(r.to)).toBe(0)
    expect(r.to).toBeGreaterThanOrEqual(today + 92)
  })
})

describe('header cells', () => {
  const r = { from: dayNum('2026-09-28'), to: dayNum('2027-01-10') }
  it('splits at months, quarters and weeks', () => {
    const m = monthCells(r)
    expect(m.map((c) => numDate(c.at))).toEqual(['2026-09-28', '2026-10-01', '2026-11-01', '2026-12-01', '2027-01-01'])
    expect(m.reduce((n, c) => n + c.days, 0)).toBe(r.to - r.from + 1)
    expect(m[1].days).toBe(31)
    expect(quarterCells(r).map((c) => numDate(c.at))).toEqual(['2026-09-28', '2026-10-01', '2027-01-01'])
    expect(quarterOf(dayNum('2026-11-05'))).toBe(4)
    const w = weekCells(r, 1)
    expect(w.every((c) => c.days === 7)).toBe(true)
  })
  it('shades weekends', () => {
    // Monday-first: Saturday and Sunday are days 5 and 6.
    expect(weekendStops(dayNum('2026-10-12'), 10, 'red')).toBe('linear-gradient(to right, transparent 50px, red 50px, red 60px, transparent 60px, transparent 60px, red 60px, red 70px, transparent 70px)')
  })
})
