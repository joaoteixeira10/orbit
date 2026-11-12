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
- **Sharing files and photos** into Orbit from a phone (links and text
  already land in Quick Notes; files need a POST share target and the
  service worker to receive them).

## Modules

- Bookmarks: saving a link from the phone's share sheet straight into
  Bookmarks (today it goes to Quick Notes); importing browser bookmarks.
- Focus timer: breaks between sessions (a long one every four), and a week
  of focus in Charts.

## Notifications

- Quiet hours by day of the week (weekends quiet longer); a snooze on iOS
  pushes (Safari shows no buttons).

## Notes and whiteboards

- Drawing together live on a whiteboard (today: saves don't overwrite each
  other, and others' saves show when you aren't drawing).

## Calendar

- Editing one occurrence of a series ("this and following").
- Two-way sync with **Google Calendar** (it needs OAuth, not an app
  password; CalDAV with iCloud, Nextcloud and others is there).
- Carrying occurrences edited on their own (RECURRENCE-ID) both ways, and
  alerts (VALARM) as Orbit reminders.
- Dragging repeating events (asking: this one or all).
- Regional holidays (by district / state); more countries.

## Pages

- Drag handles on blocks, block menu ("turn into", duplicate, colour).

## Tasks and boards

- GitHub's project boards (Projects v2, GraphQL only) as a list layout;
  GitLab merge requests as tasks of a list (they're linked by hand today).
- Git: milestones as a list layout or a filter (they're a field now);
  webhooks for tasks linked by hand.
- A critical path on the timeline (dependencies are there).
- WIP limits per board column.
- Time estimates and a weekly planner view that fits tasks into free time.

## Instance

