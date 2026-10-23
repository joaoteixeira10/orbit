# API for scripts

Everything the app does goes through Orbit's JSON API, so a script, a
shortcut on your phone or another service can add tasks, read your calendar
or create pages. Read this when you want to automate Orbit.

- **Reference**: `/api/docs` on your Orbit — every endpoint with its
  parameters and answers, and a client to try them (Scalar, served by Orbit
  itself). The raw OpenAPI document is `/api/v1/openapi.json`.
- **Authentication**: a personal **API token** from **Settings → API tokens**
  (shown once). It acts as you, in the workspaces you belong to. Tick
  **Read-only** for scripts that only read: they can't change anything in a
  workspace or create one. **Limit this token** narrows its instance
  privileges (admin work). Send it as `Authorization: Bearer orb_…`.
- Admins see and revoke everyone's tokens in **Admin → API tokens**. A token
  can't create tokens or calendar feeds, and never carries owner status.

```sh
export ORBIT=https://orbit.example.org
export ORBIT_TOKEN=orb_...        # Settings → API tokens (shown once)
auth=(-H "Authorization: Bearer $ORBIT_TOKEN" -H "Content-Type: application/json")
```

## Examples

**Your workspaces** (ids, modules, your role):

```sh
curl -s "${auth[@]}" $ORBIT/api/v1/workspaces | jq -r '.[] | "\(.id)\t\(.name)\t\(.role)\t\(.modules | join(","))"'
WS=<a workspace id>
```

**Add a task** to the workspace's first list (or `list_id`), due tomorrow at
14:30, priority high, tagged:

```sh
curl -s "${auth[@]}" -X POST $ORBIT/api/v1/workspaces/$WS/tasks \
  -d '{"title":"Essay","due":"2026-10-15","due_time":"14:30","priority":3,"tags":["maths"]}'
```

Dates are **floating** (`YYYY-MM-DD`, `HH:MM`), read in the viewer's time
zone, like Apple Reminders. `repeat` takes an RRULE (`FREQ=WEEKLY;BYDAY=MO`).

**Complete it** (a repeating task moves to its next date instead and the
answer says `"repeated": true`):

```sh
curl -s "${auth[@]}" -X PUT $ORBIT/api/v1/tasks/<task id> -d '{"completed":true}'
```

**Open tasks of a workspace** (default: open ones and those completed in the
last 14 days; `?state=done` every completed one, `?state=all` everything,
`?list=<id>` one list):

```sh
curl -s "${auth[@]}" "$ORBIT/api/v1/workspaces/$WS/tasks" | jq -r '.[] | select(.done | not) | "\(.due // "-")\t\(.title)"'
```

**Add an event** (instants in RFC 3339; `tz` is the event's own zone — the
one its times are typed in, and a repeat keeps its local time in; calendar
occurrences carry it as `tz`; `remind_minutes` alerts the members before
each occurrence):

```sh
curl -s "${auth[@]}" -X POST $ORBIT/api/v1/workspaces/$WS/events \
  -d '{"title":"Maths","starts_at":"2026-10-19T08:30:00Z","ends_at":"2026-10-19T10:00:00Z","tz":"Europe/Lisbon","rrule":"FREQ=WEEKLY;BYDAY=MO,WE","remind_minutes":10}'
```

All-day events: `"all_day": true` with `starts_at` / `ends_at` at UTC midnight
(end exclusive).

**Your calendar** across workspaces for a range (≤ 120 days): event
occurrences (series expanded, subscribed calendars included) and tasks due:

```sh
curl -s "${auth[@]}" "$ORBIT/api/v1/calendar?from=2026-10-01T00:00:00Z&to=2026-11-01T00:00:00Z" \
  | jq -r '.events[] | "\(.start)\t\(.title)"'
```

**Create a page** with content — pages are GitHub-flavoured Markdown:

```sh
curl -s "${auth[@]}" -X POST $ORBIT/api/v1/workspaces/$WS/pages -d '{
  "title": "Reading list",
  "content": "- [ ] Os Maias\n- [x] Sapiens"
}'
```

(Content sent in the editor's old JSON format is converted to Markdown.)
**Export** a workspace's pages as Markdown files in a zip:

```sh
curl -s "${auth[@]}" -o pages.zip "$ORBIT/api/v1/workspaces/$WS/export"
```

**Habits** (each person's own): `GET/POST /workspaces/{ws}/habits`, `PUT/DELETE
/habits/{id}` (`title`, `icon`, `color`, `schedule` daily | weekly, `days`
seven 0/1 from Monday, `target` per week, `remind_at` HH:MM, `archived`),
`PUT /habits/{id}/checks/{YYYY-MM-DD}` with `{done}`; a habit lists its
`checks` of the last 400 days.

**Drawings** (Excalidraw): `GET/POST /workspaces/{ws}/drawings` lists /
makes whiteboards; `GET/PUT/DELETE /drawings/{id}` — a `PUT` carries the
`version` it changes (a stale one answers `409` with the newer drawing) and
any of `title`, `scene` (Excalidraw JSON, images included, up to 16 MB),
`preview` (an SVG) and `text` (its words, for search). `POST
/notes/{id}/drawing` returns a note's drawing, made the first time.

**Notifications.** `POST /me/notifications/{id}/snooze` with `{until}` (RFC
3339, within a month) or `{minutes}` puts one off; it comes back unread and
is sent again. Quiet hours are prefs (`PUT /me/prefs`: `quiet_from`,
`quiet_to` as `HH:MM`, `""` for none; `quiet_reminders`). A note takes a
reminder with `remind_at` (RFC 3339; `""` removes it) on `PUT /notes/{id}`.

**Git: instant updates and milestones.** `POST /lists/{id}/git/hook` turns
on a webhook for a linked list — `{auto: true}` when Orbit made it, else
`{url, secret}` to add by hand; `DELETE` turns it off. The server posts to
`/api/v1/git/hook/{link}` (public, HMAC-SHA256 signed: `X-Hub-Signature-256`,
`X-Gitea-Signature` or `X-Forgejo-Signature`), which syncs the list at once.
`GET /lists/{id}/git/milestones` lists the repository's open ones
(`{ref, title, due_on}`); a task takes one with `git_milestone_id` (a `ref`,
`0` for none) and `git_milestone` (its title).

**Sync a workspace's calendar with a CalDAV calendar** (owners):
`POST /workspaces/{ws}/caldav/calendars` with `{url, username, password}`
lists the account's calendars; `PUT /workspaces/{ws}/caldav` with the same
and `{calendar_url, calendar_name}` links one and runs the first sync;
`GET` shows the link (`synced_at`, `event_count`, `last_error`), `POST
…/caldav/sync` syncs now (editors), `DELETE` stops it. Calendar changes made
through the API sync on their own.

**Move a workspace** to another Orbit: `GET /workspaces/{ws}/transfer`
(owners) downloads all of it as a zip (`workspace.json`, format
`orbit-workspace` v1, and `files/<id>`); `POST /workspaces/import` (multipart
`file`; `people=false` to leave the members out) makes a new workspace from
one and answers `{workspace, report}` — what came in, who was added, who has
no account here, which files didn't fit:

```sh
curl -s "${auth[@]}" -o ws.orbit.zip "$ORBIT/api/v1/workspaces/$WS/transfer"
curl -s "${auth[@]}" -F file=@ws.orbit.zip "$OTHER/api/v1/workspaces/import"
```

Saving a page's content needs the `version` you read (`PUT /pages/<id>` with
`{"content": …, "version": 3}`); if someone saved in between you get `409`
with the newer page in `page`.

**Search** titles and text in your workspaces:

```sh
curl -s "${auth[@]}" "$ORBIT/api/v1/search?q=essay" | jq -r '.[] | "\(.kind)\t\(.title)"'
```

## Endpoints

| Area | Endpoints |
|---|---|
| Overview | `GET /home?today=&from=&to=[&ws=]`, `GET /search?q=[&ws=]`, `GET /catalog` (modules and templates) |
| Workspaces | `GET/POST /workspaces`, `GET/PUT/DELETE /workspaces/{ws}`, `PUT /me/workspace-order` |
| Members | `GET/POST /workspaces/{ws}/members`, `PUT/DELETE /workspaces/{ws}/members/{uid}` (your own id to leave), `GET /users/lookup?q=` (exact username or email) |
| Activity | `GET /workspaces/{ws}/activity` (`?before=` an RFC 3339 time for older ones): newest first, 60 at a time, with `actor_name`; kept 90 days |
| Invite links | `GET/POST /workspaces/{ws}/invites` (owners; `{role: editor\|viewer, days: 1–30, max_uses: 1–100}`, the answer carries the token once), `DELETE /workspaces/{ws}/invites/{id}`; public, rate-limited: `GET /invites/{token}` (what it's for), `POST /invites/{token}/signup` (`{username, password, display_name?, email?}` → signed in); signed in: `POST /invites/{token}/accept` |
| Holidays | `GET /holidays/countries`; a workspace's `holidays` (a country code, `""` none; owners, `PUT /workspaces/{ws}`); calendars include them as `kind: "holiday"` all-day entries |
| Course pages, exams | `POST /courses/{id}/page` (its notes page: made once, then the same `{page_id}`); a task's `kind` is `exam` (or send `"task"` to clear); `GET /home` lists the next `exams` (60 days) |
| Courses | `GET/POST /workspaces/{ws}/courses` (with `slots`: the weekly classes `{id?, weekday 1–7, start, end, room, kind}`), `PUT/DELETE /courses/{id}` (`slots` given = the whole timetable; keep a class's `id` to keep its cancelled dates). Each class is a repeating event with `course_id`; `PUT /events/{id}` and deleting the series answer 409, `DELETE /events/{id}?occurrence=` cancels one |
| Grades (the caller's own) | `GET /workspaces/{ws}/grades`, `POST /courses/{id}/grades` (`{title, score, out_of?, weight?, date?}`), `PUT/DELETE /grades/{id}`, `PUT /courses/{id}/goal` (`{target}` on the scale, `null` clears); the scale is the workspace's `grade_scale` (20, 100, 10, 5; owners change it with `PUT /workspaces/{ws}`) |
| Pages | `GET/POST /workspaces/{ws}/pages`, `GET /workspaces/{ws}/trash`, `GET/PUT/DELETE /pages/{id}` (`?permanent=1` from the trash), `POST /pages/{id}/move`, `/restore`, `/duplicate`, `POST/DELETE /pages/{id}/favorite`, `GET /me/favorites` |
| Lists | `GET/POST /workspaces/{ws}/lists`, `PUT /workspaces/{ws}/lists/order`, `PUT/DELETE /lists/{id}` |
| Comments | `GET/POST /tasks/{id}/comments` (`{body}` Markdown; any member; `@username` notifies them), `PUT /comments/{id}` (the author), `DELETE /comments/{id}` (the author or an owner) |
| Tasks | `GET/POST /workspaces/{ws}/tasks`, `PUT/DELETE /tasks/{id}`, `POST /tasks/{id}/move` (board: `column_id`, `after_id`); `assignee_ids` is the list of who it's for (`assignee_id`, one, still works) |
| Calendar | `GET /calendar`, `GET /workspaces/{ws}/calendar`, `POST /workspaces/{ws}/events`, `GET/PUT/DELETE /events/{id}` (`?occurrence=` removes one) |
| Subscriptions | `GET/POST /workspaces/{ws}/subscriptions`, `PUT/DELETE /subscriptions/{id}`, `POST /subscriptions/{id}/refresh` |
| Uploads | `POST /uploads?target=page|task|note&target_id=` (multipart field `file`), `GET /uploads?target=&target_id=`, `GET /uploads/{id}/{name}` (raster images inline, anything else as a download), `DELETE /uploads/{id}` |
| Own channels | `GET/POST /me/channels` (`{kind: discord\|slack\|teams\|telegram\|ntfy\|gotify, name?, webhook_url \| bot_token + chat_id \| server_url + topic (+ token) \| server_url + token}`; https only; a test is sent first; secrets never come back, only `hint`), `POST /me/channels/{id}/test`, `DELETE /me/channels/{id}`; flag `personal_channels`; the kinds allowed are `personal` in `GET /notifications/channels`, set by the owner with `PUT /notifications/personal-kinds` (`{kinds}`; a channel of another kind is kept, `allowed: false`, and gets nothing) |
| Feeds | `GET/POST /me/feeds`, `DELETE /me/feeds/{id}`; the feed itself is `GET /ical/<token>.ics` (no header: the token is in the address) |
| Git sign-in | `GET/PUT /git/oauth-apps` (owner: `{apps: [{base_url, client_id, client_secret}]}` — a blank secret keeps the stored one; answers the `redirect_uri` to register), `POST /me/git/oauth/start` (`{base_url}` → `{url}` to send the browser to); the server returns to `GET /git/oauth/callback`, which ends on `/settings#git`. `GET /me/git` lists `sign_in` servers and each account's `via` (`token` or `oauth`). A list's bot: `PUT /lists/{id}/git/bot` (`{token}`, empty removes; whoever linked it or an owner) |
| Git | `GET/POST /me/git` (connect: `base_url`, `token`), `DELETE /me/git/{id}`, `GET /me/git/{id}/repos?q=`, `GET /me/git/{id}/boards?owner=&repo=` (labels, project boards); `POST /workspaces/{ws}/lists/git` (`account_id`, `owner`, `repo`, `mode`: `issues`, `labels` with `labels`, or `project` with `project_id`), `DELETE /lists/{id}/git`, `POST /lists/{id}/git/sync`; `POST/DELETE /tasks/{id}/git` (`url` of an issue or pull request). A task change that git refuses still saves and answers `git_error`. |

All under `/api/v1` except the feed. Accounts and administration (users,
roles, audit, settings, two-factor, passkeys, sessions) are in the reference.

## Errors

Errors are JSON: `{"error": "…"}`.

| Status | Meaning |
|---|---|
| `400` | Bad body — the message says what's expected (a date as `YYYY-MM-DD`, a repeat that isn't daily/weekly/monthly/yearly, an event ending before it starts, a range over 120 days, a calendar address that can't be fetched…) |
| `401` | No token, or an unknown, revoked or expired one, or a disabled account |
| `403` | Your role in the workspace doesn't allow it (a viewer writing), a read-only token writing, or an instance privilege you don't have |
| `404` | Not found — **including a workspace you aren't a member of** (its existence isn't revealed) |
| `409` | The workspace is archived; a stale page save (the body has the newer `page`); the last owner leaving; a member added twice |

## Live updates and the webhook

The app gets changes over Server-Sent Events (`/api/v1/auth/events`, opened
with a one-time ticket); scripts usually poll instead.

The **integration webhook** (Owner → Notifications) POSTs every **audit
event** — accounts, roles, workspaces and their members, settings, calendar
subscriptions and feeds; never workspace content:

```http
POST <your URL>
Content-Type: application/json
X-Orbit-Event: audit.workspace.member_add
X-Orbit-Signature: sha256=<HMAC-SHA256 of the body with your signing secret>

{"event": "audit.workspace.member_add", "time": "2026-10-14T09:30:00Z",
 "data": {"id": "…", "action": "workspace.member_add", "user_id": "…",
          "target_type": "workspace", "target_id": "…", "ip": "…",
          "detail": {"user": "ana", "role": "editor"}}}
```

Verify the signature over the exact body before trusting it.
