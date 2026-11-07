# Authentication & Authorization Blueprint

A portable reference of how PKI Management does auth, written so the design can
be lifted into another project. It records **what** each mechanism does, **the
exact parameters**, **where it lives**, and **the traps**.

Stack: Go 1.2x · Gin · GORM (sqlite/mysql/postgres) · Vue 3 + Pinia + Vite.
Snapshot: v2.1.x plus the auth-hardening change (CHANGELOG `[Unreleased]` → next release), 2026-09.
Everything that isn't auth (repo layout, Docker dev loop, CI, releases, docs, tests, update
check, themes) was in the project blueprint, which is no longer kept in this repo.

---

## Required behaviour (acceptance checklist for a project on this base)

Checked against the running app, not only the code. Everything below must work
as in PKI Management. Client-certificate login (§5.4) is optional: a project
may leave it out (LB Admin does).

**Features from PKI, design from the blueprint.** Copy what PKI does, not how it
looks: pages take the Apple-like look of PROJECT-BLUEPRINT §10.2 (Orbit is the
reference; LB Admin keeps v0's neutral greys and PKI its own until they move
over). The one PKI design every project keeps is **how the code is asked for**
— see "The two-factor code prompt" below.

### The two-factor code prompt (the design to keep)

How the app asks for an authenticator code, taken from PKI Management
(`web/src/components/ui/CodeInput.vue`). Keep it exactly like this:

```
                 [mark]  App name

   ┌──────────────────────────────────────────────┐
   │ Two-factor authentication                    │
   │                                              │
   │ Enter the 6-digit code from your             │
   │ authenticator app.                           │
   │                                              │
   │     ┌──┐ ┌──┐ ┌──┐ ┌──┐ ┌──┐ ┌──┐            │
   │     │  │ │  │ │  │ │  │ │  │ │  │            │
   │     └──┘ └──┘ └──┘ └──┘ └──┘ └──┘            │
   │                                              │
   │ [               Verify                    ]  │
   │                ───── or ─────                │
   │ [ (fingerprint)   Use a passkey           ]  │  ← only when the user has one
   │                                              │
   │ ← Back                 (key) Use a recovery code │
   └──────────────────────────────────────────────┘
```

- **Six separate boxes**, one digit each, centred; numeric keyboard on phones.
- Typing moves to the next box; Backspace clears and goes back; ← / → move.
- **Paste** of the whole code fills every box; the browser's / password
  manager's one-time-code autofill works (`autocomplete="one-time-code"`).
- The code is **sent as soon as the sixth digit is in** (no need to press
  Verify); Verify stays for the keyboard and screen readers.
- A wrong code clears the boxes and shows the error under them.
- The **Verify** button is full width (the primary colour).
- "or" + **Use a passkey** only when the account has a passkey.
- The bottom row: **← Back** on the left (to the user name and password),
  **Use a recovery code** on the right in the accent colour. Recovery mode
  swaps the boxes for one input (`xxxxx-xxxxx`) and the link becomes **Use
  authenticator code**.
- The same six boxes (and send-on-sixth-digit) wherever a code is typed:
  the sign-in second step, confirming a new authenticator (under the QR code,
  then the 10 recovery codes), **Regenerate** recovery codes, and **Disable**
  two-factor (a small dialog, confirmed with a code).
- Everything else around it (colours, fonts, the card) is the project's look
  (PROJECT-BLUEPRINT §10.2).

- **`/login?local=1` shows every way in that is enabled**: the password form
  (even with "Hide password login" on), the passkey button (when passwordless is
  on) and the SSO button (when SSO is on). It also skips auto-start SSO. It is
  the escape hatch when the IdP is down, and the Security tab says so.
- **No sign-out to set up two-factor or a passkey.** A signed-in user enrols
  TOTP or adds a passkey from Settings → Two-factor in the same session. When
  two-factor is required (policy or per user), the forced-setup overlay appears
  right after sign-in, offers TOTP **or** a passkey, and the user carries on in
  the same session once done.
- **TOTP always gives recovery codes.** Verifying the first code shows 10
  single-use recovery codes once (copy / download). Settings → Two-factor shows
  how many are left, "Regenerate" (needs a current code) and "Disable" (needs a
  current code; refused while two-factor is required). The second-factor step
  of the login page has "Use a recovery code".
- **SSO is set up in the page**, Admin → SSO (owner only, step-up when on):
  on/off, issuer URL with a **Test** button (discovery dry run), client ID,
  client secret (write-only, sealed), redirect URL to register at the provider
  (`https://<host>/api/v1/auth/oidc/callback`), extra scopes, role claim,
  default role. New users are provisioned at first sign-in; role claim values
  are matched to role names on every sign-in, else the default role.
- **Users: Disable / Enable button on each row** (not for yourself). Disabling
  asks first, then signs the user out everywhere at once (token version bumped,
  sessions revoked, API tokens refused); enabling again doesn't revive old tokens.
- **Who can act on whom**: nobody acts on the **owner's** account
  (edit, roles, disable, reset password / two-factor / passkeys, unlock, sign out,
  delete, revoke tokens) — the owner uses Settings; only the **owner** acts on an
  **admin's** account (admins can't act on each other); `users.manage` alone
  never reaches a stronger account. API tokens can't mint tokens.
- **SSO can't make admins** unless the owner turns on "Let the identity provider
  grant admin" (Owner → SSO): an admin role from the claim or as the default is
  ignored otherwise (the user gets `none`).
- **Ended sessions stop at once** (sign-out, Active sessions, refresh-token
  reuse): each request checks its session. **Every failed sign-in is audited**
  (`auth.login_failed`, the name tried). **Lockout applies to passkeys too.**
- **Admin → Security** (owner): enforce two-factor, hide password login (with
  the `/login?local=1` hint), auto-start SSO, allow passwordless passkey sign-in.
- **Admin → Owner**: feature flags (active sessions, user impersonation, step-up
  re-authentication), **Live updates** (the refresh interval), security alerts, and
  the Danger zone (ownership transfer, confirmed by typing the new owner's name).
- **Settings** (every user): Account (photo, display name, email), Appearance
  (theme, accent, **change confirmations** detailed / compact unless an admin
  enforces one), Language & time, Notifications, Password, Two-factor, API
  tokens, Active sessions (when the flag is on).

---

## 0. TL;DR — the model in one screen

| Concern | Choice |
|---|---|
| Browser session | **Access JWT (HS256, 15 min) in JS memory only** + **refresh token (opaque, 30 d) in an `HttpOnly; SameSite=Strict; Path=/api/v1/auth` cookie** |
| CSRF | Double-submit cookie (`pki_csrf` readable cookie ↔ `X-CSRF-Token` header), enforced only on `/auth/refresh` + `/auth/logout`, the two endpoints that read the cookie. Every other route uses a bearer header, so CSRF can't reach it. |
| Refresh | Rotating: each refresh marks the used row rotated and mints a new one with the **same stable `SessionID`**. A rotated token replayed within **30 s** is accepted (tab race). Past that it counts as **reuse**, which revokes the whole session plus an audit entry and alert. |
| Instant revoke | `users.token_version` is embedded in the JWT (`tv`). Bumping it kills every live access token on the next request |
| Authz lookup | **Per request**: the middleware loads the user + roles + grants from the DB. The JWT carries **no roles**, only `sub`, `tv`, `sid`, `imp`, `sua` |
| Passwords | bcrypt cost 12, constant-time dummy compare for unknown users |
| Brute force | Per-IP token bucket on auth endpoints, plus a per-account temporary lockout (5 fails → 15 min) |
| 2FA | TOTP (SHA1/6 digits/30 s, ±1 step) with the secret sealed (AES-GCM) + 10 sha256-hashed single-use recovery codes; **or** a passkey as the second factor |
| Passwordless | WebAuthn discoverable login + conditional UI (autofill) |
| SSO | OIDC auth-code + **PKCE S256 + nonce + state**, JIT provisioning by `sub`, IdP claim → role-name mapping, **one-time 60 s code** handed to the SPA (tokens never in the URL; the code references the session, and the access token is minted at exchange) |
| Pre-auth state | OIDC state, WebAuthn challenges, SSO codes and SSE tickets live in a **shared DB table** (`ephemeral_tokens`): sha256-hashed ids, namespaced by kind, single-use via delete-and-check, swept every 15 min. Works across replicas and restarts. |
| Client-cert login | mTLS via proxy header **or** app-terminated TLS. The app **always re-verifies** the chain, the clientAuth EKU, revocation and an optional OU, then maps the email SAN (case-insensitive) to a user. Emails are unique, and self-service email edits are locked while cert login is on. |
| Programmatic | Personal API tokens `pkim_<base64url 32B>`, sha256-hashed at rest, inherit the owner's RBAC, optional expiry |
| RBAC | Roles = global privileges + scoped grants (all resources or a list of resource IDs). Effective set = union. Superuser flag. |
| "Owner" | One identity-level super-admin (not a privilege, so it can't be self-granted). Gates the most dangerous config. Transferable. |
| Step-up | Dangerous routes need a fresh re-auth (password or passkey) ≤ 5 min old, carried as the `sua` claim in a re-minted access token |
| Impersonation | 15-min access token with `imp=<admin id>` and **no refresh token**. The owner can't be impersonated, and only the owner may impersonate a superuser. |
| Live updates | SSE stream authenticated by a 30 s single-use ticket (EventSource can't send headers). It pushes "privileges changed" and the SPA refetches `/auth/me`. |

---

## 1. File map (where to copy from)

Backend (`api/internal/…`):

| File | What |
|---|---|
| `auth/manager.go` | `Manager`: Authenticate, lockout, refresh-token issue/rotate, sessions list/revoke |
| `auth/jwt.go` | Claims, `IssueAccessToken`, `IssueStepUpToken`, `IssueImpersonationToken`, `ParseAccessToken` |
| `auth/password.go` | bcrypt + `DummyCompare` |
| `auth/totp.go` | TOTP generate/validate |
| `auth/apitoken.go` | API token mint + bearer path |
| `auth/middleware.go` | `Required()`, `RequireGlobal`, `RequireOwner`, `RequireOwnerOrGlobal`, context getters, forced-flow allowlists |
| `auth/oidc.go` | OIDC settings (DB), discovery cache, `BeginLogin`/`Finish` (state + nonce + PKCE verifier kept in the `ephemeral` store) |
| `auth/webauthn.go` | go-webauthn wrapper, ceremony challenges (`ephemeral` store), credential <-> model adapters |
| `auth/certlogin.go` | Cert-login settings (DB) |
| `auth/bootstrap.go` | First-run admin + `EnsureOwner` backfill |
| `ephemeral/ephemeral.go` | Shared single-use short-lived token store (`Put`/`Take`/`Sweep`) |
| `rbac/rbac.go`, `rbac/seed.go` | Privilege catalog, `Effective`, built-in roles |
| `api/auth_cookies.go` | Cookie set/clear, CSRF check |
| `api/handlers_auth.go` | login / refresh (reuse → audit + alert) / logout / me / change password / profile / rotate token / `issueTokens` = `startSession` + `accessResponse` |
| `api/handlers_oidc.go` | SSO login/callback/exchange, JIT provisioning, role mapping, admin config |
| `api/handlers_webauthn.go` | Passkey register/list/rename/delete, login begin/finish |
| `api/handlers_otp.go` | TOTP setup/verify/disable, backup codes |
| `api/handlers_certlogin.go` | Cert login, header parsing, verification, email→user, admin config + dry-run test |
| `api/handlers_stepup.go` | `requireStepUp()` middleware + step-up endpoints |
| `api/handlers_impersonate.go` | Impersonation |
| `api/handlers_sessions.go` | Self-service sessions |
| `api/handlers_apitoken.go` | API token CRUD (self + admin) |
| `api/handlers_users.go` | Admin: reset password/OTP/passkeys, force logout, last-admin guards |
| `api/handlers_security.go` | DB-backed live security policy (enforce OTP, hide local login, auto-SSO, …) |
| `api/handlers_owner.go` | Feature flags, owner transfer |
| `api/middleware.go` | Security headers/CSP, body limit, per-IP rate limiter |
| `api/router.go` | Route table + middleware order |
| `config/config.go` | Env config + release-mode secret checks |
| `keystore/keystore.go` | `Sealer` (AES-256-GCM, Argon2id-derived KEK) used for TOTP secret + OIDC client secret |
| `safedial/` | SSRF-guarded HTTP client used for OIDC discovery/JWKS/token exchange |
| `feature/feature.go` | Feature flags (`sessions`, `impersonation`, `step_up_auth`) |
| `events/hub.go` | SSE ticket (via `UseTicketStore(ephemeral)`) + in-process pub/sub |
| `jobs/scheduler.go` | Sweeps: API tokens (daily), ephemeral tokens (15 min) |
| `scripts/db-smoke.sh` + `scripts/db-smoke/compose.yml` | `make db-smoke DB=postgres\|mysql`: boot against the real engines |

Frontend (`web/src/…`):

| File | What |
|---|---|
| `lib/api.ts` | In-memory access token, `tryRefresh` (single-flight + Web Locks), 401→refresh→retry, CSRF header, step-up bridge, impersonation stash |
| `stores/auth.ts` | Pinia store: login flows, `adopt`, `fetchMe`, `restore`, privilege helpers `can`/`canOnCA`/`canOwnerGated` |
| `api/webauthn.ts` | `@simplewebauthn/browser` ceremonies |
| `views/LoginView.vue` | All login paths, SSO return, auto-login, `?local=1` escape hatch, safe `redirect` |
| `router/index.ts` | `beforeEach` guard (`meta.public`, `meta.role`) |
| `App.vue` + `components/ForcePasswordChange.vue`, `ForceOtpSetup.vue`, `ui/StepUpModal.vue` | Forced-flow overlays + step-up modal |
| `main.ts` | `await Promise.all([branding.load(), auth.restore()])` **before** `app.mount` |

Libraries: `golang-jwt/jwt/v5`, `coreos/go-oidc/v3`, `golang.org/x/oauth2`,
`go-webauthn/webauthn`, `pquerna/otp`, `golang.org/x/crypto` (bcrypt, argon2),
`gin-contrib/cors`; web: `@simplewebauthn/browser` v13.

---

## 2. Data model (GORM)

All PKs are UUIDv7 strings (`Base` = ID/CreatedAt/UpdatedAt). **Byte columns
carry no explicit SQL type** (GORM then picks `blob`/`longblob`/`bytea` per
dialect). The uniquely-indexed ones get `size:255`, so MySQL uses an indexable
`varbinary(255)`. Never write `type:blob`, because Postgres has no such type.

```go
type User struct {
  Base
  Username           string `gorm:"uniqueIndex;size:190;not null"`
  DisplayName, Email string
  PasswordHash       string `json:"-"`      // bcrypt; "" for pure SSO users
  Disabled           bool
  Owner              bool   `json:"-"`      // exactly one; identity, not privilege
  OIDCSubject        string `gorm:"index" json:"-"` // column = o_id_c_subject (!)
  TokenVersion       uint   `gorm:"default:1" json:"-"` // bump = kill live JWTs
  MustChangePassword bool
  FailedLoginCount   int        `json:"-"`
  LockedUntil        *time.Time `json:"-"`
  OTPSecretEnc       []byte `json:"-"`      // sealed
  OTPEnabled         bool
  OTPBackupCodes     string `json:"-"`      // JSON []sha256hex
  WebAuthnUserHandle []byte `gorm:"size:255;uniqueIndex" json:"-"` // random 32B, NOT the DB id
  Roles              []Role `gorm:"many2many:user_roles;"`
}

type RefreshToken struct {       // one row per rotation; SessionID groups them
  Base
  UserID, SessionID string
  TokenHash  string `gorm:"uniqueIndex"` // sha256 hex
  UserAgent, IP string
  LastUsedAt, ExpiresAt time.Time
  Revoked   bool
  RotatedAt *time.Time // set when spent by a refresh; distinguishes rotated vs logged-out
}

type EphemeralToken struct {    // shared single-use pre-auth state
  KeyHash   string `gorm:"primaryKey;size:64"` // sha256 of the random id handed out
  Kind      string // oidc_state | webauthn | sso_code | sse_ticket
  Data      string `gorm:"type:text"` // JSON
  ExpiresAt time.Time `gorm:"index"`
}

type APIToken struct { Base; UserID, Name, Prefix string; TokenHash string `gorm:"uniqueIndex"`; ExpiresAt, LastUsedAt *time.Time; Revoked bool }

type WebAuthnCredential struct {
  Base; UserID, Name string
  CredentialID []byte `gorm:"size:255;uniqueIndex"`
  PublicKey, AAGUID []byte; AttestationType, Transports string
  SignCount uint32; CloneWarning, BackupEligible, BackupState, UserPresent, UserVerified bool
  LastUsedAt *time.Time
}

type Role      struct { Base; Name string `gorm:"uniqueIndex"`; Description string; Builtin, Superuser bool; GlobalPrivileges string /*JSON []*/; Grants []RoleGrant }
type RoleGrant struct { Base; RoleID string; AllAuthorities bool; AuthorityIDs string /*JSON []*/; Privileges string /*JSON []*/ }

type Setting struct { Key string `gorm:"primaryKey"`; Value string; UpdatedAt time.Time } // key/value store for runtime policy
// Query it portably: db.Where(&models.Setting{Key: k}).First(&row). "key" is
// reserved in MySQL, and backtick quoting is invalid SQL on Postgres.
```

Rules worth keeping:
- **Store only hashes** of refresh tokens, API tokens and recovery codes (sha256
  is enough because they're high-entropy random values; bcrypt is for passwords).
- **Seal** the secrets you must read back (TOTP secret, OIDC client secret) with
  AES-GCM under a KEK. Never return them to the UI. Show a `********` placeholder
  and treat an unchanged placeholder as "keep the current value".
- WebAuthn user handle is a **random opaque value**, not the DB id (privacy +
  discoverable-login lookup key).

---

## 3. Tokens & sessions

### 3.1 Access token (JWT HS256, 15 min)
```go
type Claims struct {
  Version  uint   `json:"tv"`            // must equal users.token_version
  Sid      string `json:"sid,omitempty"` // RefreshToken.SessionID → "this device" in sessions UI
  Imp      string `json:"imp,omitempty"` // impersonator id
  StepUpAt int64  `json:"sua,omitempty"` // unix secs of last re-auth
  jwt.RegisteredClaims                    // sub=user id, iat, exp
}
```
- Parse **pins the alg family** (`*jwt.SigningMethodHMAC`) to block alg confusion.
- `JWT_SECRET` must be ≥ 32 chars in release mode, or startup fails.

### 3.2 Refresh token (opaque, 30 d, rotating)
- `base64url(32 random bytes)`, stored as sha256.
- `IssueRefreshToken` runs on every fresh login and creates a new `SessionID` (16 random bytes hex).
- `Refresh`: look up by hash, then:
  - revoked with no `rotated_at` (logout/admin) → invalid;
  - rotated **> 30 s ago** → **reuse**: revoke the whole session and return `ErrRefreshReuse` with the user.
    The handler audits `auth.refresh_reuse`, raises the `refresh_reuse` alert and clears the cookies;
  - rotated ≤ 30 s ago → allowed if the session is still live (benign tab race), minting a sibling;
  - then expired/user disabled → invalid; else set `revoked=true, rotated_at=now` (only if still unrevoked)
    and insert a new row with the same SessionID → new access token with `sid`.
- Logout revokes the **whole session**, so a just-rotated predecessor can't resurrect it inside the grace window.
- Sessions list = distinct live SessionIDs ordered by `last_used_at`.
- Revoke one session / all others / all (admin force-logout).

### 3.3 Cookies (`api/auth_cookies.go`)
```go
c.SetSameSite(http.SameSiteStrictMode)
c.SetCookie("pki_refresh", refresh, maxAge, "/api/v1/auth", domain, secure, true)  // HttpOnly
c.SetCookie("pki_csrf",    random24B, maxAge, "/",            domain, secure, false) // readable
```
- `COOKIE_SECURE` defaults to true in release mode or when the app serves TLS.
- `COOKIE_DOMAIN` is empty by default (host-only). Set it to the parent domain only for split-origin setups.
- The token response body **never** contains the refresh token.
- `/auth/refresh` and `/auth/logout` require `X-CSRF-Token == cookie pki_csrf`.

### 3.4 Instant invalidation levers
| Action | Mechanism |
|---|---|
| Logout | revoke the **whole session** the cookie belongs to + clear cookies |
| Change own password | `token_version++` + revoke **other** sessions. Returns a fresh access token for the current session (`200 {access_token}`). |
| Admin reset password | `token_version++` + revoke all sessions + `must_change_password` (request field, default `true`) |
| Rotate token (self) | `token_version++` + revoke all refresh rows + new pair |
| Force logout (admin) | `token_version++` + `RevokeAllSessions` → **instant** |
| Disable user | middleware rejects on the next request (user loaded per request) |

---

## 4. Request pipeline

Global middleware order (`router.go`):
```
SetTrustedProxies(TRUSTED_PROXIES)   // empty = trust none → ClientIP is socket peer
gin.Recovery → requestID → requestLogger → securityHeaders(CSP) → limitBody(4MiB) → metrics
CORS (only if CORS_ORIGINS set; AllowCredentials=true, +Authorization header)
```

`auth.Required()` (per authenticated group):
1. Bearer missing → 401.
2. Bearer starts with `pkim_` → API-token path (hash lookup, revoked/expiry,
   throttled `last_used_at`, disabled check). It **skips** the forced
   password/OTP flows.
3. Otherwise parse JWT → load user **with `Roles.Grants`** → 401 if missing/disabled/`tv` mismatch.
4. `MustChangePassword` → 403 `password_change_required` unless path ∈ {`/auth/me`, `/me/password`, `/auth/logout`}.
5. OTP enforced and local user has neither TOTP nor passkey → 403 `otp_setup_required`
   unless path ∈ {me, logout, otp setup/verify, webauthn register begin/finish}.
   **SSO users are exempt** because MFA is the IdP's job.
6. Put `user_id`, `rbac.Effective`, `owner`, `sid`, `imp`, `sua` in the context.

Route-level gates, composed left-to-right:
```go
authed.POST("/users/:id/impersonate", RequireOwnerOrGlobal(PUsersImpersonate), requireStepUp(), h.impersonateUser)
authed.PUT("/auth/oidc/config",       RequireOwner(),                           requireStepUp(), h.updateOIDCConfig)
authed.GET("/users",                  RequireGlobal(PUsersManage),                               h.listUsers)
```
Handlers do the per-resource checks themselves (`Effective(c).HasOnCA(priv, id)`).

Error-code contract the SPA relies on:
- `401` → try refresh, retry once, else "session expired" → /login
- `403 {"error":"password_change_required"}` / `otp_setup_required` → forced overlays
- `403 {"error":"step_up_required"}` → step-up modal → retry once
- `429` + `Retry-After: 30` → rate limited

---

## 5. Login methods

### 5.1 Password (+ second factor), `POST /auth/login` (rate-limited)
```
Authenticate(username, password)
  unknown user → DummyCompare, generic 401
  locked       → DummyCompare, generic 401 (no audit spam)
  bad password → RecordFailure; if it just locked → audit "auth.lockout"; generic 401
  disabled     → 401 (only after a correct password)
if body.otp != "":            validate TOTP, else try a backup code; fail → RecordFailure + 401 {otp_required:true}
elif user has TOTP or passkey: 200 {otp_required?, webauthn_required?, webauthn:{session_id, options}}
                               (client re-posts username+password+otp, or finishes the passkey)
issueTokens → RecordLoginSuccess → audit "auth.login"
```
The flow is **stateless between steps**: the password is re-checked on the OTP
step. Failed second factors count toward the same lockout.

### 5.2 Passkeys (WebAuthn)
- Config: `WEBAUTHN_RP_ID` (the registrable domain, effectively permanent),
  `WEBAUTHN_RP_ORIGINS` (full origins), `WEBAUTHN_RP_DISPLAY_NAME`. Empty RP ID → every handler returns 501.
  Dev default: `localhost` + `http://localhost:5173,http://localhost:8080`.
- Registration: `ResidentKey=preferred`, `UserVerification=preferred`, exclude existing creds.
- The challenge store is the shared ephemeral table (kind `webauthn`), keyed by a random opaque `session_id`, single-use, **3 min TTL**.
  Registration challenges are bound to the user who began them.
  The client gets `{session_id, options}` from *begin*, posts the raw ceremony JSON to *finish*`?sid=`.
- Discoverable login: resolve the user by `userHandle` → `web_authn_user_handle`, reject disabled users.
- After each assertion, persist `sign_count`, `clone_warning`, `backup_state` and `last_used_at`.
- Passwordless can be switched off at runtime (`passwordless_enabled` setting, default **on**).
- A registered passkey counts as MFA for the "enforce OTP" policy.
- SPA: conditional UI (`useBrowserAutofill: true`) is armed on the login page mount and
  cancelled (`WebAuthnAbortService.cancelCeremony()`) when another path is taken.

### 5.3 OIDC SSO
Settings live in the DB (`settings.oidc_config` JSON), are owner-only, and changes need step-up:
`enabled, issuer, client_id, client_secret_sealed, redirect_url, scopes (extra), role_claim, default_role`.

```
GET /auth/oidc/login     → ephemeral.Put(oidc_state, {nonce, PKCE verifier}, 10 min) → id is the state → 302 IdP
GET /auth/oidc/callback  → Take(state) single-use → exchange code w/ verifier → verify id_token (aud=client_id)
                           → check nonce → claims → provision → startSession (sets refresh cookie)
                           → ephemeral.Put(sso_code, {user_id, session_id}, 60 s) → 302 /login?sso=<code>
POST /auth/oidc/exchange → Take(code) → reload user (reject disabled) → mint access token for that session
                           → {access_token, user}   (rate-limited)
errors                   → 302 /login?sso_error=disabled|unavailable|idp_error|auth_failed|provisioning|token
```
- Scopes are always `openid profile email`, plus the extra ones.
- Username = `preferred_username` → `email` → `sub`, made unique with a `-<sub[:6]>` suffix.
- **JIT provisioning keyed on `sub`** (never email). Email and display name are synced on every login.
- Role mapping: claim values (string or list) are matched **by role name**. With no match
  the user gets `default_role`, falling back to the built-in **`none`** role (zero access).
  When `role_claim` is set, roles are **replaced on every login**.
- SSO users can't change or reset their password or edit email/display name locally, and are exempt from local MFA.
- All outbound calls (discovery, JWKS, token) go through `safedial.GuardedClient`:
  it refuses loopback/link-local/metadata IPs, follows no redirects, and checks the IP it actually dials, which defeats DNS rebinding.
- The provider is cached per issuer. `POST /auth/oidc/test` does a discovery dry-run.
- IdP walkthrough: [SSO-authentik.md](SSO-authentik.md).

### 5.4 Client-certificate (mTLS) login, `POST /auth/cert`
Settings live in the DB (`settings.certlogin_config`) and are owner-only. The fields are
`mode (proxy-header|app-mtls), ca_id | ca_bundle_pem, header_cert, header_verify, required_ou, default_role, auto_provision, login_url`.
1. Get the cert. In proxy mode it comes from the header (`X-SSL-Client-Cert`: base64 DER from HAProxy or
   URL-escaped PEM from nginx, both auto-detected). An explicit negative
   `X-SSL-Client-Verify` (anything but `SUCCESS`/`0`) is rejected early. In app-mtls mode it comes from
   `r.TLS.PeerCertificates[0]` on a second listener (`MTLS_ADDR`, `tls.RequestClientCert`).
2. **Re-verify it yourself**: `x509.Verify` against the anchor pool (put it in both Roots and
   Intermediates), `ExtKeyUsageClientAuth`, check our own revocation status, optional OU match.
3. Identity = the first **email SAN**, matched **case-insensitively** (`LOWER(email) = ?`).
   It must match **exactly one enabled** user, or auto-provision with `default_role` → `none`.
   To keep this identity trustworthy:
   - `email` is **unique** (case-insensitive) on create user, admin update, and self-service profile update (409 otherwise);
   - while cert login is enabled, **self-service email changes are refused** (403; admins still can);
   - `POST /auth/cert/test` is a **pure dry run**. It returns `would_provision: true` instead of creating the user.
4. Split-origin (prompt on click, not on page load): `login_url` = the cert host,
   `COOKIE_DOMAIN` = the parent domain, main origin added to `CORS_ORIGINS`. The CSP `connect-src` picks up the origin automatically.
- The proxy must **strip client-supplied copies** of the header. Snippets are in `reference/CERT-LOGIN.md` in the blueprint folder (from PKI Management; LB Admin and Orbit have no certificate login).

### 5.5 API tokens
`pkim_` + base64url(32B). The plaintext is shown once, the sha256 is stored, and the display
prefix is the first 12 chars. Expiry is optional (`expires_in_days`, 0 = never). The scheduler
sweeps expired tokens. Admins can list and revoke anyone's tokens (`users.manage`).
**Narrowing (recommended):** a token can be **limited** at creation to some privileges
and / or scope rules (`privileges`, `rules`). The request's rights are the owner's
narrowed by them (`rbac.Effective.Narrow`: rules are ANDed with the role rules),
so a token never has more than its owner, and it never carries owner status.

### 5.6 Bootstrap
First boot with an empty `users` table creates `ADMIN_USER`/`ADMIN_PASSWORD` as admin **and
owner**. `EnsureOwner` backfills an owner on older installs. Built-in roles are seeded
idempotently, and the Admin role's privileges are reconciled on every boot.

---

## 6. TOTP & recovery codes
- `setup` generates a secret (issuer "PKI Management", account = username) and stores it **sealed**
  with `otp_enabled=false`. It returns `secret` + `otpauth_url` for the QR code.
- `verify` checks the code, sets `otp_enabled=true` and returns **10 recovery codes** once
  (`xxxxx-xxxxx`, 5 random bytes hex; stored as sha256 of the lowercased value with dashes/spaces stripped).
- A recovery code is accepted wherever a TOTP code is (login) and is consumed on use.
- Regenerate codes or disable TOTP only with a current TOTP code. You can't disable it while `otp_enforced` is on.
- Admin: `reset-otp`, `reset-webauthn`.
- verify/disable/backup-codes are behind the auth rate limiter.

---

## 7. Authorization (RBAC)

```
Global privileges : users.manage roles.manage audit.read ca.create cert.request cert.approve
                    settings.branding* settings.notifications* users.impersonate*   (*owner-only)
Scoped privileges : ca.read ca.edit ca.delete ca.offline cert.read cert.issue cert.revoke cert.key
```
- `Effective{Superuser, Global, AllAuthorities, PerAuthority[resID]}` is built from the union of roles.
- `HasGlobal(p)`: superuser implies all, **except owner-only privileges**, which must be held
  explicitly or come from being the owner (`RequireOwnerOrGlobal`).
- `HasOnCA(p, id)`: superuser, or an all-authorities grant, or a per-resource grant.
- `ScopedCAIDs(p)` → (all, ids), used to filter list queries.
- Built-in roles (immutable): `admin` (superuser), `operator`, `viewer`, `none`.
- Escalation guards:
  - Only the owner can create a superuser account or grant/revoke superuser.
  - Only the owner can add/remove owner-only privileges on a role.
  - You can't demote, disable or delete the last active superuser, or delete yourself.
  - The owner can't be impersonated. Only the owner may impersonate a superuser.
- **Owner** (identity flag, exactly one, transferable in a transaction; see the naming note in
  the project blueprint). Owner-only:
  feature flags, owner transfer, security policy, SSO config, cert-login config,
  backup export/restore, root-key export, security alerts.
- `/auth/me` returns `privileges` (sets as arrays), `is_owner`, `features`,
  `must_setup_otp`, `has_passkey`, `webauthn_enabled`, `otp_backup_remaining`, `impersonator`.
  The SPA mirrors the checks (`can`, `canOnCA`, `canAnyCA`, `canOwnerGated`) **for UI only**. The server is authoritative.

---

## 8. Step-up, impersonation, sessions (feature-flagged)

Flags live in `settings.features` (JSON), default **off**, and only the owner can change them:
`sessions`, `impersonation`, `step_up_auth`. With a flag off, its routes return 404, or the step-up middleware becomes a no-op.

- **Step-up**: `requireStepUp()` needs `sua` within **5 min**. `POST /auth/step-up {password}`
  or `/auth/step-up/webauthn/begin|finish` (the asserted user must equal the caller) re-mints
  the access token with `sua=now`, keeping the same `sid` and `tv`. The SPA's `api()` catches
  `step_up_required`, opens the modal and retries once.
- **Impersonation**: returns a 15-min token with `imp` and no refresh. The SPA stashes the admin
  access token in memory, and the admin's refresh cookie stays. When the impersonation token
  gets a 401, the SPA returns to the admin instead of refreshing. Audit rows carry `impersonator_id`.
  The event triggers a security alert.
- **Sessions**: list (with `current` flag), revoke one, revoke others.

---

## 9. Runtime policy (DB `settings`, live, no restart)

| Key | Default | Effect |
|---|---|---|
| `otp_enforced` | false | local users must enroll TOTP or a passkey |
| `hide_local_login` | false | hide the password form when SSO/cert is on. `/login?local=1` always shows it together with every other enabled method (passkey, SSO), and `/auth/login` always works. |
| `passwordless_enabled` | **true** | allow discoverable passkey login |
| `auto_sso` | false | the login page auto-redirects to the IdP |
| `auto_cert` | false | the login page auto-posts `/auth/cert` |
| `features` | all off | `sessions`, `impersonation`, `step_up_auth` |
| `oidc_config` / `certlogin_config` | – | see §5 |

Invariants enforced on save: at most one auto-login, and only if its method is enabled. Disabling
SSO or cert login turns its auto-login off. Changes are audited and raise the `config_change` security alert.

Public `GET /branding` tells the login page what to render: `oidc_enabled`,
`cert_login_enabled`, `cert_login_url`, `hide_local_login`, `passwordless_enabled`,
`auto_sso`, `auto_cert`.

---

## 10. Environment variables (auth-relevant)

| Var | Default | Notes |
|---|---|---|
| `GIN_MODE` | debug | `release` enforces secrets + Secure cookies |
| `JWT_SECRET` | dev fallback | ≥ 32 chars in release |
| `PKI_MASTER_KEY` | dev fallback | KEK for sealing (Argon2id over a stored salt → AES-256-GCM). **Must never change.** |
| `ADMIN_USER` / `ADMIN_PASSWORD` | admin / – | first run only |
| `SECRETS_BACKEND` | env | `env` \| `file` (`<KEY>_FILE`) \| `vault` |
| `COOKIE_SECURE` | release or TLS | |
| `COOKIE_DOMAIN` | host-only | only for split-origin |
| `CORS_ORIGINS` | – | dev or split-origin; credentials allowed |
| `TRUSTED_PROXIES` | none | **set this behind a proxy**, or the IP used for rate limits and audit can be spoofed/wrong |
| `AUTH_RATE_BURST` / `AUTH_RATE_PER_MINUTE` | 8 / 12 | per-IP bucket on login, oidc exchange, cert, webauthn login, OTP verify/disable/backup |
| `LOGIN_LOCKOUT_THRESHOLD` / `LOGIN_LOCKOUT_MINUTES` | 5 / 15 | 0 disables the lockout |
| `MAX_BODY_BYTES` | 4 MiB | |
| `WEBAUTHN_RP_ID` / `WEBAUTHN_RP_ORIGINS` / `WEBAUTHN_RP_DISPLAY_NAME` | – | empty RP ID disables passkeys |
| `MTLS_ADDR` / `MTLS_CERT_FILE` / `MTLS_KEY_FILE` | – | app-terminated cert-login listener |
| `TLS_CERT_FILE` / `TLS_KEY_FILE` | – | HTTPS on the main listener (TLS 1.2+) |

---

## 11. Endpoint table

Public (rate-limited ones are marked ⏱):

| Method | Path | Purpose |
|---|---|---|
| POST ⏱ | `/auth/login` | password (+`otp`) |
| POST | `/auth/refresh` | cookie + CSRF → new access, rotated cookie |
| POST | `/auth/logout` | cookie + CSRF → revoke, clear |
| GET | `/auth/oidc/login`, `/auth/oidc/callback` | SSO redirects |
| POST ⏱ | `/auth/oidc/exchange` | one-time code → tokens |
| POST ⏱ | `/auth/cert` | client-cert login |
| POST ⏱ | `/auth/webauthn/login/begin`, `/finish?sid=` | passwordless / 2FA passkey |
| GET | `/auth/events?ticket=` | SSE |
| GET | `/branding` | login-page capabilities |

Authenticated:

| Path | Gate |
|---|---|
| `GET /auth/me`, `POST /auth/events/ticket` | – |
| `POST /auth/step-up`, `/auth/step-up/webauthn/begin`, `/finish` | – |
| `GET /me/sessions`, `POST /me/sessions/revoke-others`, `DELETE /me/sessions/:sid` | flag `sessions` |
| `PUT /me/profile` (email unique; locked while cert login on), `PUT /me/password` (→ `200 {access_token}`, ends other sessions), `POST /me/rotate-token` | – |
| `POST/GET/DELETE /me/api-tokens[/:id]` | – |
| `POST /me/otp/setup`, `verify`⏱, `disable`⏱, `backup-codes`⏱ | – |
| `POST /me/webauthn/register/begin`, `/finish?sid=&name=`; `GET /me/webauthn`; `PATCH/DELETE /me/webauthn/:id` | – |
| `/users…` CRUD, `PUT /users/:id/password` (`{new_password, must_change_password?=true}`), `POST /users/:id/reset-otp`, `/reset-webauthn`, `/logout` | `users.manage` |
| `POST /users/:id/impersonate` | owner or `users.impersonate` + step-up + flag |
| `GET/DELETE /api-tokens[/:id]` | `users.manage` |
| `/roles…`, `GET /privileges` | `roles.manage` |
| `GET/PUT /features`, `POST /owner/transfer` | owner (+ step-up on writes) |
| `GET/PUT /security/config` | owner (+ step-up on PUT) |
| `GET/PUT /auth/oidc/config`, `POST /auth/oidc/test` | owner (+ step-up on PUT) |
| `GET/PUT /auth/cert/config`, `POST /auth/cert/test` | owner (+ step-up on PUT) |

---

## 12. Frontend pattern (Vue 3 + Pinia)

```ts
// lib/api.ts — the core of it
let accessToken: string | null = null            // memory only, never localStorage

async function rawFetch(path, init) {
  const h = new Headers(init.headers)
  if (accessToken) h.set('Authorization', `Bearer ${accessToken}`)
  const csrf = readCookie('pki_csrf'); if (csrf) h.set('X-CSRF-Token', csrf)
  return fetch(`/api/v1${path}`, { ...init, credentials: 'include', headers: h })
}

// Refreshes are coordinated: the refresh token rotates and all tabs share one
// cookie, so parallel refreshes would race (and past the grace window, look like theft).
let refreshInFlight: Promise<boolean> | null = null
export function tryRefresh() {
  if (!refreshInFlight) {
    const run = navigator.locks ? navigator.locks.request('pki-auth-refresh', doRefresh) : doRefresh()
    refreshInFlight = run.finally(() => (refreshInFlight = null))   // one per tab, serialized across tabs
  }
  return refreshInFlight
}

export async function api(path, init = {}) {
  let res = await rawFetch(path, init)
  if (res.status === 401 && !path.startsWith('/auth/')) {
    if (isImpersonating()) { endImpersonation(); res = await rawFetch(path, init) }
    else if (await tryRefresh()) res = await rawFetch(path, init)
    else { accessToken = null; onSessionExpired.forEach(f => f()) }
  }
  res = await maybeStepUp(res, path, () => rawFetch(path, init)) // 403 step_up_required → modal → retry
  ...
}
```
- Boot: `main.ts` awaits `auth.restore()` (silent `/auth/refresh` → `/auth/me`) **before
  mounting**, so the router guard sees the real state.
- Router guard: a non-`public` route without a session goes to `/login?redirect=…`. `meta.role` is `admin`/`owner`.
- Post-login redirect only accepts internal paths (`startsWith('/') && !startsWith('//')`), which blocks open redirects.
- `App.vue` overlays: `ForcePasswordChange` when `user.must_change_password`,
  `ForceOtpSetup` when `mustSetupOtp`, plus a global `StepUpModal`.
- Login page order on mount: `?sso_error` → `?sso` code exchange → auto-SSO / auto-cert
  (unless `?local=1`) → arm the passkey autofill.
- `?local=1` renders every enabled method (password form, passkey, SSO), whatever
  `hide_local_login` says, and never auto-starts SSO.
- Two-factor enrolment never needs a new sign-in: `ForceOtpSetup` and Settings
  both enrol TOTP (then show the recovery codes) or register a passkey, and
  refetch `/auth/me` to continue in the same session.
- SSE: after `fetchMe`, get a ticket, open `EventSource('/api/v1/auth/events?ticket=…')`, and refetch `/auth/me` on privilege events.

---

## 13. Hardening checklist (all present here, so replicate them)

- [ ] Generic `invalid credentials` for unknown user, bad password and locked account. Constant-time dummy bcrypt.
- [ ] Per-IP token bucket on every credential and second-factor endpoint, plus per-account timed lockout.
- [ ] `TRUSTED_PROXIES` allowlist so `ClientIP()` can't be forged via `X-Forwarded-For`.
- [ ] JWT alg pinned. Secret ≥ 32 chars enforced at startup in release.
- [ ] No roles in the JWT. Load the user per request so disable/role changes apply immediately.
- [ ] `token_version` for instant global logout. Bump it on password change (keeping the caller's session), admin reset and force logout.
- [ ] Rotating refresh tokens **with reuse detection**. Mark tokens rotated (not just revoked), allow a short grace window for tab races, and revoke the whole session on a later replay. Serialize refreshes in the client.
- [ ] Pre-auth ceremony state (OIDC state, WebAuthn challenges, one-time codes, stream tickets) is **shared, hashed, single-use and swept**, never a per-process map.
- [ ] Refresh cookie `HttpOnly`, `Secure`, `SameSite=Strict`, path-scoped. CSRF double-submit on the cookie endpoints.
- [ ] Tokens never in URLs. SSO uses a one-time code, SSE a one-time ticket.
- [ ] OIDC: PKCE S256 + nonce + single-use state (10 min), `aud` check, SSRF-guarded outbound client.
- [ ] Cert login re-verifies chain + EKU + revocation, whatever the proxy says. The proxy strips spoofed headers.
- [ ] Whatever attribute maps a credential to a user (here the email SAN) is **unique** and not self-editable while that login method is on. Dry runs have no side effects.
- [ ] Secrets needing read-back are sealed (AES-GCM, Argon2id-derived KEK). Everything else is hashed.
- [ ] Security headers: CSP `default-src 'self'; script-src 'self'; manifest-src 'self' blob:; frame-ancestors 'none'; object-src 'none'`
      (`manifest-src blob:` because the SPA builds its branded web manifest as a blob — without it the app can't be installed; PROJECT-BLUEPRINT §10.1),
      `X-Frame-Options: DENY`, `nosniff`, `Referrer-Policy: no-referrer`, `COOP: same-origin`. HSTS is set at the proxy.
- [ ] Body size cap.
- [ ] Audit every auth event (`auth.login`, `auth.lockout`, `auth.oidc_login`, `auth.cert_login`,
      `auth.webauthn_*`, `auth.otp_*`, `auth.password_change`, `auth.token_rotate`, `auth.step_up`,
      `user.force_logout`, `user.impersonate`, …). The audit log is hash-chained with `/audit/verify`.
- [ ] Owner security alerts (opt-in): `impersonation`, `owner_transfer`, `admin_grant`, `config_change`, `refresh_reuse`, …
- [ ] Prometheus counter `login{method,result}`.
- [ ] Escape hatch `/login?local=1` so a broken SSO/cert config can't lock the admin out.
- [ ] Every supported database engine is exercised for real (`make db-smoke`), not only the test engine.

---

## 14. Traps (all fixed here — don't reintroduce them)

These were real bugs in this codebase, found while writing this blueprint and fixed in the
auth-hardening change (see CHANGELOG). Each is covered by a test or the DB smoke check.

1. **Per-process pre-auth state.** OIDC state, WebAuthn challenges, SSO codes and SSE
   tickets used to be in-memory maps, so a ceremony broke if the callback hit another
   replica or the process restarted. **Now:** the `ephemeral` store (shared DB table).
   *Still per-process, by design:* the per-IP rate limiter (keep a limit at the proxy; the
   per-account lockout is DB-backed and global) and SSE fan-out (a push reaches only
   streams on the replica that handled the change; the SPA re-syncs on reconnect).
   Move those to Redis or Postgres LISTEN/NOTIFY if you need strict multi-replica behaviour.
2. **No refresh-token reuse detection + multi-tab race.** **Now:** `rotated_at`, a 30 s
   grace window, and whole-session revocation on reuse. The SPA single-flights refreshes and
   serializes them across tabs with Web Locks. Logout revokes the whole session.
3. **Password change didn't end other sessions, and admin reset left live access tokens
   valid for 15 min.** **Now:** both bump `token_version` and revoke refresh tokens. Admin
   reset forces a change by default.
4. **The cert-login dry run auto-provisioned users.** **Now:** `provision=false` → `would_provision`.
5. **Non-portable SQL.** Backtick `` `key` `` in raw `Where` clauses (invalid on Postgres,
   where errors were swallowed so policies read as "off"), `type:blob` columns (Postgres
   migration failed), and a `… AS blob` alias (`blob` is reserved in MySQL, so first boot
   failed). On `main` only SQLite actually worked. **Now:** struct conditions, dialect-native
   byte types, a neutral alias, and `make db-smoke DB=postgres|mysql`. **Lesson:** unit tests
   on SQLite prove nothing about other engines, so smoke-test each supported engine.
6. **Cert-login identity by non-unique email.** A user could set their email to someone
   else's, which either blocked that person's certificate (ambiguous match) or claimed a
   future certificate identity. **Now:** unique emails (case-insensitive), case-insensitive SAN
   matching, and self-service email locked while cert login is enabled. If you port this,
   a stronger option is binding certificates to users explicitly (e.g. by issuer+serial or
   SPKI hash) instead of by email.

Remaining sharp edges to keep in mind:
- **GORM column naming.** `OIDCSubject` maps to `o_id_c_subject`. Query with struct conditions, never a hard-coded column.
- **Per-request DB cost.** `Required()` loads user+roles+grants (plus the `otp_enforced` setting and a passkey
  count for local users without OTP). Cache it if you have hot APIs.
- **OIDC** doesn't check `email_verified`. That's fine because identity is keyed on `sub`, but don't link accounts by
  email. With `role_claim` set, locally assigned roles are **overwritten** on each login. IdP-synced emails
  aren't subject to the uniqueness check; an ambiguous cert match is rejected rather than guessed.
- `WEBAUTHN_RP_ID` is effectively permanent. Changing it orphans every passkey.
- `PKI_MASTER_KEY` must never change. Sealed TOTP and OIDC secrets become unreadable.

---

## 15. Porting recipe (minimal to full)

1. **Core**: models `User`, `RefreshToken`, `Setting` · `password.go` · `jwt.go` ·
   `manager.go` · `middleware.go` (drop the owner/OTP bits) · `auth_cookies.go` ·
   `handlers_auth.go` (login/refresh/logout/me) · rate limiter + security headers ·
   `config` (JWT secret, cookie flags, trusted proxies) · SPA `lib/api.ts` + auth store
   + router guard + boot `restore()`.
2. **RBAC**: `Role`/`RoleGrant` + `rbac` package. Rename "authority/CA" to your resource type.
   Seed built-ins, including a `none` role for JIT users.
3. **Hardening**: lockout fields + `RecordFailure`/`RecordLoginSuccess`, `token_version`,
   force logout, `must_change_password` gate, bootstrap admin.
4. **MFA**: keystore `Sealer` → TOTP + recovery codes → WebAuthn (RP config, `ephemeral` challenge store, SPA ceremonies).
5. **SSO**: `ephemeral` store + `oidc.go` + `safedial` + `handlers_oidc.go` + one-time code exchange + `/branding` flags on the login page.
6. **Extras as needed**: API tokens · sessions UI · step-up · impersonation · owner concept ·
   feature flags · cert login · SSE ticket · security alerts.
7. Read §14 before you start. Every item there was a real bug here, and the fixes are the design to copy.
8. If you support more than one database, add a real-engine smoke test from day one (see `scripts/db-smoke.sh`).
