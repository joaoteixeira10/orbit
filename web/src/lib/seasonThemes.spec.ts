import { describe, expect, it } from 'vitest'
import { SEASONS, resolveSeason } from './seasonThemes'
import { cssVars } from './holidayThemes'

const at = (y: number, m: number, d: number, h = 12) => new Date(y, m - 1, d, h)
const auto = (y: number, m: number, d: number) => resolveSeason('auto', at(y, m, d))?.id ?? null

describe('season themes', () => {
  it('auto picks the season the day is in, every day of the year', () => {
    expect(auto(2027, 3, 19)).toBe('winter')
    expect(auto(2027, 3, 20)).toBe('spring')
    expect(auto(2027, 6, 20)).toBe('spring')
    expect(auto(2027, 6, 21)).toBe('summer')
    expect(auto(2027, 9, 22)).toBe('summer')
    expect(auto(2027, 9, 23)).toBe('autumn')
    expect(auto(2027, 12, 20)).toBe('autumn')
    expect(auto(2027, 12, 21)).toBe('winter')
    expect(auto(2028, 1, 15)).toBe('winter') // the winter that began the year before
    expect(auto(2028, 2, 29)).toBe('winter') // a leap day
    for (let day = 0; day < 366; day++) expect(auto(2028, 1, 1 + day)).not.toBe(null)
  })

  it('off shows none; a season shows whatever the date', () => {
    expect(resolveSeason('off', at(2027, 7, 1))).toBe(null)
    expect(resolveSeason('winter', at(2027, 7, 1))?.id).toBe('winter')
  })

  it('each season has every colour the holiday CSS reads, in light and dark', () => {
    const keys = ['bg', 'side', 'card', 'cardLine', 'rowLine', 'hover', 'field', 'text', 'muted', 'faint', 'primary', 'secondary', 'icon', 'tag', 'issue', 'late', 'prio1', 'prio2', 'prio3', 'check', 'switchOff', 'glow', 'chipBg', 'chipText', 'chipLine', 'thread', 'wire']
    for (const s of SEASONS)
      for (const mode of ['light', 'dark'] as const) {
        const vars = cssVars(s, mode)
        for (const k of keys) expect(vars[`--h-${k}`], `${s.id} ${mode} ${k}`).toBeTruthy()
      }
    // Winter's snow reads its flake colour.
    expect(cssVars(SEASONS.find((s) => s.id === 'winter')!, 'dark')['--h-flake']).toBe('#ffffff')
  })
})
