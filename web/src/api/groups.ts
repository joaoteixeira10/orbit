// Groups: sets of people carrying instance roles and added to workspaces.
import { api } from '@/lib/api'

export interface Group {
  id: string
  name: string
  description: string
  source: string // "" made in Orbit; "oidc" / "scim" kept by the identity provider
  role_ids: string[]
  members: number
  workspaces: number
}
export interface GroupDetail extends Group {
  people: { user_id: string; username: string; display_name: string; avatar_url: string }[]
  in: { id: string; name: string; role: string }[]
}
export interface WorkspaceGroup {
  group_id: string
  name: string
  role: string
  members: number
}

const j = (b: unknown) => JSON.stringify(b)
export const groupsApi = {
  list: () => api<Group[]>('/groups'),
  get: (id: string) => api<GroupDetail>(`/groups/${id}`),
  create: (b: { name: string; description?: string; role_ids?: string[] }) => api<Group>('/groups', { method: 'POST', body: j(b) }),
  update: (id: string, b: { name?: string; description?: string; role_ids?: string[] }) => api<GroupDetail>(`/groups/${id}`, { method: 'PUT', body: j(b) }),
  remove: (id: string) => api<void>(`/groups/${id}`, { method: 'DELETE' }),
  addMember: (id: string, user: string) => api<GroupDetail>(`/groups/${id}/members`, { method: 'POST', body: j({ user }) }),
  removeMember: (id: string, uid: string) => api<GroupDetail>(`/groups/${id}/members/${uid}`, { method: 'DELETE' }),
  // A workspace's (its owners).
  names: () => api<WorkspaceGroup[]>('/group-names'),
  ofWorkspace: (ws: string) => api<WorkspaceGroup[]>(`/workspaces/${ws}/groups`),
  setInWorkspace: (ws: string, groupId: string, role: string) => api<WorkspaceGroup[]>(`/workspaces/${ws}/groups`, { method: 'POST', body: j({ group_id: groupId, role }) }),
  removeFromWorkspace: (ws: string, groupId: string) => api<WorkspaceGroup[]>(`/workspaces/${ws}/groups/${groupId}`, { method: 'DELETE' }),
}
