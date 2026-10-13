# API for scripts and pipelines

> For pipelines, [lbctl](LBCTL.md) does all of this in one command
> (`lbctl disable gmr_web01_facebook --for 20m --wait`).

Everything the page does goes through Orbit's JSON API, so a deploy
pipeline can take a server out of rotation, deploy, and put it back.

- **Reference**: `/api/docs` on your Orbit (e.g. http://localhost:8080/api/docs)
  — every endpoint with its parameters and answers, and a client to try them
  (built with [Scalar](https://scalar.com), served by Orbit itself; nothing
  loads from the internet). The raw OpenAPI document is `/api/v1/openapi.json`.
  The old `/api/v1/swagger/…` address sends you there.
- **Authentication**: a personal **API token** — create it in **Settings → API
  tokens** (shown once). It has your rights, or fewer: tick **Limit this token**
  to keep only some privileges and / or add scope rules (e.g. `env=gmr`), which
  is what a pipeline should get. Send it as `Authorization: Bearer orb_…`.
  In the reference, open **Authentication** and paste `Bearer orb_…`.
- Admins see and revoke everyone's tokens in **Admin → API tokens**.
- Every change is in the audit log with the token's owner, including refused
  ones, and the optional `reason` you send.

```sh
export LB=http://localhost:8080
export LB_TOKEN=orb_...        # Settings → API tokens (shown once)
auth=(-H "Authorization: Bearer $LB_TOKEN")
```

## Find the server ids

A change names each server by **proxy**, **backend** and **server** — the
server id is the address for nginx (`web01:8001`) and the server name for
HAProxy (`gmr_web01_facebook`). List them, with their state and what this token
may do on each:

```sh
curl -s "${auth[@]}" $LB/api/v1/state | jq -r '
  .proxies[] | .name as $p | (.state.backends // [])[] | .name as $b | .servers[]
  | "\($p)\t\($b)\t\(.name)\t\(.label // "-")\t\(.admin)\t\(.status)\tcan=\(.can | to_entries | map(select(.value).key) | join(","))"'
```

```
gmr      facebook  web01:8001           gmr_web01_facebook  ready  UP  can=ready,maint
gmr-hap  facebook  gmr_web01_facebook   gmr_web01_facebook  ready  UP  can=ready,maint,drain
```

Servers outside the token's `servers.read` scope aren't listed (`hidden`
counts them per proxy).

## Take a server out, deploy, put it back

```sh
body='{"reason": "deploy 1.4.2", "targets": [
  {"proxy": "gmr",     "backend": "facebook", "server": "web01:8001"},
  {"proxy": "gmr-hap", "backend": "facebook", "server": "gmr_web01_facebook"}]}'

# optional: what would change (nginx diff / HAProxy commands); nothing is applied
curl -s "${auth[@]}" -H "Content-Type: application/json" -d "$body" $LB/api/v1/operations/maint/preview | jq

curl -sf "${auth[@]}" -H "Content-Type: application/json" -d "$body" $LB/api/v1/operations/maint > /dev/null   # disable
# ... deploy web01 ...
curl -sf "${auth[@]}" -H "Content-Type: application/json" -d "$body" $LB/api/v1/operations/ready > /dev/null   # enable
```

On HAProxy you can drain first (`/api/v1/operations/drain`): no new
connections, existing ones finish. nginx has no drain and refuses it.

### Timed maintenance

Add `"minutes"` (1 to 10080) and a `"reason"` to a disable or drain: Orbit
puts the servers back in rotation by itself after that long, and logs it as
`system`. A pipeline that dies halfway then doesn't leave servers out:

```sh
curl -sf "${auth[@]}" -H "Content-Type: application/json" \
  -d '{"reason": "deploy 1.4.2", "minutes": 20, "targets": [{"proxy": "gmr", "backend": "facebook", "server": "web01:8001"}]}' \
  $LB/api/v1/operations/maint > /dev/null
```

Enabling the server earlier ends the timer.

### Drain, then maintenance

Add `"drain_first": true` (and optionally `"drain_timeout"`, 30 to 3600 seconds,
default 300) to a disable: on HAProxy the servers drain now and Orbit puts
them in maintenance once no sessions are left, or when the timeout is up
(logged as `system`); nginx servers go to maintenance at once. A `minutes`
timer starts when maintenance does. While it drains, `intent` has
`"drain_then_maint": true` and `"drain_until"`. Needs `servers.drain` too where
it drains. In `/state` each server has
`intent`: `{"admin", "until", "reason", "by", "at", "drift"}` — its desired
state, the timer, and `drift: true` when the proxy reports another state.

All targets on one proxy are applied **all or nothing**; with several proxies,
each proxy is one such unit (a failure on one doesn't undo another — the
answer says which failed).

The privileges: `ready` needs `servers.enable`, `maint` needs
`servers.disable`, `drain` needs `servers.drain`, each on every target.

## Endpoints

| Method + path | What |
|---|---|
| `GET /api/v1/state` | Every proxy, backend and server you may see, with `can` per server (`ready`, `maint`, `drain`); `?refresh=1` polls the agents first |
| `POST /api/v1/operations/{ready\|drain\|maint}` | Change servers. Body `{"targets": [{"proxy", "backend", "server"}], "reason": "…", "scope": "…"}` (`scope`: optional name of what the change covers, shown in the log; `empty_ok: true`: you accept that it may take a service's last live server out of rotation; `minutes`: timed maintenance, see above). Returns the new state |
| `POST /api/v1/reconcile/{restore\|accept}` | Drifted servers (`intent.drift`): `restore` puts them back in their desired state (needs the privilege of that state), `accept` takes what the proxy reports as the new desired state (needs the privilege of the state they are in). Body `{"targets": […], "reason": "…"}`. Returns the new state |
| `POST /api/v1/operations/{state}/preview` | Same body; returns per proxy `changes`, `diff` (nginx) and `commands`; `frozen` (a freeze covers it) and `frozen_bypass` (you're an admin); changes nothing |
| `POST /api/v1/weights` | Change weights (the `weights` feature flag): `{"targets": [{"proxy", "backend", "server", "weight"}], "reason"}`; `servers.weight` on each; the agent's range (HAProxy 0–256, nginx 1–1000); freezes; weight 0 counts for the service rule (`empty_ok`). `/weights/preview` changes nothing |
| `POST /api/v1/check` | `{"proxy", "backend", "server"}` — run that server's health check from its proxy, as configured (`servers.read` on it); returns the agent's answer (`desc`, `curl`, `ok`, `status`, `body`, `ms`, `error`, …) |
| `GET /api/v1/history?proxy=<name>&minutes=60` | Per-minute history (24 h kept): `series` per backend and server you may see (`server` empty = the backend), points `{at, req, err, p50, p95, up}` |
| `GET /api/v1/requests` | Maintenance requests you may see: yours, the ones you may decide (`can_decide`), pending or decided in the last day |
| `POST /api/v1/requests` | `{"kind": "maint"\|"extend", "targets": […], "minutes", "reason"}` — `servers.read` on each; an extension needs the servers in a timed maintenance |
| `POST /api/v1/requests/{id}/{approve\|deny\|cancel}` | `{"note", "allow_extend_minutes"}` — `maint.approve` on every server, not your own request; cancel: its author |
| `POST /api/v1/extend` | `{"targets", "minutes", "reason"}` — extend a timed maintenance: `servers.disable`, or within your pre-approval (`intent.allowance_minutes` / `allowance_by`) |
| `PUT /api/v1/settings/grace` | `{"minutes": 5-240, "repeats": 0-10, "end": "enable"\|"keep"}` (`proxies.manage`) |
| `PUT /api/v1/settings/agents-https` | `{"on": true\|false}` — agents must use HTTPS (`proxies.manage`); `409` with `proxies` (the names still on `http://`) when turning it on too early. While on, `http://` agent URLs get `400` in `/proxies` and `/proxies/check` |
| `GET /api/v1/certificates[?refresh=1]` | (`certs.read`; the `certificates` flag, else `404`) every proxy's certificates (`proxies[].certs[]`: id, kind, path, names, issuer, `not_after`, `days_left`, `managed`, …, or `error`), `managed`, `warn_days`, `pki: {url, configured}`, `can_manage` |
| `POST /api/v1/certificates/inspect` | (`certs.manage`) `{"cert_pem", "key_pem"}` → `{cert, problems, warnings}`; `400` if it can't be read or the key doesn't match |
| `POST /api/v1/certificates/deploy` | (`certs.manage`) `{"targets": [{"proxy", "id"}], "cert_pem", "key_pem", "force", "preview", "reason"}` → `results[]` per target (`ok`, `error`, `warnings`, the agent's `result`). Audited `cert.replace` with the fingerprint, never the key |
| `PUT /api/v1/certificates/settings` | (`certs.manage`) `{"warn_days": 1-365}` |
| `PUT /api/v1/certificates/pki` | (`certs.manage`) `{"url", "token"}` — PKI Management's address and a `pkim_` API token (sealed; empty keeps it); `{"url": ""}` disconnects |
| `GET /api/v1/certificates/pki/cas` | (`certs.manage`) PKI Management's CAs the token may read |
| `POST /api/v1/certificates/pki/issue` | (`certs.manage`) `{"ca_id", "ca_name", "common_name", "sans", "days", "targets", "auto_renew", "renew_days", "force", "reason"}` — Orbit makes a key and CSR, PKI Management signs it, the agents install it; saved as a managed certificate. `409` if it wouldn't cover the current names (unless `force`) |
| `POST /api/v1/certificates/managed/{id}/renew` · `PUT …/{id}` `{"auto_renew", "renew_days"}` · `DELETE …/{id}` | (`certs.manage`) renew now; change the renewal; stop managing |
| `GET /api/v1/rollouts` | Rolling actions (the `rolling` flag): running, paused, and those ended in the last day |
| `POST /api/v1/rollouts` | `{"name", "targets": […], "order": ["label", …], "reason", "drain_first", "drain_timeout", "wait_minutes" (0 = wait for next), "health_timeout"}`; at least two servers (labels); checked now and at every step, as you |
| `POST /api/v1/rollouts/{id}/{next\|pause\|resume\|cancel}` | Go on from waiting, pause, resume, cancel |
| `GET /api/v1/schedules` | Pending scheduled changes you may see (`?status=all`: the recent ones whatever their status) |
| `POST /api/v1/schedules` | Schedule a change: the operations body plus `"to"` and `"at"` (RFC3339, 30 s to 90 days ahead); a `reason` is required. Checked now (a preview) and again when it runs, as you, with your rights then. Returns it (`id`, `status: pending`) |
| `DELETE /api/v1/schedules/{id}` | Cancel a pending one (its author, or `proxies.manage`) |
| `GET /api/v1/freezes` | Change freezes on now or later |
| `POST /api/v1/freezes` | `{"from"?, "until", "reason", "rules"?: ["env=prd"]}` (`proxies.manage`) |
| `DELETE /api/v1/freezes/{id}` | End (or drop) a freeze (`proxies.manage`) |
| `POST /api/v1/proxies/{id}/test?backend=<name>&n=20` | Send test requests through the proxy; counts which server answered (`traffic.test` on one of its servers) |
| `POST /api/v1/proxies/{id}/reset` | Reset that proxy's counters (`stats.reset` on all of its servers) |
| `GET /api/v1/audit?q=<text>&page=1&per_page=100` | Audit log, newest first (`audit.read`) |
| `GET /api/v1/audit/export?q=<text>` | The whole log (or the matches) as CSV, with each entry's chain hash (`audit.read`) |
| `GET /api/v1/audit/verify` | Check the hash chain (`audit.read`) |
| `GET /api/v1/auth/me` | Who the token belongs to and what it may do |

Proxy ids (for `test` and `reset`) are the `id` of each proxy in `/state`.
Managing proxies, users, roles and settings is in the reference too.

## Errors

| Status | Meaning |
|---|---|
| `400` | Bad body (the message says what's expected), unknown proxy, `minutes` out of range or on an enable, or no `reason` where one is needed (a timed maintenance, or when an admin requires one) |
| `401` | No token, unknown, revoked or expired token, or a disabled account |
| `403` | Outside the token's privileges or scope, or a server Orbit doesn't know — the message lists them (and it's logged) |
| `404` | Unknown path or state, or unknown backend for the traffic test |
| `423` | A change freeze covers the servers (the message says until when and why); admins go through |
| `409` | A proxy didn't apply the change: it rejected it (e.g. `nginx -t` failed; rolled back there) or its agent is unreachable (the message has the reason per proxy); or the change would take the last live server of a **service** (the `<service>` name part, else the backend; counted across all proxies) out of rotation without `"empty_ok": true` in the body, and an admin hasn't allowed that (`emptied_services` lists them; the preview says so first, with `emptied` — backends left empty on their proxy — and `empty_allowed`) |
| `502` | The agent couldn't be reached (counter reset, test connection) |

Errors are JSON: `{"error": "…"}`. A pipeline should treat anything but `200`
as "not done".

## Webhook events

With the **Integration webhook** on (Owner → Notifications) and a server
event turned on there, Orbit POSTs each event as JSON:

```http
POST <your URL>
Content-Type: application/json
X-Orbit-Event: server.down
X-Orbit-Signature: sha256=<HMAC-SHA256 of the body with your signing secret>

{"event": "server.down", "time": "2026-10-01T12:30:00Z",
 "data": {"kind": "server.down", "proxy": "gmr",
          "servers": [{"proxy": "gmr", "backend": "facebook", "server": "web01:8001",
                       "label": "gmr_web01_facebook", "note": "connect() failed"}],
          "at": "2026-10-01T12:30:00Z"}}
```

| `event` | When | Also in `data` |
|---|---|---|
| `server.down`, `server.up` | Health checks fail / pass again | `servers[].note`: the check |
| `proxy.down`, `proxy.up` | The agent is unreachable (two polls) / back | `note`: the error |
| `server.change` | Enable, disable, drain, restore | `to` (`ready`, `maint`, `drain`; empty = restore), `user_id` (empty = Orbit itself), `reason`, `until`, `note` |
| `maint.expiring` | A timed maintenance ends in 15 minutes | `until`, `user_id`, `reason` |
| `server.drift` | A server is not in its desired state | `servers[].note`: "is ready, should be maint" |
| `lbadmin.failed` | A timer, a drain or an automatic restore failed | `note`: what and why |

Check the signature over the exact bytes received before trusting a payload.
