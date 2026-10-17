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

**Add an event** (instants in RFC 3339; `tz` is the zone a repeat keeps its
local time in; `remind_minutes` alerts the members before each occurrence):

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
| Invite links | `GET/POST /workspaces/{ws}/invites` (owners; `{role: editor\|viewer, days: 1–30, max_uses: 1–100}`, the answer carries the token once), `DELETE /workspaces/{ws}/invites/{id}`; public, rate-limited: `GET /invites/{token}` (what it's for), `POST /invites/{token}/signup` (`{username, password, display_name?, email?}` → signed in); signed in: `POST /invites/{token}/accept` |
| Courses | `GET/POST /workspaces/{ws}/courses` (with `slots`: the weekly classes `{id?, weekday 1–7, start, end, room, kind}`), `PUT/DELETE /courses/{id}` (`slots` given = the whole timetable; keep a class's `id` to keep its cancelled dates). Each class is a repeating event with `course_id`; `PUT /events/{id}` and deleting the series answer 409, `DELETE /events/{id}?occurrence=` cancels one |
| Grades (the caller's own) | `GET /workspaces/{ws}/grades`, `POST /courses/{id}/grades` (`{title, score, out_of?, weight?, date?}`), `PUT/DELETE /grades/{id}`, `PUT /courses/{id}/goal` (`{target}` on the scale, `null` clears); the scale is the workspace's `grade_scale` (20, 100, 10, 5; owners change it with `PUT /workspaces/{ws}`) |
| Pages | `GET/POST /workspaces/{ws}/pages`, `GET /workspaces/{ws}/trash`, `GET/PUT/DELETE /pages/{id}` (`?permanent=1` from the trash), `POST /pages/{id}/move`, `/restore`, `/duplicate`, `POST/DELETE /pages/{id}/favorite`, `GET /me/favorites` |
| Lists | `GET/POST /workspaces/{ws}/lists`, `PUT /workspaces/{ws}/lists/order`, `PUT/DELETE /lists/{id}` |
| Comments | `GET/POST /tasks/{id}/comments` (`{body}` Markdown; any member; `@username` notifies them), `PUT /comments/{id}` (the author), `DELETE /comments/{id}` (the author or an owner) |
| Tasks | `GET/POST /workspaces/{ws}/tasks`, `PUT/DELETE /tasks/{id}`, `POST /tasks/{id}/move` (board: `column_id`, `after_id`); `assignee_ids` is the list of who it's for (`assignee_id`, one, still works) |
| Calendar | `GET /calendar`, `GET /workspaces/{ws}/calendar`, `POST /workspaces/{ws}/events`, `GET/PUT/DELETE /events/{id}` (`?occurrence=` removes one) |
| Subscriptions | `GET/POST /workspaces/{ws}/subscriptions`, `PUT/DELETE /subscriptions/{id}`, `POST /subscriptions/{id}/refresh` |
| Uploads | `POST /uploads?target=page|task|note&target_id=` (multipart field `file`), `GET /uploads?target=&target_id=`, `GET /uploads/{id}/{name}` (raster images inline, anything else as a download), `DELETE /uploads/{id}` |
| Feeds | `GET/POST /me/feeds`, `DELETE /me/feeds/{id}`; the feed itself is `GET /ical/<token>.ics` (no header: the token is in the address) |
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
