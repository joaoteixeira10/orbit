// The diagrams' endpoints (api/internal/api/handlers_diagrams.go).
import { api } from './api'

export interface DiagramSummary {
  id: string
  folder_id?: string
  title: string
  preview?: string // an SVG picture of it
  version: number
  updated_by: string
  updated_at: string
}
export interface DiagramDoc extends DiagramSummary {
  workspace_id: string
  data?: string // draw.io's XML
  created_by: string
  created_at: string
}
export interface DiagramSave {
  folder_id?: string // where a new one goes
  title?: string
  data?: string
  preview?: string
  text?: string
  version?: number
}

const j = (b: unknown) => JSON.stringify(b)
export const diagramsApi = {
  list: (ws: string) => api<DiagramSummary[]>(`/workspaces/${ws}/diagrams`),
  create: (ws: string, b: DiagramSave) => api<DiagramDoc>(`/workspaces/${ws}/diagrams`, { method: 'POST', body: j(b) }),
  get: (id: string) => api<DiagramDoc>(`/diagrams/${id}`),
  save: (id: string, b: DiagramSave) => api<DiagramDoc>(`/diagrams/${id}`, { method: 'PUT', body: j(b) }),
  remove: (id: string) => api<void>(`/diagrams/${id}`, { method: 'DELETE' }),
}
