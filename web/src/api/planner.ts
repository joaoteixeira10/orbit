// The planner's endpoints: workspaces and members, pages, task lists and
// tasks, the calendar (events, subscriptions, feeds), Home and search.
// Shapes mirror the Go handlers (api/internal/api/handlers_*.go).
import { api } from '@/lib/api'
import type { TaskFilter } from '@/lib/taskFilter'
import type { Upload } from '@/lib/uploads'

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
  grade_scale: number // what marks are out of (Grades): 20, 100, 10, 5
  holidays: string // the country whose public holidays its calendar shows ("" none)
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
  template?: boolean // offered when a new page starts empty
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
// A list's link to a repository on Gitea / Forgejo or GitHub.
export interface GitLink {
  host: string
  owner: string
  repo: string
  mode: 'issues' | 'labels' | 'project'
  url: string
  synced_at?: string
  last_error?: string
  linked_by: string
  bot_login?: string // members without their own account act as this bot ("" = read only)
  write: 'own' | 'bot' | 'none' // how my changes reach git; none = I can only read
  instant: boolean // a webhook tells Orbit of changes on git
  hook_at?: string // its last post
}
export interface GitHookSetup {
  auto: boolean // Orbit made the webhook
  url: string
  secret?: string // to add it by hand (shown once)
}
export interface GitMilestone {
  ref: number
  title: string
  due_on?: string
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
  kind: '' | 'exam' // an exam is counted down to
  tags: string[]
  start: string | null // YYYY-MM-DD (floating), on or before due: the timeline bar's left end
  due: string | null // YYYY-MM-DD (floating)
  due_time: string | null // HH:MM
  repeat: string // RRULE
  rank: number
  completed_at: string | null
  done: boolean
  assignee_ids: string[] // who it's for
  assignee_id: string | null // the first of them (older clients)
  comment_count: number
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
  git_milestone?: string // its issue's milestone (a linked list's task)
  git_milestone_id?: number
  focus_minutes?: number // time focused on it
  goal_id?: string // the goal it counts towards
  waits_on?: string[] // tasks it starts after (finish-to-start)
}
export interface TaskInput {
  list_id?: string
  column_id?: string
  parent_id?: string
  title?: string
  notes?: string
  priority?: number
  kind?: 'task' | 'exam' // "task" makes it a plain task again
  git_milestone_id?: number // a linked list's task: a milestone's ref (0 = none)
  goal_id?: string // a goal of its workspace ("" none)
  git_milestone?: string
  tags?: string[]
  start?: string // "" clears
  due?: string // "" clears
  due_time?: string
  repeat?: string
  assignee_ids?: string[]
  completed?: boolean
  after_id?: string
  waits_on?: string[] // the whole list
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
  attachments?: Upload[] // files on the note (in lists)
  remind_at?: string | null // a one-off reminder
  drawing?: { id: string; preview: string } // its drawing (Excalidraw)
}
export interface NoteInput {
  title?: string
  body?: string
  checklist?: boolean // switching converts the lines (text ↔ items)
  items?: NoteItem[]
  color?: NoteColor
  pinned?: boolean
  remind_at?: string // RFC 3339; "" takes it off
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
  kind: 'event' | 'external' | 'holiday'
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
  course_id?: string // a class of a course (changed in Courses)
  tz?: string // an Orbit event's own time zone
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
  exams: Task[] // the next ones (60 days), soonest first
}
export interface SearchHit {
  kind: 'page' | 'task' | 'event' | 'note' | 'drawing' | 'journal' | 'bookmark'
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

export interface ImportReport {
  lists: number
  tasks: number
  pages: number
  notes: number
  events: number
  courses: number
  comments: number
  files: number
  added: string[] | null
  missing: string[] | null
  skipped: string[] | null
}

export const workspacesApi = {
  list: () => api<Workspace[]>('/workspaces'),
  get: (id: string) => api<{ workspace: Workspace; members: Member[] }>(`/workspaces/${id}`),
  create: (b: { name: string; icon?: string; color?: string; kind?: WorkspaceKind; description?: string; modules?: ModuleKey[]; starter?: boolean; locale?: string }) =>
    api<Workspace>('/workspaces', { method: 'POST', body: j(b) }),
  update: (id: string, b: Partial<{ name: string; icon: string; color: string; description: string; modules: ModuleKey[]; archived: boolean; grade_scale: number; holidays: string }>) =>
    api<Workspace>(`/workspaces/${id}`, { method: 'PUT', body: j(b) }),
  remove: (id: string) => api<void>(`/workspaces/${id}`, { method: 'DELETE' }),
  // A whole workspace as a file (transfer) and back (import), between Orbits.
  importFile: (file: File, people: boolean) => {
    const form = new FormData()
    form.append('file', file, file.name || 'workspace.orbit.zip')
    form.append('people', String(people))
    return api<{ workspace: Workspace; report: ImportReport }>('/workspaces/import', { method: 'POST', body: form })
  },
  order: (ids: string[]) => api<void>('/me/workspace-order', { method: 'PUT', body: j({ ids }) }),
  mute: (id: string, muted: boolean) => api<void>(`/workspaces/${id}/mute`, { method: 'PUT', body: j({ muted }) }),
  members: (id: string) => api<Member[]>(`/workspaces/${id}/members`),
  addMember: (id: string, user: string, role: WSRole) => api<Member[]>(`/workspaces/${id}/members`, { method: 'POST', body: j({ user, role }) }),
  setRole: (id: string, uid: string, role: WSRole) => api<Member[]>(`/workspaces/${id}/members/${uid}`, { method: 'PUT', body: j({ role }) }),
  removeMember: (id: string, uid: string) => api<void>(`/workspaces/${id}/members/${uid}`, { method: 'DELETE' }),
}

// Invite links: an owner makes one (a role, days, uses); its token is shown
// once. Whoever opens /join/<token> joins, signed in or making an account.
export interface Invite {
  id: string
  workspace_id: string
  prefix: string
  role: 'editor' | 'viewer'
  created_by: string
  created_at: string
  expires_at: string
  max_uses: number
  uses: number
}
export interface InvitePreview {
  workspace: string
  icon: string
  color: string
  invited_by: string
  role: 'editor' | 'viewer'
  can_sign_up: boolean
  expires_at: string
  member_count: number
}
export const invitesApi = {
  list: (ws: string) => api<Invite[]>(`/workspaces/${ws}/invites`),
  create: (ws: string, b: { role: 'editor' | 'viewer'; days: number; max_uses: number }) =>
    api<{ invite: Invite; token: string; path: string }>(`/workspaces/${ws}/invites`, { method: 'POST', body: j(b) }),
  revoke: (ws: string, id: string) => api<void>(`/workspaces/${ws}/invites/${id}`, { method: 'DELETE' }),
  preview: (token: string) => api<InvitePreview>(`/invites/${encodeURIComponent(token)}`),
  accept: (token: string) => api<{ workspace_id: string; already_member: boolean }>(`/invites/${encodeURIComponent(token)}/accept`, { method: 'POST' }),
  signup: (token: string, b: { username: string; display_name: string; email: string; password: string }) =>
    api<{ access_token: string; workspace_id: string }>(`/invites/${encodeURIComponent(token)}/signup`, { method: 'POST', body: j(b) }),
}

export const withContent = (p: Page): Page => ({ ...p, content: p.content ?? '' })

export const pagesApi = {
  list: (ws: string) => api<PageSummary[]>(`/workspaces/${ws}/pages`),
  trash: (ws: string) => api<PageSummary[]>(`/workspaces/${ws}/trash`),
  create: (ws: string, b: { parent_id?: string | null; title?: string; icon?: string; content?: string; after_id?: string }) =>
    api<Page>(`/workspaces/${ws}/pages`, { method: 'POST', body: j(b) }),
  // An empty page comes without "content"; the app always has a string.
  get: (id: string) => api<Page>(`/pages/${id}`).then(withContent),
  // What links to a page: other pages, and tasks whose notes do.
  backlinks: (id: string) => api<{ pages: PageSummary[]; tasks: { id: string; title: string; list_id: string; done: boolean }[] }>(`/pages/${id}/backlinks`),
  update: (id: string, b: { title?: string; icon?: string; cover?: string; content?: string; version?: number; template?: boolean }) =>
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
  kind: 'gitea' | 'github' // Gitea / Forgejo, or GitHub
  base_url: string
  login: string
  last_error?: string
  checked_at?: string
  via: 'token' | 'oauth' // pasted a token, or signed in with the server
}
// The owner's registrations of Orbit on servers, for "Sign in with …".
export interface GitOAuthApps {
  apps: { base_url: string; client_id: string; has_secret: boolean }[]
  redirect_uri: string
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
  accounts: () => api<{ accounts: GitAccount[]; servers: string[]; sign_in: string[] }>('/me/git'),
  // "Sign in with …": where to send the browser.
  startSignIn: (base_url: string) => api<{ url: string }>('/me/git/oauth/start', { method: 'POST', body: j({ base_url }) }),
  oauthApps: () => api<GitOAuthApps>('/git/oauth-apps'),
  setOAuthApps: (apps: { base_url: string; client_id: string; client_secret?: string }[]) => api<GitOAuthApps>('/git/oauth-apps', { method: 'PUT', body: j({ apps }) }),
  connect: (base_url: string, token: string) => api<GitAccount>('/me/git', { method: 'POST', body: j({ base_url, token }) }),
  disconnect: (id: string) => api<void>(`/me/git/${id}`, { method: 'DELETE' }),
  repos: (id: string, q: string) => api<GitRepo[]>(`/me/git/${id}/repos?${qs({ q })}`),
  boards: (id: string, owner: string, repo: string) => api<GitBoards>(`/me/git/${id}/boards?${qs({ owner, repo })}`),
  createList: (ws: string, b: GitListInput) => api<TaskList>(`/workspaces/${ws}/lists/git`, { method: 'POST', body: j(b) }),
  unlinkList: (id: string) => api<void>(`/lists/${id}/git`, { method: 'DELETE' }),
  syncList: (id: string) => api<TaskList>(`/lists/${id}/git/sync`, { method: 'POST' }),
  setBot: (id: string, token: string) => api<TaskList>(`/lists/${id}/git/bot`, { method: 'PUT', body: j({ token }) }),
  hookOn: (id: string) => api<GitHookSetup>(`/lists/${id}/git/hook`, { method: 'POST' }),
  hookOff: (id: string) => api<void>(`/lists/${id}/git/hook`, { method: 'DELETE' }),
  milestones: (id: string) => api<GitMilestone[]>(`/lists/${id}/git/milestones`),
  linkTask: (id: string, url: string) => api<Task>(`/tasks/${id}/git`, { method: 'POST', body: j({ url }) }),
  unlinkTask: (id: string) => api<Task>(`/tasks/${id}/git`, { method: 'DELETE' }),
}

// A smart list: a saved task filter (yours, or shared with the workspace).
export interface SmartList {
  id: string
  workspace_id: string
  owner_id: string
  name: string
  color: string
  icon: string
  shared: boolean
  rank: number
  can_edit: boolean
  filter: TaskFilter
}
export type SmartListInput = Partial<Pick<SmartList, 'name' | 'color' | 'icon' | 'shared' | 'filter'>>
export const smartListsApi = {
  list: (ws: string) => api<SmartList[]>(`/workspaces/${ws}/smart-lists`),
  create: (ws: string, b: SmartListInput) => api<SmartList>(`/workspaces/${ws}/smart-lists`, { method: 'POST', body: j(b) }),
  update: (id: string, b: SmartListInput) => api<SmartList>(`/smart-lists/${id}`, { method: 'PUT', body: j(b) }),
  remove: (id: string) => api<void>(`/smart-lists/${id}`, { method: 'DELETE' }),
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
  update: (id: string, b: TaskInput) => api<{ task: Task; repeated: boolean; shifted?: number }>(`/tasks/${id}`, { method: 'PUT', body: j(b) }),
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
  snooze: (id: string, until: string) => api<void>(`/me/notifications/${id}/snooze`, { method: 'POST', body: j({ until }) }),
}

export const pushApi = {
  info: () => api<{ available: boolean; public_key: string; devices: PushDevice[] }>('/me/push'),
  subscribe: (sub: PushSubscriptionJSON) => api<void>('/me/push', { method: 'POST', body: j(sub) }),
  remove: (b: { endpoint?: string; id?: string }) => api<void>('/me/push/remove', { method: 'POST', body: j(b) }),
  test: () => api<{ devices: number }>('/me/push/test', { method: 'POST' }),
}

export interface CalDAVLink {
  id: string
  workspace_id: string
  calendar_url: string
  calendar_name: string
  host: string
  username: string
  synced_at?: string | null
  last_error?: string
  event_count: number
}
export interface CalDAVCalendar {
  url: string
  name: string
  color: string
}

export const calendarApi = {
  holidayCountries: () => api<{ code: string; name: string }[]>('/holidays/countries'),
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
  // Two-way sync with a calendar on a CalDAV server (iCloud, Nextcloud…).
  caldav: (ws: string) => api<{ link: CalDAVLink | null }>(`/workspaces/${ws}/caldav`),
  caldavCalendars: (ws: string, b: { url: string; username: string; password: string }) =>
    api<CalDAVCalendar[]>(`/workspaces/${ws}/caldav/calendars`, { method: 'POST', body: j(b) }),
  setCalDAV: (ws: string, b: { url: string; username: string; password: string; calendar_url: string; calendar_name: string }) =>
    api<{ link: CalDAVLink }>(`/workspaces/${ws}/caldav`, { method: 'PUT', body: j(b) }),
  syncCalDAV: (ws: string) => api<{ link: CalDAVLink }>(`/workspaces/${ws}/caldav/sync`, { method: 'POST' }),
  unlinkCalDAV: (ws: string) => api<void>(`/workspaces/${ws}/caldav`, { method: 'DELETE' }),
  feeds: () => api<Feed[]>('/me/feeds'),
  createFeed: (b: { name?: string; workspace_ids?: string[]; include_tasks?: boolean }) =>
    api<{ feed: Feed; url: string; webcal: string }>('/me/feeds', { method: 'POST', body: j(b) }),
  removeFeed: (id: string) => api<void>(`/me/feeds/${id}`, { method: 'DELETE' }),
}

// School: courses (shared by the workspace, with a weekly timetable that
// fills the calendar) and grades (each person's own).
export interface CourseSlot {
  id: string
  course_id: string
  weekday: number // 1 Monday … 7 Sunday
  start: string // HH:MM
  end: string
  room: string
  kind: string
  event_id: string
}
export interface Course {
  id: string
  workspace_id: string
  name: string
  code: string
  teacher: string
  room: string
  color: string
  tag: string
  term_start: string | null
  term_end: string | null
  tz: string
  weight: number
  notes: string
  rank: number
  slots: CourseSlot[]
  open_tasks: number
  average: number | null // mine, on the workspace's scale
  grade_count: number
  target: number | null // my goal
  page_id: string | null // its notes page
}
export type SlotInput = Pick<CourseSlot, 'weekday' | 'start' | 'end' | 'room' | 'kind'> & { id?: string }
export type CourseInput = Partial<Pick<Course, 'name' | 'code' | 'teacher' | 'room' | 'color' | 'tag' | 'tz' | 'weight' | 'notes'>> & {
  term_start?: string
  term_end?: string
  slots?: SlotInput[]
}
export interface Grade {
  id: string
  workspace_id: string
  course_id: string
  owner_id: string
  title: string
  score: number
  out_of: number
  weight: number
  date: string | null
  created_at: string
}
export type GradeInput = Partial<Pick<Grade, 'title' | 'score' | 'out_of' | 'weight'>> & { date?: string }

export const coursesApi = {
  list: (ws: string) => api<Course[]>(`/workspaces/${ws}/courses`),
  create: (ws: string, b: CourseInput) => api<Course>(`/workspaces/${ws}/courses`, { method: 'POST', body: j(b) }),
  update: (id: string, b: CourseInput) => api<Course>(`/courses/${id}`, { method: 'PUT', body: j(b) }),
  remove: (id: string) => api<void>(`/courses/${id}`, { method: 'DELETE' }),
  page: (id: string) => api<{ page_id: string }>(`/courses/${id}/page`, { method: 'POST' }),
  setGoal: (id: string, target: number | null) => api<void>(`/courses/${id}/goal`, { method: 'PUT', body: j({ target }) }),
  grades: (ws: string) => api<Grade[]>(`/workspaces/${ws}/grades`),
  addGrade: (course: string, b: GradeInput) => api<Grade>(`/courses/${course}/grades`, { method: 'POST', body: j(b) }),
  updateGrade: (id: string, b: GradeInput) => api<Grade>(`/grades/${id}`, { method: 'PUT', body: j(b) }),
  removeGrade: (id: string) => api<void>(`/grades/${id}`, { method: 'DELETE' }),
}

// Comments on a task (Markdown; any member, viewers too).
export interface TaskComment {
  id: string
  task_id: string
  workspace_id: string
  author_id: string
  author_name: string
  author_avatar: string
  body: string
  created_at: string
  edited_at?: string
  git_author?: string // a git login: written on the issue by someone who isn't a member
  git_error?: string // saved, but it didn't reach the issue
}
// A line of a task's activity (created, renamed, assigned, moved, done…).
export interface TaskActivity {
  id: string
  kind: 'created' | 'renamed' | 'due' | 'priority' | 'assigned' | 'unassigned' | 'moved' | 'status' | 'completed' | 'reopened' | 'notes'
  actor_id: string
  actor_name: string
  params: Record<string, any>
  created_at: string
}
export const commentsApi = {
  activity: (task: string) => api<TaskActivity[]>(`/tasks/${task}/activity`),
  list: (task: string) => api<TaskComment[]>(`/tasks/${task}/comments`),
  add: (task: string, body: string) => api<TaskComment>(`/tasks/${task}/comments`, { method: 'POST', body: j({ body }) }),
  update: (id: string, body: string) => api<TaskComment>(`/comments/${id}`, { method: 'PUT', body: j({ body }) }),
  remove: (id: string) => api<void>(`/comments/${id}`, { method: 'DELETE' }),
}

// A workspace's activity feed.
export interface ActivityLine {
  id: string
  workspace_id: string
  actor_id: string
  actor_name: string
  actor_avatar: string
  verb: 'task_added' | 'task_done' | 'page_added' | 'note_added' | 'event_added' | 'commented' | 'member_joined'
  title: string
  link: string
  created_at: string
}
export const activityApi = {
  list: (ws: string, before?: string) => api<ActivityLine[]>(`/workspaces/${ws}/activity${before ? `?${qs({ before })}` : ''}`),
}
