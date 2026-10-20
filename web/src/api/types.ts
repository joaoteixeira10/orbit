export interface UserPrefs {
  timezone: string
  theme: 'light' | 'dark' | 'system'
  accent_color: string // personal #rrggbb accent; "" = follow the org accent
  week_start?: 0 | 1 // 0 = Sunday, 1 = Monday
  time_format?: '24h' | '12h'
  device_tz?: string
}

export interface Role {
  id: string
  name: string
  description: string
  builtin: boolean
  superuser: boolean
  global_privileges: string[]
}

// The caller's instance privileges (from /auth/me). What they may do inside
// a workspace is their role there (owner / editor / viewer), not these.
export interface Privileges {
  superuser: boolean
  global: string[]
  read_only?: boolean // a read-only API token
}

export interface User {
  id: string
  username: string
  display_name: string
  email: string
  avatar_url: string
  disabled: boolean
  must_change_password: boolean
  otp_enabled: boolean
  otp_required: boolean // two-factor required for this user (on top of the global policy)
  oidc: boolean // SSO-provisioned; identity fields managed by the IdP
  has_passkey: boolean
  needs_otp_setup: boolean // two-factor required but not set up yet
  last_login_at?: string | null
  locked_until?: string | null // set only while the account is locked
  roles?: Role[]
  prefs?: UserPrefs
}

export interface APIToken {
  id: string
  created_at: string
  name: string
  prefix: string
  expires_at?: string | null
  last_used_at?: string | null
  privileges?: string[] // narrowed to these (absent = all of the owner's)
  read_only?: boolean // workspace content can be read, not changed
  user_id?: string
  username?: string // admin list only
}

export interface WebAuthnCredential {
  id: string
  created_at: string
  name: string
  backup_eligible: boolean
  backed_up: boolean
  last_used_at?: string | null
}

export interface AuditEntry {
  id: string
  user_id: string
  user_name: string // resolved actor: username, "#<id>", or "system"
  action: string
  target_type: string
  target_id: string
  target_name: string // resolved target entity name ("" when not resolvable)
  impersonator_name?: string // real admin when the action was done via impersonation
  ip: string
  detail: string
  created_at: string
}

export interface Branding {
  app_name: string
  accent_color: string
  logo_data_url: string
  favicon_data_url: string
  show_app_name: boolean
  show_logo: boolean
  login_message: string
  oidc_enabled: boolean
  hide_local_login: boolean
  passwordless_enabled: boolean
  auto_sso: boolean
  // Developer themes (owner): a curated accent + UBonito crest / wordmark / favicon.
  developer_themes_enabled?: boolean
  developer_theme?: 'blue' | 'gold' | 'red' | 'ink' | 'green'
  developer_themes_personal?: boolean // each person may pick their own
  asset_source_logo?: 'theme' | 'keep'
  asset_source_wordmark?: 'theme' | 'keep'
  asset_source_favicon?: 'theme' | 'keep'
}

export interface OIDCConfig {
  enabled: boolean
  issuer: string
  client_id: string
  client_secret: string // write-only; placeholder/empty = unchanged
  redirect_url: string
  scopes: string
  role_claim: string
  default_role: string
  allow_admin?: boolean // the identity provider may grant admin roles
  has_secret?: boolean
}

export interface VersionHistoryEntry {
  version: string
  installed_at: string
}

export interface NotifConfig {
  email: { enabled: boolean; host: string; port: number; username: string; password: string; from: string; start_tls: boolean }
  discord: { enabled: boolean; webhook_url: string }
  telegram: { enabled: boolean; bot_token: string; chat_id: string }
  ntfy: { enabled: boolean; server_url: string; topic: string; token: string }
  gotify: { enabled: boolean; server_url: string; token: string }
  teams: { enabled: boolean; webhook_url: string }
  slack: { enabled: boolean; webhook_url: string }
  webhook: { enabled: boolean; url: string; secret: string }
}

export type NotifyKind = 'reminder' | 'assigned' | 'comment' | 'workspace' | 'activity' | 'digest'
export interface Delivery {
  push: boolean // this user's devices (Web Push)
  channels: boolean // their chosen channels (email, ntfy, …)
}
export interface NotifyPrefs {
  timezone: string
  theme: 'light' | 'dark' | 'system'
  notify_events: boolean
  notify_channels: string[]
  channel_details: boolean // shared channels get the text, not just a ping
  notify_prefs: Record<NotifyKind, Delivery>
  digest_enabled: boolean
  digest_time: string // HH:MM
  locale: string
}

export interface About {
  version: string
  go_version: string
  commit: string
  build_time: string
  repository_url: string
  db_driver: string
  latest_version: string
  update_available: boolean
  update_check_enabled: boolean
  version_history: VersionHistoryEntry[]
}
