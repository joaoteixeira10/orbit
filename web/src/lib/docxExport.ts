// A page as a Word file (.docx), made in the browser and downloaded: its
// headings, bold / italics / strikethrough, links, lists and to-dos, quotes
// and callouts, code, tables, pictures — and, as the PDF does, what it shows
// in place: a sheet as a table, a diagram or a whiteboard as its picture (or
// their links, when asked). Word's own styles (Title, Heading 1–3, Quote…),
// so the document reads and edits as one made in Word.
import { Marked, type Token, type Tokens } from 'marked'
import { strToU8, zipSync } from 'fflate'
import { embedOf, type EmbedRef } from './embeds'
import type { PdfSources } from './pdf'

const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
const unescape = (s: string) => s.replace(/&amp;/g, '&').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"').replace(/&#39;/g, "'")
const MAX_EMU = 5_943_600 // 6.5 in: the page's width less its margins

interface Run {
  bold?: boolean
  italic?: boolean
  strike?: boolean
  code?: boolean
  color?: string
}

// The document being written: its relationships (links, pictures) and lists.
class Doc {
  rels: string[] = []
  media: Record<string, Uint8Array> = {}
  nums: string[] = [] // numbering instances: <w:num> per list
  private n = 0
  private pics = 0
  constructor(public src: PdfSources) {}

  link(url: string): string {
    const id = `rL${++this.n}`
    this.rels.push(`<Relationship Id="${id}" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="${esc(url)}" TargetMode="External"/>`)
    return id
  }
  // A list: a numbering instance of its own (numbers start again at 1).
  list(ordered: boolean): number {
    const id = this.nums.length + 1
    this.nums.push(`<w:num w:numId="${id}"><w:abstractNumId w:val="${ordered ? 1 : 0}"/>${ordered ? '<w:lvlOverride w:ilvl="0"><w:startOverride w:val="1"/></w:lvlOverride>' : ''}</w:num>`)
    return id
  }
  picture(dataUrl: string, w: number, h: number): string {
    const m = /^data:image\/png;base64,(.*)$/.exec(dataUrl)
    if (!m || !w || !h) return ''
    const bin = atob(m[1])
    const bytes = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
    const k = ++this.pics
    this.media[`word/media/image${k}.png`] = bytes
    const id = `rP${k}`
    this.rels.push(`<Relationship Id="${id}" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image${k}.png"/>`)
    let cx = Math.round(w * 9525)
    let cy = Math.round(h * 9525)
    if (cx > MAX_EMU) {
      cy = Math.round((cy * MAX_EMU) / cx)
      cx = MAX_EMU
    }
    return `<w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="${cx}" cy="${cy}"/><wp:docPr id="${k}" name="Picture ${k}"/><a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:nvPicPr><pic:cNvPr id="${k}" name="image${k}.png"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="${id}"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="${cx}" cy="${cy}"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`
  }
}

function run(text: string, r: Run): string {
  if (!text) return ''
  const pr = [
    r.bold ? '<w:b/>' : '',
    r.italic ? '<w:i/>' : '',
    r.strike ? '<w:strike/>' : '',
    r.code ? '<w:rFonts w:ascii="Consolas" w:hAnsi="Consolas" w:cs="Consolas"/><w:shd w:val="clear" w:color="auto" w:fill="F2F2F4"/>' : '',
    r.color ? `<w:color w:val="${r.color}"/>` : '',
  ].join('')
  // Line breaks inside a run: <w:br/>.
  const parts = text.split('\n').map((p) => `<w:t xml:space="preserve">${esc(p)}</w:t>`)
  return `<w:r>${pr ? `<w:rPr>${pr}</w:rPr>` : ''}${parts.join('<w:br/>')}</w:r>`
}

// inline turns a paragraph's tokens into runs.
function inline(doc: Doc, tokens: Token[] | undefined, r: Run = {}): string {
  let out = ''
  for (const t of tokens ?? []) {
    const tk = t as any
    switch (t.type) {
      case 'strong':
        out += inline(doc, tk.tokens, { ...r, bold: true })
        break
      case 'em':
        out += inline(doc, tk.tokens, { ...r, italic: true })
        break
      case 'del':
        out += inline(doc, tk.tokens, { ...r, strike: true })
        break
      case 'codespan':
        out += run(unescape(tk.text), { ...r, code: true })
        break
      case 'link': {
        const href = /^\//.test(tk.href) ? doc.src.origin + tk.href : tk.href
        if (!/^(https?:|mailto:)/i.test(href)) {
          out += inline(doc, tk.tokens, r)
          break
        }
        const inner = inline(doc, tk.tokens, { ...r, color: '0066CC' }).replace(/<w:rPr>/g, '<w:rPr><w:rStyle w:val="Hyperlink"/>').replace(/<w:r>(?!<w:rPr>)/g, '<w:r><w:rPr><w:rStyle w:val="Hyperlink"/></w:rPr>')
        out += `<w:hyperlink r:id="${doc.link(href)}">${inner}</w:hyperlink>`
        break
      }
      case 'br':
        out += '<w:r><w:br/></w:r>'
        break
      case 'image':
        out += run(tk.text ? `[${tk.text}]` : '', { ...r, italic: true, color: '6E6E73' })
        break
      case 'html': {
        const status = /data-status="(\w+)">([^<]*)</.exec(tk.raw)
        if (status) out += run(` ${status[2]} `, { ...r, bold: true })
        break
      }
      case 'text':
      case 'escape':
        out += tk.tokens?.length ? inline(doc, tk.tokens, r) : run(unescape(tk.text), r)
        break
      default:
        if (tk.text) out += run(unescape(tk.text), r)
    }
  }
  return out
}

const para = (body: string, pPr = '') => `<w:p>${pPr ? `<w:pPr>${pPr}</w:pPr>` : ''}${body}</w:p>`
const style = (s: string) => `<w:pStyle w:val="${s}"/>`

const TEXT_WIDTH = 9638 // twips: A4 less the margins

// table: rows of cells (each its runs), the first row a header; columns
// share the width (narrow: a sheet's row numbers, the first column).
function table(rows: string[][], header: boolean, cell = (c: string) => para(c), narrow = false): string {
  const n = Math.max(1, ...rows.map((r) => r.length))
  const first = narrow ? 600 : Math.floor(TEXT_WIDTH / n)
  const rest = n > 1 ? Math.floor((TEXT_WIDTH - first) / (n - 1)) : first
  const widths = Array.from({ length: n }, (_, i) => (i === 0 ? first : rest))
  const tr = (cells: string[], head: boolean) =>
    `<w:tr>${head ? '<w:trPr><w:tblHeader/></w:trPr>' : ''}${widths
      .map((w, i) => `<w:tc><w:tcPr><w:tcW w:w="${w}" w:type="dxa"/>${head ? '<w:shd w:val="clear" w:color="auto" w:fill="F2F2F4"/>' : ''}</w:tcPr>${cell(cells[i] ?? '')}</w:tc>`)
      .join('')}</w:tr>`
  return `<w:tbl><w:tblPr><w:tblStyle w:val="TableGrid"/><w:tblW w:w="${TEXT_WIDTH}" w:type="dxa"/><w:tblLayout w:type="fixed"/></w:tblPr><w:tblGrid>${widths.map((w) => `<w:gridCol w:w="${w}"/>`).join('')}</w:tblGrid>${rows.map((r, i) => tr(r, header && i === 0)).join('')}</w:tbl>${para('')}`
}

async function embed(doc: Doc, ref: EmbedRef): Promise<string> {
  const src = doc.src
  if (ref.kind === 'sheet') {
    const s = await src.sheet(ref.id).catch(() => null)
    if (!s) return ''
    let out = para(run(s.title, { bold: true }))
    for (const tab of s.tabs) {
      if (!tab.rows.length) continue
      if (s.tabs.length > 1) out += para(run(tab.name, { color: '6E6E73' }))
      const rows = [['', ...tab.cols], ...tab.rows.map((r) => [String(r.n), ...r.cells.map((c) => c.text)])]
      out += table(rows, true, (c) => para(run(c, {})), true)
      if (tab.more) out += para(run('…', { color: '6E6E73' }))
    }
    return out
  }
  const pic = await src.picture(ref).catch(() => null)
  if (!pic) return ''
  return para(run(pic.title, { bold: true })) + para(doc.picture(pic.png, pic.width, pic.height), '<w:jc w:val="center"/>')
}

async function blocks(doc: Doc, tokens: Token[], level = 0): Promise<string> {
  let out = ''
  for (const t of tokens) {
    const tk = t as any
    switch (t.type) {
      case 'heading':
        out += para(inline(doc, tk.tokens), style(`Heading${Math.min(tk.depth, 3)}`))
        break
      case 'paragraph': {
        const only = tk.tokens?.length === 1 ? tk.tokens[0] : null
        const ref = only?.type === 'link' && !doc.src.links ? embedOf(only.href.replace(doc.src.origin, '')) : null
        if (ref) {
          out += await embed(doc, ref)
          break
        }
        if (tk.text.trim() === '::pagetree') {
          const subs = doc.src.subpages()
          if (subs.length) {
            const id = doc.list(false)
            for (const s of subs) out += para(run(s, {}), `${style('ListParagraph')}<w:numPr><w:ilvl w:val="0"/><w:numId w:val="${id}"/></w:numPr>`)
          }
          break
        }
        if (only?.type === 'image') {
          const img = await doc.src.image(only.href).catch(() => null)
          if (img) {
            out += para(doc.picture(img.png, img.width, img.height), '<w:jc w:val="center"/>')
            break
          }
        }
        out += para(inline(doc, tk.tokens))
        break
      }
      case 'list': {
        const id = doc.list(tk.ordered)
        for (const it of tk.items as Tokens.ListItem[]) {
          const kids = it.tokens.filter((x) => x.type !== 'checkbox' && x.type !== 'list')
          const box = it.task ? run(it.checked ? '☑ ' : '☐ ', { color: '8E8E93' }) : ''
          const first = kids[0] as any
          const text = first ? inline(doc, first.tokens ?? [{ type: 'text', text: first.text }]) : ''
          out += para(box + text, `${style('ListParagraph')}${it.task ? '' : `<w:numPr><w:ilvl w:val="${Math.min(level, 8)}"/><w:numId w:val="${id}"/></w:numPr>`}${it.task ? `<w:ind w:left="${360 + level * 360}"/>` : ''}`)
          for (const rest of kids.slice(1)) out += await blocks(doc, [rest], level + 1)
          for (const sub of it.tokens.filter((x) => x.type === 'list')) out += await blocks(doc, [sub], level + 1)
        }
        break
      }
      case 'text':
        out += para(inline(doc, tk.tokens ?? [{ type: 'text', text: tk.text }]))
        break
      case 'space':
        break
      case 'code':
        for (const line of String(tk.text).split('\n')) out += para(run(line || ' ', { code: true }), style('Code'))
        break
      case 'blockquote': {
        const alert = /^\s*\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]/i.exec(tk.text)
        const color = alert ? ({ TIP: '34C759', WARNING: 'FF9500', CAUTION: 'FF3B30', IMPORTANT: 'AF52DE' } as Record<string, string>)[alert[1].toUpperCase()] ?? '0A84FF' : 'D2D2D7'
        if (alert) {
          // The "[!TIP]" marker: its name instead, on a line of its own.
          const first = tk.tokens[0]?.tokens?.[0]
          if (first && typeof first.text === 'string') first.text = first.text.replace(/^\s*\[!\w+\]\s*/, '')
        }
        let inner = await blocks(doc, tk.tokens, level)
        if (alert) inner = para(run(alert[1][0] + alert[1].slice(1).toLowerCase(), { bold: true, color })) + inner
        // Each paragraph inside, with the bar down the side (Word wants a
        // paragraph's properties in its schema's order).
        const bar = `<w:pBdr><w:left w:val="single" w:sz="18" w:space="8" w:color="${color}"/></w:pBdr>${alert ? '<w:shd w:val="clear" w:color="auto" w:fill="F6F8FB"/>' : ''}`
        out += inner.replace(/<w:p>(?:<w:pPr>([\s\S]*?)<\/w:pPr>)?/g, (_m, pr: string | undefined) => {
          const take = (re: RegExp) => (pr ?? '').match(re)?.[0] ?? ''
          const head = take(/<w:pStyle [^>]*\/>/) + take(/<w:numPr>[\s\S]*?<\/w:numPr>/)
          const ind = take(/<w:ind [^>]*\/>/) || '<w:ind w:left="240"/>'
          return `<w:p><w:pPr>${head}${bar}${ind}${take(/<w:jc [^>]*\/>/)}</w:pPr>`
        })
        break
      }
      case 'table': {
        const head = tk.header.map((c: any) => inline(doc, c.tokens))
        const rows = tk.rows.map((r: any[]) => r.map((c: any) => inline(doc, c.tokens)))
        out += table([head, ...rows], true)
        break
      }
      case 'hr':
        out += para('', '<w:pBdr><w:bottom w:val="single" w:sz="6" w:space="1" w:color="D2D2D7"/></w:pBdr>')
        break
      case 'html': {
        const summary = /<summary>([\s\S]*?)<\/summary>/.exec(tk.raw)
        if (summary) out += para(run(unescape(summary[1].replace(/<[^>]+>/g, '')), { bold: true }))
        break
      }
      default:
        if (tk.text) out += para(run(unescape(tk.text), {}))
    }
  }
  return out
}

const STYLES = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:eastAsia="Calibri" w:cs="Calibri"/><w:sz w:val="22"/><w:szCs w:val="22"/><w:lang w:val="en-GB"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="140" w:line="276" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults>
<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:qFormat/></w:style>
<w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:spacing w:after="60"/></w:pPr><w:rPr><w:sz w:val="52"/><w:b/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Subtitle"><w:name w:val="Subtitle"/><w:basedOn w:val="Normal"/><w:qFormat/><w:pPr><w:spacing w:after="280"/></w:pPr><w:rPr><w:color w:val="6E6E73"/><w:sz w:val="18"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:keepNext/><w:spacing w:before="320" w:after="120"/><w:outlineLvl w:val="0"/></w:pPr><w:rPr><w:b/><w:sz w:val="36"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:keepNext/><w:spacing w:before="260" w:after="100"/><w:outlineLvl w:val="1"/></w:pPr><w:rPr><w:b/><w:sz w:val="30"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading3"><w:name w:val="heading 3"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:keepNext/><w:spacing w:before="220" w:after="80"/><w:outlineLvl w:val="2"/></w:pPr><w:rPr><w:b/><w:sz w:val="26"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="ListParagraph"><w:name w:val="List Paragraph"/><w:basedOn w:val="Normal"/><w:qFormat/><w:pPr><w:spacing w:after="60"/><w:contextualSpacing/></w:pPr></w:style>
<w:style w:type="paragraph" w:styleId="Code"><w:name w:val="Code"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/><w:shd w:val="clear" w:color="auto" w:fill="F6F6F8"/></w:pPr><w:rPr><w:rFonts w:ascii="Consolas" w:hAnsi="Consolas" w:cs="Consolas"/><w:sz w:val="19"/></w:rPr></w:style>
<w:style w:type="character" w:styleId="Hyperlink"><w:name w:val="Hyperlink"/><w:rPr><w:color w:val="0066CC"/><w:u w:val="single"/></w:rPr></w:style>
<w:style w:type="table" w:styleId="TableGrid"><w:name w:val="Table Grid"/><w:tblPr><w:tblBorders><w:top w:val="single" w:sz="4" w:color="D2D2D7"/><w:left w:val="single" w:sz="4" w:color="D2D2D7"/><w:bottom w:val="single" w:sz="4" w:color="D2D2D7"/><w:right w:val="single" w:sz="4" w:color="D2D2D7"/><w:insideH w:val="single" w:sz="4" w:color="D2D2D7"/><w:insideV w:val="single" w:sz="4" w:color="D2D2D7"/></w:tblBorders><w:tblCellMar><w:left w:w="100" w:type="dxa"/><w:right w:w="100" w:type="dxa"/></w:tblCellMar></w:tblPr></w:style>
</w:styles>`

// Bullets (0) and numbers (1), nine levels each.
const NUMBERING = (nums: string[]) => {
  const lvls = (ordered: boolean) =>
    Array.from({ length: 9 }, (_, i) =>
      ordered
        ? `<w:lvl w:ilvl="${i}"><w:start w:val="1"/><w:numFmt w:val="${['decimal', 'lowerLetter', 'lowerRoman'][i % 3]}"/><w:lvlText w:val="%${i + 1}."/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="${720 + i * 360}" w:hanging="360"/></w:pPr></w:lvl>`
        : `<w:lvl w:ilvl="${i}"><w:start w:val="1"/><w:numFmt w:val="bullet"/><w:lvlText w:val="${['•', '◦', '▪'][i % 3]}"/><w:lvlJc w:val="left"/><w:pPr><w:ind w:left="${720 + i * 360}" w:hanging="360"/></w:pPr></w:lvl>`,
    ).join('')
  return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:abstractNum w:abstractNumId="0"><w:multiLevelType w:val="hybridMultilevel"/>${lvls(false)}</w:abstractNum><w:abstractNum w:abstractNumId="1"><w:multiLevelType w:val="hybridMultilevel"/>${lvls(true)}</w:abstractNum>${nums.join('')}</w:numbering>`
}

// docxOf is the whole Word file: the title, a line under it, the page.
export async function docxOf(title: string, meta: string, md: string, src: PdfSources): Promise<Uint8Array> {
  const doc = new Doc(src)
  const tokens = new Marked({ gfm: true }).lexer(md)
  const body = para(run(title, {}), style('Title')) + (meta ? para(run(meta, {}), style('Subtitle')) : '') + (await blocks(doc, tokens))
  const document = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"><w:body>${body}<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1134" w:right="1134" w:bottom="1134" w:left="1134" w:header="708" w:footer="708" w:gutter="0"/></w:sectPr></w:body></w:document>`
  const files: Record<string, Uint8Array> = {
    '[Content_Types].xml': strToU8(
      `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Default Extension="png" ContentType="image/png"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/><Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/><Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/></Types>`,
    ),
    '_rels/.rels': strToU8(
      `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/></Relationships>`,
    ),
    'docProps/core.xml': strToU8(
      `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><dc:title>${esc(title)}</dc:title><dc:creator>Orbit</dc:creator><dcterms:created xsi:type="dcterms:W3CDTF">${new Date().toISOString().slice(0, 19)}Z</dcterms:created></cp:coreProperties>`,
    ),
    'word/document.xml': strToU8(document),
    'word/styles.xml': strToU8(STYLES),
    'word/numbering.xml': strToU8(NUMBERING(doc.nums)),
    'word/_rels/document.xml.rels': strToU8(
      `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rStyles" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/><Relationship Id="rNumbering" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/>${doc.rels.join('')}</Relationships>`,
    ),
    ...doc.media,
  }
  return zipSync(files)
}

// downloadDocx makes the Word file and saves it as name.docx.
export async function downloadDocx(name: string, title: string, meta: string, md: string, src: PdfSources): Promise<void> {
  const bytes = await docxOf(title, meta, md, src)
  const url = URL.createObjectURL(new Blob([bytes as BlobPart], { type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document' }))
  const a = Object.assign(document.createElement('a'), { href: url, download: `${name}.docx` })
  document.body.append(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
