# Extending: Add a Business Service

The extension principle: **keep the scaffold generic; plug business in as a new microservice + gateway labels**. This walks through adding a `demo` service.

## Step 0: Try the code generator first

For standard CRUD, don't hand-write anything — "System → Code Generator" in the console generates backend + frontend from a table (single/tree/master-detail modes, see [Code Generator](/en/modules/codegen)).

## Step 1: Create the service

```bash
cd microservices/services
mkdir demo && cd demo
go mod init github.com/go-admin-kit/services/demo
```

Mirror the `bpm` service layout (`cmd/main.go`, `internal/{api,model,store,config}`, `Dockerfile`), and add the module to the root `go.work`.

## Step 2: Wire the gateway

Add a compose block with Traefik labels:

```yaml
demo-service:
  build: { context: ./services/demo, dockerfile: Dockerfile }
  labels:
    traefik.enable: "true"
    traefik.http.routers.demo.rule: "Path(`/api/v1/demo`) || PathPrefix(`/api/v1/demo/`)"
    traefik.http.routers.demo.middlewares: "auth-verify@docker"
    traefik.http.services.demo.loadbalancer.server.port: "8097"
```

::: warning The most common pitfall: 404 through the gateway
Traefik routes by an **explicit path list**. If your service adds a new top-level path but the router rule isn't updated, requests fall through to the monitor fallback and return 404. Always update the compose labels together with route changes.
:::

## Step 3: Auth & contract

- Handlers trust only the gateway-injected `X-Auth-*` headers.
- Uniform `{code, message, data}` response envelope; pagination via `page`/`page_size` returning `{list, total}`.
- Permission codes follow `{domain}:{resource}:{action}`; seed permission rows via migration.
- Internal endpoints use `X-Internal-Token` (503 when unset).

## Step 4: Data & migrations

Models carry `tenant_id` (`not null;default:1;index`); money is stored in **cents** (int64). Core schema changes go to `services/monitor/migrations/`; menu entries are seeded in the system service's `menu_seed.go`.

## Step 5: Frontend

API wrapper in `web/src/api/demo.ts`, pages under `web/src/pages/demo/`, a route entry in `router/index.tsx`, and a menu seed row. Button-level control via `usePermission().hasPerm(code)`.

## Step 6: Verify

```bash
(cd services/demo && go test ./... && go vet ./...)
(cd web && npm run lint && npm run build)
docker compose up -d --build demo-service
```

Commit convention: **Chinese-only titles/bodies**, Conventional style — see CONTRIBUTING.md.

## Need approvals?

Call the bpm internal start endpoint and register a terminal-state callback — no workflow code required. See [Workflow · business-form integration](/en/modules/bpm#business-form-mode-integration).
