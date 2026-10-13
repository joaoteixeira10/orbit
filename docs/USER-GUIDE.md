# Orbit user guide

Orbit is the web page where you watch and control the servers behind your
nginx and HAProxy load balancers. Use it to take a server out of rotation
before a deploy, put it back after, see which backends are healthy, and check
who changed what. Admins also use it to manage users, roles and the list of
proxies. This guide covers the page itself, task by task. To install or
configure Orbit, see the [README](../README.md) and
[CONFIGURATION](CONFIGURATION.md). For scripts, see the [API](API.md).

## Contents

1. [Sign in](#1-sign-in)
2. [Find your way around](#2-find-your-way-around)
3. [How server names work](#3-how-server-names-work)
4. [Watch and change servers on the Proxies page](#4-watch-and-change-servers-on-the-proxies-page)
5. [Enable and disable from Operations](#5-enable-and-disable-from-operations)
   - [Certificates](#certificates) (under Proxies, in section 4)
6. [Confirm a change](#6-confirm-a-change)
7. [Read the audit log](#7-read-the-audit-log)
8. [Check your access](#8-check-your-access)
9. [Set up your account (Settings)](#9-set-up-your-account-settings)
10. [Administer users, roles, tokens and proxies](#10-administer-users-roles-tokens-and-proxies)
11. [Owner controls](#11-owner-controls)
12. [Privileges reference](#12-privileges-reference)
13. [Troubleshooting](#troubleshooting)

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
> `https://lb.example.org/login?local=1`). It shows every way in that is
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

---

## 2. Find your way around

### The sidebar

| Section | Item | What's there | Who sees it |
|---|---|---|---|
| **Proxies** | **Proxies** | One tab per proxy: its backends and servers. The number is how many proxies. | Everyone |
| | **Certificates** | The certificates every proxy and agent serves; replace them, PKI Management, renewals. | **See certificates**, with the owner's **Certificates** flag on (the default) |
| **Operations** | **Services** | One card per service, across every proxy. The number is how many services. | Everyone |
| | **Servers** | One card per machine. The number is how many machines. | Everyone |
| **Security** | **Audit** | Who changed what, where, and whether it worked. | **View audit log** |
| | **Your access** | What you may do, and on which servers. | Everyone |
| | **Admin** | Users, roles, API tokens, proxies, audit. | Anyone with an admin privilege |
| | **Owner** | Instance-wide settings. | The owner only |

A new item glows briefly when it appears, for example right after an admin
gives you a privilege. You don't need to reload.

Under the menu, **Recent activity** lists the last 30 server changes. Each
line shows the kind (**ENABLE**, **DISABLE**, **DRAIN**, **RESET**), the first
server (`+2` means more), how long ago, and who. A failed change shows
**FAILED**. Hover a line to see all its servers, the error and the reason.

At the bottom, the status line shows the version and whether an update is
available. Click it for details.

### The top bar

| Element | What it does |
|---|---|
| Breadcrumbs | Where you are, for example `lb / proxies / gmr-nginx`. |
| **updated 3s ago** | How old the proxy data on screen is. |
| Refresh button (**Refresh now**) | Asks every proxy for fresh data now. |
| Sun / moon button | Switches between light and dark. |
| Your name and role | Opens the user menu. |

The page refreshes by itself. It also updates at once when anyone makes a
change. The owner sets how often it polls (default every 10 seconds; see
[Live updates](#live-updates)).

### The user menu

| Item | What it does |
|---|---|
| **Settings** | Your own preferences and security (see [section 9](#9-set-up-your-account-settings)). |
| **Copy access token** | Copies your current session token, to paste as a Bearer token in a shell. It expires after about 15 minutes. For scripts, make an API token instead. |
| **Rotate token** | Revokes every access token issued to you so far (any you copied) and signs out your other sessions. This session carries on. API tokens are not affected. |
| **API reference** | Opens the interactive API docs (`/api/docs`) in a new tab. |
| **Sign out** | Ends this session. |

### Banners

- **Orbit's API is unreachable. Showing the last known state.** The page
  can't reach Orbit itself.
- **Proxy gmr-nginx is unreachable: its servers show as UNKNOWN and cannot be
  changed.** One agent is down. The banner says how old the last data is.
- **Viewing as … — impersonated by …** An admin is acting as this user. Click
  **Return to my account** to stop.

### On a phone

On a phone or a narrow tablet (under 1024 px) the sidebar folds away. Tap
**☰** at the left of the top bar to open it: the same menu, counts and
Recent activity slide in from the left. It closes when you pick a page, tap
outside it, or press Esc. On small phones the top bar hides the "updated …"
time and your name; your role and the user menu stay. Wide tables (Audit,
Certificates, Admin) scroll sideways inside their card.

**Install it** to open it like an app, full screen and without the browser bar:

- **iPhone / iPad (Safari):** Share → **Add to Home Screen**.
- **Android (Chrome):** ⋮ → **Install app** (or **Add to Home screen**).
- **Desktop (Chrome, Edge):** the install icon at the right of the address bar.

The installed app is the same page: it needs the network, signs in the same
way and always shows live state (nothing is cached for offline use). Its
icon, name and colour follow your organisation's branding. Installing needs
HTTPS (or `localhost`).

---

## 3. How server names work

Orbit reads each server's name as three parts:

```
<env>_<machine>_<service>
 gmr  _ web01   _ facebook      →  env=gmr  machine=web01  service=facebook
```

- **env** is the environment. It is letters, digits and `-`, and is stored in
  lower case.
- **machine** is the host. It has no `_`.
- **service** is everything after the second `_`, so it may contain `_`.

Operations groups cards by these parts. Scope rules on roles and API tokens
match against them (see [Scope rules](#scope-rules)). An admin can give each
environment a friendly label (for example `gmr` → `Staging`) in **Admin →
Proxies**.

A server whose name doesn't fit the pattern still works. Orbit then uses:
env = the proxy name, machine = the host part of its address, service = the
backend name.

---

## 4. Watch and change servers on the Proxies page

### Pick a proxy

Each proxy has a tab. The tab shows:

- a dot: green = agent reachable, red = agent unreachable;
- the name, engine (`nginx` or `haproxy`) and label;
- an amber count of servers in maintenance or draining, and a red count of
  servers down.

Under the tabs, the header shows the engine and version, node, agent URL and
public URL, **Uptime**, and the last change:

| Pill | Meaning |
|---|---|
| **Runtime API · changed 14:02:11** | HAProxy: last change applied live. |
| **Config applied · reloaded 14:02:11** | nginx: last change written and reloaded. |
| **Last change failed at 14:02:11** | Hover it to see what was tried. |
| **No changes since agent start** / **No reload since agent start** | Nothing changed yet. |

If the proxy rejected the last change, a red box says **The last change was
rejected (…); nothing was applied.** with the error.

If some servers of this proxy are outside your scope, a note says how many are
hidden.

### Read the numbers (KPIs)

| KPI | What it counts |
|---|---|
| **Active connections** | Open connections right now. |
| **Request rate** | Requests per second (HAProxy: current; nginx: last 10 s). |
| **Requests** | Since the counters were last reset. |
| **Bytes in** / **Bytes out** | Since the counters were last reset. |
| **Servers up** | Up out of total, with how many are draining or in maintenance. |
| **Servers down** | Down, with how many backends are degraded. Red when above zero. |

### Filter, search, expand

- **Filter backends or servers**: type part of a backend name, server name or
  address. Matching backends open by themselves.
- **Show**: **All**, **Needs attention**, **Down**, **Drain / maint**. Each
  button shows how many backends match.
  - **Needs attention**: a backend that is down, degraded, fully in
    maintenance, or has any server out of rotation.
- **Expand all** / **Collapse all** opens or closes every backend shown.

The first time you open a proxy, the backends that need attention are open.

### Backend rows

Each backend row shows **Backend** (name, path, balance method or file),
**Health**, **Requests**, **Rate /s**, **Errors**, **In** and **Out**. Click it
to open its servers.

| Health | Meaning |
|---|---|
| **Healthy 3/3** | All active servers are up. |
| **Degraded 2/3** | At least one is down, at least one is up. |
| **Down** | No server is up. |
| **Maintenance** | Every server is in maintenance or draining. |
| **Unknown** | The agent is unreachable. |

Backup servers only count when a backend has nothing else. The dots after the
label are one per server; hover a dot for its name and state.

Open a backend and click **Config** to see its configuration block.

### Server rows

Columns: **Status**, **Server** (name, a **BACKUP** tag, address and options),
**Wght**, **Sessions** (current, max and a bar), **Last** (time since the last
session on HAProxy, or the last request on nginx; "–" when there was none),
**Requests**, **Rate**, **Errors**, **In**, **Out**, **Last check** (check
result and latency, and "changed 5m ago"), **Downtime** (total time down, from
HAProxy; "–" on nginx). Backend rows have **Last** and **Downtime** too.

**Run health check** (⋯ menu, or **Run check** next to a server that fails its
checks) runs the server's check **from the proxy host**, exactly as the load
balancer is configured to — so you don't have to log in to the proxy and run
curl. The dialog shows what was run ("GET /dsstatus/isAlive → status 200"), the
same as a command line to copy (`curl -sS -i -k https://192.168.2.20:8080/…`),
the answer (`HTTP 500` and the body, e.g. `{"status": 500, "message": "fail to
connect to database"}`, indented when it's JSON), how long it took, whether it
**passes** or **fails** the expectation, and the configuration lines it came
from. A server without a health check shows **no check** instead (on HAProxy:
no `check` for it) — a dead one there still gets traffic. **No check** in the
filters shows the backends that have one.

**Last hour** is a sparkline of the requests per minute over the last hour
(a red tick: a minute it wasn't up the whole time), from Orbit's own
24-hour history. **p95** is the response time: nginx's 95th percentile over
the last 5 minutes (hover for p50 and how many requests), or HAProxy's average
over its last 1024 requests. Backend rows have both too.

| Status | Meaning |
|---|---|
| **Up** | In rotation and passing checks. |
| **Down** | Failing health checks. |
| **Maintenance** | Taken out by an operator. |
| **Draining** | HAProxy only: no new connections, current ones finish. |
| **Checking** | Health not known yet. |
| **Unknown** | The agent is unreachable. |

Next to the name a server can have:

- **⏱ 23m 10s left** — a timed maintenance: the server goes back in rotation by
  itself then. Hover for the time, the reason and who set it. Red: the last
  try to put it back failed (the agent was unreachable); Orbit keeps trying.
- **should be in maintenance** (or *in rotation*, *draining*) — drift: the
  proxy reports another state than the one last set through Orbit. See
  [Desired state and drift](#desired-state-and-drift).

### Change one server: the ⋯ menu

1. Click **⋯** at the end of the server row.
2. Under **Set state**, click one:

| Button | Effect | Engines |
|---|---|---|
| **Ready** | Back into rotation. | nginx and HAProxy |
| **Drain** | No new connections; existing ones finish. | HAProxy only |
| **Maintenance** | Out of rotation. | nginx and HAProxy |
| **Drain → maintenance** | Drains, then goes to maintenance by itself once no sessions are left (or when the wait you choose is up). The row says "draining → maintenance by 14:35" meanwhile. | HAProxy only |

On nginx, **Drain** is greyed out ("nginx has no drain state"). **Maintenance**
on nginx marks the server `down` in the config and reloads nginx. On HAProxy
the change goes through the Runtime API, with no reload.

The button for the current state is greyed out. If the server is outside your
scope, the ⋯ button is greyed out and says **Outside your scope**.

Every change asks first, one server too (see [section 6](#6-confirm-a-change)).

### Desired state and drift

Orbit remembers the state it last set on every server (its **desired
state**); a server seen for the first time keeps the state it was in. When a
proxy reports something else for two polls in a row — it was rebuilt,
restarted without its state, or someone changed it by hand — the server has
**drifted**:

- the Proxies page and Operations show a banner: "2 servers are not in their
  desired state", with each server, what it is and what it should be;
- the server's row says **should be …**, and its chip in Operations has a
  dashed red border.

In the banner:

| Button | Effect | Needs |
|---|---|---|
| **Restore** | Puts the servers back in their desired state (a timer it had is kept). | the privilege of that state (e.g. `servers.disable` to put one back in maintenance) |
| **Accept** | Keeps the servers as they are; that becomes their desired state. Nothing changes on the proxy. | the privilege of the state they are in |

Both ask first and are in the audit log (**RESTORE**, **ACCEPT**). An admin can
let Orbit restore drifted servers by itself (**Admin → Proxies → Desired
state**); it is then logged as **system**.

### Change several servers: select and use the bulk bar

1. Tick the box at the start of each server row. The box in the header selects
   every server of that backend you may change.
2. A bar appears: **3 servers selected**, with their names.
3. Click **Maintenance**, **Drain** (HAProxy only) or **Ready**.
4. Confirm in the dialog (see [section 6](#6-confirm-a-change)).

**Clear** empties the selection. **Esc** closes an open ⋯ menu first, then
clears the selection.

### Weights (canary releases)

With the owner's **Weights** feature flag on, an open backend has a
**Weights…** button (for holders of **Change weights**, `servers.weight`, on its
servers, and an agent that can). The dialog lists the servers with their weight
now, a **New** weight to type, and each one's **share of new traffic** among the
servers in rotation, updated as you type. For a canary, give the new version a
small weight (e.g. 1 against 9 → 10%), watch it, then raise it.

- HAProxy: 0 to 256, applied at once (0 = no new connections). nginx: 1 to
  1000, written to the upstream file and reloaded.
- **Apply** sends every change in one call per proxy, all or nothing. It is
  logged as **WEIGHT** ("gmr_web01_facebook @ gmr: 1 → 10") and is a server
  event (Changes).
- Change freezes apply, and a weight of 0 counts as out of rotation for the
  service last-server rule.

### Traffic test

The **Traffic test** panel sends real requests through the proxy and counts
which server answered each one.

1. Set **Send** (1 to 500, default 20).
2. Pick the backend in **requests to**.
3. Click **Run**.

The result shows each server's share as a bar. Orbit tells servers apart by
the `X-Backend` header in each answer. Red bars are errors (`HTTP …` or
`connection …`). You need **Traffic test** on at least one server of that
proxy. The proxy needs a **Load balancer URL** in **Admin → Proxies**.

### Reset counters

**Reset counters** (in the header) sets requests, errors and bytes back to
zero, for everyone. A dialog asks first; click **Reset**.

> You need **Reset counters** on **every** server of that proxy. If even one
> is outside your scope, the button is greyed out ("Needs every server of this
> proxy in your scope").

### The legend

The footer lists the states and how this engine changes them:

- HAProxy: "HAProxy Runtime API: ready / drain / maint".
- nginx: "nginx: maintenance = down flag + reload (no drain)".

---

### Certificates

**Proxies → Certificates** (needs **See certificates**; the owner's
**Certificates** flag, on by default) lists every certificate the proxies
serve — nginx's `ssl_certificate`, HAProxy's `crt` files — and the agents' own
(tagged **agent**, when an agent serves HTTPS). Orbit asks the agents every
10 minutes; **Refresh** asks now.

| Column | What it shows |
|---|---|
| **Proxy** | where the file is |
| **Certificate** | its name (CN) and every name it covers; tags: **agent**, **self-signed**, **PKI · auto-renew** (managed, see below) |
| **Expires** | days left — green, orange within the alert window (30 days by default), red within 7 days or expired — and the date |
| **Issuer** | who signed it, and the key type |
| **Used by** | the nginx `server` blocks (names and `listen`) or HAProxy `bind` lines that serve it |
| **File** | the certificate file, and the key file when it's separate |

The counters on top: **Certificates**, **Expiring** (within the alert window),
**Expired**, **Can't read** (a file the agent can't read or parse). A proxy
whose agent can't list its certificates shows a red line — an old (Python) agent
says "update it to the Go lb-agent". A 🔒 in the last column means the agent
can't write that file.

**Replace a certificate** (needs **Manage certificates**):

1. Click **Replace…** on its row.
2. Paste the certificate — the full chain: the certificate first, then the
   intermediates — and its private key (no passphrase), or **load** the
   files. The key goes to the agents only; Orbit never stores or logs it.
3. **Install on** lists the files you may replace, those with the same names
   first; the one you clicked is ticked. Tick every proxy that serves the same
   certificate.
4. **Check** reads it (name, names, issuer, expiry, key, chain) and asks each
   agent what it would do: the files it writes (the old ones are kept next to
   them as hidden `.<file>.prev`), then `nginx -t` and the reload, or HAProxy's
   `set ssl cert` + `commit ssl cert` (no reload).
5. **Can't be used** stops it: a key that doesn't match, an expired
   certificate, an encrypted key. **Warnings** need **Go ahead despite the
   warnings**: names the current certificate has that the new one doesn't
   cover, a chain out of order, no intermediates, no `serverAuth` usage, fewer
   than 7 days left, or — for an agent's own certificate — one Orbit
   wouldn't trust for that agent's URL (it would lose the agent).
6. **Replace on N files.** Each agent installs it; if nginx refuses it, the old
   files are put back. The audit log records the certificate's name,
   fingerprint, issuer, expiry and where it went — never the key — and the
   **Certificates** server event goes out.

**From PKI Management** (with PKI Management connected, below): **From PKI…**
on a row issues a new certificate and installs it. Choose the **CA**; the
**common name** and **names** start as the current certificate's; **valid
for** 0 days means the CA's default. Orbit makes the key and a request
(CSR), PKI Management signs it, Orbit installs it on the files you tick —
PKI Management never sees the key. **Renew it automatically** (on by default)
issues a new one (with a new key) that many days before it expires (30 by
default). A new certificate that wouldn't cover the current one's names is
refused unless you tick **Go ahead despite the warnings**.

**Managed by PKI Management** lists those certificates: the CA, the expiry,
where they're installed, **Auto-renew** and how many days before, the last
error. **Renew now** issues and installs one at once; **Stop managing** stops
the renewals (the certificate stays). A failed automatic renewal is tried again
every hour and sends a **Certificates** event.

**PKI Management** (needs **Manage certificates**): its **URL** and an **API
token** (`pkim_…`, from PKI Management → Settings → API tokens, whose owner
has `cert.issue` on the CA). The token is stored sealed and never shown again;
leave it empty to keep it. **Test** lists the CAs it can see. **Disconnect**
forgets both. A private CA in front of PKI Management: `PKI_CA_FILE`.

**Alert** sets how many days before expiry the first alert goes out (30 by
default); then at 7 days, 1 day and on expiry — once each, per certificate —
to the channels of **Owner → Notifications → Server events → Certificates**.

From a pipeline: `lbctl certs` lists them and exits 1 if one expires within
the alert window (`--days N` to choose).

## 5. Enable and disable from Operations

Operations shows every server by what it runs, across all proxies at once. Use
it when you think "turn off facebook on gmr", not "which proxy is that on".

- **Services** has one card per service (for example `facebook`), listing the
  machines that run it.
- **Servers** has one card per machine (for example `web01`), listing the
  services on it.

In Operations, **Enable** sets **Ready** and **Disable** sets **Maintenance**.
To drain, use the Proxies page.

### The top of the page

The header counts **Services**, **Servers**, **Targets** (distinct server
names), **Proxies**, **Maint** (maintenance, draining or mixed) and **Down**.

### Environment cards

There is one card per environment: **All GMR**, with the environment label,
the number of targets, the proxies, and "12 up · 1 maint · 2 down".

- **✓ Enable all** puts every server in that environment back in rotation.
- **✕ Disable all** takes every server in that environment out.

Both ask you to **type the environment name** (for example `gmr`) to confirm.

### Filter

- **Filter by name… (Ctrl+K)**: press **Ctrl+K** (or **⌘K**) to jump here.
  It matches server names, machines, services, environments, proxies and
  backends. **Esc** clears it.
- Environment: **All**, or one environment (for example **GMR**).
- Status: **All**, **Active**, **Maint**, **Down**. A card shows if any of its
  servers matches.

### Service and machine cards

The card header shows the name, the number of targets and the machines (or
services). The state badge on the right is one of:

| State | Meaning |
|---|---|
| `ACTIVE` | Everything up. |
| `PARTIAL` | Some servers in maintenance or mixed. |
| `DEGRADED` | Something is down. |
| `MAINT` | Everything out of rotation. |
| `CHECKING` | Health not known yet. |
| `UNKNOWN` | A proxy is unreachable. |

Each line is one server. It shows its machine (or service), environment, and a
chip per proxy it runs on, coloured by its state there. A flag shows `MAINT`,
`DRAIN`, `DOWN` or `MIXED` (different states on different proxies). Hover the
name for the full detail.

- **Enable** / **Disable** on a line changes that server **on every proxy that
  serves it**.
- **Enable all** / **Disable all** at the bottom of the card changes every
  server shown on the card. The line before them says what that covers: **All
  envs**, **All GMR** or **3 shown** when a filter narrows it. You **type the
  card's name** to confirm.

A button is greyed out when nothing would change. It is also greyed out, with
"Includes servers outside your scope", when any server it would change is
outside your scope.

---


### Health checks from Operations

The top of Operations counts **No check**: targets with a server that has no
health check (filter them with **No check**; they carry a dashed **no check**
tag). A target with a place that is down (or still being checked) has
**Check**, which runs those health checks from their proxies and shows the
answers, as on the Proxies page.

### Requests, approvals and extensions

**Request…** (on a target row, when you may see the servers but not disable
them) asks for a **timed maintenance**: choose how long (30 min … 8 h, or
minutes) and why. **Extend…** (on a target in a timed maintenance, or by
clicking its **⏱** tag on the Proxies page) adds time: at once if you may
disable the servers or an approver **pre-approved** it for you, otherwise it
asks.

The **Requests** panel (Operations and Proxies) lists what waits for you — the
**Services** item in the sidebar shows how many in orange. Users with **Approve
maintenance requests** (`maint.approve`) on all its servers can **Approve** or
**Deny** it (with a note), never their own. Approving a maintenance takes the
servers out as you, timed, logged "requested by vera: …". When approving you
can also let them **extend it by up to** 30 min … 4 h without asking again.
The requester can **Withdraw** it. Approvers get an email for each new request
and requesters for each decision.

**When the window ends with an extension request still unanswered**, it is
extended by a **grace** period (default 30 minutes) — at most a few times
(default 2), telling the approvers each time (and the admins from the
second). Then it goes back in rotation with an alert, or stays in maintenance
without a timer, as an admin chose (**Admin → Proxies → Pending extension
requests**). Approved during the grace, the extension counts from the
**original** end; denied during the grace, the servers come back in 5
minutes.

### Rolling actions

With the owner's **Rolling actions** flag on, a card with two servers or more
has **Roll…** (when you may enable and disable all of them). It goes through
the servers **one at a time**, each on every proxy it is on:

1. **Out** — disabled (with **Drain first**, HAProxy servers drain until no
   sessions are left or the wait is up); it waits until they show
   Maintenance.
2. **Waiting** — "gmr_web01_facebook is out — deploy it, then Next". Click
   **Next** (or `lbctl next <id>` from the deploy), or choose to go on after N
   minutes.
3. **Back** — enabled; it waits until every instance is **Up**, within the
   time you set (default 5 minutes).
4. The next server.

In the dialog choose the servers and their order (↑ ↓), what to wait for, the
health timeout, and a reason (required). The **Rolling actions** panel on
Operations and Proxies shows where each one is, with **Next**, **Pause** /
**Resume** and **Cancel** (its author, an admin of proxies, or someone who may
change all its servers). If a step can't be done — refused, a freeze, a
service's last server — or a server isn't healthy in time, it **stops**
(shown in red, notified as a failure); the servers not reached yet are
untouched. Every step is a normal change, logged under its author with
"rolling · <name> (2/4)".

## 6. Confirm a change

### When Orbit asks

| You change… | Orbit… |
|---|---|
| One server or several | Shows the confirmation. |
| Anything that leaves a backend with no server in rotation | Shows the confirmation with a warning (and, from a target's own button in Operations, refuses it when it's the service's last live server anywhere, unless an admin allowed it). |
| A whole environment or card (**Enable all** / **Disable all**) | Shows the confirmation, and you **type the name**. |

Servers already in the target state are skipped. If any server in the change
is outside your scope, nothing is applied and a message lists them.

### What the dialog shows

The dialog comes in two styles. You choose in **Settings → Appearance →
Change confirmations**, unless an admin set one for everyone.

**Detailed** (the default) shows what will change on each proxy:

- nginx: the config file diff, then the commands it runs (reload);
- HAProxy: "Runtime API, no reload:" and the commands;
- "Already in this state; nothing to do." for a proxy with nothing to change.

It also has **Reason (optional, kept in the audit log)**, for example `deploy
1.4.2, ticket OPS-123`.

**Compact** shows only what, where, and the **Current status** (for example
`Up ×2`). It shows the reason field only when a reason is needed.

**For** (Disable and Drain, both styles): **Until enabled again** (the default),
or 15 min, 30 min, 1, 2, 4, 8 or 24 hours — a **timed maintenance**. When the
time is up Orbit puts the servers back in rotation by itself and logs it
(**ENABLE** by **system**, "timed maintenance ended: …" with your reason). The
message after the change says when ("— back in rotation at 14:30"), the server
shows **⏱ … left**, and the activity feed shows "⏱ until 14:30". Enabling the
server earlier ends the timer; disabling it again sets a new one (or none).

**When**: **Now** (the default) or **Later, at** a date and time — a
**scheduled change**. It needs a reason, and the button says **Schedule**.
Nothing changes now; the change is listed under **Scheduled changes** at the
top of Operations and Proxies (when, what, for how long, why, who), where its
author — or an admin of proxies — can **Cancel** it. At its time Orbit
makes it **as its author, with their rights then**: the same checks as a
change made now (scope, a service's last server, change freezes), logged under
their name (the log entry says "scheduled"). If it is refused then, or LB
Admin wasn't running within 15 minutes of its time, it is not made, and it
shows as failed or missed (and is notified as a failure). To take a server out
from 02:00 to 03:00, schedule a Disable at 02:00 **For** 1 hour.

**Change freeze**: if a freeze covers the servers, the dialog says so — "Not
now: change freeze until …" and the button stays off — unless you're an admin
("You are an admin, so this goes through"). A change for later is checked when
it runs.

**Drain first** (Disable, when a server is on HAProxy): tick it and choose how
long to wait for the sessions (1 to 60 minutes, default 5). HAProxy servers
drain at once — no new connections — and Orbit puts them in maintenance when
their sessions reach 0, or when the wait is up, logged as **system** ("drained"
or "drain time up, 7 sessions left"). nginx servers in the same change go to
maintenance at once (nginx has no drain). With **For** set too, the timer
starts when maintenance does. Enabling the server while it drains cancels the
maintenance step. It needs **Drain** (`servers.drain`) as well as **Disable**.

**A timed maintenance needs a reason**: the button stays off until you type
one. An admin can require a reason for every change (**Admin → Proxies →
Reasons**); the field then says **Reason (required, …)**.

> **Warning:** "This leaves no servers in rotation for gmr-nginx/facebook.
> Requests there will fail (502/503)." It means users of that backend get errors
> until a server is back. A server counts as live when it is up (or still being
> checked) and ready; servers you can't see count too.
>
> **A target's own Enable / Disable button in Operations can't take a service's
> last live server out** (by default) — counting every proxy and environment, so
> a service with one server on rda and another on gmr can lose either one, but a
> service with a single server can't. The dialog says "Not from this button: …"
> and **Disable** stays off.
> Use the card's or the environment's **Disable all** (with the typed name) or the
> **Proxies** page, which show the warning and may go ahead — or an admin can allow
> it for the target buttons too (**Admin → Proxies → A service's last server**).

### Confirm

1. If asked, type the name shown after **Type to confirm:** (not
   case-sensitive).
2. Click the button: **Enable**, **Disable** or **Drain**.

**Cancel** or **Esc** closes it without changing anything. A message then says
what was done, for example "Disabled 3 servers on gmr-nginx, gmr-haproxy".

---

## 7. Read the audit log

**Security → Audit** (needs **View audit log**) lists everything that
happened, newest first. It updates when a new change arrives, as long as you
are on the first page and not searching.

- **Search who, what, where…** filters as you type.
- **Load older entries** fetches the next 100.

| Column | What it shows |
|---|---|
| **When** | Date and time, in your time zone. |
| **What** | For server changes: **ENABLE**, **DISABLE**, **DRAIN**, **RESET**. For everything else, the event name, for example `auth.login`, `user.update`. |
| **Who** | The user. Under it: `via admin` if done while impersonating, how v0 entries were made (session or token), and the IP. |
| **Targets** | The servers (`gmr_web01_facebook @ gmr-nginx`), or the user, role or setting touched. Long lists fold behind `+3 more`. |
| **Result** | `ok` or `failed`, the error, **Reason:** if one was given, and short details for other events. |

A **failed** or refused change has a red mark on the left, `failed` in
**Result**, and the error under it. Nothing was applied on that proxy.

**Imported v0 history.** If your team came from Orbit v0, an admin may have
imported its log. Those entries keep their original time. v0's actions show as
the same kinds (**ENABLE**, **DISABLE**, …) or as `v0.<action>` for the rest.
When a v0 actor has no v1 account, **Who** shows the v0 name.

**Verify integrity** checks the hash chain: **Chain intact (N entries)**, or
**Tampering detected at entry #…** if a stored entry was changed. **Download
CSV** saves the whole log — or what the search matches — with the chain hash
of each entry (the export itself is logged).

Group changes show what they covered on top of the targets, for example **All
GMR** or **facebook · All envs**; the recent activity feed shows the same
("All GMR · 14 servers").

---

## 8. Check your access

**Security → Your access** answers "what can I do here?".

- **Signed in as** your name and roles.
- **Your privileges**:
  - **Servers**: each server privilege you hold, with **every server** or the
    scope rules that limit it. "No access to servers yet — ask an admin for a
    role." means you have none.
  - **Administration**: your global privileges.
- **Servers you can change**: "You can change 12 of the 40 servers you can
  see.", then their names.

Buttons take you to **Proxies**, **Admin** (if you have access) and
**Settings**.

---

## 9. Set up your account (Settings)

Open it from the user menu. Settings follow you across devices. The left menu
jumps to each part.

### Account

- **Upload photo**, **Display name**, **Email address**, then **Save
  changes**.
- SSO accounts: name and email come from your identity provider and update on
  each sign-in.

### Appearance

- **Theme**: **Light**, **Dark** or **System**.
- **Accent colour**: your own accent, over the organisation's. Pick a swatch
  or a colour. **Reset to default** removes it.
- **Change confirmations**: **Detailed** ("What will change on each proxy: the
  nginx file diff or the HAProxy commands.") or **Compact** ("Just what, where
  and its current state."). If it says **Set for everyone by an
  administrator.**, the buttons are locked.

### Language & time

- **Language**: **English** or **Português**. A few screens are still
  English-only.
- **Timezone**: **Device**, or a fixed zone (`UTC`, `Europe/Lisbon`, …). Used
  for all dates and times.

### Notifications

- **Server alerts by email**: turn it on to get an email, at your account's
  address, about the servers you may see (your `servers.read` scope): a server
  goes down and comes back, an agent is unreachable and is back, changes made
  by others (not your own), a timed maintenance ends in 15 minutes, drift, and
  timers or automatic restores that failed.
- It needs the owner's **Email** channel; the switch is off until then.
- Click **Save preferences**.

The team channels (Discord, Telegram, ntfy, Gotify, Teams, Slack) carry the
organisation's server events, which the owner sets in **Owner → Notifications
→ Server events**.

### Password

**Current password**, **New password** (at least 8 characters), **Confirm new
password**, then **Update password**. Your other sessions are signed out. SSO
accounts don't have this; the provider manages the password.

### Two-factor

Shown for accounts that sign in with a password (not SSO).

**Set it up:**

1. Click **Set up two-factor**.
2. Scan the QR code with your authenticator app (or type the **Secret key**).
3. Type the 6-digit code in the six boxes. It is sent on the sixth digit (or
   click **Verify & enable**).
4. You now see **10 recovery codes**. They are shown **only now**. Click
   **Copy** or **Download** (`orbit-recovery-codes.txt`), store them safely,
   then click **Done**.

Each recovery code works once, in place of a code, if you lose your phone.

**Once it's on:**

- **Recovery codes** shows how many are left. **Regenerate**: type a current
  code, click **Generate**, and save the new set. The old codes stop working.
- **Disable two-factor**: click **Disable**, type a current code, and click
  **Disable**. This is refused while two-factor is required for you or for
  everyone.

### Passkeys

Shown when the server has passkeys configured and your browser supports them.
A passkey counts as two-factor.

1. Click **Add a passkey**.
2. Give it a name (for example "MacBook Touch ID").
3. Click **Create passkey** and follow the browser.

The list shows when each was added and last used. **Synced** means it is
backed up (for example in iCloud Keychain). The bin icon removes one.

### API tokens

Personal access tokens for scripts. A token has your rights, or fewer if you
limit it. Send it as `Authorization: Bearer <token>`. See [API](API.md).

1. Click **New token**.
2. Enter a **Name** (for example `CI deploy`) and pick **Expires**: **Never**,
   **30 days**, **90 days** or **1 year**.
3. Optional: tick **Limit this token (fewer rights than yours)**.
   - **Only these privileges (none ticked = all of yours)**: tick the ones the
     token needs. Only privileges you hold are listed.
   - **Only on servers matching (one rule per line, empty = wherever you
     can)**: scope rules, same syntax as roles (see [Scope
     rules](#scope-rules)). The token gets the overlap of your scope and these
     rules.
   - Pick at least one privilege or one rule, or untick the limit.
4. Click **Create token**.
5. **Copy the token now.** It is not shown again.

The list shows each token's prefix, any limits (**limited** with the
privileges and rules), when it was added, last used, and when it expires. The
bin icon revokes it; anything using it stops at once.

### Active sessions

Shown only when the owner turned on the **Active sessions** feature flag. It
lists the devices you are signed in on, with IP and last use. **Current** is
this one. **Revoke** signs one out. **Sign out all other sessions** keeps only
this one.

---

## 10. Administer users, roles, tokens and proxies

**Security → Admin** appears when you hold any admin privilege. You see only
the tabs you may use:

| Tab | Needs |
|---|---|
| **Users** | **Manage users** |
| **Roles** | **Manage roles** |
| **API tokens** | **Manage users** |
| **Proxies** | **Manage proxies** |
| **Branding** / **Notifications** | Delegated by the owner (non-owners only; the owner has them in the Owner area) |
| **Audit** | **View audit log** |

**View audit log** counts, so the built-in `operator` and `viewer` roles see
**Admin** with only the **Audit** tab.

Admin pages update live: when another admin changes a user or role, the row
glows briefly.

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
4. Pick **Roles**.
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
| **Delete** | Removes the account for good. Not shown for yourself. |

Safety rules the server enforces:

- Only the owner can create an admin or give or take the `admin` role.
- You can't disable or delete the last active admin, or disable yourself.

### Roles

Each role is a card with **Members** on the left and **Access** on the right.

- **Members** lists the users in the role. **×** removes one. **Add a
  member…** plus **Add** adds one. Seeing members needs **Manage users**.
- **Access** shows **Full access (superuser)**, or the global privileges, then
  each server grant: **every server** or its rules → its privileges.

Built-in roles have a **built-in** tag and can't be edited or deleted:

| Role | Gives |
|---|---|
| `admin` | Full access (superuser): every privilege on every server, except the owner-only ones. |
| `operator` | See, enable, disable and drain every server, traffic test, reset counters, and read the audit log. |
| `viewer` | See every server and the audit log. Changes nothing. |
| `none` | Nothing. The usual default for new SSO users until an admin gives them a role. |

A user's access is the **sum** of all their roles.

**Create a role**

1. Click **New role**.
2. Enter a **Name** (for example `Staging operators`) and an optional
   **Description**.
3. Under **Global privileges**, click the ones to give (**Select all** /
   **Clear all**). Owner-only ones have a lock and only the owner can give
   them.
4. Under **Server grants**, set up one or more grants. Each grant is:
   - **All servers** ("Every server on every proxy"), **or**
   - untick it and write **Scope rules**;
   - then tick the server privileges this grant gives.
5. **Add grant** adds another (for example "see everything" plus "change only
   staging"). The bin removes one.
6. Click **Save role**. Roles apply to their users at once.

#### Scope rules

A scope rule picks servers by their name parts (see
[section 3](#3-how-server-names-work)) and where they live.

| Key | Matches |
|---|---|
| `env` | The environment part (`gmr` in `gmr_web01_facebook`). |
| `machine` | The machine part (`web01`). |
| `service` | The service part (`facebook`). |
| `proxy` | The proxy's name in Orbit (`gmr-nginx`). |
| `backend` | The backend (pool) name. |

How rules combine:

- Each part is `key=glob`. `*` matches anything, `?` one character.
  Matching ignores case.
- **Parts on one line must all match.**
- **One rule per line. A server is in scope if any line matches.**

Examples:

```
env=gmr                          every server in gmr
service=facebook                 facebook, in every environment
env=rda machine=web0*            rda servers on web01, web02, …
proxy=gmr-haproxy backend=api    only the api backend on gmr-haproxy
```

As you type, the line under the box shows **Matches 7 of the 40 servers you
can see:** and the first few matches. A mistake shows **Not a rule part:
"…"**, naming the bad part. Only servers you can see are counted, so the
real total may be higher.

### API tokens tab

Every user's API tokens: **Name** (with its prefix), **Owner**, **Scope** ("all
of the owner's rights", or the privileges and rules it was limited to),
**Last used**, **Expires**. The bin icon **Revoke**s one. Anything using it
stops at once.

### Proxies tab

The list of proxies Orbit manages. Each runs `lb-agent` next to nginx or
HAProxy (see [AGENTS](AGENTS.md)). Columns: **Name**, **Agent** (a green lock
for an `https://` agent; an open grey lock for plain HTTP, where the token
travels unencrypted), **Load balancer URL**, **Agent token** (`stored` or
`missing`).

**Add or edit a proxy**

1. Click **Add proxy** (or the pencil on a row).
2. Fill:
   - **Name**: lower-case letters, digits, `-` and `_`. It appears in scope
     rules (`proxy=gmr-nginx`) and the audit log.
   - **Label**: a friendly name (for example `GMR`).
   - **Agent URL**: for example `http://proxy-gmr:9000`, or
     `https://proxy-gmr:9000` when the agent serves HTTPS (see
     [AGENTS, step 7](AGENTS.md#7-keep-the-agent-port-private-and-use-tls)).
   - **Agent token**: the agent's `AGENT_TOKEN`. It is stored sealed and never
     shown again. When editing, leave it empty to keep the stored one.
3. Click **Test connection**. You should see "Connected: haproxy on node …".
   A TLS problem says what to set, for example "trust the agent's CA with
   AGENT_CA_FILE" or "the agent wants Orbit's client certificate".
4. Fill **Load balancer URL** (where Orbit sends the traffic test) and
   **Public URL** (the link on the Proxies page; defaults to the load balancer
   URL).
5. **Position** sets the tab order.
6. Click **Add proxy** or **Save**.

**Remove** (bin icon) asks you to type the proxy's name. Orbit stops showing
and managing it. The proxy itself keeps running as it is.

**Change confirmations** sets the dialog style for everyone: **Each user
chooses**, **Always detailed** or **Always compact**. See
[section 6](#6-confirm-a-change).

**Environments** gives each environment a label: the key is the env part of
server names (`gmr`), the label is what people see (`Staging`). A label with
"prod" in it gets the globe icon in Operations. **Add environment**, then
**Save**.

**A service's last server** (off by default): whether a target's own **Enable /
Disable** button in Operations may take the last live server of a service
(counting every proxy and environment) out of rotation. Off, those are refused; group actions (**Disable all**, typed name) and
the Proxies page may, after the warning, either way. On, the target buttons may too.

**Reasons** (off by default): on, every change — on the page or through the
API — needs a reason. A timed maintenance always needs one.

**Desired state** (off by default): what happens to a server that drifted (see
[Desired state and drift](#desired-state-and-drift)). Off, it is only flagged,
with **Restore** and **Accept**. On, Orbit restores it by itself after the
poll that confirms the drift, logged as **system**; if that fails it waits 5
minutes before trying that proxy again.

**Agents must use HTTPS** (off by default): on, Orbit refuses `http://`
agent URLs — when adding or editing a proxy, testing a connection, and when
talking to an agent. It can only be turned on once every proxy's agent URL is
`https://`; otherwise it names the proxies still on plain HTTP.

**Change freezes**: while one is on, changes on the servers it covers are
refused — from the page, the API, `lbctl` and scheduled changes — except by
admins (superusers). Timed maintenance still ends on time, drains still finish.
Everyone sees a red banner ("Change freeze until Fri 18:00 · Black Friday ·
only admins can change servers it covers"), from a day before it starts. Add
one with **From** (empty = now), **Until** (at most 90 days), a **Reason** and,
optionally, scope rules for the servers it covers (e.g. `env=prd`; empty =
every server). **End it** stops it early (a planned one is deleted).

### Branding and Notifications (when delegated)

If the owner gave you **Manage branding** or **Manage notifications**, these
tabs appear here. They work as described under [Owner
controls](#11-owner-controls).

---

## 11. Owner controls

The instance **owner** is one account with the final say. It is not a role and
can't be granted. The first account is the owner. **Security → Owner** has
these tabs: **Owner**, **Branding**, **Notifications**, **Security**, **SSO**.

### Owner tab

**Feature flags.** All are off by default except **Certificates**. Only the
owner can change them.

| Flag | What it turns on |
|---|---|
| **Certificates** (on by default) | **Proxies → Certificates**, `/api/v1/certificates`, `lbctl certs`, the background checks, automatic renewals and expiry alerts. Off: all of that is gone (the agents keep serving their certificates). |
| **Active sessions** | **Settings → Active sessions** for every user. |
| **User impersonation** | The **Impersonate** button for holders of **Impersonate users**. |
| **Rolling actions** | **Roll…** on Operations cards, `POST /api/v1/rollouts` and `lbctl roll`. |
| **Weights (canary releases)** | The **Weights…** dialog on the Proxies page, `POST /api/v1/weights` and `lbctl weight`. |
| **Step-up re-authentication** | The **Confirm it's you** prompt before security and SSO changes, feature flags, owner transfer and impersonation. |

**Backups.** One encrypted archive of the whole database — users, roles,
proxies with their agent tokens, settings, desired states, scheduled changes,
freezes and the audit log.

- **Download a backup now**: sealed with `ORBIT_MASTER_KEY` (restore it with the
  same key), or tick **With a passphrase** (12+ characters, typed twice) to
  keep it apart from the key. Either way the secrets inside — agent tokens,
  two-factor, SSO and notification secrets — stay sealed with `ORBIT_MASTER_KEY`:
  restored with another key, Orbit starts but those must be set again.
  Downloading is logged and can send a security alert (**An encrypted backup is
  downloaded**).
- **Automatic backups**: every N hours (checked hourly), keeping the N newest,
  sealed with `ORBIT_MASTER_KEY`, to a **directory** in the container (mount a
  volume there) or an **S3** bucket (keys in `BACKUP_S3_ACCESS_KEY` /
  `BACKUP_S3_SECRET_KEY`). **Run now** writes one at once, to test the
  destination. The card shows how the last one went; a failure is retried
  every hour.
- **Restore**: stop Orbit and run `orbit restore-backup` (see
  [Configuration](CONFIGURATION.md#commands-in-the-binary)); the old database is
  kept as a copy.

<a id="live-updates"></a>**Live updates.** How often Orbit polls every proxy
and open pages refresh. Choose 1 to 300 seconds and click **Save**. The default
is 10 s (`PROXY_POLL_SECONDS`). **Use the default** returns to it. Shorter is
fresher but asks more of the agents.

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

- **Identity**: **Application name** and **Accent colour**.
- **Display options**: **Show logo in sidebar**, **Show application name in
  sidebar**, **Custom login page message** (a welcome line on the sign-in
  card).
- **Assets**: **Upload logo** (SVG or PNG) and **Upload favicon** (ICO, PNG or
  SVG, 32×32 recommended). **Remove** clears one. Without them Orbit shows
  its own mark: the dark proxy crest (one source, two live routes and a dashed
  standby), in the sidebar, on the sign-in page and as the favicon.
- **Developer themes** (off by default): turn on **Enable developer themes**
  and pick one of four — **Steel Blue**, **Gold Leaf**, **Blood Red**, **Ink**.
  A theme sets the accent colour for everyone (the accent picker is locked,
  with a note) and brings the UBonito proxy crest (the blackletter "U" on the
  proxy mark) and the blackletter **UBonito** wordmark. Under **Apply theme
  assets — or keep your own**, choose per asset: **Logo / mark** (theme crest
  or your logo / the default), **Wordmark** (UBonito or the app name),
  **Favicon** (theme or yours / the default). **Ink** follows light and dark
  mode (a bone crest in light mode, a dark one in dark mode); the others look
  the same in both. While a theme is on, **Show logo** and **Show application
  name** are greyed out: the theme always shows its crest and wordmark. A
  user's personal accent (Settings → Appearance) still wins over the theme's
  colour for that user, never over its crest or wordmark. Turning the flag off
  puts everything back as it was.
- **Live preview** shows the result before you save — the tab, the sidebar
  brand and a button, with the theme.
- Click **Save branding**.

> Try ↑ ↑ ↓ ↓ ← → ← → B A anywhere in Orbit.

### Notifications

The channels users and alerts can use. Turn one on, fill its settings, and
click **Test** (it saves your edits first, then sends a test):

| Channel | Settings |
|---|---|
| **Email (SMTP)** | Host, Port, Username, Password, From address, Use STARTTLS |
| **Discord** | Webhook URL |
| **Telegram** | Bot token, Chat ID |
| **ntfy** | Server URL, Topic, Token (optional) |
| **Gotify** | Server URL, App token |
| **Microsoft Teams** | Workflow URL: in the Teams channel, **Workflows → "Send webhook alerts to a channel"**, and paste its URL. Messages come as an Adaptive Card. |
| **Slack** | Webhook URL of a Slack app's **Incoming Webhook** |
| **Integration webhook** | URL, Signing secret. A signed JSON POST on Orbit events (HMAC-SHA256 in `X-Orbit-Signature`, the event in `X-Orbit-Event`). |

**Server events** (all off by default) — what goes to those channels for
everyone:

| Event | When |
|---|---|
| **A server goes down, and is back** | Its health checks fail / pass again. The same event for the same servers isn't repeated within 2 minutes (a flapping server doesn't flood the channel). |
| **An agent is unreachable, and is back** | Two failed polls in a row; then when it answers again. |
| **Changes** | Enable, disable, drain, restore — by a user, `lbctl` or Orbit itself (a timer ending, drain then maintenance, an automatic restore), with who, why and until when. |
| **A timed maintenance ends soon** | 15 minutes before, once per timer, with who set it and why (not for timers shorter than that). |
| **Drift and failures** | A server is not in its desired state; a timer, a drain or an automatic restore failed. |
| **Certificates** | A certificate a proxy or agent serves expires soon (at the alert threshold, 7 days, 1 day, expired — once each), a certificate was replaced, or an automatic renewal failed. Only to these channels (not to personal email alerts). |

**Send to**: the enabled team channels (all of them unless you untick some).
**Email to**: comma-separated addresses (needs the Email channel). The
integration webhook gets the same events as JSON. Messages are in English,
with times in UTC.

Click **Save**.

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
`https://lb.example.org/api/v1/auth/oidc/callback`), **Extra scopes**, **Role
claim** and **Default role**. Click **Save SSO settings**.

New users are created at their first sign-in. If a role claim is set, its
values are matched to role names at each sign-in. Otherwise, or with no match,
the user gets the default role.

Step-by-step guides: [SSO-entra](SSO-entra.md) (Microsoft Entra ID) and
[SSO-authentik](SSO-authentik.md).

---

## 12. Privileges reference

### Global privileges

| Id | Shown as | Allows |
|---|---|---|
| `users.manage` | **Manage users** | Create, edit, disable, reset, unlock, sign out and delete accounts; see and revoke every user's API tokens; see role members. |
| `roles.manage` | **Manage roles** | Create, edit and delete roles and their scope. |
| `audit.read` | **View audit log** | **Security → Audit** (search, verify integrity, download CSV). |
| `proxies.manage` | **Manage proxies** | Add, edit and remove proxies and their agent tokens; environment labels; the change confirmations policy. |
| `certs.read` | **See certificates** | **Proxies → Certificates**, `lbctl certs`. In the built-in operator and viewer roles. |
| `certs.manage` | **Manage certificates** | Replace certificates through the agents, connect PKI Management, issue and renew from it, the alert threshold. |
| `users.impersonate` | **Impersonate users** | Sign in as another user (not the owner; admins only by the owner). Needs the **User impersonation** flag. **Owner-only.** |
| `settings.branding` | **Manage branding** | App name, accent, logo, favicon, sign-in message. **Owner-only.** |
| `settings.notifications` | **Manage notifications** | Notification channels. **Owner-only.** |

**Owner-only** privileges: the owner holds them, and only the owner can put
them in a role. The `admin` role (superuser) does **not** include them.

Not privileges at all, but owner-only: feature flags, live updates, security
alerts, ownership transfer, the Security tab and SSO.

### Server privileges

Given on **every server** or on the servers matching a grant's scope rules.

| Id | Shown as | Allows |
|---|---|---|
| `servers.read` | **See servers** | See servers, their state and stats. Servers you can't see are hidden. |
| `servers.enable` | **Enable (ready)** | Put a server back in rotation (**Ready** / **Enable**). |
| `servers.disable` | **Disable (maintenance)** | Take a server out (**Maintenance** / **Disable**). |
| `servers.drain` | **Drain (HAProxy)** | Drain a server on HAProxy. |
| `servers.weight` | **Change weights** | Change server weights (**Weights…**, with the owner's flag on). |
| `maint.approve` | **Approve maintenance requests** | Approve or deny requests for maintenance and extensions on these servers (never your own); approving a maintenance takes the servers out on your behalf. |
| `stats.reset` | **Reset counters** | Reset a proxy's counters. Needs it on **every** server of that proxy. |
| `traffic.test` | **Traffic test** | Send test requests through a proxy. Needs it on at least one server there. |

---

## Troubleshooting

- **"invalid credentials" with the right password** → The account may be
  locked after failed attempts. Wait 15 minutes, or ask an admin to **Unlock**
  it in **Admin → Users**.
- **The password form is gone** → The owner hid it for SSO. Open
  `/login?local=1`.
- **Lost the authenticator app** → Use a recovery code (**Use a recovery
  code**), or ask an admin for **Reset 2FA** in **Edit user**.
- **No "Sign in with a passkey" button** → The owner has passwordless sign-in
  off, the server has no passkey domain set, or the browser doesn't support
  passkeys.
- **A server's ⋯ button or checkbox is greyed out** → The server is outside
  your scope, or its proxy is unreachable. Check **Your access**.
- **Drain is greyed out** → The proxy runs nginx, which has no drain state. Use
  **Maintenance**.
- **Reset counters is greyed out** → You need **Reset counters** on every
  server of that proxy.
- **Enable all / Disable all is greyed out** → Nothing would change, or it
  includes servers outside your scope.
- **Servers show "Unknown"** → The proxy's agent is unreachable. See the red
  banner, then [AGENTS](AGENTS.md).
- **An SSO user sees nothing** → They have the `none` role. Give them a role,
  or map a group with **Role claim** in the SSO settings.

More in [TROUBLESHOOTING](TROUBLESHOOTING.md).
