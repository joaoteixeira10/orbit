# Troubleshooting

What to do when something in Orbit doesn't work: you can't sign in, a proxy
shows as unreachable, a change is refused, the page looks stale, or the dev
loop misbehaves. Each entry is a **symptom** (often the exact message you see)
and the fix. For settings see [CONFIGURATION.md](CONFIGURATION.md); for how
the parts fit together see [ARCHITECTURE.md](ARCHITECTURE.md).

Most answers start with the log:

```sh
docker compose logs --tail=200 api          # production (docker-compose.yml)
docker compose -f docker-compose.dev.yml logs --tail=200 api   # dev
```

## Sign-in

- **I don't know the first admin's password.** On an empty database Orbit
  creates `ADMIN_USER` (default `admin`). With `ADMIN_PASSWORD` set, that's the
  password. Without it, a temporary password is printed **once** in the log:

  ```sh
  docker compose logs api | grep "First sign-in"
  #   First sign-in: user admin, temporary password k7mq-8ra2-xw4n-p3de
  ```

  Either way you choose a new password at the first sign-in.
  `ADMIN_USER` and `ADMIN_PASSWORD` are read only when the database has **no
  users**; changing them later does nothing.

  > If the log is gone (the container was recreated) and nobody has signed in
  > yet, the only way back is a fresh database: stop the stack and remove the
  > data volume (`docker compose down -v`). **This deletes everything** in
  > Orbit, so do it only on a new install.

- **"invalid credentials" with the right password.** The account may be
  **locked**: after `LOGIN_LOCKOUT_THRESHOLD` (default 5) wrong passwords or
  codes in a row, it is locked for `LOGIN_LOCKOUT_MINUTES` (default 15). While
  locked, sign-in says `invalid credentials` even for the right password. Wait,
  or have an admin open **Admin → Users** (the user shows `locked`) and click
  **Unlock**. The lockout is in the audit log as `auth.lockout`.

- **"too many attempts; please slow down".** That's the **per-IP throttle** on
  the sign-in and code endpoints (HTTP 429, `AUTH_RATE_BURST` default 8,
  refilled at `AUTH_RATE_PER_MINUTE` default 12). Wait half a minute. If
  everyone behind the same reverse proxy hits it together, Orbit sees one IP
  for all of them: list the proxy in `TRUSTED_PROXIES` so it uses
  `X-Forwarded-For` (see [reverse-proxies.md](reverse-proxies.md)).

- **"invalid authentication code".** The six-digit code was wrong or expired.
  Check the phone's clock (TOTP codes depend on the time). The attempt counts
  toward the lockout.

- **I lost my authenticator app.** On the code step, choose **Use a recovery
  code** and enter one of the ten codes you saved when you set it up (each
  works once). Then set up the app again in **Settings**. Without recovery
  codes, ask an admin: **Admin → Users →** your user **→ Reset 2FA** (and
  **Reset passkeys** if you lost those too). You set it up again at the next
  sign-in if two-factor is required for you.

  > There is no command-line reset. If the only admin loses both the app and
  > the recovery codes, nobody can reset them. Keep the recovery codes, and have
  > a second admin.

- **SSO is down, or the login page only offers SSO.** Open **`/login?local=1`**:
  it always shows the password form and never starts SSO on its own, even with
  `hide_local_login` or `auto_sso` on. Local accounts (like the first admin)
  still work there. The SSO messages point at the cause:

  | Message | Cause |
  |---|---|
  | `SSO is not enabled.` | SSO isn't turned on (Owner → SSO) |
  | `The identity provider is unavailable.` | Orbit can't reach the provider or read its settings (see the log: `oidc begin failed`) |
  | `The identity provider returned an error.` | The provider refused (wrong client, redirect URL not registered, user not allowed) |
  | `SSO sign-in failed.` | The callback didn't check out (see the log: `oidc callback failed`) |
  | `Your account could not be provisioned (it may be disabled).` | The matching Orbit user is disabled, or couldn't be created |

- **Passkeys don't work** (`passkeys are not enabled on this server`, or the
  browser refuses). Passkeys need `WEBAUTHN_RP_ID` = the host name people type
  (e.g. `lb.example.org`) and `WEBAUTHN_RP_ORIGINS` = its full origin(s) (e.g.
  `https://lb.example.org`). Empty RP ID turns passkeys off; only dev defaults
  to `localhost`. Browsers want HTTPS (except on `localhost`) and a host name,
  not an IP address.

  > Changing `WEBAUTHN_RP_ID` later orphans every registered passkey. Users
  > sign in with their password and register them again.

## Proxies

- **"Proxy gmr is unreachable: its servers show as UNKNOWN and cannot be
  changed."** Orbit can't get the state from that proxy's lb-agent. The
  banner shows the error and how old the last data is. Orbit keeps showing
  the **last good state** but won't change those servers until the agent
  answers. Check the error under the banner:

  - **`lb-agent unreachable at http://…: connection refused`** (or a timeout):
    the agent isn't running, the port is wrong, or a firewall is in the way.
    From the Orbit host, `curl -H "Authorization: Bearer $AGENT_TOKEN" http://<agent>:<port>/v1/state`
    should answer JSON.
  - **Orbit runs in Docker and the agent URL says `localhost`**: inside the
    container, `localhost` is the container itself. Use the host's name or IP,
    or **`host.docker.internal`** for agents on the same machine (that's what
    the lab uses, e.g. `http://host.docker.internal:9001`). On Linux with
    Docker Engine that name exists only if the service has
    `extra_hosts: ["host.docker.internal:host-gateway"]`.
  - **`the agent refused the token (check it in Admin → Proxies)`**: the token
    stored for the proxy isn't the agent's `AGENT_TOKEN`. In **Admin → Proxies**,
    edit the proxy, paste the agent's token, and use **Test connection**. For the
    lab, the token is in `lab/<name>/.env`; an agent reads it only at start.
  - **`not polled yet`**: the proxy was just added or Orbit just started.
    It fills in after the first poll.
  - **`the stored agent token can't be read (was ORBIT_MASTER_KEY changed?)`**:
    see [Data](#data).

- **The data looks old.** The header shows how long ago the page was
  **updated**. Click **Refresh now** to poll every agent at once. If Orbit
  itself can't be reached, the page says `Orbit's API is unreachable.
  Showing the last known state.`

- **"outside your scope: …".** The change includes servers you don't have the
  privilege for (`servers.enable` for Ready, `servers.drain` for Drain,
  `servers.disable` for Maintenance). **Nothing** was changed, and the refusal
  is in the audit log. Check what you have on **Your access** (in the sidebar), or ask an admin
  to look at your role in **Admin → Roles**, where each scope rule shows how many
  servers it matches. A proxy with servers you can't see says
  `… servers of this proxy are outside your scope and not shown.`

- **Reset counters or the traffic test is greyed out.** Resetting counters
  touches the whole proxy: it needs `stats.reset` on **every** server of it
  (`resetting counters needs every server of this proxy in your scope`). The
  traffic test needs `traffic.test` on at least one of its servers.

- **"nginx rejected the change, rolled back: …".** The agent edited the
  upstream file, ran `nginx -t`, and nginx refused the configuration, so the
  agent put the files back. Nothing changed. `nginx -t` checks the **whole**
  configuration: the cause is often a hand edit elsewhere, and every change will
  fail until it's fixed. Run `nginx -t` on the proxy host and fix what it
  reports. The proxy's page shows `Last change failed at …`. HAProxy's
  equivalent is `HAProxy rejected the change, rolled back: …`.

- **"nginx has no drain state; use maint".** nginx can only put a server in
  maintenance (the `down` flag), not drain it. On nginx the page greys out
  Drain (`nginx has no drain state`); an API call with `drain` to an nginx
  proxy gets this error. Use
  `maint`, or drain on HAProxy.

- **A change worked on one proxy and failed on another.** Each proxy applies
  **all or nothing**, but proxies are independent: the one that worked keeps
  the change. The error names the proxy that failed (`gmr-hap: …`). Fix it and
  run the same change again; servers already in that state are skipped.

- **"unknown server …"**. The server isn't in the agent's configuration any
  more (renamed or removed since the last poll). Refresh and try again.

- **"No preview: this agent has no preview (older lb-agent)".** The agent
  predates `/preview`. Replace it with the Go `lb-agent` (see [AGENTS.md](AGENTS.md)).

- **The traffic test shows `unknown`, `connection error` or `HTTP …`.** It
  sends requests from **Orbit** to the proxy's **load balancer URL** (plus
  the backend's path) and counts the **`X-Backend`** response header:

  | Result | Meaning |
  |---|---|
  | `unknown` | The answer had no `X-Backend` header. The backend servers must send it (the lab machines add `X-Backend <env>_<machine>_<service>`). |
  | `connection error` | Orbit couldn't reach the load balancer URL, or the request took over 3 seconds. From Docker, use `host.docker.internal` or a real host name, as for the agent. |
  | `HTTP 404`, `HTTP 502`, … | The load balancer answered with an error: wrong path, or no healthy server in that backend. |

  With no URL at all: `this proxy has no load balancer URL (set it in Admin → Proxies)`.

## The page

- **The API reference is blank in dev.** Open it through Vite,
  **http://localhost:5173/api/docs**, not :8080: in dev, Vite serves Scalar's
  script. If it says `Scalar is not installed: run npm install (make dev restarts it).`,
  the dev container's packages predate the dependency. The containers install
  packages only when they start, so after a pull that changes `package.json`
  or `go.mod`:

  ```sh
  docker compose -f docker-compose.dev.yml restart web api
  ```

- **A change made on the proxy itself doesn't show.** Changes made through
  Orbit show at once (the server pushes them). Changes made outside it —
  a hand edit, the HAProxy socket, a server going down — show at the next
  poll. That's the **refresh interval**: the owner sets it in **Owner → Live
  updates** (1–300 seconds); the default is `PROXY_POLL_SECONDS` (10). It
  applies without a restart.

- **The page only updates on the interval, never at once.** The live stream
  (Server-Sent Events on `/api/v1/auth/events`) is being blocked or buffered,
  usually by a reverse proxy. The page still works, just slower. See
  [reverse-proxies.md](reverse-proxies.md).

- **The change dialog shows no diff or commands.** You, or an admin for
  everyone, chose **compact** confirmations. Switch in **Settings →
  Appearance**; an admin can enforce one style in **Admin → Proxies**.

## Data

- **`ORBIT_MASTER_KEY` was changed.** The key seals agent tokens, TOTP secrets
  and the SSO client secret; with a different key they can't be read:

  - proxies fail with `the stored agent token can't be read (was ORBIT_MASTER_KEY changed?)`;
  - authenticator codes are refused (`invalid authentication code`) —
    recovery codes still work, they are hashes;
  - SSO says `The identity provider is unavailable.`

  **Put the old key back** and restart. If it's lost: re-enter each proxy's
  agent token in **Admin → Proxies**, re-enter the client secret in
  **Owner → SSO**, and **Reset 2FA** for each user with an authenticator app
  (they then set it up again in **Settings**).

  > Keep `ORBIT_MASTER_KEY` with your backups. A database backup without its key
  > loses every sealed secret.

- **"Tampering detected at entry #…"** (Security → Audit → **Verify integrity**).
  An audit entry's content no longer matches its hash (`entry content altered`)
  or the chain has a gap (`broken chain link`: a row was deleted or inserted).
  Orbit never rewrites the chain, and there is no "repair": find out what
  happened. Check who can write the SQLite file (`DB_URL`, the `/data` volume),
  compare with the copy in syslog if `SYSLOG_ADDR` is set (each forwarded entry
  carries its hash), and restore the database from a backup if needed.

- **"unable to open database file" at start.** The container runs as the
  non-root user `65532`. A fresh named volume works (the image ships `/data`
  owned by that user); a **bind-mounted** host directory must be writable by
  uid 65532.

- **"orbit: configuration error: JWT_SECRET must be at least 32 characters
  in release mode"** (or the same for `ORBIT_MASTER_KEY`). Set both in `.env`
  (e.g. `openssl rand -hex 32`). Compose says `set JWT_SECRET in .env` when one
  is missing.

- **"the v0 audit history was already imported (use --force to import it again)".**
  `orbit import-v0-audit` runs once. `--force` appends the whole history
  again, duplicating it.

## Dev loop

- **Start it** with `make dev`: the API (Go, live reload with `air`) on
  **:8080** and the page (Vite) on **http://localhost:5173**. Sign in as
  `admin` / `admin` and choose a new password. Use :5173; the dev API on
  :8080 serves only a placeholder page
  (`Orbit API is running. The page isn't built yet (make dev serves it on :5173).`).
- **A port is taken.** The dev loop uses 8080 and 5173; the lab uses 8081–8083
  (load balancers) and 9001–9003 (agents). Stop whatever holds them.
- **I forgot my dev password.** `admin` / `admin` applies only to an empty
  database. Remove the dev data volume (`docker volume ls | grep orbit-dev-data`,
  then `docker volume rm …`) with the dev stack stopped, and start again.
- **Go or package changes aren't picked up.** Source edits reload by
  themselves (both watchers poll, because Docker Desktop bind mounts don't
  deliver file events). New dependencies need a restart:
  `docker compose -f docker-compose.dev.yml restart web api`.
- **`make lab-seed` says `skipped: no AGENT_TOKEN in …/.env`.** Create the
  lab tokens first (`lab/<name>/.env`, see [../README.md](../README.md#the-lab)),
  start the lab, and keep `make dev` running: `lab-seed` runs inside its API
  container.
