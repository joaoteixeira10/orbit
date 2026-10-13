# Architecture

How Orbit v1 is built: the pieces, how a request flows through them, where
the data lives and why things are the way they are. Read this before you change
the code, add a feature, or need to reason about what happens when an agent or
the admin goes down. For using the page see [USER-GUIDE.md](USER-GUIDE.md); for
settings see [CONFIGURATION.md](CONFIGURATION.md); for the security model see
[SECURITY.md](SECURITY.md).

## Goals

- **One place to take servers in and out of rotation** — enable, drain or
  disable one server, a service, a machine or an environment, on every nginx or
  HAProxy load balancer that serves it.
- **See before you change** — every change can be previewed per proxy (the
  nginx file diff, or the HAProxy commands), and each proxy applies it **all or
  nothing**.
- **Least privilege** — roles grant privileges on every server or only on the
  servers matching **scope rules** on name parts (`env=gmr`,
  `env=rda machine=web0*`).
- **Every change is traceable** — changes and refusals go to a hash-chained
  audit log, optionally also to syslog.
- **Easy to run** — one container and one volume for the admin; one static
  binary per proxy (lb-agent).

## The pieces

| Piece | What it is | Where |
|---|---|---|
| **orbit** | One Go binary: the `/api/v1` JSON API, the Vue SPA embedded in it, the proxy poller and the background jobs. Ships as one distroless, non-root, multi-arch image. | `api/`, `web/`, `Dockerfile` |
| **SQLite** | The only database: users, roles, tokens, the proxy registry, settings, the audit log. One file, `DB_URL` (default `/data/orbit.db` in the image). | a Docker volume (`orbit-data`) |
| **lb-agent** | One static Go binary on each proxy machine, next to nginx or HAProxy. It reports the state, changes servers and certificates. Orbit talks to it over HTTP, HTTPS or mutual TLS with a per-proxy token. | `api/cmd/lb-agent`, `api/internal/agent` — see [AGENTS.md](AGENTS.md) |
| **The lab** | Demo proxy machines for development: nginx `gmr` and `rda`, HAProxy `gmr-hap` (in front of the same machines as `gmr`), each its own compose project. | `lab/` |

The browser only ever talks to Orbit. Agent tokens stay on the server.

```
  Browser (Vue SPA)                                Scripts / pipelines
        │  access JWT + refresh cookie                   │  Bearer orb_… (API token)
        ▼                                                ▼
┌──────────────────────── orbit: one Go binary, one container ────────────────────────┐
│                                                                                         │
│  Gin router ─► middleware ─► auth.Required ─► handlers ─► audit.Recorder ──► syslog     │
│   /api/v1        (request id, logs,   (JWT or API token,           (hash chain)  (opt.) │
│   /api/docs       CSP, body limit,     RBAC per request)                                │
│   SPA (embed)     metrics, rate limit)                                                  │
│                                                                                         │
│  events.Hub (SSE /api/v1/auth/events) ◄── "proxies", "privileges", "users", "roles"     │
│                                                                                         │
│  proxies.Service: poller ─► last good state per proxy ─► /state (filtered per caller)   │
│                   agent client (apply, preview, reset)   traffic test                   │
│                                                                                         │
│  jobs.Scheduler (cron): update check, token sweeps                                      │
│                                                                                         │
│  GORM ─► SQLite (/data/orbit.db)       keystore: AES-256-GCM, KEK from ORBIT_MASTER_KEY │
└──────────────┬───────────────────────────────────────────────┬──────────────────────────┘
               │ HTTP, Authorization: Bearer <AGENT_TOKEN>      │ HTTP GET <lb_url><path>
               ▼                                               ▼ (traffic test only)
   ┌─────────────────────────────┐                  ┌──────────────────────────┐
   │ lb-agent (per proxy host)   │                  │ the load balancer itself │
   │  ENGINE=nginx:  upstream    │ ───────────────► │ nginx or HAProxy         │
   │   files, nginx -t + reload  │                  └──────────────────────────┘
   │  ENGINE=haproxy: Runtime    │
   │   API on the stats socket   │
   └─────────────────────────────┘
```

## Stack

| Layer | Choice |
|---|---|
| Backend | **Go 1.26** (built with `golang:1.26.9`), **Gin** (HTTP), **GORM** with **glebarez/sqlite** (pure Go SQLite, no CGO), `golang-jwt`, `pquerna/otp` (TOTP), `go-webauthn` (passkeys), `coreos/go-oidc` + `x/oauth2` (SSO), `robfig/cron`, `zap` (logs), Prometheus client, `swaggo/swag` (OpenAPI) |
| Frontend | **Vue 3**, **Vite**, **Pinia**, Vue Router, **Tailwind CSS** 3, **vue-i18n** (English and Portuguese, with a parity test), `lucide-vue-next` icons, `@simplewebauthn/browser` |
| API reference | **Scalar** at `/api/docs`, served by Orbit from its own origin (`/scalar/standalone.js`, no CDN), over the committed OpenAPI document at `/api/v1/openapi.json` |
| Agent | **Go**, the same module (`api/cmd/lb-agent`, `api/internal/agent`): one static binary, standard library plus the shared `secrets` and `tlsfiles` packages |
| Image | Multi-stage `Dockerfile`: Node builds the SPA, Go embeds it and cross-compiles amd64 + arm64, runtime is `gcr.io/distroless/static-debian12:nonroot` |
| Tests | Go unit + router integration tests (the agent's too, with HTTPS and mutual TLS), Vitest, Playwright against a real nginx with the Go agent |

## Repository layout

| Path | What |
|---|---|
| `api/cmd/server/` | The `orbit` binary: wiring in `main.go`, plus the `healthcheck` and `import-v0-audit` subcommands |
| `api/cmd/lbctl/` | `lbctl`, the command-line tool: API calls with a personal token ([LBCTL.md](LBCTL.md)); also in the image as `/lbctl` and attached to releases |
| `api/cmd/lb-agent/` + `api/internal/agent/` | **lb-agent**: the HTTP API (`agent.go`), the nginx and HAProxy drivers, health checks from the config, the nginx file diff, config + TLS serving (`serve.go`); released for linux amd64 / arm64 and in the image as `/lb-agent` |
| `api/internal/tlsfiles/` | Certificates from files (reloaded when they change) or PEM, CA bundles — for lb-agent's server certificate and Orbit's client certificate |
| `api/cmd/labseed/` | `make lab-seed`: adds the lab proxies and test users to a dev instance |
| `api/internal/api/` | Router, middleware and every HTTP handler |
| `api/internal/auth/` | Passwords, JWTs, refresh tokens, API tokens, TOTP, passkeys, OIDC, the `Required` middleware, first-admin bootstrap |
| `api/internal/rbac/` | Privilege catalog, scope rules, built-in roles, effective permissions |
| `api/internal/proxies/` | Agent client, poller, last good state, operations, traffic test, the one-time `PROXIES` import |
| `api/internal/audit/` | Hash-chained audit recorder, verify, syslog forwarding |
| `api/internal/events/` | SSE hub and stream tickets |
| `api/internal/jobs/` | Cron scheduler |
| `api/internal/models/` | GORM entities (`models.go`) |
| `api/internal/{config,secrets,db,keystore,ephemeral,safedial,feature,metrics,notify,version,importv0,webui}/` | Config from env, secrets providers (env / file / Vault), DB open + migrate, sealing, single-use tokens, SSRF-guarded HTTP client, feature flags, Prometheus, notification channels, version + update check, v0 audit import, the embedded SPA |
| `api/docs/` | Generated OpenAPI (`make swag`), committed |
| `web/src/` | The SPA: `views/`, `components/`, `stores/`, `lib/`, `i18n/`, `router/`, `api/` |
| `web/e2e/` | Playwright tests |
| `lab/` | Demo proxies (`gmr`, `rda`, `gmr-hap`) and their machines (`shared/`); `lab/v0-data/` holds v0 data, kept out of git |
| `docs/` | These docs and the blueprints |
| `scripts/` | `set-version.sh` (see [RELEASING.md](RELEASING.md)) |
| `.gitea/workflows/` | Test, E2E, E2E (lab) and Security on pull requests; Build & Push and Release by hand only (see [RELEASING.md](RELEASING.md)) |
| root | `Dockerfile`, `docker-compose.yml` (pull), `docker-compose-build.yml` (build from source), `docker-compose.dev.yml` (live reload), `Makefile`, `.env.example`, `CHANGELOG.md`, `BACKLOG.md`, `LICENSE` |

## Data model

All entities are in `api/internal/models/models.go`. The schema is created by
**GORM AutoMigrate** at every start (idempotent). Every entity except `Setting`
and `EphemeralToken` embeds `Base`: a **UUIDv7** id (time-ordered, so
`ORDER BY id` is insertion order — the audit chain relies on it) plus
`created_at` / `updated_at`.

| Entity | What it holds |
|---|---|
| `User` | Username, display name, email, bcrypt hash (empty for SSO users), `disabled`, `owner`, `oidc_subject`, `token_version`, `must_change_password`, failed-login count and `locked_until`, last sign-in, the **sealed** TOTP secret, hashed recovery codes, `otp_required` (per-user "require two-factor"), the WebAuthn user handle. Roles through the `user_roles` join table. |
| `Role` | Name, description, `builtin`, `superuser`, global privileges (JSON list). Built-ins: `admin` (superuser), `operator`, `viewer`, `none`. |
| `RoleGrant` | Server-scoped privileges of a role: on **every server** (`all_servers`) or on the servers matching its **scope rules** (JSON list). |
| `UserPrefs` | Per-user timezone, theme, accent colour, notification choices, and `change_details` (`detailed` or `compact` change confirmations). |
| `RefreshToken` | sha256 of each refresh token, its `session_id` (stable across rotations), user agent, IP, expiry, `revoked`, `rotated_at` (for reuse detection). |
| `APIToken` | sha256 of a personal `orb_…` token, a display prefix, expiry, last use, and the optional **narrowing** (fewer privileges and / or scope rules). |
| `WebAuthnCredential` | A passkey: credential id, public key, sign count, flags. Nothing secret, so nothing sealed. |
| `Proxy` | Name (used in scope rules and the audit log), label, agent URL, the **sealed** agent token, `lb_url` (traffic test), `public_url` (the link on the page), position. |
| `ServerIntent` | The **desired state** of one server (proxy id, backend, server): `admin` (ready / maint / drain), the timer of a timed maintenance (`until`, UTC), reason, who set it (`user_id`, empty = as found or Orbit itself), the last failed attempt to end the timer. Removed with its proxy. |
| `Freeze` | A change freeze: `starts_at`, `until`, reason, scope rules (JSON; empty = every server), who added it. |
| `Sample` | One minute of one server (or backend) of a proxy: requests and errors in it, p50 / p95, up all minute. Written by the poller, kept 24 hours (pruned hourly). No `Base`: an integer key. |
| `ManagedCert` | A certificate from PKI Management that Orbit keeps installed: the CA, common name and names (JSON), validity asked, the targets (`[{proxy, id}]`: agent certificate ids), auto-renew and how many days before, the last PKI certificate id, serial and expiry, last attempt and error. **No key**: each issue makes a new one, which goes only to the agents. |
| `MaintRequest` | A request for a timed maintenance or an extension: targets (JSON), minutes, reason, requester, status (pending / approved / denied / cancelled / expired), who decided and a note, the grace count and the current grace end, and (extend) the original end. `ServerIntent` also keeps a pre-approved extension (`allowance_minutes`, `allowance_user`). |
| `Rollout` | A rolling action: units (a label and its instances, JSON) in order, options, the current unit, phase (out / waiting / back) and since when, status, result. Moved on by the 5 s tick. |
| `ScheduledChange` | A change for later: `at`, the state, the operations body (JSON), a summary, reason, author, `status` (pending / done / failed / cancelled / missed), result, when it ran. |
| `Setting` | Key / JSON value store for runtime policy: `environments` (e.g. `gmr` → Staging), `proxy_refresh_seconds`, `change_details_policy`, `features`, `otp_enforced`, `hide_local_login`, `passwordless_enabled`, `auto_sso`, `oidc_config`, `notification_config`, `security_alerts`, `version_history`, `kek_salt`, `v0_audit_imported`, and the branding keys (app name, accent, logo, …). |
| `AuditLog` | Who (`user_id`, `impersonator_id`), action, target, IP, detail (JSON), and `prev_hash` / `hash` for the chain. |
| `EphemeralToken` | Short-lived, single-use state: OIDC login state, WebAuthn challenges, SSO one-time codes, SSE tickets. Only a sha256 of the id is stored; rows are deleted on use and swept when expired. |

What is **not** in the database: the proxies' live state. It is polled from
the agents and kept in memory (see below). Their desired states are
(`ServerIntent`).

`Setting` also holds `allow_empty_backends`, `require_reason` and
`reconcile_auto` (Admin → Proxies).

## Request pipeline

Every request passes the same middleware, in this order (`api/internal/api/router.go`):

1. `gin.Recovery`, then a **request id** (`X-Request-Id`, taken from the request or generated).
2. A **JSON request log** (method, path, status, latency, client IP, request id).
3. **Security headers**: CSP `script-src 'self'` (no inline script), `X-Frame-Options: DENY`, `nosniff`, `no-referrer`.
4. A **body limit** (`MAX_BODY_BYTES`, default 4 MiB) and Prometheus metrics.
5. Optional **CORS** (dev only, `CORS_ORIGINS`).

The client IP comes from `X-Forwarded-For` only for proxies listed in
`TRUSTED_PROXIES` (empty = trust none). Sign-in and second-factor endpoints
have a **per-IP token bucket** on top (`AUTH_RATE_BURST`, `AUTH_RATE_PER_MINUTE`);
an empty bucket answers `429` with `too many attempts; please slow down`.

Authenticated routes run `auth.Required()`:

- A bearer starting with `orb_` is an **API token**; anything else must be a
  valid **access JWT** (15 minutes, kept in memory by the SPA).
- The user is loaded with roles and grants **on every request**, so disabling
  an account, changing a role or bumping `token_version` takes effect at once.
  The JWT carries no roles.
- A user with `must_change_password`, or who must set up two-factor, can only
  reach the endpoints for that until done.
- Global privileges (`users.manage`, `roles.manage`, `audit.read`,
  `proxies.manage`) are checked by `RequireGlobal` on the route; owner-only
  routes by `RequireOwner`. **Server-scoped** privileges are checked **inside
  the handlers**, per server, because they need the server.

The refresh token rides in an HttpOnly, path-scoped cookie (30 days, rotated on
every use, reuse detected) with a CSRF double-submit cookie. Sign-in options:
password (bcrypt), TOTP or passkey as second factor, passwordless passkeys, and
OIDC SSO with just-in-time users. The full design is in
[AUTH-BLUEPRINT.md](AUTH-BLUEPRINT.md); what Orbit took from it is in
[BLUEPRINT-FIT.md](BLUEPRINT-FIT.md).

### Scope rules

A rule is space-separated `key=glob` parts that must all match; a server is in
scope if **any** rule of the grant matches (`api/internal/rbac/rbac.go`). Keys:

| Key | Value |
|---|---|
| `env`, `machine`, `service` | The parts of the server's label `<env>_<machine>_<service>` (e.g. `gmr_web01_facebook`) |
| `proxy` | The proxy's name in the registry |
| `backend` | The nginx upstream or HAProxy backend |

Globs use `*`, `?` and `[..]`, case-insensitive. A server without a parseable
label falls back to env = proxy name, machine = host of its address,
service = backend.

## The proxy subsystem

`api/internal/proxies` owns everything that talks to agents.

**The poller.** `Service.Run` polls every proxy's `GET /v1/state` in parallel,
then waits the **refresh interval** before the next round. The interval is the
owner's setting (Owner → Live updates, `proxy_refresh_seconds`, 1–300 s) or
else `PROXY_POLL_SECONDS` (default 10). It is read before each wait, so a new
setting applies without a restart. Polls use a 3-second timeout.

**Last good state.** Each proxy has a snapshot in memory: the last state that
came back, when it was fetched, whether the last poll worked, and its error.
When an agent is down, the last good state stays and the proxy is marked
unreachable; the page shows its servers as unknown and won't change them. A
proxy that was never reached says `not polled yet`. Snapshots are not stored
in the database: after a restart they come back with the first poll.

**Change detection.** Each state is hashed without its clock field (`now`).
When a hash, the reachability or the error changes, the service calls its
`OnChange` hook, which broadcasts the SSE event `"proxies"`.

**Desired state.** After each good poll, a server seen for the first time gets
its state as its desired one (`ServerIntent`), and each server whose reported
state differs from its desired one counts one more poll; at **two polls in a
row** it has drifted (a poll that raced a change never counts twice). A change
through Orbit updates the desired state once the agent took it — under a
per-proxy lock, so a user's change and Orbit's own (timers, restores) never
interleave. `POST /api/v1/reconcile/restore` applies the desired states again
(the timer is kept); `/accept` makes the reported state the desired one. With
`reconcile_auto` on, `Run` restores drifted servers after each poll, logged as
`system` (`server.restore`); a failure holds that proxy back for 5 minutes.

**Timed maintenance.** `minutes` on a disable or drain stores `until` with the
desired state. A goroutine started by `Run` checks every 5 seconds and enables
the servers that are due, one agent call per proxy, logged as `system`
(`server.enable`, "timed maintenance ended: <reason>"). If the agent can't be
reached, the failure is logged once, kept on the row (`timer_error`) and
retried every tick. Timers live in the database, so they survive a restart.

**`GET /api/v1/state`** returns, for the caller:

- every proxy with its state, **filtered** to the servers the caller has
  `servers.read` on (`hidden` counts the rest);
- on each server, `can: {ready, maint, drain}` — what the caller may do there —
  and `intent: {admin, until, reason, by, at, drift}`;
- `can_test` (traffic test: `traffic.test` on at least one server) and
  `can_reset` (reset counters: `stats.reset` on **every** server of the proxy);
- the agent URL only for holders of `proxies.manage`;
- the environment labels, the last 40 server changes (the sidebar feed) and the
  page settings (`refresh_seconds`, `change_details_policy`,
  `allow_empty_backends`, `require_reason`, `auto_restore`).

`?refresh=1` polls all agents first (up to 5 seconds).

**Operations.** `POST /api/v1/operations/{ready|drain|maint}` takes a list of
`{proxy, backend, server}` targets and an optional reason:

1. Each target is looked up in the last known state and checked against the
   privilege for that state (`servers.enable`, `servers.drain`,
   `servers.disable`). A server the admin doesn't know counts as out of scope
   (except for superusers).
2. If any target is refused, **nothing** is sent: the answer is `403`
   `outside your scope: …`, and the refusal is audited.
3. Otherwise the targets are grouped per proxy and sent as **one agent call per
   proxy**, in parallel. Each agent applies its part **all or nothing**.
4. One audit entry records the targets, the reason, and `ok` or the errors.
   If any proxy failed, the answer is `409` with one line per failed proxy.

> All or nothing is **per proxy**, not across proxies. If `gmr` applies and
> `gmr-hap` fails, `gmr` keeps the change. The error names the proxy that failed.

**Preview.** `POST /api/v1/operations/{to}/preview` does the same checks and
asks each agent's `/preview`: the list of servers that would change, the nginx
file diff or the HAProxy commands. Nothing changes and nothing is audited.

**Traffic test.** `POST /api/v1/proxies/{id}/test?backend=…&n=…` sends `n`
requests (1–500, default 20) from Orbit to the proxy's `lb_url` plus the
backend's path, and counts the `X-Backend` response header. Answers without it
count as `unknown`, HTTP errors as `HTTP <code>`, failures as
`connection error`.

**Reset counters.** `POST /api/v1/proxies/{id}/reset` calls the agent's
`/v1/stats/reset`; it needs `stats.reset` on every server of that proxy.

**Registry.** `/api/v1/proxies` (list, add, edit, remove, `check`) needs
`proxies.manage`. The agent token is sealed on the way in and never returned;
editing with an empty token keeps the stored one. Any registry change triggers
a poll. On an **empty** registry, v0's `PROXIES` variable is imported once at
start-up.

## Proxy features in detail

How each proxy feature works, end to end. The user's view is in
[USER-GUIDE.md](USER-GUIDE.md); the calls in [API.md](API.md).

- **Certificates** (feature flag `certificates`, **on** by default; `certs.read`,
  `certs.manage`): the agents list what they serve (`GET /v1/certs`: nginx from
  `nginx -T`, HAProxy from `show ssl cert`, plus the agent's own) and replace
  it (`PUT /v1/certs/<id>`, `POST …/preview`): checked first (parses, key
  matches, valid now; dropped names, chain order, no intermediates, no
  serverAuth, < 7 days need `force`), then nginx: files written (previous as
  hidden `.<file>.prev`), `nginx -t` + reload, put back if refused; HAProxy:
  `set ssl cert` + `commit ssl cert` (live), then the file; the agent's own:
  files rewritten, `tlsfiles` loads them on the next handshake. Orbit
  (`handlers_certs.go`) keeps the last list per proxy (`certStore`, refreshed
  every 10 minutes by `certTick`, on demand, and for proxies not asked yet),
  `inspect` (parse + checks, nothing sent), `deploy` (per target, one after
  another; an agent's own certificate must verify against `AGENT_CA_FILE` /
  the system CAs for the agent URL's host, else `force`), audited
  `cert.replace` with CN, names, issuer, serial, fingerprint, expiry, done /
  failed — never the key — and a `cert.replaced` event. **PKI Management**
  (`internal/pkiclient`, guarded client, `PKI_CA_FILE`): URL + sealed `pkim_`
  token in settings; `issue` makes an ECDSA P-256 key and a CSR here, `POST
  /api/v1/certificates {ca_id, type: webserver, common_name, sans, days_valid,
  csr_pem}` there (needs `cert.issue` on the CA), and deploys the fullchain +
  key; a `ManagedCert` row remembers what and where (saved only if installed
  somewhere). Renewal (by hand or `autoRenew` when `RenewDays` are left) is a
  new issue with a new key, deployed with `force` (same names), audited
  `cert.pki_issue` as the user or `system`; failures → `cert.failed`. Expiry
  alerts → `cert.expiring` (organisation channels only, the **Certificates**
  toggle). `/metrics`: `lb_cert_expiry_timestamp_seconds{proxy, kind, path,
  cn}`; `lbctl certs`. UI: Proxies → Certificates (`CertificatesView`,
  `CertReplaceDialog`, `CertPkiDialog`).

- **Change confirmations**: *detailed* (the per-proxy preview: nginx diff / HAProxy commands,
  and the optional reason) or *compact* (what, where and its current status, like "Disable
  `qscdidcert3` on `gmr`? Current status: UP"). Each user picks in Settings → Appearance;
  an admin with `proxies.manage` can set it for everyone in Admin → Proxies (the user's
  choice is then shown as set by an administrator). Both modes keep the safety checks: the
  empty-backend warning and the typed name for group actions.
- **Every change is confirmed** (Orbit): one server too, enable as well as disable, from
  Operations and the Proxies page — the dialog (detailed or compact) always opens; group
  actions also need the typed name. (v0 applied a single safe change at once.)
- **Audit page** (Security → Audit, `audit.read`): search, load older, **Verify integrity**
  (the hash chain: "Chain intact (N entries)" / "Tampering detected at entry #…") and
  **Download CSV** — the whole log or what the search matches, with each entry's chain
  hash, served by `GET /api/v1/audit/export` (the export itself is audited; cells that
  look like formulas are neutralised). It's the only audit view (no Admin tab).
- **Group changes are named**: the page sends what a change covered (`scope`, e.g. "All
  GMR", "facebook · All envs"); the audit log and the recent-activity feed show it
  ("All GMR · 14 servers") instead of only the first server.
- **A target's own button can't take a service's last server out** (by default): in
  Operations, a target's **Enable / Disable** can't take the last live server (up or
  checking, and ready — counting servers the user can't see) of a **service** out of
  rotation. The service is the `<service>` part of the server names (else the backend
  name), counted **across every proxy and environment**: cspproxy with one server on rda
  and one on gmr + gmr-hap may lose either; analytics with only docker108 may not. The
  dialog blocks it and the server refuses it (409, `emptied_services`, audited). A backend
  left empty on one proxy while the service lives elsewhere only gets the warning. **Group actions** (a card's or an environment's **Disable
  all**, with the typed name) and **the Proxies page** (⋯ menu, bulk bar) are not limited:
  they show the warning and may go ahead — the page sends `empty_ok: true` for them. An
  admin with `proxies.manage` can let the target buttons do it too (Admin → Proxies →
  A service's last server). API callers are limited unless they send `empty_ok`.
- **Timed maintenance** (Phase E): Disable / Drain have **For** — until enabled again (the
  default), 15 / 30 min, 1 / 2 / 4 / 8 / 24 h (API: `minutes`, 1–10080). It **needs a
  reason**. When the time is up Orbit enables the servers itself (5 s tick, logged as
  `system`, "timed maintenance ended: <reason>"); an unreachable agent is logged once and
  retried. The row shows "⏱ 23m left", the toast "back in rotation at 14:30", the feed
  "⏱ until 14:30". Enabling earlier ends the timer. Timers are stored, so a restart keeps
  them.
- **Drain, then maintenance** (Phase F): Disable has **Drain first** where a target is on
  HAProxy (wait 1–60 min, default 5; API `drain_first` / `drain_timeout`), and the ⋯ menu has
  **Drain → maintenance**. HAProxy servers drain; the 5 s timer loop sets maintenance at 0
  sessions (last poll) or at the deadline, logged as `system`; nginx servers go straight to
  maintenance. The intent stores `drain_then_maint`, `drain_until`, `next_minutes` (the timed
  maintenance starts at maintenance). Needs `servers.drain` too; enabling cancels it.
- **Health check from the proxy** (asked 2026-10-08): lb-agent `healthcheck.py` builds the check
  from the load balancer's own config — never a caller's URL — (HAProxy: `option httpchk`
  method / path / version + `Host`, `http-check send` and `expect` status / rstatus / string /
  rstring / `!`, `defaults` and `default-server` merged with the server line, `check`, `no-check`,
  `port`, `addr`, `check-ssl`, `verify none`, global `ssl-server-verify none`; protocol checks →
  TCP connect with a note; nginx: its `HEALTH_PATH`), runs it from the proxy host (5 s, 4 KB of
  body), and answers desc / curl / config lines / ok / status / body / ms / error at `POST
  /v1/check`. Orbit `POST /check` (servers.read on that server). UI: the Check dialog (passes /
  fails, JSON indented, the command to copy), from the Proxies ⋯ menu, **Run check** on failing
  servers, **Check** on down targets in Operations. State: `checked`, `check_desc` per server →
  the Operations **No check** stat and filter, a dashed **no check** tag, the Proxies filter.
  `lbctl check` (exit 1 if one fails).
- **Latency and history** (Phase F): agents report `latency` in server and backend stats — nginx
  p50 / p95 over 5 minutes from `$upstream_response_time` (the last attempt), HAProxy `{avg: rtime}`.
  The poller adds each good poll to one-minute buckets (requests and errors as deltas, counter
  resets handled, the latest latency, up all minute) and writes `Sample` rows when the minute is
  over; 24 h kept. `GET /history` (scope-filtered) feeds the Proxies page's **Last hour** sparkline
  and the **p95** column. `/metrics` adds a collector over the snapshots (`lb_proxy_reachable`,
  `lb_server_up / in_rotation / maintenance / requests_total / errors_total / latency_ms{quantile} /
  sessions / weight`, `lb_backend_requests_total`). `deploy/`: turnkey Prometheus + Grafana compose,
  scrape config, 8 alert rules (promtool-checked; LBCertificateExpiring since Phase H), the dashboard.
- **Maintenance requests and approvals** (Phase F): privilege `maint.approve` (scoped). A user with
  `servers.read` asks (`MaintRequest`: maint for N min, or extend by N min); approvers (maint.approve
  on every server, not the requester) Approve — `runOperation` as the approver with `viaApproval`
  (maint.approve stands in for servers.disable), timed, logged "requested by …" — or Deny; the
  approval can pre-approve extensions (`ServerIntent.allowance_minutes/_user`). `POST /extend`:
  servers.disable, or within the allowance. Bounded grace: `proxies.Service.OnExpiry` lets the API
  hold a due window while an extension request is pending (grace minutes × repeats, escalating to
  admins from the 2nd, then back in rotation or kept without a timer); approved in grace counts from
  the original end, denied → 5 min. Emails to approvers (new) and requesters (decided), org
  channels with the "Maintenance requests" event. UI: Request… / Extend… / ⏱ tag, the Requests
  panel, an orange count on Services, Admin → Proxies grace card. lbctl request / extend / requests /
  approve / deny.
- **Rolling actions** (Phase F, feature flag `rolling`, off): `Rollout` rows moved on by the
  5 s tick (`OnTick`). Units = a server label with all its instances, in the chosen order;
  phases out (maint, `drain_first` optional, until every instance is MAINT, ≤ drain wait + 2
  min) → waiting (Next — page, `POST /rollouts/{id}/next`, `lbctl next` — or N minutes) → back
  (ready, until every instance is UP within the health timeout) → next unit. Every step is
  `runOperation` as the author (scope, the service rule, freezes), audited "rolling · name (i/n)";
  any refusal or a server not healthy in time stops it (failed, `lbadmin.failed`), the rest
  untouched. One rollout per server at a time. Operations **Roll…** dialog (choose, order,
  options) and a panel (Operations and Proxies) with Next / Pause / Resume / Cancel; `lbctl
  roll`, `rollouts`, `rollout ID --until waiting|end`, `next`.
- **Weights / canary** (Phase F, feature flag `weights`, off): lb-agent `POST /v1/weights`
  (+ `/preview`; HAProxy `set server … weight` 0–256 live, kept in the server-state file; nginx
  `weight=N` 1–1000 in the upstream file, `nginx -t` + reload; all or nothing; `/v1/state`
  advertises `weights: {min, max}`). Orbit `POST /api/v1/weights` (`servers.weight` per server,
  the agent's range, freezes, weight 0 counts for the service rule, audited `server.weight`
  "old → new", a Changes event). A backend's **Weights…** dialog shows each server's share of new
  traffic. `lbctl weight`.
- **Encrypted backups** (Phase F, PKI's `backup` adapted): the archive is the whole SQLite
  database (`VACUUM INTO`, gzip) sealed with AES-256-GCM in a JSON envelope (`orbit-backup`
  v1), mode `master_key` (the KEK salt in the envelope) or `passphrase` (Argon2id, 12+ chars);
  secrets inside stay sealed with ORBIT_MASTER_KEY. Owner → Owner → Backups: download now, automatic
  (hourly check, every N hours, keep N, a directory or S3 via minio-go, keys from the secrets
  backend), Run now, last-run status. Restore is `orbit restore-backup <file> [--force]`
  offline (keeps the old file), never an upload into a running instance.
- **Scheduled changes** (Phase F): the dialog's **When: Now / Later, at** (reason required),
  `POST /schedules` (`to`, `at` 30 s–90 days ahead), `lbctl --at`. Checked when planned (a
  preview as the caller) and when run by the 5 s tick (`OnTick`): as the author with their rights
  then (`rbac.Build` of their roles), the same `runOperation` as a change now — scope, the
  service rule, freezes — audited as theirs with `scheduled`. Not run more than 15 min late
  (missed). Listed (pending, the ones you may see) on Operations and Proxies with Cancel (author
  or `proxies.manage`); failed / missed → an `lbadmin.failed` notification.
- **Change freezes** (Phase F): Admin → Proxies (from, until ≤ 90 days, reason, optional scope
  rules); during one, changes on covered servers are refused with 423 except by superusers
  (Orbit's own timers still run); the preview reports `frozen` / `frozen_bypass`; a red
  banner for everyone from a day before.
- **Server notifications** (Phase F): `proxies.Service.OnEvent` gets what the poller sees
  (server down / up from status transitions, agent unreachable after 2 failed polls / back),
  what Orbit does itself (timers, drains, automatic restores, as `server.change`), drift on
  reaching it, failures, and `maint.expiring` 15 min before a timer ends (`warned_at`, once;
  not for timers that short). The API adds users' changes. `notify_events.go` sends them:
  the organisation's (Owner → Notifications → Server events: toggles, team channels, email
  list, the webhook) and personal emails (Settings → Notifications, scoped by `servers.read`,
  not one's own changes). A 2-minute flap guard on health events. English, times in UTC.
  New channels: Teams (Workflows webhook, Adaptive Card) and Slack (incoming webhook).
- **`lbctl`** (Phase F, `api/cmd/lbctl`, docs/LBCTL.md): the CLI for pipelines — `status`,
  `disable` / `drain` / `enable` (`--reason`, `--for`, `--drain-first`, `--wait`, `--dry-run`,
  `--allow-empty`), `restore` / `accept`; patterns are label globs, names, `proxy/backend/server`,
  `@proxy`; a pattern matching nothing is an error; exit 0 / 1 (not done) / 2 (usage). Stdlib only.
  `make lbctl` cross-builds it; the image has `/lbctl`; Release attaches the binaries + SHA256SUMS.
- **Reasons** (Admin → Proxies, off by default): on, every change (page or API) needs a
  reason; the compact dialog then shows the field too.
- **Desired state and drift** (Phase E): Orbit stores the state it last set on every
  server (`ServerIntent`; a server first seen keeps the state it had). A proxy that reports
  something else for two polls in a row (rebuilt, restarted without its state, changed by
  hand) **drifted**: a banner on Proxies and Operations lists the servers ("is in rotation,
  should be in maintenance"), the row says "should be …", the Operations chip gets a dashed
  red border. **Restore** applies the desired state again (needs that state's privilege;
  the timer is kept), **Accept** takes the proxy's state as the new desired one (needs the
  privilege of the state it is in); both confirm and are audited (`server.restore`,
  `server.accept`). **Desired state** (Admin → Proxies, off by default): on, the poller
  restores drifted servers itself, logged as `system`, 5-minute back-off per proxy on
  failure.
- **No "protected environment" flag** (Phase E's guardrail was dropped, 2026-10-01): the
  typed name for group actions and the service last-server rule are the safety rules.
- **Server and backend rows** show HAProxy's **Last** session and **Downtime** columns
  (nginx: last request, no downtime), as in the HAProxy stats page.

## The agent API

lb-agent (`api/internal/agent`) serves a small HTTP API on `AGENT_PORT`
(default 9000): plain HTTP, HTTPS (`TLS_CERT_FILE` + `TLS_KEY_FILE`, reloaded
when the files change) or HTTPS requiring a client certificate
(`TLS_CLIENT_CA_FILE`), chosen per agent. Every call needs
`Authorization: Bearer $AGENT_TOKEN` (compared in constant time) in every
mode; the agent refuses to start without a token. The token (and an inline TLS
key) come through `SECRETS_BACKEND` — env, file or Vault — the same `secrets`
package as Orbit. It is a port of the Python agent with the same API and
JSON (the Python agent was removed on 2026-10-09).

Orbit follows the scheme of each agent URL. Its agent clients
(`proxies.newHTTPClient`) use `config.AgentTLS()`: the system CAs plus
`AGENT_CA_FILE`, and the client certificate from `AGENT_CLIENT_CERT_FILE` /
`AGENT_CLIENT_KEY_FILE` (or `AGENT_CLIENT_KEY` from the secrets backend),
sent when an agent asks. A TLS failure is reported with the setting that fixes
it. With **Agents must use HTTPS** on (`agents_https_only`), `agentFor`
refuses `http://` agents, the registry refuses `http://` URLs, and the setting
can't be turned on while one remains.

| Call | What it does |
|---|---|
| `GET /v1/state` | The state document: `engine`, `node`, `states` it supports, load balancer info, last reload / change, frontend stats, and `backends` with their servers, stats, path and config |
| `POST /v1/servers/<ready\|drain\|maint>` | Body `{"servers": [{"backend": …, "server": …}]}`. Applies all or nothing, returns the new state |
| `POST /v1/servers/<ready\|drain\|maint>/preview` | Same body; changes nothing, returns `changes`, `diff` (nginx) and `commands` |
| `POST /v1/backends/<backend>/servers/<server>/<ready\|drain\|maint>` | One server |
| `POST /v1/weights` (+ `/preview`) | Server weights, all or nothing |
| `POST /v1/check` | One server's health check, built from the load balancer's config, run from the proxy host |
| `POST /v1/stats/reset` | Clears the counters, returns the new state |

Errors come as `{"error": …}`: `401` bad token, `404` unknown server, `400` bad
request or a state the engine doesn't have, `409` the load balancer rejected the
change, `503` (preview) the load balancer's state can't be read.

`ENGINE` picks the driver:

| | nginx (`nginx.go`) | HAProxy (`haproxy.go`) |
|---|---|---|
| States | `ready`, `maint` (no drain) | `ready`, `drain`, `maint` |
| How | Adds / removes the `down` flag on `server` lines in the files matched by `UPSTREAM_FILES`, then `TEST_CMD` (`nginx -t`) and `RELOAD_CMD` | `set server <backend>/<server> state …` on the Runtime API socket (`HAPROXY_SOCKET`), then saves `show servers state` to the state file |
| All or nothing | One file edit + one reload; if nginx rejects it, the files are restored | Commands one by one; if one fails, the ones already applied are put back |
| Server id | The address (`web01:8001`); the label comes from a `# name=` comment | HAProxy's server name (`gmr_web01_facebook`) |
| Extras | Health checks from the proxy host, stats from the `lb` access log, reloads after hand edits (`AUTO_RELOAD=1`) | Health, sessions and stats from `show stat` |

Install and configuration: [AGENTS.md](AGENTS.md).

## Look and branding

**v0's look, not PKI's.** v0's colour tokens (light + dark: `--bg #eef0f3`,
`--surface #fff`, `--text #16191d`, `--accent #2463eb`, up / down / maint /
drain ramps), system font with mono uppercase labels, 13.5 px base, 7–8 px
radii, the 236 px sidebar (the default mark, sections, counts) with breadcrumbs
on top, and v0's sign-in card. From PKI only the six-box code input (see
AUTH-BLUEPRINT "Required behaviour").

**The crest.** The developer themes (from PKI, ported 2026-10-01) use the
**UBonito proxy crest** (`proxy-assets/proxy/ubonito/`, the blackletter "U" on
the proxy mark) in `web/public/dev-themes/proxy-ubonito-{blue,gold,red,dark,bone}.svg`
+ `-512.png`. The design files draw the "U" as `<text>` in UnifrakturCook;
`scripts/bake-crests.py` bakes it into a path and drops the C2PA manifest. The
wordmark and the theme colours come from `wordmark-assets/` (the same as
PKI's). The Konami egg is PKI's, unchanged. The design packs stay out of git.

**The default mark** is the neutral proxy crest from `proxy-assets/proxy/neutral/`
(dark variant): `web/public/favicon.svg`, `favicon-180.png`, `favicon-512.png`.
It replaced v0's "lb" tile and PKI's shield.

## Live updates

Pages update through **Server-Sent Events** on `GET /api/v1/auth/events`.
The browser's `EventSource` can't send an `Authorization` header, and a bearer
token must never go in a URL, so the SPA first asks for a **ticket**
(`POST /api/v1/auth/events/ticket`: single-use, 30 seconds, stored in the
ephemeral table) and opens the stream with `?ticket=…`. The stream sends a
heartbeat every 25 seconds and `X-Accel-Buffering: no` for nginx in front.

| Event | Sent to | The page then |
|---|---|---|
| `proxies` | everyone | re-fetches `/state` (its own filtered view) |
| `privileges` | the affected users | re-fetches `/auth/me` |
| `users`, `roles` | everyone | the admin lists re-fetch |

Events carry no data, only a type: each client re-fetches what it may see.
The SPA also re-fetches `/state` on the refresh interval, so a lost stream
(e.g. a proxy that buffers it) only makes the page slower, not wrong. On error
it reconnects after 5 seconds with a new ticket. Subscriptions live in the
process: after a restart, pages reconnect on their own.

## Audit chain

`audit.Recorder.Record` writes every mutating action, and every refused server
change. Writes are serialized by a mutex. Each entry stores
`hash = sha256(prev_hash ‖ canonical fields)`, where the fields are user,
impersonator, action, target, IP, detail and the timestamp.
`GET /api/v1/audit/verify` (Security → Audit → **Verify integrity**) walks the
chain in id order and reports the first entry whose link or content doesn't
match. An audit write failure is logged and never fails the operation it
describes.

With `SYSLOG_ADDR` set, each committed entry is also sent to syslog as JSON,
including its chain hash, so a SIEM holds an independent copy.
`orbit import-v0-audit <v0 orbit.db>` appends v0's history at the end of
the chain with the original times (once; `--force` to repeat).

## Background work

| What | When | Where |
|---|---|---|
| Proxy polling, then the automatic restore of drifted servers (when on) | every refresh interval | `proxies.Service.Run`, a goroutine started in `main.go` |
| End timed maintenance, finish drains, warn of expiring timers, then the API's scheduled changes (`Service.Tick`, `OnTick`) | every 5 s | a goroutine started by `proxies.Service.Run` |
| Update check (latest Gitea release, compared by semver) | at start and `@every 6h`, if `UPDATE_CHECK` is on | `jobs.Scheduler` |
| Delete expired API tokens | `@daily` | `jobs.Scheduler` |
| Delete expired ephemeral tokens | `@every 15m` | `jobs.Scheduler` |
| Certificates (flag `certificates`): ask every agent for its certificates, alert on expiry (threshold, 7, 1, 0 days; once each per fingerprint, remembered in `cert_warned`), renew the managed certificates that are due (a failure waits an hour) | every 10 min, on the 5 s tick (`certTick`, one run at a time) | `api.Handlers`, through `proxies.Service.OnTick` |
| The owner's automatic backup, when due (`internal/backup`: `VACUUM INTO`, gzip, sealed; a directory or S3) | `@every 1h` | `jobs.Scheduler` |

The scheduler is `robfig/cron` in the same process
(`api/internal/jobs/scheduler.go`). Timed maintenance runs with the poller
instead, at a 5-second tick, because it must be on time; scheduled changes
(Phase F) will join one of them.

## Start-up

`main.go` does, in order: load config (fail fast on a bad one: in release mode
`JWT_SECRET` and `ORBIT_MASTER_KEY` need 32+ characters), open SQLite, migrate,
open the keystore, set up auth and audit (and syslog), backfill the audit chain
for any rows without a hash, seed the built-in roles, create the first admin on
an empty database, make sure one **owner** exists, record the version, import
`PROXIES` once, start the poller, build the router, start the scheduler and
listen (HTTPS when `TLS_CERT_FILE` and `TLS_KEY_FILE` are both set). Every
step is safe to repeat on each start.

## Design decisions

**PKI Management's features, v0's design.** The platform (auth, audit, config,
jobs, events, image, CI, release flow) is ported from PKI Management, the same
author's Go + Vue project, minus everything about certificates and minus
client-certificate login. The look — colours, fonts, sidebar, sign-in card —
and the proxy pages' behaviour are v0's. From PKI's UI only the six-box code
input stayed. The blueprints ([PROJECT-BLUEPRINT.md](PROJECT-BLUEPRINT.md),
[AUTH-BLUEPRINT.md](AUTH-BLUEPRINT.md)) describe the reusable base (PKI's
design, generic patterns only — nothing about proxies);
[BLUEPRINT-FIT.md](BLUEPRINT-FIT.md) lists what was adopted, adapted and
skipped, and why.

**Sign-in inside Orbit.** v0 had a separate sign-in service (lb-idp). v1
does it itself, like PKI; Entra ID or Authentik stay possible as OIDC SSO.

**The agent in Go (2026-10-08).** It started as a Python script (stdlib only,
easy to audit). It is now one static binary in the same Go module: nothing to
install on the proxy (no Python version to match), the same secrets backends
as Orbit (env file or Vault), and TLS / mutual TLS from the standard
library. The API and JSON didn't change, so agents move one at a time; the
Python agent was removed on 2026-10-09 (it is in the git history).

**SQLite only.** Nothing here needs another engine yet, and an engine only
counts as supported once it's smoke-tested. `DB_DRIVER` accepts only `sqlite`.
The models stay portable (AutoMigrate, no dialect SQL), so Postgres is a driver
and a test away if it's ever needed.

**One admin instance.** The rate limiter, the SSE hub, the audit lock and the
proxy snapshots are per process. Single-use tokens are already in the shared
database, but running several replicas is not supported.

**Secrets sealed, tokens hashed.** Agent tokens, TOTP secrets and the OIDC
client secret are **sealed** with AES-256-GCM. The key comes from
`ORBIT_MASTER_KEY` through Argon2id over a random salt stored in the settings
table (`kek_salt`). Refresh tokens, API tokens, recovery codes and ephemeral
ids are only stored as sha256 hashes. That's why `ORBIT_MASTER_KEY` must never
change (see [TROUBLESHOOTING.md](TROUBLESHOOTING.md#data)).

**Scope rules on name parts.** Servers follow a naming scheme,
`<env>_<machine>_<service>`, so access is granted by rules on those parts (and
on proxy and backend) instead of lists of servers. New servers that follow the
scheme fall into the right scope without editing roles. The roles page shows
how many servers each rule matches right now.

**The admin polls, the page is pushed.** The admin polls agents on one
interval; pages get a push when something changed and a timer as a fallback.
That keeps the agents' load independent of how many pages are open.

**Checks in the admin, all-or-nothing in the agent.** The admin decides who
may change what; the agent is the only one that knows how to change its load
balancer safely, so it owns the rollback.

**Where v0 went.** The Python admin (`admin-host/`) and lb-idp (`idp/`) were
removed from the tree; they are kept at the git tag **`v0-python`**. lb-agent
came along unchanged into `agent/` (rewritten in Go on 2026-10-08 and removed
on 2026-10-09), the demo proxies into `lab/`.
