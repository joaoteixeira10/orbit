// A PowerPoint file (.pptx) as slides to look at: each slide's text boxes
// and pictures where they sit (a placeholder without a place of its own
// takes its layout's, else its master's), the text's size, weight, colour
// and alignment, the slide's background colour — read from the XML inside
// (unzipped with fflate). Charts, SmartArt and effects aren't drawn: Download
// opens the real thing.
import { strFromU8, unzipSync } from 'fflate'
import { dataUrl, relsOf } from './docx'

export interface PptRun {
  text: string
  bold?: boolean
  italic?: boolean
  underline?: boolean
  size?: number // points
  color?: string
}
export interface PptPara {
  runs: PptRun[]
  align?: 'left' | 'center' | 'right' | 'justify'
  level: number
  bullet: boolean
}
export interface PptShape {
  x: number // fractions of the slide
  y: number
  w: number
  h: number
  fill?: string
  picture?: string // a data: URL
  paras?: PptPara[]
  table?: string[][]
  title?: boolean
}
export interface PptSlide {
  background?: string
  shapes: PptShape[]
}
export interface PptDeck {
  width: number // points
  height: number
  slides: PptSlide[]
}

const kids = (el: Element | null | undefined) => (el ? [...el.children] : [])
const kid = (el: Element | null | undefined, name: string) => kids(el).find((c) => c.localName === name) ?? null
const attr = (el: Element | null | undefined, name: string) => {
  if (!el) return null
  for (const a of [...el.attributes]) if (a.localName === name) return a.value
  return null
}
const deep = (el: Element | null | undefined, name: string) => (el ? el.getElementsByTagNameNS('*', name)[0] ?? null : null)
const xmlOf = (files: Record<string, Uint8Array>, part: string) => (files[part] ? new DOMParser().parseFromString(strFromU8(files[part]), 'application/xml') : null)

const ALIGN: Record<string, PptPara['align']> = { l: 'left', ctr: 'center', r: 'right', just: 'justify' }

// A colour: an explicit one (srgbClr); theme colours fall back to none.
function colorOf(el: Element | null): string | undefined {
  const fill = el && (el.localName === 'solidFill' ? el : kid(el, 'solidFill'))
  const v = attr(kid(fill, 'srgbClr'), 'val')
  return v && /^[0-9a-f]{6}$/i.test(v) ? `#${v}` : undefined
}

interface Frame {
  x: number
  y: number
  w: number
  h: number
}
function frameOf(xfrm: Element | null): Frame | null {
  const off = kid(xfrm, 'off')
  const ext = kid(xfrm, 'ext')
  if (!off || !ext) return null
  return { x: Number(attr(off, 'x') ?? 0), y: Number(attr(off, 'y') ?? 0), w: Number(attr(ext, 'cx') ?? 0), h: Number(attr(ext, 'cy') ?? 0) }
}

// A placeholder's key: its type (body when none) and index.
const phKey = (sp: Element) => {
  const ph = deep(kid(sp, 'nvSpPr') ?? kid(sp, 'nvPicPr'), 'ph')
  if (!ph) return null
  return { type: attr(ph, 'type') ?? 'body', idx: attr(ph, 'idx') ?? '' }
}

// The places of a layout's (or master's) placeholders.
function placesOf(doc: Document | null): { type: string; idx: string; frame: Frame }[] {
  const out: { type: string; idx: string; frame: Frame }[] = []
  for (const sp of doc ? [...doc.getElementsByTagNameNS('*', 'sp')] : []) {
    const k = phKey(sp)
    const f = frameOf(deep(kid(sp, 'spPr'), 'xfrm'))
    if (k && f) out.push({ ...k, frame: f })
  }
  return out
}
function placeFor(places: { type: string; idx: string; frame: Frame }[], k: { type: string; idx: string }): Frame | null {
  const same = (t: string) => (t === 'ctrTitle' ? 'title' : t === 'subTitle' ? 'body' : t)
  return (
    places.find((p) => k.idx && p.idx === k.idx)?.frame ??
    places.find((p) => same(p.type) === same(k.type))?.frame ??
    null
  )
}

function parasOf(body: Element | null, title: boolean): PptPara[] {
  return kids(body)
    .filter((p) => p.localName === 'p')
    .map((p) => {
      const pPr = kid(p, 'pPr')
      const runs: PptRun[] = []
      for (const r of kids(p)) {
        if (r.localName === 'br') runs.push({ text: '\n' })
        if (r.localName !== 'r' && r.localName !== 'fld') continue
        const rPr = kid(r, 'rPr')
        const sz = attr(rPr, 'sz')
        runs.push({
          text: kid(r, 't')?.textContent ?? '',
          bold: attr(rPr, 'b') === '1',
          italic: attr(rPr, 'i') === '1',
          underline: !!attr(rPr, 'u') && attr(rPr, 'u') !== 'none',
          size: sz ? Number(sz) / 100 : undefined,
          color: colorOf(rPr),
        })
      }
      const level = Number(attr(pPr, 'lvl') ?? 0)
      const bullet = !title && !kid(pPr, 'buNone') && (!!kid(pPr, 'buChar') || !!kid(pPr, 'buAutoNum') || level > 0)
      return { runs, align: ALIGN[attr(pPr, 'algn') ?? ''], level, bullet }
    })
}

export function readPptx(bytes: Uint8Array): PptDeck {
  const files = unzipSync(bytes)
  const pres = xmlOf(files, 'ppt/presentation.xml')
  if (!pres) throw new Error('not a PowerPoint file')
  const size = deep(pres.documentElement, 'sldSz')
  const cx = Number(attr(size, 'cx') ?? 12192000)
  const cy = Number(attr(size, 'cy') ?? 6858000)
  const presRels = relsOf(files, 'ppt/presentation.xml')
  // Each slide's r:id (it also has a plain id).
  const relId = (el: Element) => [...el.attributes].find((a) => a.localName === 'id' && a.prefix)?.value ?? ''
  const slideParts = [...pres.getElementsByTagNameNS('*', 'sldId')].map((s) => presRels.get(relId(s))?.target).filter((t): t is string => !!t && !!files[t])

  const slides: PptSlide[] = slideParts.map((part) => {
    const doc = xmlOf(files, part)!
    const rels = relsOf(files, part)
    const layoutPart = [...rels.values()].find((r) => /slideLayouts\//.test(r.target))?.target
    const layout = layoutPart ? xmlOf(files, layoutPart) : null
    const masterPart = layoutPart ? [...relsOf(files, layoutPart).values()].find((r) => /slideMasters\//.test(r.target))?.target : undefined
    const master = masterPart ? xmlOf(files, masterPart) : null
    const layoutPlaces = placesOf(layout)
    const masterPlaces = placesOf(master)
    const shapes: PptShape[] = []
    const pictureOf = (el: Element) => {
      const r = rels.get(attr(deep(el, 'blip'), 'embed') ?? '')
      const data = r && !r.external ? files[r.target] : undefined
      return data ? (dataUrl(data, r!.target) ?? undefined) : undefined
    }
    const add = (f: Frame | null, s: Omit<PptShape, 'x' | 'y' | 'w' | 'h'>) => {
      if (f && f.w > 0 && f.h > 0) shapes.push({ x: f.x / cx, y: f.y / cy, w: f.w / cx, h: f.h / cy, ...s })
    }
    // walk a shape tree; a group maps its children's frames into its own.
    const walk = (tree: Element, map: (f: Frame) => Frame) => {
      for (const el of kids(tree)) {
        if (el.localName === 'sp') {
          const k = phKey(el)
          let f = frameOf(deep(kid(el, 'spPr'), 'xfrm'))
          f = f ? map(f) : k ? (placeFor(layoutPlaces, k) ?? placeFor(masterPlaces, k)) : null
          const title = !!k && /title/i.test(k.type)
          const paras = parasOf(kid(el, 'txBody'), title)
          const fill = colorOf(kid(el, 'spPr'))
          if (paras.some((p) => p.runs.some((r) => r.text.trim())) || fill) add(f, { paras, fill, title })
        } else if (el.localName === 'pic') {
          const f = frameOf(deep(kid(el, 'spPr'), 'xfrm'))
          const src = pictureOf(el)
          if (src) add(f ? map(f) : null, { picture: src })
        } else if (el.localName === 'graphicFrame') {
          const f = frameOf(kid(el, 'xfrm'))
          const tbl = deep(el, 'tbl')
          if (tbl) {
            const rows = kids(tbl).filter((r) => r.localName === 'tr').map((tr) => kids(tr).filter((c) => c.localName === 'tc').map((tc) => tc.textContent?.trim() ?? ''))
            add(f ? map(f) : null, { table: rows })
          }
        } else if (el.localName === 'grpSp') {
          const x = deep(kid(el, 'grpSpPr'), 'xfrm')
          const g = frameOf(x)
          const chOff = kid(x, 'chOff')
          const chExt = kid(x, 'chExt')
          if (!g || !chOff || !chExt) {
            walk(el, map)
            continue
          }
          const ox = Number(attr(chOff, 'x') ?? 0)
          const oy = Number(attr(chOff, 'y') ?? 0)
          const sx = g.w / (Number(attr(chExt, 'cx') ?? 0) || g.w || 1)
          const sy = g.h / (Number(attr(chExt, 'cy') ?? 0) || g.h || 1)
          walk(el, (f) => map({ x: g.x + (f.x - ox) * sx, y: g.y + (f.y - oy) * sy, w: f.w * sx, h: f.h * sy }))
        }
      }
    }
    const tree = deep(doc.documentElement, 'spTree')
    if (tree) walk(tree, (f) => f)
    const bg = colorOf(deep(deep(doc.documentElement, 'bg'), 'bgPr')) ?? colorOf(deep(deep(master?.documentElement, 'bg'), 'bgPr'))
    return { background: bg, shapes }
  })
  return { width: cx / 12700, height: cy / 12700, slides }
}
