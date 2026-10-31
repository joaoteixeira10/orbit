// Drawings (Excalidraw): whiteboards, the drawings on notes, and Quick Draw
// (one's own sketches, no workspace).
import { api } from '@/lib/api'

export interface Drawing {
  id: string
  workspace_id: string
  note_id?: string
  title: string
  scene?: string
  preview?: string
  version: number
  created_by: string
  updated_by: string
  created_at: string
  updated_at: string
}

// What a canvas hands over to save.
export interface Snapshot {
  scene: string
  preview: string
  text: string
}

const j = (b: unknown) => JSON.stringify(b)

export const drawingsApi = {
  // ws "me": Quick Draw, the person's own sketches.
  list: (ws: string) => api<Drawing[]>(ws === 'me' ? '/me/drawings' : `/workspaces/${ws}/drawings`),
  // scene: how it starts (a Quick Draw sketch starts as one; else Excalidraw's).
  create: (ws: string, title: string, scene?: string) => api<Drawing>(ws === 'me' ? '/me/drawings' : `/workspaces/${ws}/drawings`, { method: 'POST', body: j({ title, scene }) }),
  get: (id: string) => api<Drawing>(`/drawings/${id}`),
  // together: saved by the tab that saves for everyone drawing it at once.
  save: (id: string, b: Partial<Snapshot> & { title?: string; version: number }, together = false) =>
    api<Drawing>(`/drawings/${id}`, { method: 'PUT', body: j(b), headers: together ? { 'X-Orbit-Collab': '1' } : undefined }),
  remove: (id: string) => api<void>(`/drawings/${id}`, { method: 'DELETE' }),
  // make is a new whiteboard with all it starts with (a copy, an import), in a folder.
  make: (ws: string, b: { title: string; scene?: string; preview?: string; text?: string; folder_id?: string }) =>
    api<Drawing>(ws === 'me' ? '/me/drawings' : `/workspaces/${ws}/drawings`, { method: 'POST', body: j(b) }),
  // move puts a sketch or whiteboard in a workspace ("me": back to Quick Draw).
  move: (id: string, ws: string) => api<Drawing>(`/drawings/${id}/move`, { method: 'POST', body: j({ workspace_id: ws === 'me' ? '' : ws }) }),
  // ofNote is a note's drawing (made the first time).
  ofNote: (noteId: string) => api<Drawing>(`/notes/${noteId}/drawing`, { method: 'POST' }),
}

// previewSrc shows an SVG preview as an image (it runs nothing that way).
export const previewSrc = (svg: string) => `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`
