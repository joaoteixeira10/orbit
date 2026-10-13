import { describe, it, expect } from 'vitest'
import { daysUntil, expiryTone } from './format'

const inDays = (n: number) => new Date(Date.now() + n * 86_400_000).toISOString()

describe('daysUntil', () => {
  it('is positive for future dates and negative for past dates', () => {
    expect(daysUntil(inDays(10))).toBeGreaterThanOrEqual(9)
    expect(daysUntil(inDays(-5))).toBeLessThan(0)
  })
})

describe('expiryTone', () => {
  it('classifies remaining lifetime into ok / warn / crit buckets', () => {
    expect(expiryTone(inDays(3))).toBe('crit') // < 7 days
    expect(expiryTone(inDays(15))).toBe('warn') // < 30 days
    expect(expiryTone(inDays(100))).toBe('ok') // >= 30 days
    expect(expiryTone(inDays(-1))).toBe('crit') // already expired
  })
})
