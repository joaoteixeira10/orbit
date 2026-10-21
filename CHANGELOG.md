# Changelog

All notable changes to Orbit. Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versions follow [SemVer](https://semver.org/). Fill `[Unreleased]` as changes
merge, not at release time.

## [Unreleased]

### Added

- **Workspaces with their own modules** — create one from a template
  (School, Work, Personal, Shared, Blank), choose its modules (Overview,
  Notes, Pages, Tasks, Board, Timeline, Charts, Calendar), colour and icon;
  start with localised example lists, a welcome page and a first note.
  Archive or delete it; reorder the sidebar.
- **Sharing** — owners invite accounts by username or email as owner, editor
  or viewer. Content is private to members (others, admins included, get
  404). The last owner can't leave; deleting an account hands shared
  workspaces to another member.
- **Pages** — stored as GitHub-flavoured Markdown, read like a README
  (tick checkboxes while reading), edited in a Notion-like visual editor
  (`/` commands, checklists, tables, quotes, code, callouts as GitHub alerts,
  a selection toolbar) or as Markdown source — each person's choice, on the
  same page. Code blocks are highlighted (in their fence's language, or one
  detected among about 30), with a language picker in the visual editor and a
  copy button in the reading view. Download or copy a page as `.md`, import `.md` files, export a
  workspace as a zip of `.md` files in its page tree. Nested pages,
  favourites, duplicate, move, trash (emptied after 30 days). Saves carry a version, so a stale
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
- **Notifications** — a bell with everything that concerns you: reminders,
  tasks given to you, being added to a workspace (or your role changed,
  removed), other members' activity in shared workspaces (a burst folded into
  one, mutable per workspace) and an optional **daily summary** at your time.
  **Web Push** brings them to your devices even with Orbit closed (iPhone and
  iPad: from the Home Screen app); per kind you choose devices and/or the
  owner's channels (email, ntfy, Telegram, …). Push and channel messages are
  written in your language.
- **Today** and per-workspace **Overview**; **search** and a **⌘K command
  palette** across workspaces.
- **Quick notes** — a Notes module of sticky notes: jot a text or a
  checklist on a coloured card, pin what matters, reorder by dragging, tick
  items on the card, turn a note into a page or into tasks; undo a delete.
  Pinned notes show on Today and on the overview; notes are searchable.
- **Quick Notes** — your own notes, in no workspace and seen by no one else:
  jot one on Today, with ⌥N / Alt+N anywhere, or from ⌘K; later move it into
  a workspace or turn it into a page or tasks there.
- **Timeline** — a Gantt chart of a workspace's tasks, by list: tasks have
  a **start date** now, bars run from start to due; drag a bar to move it or
  its ends to change the dates, click an empty row to schedule a task;
  weeks, months or quarters.
- **Charts** — Microsoft Planner-style: status (not started, in progress,
  late, completed), tasks by list, priority and person, and completed vs
  created per week.
- **Task filters and grouping** — filter by keyword, due (overdue, today,
  this week, next week, later, no date), progress, priority, person, tags
  and list; one filter per workspace, shared by Tasks, Board, Timeline and
  Charts. Tasks group by status, due date, priority or person.
- **Git (Gitea / Forgejo)** — connect your account on a server with a
  personal access token (Settings → Git; sealed at rest, each person their
  own). Make a task list from a repository: its issues as open / closed, with
  labels as columns, or one of its project boards (servers whose API has
  them) — kept in step both ways. Titles, descriptions, due dates, labels
  (as tags), the assignee and the column reach the issue at once, as you;
  changes on git come back every few minutes or with **Sync now**. New tasks
  open issues; deleting a task leaves its issue alone. Any task can also be
  linked to one issue or pull request by its address: closing the issue (or
  merging the pull request) completes the task, and completing it closes the
  issue. A change git refuses is kept and sent again. The `git_integration`
  feature flag turns it off; `GIT_SERVERS` allows servers on your own
  network. GitHub is planned.
- **Developer themes and the Konami egg** (as in PKI Management and LB
  Admin) — Owner → Branding: four curated themes (Steel Blue, Gold Leaf,
  Blood Red, Ink) lock the accent and bring the UBonito Orbit crest, the
  "UBonito" wordmark and the favicon, each of which can be kept as your own;
  Ink follows light / dark. ↑↑↓↓←→←→BA shows who built it. Text on the accent
  now follows the theme (dark on Ink's bone, not white). The themes' code is
  the shared kit in `../blueprint/shared/dev-themes`, the same as in LB Admin
  and PKI Management; `make crests` bakes the crests from `../assets`.
  A fifth theme, **Forest Green**. **Each person can pick their own** theme
  (or none) in Settings → Appearance when the owner allows it. Links and
  titles use a readable shade of the theme's colour (4.5:1 on the page, in
  light and dark). The **sign-in page** takes the theme's dark background. The
  crests come as a full icon set: a square one for the iPhone / iPad home
  screen (no black corners) and 192 / 512 / maskable icons for installing.
- **Images and files** in pages (paste, drop, or **/ Image or file** in either
  editor), **attachments on tasks** and **images and files on notes** (the
  first image on the card). Kept in the database, so backups carry them; only
  the workspace's members can open them; images show inline, other files
  download. The page export zips them with the pages. `UPLOAD_MAX_MB` (25)
  and `UPLOAD_QUOTA_MB` (2048).
- **Sums in notes**, like iPhone's Math Notes: a line ending in "=" shows its
  answer (Tab writes it in); named values (`rent = 650`), percentages,
  decimal commas and currency signs. Cards show the answers too. Also in
  pages (reading view, visual editor and Markdown source) and task notes.
- **Calendar side panes, as in Outlook**: on the left a small month to move
  around and the calendars, each ticked to show or hide it (workspaces; in a
  workspace its events, tasks and each subscription); on the right the
  agenda of the next two weeks and the open tasks, which drag onto a day or
  an hour to plan them.
- **Public holidays on the calendar**: a workspace picks a country
  (Portugal, Spain, France, Italy, Germany, UK, Netherlands, Belgium, US,
  Brazil); its national holidays show as all-day entries, computed from
  their rules.
- **Exams and course pages**: mark a task as an exam and it counts down —
  in lists, as its course's next exam, in an Exams card on the overview;
  each course gets its own notes page in one click.
- **Workspace activity feed** on the overview: who added, completed,
  commented, joined — runs of the same thing as one line, kept 90 days.
- **Full-text search**: ranked results, accents ignored ("acao" finds
  "ação"), words found by their start, task comments included (they open
  their task); an SQLite FTS5 index kept up to date by triggers.
- **"Sign in with Gitea / Forgejo"**: the owner registers Orbit as an
  OAuth2 application on a server once; then people connect their own account
  with one click (no token to make or paste). Tokens sealed and renewed.
- **Git: nobody's change goes as someone else.** A change to a linked task
  is sent with its author's own account; members without one can only read
  the list's tasks — unless whoever linked it gives the list a **bot
  account** (Edit list → Members without their own git account), whose name
  then appears on git (comments signed with theirs). Failed changes are
  retried as their author.
- **Your own notification channels**: each person adds their own Discord,
  Slack or Teams webhook, Telegram chat, ntfy topic or Gotify app (Settings →
  Notifications); theirs alone, so they get the full text. Public https only,
  tested before they're kept, stored sealed. Flag `personal_channels` (on),
  with the kinds people may add chosen by the owner.
- **Shared notification channels get a ping, not the content.** Slack,
  Teams, Discord, Telegram, ntfy and Gotify — places the owner set up, which
  others may read — receive "Orbit: a reminder for you" and a link unless the
  person turns on **Include titles in shared channels**; email (the
  person's own) keeps the full text.
- **Comment counts** on task rows and board cards (💬 2).
- **Chips on task rows**, as in Plane: the date, priority and assignees at
  the end of each row (wide screens), each changed right there.
- **A task's activity** among its comments: created, renamed, dated,
  prioritised, assigned, moved, status, completed, reopened, notes edited
  (hide or show it).
- **Comments follow the git issue** on linked tasks, both ways: an Orbit
  comment goes to the issue (as its author with an account there, else
  signed "Name (in Orbit)" by the linker); the issue's comments come in, also
  edits and deletions.
- **Comments on tasks**, as in Planner: Markdown, by any member (viewers
  too), with @mentions (suggested as you type); the assignees, the creator
  and earlier commenters are told (one inbox entry per task while unread),
  the mentioned are told they were. A new notification kind, **Comments and
  mentions**. Authors edit theirs; owners can delete any.
- **Several assignees on a task** ("For" is now **Assignees**): each gets
  the reminders and the "assigned you" notification; filters, grouping and
  charts count each one; git sync carries all of them. The API takes
  `assignee_ids` (`assignee_id` still works, as the first one).
- **Clicking a task opens its details** (as in Plane); it's renamed there.
- **Task notes in Markdown** — rendered in the task's details, with
  checklists that tick; a click edits the source. Lists and boards show the
  first line as plain text.
- **Courses** (module) — subjects with teacher, room, colour and their
  weekly classes, shown as the week's timetable. Every class is a repeating
  event on the calendar (and in Today and calendar feeds) for the term;
  cancel one class from the calendar, change the rest in the course. Tasks
  tagged with the course's tag are its tasks.
- **Grades** (module) — your own marks per course (nobody else sees them),
  out of anything, with weights; weighted course and overall averages on the
  workspace's scale (20, 100, 10, 5); a goal per course and **What do I
  need?** for the next mark. Both modules are on in the School template.
- **Invite links** — a workspace's owners make a link (editor or viewer, 1
  to 30 days, one or more uses), shown once; whoever opens it joins with one
  click, or makes an account right there (not when the instance signs people
  in only with SSO). Links can be revoked and stop when their maker is no
  longer an owner; owners are told who joined. Flag `workspace_invites` (on).
  A sign-in that goes through SSO now comes back to where it was going (the
  link, say).
- **Read-only API tokens.**
- **Apple-style interface** — system font and colours, translucent sidebar,
  sheets, segmented controls, round checkboxes, light and dark; installable,
  with a bottom tab bar on phones. English and European Portuguese.
- **Metrics** — `orbit_users`, `orbit_workspaces`, `orbit_pages`,
  `orbit_tasks_open`, `orbit_notes`, `orbit_calendar_subscriptions_failing`;
  a Grafana dashboard and alert rules in `deploy/`.

### Changed

- Built on LB Admin's code (the blueprint stack): accounts, sign-in (password,
  two-factor, passkeys, SSO), roles, owner, feature flags, audit log, backups,
  notifications. Instance roles are now `admin`, `member` and `none`, and hold
  only instance privileges (`workspaces.create` is new); the server-scoped
  grants and scope rules are gone. See [docs/BLUEPRINT-FIT.md](docs/BLUEPRINT-FIT.md).

- **The integration webhook carries the audit events** (`X-Orbit-Event:
  audit.<action>`): accounts, roles, workspaces and their members, settings —
  never workspace content. See [docs/API.md](docs/API.md#live-updates-and-the-webhook).

### Security

- **Calendar subscriptions can't stall the background jobs**: a feed's
  repeating events are expanded within a step budget (per series and per
  feed), and series repeating every minute or second are skipped.
- **Calendar-feed addresses stay out of the request log** (`/ical/…`).
- **Two-factor can't be removed by setting it up again**: setup is refused
  while it's on; turning it off still needs a code.
- **Impersonation leaves nothing behind**: while acting as someone, new
  sessions, API tokens, password and two-factor changes, passkeys, git
  accounts, push devices and calendar feeds are refused.
- **Password re-checks are throttled**: "confirm it's you" and changing your
  password go through the sign-in rate limit and count toward the account
  lockout, which they respect.
- **"Sign out everywhere" and an admin password reset revoke API tokens** too.
- **Images from other sites wait for a click** in pages (reading view and
  visual editor): they'd tell their site who reads the page, and when.
  Settings → Appearance → Images from other sites: Ask first / Always show.
  Fixed on the way: a page with an image on a line of its own couldn't be
  opened in the visual editor.
- **Read-only API tokens are read-only everywhere** (every write is refused,
  admin routes included).
- **Web Push and git servers can't be used to probe the LAN**: they go through
  a public-only guard (private networks refused; git servers on your network
  only when listed in `GIT_SERVERS`).
- **A linked git list stops using a departed member's token**: when whoever
  linked it leaves the workspace, it stops syncing and says why.
- Sign-in tokens are accepted only as HS256; HSTS is sent when Orbit serves
  TLS itself; the user lookup (adding members) is rate-limited; `go.mod`
  pins the Go toolchain (1.26.9).

### Removed

- LB Admin's proxy domain (proxies, lb-agent, lbctl, certificates, the lab)
  and the developer themes.
