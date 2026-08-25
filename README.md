# Platform Observability

A minimal, self-contained observability project that demonstrates **collecting,
visualizing, and investigating application telemetry** with a modern Grafana
stack **Prometheus, Grafana, Grafana Alloy, and Loki** — around a small Go API.

The label and metric conventions (`service`, `level`, `http_requests_total`,
`http_request_duration_seconds`, recording/alert rules) mirror the Kubernetes
[`platform-monitoring`](./platform-monitoring) reference stack, scaled down to a
single-node Docker Compose setup you can run on a laptop.

## Architecture

```
Go API (demoapi)
 ├── Metrics  ──/metrics──→  Prometheus  ──→  Grafana
 │
 └── Logs (JSON stdout) ──→  Alloy  ──→  Loki  ──→  Grafana
```

- **Prometheus** scrapes the Go API's `/metrics` endpoint directly.
- **Alloy** discovers Docker containers, tails their stdout, extracts the JSON
  `level` field into a Loki label, and ships logs to **Loki**.
- **Grafana** ships pre-provisioned datasources and a dashboard combining
  metrics and logs.

## Stack

| Component | Role | Port | Image |
|-----------|------|------|-------|
| `app` (Go) | Demo API emitting metrics + JSON logs | 8080 | built locally |
| Prometheus | Metrics collection & alerting rules | 9090 | `prom/prometheus:v2.54.1` |
| Grafana Alloy | Log/telemetry collection agent | 12345 | `grafana/alloy:v1.3.1` |
| Loki | Centralized log storage | 3100 | `grafana/loki:3.1.1` |
| Grafana | Dashboards & visualization | 3000 | `grafana/grafana:11.1.4` |

## Layout

```
app/                              Go demo API
  main.go                         HTTP server, Prometheus metrics, JSON logs
  Dockerfile                      multi-stage → distroless image
  go.mod / go.sum
observability/
  prometheus/prometheus.yml       scrape config
  prometheus/alert-rules.yml      recording + alert rules
  alloy/config.alloy              Docker log discovery → Loki
  loki/loki-config.yml            single-binary Loki config
  grafana/dashboard.json          the demo dashboard
  grafana/provisioning/           auto-provisioned datasources + dashboards
docker-compose.yml
```

## Run it

Requires Docker + Docker Compose.

```bash
docker compose up -d --build
```

Then open:

| URL | What |
|-----|------|
| http://localhost:3000 | Grafana → dashboard **"Demo API — Observability"** (login `admin` / `admin`, anonymous viewing enabled) |
| http://localhost:9090 | Prometheus (try `service:http_error_percent`, check **Status → Targets** and **Alerts**) |
| http://localhost:8080 | The demo API (`/`, `/work`, `/error`, `/healthz`, `/metrics`) |
| http://localhost:12345 | Alloy UI (component graph) |

The API self-generates traffic (`GENERATE_LOAD=true`) so dashboards and logs
fill in within ~30s. Set it to `false` in `docker-compose.yml` to drive traffic
yourself.

Tear down (add `-v` to also drop stored metrics/logs):

```bash
docker compose down
```

## Investigating telemetry (the point of the demo)

1. **Metrics** - the dashboard's golden-signal row shows request rate, 5xx error
   rate, P95 latency, and in-flight requests. `/error` fails ~35% of the time, so
   the error-rate stat turns red and the **HighErrorRate** alert fires in
   Prometheus after 2m.
2. **Logs** - the bottom row queries Loki. Filter to failures with
   `{service="demoapi", level="error"} | json` and correlate a latency spike in
   the metrics panels with the corresponding error log lines by time.

## Endpoints

| Method | Path | Behavior |
|--------|------|----------|
| GET | `/` | JSON hello |
| GET | `/healthz` | health check |
| GET | `/work` | variable latency (occasional slow tail) |
| GET | `/error` | ~35% return HTTP 500 |
| GET | `/metrics` | Prometheus exposition |
