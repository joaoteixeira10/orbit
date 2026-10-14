# Roadmap

What Orbit could do next, roughly in the order it would pay off. The
"Coming soon" modules in the New workspace sheet come from the module catalog
(`api/internal/planner/modules.go`): turning one on is adding its key to
`Catalog` without `Soon`, plus its views.

## Next

- **Offline and Web Push.** A service worker caching the app shell and the
  workspaces you opened, a queue of changes made offline (synced in order,
  conflicts shown like a page's 409), and Web Push so reminders arrive with
  Orbit closed. The blueprint says "installable, not offline" for admin
  consoles; a planner is different.
- **Real-time co-editing of pages.** Today two people editing one page get a
  "someone else changed this page" banner. Yjs (with TipTap's collaboration
  extension) over the existing SSE stream plus a small update endpoint would
  merge edits live, with cursors.
- **Invite links.** Today an owner adds an existing account. A link (single
  use, expiring) that creates an account and joins the workspace would make
  "Shared" work for family and classmates without an admin.
- **Inbox / quick capture.** One place (and a share target on phones) where
  anything lands first, to be sorted into a workspace later.

## Modules

- **Habits** — daily or weekly check-ins with streaks and a year grid.
- **Journal** — one page per day, with mood and a "this day last year".
- **Goals** — goals with progress from linked tasks, and a target date.
- **Courses** (school) — subjects with a timetable that fills the calendar,
  each with its pages, assignments and exams.
- **Grades** (school) — marks per subject with weights, the average, and
  "what do I need in the exam".
- **Bookmarks** — links with a preview, tags and a read-later state.
- **Focus timer** — Pomodoro tied to a task, with time logged on it.

## Notes

- Reminders on a note ("remind me tomorrow at 9").
- Images and drawings on a note (with the upload store below).
- Quick capture from anywhere: a global shortcut and the phone share sheet
  landing in a workspace's notes.

## Calendar

- Editing one occurrence of a series ("this and following").
- Two-way **CalDAV** sync (iCloud, Google, Nextcloud) — `emersion/go-ical`
  is already the parser, and `go-webdav` speaks CalDAV.
- Week-view drag to move and resize; time-zone choice per event.
- Holidays by country as a built-in subscription.

## Pages

- Drag handles on blocks, block menu ("turn into", duplicate, colour).
- Images and files (an upload store, with a size quota per instance).
- @-mentions of people, pages and dates; backlinks.
- Page templates (meeting notes, class notes, weekly review).
- Databases: a table of pages with properties, filtered views.
- Export to Markdown / PDF; import from Notion and Apple Notes exports.

## Tasks and boards

- Saved filters as smart lists in the sidebar (the filter exists; naming
  and keeping several is what's missing).
- Dependencies on the timeline (finish-to-start arrows, moving a task moves
  what waits on it) and a critical path.
- Exporting the charts (PNG / CSV) and a per-person workload view.
- Natural-language repeat ("every other Tuesday") in quick add.
- Swimlanes on boards (by assignee or priority); WIP limits per column.
- Time estimates and a weekly planner view that fits tasks into free time.

## Instance

- Full-text search with SQLite FTS5 (today: `LIKE` over titles and text).
- Per-workspace export / import (JSON + attachments), so a workspace can move
  between instances.
- A workspace activity feed ("Ana completed 3 tasks in Shopping").
- PostgreSQL support once it is smoke-tested (blueprint §11.5).
