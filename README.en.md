# 🚀 Go Admin Kit · Microservices Admin Scaffold

<p align="center">
  <strong>Production-grade Go microservices admin scaffold — infrastructure only, batteries included.</strong><br/>
  🐹 Go + Gin &nbsp;·&nbsp; ⚛️ React 19 + Ant Design 6 &nbsp;·&nbsp; 🧩 Traefik gateway + 7 services
</p>

<p align="center">
  <a href="https://superiorchuo.github.io/go-admin-kit/"><strong>🖥️ Live Demo →</strong></a><br/>
  <sub>Front-end-only demo mode (mock data, any credentials work). Full stack: clone &amp; <code>docker compose up</code>.</sub>
</p>

<p align="center">
  <a href="README.md">中文文档</a> · <a href="LICENSE">MIT License</a>
</p>

---

## Why Go Admin Kit

Most admin scaffolds are monoliths. Go Admin Kit gives you a **real microservices architecture** you can grow into, without business-domain baggage:

- **Traefik gateway + ForwardAuth**: one place verifies JWT; downstream services only trust gateway-injected `X-Auth-*` headers.
- **7 infrastructure services**, split by domain: `auth` (login / JWT rotation &amp; revocation / OAuth / TOTP), `identity` (users / roles / permissions / departments), `system` (menus / dicts / notices / hot settings / code generator), `audit` (logs, NATS login events), `file` (MinIO / local), `monitor` (health / metrics / server &amp; DB &amp; Redis dashboards / cron jobs), plus a `shared` library.
- **React 19 + Ant Design 6** front end with dark-space / light dual themes and a glassmorphism look.
- **Code generator**: pick a table, tick the fields, get a CRUD starter kit (Go model / store / handlers / routes + React list page + menu SQL) as preview or zip.
- **Engineering done for you**: goose versioned migrations, OpenAPI 3.1 contracts with CI drift checks, Prometheus metrics, optional OTel + Jaeger tracing, Playwright E2E through the gateway, secret-scanning pre-commit hook.
- **RBAC with data scopes** (all / department &amp; below / self) and optional multi-tenant (`tenant_id`) support.

Adding business capability = add one microservice + one gateway label. The base stays clean.

## Quick start

```bash
git clone https://github.com/SuperiorChuo/go-admin-kit.git
cd go-admin-kit/microservices
cp .env.example .env
docker compose up -d --build
# open http://localhost:8000  (admin / admin123)
```

## Stack

| Layer | Tech |
|---|---|
| Backend | Go 1.26 · Gin · GORM · PostgreSQL 16 · Redis 7 · goose |
| Gateway / Bus | Traefik (ForwardAuth) · NATS JetStream |
| Frontend | React 19 · TypeScript · Vite · Ant Design 6 · Redux Toolkit |
| Observability | Prometheus · Grafana · OpenTelemetry + Jaeger (optional) |
| Storage | MinIO (S3-compatible) or local |
| CI | GitHub Actions: per-service test+vet, lint+build, OpenAPI drift, migration rehearsal, compose smoke + Playwright E2E |

## Scope

This repository is the **scaffold distribution line**: platform-neutral infrastructure only, synced from an internally maintained full-featured upstream. Business domains (IM, call center, CRM, …) never land here — see [docs/sync-policy.md](docs/sync-policy.md).

Issues and PRs are welcome for anything scaffold-related: base bugs, engineering, docs.

## License

[MIT](LICENSE)
