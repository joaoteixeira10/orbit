# Changelog

All notable changes to Orbit. Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versions follow [SemVer](https://semver.org/). Fill `[Unreleased]` as changes
merge, not at release time.

## [Unreleased]

### Added

- **Workspaces with their own modules** — create one from a template
  (School, Work, Personal, Shared, Blank), choose its modules (Overview,
  Pages, Tasks, Board, Calendar), colour and icon; start with localised
  example lists and a welcome page. Archive or delete it; reorder the sidebar.
- **Sharing** — owners invite accounts by username or email as owner, editor
  or viewer. Content is private to members (others, admins included, get
  404). The last owner can't leave; deleting an account hands shared
  workspaces to another member.
- **Pages** — a block editor (TipTap): `/` commands, checklists, tables,
  quotes, code, a selection toolbar; nested pages, favourites, duplicate,
  move, trash (emptied after 30 days). Saves carry a version, so a stale
  editor gets a 409 with the newer page instead of overwriting it.
- **Tasks** — lists with due date and time (floating, like Reminders),
  repeats (completing one moves it to its next date), priority, tags,
  sub-tasks, assignee; smart lists Today, Scheduled, All, Completed; quick add
  parses dates, times, `!!` and `#tags` in English and Portuguese.
- **Boards** — each list's columns are its statuses; drag cards between
  columns (a done column completes the task), edit columns per list.
- **Calendar** — month, week and day views; events with repeats (zoned, DST-safe),
  reminders, all-day and multi-day; delete one occurrence of a series; due
  tasks shown and checkable; a merged calendar of every workspace.
- **Calendar subscriptions** — subscribe a workspace to an ICS address
  (webcal/https); fetched at once, refreshed on a timer, address stored sealed,
  SSRF-guarded. Handles Outlook time zones, edited and cancelled occurrences.
- **Calendar feeds** — a private ICS address of your events and due tasks for
  Apple Calendar, Google Calendar or Outlook; revocable.
- **Reminders** — tasks with a time and events with a reminder notify open
  tabs (and system notifications when allowed) and, if chosen, the
  instance's channels (email, ntfy, Telegram, …).
- **Today** and per-workspace **Overview**; **search** and a **⌘K command
  palette** across workspaces.
- **Read-only API tokens.**
- **Apple-style interface** — system font and colours, translucent sidebar,
  sheets, segmented controls, round checkboxes, light and dark; installable,
  with a bottom tab bar on phones. English and European Portuguese.
- **Metrics** — `orbit_users`, `orbit_workspaces`, `orbit_pages`,
  `orbit_tasks_open`, `orbit_calendar_subscriptions_failing`; a Grafana
  dashboard and alert rules in `deploy/`.

### Changed

- Built on LB Admin's code (the blueprint stack): accounts, sign-in (password,
  two-factor, passkeys, SSO), roles, owner, feature flags, audit log, backups,
  notifications. Instance roles are now `admin`, `member` and `none`, and hold
  only instance privileges (`workspaces.create` is new); the server-scoped
  grants and scope rules are gone. See [docs/BLUEPRINT-FIT.md](docs/BLUEPRINT-FIT.md).

- **The integration webhook carries the audit events** (`X-Orbit-Event:
  audit.<action>`): accounts, roles, workspaces and their members, settings —
  never workspace content. See [docs/API.md](docs/API.md#live-updates-and-the-webhook).

### Removed

- LB Admin's proxy domain (proxies, lb-agent, lbctl, certificates, the lab)
  and the developer themes.
