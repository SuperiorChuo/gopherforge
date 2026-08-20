---
description: A concise production deployment summary for GopherForge covering official images, secrets, Docker Compose, TLS, backups and rollback.
---

# Production Deployment (Summary)

The full production guide is maintained in Chinese: [生产部署（中文）](/reference/deployment) · [source on GitHub](https://github.com/SuperiorChuo/gopherforge/blob/main/docs/deployment.md).

> Current release: `v0.6.0` ([Release](https://github.com/SuperiorChuo/gopherforge/releases/tag/v0.6.0)). It is a 0.x release: APIs and database schemas may change. Complete backups, migration compatibility checks and a rollback rehearsal before production use. See the [upgrade notes](/en/reference/upgrade).

## Quick start with official images (v0.2.0+, recommended)

Every release pushes 8 images (7 Go services + frontend; the migrate job reuses the monitor image) to ghcr.io — both `linux/amd64` and `linux/arm64` from `v0.3.0` (v0.2.0 and earlier are amd64-only). Images are dual-tagged `vX.Y.Z` + `sha-<7chars>`; `latest` moves only on stable releases.

```bash
git clone https://github.com/SuperiorChuo/gopherforge.git /opt/gopherforge
cd /opt/gopherforge/microservices
cp .env.example .env && chmod 600 .env      # set strong JWT_SECRET / DB / Redis passwords first

docker network inspect go-admin-kit-net >/dev/null 2>&1 || \
  docker network create --subnet 172.28.0.0/16 go-admin-kit-net
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml up -d   # data stack

export IMAGE_PREFIX=ghcr.io/superiorchuo/gopherforge/go-admin-kit
export IMAGE_TAG=v0.6.0
docker compose pull && docker compose up -d --no-build                   # app stack
```

Upgrade / rollback later = change `IMAGE_TAG` and re-run `pull` + `up -d --no-build`.

## Checklist summary

1. **Rotate secrets**: `JWT_SECRET` (≥32 chars), PostgreSQL/Redis/MinIO/Grafana credentials, default admin password, `CORS_ALLOW_ORIGINS`. With `APP_ENV=production`, weak or placeholder values are **rejected at startup**.
2. **Single-server layout**: Docker Compose behind the Traefik gateway; keep service ports on loopback (`SERVICES_BIND_IP=127.0.0.1`) and terminate TLS in front (Nginx).
3. **Migrations** run automatically via the migrate container (goose, single source of truth under `services/monitor/migrations/`). Forward-only — back up before destructive upgrades.
4. **Backups & log rotation** are not bundled: configure a daily `pg_dump` cron and Docker json-file log limits on day one.
5. **Observability**: enable the Prometheus/Grafana compose profile; `/api/v1/health/ready` for liveness probes.
