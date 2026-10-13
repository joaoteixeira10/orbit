# Configuration

This page lists every setting Orbit v1 reads. Most of it is **environment
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
| `ORBIT_IMAGE` | – | `harbor.ubonito.org/proxy_manager/orbit` | Image `docker-compose.yml` runs. |
| `ORBIT_VERSION` | – | `latest` | Image tag. **Pin a release** (e.g. `v1.0.0`) for repeatable deploys. Update with `docker compose pull && docker compose up -d`. |
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
| `ORBIT_MASTER_KEY` | **release** | dev fallback | Key that **seals secrets at rest**: TOTP secrets, the SSO client secret and the agent tokens. At least 32 characters in release. Make one with `openssl rand -hex 32`. |

> **`ORBIT_MASTER_KEY` must never change.** Orbit derives its encryption key
> from it (Argon2id, with a salt stored in the database). With a different key
> the sealed secrets can't be read: authenticator codes stop working, SSO
> fails, and every proxy shows the agent token as wrong. There is no key
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
| `CORS_ORIGINS` | – | | Comma list of origins allowed to call the API from a browser. **Dev only** (the Vite server on `:5173`). In production the page is served from the same origin and needs no CORS. |

## Database

| Variable | Req | Default | Notes |
|---|---|---|---|
| `DB_DRIVER` | – | `sqlite` | Only `sqlite` is supported. Any other value stops start-up. See [BLUEPRINT-FIT.md](BLUEPRINT-FIT.md). |
| `DB_URL` | – | `orbit.db` | Path of the SQLite file. Goes through the secrets backend. The compose files set `/data/orbit.db` (the volume). |

The database holds everything: users, roles, proxies (with sealed agent
tokens), the audit log and the runtime settings below. Back up the file and
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
| `WEBAUTHN_RP_ID` | – | empty; dev: `localhost` | The public host name users see, e.g. `lb.example.org`. **Empty turns passkeys off.** It is effectively permanent: changing it orphans every registered passkey. |
| `WEBAUTHN_RP_ORIGINS` | – | empty; dev: `http://localhost:5173`, `http://localhost:8080` | Comma list of full origins, e.g. `https://lb.example.org`. |
| `WEBAUTHN_RP_DISPLAY_NAME` | – | `Orbit` | Name shown in the browser's passkey prompt. |

## Proxies

| Variable | Req | Default | Notes |
|---|---|---|---|
| `PROXY_POLL_SECONDS` | – | `10` | How often every lb-agent is polled, and so how fresh the page is. Minimum 1. This is only the **default**: the owner can change the interval at runtime (Owner → Live updates, 1 to 300 seconds) without a restart. |
| `PROXIES` | – | | **One-time import** from Orbit v0. v0's `PROXIES` JSON fills an **empty** registry at start-up; the agent tokens are sealed on the way in. It does nothing once any proxy exists. Remove it after the first start and manage proxies in Admin → Proxies. |

The v0 format is a JSON list; `name`, `agent` and `token` are required:

```sh
PROXIES=[{"name":"gmr","label":"GMR","agent":"http://gmr:9000","token":"...","lb_url":"http://gmr:80","public_url":"https://gmr.example.org"}]
```

## Observability and updates

| Variable | Req | Default | Notes |
|---|---|---|---|
| `METRICS_ENABLED` | – | `false` | Serves Prometheus metrics at `/metrics`: Orbit's own, and every proxy's servers (up, in rotation, maintenance, requests, errors, latency, sessions, weight; see [deploy/](../deploy/README.md)). The endpoint has **no sign-in**: block it at your reverse proxy or keep it on an internal network. |
| `SYSLOG_ADDR` | – | | Also sends every audit entry as one JSON line to syslog (facility AUTH), with its chain hash. `host:port` uses UDP (port 514 when left out); a path like `/dev/log` uses the local socket. If the address can't be reached at start-up, Orbit stops. |
| `UPDATE_CHECK` | – | `true` | Checks for a newer release every 6 hours and shows it in About. `false` turns it off. |
| `REPO_URL` | – | `https://gitea.ubonito.org/jteixeira/Orbit` | Repository for the About link and the update check (a Gitea repo URL). |

### Agents (TLS)

Orbit follows each proxy's agent URL: `http://` (the token only) or
`https://`. For HTTPS agents it checks the certificate against the system CAs
plus `AGENT_CA_FILE`, and sends its client certificate to agents that require
one (mutual TLS). Nothing here is needed for plain-HTTP agents. Certificates
from files are loaded again when the files change. How to set agents up:
[AGENTS.md, step 7](AGENTS.md#7-keep-the-agent-port-private-and-use-tls).

| Variable | Req | Default | Notes |
|---|---|---|---|
| `AGENT_CA_FILE` | – | | PEM bundle of the CA(s) that signed the agents' certificates, added to the system's. Unreadable or empty stops start-up. |
| `AGENT_CLIENT_CERT_FILE` | if mutual TLS | | Orbit's client certificate (PEM, `clientAuth`), signed by the agents' `TLS_CLIENT_CA_FILE`. Needs a key. |
| `AGENT_CLIENT_KEY_FILE` | with the cert | | Its private key. |
| `AGENT_CLIENT_KEY` | – | | The key as PEM **through `SECRETS_BACKEND`** (e.g. from Vault) instead of `AGENT_CLIENT_KEY_FILE`; read only when that is unset. Not reloaded until a restart. |

**Agents must use HTTPS** (Admin → Proxies, off by default) refuses `http://`
agent URLs; see [Settings on the page](#settings-on-the-page-not-environment).

### PKI Management (certificates)

PKI Management's URL and API token are set on the page (**Proxies →
Certificates → PKI Management**; the token is stored sealed). Calls to it go
through the guarded client: no loopback, link-local or metadata addresses, no
redirects.

| Variable | Req | Default | Notes |
|---|---|---|---|
| `PKI_CA_FILE` | – | | PEM bundle of the CA(s) that signed PKI Management's HTTPS certificate, added to the system's. Unreadable or empty: issuing fails with that error. |

## Secrets backends (file and vault)

Both ways stay supported: an env file is the default and needs nothing else;
a file or Vault keeps the secrets out of it. lb-agent has the same three
backends for its token and TLS key ([AGENTS.md](AGENTS.md#secrets-env-file-file-or-vault)).
What goes through the backend: `JWT_SECRET`, `ORBIT_MASTER_KEY`, `DB_URL`,
`ADMIN_PASSWORD`, `BACKUP_S3_ACCESS_KEY`, `BACKUP_S3_SECRET_KEY` and
`AGENT_CLIENT_KEY`.

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
`ADMIN_PASSWORD`, `BACKUP_S3_ACCESS_KEY`, `BACKUP_S3_SECRET_KEY`,
`AGENT_CLIENT_KEY`) with string values. Rotating one in Vault takes effect
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
| `CORS_ORIGINS` | API | `http://localhost:5173` in `docker-compose.dev.yml`. |
| `VITE_PROXY_TARGET` | Vite dev server | Where `/api` is proxied (`http://api:8080` in the dev container; default `http://localhost:8080`). |
| `VITE_POLL` | Vite dev server | Any value makes Vite poll for file changes (Docker Desktop bind mounts). |
| `LAB_HOST` | `labseed` (`make lab-seed`) | How the dev API reaches the lab ports. Default `host.docker.internal`. |

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
| `orbit import-v0-audit <v0 orbit.db> [--force]` | Appends Orbit v0's audit history to the chained audit log, with the original times, then exits. It uses the same configuration as the server. A second run is refused unless you add `--force`. |


## Settings on the page (not environment)

These live in the database (`settings` table, per-user preferences apart) and
take effect at once, without a restart. Every change is written to the audit
log. **Owner** means the one instance owner; a role can't grant it. Where
"step-up" is marked, the owner must re-enter a password or passkey first when
the `step_up_auth` feature flag is on.

| Setting | Where on the page | Who can change it | Default |
|---|---|---|---|
| **Security policy**: `otp_enforced` (every local user needs two-factor), `hide_local_login` (hide the password form when SSO is on; `/login?local=1` still shows it), `passwordless_enabled` (the "Sign in with a passkey" button), `auto_sso` (start SSO right away on the sign-in page; needs SSO on) | Owner → Security | Owner (step-up) | off, off, **on**, off |
| **SSO (OIDC)**: on/off, issuer, client id, client secret (stored sealed, never shown again), redirect URL, extra scopes, role claim, default role for new users | Owner → SSO | Owner (step-up) | off; default role `none` |
| **Feature flags**: `sessions` (users see and end their sessions), `impersonation`, `step_up_auth`, `weights` (change server weights), `rolling` (rolling actions), `certificates` (the Certificates page, renewals, expiry alerts) | Owner → Owner | Owner (step-up) | all off, except `certificates` (on) |
| **Certificates**: the alert threshold (`cert_warn_days`), the PKI Management URL and token (`pki_url`, `pki_token`, sealed), managed certificates (CA, names, where installed, auto-renew and how many days before) | Proxies → Certificates | `certs.manage` | 30 days; not connected; none |
| **Refresh interval** (how often agents are polled and pages refresh) | Owner → Owner, **Live updates** | Owner | `PROXY_POLL_SECONDS` |
| **Security alerts**: which events alert (impersonation, owner transfer, admin role granted, security/SSO config changed, session token reuse), which channels, which email | Owner → Owner, **Security alerts** | Owner | all off |
| **Owner transfer** | Owner → Owner, danger zone | Owner (step-up) | the first admin |
| **Branding**: app name, accent colour, logo, favicon, show name, show logo, sign-in message | Owner → Branding | Owner, or a role with `settings.branding` | "Orbit" |
| **Developer themes**: `developer_themes_enabled`, `developer_theme` (`blue`, `gold`, `red`, `ink`; anything else is stored as `blue`), and per asset `asset_source_logo` / `_wordmark` / `_favicon` (`theme` or `keep`) | Owner → Branding | Owner, or a role with `settings.branding` | off; `blue`; `theme` |
| **Notification channels**: email, Discord, Telegram, ntfy, Gotify, Microsoft Teams, Slack, signed webhook (stored sealed) | Owner → Notifications | Owner, or a role with `settings.notifications` | none |
| **Automatic backups**: on / off, every N hours, keep N, a directory or an S3 bucket (prefix, endpoint, region) | Owner → Owner → Backups | Owner (step-up) | off |
| **Server events** for the organisation: server down / up, agent unreachable / back, changes, timed maintenance ending, drift and failures; the channels; the email recipients | Owner → Notifications | Owner, or a role with `settings.notifications` | all off |
| Per user: **server alerts by email** for the servers in their scope | Settings → Notifications | the user | off |
| **Proxy registry**: name, label, agent URL, agent token (stored sealed), load balancer URL, public URL | Admin → Proxies | `proxies.manage` | empty (or `PROXIES`) |
| **Environment names** (e.g. `gmr` → "Staging") | Admin → Proxies | `proxies.manage` | none |
| **Change confirmations**: each user chooses, or everyone gets `detailed` or `compact` | Admin → Proxies | `proxies.manage` | each user chooses |
| **A service's last server**: may a target's own button in Operations take a service's last live server (on every proxy) out of rotation? (Group actions and the Proxies page may, after the warning.) | Admin → Proxies | `proxies.manage` | off (refused) |
| **Pending extension requests**: grace minutes, how many times, then back in rotation or keep in maintenance | Admin → Proxies | `proxies.manage` | 30 min, 2 times, back in rotation |
| **Reasons**: every change needs a reason (a timed maintenance always does) | Admin → Proxies | `proxies.manage` | off |
| **Desired state**: restore drifted servers automatically (else only flagged) | Admin → Proxies | `proxies.manage` | off |
| **Agents must use HTTPS**: refuse `http://` agent URLs (can't be turned on while a proxy still has one) | Admin → Proxies | `proxies.manage` | off |
| **Let the identity provider grant admin** (SSO) | Owner → SSO | owner | off |
| Per user: **require two-factor**, unlock, reset password or two-factor | Admin → Users | `users.manage` | off |
| Per user: change confirmation style, theme, personal accent, time zone (the language is kept in the browser) | Settings | the user | detailed |

The owner and delegated roles are explained in [SECURITY.md](SECURITY.md#authorization).

## lb-agent settings

Each proxy host runs **lb-agent** next to nginx or HAProxy. It has its own
environment; install steps are in [AGENTS.md](AGENTS.md).

| Variable | Engine | Default | Notes |
|---|---|---|---|
| `AGENT_TOKEN` | both | none, **required** | Bearer token Orbit must send. The agent refuses to start without it. Use a different one per proxy (`openssl rand -hex 32`), and paste it in Admin → Proxies. Through `SECRETS_BACKEND`. |
| `AGENT_PORT` | both | `9000` | Port of the agent's API. Open it only to Orbit. |
| `AGENT_BIND` | both | all addresses | Address to listen on. |
| `ENGINE` | both | `nginx` | `nginx` or `haproxy`. |
| `NODE_NAME` | both | host name | Name the agent reports. |
| `SECRETS_BACKEND` | both | `env` | `env`, `file` (`AGENT_TOKEN_FILE`, `TLS_KEY_FILE`…) or `vault` (`VAULT_ADDR`, `VAULT_KV_MOUNT`, `VAULT_KV_PATH`, `VAULT_TOKEN` or `VAULT_ROLE_ID` + `VAULT_SECRET_ID`), as for Orbit. |
| `TLS_CERT_FILE` | both | | With a key: serve HTTPS (TLS 1.2+). Reloaded when the file changes. Or `TLS_CERT` (PEM) through the secrets backend. |
| `TLS_KEY_FILE` | both | | The certificate's key. Or `TLS_KEY` (PEM) through the secrets backend, e.g. from Vault. |
| `TLS_CLIENT_CA_FILE` | both | | Mutual TLS: require a client certificate signed by this CA (Orbit's `AGENT_CLIENT_CERT_FILE`). Needs HTTPS. |
| `UPSTREAM_FILES` | nginx | `/etc/nginx/upstreams/*.conf` | Glob of the files holding the `upstream` blocks the agent edits. |
| `ACCESS_LOG` | nginx | `/var/log/lb/access.log` | nginx access log in the `lb` format, tailed for per-server stats. |
| `HEALTH_PATH` | nginx | `/health` | Path the agent calls on each backend server for health. |
| `STUB_STATUS_URL` | nginx | `http://127.0.0.1/nginx_status` | nginx `stub_status` page. |
| `TEST_CMD` | nginx | `nginx -t` | Validates a change before it is applied. |
| `RELOAD_CMD` | nginx | `nginx -s reload` | Applies a change, e.g. `systemctl reload nginx`. |
| `DUMP_CMD` | nginx | `nginx -T` | Prints the full configuration (the config view). |
| `PID_FILE` | nginx | `/run/nginx.pid` | nginx PID file. |
| `AUTO_RELOAD` | nginx | `1` | `1` reloads nginx when someone edits the upstream files by hand. |
| `HAPROXY_SOCKET` | haproxy | `/var/run/haproxy/admin.sock` | HAProxy Runtime API socket. |
| `HAPROXY_CONFIG` | haproxy | `/usr/local/etc/haproxy/haproxy.cfg` | HAProxy configuration file. |
| `HAPROXY_STATE_FILE` | haproxy | `/var/lib/haproxy/server-state` | Where server states are saved so they survive reloads. |
| `HAPROXY_IGNORE` | haproxy | `stats` | Comma list of frontends / backends not to show. |

## See also

- [SECURITY.md](SECURITY.md): what these settings protect, and the operator checklist.
- [reverse-proxies.md](reverse-proxies.md): TLS, `TRUSTED_PROXIES`, cookies behind nginx or HAProxy.
- [SSO-entra.md](SSO-entra.md), [SSO-authentik.md](SSO-authentik.md): filling in Owner → SSO.
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md): start-up errors such as a short secret.
- [ARCHITECTURE.md](ARCHITECTURE.md), [API.md](API.md).
