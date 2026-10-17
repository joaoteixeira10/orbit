import { describe, it, expect } from 'vitest'
import { renderMarkdown } from './markdown'
import { mediaPrefs, remoteHost, reveal, safeImageSrc, showsAtOnce } from './media'

describe('images from other sites', () => {
  it('wait behind a placeholder in the reading view; Orbit\'s own show', () => {
    const html = renderMarkdown('![chart](https://cdn.example.org/a.png) ![mine](/api/v1/uploads/1/a.png)')
    expect(html).toContain('class="md-remote-img"')
    expect(html).toContain('data-src="https://cdn.example.org/a.png"')
    expect(html).not.toContain('<img src="https://cdn.example.org')
    expect(html).toContain('data-upload="/api/v1/uploads/1/a.png"') // its src comes with the token
  })

  it('show at once when the reader chose so, or after one from that site', () => {
    mediaPrefs.remoteImages = 'ask'
    expect(showsAtOnce('https://cdn.example.org/a.png')).toBe(false)
    reveal('https://cdn.example.org/x.png')
    expect(showsAtOnce('https://cdn.example.org/a.png')).toBe(true)
    expect(showsAtOnce('https://other.example.net/a.png')).toBe(false)
    mediaPrefs.remoteImages = 'always'
    expect(showsAtOnce('https://other.example.net/a.png')).toBe(true)
    mediaPrefs.remoteImages = 'ask'
  })

  it('never put a script address in an image', () => {
    expect(safeImageSrc('javascript:alert(1)')).toBe('')
    expect(remoteHost('javascript:alert(1)')).toBe('')
    expect(safeImageSrc('data:image/png;base64,AAAA')).toContain('data:image/png')
    expect(safeImageSrc('data:text/html,<script>')).toBe('')
  })
})

describe('images in the visual editor', () => {
  it('a page with an image loads as a valid document and keeps its Markdown', async () => {
    const { Editor } = await import('@tiptap/core')
    const { pageExtensions } = await import('@/components/editor/extensions')
    const md = 'A chart from another site:\n\n![Sales chart](https://images.example.org/sales.png)\n\nAnd text after it.'
    const ed = new Editor({ extensions: pageExtensions(), content: md, contentType: 'markdown' })
    expect(() => ed.state.doc.check()).not.toThrow()
    expect(ed.getMarkdown().trim()).toBe(md)
    ed.destroy()
  })
})
