// The sheets' endpoints (api/internal/api/handlers_sheets.go).
import { api } from './api'
import type { SheetData } from './sheet'

export interface SheetSummary {
  id: string
  title: string
  version: number
  updated_by: string
  updated_at: string
}
export interface SheetDoc extends SheetSummary {
  workspace_id: string
  data?: SheetData
  created_by: string
  created_at: string
}

const j = (b: unknown) => JSON.stringify(b)
export const sheetsApi = {
  list: (ws: string) => api<SheetSummary[]>(`/workspaces/${ws}/sheets`),
  create: (ws: string, b: { title: string; data?: SheetData; text?: string }) => api<SheetDoc>(`/workspaces/${ws}/sheets`, { method: 'POST', body: j(b) }),
  get: (id: string) => api<SheetDoc>(`/sheets/${id}`),
  save: (id: string, b: { title?: string; data?: SheetData; text?: string; version: number }) => api<SheetDoc>(`/sheets/${id}`, { method: 'PUT', body: j(b) }),
  remove: (id: string) => api<void>(`/sheets/${id}`, { method: 'DELETE' }),
}
