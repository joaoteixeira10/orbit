import { describe, it, expect } from 'vitest'
import { Editor } from '@tiptap/core'
import { pageExtensions, minimalEscape } from './extensions'

function load(md: string) {
  return new Editor({ extensions: pageExtensions(), content: md, contentType: 'markdown' })
}
// Markdown in → visual editor → Markdown out.
function roundTrip(md: string): string {
  const ed = load(md)
  const out = ed.getMarkdown()
  ed.destroy()
  return out.replace(/\n{3,}/g, '\n\n').trim()
}

describe('page Markdown', () => {
  it('keeps the usual Markdown as written', () => {
    const md = [
      '# Title',
      '',
      'Some **bold**, *italic*, ~~gone~~, `code` and a [link](https://example.org).',
      '',
      '- one',
      '- two',
      '  - nested',
      '',
      '1. first',
      '2. second',
      '',
      '- [ ] open',
      '- [x] done',
      '',
      '> a quote',
      '',
      '```go',
      'fmt.Println("hi")',
      '```',
      '',
      '---',
    ].join('\n')
    expect(roundTrip(md)).toBe(md)
  })

  it('writes what Markdown lacks the way GitHub shows it', () => {
    expect(roundTrip('<mark>marked</mark> and <u>under</u>')).toBe('<mark>marked</mark> and <u>under</u>')
    expect(roundTrip('> [!WARNING]\n> Mind the step.')).toBe('> [!WARNING]\n> Mind the step.')
    const ed = load('<mark>m</mark> <u>u</u>\n\n> [!TIP]\n> x')
    const json = JSON.stringify(ed.getJSON())
    ed.destroy()
    expect(json).toContain('"highlight"')
    expect(json).toContain('"underline"')
    expect(json).toContain('"emoji":"💡"')
  })

  it('passes through what the editor does not edit', () => {
    for (const md of [
      '<details>\n<summary>More</summary>',
      'A footnote[^1] here.\n\n[^1]: The note.',
      'Press <kbd>⌘</kbd> and x<sup>2</sup>.',
      '![A cat](https://example.org/cat.png)',
      'Line one  \nLine two',
    ]) {
      expect(roundTrip(md)).toBe(md)
    }
  })

  it('escapes only what would turn into formatting', () => {
    expect(roundTrip('2 * 3 = 6, snake_case and a < b')).toBe('2 * 3 = 6, snake_case and a < b')
    expect(minimalEscape('*not italic*')).toBe('\\*not italic\\*')
    expect(minimalEscape('[label](x)')).toBe('\\[label](x)')
    expect(minimalEscape('<b>')).toBe('&lt;b>')
    // Whatever was escaped reads back as the same text.
    for (const text of ['*a*', '_b_', '~~c~~', '`d`', '[e](f)', '<g>', 'h &amp; i', 'back\\*slash']) {
      const ed = load(minimalEscape(text))
      expect(ed.getText()).toBe(text)
      ed.destroy()
    }
  })

  it('is stable: a second round trip changes nothing', () => {
    const md = '| A | B |\n| --- | --- |\n| 1 | 2 |\n\nText *with* `marks`, <mark>hl</mark>.\n\n- [ ] a\n  - [x] b'
    const once = roundTrip(md)
    expect(roundTrip(once)).toBe(once)
  })
})
