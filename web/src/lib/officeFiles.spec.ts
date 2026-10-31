import { describe, expect, it } from 'vitest'
import { strToU8, zipSync } from 'fflate'
import { docxToHtml } from './docx'
import { readPptx } from './pptx'
import { viewKind } from './fileView'

const W = 'xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"'
const P = 'xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"'
const zip = (files: Record<string, string>) => zipSync(Object.fromEntries(Object.entries(files).map(([k, v]) => [k, strToU8(v)])))
const rels = (list: [string, string, string?][]) =>
  `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">${list.map(([id, t, mode]) => `<Relationship Id="${id}" Target="${t}"${mode ? ` TargetMode="${mode}"` : ''} Type="x"/>`).join('')}</Relationships>`

describe('a Word file as a page', () => {
  it('has its headings, runs, lists, tables and links — and nothing that runs', () => {
    const body = `<w:document ${W}><w:body>
      <w:p><w:pPr><w:pStyle w:val="Title"/></w:pPr><w:r><w:t>Report</w:t></w:r></w:p>
      <w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Intro</w:t></w:r></w:p>
      <w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Bold</w:t></w:r><w:r><w:t xml:space="preserve"> and </w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>italic</w:t></w:r></w:p>
      <w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr><w:r><w:t>One</w:t></w:r></w:p>
      <w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr><w:r><w:t>Two</w:t></w:r></w:p>
      <w:p><w:hyperlink r:id="rL"><w:r><w:t>site</w:t></w:r></w:hyperlink><w:r><w:t>&lt;script&gt;x&lt;/script&gt;</w:t></w:r></w:p>
      <w:tbl><w:tr><w:tc><w:p><w:r><w:t>A1</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>B1</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
    </w:body></w:document>`
    const numbering = `<w:numbering ${W}><w:abstractNum w:abstractNumId="0"><w:lvl w:ilvl="0"><w:numFmt w:val="decimal"/></w:lvl></w:abstractNum><w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num></w:numbering>`
    const html = docxToHtml(zip({ 'word/document.xml': body, 'word/numbering.xml': numbering, 'word/_rels/document.xml.rels': rels([['rL', 'https://example.org/', 'External']]) }))
    expect(html).toContain('<h1>Report</h1>')
    expect(html).toContain('<h2>Intro</h2>')
    expect(html).toContain('<strong>Bold</strong> and <em>italic</em>')
    expect(html).toContain('<ol><li>One</li><li>Two</li></ol>')
    expect(html).toContain('href="https://example.org/"')
    expect(html).toContain('<td><p>A1</p></td>')
    expect(html).not.toContain('<script')
    expect(html).toContain('&lt;script&gt;')
  })
})

describe('a PowerPoint file as slides', () => {
  it('has its slides in order, text where it sits, a placeholder placed by its layout', () => {
    const slide = (title: string, body: string) => `<p:sld ${P}><p:cSld><p:spTree>
      <p:sp><p:nvSpPr><p:cNvPr id="2" name="T"/><p:cNvSpPr/><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr><p:spPr/><p:txBody><a:p><a:r><a:t>${title}</a:t></a:r></a:p></p:txBody></p:sp>
      <p:sp><p:nvSpPr><p:cNvPr id="3" name="B"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="0" y="3429000"/><a:ext cx="6096000" cy="1000000"/></a:xfrm><a:solidFill><a:srgbClr val="FFCC00"/></a:solidFill></p:spPr>
        <p:txBody><a:p><a:pPr><a:buChar char="•"/></a:pPr><a:r><a:rPr sz="2400" b="1"/><a:t>${body}</a:t></a:r></a:p></p:txBody></p:sp>
    </p:spTree></p:cSld></p:sld>`
    const layout = `<p:sldLayout ${P}><p:cSld><p:spTree><p:sp><p:nvSpPr><p:cNvPr id="2" name="T"/><p:cNvSpPr/><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr><p:spPr><a:xfrm><a:off x="609600" y="0"/><a:ext cx="10972800" cy="1143000"/></a:xfrm></p:spPr></p:sp></p:spTree></p:cSld></p:sldLayout>`
    const deck = readPptx(
      zip({
        'ppt/presentation.xml': `<p:presentation ${P}><p:sldIdLst><p:sldId id="256" r:id="r2"/><p:sldId id="257" r:id="r1"/></p:sldIdLst><p:sldSz cx="12192000" cy="6858000"/></p:presentation>`,
        'ppt/_rels/presentation.xml.rels': rels([['r1', 'slides/slide1.xml'], ['r2', 'slides/slide2.xml']]),
        'ppt/slides/slide1.xml': slide('Second', 'b'),
        'ppt/slides/slide2.xml': slide('First', 'Point'),
        'ppt/slides/_rels/slide1.xml.rels': rels([['l', '../slideLayouts/slideLayout1.xml']]),
        'ppt/slides/_rels/slide2.xml.rels': rels([['l', '../slideLayouts/slideLayout1.xml']]),
        'ppt/slideLayouts/slideLayout1.xml': layout,
      }),
    )
    expect(deck.width).toBe(960)
    expect(deck.slides.map((s) => s.shapes[0].paras?.[0].runs[0].text)).toEqual(['First', 'Second'])
    const [title, body] = deck.slides[0].shapes
    expect(title.title).toBe(true)
    expect(title.x).toBeCloseTo(0.05, 2) // from its layout
    expect(body.y).toBeCloseTo(0.5, 2)
    expect(body.fill).toBe('#FFCC00')
    expect(body.paras?.[0]).toMatchObject({ bullet: true, runs: [{ text: 'Point', bold: true, size: 24 }] })
  })
})

describe('what a file is shown as', () => {
  it('goes by its name, then its type', () => {
    expect(viewKind('a.pdf', 'application/octet-stream')).toBe('pdf')
    expect(viewKind('Report.DOCX', 'application/zip')).toBe('docx')
    expect(viewKind('b.xlsx', 'application/zip')).toBe('sheet')
    expect(viewKind('deck.pptx', 'application/zip')).toBe('pptx')
    expect(viewKind('photo', 'image/jpeg')).toBe('image')
    expect(viewKind('logo.svg', 'image/svg+xml')).toBe('none')
    expect(viewKind('clip.mov', 'application/octet-stream')).toBe('video')
    expect(viewKind('notes.md', 'text/plain')).toBe('markdown')
    expect(viewKind('x.bin', 'application/octet-stream')).toBe('none')
  })
})
