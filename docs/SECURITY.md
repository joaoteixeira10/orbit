# Security

This page describes how Orbit protects the planners people keep in it: who
can sign in, who can see and change which workspace, how secrets are stored,
what leaves the server, what the audit log proves, and what is still your job
as operator. Read it before you put Orbit in production, before you invite
people, and when a security review asks how it works. The sign-in design is
ported from PKI Management via LB Admin and written up in
[AUTH-BLUEPRINT.md](AUTH-BLUEPRINT.md); this page is the current version for
Orbit. Every setting named here is in [CONFIGURATION.md](CONFIGURATION.md).

## Threat model

**What Orbit protects.**

1. **Workspace content** — pages, tasks, events: diaries, school work,
   household plans. It must be visible only to the workspace's members.
2. **Accounts** — passwords, second factors, sessions, API tokens.
3. **Secrets entrusted to it** — the secret addresses of subscribed calendars
   (a Google / iCloud address gives read access to that calendar), the SSO
   client secret, notification credentials.
4. **The record of who did what** with accounts, roles, workspaces and their
   memberships, in a tamper-evident audit log.

**Trust boundaries.**

| Boundary | What crosses it | How it is protected |
|---|---|---|
| Browser → Orbit | Sign-in, the app, the API | TLS (at your proxy or in the app), short access tokens in memory, `HttpOnly` refresh cookie, CSRF check, CSP, rate limits |
| Script → Orbit | API calls | Personal API tokens (`orb_…`), hashed, optionally limited or read-only |
| Calendar app → Orbit | `GET /ical/<token>.ics` | A long random token per feed, stored hashed, revocable; its own rate limit |
| Orbit → the internet | Subscribed calendars, SSO discovery, notifications, update check | An SSRF-guarded HTTP client, size and time limits |
| Anyone with the host | Database file, environment | `ORBIT_MASTER_KEY` seals secrets at rest; the rest is your host's security |

**Who is trusted.** The **owner** and holders of `users.manage` and
`roles.manage` are trusted administrators **of accounts** — not readers of
content (see [Workspace privacy](#workspace-privacy)). The identity provider
behind SSO is trusted to say who a user is and which instance roles they have.
The members of a workspace trust each other with its content, in the measure
of their role.

## Authentication

**Passwords.** bcrypt with cost 12. An unknown user name still runs a bcrypt
compare, so the response time doesn't reveal which accounts exist. New
passwords need at least 8 characters. A password set by an admin, and the first
admin's password, are **temporary**: the user can do nothing but change it at
the next sign-in.

**Lockout and throttling.**

- **Per account:** after `LOGIN_LOCKOUT_THRESHOLD` (5) wrong passwords or codes
  in a row, the account is locked for `LOGIN_LOCKOUT_MINUTES` (15). The lock is
  stored in the database and audited (`auth.lockout`). A locked account gets the
  same generic "invalid credentials" answer, so the lock isn't disclosed. An
  admin can unlock early.
- **Per IP:** a token bucket (`AUTH_RATE_BURST` 8, `AUTH_RATE_PER_MINUTE` 12)
  guards sign-in, the SSO code exchange, passkey sign-in and the two-factor
  routes, and answers `429`. The client IP is the socket peer unless the request
  comes through a proxy listed in `TRUSTED_PROXIES`, so a forged
  `X-Forwarded-For` can't dodge the limit or fake the audit IP.

**Sessions.**

- **Access token:** a JWT (HS256, signed with `JWT_SECRET`), valid 15 minutes.
  The page keeps it **in memory only**, never in `localStorage`, and sends it
  as a bearer header, so ordinary API calls can't be forged cross-site. Tokens
  signed with any other algorithm are rejected.
- **Refresh token:** 32 random bytes, valid 30 days, stored only as a SHA-256
  hash. It lives in the `orbit_refresh` cookie: `HttpOnly`, `SameSite=Strict`,
  `Secure` in release, and sent only to `/api/v1/auth`. Every refresh **rotates**
  it.
- **Reuse detection:** a rotated refresh token that comes back within 30
  seconds (two tabs refreshing at once) is tolerated. Later, it means someone
  else holds a copy: the **whole session is revoked**, the event is audited as
  `auth.refresh_reuse`, and the owner can be alerted.
- **CSRF:** the two cookie endpoints (`/auth/refresh`, `/auth/logout`) need the
  `X-CSRF-Token` header to match the readable `orbit_csrf` cookie (double submit).
- **`token_version`:** each access token carries the user's token version. A
  password change, an admin password reset, an admin's force sign-out or a
  token rotation bumps it, which rejects every live access token at once and
  revokes the refresh tokens (a password change keeps your current session). A disabled or deleted account stops working on its next
  request.

**Two-factor.** Either kind satisfies the "two-factor required" policy.

- **TOTP** (authenticator app): 6 digits, 30-second steps, ±1 step. The secret
  is **sealed** (AES-256-GCM) in the database. You get **10 recovery codes**,
  each single-use and stored as a SHA-256 hash. Wrong codes count toward the
  lockout.
- **Passkeys** (WebAuthn): as a second factor or for passwordless sign-in
  (owner can turn passwordless off). Challenges are single-use, bound to the
  user who started the ceremony, and expire after 3 minutes. A passkey counts
  as two-factor. Passkeys need `WEBAUTHN_RP_ID`.
- Two-factor can be required for **everyone** (`otp_enforced`, owner) or **per
  user** (Admin → Users). Until they enrol, such users can only enrol.

**SSO (OpenID Connect).** Authorization code flow with **PKCE**, a random
**state** (CSRF guard) and **nonce** (replay guard); the ID token's signature and
nonce are checked. The login state lives 10 minutes in the database and is
used once. After the callback the page gets a **one-time code valid 60
seconds**, never a token in the URL. Users are created on first sign-in, keyed
by the `sub` claim. Values of the role claim are matched to role names
(re-synced on every sign-in when a role claim is set); without a match the user gets the default role,
`none` unless the owner picks another. The client secret is sealed and never
shown again. SSO users have no local password or second factor: **MFA is the
identity provider's job**. `/login?local=1` always shows the password form, so
a broken SSO setup can't lock the owner out. Setup: [SSO-entra.md](SSO-entra.md),
[SSO-authentik.md](SSO-authentik.md).

**API tokens.** Personal tokens start with `orb_`, carry 32 random bytes, and
are stored only as a SHA-256 hash; the plain token is shown once. They can
expire (expired ones are deleted daily) and can be revoked by their owner or by
anyone with `users.manage` (Admin → API tokens). A token acts as its user, with
two limits:

- It can be **limited** to some instance privileges and/or made
  **read-only** (it can read the user's workspaces but not change anything in
  them, nor create workspaces). The limit only ever narrows: a token never has
  more than its user has today.
- It **never carries owner status**, so owner-only routes refuse it, even for
  the owner's own tokens.

Tokens skip the interactive "change your password" and "set up two-factor"
gates. A disabled user's tokens stop working.

**Step-up** (feature flag `step_up_auth`, off by default). When on, the most
dangerous actions need a fresh password or passkey check within the last 5
minutes: feature flags, security policy, SSO settings, owner transfer and
starting an impersonation.

**Impersonation** (feature flag `impersonation`, off by default). A holder of
`users.impersonate` can act as another user to see what they see. The owner
can never be impersonated; only the owner can impersonate an admin; nobody can
impersonate themselves or a disabled account. The session is one 15-minute
access token with no refresh. Everything done meanwhile is audited under the
target **and** the real admin (`impersonator_id`).

## Authorization

There are two independent layers.

**Instance privileges** (roles) run the instance:

| Privilege | Allows |
|---|---|
| `workspaces.create` | creating workspaces (the creator becomes its owner) |
| `users.manage` | creating, disabling, resetting and signing out accounts; seeing every API token |
| `roles.manage` | creating and editing roles |
| `audit.read` | reading and exporting the audit log |
| Owner-gated: `settings.branding`, `settings.notifications`, `users.impersonate` | branding, notification channels, impersonation — held by the owner, grantable only by the owner |

**Built-in roles** (they can't be edited): `admin` (superuser: every
privilege except the owner-gated ones), `member` (`workspaces.create`; the
default for accounts an admin creates without choosing roles), `none`
(nothing; the default for new SSO users — they can still be invited to
workspaces).

**Workspace roles** (on the membership, set by the workspace's owners):

| Role | Can |
|---|---|
| `owner` | everything below, plus settings, modules, members and their roles, archive, delete |
| `editor` | create, change and delete pages, tasks, lists, events and subscriptions |
| `viewer` | read |

### Workspace privacy

- Every planner request loads the resource, then checks the caller's
  **membership** of its workspace. **Not a member → `404`**, whatever the
  instance role: admins and the owner can't open other people's workspaces,
  and an id reveals nothing about whether it exists.
- Role checks follow: a viewer writing gets `403`; a write through a
  **read-only API token** gets `403`; a write in an **archived** workspace gets
  `409`.
- Search, Today, the merged calendar and favourites only ever query the
  caller's own workspaces.
- Live updates about a workspace go only to its members' streams.
- **The last owner** can't leave or be demoted. **Deleting an account**
  deletes the workspaces it was alone in and hands the others to a remaining
  member (an editor first) — content is never orphaned or exposed.
- **Assignees** must be members of the workspace.
- **Impersonation** (flag `impersonation`, off by default) does show the
  impersonated user's workspaces — that is what it is for. Leave it off unless
  you need it; every impersonation is audited and can raise a security alert.

**The owner.** Exactly one user is the **instance owner**: first the bootstrap
admin, later whoever the owner transfers it to. Ownership is an identity, not a
privilege, so no role can grant it. Owner-only: the security policy, SSO
settings and feature flags; security alerts; backups; owner transfer; creating
an admin, and granting or removing the `admin` role; granting the owner-gated
privileges through a custom role.

**Escalation guards.**

- Built-in roles can't be edited.
- A non-owner can't create an admin, grant or revoke the `admin` role, or add or
  remove owner-gated privileges on a role.
- The last active admin can't be disabled, demoted or deleted. You can't
  disable or delete yourself.
- API tokens and impersonation can't exceed or reach the owner.
- **Nobody acts on the owner's account** (edit, roles, disable, reset password /
  two-factor / passkeys, unlock, sign out, delete, revoke its tokens) — the owner
  manages it in Settings. **Only the owner acts on an admin's account.**
- The identity provider can't grant the admin role (by claim or default role)
  unless the owner turned on **Let the identity provider grant admin** in SSO.
- An API token can't create API tokens or calendar feeds.

**Sessions and sign-in records.** Signing out, ending a session (Active
sessions) or a detected refresh-token reuse stops that session's access token
at once (each request checks its session). Every failed sign-in is audited as
`auth.login_failed` with the name tried (never the password), and lockouts as
`auth.lockout`.

**Backups** are owner-only (step-up when on): the archive holds every
workspace, every user's password hash, the sealed secrets and the audit log.
It is sealed with AES-256-GCM — with a key derived (Argon2id) from
`ORBIT_MASTER_KEY`, or from a passphrase of 12+ characters — and the secrets
inside stay sealed with `ORBIT_MASTER_KEY`. Restore is a command run with Orbit
stopped, never an upload. Keep the archives and `ORBIT_MASTER_KEY` apart.

## Crypto and secrets at rest

| Secret | At rest | Notes |
|---|---|---|
| Passwords | bcrypt, cost 12 | |
| Refresh tokens, API tokens, recovery codes, calendar-feed tokens | SHA-256 hash | Plain value shown once or kept only in the cookie |
| TOTP secrets | Sealed, AES-256-GCM | |
| SSO client secret | Sealed, AES-256-GCM | Never returned to the page |
| Subscribed calendar addresses | Sealed, AES-256-GCM | Only the host is ever shown; errors never echo the address |
| Notification channel secrets (SMTP password, webhook URLs, bot tokens, the webhook signing secret) | The whole configuration sealed in the `settings` table | Masked when read back (write-only on the page) |
| Short-lived login state (SSO state, passkey challenges, SSO codes, live-update tickets) | In the database, keyed by a SHA-256 of a random id | Single-use, expire in minutes, swept every 15 minutes |
| Workspace content | Plain, in the database | Protected by access control and your host; encrypt the disk / volume if the host is shared |

**Sealing.** `ORBIT_MASTER_KEY` goes through **Argon2id** (1 pass, 64 MiB, 4
lanes) with a random 16-byte salt kept in the `settings` table (`kek_salt`,
created on the first start). The result is the AES-256 key. Each sealed value
has its own random 12-byte nonce, and GCM detects tampering. The key never
leaves memory.

> **`ORBIT_MASTER_KEY` must never change, and must be backed up.** With another
> key the sealed secrets can't be read. With a copy of the database **and** the
> key, an attacker can read every sealed secret, so keep them apart.

`JWT_SECRET` signs the access tokens. Anyone who has it can mint tokens for any
user. Release mode refuses either secret under 32 characters.

## Audit

- **Recorded:** sign-ins and failures, lockouts, refresh-token reuse; user,
  role, token and settings changes; owner actions and backups; workspace
  **creation, archiving, deletion**, **members** added, removed and re-roled;
  calendar **subscriptions** created; calendar **feeds** created and revoked.
  Each entry has the user, the real admin when impersonating, the client IP
  and the time.
- **Not recorded:** edits to content (pages, tasks, events). There are too
  many and they'd drown the security record; the pages keep who edited them
  last (`updated_by`).
- **Hash chain:** each entry stores `sha256(previous hash ‖ entry)`. Writes are
  serialised so the chain is well-defined. **`GET /api/v1/audit/verify`** (Audit
  log → Verify) walks the whole chain and reports the first entry that was
  changed, inserted or removed.
- **Syslog:** with `SYSLOG_ADDR`, every entry also goes out as JSON, with its
  chain hash, to syslog or your SIEM.

> The chain makes after-the-fact edits **detectable**, not impossible. The hash
> has no secret key, so someone who can write the database can rewrite the
> whole chain consistently. The syslog copy is your independent record.

## Transport

- **TLS:** terminate it at a reverse proxy (see [reverse-proxies.md](reverse-proxies.md))
  or in the app with `TLS_CERT_FILE` / `TLS_KEY_FILE` (TLS 1.2 or newer). Without
  TLS, passwords and tokens cross the network in clear. Set **HSTS** at your
  proxy; Orbit doesn't.
- **`TRUSTED_PROXIES`:** list your proxy's address, or the rate limit and
  audit IP see only the proxy.
- **Cookies:** `SameSite=Strict`, refresh cookie `HttpOnly` and path-scoped,
  `Secure` in release or with TLS (`COOKIE_SECURE`).
- **Headers on every response:** `Content-Security-Policy` with
  `default-src 'self'; script-src 'self'` (no inline scripts, no CDN),
  `manifest-src 'self' blob:` (the page builds its branded install manifest
  in the browser), `frame-ancestors 'none'`, `object-src 'none'`; `X-Frame-Options: DENY`,
  `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`,
  `Cross-Origin-Opener-Policy: same-origin`.
- **API reference** (`/api/docs`): Scalar's script is bundled and served from
  Orbit's own origin, so the CSP stays `script-src 'self'`; its hosted
  extras are turned off.
- **Limits:** request bodies up to `MAX_BODY_BYTES` (4 MiB), a page document
  up to 2 MiB (and it must be an editor document — `{"type":"doc",…}`),
  calendar ranges up to 120 days; header read timeout 10 s, read timeout 30 s,
  idle timeout 120 s.
- **Calendar feeds** (`/ical/<token>.ics`) are public by design — the token is
  the credential, as with Google's "secret address". 32 random bytes, stored
  as a SHA-256, shown once, revocable in Settings; a separate per-IP rate
  limit; the feed only includes workspaces its owner **still** belongs to, and
  stops when the owner is disabled.
- **Outbound requests** for SSO, notifications and the update check use an
  SSRF-guarded client: it won't connect to loopback, link-local (including the
  cloud metadata address) or unspecified addresses, checks the IP it actually
  dials, and never follows redirects. Private LAN addresses stay reachable.
- **Subscribed calendars** are fetched through the same guarded dialer, with
  three differences: up to **three redirects** (calendar hosts use them), each
  hop dialed through the guard and limited to `http`/`https`; at most
  **8 MiB**; a **30 s** timeout. A failure is shown to the editors without the
  address. The scheduler refreshes each subscription at its interval (at least
  every 15 minutes, by default hourly).
- **CORS** is off unless `CORS_ORIGINS` is set (dev only).

## Operator checklist

- [ ] `GIN_MODE=release` (the compose files set it).
- [ ] `JWT_SECRET` and `ORBIT_MASTER_KEY`: 32+ random characters each
      (`openssl rand -hex 32`), kept out of git.
- [ ] **Back up `ORBIT_MASTER_KEY`**, and back up the database (`/data/orbit.db`)
      separately (Owner → Owner → Backups). Test a restore.
- [ ] HTTPS in front of Orbit, with HSTS at the proxy; `PUBLIC_URL` set.
- [ ] `TRUSTED_PROXIES` set to your proxy's address.
- [ ] `WEBAUTHN_RP_ID` / `WEBAUTHN_RP_ORIGINS` set to the public host name
      if you want passkeys.
- [ ] `/metrics` (if enabled) blocked at the proxy.
- [ ] `SYSLOG_ADDR` pointing at a log store Orbit's host can't edit.
- [ ] First sign-in done, password changed, two-factor set up for the owner.
- [ ] Consider `otp_enforced`, and step-up (`step_up_auth`).
- [ ] Owner security alerts on, with a notification channel.
- [ ] Leave `impersonation` off unless you need it.
- [ ] Give `users.manage` and `admin` only to people you would trust as owner.
- [ ] Run Audit log → Verify now and then.

## OWASP Top 10

| Area | Status |
|---|---|
| A01 Broken access control | Every planner request checks membership (404 to non-members, admins included) and the workspace role; read-only tokens and archived workspaces refuse writes; owner-only controls outside RBAC; escalation guards. Covered by full-router tests (`api/internal/api/planner_test.go`). |
| A02 Cryptographic failures | AES-256-GCM sealing under an Argon2id key, bcrypt 12, HS256 with algorithm check, CSPRNG tokens stored as hashes. |
| A03 Injection | GORM with bound parameters (search escapes `LIKE` wildcards); request bodies validated (`oneof`, colours, dates, RRULEs, editor documents); the app is served from an embedded file system. |
| A04 Insecure design | Membership-based privacy by construction; optimistic page versions (no silent overwrites); typed-name confirmation to delete a workspace. |
| A05 Misconfiguration | Release mode refuses weak secrets; secure cookies by default; CSP and hardening headers; CORS off; generic `500` messages; distroless, non-root image. |
| A06 Vulnerable components | CI runs `govulncheck`, `npm audit` and gitleaks on every PR. |
| A07 Authentication failures | Memory access token + rotating `HttpOnly` refresh cookie with reuse detection; CSRF double submit; lockout + per-IP limit; constant-cost unknown-user check; TOTP and passkeys; OIDC with PKCE, state and nonce. |
| A08 Integrity failures | Hash-chained audit log with a verify endpoint and an external syslog copy. |
| A09 Logging and monitoring | Security-relevant actions audited; secrets never logged (`json:"-"` on sensitive fields); optional syslog, Prometheus and owner security alerts. |
| A10 SSRF | Calendar subscriptions, SSO, notifications and the update check go through the guarded dialer (loopback, link-local and metadata refused, the dialed IP checked, redirects re-checked hop by hop for calendars only). |

## Accepted risks and known gaps

- **Private LAN addresses are reachable** from calendar subscriptions (a
  self-hosted Nextcloud is a legitimate source), so an editor can make Orbit
  fetch an internal URL. They only learn whether it answered with a calendar
  (the error says "couldn't reach" / "answered 404" / "not an iCalendar"),
  never the content of a non-calendar answer.
- **A feed address is a bearer secret.** Anyone who has it sees those events
  until it is revoked. Calendar apps cache what they fetched.
- **Content is not end-to-end encrypted.** The server (and whoever runs it)
  can read every workspace. Orbit's privacy is against other users and
  admins-through-the-app, not against the host.
- **The audit chain has no secret key** (see Audit). Rely on the syslog copy.
- **Rate limits and live updates are per process.** Orbit runs as a single
  instance; add a limit at your proxy too.
- **The lockout is temporary** by design (limits lockout as a denial of
  service).
- **SSO trusts the identity provider** for the roles it names (except admin,
  which needs the owner's opt-in).
- **Public endpoints** (no sign-in): `/api/v1/healthz`, `/readyz`, `/version`,
  `/branding`, the sign-in routes, `/ical/<token>.ics`, the API reference
  (`/api/docs`, `/api/v1/openapi.json` — it lists endpoints, not data), and
  `/metrics` when enabled.

## Reporting a problem

Please don't open a public issue with exploit details. Contact the maintainer
privately through the repository, or open a private issue. Include the version
(About, or `GET /api/v1/version`), what an attacker could do, and the steps to
reproduce. For general trouble, see [TROUBLESHOOTING.md](TROUBLESHOOTING.md).
