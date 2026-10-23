import { describe, it, expect } from 'vitest'
import { builtInTemplates, fill, isoWeek } from './pageTemplates'

// Friday 23 October 2026, 14:05.
const now = new Date(2026, 9, 23, 14, 5)

describe('page templates', () => {
  it('fills the placeholders, leaves the unknown ones', () => {
    const c = { now, locale: 'en-GB', me: 'Ana', workspace: 'Team' }
    expect(fill('{{weekday}}, {{date}} at {{time}} — week {{week}} of {{year}}', c)).toBe('Friday, 23 October 2026 at 14:05 — week 43 of 2026')
    expect(fill('From {{monday}}; {{ me }} in {{workspace}}; {{iso}}; {{nope}}', c)).toBe('From 19 October 2026; Ana in Team; 2026-10-23; {{nope}}')
    expect(fill('{{date}}', { now, locale: 'pt-PT' })).toBe('23 de outubro de 2026')
  })
  it('numbers weeks the ISO way', () => {
    expect(isoWeek(new Date(2026, 0, 1))).toBe(1) // a Thursday
    expect(isoWeek(new Date(2027, 0, 1))).toBe(53) // a Friday: 2026's last week
    expect(isoWeek(new Date(2026, 11, 28))).toBe(53)
  })
  it('has the three built in, in both languages', () => {
    expect(builtInTemplates('en').map((x) => x.key)).toEqual(['meeting', 'class', 'weekly'])
    expect(builtInTemplates('pt')[2].title).toBe('Revisão da semana {{week}}')
    expect(builtInTemplates('fr')[0].title).toBe('Meeting — {{date}}')
  })
})
