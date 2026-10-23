// Bookmarks: the API, and their pictures (fetched with the session from
// Orbit — the site never sees who looks).
import { api, fetchAuthedBlob } from '@/lib/api'

export interface Bookmark {
  id: string
  workspace_id: string
  url: string
  title: string
  description: string
  site_name: string
  tags: string[]
  notes: string
  read_later: boolean
  read_at?: string | null
  has_image: boolean
  created_at: string
}
export type BookmarkInput = Partial<Pick<Bookmark, 'url' | 'title' | 'description' | 'tags' | 'notes' | 'read_later'>> & { read?: boolean }

const j = (b: unknown) => JSON.stringify(b)
export const bookmarksApi = {
  list: (ws: string) => api<Bookmark[]>(`/workspaces/${ws}/bookmarks`),
  create: (ws: string, b: BookmarkInput) => api<Bookmark>(`/workspaces/${ws}/bookmarks`, { method: 'POST', body: j(b) }),
  update: (id: string, b: BookmarkInput) => api<Bookmark>(`/bookmarks/${id}`, { method: 'PUT', body: j(b) }),
  refresh: (id: string) => api<Bookmark>(`/bookmarks/${id}/refresh`, { method: 'POST' }),
  remove: (id: string) => api<void>(`/bookmarks/${id}`, { method: 'DELETE' }),
}

const pictures = new Map<string, Promise<string>>()
// pictureOf is a bookmark's picture as a blob: URL (cached).
export function pictureOf(id: string): Promise<string> {
  let p = pictures.get(id)
  if (!p) {
    p = fetchAuthedBlob(`/bookmarks/${id}/image`).then((b) => URL.createObjectURL(b))
    pictures.set(id, p)
    p.catch(() => pictures.delete(id))
  }
  return p
}

// forgetPicture drops a cached picture (it was read again).
export function forgetPicture(id: string) {
  pictures.delete(id)
}

// hostOf is a link's site, without www.
export function hostOf(url: string): string {
  try {
    return new URL(url).host.replace(/^www\./, '')
  } catch {
    return url
  }
}
