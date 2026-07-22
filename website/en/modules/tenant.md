# Multi-tenancy & Packages

A SaaS foundation built in: **shared database + tenant_id row isolation**, tenant-code login.

## Isolation model

- Every tenant-scoped table carries `tenant_id` (`not null;default:1;index`).
- The gateway injects `X-Auth-Tenant-ID`; services put it on the request context.
- **Two layers of defence**: hand-written DAO scoping is layer one; the **tenant isolation GORM plugin** is layer two — any model with a `tenant_id` column gets automatic query filtering, create-time fill and tenant-constrained update/delete. A missed manual scope no longer leaks data.
- Platform-level tables (tenants, tenant_packages — no `tenant_id` column) are naturally exempt; explicit cross-tenant operations use the `DisableScope` escape hatch.

## Packages = permission bundles

A package caps the permission set a tenant may grant. Assigning out-of-package permissions to a role is rejected. Platform admins can act on behalf of a tenant from the console.

## Known limits

The plugin does not cover raw SQL (`Raw`/`Exec`); the monolith product line is single-tenant.
