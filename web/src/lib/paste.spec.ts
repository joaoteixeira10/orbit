import { describe, expect, it } from 'vitest'
import { cleanPastedHTML } from './paste'

describe('pasted HTML', () => {
  it("unwraps Google Docs' bold wrapper, keeps real bold", () => {
    const gd = '<meta charset="utf-8"><b style="font-weight:normal;" id="docs-internal-guid-abc"><p><span style="font-weight:700">Bold</span> plain</p></b>'
    expect(cleanPastedHTML(gd)).toBe('<p><span style="font-weight:700">Bold</span> plain</p>')
  })
  it("drops Office's leftovers", () => {
    expect(cleanPastedHTML('<!--StartFragment--><p>Hi<o:p></o:p></p><!--EndFragment-->')).toBe('<p>Hi</p>')
  })
})
