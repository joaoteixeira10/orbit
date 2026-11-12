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
   (a Google / iCloud address gives read access to that calendar), the app
   passwords of synced CalDAV calendars (sealed; never shown again), the SSO
   client secret, notification credentials.
4. **The record of who did what** with accounts, roles, workspaces and their
   memberships, in a tamper-evident audit log.

**Trust boundaries.**

| Boundary | What crosses it | How it is protected |
|---|---|---|
| Browser → Orbit | Sign-in, the app, the API | TLS (at your proxy or in the app), short access tokens in memory, `HttpOnly` refresh cookie, CSRF check, CSP, rate limits |
| Script → Orbit | API calls | Personal API tokens (`orb_…`), hashed, optionally limited or read-only |
| Member → a restricted page | A page (and its sub-pages) narrowed to some members | One check (`page_access.go`) on every page path: the page by id, the tree, trash, favorites, Home, search, backlinks, the graph, history, page files, exports and mention notices. Narrower under a restricted page, never wider; a copy keeps the restriction. If none of its people are members any more, the workspace's owners see it |
| Guest → Orbit (guest links, public forms) | A page (and its sub-pages) read or edited; a form's answers | The link's secret only (no account): 24 random bytes, kept hashed (and sealed to show again to the workspace); rate-limited, `noindex`. A guest gets exactly the pages the link opens, and the sheets, diagrams, whiteboards and files shown in them; edits are versioned (someone's newer save wins) and kept in the page's history. A public form writes only a new row of its database, its values checked against the questions (no person fields in public forms; a honeypot drops robots). Links expire, are taken back at once, and the owner can switch each kind off |
| Assistant → Orbit (MCP) | Tool calls at `/mcp` | A bearer token only (no cookies): an API token, or one made by OAuth 2.1 with PKCE (S256 required; codes single-use, 10 minutes, bound to the app and its registered redirect). Each tool is an API call with the same token, so the person's access and a read-only token's limit apply. Apps register themselves (RFC 7591) with https, loopback or private-scheme redirects only; the consent page names the app and where it returns. The resulting token is listed and revocable with the person's API tokens |
| Orbit → the assistant's model | A person's question, what's on their screen, and what Orbit's tools found for it | The owner chooses the model's service (Claude, OpenAI-style, Ollama) and its key, sealed with the master key and never sent back. The tools are the MCP server's, each an API call as the asking person, so it sees and changes exactly what they could; Ask mode offers only those that read. In Agent mode a change the model calls for is only lined up — none is made until the person approves it (each ticked, approved once), then made as them, marked `via` in page history and audited (`assistant.apply`); it can't be used, or approved, while impersonating. Called through the SSRF-guarded client (an owner's Ollama may be on your network, never loopback); an optional daily limit per person. What the tools read goes to that service — an Ollama of your own keeps it in your network. Conversations are kept per person, deleted by them (and with their account). When the owner allows it, a person may use their own model (their key, sealed the same way): only a public https address through the public-only guard (no private, loopback or link-local address), unless the operator lists the server in `AI_SERVERS`. A workspace's owners can keep the assistant out of it: every workspace check refuses its calls there, and it isn't listed or searched. Files attached to a conversation are uploads of the person's alone (no workspace; not in their Files or search), within the upload size and quota limits, deleted with the conversation (unsent ones after a day); putting one in a workspace's Files is a change like any other, approved first and made as the person, as an editor |
| Calendar app → Orbit | `GET /ical/<token>.ics` | A long random token per feed, stored hashed, revocable; its own rate limit |
| Orbit → the internet | Subscribed calendars, SSO discovery, notifications, update check, git servers, a shared Google Sheets spreadsheet | An SSRF-guarded HTTP client, size and time limits (git servers named in `GIT_SERVERS` may be on your network). Google Sheets: only the export address made from the link's id, through the public-only client, redirects followed on Google's hosts alone, 20 MB |
| Orbit → Google (Calendar sync) | The OAuth sign-in (code + PKCE), access tokens, CalDAV | The client secret and each link's refresh token are sealed with `ORBIT_MASTER_KEY`; the sign-in's state is single-use and tied to the browser by a cookie; only the owner sets the client up; access tokens live in memory only |
| Browser → the draw.io editor | A diagram being edited (Diagrams) | The editor runs in a sandboxed frame from the one address the owner sets (CSP `frame-src`); Orbit talks to it by `postMessage`, checking the sender is that frame at that origin. Its pictures (SVG) are shown as `<img>`, where scripts don't run |
| Mermaid text → a drawing (Diagrams, pages) | A Mermaid diagram's text, written by any editor of the workspace | Drawn in the browser by mermaid.js with `securityLevel: 'strict'` (its SVG sanitised by DOMPurify; no click handlers, no HTML labels); the stored picture is shown as `<img>`, where scripts don't run. The server only stores the text and the SVG, capped |
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
  in a row — at sign-in, in "confirm it's you" or when changing the password —
  the account is locked for `LOGIN_LOCKOUT_MINUTES` (15). The lock is
  stored in the database and audited (`auth.lockout`). A locked account gets the
  same generic "invalid credentials" answer, so the lock isn't disclosed. An
  admin can unlock early.
- **Per IP:** a token bucket (`AUTH_RATE_BURST` 8, `AUTH_RATE_PER_MINUTE` 12)
  guards sign-in, the SSO code exchange, passkey sign-in, the two-factor
  routes, "confirm it's you" and the password change, and answers `429`. The client IP is the socket peer unless the request
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
  **read-only** (every request other than `GET` is refused, admin routes
  included). The limit only ever narrows: a token never has more than its
  user has today.
- Tokens don't carry the token version, so an admin's **sign out everywhere**
  and an **admin password reset** delete the user's API tokens too (changing
  your own password doesn't).
- It **never carries owner status**, so owner-only routes refuse it, even for
  the owner's own tokens.

Tokens skip the interactive "change your password" and "set up two-factor"
gates. A disabled user's tokens stop working.

**Step-up** (feature flag `step_up_auth`, off by default). When on, the most
dangerous actions need a fresh password or passkey check within the last 5
minutes: feature flags, security policy, SSO settings, owner transfer and
starting an impersonation.

**Sign-in providers.** Several OpenID Connect providers (Google is one) can
be set by the owner (step-up), their client secrets sealed. Each login's
state names its provider; an account's subject is the provider's
(`<id>|<sub>`, the first provider's as before), so two providers can't map
to one account. A provider limited to email domains admits only a verified
email of them (Google: its `hd` claim must agree). A provider's groups
claim keeps that provider's groups only (others' and Orbit's own are never
touched); a sync that would leave a workspace without an owner is skipped.

**SAML 2.0 sign-in** (a provider of kind `saml`; gosaml2). Orbit is the
service provider, its key made once and kept sealed. A response is accepted
only signed by a certificate in the identity provider's metadata, from its
entity ID, for Orbit's audience and reply URL, within its time window, and
answering a request Orbit made in that browser's sign-in (its RelayState,
single use, and the assertion's InResponseTo) — so no unsolicited
(IdP-initiated) or replayed sign-ins. Metadata fetched by URL goes through
the guarded client. Accounts are per provider (`<id>|<NameID>`).

**SCIM provisioning** (`/scim/v2`, off by default). The identity provider
authenticates with a bearer token the owner makes (step-up), kept only as a
SHA-256 hash, compared in constant time. It can create accounts (never with
an admin role by default), change and deactivate only the accounts it
created, and change only its own groups (source `scim`); a local account's
userName is refused, not taken over. Deactivating ends the account's
sessions. Every SCIM change is audited (`scim.*`). An account SCIM made is
linked at its first SSO sign-in by email or username — only such accounts,
never one made otherwise.

**Groups.** A group carries instance roles (added to its members' own when
privileges are worked out, on every request) and workspace roles (kept as
the members' rows: the higher of their own and their groups'). Managing
groups needs `users.manage`; a group with a superuser role or an owner-gated
privilege is changed — roles, people, deletion — only by the owner, and
someone who is an admin through a group counts as one (another admin can't
manage their account). No group change may leave a workspace without an
owner (refused, nothing changed).

**Locked notes and pages.** A person can lock a note or a page with a PIN.
The text is encrypted in their browser (AES-GCM, 256-bit) with their lock
key — random, kept on the server only wrapped by a key derived from the PIN
(PBKDF2-SHA-256, 600 000 iterations, a random salt). The server never sees
the PIN or the key; locking clears the readable text, its search words, a
page's history and its live-editing state, and the API refuses readable text
for a locked item, or any change to it from anyone but who locked it. Not
covered: titles, attachments, a note's drawing. A lost PIN can't be
recovered; setting or forgetting it is audited and refused while
impersonating (an impersonating admin can't open what someone locked, as
they don't know the PIN). Web Crypto needs a secure context: HTTPS, or
localhost.

**Impersonation** (feature flag `impersonation`, off by default). A holder of
`users.impersonate` can act as another user to see what they see. The owner
can never be impersonated; only the owner can impersonate an admin; nobody can
impersonate themselves or a disabled account. The session is one 15-minute
access token with no refresh, and nothing that would outlast it can be made
meanwhile: new sessions (`rotate-token`), API tokens, password, two-factor and
passkey changes, git accounts, push devices and calendar feeds are refused.
Everything done meanwhile is audited under the target **and** the real admin
(`impersonator_id`) — every change, not only the kinds audited anyway (a task
saved, a page edited: `task.update`, `page.update`…; the same change to the
same thing again within three minutes is one entry). The audit log marks them
with a "via <admin>" pill.

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
- **Notification channels don't carry workspace content by default.** The
  shared channels (Slack, Teams, Discord, Telegram, ntfy, Gotify) are one
  place the owner set up, which others may read; a person's notifications
  reach them only as a ping ("Orbit: a reminder for you" and a link — the
  kind, never titles, names or workspaces) unless that person turns on
  **Include titles in shared channels**. Email goes to the person's own
  address and has the full text.
- **Grades are personal.** Courses and their timetable belong to the
  workspace, but a grade or a goal is its owner's alone: every query filters
  by the caller, so other members — owners included — get `404` for someone
  else's. Viewers may keep their own grades (not through a read-only token,
  nor in an archived workspace). Live updates about grades go only to their
  owner's sessions. Deleting an account deletes its grades and goals.
- **Invite links** (flag `workspace_invites`, on by default). Only owners
  make them, with role editor or viewer (never owner), up to 30 days and 100
  uses. The token is 24 random bytes, shown once and stored as a SHA-256
  (plus a six-character prefix to tell links apart); it never reaches the
  request log or the audit log. A link dies when it expires, is used up or
  revoked, its workspace is archived or deleted, or its maker stops being an
  owner (or is deleted). Uses are counted in the same update that checks
  them, so a one-use link can't be used twice at once. The public lookup and
  sign-up are rate-limited like sign-in; sign-up makes a **member**-role
  account (no instance privileges), takes the usual password rules, and is
  refused when the sign-in page hides the password form for SSO. Joining
  isn't possible while impersonating. Audited: `workspace.invite_create`,
  `workspace.invite_revoke`, `workspace.invite_accept`, and `user.create`
  with `via: invite`.
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
| Git access tokens | Sealed, AES-256-GCM | Never returned to the page; each user's own, used only for what they do (and the sync of lists they linked) |
| Notification channel secrets (SMTP password, webhook URLs, bot tokens, the webhook signing secret) | The whole configuration sealed in the `settings` table | Masked when read back (write-only on the page) |
| Short-lived login state (SSO state, passkey challenges, SSO codes, live-update tickets) | In the database, keyed by a SHA-256 of a random id | Single-use, expire in minutes, swept every 15 minutes |
| Workspace content | Plain, in the database | Protected by access control and your host; encrypt the disk / volume if the host is shared |
| Uploads (files on pages, tasks, notes) | Plain, in the database (their own table) | Only those who can see what a file is on can fetch it (bearer token, so never by a bare link); the type is sniffed, not taken from the browser; only PNG / JPEG / GIF / WebP are served inline, everything else (SVG and HTML included) as a download, with `nosniff` and a `sandbox` CSP; names are cleaned; size and quota limits |

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
  calendar **subscriptions** created; **CalDAV syncs** set up and stopped;
  calendar **feeds** created and revoked; workspaces **exported** and
  **imported**.
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
  `Cross-Origin-Opener-Policy: same-origin`; `Strict-Transport-Security` when
  Orbit serves TLS itself (behind a proxy, set it there).
- **API reference** (`/api/docs`): Scalar's script is bundled and served from
  Orbit's own origin, so the CSP stays `script-src 'self'`; its hosted
  extras are turned off.
- **Limits:** request bodies up to `MAX_BODY_BYTES` (4 MiB), a page up to
  2 MiB of Markdown (valid UTF-8),
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
- **Web Push, git servers and personal notification channels** — addresses
  any user can name — use the
  **public-only** variant, which also refuses private networks (RFC 1918,
  carrier-grade NAT, IPv6 unique-local), so they can't be used to probe the
  LAN. A git server on your network is allowed only when the operator lists it
  in `GIT_SERVERS` (then dialed directly, no redirects). Git tokens are sealed;
  a list linked to a repository stops syncing when whoever linked it leaves the
  workspace (their token isn't used any more, unless the list has a bot
  account). **Nobody's change goes to git as someone else:** a change is sent
  with its author's own account, or — when they have none — the list's bot
  account (sealed, set by whoever linked the list or an owner, audited
  `git.bot_set`); without one, the API refuses changes that would reach git
  (`403`, `git_read_only`). Reading (the sync) uses whoever linked the list,
  or the bot once they've left; failed changes are retried as their author.
- **"Sign in with GitHub, GitLab, Gitea or Forgejo"** (OAuth2 authorization code with PKCE):
  the owner's client secrets are sealed; the flow's state is single-use, kept
  server-side (10 minutes) and tied to the browser that started it by an
  HttpOnly `SameSite=Lax` cookie, so a sign-in link someone else started
  can't attach your server account to their Orbit account. The access and
  refresh tokens are sealed; the access token is renewed shortly before it
  expires. Personal channels must be
  `https://`, take a test message before they're kept, are stored sealed and
  are never shown again (only their host); adding and removing one is
  audited, and the owner's `personal_channels` flag turns them off — or
  limits them to some kinds (a channel of a kind no longer allowed is kept
  but nothing is sent to it).
- **Subscribed calendars** are fetched through the same guarded dialer, with
  three differences: up to **three redirects** (calendar hosts use them), each
  hop dialed through the guard and limited to `http`/`https`; at most
  **8 MiB**; a **30 s** timeout. A failure is shown to the editors without the
  address. The scheduler refreshes each subscription at its interval (at least
  every 15 minutes, by default hourly).
- **Bookmarks** read the page a member pastes (and its picture) through the
  public-only guarded client — loopback, link-local and private networks
  refused, each redirect hop (five at most) checked again; the page is read
  up to 1 MiB, the picture up to 600 KiB and only as PNG, JPEG, GIF or
  WebP (sniffed). The picture is served from Orbit to members, with
  `Content-Security-Policy: sandbox`, so no viewer's browser calls the site.
- **Drawings** (Excalidraw) are JSON stored as sent (capped at 16 MB); their
  preview must be an SVG and is only ever shown as an image (`<img>`, where an
  SVG runs nothing). Excalidraw and React load as their own chunk from this
  origin, its fonts too (no CDN); the CSP allows `blob:` images for the
  pictures drawn into a drawing. (A known advisory in KaTeX, which Excalidraw
  pulls in through its Mermaid import, only concerns math in a Mermaid
  diagram someone pastes into their own drawing.)
- **Formulas** (database formula properties, sheets) are parsed into a tree
  and walked by Orbit's own small evaluator (`web/src/lib/formula.ts`): no
  `eval`, no `Function`, no access to anything but the cells and properties
  named. A sheet is JSON stored as sent (an object, capped at 8 MB); its
  values are worked out in each viewer's browser. Excel files are read and
  written in the browser (XML parsed with `DOMParser`, never run; at most
  500 000 cells read), so nothing of an imported file reaches the server but
  the cells kept from it.
- **Git webhooks** (instant updates) post to a public endpoint per linked
  list; a post counts only if signed (HMAC-SHA256 of the body) with the
  list's secret — or, from GitLab, if it carries that secret as its
  `X-Gitlab-Token` (compared in constant time) — — random, sealed, shown once when added by hand — and it
  only triggers a sync (its content isn't trusted or read). Per-IP rate
  limited.
- **Synced CalDAV calendars** (owners set them up; the `caldav_sync` flag) are
  reached through the guarded dialer too (LAN servers allowed — a home
  Nextcloud is the point), redirects followed by hand (three at most, the
  method kept, `http`/`https` only); answers are capped (32 MiB a listing,
  1 MiB an object). Writes are conditional (`If-Match` / `If-None-Match`), so
  a change made on the server meanwhile is never overwritten blindly. Errors
  shown never carry the URL.
- **Images from other sites** in pages are shown only after a click (or always,
  if the reader chooses so): the CSP allows `https:` images so that pages can
  show them, and a remote image would otherwise tell its site who reads a page
  and when. A clicked image is fetched without a referrer.
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
- [ ] SCIM provisioning off unless an identity provider uses it; a new token
      if the old one may have leaked.
- [ ] A sign-in provider for your organisation only: set its **Allowed email
      domains** (Google especially).
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
| A10 SSRF | Calendar subscriptions, SSO, notifications and the update check go through the guarded dialer (loopback, link-local and metadata refused, the dialed IP checked, redirects re-checked hop by hop for calendars only); Web Push and git servers through the public-only one (private networks refused too, except git servers in `GIT_SERVERS`). Calendar feeds are expanded within a step budget, so a hostile ICS file can't stall the refresh. |

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
