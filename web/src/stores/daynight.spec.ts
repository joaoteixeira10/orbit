import { describe, expect, it } from 'vitest'
import { isNight } from './prefs'

const at = (h: number, m = 0) => new Date(2026, 10, 18, h, m)

describe('day and night theme', () => {
  it('is light from day to night, dark from night past midnight to day', () => {
    expect(isNight(at(6, 59), '07:00', '20:00')).toBe(true)
    expect(isNight(at(7, 0), '07:00', '20:00')).toBe(false)
    expect(isNight(at(19, 59), '07:00', '20:00')).toBe(false)
    expect(isNight(at(20, 0), '07:00', '20:00')).toBe(true)
    expect(isNight(at(0, 30), '07:00', '20:00')).toBe(true)
  })
  it('works the other way round too (dark in the afternoon, light at night)', () => {
    expect(isNight(at(15), '22:00', '13:00')).toBe(true)
    expect(isNight(at(23), '22:00', '13:00')).toBe(false)
  })
  it('never dark when both times are the same', () => {
    expect(isNight(at(3), '08:00', '08:00')).toBe(false)
  })
})
