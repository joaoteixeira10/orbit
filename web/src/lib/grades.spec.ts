import { describe, expect, it } from 'vitest'
import { average, needed, fmtMark, passes } from './grades'

const g = [
  { score: 12, out_of: 20, weight: 1 }, // 0.6
  { score: 80, out_of: 100, weight: 3 }, // 0.8
  { score: 5, out_of: 0, weight: 1 }, // ignored
]

describe('grades', () => {
  it('averages marks on any scale by their weights', () => {
    expect(average([])).toBeNull()
    expect(average(g)).toBeCloseTo(0.75)
  })
  it('works out what the next mark must be', () => {
    expect(needed(g, 0.75, 4)).toBeCloseTo(0.75)
    expect(needed(g, 0.5, 4)).toBeCloseTo(0.25)
    expect(needed(g, 0.95, 1)).toBeGreaterThan(1)
    expect(needed([], 0.7, 2)).toBeCloseTo(0.7)
  })
  it('formats marks', () => {
    expect(fmtMark(14.46, 'en')).toBe('14.5')
    expect(fmtMark(15, 'en')).toBe('15')
    expect(fmtMark(9.5, 'pt-PT')).toBe('9,5')
    expect(passes(0.5)).toBe(true)
    expect(passes(0.47)).toBe(false)
  })
})
