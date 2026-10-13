# Security

This page describes how Orbit v1 protects itself and the load balancers
behind it: who can sign in, what each person may change, how secrets are
stored, what the audit log proves, and what is still your job as operator.
Read it before you put Orbit in production, before you give someone a
role, and when a security review asks how it works. The sign-in design is
ported from PKI Management and written up in [AUTH-BLUEPRINT.md](AUTH-BLUEPRINT.md);
this page is the short, current version for Orbit (no client-certificate
login, no backups, no certificate authorities). Every setting named here is in
[CONFIGURATION.md](CONFIGURATION.md).

## Threat model

**What Orbit protects.**

1. **The ability to take servers out of rotation.** Whoever can disable or
   drain servers can take a service down. This is the main asset: every rule
   below exists so that only the right people can change only the right
   servers.
2. **The agent tokens.** Each lb-agent accepts any change that carries its
   token. A leaked token bypasses Orbit entirely, so the tokens are sealed in
   the database and never sent to the browser.
3. **The record of who did what.** Every change, and every refused change, is
   in a tamper-evident audit log.

**Trust boundaries.**

| Boundary | What crosses it | How it is protected |
|---|---|---|
| Browser → Orbit | Sign-in, page, API | TLS (at your proxy or in the app), short access tokens in memory, `HttpOnly` refresh cookie, CSRF check, CSP, rate limits |
| Script / pipeline → Orbit | API calls | Personal API tokens (`orb_…`), hashed, optionally limited |
| Orbit → lb-agent | Server state and changes | One bearer token per proxy, over plain HTTP, HTTPS or mutual TLS — chosen per agent (see [The agents](#the-agents)) |
| Orbit → the internet | SSO discovery, notifications, update check | An SSRF-guarded HTTP client |
| Anyone with the host | Database file, environment | `ORBIT_MASTER_KEY` seals secrets at rest; the rest is your host's security |

**Who is trusted.** The **owner** and holders of `users.manage`,
`roles.manage` and `proxies.manage` are trusted administrators. The identity
provider behind SSO is trusted to say who a user is and which roles they have.

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

- It can be **limited** to some privileges and/or scope rules. The limit only
  ever narrows: a token never has more than its user has today.
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

**Privileges.** Two kinds:

| Global (not tied to servers) | Server (tied to servers) |
|---|---|
| `users.manage`, `roles.manage`, `audit.read`, `proxies.manage` | `servers.read`, `servers.enable`, `servers.disable`, `servers.drain`, `servers.weight` (not used yet), `stats.reset`, `traffic.test` |
| Owner-gated: `settings.branding`, `settings.notifications`, `users.impersonate` | |

A **role** holds global privileges plus **grants**: server privileges on
**every server**, or only on servers matching **scope rules**. A user's rights
are the union of their roles.

**Requests and approvals** keep four eyes: nobody approves their own request;
approving needs `maint.approve` on every server of it, and the change is made
and logged as the approver ("requested by …"). A pre-approved extension is
for one user and one maintenance, used up as it is taken.

**Backups** are owner-only (step-up when on): the archive holds every
user's password hash, the sealed secrets and the audit log. It is sealed with
AES-256-GCM — with a key derived (Argon2id) from `ORBIT_MASTER_KEY`, or from a
passphrase of 12+ characters — and the secrets inside stay sealed with
`ORBIT_MASTER_KEY`, so a leaked passphrase archive alone doesn't expose them.
Restore is a command run with Orbit stopped, never an upload. Keep the
archives and `ORBIT_MASTER_KEY` apart.

**Scheduled changes** run as their author with the author's rights **when they
run** (roles, scope, a disabled account stops them), never the rights at the
time they were planned; a change planned with a narrowed API token is checked
against the token when planned and against the user when it runs (it never
gets wider than the user). **Change freezes** refuse changes (423) except by
superusers; Orbit's own timers still run.

**Scope rules.** One rule per line. A rule is `key=glob` parts separated by
spaces; all parts must match. A server is in scope when any rule matches. Keys
are the parts of the server name `<env>_<machine>_<service>` plus where it
lives: `env`, `machine`, `service`, `proxy`, `backend`. Globs use `*`, `?` and
`[…]`, case-insensitive. Examples: `env=gmr`, `service=facebook`,
`env=rda machine=web0*`. Rules are validated on save; a rule that can't be
parsed never widens access.

**Built-in roles** (they can't be edited):

| Role | Rights |
|---|---|
| `admin` | **Superuser**: every global and server privilege, except the owner-gated ones |
| `operator` | `audit.read`; read, enable, disable, drain, reset counters and traffic test on every server |
| `viewer` | `audit.read`; read every server |
| `none` | Nothing (default for new SSO users) |

**Enforcement on every change.** The server, not the page, decides:

- Global privileges are checked on the route before the handler runs.
- Server changes (`/operations/<ready|drain|maint>`) check the matching
  privilege (`servers.enable`, `servers.disable`, `servers.drain`) **per target
  server**, every time. If any target is out of scope, **nothing** is changed,
  the answer is `403` naming the servers, and the refusal is audited.
- An **unknown server** counts as out of scope. For a superuser it is passed on
  and the agent refuses it, so the change fails. An unknown proxy is rejected
  with `400`.
- Servers outside your `servers.read` scope are hidden from the state you
  receive. Resetting a proxy's counters needs `stats.reset` on **every** server
  of that proxy; the traffic test needs `traffic.test` on at least one.

**The owner.** Exactly one user is the **instance owner**: first the bootstrap
admin, later whoever the owner transfers it to. Ownership is an identity, not a
privilege, so no role can grant it. Owner-only:

- the security policy, SSO settings and feature flags;
- the refresh interval and security alerts;
- owner transfer;
- creating an admin, and granting or removing the `admin` role;
- granting the owner-gated privileges (`settings.branding`,
  `settings.notifications`, `users.impersonate`) through a custom role. The
  superuser flag does not include them.

**Escalation guards.**

- Built-in roles can't be edited.
- A non-owner can't create an admin, grant or revoke the `admin` role, or add or
  remove owner-gated privileges on a role.
- The last active admin can't be disabled, demoted or deleted. You can't
  disable or delete yourself.
- API tokens and impersonation can't exceed or reach the owner (above).
- **Nobody acts on the owner's account** (edit, roles, disable, reset password /
  two-factor / passkeys, unlock, sign out, delete, revoke its tokens) — the owner
  manages it in Settings. **Only the owner acts on an admin's account**: admins
  can't act on each other, and `users.manage` alone can't reach a stronger account.
- The identity provider can't grant the admin role (by claim or default role)
  unless the owner turned on **Let the identity provider grant admin** in SSO.
- An API token can't create API tokens (a limited token would otherwise
  get around its own limit); tokens are made in Settings.

**Sessions and sign-in records.** Signing out, ending a session (Active
sessions) or a detected refresh-token reuse stops that session's access token
at once (each request checks its session). Every failed sign-in is audited as
`auth.login_failed` with the name tried (never the password), and lockouts as
`auth.lockout`.

## Crypto and secrets at rest

| Secret | At rest | Notes |
|---|---|---|
| Passwords | bcrypt, cost 12 | |
| Refresh tokens, API tokens, recovery codes | SHA-256 hash | Plain value shown once or kept only in the cookie |
| TOTP secrets | Sealed, AES-256-GCM | |
| SSO client secret | Sealed, AES-256-GCM | Never returned to the page |
| Agent tokens | Sealed, AES-256-GCM | Never returned to the page (`has_token` only); "Test connection" uses the stored one server side |
| Notification channel secrets (SMTP password, webhook URLs, bot tokens, the webhook signing secret) | **AES-GCM**, the whole configuration sealed in the `settings` table | Masked when read back (write-only on the page); a configuration saved by an older build is sealed at start-up |
| Short-lived login state (SSO state, passkey challenges, SSO codes, live-update tickets) | In the database, keyed by a SHA-256 of a random id | Single-use, expire in minutes, swept every 15 minutes |

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

- **Everything is recorded:** sign-ins, lockouts, refresh-token reuse, user,
  role, token and proxy changes, settings, and **every server change and every
  refusal**. A server change records the targets, whether it worked, the error
  if not, and the optional **reason** the user typed. A refusal records
  `refused, outside your scope: …`. Each entry has the user, the real admin when
  impersonating, the client IP and the time.
- **Hash chain:** each entry stores `sha256(previous hash ‖ entry)`. Writes are
  serialised so the chain is well-defined. **`GET /api/v1/audit/verify`** (Admin →
  Audit) walks the whole chain and reports the first entry that was changed,
  inserted or removed.
- **Syslog:** with `SYSLOG_ADDR`, every entry also goes out as JSON, with its
  chain hash, to syslog or your SIEM. Forwarding never blocks a change; if the
  queue is full an entry is kept only in the database and a warning is logged.
- v0's history can be appended once with `orbit import-v0-audit`.

> The chain makes after-the-fact edits **detectable**, not impossible. The hash
> has no secret key, so someone who can write the database can rewrite the
> whole chain consistently. The syslog copy is your independent record: keep it
> somewhere Orbit's host can't edit.

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
- **Limits:** request bodies up to `MAX_BODY_BYTES` (4 MiB); header read
  timeout 10 s, read timeout 30 s, idle timeout 120 s.
- **Outbound requests** for SSO, notifications and the update check use an
  SSRF-guarded client: it won't connect to loopback, link-local (including the
  cloud metadata address) or unspecified addresses, checks the IP it actually
  dials, and never follows redirects. Private LAN addresses stay reachable.
- **CORS** is off unless `CORS_ORIGINS` is set (dev only).

## The agents

- Each proxy has **its own token** (`AGENT_TOKEN`). The agent refuses to start
  without one and compares it in constant time. Orbit stores it sealed and
  never sends it to the browser: the page only ever talks to Orbit.
- Orbit never follows a redirect from an agent, so the token isn't sent
  anywhere else.
- **Each agent serves plain HTTP, HTTPS, or HTTPS with a client certificate**,
  chosen per agent ([AGENTS.md, step 7](AGENTS.md#7-keep-the-agent-port-private-and-use-tls)).
  Over plain HTTP the token travels in clear on every request. Over HTTPS LB
  Admin checks the agent's certificate (system CAs plus `AGENT_CA_FILE`; TLS
  1.2+). With mutual TLS (`TLS_CLIENT_CA_FILE` on the agent) only a caller
  with a certificate from that CA — Orbit's `AGENT_CLIENT_CERT_FILE` — gets
  as far as the token check, so a leaked token alone is not enough. The token
  is required in every mode.
- **Admin → Proxies → Agents must use HTTPS** (off by default) makes Orbit
  refuse `http://` agent URLs; it can't be turned on while one remains. Turn
  it on once every agent has moved.
- **Keep agent ports private** in every mode: firewall `AGENT_PORT` (9000) so
  only Orbit's host can reach it (`AGENT_BIND` limits the agent to one
  address).
- The agent's token and TLS key can come from a file or HashiCorp Vault
  (`SECRETS_BACKEND`) instead of its env file; Orbit's client key too
  (`AGENT_CLIENT_KEY`). Certificate files are reloaded when they change.
- Use a different token per proxy, and change it (on the agent, then in Admin
  → Proxies) when someone who knew it leaves.

Install and token steps are in [AGENTS.md](AGENTS.md).

## Certificates and private keys

- **Who**: seeing the certificates needs `certs.read`; replacing them,
  connecting PKI Management and renewals need `certs.manage` (a global
  privilege — it can change what every proxy serves). The owner can turn the
  whole feature off (`certificates` flag).
- **Private keys are never stored by Orbit.** An uploaded key passes through
  Orbit's memory to the agents (over the agent link — use HTTPS or mutual
  TLS for agents across untrusted networks) and is written only on the proxy,
  with the file's mode and owner kept (the previous one kept as a hidden
  `.<file>.prev` with the same mode). From PKI Management, Orbit makes the
  key and sends PKI Management only a CSR; a renewal makes a new key.
- **The audit log** records the certificate's name, names, issuer, serial,
  fingerprint and expiry and where it went — never the key.
- **PKI Management's API token** is sealed with `ORBIT_MASTER_KEY` like the agent
  tokens and never returned. Give it the least: an API token of a PKI user
  with `cert.issue` (and `ca.read`) on the CAs Orbit should use — limited
  if PKI Management allows. Its calls go through the SSRF-guarded client; a
  private CA in front of it: `PKI_CA_FILE`.
- **Agent certificates**: replacing an agent's own certificate with one Orbit
  wouldn't trust for that agent's URL is refused unless forced, so Orbit
  doesn't cut itself off.
- Every check that matters runs on the agent too (key matches, valid,
  names), so a direct API call can't install a broken certificate.

## Operator checklist

- [ ] `GIN_MODE=release` (the compose files set it).
- [ ] `JWT_SECRET` and `ORBIT_MASTER_KEY`: 32+ random characters each
      (`openssl rand -hex 32`), kept out of git.
- [ ] **Back up `ORBIT_MASTER_KEY`**, and back up the database (`/data/orbit.db`)
      separately. Test a restore.
- [ ] HTTPS in front of Orbit, with HSTS at the proxy.
- [ ] `TRUSTED_PROXIES` set to your proxy's address.
- [ ] `WEBAUTHN_RP_ID` / `WEBAUTHN_RP_ORIGINS` set to the public host name
      if you want passkeys.
- [ ] Agent ports reachable only from Orbit; one token per proxy.
- [ ] `/metrics` (if enabled) blocked at the proxy.
- [ ] `SYSLOG_ADDR` pointing at a log store Orbit's host can't edit.
- [ ] First sign-in done, password changed, two-factor set up for the owner.
- [ ] Consider `otp_enforced`, and step-up (`step_up_auth`).
- [ ] Owner security alerts on, with a notification channel.
- [ ] `PROXIES` removed from the environment after the first start.
- [ ] Give `users.manage` and `admin` only to people you would trust as owner
      (see accepted risks).
- [ ] Run `/audit/verify` now and then.

## OWASP Top 10

| Area | Status |
|---|---|
| A01 Broken access control | Every route gated; server privileges checked per target on every change; unknown servers refused; owner-only controls outside RBAC; escalation guards (the owner's and admins' accounts are protected from `users.manage`; API tokens can't mint tokens). See accepted risks. |
| A02 Cryptographic failures | AES-256-GCM sealing under an Argon2id key, bcrypt 12, HS256 with algorithm check, CSPRNG tokens stored as hashes. The agent link is HTTP, HTTPS or mutual TLS per agent; "agents must use HTTPS" can enforce TLS. |
| A03 Injection | GORM with bound parameters; request bodies validated (names, URLs, `oneof` lists, scope-rule syntax); the page is served from an embedded file system (no path traversal); agent URLs must be `http(s)://`. |
| A04 Insecure design | All-or-nothing changes per proxy, preview before apply, typed confirmation for group actions, audit with reasons. |
| A05 Misconfiguration | Release mode refuses weak secrets; secure cookies by default; CSP and hardening headers; CORS off; generic `500` messages; distroless, non-root image. |
| A06 Vulnerable components | CI runs `govulncheck`, `npm audit` and gitleaks on every PR. |
| A07 Authentication failures | Memory access token + rotating `HttpOnly` refresh cookie with reuse detection; CSRF double submit; lockout + per-IP limit; constant-cost unknown-user check; TOTP and passkeys; OIDC with PKCE, state and nonce. |
| A08 Integrity failures | Hash-chained audit log with a verify endpoint and an external syslog copy. |
| A09 Logging and monitoring | Every change and refusal audited; secrets never logged (`json:"-"` on sensitive fields); optional syslog, Prometheus and owner security alerts. |
| A10 SSRF | SSO, notification and update-check requests go through the guarded client. Agent and load-balancer requests refuse link-local and metadata addresses (private ones are allowed; see accepted risks). |

## Accepted risks and known gaps

- **Plain-HTTP agents are allowed by default**, so existing proxies keep working. Move them to HTTPS or mutual TLS and turn on "Agents must use HTTPS" (above).
- **Agent and load-balancer URLs may be private or loopback** (that's where
  agents live), so `proxies.manage` can make Orbit call internal addresses
  and see the error text; link-local ones (cloud metadata, `169.254.0.0/16`,
  `fe80::/10`) are refused. It is a trusted privilege.
- **The audit chain has no secret key** (see Audit). Rely on the syslog copy.
- **Rate limits are per process.** Orbit runs as a single instance; add a
  limit at your proxy too.
- **The lockout is temporary** by design (limits lockout as a denial of
  service). It holds for every way in, passkeys included.
- **SSO trusts the identity provider** for the roles it names (except admin,
  which needs the owner's opt-in). Control group membership at the provider.
- **Public endpoints** (no sign-in): `/api/v1/healthz`, `/readyz`, `/version`,
  `/branding`, the sign-in routes, the API reference `/api/docs` and
  `/api/v1/openapi.json`, and `/metrics` when enabled.
- **The API reference is public** (`/api/docs`, `/api/v1/openapi.json`): it
  lists the endpoints, not data; every call still needs a token.
- **No built-in backups** yet (backlog, Phase F).

## Reporting a problem

Please don't open a public issue with exploit details. Contact the maintainer
privately through the repository, or open a private issue. Include the version
(About page, or `GET /api/v1/version`), what an attacker could do, and the
steps to reproduce. For general trouble, see [TROUBLESHOOTING.md](TROUBLESHOOTING.md).
