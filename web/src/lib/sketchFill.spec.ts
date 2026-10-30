import { describe, expect, it } from 'vitest'
import { floodFill } from './sketch'

// A 10×10 picture with a square outline from (2,2) to (7,7).
function square(gapAt?: [number, number]) {
  const w = 10
  const a = new Uint8Array(w * w)
  for (let i = 2; i <= 7; i++) {
    for (const [x, y] of [[i, 2], [i, 7], [2, i], [7, i]]) a[y * w + x] = 255
  }
  if (gapAt) a[gapAt[1] * w + gapAt[0]] = 0
  return a
}
const filled = (m: Uint8Array, x: number, y: number) => m[y * 10 + x] === 1

describe('sketch bucket', () => {
  it('fills inside a closed shape, and under its line, not outside', () => {
    const r = floodFill(square(), 10, 10, 4, 4, 0)!
    expect(filled(r.mask, 4, 4)).toBe(true)
    expect(filled(r.mask, 2, 4)).toBe(true) // under the line (a pixel more)
    expect(filled(r.mask, 0, 0)).toBe(false)
    expect(r.box).toEqual({ x0: 2, y0: 2, x1: 7, y1: 7 })
  })
  it('a small gap in the line still closes it', () => {
    const open = floodFill(square([7, 4]), 10, 10, 4, 4, 0)!
    expect(filled(open.mask, 0, 0)).toBe(true) // with no gap allowance, it leaks
    const closed = floodFill(square([7, 4]), 10, 10, 4, 4, 1)!
    expect(filled(closed.mask, 9, 9)).toBe(false)
  })
  it('nothing when started on a line', () => {
    expect(floodFill(square(), 10, 10, 2, 4, 0)).toBeNull()
  })
})
