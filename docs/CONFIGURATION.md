# Configuration

This page lists every setting Orbit reads. Most of it is **environment
variables**, read once at start-up. The rest is **runtime settings** that the
owner or an admin changes on the page and that live in the database. Read this
when you deploy Orbit, move it to another host, or wonder why a value you
set has no effect. The code is the source of truth: `api/internal/config/config.go`
and `api/internal/secrets/`.

Copy [`.env.example`](../.env.example) to `.env` and fill it in. `docker compose`
passes the whole `.env` to the container. The first sign-in is in the
[README](../README.md#quick-start); what each page does is in the
[USER-GUIDE](USER-GUIDE.md).

## How to read the tables

- **Req**: **release** means Orbit refuses to start without it when
  `GIN_MODE=release`. A dash means it is optional and falls back to the default.
- **Default**: what Orbit uses when the variable is unset or empty. An empty
  cell means "unset, feature off".
- **Release mode** (`GIN_MODE=release`, what `docker-compose.yml` sets):
  `JWT_SECRET` and `ORBIT_MASTER_KEY` must be set and at least **32 characters**,
  and auth cookies are `Secure` by default.
- **Dev mode** (`GIN_MODE=debug`, the default): missing secrets fall back to
  fixed, insecure dev values, and passkeys work on `localhost` out of the box.
  Never run production in dev mode.

> **The compose file wins over `.env`.** `docker-compose.yml` sets `GIN_MODE`,
> `DB_DRIVER`, `DB_URL`, `JWT_SECRET` and `ORBIT_MASTER_KEY` in its `environment:`
> block, which overrides the same keys in `.env`. So the production stack is
> always in release mode, with the database at `/data/orbit.db`.

## Deployment image (compose only)

These are read by `docker compose`, not by Orbit.

| Variable | Req | Default | Notes |
|---|---|---|---|
| `ORBIT_IMAGE` | – | `harbor.ubonito.org/orbit/orbit` | Image `docker-compose.yml` runs (a placeholder: point it at your registry). |
| `ORBIT_VERSION` | – | `latest` | Image tag. **Pin a release** (e.g. `v0.1.0`) for repeatable deploys. Update with `docker compose pull && docker compose up -d`. |
| `ORBIT_PORT` | – | `8080` | Host port mapped to the container's `:8080`. |
| `VERSION`, `COMMIT`, `BUILD_TIME` | – | from git | Build arguments for `docker-compose-build.yml` (`make up-build` fills them). They stamp the version shown in About. |

The image is distroless and runs as the `nonroot` user. It keeps its data in
the `/data` volume and has a built-in health check (see
[Commands](#commands-in-the-binary)).

## Secrets

These four values go through the **secrets backend** (see
[Secrets backends](#secrets-backends-file-and-vault)): `JWT_SECRET`,
`ORBIT_MASTER_KEY`, `DB_URL` and `ADMIN_PASSWORD`. Everything else is always a
plain environment variable.

| Variable | Req | Default | Notes |
|---|---|---|---|
| `SECRETS_BACKEND` | – | `env` | Where the four secrets come from: `env`, `file` or `vault`. Any other value stops start-up. |
| `JWT_SECRET` | **release** | dev fallback | HMAC key that signs the access tokens (HS256). At least 32 characters in release. Make one with `openssl rand -hex 32`. Changing it signs everyone out, nothing else. |
| `ORBIT_MASTER_KEY` | **release** | dev fallback | Key that **seals secrets at rest**: TOTP secrets, the SSO client secret, notification secrets and the addresses of subscribed calendars. At least 32 characters in release. Make one with `openssl rand -hex 32`. |

> **`ORBIT_MASTER_KEY` must never change.** Orbit derives its encryption key
> from it (Argon2id, with a salt stored in the database). With a different key
> the sealed secrets can't be read: authenticator codes stop working, SSO
> fails, notification channels lose their secrets, and subscribed calendars stop refreshing. There is no key
> rotation. **Back it up** somewhere safe, next to your database backups. A
> database without its master key is only half a backup.

## First admin (first start only)

Used once, when the database has no users. Later changes have no effect.

| Variable | Req | Default | Notes |
|---|---|---|---|
| `ADMIN_USER` | – | `admin` | User name of the first admin. This account is also the instance **owner**. |
| `ADMIN_PASSWORD` | – | random | Its first password. Leave it empty and Orbit prints a random temporary password once in the log (`docker compose logs api \| grep -A1 "First sign-in"`). Either way the admin must choose a new password at first sign-in. |

## HTTP, TLS and cookies

| Variable | Req | Default | Notes |
|---|---|---|---|
| `GIN_MODE` | – | `debug` | `debug`, `release` or `test`. Use `release` in production: it enforces the secret rules and `Secure` cookies, and logs JSON. |
| `HTTP_ADDR` | – | `:8080` | Listen address of the API and the page. |
| `TLS_CERT_FILE` | – | | Certificate file. Set it **together with** `TLS_KEY_FILE` and Orbit serves HTTPS itself (TLS 1.2 or newer). Setting only one stops start-up. Unset: plain HTTP, TLS ends at your reverse proxy. |
| `TLS_KEY_FILE` | – | | Private key for `TLS_CERT_FILE`. |
| `COOKIE_SECURE` | – | `true` in release or with TLS, else `false` | Marks the sign-in cookies `Secure` (sent over HTTPS only). Set `true` behind a TLS proxy in dev mode; set `false` only on a plain-HTTP test box. |
| `COOKIE_DOMAIN` | – | host only | Parent domain for the sign-in cookies (e.g. `example.org`) to share them across subdomains. Leave empty for a single host. |
| `TRUSTED_PROXIES` | – | trust none | Comma list of IPs or CIDRs of your reverse proxies. Only these may set `X-Forwarded-For`. **Set it behind a proxy**, or every request seems to come from the proxy (one shared rate limit, one IP in the audit log). An invalid list is logged and ignored (trust none). See [reverse-proxies.md](reverse-proxies.md). |
| `MAX_BODY_BYTES` | – | `4194304` (4 MiB) | Largest request body. Branding logos are sent inline, so they count too. |
| `CORS_ORIGINS` | – | | Comma list of origins allowed to call the API from a browser. **Dev only** (the Vite server, `:5180` on the host). In production the page is served from the same origin and needs no CORS. |
| `PUBLIC_URL` | – | from the request | The address people use to reach Orbit (scheme and host, no trailing slash), e.g. `https://orbit.example.org`. Calendar-feed addresses (Settings → Calendar feeds) are built from it; unset, they use the request's host and `X-Forwarded-Proto`. An `https://` address also identifies the instance to the Web Push services (Google, Apple, Mozilla), and is the address assistants are sent to sign in with (MCP's OAuth) — set it behind a proxy. |

## Database

| Variable | Req | Default | Notes |
|---|---|---|---|
| `DB_DRIVER` | – | `sqlite` | `sqlite` (one file, the default) or `postgres`. Any other value stops start-up. |
| `DB_URL` | – | `orbit.db` | SQLite: the file's path (the compose files set `/data/orbit.db`, the volume). PostgreSQL: a connection string, `postgres://orbit:secret@db:5432/orbit?sslmode=require` (`sslmode=disable` only on a private network). Goes through the secrets backend. |

**PostgreSQL** suits an instance whose database lives on its own server (or
a managed one). Everything works the same — search uses PostgreSQL's full
text (accent-insensitive when the `unaccent` extension can be created; Orbit
tries) — except the in-app **backups**, which are of SQLite: back PostgreSQL
up with its own tools (`pg_dump`, or your provider's snapshots), and keep
`ORBIT_MASTER_KEY` safely apart (the secrets in the database are sealed with
it). Orbit creates its tables on start; give its user a database of its own.
There is no tool to move an SQLite Orbit into PostgreSQL yet — start empty,
or move workspaces across with workspace export / import.

The database holds everything: users, roles, workspaces and their content,
the audit log and the runtime settings below. Back up the file and
`ORBIT_MASTER_KEY` together.

## Sign-in hardening

| Variable | Req | Default | Notes |
|---|---|---|---|
| `AUTH_RATE_BURST` | – | `8` | Per-IP bucket size for sign-in, SSO code exchange, passkey sign-in and the two-factor routes. |
| `AUTH_RATE_PER_MINUTE` | – | `12` | Tokens added back to that bucket per minute. When empty the client gets `429`. |
| `LOGIN_LOCKOUT_THRESHOLD` | – | `5` | Wrong passwords or codes in a row before an account is locked. `0` turns the lockout off (the per-IP limit still applies). |
| `LOGIN_LOCKOUT_MINUTES` | – | `15` | How long the lock lasts. It ends by itself, or an admin unlocks the user. |

## Passkeys (WebAuthn)

| Variable | Req | Default | Notes |
|---|---|---|---|
| `WEBAUTHN_RP_ID` | – | empty; dev: `localhost` | The public host name users see, e.g. `orbit.example.org`. **Empty turns passkeys off.** It is effectively permanent: changing it orphans every registered passkey. |
| `WEBAUTHN_RP_ORIGINS` | – | empty; dev: `http://localhost:5173`, `http://localhost:8080` (the dev compose file sets `:5180` and `:8090`) | Comma list of full origins, e.g. `https://orbit.example.org`. |
| `WEBAUTHN_RP_DISPLAY_NAME` | – | `Orbit` | Name shown in the browser's passkey prompt. |

## Uploads

| Variable | Req | Default | Notes |
|---|---|---|---|
| `UPLOAD_MAX_MB` | – | `25` | Largest file someone can add to a page, task or note. |
| `UPLOAD_QUOTA_MB` | – | `2048` | All uploads together, for the instance; `0` = no limit. Past it, uploads answer `507`. |
| `IMPORT_MAX_MB` | – | `512` | Largest workspace export someone can import (its files included). Files past `UPLOAD_MAX_MB`, or past the quota, are left out and listed. |

Uploads are kept **in the database** (a table of their own), so backups and
restores carry them with everything else. Plan the disk accordingly.

## Diagrams (draw.io)

| Variable | Req | Default | Notes |
|---|---|---|---|
| `DRAWIO_URL` | – | `https://embed.diagrams.net` | Where the Diagrams module loads the draw.io editor from (in a frame). The owner can override it in **Owner → Owner → Diagrams (draw.io)**; empty there goes back to this. It is also the one outside address the Content-Security-Policy lets be framed. |

draw.io's own editor needs the browser to reach the internet; the diagram is
handed to it inside the browser (`postMessage`) and saved only in Orbit. To
load nothing from outside, run draw.io's container next to Orbit and point
`DRAWIO_URL` at it — the commented `drawio` service in `docker-compose.yml`
(`jgraph/drawio`, Apache 2.0). Serve it on HTTPS when Orbit is (a browser
won't frame `http://` in an `https://` page).

## Git

| Variable | Req | Default | Notes |
|---|---|---|---|
| `GIT_SERVERS` | – | | Comma list of Gitea / Forgejo (or GitHub Enterprise) base URLs (e.g. `https://gitea.lan,http://git.home:3000`) that users may connect **even on a private network** — your own server. They are also suggested in Settings → Git, and may use plain `http://`. Any other server must be public and use `https`: for those Orbit's public-only guard refuses private, loopback and link-local addresses (Web Push uses the same guard). |

Each person connects their own account (Settings → Git) with a personal
access token with the permissions *issue: read and write*, *repository:
read*, *user: read*. Tokens are sealed with `ORBIT_MASTER_KEY`. Linked lists
sync every 3 minutes; the `git_integration` feature flag turns all of it off.

## Observability and updates

| Variable | Req | Default | Notes |
|---|---|---|---|
| `METRICS_ENABLED` | – | `false` | Serves Prometheus metrics at `/metrics`: HTTP requests and latency, sign-ins, and counts of users, workspaces, pages, open tasks and failing calendar subscriptions (see [deploy/](../deploy/README.md)). The endpoint has **no sign-in**: block it at your reverse proxy or keep it on an internal network. |
| `SYSLOG_ADDR` | – | | Also sends every audit entry as one JSON line to syslog (facility AUTH), with its chain hash. `host:port` uses UDP (port 514 when left out); a path like `/dev/log` uses the local socket. If the address can't be reached at start-up, Orbit stops. |
| `UPDATE_CHECK` | – | `true` | Checks for a newer release every 6 hours and shows it in About. `false` turns it off. |
| `REPO_URL` | – | `https://gitea.ubonito.org/jteixeira/Orbit` | Repository for the About link and the update check (a Gitea repo URL). |

## Secrets backends (file and vault)

Both ways stay supported: an env file is the default and needs nothing else;
a file or Vault keeps the secrets out of it.
What goes through the backend: `JWT_SECRET`, `ORBIT_MASTER_KEY`, `DB_URL`,
`ADMIN_PASSWORD`, `BACKUP_S3_ACCESS_KEY` and `BACKUP_S3_SECRET_KEY`.

**`SECRETS_BACKEND=env`** (default): the secrets are plain environment
variables.

**`SECRETS_BACKEND=file`**: for Docker or Kubernetes secrets. For each secret
Orbit reads the file named by `<KEY>_FILE`, trimming white space, and falls
back to the plain variable when `<KEY>_FILE` is unset. A `_FILE` that can't be
read stops start-up.

| Variable | Notes |
|---|---|
| `JWT_SECRET_FILE` | File holding `JWT_SECRET`. |
| `ORBIT_MASTER_KEY_FILE` | File holding `ORBIT_MASTER_KEY`, e.g. `/run/secrets/master_key`. |
| `DB_URL_FILE` | File holding `DB_URL`. |
| `ADMIN_PASSWORD_FILE` | File holding `ADMIN_PASSWORD`. |
| `BACKUP_S3_ACCESS_KEY_FILE`, `BACKUP_S3_SECRET_KEY_FILE` | Files holding the S3 keys for backups. |

`<KEY>_FILE` is ignored unless `SECRETS_BACKEND=file`.

**`SECRETS_BACKEND=vault`**: HashiCorp Vault, KV version 2. Orbit logs in,
reads one secret once at start-up, and stops if Vault fails. Inside that
secret, use the same key names (`JWT_SECRET`, `ORBIT_MASTER_KEY`, `DB_URL`,
`ADMIN_PASSWORD`, `BACKUP_S3_ACCESS_KEY`, `BACKUP_S3_SECRET_KEY`) with
string values. Rotating one in Vault takes effect
at the next restart. A key missing from Vault falls back to
the environment variable of the same name.

| Variable | Req | Default | Notes |
|---|---|---|---|
| `VAULT_ADDR` | **if vault** | | Vault address, e.g. `https://vault.example.org:8200`. |
| `VAULT_KV_MOUNT` | – | `secret` | Mount of the KV v2 engine. |
| `VAULT_KV_PATH` | **if vault** | | Path of the secret inside the mount, e.g. `orbit`. |
| `VAULT_TOKEN` | one of these | | Vault token. Used first when set. |
| `VAULT_ROLE_ID` | one of these | | AppRole role id, used when `VAULT_TOKEN` is empty. |
| `VAULT_SECRET_ID` | with `VAULT_ROLE_ID` | | AppRole secret id. |

> **Using file or vault with compose.** `docker-compose.yml` insists on
> `JWT_SECRET` and `ORBIT_MASTER_KEY` in `.env` (`${VAR:?}`). To keep them out of
> `.env`, remove those two lines from the `environment:` block of your copy.

## Dev only

| Variable | Used by | Notes |
|---|---|---|
| `CORS_ORIGINS` | API | `http://localhost:5180,http://localhost:5173` in `docker-compose.dev.yml` (the second is Vite seen from inside its container, for screenshots and E2E). |
| `VITE_PROXY_TARGET` | Vite dev server | Where `/api` is proxied (`http://api:8080` in the dev container; default `http://localhost:8080`). |
| `VITE_POLL` | Vite dev server | Any value makes Vite poll for file changes (Docker Desktop bind mounts). |

The dev compose file also sets throwaway secrets and `ADMIN_PASSWORD=admin` on
purpose. Never reuse them.

## Backups

| Variable | Req | Default | Notes |
|---|---|---|---|
| `BACKUP_S3_ACCESS_KEY`, `BACKUP_S3_SECRET_KEY` | for S3 | – | The keys for automatic backups to S3 (Owner → Owner → Backups). Read through `SECRETS_BACKEND` (env, file or vault), never stored on the page. |
| `BACKUP_PASSPHRASE` / `BACKUP_PASSPHRASE_FILE` | for restore | – | Only for `orbit restore-backup` of a passphrase archive. |

For backups to a directory, mount a volume where the page says (e.g.
`./backups:/backups`), writable by the image's user (65532).

## Commands in the binary

| Command | What it does |
|---|---|
| `orbit` | Starts the server. |
| `orbit healthcheck` | Calls `http://127.0.0.1:<port of HTTP_ADDR>/api/v1/healthz` (`https://` when `TLS_CERT_FILE` and `TLS_KEY_FILE` are set; the loopback certificate isn't checked) and exits `0` on `200`, else `1`. The image's `HEALTHCHECK` uses it (distroless has no shell or curl). |
| `orbit restore-backup <archive> [--force]` | Writes the database inside an encrypted backup to `DB_URL`, **with Orbit stopped**, then exits. A master-key archive opens with `ORBIT_MASTER_KEY`; a passphrase archive with `BACKUP_PASSPHRASE` (or `BACKUP_PASSPHRASE_FILE`). An existing database is refused unless `--force`, and is then kept as `<DB_URL>.before-restore-<time>`. With compose: `docker compose stop api`, `docker compose run --rm -v ./backup.json:/restore.json:ro api restore-backup /restore.json --force`, `docker compose start api`. |


## Settings on the page (not environment)

These live in the database (`settings` table, per-user preferences apart) and
take effect at once, without a restart. Every change is written to the audit
log. **Owner** means the one instance owner; a role can't grant it. Where
"step-up" is marked, the owner must re-enter a password or passkey first when
the `step_up_auth` feature flag is on.

| Setting | Where on the page | Who can change it | Default |
|---|---|---|---|
| **Security policy**: `otp_enforced` (every local user needs two-factor), `hide_local_login` (hide the password form when SSO is on; `/login?local=1` still shows it), `passwordless_enabled` (the "Sign in with a passkey" button), `auto_sso` (start SSO right away on the sign-in page; needs SSO on) | Owner → Security | Owner (step-up) | off, off, **on**, off |
| **SSO (OIDC)**: on/off, issuer, client id, client secret (stored sealed, never shown again), redirect URL, extra scopes, role claim, default role for new users, whether the identity provider may grant admin | Owner → SSO | Owner (step-up) | off; default role `none` |
| **Feature flags**: `sessions` (users see and end their sessions), `impersonation`, `step_up_auth`, `calendar_subscriptions` (workspaces subscribe to ICS calendars), `calendar_feeds` (users publish private ICS feeds), `git_integration` (GitHub, Gitea and Forgejo accounts, linked lists and issues), `caldav_sync` (two-way sync of workspace calendars with CalDAV calendars), `workspace_invites` (owners make invite links; people join through them, making an account if needed), `personal_channels` (each person adds their own Discord / Slack / Teams / Telegram / ntfy / Gotify channel) | Owner → Owner | Owner (step-up) | off, off, off, **on**, **on**, **on**, **on**, **on**, **on** |
| **Security alerts**: which events alert (impersonation, owner transfer, admin role granted, security/SSO config changed, session token reuse), which channels, which email | Owner → Owner, **Security alerts** | Owner | all off |
| **Owner transfer** | Owner → Owner, danger zone | Owner (step-up) | the first admin |
| **Branding**: app name, accent colour, logo, favicon, show name, show logo, sign-in message | Owner → Branding | Owner, or a role with `settings.branding` | "Orbit" |
| **Developer themes**: `developer_themes_enabled`, `developer_theme` (`blue`, `gold`, `red`, `ink`, `green`; anything else is stored as `blue`), per asset `asset_source_logo` / `_wordmark` / `_favicon` (`theme` or `keep`), and `developer_themes_personal` (each person may pick their own) | Owner → Branding | Owner, or a role with `settings.branding` | off; `blue`; `theme`; off |
| Per user: **developer theme** (as the instance, none, or one of the five) — when the owner allows it | Settings → Appearance | the user | as the instance |
| **Notification channels**: email, Discord, Telegram, ntfy, Gotify, Microsoft Teams, Slack, signed webhook (stored sealed) | Owner → Notifications | Owner, or a role with `settings.notifications` | none |
| **Automatic backups**: on / off, every N hours, keep N, a directory or an S3 bucket (prefix, endpoint, region) | Owner → Owner → Backups | Owner (step-up) | off |
| Per user: **require two-factor**, unlock, reset password or two-factor | Admin → Users | `users.manage` | off |
| Per user: theme, personal accent, time zone (for reminders), week start, time format, language (kept in the browser; the server keeps the last one for push and channel messages) | Settings | the user | automatic; device zone; by language |
| Per user: **notifications** — per kind (reminders, assigned, workspaces, activity, summary) to devices (Web Push) and/or channels; which channels; the daily summary and its time; push devices | Settings → Notifications | the user | about you → devices; activity → bell only; summary off |
| Per user and workspace: **mute activity** | the workspace's `⋯` | the user | off |
| Per user: **calendar feeds** | Settings → Calendar feeds | the user | none |
| Per user: **git accounts** (server and token) | Settings → Git | the user | none |
| Per workspace: name, icon, colour, modules and their order, members and their roles, archived | the workspace's settings (`⋯`) | the workspace's owners | from the template |

The owner and delegated roles are explained in [SECURITY.md](SECURITY.md#authorization).

## See also

- [SECURITY.md](SECURITY.md): what these settings protect, and the operator checklist.
- [reverse-proxies.md](reverse-proxies.md): TLS, `TRUSTED_PROXIES`, cookies and live updates behind nginx, HAProxy, Caddy or Traefik.
- [SSO-entra.md](SSO-entra.md), [SSO-authentik.md](SSO-authentik.md): filling in Owner → SSO.
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md): start-up errors such as a short secret.
- [ARCHITECTURE.md](ARCHITECTURE.md), [API.md](API.md).
