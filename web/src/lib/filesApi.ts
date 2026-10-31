// Folders, and a workspace's sheets, diagrams, whiteboards and pages as
// files in them (api/internal/api/handlers_folders.go and each one's list),
// and its files: of their own, or the attachments of its pages, tasks and
// notes, each where what it's on is (handlers_files.go). A folder joined to
// a task list is a project. Pages show here only when filed in a folder
// (the rest are in Pages).
import { api } from './api'
import type { Upload } from './uploads'
import { sheetsApi } from './sheetsApi'
import { diagramsApi } from './diagramsApi'
import { drawingsApi } from '@/components/draw/drawing'
import { pagesApi } from '@/api/planner'

export type FileKind = 'sheet' | 'diagram' | 'board' | 'page' | 'file'
export const FILE_KINDS: FileKind[] = ['page', 'sheet', 'diagram', 'board', 'file']
// The module each kind belongs to.
export const KIND_MODULE: Record<FileKind, string> = { sheet: 'sheets', diagram: 'diagrams', board: 'whiteboards', page: 'pages', file: 'files' }

// What is right inside a folder, of each kind.
export interface FolderCounts {
  sheets: number
  diagrams: number
  boards: number
  pages: number
  files: number
  folders: number
}

export interface Folder {
  id: string
  workspace_id: string
  parent_id: string // "" at the top
  name: string
  color?: string
  list_id?: string // its task list (a project)
  counts?: FolderCounts
  created_by: string
  created_at: string
}

export interface FileItem {
  kind: FileKind
  id: string
  folder_id: string
  title: string
  preview?: string // an SVG picture (diagrams, whiteboards)
  icon?: string // a page's emoji
  updated_at: string
  updated_by: string
  created_at: string
  created_by: string
  size: number // bytes
  upload?: FileUpload // a file: what the server says of it
}

// A file (an upload) as Files shows it: where, and what it's on.
export interface FileUpload extends Upload {
  folder_id: string
  on_kind?: 'page' | 'task' | 'note'
  on_id?: string
  on_title?: string
  versions: number
  empties_at?: string // in the trash: when it goes for good
}

export interface FileVersion extends Upload {
  current: boolean
}

export interface FileUsage {
  used: number // bytes this workspace's files take
  files: number
  quota: number // the instance's limit (0 none)
  all: number // what the instance's files take
}

const fromUpload = (u: FileUpload): FileItem => ({
  kind: 'file',
  id: u.id,
  folder_id: u.folder_id ?? '',
  title: u.name,
  updated_at: u.created_at,
  updated_by: u.owner_id,
  created_at: u.created_at,
  created_by: u.owner_id,
  size: u.size,
  upload: u,
})

const j = (b: unknown) => JSON.stringify(b)
// ws "me" is someone's own drive (My files): their sketches and files.
const base = (ws: string) => (ws === 'me' ? '/me' : `/workspaces/${ws}`)

type Listed = { id: string; folder_id?: string; title: string; preview?: string; icon?: string; updated_at: string; updated_by?: string; created_at?: string; created_by?: string; size?: number }
const asFile = (kind: FileKind, x: Listed): FileItem => ({
  kind,
  id: x.id,
  folder_id: x.folder_id ?? '',
  title: x.title,
  preview: x.preview || undefined,
  icon: x.icon || undefined,
  updated_at: x.updated_at,
  updated_by: x.updated_by ?? '',
  created_at: x.created_at ?? x.updated_at,
  created_by: x.created_by ?? x.updated_by ?? '',
  size: x.size ?? 0,
})

export const filesApi = {
  folders: (ws: string) => api<Folder[]>(`${base(ws)}/folders`),
  createFolder: (ws: string, b: { name: string; parent_id?: string; color?: string; list_id?: string }) => api<Folder>(`${base(ws)}/folders`, { method: 'POST', body: j(b) }),
  // list_id joins a task list ("" none; a list in another folder moves here).
  updateFolder: (id: string, b: { name?: string; parent_id?: string; color?: string; list_id?: string }) => api<Folder>(`/folders/${id}`, { method: 'PUT', body: j(b) }),
  removeFolder: (id: string) => api<void>(`/folders/${id}`, { method: 'DELETE' }),
  // move puts files and folders in a folder ("" the top).
  move: (ws: string, folderId: string, what: { kind: FileKind | 'folder'; id: string }[]) => {
    const of = (k: string) => what.filter((w) => w.kind === k).map((w) => w.id)
    return api<void>(`${base(ws)}/files/move`, { method: 'POST', body: j({ folder_id: folderId, sheets: of('sheet'), diagrams: of('diagram'), drawings: of('board'), pages: of('page'), files: of('file'), folders: of('folder') }) })
  },
  // Files (uploads): of their own, and attachments where what they're on is.
  uploads: async (ws: string, trash = false) => (await api<FileUpload[]>(`${base(ws)}/files${trash ? '?trash=1' : ''}`)).map(fromUpload),
  upload(ws: string, folderId: string, file: File): Promise<Upload> {
    const form = new FormData()
    form.append('file', file, file.name || 'file')
    return api<Upload>(`/uploads?target=file&target_id=${ws}&folder_id=${encodeURIComponent(folderId)}`, { method: 'POST', body: form })
  },
  updateFile: (id: string, b: { name?: string; folder_id?: string }) => api<Upload>(`/files/${id}`, { method: 'PUT', body: j(b) }),
  versions: (id: string) => api<FileVersion[]>(`/files/${id}/versions`),
  makeCurrent: (id: string) => api<Upload>(`/files/${id}/current`, { method: 'POST' }),
  restore: (id: string) => api<Upload>(`/files/${id}/restore`, { method: 'POST' }),
  // trash (default) or delete now.
  removeFile: (id: string, forever = false) => api<void>(`/uploads/${id}${forever ? '?forever=1' : ''}`, { method: 'DELETE' }),
  usage: (ws: string) => api<FileUsage>(`${base(ws)}/files/usage`),
  // files lists the kinds asked for, as files.
  async files(ws: string, kinds: FileKind[]): Promise<FileItem[]> {
    const lists = await Promise.all(
      kinds.map(async (k) => {
        if (k === 'page') return (await pagesApi.list(ws)).filter((p) => p.folder_id).map((p) => asFile(k, p))
        if (k === 'file') return filesApi.uploads(ws)
        const xs = (k === 'sheet' ? await sheetsApi.list(ws) : k === 'diagram' ? await diagramsApi.list(ws) : await drawingsApi.list(ws)) as Listed[]
        return xs.map((x) => asFile(k, x))
      }),
    )
    return lists.flat()
  },
}

// countParts: what a folder holds, kind by kind (the kinds it has some of).
export function countParts(c?: FolderCounts): { key: keyof FolderCounts; n: number }[] {
  if (!c) return []
  return (['folders', 'pages', 'sheets', 'diagrams', 'boards', 'files'] as const).filter((k) => c[k]).map((k) => ({ key: k, n: c[k] }))
}

// pathTo is a folder's way down from the top (the top not included).
export function pathTo(folders: Folder[], id: string): Folder[] {
  const by = new Map(folders.map((f) => [f.id, f]))
  const out: Folder[] = []
  const seen = new Set<string>()
  for (let f = by.get(id); f && !seen.has(f.id); f = by.get(f.parent_id)) {
    seen.add(f.id)
    out.unshift(f)
  }
  return out
}

// inside: every folder within id (id included), to keep a folder from
// going into itself.
export function inside(folders: Folder[], id: string): Set<string> {
  const out = new Set([id])
  let grew = true
  while (grew) {
    grew = false
    for (const f of folders) {
      if (!out.has(f.id) && out.has(f.parent_id)) {
        out.add(f.id)
        grew = true
      }
    }
  }
  return out
}

// tree is the folders in order, each with its depth (for a picker).
export function tree(folders: Folder[]): { folder: Folder; depth: number }[] {
  const kids = new Map<string, Folder[]>()
  for (const f of folders) kids.set(f.parent_id, [...(kids.get(f.parent_id) ?? []), f])
  const out: { folder: Folder; depth: number }[] = []
  const walk = (parent: string, depth: number, seen: Set<string>) => {
    for (const f of (kids.get(parent) ?? []).sort((a, b) => a.name.localeCompare(b.name))) {
      if (seen.has(f.id)) continue
      seen.add(f.id)
      out.push({ folder: f, depth })
      walk(f.id, depth + 1, seen)
    }
  }
  walk('', 0, new Set())
  return out
}

// sizeText: bytes as people read them.
export function sizeText(n: number, locale: string): string {
  const units = ['B', 'KB', 'MB', 'GB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: i ? 1 : 0 }).format(v)} ${units[i]}`
}
