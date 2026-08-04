# 🚀 GopherForge · Go Microservices Admin Scaffold

**GopherForge** (formerly `go-admin-kit`) is an **open-source, production-grade Go microservices admin scaffold**: Go + Gin backend split into 7 infrastructure services, React 19 + Ant Design 6 frontend, Traefik gateway with unified auth, built-in RBAC, multi-tenancy, audit logs, monitoring and a code generator — the whole stack boots with one `make compose-up` (data stack and app stack are separate, so rebuilding the app never touches your data).

> 0.x project: APIs, database schemas and generated code formats may still change. Latest stable: see [Releases](https://github.com/SuperiorChuo/gopherforge/releases/latest) / [CHANGELOG](CHANGELOG.md); upgrade notes in the [docs](https://superiorchuo.github.io/gopherforge/docs/en/reference/upgrade). Before production use, follow the deployment guide for secret rotation, migration backups and rollback rehearsal.

- **Who it's for**: Go teams building internal admin platforms or SaaS back-offices; teams that prefer **React over Vue** (most Go admin scaffolds ship Vue); projects that want real microservices as a starting point without business-module baggage.
- **How it differs**: infrastructure only, zero business coupling — see the [comparison with gin-vue-admin, go-admin & RuoYi](docs/comparison.md).
- **Time to running**: clone, `make compose-up`, ~3 minutes for gateway + 7 services + frontend + PostgreSQL/Redis/NATS. Or try the [Live Demo](https://superiorchuo.github.io/gopherforge/) first (front-end-only mock data, any credentials work).

<p align="center">
  <strong>Production-grade Go microservices admin scaffold — infrastructure only, batteries included.</strong><br/>
  🐹 Go + Gin &nbsp;·&nbsp; ⚛️ React 19 + Ant Design 6 &nbsp;·&nbsp; 🧩 Traefik gateway + 7 services
</p>

<p align="center">
  <a href="https://superiorchuo.github.io/gopherforge/"><strong>🖥️ Live Demo →</strong></a> · <a href="https://superiorchuo.github.io/gopherforge/docs/"><strong>📖 Documentation</strong></a><br/>
  <sub>Front-end-only demo mode (mock data, any credentials work). Full stack: clone &amp; <code>make compose-up</code>, or pull the official multi-arch images from ghcr. See <a href="CHANGELOG.md">CHANGELOG</a> for release notes.</sub>
</p>

<p align="center">
  <a href="README.md">中文文档</a> · <a href="LICENSE">MIT License</a>
</p>

---

## Why GopherForge

Most admin scaffolds are monoliths. GopherForge (formerly go-admin-kit) gives you a **real microservices architecture** you can grow into, without business-domain baggage:

- **Traefik gateway + ForwardAuth**: one place verifies JWT; downstream services only trust gateway-injected `X-Auth-*` headers.
- **7 infrastructure services**, split by domain: `auth` (login / JWT rotation &amp; revocation / OAuth / TOTP / **OAuth2 + OIDC provider** — authorization_code + PKCE, client_credentials, RS256 `id_token` with JWKS &amp; discovery endpoints, console-managed clients and tokens), `identity` (users / roles / permissions / departments), `system` (menus / dicts / notices / hot settings / code generator), `audit` (logs, NATS login events, optional retention-days auto-cleanup), `file` (MinIO / local), `monitor` (health / metrics / server &amp; DB &amp; Redis dashboards / cron jobs / one-glance service-health probe / distributed-job heartbeats), `bpm` (lightweight approval-flow engine: DingTalk-style designer, no-code flow forms, AND/OR/sequential approval, conditional branches, timeout auto-actions, add-sign &amp; delegation, analytics), plus a `shared` library.
- **Alerting loop built in (optional)**: node_exporter host metrics + Prometheus alert rules (service down / low disk / high memory / 5xx surge) + Alertmanager grouping &amp; dedup, delivered as in-console notifications via the notify webhook.
- **React 19 + Ant Design 6** front end with dark-space / light dual themes and a glassmorphism look.
- **Code generator**: pick a table, tick the fields, get a CRUD starter kit (Go model / store / handlers / routes + React list page + menu SQL) as preview or zip.
- **Engineering done for you**: goose versioned migrations, OpenAPI 3.1 contracts with CI drift checks, Prometheus metrics, optional OTel + Jaeger tracing, Playwright E2E through the gateway, secret-scanning pre-commit hook.
- **RBAC with data scopes** (all / department &amp; below / self) and optional multi-tenant (`tenant_id`) support.

Adding business capability = add one microservice + one gateway label. The base stays clean.

## Quick start

```bash
git clone https://github.com/SuperiorChuo/gopherforge.git
cd gopherforge/microservices && cp .env.example .env && cd ..
make compose-up      # shared network → infra (data) stack → app stack
# open http://localhost:8000  (admin / admin123)
```

Prefer not to build locally? Pull the **official multi-arch images** (amd64 / arm64, pushed to ghcr on every stable release):

```bash
cd microservices
export IMAGE_PREFIX=ghcr.io/superiorchuo/gopherforge/go-admin-kit
export IMAGE_TAG=v0.4.0          # use the latest stable tag
docker compose pull && docker compose up -d --no-build
```

Production hardening (Nginx/HTTPS, secrets, backups, upgrades) → [deployment guide](https://superiorchuo.github.io/gopherforge/docs/en/reference/deployment).

Without make — the equivalent three commands (data stack is separate from the app stack, so app rebuilds never touch data):

```bash
cd microservices
docker network inspect go-admin-kit-net >/dev/null 2>&1 || \
  docker network create --subnet 172.28.0.0/16 go-admin-kit-net
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml up -d
docker compose up -d --build
```

## Stack

| Layer | Tech |
|---|---|
| Backend | Go 1.26 · Gin · GORM · PostgreSQL 16 · Redis 7 · goose |
| Gateway / Bus | Traefik (ForwardAuth) · NATS JetStream |
| Frontend | React 19 · TypeScript · Vite · Ant Design 6 · Redux Toolkit |
| Observability | Prometheus + node_exporter · Grafana · Alertmanager alert loop · OpenTelemetry + Jaeger (optional) |
| Storage | MinIO (S3-compatible) or local |
| CI | GitHub Actions: per-service test+vet, lint+build, OpenAPI drift, migration rehearsal, compose smoke + Playwright E2E |

## Scope

This repository is the **scaffold distribution line**: platform-neutral infrastructure only, synced from an internally maintained full-featured upstream. Business domains (IM, call center, CRM, …) never land here — see [docs/sync-policy.md](docs/sync-policy.md).

Issues and PRs are welcome for anything scaffold-related: base bugs, engineering, docs.

## Contributing

- [CONTRIBUTING.md](CONTRIBUTING.md) — dev setup, verification commands and the commit convention (**commit subject and body must both be written in Chinese**).
- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) — Contributor Covenant v2.1 (Chinese text).
- [SECURITY.md](SECURITY.md) — how to report a vulnerability, production hardening requirements, and known gaps — notably **there is no CSRF protection**: the front end is a SPA carrying pure `Authorization: Bearer` tokens, so if you switch to cookie-borne tokens you must add CSRF defenses yourself.

## License

[MIT](LICENSE)
