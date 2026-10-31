// A Word file (.docx) as a page to read: its paragraphs and headings, bold,
// italics, underline, lists, tables, links and pictures — read from the XML
// inside (unzipped with fflate), not a copy of Word's layout. The HTML it
// makes is cleaned (DOMPurify) before it's shown.
import { strFromU8, unzipSync } from 'fflate'
import DOMPurify from 'dompurify'

const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
const kids = (el: Element) => [...el.children]
const kid = (el: Element | null | undefined, name: string) => (el ? kids(el).find((c) => c.localName === name) ?? null : null)
// w:val (or any attribute) by its local name.
const attr = (el: Element | null | undefined, name: string) => {
  if (!el) return null
  for (const a of [...el.attributes]) if (a.localName === name) return a.value
  return null
}

const MIME: Record<string, string> = { png: 'image/png', jpg: 'image/jpeg', jpeg: 'image/jpeg', gif: 'image/gif', bmp: 'image/bmp', svg: 'image/svg+xml', webp: 'image/webp' }

export function dataUrl(bytes: Uint8Array, name: string): string | null {
  const type = MIME[name.split('.').pop()?.toLowerCase() ?? '']
  if (!type) return null
  let s = ''
  for (let i = 0; i < bytes.length; i += 0x8000) s += String.fromCharCode(...bytes.subarray(i, i + 0x8000))
  return `data:${type};base64,${btoa(s)}`
}

// rels reads a part's relationships: id → target (a path inside the zip, or
// an address for external ones).
export function relsOf(files: Record<string, Uint8Array>, part: string): Map<string, { target: string; external: boolean }> {
  const dir = part.slice(0, part.lastIndexOf('/'))
  const name = part.slice(part.lastIndexOf('/') + 1)
  const raw = files[`${dir}/_rels/${name}.rels`]
  const out = new Map<string, { target: string; external: boolean }>()
  if (!raw) return out
  const doc = new DOMParser().parseFromString(strFromU8(raw), 'application/xml')
  for (const r of [...doc.getElementsByTagName('Relationship')]) {
    const external = r.getAttribute('TargetMode') === 'External'
    const t = r.getAttribute('Target') ?? ''
    out.set(r.getAttribute('Id') ?? '', { target: external ? t : resolve(dir, t), external })
  }
  return out
}

// resolve a relative path against a directory inside the zip.
export function resolve(dir: string, t: string): string {
  if (t.startsWith('/')) return t.slice(1)
  const parts = dir ? dir.split('/') : []
  for (const p of t.split('/')) {
    if (p === '..') parts.pop()
    else if (p && p !== '.') parts.push(p)
  }
  return parts.join('/')
}

export function docxToHtml(bytes: Uint8Array): string {
  const files = unzipSync(bytes)
  const main = files['word/document.xml']
  if (!main) throw new Error('not a Word document')
  const doc = new DOMParser().parseFromString(strFromU8(main), 'application/xml')
  const rels = relsOf(files, 'word/document.xml')

  // Lists: which numbering is bullets, which numbers.
  const ordered = new Set<string>() // `${numId}:${ilvl}`
  const numbering = files['word/numbering.xml']
  if (numbering) {
    const nd = new DOMParser().parseFromString(strFromU8(numbering), 'application/xml')
    const abstract = new Map<string, Element>()
    for (const a of [...nd.getElementsByTagNameNS('*', 'abstractNum')]) abstract.set(attr(a, 'abstractNumId') ?? '', a)
    for (const n of [...nd.getElementsByTagNameNS('*', 'num')]) {
      const a = abstract.get(attr(kid(n, 'abstractNumId'), 'val') ?? '')
      for (const lvl of a ? [...a.getElementsByTagNameNS('*', 'lvl')] : []) {
        const fmt = attr(kid(lvl, 'numFmt'), 'val')
        if (fmt && fmt !== 'bullet' && fmt !== 'none') ordered.add(`${attr(n, 'numId')}:${attr(lvl, 'ilvl')}`)
      }
    }
  }

  const picture = (el: Element): string => {
    const blip = el.getElementsByTagNameNS('*', 'blip')[0]
    const id = blip ? (attr(blip, 'embed') ?? '') : ''
    const r = rels.get(id)
    const data = r && !r.external ? files[r.target] : undefined
    const url = data ? dataUrl(data, r!.target) : null
    if (!url) return ''
    const ext = el.getElementsByTagNameNS('*', 'extent')[0]
    const w = ext ? Math.round(Number(attr(ext, 'cx') ?? 0) / 9525) : 0 // EMU → px
    return `<img src="${url}" alt=""${w ? ` style="width:${Math.min(w, 900)}px"` : ''}>`
  }

  const runs = (p: Element): string => {
    let out = ''
    for (const c of kids(p)) {
      if (c.localName === 'r') {
        const pr = kid(c, 'rPr')
        let text = ''
        for (const x of kids(c)) {
          if (x.localName === 't') text += esc(x.textContent ?? '')
          else if (x.localName === 'tab') text += '\t'
          else if (x.localName === 'br' || x.localName === 'cr') text += '<br>'
          else if (x.localName === 'drawing' || x.localName === 'pict') text += picture(x)
        }
        if (!text) continue
        const on = (n: string) => {
          const e = kid(pr, n)
          return !!e && attr(e, 'val') !== '0' && attr(e, 'val') !== 'false'
        }
        if (on('b')) text = `<strong>${text}</strong>`
        if (on('i')) text = `<em>${text}</em>`
        if (kid(pr, 'u') && attr(kid(pr, 'u'), 'val') !== 'none') text = `<u>${text}</u>`
        if (on('strike')) text = `<s>${text}</s>`
        const color = attr(kid(pr, 'color'), 'val')
        if (color && /^[0-9a-f]{6}$/i.test(color) && color.toLowerCase() !== '000000') text = `<span style="color:#${color}">${text}</span>`
        out += text
      } else if (c.localName === 'hyperlink') {
        const r = rels.get(attr(c, 'id') ?? '')
        const inner = runs(c)
        out += r?.external && /^https?:\/\//i.test(r.target) ? `<a href="${esc(r.target)}" target="_blank" rel="noopener noreferrer">${inner}</a>` : inner
      } else if (c.localName === 'ins' || c.localName === 'smartTag' || c.localName === 'sdt' || c.localName === 'sdtContent' || c.localName === 'fldSimple') {
        out += runs(c)
      }
    }
    return out
  }

  const table = (tbl: Element): string => {
    let html = '<table>'
    for (const tr of kids(tbl).filter((x) => x.localName === 'tr')) {
      html += '<tr>'
      for (const tc of kids(tr).filter((x) => x.localName === 'tc')) {
        const span = Number(attr(kid(kid(tc, 'tcPr'), 'gridSpan'), 'val') ?? 1)
        if (attr(kid(kid(tc, 'tcPr'), 'vMerge'), 'val') === null && kid(kid(tc, 'tcPr'), 'vMerge')) continue // merged down
        html += `<td${span > 1 ? ` colspan="${span}"` : ''}>${body(tc)}</td>`
      }
      html += '</tr>'
    }
    return html + '</table>'
  }

  // body turns a run of block elements into HTML, lists gathered.
  const body = (parent: Element): string => {
    let html = ''
    const stack: { tag: string; key: string }[] = [] // open lists
    const closeTo = (n: number) => {
      while (stack.length > n) html += `</li></${stack.pop()!.tag}>`
    }
    for (const el of kids(parent)) {
      if (el.localName === 'tbl') {
        closeTo(0)
        html += table(el)
        continue
      }
      if (el.localName === 'sdt') {
        closeTo(0)
        const content = kid(el, 'sdtContent')
        if (content) html += body(content)
        continue
      }
      if (el.localName !== 'p') continue
      const pr = kid(el, 'pPr')
      const style = (attr(kid(pr, 'pStyle'), 'val') ?? '').toLowerCase()
      const num = kid(pr, 'numPr')
      const content = runs(el)
      if (num) {
        const level = Number(attr(kid(num, 'ilvl'), 'val') ?? 0)
        const id = attr(kid(num, 'numId'), 'val') ?? ''
        const tag = ordered.has(`${id}:${level}`) ? 'ol' : 'ul'
        if (stack.length > level + 1) closeTo(level + 1)
        if (stack.length === level + 1) html += '</li><li>'
        while (stack.length < level + 1) {
          html += `<${tag}><li>`
          stack.push({ tag, key: id })
        }
        html += content
        continue
      }
      closeTo(0)
      const align = attr(kid(pr, 'jc'), 'val')
      const st = align === 'center' || align === 'right' ? ` style="text-align:${align}"` : ''
      const h = /^(heading|titre|überschrift)\s*([1-6])$/.exec(style) ?? /^heading([1-6])$/.exec(style)
      if (style === 'title') html += `<h1${st}>${content}</h1>`
      else if (style === 'subtitle') html += `<p class="subtitle"${st}>${content}</p>`
      else if (h) html += `<h${Math.min(6, Number(h[h.length - 1]) + 1)}${st}>${content}</h${Math.min(6, Number(h[h.length - 1]) + 1)}>`
      else if (style === 'quote' || style === 'intensequote') html += `<blockquote>${content}</blockquote>`
      else html += content ? `<p${st}>${content}</p>` : '<p><br></p>'
    }
    closeTo(0)
    return html
  }

  const b = doc.getElementsByTagNameNS('*', 'body')[0]
  return DOMPurify.sanitize(b ? body(b) : '', { ADD_ATTR: ['target'] })
}
