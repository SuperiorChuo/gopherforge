# 🚀 GopherForge · Go Microservices Admin Scaffold

**GopherForge** (formerly `go-admin-kit`) is an **open-source, production-grade Go microservices admin scaffold**: Go + Gin backend split into 8 infrastructure services, React 19 + Ant Design 6 frontend, Traefik gateway with unified auth, built-in RBAC, multi-tenancy, audit logs, monitoring and a code generator — the whole stack boots with one `docker compose up`.

- **Who it's for**: Go teams building internal admin platforms or SaaS back-offices; teams that prefer **React over Vue** (most Go admin scaffolds ship Vue); projects that want real microservices as a starting point without business-module baggage.
- **How it differs**: infrastructure only, zero business coupling — see the [comparison with gin-vue-admin, go-admin & RuoYi](docs/comparison.md).
- **Time to running**: clone, `docker compose up -d --build`, ~3 minutes for gateway + 8 services + frontend + PostgreSQL/Redis/NATS. Or try the [Live Demo](https://superiorchuo.github.io/gopherforge/) first (front-end-only mock data, any credentials work).

<p align="center">
  <strong>Production-grade Go microservices admin scaffold — infrastructure only, batteries included.</strong><br/>
  🐹 Go + Gin &nbsp;·&nbsp; ⚛️ React 19 + Ant Design 6 &nbsp;·&nbsp; 🧩 Traefik gateway + 8 services
</p>

<p align="center">
  <a href="https://superiorchuo.github.io/gopherforge/"><strong>🖥️ Live Demo →</strong></a><br/>
  <sub>Front-end-only demo mode (mock data, any credentials work). Full stack: clone &amp; <code>docker compose up</code>.</sub>
</p>

<p align="center">
  <a href="README.md">中文文档</a> · <a href="LICENSE">MIT License</a>
</p>

---

## Why GopherForge

Most admin scaffolds are monoliths. GopherForge (formerly go-admin-kit) gives you a **real microservices architecture** you can grow into, without business-domain baggage:

- **Traefik gateway + ForwardAuth**: one place verifies JWT; downstream services only trust gateway-injected `X-Auth-*` headers.
- **8 infrastructure services**, split by domain: `auth` (login / JWT rotation &amp; revocation / OAuth / TOTP), `identity` (users / roles / permissions / departments), `system` (menus / dicts / notices / hot settings / code generator), `audit` (logs, NATS login events), `file` (MinIO / local), `monitor` (health / metrics / server &amp; DB &amp; Redis dashboards / cron jobs), `bpm` (lightweight approval-flow engine), plus a `shared` library.
- **React 19 + Ant Design 6** front end with dark-space / light dual themes and a glassmorphism look.
- **Code generator**: pick a table, tick the fields, get a CRUD starter kit (Go model / store / handlers / routes + React list page + menu SQL) as preview or zip.
- **Engineering done for you**: goose versioned migrations, OpenAPI 3.1 contracts with CI drift checks, Prometheus metrics, optional OTel + Jaeger tracing, Playwright E2E through the gateway, secret-scanning pre-commit hook.
- **RBAC with data scopes** (all / department &amp; below / self) and optional multi-tenant (`tenant_id`) support.

Adding business capability = add one microservice + one gateway label. The base stays clean.

## Quick start

```bash
git clone https://github.com/SuperiorChuo/gopherforge.git
cd gopherforge/microservices
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
