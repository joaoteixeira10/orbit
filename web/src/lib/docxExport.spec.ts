import { describe, expect, it } from 'vitest'
import { strFromU8, unzipSync } from 'fflate'
import { docxOf } from './docxExport'
import { docxToHtml } from './docx'
import type { PdfSources } from './pdf'

const PNG = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='
const src: PdfSources = {
  origin: 'https://orbit.example',
  subpages: () => ['Night shift'],
  sheet: async () => ({ title: 'Fees', tabs: [{ name: 'Sheet 1', cols: ['B'], rows: [{ n: 3, cells: [{ text: 'Yearly' }] }], more: false }] }),
  picture: async () => ({ title: 'Flow', png: PNG, width: 900, height: 300 }),
  image: async () => null,
}

describe('a page as a Word file', () => {
  it('is a Word document with the page in it — read back as Word reads it', async () => {
    const md = '# Plan\n\nSome **bold**, *italics* and a [link](https://example.org).\n\n- one\n- two\n\n1. first\n2. second\n\n- [x] done\n\n> [!TIP]\n> Go.\n\n| A | B |\n| - | - |\n| 1 | 2 |\n\n[Fees](/w/a/s/s1)\n\n[Flow](/w/a/d/d1)\n\n::pagetree'
    const bytes = await docxOf('My page', 'Work · today', md, src)
    const files = unzipSync(bytes)
    expect(Object.keys(files)).toEqual(expect.arrayContaining(['[Content_Types].xml', 'word/document.xml', 'word/styles.xml', 'word/numbering.xml', 'word/media/image1.png']))
    const xml = strFromU8(files['word/document.xml'])
    // Well-formed XML.
    expect(new DOMParser().parseFromString(xml, 'application/xml').getElementsByTagName('parsererror')).toHaveLength(0)
    expect(strFromU8(files['word/_rels/document.xml.rels'])).toContain('Target="https://example.org"')
    const html = docxToHtml(bytes)
    expect(html).toContain('<h1>My page</h1>')
    expect(html).toContain('<h2>Plan</h2>')
    expect(html).toContain('<strong>bold</strong>')
    expect(html).toContain('<ol><li>first</li><li>second</li></ol>')
    expect(html).toContain('<ul><li>one</li><li>two</li></ul>')
    expect(html).toMatch(/☑ <\/span>done/)
    expect(html).toContain('Tip')
    expect(html).toContain('Yearly')
    expect(html).toContain('<img src="data:image/png')
    expect(html).toContain('Night shift')
  })
})
