// What can be done to a file, whatever its kind: open it, its link,
// rename, copy, download, delete — and bring a file in from the computer.
import type { RouteLocationRaw } from 'vue-router'
import { sheetsApi } from './sheetsApi'
import { diagramsApi } from './diagramsApi'
import { drawingsApi } from '@/components/draw/drawing'
import { fileName } from './markdown'
import { locale, todayStr } from './dates'
import { fileOf, labelsOf, lightPicture } from './drawio'
import { kindOf as mermaidKind, labelsOf as mermaidLabels } from './mermaid'
import { textOf } from './sheet'
import { pagesApi } from '@/api/planner'
import { filesApi, type FileItem, type FileKind } from './filesApi'
import { downloadUpload } from './uploads'

// onRoute: where an attachment's page, task or note opens.
export function onRoute(ws: string, kind: string, id: string): RouteLocationRaw {
  if (kind === 'page') return { name: 'ws-page', params: { ws, page: id } }
  if (kind === 'task') return { name: 'ws-tasks', params: { ws, list: 'all' }, query: { task: id } }
  return ws ? { name: 'ws-notes', params: { ws }, query: { note: id } } : { name: 'my-notes', query: { note: id } }
}

export function routeOf(ws: string, f: { kind: FileKind; id: string }): RouteLocationRaw {
  if (ws === 'me') return { name: 'my-drawing', params: { id: f.id } } // My files: a Quick Draw sketch
  if (f.kind === 'sheet') return { name: 'ws-sheet', params: { ws, id: f.id } }
  if (f.kind === 'diagram') return { name: 'ws-diagram', params: { ws, id: f.id } }
  if (f.kind === 'page') return { name: 'ws-page', params: { ws, page: f.id } }
  return { name: 'ws-whiteboard', params: { ws, id: f.id } }
}

// linkOf: its address (on its own line in a page, it shows there).
export function linkOf(ws: string, f: { kind: FileKind; id: string; upload?: { url: string } }): string {
  if (f.kind === 'file') return `${location.origin}${f.upload?.url ?? ''}`
  if (ws === 'me') return `${location.origin}/draw/${f.id}`
  const path = f.kind === 'sheet' ? `s/${f.id}` : f.kind === 'diagram' ? `d/${f.id}` : f.kind === 'page' ? `p/${f.id}` : `whiteboards/${f.id}`
  return `${location.origin}/w/${ws}/${path}`
}

export async function rename(f: FileItem, title: string): Promise<void> {
  if (f.kind === 'file') await filesApi.updateFile(f.id, { name: title })
  else if (f.kind === 'sheet') await sheetsApi.save(f.id, { title, version: 0 })
  else if (f.kind === 'diagram') await diagramsApi.save(f.id, { title })
  else if (f.kind === 'page') await pagesApi.update(f.id, { title })
  else await drawingsApi.save(f.id, { title, version: 0 })
}

export async function remove(f: FileItem): Promise<void> {
  if (f.kind === 'file') await filesApi.removeFile(f.id) // to the trash, every version
  else if (f.kind === 'sheet') await sheetsApi.remove(f.id)
  else if (f.kind === 'diagram') await diagramsApi.remove(f.id)
  else if (f.kind === 'page') await pagesApi.trashPage(f.id) // to the trash, with the pages inside it
  else await drawingsApi.remove(f.id)
}

// duplicate makes a copy next to it, named "… (copy)".
export async function duplicate(ws: string, f: FileItem, copyWord: string): Promise<{ kind: FileKind; id: string }> {
  const title = `${f.title || ''} (${copyWord})`.trim()
  if (f.kind === 'page') return { kind: 'page', id: (await pagesApi.duplicate(f.id)).id } // the server names it, in the same folder
  if (f.kind === 'sheet') {
    const s = await sheetsApi.get(f.id)
    const c = await sheetsApi.create(ws, { title, data: s.data, text: s.data ? textOf(s.data) : '', folder_id: f.folder_id })
    return { kind: 'sheet', id: c.id }
  }
  if (f.kind === 'diagram') {
    const d = await diagramsApi.get(f.id)
    const text = !d.data ? '' : d.kind === 'mermaid' ? mermaidLabels(d.data) : labelsOf(d.data)
    const c = await diagramsApi.create(ws, { title, kind: d.kind, data: d.data, preview: d.preview, text, folder_id: f.folder_id })
    return { kind: 'diagram', id: c.id }
  }
  if (f.kind === 'file') throw new Error('a file is copied by downloading it')
  const d = await drawingsApi.get(f.id)
  const c = await drawingsApi.make(ws, { title, scene: d.scene, preview: d.preview, folder_id: f.folder_id })
  return { kind: 'board', id: c.id }
}

function save(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob)
  const a = Object.assign(document.createElement('a'), { href: url, download: name })
  document.body.append(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

// download saves it as a file the matching app opens: Excel (.xlsx),
// draw.io (.drawio) or Mermaid (.mmd), Excalidraw (.excalidraw), Markdown (.md).
export async function download(f: FileItem, untitled: string): Promise<void> {
  if (f.kind === 'file') return downloadUpload(f.upload!.url, f.title || untitled)
  const name = fileName(f.title || untitled)
  if (f.kind === 'page') {
    const p = await pagesApi.get(f.id)
    save(new Blob([`# ${p.title || untitled}\n\n${p.content}`], { type: 'text/markdown;charset=utf-8' }), `${name}.md`)
  } else if (f.kind === 'sheet') {
    const s = await sheetsApi.get(f.id)
    const [{ writeXlsx }, { Book, readData }, { dayOf }] = await Promise.all([import('./xlsx'), import('./sheet'), import('./formula')])
    const data = readData(s.data, 'Sheet 1')
    save(new Blob([writeXlsx(data, new Book(data, dayOf(todayStr()), locale().startsWith('pt'))) as BlobPart], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }), `${name}.xlsx`)
  } else if (f.kind === 'diagram') {
    const d = await diagramsApi.get(f.id)
    if (d.kind === 'mermaid') save(new Blob([d.data ?? ''], { type: 'text/plain' }), `${name}.mmd`)
    else save(new Blob([d.data ?? ''], { type: 'application/vnd.jgraph.mxfile' }), `${name}.drawio`)
  } else {
    const d = await drawingsApi.get(f.id)
    save(new Blob([d.scene || '{"type":"excalidraw","version":2,"elements":[],"appState":{},"files":{}}'], { type: 'application/json' }), `${name}.excalidraw`)
  }
}

// kindOfFile: which kind a file from the computer becomes (none: not one Orbit reads).
export function kindOfFile(name: string): FileKind | null {
  if (/\.(xlsx|csv|tsv)$/i.test(name)) return 'sheet'
  if (/\.(drawio|dio|xml|mmd|mermaid)$/i.test(name) || /\.drawio\.svg$/i.test(name)) return 'diagram'
  if (/\.excalidraw$/i.test(name)) return 'board'
  if (/\.svg$/i.test(name)) return 'diagram' // if draw.io saved it with its diagram
  return null
}

// opensAs: a file only Orbit's own editors open — a draw.io, Mermaid or
// Excalidraw file — and what it becomes (null: one kept as a file; a
// spreadsheet or an SVG may be wanted as it is).
export function opensAs(name: string): FileKind | null {
  if (/\.excalidraw$/i.test(name)) return 'board'
  if (/\.(drawio|dio|mmd|mermaid)$/i.test(name) || /\.drawio\.svg$/i.test(name)) return 'diagram'
  return null
}

// bringIn makes a new file from one on the computer, in a folder.
export async function bringIn(ws: string, file: File, folderId: string, words: { tab: string; notDrawio: string; notExcalidraw: string; notMermaid: string }): Promise<{ kind: FileKind; id: string; title: string }> {
  const kind = kindOfFile(file.name)
  const title = file.name.replace(/\.(drawio\.svg|xlsx|csv|tsv|drawio|dio|xml|svg|excalidraw|mmd|mermaid)$/i, '')
  if (kind === 'sheet') {
    const { readCsv, readXlsx } = await import('./xlsx')
    const data = /\.xlsx$/i.test(file.name) ? readXlsx(new Uint8Array(await file.arrayBuffer())) : readCsv(await file.text(), title.slice(0, 31) || words.tab)
    const s = await sheetsApi.create(ws, { title, data, text: textOf(data), folder_id: folderId })
    return { kind, id: s.id, title }
  }
  if (kind === 'diagram' && /\.(mmd|mermaid)$/i.test(file.name)) {
    const data = (await file.text()).trim()
    if (!mermaidKind(data)) throw new Error(words.notMermaid)
    const { renderMermaid } = await import('./mermaid')
    const d = await diagramsApi.create(ws, { title, kind: 'mermaid', data, text: mermaidLabels(data), preview: (await renderMermaid(data)).svg, folder_id: folderId })
    return { kind, id: d.id, title }
  }
  if (kind === 'diagram') {
    const text = await file.text()
    const data = fileOf(text)
    if (!data) throw new Error(words.notDrawio)
    const d = await diagramsApi.create(ws, { title, data, text: labelsOf(data), preview: /\.svg$/i.test(file.name) ? lightPicture(text) : undefined, folder_id: folderId })
    return { kind, id: d.id, title }
  }
  if (kind === 'board') {
    const text = await file.text()
    let ok = false
    try {
      ok = JSON.parse(text)?.type === 'excalidraw'
    } catch {
      ok = false
    }
    if (!ok) throw new Error(words.notExcalidraw)
    const d = await drawingsApi.make(ws, { title, scene: text, folder_id: folderId })
    return { kind, id: d.id, title }
  }
  throw new Error(words.notDrawio)
}
