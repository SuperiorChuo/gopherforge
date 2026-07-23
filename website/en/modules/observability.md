# Monitoring & Observability

- **Console pages**: server CPU/memory/disk, PostgreSQL connections & slow queries, Redis memory & hit rate, cron job registry — plus a **service-health overview** card (concurrent `/ready` probes across all services, unhealthy first, 10 s auto-refresh) and a **distributed-job heartbeats** card.
- **Health**: every service exposes `/api/v1/health/live` and `/ready` (DB ping), wired into compose health checks; monitor's `/monitor/services` aggregates the probes for the overview card.
- **Job heartbeats**: in-process crons, standalone workers and host shell scripts all report liveness via `shared/pkg/jobbeat` (one call per run into `ops_job_heartbeats`); anything silent for over 2× its interval is flagged stale in the console. Shell scripts join with a single curl.
- **Metrics**: Prometheus scraping (zero-dependency `shared/pkg/metrics`: HTTP counters/error rates/latency histograms + Go runtime + DB pool) + node_exporter host metrics + Grafana dashboards under `platform/deploy/` (optional profile).
- **Alerting loop (optional)**: Prometheus alert rules (service down / low disk / high memory / 5xx surge, with `for` windows absorbing rolling-update noise) → Alertmanager (grouping, dedup, resolved notifications, inhibition) → notify webhook → in-console notifications. If notify isn't running, failed delivery is just logged.
- **Tracing (optional)**: OpenTelemetry SDK pre-wired; set `OTEL_EXPORTER_OTLP_ENDPOINT` to enable, with request_id propagated through logs and responses.
- **Logs**: login logs (durable NATS consumption), operation logs (middleware-level with latency and request_id, CSV export), audit logs — all queryable via the audit service.
