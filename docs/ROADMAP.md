# Roadmap

What Orbit could do next, roughly in the order it would pay off. The
"Coming soon" modules in the New workspace sheet come from the module catalog
(`api/internal/planner/modules.go`): turning one on is adding its key to
`Catalog` without `Soon`, plus its views.

## Next

- **Offline.** The service worker (today only for Web Push) caching the app
  shell and the workspaces you opened, and a queue of changes made offline
  (synced in order, conflicts shown like a page's 409). The blueprint says
  "installable, not offline" for admin consoles; a planner is different.
- **Real-time co-editing of pages.** Today two people editing one page get a
  "someone else changed this page" banner. Yjs (with TipTap's collaboration
  extension) over the existing SSE stream plus a small update endpoint would
  merge edits live, with cursors.
- **Share target on phones.** The installed app as a share target, so a link
  or text shared from another app lands in Quick Notes (the inbox, Quick
  Notes and ⌥N already exist).

## Modules

- **Habits** — daily or weekly check-ins with streaks and a year grid.
- **Journal** — one page per day, with mood and a "this day last year".
- **Goals** — goals with progress from linked tasks, and a target date.
- **Bookmarks** — links with a preview, tags and a read-later state.
- **Focus timer** — Pomodoro tied to a task, with time logged on it.

## Notifications

- Snooze ("remind me again in an hour") from the notification itself.
- Quiet hours: nothing reaches devices or channels at night.

## Notes

- Reminders on a note ("remind me tomorrow at 9").
- Drawings on a note (images and files are there).

## Calendar

- Editing one occurrence of a series ("this and following").
- Two-way **CalDAV** sync (iCloud, Google, Nextcloud) — `emersion/go-ical`
  is already the parser, and `go-webdav` speaks CalDAV.
- Dragging repeating events (asking: this one or all).
- Regional holidays (by district / state); more countries.

## Pages

- Drag handles on blocks, block menu ("turn into", duplicate, colour).
- @-mentions of people, pages and dates; backlinks.
- Page templates (meeting notes, class notes, weekly review).
- Databases: a table of pages with properties, filtered views.
- Export to PDF; import from Notion and Apple Notes exports (Markdown
  export and import are there).

## Tasks and boards

- **GitHub** for the git integration: a second client with the same methods
  as `internal/gitsync/gitea.go` (issues, labels, assignees; Projects v2 is
  GraphQL-only, so boards there need their own mapping), and `Kind` on
  `GitAccount` / per-link choosing it. GitLab after that.
- Git: webhooks from the server for instant updates (today a 3-minute poll
  and **Sync now**), milestones as a field.
- Git: an "only as me" choice — today every editor of a linked list writes
  to git as whoever linked it (a decision for the owner).

- Saved filters as smart lists in the sidebar (the filter exists; naming
  and keeping several is what's missing).
- Dependencies on the timeline (finish-to-start arrows, moving a task moves
  what waits on it) and a critical path.
- Exporting the charts (PNG / CSV) and a per-person workload view.
- Natural-language repeat ("every other Tuesday") in quick add.
- Swimlanes on boards (by assignee or priority); WIP limits per column.
- Time estimates and a weekly planner view that fits tasks into free time.

## Instance

- Per-workspace export / import (JSON + attachments), so a workspace can move
  between instances.
- PostgreSQL support once it is smoke-tested (blueprint §11.5).
- Notification channels (Slack, Teams, webhooks) get the titles of what they
  report; a choice to send only "you have a notification" (a decision for
  the owner).
