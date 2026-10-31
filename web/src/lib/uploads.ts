// Files people add to pages, tasks and notes (the server's /uploads). A file
// belongs to what it's on; only those who can see that can fetch it — with
// the bearer token, which an <img> or a plain link doesn't send. So images
// are fetched here and shown from a blob: URL, and file links download
// through here too.
import { api, downloadAuthed, fetchAuthedBlob } from './api'

export type UploadTarget = 'page' | 'task' | 'note' | 'file'

export interface Upload {
  id: string
  workspace_id: string
  owner_id: string
  target: UploadTarget
  target_id: string
  name: string
  type: string
  size: number
  url: string // /api/v1/uploads/<id>/<name>
  image: boolean // shown in the page (a raster image)
  created_at: string
  lineage: string // the first version's id: one file's versions share it
  folder_id?: string
  trashed_at?: string
  replaced_at?: string
  was_kind?: 'page' | 'task' | 'note' // what it was on, gone for good
  was_title?: string
}

export const uploadsApi = {
  list: (target: UploadTarget, targetId: string) => api<Upload[]>(`/uploads?target=${target}&target_id=${targetId}`),
  remove: (id: string) => api<void>(`/uploads/${id}`, { method: 'DELETE' }),
}

// uploadFile sends one file to a page, task or note.
export function uploadFile(target: UploadTarget, targetId: string, file: File): Promise<Upload> {
  const form = new FormData()
  form.append('file', file, file.name || 'file')
  return api<Upload>(`/uploads?target=${target}&target_id=${targetId}`, { method: 'POST', body: form })
}

const PREFIX = '/api/v1/uploads/'

// isUploadSrc: an address of one of Orbit's uploads (a path or this origin).
export function isUploadSrc(src: string): boolean {
  try {
    const u = new URL(src, location.origin)
    return u.origin === location.origin && u.pathname.startsWith(PREFIX)
  } catch {
    return false
  }
}

function apiPath(src: string): string {
  return new URL(src, location.origin).pathname.slice('/api/v1'.length)
}

// Blob URLs of images already fetched this session (one fetch per file).
const blobs = new Map<string, Promise<string>>()

// uploadBlobUrl is a blob: URL for an upload, to put in an <img>.
export function uploadBlobUrl(src: string): Promise<string> {
  const key = apiPath(src)
  let p = blobs.get(key)
  if (!p) {
    p = fetchAuthedBlob(key).then((b) => URL.createObjectURL(b))
    p.catch(() => blobs.delete(key))
    blobs.set(key, p)
  }
  return p
}

// downloadUpload saves an upload as a file.
export function downloadUpload(src: string, name: string): Promise<void> {
  return downloadAuthed(apiPath(src), name)
}

// fileSize is a size for people: 12 KB, 3.4 MB.
export function fileSize(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${Math.round(n / 1024)} KB`
  return `${(n / 1024 / 1024).toFixed(n < 10 * 1024 * 1024 ? 1 : 0)} MB`
}

// markdownFor is how an upload goes into a page: an image, or a link.
export function markdownFor(u: Upload): string {
  const label = u.name.replace(/[[\]]/g, '')
  return u.image ? `![${label}](${u.url})` : `[${label}](${u.url})`
}
