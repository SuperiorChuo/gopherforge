# 🚀 GopherForge · Go Microservices Platform

**GopherForge** (formerly `go-admin-kit`) is an **open-source, enterprise-grade Go microservices admin & business platform**: Go + Gin backend split into **16 Go services** (infrastructure — auth / identity / system / audit / file / monitor / notify / AI / BPM / payments — plus business lines: IM support, call center, CRM, WeChat official accounts, tickets, visibility), React 19 + Ant Design 6 frontend, Traefik gateway with unified auth, built-in RBAC, multi-tenancy, audit logs, monitoring and a code generator — the whole stack boots with one `make compose-up` (data stack and app stack are separate, so rebuilding the app never touches your data).

> 0.x project: APIs, database schemas and generated code formats may still change. Latest stable: see [Releases](https://github.com/SuperiorChuo/gopherforge/releases/latest) / [CHANGELOG](CHANGELOG.md); upgrade notes in the [docs](https://superiorchuo.github.io/gopherforge/docs/en/reference/upgrade). Before production use, follow the deployment guide for secret rotation, migration backups and rollback rehearsal.

- **Who it's for**: Go teams building internal admin platforms or SaaS back-offices; teams that prefer **React over Vue** (most Go admin scaffolds ship Vue); projects that want a real microservices starting point with ready-made business domains to trim.
- **How it differs**: infrastructure and business domains are layered and independently trimmable — see the [comparison with gin-vue-admin, go-admin & RuoYi](docs/comparison.md).
- **Time to running**: clone, `make compose-up`, and the gateway + 16 services + frontend + PostgreSQL/Redis/NATS come up (elapsed time depends on machine resources and image pulls). Or try the [Live Demo](https://superiorchuo.github.io/gopherforge/) first (front-end-only mock data, any credentials work).

<p align="center">
  <strong>Enterprise-grade Go microservices platform — base and business domains in one.</strong><br/>
  🐹 Go + Gin &nbsp;·&nbsp; ⚛️ React 19 + Ant Design 6 &nbsp;·&nbsp; 🧩 Traefik gateway + 16 services
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

Most admin scaffolds are monoliths. GopherForge (formerly go-admin-kit) gives you a **real microservices architecture** you can grow into, with both the platform base and ready business domains:

- **Traefik gateway + ForwardAuth**: one place verifies JWT; downstream services only trust gateway-injected `X-Auth-*` headers.
- **16 Go services**, split by domain: base — `auth` (login / JWT rotation &amp; revocation / OAuth / TOTP / **OAuth2 + OIDC provider**), `identity` (users / roles / permissions / departments), `system` (menus / dicts / notices / hot settings / code generator), `audit` (logs, NATS login events, optional retention-days auto-cleanup), `file` (MinIO / local), `monitor` (health / metrics / dashboards / cron jobs / shared goose migrations), `ai` (model management with AES-GCM encrypted keys, embedded agent worker, usage &amp; quotas), `notify` (in-console messages / templates / channels), `bpm` (lightweight approval-flow engine), `pay` (**mock channel only** — the WeChat/Alipay adapters are not implemented yet), plus business lines — `im` (customer support), `cc` (call center), `crm`, `mp`, `visibility`, `ticket` — and a `shared` library.
- **Alerting loop built in (optional)**: node_exporter host metrics + Prometheus alert rules (service down / low disk / high memory / 5xx surge) + Alertmanager grouping &amp; dedup, delivered as in-console notifications via the notify webhook.
- **React 19 + Ant Design 6** front end with dark-space / light dual themes and a glassmorphism look.
- **Code generator**: pick a table, tick the fields, get a CRUD starter kit (Go model / store / handlers / routes + React list page + menu SQL) as preview or zip.
- **Engineering done for you**: goose versioned migrations, OpenAPI 3.1 contracts with CI drift checks, Prometheus metrics, optional OTel + Jaeger tracing, Playwright E2E through the gateway, secret-scanning pre-commit hook.
- **RBAC with data scopes** (all / department &amp; below / self) and optional multi-tenant (`tenant_id`) support.

Adding business capability = add one microservice + one gateway label; the experimental business lines can be trimmed the same way.

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
export IMAGE_TAG=v0.8.0          # use the latest stable tag
docker compose pull && docker compose up -d --no-build
```

Production hardening (Nginx/HTTPS, secrets, backups, upgrades) → [deployment guide](https://superiorchuo.github.io/gopherforge/docs/en/reference/deployment).

Without make — the equivalent three commands (data stack is separate from the app stack, so app rebuilds never touch data):

```bash
cd microservices
docker network inspect go-admin-kit-net >/dev/null 2>&1 || \
  docker network create --subnet 172.28.0.0/16 go-admin-kit-net
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml up -d
docker compose --profile compose-fallback up -d --build
```

## Stack

| Layer | Tech |
|---|---|
| Backend | Go 1.27 · Gin · GORM · PostgreSQL 18 · Redis 8 · goose |
| Gateway / Bus | Traefik (ForwardAuth) · NATS JetStream |
| Frontend | React 19 · TypeScript · Vite · Ant Design 6 · Redux Toolkit |
| Observability | Prometheus + node_exporter · Grafana · Alertmanager alert loop · OpenTelemetry + Jaeger (optional) |
| Storage | MinIO (S3-compatible) or local |
| CI | GitHub Actions: per-service test+vet, lint+build, OpenAPI drift, migration rehearsal, compose smoke + Playwright E2E |

## Scope

This repository is the **public product line**: the complete platform (infrastructure base + business domains), synced per [docs/sync-policy.md](docs/sync-policy.md) — internal materials and private history stay out.

Known boundaries shipping with this release:

- **Payments**: mock channel only (isolated testing / demo). The WeChat/Alipay adapters are **not implemented**; configuring merchant credentials does not enable real payments.
- **Experimental business lines** (IM, call center, CRM, official accounts, tickets, visibility): APIs and schemas may still change; the call center additionally needs `CC_FS_API_BASE` and a FreeSWITCH media deployment for full acceptance.

Issues and PRs are welcome for anything in scope: product bugs, engineering, docs.

## Contributing

- [CONTRIBUTING.md](CONTRIBUTING.md) — dev setup, verification commands and the commit convention (**commit subject and body must both be written in Chinese**).
- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) — Contributor Covenant v2.1 (Chinese text).
- [SECURITY.md](SECURITY.md) — how to report a vulnerability, production hardening requirements, and known gaps — notably **there is no CSRF protection**: the front end is a SPA carrying pure `Authorization: Bearer` tokens, so if you switch to cookie-borne tokens you must add CSRF defenses yourself.

## License

[MIT](LICENSE)
