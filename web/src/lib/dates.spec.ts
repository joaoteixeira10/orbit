import { describe, expect, it } from 'vitest'
import { fromZoneInput, repeatLabel, sameZoneTime, toZoneInput, zoneName, zoneOffset } from './dates'

describe('time zones', () => {
  it('reads a wall clock in a zone as an instant, and back', () => {
    // New York is UTC-4 in summer, UTC-5 in winter.
    expect(fromZoneInput('2026-07-01T10:00', 'America/New_York').toISOString()).toBe('2026-07-01T14:00:00.000Z')
    expect(fromZoneInput('2026-12-01T10:00', 'America/New_York').toISOString()).toBe('2026-12-01T15:00:00.000Z')
    expect(fromZoneInput('2026-07-01T10:00', 'Asia/Tokyo').toISOString()).toBe('2026-07-01T01:00:00.000Z')
    expect(toZoneInput(new Date('2026-07-01T14:00:00Z'), 'America/New_York')).toBe('2026-07-01T10:00')
    expect(toZoneInput(new Date('2026-12-31T23:30:00Z'), 'Europe/Lisbon')).toBe('2026-12-31T23:30')
    expect(toZoneInput(new Date('2026-12-31T23:30:00Z'), 'Europe/Berlin')).toBe('2027-01-01T00:30')
  })

  it('handles the daylight-saving changes', () => {
    // Lisbon springs forward at 01:00 on 2026-03-29: 01:30 doesn't exist.
    expect(fromZoneInput('2026-03-29T00:30', 'Europe/Lisbon').toISOString()).toBe('2026-03-29T00:30:00.000Z')
    expect(fromZoneInput('2026-03-29T02:30', 'Europe/Lisbon').toISOString()).toBe('2026-03-29T01:30:00.000Z')
    expect(zoneOffset(Date.UTC(2026, 0, 15), 'Europe/Lisbon')).toBe(0)
    expect(zoneOffset(Date.UTC(2026, 6, 15), 'Europe/Lisbon')).toBe(3600000)
  })

  it('compares zones and names them', () => {
    const summer = new Date('2026-07-01T12:00:00Z')
    expect(sameZoneTime('Europe/Lisbon', 'Europe/London', summer)).toBe(true)
    expect(sameZoneTime('Europe/Lisbon', 'Europe/Madrid', summer)).toBe(false)
    expect(zoneName('America/New_York')).toBe('New York')
    expect(zoneName('America/New_York', summer)).toBe('New York (GMT-4)')
  })
})

describe('repeatLabel', () => {
  it('names presets and simple custom rules in words', () => {
    expect(repeatLabel('')).toBe('')
    expect(repeatLabel('FREQ=WEEKLY;INTERVAL=2')).toBe('Every 2 weeks')
    expect(repeatLabel('FREQ=WEEKLY;INTERVAL=2;BYDAY=TU')).toBe('Every other Tuesday')
    expect(repeatLabel('FREQ=WEEKLY;BYDAY=MO,FR')).toBe('Every Monday and Friday')
    expect(repeatLabel('FREQ=DAILY;INTERVAL=3')).toBe('Every 3 days')
    expect(repeatLabel('FREQ=MONTHLY;INTERVAL=2')).toBe('Every other month')
    expect(repeatLabel('FREQ=MONTHLY;BYMONTHDAY=15')).toBe('Custom')
  })
})
