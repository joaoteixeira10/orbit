# Monitoring Orbit

Orbit serves Prometheus metrics at `/metrics` when `METRICS_ENABLED=true`
(no sign-in: keep it on an internal network or block it at the reverse proxy).
Besides its own HTTP metrics (`orbit_http_*`), it exports the last poll of every
proxy:

| Metric | Labels | What |
|---|---|---|
| `lb_proxy_reachable` | `proxy` | 1 when the last poll of the agent worked |
| `lb_server_up` | `proxy, backend, server, label, env, service` | 1 when the server passes its health checks |
| `lb_server_in_rotation` | same | 1 when it is up and ready (gets traffic) |
| `lb_server_maintenance` | same | 1 in maintenance or draining |
| `lb_server_requests_total`, `lb_server_errors_total` | same | counters (since the agent's counters were reset) |
| `lb_server_latency_ms` | same + `quantile` (`0.5`, `0.95` nginx; `avg` HAProxy) | response time |
| `lb_server_sessions`, `lb_server_weight` | same | current sessions (HAProxy), weight |
| `lb_backend_requests_total` | `proxy, backend` | requests per backend |
| `lb_cert_expiry_timestamp_seconds` | `proxy, kind, path, cn` | when each certificate a proxy (`kind="lb"`) or agent (`kind="agent"`) serves expires, Unix time (the `certificates` flag; refreshed every 10 minutes) |

## Turnkey: Prometheus + Grafana

```sh
# 1. METRICS_ENABLED=true on Orbit, restart it.
# 2. From this directory:
docker compose -f docker-compose.monitoring.yml up -d
# 3. Grafana http://localhost:3000 (admin / admin) → Dashboards → "Orbit"
#    Prometheus http://localhost:9090
```

Prometheus scrapes `host.docker.internal:8080` (Orbit on the host). If LB
Admin runs in another compose project, change the target in `prometheus.yml`
to its service (e.g. `api:8080`) and join that network. Change the Grafana
admin password.

## Your own Prometheus and Grafana

- **Dashboard**: import `grafana-dashboard.json` (Dashboards → New → Import),
  choose your Prometheus. Panels: agents reachable, servers up / in maintenance
  / down, requests and error ratio per backend, p95 response time per server,
  servers in rotation per service, HAProxy sessions, a server table, and LB
  Admin's own p95 per route.
- **Alerts**: add `prometheus-alerts.yml` under `rule_files` and scrape LB
  Admin with `job_name: orbit` (the rules use that job label). Rules: LB
  Admin down, an agent unreachable, a server down (not in maintenance), a
  service with no server in rotation, error ratio over 5%, p95 over 1 s, a
  server in maintenance for a day, a certificate expiring within 14 days.

The page has the last hour per server too (Proxies → a backend → **Last
hour**), from Orbit's own 24-hour history (`GET /api/v1/history`).
