// Where a page's PDF gets what the page shows in place: sheets (their cells
// as they read), diagrams and whiteboards (their pictures, drawn as PNG),
// the page's own pictures (fetched with the sign-in), its sub-pages.
import type { PdfSources } from './pdf'
import type { EmbedRef } from './embeds'
import { sheetsApi } from './sheetsApi'
import { diagramsApi } from './diagramsApi'
import { drawingsApi } from '@/components/draw/drawing'
import { Book, addr, parseAddr, readData, show } from './sheet'
import { dayOf } from './formula'
import { locale, todayStr } from './dates'
import { uploadBlobUrl } from './uploads'

const MAX_ROWS = 60
const MAX_COLS = 12

// pngOfUrl draws a picture (any the browser shows) as a PNG data URL, at
// twice its size for sharpness (and the size it's shown at).
async function pngOfUrl(url: string, w0?: number, h0?: number): Promise<{ png: string; width: number; height: number }> {
  const img = new Image()
  img.src = url
  await img.decode()
  const width = w0 ?? (img.naturalWidth || 600)
  const height = h0 ?? (img.naturalHeight || 400)
  const canvas = document.createElement('canvas')
  canvas.width = width * 2
  canvas.height = height * 2
  const g = canvas.getContext('2d')!
  g.fillStyle = '#ffffff'
  g.fillRect(0, 0, canvas.width, canvas.height)
  g.drawImage(img, 0, 0, canvas.width, canvas.height)
  return { png: canvas.toDataURL('image/png'), width, height }
}

// An SVG's own size: its width and height, else its viewBox.
function svgSize(svg: string): { w: number; h: number } {
  const tag = /<svg\b[^>]*>/.exec(svg)?.[0] ?? ''
  // A size in percent says nothing of its own size.
  const num = (a: string) => {
    const v = new RegExp(`\\s${a}="([^"]+)"`).exec(tag)?.[1] ?? ''
    return v.includes('%') ? 0 : Number(/^[\d.]+/.exec(v)?.[0])
  }
  const vb = /viewBox="([\d.\s-]+)"/.exec(tag)?.[1].trim().split(/\s+/).map(Number)
  const w = num('width') || vb?.[2] || 600
  const h = num('height') || vb?.[3] || 400
  return { w: Math.ceil(w), h: Math.ceil(h) }
}

// sizedSvg sets an SVG's width and height to its own size, so a "100%"
// doesn't draw it at nothing.
export function sizedSvg(svg: string): string {
  const { w, h } = svgSize(svg)
  return svg.replace(/<svg\b[^>]*>/, (tag) => tag.replace(/\s(width|height)="[^"]*"/g, '').replace(/^<svg/, `<svg width="${w}" height="${h}"`))
}

export async function svgPng(svg: string) {
  const { w, h } = svgSize(svg)
  return pngOfUrl(`data:image/svg+xml;charset=utf-8,${encodeURIComponent(sizedSvg(svg))}`, w, h)
}

export function pageSources(opts: { subpages: () => string[]; tabName: string }): PdfSources {
  return {
    origin: location.origin,
    subpages: opts.subpages,
    async sheet(id) {
      const s = await sheetsApi.get(id)
      const data = readData(s.data, opts.tabName)
      const loc = locale()
      const book = new Book(data, dayOf(todayStr()), loc.startsWith('pt'))
      const tabs = data.tabs.map((tab) => {
        let rows = 0
        let cols = 0
        for (const [a, c] of Object.entries(tab.cells)) {
          const p = parseAddr(a)
          if (p && c.v !== '') {
            rows = Math.max(rows, p.r + 1)
            cols = Math.max(cols, p.c + 1)
          }
        }
        rows = Math.min(rows, MAX_ROWS)
        cols = Math.min(cols, MAX_COLS)
        const out: string[][] = []
        for (let r = 0; r < rows; r++) {
          const row: string[] = []
          for (let c = 0; c < cols; c++) row.push(show(book.value(tab.id, r, c), tab.cells[addr(r, c)], loc, { yes: '✓', no: '' }).text)
          out.push(row)
        }
        return { name: tab.name, rows: out }
      })
      return { title: s.title, tabs }
    },
    async picture(ref: EmbedRef) {
      if (ref.kind === 'diagram') {
        const d = await diagramsApi.get(ref.id)
        let svg = d.preview
        if (!svg && d.kind === 'mermaid' && d.data) svg = (await (await import('./mermaid')).renderMermaid(d.data)).svg
        return svg ? { title: d.title, ...(await svgPng(svg)) } : null
      }
      const b = await drawingsApi.get(ref.id)
      return b.preview ? { title: b.title, ...(await svgPng(b.preview)) } : null
    },
    async image(src) {
      // The page's own pictures only (others would need their site).
      if (!src.startsWith('/api/v1/uploads/')) return null
      return pngOfUrl(await uploadBlobUrl(src))
    },
  }
}
