# Multi-tenancy & Packages

A SaaS foundation built in: **shared database + `tenant_id` row isolation**, tenant-code login, packages as per-tenant permission caps.

## Isolation model

- Every tenant-scoped table carries `tenant_id` (`not null; default:1; index`, part of business unique indexes such as "username unique per tenant").
- **Two layers of defence**: hand-written DAO scoping is layer one; the **tenant isolation GORM plugin** is layer two. Any model with a `tenant_id` column (schema-detected, no allowlist) gets: automatic `WHERE tenant_id = …` on queries, tenant fill on create, and a tenant constraint appended to updates/deletes — cross-tenant writes by guessed id hit nothing. GORM's `ErrMissingWhereClause` guard is preserved.
- Platform tables (`tenants`, `tenant_packages`, `permissions`, `menus` — no `tenant_id`) are naturally exempt; explicit cross-tenant work uses the `tenant.DisableScope(ctx)` escape hatch.

## Login & tenant context

`tenant_code` is optional at login — empty falls back to the subdomain (`acme.example.com` → `acme`), then to the `default` tenant. The tenant id travels in the JWT; after Traefik ForwardAuth the gateway injects `X-Auth-Tenant-ID` (plus user/platform-admin headers) which services read into the request context. Service ports bind to loopback, so those headers can only come from the gateway.

## Packages = permission bundles

A package caps the permission set a tenant may grant. `POST /roles/:id/permissions` verifies the assignment ⊆ package (violations listed in the error). Exempt: platform admins, tenants without a package. **Shrinking a package does not revoke** already-granted permissions — it only blocks new grants.

Platform admins can act as a tenant via the `X-Act-Tenant-ID` header (console "act as tenant" entry).

## Endpoint quick reference

| Method | Path | Permission |
|--------|------|------------|
| GET/POST | `/api/v1/tenants` | `system:tenant:list` / `create` |
| GET/PUT | `/api/v1/tenants/:id` | `system:tenant:detail` / `update` |
| GET/POST | `/api/v1/tenant-packages` (+`/all`) | `system:tenant-package:*` |
| PUT/DELETE | `/api/v1/tenant-packages/:id` | delete rejected while tenants are bound |

## Known limits

The plugin does not cover raw SQL (`Raw`/`Exec`) — in-repo DAOs are pure ORM; add tenant predicates yourself if you write raw SQL. The monolith product line is single-tenant.
