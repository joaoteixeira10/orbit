// Apps: other sites (a Gitea, a Grafana, a wiki) pinned in a workspace and
// shown in a frame, one tab each, on the sites Orbit's owner allows.
import { api } from '@/lib/api'

export interface WorkspaceApp {
  id: string
  workspace_id: string
  name: string
  url: string
  position: number
  allowed: boolean // on an allowed site still
}
export interface AppCheck {
  frameable: 'yes' | 'no' | 'unknown'
  why?: 'headers' | 'elsewhere'
  site?: string
}

const j = (b: unknown) => JSON.stringify(b)
export const appsApi = {
  sites: () => api<{ sites: string[] }>('/embed-sites'),
  list: (ws: string) => api<WorkspaceApp[]>(`/workspaces/${ws}/apps`),
  create: (ws: string, b: { name?: string; url: string }) => api<WorkspaceApp>(`/workspaces/${ws}/apps`, { method: 'POST', body: j(b) }),
  update: (id: string, b: { name?: string; url?: string }) => api<WorkspaceApp>(`/apps/${id}`, { method: 'PUT', body: j(b) }),
  remove: (id: string) => api<void>(`/apps/${id}`, { method: 'DELETE' }),
  order: (ws: string, ids: string[]) => api<void>(`/workspaces/${ws}/apps/order`, { method: 'PUT', body: j({ ids }) }),
  check: (id: string) => api<AppCheck>(`/apps/${id}/check`),
}

// hostOf is an address's host, for names and messages.
export function hostOf(url: string): string {
  try {
    return new URL(url).host
  } catch {
    return url
  }
}
