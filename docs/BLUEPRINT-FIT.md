# How the blueprints apply to Orbit

[PROJECT-BLUEPRINT.md](PROJECT-BLUEPRINT.md) and [AUTH-BLUEPRINT.md](AUTH-BLUEPRINT.md)
come from the canonical copy in `../blueprint` (first copied from LB Admin,
2026-10-13; synced with `../blueprint/sync.sh docs`); they describe PKI
Management, the reference implementation. This page records what
Orbit **adopts** (the code was ported), **adapts** (same design, Orbit's
domain), **skips**, and what it **adds**.

**Decided** (2026-10-13):

- Orbit is built on the same stack — **Go (Gin, GORM, SQLite) + Vue 3 (Vite,
  Pinia, Tailwind)** — starting from LB Admin's code with the proxy domain
  removed.
- **Multi-user, self-hosted.** Accounts and sign-in work as in the blueprint
  (passwords, two-factor, passkeys, OIDC SSO). People share workspaces by
  membership.
- **Workspace content is private to its members.** Instance roles (admin,
  member) manage accounts, never content: an admin can't open someone's
  workspace. This replaces LB Admin's server-scoped grants.
- **Notion-style block pages** (TipTap / ProseMirror JSON), **Reminders-style
  tasks**, **kanban boards** over the same tasks, **calendars** with ICS
  subscriptions in and private ICS feeds out.
- **Installable now, offline later**: the blueprint's "no service worker" holds
  for v0.x; an offline cache with a sync queue is on the roadmap.
- **English and European Portuguese**, with the parity test.
- **An Apple look**: the system font, Apple's system colours, translucent
  sidebar material, sheets, segmented controls, round checkboxes. The
  developer themes (blackletter crests) don't fit it and are not carried over.

## PROJECT-BLUEPRINT

| § | Item | Decision | Notes for Orbit |
|---|---|---|---|
| 1 | Everything in Docker, `make` wraps containers | **Adopt** | Dev stack on host ports 8090 / 5180 so it runs next to LB Admin's |
| 1 | One artifact: the binary embeds the SPA | **Adopt** | |
| 1, 4 | Config from env, secrets provider (env / file / Vault), release-mode checks | **Adopt** | New: `PUBLIC_URL` (calendar-feed addresses) |
| 1, 5.3 | Runtime policy in `settings` | **Adopt** | |
| 2 | Repository layout | **Adopt** | New packages: `planner` (domain rules, ICS, recurrence), `calsync` (subscriptions) |
| 3 | Dockerfile, compose files | **Adopt** | No companion binaries |
| 5.1 | UUIDv7 keys | **Adopt** | |
| 5.2 | Auto-migrate + idempotent seeds | **Adopt** | Built-in roles: `admin`, `member`, `none` |
| 6.1 | `/api/v1`, `{"error"}`, swaggo OpenAPI, Scalar | **Adopt** | ~45 planner routes |
| 6.2 | Middleware stack | **Adopt** | CORS allows `X-Orbit-Client` |
| 6.3 | Logs, `/metrics`, health | **Adopt** | Domain gauges counted at scrape time |
| 6.4 | cron scheduler | **Adopt** | Calendar refresh (5 min), reminders (each minute), trash (daily) |
| 6.5 | Hash-chained audit | **Adapt** | Security and administration events, workspace creation / membership / deletion, calendar subscriptions and feeds. Content edits are *not* audited (they'd flood the chain) |
| 6.6 | SSE live updates | **Adapt** | `{"type":"ws","ws","what","origin"}` to every member; the tab that made the change ignores its own (`X-Orbit-Client`) |
| 6.7 | Notifications | **Adapt** | Personal reminders through the channels each user picks; no organisation-wide event broadcast |
| 6.8 | Encrypted backups | **Adopt** | |
| 7 | Owner, feature flags | **Adopt** | Flags: sessions, impersonation, step-up (off); calendar subscriptions, calendar feeds (on) |
| 8 | Version, update check, About | **Adopt** | |
| 9.1 | Org branding | **Adopt** | |
| 9.2 | Developer themes | **Skip** | Doesn't fit the Apple look; the backend keys are unused |
| 10 | Vue shell, tokens, dark mode, prefs, i18n | **Adapt** | New shell (sidebar with workspaces, ⌘K palette, phone tab bar). Prefs add week start, time format, the device's time zone (for reminders) |
| 10.1 | Mobile + installable | **Adopt** | Plus an iOS-style bottom tab bar on phones |
| 11 | Tests | **Adopt** | Planner unit tests (recurrence across DST, ICS import/export), full-router tests of the planner API, Vitest |
| 12, 13 | CI, releases | **Adopt** | Start at `v0.1.0` |
| 14 | Docs | **Adapt** | User guide rewritten for the planner; this page; [ROADMAP.md](ROADMAP.md) |

## AUTH-BLUEPRINT

| § | Item | Decision | Notes |
|---|---|---|---|
| 0–6 | Tokens, refresh rotation, CSRF, passwords, TOTP, passkeys, OIDC | **Adopt** | As LB Admin (no client-certificate login) |
| 5.5 | API tokens | **Adapt** | Narrowed by instance privileges and/or **read-only** (can read workspaces, not change them). Scope rules are gone |
| 7 | RBAC | **Adapt** | Instance privileges only: `users.manage`, `roles.manage`, `audit.read`, `workspaces.create` + the owner-gated three. Workspace roles (owner / editor / viewer) live on the membership |
| 7 | Owner | **Adopt** | |
| 8 | Step-up, impersonation, sessions | **Adopt, flags off** | Impersonation shows the impersonated user's workspaces — keep it off unless you need it |
| 12 | SSE ticket | **Adopt** | |

## Orbit's own additions

- **Workspaces with chosen modules**, from templates (School, Work, Personal,
  Shared, Blank) that also seed localised lists and pages.
- **Membership roles** and the rule that the last owner stays an owner;
  deleting an account hands shared workspaces to another member and deletes
  the ones it was alone in.
- **Optimistic page versions** (409 with the newer page) instead of silent
  last-write-wins.
- **ICS import hardening**: SSRF-guarded fetches (redirects followed hop by
  hop through the same guard), size and time limits, Windows time-zone names,
  edited occurrences (`RECURRENCE-ID`), cancelled events, sealed addresses.
- **Floating due dates** for tasks (like Apple Reminders) and **zoned
  recurrence** for events (a weekly 09:00 stays 09:00 across DST).
