// The planner's endpoints: workspaces and members, pages, task lists and
// tasks, the calendar (events, subscriptions, feeds), Home and search.
// Shapes mirror the Go handlers (api/internal/api/handlers_*.go).
import { api } from '@/lib/api'

export type WSRole = 'owner' | 'editor' | 'viewer'
export type ModuleKey = 'home' | 'notes' | 'pages' | 'tasks' | 'board' | 'timeline' | 'charts' | 'calendar' | string
export type WorkspaceKind = 'school' | 'work' | 'personal' | 'shared' | 'custom'

export interface CatalogModule {
  key: ModuleKey
  icon: string
  soon: boolean
}
export interface Column {
  id: string
  name: string
  color?: string
  done: boolean
}
export interface Template {
  kind: WorkspaceKind
  icon: string
  color: string
  modules: ModuleKey[]
  lists: { name: string; color: string; icon: string; columns: Column[] }[]
  pages: string[]
}

export interface Workspace {
  id: string
  name: string
  icon: string
  color: string
  kind: WorkspaceKind
  description: string
  modules: ModuleKey[]
  role: WSRole
  position: number
  member_count: number
  muted: boolean // I muted others' activity in it
  created_by: string
  archived_at?: string | null
  created_at: string
  updated_at: string
}
export interface Member {
  user_id: string
  username: string
  display_name: string
  avatar_url: string
  role: WSRole
  joined_at: string
}

export interface PageSummary {
  id: string
  workspace_id: string
  parent_id: string | null
  title: string
  icon: string
  rank: number
  updated_at: string
  trashed_at?: string | null
  favorite: boolean
}
export interface Page extends PageSummary {
  cover: string
  content: string
  version: number
  created_by: string
  updated_by: string
  created_at: string
  can_edit: boolean
}

export interface TaskList {
  id: string
  workspace_id: string
  name: string
  color: string
  icon: string
  columns: Column[]
  rank: number
  open_count: number
  git?: GitLink // the list holds a repository's issues
}
// A list's link to a repository on Gitea / Forgejo.
export interface GitLink {
  host: string
  owner: string
  repo: string
  mode: 'issues' | 'labels' | 'project'
  url: string
  synced_at?: string
  last_error?: string
  linked_by: string
}
export interface Task {
  id: string
  workspace_id: string
  list_id: string
  column_id: string
  parent_id: string | null
  title: string
  notes: string
  priority: 0 | 1 | 2 | 3
  tags: string[]
  start: string | null // YYYY-MM-DD (floating), on or before due: the timeline bar's left end
  due: string | null // YYYY-MM-DD (floating)
  due_time: string | null // HH:MM
  repeat: string // RRULE
  rank: number
  completed_at: string | null
  done: boolean
  assignee_id: string | null
  created_by: string
  created_at: string
  updated_at: string
  // Its issue or pull request on git (a task of a linked list, or linked by hand).
  git_host?: string
  git_repo?: string // owner/repo
  git_number?: number
  git_url?: string
  git_state?: 'open' | 'closed' | 'merged'
  git_pull?: boolean
  git_dirty?: boolean // a change didn't reach git yet
  git_error?: string // in a change's answer: saved, but git refused it
}
export interface TaskInput {
  list_id?: string
  column_id?: string
  parent_id?: string
  title?: string
  notes?: string
  priority?: number
  tags?: string[]
  start?: string // "" clears
  due?: string // "" clears
  due_time?: string
  repeat?: string
  assignee_id?: string
  completed?: boolean
  after_id?: string
}

// A quick note: a short text, or a checklist (items) when checklist is set.
export type NoteColor = '' | 'yellow' | 'orange' | 'red' | 'pink' | 'purple' | 'blue' | 'teal' | 'green' | 'brown' | 'gray'
export interface NoteItem {
  id: string // "" for a new item; the server gives it one
  text: string
  done: boolean
}
export interface Note {
  id: string
  workspace_id: string // "" for a personal note (Quick Notes)
  owner_id: string // personal notes: whose
  title: string
  body: string
  checklist: boolean
  items: NoteItem[]
  color: NoteColor
  pinned: boolean
  rank: number
  created_by: string
  updated_by: string
  created_at: string
  updated_at: string
}
export interface NoteInput {
  title?: string
  body?: string
  checklist?: boolean // switching converts the lines (text ↔ items)
  items?: NoteItem[]
  color?: NoteColor
  pinned?: boolean
}

// One entry of the notification inbox. It says what happened (kind, verb,
// params), not the sentence: lib/inbox.ts words it in the reader's language.
export interface InboxItem {
  id: string
  kind: 'reminder' | 'assigned' | 'workspace' | 'activity' | 'digest'
  verb: string
  workspace_id: string
  actor_id: string
  params: Record<string, any>
  link: string
  count: number
  read: boolean
  created_at: string
  updated_at: string
}
export interface PushDevice {
  id: string
  user_agent: string
  last_used_at?: string | null
  created_at: string
}

export interface Occurrence {
  kind: 'event' | 'external'
  id: string
  workspace_id: string
  subscription_id?: string
  title: string
  location?: string
  notes?: string
  start: string
  end: string
  all_day: boolean
  color: string
  recurring: boolean
}
export interface CalendarData {
  events: Occurrence[]
  tasks: Task[]
}
export interface CalEvent {
  id: string
  workspace_id: string
  title: string
  notes: string
  location: string
  starts_at: string
  ends_at: string
  all_day: boolean
  tz: string
  rrule: string
  color: string
  remind_minutes: number | null
  exdates: string[]
}
export interface EventInput {
  title?: string
  notes?: string
  location?: string
  starts_at?: string
  ends_at?: string
  all_day?: boolean
  tz?: string
  rrule?: string
  color?: string
  remind_minutes?: number // < 0 removes
}
export interface Subscription {
  id: string
  workspace_id: string
  name: string
  url_host: string
  color: string
  refresh_minutes: number
  fetched_at?: string | null
  last_error?: string
  event_count: number
}
export interface Feed {
  id: string
  name: string
  prefix: string
  workspace_ids: string[]
  include_tasks: boolean
  last_used_at?: string | null
  created_at: string
}

export interface Home {
  overdue: Task[]
  today: Task[]
  upcoming: Task[]
  events: Occurrence[]
  pages: PageSummary[]
  notes: Note[] // pinned
  mine: Note[] // my latest personal notes, not pinned (Today only)
}
export interface SearchHit {
  kind: 'page' | 'task' | 'event' | 'note'
  id: string
  workspace_id: string
  title: string
  icon?: string
  snippet?: string
  due?: string | null
  done?: boolean
  start?: string
  list_id?: string
}

const j = (body: unknown) => JSON.stringify(body)
const qs = (p: Record<string, string | undefined>) =>
  new URLSearchParams(Object.entries(p).filter(([, v]) => v !== undefined && v !== '') as [string, string][]).toString()

export const plannerApi = {
  catalog: () => api<{ modules: CatalogModule[]; templates: Template[] }>('/catalog'),
  home: (p: { today: string; from: string; to: string; ws?: string }) => api<Home>(`/home?${qs(p)}`),
  search: (q: string, ws?: string) => api<SearchHit[]>(`/search?${qs({ q, ws })}`),
  lookupUser: (q: string) => api<{ id: string; username: string; display_name: string; avatar_url: string }>(`/users/lookup?${qs({ q })}`),
}

export const workspacesApi = {
  list: () => api<Workspace[]>('/workspaces'),
  get: (id: string) => api<{ workspace: Workspace; members: Member[] }>(`/workspaces/${id}`),
  create: (b: { name: string; icon?: string; color?: string; kind?: WorkspaceKind; description?: string; modules?: ModuleKey[]; starter?: boolean; locale?: string }) =>
    api<Workspace>('/workspaces', { method: 'POST', body: j(b) }),
  update: (id: string, b: Partial<{ name: string; icon: string; color: string; description: string; modules: ModuleKey[]; archived: boolean }>) =>
    api<Workspace>(`/workspaces/${id}`, { method: 'PUT', body: j(b) }),
  remove: (id: string) => api<void>(`/workspaces/${id}`, { method: 'DELETE' }),
  order: (ids: string[]) => api<void>('/me/workspace-order', { method: 'PUT', body: j({ ids }) }),
  mute: (id: string, muted: boolean) => api<void>(`/workspaces/${id}/mute`, { method: 'PUT', body: j({ muted }) }),
  members: (id: string) => api<Member[]>(`/workspaces/${id}/members`),
  addMember: (id: string, user: string, role: WSRole) => api<Member[]>(`/workspaces/${id}/members`, { method: 'POST', body: j({ user, role }) }),
  setRole: (id: string, uid: string, role: WSRole) => api<Member[]>(`/workspaces/${id}/members/${uid}`, { method: 'PUT', body: j({ role }) }),
  removeMember: (id: string, uid: string) => api<void>(`/workspaces/${id}/members/${uid}`, { method: 'DELETE' }),
}

export const withContent = (p: Page): Page => ({ ...p, content: p.content ?? '' })

export const pagesApi = {
  list: (ws: string) => api<PageSummary[]>(`/workspaces/${ws}/pages`),
  trash: (ws: string) => api<PageSummary[]>(`/workspaces/${ws}/trash`),
  create: (ws: string, b: { parent_id?: string | null; title?: string; icon?: string; content?: string; after_id?: string }) =>
    api<Page>(`/workspaces/${ws}/pages`, { method: 'POST', body: j(b) }),
  // An empty page comes without "content"; the app always has a string.
  get: (id: string) => api<Page>(`/pages/${id}`).then(withContent),
  update: (id: string, b: { title?: string; icon?: string; cover?: string; content?: string; version?: number }) =>
    api<Page>(`/pages/${id}`, { method: 'PUT', body: j(b) }).then(withContent),
  move: (id: string, parent_id: string | null, after_id: string) => api<PageSummary>(`/pages/${id}/move`, { method: 'POST', body: j({ parent_id, after_id }) }),
  trashPage: (id: string) => api<void>(`/pages/${id}`, { method: 'DELETE' }),
  destroy: (id: string) => api<void>(`/pages/${id}?permanent=1`, { method: 'DELETE' }),
  restore: (id: string) => api<PageSummary>(`/pages/${id}/restore`, { method: 'POST' }),
  duplicate: (id: string) => api<Page>(`/pages/${id}/duplicate`, { method: 'POST' }),
  favorite: (id: string, on: boolean) => api<void>(`/pages/${id}/favorite`, { method: on ? 'POST' : 'DELETE' }),
  favorites: () => api<PageSummary[]>('/me/favorites'),
}

export interface GitAccount {
  id: string
  kind: 'gitea'
  base_url: string
  login: string
  last_error?: string
  checked_at?: string
}
export interface GitRepo {
  id: number
  full_name: string
  name: string
  description: string
  private: boolean
  html_url: string
  owner: { login: string }
}
export interface GitBoards {
  supported: boolean // the server's API has project boards
  projects: { id: number; title: string }[]
  labels: { id: number; name: string; color: string }[]
}
export interface GitListInput {
  account_id: string
  owner: string
  repo: string
  mode: GitLink['mode']
  project_id?: number
  labels?: string[]
  name?: string
}

export const gitApi = {
  accounts: () => api<{ accounts: GitAccount[]; servers: string[] }>('/me/git'),
  connect: (base_url: string, token: string) => api<GitAccount>('/me/git', { method: 'POST', body: j({ base_url, token }) }),
  disconnect: (id: string) => api<void>(`/me/git/${id}`, { method: 'DELETE' }),
  repos: (id: string, q: string) => api<GitRepo[]>(`/me/git/${id}/repos?${qs({ q })}`),
  boards: (id: string, owner: string, repo: string) => api<GitBoards>(`/me/git/${id}/boards?${qs({ owner, repo })}`),
  createList: (ws: string, b: GitListInput) => api<TaskList>(`/workspaces/${ws}/lists/git`, { method: 'POST', body: j(b) }),
  unlinkList: (id: string) => api<void>(`/lists/${id}/git`, { method: 'DELETE' }),
  syncList: (id: string) => api<TaskList>(`/lists/${id}/git/sync`, { method: 'POST' }),
  linkTask: (id: string, url: string) => api<Task>(`/tasks/${id}/git`, { method: 'POST', body: j({ url }) }),
  unlinkTask: (id: string) => api<Task>(`/tasks/${id}/git`, { method: 'DELETE' }),
}

export const tasksApi = {
  lists: (ws: string) => api<TaskList[]>(`/workspaces/${ws}/lists`),
  createList: (ws: string, b: { name: string; color?: string; icon?: string; columns?: Column[] }) =>
    api<TaskList>(`/workspaces/${ws}/lists`, { method: 'POST', body: j(b) }),
  updateList: (id: string, b: Partial<{ name: string; color: string; icon: string; columns: Column[] }>) =>
    api<TaskList>(`/lists/${id}`, { method: 'PUT', body: j(b) }),
  removeList: (id: string) => api<void>(`/lists/${id}`, { method: 'DELETE' }),
  orderLists: (ws: string, ids: string[]) => api<void>(`/workspaces/${ws}/lists/order`, { method: 'PUT', body: j({ ids }) }),
  list: (ws: string, p: { list?: string; state?: 'active' | 'done' | 'all' } = {}) => api<Task[]>(`/workspaces/${ws}/tasks?${qs(p)}`),
  create: (ws: string, b: TaskInput) => api<Task>(`/workspaces/${ws}/tasks`, { method: 'POST', body: j(b) }),
  update: (id: string, b: TaskInput) => api<{ task: Task; repeated: boolean }>(`/tasks/${id}`, { method: 'PUT', body: j(b) }),
  move: (id: string, b: { column_id: string; list_id?: string; after_id?: string }) => api<Task>(`/tasks/${id}/move`, { method: 'POST', body: j(b) }),
  remove: (id: string) => api<void>(`/tasks/${id}`, { method: 'DELETE' }),
}

// Notes of a workspace, or — ws "me" — the user's own Quick Notes.
const notesBase = (ws: string) => (ws === 'me' ? '/me/notes' : `/workspaces/${ws}/notes`)
export const notesApi = {
  list: (ws: string) => api<Note[]>(notesBase(ws)),
  create: (ws: string, b: NoteInput) => api<Note>(notesBase(ws), { method: 'POST', body: j(b) }),
  update: (id: string, b: NoteInput) => api<Note>(`/notes/${id}`, { method: 'PUT', body: j(b) }),
  remove: (id: string) => api<void>(`/notes/${id}`, { method: 'DELETE' }),
  order: (ws: string, ids: string[]) => api<void>(`${notesBase(ws)}/order`, { method: 'PUT', body: j({ ids }) }),
  // move puts a note in a workspace ("" = my Quick Notes).
  move: (id: string, workspace_id: string) => api<Note>(`/notes/${id}/move`, { method: 'POST', body: j({ workspace_id }) }),
}

export const inboxApi = {
  list: (before?: string) => api<{ items: InboxItem[]; unread: number; more: boolean }>(`/me/notifications?${qs({ before })}`),
  read: (b: { ids?: string[]; all?: boolean }) => api<void>('/me/notifications/read', { method: 'POST', body: j(b) }),
}

export const pushApi = {
  info: () => api<{ available: boolean; public_key: string; devices: PushDevice[] }>('/me/push'),
  subscribe: (sub: PushSubscriptionJSON) => api<void>('/me/push', { method: 'POST', body: j(sub) }),
  remove: (b: { endpoint?: string; id?: string }) => api<void>('/me/push/remove', { method: 'POST', body: j(b) }),
  test: () => api<{ devices: number }>('/me/push/test', { method: 'POST' }),
}

export const calendarApi = {
  workspace: (ws: string, p: { from: string; to: string; from_date?: string; to_date?: string }) => api<CalendarData>(`/workspaces/${ws}/calendar?${qs(p)}`),
  mine: (p: { from: string; to: string; from_date?: string; to_date?: string }) => api<CalendarData>(`/calendar?${qs(p)}`),
  createEvent: (ws: string, b: EventInput) => api<CalEvent>(`/workspaces/${ws}/events`, { method: 'POST', body: j(b) }),
  getEvent: (id: string) => api<CalEvent>(`/events/${id}`),
  updateEvent: (id: string, b: EventInput) => api<CalEvent>(`/events/${id}`, { method: 'PUT', body: j(b) }),
  removeEvent: (id: string, occurrence?: string) => api<void>(`/events/${id}${occurrence ? `?${qs({ occurrence })}` : ''}`, { method: 'DELETE' }),
  subscriptions: (ws: string) => api<Subscription[]>(`/workspaces/${ws}/subscriptions`),
  subscribe: (ws: string, b: { name?: string; url: string; color?: string; refresh_minutes?: number }) =>
    api<Subscription>(`/workspaces/${ws}/subscriptions`, { method: 'POST', body: j(b) }),
  updateSubscription: (id: string, b: Partial<{ name: string; url: string; color: string; refresh_minutes: number }>) =>
    api<Subscription>(`/subscriptions/${id}`, { method: 'PUT', body: j(b) }),
  refreshSubscription: (id: string) => api<Subscription>(`/subscriptions/${id}/refresh`, { method: 'POST' }),
  unsubscribe: (id: string) => api<void>(`/subscriptions/${id}`, { method: 'DELETE' }),
  feeds: () => api<Feed[]>('/me/feeds'),
  createFeed: (b: { name?: string; workspace_ids?: string[]; include_tasks?: boolean }) =>
    api<{ feed: Feed; url: string; webcal: string }>('/me/feeds', { method: 'POST', body: j(b) }),
  removeFeed: (id: string) => api<void>(`/me/feeds/${id}`, { method: 'DELETE' }),
}
