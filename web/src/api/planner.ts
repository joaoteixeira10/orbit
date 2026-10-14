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
  workspace_id: string
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
  members: (id: string) => api<Member[]>(`/workspaces/${id}/members`),
  addMember: (id: string, user: string, role: WSRole) => api<Member[]>(`/workspaces/${id}/members`, { method: 'POST', body: j({ user, role }) }),
  setRole: (id: string, uid: string, role: WSRole) => api<Member[]>(`/workspaces/${id}/members/${uid}`, { method: 'PUT', body: j({ role }) }),
  removeMember: (id: string, uid: string) => api<void>(`/workspaces/${id}/members/${uid}`, { method: 'DELETE' }),
}

export const pagesApi = {
  list: (ws: string) => api<PageSummary[]>(`/workspaces/${ws}/pages`),
  trash: (ws: string) => api<PageSummary[]>(`/workspaces/${ws}/trash`),
  create: (ws: string, b: { parent_id?: string | null; title?: string; icon?: string; content?: string; after_id?: string }) =>
    api<Page>(`/workspaces/${ws}/pages`, { method: 'POST', body: j(b) }),
  get: (id: string) => api<Page>(`/pages/${id}`),
  update: (id: string, b: { title?: string; icon?: string; cover?: string; content?: string; version?: number }) =>
    api<Page>(`/pages/${id}`, { method: 'PUT', body: j(b) }),
  move: (id: string, parent_id: string | null, after_id: string) => api<PageSummary>(`/pages/${id}/move`, { method: 'POST', body: j({ parent_id, after_id }) }),
  trashPage: (id: string) => api<void>(`/pages/${id}`, { method: 'DELETE' }),
  destroy: (id: string) => api<void>(`/pages/${id}?permanent=1`, { method: 'DELETE' }),
  restore: (id: string) => api<PageSummary>(`/pages/${id}/restore`, { method: 'POST' }),
  duplicate: (id: string) => api<Page>(`/pages/${id}/duplicate`, { method: 'POST' }),
  favorite: (id: string, on: boolean) => api<void>(`/pages/${id}/favorite`, { method: on ? 'POST' : 'DELETE' }),
  favorites: () => api<PageSummary[]>('/me/favorites'),
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

export const notesApi = {
  list: (ws: string) => api<Note[]>(`/workspaces/${ws}/notes`),
  create: (ws: string, b: NoteInput) => api<Note>(`/workspaces/${ws}/notes`, { method: 'POST', body: j(b) }),
  update: (id: string, b: NoteInput) => api<Note>(`/notes/${id}`, { method: 'PUT', body: j(b) }),
  remove: (id: string) => api<void>(`/notes/${id}`, { method: 'DELETE' }),
  order: (ws: string, ids: string[]) => api<void>(`/workspaces/${ws}/notes/order`, { method: 'PUT', body: j({ ids }) }),
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
