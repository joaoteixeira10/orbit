import { describe, it, expect } from 'vitest'
import { Editor } from '@tiptap/core'
import { pageExtensions } from '@/components/editor/extensions'
import { normalizeMarkdown, renderMarkdown, toggleTask, fromFile, pageFile, textToMarkdown, firstLine } from './markdown'

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
    expect(fromFile('# Trip\n\nPack bags\n', 'x.md')).toEqual({ title: 'Trip', content: 'Pack bags', tags: [], fields: [] })
    expect(fromFile('Just text', 'Ideas.md')).toEqual({ title: 'Ideas', content: 'Just text', tags: [], fields: [] })
    expect(pageFile('Trip', 'Pack bags')).toBe('# Trip\n\nPack bags\n')
    // Tags and properties: a YAML header, and back.
    const file = pageFile('Trip', 'Pack bags', { tags: ['travel', 'Q3 plans'], fields: [{ key: 'Due', type: 'date', value: '2026-11-04' }, { key: 'Budget', type: 'number', value: 1200 }, { key: 'Booked', type: 'checkbox', value: true }, { key: 'Note: first', type: 'text', value: 'window seat' }] })
    expect(file).toBe('---\ntags: ["travel","Q3 plans"]\nDue: "2026-11-04"\nBudget: 1200\nBooked: true\n"Note: first": "window seat"\n---\n# Trip\n\nPack bags\n')
    expect(fromFile(file, 'x.md')).toEqual({ title: 'Trip', content: 'Pack bags', tags: ['travel', 'Q3 plans'], fields: [{ key: 'Due', type: 'date', value: '2026-11-04' }, { key: 'Budget', type: 'number', value: 1200 }, { key: 'Booked', type: 'checkbox', value: true }, { key: 'Note: first', type: 'text', value: 'window seat' }] })
    // Obsidian's list of tags.
    expect(fromFile('---\ntags:\n  - books\n  - "to read"\nauthor: Le Guin\n---\nText', 'Dune.md')).toMatchObject({ title: 'Dune', tags: ['books', 'to read'], fields: [{ key: 'author', type: 'text', value: 'Le Guin' }] })
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

describe('a one-line preview', () => {
  it('is the first line, as plain text', () => {
    expect(firstLine('')).toBe('')
    expect(firstLine('\n\n## Call **Ana** about [the report](https://x.org)\nmore')).toBe('Call Ana about the report')
    expect(firstLine('- [ ] buy `milk`\n- [x] eggs')).toBe('buy milk')
    expect(firstLine('```js\nconst a = 1\n```\n> _see_ ~~this~~ ![img](a.png)')).toBe('see this')
    expect(firstLine('---\n1. first step')).toBe('first step')
    expect(firstLine('snake_case_name and 2 * 3 * 4')).toBe('snake_case_name and 2 * 3 * 4')
    expect(firstLine(textToMarkdown('# not a heading'))).toBe('# not a heading')
  })
})
