# Changelog

All notable changes to Orbit. Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versions follow [SemVer](https://semver.org/). Fill `[Unreleased]` as changes
merge, not at release time.

## [Unreleased]

## [0.4.0] - 2026-11-05

### Added

- **Holiday themes** — Christmas, Halloween, New Year's Eve, Easter, Santos
  Populares and Lunar New Year: every page and the sidebar in the holiday's
  colours (light and dark), its mark, the Today tile in its pattern, falling
  snow, bats, confetti or petals; on the overviews also lights, bunting or
  things on threads along the top and art on the cards, and a countdown by
  the greeting. The owner turns
  them on (Owner → Branding): each in its season, or one now. Everyone sees
  it; each person can turn off only the falling part (Settings → Appearance,
  or the switch in the sidebar). Never in the way: nothing takes a click,
  and with reduced motion nothing moves.

### Removed

- **Face ID / Touch ID / Windows Hello sign-in and the lock** (from 0.3.0) —
  on a phone it was a passkey under another name, so passkeys alone it
  is. A device that had it turned on keeps it as an ordinary passkey (Settings
  → Passkeys, where it can be renamed or removed).

## [0.3.0] - 2026-11-04

### Added

- **Face ID, Touch ID, Windows Hello, a fingerprint** — when the owner allows
  it (Owner → Security), each person turns on their device's own lock: Orbit
  offers it after a password or SSO sign-in, and Settings → Face ID and lock
  has the switch. Then that device signs in with it (asked at once, and a
  button of its own on the sign-in page), and can **lock Orbit** after a
  while idle until Face ID. The owner may cap the lock's wait or require it.
  Passkeys underneath, made on the device and verified.
- **Workspaces in your own order** — drag them in the sidebar, or **Move up**
  / **Move down** in a workspace's `⋯`; each person's order is their own.

### Fixed

- **A task's details in dark mode** — one dark grey for the whole sheet with
  the cards a step lighter (the fields sat on black).
- **Dark-mode touches that never applied** — board columns and lanes, cards'
  hover, git badges, notes' capture box and colours, the calendar's events on
  hover, list columns and sheets: their dark styles were lost (and leaked
  onto the page) through a selector the style compiler rewrote.

## [0.2.0] - 2026-11-04

### Added

- **Archive a list** — a finished project's list put away, its tasks kept:
  out of My Lists, Today, the board, the timeline, the calendar, feeds and
  reminders (search still finds it). **Archived lists** under My Lists holds
  them; **Restore** brings one back.
- **Quick notes coloured as an editor would** — Markdown (headings, bold,
  lists, links, ``` blocks with their code coloured), code (JSON, YAML,
  shell, SQL, Python, Go, Dockerfile, nginx… — monospaced and coloured), or
  plain text: guessed from the text as you type, shown in the note's corner,
  and changeable there. The cards show the same colours; a code note turns
  into a page as one code block.
- **A calendar for each list, in colours of your own** — in a workspace's
  calendar each list is a calendar of its own, in the list's colour (they
  were one "Tasks"), and the workspace's own events are **Events** (they
  carried its name). Every calendar's colour can be changed from the left
  pane — workspaces, Events, lists, subscribed calendars, Holidays — for you
  only; an event with a colour of its own keeps it.
- **Editors in their own look** — Settings → Appearance → Editors: Sheets,
  diagrams (draw.io) and whiteboards follow the theme, or stay light or dark
  (Sheets and draw.io light in a dark Orbit, say).
- **Tasks and bookmarks by their tags in ⌘K** — a tag finds its tasks and
  bookmarks, as it finds pages.
- **Link previews in Slack, Teams and chats** — a guest link shows its
  page's title and first words; any other Orbit link only the instance's name
  and icon, never what's behind it.
- **Documentation** — Orbit's guides in one place (user menu → Documentation,
  `/docs`, beside the API reference): user guide, configuration, reverse
  proxies, SSO with Authentik / Entra, troubleshooting, security, the API,
  what's new — built into the version you run, with a search across them.
  Assistants get them too: the MCP's `orbit_help` answers how something is
  done in Orbit from them.
- **Quick Draw full screen** — a button at the bottom right fills the
  screen with the sketch and its tools (for drawing on a tablet); Esc or the
  button again leaves.
- **Focus timer lengths of your own** — **+** beside 15, 25 and 50 adds a
  length (1–180 minutes), kept on your account for every device; the last
  one picked is picked again.
- **Bigger notes** — ⤢ in a note's toolbar and the Quick Note box opens notes
  wide, with room to write (remembered on the device).
- **Impersonation in the audit log** — its end is recorded too (returned, or
  timed out), and **Impersonations** shows only those: starts, ends and what
  was done in between.

### Changed

- **A task's details, redone** — the list it's on, the title and notes, and
  a bar saying how its date stands (*Overdue by 4 days*, *Due today*…) with
  Today / Tomorrow / Next week; then Schedule (start → due, time, repeat),
  Planning (status from the board's columns, an estimate of any length,
  priority, list, goal…), People &
  tags; **Delete** and **Start focus** at the bottom. Beside it, **Activity**:
  the comments, the changes and the files, with the box at the bottom —
  drop files on it, or its clip, to attach them.
- **Task comments beside the details** (Planner's way) on a wide window.
- **Quick Draw's eraser: Part or Whole line** — a clear choice in the toolbar
  (remembered), and Shift held swaps it while erasing; whole-line erasing
  shows a red ring. The toolbar no longer gets cut off when it's long.
- **A note turned into a page reads as written** — its lists ("- ", "* ",
  "•", "1. "), tasks ("[ ] "), headings, quotes, links and code (``` blocks,
  `inline`) become those on the page, and line breaks stay (they came out as
  escaped text before).
- **A forced password change needs a new password** — the temporary one (an
  admin's, or `ADMIN_PASSWORD`) again is refused. Only then: no password
  history.

- **Behind Cloudflare** — the reverse-proxy guide explains a proxied (orange
  cloud) record: trust Cloudflare's ranges in `TRUSTED_PROXIES` too, or the
  audit log and sign-in limits see Cloudflare's addresses, not people's;
  plus SSL mode, timeouts, the upload limit and Rocket Loader.

### Fixed

- **Task notes and comments keep their lines** — a line typed on its own
  reads on its own (they ran together in one paragraph). Long notes, while
  written in, scroll in their own box: the task's fields and buttons stay
  in reach.
- **Enter in a new page's title** sometimes left you in the title (its
  editor still loading); it now goes on into the page once it's there.
- **Impersonating showed the admin's workspaces**, not the person's (and,
  back, theirs): the sidebar and the live updates now follow whoever is
  signed in.
- **Search on SQLite kept indexing the old way after an upgrade** — its
  triggers were made once and never again, so page tags (and now task tags)
  weren't found on an instance that existed before. A new index version now
  makes them again.
- **Quick Draw's own colours were lost on reload** — they're sent with the
  sign-in again (and so are the new editor looks and calendar colours).

### Security

- The runtime image is now **distroless Debian 13** (Debian 12's had
  outdated ca-certificates and tzdata); the image scans clean (Trivy).
- **go-jose 4.1.6** (used by SSO sign-in): three denial-of-service
  advisories fixed (one high).
- **golang.org/x/net 0.62** (two HTML-parsing advisories) and
  **golang.org/x/crypto** without its unmaintained `openpgp` package.
- **KaTeX 0.18.11** (in Mermaid diagrams): a prototype-pollution advisory
  fixed. `govulncheck` and `npm audit` report nothing.

## [0.1.0] - 2026-10-31

### Security

- **Go 1.26.10** — the toolchain moves up for GO-2026-6809 (crypto/x509
  certificate checks, reached through passkey sign-in).
- **vue-i18n 11.4.14** — moves up a major version for GHSA-8rc2-428q-7ww3 and
  GHSA-83rm-6c8r-g728 (`escapeParameter`, `srcdoc`). Orbit used neither, so
  it was not reachable; `npm audit --audit-level=high` is clean again.

### Changed

- **Page history keeps every save** (it used to merge one person's saves
  over ten minutes, so the version before an edit could be lost); the owner
  sets how many each page keeps (Owner → Page history, 5–1000, default 50).
- **Download as PDF** makes the file itself (no print dialog), with the
  diagrams' and whiteboards' pictures drawn in and sheets as compact tables
  (column letters, row numbers, only the cells used, numbers right-aligned,
  in the middle of the page); a title stays on the page with what it names.
  Or, chosen when downloading (remembered), with them as links.
- **A side panel where something is open, as Folio's**: the graph of
  what's open (a page, sheet, diagram, whiteboard, task list or folder),
  depth 1–3, small dots; on pages also the outline and the notes; each part
  folds; opens by itself at 1440 px+, a tab reopens it. Screens with nothing
  open (Today, a workspace's home, the graph itself) have none.
- **Calendar**: framed like draw.io's editor (a bordered box, a gray band
  for the days), the side panes boxed too; events outlined in their colour
  with a gap between one and the next; hour and half-hour lines; the month
  title no longer gets cut.

### Added

- **Tags and properties on any page** — tags (chips under the title, the
  workspace's suggested; a tag opens Pages filtered by it; a tag bar in
  Pages) and the page's own properties (text, number, date, checkbox, link);
  found by ⌘K; a Markdown file's YAML header out and in (Obsidian's style),
  and through the assistants' tool. API: `tags`, `fields` on a page,
  `GET /workspaces/{ws}/tags`, `GET /workspaces/{ws}/pages?tag=`.
- **Presence** — the initials of everyone else on a page, sheet, diagram or
  whiteboard right now, ringed when editing (`POST /presence/{kind}/{id}`,
  a heartbeat per tab).
- **Editing together** — pages in the Visual editor edited by several
  people at once (Yjs: each one's typing and cursor shows on the others'
  screens as it happens), and whiteboards drawn together (sketches and
  Excalidraw: each stroke, shape or picture shows at once). The server keeps
  each document's changes in order and relays them
  (`/collab/{kind}/{id}`, `…/seed`, `…/awareness`, `…/compact`); the
  Markdown / scene saves once for everyone (`X-Orbit-Collab`); saved another
  way, the people there start again from it.
- **Download a page as Word (.docx)** — Word's own styles (Title, Headings,
  lists), tables, pictures, links, callouts; sheets and diagrams drawn in or
  as links, as for the PDF.
- **A page as clean Markdown for agents** — each guest link has a Markdown
  address (`GET /shared/{token}/markdown`, `?all=1` with its pages inside):
  sheets as Markdown tables, Mermaid diagrams as code, links between the
  shared pages leading to their Markdown.
- **Files shown in place** — a PDF's pages, a Word document as a page, an
  Excel file's tabs or a CSV as a table, a PowerPoint deck's slides,
  pictures, video, audio, Markdown and text open over what you're doing (from
  Files, attachments, links in pages, search), with their versions and
  Download; `/w/{ws}/f/{id}` is a file's own address. Read in the browser
  (pdf.js; Word and PowerPoint from their XML), no new dependencies.
- **Files in search** — ⌘K finds files by name and by what they say: Word,
  Excel, PowerPoint, OpenDocument and text files read by the server, PDFs by
  the app (`PUT /files/{id}/text`); files from before are read at start.
  `GET /files/{id}`.
- **My files** — your own drive (sidebar, under Quick Draw): your Quick Draw
  sketches, files you upload, your Quick Notes' attachments, in folders of
  your own, with versions and the trash; only you see it. API:
  `GET/POST /me/folders`, `POST /me/files/move`, `GET /me/files`
  (`?trash=1`), `GET /me/files/usage`, `POST /uploads?target=file&target_id=me`.
- **Files of every kind, in folders** — **Upload** (or drop) any file into
  a folder of Files; the attachments of tasks, pages and notes show there
  too, where what they're on is (a task's in its list's folder), *On the
  task "…"*. A file of the same name in the same place is its next
  **version** (links show the current one; an older one can be made current
  again). Deleting puts a file in a **Trash** for 30 days (restore, or delete
  now); a task, note or page deleted for good sends its files there as files
  of their own. **Unfiled** shows what's in no folder; Details shows the
  space used. API: `GET /workspaces/{ws}/files` (`?trash=1`), `…/files/usage`,
  `PUT /files/{id}`, `GET /files/{id}/versions`, `POST /files/{id}/current`,
  `POST /files/{id}/restore`, `POST /uploads?target=file`,
  `DELETE /uploads/{id}?forever=1`.
- **Every list has its own folder** — made with it (by hand, from a template,
  from git), renamed with it, kept with its contents when the list goes;
  the list's sheet can pick another folder or none (`folder_id` when making
  one). **Files** is on by default (every template, and existing workspaces
  with tasks, pages, sheets, diagrams or whiteboards); existing lists got
  their folders on upgrade.
- **Where files are kept** — each file's bytes once, by SHA-256 (the same
  file attached twice is stored once and counts once against the quota), in
  the database (default), a directory (`UPLOAD_STORE=dir`, `UPLOAD_DIR`) or an
  S3 bucket (`UPLOAD_STORE=s3`, `UPLOAD_S3_*`). `orbit move-files <store>
  [--clean]` moves them, each checked against its sum; starting with another
  store than the recorded one is refused. Automatic backups copy a
  directory store's new files beside their archives. Files kept the old way
  move by themselves on the first start.
- **A list picks its folder** — the list's edit sheet has a **Folder**
  choice (one there is, a new one named after the list, or none), the other
  way round from the folder's **Task list…**.
- **Bucket in Quick Draw** — fills what the lines close off with the
  current colour, under the lines (small gaps still count as closed; key G).
- **Sketch in "/"** — a page's **/** menu makes a Quick Draw sketch in
  place, as it does a sheet or a diagram.
- **More page blocks** — toggle sections, 2–5 columns, a live sub-pages list
  (`::pagetree`), coloured status badges; glossary pages whose terms are
  explained (on hover) where other pages use them; pastes from Google Docs,
  Word and Excel keep their formatting without the wrappers.
- **Undo for page structure** — "Recent changes" in Pages: your own pages
  made, moved, renamed, copied or trashed in the last two weeks, each undone
  (asking first when the page changed since; `GET /me/page-changes`,
  `POST /page-changes/{id}/undo`).
- **Outline beside a page** — its headings to jump to, its callouts and
  checklist items in one list (tick from there); remembered open.
- **Page restrictions** — a page (and what's under it) open to only some of
  the workspace's people (`PUT /pages/{id}/access`); to the others it isn't
  there anywhere (tree, search, Home, links, graph, history, files, exports).
- **Guest links** — a page (and, if chosen, the pages inside it) opens to
  anyone with its link, without an account, to read or to edit (versioned,
  in its history); with what it shows (sheets, diagrams, whiteboards,
  files); expiring or until taken back; an owner switch (`guest_links`).
- **Forms** — a database's questions, each answer a new row; answered in
  Orbit or, made public, by anyone with the link (required fields, checked
  values, a honeypot; owner switch `public_forms`).
- **MCP server for assistants** — Claude, ChatGPT, Cursor and others use
  Orbit as the person at `/mcp` (about 30 tools: pages, tasks, comments,
  calendar, notes, files, Mermaid diagrams, history, graph, search), signed
  in with an API token or Orbit's OAuth 2.1 (PKCE, dynamic registration, a
  consent page with "only read"); the token is the person's, revocable in
  Settings. Settings → Assistants (MCP) says how to connect each.
- **Page history** — every save kept with who made it (one person's saves
  close together are one version); read one with its changes marked,
  restore it, or compare up to five versions or pages side by side
  (`GET /pages/{id}/versions`, `GET /page-versions/{id}`,
  `POST /page-versions/{id}/restore`).
- **Graph** — a workspace's things and how they connect (pages, sheets,
  diagrams, whiteboards, lists, folders, mentioned people; links, embeds,
  mentions, sub-pages, folders), with kinds to hide, find, pan and zoom
  (`GET /workspaces/{ws}/graph`); a small graph 1–3 links deep under each
  page and in Files' details.
- **Projects: a folder and a task list** — a folder joins one task list
  (a list is in one folder at most); inside the folder the list shows with
  its progress, and the list shows its folder. Pages can be filed in
  folders too (`folder_id`), and a folder tile says what it holds of every
  kind — from any module (`counts` on folders; `done_count` and
  `folder_id` on lists).
- **Mermaid diagrams** — a second kind of diagram, written as text and drawn
  beside it (Mermaid): templates (flowchart, sequence, class, state, ER,
  Gantt, timeline, pie, mind map), errors said under the text; on a
  flowchart, Node and Connect on the drawing edit the text. Download as
  .mmd, SVG or PNG; .mmd files import. (`kind` on diagrams.)
- **Sheets, diagrams and whiteboards inside pages** — `/` makes a new one
  (or picks one there is) at that line; it shows in place in both the
  editor and the reading view, and a double click opens it in a big editor
  over the page (a whiteboard with View / Edit). Each stays its own
  document; in Markdown it is its link alone on a line.
- **Move drawings** — a Quick Draw sketch moves into a workspace (as a
  whiteboard), a whiteboard to another workspace or back to Quick Draw
  (`POST /drawings/{id}/move`), as quick notes do.
- **Google Calendar, both ways** — the owner registers a Google OAuth client
  (Owner → Google Calendar); workspace owners sign in with Google, pick a
  calendar, and it syncs through Google's CalDAV interface with the same
  engine as iCloud / Nextcloud (tokens refreshed, refresh token sealed).
- **PostgreSQL** — `DB_DRIVER=postgres` with `DB_URL` a connection string;
  search uses PostgreSQL full text (triggers, title weighted, prefix,
  accent-insensitive with `unaccent`). The API's tests and the end-to-end
  suite pass on both engines (`make test-pg`). In-app backups stay
  SQLite's (Postgres: pg_dump; the owner's Backups card says so).
- **Board swimlanes and column limits** — lanes by person or by priority
  (drag across lanes to reassign or reprioritise); a column's limit shows
  its count red when passed.
- **Time estimates and a week planner** — a task's estimate (details, or
  "~2h" in quick add); Plan in the calendar fits open tasks into free
  working hours around events, saved as personal time blocks
  (`GET/PUT /me/blocks`).
- **Block handles in pages** — beside the block under the pointer: + adds
  a block (with the / menu), ⋮⋮ drags it, or opens its menu: turn into,
  colour (as a GitHub alert callout), duplicate, move up / down, delete.
- **Repeating events: "only this" and "this and following"** — editing an
  occurrence asks which; the series splits (a COUNT keeps counting) or the
  one occurrence leaves it; delete "this and following" too
  (`PUT /events/{id}?scope=this|following&occurrence=`).
- **Folders and Files** — a workspace's sheets, diagrams and whiteboards in
  shared folders that nest; each module shows its own kind in them, and a
  new Files module shows them all (tiles or a list, a details panel, drag
  to move, copy, rename, download, import several at once). Deleting a
  folder moves what it held up; moving doesn't make a new version.
- **Quick Draw's own canvas** — new sketches get a drawing canvas made for
  the hand (old ones stay on Excalidraw): pen, pencil, marker, highlighter
  and brush with stylus pressure (perfect-freehand); a full colour picker
  (shades, hue, code, opacity, eyedropper, recent, "My colours" kept in
  your preferences); hold to snap a line, circle, ellipse, triangle or
  rectangle; an eraser that rubs out parts of strokes; a lasso to move,
  recolour, copy or delete; plain / lined / squared / dotted paper; layers,
  with a photo or a PDF page (pdf.js, loaded when needed) under the ink;
  palm rejection, two-finger undo and three-finger redo.
- **Diagrams** — a module of draw.io diagrams: the draw.io editor in a
  frame (from `DRAWIO_URL` or the owner's address; `embed.diagrams.net` by
  default, or your own `jgraph/drawio`), saved as you draw with a picture
  and the labels for search; `.drawio` / draw.io `.svg` in, `.drawio`,
  editable `.svg` and `.png` out; shown in a page from its link; carried by
  workspace export. CSP `frame-src` allows that one address.
- **Spreadsheets in and out** — a database's view downloads as Excel or
  CSV, and rows come in from an Excel file, a CSV or a Google Sheets link
  (columns matched to properties by name, new ones made); a sheet comes in
  from a Google Sheets link too (`POST /workspaces/{ws}/google-sheet`,
  Google's hosts only).
- **Folded sub-tasks** — a task with sub-tasks shows a bar of how many are
  done, which opens and shuts them; the task details are wider.
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
- **Bookmarks** (a new module): paste a link, Orbit reads its title,
  description and picture (fetched once by the server, public sites only —
  viewers never call the site); tags, notes, read later; found by search.
- **Journal** (a new module): a page a day, each person's own, with a mood;
  a month of moods at a glance; "On this day" from earlier years; found by
  search, by its writer only.
- **Goals** (a new module): a date to reach, progress from the tasks
  towards it (or a slider), on track / behind / overdue, achieved; a task
  picks its goal in its details.
- **Focus timer** (a new module): 15 / 25 / 50-minute sessions, on a task or
  not, kept on the server (across tabs and devices; the end is pushed); the
  time is logged on the task; the sidebar shows the session running.
- **Habits** (a new module): each person's own — every day, some weekdays
  or N times a week; tap a day to mark it done; streaks, the best one, the
  last 30 days, a year grid; a reminder at a time on the days it counts.
- **Function suggestions** in every formula (sheets, the formula bar,
  database formulas): the functions that fit what's typed, with what each
  does, in English or Portuguese (Tab puts one in), and the arguments of
  the function being written, the current one in bold.
- **Sheets** (a new module): spreadsheets as in Excel — cells, ranges,
  `$` references, other tabs (`'My tab'!A1`), Excel's formulas in English
  or Portuguese (lookups, SUMIFS…), Excel's errors; click cells into a
  formula; fill handle (series, references moved), ⌘D / ⌘R, copy-paste with
  Excel and Google Sheets; inserting / deleting rows and columns moves every
  reference; bold, italic, alignment, number formats, frozen rows, column
  widths, tabs; undo. Saved as it goes (version-guarded), found by search,
  carried by workspace export. `.xlsx` and `.csv` in and out (written and
  read in the app). A sheet's link alone on a page's line shows it there.
- **Text that edits like VS Code**: the Markdown editor, task notes, the
  journal and notes take Visual Studio Code's keys — several cursors
  (⌥-click, ⌘D, ⌘⇧L, ⌘⌥↑↓, ⇧⌥I), box selections (⇧⌥-drag), moving and
  copying lines (⌥↑↓, ⇧⌥↑↓), ⌘⇧K, ⌘L, ⌘↵ / ⌘⇧↵, find and replace, go to
  line; Markdown is coloured as you type. CodeMirror 6 underneath; sums,
  lists, files and @-mentions as before. Escape that an editor used (back to
  one cursor) no longer closes the sheet or note around it.
- **Formulas and totals in databases**: a formula property works itself out
  for every row, written as in Excel (`=[@Price]*[@Qty]`, `SUM([Hours])`,
  IF, ROUND, COUNTIF, DATEDIF… in English or Portuguese, `,` or `;`), with
  Excel's errors; a totals row under a table (sum, average, min, max, count,
  empty, checked), each view its own. Never run as code: parsed and walked.
- **GitLab in the git integration**: gitlab.com or a self-managed GitLab,
  by token or by signing in (OAuth); lists from projects (subgroups too),
  issues both ways by iid (state, labels by name, assignees by id, due
  dates, milestones), comments as notes (system notes left out), webhooks
  with a token, merge requests linked by hand (read).
- **Databases**: a page becomes a database of the pages inside it — typed
  properties (text, number, select, multi-select, date, checkbox, person,
  link) edited in place, table and board views with filters, sorts and
  hidden properties, a row's values on its page. Values save as patches
  (two people editing a row don't overwrite each other). Exports write a
  table; Notion databases import as databases.
- **Pages as PDF; import from Notion and Apple Notes**: a page's ⋯ → Export
  as PDF prints a clean copy (pictures included) to save as PDF. **Import**
  takes Markdown files, a Notion export (.zip, nested zips too: the page
  tree, links between pages, pictures and files, databases as tables) and
  Apple Notes as exported to HTML, text or Markdown (folders as pages,
  checklists kept).
- **Task dependencies**: a task waits on others (finish-to-start, no loops)
  — set in its details or by dragging from one timeline bar to another;
  arrows on the timeline (red when too early); moving a task's end moves
  what waits on it, down the chain. Carried by export / import.
- **Charts: downloads and workload**: each chart downloads as a PNG (drawn
  on white with its title, workspace and date) or as CSV; the tasks shown
  download as CSV; a **Workload** chart shows everyone's open tasks by when
  they're due, each cell opening those tasks.
- **@-mentions in pages**: type @ for a person (told once), a page (a link,
  opened in the app) or a date (a chip opening the calendar); in both
  editors, as plain Markdown. **Linked from** at the bottom of a page lists
  the pages and tasks that link to it.
- **Page templates**: a new, empty page starts as meeting notes, class
  notes or a weekly review, or from any page made a template (`⋯` → Use as a
  template); `{{date}}`, `{{weekday}}`, `{{week}}`, `{{me}}`… are filled in.
- **Smart lists**: a task filter saved with a name, colour and icon, shown
  with Today and Scheduled (with its count). Yours, or shared with the
  workspace; its filter changes in place and is kept on the server; a task
  added there gets its tag, priority and person. Carried by workspace
  export / import.
- **Repeats in quick add**: "every other Tuesday", "every Monday and
  Friday", "every 3 days", "every weekday", and in Portuguese "terça sim,
  terça não", "todas as quintas", "de 15 em 15 dias"… The task starts on the
  first day the repeat falls on; custom rules show in words ("Every other
  Tuesday") instead of "Custom".
- **Quick Draw**: your own sketches, like Quick Notes — a tile with a **+**
  in the sidebar (and in the phone's tab bar, and ⌘K → New sketch).
- **Whiteboards** (a new module) and **drawings on notes**, with Excalidraw:
  hand-drawn shapes, arrows, text and images; saved as you draw (a stale
  save never overwrites a newer one), shown as pictures on cards, found by
  their words, carried by workspace export / import. Excalidraw (and React)
  load only when a drawing opens; its fonts come from Orbit itself.
- **Snooze** a notification (in the bell, or the Snooze button on a push):
  it comes back unread at that time.
- **Quiet hours**: nothing reaches devices or channels between two times —
  it waits, and one summary comes when they end; reminders can be let
  through.
- **Reminders on notes**: "remind me tomorrow at 9" on any note.
- **Git: instant updates and milestones**: a webhook (added by Orbit when
  the account may, else by hand with the address and secret shown) syncs a
  linked list the moment an issue changes; a task shows its issue's
  milestone and picks one in its details.
- **GitHub in the git integration**: connect a GitHub (or GitHub
  Enterprise) account by token or by signing in; lists from repositories
  (open / closed, or labels as columns), tasks linked to issues and pull
  requests, comments — both ways, as on Gitea. GitHub's issues have no due
  dates (they stay in Orbit) and its boards no REST API (use labels).
- **Two-way calendar sync (CalDAV)**: a workspace's calendar and a calendar
  in iCloud, Nextcloud, Fastmail or any CalDAV server stay in step — new,
  changed and deleted events both ways, right away from Orbit and every five
  minutes from the server; the later change wins; what Orbit doesn't keep
  (alerts, invitees, moved occurrences) is left as it was on the server.
  Owners set it up in Calendars; the `caldav_sync` flag (on) turns it off.
- **Move a workspace to another Orbit**: Workspace settings → Export
  workspace (a `.orbit.zip`), then New workspace → Import a workspace. New
  ids, links and files follow; members found by username or email join with
  their roles. `IMPORT_MAX_MB` caps the file.
- **Share into Quick Notes from a phone**: the installed app is a share
  target on Android (a link or text from any app becomes a Quick Note);
  `/share?title=&text=&url=` does the same for an iPhone shortcut.
- **Week and Day views: drag events** to another time or day, and their
  bottom edge to change the length (15-minute steps).
- **A time zone per event**: times typed in the event's zone ("10:00, New
  York"), shown in yours, with "Your time" in the sheet.
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

### Fixed

- Quick add: pressing Enter then Escape at once no longer adds the task
  twice.
- Notion / Apple Notes import: fflate 0.8.3 (a malformed ZIP64 file could
  hang the unzipping).
- An empty page opened right after signing in now opens ready to write
  (it waited for the workspaces and opened read-only).

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
