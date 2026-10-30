// Guest links and forms (api/internal/api/handlers_share.go,
// handlers_forms.go): made by the workspace's people; opened by anyone with
// the link — those calls carry no sign-in (pub).
import { api, ApiError } from './api'

export interface ShareLink {
  id: string
  page_id: string
  mode: 'read' | 'edit'
  subpages: boolean
  expires_at?: string
  last_used_at?: string
  created_by: string
  created_at: string
  url: string
}

export interface FormField {
  prop: string // a property's id, or "title" (the row's name)
  label?: string
  help?: string
  required?: boolean
}
export interface FormDef {
  id: string
  page_id: string
  title: string
  intro: string
  thanks: string
  public: boolean
  open: boolean
  answers: number
  fields: FormField[]
  url?: string // a public one's link
}
// A form as someone answering sees it.
export interface FillField {
  prop: string
  label: string
  help?: string
  required?: boolean
  type: 'title' | 'text' | 'number' | 'select' | 'multi_select' | 'date' | 'checkbox' | 'person' | 'url'
  options?: { id: string; name: string; color?: string }[]
}
export interface FillForm {
  title: string
  intro?: string
  thanks?: string
  open: boolean
  fields: FillField[]
}

export interface GuestPage {
  id: string
  parent_id: string | null
  title: string
  icon: string
  cover?: string
  content?: string
  version?: number
  updated_at: string
}
export interface GuestShare {
  mode: 'read' | 'edit'
  can_edit: boolean
  workspace: string
  root: string
  page: GuestPage
  pages: GuestPage[]
}

const j = (b: unknown) => JSON.stringify(b)

// pub calls the API without a sign-in (a guest, someone answering a form).
export async function pub<T>(path: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(`/api/v1${path}`, { ...init, headers: { 'Content-Type': 'application/json', ...(init.headers ?? {}) }, credentials: 'omit' })
  if (!res.ok) {
    let body: any
    try {
      body = await res.json()
    } catch {
      /* not JSON */
    }
    throw new ApiError(res.status, body?.error ?? res.statusText, body)
  }
  return (res.status === 204 ? undefined : await res.json()) as T
}

export const shareApi = {
  links: (pageId: string) => api<ShareLink[]>(`/pages/${pageId}/links`),
  createLink: (pageId: string, b: { mode?: 'read' | 'edit'; subpages?: boolean; expires_in_days?: number }) => api<ShareLink>(`/pages/${pageId}/links`, { method: 'POST', body: j(b) }),
  updateLink: (id: string, b: { mode?: 'read' | 'edit'; subpages?: boolean; expires_in_days?: number }) => api<ShareLink>(`/links/${id}`, { method: 'PUT', body: j(b) }),
  removeLink: (id: string) => api<void>(`/links/${id}`, { method: 'DELETE' }),

  forms: (pageId: string) => api<FormDef[]>(`/pages/${pageId}/forms`),
  createForm: (pageId: string, b: Partial<FormDef>) => api<FormDef>(`/pages/${pageId}/forms`, { method: 'POST', body: j(b) }),
  updateForm: (id: string, b: Partial<FormDef>) => api<FormDef>(`/forms/${id}`, { method: 'PUT', body: j(b) }),
  removeForm: (id: string) => api<void>(`/forms/${id}`, { method: 'DELETE' }),
  fill: (id: string) => api<FillForm>(`/forms/${id}`),
  answer: (id: string, values: Record<string, unknown>) => api<{ ok: boolean; thanks?: string }>(`/forms/${id}/answers`, { method: 'POST', body: j({ values }) }),
}

// The guest's side.
export const guestApi = {
  open: (token: string) => pub<GuestShare>(`/shared/${token}`),
  page: (token: string, id: string) => pub<GuestPage>(`/shared/${token}/pages/${id}`),
  save: (token: string, id: string, b: { title?: string; content?: string; version: number }) => pub<GuestPage>(`/shared/${token}/pages/${id}`, { method: 'PUT', body: j(b) }),
  embed: <T>(token: string, kind: 'sheet' | 'diagram' | 'board', id: string) => pub<T>(`/shared/${token}/embed/${kind}/${id}`),
  // An upload's address through the link (an <img> loads it as it is).
  uploadUrl: (token: string, src: string) => src.replace(/^\/api\/v1\/uploads\//, `/api/v1/shared/${token}/uploads/`),
  form: (token: string) => pub<FillForm>(`/f/${token}`),
  answer: (token: string, values: Record<string, unknown>) => pub<{ ok: boolean; thanks?: string }>(`/f/${token}`, { method: 'POST', body: j({ values }) }),
}
