import { describe, it, expect } from 'vitest'
import { Editor } from '@tiptap/core'
import { pageExtensions } from '@/components/editor/extensions'
import { normalizeMarkdown, renderMarkdown, toggleTask, fromFile, pageFile, textToMarkdown } from './markdown'

describe('reading a page', () => {
  it('renders GitHub Markdown, alerts and footnotes', () => {
    const html = renderMarkdown('# Hi\n\n> [!TIP]\n> Drink water.\n\nA note[^1].\n\n[^1]: Here.\n\n| A |\n| - |\n| 1 |')
    expect(html).toContain('<h1')
    expect(html).toContain('md-alert-tip')
    expect(html).toContain('Drink water.')
    expect(html).not.toContain('[!TIP]')
    expect(html).toContain('<table')
    expect(html).toMatch(/footnote/i)
  })
  it('numbers the task boxes and sanitises', () => {
    const html = renderMarkdown('- [ ] a\n- [x] b\n\n<script>alert(1)</script><img src=x onerror=alert(1)><a href="javascript:alert(1)">x</a>')
    expect(html).toContain('data-task="0"')
    expect(html).toMatch(/data-task="1"[^>]*checked|checked[^>]*data-task="1"/)
    expect(html).not.toContain('<script')
    expect(html).not.toContain('onerror')
    expect(html).not.toContain('javascript:')
  })
  it('opens links in a new tab', () => {
    expect(renderMarkdown('[x](https://example.org)')).toContain('target="_blank"')
  })
})

describe('changing a page', () => {
  it('ticks the right task, skipping code', () => {
    const md = '- [ ] a\n\n```\n- [ ] not a task\n```\n\n1. [ ] b\n- [x] c'
    expect(toggleTask(md, 1)).toBe('- [ ] a\n\n```\n- [ ] not a task\n```\n\n1. [x] b\n- [x] c')
    expect(toggleTask(md, 2)).toContain('- [ ] c')
    expect(toggleTask(md, 9)).toBe(md)
  })
  it('normalises the same way whatever saved it', () => {
    expect(normalizeMarkdown('\n\n# A   \n\n\n\ntext  \nmore\n\n```\n\n\ncode\n```\n\n\n')).toBe('# A\n\ntext  \nmore\n\n```\n\n\ncode\n```')
  })
})

describe('files', () => {
  it('reads a heading as the title, else the file name', () => {
    expect(fromFile('# Trip\n\nPack bags\n', 'x.md')).toEqual({ title: 'Trip', content: 'Pack bags' })
    expect(fromFile('Just text', 'Ideas.md')).toEqual({ title: 'Ideas', content: 'Just text' })
    expect(pageFile('Trip', 'Pack bags')).toBe('# Trip\n\nPack bags\n')
  })
  it('writes a note as Markdown that reads back as the same text', () => {
    const text = '- not a list\n2 * 3 = 6 and *stars*\n# not a heading\n<b>tag</b> and [x](y)'
    const ed = new Editor({ extensions: pageExtensions(), content: textToMarkdown(text), contentType: 'markdown' })
    expect(ed.getText({ blockSeparator: '\n' })).toBe(text)
    ed.destroy()
  })
})

describe('rendering again and again', () => {
  it('keeps footnotes working across pages, alerts included', () => {
    for (let i = 0; i < 3; i++) {
      expect(renderMarkdown('a[^1]\n\n> [!TIP]\n> b[^2]\n\n[^1]: one\n[^2]: two')).toContain('footnote-ref')
      expect(renderMarkdown('> [!NOTE]\n> plain')).toContain('md-alert-note')
    }
  })
})
