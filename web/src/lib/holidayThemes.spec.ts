import { describe, expect, it } from 'vitest'
import { HOLIDAYS, countdown, cssVars, easterSunday, resolveHoliday } from './holidayThemes'

const at = (y: number, m: number, d: number, h = 12) => new Date(y, m - 1, d, h)
const auto = (y: number, m: number, d: number) => resolveHoliday('auto', at(y, m, d))?.id ?? null
const byId = (id: string) => HOLIDAYS.find((h) => h.id === id)!

describe('holiday themes', () => {
  it('auto picks the holiday whose window holds the day, and none outside', () => {
    expect(auto(2026, 10, 14)).toBe(null)
    expect(auto(2026, 10, 15)).toBe('halloween')
    expect(auto(2026, 11, 1)).toBe('halloween')
    expect(auto(2026, 11, 2)).toBe(null)
    expect(auto(2026, 11, 30)).toBe(null)
    expect(auto(2026, 12, 1)).toBe('christmas')
    expect(auto(2026, 12, 26)).toBe('christmas')
    expect(auto(2026, 12, 27)).toBe('nye')
    expect(auto(2027, 1, 1)).toBe('nye') // the window that began the year before
    expect(auto(2027, 1, 2)).toBe(null)
    expect(auto(2026, 5, 31)).toBe(null)
    expect(auto(2026, 6, 1)).toBe('santos')
    expect(auto(2026, 6, 30)).toBe('santos')
    expect(auto(2026, 7, 1)).toBe(null)
  })

  it('Easter moves with the year: Palm Sunday to Easter Monday', () => {
    expect(easterSunday(2027)).toEqual(new Date(2027, 2, 28))
    expect(easterSunday(2028)).toEqual(new Date(2028, 3, 16))
    expect(auto(2027, 3, 20)).toBe(null)
    expect(auto(2027, 3, 21)).toBe('easter')
    expect(auto(2027, 3, 29)).toBe('easter')
    expect(auto(2027, 3, 30)).toBe(null)
    expect(auto(2028, 4, 16)).toBe('easter')
  })

  it('Lunar New Year: a week before to fifteen days after', () => {
    expect(auto(2027, 1, 29)).toBe(null)
    expect(auto(2027, 1, 30)).toBe('lunar')
    expect(auto(2027, 2, 6)).toBe('lunar')
    expect(auto(2027, 2, 21)).toBe('lunar')
    expect(auto(2027, 2, 22)).toBe(null)
    expect(auto(2031, 2, 1)).toBe(null) // no date known: no window
  })

  it('off is none; a holiday picked shows whatever the date', () => {
    expect(resolveHoliday('off', at(2026, 12, 25))).toBe(null)
    expect(resolveHoliday('halloween', at(2026, 3, 3))?.id).toBe('halloween')
  })

  it('counts down in days to the peak, the day itself, and NYE in hours on Dec 31', () => {
    expect(countdown(byId('christmas'), at(2026, 12, 23))).toEqual({ unit: 'days', n: 2, year: 2026 })
    expect(countdown(byId('christmas'), at(2026, 12, 24))).toEqual({ unit: 'days', n: 1, year: 2026 })
    expect(countdown(byId('christmas'), at(2026, 12, 25))).toEqual({ unit: 'today' })
    expect(countdown(byId('christmas'), at(2026, 11, 4))).toEqual({ unit: 'days', n: 51, year: 2026 })
    expect(countdown(byId('nye'), at(2026, 12, 28, 0))).toEqual({ unit: 'days', n: 4, year: 2027 })
    expect(countdown(byId('nye'), at(2026, 12, 31, 18))).toEqual({ unit: 'hours', n: 6, year: 2027 })
    expect(countdown(byId('nye'), at(2027, 1, 1))).toEqual({ unit: 'today' })
    expect(countdown(byId('santos'), at(2026, 6, 13))).toEqual({ unit: 'today' })
  })

  it('every holiday has the same tokens in light and dark', () => {
    const keys = Object.keys(cssVars(byId('christmas'), 'light')).filter((k) => !['--h-flake', '--h-cap', '--h-capLine', '--h-bulbCap'].includes(k))
    for (const h of HOLIDAYS) {
      for (const mode of ['light', 'dark'] as const) {
        const v = cssVars(h, mode)
        for (const k of keys) expect(v[k], `${h.id} ${mode} ${k}`).toBeTruthy()
      }
    }
  })
})
