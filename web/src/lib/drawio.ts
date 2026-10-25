// draw.io: the editor's address for Diagrams (it runs in a frame and talks
// by messages — https://www.drawio.com/doc/faq/embed-mode), and reading its
// files: the labels (for search), a file out of an .svg draw.io wrote, a
// picture handed over as a data: URL.
import { inflateSync, strFromU8 } from 'fflate'

// editorUrl is the editor's address: embedded, talking JSON, autosaving,
// without its own Save and Exit buttons, in the app's language and light.
export function editorUrl(base: string, lang: string, dark: boolean): string {
  const q = new URLSearchParams({ embed: '1', proto: 'json', spin: '1', libraries: '1', noSaveBtn: '1', noExitBtn: '1', saveAndExit: '0', lang, dark: dark ? '1' : '0' })
  return `${base.replace(/\/+$/, '')}/?${q}`
}

// originOf: an address's origin ("" when it isn't one).
export function originOf(url: string): string {
  try {
    return new URL(url).origin
  } catch {
    return ''
  }
}

const parseXml = (s: string) => new DOMParser().parseFromString(s, 'application/xml')

// pages are a file's diagrams (draw.io's pages) as their models' XML; a
// page draw.io kept compressed (base64 of deflate of the URL-encoded XML) is
// opened.
function pages(xml: string): { name: string; model: Element | null }[] {
  const doc = parseXml(xml)
  if (doc.getElementsByTagName('parsererror').length) return []
  const root = doc.documentElement
  if (root.tagName === 'mxGraphModel') return [{ name: '', model: root }]
  return [...doc.getElementsByTagName('diagram')].map((d) => {
    const name = d.getAttribute('name') ?? ''
    const inner = d.getElementsByTagName('mxGraphModel')[0]
    if (inner) return { name, model: inner }
    const packed = (d.textContent ?? '').trim()
    if (!packed) return { name, model: null }
    try {
      const bin = Uint8Array.from(atob(packed), (c) => c.charCodeAt(0))
      const m = parseXml(decodeURIComponent(strFromU8(inflateSync(bin)))).documentElement
      return { name, model: m.tagName === 'mxGraphModel' ? m : null }
    } catch {
      return { name, model: null }
    }
  })
}

// A label's words: draw.io labels may be HTML.
function words(label: string): string {
  if (!/[<&]/.test(label)) return label
  return new DOMParser().parseFromString(label, 'text/html').body.textContent ?? ''
}

// labelsOf is a file's words: its pages' names and its shapes' labels (and
// their properties), one line each, for search.
export function labelsOf(xml: string): string {
  const out: string[] = []
  for (const p of pages(xml)) {
    if (p.name && !/^Page-\d+$/.test(p.name)) out.push(p.name)
    if (!p.model) continue
    for (const el of p.model.querySelectorAll('mxCell, UserObject, object')) {
      for (const attr of el.tagName === 'mxCell' ? ['value'] : [...el.getAttributeNames()].filter((a) => a !== 'id' && a !== 'placeholders')) {
        const v = words(el.getAttribute(attr) ?? '').replace(/\s+/g, ' ').trim()
        if (v) out.push(v)
      }
    }
  }
  return [...new Set(out)].join('\n')
}

// isDrawio: text that is a draw.io file.
export const isDrawio = (s: string) => /^\s*<(mxfile|mxGraphModel)[\s>]/.test(s)

// fileOf finds a draw.io file in what was opened: a .drawio / .xml as it
// is, an .svg draw.io wrote (the file is in its "content"); else "".
export function fileOf(text: string): string {
  if (isDrawio(text)) return text.trim()
  if (/<svg[\s>]/.test(text)) {
    const svg = parseXml(text).documentElement
    const content = svg.getAttribute('content') ?? ''
    if (isDrawio(content)) return content
  }
  return ''
}

// bytesOfDataUrl reads a data: URL (base64 or not): its bytes and type.
// (Not with fetch: the CSP's connect-src keeps fetch to Orbit.)
export function bytesOfDataUrl(url: string): { bytes: Uint8Array; type: string } {
  const m = /^data:([^;,]*)[^,]*?(;base64)?,(.*)$/s.exec(url)
  if (!m) return { bytes: new Uint8Array(), type: '' }
  const bytes = m[2] ? Uint8Array.from(atob(m[3]), (c) => c.charCodeAt(0)) : new TextEncoder().encode(decodeURIComponent(m[3]))
  return { bytes, type: m[1] }
}

// textOfDataUrl reads a data: URL's text.
export const textOfDataUrl = (url: string) => new TextDecoder().decode(bytesOfDataUrl(url).bytes)

// lightPicture keeps a draw.io SVG in its light colours (it follows the
// screen's dark mode otherwise; Orbit shows diagrams on white).
export const lightPicture = (svg: string) => svg.replace(/color-scheme:\s*light dark/g, 'color-scheme: light')

// svgUrl is an SVG as a data: URL, for an <img> (where its scripts, if
// any, never run).
export const svgUrl = (svg: string) => `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`
