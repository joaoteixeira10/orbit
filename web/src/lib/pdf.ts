// A page as a PDF file, made in the browser (pdfmake, loaded when first
// needed) and downloaded — no print dialog. Real text, working links; its
// headings, lists, to-dos, tables, quotes and callouts, code, pictures; and
// what it shows in place drawn in: a sheet as a table of its cells (each
// tab), a diagram or a whiteboard as its picture.
import { Marked, type Token, type Tokens } from 'marked'
import { embedOf, type EmbedRef } from './embeds'

// What a page shows in place, read for the PDF (null: can't).
export interface PdfSources {
  sheet: (id: string) => Promise<{ title: string; tabs: { name: string; rows: string[][] }[] } | null>
  picture: (ref: EmbedRef) => Promise<{ title: string; png: string; width: number; height: number } | null>
  image: (src: string) => Promise<{ png: string; width: number; height: number } | null>
  subpages: () => string[]
  origin: string
}

const PAGE_WIDTH = 515 // A4 less the margins, in points
// Roboto has no emoji: they'd print as nothing.
const noEmoji = (s: string) => s.replace(/[\p{Extended_Pictographic}\u{FE0F}\u{200D}]/gu, '').replace(/\s{2,}/g, ' ')

type Content = any

// inline turns a paragraph's tokens into pdfmake text runs.
export function inline(tokens: Token[] | undefined, origin: string, style: Record<string, unknown> = {}): Content[] {
  const out: Content[] = []
  for (const t of tokens ?? []) {
    const tk = t as any
    switch (t.type) {
      case 'strong':
        out.push(...inline(tk.tokens, origin, { ...style, bold: true }))
        break
      case 'em':
        out.push(...inline(tk.tokens, origin, { ...style, italics: true }))
        break
      case 'del':
        out.push(...inline(tk.tokens, origin, { ...style, decoration: 'lineThrough' }))
        break
      case 'codespan':
        out.push({ text: unescape(tk.text), ...style, style: 'code', background: '#f2f2f4' })
        break
      case 'link': {
        const href = /^\//.test(tk.href) ? origin + tk.href : tk.href
        out.push(...inline(tk.tokens, origin, { ...style, link: href, color: '#0066cc' }))
        break
      }
      case 'br':
        out.push({ text: '\n', ...style })
        break
      case 'image':
        out.push({ text: tk.text ? `[${tk.text}]` : '', ...style, italics: true, color: '#6e6e73' })
        break
      case 'html': {
        const status = /data-status="(\w+)">([^<]*)</.exec(tk.raw)
        if (status) out.push({ text: ` ${status[2]} `, ...style, bold: true, fontSize: 8.5, background: '#eef0f3' })
        break // other inline tags (<mark>, <u>…): their text comes as text tokens
      }
      case 'text':
      case 'escape':
        if (tk.tokens?.length) out.push(...inline(tk.tokens, origin, style))
        else out.push({ text: noEmoji(unescape(tk.text)), ...style })
        break
      default:
        if (tk.text) out.push({ text: noEmoji(unescape(tk.text)), ...style })
    }
  }
  return out
}

const unescape = (s: string) => s.replace(/&amp;/g, '&').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"').replace(/&#39;/g, "'")

// fit sizes a picture to the page (never bigger than it is).
const fit = (w: number, h: number) => (w > PAGE_WIDTH ? { width: PAGE_WIDTH } : { width: w, height: h })

// blocks turns block tokens into pdfmake content.
export async function blocks(tokens: Token[], src: PdfSources): Promise<Content[]> {
  const out: Content[] = []
  for (const t of tokens) {
    const tk = t as any
    switch (t.type) {
      case 'heading':
        out.push({ text: inline(tk.tokens, src.origin), style: `h${Math.min(tk.depth, 3)}` })
        break
      case 'paragraph': {
        const only = tk.tokens?.length === 1 ? tk.tokens[0] : null
        // A link to a sheet, a diagram or a whiteboard alone: the thing itself.
        const ref = only?.type === 'link' ? embedOf(only.href.replace(src.origin, '')) : null
        if (ref) {
          out.push(...(await embed(ref, src)))
          break
        }
        if (tk.text.trim() === '::pagetree') {
          const subs = src.subpages()
          if (subs.length) out.push({ ul: subs.map(noEmoji), margin: [0, 0, 0, 8] })
          break
        }
        // A picture alone: drawn.
        if (only?.type === 'image') {
          const img = await src.image(only.href).catch(() => null)
          if (img) {
            out.push({ image: img.png, ...fit(img.width, img.height), alignment: 'center', margin: [0, 2, 0, 10] })
            break
          }
        }
        out.push({ text: inline(tk.tokens, src.origin), margin: [0, 0, 0, 8] })
        break
      }
      case 'list':
        out.push({
          [tk.ordered ? 'ol' : 'ul']: await Promise.all(
            tk.items.map(async (it: Tokens.ListItem) => {
              const inner = await blocks(it.tokens.filter((x) => x.type !== 'checkbox'), src)
              if (it.task) {
                const first = inner[0] ?? { text: '' }
                return { stack: [{ text: [{ text: it.checked ? '[x] ' : '[  ] ', color: '#8e8e93' }, ...(Array.isArray(first.text) ? first.text : [first.text ?? ''])], color: it.checked ? '#8e8e93' : undefined }, ...inner.slice(1)] }
              }
              return inner.length === 1 ? inner[0] : { stack: inner }
            }),
          ),
          ...(tk.items.some((i: Tokens.ListItem) => i.task) ? { type: 'none' } : {}),
          margin: [0, 0, 0, 8],
        })
        break
      case 'text':
        out.push({ text: inline(tk.tokens ?? [{ type: 'text', text: tk.text }] as Token[], src.origin) })
        break
      case 'space':
        break
      case 'code':
        out.push({ table: { widths: ['*'], body: [[{ text: tk.text, style: 'code', margin: [6, 4, 6, 4] }]] }, layout: 'noBorders', fillColor: '#f6f6f8', margin: [0, 0, 0, 8] })
        break
      case 'blockquote': {
        const alert = /^\s*\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]/i.exec(tk.text)
        const color = alert ? ({ TIP: '#34c759', WARNING: '#ff9500', CAUTION: '#ff3b30', IMPORTANT: '#af52de' } as Record<string, string>)[alert[1].toUpperCase()] ?? '#0a84ff' : '#d2d2d7'
        const inner = await blocks(tk.tokens, src)
        if (alert && inner[0]?.text) {
          // The "[!TIP]" marker: its name instead.
          const first = inner[0]
          const runs = Array.isArray(first.text) ? first.text : [first.text]
          if (typeof runs[0]?.text === 'string') runs[0].text = runs[0].text.replace(/^\s*\[!\w+\]\s*/, '')
          inner.unshift({ text: alert[1][0] + alert[1].slice(1).toLowerCase(), bold: true, color, margin: [0, 0, 0, 2] })
        }
        out.push({
          table: { widths: [3, '*'], body: [[{ text: '', fillColor: color }, { stack: inner, margin: [8, 4, 4, 0] }]] },
          layout: 'noBorders',
          fillColor: alert ? '#f6f8fb' : undefined,
          margin: [0, 0, 0, 8],
        })
        break
      }
      case 'table': {
        const head = tk.header.map((c: any) => ({ text: inline(c.tokens, src.origin), bold: true, fillColor: '#f6f6f8' }))
        const rows = tk.rows.map((r: any[]) => r.map((c: any) => ({ text: inline(c.tokens, src.origin) })))
        out.push({ table: { headerRows: 1, widths: head.map(() => '*'), body: [head, ...rows] }, layout: thinLines, fontSize: 9.5, margin: [0, 0, 0, 10] })
        break
      }
      case 'hr':
        out.push({ canvas: [{ type: 'line', x1: 0, y1: 4, x2: PAGE_WIDTH, y2: 4, lineWidth: 0.5, lineColor: '#d2d2d7' }], margin: [0, 6, 0, 10] })
        break
      case 'html': {
        // <details><summary>…</summary>: the title, bold; <div data-columns>…: nothing (what's inside comes next).
        const summary = /<summary>([\s\S]*?)<\/summary>/.exec(tk.raw)
        if (summary) out.push({ text: noEmoji(summary[1].replace(/<[^>]+>/g, '')), bold: true, margin: [0, 4, 0, 4] })
        break
      }
      default:
        if (tk.text) out.push({ text: noEmoji(tk.text), margin: [0, 0, 0, 8] })
    }
  }
  return out
}

const thinLines = {
  hLineWidth: () => 0.5,
  vLineWidth: () => 0.5,
  hLineColor: () => '#d2d2d7',
  vLineColor: () => '#d2d2d7',
  paddingLeft: () => 5,
  paddingRight: () => 5,
  paddingTop: () => 3,
  paddingBottom: () => 3,
}

async function embed(ref: EmbedRef, src: PdfSources): Promise<Content[]> {
  if (ref.kind === 'sheet') {
    const s = await src.sheet(ref.id).catch(() => null)
    if (!s) return []
    const out: Content[] = [{ text: noEmoji(s.title), bold: true, margin: [0, 4, 0, 4] }]
    for (const tab of s.tabs) {
      if (!tab.rows.length) continue
      if (s.tabs.length > 1) out.push({ text: tab.name, color: '#6e6e73', fontSize: 9, margin: [0, 2, 0, 2] })
      const cols = Math.max(...tab.rows.map((r) => r.length))
      out.push({ table: { body: tab.rows.map((r) => Array.from({ length: cols }, (_, i) => ({ text: r[i] ?? '' }))) }, layout: thinLines, fontSize: 9, margin: [0, 0, 0, 10] })
    }
    return out
  }
  const pic = await src.picture(ref).catch(() => null)
  if (!pic) return []
  return [
    { text: noEmoji(pic.title), bold: true, margin: [0, 4, 0, 4] },
    { image: pic.png, ...fit(pic.width, pic.height), alignment: 'center', margin: [0, 0, 0, 10] },
  ]
}

// docOf is the whole PDF's definition: title, a line under it, the page.
export async function docOf(title: string, meta: string, md: string, src: PdfSources): Promise<Content> {
  const tokens = new Marked({ gfm: true }).lexer(md)
  return {
    info: { title },
    pageSize: 'A4',
    pageMargins: [40, 48, 40, 48],
    content: [{ text: noEmoji(meta), style: 'meta' }, { text: noEmoji(title), style: 'title' }, ...(await blocks(tokens, src))],
    footer: (page: number, pages: number) => ({ text: `${page} / ${pages}`, alignment: 'center', fontSize: 8, color: '#8e8e93', margin: [0, 16, 0, 0] }),
    defaultStyle: { font: 'Roboto', fontSize: 10.5, lineHeight: 1.3, color: '#1d1d1f' },
    styles: {
      meta: { fontSize: 8.5, color: '#6e6e73', margin: [0, 0, 0, 4] },
      title: { fontSize: 24, bold: true, margin: [0, 0, 0, 14] },
      h1: { fontSize: 18, bold: true, margin: [0, 12, 0, 6] },
      h2: { fontSize: 15, bold: true, margin: [0, 10, 0, 5] },
      h3: { fontSize: 12.5, bold: true, margin: [0, 8, 0, 4] },
      code: { fontSize: 9, color: '#1d1d1f' },
    },
  }
}

// downloadPdf makes the PDF and saves it as name.pdf.
export async function downloadPdf(name: string, doc: Content): Promise<void> {
  const [{ default: pdfMake }, { default: vfs }] = await Promise.all([import('pdfmake/build/pdfmake'), import('pdfmake/build/vfs_fonts')])
  pdfMake.addVirtualFileSystem(vfs)
  pdfMake.fonts = { Roboto: { normal: 'Roboto-Regular.ttf', bold: 'Roboto-Medium.ttf', italics: 'Roboto-Italic.ttf', bolditalics: 'Roboto-MediumItalic.ttf' } }
  await pdfMake.createPdf(doc).download(`${name}.pdf`)
}
