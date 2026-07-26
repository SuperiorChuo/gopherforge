# API Reference

All GopherForge APIs go through the **Traefik gateway** with uniform conventions. This page is the hub: general rules + per-module endpoint tables + the machine-readable contract.

## General conventions

| Convention | Detail |
|------------|--------|
| Entry point | Always via the gateway: `http://<gateway>/api/v1/...` (local default `localhost:8000`) — never call service ports directly |
| Auth | `Authorization: Bearer <access_token>`; login via `POST /api/v1/login` (username/password/captcha, optional tenant code), refresh-token rotation |
| Authorization | Gateway ForwardAuth injects `X-Auth-*` headers; per-endpoint permission codes enforced by service middleware (bpm validates Bearer JWTs itself) |
| Response envelope | Uniform `{ "code": 200, "message": "success", "data": ... }`; list payloads are `{ list, total }` |
| Pagination | `page` / `page_size` query params (default 1 / 10) |

## Per-module endpoint tables

Module docs maintain **endpoint tables with permission codes**, extracted from route registrations: [RBAC](/en/modules/rbac) · [Multi-tenancy](/en/modules/tenant) · [Workflow BPM](/en/modules/bpm) · [Code Generator](/en/modules/codegen) · [Audit Logs](/en/modules/audit) · [Auth & Security](/en/modules/auth).

## Machine-readable contract (OpenAPI 3.1)

- **Browse online**: [interactive contract viewer](https://superiorchuo.github.io/gopherforge/docs/api-reference.html) — reads the spec straight from the repo's main branch, always in sync with code.
- Source: [`microservices/services/monitor/docs/openapi.json`](https://github.com/SuperiorChuo/gopherforge/blob/main/microservices/services/monitor/docs/openapi.json), generated from code (`make api-contract`).
- **CI drift gate**: CI fails when contract and code diverge — the contract is enforced, not decorative.

::: tip Coverage (honestly)
The machine-readable contract currently covers the **monitor service + common endpoints** (first service on the contract toolchain); for other services, the per-module endpoint tables above are authoritative. Contract coverage expands service by service.
:::
