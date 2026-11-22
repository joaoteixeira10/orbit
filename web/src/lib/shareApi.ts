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

// What a page read without an account loads through: a guest link's secret,
// or "site/<name>" for a published website (the same paths under it).
const scope = (token: string) => (token.startsWith('site/') ? token : `shared/${token}`)

// The guest's side.
export const guestApi = {
  open: (token: string) => pub<GuestShare>(`/shared/${token}`),
  page: (token: string, id: string) => pub<GuestPage>(`/shared/${token}/pages/${id}`),
  save: (token: string, id: string, b: { title?: string; content?: string; version: number }) => pub<GuestPage>(`/shared/${token}/pages/${id}`, { method: 'PUT', body: j(b) }),
  embed: <T>(token: string, kind: 'sheet' | 'diagram' | 'board', id: string) => pub<T>(`/${scope(token)}/embed/${kind}/${id}`),
  // An upload's address through the link (an <img> loads it as it is).
  uploadUrl: (token: string, src: string) => src.replace(/^\/api\/v1\/uploads\//, `/api/v1/${scope(token)}/uploads/`),
  form: (token: string) => pub<FillForm>(`/f/${token}`),
  answer: (token: string, values: Record<string, unknown>) => pub<{ ok: boolean; thanks?: string }>(`/f/${token}`, { method: 'POST', body: j({ values }) }),
}

// A published website (no account).
export interface PublicSite {
  slug: string
  title: string
  home: string
  roots: string[]
  pages: GuestPage[]
  indexed: boolean
}
export interface SiteHit {
  id: string
  title: string
  icon: string
  snippet: string
}
export const siteApi = {
  open: (slug: string) => pub<PublicSite>(`/site/${slug}`),
  page: (slug: string, id: string) => pub<GuestPage>(`/site/${slug}/pages/${id}`),
  search: (slug: string, q: string) => pub<SiteHit[]>(`/site/${slug}/search?q=${encodeURIComponent(q)}`),
}

// A workspace's website, for its owners.
export interface SiteSettings {
  id: string
  workspace_id: string
  slug: string
  title: string
  domain: string
  page_ids: string[]
  published: boolean
  indexed: boolean
  path: string
}
export const siteAdminApi = {
  get: (ws: string) => api<{ site: SiteSettings | null; suggested_slug?: string }>(`/workspaces/${ws}/site`),
  save: (ws: string, b: Partial<Pick<SiteSettings, 'slug' | 'title' | 'domain' | 'page_ids' | 'published' | 'indexed'>>) =>
    api<SiteSettings>(`/workspaces/${ws}/site`, { method: 'PUT', body: j(b) }),
  remove: (ws: string) => api<void>(`/workspaces/${ws}/site`, { method: 'DELETE' }),
}

// A workspace's pages copied into a git repository (Markdown, one way).
export interface PagesRepo {
  id: string
  workspace_id: string
  host: string
  owner: string
  repo: string
  branch: string
  folder: string
  auto: boolean
  copied_at?: string | null
  commit?: string
  last_error?: string
}
export interface PagesRepoAccount {
  base_url: string
  kind: string
  login: string
}
type PagesRepoAnswer = { repo: PagesRepo | null; accounts: PagesRepoAccount[] }
export const pagesRepoApi = {
  get: (ws: string) => api<PagesRepoAnswer>(`/workspaces/${ws}/pages-repo`),
  save: (ws: string, b: { host: string; repo: string; branch: string; folder: string; auto: boolean }) =>
    api<PagesRepoAnswer>(`/workspaces/${ws}/pages-repo`, { method: 'PUT', body: j(b) }),
  copy: (ws: string) => api<PagesRepoAnswer>(`/workspaces/${ws}/pages-repo/copy`, { method: 'POST' }),
  remove: (ws: string) => api<void>(`/workspaces/${ws}/pages-repo`, { method: 'DELETE' }),
}
