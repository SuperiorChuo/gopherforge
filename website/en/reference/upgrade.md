# Upgrading

SemVer applies; during **0.x** APIs and schemas may change — breaking changes are called out in the [changelog](/en/changelog) and here. Migrations (goose) are forward-only: **back up before upgrading**.

## General procedure (image-based, v0.2.0+)

```bash
cd /opt/gopherforge/microservices
export IMAGE_PREFIX=ghcr.io/superiorchuo/gopherforge/go-admin-kit
export IMAGE_TAG=v0.3.0            # target version
docker compose pull && docker compose up -d --no-build
```

Rollback = switch `IMAGE_TAG` back (only safe while new migrations are backward-compatible). Source builds: `git pull` to the tag, then `make compose-up`.

## 0.2.0 → 0.3.0 notes

No breaking changes; in order of impact:

1. **Security dependency upgrades** (the main reason to move): grpc 1.82.1 (GHSA-hrxh-6v49-42gf) and x/crypto 0.54 / x/net 0.56 in bpm (the 2026-07-25 ssh/agent/knownhosts + html/idna HIGH CVEs). v0.2.0 images still carry the vulnerable versions.
2. **Stricter bpm weak-credential detection** (the only observable behavior change): in production, `dev-`-prefixed tokens (e.g. a `dev-xxx` `BPM_INTERNAL_TOKEN`) are now treated as placeholders and fail closed — internal endpoints return 503. Switch to a strong random token before upgrading if you actually use that shape.
3. **New audit log retention policy** (opt-in, off by default): set `AUDIT_LOG_RETENTION_DAYS=N` to enable; unset = identical behavior to 0.2.0. See [Audit Logs](/en/modules/audit).
4. **Official images are multi-arch from this release** (`linux/amd64` + `linux/arm64`) — no more local builds on arm64.

## 0.1.0 → 0.2.0 notes

1. **Weak production credentials now refuse to start** (most likely to bite): `APP_ENV=production` validates `JWT_SECRET` (≥32 chars), DB/Redis passwords, object-storage keys (s3/minio mode) and, when enabled, OAuth/SMTP secrets. Fix `.env` before upgrading — the error names the offending variable.
2. **Compose split into two stacks**: stateful services moved to `docker-compose.infra.yml` (project `go-admin-kit-infra`), joined via the external `go-admin-kit-net` network (`make compose-up` creates it). Volume names unchanged — zero data migration.
3. **New bpm-service** (approval workflow): new service, tables (migrations auto-run) and gateway routes. Skip `BPM_INTERNAL_TOKEN` if unused — internal endpoints just return 503.
4. **Versioned image names**: `${IMAGE_PREFIX:-go-admin-kit}-<service>:${IMAGE_TAG:-latest}` — identical behaviour when unset; enables ghcr deployment and tag-based rollback.
5. **Go toolchain 1.26.5** for source builds (`toolchain` directive; the `go` language version is unchanged, so downstream modules aren't forced up).
6. New opt-in features: OAuth2/OIDC server (PKCE, introspection, JWKS), ops panel (job heartbeats / service health / alerting), add-sign & delegation in BPM, service `/metrics`, department leaders, a Kubernetes guide.

## Release candidates

`-rc.N` prereleases are preview-only: changelog sections optional, `latest` image untouched. Production tracks stable tags only.
