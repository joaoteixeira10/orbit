// Folders, and a workspace's sheets, diagrams and whiteboards as files in
// them (api/internal/api/handlers_folders.go and each one's list).
import { api } from './api'
import { sheetsApi } from './sheetsApi'
import { diagramsApi } from './diagramsApi'
import { drawingsApi } from '@/components/draw/drawing'

export type FileKind = 'sheet' | 'diagram' | 'board'
export const FILE_KINDS: FileKind[] = ['sheet', 'diagram', 'board']
// The module each kind belongs to.
export const KIND_MODULE: Record<FileKind, string> = { sheet: 'sheets', diagram: 'diagrams', board: 'whiteboards' }

export interface Folder {
  id: string
  workspace_id: string
  parent_id: string // "" at the top
  name: string
  color?: string
  created_by: string
  created_at: string
}

export interface FileItem {
  kind: FileKind
  id: string
  folder_id: string
  title: string
  preview?: string // an SVG picture (diagrams, whiteboards)
  updated_at: string
  updated_by: string
  created_at: string
  created_by: string
  size: number // bytes
}

const j = (b: unknown) => JSON.stringify(b)

type Listed = { id: string; folder_id?: string; title: string; preview?: string; updated_at: string; updated_by: string; created_at?: string; created_by?: string; size?: number }
const asFile = (kind: FileKind, x: Listed): FileItem => ({
  kind,
  id: x.id,
  folder_id: x.folder_id ?? '',
  title: x.title,
  preview: x.preview || undefined,
  updated_at: x.updated_at,
  updated_by: x.updated_by,
  created_at: x.created_at ?? x.updated_at,
  created_by: x.created_by ?? x.updated_by,
  size: x.size ?? 0,
})

export const filesApi = {
  folders: (ws: string) => api<Folder[]>(`/workspaces/${ws}/folders`),
  createFolder: (ws: string, b: { name: string; parent_id?: string; color?: string }) => api<Folder>(`/workspaces/${ws}/folders`, { method: 'POST', body: j(b) }),
  updateFolder: (id: string, b: { name?: string; parent_id?: string; color?: string }) => api<Folder>(`/folders/${id}`, { method: 'PUT', body: j(b) }),
  removeFolder: (id: string) => api<void>(`/folders/${id}`, { method: 'DELETE' }),
  // move puts files and folders in a folder ("" the top).
  move: (ws: string, folderId: string, what: { kind: FileKind | 'folder'; id: string }[]) => {
    const of = (k: string) => what.filter((w) => w.kind === k).map((w) => w.id)
    return api<void>(`/workspaces/${ws}/files/move`, { method: 'POST', body: j({ folder_id: folderId, sheets: of('sheet'), diagrams: of('diagram'), drawings: of('board'), folders: of('folder') }) })
  },
  // files lists the kinds asked for, as files.
  async files(ws: string, kinds: FileKind[]): Promise<FileItem[]> {
    const lists = await Promise.all(
      kinds.map(async (k) => {
        const xs = (k === 'sheet' ? await sheetsApi.list(ws) : k === 'diagram' ? await diagramsApi.list(ws) : await drawingsApi.list(ws)) as Listed[]
        return xs.map((x) => asFile(k, x))
      }),
    )
    return lists.flat()
  },
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
