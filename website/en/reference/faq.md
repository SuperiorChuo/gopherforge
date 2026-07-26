# FAQ

### Where is my database container after `make compose-up`?

The stack is split in two: stateful services (PostgreSQL/Redis/NATS/MinIO) live in a separate infra stack under the fixed project name `go-admin-kit-infra`. `docker compose down` only stops the app stack — data is never touched. Inspect or reset the data stack explicitly:

```bash
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml ps
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml down -v   # ⚠️ wipes volumes
```

Always pass `-p go-admin-kit-infra` when operating the infra stack.

### Port conflicts?

Change `FRONTEND_PORT` / `GATEWAY_PORT` / `POSTGRES_PORT` / `REDIS_PORT` in `microservices/.env` (defaults 3000/8000/5432/6379).

### Apple Silicon?

Local builds fully support arm64 — set `DOCKER_SOCK=/var/run/docker.sock.raw` in `.env` for Docker Desktop. Official ghcr images ship **`linux/arm64` from `v0.3.0`** (v0.2.0 and earlier are amd64-only; build those locally).

### The migrate container exited — is that normal?

Yes. It's a one-shot job: wait for PG (up to 30 retries) → run goose migrations → exit 0. Services wait for it via `service_completed_successfully`. A non-zero exit is the actual failure signal.

### Is NATS optional?

No — login audit logging depends on it (auth publishes login events; audit consumes them durably).

### Production startup is rejected with a config error?

That's the weak-credential guard: with `APP_ENV=production`, `JWT_SECRET` (≥32 chars, non-placeholder), DB and Redis passwords are always validated; object-storage keys only when `UPLOAD_STORAGE_TYPE=s3/minio`; OAuth client secrets and SMTP passwords only when those features are actually enabled. The error names the offending variable — set a strong value.

### No geo location in login logs?

Download the offline IP database: `scripts/download-ip2region.sh`, then restart `system-service` and `audit-service`. Without it the service degrades to an online lookup.

### CSRF protection?

**Not implemented** — a documented boundary. The SPA sends `Authorization: Bearer` headers, which narrows the CSRF surface; if you switch tokens into cookies (especially `SameSite=None`), add CSRF tokens or Origin checks yourself.

### BPM internal endpoints return 503?

`BPM_INTERNAL_TOKEN` is unset or still a placeholder — fail-closed by design. Configure a strong random token on both sides.

### Where is `/metrics`?

Reachable only inside the container network (the gateway does not route it). Enable the monitoring profile for Prometheus/Grafana; `METRICS_ENABLED=false` turns it off.

More: [Deployment](/en/reference/deployment) · [Upgrading](/en/reference/upgrade) · [GitHub Issues](https://github.com/SuperiorChuo/gopherforge/issues)
