# Orbit behind a reverse proxy

This guide puts Orbit behind nginx, HAProxy, Caddy or Traefik, with the proxy
terminating HTTPS. That is the usual production setup: the Orbit container
listens on plain HTTP on port `8080`, and the proxy in front handles TLS and the
public host name. You need it before you turn on passkeys or SSO, since both
depend on the public HTTPS address. If you'd rather have no proxy, Orbit can
serve HTTPS itself (step 8). All settings are in
[CONFIGURATION.md](CONFIGURATION.md).

Replace `orbit.example.org` with Orbit's public host name throughout.

## 1. Give Orbit its own host name

- Serve Orbit at the **root** of its own host name (`https://orbit.example.org/`).
  Its pages, the API under `/api/v1` and the redirects after SSO use absolute
  paths, so a sub-path such as `https://example.org/lb/` doesn't work.
- Proxy **everything** on that host to Orbit's port `8080`: the pages, the
  API, and the API reference at `/api/docs`.
- Keep port `8080` off the public network. With the stock `docker-compose.yml`,
  set `LB_PORT=127.0.0.1:8080` in `.env` so the port is published on loopback
  only, or don't publish it at all when the proxy is on the same Docker
  network.

## 2. Set Orbit for its public address

In Orbit's `.env`:

| Setting | Value | Why |
|---|---|---|
| `GIN_MODE` | `release` (the compose files set it) | enforces strong secrets and makes auth cookies `Secure` |
| `TRUSTED_PROXIES` | the proxy's address or CIDR as Orbit sees it | client IPs for rate limits and the audit log come from `X-Forwarded-For` only from these |
| `WEBAUTHN_RP_ID` | `orbit.example.org` | passkeys are bound to this host name; empty disables passkeys |
| `WEBAUTHN_RP_ORIGINS` | `https://orbit.example.org` | the full origin the browser uses (add `:port` if it isn't 443) |
| `PUBLIC_URL` | `https://orbit.example.org` | calendar-feed addresses are built from it (else from each request's host and `X-Forwarded-Proto`) |
| `COOKIE_DOMAIN` | empty | host-only cookies, right for one host |
| `CORS_ORIGINS` | empty | the pages and the API share one origin |

```sh
GIN_MODE=release
TRUSTED_PROXIES=127.0.0.1
WEBAUTHN_RP_ID=orbit.example.org
WEBAUTHN_RP_ORIGINS=https://orbit.example.org
PUBLIC_URL=https://orbit.example.org
```

**Which address goes in `TRUSTED_PROXIES`?** The one Orbit sees as the
connection's source. A proxy on the same host through a published Docker port
often shows up as the Docker bridge gateway (for example `172.17.0.1` or
`172.18.0.1`), not `127.0.0.1`. A proxy on the same Docker network shows up
with its container IP; use the network's CIDR. If every audit entry shows the
same private address, that's your proxy: add it.

**Behind Cloudflare as well?** Whether Cloudflare sits in front depends on the
host name's DNS record there:

| Record | The path | `TRUSTED_PROXIES` |
|---|---|---|
| **DNS only** (grey cloud) | browser → your proxy → Orbit | your proxy |
| **Proxied** (orange cloud) | browser → Cloudflare → your proxy → Orbit | your proxy **and** Cloudflare's ranges |

Each proxy on the path adds the address it got the request from to
`X-Forwarded-For`. Orbit reads that list from the right, skipping trusted
addresses, and takes the first one that isn't: the person's. When the record
is proxied but only your own proxy is trusted, Orbit stops at a Cloudflare edge
address instead (`172.64.…`, `162.158.…`, `104.16.…`), and the audit log and
the sign-in rate limit see Cloudflare, not people.

So for a proxied record, trust Cloudflare's ranges too, from
<https://www.cloudflare.com/ips/> (they change rarely; check them now and
then). Here `192.168.1.10` stands for your own proxy:

```sh
TRUSTED_PROXIES=192.168.1.10,173.245.48.0/20,103.21.244.0/22,103.22.200.0/22,103.31.4.0/22,141.101.64.0/18,108.162.192.0/18,190.93.240.0/20,188.114.96.0/20,197.234.240.0/22,198.41.128.0/17,162.158.0.0/15,104.16.0.0/13,104.24.0.0/14,172.64.0.0/13,131.0.72.0/22,2400:cb00::/32,2606:4700::/32,2803:f800::/32,2405:b500::/32,2405:8100::/32,2a06:98c0::/29,2c0f:f248::/32
```

Orbit doesn't read Cloudflare's `CF-Connecting-IP` header on purpose: anyone
who reaches your proxy without going through Cloudflare could send a fake one.
`X-Forwarded-For` is only believed as far as the proxies you trust added to it.

Both ways work. Proxied hides your public IP and puts Cloudflare's filtering in
front; DNS only is one hop less. If proxied:

- **SSL/TLS mode: Full (strict).** Cloudflare then talks HTTPS to your proxy and
  checks its certificate. "Flexible" sends the last hop as plain HTTP.
- **Live updates work as they are.** Cloudflare closes a connection that's
  quiet for 100 s; Orbit's event stream sends a heartbeat every 25 s.
- **Uploads:** Cloudflare's free and Pro plans refuse bodies over 100 MB, so an
  `UPLOAD_MAX_MB` above 100 won't work through it (the default is 25).
- **Leave Rocket Loader off.** It rewrites the page's scripts.

> **Empty `TRUSTED_PROXIES` trusts no proxy.** That's the safe default, but then
> every request seems to come from the proxy. All users share one sign-in rate
> limit, and the audit log shows the proxy's IP. Don't set it wider than your
> proxies, or a client could fake its IP with its own `X-Forwarded-For`.

> **The passkey RP ID is effectively permanent.** Changing `WEBAUTHN_RP_ID`
> later makes every registered passkey unusable. Pick the final host name first.

**SSO.** The **Redirect URL** in **Owner → SSO**, and the one registered at your
identity provider, use the public host:
`https://orbit.example.org/api/v1/auth/oidc/callback`. See
[SSO-entra.md](SSO-entra.md) and [SSO-authentik.md](SSO-authentik.md).

## 3. What the proxy must do

| Need | Why |
|---|---|
| Terminate TLS for `orbit.example.org` | auth cookies are `Secure` in release mode; over plain HTTP the browser drops them |
| Pass `Host` and set `X-Forwarded-For` | client IPs (with `TRUSTED_PROXIES`) |
| **Don't buffer** `/api/v1/auth/events` | a Server-Sent Events stream that pushes live updates (privilege changes, workspace changes, reminders). Buffered, the page stops updating live. |
| Read timeout of **60 s or more** on that stream | it's held open for as long as the page is; Orbit sends a heartbeat every 25 s |
| Accept bodies up to **4 MiB** | `MAX_BODY_BYTES` default; branding images are sent as data URLs. nginx's default is 1 MiB. |
| Add `Strict-Transport-Security` | Orbit sets CSP, `X-Frame-Options` and the other security headers itself, but not HSTS |
| Block `/metrics` from the internet | only there with `METRICS_ENABLED=true`; scrape it from inside |

Health checks: `GET /api/v1/healthz` (the process is up) and
`GET /api/v1/readyz` (`503` until the database is reachable).

## 4. nginx

```nginx
server {
    listen 80;
    server_name orbit.example.org;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    http2 on;
    server_name orbit.example.org;

    ssl_certificate     /etc/ssl/orbit.example.org/fullchain.pem;
    ssl_certificate_key /etc/ssl/orbit.example.org/privkey.pem;

    add_header Strict-Transport-Security "max-age=31536000" always;
    client_max_body_size 4m;

    proxy_set_header Host              $host;
    proxy_set_header X-Real-IP         $remote_addr;
    proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;

    location / {
        proxy_pass http://127.0.0.1:8080;
    }

    # Live updates (Server-Sent Events): no buffering, long read timeout.
    location = /api/v1/auth/events {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 1h;
    }

    location = /metrics {
        deny all;
    }
}
```

Orbit also sends `X-Accel-Buffering: no` on the stream, which tells nginx not
to buffer it. The explicit `proxy_buffering off` keeps it working if something
strips that header.

Then `TRUSTED_PROXIES=127.0.0.1` (or the Docker gateway, see step 2).

## 5. HAProxy

```haproxy
frontend https-in
    bind :80
    bind :443 ssl crt /etc/haproxy/certs/orbit.example.org.pem alpn h2,http/1.1
    http-request redirect scheme https unless { ssl_fc }
    http-request deny if { path /metrics }
    # replace any client-sent value, so it can't be faked
    http-request set-header X-Forwarded-For %[src]
    http-request set-header X-Forwarded-Proto https
    http-response set-header Strict-Transport-Security "max-age=31536000"
    default_backend orbit

backend orbit
    # the live-updates stream stays open; Orbit pings every 25 s
    timeout server 1h
    option httpchk GET /api/v1/healthz
    http-check expect status 200
    server app1 127.0.0.1:8080 check
```

HAProxy doesn't buffer responses, so the event stream works as is. It only
needs a `timeout server` above the 25 s heartbeat.

> Prefer `http-request set-header X-Forwarded-For %[src]` over `option
> forwardfor` here. `option forwardfor` adds a second header line next to one
> the client sent, and Orbit reads the first line.

`crt` is one PEM file with the certificate chain and the key. Then
`TRUSTED_PROXIES=127.0.0.1`.

## 6. Caddy

Caddy gets and renews the certificate itself, sets `X-Forwarded-For`
(replacing untrusted client values), and flushes `text/event-stream` responses
right away. So the config is short:

```caddy
orbit.example.org {
    header Strict-Transport-Security "max-age=31536000"
    respond /metrics 403
    request_body {
        max_size 4MB
    }
    reverse_proxy 127.0.0.1:8080
}
```

Then `TRUSTED_PROXIES=127.0.0.1` (or the Docker gateway, see step 2).

## 7. Traefik (Docker labels)

Traefik and Orbit on one Docker network, with Traefik's certificate
resolver (here `le`) and a `websecure` entrypoint on `:443` already set up.
Add this to Orbit's service, for example in a `docker-compose.override.yml`:

```yaml
services:
  api:
    networks: [proxy]
    environment:
      TRUSTED_PROXIES: 172.16.0.0/12     # the Docker network's CIDR
      WEBAUTHN_RP_ID: orbit.example.org
      WEBAUTHN_RP_ORIGINS: https://orbit.example.org
    labels:
      - traefik.enable=true
      - traefik.docker.network=proxy
      - traefik.http.routers.orbit.rule=Host(`orbit.example.org`)
      - traefik.http.routers.orbit.entrypoints=websecure
      - traefik.http.routers.orbit.tls.certresolver=le
      - traefik.http.routers.orbit.middlewares=orbit-hsts
      - traefik.http.middlewares.orbit-hsts.headers.stsSeconds=31536000
      - traefik.http.services.orbit.loadbalancer.server.port=8080
      - traefik.http.services.orbit.loadbalancer.healthcheck.path=/api/v1/healthz

networks:
  proxy:
    external: true
```

Traefik streams `text/event-stream` responses without buffering, and by
default replaces `X-Forwarded-For` from clients it doesn't trust. It has no
simple per-path block, so leave `METRICS_ENABLED` off, or scrape `/metrics`
from inside the Docker network.

Set `LB_PORT=127.0.0.1:8080` in `.env` so the stock `ports:` line doesn't
publish `8080` to the world.

## 8. Or let Orbit terminate TLS itself

With nothing in front, Orbit can serve HTTPS on its own port:

```sh
TLS_CERT_FILE=/certs/orbit.example.org.crt   # certificate chain, PEM
TLS_KEY_FILE=/certs/orbit.example.org.key    # private key, PEM
```

- Set **both** or neither; one alone stops Orbit at start.
- It then serves HTTPS (TLS 1.2 or later) on `HTTP_ADDR` (default `:8080`), and
  auth cookies are `Secure` even outside release mode.
- The files are read at start. **Restart Orbit after renewing the
  certificate.**
- Mount the files into the container (for example `./certs:/certs:ro`) and
  publish the port you want, e.g. `LB_PORT=443`.
- `TRUSTED_PROXIES` stays empty: there's no proxy.
- Health checks now use `https://`.

You can also combine both: the proxy terminates the public TLS and talks HTTPS
to Orbit, to encrypt the hop between them.

## 9. Check it

1. `curl -s https://orbit.example.org/api/v1/healthz` returns `{"status":"ok"}`.
2. Sign in. In the browser's developer tools, **Network**: the `events` request
   stays open (type `eventsource`) instead of finishing.
3. Open **Admin → Users** in two tabs and add a user in one. The other tab
   shows it without a reload.
4. In **Security → Audit**, your own sign-in shows your real IP, not the proxy's.

## Troubleshooting

- **Signed in, then thrown back to the login page on the next reload** → The
  browser got `Secure` cookies over plain HTTP and dropped them. Open the site
  over `https://`, or (dev and test only) set `COOKIE_SECURE=false`.
- **The page doesn't update live; changes show only after a reload** → The
  proxy buffers `/api/v1/auth/events` or closes it early. Turn buffering off
  for that path and raise the read timeout (step 4 to 7).
- **The audit log shows the proxy's IP for everyone; users get "too many
  attempts; please slow down" together** → `TRUSTED_PROXIES` is empty or
  doesn't contain the address Orbit sees. See step 2.
- **The audit log shows Cloudflare addresses** (`172.64.…`, `162.158.…`,
  `104.16.…`) → The DNS record is proxied (orange cloud) and Cloudflare's
  ranges aren't in `TRUSTED_PROXIES`. See "Behind Cloudflare as well?" in
  step 2.
- **Passkeys: the browser refuses to create or use one** →
  `WEBAUTHN_RP_ID` must be the public host name (no scheme, no port), and
  `WEBAUTHN_RP_ORIGINS` the exact `https://` origin in the address bar.
- **Passkey options missing** → `WEBAUTHN_RP_ID` is empty in release mode,
  which disables passkeys.
- **SSO: redirect URI mismatch at the identity provider** → The **Redirect
  URL** in **Owner → SSO** must use the public host:
  `https://orbit.example.org/api/v1/auth/oidc/callback`, registered identically at
  the provider.
- **Saving a logo fails with `413`** → The proxy's body limit is lower than
  4 MiB (nginx defaults to 1 MiB). Set `client_max_body_size 4m;`.
- **Blank page or 404s under a sub-path** → Orbit needs its own host name at
  `/` (step 1).
- **Orbit stops at start: "TLS_CERT_FILE and TLS_KEY_FILE must be set
  together"** → Set both, or neither.

More in [TROUBLESHOOTING.md](TROUBLESHOOTING.md) and [SECURITY.md](SECURITY.md).
