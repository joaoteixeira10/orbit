# Orbit

One web admin for several load balancers — nginx or HAProxy. Take servers in
and out of rotation (enable / drain / disable) for one server, a whole service,
a machine or an environment, on every proxy that serves them, with a preview
of exactly what will change.

- Each proxy machine runs **lb-agent** (one static Go binary) next to its load
  balancer; Orbit talks to every agent with a per-proxy token (stored
  sealed), over HTTP, HTTPS or mutual TLS — chosen per agent.
- People sign in to Orbit itself: password with optional two-factor
  (authenticator app or passkey), passkeys, or SSO (OpenID Connect: Entra ID,
  Authentik, …). Roles give privileges on **every server** or only on the
  servers matching **scope rules** (`env=gmr`, `env=rda machine=web0*`).
- Every change and every refusal goes to a tamper-evident audit log
  (optionally also to syslog / your SIEM).

## What's where

| Path | What |
|---|---|
| [api/](api/) | Orbit's backend (Go, Gin, GORM + SQLite); API reference at `/api/docs` (Scalar), see [docs/API.md](docs/API.md). Also `lbctl` (`cmd/lbctl`) and **lb-agent** (`cmd/lb-agent`) |
| [web/](web/) | The page (Vue 3, Vite, Tailwind), English and Portuguese |
| [lab/](lab/) | Demo proxy machines: nginx (`gmr`, `rda`) and HAProxy (`gmr-hap`, in front of the same machines as `gmr`) |
| [docs/](docs/) | The guides and blueprints (see [Documentation](#documentation)) |
| [deploy/](deploy/) | Monitoring: Prometheus + Grafana in one command, the dashboard and alert rules |
| [BACKLOG.md](BACKLOG.md) | The plan, phase by phase |

The Python v0 (its admin and the lb-idp sign-in service) is retired; it's kept
at the git tag `v0-python`.

## Quick start

```sh
cp .env.example .env        # then set JWT_SECRET and ORBIT_MASTER_KEY (32+ characters each)
docker compose up -d        # harbor.ubonito.org/proxy_manager/orbit, on http://localhost:8080
```

**First sign-in.** On an empty database Orbit creates the admin account
`ADMIN_USER` (default `admin`), who is also the instance **owner**:

- with `ADMIN_PASSWORD` set, sign in with it;
- without it, a temporary password is printed once in the log:

  ```sh
  docker compose logs api | grep "First sign-in"
  # First sign-in: user admin, temporary password k7mq-8ra2-xw4n-p3de
  ```

Either way you choose your own password at the first sign-in.

**Add your proxies** in Admin → Proxies: a name (used in scope rules, e.g.
`gmr-nginx`), the agent URL and its `AGENT_TOKEN`, and the load balancer URL
for the traffic test. **Test connection** checks them before you save.

## Development

```sh
make dev                    # API with live reload on :8080, the page with Vite on http://localhost:5173 (admin / admin)
make test lint              # Go tests + golangci-lint, in containers
make web-test web-lint      # Vitest + ESLint
make lb-agent               # lb-agent binaries (linux amd64 / arm64) into dist/lb-agent
make e2e-lab                # Playwright against Orbit + a real nginx / lb-agent lab (all in compose)
make help                   # everything else
```

The dev containers install packages when they start: after `package.json` or
`go.mod` changes (e.g. a pull), restart them with
`docker compose -f docker-compose.dev.yml restart web api`.

## The lab

Three demo proxies and their machines, each a compose project:

```sh
for p in gmr rda gmr-hap; do                      # each proxy gets its own agent token
  echo "AGENT_TOKEN=$(openssl rand -hex 32)" > lab/$p/.env
done
(cd lab/gmr && docker compose up -d --build)
(cd lab/rda && docker compose up -d --build)
(cd lab/gmr-hap && docker compose up -d --build)  # after gmr (it joins gmr's network)
```

With `make dev` running, `make lab-seed` adds them to the dev instance (tokens
read from `lab/<name>/.env`, stored sealed), names the environments, and creates
test users (`olivia` operator, `victor` viewer, `gina` staging only, `noah` no
role) whose passwords it prints once. Or add them by hand in Admin → Proxies
(from Orbit in Docker, the host is `host.docker.internal`):

| Name | Agent URL | Load balancer URL | Token |
|---|---|---|---|
| `gmr` | `http://host.docker.internal:9001` | `http://host.docker.internal:8081` | `AGENT_TOKEN` in `lab/gmr/.env` |
| `rda` | `http://host.docker.internal:9002` | `http://host.docker.internal:8082` | `AGENT_TOKEN` in `lab/rda/.env` |
| `gmr-hap` | `http://host.docker.internal:9003` | `http://host.docker.internal:8083` | `AGENT_TOKEN` in `lab/gmr-hap/.env` |

and in Admin → Proxies → Environments: `gmr` → Staging, `rda` → Production.

## Coming from v0

- **Proxies**: v0's `PROXIES` JSON (same format) imported once at start-up into
  an empty registry; set `PROXIES` for the first start, then remove it.
- **Audit history**: `orbit import-v0-audit /path/to/v0/data/orbit.db`
  appends v0's log to the chained one, keeping the original times.
- v0's API tokens and lb-idp's users aren't moved: recreate them.

## Documentation

| Doc | What's in it |
|---|---|
| [User guide](docs/USER-GUIDE.md) | Using the page, task by task: sign-in, Proxies, Operations, confirmations, audit, Settings, Admin, Owner |
| [Configuration](docs/CONFIGURATION.md) | Every environment variable (required or not, default), secrets backends, settings made on the page |
| [Security](docs/SECURITY.md) | Threat model, sign-in and access, secrets at rest, audit, operator checklist, known risks |
| [Agents](docs/AGENTS.md) | Installing lb-agent next to nginx or HAProxy, server naming, the traffic test, adding a proxy |
| [Reverse proxies](docs/reverse-proxies.md) | Putting Orbit behind nginx, HAProxy, Caddy or Traefik with TLS |
| [SSO with Entra ID](docs/SSO-entra.md), [SSO with Authentik](docs/SSO-authentik.md) | Single sign-on and role mapping |
| [API](docs/API.md) | Scripts and pipelines: tokens, taking servers out and back, errors; the reference is at `/api/docs` |
| [lbctl](docs/LBCTL.md) | The command-line tool for deploy pipelines: `lbctl disable … --for 20m --wait`, patterns, exit codes |
| [Troubleshooting](docs/TROUBLESHOOTING.md) | Symptom → fix |
| [Architecture](docs/ARCHITECTURE.md) | How it's built: pieces, data model, the proxy subsystem, decisions |
| [Releasing](docs/RELEASING.md) | The release runbook (build and release are manual) |
| [Auth blueprint](docs/AUTH-BLUEPRINT.md), [Project blueprint](docs/PROJECT-BLUEPRINT.md), [Blueprint fit](docs/BLUEPRINT-FIT.md) | The designs Orbit follows and what must work |
| [Backlog](BACKLOG.md), [Changelog](CHANGELOG.md) | The plan, phase by phase, and what changed |

## License

[Apache-2.0](LICENSE).
