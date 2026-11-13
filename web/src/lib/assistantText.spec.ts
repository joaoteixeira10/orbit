import { describe, expect, it } from 'vitest'
import { tidy } from './assistantText'

describe('tidy', () => {
  it('shows an unclosed link as its words', () => {
    expect(tidy('to your [Shopping](/w/01a1371f-776a/tasks/01a1371f-776b-71eb-bead-d0')).toBe('to your Shopping')
    expect(tidy('in [Home](/w/abc\nnext')).toBe('in Home\nnext')
  })
  it('leaves whole links and other text alone', () => {
    const ok = 'see [Shopping](/w/abc/tasks/def) and [docs](https://x.test/a)'
    expect(tidy(ok)).toBe(ok)
    expect(tidy('[a] (b) [c](')).toBe('[a] (b) [c](')
  })
})
