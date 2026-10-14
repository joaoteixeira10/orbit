# Monitoring Orbit

Orbit exposes Prometheus metrics at `/metrics` when `METRICS_ENABLED=true`
(no sign-in: keep it on an internal network or block it at the reverse
proxy). This folder runs Prometheus and Grafana next to it, with a dashboard
and alert rules ready to load.

## Metrics

| Metric | Labels | Meaning |
|---|---|---|
| `orbit_http_requests_total` | `method, route, status` | requests, by route template (bounded) |
| `orbit_http_request_duration_seconds` | `method, route` | latency histogram |
| `orbit_logins_total` | `method` (`password`, `oidc`, `passkey`), `result` | sign-in outcomes |
| `orbit_users` | — | accounts, disabled ones included |
| `orbit_workspaces` | — | workspaces, archived ones included |
| `orbit_pages` | — | pages not in the trash |
| `orbit_tasks_open` | — | tasks not completed |
| `orbit_notes` | — | quick notes |
| `orbit_calendar_subscriptions_failing` | — | subscribed calendars whose last refresh failed |

The `orbit_users` … `orbit_calendar_subscriptions_failing` gauges are counted
from the database when Prometheus scrapes. Go runtime and process metrics come
with the default registry.

## Run it

```sh
cd deploy && docker compose -f docker-compose.monitoring.yml up -d
```

Prometheus (`:9090`) scrapes `host.docker.internal:8080` (Orbit on the host).
If Orbit runs elsewhere, edit the target in `prometheus.yml`. Grafana
(`:3000`, admin / admin on first start) has Prometheus as a data source and
the dashboard provisioned.

- **Dashboard** (`grafana-dashboard.json`): users, workspaces, pages, open
  tasks, failing calendars, requests per second by status, p95 latency by
  route, sign-ins by method and result. Import it into another Grafana with
  *Dashboards → Import* and choose your Prometheus.
- **Alerts** (`prometheus-alerts.yml`): loaded by this stack's Prometheus.
  In your own, add it under `rule_files` and scrape Orbit with
  `job_name: orbit` (the rules use that job label). Rules: Orbit
  down, more than 2% server errors, p95 over a second, a burst of failed
  sign-ins, calendar subscriptions failing for 6 hours.
