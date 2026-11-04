import { describe, expect, it } from 'vitest'
import { bioKind, lockAfter, lockChoices } from './biometric'

describe('the lock', () => {
  it('follows the device’s choice within the owner’s rule', () => {
    expect(lockAfter(-1, 0, false)).toBe(-1) // never, no rule
    expect(lockAfter(15, 0, false)).toBe(15)
    expect(lockAfter(60, 15, false)).toBe(15) // capped
    expect(lockAfter(-1, 15, false)).toBe(-1) // not using it: the cap doesn't force it
    expect(lockAfter(-1, 15, true)).toBe(15) // required: at the cap
    expect(lockAfter(-1, 0, true)).toBe(5) // required, no cap: 5 minutes
    expect(lockAfter(0, 15, true)).toBe(0) // "when I come back" is within any cap
  })
  it('offers only the choices the rule leaves', () => {
    expect(lockChoices(0, false)).toEqual([-1, 0, 1, 5, 15, 60])
    expect(lockChoices(15, false)).toEqual([-1, 0, 1, 5, 15])
    expect(lockChoices(5, true)).toEqual([0, 1, 5])
  })
})

describe('what the device calls it', () => {
  it.each([
    ['Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)', 5, 'faceId'],
    ['Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)', 5, 'faceId'], // an iPad asking for the desktop site
    ['Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)', 0, 'touchId'],
    ['Mozilla/5.0 (Windows NT 10.0; Win64; x64)', 0, 'hello'],
    ['Mozilla/5.0 (Linux; Android 14; Pixel 8)', 5, 'fingerprint'],
    ['Mozilla/5.0 (X11; Linux x86_64)', 0, 'device'],
  ])('%s', (ua, touch, want) => {
    expect(bioKind(ua, touch)).toBe(want)
  })
})
