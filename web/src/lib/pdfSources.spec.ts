import { describe, expect, it } from 'vitest'
import { sizedSvg } from './pdfSources'

describe('pdf sources', () => {
  it('gives an SVG its own size, once', () => {
    expect(sizedSvg('<svg width="100%" xmlns="x" height="20" viewBox="0 0 40 20"><g/></svg>')).toBe('<svg width="40" height="20" xmlns="x" viewBox="0 0 40 20"><g/></svg>')
  })
})
