import { describe, expect, it } from 'vitest'
import { Marked } from 'marked'
import { blocks, inline } from './pdf'
import type { PdfSources } from './pdf'

const src: PdfSources = {
  origin: 'https://orbit.example',
  subpages: () => ['Night shift'],
  sheet: async () => ({ title: 'Fees', tabs: [{ name: 'Sheet 1', cols: ['B', 'E'], rows: [{ n: 3, cells: [{ text: 'Yearly' }, { text: '40', align: 'right' }] }], more: false }] }),
  picture: async (r) => ({ title: `Flow ${r.id}`, png: 'data:image/png;base64,AAAA', width: 900, height: 300 }),
  image: async () => null,
}
const lex = (md: string) => new Marked({ gfm: true }).lexer(md)

describe('pdf', () => {
  it('keeps inline formatting and links (absolute)', () => {
    const runs = inline((lex('Some **bold**, *it* and [a page](/w/1/p/2) 🎉')[0] as any).tokens, src.origin)
    expect(runs.find((r) => r.text === 'bold')?.bold).toBe(true)
    expect(runs.find((r) => r.text === 'it')?.italics).toBe(true)
    expect(runs.find((r) => r.text === 'a page')?.link).toBe('https://orbit.example/w/1/p/2')
    expect(runs.map((r) => r.text).join('')).not.toContain('🎉')
  })

  it('draws sheets, diagrams and sub-pages in, as they are', async () => {
    const md = '# Plan\n\n[Fees](/w/a/s/s1)\n\n[Flow](/w/a/d/d1)\n\n::pagetree\n\n- [x] done\n- [ ] open\n\n> [!TIP]\n> Go.\n\n| A | B |\n| - | - |\n| 1 | 2 |'
    const out = await blocks(lex(md), src)
    const flat = JSON.stringify(out)
    expect(out[0].style).toBe('h1')
    expect(flat).toContain('"Yearly"')
    expect(flat).toContain('"E"') // its own column letters…
    expect(flat).toContain('"3"') // …and row numbers
    expect(flat).toContain('data:image/png;base64,AAAA')
    const kept = (c: any) => (c.unbreakable ? c.stack : [c])
    expect(out.flatMap(kept).find((c: any) => c.image)?.width).toBe(515) // fitted to the page, its title kept with it
    expect(flat).toContain('Night shift')
    expect(flat).toContain('[x] ')
    expect(flat).toContain('"Tip"')
    expect(out.flatMap(kept).some((c: any) => c.table?.headerRows === 1)).toBe(true)
  })
})
