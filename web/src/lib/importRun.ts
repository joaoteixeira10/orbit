// The import's second half: the planned pages made through the API (parents
// first), their files uploaded to them, links pointed at the new pages.
// HTML (Apple Notes, Notion's HTML export) becomes Markdown through the
// visual editor's own schema, so it reads as if written here.
import { Editor } from '@tiptap/core'
import { pageExtensions } from '@/components/editor/extensions'
import { pagesApi } from '@/api/planner'
import { uploadFile } from './uploads'
import { normalizeMarkdown, readFrontmatter, textToMarkdown, type PageField } from './markdown'
import { baseOf, cleanName, csvRows, dirOf, isImage, mimeOf, plan, resolve, rewriteLinks, splitTitle, type InFile, type Planned } from './importPages'
import { guessType, importValue, newId, type Database, type Props } from './database'
import { i18n } from '@/i18n'

export interface ImportResult {
  pages: number
  files: number
  failed: string[] // files that couldn't be read or saved
}

const decoder = new TextDecoder()

// Lists of to-dos as other apps write them in HTML become the editor's.
function taskLists(doc: Document) {
  for (const li of doc.querySelectorAll('li')) {
    const box = li.querySelector(':scope > input[type=checkbox], :scope > label > input[type=checkbox], :scope > .checkbox, :scope > div > .checkbox')
    const ul = li.parentElement
    const checklist = !!ul?.matches('ul.checklist, ul.to-do-list, ul[data-type=taskList]')
    if (!box && !checklist) continue
    const checked = box instanceof HTMLInputElement ? box.checked || box.hasAttribute('checked') : !!(box?.matches('.checkbox-on') || li.matches('.checked, .done, [data-checked=true]'))
    box?.remove()
    ul?.setAttribute('data-type', 'taskList')
    li.setAttribute('data-type', 'taskItem')
    li.setAttribute('data-checked', String(checked))
    if (!li.querySelector(':scope > p')) {
      const p = doc.createElement('p')
      while (li.firstChild) p.append(li.firstChild)
      li.append(p)
    }
  }
}

// htmlToMarkdown reads an HTML page into Markdown (its images and links
// already pointed where they should).
export function htmlToMarkdown(body: string): string {
  const ed = new Editor({ extensions: pageExtensions(), content: body, contentType: 'html' })
  const md = normalizeMarkdown(ed.getMarkdown())
  ed.destroy()
  return md
}

function dataUrlFile(src: string, n: number): File | null {
  const m = /^data:([\w/+.-]+);base64,(.*)$/s.exec(src)
  if (!m) return null
  const bin = atob(m[2].replace(/\s+/g, ''))
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  return new File([bytes], `image-${n}.${m[1].split('/')[1]?.replace('jpeg', 'jpg') ?? 'png'}`, { type: m[1] })
}

// A Notion database (its CSV): the database it becomes, and its rows'
// values by name (rows with no page of their own become pages).
interface Table {
  db: Database
  rows: Map<string, Props>
  headers: string[]
}
function tableOf(text: string): Table {
  const rows = csvRows(text)
  const headers = rows[0] ?? []
  const data = rows.slice(1)
  const props = headers.slice(1).map((h, i) => ({ id: newId(), name: h.trim() || `#${i + 2}`, type: guessType(data.map((r) => r[i + 1] ?? '')) }))
  const values = new Map<string, Props>()
  for (const r of data) {
    const v: Props = {}
    props.forEach((p, i) => {
      const x = importValue(p, r[i + 1] ?? '')
      if (x !== undefined) v[p.id] = x
    })
    values.set(cleanName(r[0] ?? ''), v)
  }
  return { db: { props, views: [{ id: newId(), name: i18n.global.t('db.table'), type: 'table' }] }, rows: values, headers }
}
// Notion writes a row's values at the top of its page ("Status: Done"):
// they're properties now.
function dropPropLines(md: string, headers: string[]): string {
  const names = new Set(headers.map((h) => h.trim().toLowerCase()))
  const lines = md.split('\n')
  let i = 0
  while (i < lines.length && (lines[i].trim() === '' || names.has(lines[i].split(':')[0].trim().toLowerCase()))) i++
  return i ? lines.slice(i).join('\n') : md
}

// importPages makes the pages under `parent` (null: at the top), telling
// progress(done, total) as it goes.
export async function importPages(ws: string, files: InFile[], parent: string | null, progress: (done: number, total: number) => void): Promise<ImportResult> {
  const planned = plan(files)
  const byPath = new Map(files.map((f) => [f.path, f]))
  const res: ImportResult = { pages: 0, files: 0, failed: [] }
  const ids = new Map<string, string>() // planned key → page id
  // Paths that are pages: their files (Notion links "X.md" and "X.csv").
  const pageOf = new Map<string, string>()
  for (const p of planned) {
    pageOf.set(p.key, p.key)
    if (p.file) pageOf.set(p.file.path, p.key)
    if (p.kind === 'csv') pageOf.set(`${p.key}_all.csv`, p.key)
  }
  const tables = new Map<string, Table>()
  for (const p of planned) if (p.kind === 'csv' && p.file) tables.set(p.key, tableOf(decoder.decode(p.file.data)))
  const total = planned.length * 2
  let done = 0
  const tick = () => progress(++done, total)

  // 1. The pages, with their titles (parents first).
  for (const p of planned) {
    try {
      let title = p.title
      if (p.kind === 'md' && p.file) title = splitTitle(decoder.decode(p.file.data)).title || title
      if (p.kind === 'html' && p.file) {
        const t = /<title[^>]*>([^<]*)<\/title>/i.exec(decoder.decode(p.file.data))?.[1]?.trim()
        if (t) title = t
      }
      const pid = p.parent ? ids.get(p.parent) : parent
      // A database (Notion's CSV); a row of one gets its values.
      const table = tables.get(p.key)
      const of = p.parent ? tables.get(p.parent) : undefined
      const values = of?.rows.get(title)
      if (values) of!.rows.delete(title)
      const page = await pagesApi.create(ws, { title: title.slice(0, 300), parent_id: pid ?? null, db: table?.db, props: values && Object.keys(values).length ? values : undefined })
      ids.set(p.key, page.id)
      res.pages++
    } catch {
      res.failed.push(p.file?.path ?? p.key)
    }
    tick()
  }

  // Rows of a database with no page of their own.
  for (const [key, table] of tables) {
    const id = ids.get(key)
    if (!id) continue
    for (const [title, values] of table.rows) {
      try {
        await pagesApi.create(ws, { title: title.slice(0, 300) || i18n.global.t('pages.untitled'), parent_id: id, props: Object.keys(values).length ? values : undefined })
        res.pages++
      } catch {
        res.failed.push(title)
      }
    }
  }

  // 2. Their documents: links to the new pages, files uploaded.
  for (const p of planned) {
    const id = ids.get(p.key)
    if (!id) {
      tick()
      continue
    }
    try {
      const uploaded = new Map<string, string>()
      const used = new Set<string>()
      const upload = async (path: string): Promise<string | null> => {
        if (uploaded.has(path)) return uploaded.get(path)!
        const f = byPath.get(path)
        if (!f) return null
        const u = await uploadFile('page', id, new File([f.data as BlobPart], baseOf(path), { type: mimeOf(path) }))
        res.files++
        uploaded.set(path, u.url)
        return u.url
      }
      const to = async (path: string): Promise<string | null> => {
        const key = pageOf.get(path)
        if (key && ids.get(key)) return `/w/${ws}/p/${ids.get(key)}`
        used.add(path)
        return upload(path).catch(() => null)
      }
      let md = ''
      let text = p.file ? decoder.decode(p.file.data) : ''
      // A Markdown file's YAML header (Obsidian's and the like): its tags and properties.
      let meta: { tags: string[]; fields: PageField[] } | null = null
      if (p.kind === 'md') {
        const fm = readFrontmatter(text.replace(/^\uFEFF/, '').replace(/\r\n?/g, '\n'))
        if (fm.tags.length || fm.fields.length) meta = { tags: fm.tags, fields: fm.fields }
        text = fm.rest
      }
      const dir = p.file ? dirOf(p.file.path) : p.key
      const of = p.parent ? tables.get(p.parent) : undefined
      if (p.kind === 'md') md = await rewriteLinks(of ? dropPropLines(splitTitle(text).body, of.headers) : splitTitle(text).body, dir, to)
      else if (p.kind === 'txt') md = textToMarkdown(text.replace(/^﻿/, '').replace(/\r\n?/g, '\n'))
      else if (p.kind === 'csv') md = '' // a database: its rows show
      else if (p.kind === 'html') {
        const doc = new DOMParser().parseFromString(text, 'text/html')
        doc.querySelectorAll('script, style, head, title').forEach((x) => x.remove())
        // Notion's HTML repeats the title at the top.
        doc.querySelector('header h1.page-title')?.closest('header')?.remove()
        taskLists(doc)
        let n = 0
        for (const img of doc.querySelectorAll('img')) {
          const src = img.getAttribute('src') ?? ''
          let url: string | null = null
          const inline = src.startsWith('data:') ? dataUrlFile(src, ++n) : null
          if (inline) {
            const u = await uploadFile('page', id, inline)
            res.files++
            url = u.url
          } else if (resolve(dir, src)) url = await to(resolve(dir, src))
          if (url) img.setAttribute('src', url)
          else if (!/^https?:/i.test(src)) img.remove()
        }
        for (const a of doc.querySelectorAll('a[href]')) {
          const target = resolve(dir, a.getAttribute('href') ?? '')
          if (!target) continue
          const url = await to(target)
          if (url) a.setAttribute('href', url)
        }
        md = htmlToMarkdown(doc.body.innerHTML)
      }
      // Files of its folder nothing links to (Apple's attachments): at the end.
      const extra = files.filter((f) => dirOf(f.path) === p.folder && !pageOf.has(f.path) && !used.has(f.path) && !/\.(md|markdown|html?|txt|csv)$/i.test(f.path))
      const tail: string[] = []
      for (const f of extra) {
        const url = await upload(f.path).catch(() => null)
        if (url) tail.push(isImage(f.path) ? `![${baseOf(f.path)}](${url})` : `[${baseOf(f.path)}](${url})`)
      }
      if (tail.length) md = `${md}${md ? '\n\n' : ''}${tail.join('\n\n')}`
      if (md.trim()) await pagesApi.update(id, { content: md, version: 1 })
      if (meta) await pagesApi.update(id, meta)
    } catch {
      res.failed.push(p.file?.path ?? p.key)
    }
    tick()
  }
  return res
}

export type { Planned }
