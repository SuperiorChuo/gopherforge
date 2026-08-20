---
aside: false
description: Overview of GopherForge's 7 Go services, Traefik ForwardAuth, data layer, frontend and observability paths.
---

<script setup lang="ts">
import { withBase } from 'vitepress'

const architectureDemoUrl = withBase('/architecture/gopherforge-system-architecture.html')
</script>

# Architecture

GopherForge is a **real microservices architecture**: 7 Go services split by domain plus a shared library, an SPA frontend, a Traefik gateway as the single entry with unified auth, all orchestrated by Docker Compose.

## Interactive system architecture

<div class="architecture-demo">
  <iframe
    class="architecture-demo__frame"
    :src="architectureDemoUrl"
    title="GopherForge interactive system architecture"
    loading="lazy"
    sandbox="allow-scripts allow-downloads allow-popups"
    allowfullscreen
  />
  <div class="architecture-demo__actions">
    <a :href="architectureDemoUrl" target="_blank" rel="noopener noreferrer">Open full screen</a>
  </div>
</div>

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

- **PostgreSQL 18** (pgvector image), one shared database, tables prefixed per service.
- **Single migration source of truth**: versioned goose SQL under `services/monitor/migrations/`, executed by the migrate container; experimental services (e.g. bpm) self-manage tables via GORM AutoMigrate.
- **Redis 8**: rate limiting, online users, token blacklist, permission cache.
- **NATS JetStream**: decouples login events from auth to audit (durable consumption).

## Frontend

React 19 + TypeScript + Vite 8 + Ant Design 6, Redux Toolkit, Axios interceptors unwrapping the `{code, message, data}` envelope with transparent token refresh. Dual dark/light themes.

Dedicated **Frontend** documentation (stack, directory layout, request layer, routing & permissions, page conventions, state & theme, demo mode) lives under the [Frontend](/en/frontend/overview) section of the nav. **Read [Page Development](/en/frontend/page-dev) before writing a new page** — the shared list-page trio and component conventions will save you rework.

## CI gates

Per-service `go test` + `go vet`, frontend lint/build/audit, plus three distinctive gates: **OpenAPI contract drift detection**, **migration rehearsal** on a clean database, and a full-stack **smoke + Playwright E2E** job.

<style scoped>
.architecture-demo {
  margin: 24px 0 40px;
  overflow: hidden;
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background: var(--vp-c-bg-soft);
}

.architecture-demo__frame {
  display: block;
  width: 100%;
  height: min(74vh, 760px);
  min-height: 620px;
  border: 0;
  color-scheme: light dark;
}

.architecture-demo__actions {
  display: flex;
  justify-content: flex-end;
  padding: 10px 14px;
  border-top: 1px solid var(--vp-c-divider);
}

.architecture-demo__actions a {
  color: var(--vp-c-brand-1);
  font-size: 14px;
  font-weight: 600;
  text-decoration: none;
}

.architecture-demo__actions a:hover {
  color: var(--vp-c-brand-2);
}

@media (max-width: 768px) {
  .architecture-demo {
    margin-right: -16px;
    margin-left: -16px;
    border-right: 0;
    border-left: 0;
    border-radius: 0;
  }

  .architecture-demo__frame {
    height: 580px;
    min-height: 580px;
  }
}
</style>
