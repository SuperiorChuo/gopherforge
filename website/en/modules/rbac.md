# RBAC

Full **user–role–permission** RBAC provided by the identity + system services, across three granularities: API, button and data.

| Object | Notes |
|------|------|
| Users | Department, multiple positions, enable/disable, Excel import/export |
| Roles | Permission set + data scope; `super_admin` bypasses checks |
| Permissions | Code convention `{domain}:{resource}:{action}` |
| Menus | Seeded tree filtered by role; drives the sidebar |
| Departments | Tree with a leader (used by the workflow "department leader" rule) |
| Positions | Many-to-many with users |

## Three granularities

1. **API level** — route middleware checks the user's permission set, cached in a Redis Set (`user:permissions:<userID>`, 1 h TTL, DB fallback on miss). `super_admin` and the `*` / `*:*:*` wildcard codes bypass. Assigning role permissions invalidates the cache immediately.
2. **Button level** — the `usePermission()` hook:

```tsx
const { hasPerm } = usePermission()
{hasPerm('system:user:create') && <Button onClick={openCreate}>New user</Button>}
```

3. **Data level** — the role's `data_scope` is applied by a GORM plugin before list queries (users, files, login/operation logs): `all` · `department` · `department_tree` · `self` (default) · `custom` (explicit department list) · `none`.

## Walkthrough: onboard a teammate

Create a department (set its leader if you use workflow's dept-leader rule) → create a role (tick permission points, pick a data scope) → create the user (department, position, role) → log in as them to verify menus, buttons and list data all narrow accordingly. Bulk onboarding: Excel import (`GET /users/import-template` → `POST /users/import`, ≤5 MB, per-row error reporting; export via `GET /users/export` respects data scope).

## Endpoint quick reference

Six resource groups share the same CRUD shape and `{resource}:{action}` permission codes:

| Method | Path | Permission |
|--------|------|------------|
| GET/POST | `/api/v1/users` | `system:user:list` / `system:user:create` |
| PUT | `/api/v1/users/:id`, `/users/:id/status`, `POST /users/:id/roles` | `system:user:update` |
| GET/POST | `/api/v1/roles`, `POST /roles/:id/permissions` | `system:role:*` (package-capped, see [Multi-tenancy](/en/modules/tenant)) |
| GET | `/api/v1/permissions/tree`, `/menus/tree`, `/departments/tree`, `/posts` | `system:{resource}:list` |

Full listing: OpenAPI contract (`make api-contract`).

## Advice for extenders

Seed new permission codes via SQL migration and grant them to `super_admin`; never hard-code role names in business logic — check permission codes only.
