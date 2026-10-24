// Importing pages from elsewhere: Markdown files, a Notion export ("Markdown
// & CSV", the .zip — zips inside it too) and Apple Notes as exported (HTML,
// text or Markdown files, or a .zip of them). The files become a tree of
// pages: a file's folder holds its sub-pages (Notion's "Page <id>.md" and its
// "Page <id>/" folder), folders without a page become one (Apple Notes'
// folders), links between the files become links between the pages, and the
// images and files they use are uploaded to their page. A Notion database
// (CSV) becomes a page with its table, its rows the pages inside it.
//
// This module plans and converts; the runner (importPages) makes the pages
// through the API, as if typed in — permissions, search and all.
import { unzipSync } from 'fflate'

export interface InFile {
  path: string // within the import, "/"-separated, no leading slash
  data: Uint8Array
}

export type PageKind = 'md' | 'html' | 'txt' | 'csv'
export interface Planned {
  key: string // the page's own path without its extension (and Notion's _all)
  title: string
  parent: string | null // the parent's key
  kind: PageKind | 'folder'
  file?: InFile
  folder: string // where its files and sub-pages are
}

const PAGE_EXT: Record<string, PageKind> = { md: 'md', markdown: 'md', html: 'html', htm: 'html', txt: 'txt', csv: 'csv' }
const NOTION_ID = /\s+[0-9a-f]{32}$/i
const JUNK = /(^|\/)(__MACOSX|\.DS_Store|Thumbs\.db|desktop\.ini)(\/|$)/

const ext = (p: string) => (/\.([^./]+)$/.exec(p)?.[1] ?? '').toLowerCase()
const dirOf = (p: string) => (p.includes('/') ? p.slice(0, p.lastIndexOf('/')) : '')
const baseOf = (p: string) => p.slice(p.lastIndexOf('/') + 1)
const noExt = (p: string) => p.replace(/\.[^./]+$/, '')
export const cleanName = (s: string) => s.replace(NOTION_ID, '').trim()

// gather opens what was picked: zips (and zips in them) become their files.
export async function gather(picked: File[]): Promise<InFile[]> {
  const out: InFile[] = []
  const add = (path: string, data: Uint8Array, depth: number) => {
    path = path.replace(/\\/g, '/').replace(/^\/+/, '')
    if (!path || path.endsWith('/') || JUNK.test(path)) return
    if (ext(path) === 'zip' && depth < 3) {
      const inner = unzipSync(data)
      // A zip in a zip (Notion's "Part-1.zip"): its files sit where it was.
      const at = depth ? dirOf(path) : ''
      for (const [p, d] of Object.entries(inner)) add(at ? `${at}/${p}` : p, d, depth + 1)
      return
    }
    out.push({ path, data })
  }
  for (const f of picked) add(f.name, new Uint8Array(await f.arrayBuffer()), 0)
  return out
}

// plan lays the files out as pages: what each is called and where it goes.
export function plan(files: InFile[]): Planned[] {
  const pages = new Map<string, Planned>()
  // Notion writes a database twice (X.csv, X_all.csv): the _all one has every row.
  const csvAll = new Set(files.filter((f) => /_all\.csv$/i.test(f.path)).map((f) => f.path.replace(/_all\.csv$/i, '.csv')))
  for (const f of files) {
    const kind = PAGE_EXT[ext(f.path)]
    if (!kind || (kind === 'csv' && csvAll.has(f.path))) continue
    const key = noExt(f.path).replace(/_all$/i, '')
    // A note alone in a folder of its name (Apple's export) keeps its files there.
    const dir = dirOf(key)
    const folder = baseOf(dir) && baseOf(dir) === baseOf(key) && files.filter((x) => dirOf(x.path) === dir && PAGE_EXT[ext(x.path)]).length === 1 ? dir : key
    pages.set(key, { key, title: cleanName(baseOf(key)), parent: null, kind, file: f, folder })
  }
  // The folder of each page; then folders that hold pages but are none.
  const byFolder = new Map<string, string>()
  for (const p of pages.values()) byFolder.set(p.folder, p.key)
  const dirs = new Set<string>()
  for (const p of [...pages.values()]) {
    for (let d = dirOf(p.folder === p.key ? p.key : p.folder); d; d = dirOf(d)) dirs.add(d)
  }
  // The one folder everything is in (an export's wrapper) is no page.
  const tops = new Set([...pages.values()].map((p) => p.key.split('/')[0]))
  const wrapper = tops.size === 1 && [...pages.values()].every((p) => p.key.includes('/')) ? [...tops][0] : ''
  for (const d of [...dirs].sort((a, b) => a.length - b.length)) {
    if (d === wrapper || byFolder.has(d)) continue
    const key = d
    pages.set(key, { key, title: cleanName(baseOf(d)), parent: null, kind: 'folder', folder: d })
    byFolder.set(d, key)
  }
  for (const p of pages.values()) {
    const outer = p.folder === p.key ? dirOf(p.key) : dirOf(p.folder)
    p.parent = byFolder.get(outer) ?? null
  }
  // Parents before their pages.
  const depth = (p: Planned): number => (p.parent ? 1 + depth(pages.get(p.parent)!) : 0)
  return [...pages.values()].sort((a, b) => depth(a) - depth(b) || a.key.localeCompare(b.key))
}

// resolve is the path a link in a file at `dir` points to (decoded), or ''
// for links out (web, mail, this app, anchors).
export function resolve(dir: string, href: string): string {
  if (!href || /^[a-z][\w+.-]*:/i.test(href) || href.startsWith('#') || href.startsWith('/')) return ''
  let p = href.split('#')[0].split('?')[0]
  try {
    p = decodeURIComponent(p)
  } catch {
    /* keep it as written */
  }
  const parts = (dir ? dir.split('/') : []).concat(p.split('/'))
  const out: string[] = []
  for (const s of parts) {
    if (s === '..') out.pop()
    else if (s && s !== '.') out.push(s)
  }
  return out.join('/')
}

// The Markdown links and images of a text: [text](href "title") / ![alt](src).
const MD_LINK = /(!?)\[((?:\\.|[^\]\\])*)\]\(\s*<?([^)\s>]+)>?(\s+"[^"]*")?\s*\)/g

// rewriteLinks swaps each link's target for to(path) when it gives one.
export async function rewriteLinks(md: string, dir: string, to: (path: string, image: boolean) => Promise<string | null>): Promise<string> {
  const found = [...md.matchAll(MD_LINK)]
  let out = ''
  let last = 0
  for (const m of found) {
    const target = resolve(dir, m[3])
    const url = target ? await to(target, m[1] === '!') : null
    out += md.slice(last, m.index!) + (url ? `${m[1]}[${m[2]}](${url}${m[4] ?? ''})` : m[0])
    last = m.index! + m[0].length
  }
  return out + md.slice(last)
}

// titleOf takes a Markdown page's "# Title" off its top (Notion repeats the
// page's name there).
export function splitTitle(md: string): { title: string; body: string } {
  const text = md.replace(/^﻿/, '').replace(/\r\n?/g, '\n')
  const m = /^\s*#\s+(.+?)\s*#*\s*(?:\n|$)/.exec(text)
  return m ? { title: m[1].trim(), body: text.slice(m[0].length).replace(/^\n+/, '') } : { title: '', body: text }
}

// csvRows reads CSV (RFC 4180).
export function csvRows(text: string): string[][] {
  const rows: string[][] = []
  let row: string[] = []
  let cell = ''
  let quoted = false
  const s = text.replace(/^﻿/, '')
  for (let i = 0; i < s.length; i++) {
    const c = s[i]
    if (quoted) {
      if (c === '"' && s[i + 1] === '"') {
        cell += '"'
        i++
      } else if (c === '"') quoted = false
      else cell += c
    } else if (c === '"') quoted = true
    else if (c === ',') {
      row.push(cell)
      cell = ''
    } else if (c === '\n' || c === '\r') {
      if (c === '\r' && s[i + 1] === '\n') i++
      row.push(cell)
      rows.push(row)
      row = []
      cell = ''
    } else cell += c
  }
  if (cell || row.length) {
    row.push(cell)
    rows.push(row)
  }
  return rows.filter((r) => r.some((x) => x.trim()))
}

// csvTable writes CSV as a Markdown table.
export function csvTable(text: string): string {
  const rows = csvRows(text)
  if (!rows.length) return ''
  const n = Math.max(...rows.map((r) => r.length))
  const cell = (s = '') => s.replace(/\|/g, '\\|').replace(/\r?\n/g, '<br>').trim() || ' '
  const line = (r: string[]) => `| ${Array.from({ length: n }, (_, i) => cell(r[i])).join(' | ')} |`
  return [line(rows[0]), `|${' --- |'.repeat(n)}`, ...rows.slice(1).map(line)].join('\n')
}

// MIME types for uploads, by extension.
const MIME: Record<string, string> = {
  png: 'image/png', jpg: 'image/jpeg', jpeg: 'image/jpeg', gif: 'image/gif', webp: 'image/webp', svg: 'image/svg+xml', heic: 'image/heic',
  pdf: 'application/pdf', txt: 'text/plain', csv: 'text/csv', mp4: 'video/mp4', mov: 'video/quicktime', mp3: 'audio/mpeg', m4a: 'audio/mp4',
  zip: 'application/zip', docx: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  xlsx: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', pptx: 'application/vnd.openxmlformats-officedocument.presentationml.presentation',
}
export const mimeOf = (path: string) => MIME[ext(path)] ?? 'application/octet-stream'
export const isImage = (path: string) => mimeOf(path).startsWith('image/')
export { baseOf, dirOf }
