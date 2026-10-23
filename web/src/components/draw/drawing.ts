// Drawings (Excalidraw): whiteboards and the drawings on notes.
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
  list: (ws: string) => api<Drawing[]>(`/workspaces/${ws}/drawings`),
  create: (ws: string, title: string) => api<Drawing>(`/workspaces/${ws}/drawings`, { method: 'POST', body: j({ title }) }),
  get: (id: string) => api<Drawing>(`/drawings/${id}`),
  save: (id: string, b: Partial<Snapshot> & { title?: string; version: number }) => api<Drawing>(`/drawings/${id}`, { method: 'PUT', body: j(b) }),
  remove: (id: string) => api<void>(`/drawings/${id}`, { method: 'DELETE' }),
  // ofNote is a note's drawing (made the first time).
  ofNote: (noteId: string) => api<Drawing>(`/notes/${noteId}/drawing`, { method: 'POST' }),
}

// previewSrc shows an SVG preview as an image (it runs nothing that way).
export const previewSrc = (svg: string) => `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`
