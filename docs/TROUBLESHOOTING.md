# Troubleshooting

What to do when something in Orbit doesn't work: you can't sign in, a
workspace or page won't open or save, a subscribed calendar won't load,
reminders don't arrive, or the dev loop misbehaves. Each entry is a
**symptom** (often the exact message you see) and the fix. For settings see
[CONFIGURATION.md](CONFIGURATION.md); for how the parts fit together see
[ARCHITECTURE.md](ARCHITECTURE.md).

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
  | `That account's email isn't one this Orbit lets in.` | The provider is kept to some email domains (Owner → SSO → Allowed email domains), and the account isn't of one, or its email isn't verified |

  SAML: the log says `saml begin failed` (Orbit couldn't read the identity
  provider's metadata — its URL, or the pasted XML) or `saml response
  refused` (the answer wasn't signed by a certificate in the metadata, came
  from another issuer, or wasn't for Orbit's Entity ID / Reply URL — check
  both at the provider against Owner → SSO). A sign-in started at the
  identity provider's portal (IdP-initiated) is refused on purpose: start it
  from Orbit's login page.

- **SCIM provisioning answers `401`.** It's off (Owner → SSO → Provisioning),
  or the identity provider has an old token: **Make a new token** and paste
  it there. Orbit answers `409` for a userName a local account already has —
  SCIM never takes over an account it didn't make.

- **"Locking needs Orbit opened over HTTPS".** Locking notes and pages
  encrypts in the browser, which browsers allow only on a secure page: HTTPS,
  or `localhost`. Put Orbit behind HTTPS (see
  [reverse-proxies.md](reverse-proxies.md)).

- **Passkeys don't work** (`passkeys are not enabled on this server`, or the
  browser refuses). Passkeys need `WEBAUTHN_RP_ID` = the host name people type
  (e.g. `orbit.example.org`) and `WEBAUTHN_RP_ORIGINS` = its full origin(s) (e.g.
  `https://orbit.example.org`). Empty RP ID turns passkeys off; only dev defaults
  to `localhost`. Browsers want HTTPS (except on `localhost`) and a host name,
  not an IP address.

  > Changing `WEBAUTHN_RP_ID` later orphans every registered passkey. Users
  > sign in with their password and register them again.

## Workspaces and pages

- **"workspace not found" / a link to a workspace sends you to Today.** You
  aren't a member. Workspaces are private to their members — admins included —
  so Orbit answers as if it didn't exist. Ask one of its owners to invite you
  (workspace `⋯` → Members).
- **No "+" next to Workspaces / "Ask someone to invite you…".** Your account
  has no `workspaces.create` privilege (role `none`, the default for new SSO
  users). An admin gives you the `member` role in **Admin → Users**.
- **"you need to be an editor of this workspace".** You're a viewer there. An
  owner can change your role.
- **"the workspace is archived; restore it to make changes".** An owner
  archived it; it is read-only until an owner restores it (workspace `⋯` →
  Settings → Restore workspace).
- **"a workspace needs at least one owner" / "the last owner can't leave".**
  Make someone else an owner first, or delete the workspace.
- **"Someone else changed this page while you were editing."** Another member
  saved the page after you opened it. **Load theirs** discards your unsaved
  edits; **Keep mine** saves yours over theirs. Nothing is lost silently.
- **"This page has content this editor can't show, so editing is off."** The
  stored document has a block this version's editor doesn't know (e.g. written
  through the API). It stays read-only so it isn't damaged.
- **A page is gone.** Look in the workspace's **Trash** (Pages → Trash, or the
  workspace `⋯`). Pages stay there 30 days.

## Calendars

- **Subscribing says "couldn't reach the calendar: …".** Orbit's server, not
  your browser, fetches the address: it must be reachable from the Orbit host
  (DNS, firewall, proxy). Loopback, link-local and cloud-metadata addresses are
  refused on purpose (`couldn't reach the calendar: safedial: no allowed
  address for host`).
- **"the calendar's server answered 401 / 403 / 404".** The address is wrong
  or private. Use the calendar's **secret address in iCal format**: Google
  Calendar → the calendar's *Settings and sharing* → *Secret address in iCal
  format*; iCloud → share the calendar as **Public Calendar** and copy the
  `webcal://` link; Outlook → *Settings → Calendar → Shared calendars →
  Publish a calendar* → the **ICS** link.
- **"the address didn't return an iCalendar (.ics) file".** It answered, but
  with a web page or something else — usually the calendar's normal sharing
  link instead of its ICS address.
- **"the calendar is larger than 8 MiB" / too many redirects.** Use a smaller
  calendar (or one per subject); Orbit keeps a quarter back and a year ahead.
- **A subscribed calendar shows an error in red** (Calendars sheet). The last
  refresh failed; the events from the last good fetch stay. Fix the address
  (edit it) or **Refresh now**.
- **"the stored address can't be opened (was ORBIT_MASTER_KEY changed?)".**
  See [Data](#data); then remove and add the subscription again.
- **Times are an hour off.** Times show in the browser's time zone. Repeating
  events keep their local time in the zone they were created in (e.g. 09:00 in
  Lisbon across daylight saving). Check the device's time zone.
- **My Orbit feed doesn't update in Apple Calendar / Google / Outlook.**
  Calendar apps fetch subscriptions on their own schedule: Apple Calendar
  every 5 minutes to weekly (*Get Info → Auto-refresh*), Google every few
  hours (no setting), Outlook every few hours. Orbit asks for 30 minutes but
  can't force it.
- **"calendar feeds are turned off on this instance" / no Calendars button.**
  The owner turned off the `calendar_feeds` or `calendar_subscriptions` flag
  (Owner → Owner → Feature flags).

## Reminders

- **No reminder for a task.** Only tasks with a **due time** remind (a date
  alone doesn't), and only open ones, at that minute, to the assignee (else the
  creator). Events remind only when they have an **Alert**.
- **The reminder comes at the wrong hour.** Task times are floating ("14:30
  wherever you are") and placed in your time zone: **Settings → Language &
  time → Time zone**. With "This device", Orbit uses the zone your browser
  last reported — open Orbit once on the device you use.
- **Only in the bell, nothing by email / ntfy / Telegram.** In **Settings →
  Notifications**, turn on **Channels** for that kind and tick a channel under
  **Your channels**. Channels exist only if the owner set them up (Owner →
  Notifications); email goes to the address in **Settings → Account**.
- **No notification on this device.** **Settings → Notifications → This
  device → Turn on**, and allow the site when the browser asks. If it says
  *Blocked*, allow notifications for the site in the browser's settings. On
  iPhone and iPad it works only in the app added to the Home Screen. **Send a
  test** checks the whole path. Push needs HTTPS (or `localhost`); behind a
  firewall, the server must reach the browsers' push services (Google,
  Apple, Mozilla) on 443.
- **Too many notifications from a shared workspace.** `⋯` next to its name →
  **Mute activity**, or turn **Shared workspace activity** off for devices.
- **No daily summary.** It's sent at the chosen time in your time zone, and
  not on a day with nothing due and no events.

## Data

- **`ORBIT_MASTER_KEY` was changed.** The key seals TOTP secrets, the SSO
  client secret, notification secrets and subscribed calendar addresses; with
  a different key they can't be read: authenticator codes are refused
  (recovery codes still work, they are hashes), SSO says `The identity
  provider is unavailable.`, subscriptions fail. **Put the old key back** and
  restart. If it's lost: re-enter the SSO client secret and the notification
  settings, **Reset 2FA** for users with an authenticator app, and re-add
  subscriptions.

  > Keep `ORBIT_MASTER_KEY` with your backups. A database backup without its key
  > loses every sealed secret.

- **"Tampering detected at entry #…"** (Audit log → **Verify integrity**). An
  audit entry's content no longer matches its hash, or the chain has a gap.
  Orbit never rewrites the chain: find out who can write the SQLite file
  (`DB_URL`, the `/data` volume), compare with the syslog copy if
  `SYSLOG_ADDR` is set, and restore from a backup if needed.
- **"unable to open database file" at start.** The container runs as the
  non-root user `65532`. A fresh named volume works; a **bind-mounted** host
  directory must be writable by uid 65532.
- **"orbit: configuration error: JWT_SECRET must be at least 32 characters
  in release mode"** (or `ORBIT_MASTER_KEY`). Set both in `.env`
  (`openssl rand -hex 32`).

## The app

- **It looks stale; changes by others don't appear.** The live stream
  (Server-Sent Events on `/api/v1/auth/events`) is being blocked or buffered,
  usually by a reverse proxy — see [reverse-proxies.md](reverse-proxies.md).
  Reloading the page always shows the current state.
- **"Add to Home Screen" doesn't install it as an app.** It needs HTTPS (or
  `localhost`). On Android / desktop Chrome, use the install icon in the
  address bar.

## Dev loop

- **Start it** with `make dev`: the API (Go, live reload with `air`) on
  **:8090** and the app (Vite) on **http://localhost:5180**. Sign in as
  `admin` / `admin` and choose a new password. Use :5180; the dev API on its
  own serves only a placeholder page.
- **A port is taken.** The dev stack uses 8090 and 5180 on the host. Stop
  whatever holds them, or change the left-hand ports in
  `docker-compose.dev.yml` (and `CORS_ORIGINS`, `WEBAUTHN_RP_ORIGINS`,
  `PUBLIC_URL` there to match).
- **I forgot my dev password.** `admin` / `admin` applies only to an empty
  database. With the dev stack stopped, remove the `orbit_orbit-dev-data`
  volume (`docker volume ls | grep dev-data`) and start again.
- **Go or package changes aren't picked up.** Source edits reload by
  themselves (both watchers poll). New dependencies need a restart:
  `docker compose -f docker-compose.dev.yml restart web api`.
- **The API reference is blank in dev.** Open it through Vite,
  **http://localhost:5180/api/docs**: in dev, Vite serves Scalar's script.
