# Architecture

How Orbit is built and why. Orbit follows the project blueprint
([PROJECT-BLUEPRINT.md](PROJECT-BLUEPRINT.md), [AUTH-BLUEPRINT.md](AUTH-BLUEPRINT.md));
[BLUEPRINT-FIT.md](BLUEPRINT-FIT.md) lists where it differs. This page covers
Orbit's own parts: the planner.

## Goals

1. **One planner for every part of life**, split into workspaces that each
   have only the modules they need.
2. **Private by default, shared on purpose.** A workspace belongs to its
   members; instance administration never grants access to content.
3. **Feels like an Apple app** on a laptop and on a phone.
4. **One small artifact** (a Go binary with the app embedded, SQLite on a
   volume), self-hosted, nothing else to run.

## Stack and layout

Go 1.26 · Gin · GORM + SQLite (pure Go) · Vue 3 + Vite + Pinia + Tailwind 3 ·
TipTap 3 (ProseMirror) · SortableJS · `emersion/go-ical` + `teambition/rrule-go`.

| Path | What |
|---|---|
| `api/internal/models/planner.go` | Workspaces, members, pages, favourites, task lists, tasks, events, subscriptions, external events, feeds |
| `api/internal/planner/` | Domain rules without HTTP: module catalog and templates, board columns, ranks, recurrence, ICS import/export, page text, the welcome page, Portuguese names |
| `api/internal/calsync/` | Fetching subscribed calendars (guarded client, limits, sealed addresses) |
| `api/internal/caldav/` | Two-way CalDAV sync: a small client (discovery, listing, multiget, conditional PUT / DELETE) and the sync (ETags and stamps, later change wins) |
| `api/internal/transfer/` | A workspace as a zip and back (export / import between instances) |
| `api/internal/api/handlers_{workspaces,pages,tasks,calendar,feeds,home}.go` | The planner API |
| `api/internal/jobs/planner.go` | Calendar refresh, reminders, trash |
| `web/src/api/planner.ts` | The typed client |
| `web/src/stores/{workspaces,pages,tasks}.ts` | Client state, kept fresh by live updates |
| `web/src/components/AppShell.vue` | Sidebar, ⌘K palette, sheets, phone tab bar |
| `web/src/views/` | Home, Tasks, Board, Calendar, Pages, Page, Trash (+ the account and admin views) |
| `web/src/styles/main.css` | The design system: tokens and component classes |

## Data model

```
Workspace ─┬─ WorkspaceMember (user, role: owner|editor|viewer, sidebar position)
           ├─ Page (parent → Page, Markdown, plain text, rank, version, trashed_at)
           │    └─ PageFavorite (per user)
           ├─ TaskList (columns JSON: [{id, name, color, done}])
           │    └─ Task (column, parent → Task, due date + time, repeat, rank, completed_at, assignee)
           ├─ Event (starts/ends UTC, all_day, tz, rrule, until, exdates, remind_minutes)
           └─ CalendarSubscription (sealed URL) ─ ExternalEvent (expanded occurrences)
User ─ CalendarFeed (token hash, which workspaces, include tasks)
```

- **Modules** are a JSON list on the workspace (order = sidebar order). The
  catalog lives in code; "soon" modules are listed but not selectable.
- **Columns are statuses.** A task's completion follows its column's `done`
  flag; the Tasks view groups by column, the Board shows them side by side,
  so both views are the same data.
- **Ranks** are floats spaced 1024 apart; a move takes the midpoint of its
  neighbours. Lists and workspaces are renumbered on explicit reorder.
- **Due dates are floating** (`2026-10-20`, `14:30`), as in Apple Reminders:
  "Thursday at 9" means 9 wherever you are. Reminders place them in the
  recipient's zone (their setting, else the zone their browser last reported).
- **Events are instants** plus the zone they repeat in, so a weekly 09:00
  stays 09:00 across daylight saving. `until` (computed on save) bounds range
  queries; `exdates` remove single occurrences.
- **Page versions**: saving content needs the version the editor started
  from; a mismatch is a 409 carrying the newer page.

## Access

Every planner handler loads the resource, then calls `wsAccess(ws, need)`:

1. not a member → **404** (ids reveal nothing);
2. a write through a **read-only API token** → 403;
3. role below `need` (viewer < editor < owner) → 403;
4. a write in an **archived** workspace → 409.

Instance roles (`admin`, `member`, `none`) only gate administration and
`workspaces.create`. Creating a workspace makes you its owner. The last owner
can't leave or be demoted; deleting an account deletes the workspaces it was
alone in and promotes another member elsewhere (`releaseWorkspaces`).

## Live updates

After a change the API publishes `{"type":"ws","ws":…,"what":"pages|tasks|lists|events|subscriptions|members|workspace","origin":…}`
to every member's SSE streams. `origin` is the `X-Orbit-Client` header of the
request (a per-tab id), so the tab that made the change skips the refetch.
The stores refetch what changed. Reminders arrive the same way
(`{"type":"reminder"}`).

## Calendars

- **Range queries** (`/calendar`, `/workspaces/:ws/calendar`, ≤ 120 days)
  expand series with rrule-go, add subscribed occurrences and due tasks.
- **Subscriptions** are fetched at once on creation (a bad address is a
  400 with the reason), then every `refresh_minutes` by the scheduler. A
  fetch replaces the subscription's stored occurrences (a quarter back, a
  year ahead, ≤ 5000). The fetch goes through `safedial` (no loopback,
  link-local, metadata), follows up to three redirects each re-checked,
  ≤ 8 MiB, 30 s. Errors never echo the URL (it's a secret).
- **Feeds** (`/ical/<token>.ics`, public, own rate limit) write series as
  RRULE with TZID, all-day dates as DATE values, tasks as all-day or 30-minute
  floating events. Only workspaces the owner still belongs to are included.

## Frontend

- **Design system** in `styles/main.css`: Apple's system colours as CSS
  variables (light in `:root`, dark in `html.dark`), the system font stack
  (SF Pro on Apple devices), and component classes (`.btn*`, `.seg`, `.card`,
  `.group`/`.group-row`, `.popover`, `.menu-item`, `.check`, `.tile`,
  `.material`). `--ws` carries the current workspace's colour.
- **Shell**: a translucent sidebar (Today, Calendar, Favorites, each
  workspace with its modules and page tree), a drawer below 1024 px, a bottom
  tab bar below 640 px, ⌘K palette, ⌘\ to hide the sidebar.
- **Pages**: stored as GitHub-flavoured Markdown. A reading view (marked +
  DOMPurify, GitHub alerts, footnotes, tickable task boxes), and two editors
  over the same Markdown: visual (TipTap with StarterKit, task lists, tables,
  highlight, callouts, `@tiptap/markdown` to read and write Markdown,
  pass-through nodes for what it can't edit, a slash menu, a bubble toolbar)
  and source (a text area with list helpers). Every save is normalised, so
  switching editors doesn't reformat a page; autosave with versions.
- **Drag and drop**: SortableJS through `lib/sortable.ts`, which puts the
  dragged element back and reports the move, so Vue stays the only owner of
  the DOM.

## Decisions

| Decision | Why |
|---|---|
| Markdown for pages, edited visually or as source | Portable (download, zip export, git), readable anywhere; one stored format so people with different editors share pages. Highlight, underline and callouts use GitHub's forms (`<mark>`, `<u>`, `> [!TIP]`); what the visual editor can't edit passes through untouched |
| Full-text search in SQLite FTS5 (`internal/search`) | Ranked, accent-insensitive, by word start; kept up to date by SQLite triggers on pages, tasks, notes, events and comments, so every writer (the app, the syncs, imports) is covered. Without FTS5 (another engine) search falls back to `LIKE` |
| No content in the audit log | Thousands of edits would drown the security events the chain is for |
| SSE + refetch, not CRDTs | Simple and correct for planners; real-time co-editing is a roadmap item |
| Floating task dates, zoned events | Matches how people think about to-dos versus appointments |
| SQLite only | One file to back up; other engines once smoke-tested |
