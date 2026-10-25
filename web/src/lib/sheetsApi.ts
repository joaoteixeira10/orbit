// The sheets' endpoints (api/internal/api/handlers_sheets.go).
import { api, postFile } from './api'
import type { SheetData } from './sheet'

export interface SheetSummary {
  id: string
  folder_id?: string
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
  create: (ws: string, b: { title: string; data?: SheetData; text?: string; folder_id?: string }) => api<SheetDoc>(`/workspaces/${ws}/sheets`, { method: 'POST', body: j(b) }),
  get: (id: string) => api<SheetDoc>(`/sheets/${id}`),
  save: (id: string, b: { title?: string; data?: SheetData; text?: string; version: number }) => api<SheetDoc>(`/sheets/${id}`, { method: 'PUT', body: j(b) }),
  remove: (id: string) => api<void>(`/sheets/${id}`, { method: 'DELETE' }),
  // A Google Sheets spreadsheet (shared "Anyone with the link"), as .xlsx,
  // and its name.
  google: async (ws: string, url: string) => {
    const f = await postFile(`/workspaces/${ws}/google-sheet`, { url })
    return { bytes: new Uint8Array(await f.blob.arrayBuffer()), name: f.name.replace(/\.xlsx$/i, '') }
  },
}

// isGoogleSheet: a Google Sheets link.
export const isGoogleSheet = (s: string) => /^\s*https:\/\/docs\.google\.com\/spreadsheets\/d\//i.test(s)
