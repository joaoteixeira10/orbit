# Project Blueprint

How this project is built, run, tested, documented and released, written so a
**new project can start with the same structure**, in the same stack or another
language. The auth design has its own document:
[AUTH-BLUEPRINT.md](AUTH-BLUEPRINT.md). This one covers everything else.

Each section gives the **principle** (portable), then **how it's done here**
(the reference implementation, with file paths), and, where the choice is
language-specific, **equivalents** for other stacks.

Reference stack: Go 1.26 · Gin · GORM (SQLite/MySQL/PostgreSQL) · Vue 3 + Vite +
Pinia + Tailwind 3 · Docker · Gitea Actions. Snapshot 2026-09 (v2.1.x); consolidated
2026-10-14 from the copies in PKI Management, LB Admin and Orbit (the canonical copy lives in the `blueprint` folder next to the projects).

---

## 0. New-project checklist (do these in order)

1. **Repo skeleton** (§2): `api/` + `web/` + `docs/` + `scripts/` + `deploy/` + `.gitea/workflows/`,
   `Makefile`, `Dockerfile`, compose files, `.env.example`, `CHANGELOG.md`, `README.md`, `LICENSE`.
2. **Docker dev loop first** (§3): `docker compose -f docker-compose.dev.yml up` must give live
   reload of backend *and* frontend before any feature work. No host toolchains.
3. **Config from env + secrets provider** (§4), with release-mode fail-fast checks.
4. **Database** (§5): UUIDv7 primary keys, auto-migration, a key/value `settings` table,
   idempotent startup migrations, `make db-smoke` for every engine you claim to support.
5. **API skeleton** (§6): `/api/v1`, error shape, middleware stack (request id, logging,
   security headers, body limit, metrics), `/healthz` + `/readyz` + `/version`, a `healthcheck`
   subcommand, the SPA embedded in the binary, graceful shutdown.
6. **Auth**: copy [AUTH-BLUEPRINT.md](AUTH-BLUEPRINT.md) §15 (porting recipe).
7. **Owner + feature flags** (§7) and **update check / About** (§8).
8. **Frontend shell** (§10): design tokens as CSS variables, dark mode, i18n with a parity
   test, an API client, a router guard, stores, branding/theming (§9), and the phone drawer +
   installable manifest (§10.1), and the Apple-like look copied from Orbit (§10.2).
9. **Tests** (§11): unit + full-router integration on an in-memory DB, frontend unit, one
   Playwright smoke, DB smoke, security scans.
10. **CI** (§12): Test, E2E and Security run on every PR. Build & Push and Release are manual.
11. **Release machinery** (§13): `scripts/set-version.sh`, Keep a Changelog, release notes =
    changelog section + commit list, `docs/RELEASING.md` runbook.
12. **Docs** (§14): README structure, `docs/` per concern, env-var reference tables,
    runbooks, blueprints.

---

## 1. Principles

- **Everything runs in Docker.** Development (with live reload), tests, lint, codegen, builds
  and releases. Nothing needs Go/Node/etc. on the host. `make` targets wrap containers.
- **One artifact.** The backend binary embeds the built SPA and serves it, so production is
  one container and one port (plus a volume). The SPA and API share an origin, so no CORS
  is needed in production.
- **Config is environment variables** (12-factor). Secrets go through a pluggable provider
  (env / `*_FILE` / Vault). Release mode refuses weak or missing secrets.
- **Runtime policy lives in the database** (`settings` table), is editable in the UI and
  takes effect live. The env holds only what's needed before the DB is reachable, or what's
  deployment-shaped.
- **Secure by default.** Distroless non-root image, security headers, body limits, SSRF-guarded
  outbound HTTP, audit log, rate limits. New risky features ship **behind a feature flag, off**.
- **Idempotent startup.** Every boot runs migrations, seeds and backfills that are safe to repeat.
- **Releases are deliberate and manual**: bump → build → release, never automatic on merge.
- **Docs are part of the product.** Every feature lands with its docs and a CHANGELOG entry.
  Every config file starts with a comment explaining *why*.
- **Portable over clever.** No dialect-specific SQL, no CGO (pure-Go SQLite), cross-compiled
  multi-arch images (no QEMU).

---

## 2. Repository layout

```
.
├── api/                         # backend (Go module)
│   ├── cmd/server/main.go       # entrypoint + `healthcheck` subcommand + OpenAPI general annotations
│   ├── cmd/migrator/            # one-off data migration tool (separate Docker target)
│   ├── cmd/<cli>/               # optional: a stdlib CLI over the API (API tokens), in the image + releases
│   ├── cmd/<agent>/             # optional: a companion daemon on other hosts (same module, own binary)
│   ├── internal/<package>/      # one package per concern (see below)
│   ├── docs/                    # generated OpenAPI (committed on purpose)
│   ├── .air.toml                # live-reload config (dev)
│   └── .golangci.yml            # linter config
├── web/                         # SPA (Vue 3 + Vite)
│   ├── src/{api,components,components/ui,components/charts,i18n,lib,router,stores,styles,views}
│   ├── public/                  # favicons, manifest.webmanifest, theme assets, self-hosted fonts
│   ├── e2e/                     # Playwright specs
│   └── vite.config.ts           # dev proxy + polling + vitest config
├── docs/v2/                     # all long-form docs (see §14)
├── scripts/                     # set-version.sh, db-smoke.sh (+ db-smoke/compose.yml)
├── deploy/                      # monitoring stack: Prometheus + Grafana + alert rules
├── .gitea/workflows/            # test.yml e2e.yml security.yml build.yml release.yml
├── Dockerfile                   # multi-stage: web → api → [migrator] → runtime
├── docker-compose.yml           # production: pull the published image
├── docker-compose-build.yml     # production-like, built from source
├── docker-compose.dev.yml       # development: live reload of API + SPA
├── Makefile                     # every task, containerized
├── .env.example                 # every variable, grouped and commented
├── CHANGELOG.md  README.md  LICENSE (Apache-2.0)
```

Backend packages (`api/internal/`), one concern each. Service packages never import
`api` (HTTP), so they stay reusable and testable:

| Package | Role |
|---|---|
| `api` | HTTP layer: router, handlers (`handlers_<area>.go`), DTOs (`dto*.go`), middleware, validation helpers |
| `models` | ORM entities + `Base` (UUIDv7) + `AllModels()` (migration list) |
| `db` | open DB per driver, auto-migrate, key-encryption-key migration |
| `config`, `secrets` | env loading; secrets providers (env / file / vault) |
| `auth`, `rbac`, `ephemeral` | authentication, privileges, shared single-use tokens (see AUTH-BLUEPRINT) |
| `audit` | hash-chained audit log |
| `feature` | feature flags |
| `version` | build stamp + update checker |
| `jobs` | cron scheduler |
| `events` | SSE pub/sub for live UI updates |
| `notify` | email/Discord/Telegram/ntfy/Gotify + signed webhook |
| `metrics` | Prometheus domain metrics (imports nothing from the app, so it stays cycle-free) |
| `keystore` | AES-256-GCM sealing with an Argon2id-derived key |
| `backup` | encrypted export/import, scheduled backups (dir / S3) |
| `safedial` | SSRF-guarded HTTP client for all admin-configurable outbound calls |
| `webui` | `//go:embed` of the built SPA |
| domain packages | the project's own (PKI: `pki`, `ca`, `cert`, `ocsp`, `acme`) |

---

## 3. Docker everywhere

### 3.1 Development with live reload (`docker-compose.dev.yml`)
`docker compose -f docker-compose.dev.yml up`, then open `http://localhost:5173`.

| Service | Image | Command | Key points |
|---|---|---|---|
| `api` | `golang:1.26.9` | `go install github.com/air-verse/air@latest && exec air` | bind-mount `.:/src`; named volumes for build cache, module cache, `/go/bin` (keeps `air` between restarts) and `/data` (dev DB); dev env with insecure throwaway secrets and `CORS_ORIGINS=http://localhost:5173`; port 8080 |
| `web` | `node:22-alpine` | `npm install && npm run dev` | bind-mount `.:/src`; **`node_modules` in a named volume** so the container's Linux/musl native binaries (esbuild, rollup) never mix with the host's; `VITE_PROXY_TARGET=http://api:8080`; `VITE_POLL=1`; port 5173 |

**Why polling.** Bind mounts on Docker Desktop (macOS/Windows) don't deliver inotify events,
so without polling neither edits nor `git pull`s are noticed. Both watchers poll:
- `api/.air.toml`: `poll = true`, `poll_interval = 500`, `cmd = "go build -o ./tmp/server ./cmd/server"`,
  `exclude_dir = ["tmp","docs"]`, `exclude_regex = ["_test\\.go"]`, `send_interrupt = true`, `kill_delay = "2s"`.
- `web/vite.config.ts`: `server: { host: true, port: 5173, proxy: { '/api': { target: process.env.VITE_PROXY_TARGET || 'http://localhost:8080', changeOrigin: true } } }`,
  `watch: process.env.VITE_POLL ? { usePolling: true, interval: 300 } : undefined`.

The Vite proxy keeps `/api` same-origin in dev too, so cookies and CSRF behave as in
production. A gitignored `docker-compose.override.yml` can inject real local secrets
(`-f docker-compose.dev.yml -f docker-compose.override.yml`).

**Equivalents in other stacks:**

| Stack | Backend reload | Frontend |
|---|---|---|
| Go | `air` (poll) | Vite (`usePolling`) |
| Python | `uvicorn --reload` / `watchfiles` (set `WATCHFILES_FORCE_POLLING=true`) | same |
| Node/TS | `tsx watch` / `nodemon --legacy-watch` (`CHOKIDAR_USEPOLLING=1`) | same |
| Rust | `cargo watch -x run` (`--poll`) | same |
| .NET | `dotnet watch` (`DOTNET_USE_POLLING_FILE_WATCHER=1`) | same |
| Java/Kotlin | Spring DevTools / Gradle `--continuous` | same |

### 3.2 Toolchain in containers (`Makefile`)
Nothing on the host. The Makefile runs Go in a pinned container with named cache volumes:
```make
DOCKER_GO = docker run --rm -v "$(CURDIR)":/src \
  -v pki-go-cache:/root/.cache/go-build -v pki-go-modcache:/go/pkg/mod \
  -w /src/api -e GOFLAGS=-buildvcs=false golang:1.26.9
```
Targets: `go ARGS=…`, `tidy`, `build`, `test`, `vet`, `lint` (golangci-lint pinned, installed once
into a `/go/bin` volume), `fmt`, `swag`, `db-smoke DB=postgres|mysql`, `set-version VERSION=x.y.z`,
`image`, `release-image`, `pull`, `up`, `up-build`, `down`, `run`. The frontend checks run in the dev
compose's `web` service (`docker compose -f docker-compose.dev.yml run --rm --no-deps web npm test`).

> The `web-*` Make targets still call host `npm`. In a new project, route them through the
> `web` container too, so the whole Makefile is host-toolchain-free.

### 3.3 Production image (`Dockerfile`)
```
stage web      FROM --platform=$BUILDPLATFORM node:22-alpine   → npm build (arch-independent, never emulated)
stage api      FROM --platform=$BUILDPLATFORM golang:1.26.9   → COPY web dist → internal/webui/dist
               CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags "-s -w -X …version.Version=$VERSION …"
stage migrator FROM distroless/static:nonroot                  → optional: `docker build --target migrator`
final          FROM gcr.io/distroless/static-debian12:nonroot  → /pki-management binary, USER nonroot, VOLUME /data, EXPOSE 8080
               HEALTHCHECK CMD ["/pki-management","healthcheck"]
```
- **Multi-arch by cross-compiling** on the build platform (amd64 + arm64). No QEMU/binfmt needed.
  Pure-Go dependencies only (the SQLite driver is `glebarez/sqlite`).
- **Healthcheck subcommand**: distroless has no shell or curl, so the binary itself probes
  `http://127.0.0.1:<port>/api/v1/healthz` (3 s timeout, exit 0/1).
- Optional tools get their own `--target` placed *before* the final stage, so a plain
  `docker build` still yields the server image.
- `.dockerignore` excludes `.git` (so version/commit arrive as build args), `node_modules`,
  built dist, `*.db`, `.env`, root `docs/`.
- SPA embedding: `api/internal/webui/embed.go` has `//go:embed all:dist`. A placeholder
  `dist/index.html` is committed so the package compiles before any frontend build. The
  router's `NoRoute` sends `/api/*` → JSON 404, an existing file → static, anything else →
  `index.html` (history-mode fallback).

### 3.4 Compose files
| File | Purpose |
|---|---|
| `docker-compose.yml` | production: pulls `${REGISTRY}/…:${PKI_VERSION:-latest}`, `env_file: .env`, required secrets as `${JWT_SECRET:?set JWT_SECRET in .env}`, named data volume, `restart: unless-stopped`. TLS is terminated upstream (HAProxy/NPM). |
| `docker-compose-build.yml` | same, but `build:` from source with VERSION/COMMIT/BUILD_TIME args; **shares the data volume name**, so switching between pulled and built keeps the data |
| `docker-compose.dev.yml` | §3.1 |
| `scripts/db-smoke/compose.yml` | Postgres/MySQL under profiles + API from source (§11.5) |
| `deploy/docker-compose.monitoring.yml` | Prometheus + Grafana with provisioning and alert rules |

---

## 4. Configuration & secrets

- `api/internal/config/config.go` is the **source of truth**. Helpers are `env(key, def)`,
  `envInt`, `envBool` and `splitNonEmpty`, where an empty value means use the default.
  There are no config files.
- **Secrets provider** (`api/internal/secrets`): `Provider{Get(key) (string, error)}`.
  - `SECRETS_BACKEND=env` (default), `file` (reads `<KEY>_FILE`, the Docker/K8s secrets convention),
    or `vault` (KV v2, token or AppRole, read once at startup).
  - A missing value returns `""`. A backend error aborts startup.
  - Only real secrets go through it: JWT secret, master key, DB URL, admin password, S3 keys,
    and the project's own (integration tokens, TLS private keys).
  - **Env file or Vault — both stay.** `env` (an `.env` file / compose `env_file`) is the default
    and needs nothing else; `vault` is opt-in per instance. Vault settings: `VAULT_ADDR`,
    `VAULT_KV_MOUNT` (default `secret`), `VAULT_KV_PATH` (the KV v2 secret holding the keys, by
    their env names), and `VAULT_TOKEN` **or** AppRole `VAULT_ROLE_ID` + `VAULT_SECRET_ID`. The
    secret is read **once at startup**; a key missing from it falls back to the environment, so
    non-secret config keeps coming from env. Rotating a secret in Vault = restart the service.
  - **Every binary of the project uses the same provider** (the server, a companion agent or
    daemon, migration tools), so an operator configures secrets one way everywhere. A secret
    that is a file on disk (a TLS key) can also be given inline as `<KEY>` through the provider,
    with `<KEY>_FILE` / a path variable kept for the plain case.
- **TLS material from files reloads itself** (`internal/tlsfiles`): a server or client certificate
  loaded from files is loaded again when their mtime / size change (checked per handshake; a
  half-written renewal keeps the old one), so renewing a certificate never needs a restart. CA
  bundles add to the system pool (`*_CA_FILE`). Outbound TLS errors are reported with the
  setting that fixes them (unknown CA → the CA-file variable, client certificate wanted → the
  client-cert variables).
- **Release-mode strictness**: `GIN_MODE=release` requires secrets ≥ 32 chars, or startup fails.
  Dev mode uses obvious `dev-insecure-…-change-me` fallbacks so the dev loop needs zero config.
- `.env.example` lists **every** variable, grouped (Deployment image, Observability & throttling,
  Update check, HTTP/TLS, Database, Secrets, Bootstrap admin, CORS, WebAuthn, mTLS) and commented.
- `docs/v2/CONFIGURATION.md` is the reference table: `| Variable | Req | Default | Notes |`, with a
  legend for *Req* (`–`, `release`, `1st run`, `if X`).

---

## 5. Data layer

### 5.1 UUIDv7 primary keys
Every entity embeds:
```go
// UUIDv7 is time-ordered and lexically sortable, so ORDER BY id keeps insertion
// order (newest-first lists, owner backfill, and the audit hash chain rely on it).
type Base struct {
    ID        string    `gorm:"primaryKey;size:36" json:"id"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
func (b *Base) BeforeCreate(*gorm.DB) error {
    if b.ID == "" { id, err := uuid.NewV7(); if err != nil { return err }; b.ID = id.String() }
    return nil
}
```
- **Why v7 over v4**: it keeps `ORDER BY id` = insertion order (sorting, pagination, the audit
  chain), and still removes enumerable sequential ids from the API and from any URL you embed.
- IDs are **strings everywhere**: `size:36` FKs, `*string` for nullable FKs, `string` in the
  frontend types. Never `Number()` an id.
- Callers may **preset** an id; the hook fills only an empty one. Restore and migration rely on that.
- Natural-key tables skip `Base`: `settings` (PK `key`), nonces, `ephemeral_tokens` (PK hash).
- `models/base_test.go` asserts the hook stamps v7 and `Order("id")` equals creation order.
- **Monotonicity**: v7 isn't strictly monotonic within one millisecond. When order matters in a
  bulk import, bump the previous value (the migrator does this).
- Validate the UUID format at the HTTP edge. Here `idParam` only rejects empty, and unknown ids fall
  through to a 404. Adding a format check is cheap.

**Generating UUIDv7 elsewhere:** Python 3.14 `uuid.uuid7()` (or `uuid6`/`uuid-utils` before 3.14),
Node `uuid` package `v7()`, Rust `uuid` crate `Uuid::now_v7()`, .NET 9 `Guid.CreateVersion7()`,
Java `uuid-creator`, PostgreSQL 18 `uuidv7()`. Store as `uuid` natively on Postgres, `CHAR(36)`/
`BINARY(16)` on MySQL, text on SQLite.

### 5.2 Schema & startup migrations
- **Schema**: ORM auto-migrate over one ordered list (`models.AllModels()`), which is additive-only.
  (For stricter environments, swap in versioned migrations such as goose, Alembic or Flyway. Keep the
  "startup runs everything idempotently" property.)
- **Startup sequence** (`cmd/server/main.go`): `healthcheck` subcommand? → load config → logger →
  open DB → auto-migrate → key-encryption migration → services → idempotent data migrations and
  seeds (`MigrateSettingsPrivilege`, `SeedBuiltins`, `Bootstrap` admin, `MigrateUserRoles`,
  `EnsureOwner`, audit `BackfillChain`) → `recordVersionHistory` → router → scheduler → listeners →
  wait for SIGINT/SIGTERM → `Shutdown(10s)`.
- Server timeouts: `ReadHeaderTimeout 10s`, `ReadTimeout 30s`, `IdleTimeout 120s`, **no
  `WriteTimeout`** (SSE streams are long-lived).
- **One-off data migrations** (e.g. int ids → UUIDv7) are a separate tool
  (`cmd/migrator`, its own Docker target). It writes a **new** database (never in place), in one
  transaction, remaps every FK, supports `-dry-run`, and verifies invariants before committing.

### 5.3 Settings key/value store
`settings(key PK, value TEXT /*JSON*/, updated_at)`. Each feature owns a constant key and a typed
`Load()`/`Save()` pair. Values are read live on each use, so UI changes apply without a restart.
Upsert with `Save`. Query portably with `Where(&Setting{Key: k})`. Keys in use include
`features`, `oidc_config`, `certlogin_config`, `notification_config`, `security_alerts`,
`scheduled_backup`, `version_history`, `kek_salt`, the policy booleans and the branding keys.

**Write-only secrets in settings**: GET returns a `********` placeholder plus `has_secret`. PUT with the
placeholder or an empty value keeps the stored secret (`Merge`). Seal secrets you must read back
(AES-GCM under the master key). Here the OIDC secret is sealed but the **notification secrets are
only masked, not sealed**. Seal them too in a new project.

### 5.4 Portability rules (all learned the hard way here)
- No dialect-only quoting (`` `key` ``) and no reserved words as names or aliases (`key`, `blob`, `order`, `user`).
- No hard-coded column types for binary data (`type:blob` breaks Postgres). Let the ORM pick,
  and give indexed byte columns a size.
- ORM column-name mangling (`OIDCSubject` → `o_id_c_subject`): use struct conditions, not strings.
- Unit tests on SQLite prove nothing about other engines. Run **`make db-smoke`** (§11.5) for each
  engine you claim to support, and ideally in CI.

---

## 6. Backend patterns

### 6.1 API conventions
- Everything under **`/api/v1`**. Public: `healthz`, `readyz`, `version`, `branding`, and auth entry
  points. Everything else sits in an `authed` group. Protocol endpoints that must be public (here
  OCSP/CRL/ACME) live at the root behind their own rate limiter.
- **Error shape is always `{"error": "<message>"}`**. `internalError(c, err, msg)` logs the real
  error with the path and returns only `msg`.
- Validation: framework binding tags (`required`, `min`, `oneof`, `email`) plus small helpers
  (`validHexColor`, `validImageDataURL`, `sanitizeFilename`). **Partial updates use pointer
  fields**, so an absent field means unchanged.
- **Pagination**: `?page=&per_page=` (1-based, clamped). Response `{items,total,page,per_page}`.
- Global privileges are checked by route middleware. Per-resource scope is checked in the handler
  on the loaded row.
- A security-sensitive "machine" error code (`password_change_required`, `step_up_required`)
  lets the SPA react generically.
- OpenAPI via annotations (swaggo) on every handler. `make swag` regenerates, the spec is
  committed and served at `/api/v1/openapi.json`. The reference page is **Scalar** at
  `/api/docs` (not Swagger UI): its standalone script is bundled with the SPA build and served
  from the same origin (`/scalar/standalone.js`), so the CSP stays `script-src 'self'` and
  nothing loads from a CDN; external fonts and Scalar's hosted features (AI agent, MCP, the
  developer toolbar) are off, and it uses the app's colours.
  `/api/v1/swagger/*` redirects to `/api/docs`. **Never Swagger UI.** "swag" / `swagger.json`
  are only the generator's name and output (a Swagger 2.0 document, which Scalar reads).
  *Variant (PKI Management):* a second Vite entry (`web/api-docs.html`) at `/api/v1/docs`, so
  Scalar's styles never touch the SPA; same rules (bundled, strict CSP, hosted features off).

### 6.2 Middleware stack (order matters)
`Recovery → requestID (honour inbound X-Request-Id) → requestLogger (method, path, status, latency,
ip, request_id) → securityHeaders (CSP, nosniff, DENY, no-referrer, COOP) → limitBody (4 MiB,
exemptions) → metrics`, then CORS **only if `CORS_ORIGINS` is set**. `SetTrustedProxies(TRUSTED_PROXIES)`
(empty = trust none) so `ClientIP()` can't be spoofed. The auth endpoints have their own per-IP limiter.

### 6.3 Observability
- **Logs**: structured JSON in release (zap production), console in dev. Every request log carries
  `request_id`. Apps produce logs and the platform ships them (`docs/v2/LOGGING-elk.md`: Filebeat or Vector).
- **Metrics**: Prometheus at `/metrics` (root, **off by default**, `METRICS_ENABLED`). HTTP
  `…_http_requests_total{method,route,status}` and `…_duration_seconds` use `c.FullPath()` for
  bounded cardinality. Domain counters and gauges are refreshed by the scheduler every minute.
  `deploy/` ships a Grafana dashboard and alert rules (instance down, 5xx rate, p95 latency,
  domain alerts).
- **Health**: `/healthz` = liveness (always OK). `/readyz` = DB ping (503 when down). `/version` =
  name, api, version, commit. The Docker `HEALTHCHECK` uses the `healthcheck` subcommand.

### 6.4 Background jobs (`internal/jobs`, robfig/cron)
One scheduler registers every job, and the jobs that need it are primed once at startup
(`go job()`). Here: CRL refresh (1h), expiry scan (daily 09:00), update check (6h), API-token sweep
(daily), ACME cleanup (1h), ephemeral-token sweep (15m), metrics refresh (1m), scheduled backup (1h,
self-throttled). `Stop()` waits for running jobs.

### 6.5 Audit log (tamper-evident)
- One row per mutating action: `user_id`, `impersonator_id`, `action` (`area.verb`, e.g.
  `user.password_reset`), `target_type/id`, `ip`, JSON `detail`.
- Hash chain: `hash = sha256(prev_hash ‖ 0x1e ‖ canonical(entry))`, where the canonical form joins
  fields with 0x1f and a UTC RFC3339Nano timestamp. Inserts are serialized.
- `GET /audit/verify` walks the chain and reports the first broken row. `BackfillChain` at startup
  is idempotent.
- An audit failure is logged, never fails the user action.
- Handlers call `recordCtx(c, …)`, which attributes impersonated actions to the real admin.
- The insert lock is in-process, so with several replicas use a DB lock/sequence.

### 6.6 Live UI updates (SSE)
The client gets a 30 s single-use **ticket** (`POST /auth/events/ticket`), then opens
`EventSource('/api/v1/auth/events?ticket=…')` (EventSource can't send headers, and tokens never go in
URLs). The server sends `: connected`, a `: ping` every 25 s, and `data: {"type":"…"}` events
(`privileges`, `cas`, `certificates`, `roles`, `users`). Publishing never blocks: a full buffer drops
the message. The client reconnects after 5 s with a fresh ticket and re-fetches on each event type.
Fan-out is per process.

### 6.7 Notifications, alerts, webhooks
- Channels: email (SMTP), Discord, Telegram, ntfy, Gotify, configured in the UI.
- The generic **integration webhook** is signed: `X-PKI-Signature: sha256=<HMAC(secret, body)>`,
  plus `X-PKI-Event`.
- **Owner security alerts**: a catalog of high-signal events, all opt-in, each dispatched async
  and never blocking the action.
- All outbound URLs go through `safedial`. It blocks loopback, link-local and metadata IPs, checks
  the IP actually dialed (defeats DNS rebinding), never follows redirects, and deliberately allows
  private LAN ranges (homelab targets).

### 6.8 Backup (`internal/backup`, Owner → Owner → Backups)
**What:** the whole SQLite database (`VACUUM INTO` a temp file: a consistent snapshot of the live
database), gzipped, sealed with AES-256-GCM in a JSON envelope (`format: <app>-backup`, v1,
app version, time, KDF salt, ciphertext). Two modes:
- **Master-key** — the key is derived from the master key (`<APP>_MASTER_KEY`) with the instance's KEK salt, which
  travels in the envelope; restoring needs the same master key. Used by automatic backups.
- **Passphrase** — Argon2id from a passphrase of 12+ characters (its own salt); to keep the archive
  apart from the key. Secrets inside (TOTP, SSO and notification secrets, integration tokens) stay sealed
  with the master key: restored under another key, the app starts but those must be set again.

**Download now:** `POST /backup/export` (owner, step-up when on, audited, optional security alert).

**Automatic backups** (owner, `PUT /backup/schedule`; checked `@every 1h` by `jobs.Scheduler`, written
when `interval_hours` has passed since the last good one; a failure is retried the next hour, and the
card shows the last run — when, where, or the error; **Run now** writes one at once to test the
destination). Files are `<app>-backup-masterkey-<UTC yyyymmdd-hhmmss>.json`; `keep` N keeps the N
newest (0 = all), pruned after each run. Two destinations:
- **Directory** (`dest: dir`): an absolute path inside the container, e.g. `/backups`; mount a volume
  there (`./backups:/backups`) writable by the image's user 65532. Files are written `0600`, the
  directory created `0700`.
- **S3 / S3-compatible** (`dest: s3`, minio-go): bucket, prefix (key = `<prefix>/<file>`), endpoint
  (`host[:port]`, empty = AWS) and region; TLS always. The keys come only from the secrets backend
  (`BACKUP_S3_ACCESS_KEY`, `BACKUP_S3_SECRET_KEY` via env, file or vault), never from the page or the
  database. Pruning lists the prefix and removes the oldest beyond `keep`.

**Restore:** `<app> restore-backup <archive> [--force]`, **with the app stopped** — never an upload
into a running instance. It decrypts with the master key or `BACKUP_PASSPHRASE` /
`BACKUP_PASSPHRASE_FILE`, checks it really is a SQLite file, refuses an existing database unless
`--force`, then keeps it as `<db>.before-restore-<time>` (and its `-wal` / `-shm`) and writes the new
one atomically. Tested end to end: an export restored and booted on (two-factor sign-in, a sealed
secret, the audit chain).

---

## 7. The instance owner & feature flags

### 7.1 Owner ("god mode")
- **Exactly one** user has `owner = true`. It's an **identity, not a privilege**: RBAC can't grant
  it, so no admin can self-escalate into it.
- The bootstrap admin becomes the owner. `EnsureOwner` backfills older installs (earliest superuser,
  else earliest user).
- Transfer (`POST /owner/transfer`) clears and sets in one transaction and needs step-up. It is
  audited and alerted.
- Owner-only: feature flags, security policy, SSO/cert-login config, backup/restore, root-key
  export, security alerts. **Owner-gated privileges** (branding, notifications, impersonate) are not
  implied by the superuser flag, but the owner may delegate them through a custom role.
- The owner can't be impersonated. Only the owner may impersonate or create superusers.
- UI: a separate **Owner** area (`/owner`, crown icon) shows the owner-only tabs.

**Naming.** "Owner" is well understood (Discord server owner, GitHub organization owner) and
signals "one, transferable, above admins", so it's a fine default for a generic app. For this PKI
the more precise term is **Custodian**: a *key custodian* is the established PKI role accountable
for root keys, backups and key ceremonies, which is exactly this role's power set. Avoid
"Superadmin" (sounds like a grantable role) and "Root" (sounds like a login account). If you
rename, change only UI labels and docs. Keep `owner` in code, API and DB to avoid a migration.

### 7.2 Feature flags (`internal/feature`)
- A `Defaults` map in code, **off** unless a flag says otherwise. Stored values are one JSON map in `settings.features`
  merged over the defaults, so **unknown stored keys are ignored and new flags appear (off)
  without a migration**.
- `GET /features` is open to any authenticated user (the SPA gates UI; the flags also arrive in
  `/auth/me`). `PUT` is owner + step-up and rejects unknown names.
- When a flag is off its routes return **404** (the instance behaves exactly as before), or its
  middleware becomes a no-op.
- The owner UI lists the flags with a label and a hint, toggles optimistically and reverts on error.
- Use flags for risky or behaviour-changing features (sessions UI, impersonation, step-up), not
  for permanent config.
- A flag can switch off a **whole feature area** the owner may not want (e.g. a module that
  writes to remote systems): routes 404, the menu entry and pages disappear, background jobs for
  it skip, and outbound calls for it are refused. Default **on** is allowed for an area that
  shipped on (say so in the flag's hint); everything new and risky defaults off.

---

## 8. Version, update check & About

- **Build stamp** (`internal/version`): `Version`, `Commit` and `BuildTime` are set by `-ldflags -X`
  from Docker build args. `Version` has a committed fallback, bumped by `set-version.sh`.
  The bare version is stamped (`2.1.0`); tags and images use `v2.1.0`; the UI adds the `v`
  (`withV()`), avoiding `vv`.
- **Update checker**: default repo URL constant, overridable with `REPO_URL`, disabled with
  `UPDATE_CHECK=false`.
  - It derives the Gitea API `…/api/v1/repos/<owner>/<repo>/releases/latest`. GitHub is the same
    idea at `api.github.com/repos/<o>/<r>/releases/latest`.
  - It polls every 6 h and once at startup, through the SSRF-guarded client (10 s).
  - A non-2xx is a silent no-op (no releases yet). The latest tag is cached in memory.
  - `UpdateAvailable = latest != current`. That's **not semver ordering**; a new project should
    compare properly, so a dev build newer than the release isn't flagged.
- **Version history**: at every boot, append `{version, installed_at}` to `settings.version_history`
  when the version changed.
- **`GET /about`** (authed) returns `{version, go_version, commit, build_time, repository_url,
  db_driver, latest_version, update_available, update_check_enabled, version_history[]}`.
  `GET /version` (public) returns `{name, api, version, commit}`.
- **UI** (`components/ServerStatus.vue` in the sidebar footer) shows a status dot. Red means the server
  is offline, grey that updates are off or there is no release info, amber that an update is available,
  green that it's up to date. Clicking opens About: version, repo link, "Changes" → `/releases`, DB
  driver, and version history (3 entries + "show more").

---

## 9. Branding & developer themes

### 9.1 Org branding (owner, or a holder of `settings.branding`)
Settings keys: `app_name`, `accent_color` (`#rrggbb`), `logo_data_url`, `favicon_data_url` (validated
image data URLs), `show_app_name`, `show_logo`, `login_message`. `GET /branding` is **public**, so the
SPA themes itself before login and also learns the login-page capabilities. `PUT /settings` is a
pointer-field partial patch, audited.

### 9.2 Developer themes (curated presets)
**Every project has them**, with the same four themes (Steel Blue, Gold Leaf, Blood Red, Ink), the
same settings and the same Branding UI; only the crest changes per project. **The code is one shared
kit** — `devThemes.ts`, the Branding card (`DevThemesCard.vue`), `dev-themes.css`, the wordmark font,
the Konami egg and `bake-crests.py` — kept in the blueprint folder (`shared/dev-themes/`, see its
README) and copied into each project with `sync-themes.sh`; never edit a project's copy. Crests are
named the same everywhere: `web/public/dev-themes/crest-<ink>.svg` + `crest-<ink>-512.png`.

**The icons are the project owner's, never the implementer's.** Crests, favicons, marks and app
icons are designed and delivered by the owner as a design pack; whoever builds the project (a
person or an AI agent) must not draw, generate, redraw, recolour, crop or "improve" an icon, nor
invent one when it's missing — ask the owner. The only changes allowed are mechanical: bake text
into a path, strip metadata, copy the files into `web/public`. The default app icon (`favicon.svg`
and its PNGs, §9.3) follows the same rule.

- **The design pack**: one SVG per ink, `<…>-{blue,gold,red,dark,bone}.svg`, each with a
  `-512.png` (more sizes may come with it), plus the designer's preview page. It stays **out of
  git**: a sibling folder next to the repo (`../<project>_assets/`, e.g. `../orbit_assets/orbit/`)
  or a gitignored folder inside it (LB Admin: `proxy-assets/`). Only the files the app uses go in
  `web/public/dev-themes/`.
- **Baking**: text in a design file (the blackletter "U") must be **baked into a path**
  (`make crests` → `scripts/bake-crests.py <pack>`: fontTools in a Python container, reading the self-hosted font; it
  also drops the C2PA manifest, no longer valid once the file changes), so the favicon and `<img>`
  show the real glyph without the font. The PNGs are copied as delivered. A new pack from the
  owner = run the script again and commit the output.
- Ink: the `dark` crest in the dark UI, `bone` in the light UI.

- `developer_themes_enabled` + `developer_theme ∈ {blue, gold, red, ink}` (clamped server-side)
  + `asset_source_{logo,wordmark,favicon} ∈ {theme, keep}`.
- A theme **locks the accent** and supplies a matching mark, wordmark and favicon. Per asset, the
  owner keeps the theme's or their own.
- `web/src/lib/devThemes.ts` defines each theme: `accent`, `onAccent`, `accentStrong`, optional light-UI
  variants (Ink is the only theme that changes between light and dark), `mark`/`halo` colours for the
  wordmark, and `crest` (SVG) + `crestPng` (512 px, because iOS home-screen and PWA icons refuse SVG).
- Resolvers: `resolveThemeAccent(t, isDark)`, `resolveCrest`, `resolveCrestPng`.
- Assets in `web/public/dev-themes/` are **font-independent** SVGs, with the glyph baked as a path,
  because a favicon paints before `@font-face` loads. The wordmark font is self-hosted
  (`web/public/fonts/`).
- **Text on the accent** always uses `--accent-fg` (a Tailwind `text-accent-fg`), never a hard-coded
  white: Ink's accent in the dark UI is bone, and white on it is unreadable (Orbit found this in its
  selected sidebar tile, menus, chips and checkboxes).
- The **Konami egg** (`KonamiEgg.vue`, ↑↑↓↓←→←→BA) comes with the themes: the "Engineered by"
  card, the same in every project.
- Admin UI: theme cards, a master toggle, per-asset segmented controls, the accent picker locked with a note
  while a theme is on, and a **live preview** that scopes the CSS variables onto real components
  (browser-tab strip, sidebar brand, `.btn-primary`), so it renders exactly like the saved result.

### 9.3 Resolution order (`web/src/stores/branding.ts`)
- **Accent**: personal accent (`UserPrefs.accent_color`) → developer theme (light/dark variant) →
  org accent → CSS default. `applyAccent()` writes `--accent`, `--accent-strong`, `--accent-glow`,
  `--accent-soft`, `--accent-bg`, `--accent-text` and `--accent-fg` inline on `<html>`, derived with
  `color-mix()`, or removes them to fall back to the stylesheet tokens.
- **Logo / wordmark / favicon**: developer theme (if its source is `theme`) → org upload → the default
  mark. A personal accent never changes assets.
- **The default mark** is `web/public/favicon.svg` (also the sidebar and sign-in mark — bind it
  to a constant such as `DEFAULT_MARK` so Vite serves the file instead of inlining it),
  `favicon-180.png` (apple-touch), and `favicon-192.png` + `favicon-512.png` (install manifest).
  Only **one** `rel=icon` link: the branding store repoints it, and a second (e.g. a PNG fallback)
  would win over a theme's favicon.
- `applyFavicon()` swaps `<link rel=icon>`, `apple-touch-icon` and `<meta name=theme-color>`, and generates a
  **web manifest as a Blob URL** that replaces the static one (§10.1). Every URL in it is
  **absolute** (`new URL(path, location.origin)`): inside a `blob:` manifest a relative `/` doesn't
  resolve to the site, and Chrome drops `start_url` and the icons. Toggling light/dark re-applies both functions.

---

## 10. Frontend architecture

- **Stack**: Vue 3 (Composition API) + TypeScript strict + Vite + Pinia + vue-router + vue-i18n +
  Tailwind 3 + lucide icons + the system font stack (no web fonts; the look is §10.2).
  Lint: ESLint flat config (correctness only). **No Prettier on purpose**, to avoid reformatting churn.
  Typecheck: `vue-tsc --noEmit`.
- **Folders**: `api/` (typed endpoint groups + `types.ts`), `lib/` (`api.ts` fetch wrapper, `live.ts`
  SSE, `format.ts`, `palette.ts`, `devThemes.ts`, `highlight.ts`), `stores/` (`auth`, `branding`,
  `prefs`, `toast`), `components/ui/` (Badge, Checkbox, CodeInput, Confirm, CopyButton, Overlay,
  Toggle, Toasts, StepUpModal, …), `components/charts/`, `views/` (one per top-level page), `router/`.
- **API client** (`lib/api.ts`): base `/api/v1`, an `ApiError(status, message)` from `body.error`,
  401 → refresh → retry, 403 `step_up_required` → modal → retry. Details in AUTH-BLUEPRINT §12.
- **Boot** (`main.ts`): apply the cached theme first (no flash), then
  `await Promise.all([branding.load(), auth.restore()])`, then mount. The router guard sees the real state.
- **Design tokens**: all colours are CSS variables in `styles/main.css` (`:root` light,
  `html.dark` dark), and Tailwind colours map to them (`bg`, `surface(-2/-3)`, `text(-2/-3)`,
  `border`, `accent*`, `ok*`, `warn*`, `crit*`). `darkMode: 'class'`. Component classes (`.card`, `.btn-primary`,
  `.input`, …) live in `@layer components`. Charts read the computed variables (`usePalette()`)
  because SVG attributes can't use `var()`.
- **Prefs**: theme (`light|dark|system`), timezone, layout and accent are stored **server-side**
  (`/me/prefs`, per user, following them across devices) and cached in `localStorage` for first paint.
  They hydrate from `/auth/me`. Setters update locally, then persist best-effort.
- **i18n**: `vue-i18n` (composition). The locale is stored in `localStorage` (`pki_locale`) and
  detected from `navigator.language`, falling back to `en`. It sets `<html lang>`. `const pt: typeof en`
  gives compile-time key parity, and **`messages.spec.ts` asserts every locale has exactly the English
  key set with no empty values**. Migrate inline strings incrementally with an `en` fallback.
- **Gating**: UI checks mirror the server (`can`, `canOnCA`, `canOwnerGated`, `features.*`,
  `isOwner`) **for display only**. The server is authoritative.
- **Screens every project has** (as in PKI Management):
  - **Settings** with a left menu. Personal: Account (photo, display name, email),
    Appearance (theme light / dark / system, personal accent), Language & time,
    Notifications. Security: Password, Two-factor (TOTP status, recovery codes left,
    Regenerate, Disable), API tokens, Active sessions, and the project's own options.
  - **Admin** tabs: Users, Roles, Branding, Notifications, Security, SSO, plus the project's own.
  - **Owner** tab: Feature flags (active sessions, user impersonation, step-up, and the
    project's own), Security alerts (impersonation started, ownership transferred, admin
    role granted, backup exported, …), Backups, then a full-width red **Danger
    zone** at the bottom with Transfer ownership, confirmed by typing the new owner's name.
  - **Refresh interval** (Owner → Live updates) when the app polls something outside: the
    owner sets it at runtime (no restart) over an env default; pages also refresh at once when
    a change is pushed (SSE).
  - Pages sit **left-aligned at full width**, not in a centered column; forms
    that read badly when wide (Security, SSO) keep a max width, still left-aligned.
  - Details and the auth rules behind them: AUTH-BLUEPRINT "Required behaviour".

### 10.1 Mobile and installable (PWA)

Every project works on a phone and can be installed to the home screen (iOS "Add to Home
Screen", Android / desktop Chrome "Install"). Features as in PKI Management, with the look of
§10.2.

- **Installable, not offline.** No service worker: an admin console shows live state, and a
  cached page would show stale state as if it were current. Chrome no longer needs one to install.
- **`index.html`**: `viewport` with `viewport-fit=cover`; `theme-color`; `mobile-web-app-capable`
  and `apple-mobile-web-app-capable`; `apple-mobile-web-app-title`;
  `apple-mobile-web-app-status-bar-style`; `apple-touch-icon` → `favicon-180.png` (iOS refuses
  SVG there); `<link rel="manifest" href="/manifest.webmanifest">`.
- **Static `web/public/manifest.webmanifest`**, valid before the SPA runs: `id`, `name`,
  `short_name`, `start_url` and `scope` `/`, `display: standalone`, `background_color`,
  `theme_color`, PNG icons 192 and 512. The branding store then swaps in the branded blob
  manifest (§9.3). Register `mime.AddExtensionType(".webmanifest", "application/manifest+json")`
  before serving the SPA: Go's built-in table lacks it and only finds it in the host's
  `/etc/mime.types`, so on a host without that file it goes out as `text/plain`.
- **CSP `manifest-src 'self' blob:`**, or the browser blocks the blob manifest and, in
  production only (the Vite dev server sends no CSP), the app can't be installed. Assert it
  in the security-headers test.
- **The shell below 1024 px**: the sidebar becomes a drawer (fixed, the desktop sidebar width capped at
  `85vw`, full height, slides in from the left) behind a menu button at the left of the top bar, over a
  dimmed backdrop. It keeps everything the desktop sidebar has. It closes on navigation (watch
  `route.fullPath`), a backdrop tap and Esc, and when the window grows past the breakpoint; it
  is `inert` while closed and the page behind it doesn't scroll while open. Below 640 px the
  top bar drops secondary text (timestamps, the user's name) and keeps the actions.
- **Safe areas**: with `viewport-fit=cover`, the shell pads with
  `max(<normal>, env(safe-area-inset-*))` (top bar, content, drawer, floating buttons), so an
  installed app clears the notch and the home bar. The insets are 0 everywhere else.
- **Pages**: wide tables scroll sideways inside their card (`overflow-x: auto` + a `min-width`)
  rather than becoming cards; multi-column grids collapse to one column; dialogs are
  `width: min(<size>, 100%)`. Respect `prefers-reduced-motion` for the drawer.
- **Test it** against a production build (the CSP matters): Chromium's
  `Page.getAppManifest` (no errors, absolute `start_url` and icons) and
  `Page.getInstallabilityErrors` (empty) over CDP, plus an E2E at a phone viewport that opens
  the drawer, navigates and sees it close.


### 10.2 The look: Apple-like (every project)

Every project looks and feels like an Apple app — macOS on a laptop, iOS on a phone — in light
and dark. **Reference implementation: Orbit** (`../orbit`: `web/src/styles/main.css`,
`components/AppShell.vue`, `components/ui/`). Copy its stylesheet and UI components rather than
redrawing them; a project changes the accent, not the language. (PKI Management and LB Admin
predate this rule and keep their own look until they are moved over; see `fit/`.)

**Principles**
- Content first, chrome quiet: hairline separators instead of borders, soft layered shadows,
  translucent materials, generous white space, one accent colour.
- Apple's system components, not lookalikes of other design systems: grouped lists, sheets,
  segmented controls, switches, round checkboxes, icon tiles, large titles, popover menus.
- Native behaviour: the system font, system colours that adapt to light / dark, 44 px touch
  targets, `Esc` closes, `⌘K` / `Ctrl K` searches, reduced motion respected.

**Tokens** (CSS variables, light in `:root`, dark in `html.dark`; Tailwind maps to them, §10)
- **Type**: the system stack — `-apple-system, BlinkMacSystemFont, "SF Pro Text"…` (`--font-sans`),
  `"SF Pro Display"` for titles (`--font-display`), `ui-monospace, "SF Mono"…` (`--font-mono`). No
  web fonts (the one exception: a developer theme's wordmark, §9.2). Scale: large title 30/700,
  title 2 22/700, title 3 17/600, body 14–15, subhead 13, footnote 12, caps label 11/600; titles
  get a slight negative letter-spacing.
- **Colour**: Apple's system colours with their light and dark values — blue `#007AFF` /
  `#0A84FF` (default accent), green `#34C759` / `#30D158` (ok, switches on), orange `#FF9500`
  (warn), red `#FF3B30` (danger), indigo, purple, pink, teal, yellow. Text `#1D1D1F` / `#F5F5F7`
  with secondary `#6E6E73`, tertiary `#8E8E93` and faint `#AEAEB2`. Fills are translucent greys
  (`rgba(120,120,128,.12)` …), separators `rgba(60,60,67,.13)`. Backgrounds: content white /
  `#1C1C1E`, grouped `#F2F2F7` / black, surfaces `#FFF` / `#2C2C2E`.
- **Accent ramp**: `--accent`, `--accent-strong`, `--accent-soft`, `--accent-bg`, `--accent-text`,
  `--accent-fg` (text **on** the accent: never hard-code white, §9.2), `--ring` (focus) — derived
  from one colour by the branding store (§9.3). `--ws` can carry a per-item colour (Orbit: the
  workspace's).
- **Shape**: radii 7 / 9 / 12 / 16 px (`--r-sm…--r-xl`); controls 9, cards and groups 12, sheets 16+;
  separators 0.5 px.
- **Depth**: `--shadow-sm` for cards in light mode only (dark relies on the hairline), `--shadow`
  for raised things, `--shadow-lg` for popovers and sheets.
- **Motion**: `--ease: cubic-bezier(.25,.1,.25,1)`, `--spring: cubic-bezier(.32,.72,0,1)`, 0.2 s;
  sheets rise 14 px with a spring, popovers scale from 0.97, buttons press to 0.97; everything off
  under `prefers-reduced-motion`.

**Components** (`@layer components` classes, plus `components/ui/`)
- **Shell**: a translucent sidebar (`.material`: `backdrop-filter: saturate(180%) blur(24px)`) with
  the brand, search (⌘K palette), big tiles for the main places (as Reminders' smart lists; Orbit: Today, Calendar) and sections with
  caps labels; the content area is white / `#1C1C1E`, **left-aligned at full width** (§10). Below
  1024 px the sidebar is a drawer; below 640 px a **bottom tab bar** (`.tabbar.material`) takes over.
- **Lists**: `.group` + `.group-row` — iOS inset grouped lists (rounded 12, hairlines between rows,
  44 px rows, an icon `.tile` on the left, a value or chevron on the right). `.card` for boxed
  sections with a title row.
- **Controls**: `.btn` (grey fill), `.btn-primary` (accent), `.btn-tinted` (accent at 10–16 %),
  `.btn-plain` (accent text, for in-row actions), `.btn-ghost`, `.btn-danger`, `.icon-btn` (30 px
  square); `.input` / `.field` (filled, no border, focus ring); `.seg` segmented control (a fill
  track, the selected segment a raised white pill); `Toggle.vue` is the iOS switch (green when on);
  `.check` the round Reminders checkbox; `.chip` filter pills that turn accent when on.
- **Overlays**: `Overlay.vue` is the sheet — a centred panel on wide screens, a bottom sheet on
  phones, or an inspector from the right (`mode="side"`); title row, body, footer (Cancel left of
  the primary action; a destructive action on the far left). `Confirm.vue` for alerts, `.popover` +
  `.menu-item` for menus (the hovered item fills with the accent), `Toasts.vue` top right
  (notification-style cards).
- **Icons**: lucide, 1.5–2 px strokes at 16–18 px; coloured `.tile`s (white glyph on a system
  colour with a faint top gradient, as in iOS Settings) for lists, workspaces and settings rows.
  App icons and crests are the owner's (§9.2).
- **Focus and selection**: `:focus-visible` is a 3 px `--ring` halo, no outlines; selection uses
  `--accent-soft`; scrollbars are thin overlay-style.

**Dark mode**: `light | dark | system` per user (§10), applied before first paint; every token has
both values; `<meta name="theme-color">` and the manifest colours follow the page (white /
`#1C1C1E`), as Apple's apps do. Check every screen in both — a theme accent can be light (Ink), so
text on the accent uses `--accent-fg`.

**Checklist for a new screen**: a large title (+ subhead), content in `.card` / `.group`, actions
as `.btn-primary` / `.btn-plain` in the title row or the sheet footer, forms in a sheet, both
themes and a phone width checked (screenshots), strings in every locale.

---

## 11. Testing strategy

| Layer | Where | How |
|---|---|---|
| Unit | `api/internal/*/…_test.go` | plain `testing` (no assertion library), table tests + `t.Run`, `t.Helper()` in every helper, one-sentence doc comment per test stating the invariant |
| Integration | `api/internal/api/*_test.go` | the **full router** over in-memory SQLite (`newTestRouter`/`newTestRouterDB`) with real services, a fixed-key sealer and a seeded admin; helpers `do`, `authdo`, `loginTok`, `loginCookies`, `reqWithCookies` |
| Frontend unit | `web/src/**/*.spec.ts` | Vitest + jsdom + @vue/test-utils, config inside `vite.config.ts`; stub globals per test (`vi.stubGlobal('fetch', …)`) |
| E2E | `web/e2e/*.spec.ts` | Playwright, chromium only, `E2E_BASE_URL`; role-based selectors first; timestamp-suffixed names so runs are idempotent |
| Real DB engines | `scripts/db-smoke.sh` | §11.5 |
| Security | `.gitea/workflows/security.yml` | govulncheck, `npm audit --omit=dev --audit-level=high`, gitleaks (full history) |

### 11.1 Patterns worth copying
- **In-memory DB per test**, migrated from the same `AllModels()` list. No shared global state.
- **Injectable clock**: a `now func() time.Time` field on stores and hubs, so expiry tests don't sleep.
- **Stubbable I/O**: function fields for DNS lookups (`lookupTXT`, `lookupCAA`), `httptest` servers for
  webhooks and a fake Vault, and an unguarded HTTP client used only in tests to reach loopback.
- **Generated fixtures**: build certificates and keys in the test (`makeChain`); no `testdata/` blobs.
- Test the **contract**, not the implementation: cookie flags, CSRF 403, single-use tokens,
  "a dry run creates nothing", "a replayed token revokes the session".

### 11.2 CI commands
Backend: `test -z "$(gofmt -l api)"` → `go vet` → `golangci-lint` (pinned; gosec with justified
excludes; errcheck/gosec relaxed for tests) → `go build` → `go test -coverprofile … ./...` (coverage
printed, no gate). Frontend: `npm ci` → `lint` → `typecheck` → `test`.

### 11.3 E2E in CI
Build the SPA → copy it into the embed dir → build the binary → start it with a fresh SQLite and
throwaway secrets → poll `/readyz` → run Playwright. All in **one step**, because act_runner doesn't
keep background processes between steps.

### 11.4 Frontend specs here
- Locale parity.
- `tryRefresh` single-flight.
- Formatting helpers.
- UI primitives (Badge, Toggle, Confirm via `document.body` because Overlay teleports, CopyButton).

### 11.5 DB smoke (`make db-smoke DB=postgres|mysql`)
Boot the API from source against the real engine, then:
1. Log in, create data, and set a DB-backed setting.
2. **Restart**.
3. Check that the setting persisted, that sealed data is still usable after the restart, and that
   token rotation works.
4. Grep the logs for SQL errors.

Tear everything down in a `trap`. This caught three bugs that made Postgres and MySQL unusable
while every SQLite unit test was green.

---

## 12. CI/CD (Gitea Actions; the same YAML works on GitHub Actions)

| Workflow | Trigger | Does |
|---|---|---|
| `test.yml` | every PR + manual | backend and frontend jobs (§11.2); concurrency per branch, cancel in progress |
| `e2e.yml` | every PR + manual | §11.3 |
| `security.yml` | every PR + manual + **weekly cron** (new advisories hit unchanged code) | three independent jobs so one failure doesn't hide the others |
| `build.yml` | **manual**, input `version` (bare semver) | buildx, `linux/amd64,linux/arm64` (cross-compiled, **no QEMU**), one build pushed to **two registries** (Harbor + Gitea packages) as `:vX.Y.Z` and `:latest`; the Gitea registry host is derived from `GITHUB_SERVER_URL`/`GITHUB_REPOSITORY` (nothing hard-coded) |
| `release.yml` | **manual**, input `version` | tags `vX.Y.Z` at `main` HEAD and creates the release with notes = CHANGELOG section + commit list (§13.3); needs `jq` and `curl` |

- Setup-action caches (Go modules keyed on `go.sum`, npm on `package-lock.json`).
- One build for all registries (building per registry had filled the runner's disk).
- Secrets: `HARBOR_*`, optional `PACKAGES_TOKEN` and `RELEASE_TOKEN` (fall back to `github.token`).
- Every workflow starts with a header comment explaining its purpose and the non-obvious choices.
- Branching: short-lived `feat/…`, `fix/…`, `chore/…` branches off a protected `main`, landed by PR
  after the checks pass (rebase-merge). Nothing is released on merge.

---

## 13. Versioning, changelog & release

### 13.1 SemVer and version markers
Patch = fixes, minor = features, major = breaking changes. The version lives in several places, all
rewritten by **`make set-version VERSION=x.y.z`** (`scripts/set-version.sh`: validates bare semver, uses
`perl -pi` for macOS/Linux parity):

| File | Marker |
|---|---|
| `web/package.json` | `"version"` |
| `api/internal/version/version.go` | `Version = "…"` (fallback when no ldflags) |
| `.env.example` | `PKI_VERSION=v…` (compose image tag) |
| `api/cmd/server/main.go` | OpenAPI `@version` |
| `api/docs/{docs.go,swagger.json,swagger.yaml}` | generated spec version |

### 13.2 CHANGELOG (Keep a Changelog)
- `## [Unreleased]` at the top, filled **as changes merge**, not at release time.
- Version sections `## [X.Y.Z] - YYYY-MM-DD`, newest first. Subsections `### Security / Added / Changed /
  Fixed / Removed / Documentation`.
- Each bullet starts with a **bold lead-in sentence**, then the detail: the API contract change, the
  new setting or env names, and a link to the doc.
- Breaking releases get a `> **⚠️ Breaking — …**` callout.
- User-facing only. The commit list covers the rest.

### 13.3 Release notes
`release.yml` extracts the CHANGELOG section with an awk scan: the `## ` heading containing `[X.Y.Z]`
(falling back to `[Unreleased]`) up to the next `## `. It then builds:
```
## What's new
<changelog section>
---
**All changes since <prev tag>:**
- <subject> (<short sha>)   ← git log --no-merges <prev>..HEAD
```

### 13.4 Runbook (`docs/v2/RELEASING.md`)
0. **Pre-flight**: `main` is green, pick the version, and check the runner has free disk.
1. **Bump**: `make set-version` + roll `[Unreleased]` → `[X.Y.Z] - date` + a fresh empty
   `[Unreleased]`. Then open a `release/vX.Y.Z` PR and merge it.
2. **Build & Push** (manual) with the bare version.
3. **Release** (manual) with the same version.
4. **Verify**: both registries, both architectures, the release notes, and `/version` + UI version.

A botched release is redone by deleting the release and the tag, fixing `main` and re-running.
Rules: always **bump before build** and always pass the **bare** version (workflows add the `v`).

---

## 14. Documentation system

### 14.1 Where things go
| Doc | Purpose |
|---|---|
| `README.md` | what it is · Features (bold name — one line — link) · Quick start (3 commands + a "back up the master key" callout) · Deployment · Configuration · Development (make-target table + Tests) · Security · **Documentation index table** · Architecture |
| `CHANGELOG.md` | §13.2 |
| `docs/v2/ARCHITECTURE.md` | goals, decisions, stack, layout, data model, subsystems, milestones, resolved decisions |
| `docs/v2/CONFIGURATION.md` | every env var: `Variable | Req | Default | Notes`, grouped |
| `docs/v2/SECURITY.md` | threat model, authn/authz, crypto, operator duties, OWASP table, accepted risks, reporting |
| `docs/v2/USER-GUIDE.md` | task-oriented walkthrough with a Contents list |
| `docs/v2/RELEASING.md` | runbook (§13.4) |
| `docs/v2/<FEATURE>.md` | one per substantial feature or integration (ACME, CERT-LOGIN, SSO-<idp>, METRICS, LOGGING-elk, SECRETS, reverse-proxies) |
| `docs/v2/MIGRATION-*.md` | why, what the tool does, usage, and **verification performed** with results |
| `docs/v2/*-BLUEPRINT.md` | portable designs for reuse (this file, AUTH-BLUEPRINT) |
| `deploy/README.md` | monitoring stack |
| `api/docs/` | generated OpenAPI, committed |

### 14.2 Writing style
- Open with one paragraph: what this is and when you need it. Then numbered, task-shaped sections.
- **Bold** key terms, `backticks` for identifiers, env vars and paths, `>` callouts for warnings.
- Tables for reference data. Runbooks start with a TL;DR code block, then "why the order matters",
  numbered phases, "rules that bite", and how to redo a botched run. Name the real past mistake.
- `## Troubleshooting`: **symptom** → fix bullets.
- Relative links between docs. Each doc points to its neighbours.
- **Every config file and script starts with a comment explaining purpose and non-obvious choices**
  (workflows, Makefile, vite config, lint configs, `.gitignore`, compose files).
- Code: every package has a `// Package x …` doc comment, comments explain **why**, and non-test code
  has no TODO/FIXME (open an issue or do it).

### 14.3 The rule
A change isn't done until its docs, CHANGELOG `[Unreleased]` entry, `.env.example` and OpenAPI spec
are updated in the same PR.

---

## 15. Inconsistencies in this repo (fix these when copying)

- **UI version source.** RELEASING.md says the SPA bakes its version from `package.json`
  (`__APP_VERSION__`), but nothing in `web/src` reads `__APP_VERSION__`. The UI shows the server's
  version from `/about`. Pick one: drop the define, or show both and warn when they differ.
- **Build stamp gaps.** CI passes no `BUILD_TIME` (images report an empty build time), and passes the
  full SHA where the Makefile passes the short one.
- **Dockerfile uses `npm install`**. Use `npm ci` for reproducible images.
- **Unpinned tools**: `air`, `swag` and `govulncheck` are `@latest` and gitleaks resolves the latest release,
  while golangci-lint is pinned. Pin all of them, or document why `@latest` is intentional.
  `@latest` can also outgrow your toolchain: govulncheck v1.8+ needs Go 1.26 to build, which
  silently broke the scan on Go 1.25 (now installed with `GOTOOLCHAIN=auto`).
- **`make run`** passes `--env-file ../.env.example`, which resolves outside the repo.
- **Host npm in the `web-*` Make targets** (§3.2).
- **Update check isn't semver-ordered** (§8).
- **Notification secrets are masked, not sealed** (§5.3).
- **Single-instance assumptions**: audit insert lock, per-IP rate limiter, SSE fan-out.
- **`idParam` doesn't validate UUID format** (§5.1).
- Stale text: metrics docs still say "numeric CA id"; `deploy/README.md` mentions a
  `build-gitea.yml` that no longer exists; a compose comment says `make release-image` builds the
  published image (CI does).
- No `.editorconfig`, PR template or dependency-update bot (Renovate). Consider adding them.

---

## 16. Translating the stack

| Concern | Here (Go/Vue) | Python | Node/TS | .NET | Rust |
|---|---|---|---|---|---|
| HTTP | Gin | FastAPI | Fastify / Hono | ASP.NET Core minimal APIs | axum |
| ORM + auto-migrate | GORM `AutoMigrate` | SQLAlchemy + Alembic | Prisma / Drizzle | EF Core migrations | sqlx / SeaORM |
| UUIDv7 | `uuid.NewV7()` | `uuid.uuid7()` (3.14) | `uuid` `v7()` | `Guid.CreateVersion7()` | `Uuid::now_v7()` |
| Live reload (poll) | air | uvicorn `--reload` | tsx watch | dotnet watch | cargo-watch |
| Embed the SPA | `//go:embed` | serve `dist/` via StaticFiles | `@fastify/static` | `UseStaticFiles` + fallback | `rust-embed` |
| Single static image | distroless static | distroless python / slim | distroless nodejs | chiseled .NET | distroless static (musl) |
| Cron | robfig/cron | APScheduler | node-cron | Quartz / hosted services | tokio-cron-scheduler |
| Metrics | client_golang | prometheus_client | prom-client | prometheus-net | prometheus crate |
| OpenAPI | swaggo annotations | built into FastAPI | zod-openapi / fastify-swagger | Swashbuckle | utoipa |
| Unit + integration | `testing` + httptest + in-mem SQLite | pytest + TestClient | vitest + supertest | xUnit + WebApplicationFactory | `cargo test` + axum-test |
| Vuln scan | govulncheck | pip-audit | npm audit | `dotnet list package --vulnerable` | cargo-audit |

The frontend (Vue/Vite/Tailwind/Pinia), Docker layout, CI shape, release machinery and docs system
carry over unchanged.
