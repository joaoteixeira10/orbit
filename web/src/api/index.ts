import { api } from '@/lib/api'
import type { User, Role, AuditEntry, Branding, About, NotifConfig, NotifyPrefs, OIDCConfig, APIToken } from './types'

// Typed endpoint groups for /api/v1: accounts and administration here, the
// planner (workspaces, pages, tasks, calendar) in ./planner.

export interface Paged<T> {
  items: T[]
  total: number
  page: number
  per_page: number
}

// ---- Users ----
export interface CreateUserInput {
  username: string
  display_name?: string
  email?: string
  password?: string
  generate_password?: boolean
  must_change_password?: boolean
  otp_required?: boolean
  role_ids: string[]
}
export interface UpdateUserInput {
  display_name?: string
  email?: string
  disabled?: boolean
  otp_required?: boolean
  role_ids?: string[]
}
export const usersApi = {
  list: () => api<User[]>('/users'),
  create: (body: CreateUserInput) =>
    api<{ user: User; generated_password: string }>('/users', { method: 'POST', body: JSON.stringify(body) }),
  update: (id: string, body: UpdateUserInput) =>
    api<User>(`/users/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  // mustChange (default true) forces the user to pick a new password at next sign-in.
  resetPassword: (id: string, newPassword: string, mustChange = true) =>
    api<void>(`/users/${id}/password`, {
      method: 'PUT',
      body: JSON.stringify({ new_password: newPassword, must_change_password: mustChange }),
    }),
  // Generates a one-time temporary password (always must-change), returned once.
  generatePassword: (id: string) =>
    api<{ generated_password: string }>(`/users/${id}/password`, {
      method: 'PUT',
      body: JSON.stringify({ generate: true }),
    }),
  unlock: (id: string) => api<void>(`/users/${id}/unlock`, { method: 'POST' }),
  resetOtp: (id: string) => api<void>(`/users/${id}/reset-otp`, { method: 'POST' }),
  resetWebauthn: (id: string) => api<void>(`/users/${id}/reset-webauthn`, { method: 'POST' }),
  forceLogout: (id: string) => api<void>(`/users/${id}/logout`, { method: 'POST' }),
  impersonate: (id: string) =>
    api<{ access_token: string; expires_in: number; user: User }>(`/users/${id}/impersonate`, { method: 'POST' }),
  endImpersonation: (id: string, reason: 'returned' | 'expired') =>
    api<void>(`/users/${id}/impersonate/end`, { method: 'POST', body: JSON.stringify({ reason }) }),
  remove: (id: string) => api<void>(`/users/${id}`, { method: 'DELETE' }),
}

// ---- Every user's API tokens (users.manage) ----
export const apiTokensAdminApi = {
  list: () => api<APIToken[]>('/api-tokens'),
  revoke: (id: string) => api<void>(`/api-tokens/${id}`, { method: 'DELETE' }),
}

// ---- Roles ----
export interface RoleInput {
  name: string
  description?: string
  global_privileges: string[]
}
export const rolesApi = {
  list: () => api<Role[]>('/roles'),
  get: (id: string) => api<Role>(`/roles/${id}`),
  create: (body: RoleInput) => api<Role>('/roles', { method: 'POST', body: JSON.stringify(body) }),
  update: (id: string, body: RoleInput) => api<Role>(`/roles/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  remove: (id: string) => api<void>(`/roles/${id}`, { method: 'DELETE' }),
}

// ---- Orbit's own guides (the docs page) ----
export interface Guide {
  slug: string
  title: string
  file: string
}
export interface GuideSection {
  slug: string
  guide: string
  heading: string
  anchor: string
  text: string
}
export const guidesApi = {
  list: () => api<Guide[]>('/docs'),
  get: (slug: string) => api<Guide & { markdown: string }>(`/docs/${encodeURIComponent(slug)}`),
  search: (q: string) => api<GuideSection[]>(`/docs?q=${encodeURIComponent(q)}`),
}

// ---- Audit + settings ----
export const auditApi = {
  list: (page = 1, perPage = 50, q = '', impersonation = false) =>
    api<Paged<AuditEntry>>(`/audit?page=${page}&per_page=${perPage}${q ? `&q=${encodeURIComponent(q)}` : ''}${impersonation ? '&impersonation=1' : ''}`),
  verify: () => api<{ ok: boolean; checked: number; broken_at?: string; reason?: string }>('/audit/verify'),
}
export const settingsApi = {
  branding: () => api<Branding>('/branding'),
  update: (body: Partial<Branding>) => api<Branding>('/settings', { method: 'PUT', body: JSON.stringify(body) }),
}
export const aboutApi = {
  get: () => api<About>('/about'),
}
export interface ProfileInput {
  email?: string
  display_name?: string
  avatar_url?: string
}
export const meApi = {
  updateProfile: (body: ProfileInput) => api<User>('/me/profile', { method: 'PUT', body: JSON.stringify(body) }),
  rotateToken: () =>
    api<{ access_token: string; refresh_token: string; expires_in: number }>('/me/rotate-token', { method: 'POST' }),
  otpSetup: () => api<{ secret: string; otpauth_url: string }>('/me/otp/setup', { method: 'POST' }),
  otpVerify: (code: string) =>
    api<{ backup_codes: string[] }>('/me/otp/verify', { method: 'POST', body: JSON.stringify({ code }) }),
  otpDisable: (code: string) => api<void>('/me/otp/disable', { method: 'POST', body: JSON.stringify({ code }) }),
  otpRegenerateBackup: (code: string) =>
    api<{ backup_codes: string[] }>('/me/otp/backup-codes', { method: 'POST', body: JSON.stringify({ code }) }),
  apiTokens: () => api<APIToken[]>('/me/api-tokens'),
  // privileges / rules (optional) narrow the token below your own rights.
  createApiToken: (name: string, expires_in_days: number, privileges: string[] = [], read_only = false) =>
    api<{ token: string; api_token: APIToken }>('/me/api-tokens', {
      method: 'POST',
      body: JSON.stringify({ name, expires_in_days, privileges, read_only }),
    }),
  revokeApiToken: (id: string) => api<void>(`/me/api-tokens/${id}`, { method: 'DELETE' }),
}

// ---- OIDC / SSO ----
export const oidcApi = {
  getConfig: () => api<OIDCConfig>('/auth/oidc/config'),
  updateConfig: (cfg: OIDCConfig) => api<OIDCConfig>('/auth/oidc/config', { method: 'PUT', body: JSON.stringify(cfg) }),
  testDiscovery: (issuer: string) =>
    api<{ status: string }>('/auth/oidc/test', { method: 'POST', body: JSON.stringify({ issuer }) }),
  exchange: (code: string) =>
    api<{ access_token?: string; refresh_token?: string }>('/auth/oidc/exchange', {
      method: 'POST',
      body: JSON.stringify({ code }),
    }),
}

// ---- Security / login policy (admin) ----
export interface SecurityConfig {
  otp_enforced: boolean
  hide_local_login: boolean
  passwordless_enabled: boolean
  auto_sso: boolean
}
export const securityApi = {
  get: () => api<SecurityConfig>('/security/config'),
  // Send only the changed field(s).
  update: (patch: Partial<SecurityConfig>) =>
    api<SecurityConfig>('/security/config', { method: 'PUT', body: JSON.stringify(patch) }),
}

// ---- Preferences ----
export const prefsApi = {
  get: () => api<NotifyPrefs>('/me/prefs'),
  update: (body: Partial<NotifyPrefs>) => api<NotifyPrefs>('/me/prefs', { method: 'PUT', body: JSON.stringify(body) }),
}

// A person's own notification channels (their Discord webhook, ntfy topic…).
export interface PersonalChannel {
  id: string
  kind: 'discord' | 'slack' | 'teams' | 'telegram' | 'ntfy' | 'gotify'
  name: string
  hint: string
  last_error?: string
  allowed: boolean // its kind is allowed by the owner (else it gets nothing)
  created_at: string
}
export type PersonalChannelInput = {
  kind: PersonalChannel['kind']
  name?: string
  webhook_url?: string
  bot_token?: string
  chat_id?: string
  server_url?: string
  topic?: string
  token?: string
}
export const myChannelsApi = {
  list: () => api<PersonalChannel[]>('/me/channels'),
  add: (b: PersonalChannelInput) => api<PersonalChannel>('/me/channels', { method: 'POST', body: JSON.stringify(b) }),
  test: (id: string) => api<void>(`/me/channels/${id}/test`, { method: 'POST' }),
  remove: (id: string) => api<void>(`/me/channels/${id}`, { method: 'DELETE' }),
}

// ---- Notifications ----
export const notifyApi = {
  // enabled: the owner's channels; personal: the kinds people may add as their own.
  channels: () => api<{ enabled: string[]; personal: string[] }>('/notifications/channels'),
  setPersonalKinds: (kinds: string[]) => api<{ personal: string[] }>('/notifications/personal-kinds', { method: 'PUT', body: JSON.stringify({ kinds }) }),
  getConfig: () => api<NotifConfig>('/notifications/config'),
  updateConfig: (cfg: NotifConfig) => api<NotifConfig>('/notifications/config', { method: 'PUT', body: JSON.stringify(cfg) }),
  test: (channel: string) => api<{ status: string }>('/notifications/test', { method: 'POST', body: JSON.stringify({ channel }) }),
}

// ---- Owner ("god mode") + feature flags ----
export const featuresApi = {
  // GET is also surfaced via /auth/me; this is for the owner-only write.
  list: () => api<Record<string, boolean>>('/features'),
  update: (flags: Record<string, boolean>) =>
    api<Record<string, boolean>>('/features', { method: 'PUT', body: JSON.stringify(flags) }),
}
export const ownerApi = {
  transfer: (userId: string) => api<void>('/owner/transfer', { method: 'POST', body: JSON.stringify({ user_id: userId }) }),
  getSecurityAlerts: () => api<SecurityAlertConfig>('/security-alerts'),
  updateSecurityAlerts: (patch: {
    events?: Record<string, boolean>
    channels?: string[]
    email?: string
  }) => api<SecurityAlertConfig>('/security-alerts', { method: 'PUT', body: JSON.stringify(patch) }),
}

export interface SecurityAlertConfig {
  events: Record<string, boolean>
  channels: string[] | null
  email: string
  available_channels: string[]
}

// ---- Active sessions (behind the "sessions" feature flag) ----
export interface SessionInfo {
  id: string
  user_agent: string
  ip: string
  last_used_at: string
  created_at: string
  current: boolean
}
export const sessionsApi = {
  list: () => api<{ sessions: SessionInfo[] }>('/me/sessions'),
  revoke: (sid: string) => api<void>(`/me/sessions/${sid}`, { method: 'DELETE' }),
  revokeOthers: () => api<void>('/me/sessions/revoke-others', { method: 'POST' }),
}
