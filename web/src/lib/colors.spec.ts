import { describe, expect, it } from 'vitest'
import { hexOf, hexToHsv, hsvToHex, lightness } from './colors'

describe('colors', () => {
  it('reads typed colours', () => {
    expect(hexOf('#ABC')).toBe('#aabbcc')
    expect(hexOf('ff9500')).toBe('#ff9500')
    expect(hexOf('red')).toBe('')
  })
  it('goes to HSV and back', () => {
    for (const c of ['#ff3b30', '#34c759', '#007aff', '#000000', '#ffffff', '#8e8e93', '#af52de']) expect(hsvToHex(hexToHsv(c))).toBe(c)
    expect(hexToHsv('#ff0000')).toEqual({ h: 0, s: 1, v: 1 })
    expect(hsvToHex({ h: 120, s: 1, v: 1 })).toBe('#00ff00')
  })
  it('knows light from dark', () => {
    expect(lightness('#ffffff')).toBe(1)
    expect(lightness('#000000')).toBe(0)
  })
})
