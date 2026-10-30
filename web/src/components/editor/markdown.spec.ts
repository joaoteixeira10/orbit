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

  it('shows a sheet, a diagram or a whiteboard alone on its line as a block', () => {
    const md = 'Intro.\n\n[Budget \\[2026\\]](/w/w1/s/abc)\n\n[Flow](/w/w1/d/d1)\n\n[Board](/w/w1/whiteboards/b1)\n\nAfter.'
    const ed = load(md)
    const blocks = (ed.getJSON().content ?? []).map((n: any) => (n.type === 'docEmbed' ? `${n.attrs.kind}:${n.attrs.id}:${n.attrs.title}` : n.type))
    ed.destroy()
    expect(blocks).toEqual(['paragraph', 'sheet:abc:Budget [2026]', 'diagram:d1:Flow', 'board:b1:Board', 'paragraph'])
    expect(roundTrip(md)).toBe(md)
    // Not alone on its line (in a sentence, or with a line after it): a link.
    for (const inline of ['See [Flow](/w/w1/d/d1) here.', '[Flow](/w/w1/d/d1)\nmore', 'text\n[Flow](/w/w1/d/d1)', '[Page](/w/w1/p/p1)']) {
      const e = load(inline)
      expect(JSON.stringify(e.getJSON())).not.toContain('docEmbed')
      e.destroy()
      expect(roundTrip(inline)).toBe(inline)
    }
  })

  it('is stable: a second round trip changes nothing', () => {
    const md = '| A | B |\n| --- | --- |\n| 1 | 2 |\n\nText *with* `marks`, <mark>hl</mark>.\n\n- [ ] a\n  - [x] b'
    const once = roundTrip(md)
    expect(roundTrip(once)).toBe(once)
  })
})
