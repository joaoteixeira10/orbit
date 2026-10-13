# Changelog

All notable changes to Orbit. Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versions follow [SemVer](https://semver.org/). Fill `[Unreleased]` as changes
merge, not at release time.

## [Unreleased]

## [1.1.0] - 2026-10-12

### Added

- **Phone layout** — under 1024 px the sidebar is a drawer behind a menu
  button in the top bar (with the counts and Recent activity), closing on
  navigation, outside tap or Esc; the shell clears the notch and home bar when
  installed. Replaces v0's wrapping bar of links, which hid the activity feed.

### Fixed

- **Installing the app on Android / Chrome** — the branded web manifest was a
  `blob:` URL that the CSP blocked (no `manifest-src`), and its relative URLs
  didn't resolve; there was no static manifest to fall back on. Now: a static
  `/manifest.webmanifest`, `manifest-src 'self' blob:`, absolute URLs, a
  192 px icon next to the 512 px one, and the iOS home-screen title. iPhone
  "Add to Home Screen" already worked (it reads the apple-touch-icon).

## [1.0.0] - 2026-10-11

### Added

- **Orbit v1 foundation on the PKI Management stack** — Go (Gin, GORM, SQLite)
  API with the Vue 3 SPA embedded, one distroless multi-arch image, Docker dev
  loop with live reload (`make dev`), containerized `make` targets, Test / E2E /
  Security / Build & Push / Release workflows. See [docs/BLUEPRINT-FIT.md](docs/BLUEPRINT-FIT.md).
- **Authentication like PKI Management, without certificate login** — local
  users, bcrypt, lockout, rotating refresh tokens with reuse detection, TOTP with
  sealed secrets and recovery codes, passkeys, OIDC SSO with JIT provisioning,
  personal API tokens (`orb_…`), owner, step-up / impersonation / sessions
  behind feature flags, hash-chained audit log.
- **Server-scoped RBAC** — privileges `servers.read / enable / disable / drain /
  weight`, `stats.reset`, `traffic.test`, granted on every server or by **scope
  rules** on server-name parts (`env=gmr`, `env=rda machine=web0*`, …). Built-in
  roles admin / operator / viewer / none.
- **First admin** — `ADMIN_USER`; with `ADMIN_PASSWORD` empty a temporary
  password is printed once to the log; the first sign-in always asks for a new
  password.

- **Proxies, Operations and Audit pages (v0 parity)** — the proxy registry in
  the database (agent tokens sealed), a background poller keeping each proxy's
  last good state, and v0's pages in v0's design: proxy tabs with backends,
  servers, multi-select and the traffic test; Services / Servers cards with
  enable / disable per target, card or environment; a per-proxy preview of every
  change (nginx diff / HAProxy commands), typed-name confirmation for group
  actions, an empty-backend warning and an optional reason kept in the audit log.
- **Limited API tokens** — a token can carry fewer privileges and / or scope
  rules than its owner; admins list and revoke every token.
- **Users and roles as in v0** — require two-factor per user, unlock, generated
  temporary passwords, disable / enable (signs the user out), last sign-in; role
  cards with their members and a live "matches N servers".
- **Coming from v0** — `PROXIES` is imported once into an empty registry, and
  `orbit import-v0-audit` appends v0's audit history to the chained log.

- **Refresh interval setting** — the owner sets how often proxies are polled
  and pages refresh (Owner → Live updates); `PROXY_POLL_SECONDS` is the default,
  10 seconds (changes still show at once: the server pushes them).
- **Compact change confirmations** — each user chooses detailed (the
  per-proxy preview) or compact (what, where, current status); an admin can
  enforce one for everyone (Admin → Proxies).

- **Every change asks first** — one server too (v0 applied a single safe change at once).
- **A target's own button can't take a service's last server out by default** —
  in Operations, a target's Enable / Disable can't take the last live server of a
  service, counted across every proxy and environment, out of rotation (refused
  and audited); group actions (Disable all, typed name) and the
  Proxies page may, after the warning. An admin can allow it for the target
  buttons too (Admin → Proxies → A service's last server).
- **Audit page: Verify integrity and Download CSV** (the whole log or the
  search matches, via `GET /api/v1/audit/export`); the Admin → Audit tab is gone.
- **Group changes named in the log and the feed** ("All GMR · 14 servers").
- **HAProxy Last session and Downtime columns** on the Proxies page (nginx shows
  the last request).
- **Timed maintenance** — Disable / Drain "for 15 min … 24 h" (API: `minutes`);
  it needs a reason, and Orbit puts the servers back in rotation by itself
  when the time is up (logged as `system`; retried if the agent is down).
- **Desired state and drift** — Orbit remembers the state it last set on
  every server; a proxy that reports another one (rebuilt, changed by hand) is
  flagged with **Restore** and **Accept** (`POST /api/v1/reconcile/{restore|accept}`).
  An admin can let Orbit restore it by itself (Admin → Proxies → Desired state).
- **Require a reason for every change** (Admin → Proxies → Reasons, off by default).
- **Run a health check from the proxy** — **Run health check** on the Proxies
  page (⋯, or **Run check** on a failing server), **Check** on down targets in
  Operations, `POST /api/v1/check`, `lbctl check`: lb-agent's new `/v1/check`
  runs the check as HAProxy is configured (`option httpchk`, `http-check
  send/expect`, `default-server`, `check port`, `check-ssl verify none`, …) or
  nginx's, and shows the command, the answer and its body. **No check**: a
  count on Operations, a filter and a tag for servers without a health check.
  lb-agent now has four files (`healthcheck.py`).
- **Latency and history** — p50 / p95 response time per server and backend
  (nginx, from the access log; HAProxy's average), a **Last hour** requests
  sparkline and a **p95** column on the Proxies page (24 h of per-minute
  history, `GET /api/v1/history`), proxy metrics on `/metrics`, and a
  monitoring kit in `deploy/` (Prometheus + Grafana in one command, the
  dashboard, 7 alert rules).
- **Maintenance requests, approvals and extensions** — new privilege
  `maint.approve`; **Request…** / **Extend…** on Operations (and the ⏱ tag on
  Proxies), a **Requests** panel with Approve / Deny / Withdraw and a sidebar
  badge; approvals can pre-approve extensions; bounded grace for extension
  requests still pending when the window ends (Admin → Proxies); emails to
  approvers and requesters; `lbctl request`, `extend`, `requests`, `approve`,
  `deny`.
- **Rolling actions** (feature flag `rolling`, off) — **Roll…** on an Operations
  card, `POST /api/v1/rollouts`, `lbctl roll` / `rollout --until waiting` /
  `next`: one server at a time (every proxy it is on) — out (draining first
  where it can), wait for Next or N minutes, back, healthy, next; stops if a
  step is refused or a server doesn't come back.
- **Weights / canary** (feature flag `weights`, off) — a backend's **Weights…**
  on the Proxies page (with each server's share of new traffic), `POST
  /api/v1/weights`, `lbctl weight`; lb-agent's new `/v1/weights` (HAProxy live
  0–256, nginx `weight=` 1–1000 with a reload, all or nothing).
- **Encrypted backups** (from PKI Management) — Owner → Owner → Backups: download
  the whole database as one archive (sealed with ORBIT_MASTER_KEY or a passphrase),
  automatic backups to a directory or S3 (every N hours, keep N), restore with
  `orbit restore-backup` with Orbit stopped.
- **Scheduled changes** — the change dialog's **Later, at** (and `lbctl … --at
  02:00`, `POST /api/v1/schedules`): made at that time as its author, with
  their rights then and the same checks; listed with Cancel on Operations and
  Proxies; failed / missed ones are notified.
- **Change freezes** — Admin → Proxies: from / until / reason / optional scope
  rules; changes on the servers covered are refused (423) except by admins; a
  banner for everyone.
- **Server notifications** — Owner → Notifications → Server events: server
  down / back up (flap-guarded), agent unreachable / back, changes (who, why,
  until when — users, lbctl and Orbit's own timers / drains / restores),
  a timed maintenance ending in 15 minutes, drift and failures; to the team
  channels, an email list and the signed webhook. Users can get them by email
  for the servers in their scope (Settings → Notifications). New channels:
  **Microsoft Teams** (Workflows, Adaptive Card) and **Slack**.
- **`lbctl`** — the command-line tool for deploy pipelines (docs/LBCTL.md):
  `lbctl disable gmr_web01_facebook --reason … --for 20m --drain-first --wait`,
  `enable`, `drain`, `status`, `restore`, `accept`, `--dry-run`; release
  binaries for linux / macOS, and `/lbctl` in the image.
- **Drain, then maintenance** (HAProxy) — Disable with "Drain first" (or the ⋯
  menu's Drain → maintenance; API `drain_first`, `drain_timeout`): drain now,
  maintenance by Orbit once the sessions are gone or the wait is up; nginx
  goes to maintenance at once.
- **Developer themes and the Konami egg** (from PKI Management) — Owner →
  Branding: four curated themes (Steel Blue, Gold Leaf, Blood Red, Ink) lock the
  accent and bring the UBonito proxy crest, the "UBonito" wordmark and the
  favicon, each of which can be kept as your own; Ink follows light / dark.
  ↑↑↓↓←→←→BA shows who built it.
- **New default mark** — the neutral proxy crest replaces the "lb" tile and the
  shield favicon (sidebar, sign-in page, favicon, home-screen and install icons).

- **Documentation for v1** — user guide, configuration, security, agents,
  reverse proxies, SSO with Entra ID and Authentik, API, troubleshooting,
  architecture and releasing, under `docs/` with the blueprints.
- **Tests** — end-to-end against a real nginx / lb-agent lab (`make e2e-lab`,
  workflow "E2E (lab)"), and a restart test for data, sessions and sealed secrets.
- **LICENSE** — Apache-2.0.
- **Certificates on the proxies** (feature flag `certificates`, on by default)
  — **Proxies → Certificates** lists what every proxy (nginx `ssl_certificate`,
  HAProxy `crt`) and agent serves, with days left, issuer and where it's used.
  **Replace…** installs an uploaded certificate through the agents: checked
  (key, validity, names, chain), previewed, then nginx `nginx -t` + reload
  (put back if refused) or HAProxy `set` + `commit ssl cert` (live), the old
  file kept as a hidden `.prev`. **From PKI…** has PKI Management sign a CSR
  made by Orbit (the key never leaves Orbit and the agents) and can renew
  it automatically. Expiry alerts (threshold, 7, 1, 0 days), the
  **Certificates** server event, `lb_cert_expiry_timestamp_seconds`, the
  `LBCertificateExpiring` rule, `lbctl certs`, privileges `certs.read`
  (operator, viewer) and `certs.manage`. The lab serves TLS on gmr (`:8441`)
  and gmr-hap (`:8446`).
- **Agent TLS, optional per agent** — lb-agent serves HTTPS with
  `TLS_CERT_FILE` + `TLS_KEY_FILE` (reloaded when the files change) and
  requires a client certificate with `TLS_CLIENT_CA_FILE`; plain HTTP keeps
  working. Orbit follows each agent URL's scheme, trusts `AGENT_CA_FILE`
  besides the system CAs and sends `AGENT_CLIENT_CERT_FILE` /
  `AGENT_CLIENT_KEY_FILE` when asked. **Admin → Proxies → Agents must use
  HTTPS** (off) refuses `http://` agents once none is left; a lock marks HTTPS
  agents. TLS failures say which setting fixes them. The token stays required.
- **Agent secrets from a file or HashiCorp Vault** — lb-agent reads
  `AGENT_TOKEN` (and `TLS_KEY`) through `SECRETS_BACKEND=env|file|vault`, like
  Orbit; Orbit's agent client key can come from Vault too
  (`AGENT_CLIENT_KEY`). The env file stays the default.

### Changed

- **API reference: Scalar instead of Swagger UI** — at `/api/docs`, served from
  Orbit itself (no CDN), in Orbit's colours; the spec is
  `/api/v1/openapi.json` and the old `/api/v1/swagger/…` address redirects.
- **The six-box code input everywhere a code is typed** — sign-in, confirming a
  new authenticator, regenerating recovery codes and turning two-factor off
  (now a small dialog); the code is sent on the sixth digit.
- **v0's design, not PKI Management's** — colours, fonts, controls, sidebar and
  sign-in card from v0; only PKI's six-box code input is kept.
- **v0 retired** — the Python admin and the lb-idp sign-in service are removed
  (tag `v0-python`); lb-agent lives in `agent/`, the demo proxies in `lab/`.
- **lb-agent in Go** — one static binary (`api/cmd/lb-agent`, linux amd64 /
  arm64 on releases, `/lb-agent` in the image, `make lb-agent`) with the same
  API, environment variables and behaviour as the Python agent; the lab and the
  E2E tests run it.

### Removed

- **The Python lb-agent** (`agent/`, its tests, `make agent-test`, the CI job,
  `python3` in the lab images). Replace any still running with the Go binary;
  Orbit tells you ("update it to the Go lb-agent") where it matters. The
  lab's start scripts moved to `lab/shared/`.
- The update check compares versions by semver (PKI Management compared for
  inequality only).

### Fixed

- **Built with Go 1.26.9** (was 1.25.13, which is out of support) and
  `golang.org/x/net` v0.60.0: fixes 12 standard-library vulnerabilities
  reachable from Orbit, lbctl and lb-agent (HTTP/1 and HTTP/2 server and
  client, MIME header limits, TLS ECH, `html/template`), e.g. GO-2026-6617.
- **Vue 3.5.43 and source-map-js 1.2.2** in the page: fix GHSA-g2v6-rqmx-r4w6
  (XSS in `@vue/server-renderer`) and GHSA-68fv-2mgg-jv7q (source-map DoS).
- **Stricter account rules** — nobody can act on the owner's account (the
  owner uses Settings), and only the owner can act on an admin's: admins can no
  longer reset, disable, sign out, edit, unlock, delete or revoke tokens of each other.
- **SSO can't make admins** unless the owner allows it (Owner → SSO → Let the
  identity provider grant admin; off by default).
- **Ended sessions stop at once** (sign-out, Active sessions, token reuse) instead
  of after up to 15 minutes; **failed sign-ins are audited**; **lockout applies to
  passkey sign-in** too; agent / load-balancer requests refuse link-local and
  cloud-metadata addresses.
- **The owner's and admins' accounts are protected** — `users.manage` alone can
  no longer reset, sign out, edit or delete the owner or an admin (and an admin
  can't do it to the owner), which was a way to take over a stronger account.
- **An API token can't create API tokens** — a limited token could mint an
  unlimited one.
- **Notification secrets sealed at rest** — the SMTP password, webhook URLs,
  bot tokens and the webhook signing secret were stored in plain text; they're
  now sealed with `ORBIT_MASTER_KEY` (an existing configuration is sealed at start-up).
- **`orbit healthcheck` with in-app TLS** — it probed plain HTTP, so the
  container showed as unhealthy.
- **Data volume writable by the non-root user** — the image ships an empty
  `/data` owned by `nonroot`, so SQLite can create its database on a fresh
  volume.

## v0 (Python) — tag `v0-python`

The first version: Python admin page, lb-agent, lb-idp sign-in service, docs.
It keeps running until v1.0 reaches parity. See the v0 section of
[BACKLOG.md](BACKLOG.md).
