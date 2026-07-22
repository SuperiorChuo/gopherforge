# Monitoring & Observability

- **Console pages**: server CPU/memory/disk, PostgreSQL connections & slow queries, Redis memory & hit rate, cron job registry.
- **Health**: every service exposes `/api/v1/health/live` and `/ready` (DB ping), wired into compose health checks.
- **Metrics**: Prometheus scraping + Grafana dashboards under `platform/deploy/` (optional profile).
- **Tracing (optional)**: OpenTelemetry SDK pre-wired; set `OTEL_EXPORTER_OTLP_ENDPOINT` to enable, with request_id propagated through logs and responses.
- **Logs**: login logs (durable NATS consumption), operation logs (middleware-level with latency and request_id, CSV export), audit logs — all queryable via the audit service.
