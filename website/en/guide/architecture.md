# Architecture

GopherForge is a **real microservices architecture**: 7 Go services split by domain plus a shared library, an SPA frontend, a Traefik gateway as the single entry with unified auth, all orchestrated by Docker Compose.

## Services

| Service | Responsibility |
|------|------|
| **auth** | Login, JWT access/refresh issuing & revocation, captcha, TOTP, OAuth, gateway ForwardAuth verification |
| **identity** | Users, roles, permissions, departments, positions, tenants & packages, data scopes, tenant isolation GORM plugin |
| **system** | Menus, dictionaries, notices, hot-reloadable settings, online users, SMS, error codes, code generator |
| **audit** | Login/operation/audit logs; consumes login events from NATS durably |
| **file** | Upload/download with local / MinIO / any S3-compatible storage |
| **monitor** | Server/PostgreSQL/Redis monitoring, cron jobs, health checks, Prometheus metrics; owns shared goose migrations and the gateway fallback route |
| **bpm** | Lightweight approval workflow engine — see [Workflow](/en/modules/bpm) |
| **shared** | Cross-service Go module: logging, response envelope, masking, error codes, Excel, IP geolocation |

## Request path

```
Browser
  └─▶ Traefik gateway :8000
        ├─ ForwardAuth ─▶ auth (verifies, injects X-Auth-* headers)
        └─ routes by PathPrefix ─▶ services
                                     └─ trust only gateway-injected
                                        X-Auth-User-ID / X-Auth-Tenant-ID
```

Security conventions: host ports bind loopback only, all external traffic goes through the gateway; services never parse JWTs themselves; internal service-to-service calls use a shared `X-Internal-Token`.

## Data layer

- **PostgreSQL 16** (pgvector image), one shared database, tables prefixed per service.
- **Single migration source of truth**: versioned goose SQL under `services/monitor/migrations/`, executed by the migrate container; experimental services (e.g. bpm) self-manage tables via GORM AutoMigrate.
- **Redis 7**: rate limiting, online users, token blacklist, permission cache.
- **NATS JetStream**: decouples login events from auth to audit (durable consumption).

## Frontend

React 19 + TypeScript + Vite 8 + Ant Design 6, Redux Toolkit, Axios interceptors unwrapping the `{code, message, data}` envelope with transparent token refresh. Dual dark/light themes.

## CI gates

Per-service `go test` + `go vet`, frontend lint/build/audit, plus three distinctive gates: **OpenAPI contract drift detection**, **migration rehearsal** on a clean database, and a full-stack **smoke + Playwright E2E** job.
