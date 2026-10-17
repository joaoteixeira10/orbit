# Orbit user guide

Orbit is a planner you host yourself: one **workspace** per part of your life
— school, work, home, a group — each with only the modules it needs (notes,
pages, tasks, a board, a timeline, charts, a calendar), shared with the people you choose. This guide
covers the app, task by task. To install or configure Orbit, see the
[README](../README.md) and [CONFIGURATION](CONFIGURATION.md). For scripts, see
the [API](API.md).

## Contents

1. [Sign in](#1-sign-in)
2. [Find your way around](#2-find-your-way-around)
3. [Workspaces](#3-workspaces)
4. [Today and the overview](#4-today-and-the-overview)
5. [Pages](#5-pages)
6. [Notes](#6-notes)
7. [Tasks](#7-tasks)
8. [Boards](#8-boards)
9. [Timeline](#9-timeline)
10. [Charts](#10-charts)
11. [Calendar](#11-calendar)
12. [Notifications](#12-notifications)
13. [Your settings](#13-your-settings)
14. [Administration](#14-administration)
15. [Owner controls](#15-owner-controls)
16. [On a phone](#16-on-a-phone)

---

## 1. Sign in

### With a user name and password

1. Open Orbit. You land on the **Sign in** card.
2. Type your **Username** and **Password**.
3. Click **Sign in**.

If your account has two-factor, a second step follows (see below).

### The first sign-in: set a new password

A new account, or one an admin just reset, has a **temporary password**. After
you sign in with it, a **Set a new password** box covers the page. You can't
skip it.

1. Enter the **Current password** (the temporary one).
2. Enter a **New password (min 8)** and type it again in **Confirm new password**.
3. Click **Update password**.

You carry on in the same session. Your other sessions are signed out. **Sign
out instead** leaves without changing anything.

The very first account (the instance **owner**) works the same way. The
[README](../README.md) explains where its first password comes from.

### Two-factor: the code prompt

If you set up an authenticator app, the **Two-factor authentication** card
asks for a code.

- Type the 6 digits from your app into the **six boxes**. The cursor moves on
  by itself. Backspace goes back. ← and → move between boxes.
- **Paste** works: a whole code fills every box. Password managers and the
  browser's one-time-code autofill work too.
- The code is **sent as soon as the sixth digit is in**. You don't need to
  click **Verify** (it is there for the keyboard and screen readers).
- A wrong code clears the boxes and shows the error.

Other links on the card:

| Link | What it does |
|---|---|
| **Use a passkey** | Shown only if your account has a passkey. Confirms with the passkey instead of a code. |
| **Use a recovery code** | Swaps the boxes for one field (`xxxxx-xxxxx`). Type one of your recovery codes. Each works once. **Use authenticator code** switches back. |
| **Back** | Returns to the user name and password. |

If your account has a passkey but no authenticator app, you only see **Use a
passkey**.

### Two-factor required: set it up now

When an admin requires two-factor for you (or for everyone), a **Set up
two-factor authentication** box appears right after you sign in: "Your
administrator requires 2FA before you can continue."

- Scan the QR code, enter the code, and save the recovery codes (see
  [Two-factor](#two-factor) in Settings), **or**
- click **Set up a passkey instead**, if passkeys are on.

You carry on in the same session once it's done. **Sign out instead** leaves.

### With a passkey (no password)

If the owner allowed passwordless sign-in and your browser supports it, the
card shows **Sign in with a passkey**. Click it and confirm with Touch ID,
Windows Hello, a security key or your password manager. The browser may also
offer your passkey as soon as you click the **Username** field.

You add passkeys in **Settings → Passkeys** first.

### With single sign-on (SSO)

If the owner set up SSO, the card shows **Sign in with SSO**. It takes you to
your company's identity provider and back. Your first SSO sign-in creates your
Orbit account. Its role comes from your provider's groups or the default
role (often `none`, which can see nothing until an admin gives you a role).

The owner may also:

- **hide the password form**, so only SSO (and passkeys) show, or
- **auto-start SSO**, so opening the page goes straight to the provider.

> **Escape hatch:** open `/login?local=1` (for example
> `https://orbit.example.org/login?local=1`). It shows every way in that is
> enabled, including the password form, and skips auto-start SSO. Use it when
> the identity provider is down.

Setting up SSO: [SSO-entra](SSO-entra.md), [SSO-authentik](SSO-authentik.md).

### Too many failed attempts

After **5** failed attempts in a row (wrong password or wrong two-factor code; a successful sign-in starts the count again), the
account is **locked for 15 minutes** (the defaults; see [CONFIGURATION](CONFIGURATION.md)).
While locked, sign-in says "invalid credentials" even with the right password.
Wait, or ask an admin to click **Unlock** on your row in **Admin → Users**.

### "Confirm it's you"

When the owner turns on **Step-up re-authentication**, some risky actions
first ask you to **Confirm it's you**: re-enter your **Password** and click
**Confirm**, or click **Use a passkey**. This covers security and SSO changes,
feature flags, ownership transfer and impersonation.

## 2. Find your way around

The **sidebar** on the left (a drawer behind the ☰ button on narrow screens):

- **Search ⌘K** (Ctrl+K on Windows and Linux) opens the **command palette**:
  type to find pages, tasks, notes and events in all your workspaces, jump
  to a workspace, or run an action (new page, new task, new note, new
  workspace, light or dark appearance, settings). Arrows move, Enter opens, Esc closes.
- **Today** and **Calendar**: across every workspace.
- **Favorites**: pages you starred.
- **Workspaces**: each with its icon and colour. The chevron shows its modules
  (and, under **Pages**, its page tree); the `⋯` opens its settings, modules,
  members and trash. **+** creates a workspace.
- **Archived** workspaces gather at the bottom.
- Your **name** at the bottom opens Settings, the Audit log, Admin and Owner
  (if you may), the appearance switch, the API reference and **Sign out**.
  The status line below it opens **About** (version, updates).

⌘\ (Ctrl+\) hides the sidebar; the panel button at the top brings it back.
Everything updates live: when someone else changes a shared workspace, you see
it without reloading.

## 3. Workspaces

**Create one.** Click **+** next to Workspaces (or **New workspace** in ⌘K):

1. Pick a **template** — **School** (classes, assignments, exams), **Work**
   (projects on a board, meetings), **Personal** (reminders, appointments, a
   journal), **Shared** (shopping, chores, a common calendar) or **Blank**.
2. Name it, pick a **colour** and an **icon**.
3. Choose its **modules**: **Overview**, **Notes**, **Pages**, **Tasks**,
   **Board**, **Timeline**, **Charts**, **Calendar**. Modules marked **Coming soon** (habits, journal, goals,
   courses, grades, bookmarks, focus timer) are on the [roadmap](ROADMAP.md).
4. **Start with examples** creates the template's lists (with their board
   columns, e.g. *To do / Doing / Handed in*), a welcome page and a first
   pinned note, in the language you use Orbit in.
5. **Create**.

You need the **Create workspaces** privilege (the `member` role has it). Without
it you only see workspaces you're invited to.

**Settings** (`⋯` next to its name → Workspace settings), three tabs:

- **General** — name, description, colour and icon (saved as you type);
  **Archive workspace** (read-only, moved to Archived; **Restore workspace**
  brings it back); **Delete workspace** (type its name to confirm — everything
  in it is deleted for everyone). Members who aren't owners see **Leave
  workspace** instead.
- **Modules** — turn modules on and off (turning one off hides it; nothing is
  deleted) and drag them to change their order in the sidebar.
- **Members** — invite an existing account by **username or email** as
  **Owner**, **Editor** or **Viewer**; change roles; remove members.

| Role | Can |
|---|---|
| **Owner** | everything, including settings, modules, members, archive and delete |
| **Editor** | add, change and delete pages, tasks, lists, events and subscribed calendars |
| **Viewer** | read |

A workspace is **private to its members** — instance admins can't open it.
It always keeps at least one owner. The sidebar lists workspaces in the order
you created or joined them.

## 4. Today and the overview

**Today** (the top of the sidebar) greets you and shows, across every
workspace:

- **Today** — overdue tasks (red) and tasks due today, with a round checkbox
  to complete them and ⓘ for details;
- **Up Next** — this week's events by day (the one happening **now** is
  highlighted), including subscribed calendars;
- **This Week** — tasks due in the next seven days;
- **Jot something down…** — a quick note to your own [Quick Notes](#quick-notes-your-own);
- **Pinned Notes** — your pinned notes (click one to open it), and **Quick
  Notes**, the latest of your own;
- **Recent Pages** — the last pages edited.

Tasks show only when they're assigned to you or to nobody. A workspace's own
**Overview** (click its name) shows the same for that workspace, plus a card
per module, its members and an **Invite** button for owners.

## 5. Pages

**Create** a page with **+** next to **Pages** in the sidebar, **New page** on
the Pages screen, or ⌘K → *New page here*. Hover a page in the tree for **+**
(a page inside it) and `⋯` (Add to Favorites, Duplicate, Copy link, Move to…,
Move to Trash). Drag pages in the tree to reorder them.

**Pages are Markdown** (GitHub-flavoured). A page opens as it **reads** —
rendered like a README on GitHub — and you can tick its checkboxes right
there. **Edit** (or the **E** key, or a double click) opens it for writing;
**Done** goes back to reading. A new page opens ready to write.

You write in one of two editors, and switch any time with **Visual |
Markdown** at the top while editing. Your default is in **Settings →
Language & time → Edit pages in**. Others can use the other editor on the
same page: it's the same Markdown.

- **Visual** — like Notion. Type **/** on an empty line for the block menu:
  text, headings 1–3, to-do list, bulleted and numbered lists, quote, code
  block, divider, table, callout. Markdown shortcuts work too (`# `, `- `,
  `[] `, `> `, ``` ``` ```). Select text for the formatting bar: bold, italic,
  underline, strikethrough, code, highlight, link (⌘K), and turn into text or
  a heading. A **callout** is one of GitHub's five alerts — note, tip,
  important, warning, caution (click its icon to choose).
- **Markdown** — the page's source. Enter continues a list, task or quote (an
  empty item ends it), Tab / ⇧Tab indent, ⌘B ⌘I ⌘E ⌘K make bold, italic,
  code and a link of the selection.

What Markdown has no syntax for is written the way GitHub shows it:
highlight as `<mark>…</mark>`, underline as `<u>…</u>`, callouts as
`> [!TIP]`. Markdown the visual editor can't edit — HTML blocks like
`<details>`, `<kbd>`, footnotes — is kept exactly as written and shown in it
as grey source. When the visual editor saves, the Markdown is tidied into one
style (`-` bullets, `**bold**`, one blank line between blocks).

Add an **icon** (emoji) and a **cover** (gradient or colour) from the header.

**As files.** A page's `⋯` → **Download as Markdown** (a `.md` with the title
as its first heading) or **Copy as Markdown**. On the Pages screen, **Import
Markdown** turns `.md` files into pages (a first `# Heading` becomes the
title) and **Export as Markdown (.zip)** downloads every page as `.md` files
in folders that follow the page tree — ready to keep in git.

**Saving** is automatic; the header says *Saving…* / *Saved* / *Edited … by
…*. ⌘S saves at once. If someone else saved the page while you were editing,
a banner says **Someone else changed this page**: **Load theirs** (drops your
unsaved edits) or **Keep mine** (saves yours over theirs).

**Pages inside** a page are listed under it, with **Add a page inside**.

**Pages screen** (Pages in the sidebar): every page as a grid or a list,
sorted by **Recently edited** or **Title**, with a filter. **Trash** keeps
deleted pages (and the pages inside them) for **30 days**: **Restore**, or
**Delete for good**.

## 6. Notes

**Notes** are quick sticky notes: a few lines or a checklist on a coloured
card, for what you'd jot on a Post-it. Longer writing belongs in a page.

- **Take a note…** at the top: type a title and text, or click ☑ for a
  checklist. A click outside, **Close**, Esc or ⌘↵ adds it.
- In a **checklist**, Enter adds an item, Backspace on an empty one removes
  it, and ticked items move under *N checked items*. Tick items right on the
  card.
- Hover a card for its tools: **pin** (pinned notes come first, and show on
  Today), **colour**, **checkboxes on / off** (a text becomes a checklist line
  by line, and back), and `⋯` → **Turn into page**, **Turn into tasks** (each
  open item or line becomes a task in the first list), **Copy text**,
  **Delete** (with **Undo** for a few seconds).
- Click a card to open it larger; it saves as you type. Emptying a note and
  closing it deletes it.
- **Drag** cards to reorder them (between Pinned and Others to pin or unpin).
- **Search notes** and the colour dots at the top narrow the wall.

### Quick Notes (your own)

**Quick Notes** (under Today and Calendar in the sidebar) is your scratchpad:
notes that belong to no workspace and only you see — for what comes up in the
moment. Jot one from anywhere:

- **Jot something down…** at the top of **Today**;
- the **+** on **Quick Notes** in the sidebar, or **⌥N** (Alt+N on Windows and
  Linux) on any page, or ⌘K → **Quick note**;
- the box on the Quick Notes page.

They work like any notes (pin, colour, checklist, search). Today shows the
latest ones; pinned ones join **Pinned Notes**. When you know where one
belongs, `⋯` → **Move to…** a workspace you edit (or back to Quick Notes), or
**Turn into page** / **Turn into tasks** and pick the workspace.

## 7. Tasks

The **Tasks** screen has two panes (two screens on a phone):

- **Smart lists**: **Today** (overdue and due today), **Scheduled** (with a
  date, by day), **All** (open, by list), **Completed** (by day completed).
- **My Lists**: the workspace's lists with their colour, icon and open count.
  **Add List** creates one (name, colour, icon, and its columns — see Boards).

**Add a task**: **New Task** at the bottom of a section, type, Enter (the
field stays for the next one). **Quick add** reads, in English or Portuguese:

| Type | Gives |
|---|---|
| `today` / `hoje`, `tomorrow` / `amanhã`, `next week` / `próxima semana`, a weekday (`friday`, `sexta`) | the due date (a weekday means the next one) |
| `14:30`, `9h`, `2pm`, `at 18:00` / `às 18h` | the due time (today if no date) |
| `!`, `!!`, `!!!` | priority low, medium, high |
| `#maths` | a tag |

So *Essay tomorrow 14:30 !! #maths* is "Essay", due tomorrow at 14:30, medium
priority, tagged maths.

**Complete** with the round checkbox (it fades after a moment; the eye button
shows or hides completed tasks). Click a title to rename it in place. **ⓘ**
opens the details: notes, **Start date**, **Date** and **Time**, **Repeat** (every day,
weekday, week, 2 weeks, month, year), **Priority**, **List**, **Status** (its
board column), **For** (a member, in shared workspaces), **tags** and
**Sub-tasks**. **Delete** is at the bottom.

**Repeating tasks**: completing one moves it to its next date (counted from
its due date, like Apple Reminders) instead of closing it.

Inside a list, tasks are grouped by status (the list's open columns); drag
them to reorder or move between statuses. **Group by** at the top groups a
list (or All) by **due date** (overdue, today, this week, next week, later,
no date), **priority** or **person** instead; a task added under a group gets
its priority, person or date.

**Filter** (at the top of Tasks, Board, Timeline and Charts) narrows the tasks
by keyword, **due** (overdue, today, this week, next week, later, no date),
**progress** (not started, in progress, completed), **priority**, **assigned
to** (me, unassigned, a member), **tags** and **lists**. Choices of one kind
are "any of"; kinds combine. A workspace has one filter, kept in your browser
and shared by the four views; the badge shows how many kinds are on and ✕
clears it.

**Start date.** Besides its due date a task can have a **Start date** (in its
details): when work on it begins. Lists then show it as a span (*Mon –
Friday*), and the Timeline draws a bar from start to due. A start after the
due date moves the due date along (and the other way round); a repeating
task's start moves with it.

### Git: issues as tasks

Connect your account on a **Gitea** or **Forgejo** server first, in
**Settings → Git**: the server's address and a personal access token (make
one in the server's *Settings → Applications* with the permissions *issue:
read and write*, *repository: read*, *user: read*). Orbit acts as you, with
your token; everyone connects their own.

**A list from a repository.** **Add List → From a Git repository…**, pick
the repository and how its issues are laid out:

- **Open and closed** — open issues in one column; closed ones are done.
- **Labels as columns** — pick labels (in order) that are steps, like
  *Doing* and *Review*; an issue sits in its label's column (or **Open**
  with none of them), and moving the task changes its label.
- **Project board** — one of the repository's boards, with its columns
  (servers whose API has project boards; the sheet tells you otherwise).

Then it goes both ways. Title, description, due date, labels (shown as
tags; tags that aren't labels stay Orbit's own), who it's for (a member who
connected the same server) and the column reach the issue as soon as you
change them; completing a task closes its issue. Changes made on git come
back every few minutes, or at once with **Sync now** in **Edit list**,
which also shows when it last synced and why it failed, if it did. A **new
task** in the list opens an issue. **Deleting** a task leaves its issue
alone (and it doesn't come back). Each task shows its issue's number in
green (open) or purple (closed): click it to open the issue. If git refuses
a change (the server is down, the token expired), the change is kept, you're
told, and it is sent again by the next sync. **Stop syncing** in **Edit
list** keeps the tasks.

**One task, one issue.** In any other list, a task's details have an
**Issue** field: paste the address of an issue or pull request. Closing the
issue — or merging the pull request — completes the task, and completing
the task closes the issue.

## 8. Boards

The **Board** shows a list's tasks as columns — the list's **statuses**. The
strip at the top switches lists; **Edit list** changes the list; **New list**
adds one.

- **Drag cards** between and within columns (a long press on touch screens).
  Dropping a card in a **done** column (marked ✓) completes it; taking it out
  reopens it.
- **Add a card** at the bottom of a column.
- A column's `⋯`: rename, colour, mark as done / open, move left or right,
  delete (its cards move to another column; you're asked first).
- **Add column** at the end.
- Click a card for its details (same as in Tasks).

A list needs at least one open and one done column. The Board and Tasks show
the same tasks: completing a task in one moves it in the other.

## 9. Timeline

The **Timeline** is a Gantt chart of the workspace's tasks, grouped by list
(the list's row shows its overall span; its chevron folds it). Each task is a
bar from its start to its due date in its list's colour; a task with only a
due date is a one-day bar. Late tasks are ringed in red, completed ones
striped, and sub-task progress is a line along the bottom of the bar.

- **Weeks / Months / Quarters** zoom; **Today** scrolls back to today (the
  red line). The chart grows as you scroll past either end.
- **Drag a bar** to move the task (start and due move together); drag its
  **left or right edge** to change the start or the due date. A label shows
  the dates while you drag. On a touch screen, hold a bar a moment first.
  With the keyboard: ← → move a focused bar a day, ⇧← ⇧→ change its length.
- Click a bar or a title for the task's details; the round checkbox completes
  it; **New Task** adds one starting today.
- A task with **no dates** has an empty row: hover it and click a day to
  schedule it there.
- The eye button shows or hides completed tasks; **Filter** applies.

## 10. Charts

**Charts** shows how the work is going, like Microsoft Planner:

- **Open**, **Late**, **Due this week** and **Completed** (last 7 days);
- **Status**: not started (in a list's first column), in progress (in a later
  open column), late (past its due date) and completed;
- **By list**, **By priority**, **By tag** and, in shared workspaces, **By
  person** — bars split by status;
- **Trend**: tasks created and completed per week over the last 12 weeks.

Hover (or focus) any part for the numbers. Click a part to open Tasks
filtered to it — e.g. a list's *Late* segment shows that list's overdue
tasks. **Filter** applies to every chart.

## 11. Calendar

**Calendar** in a workspace shows its events, its subscribed calendars and
its tasks with a due date. **Calendar** at the top of the sidebar shows every
workspace together, in each workspace's colour (the key is under the title).

- **Day**, **Week**, **Month** (top right), **‹ Today ›** to move. Today is
  the red circle; in Day and Week a red line marks the time.
- **New event**: **+**, or click an empty slot (Week, Day) or a day (Month).
  Title, location, **All-day**, **Starts** / **Ends**, **Repeat**, **Alert**
  (none, at time of event, 5–30 minutes, 1 hour, 1 day before), colour (or the
  workspace's), notes. From the merged calendar you also choose the workspace.
- **Open an event** to edit it. Editing a repeating event changes the whole
  series. **Delete** on a repeating event asks: **Delete this event only** or
  **Delete all events**.
- **Drag** an event to another day in Month to move it (not repeating ones).
- **Tasks** appear on their due date with a checkbox; tick them right there.

**Subscribe to another calendar** (editors; **Calendars** in a workspace's
calendar): **Add calendar**, paste its **address** (`https://…` or
`webcal://…`), name it, pick a colour, **Subscribe**. Orbit fetches it at once
(a wrong address is reported straight away) and then every hour; its events
show read-only. Where to find the address:

- **Google Calendar**: the calendar's *Settings and sharing* → **Secret
  address in iCal format**.
- **iCloud**: share the calendar as a **Public Calendar** and copy its link.
- **Outlook**: *Settings → Calendar → Shared calendars → Publish a calendar* →
  the **ICS** link.
- A school timetable or a holidays calendar: their *.ics* / *webcal* link.

Orbit stores the address encrypted and shows only its host. **Refresh now**
fetches again; a failed refresh shows its error in red and keeps the last good
events.

**See Orbit in your phone's calendar**: Settings → [Calendar
feeds](#calendar-feeds).

## 12. Notifications

Everything lands in the **bell** (top of the sidebar; top bar on a phone),
with a red count of what's unread. Click one to open what it's about; **Mark
all as read** clears the count. What you get:

- **Reminders** — a **task with a due time** reminds whoever it's for (else
  whoever created it) at that time, in your time zone (a date alone doesn't
  remind); an **event with an Alert** reminds every member of its workspace
  before each occurrence.
- **Assigned to you** — someone gives you a task (when they create it, or
  later).
- **Workspaces** — someone adds you to a workspace, changes your role or
  removes you.
- **Shared workspace activity** — the other members adding tasks, pages,
  notes and events, completing tasks, joining. A burst of the same thing by
  the same person is one notification (*Ana added 4 tasks*). To quiet a
  workspace, `⋯` next to its name → **Mute activity**.
- **Daily summary** — when turned on, every morning at the time you choose:
  today's events, the tasks due today and the overdue ones. Nothing is sent on
  an empty day.

**Settings → Notifications** chooses where each kind goes besides the bell:

- **This device** — **Turn on** makes notifications appear as system
  notifications, even with Orbit closed (Web Push). **Send a test** checks it.
  Each browser or installed app is its own device; your devices are listed
  with **Remove**. On iPhone and iPad, add Orbit to the Home Screen first and
  turn it on from there.
- **What reaches you** — per kind, a switch for **Devices** and, when the
  owner set up channels, one for **Channels**. By default, what's about you
  reaches your devices and others' activity stays in the bell.
- **Daily summary** — on or off, and the time.
- **Your channels** — which of the owner's channels (email, ntfy, Telegram,
  Discord, Slack, Teams, Gotify) to use.

Notifications sent outside the app (devices, channels) are written in the
language you last used Orbit in. The bell keeps 90 days.

## 13. Your settings

Your name at the bottom of the sidebar → **Settings**. Sections:

- **Account** — photo, display name, email.
- **Appearance** — **Light**, **Dark** or **Automatic**; a personal **accent
  colour**.
  When the owner lets each person choose, **Developer theme**: as the
  instance (the default), none, or one of the five themes — for you only.
- **Language & time** — **English** or **Português**; **Time zone** (for
  reminders; "This device" by default); **Week starts on** Monday or Sunday;
  **Time format** 13:00 or 1:00 PM.
- **Reminders** — notifications on this device; **Also send reminders**
  through the channels you tick.
- **Git** — your accounts on Gitea or Forgejo servers: the server's address
  and a personal access token (see [Git: issues as tasks](#git-issues-as-tasks)).
  Connecting the same server again replaces the token; the bin icon
  disconnects.
- <a id="calendar-feeds"></a>**Calendar feeds** — **New feed**: a private
  address with your events (and tasks with a due date, unless you untick it)
  from all your workspaces or the ones you pick. The address is shown **once**:
  copy it, or **Open in Calendar**. Add it in Apple Calendar (*File → New
  Calendar Subscription*), Google Calendar (*Other calendars → From URL*) or
  Outlook (*Add calendar → Subscribe from web*). Anyone with the address sees
  those events: revoke a feed (bin icon) to stop it.
- **Password**, **Two-factor** (authenticator app, recovery codes),
  **Passkeys**, **API tokens**, and **Active sessions** when the owner turned
  them on.

**API tokens**: **New token**, a name and an expiry. **Read-only** lets the
token read your workspaces but not change them. **Limit this token's instance
privileges** keeps only some admin privileges. The token is shown once. See
[API](API.md).

## 14. Administration

Your name → **Admin** appears when you hold an admin privilege. Tabs:
**Users** and **API tokens** (need **Manage users**), **Roles** (**Manage
roles**), and **Branding** / **Notifications** when the owner delegated them.
**Audit log** (your name → Audit log) needs **View audit log**: every sign-in,
account, role, workspace and membership change, with **Verify integrity**
(checks the tamper-evident chain) and **CSV** export. Admin screens update
live.

> Admins manage **accounts**, not content: they can't open other people's
> workspaces.

### Users

The table shows **Username** (with **you** on your own row), **Type** (`SSO`
or **Local**), **Email**, **Roles**, **Status**, **Last sign-in** (for
example "3 days ago", or **never**) and **Actions**.

> **Who can act on whom.** Nobody can change the **owner's** account from here
> (the owner does it in Settings), and only the **owner** can change an
> **admin's** account: admins can't reset, disable, sign out, edit or delete
> each other. The server refuses it ("only the owner can change an admin's
> account").

Status chips:

| Chip | Meaning |
|---|---|
| **active** | Can sign in. |
| **disabled** | Can't sign in or use API tokens. |
| **locked** | Too many failed sign-ins. Hover it to see until when. |
| **temporary password** | Must choose a new password at the next sign-in. |
| **two-factor** | Has TOTP and/or a passkey (hover it to see which). |
| **two-factor not set up** | Two-factor is required but not done yet. |

**Create a user**

1. Click **New user**.
2. Fill **Username**, **Display name** and **Email**.
3. Leave **Generate a temporary password** ticked (recommended). Or untick it
   and type a **Password (min 8)**, and choose whether to **Require password
   change on first login**.
4. Pick **Roles** — leave them alone for the default, `member` (can create
   workspaces); untick everything for none (only workspaces they're invited to).
5. Tick **Require two-factor** if the user must set up an authenticator or a
   passkey at the first sign-in.
6. Click **Create user**.

With a generated password, the **User created** box shows it **once**. Click
**Copy password** and hand it over safely. The user must choose a new one at
the first sign-in.

SSO users don't need creating. They appear at their first SSO sign-in.

**Row actions** (icon buttons; hover for the name):

| Action | What it does |
|---|---|
| **Edit** | Change display name, email, roles, **Disable account**, **Require two-factor**; **Reset 2FA** (clears a lost authenticator); **Reset passkeys** (removes all their passkeys). |
| **Unlock** | Shown only when locked. The user can sign in again at once. |
| **Reset password** | Local accounts only. **Generate a temporary password** (shown once), or type one and choose **Require a new password at next sign-in**. The user is signed out everywhere. |
| **Force sign-out** | Signs the user out of every device. They must sign in again. |
| **Disable** / **Enable** | Disable asks first, then signs the user out everywhere. They can't sign in or use API tokens until enabled. Enabling does not bring old sessions back. Not shown for yourself. |
| **Impersonate** | Only when the owner turned on impersonation and you hold **Impersonate users**. Signs you in as that user for up to 15 minutes, with a banner. Not for yourself, disabled users or the owner; only the owner may impersonate an admin. Actions show `via <you>` in the audit log. |
| **Delete** | Removes the account for good. Not shown for yourself. Workspaces the user was alone in are deleted with it; in shared ones another member (an editor first) becomes owner if they were the last. |

Safety rules the server enforces:

- Only the owner can create an admin or give or take the `admin` role.
- You can't disable or delete the last active admin, or disable yourself.

### Roles

Roles give **instance** privileges — running Orbit, not access to
workspaces. Each role is a card with its **Members** and **Access**. Built-in
roles (can't be edited):

| Role | Privileges |
|---|---|
| `admin` | everything except the owner-gated privileges (superuser) |
| `member` | **Create workspaces** (the default for new accounts) |
| `none` | nothing — only workspaces they're invited to (the default for new SSO users) |

**New role**: a name, a description, and its privileges: **Create
workspaces**, **Manage users**, **Manage roles**, **View audit log**, and —
granted only by the owner — **Impersonate users**, **Manage branding**,
**Manage notifications**. Add or remove members from a role's card.

### API tokens

Every user's API tokens: **Name** (with its prefix), **Owner**, **Limits**
(read-only, or the privileges it keeps), **Last used**, **Expires**. The bin
icon **Revoke**s one; anything using it stops at once.

## 15. Owner controls

The instance **owner** is one account with the final say: the first admin,
until they transfer it. Your name → **Owner** has these tabs: **Owner**,
**Branding**, **Notifications**, **Security**, **SSO**.

### Owner tab

**Feature flags** (only the owner changes them):

| Flag | Default | What it turns on |
|---|---|---|
| **Active sessions** | off | **Settings → Active sessions** for every user. |
| **User impersonation** | off | **Impersonate** for holders of **Impersonate users** — which shows that user's workspaces. |
| **Calendar subscriptions** | on | Subscribing workspaces to outside calendars. |
| **Calendar feeds** | on | Private ICS feeds in Settings. |
| **Step-up re-authentication** | off | **Confirm it's you** before security and SSO changes, feature flags, backups, owner transfer and impersonation. |

**Backups.** One encrypted archive of the whole database — accounts, roles,
every workspace and its content, settings and the audit log.

- **Download a backup now**: sealed with `ORBIT_MASTER_KEY` (restore it with the
  same key), or tick **With a passphrase** (12+ characters, typed twice) to
  keep it apart from the key. Either way the secrets inside — two-factor, SSO
  and notification secrets, calendar addresses — stay sealed with
  `ORBIT_MASTER_KEY`.
- **Automatic backups**: every N hours, keeping the N newest, to a
  **directory** in the container (mount a volume there) or an **S3** bucket
  (keys in `BACKUP_S3_ACCESS_KEY` / `BACKUP_S3_SECRET_KEY`). **Run now** tests
  the destination.
- **Restore**: stop Orbit and run `orbit restore-backup` (see
  [Configuration](CONFIGURATION.md#commands-in-the-binary)); the old database is
  kept as a copy.

**Security alerts.** Get a message when one of these happens:

- **Impersonation started**
- **Ownership transferred**
- **Admin role granted**
- **Security/SSO config changed**
- **Session token reuse (possible theft)**

Under **Notify via**, tick the channels (set them up in **Notifications**
first). With email, **Alert email recipient** defaults to the owner's email.

**Danger zone: Transfer ownership.**

1. Pick an active user in **Select a user…**.
2. Click **Transfer ownership**.
3. Type the user's name to confirm, and click **Transfer ownership**.

> You lose owner-only access at once. Only the new owner can give it back.

### Branding

**Application name**, **Accent colour**, **Show logo / application name in
sidebar**, a **Custom login page message**, and your own **logo** and
**favicon** (without them Orbit shows its planet icon).

**Developer themes** (off by default): turn on **Enable developer themes** and
pick one of five — **Steel Blue**, **Gold Leaf**, **Blood Red**, **Ink**,
**Forest Green**. A
theme sets the accent colour for everyone (the accent picker is locked, with a
note) and brings the UBonito Orbit crest (the blackletter "U" on Orbit's ring)
and the blackletter **UBonito** wordmark. Under **Apply the theme's assets — or
keep your own**, choose per asset: **Logo / mark** (theme crest, or your logo /
the default), **Wordmark** (UBonito or the app name), **Favicon** (the theme's,
or yours / the default). **Ink** follows light and dark mode (a bone crest in
light mode, a dark one in dark mode); the others look the same in both. While a
theme is on, **Show logo** and **Show application name** are greyed out: the
theme always shows its crest and wordmark. A personal accent (Settings →
Appearance) still wins over the theme's colour for that person, never over its
crest or wordmark. Turning the themes off puts everything back. Links and
titles use a darker or lighter shade of the theme's colour where the colour
itself would be hard to read on the page.

With a theme, the **sign-in page** stands on the theme's dark background, and
a phone or computer that installs Orbit (or adds it to the Home Screen) gets
the theme's crest as its app icon.

**Let each person choose** (under the switch): everyone can then pick another
theme, or none, for themselves in **Settings → Appearance → Developer theme**.
The theme chosen here stays the default and the sign-in page's.

**Live preview** shows the result before you save — the tab, the sidebar brand
and a button; **Save branding**.

> Try ↑ ↑ ↓ ↓ ← → ← → B A anywhere in Orbit.

### Notifications

The channels reminders and alerts can use. Turn one on, fill its settings and
click **Test**:

| Channel | Settings |
|---|---|
| **Email (SMTP)** | Host, Port, Username, Password, From address, Use STARTTLS |
| **Discord** | Webhook URL |
| **Telegram** | Bot token, Chat ID |
| **ntfy** | Server URL, Topic, Token (optional) |
| **Gotify** | Server URL, App token |
| **Microsoft Teams** | Workflow URL: in the Teams channel, **Workflows → "Send webhook alerts to a channel"**, and paste its URL. Messages come as an Adaptive Card. |
| **Slack** | Webhook URL of a Slack app's **Incoming Webhook** |
| **Integration webhook** | URL, Signing secret. A signed JSON POST for every audit-log event — accounts, roles, workspaces and their members, settings; never workspace content (HMAC-SHA256 in `X-Orbit-Signature`, the event in `X-Orbit-Event`). See [API](API.md#live-updates-and-the-webhook). |

Each user picks which of these their reminders also go to (Settings →
Reminders). Click **Save**.

### Security

| Switch | Effect |
|---|---|
| **Enforce two-factor authentication** | Every local user without two-factor must set it up at the next sign-in. SSO accounts are exempt (the provider handles MFA). |
| **Hide password login** | Hides the user name and password form when SSO is on. It is always back at `/login?local=1`. |
| **Auto-start SSO sign-in** | Opening the sign-in page goes straight to the SSO provider. Skipped at `/login?local=1`. Needs SSO on. |
| **Allow passwordless passkey sign-in** | Shows **Sign in with a passkey**. Passkeys as a second factor work either way. |

> Before you hide the password form or auto-start SSO, test an SSO sign-in in
> a private window, and bookmark `/login?local=1`.

### SSO

**Single sign-on (OIDC)**: the on/off switch, **Issuer URL** with **Test**,
**Client ID**, **Client secret** (write-only), **Redirect URL** (register this
exact URL at your provider, for example
`https://orbit.example.org/api/v1/auth/oidc/callback`), **Extra scopes**, **Role
claim** and **Default role**. Click **Save SSO settings**.

New users are created at their first sign-in. If a role claim is set, its
values are matched to role names at each sign-in. Otherwise, or with no match,
the user gets the default role.

Step-by-step guides: [SSO-entra](SSO-entra.md) (Microsoft Entra ID) and
[SSO-authentik](SSO-authentik.md).

## 16. On a phone

Open Orbit in Safari and **Share → Add to Home Screen** (Chrome on Android:
**Install app**). It opens full screen, without the browser bar. On a phone:

- the sidebar is behind ☰ (top left); ⌕ (top right) opens search;
- a **tab bar** at the bottom holds the current workspace's modules (or Today
  and Calendar);
- the calendar's Month shows dots, with the chosen day's list underneath;
- Tasks show the lists first, then a list;
- the Timeline keeps a narrow column of names; hold a bar to drag it;
- boards scroll sideways one column at a time; long-press a card to drag it.

Orbit needs a connection: it doesn't work offline yet ([roadmap](ROADMAP.md)).
