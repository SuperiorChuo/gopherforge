# Production Deployment

The full production guide is currently maintained in Chinese: [生产部署（中文）](/reference/deployment) · [source on GitHub](https://github.com/SuperiorChuo/gopherforge/blob/main/docs/deployment.md).

Checklist summary:

1. **Rotate secrets**: `JWT_SECRET`, PostgreSQL/Redis/MinIO/Grafana credentials, default admin password, `CORS_ALLOW_ORIGINS`.
2. **Single-server layout**: Docker Compose behind the Traefik gateway; bind host ports to loopback and put TLS termination in front.
3. **Migrations** run automatically via the migrate container (goose, single source of truth under `services/monitor/migrations/`).
4. **Backups & ops scripts** ship under `scripts/ops/` (backup, cleanup, rotation, rollback).
5. **Observability**: enable the Prometheus/Grafana compose profile; health endpoints for liveness probes.
