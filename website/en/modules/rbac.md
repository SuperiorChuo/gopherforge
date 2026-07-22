# RBAC

Full **user–role–permission** RBAC across three granularities: API, button and data.

| Object | Notes |
|------|------|
| Users | Department, multiple positions, enable/disable, Excel import/export |
| Roles | Permission set + data scope; `super_admin` bypasses checks |
| Permissions | Code convention `{domain}:{resource}:{action}` |
| Menus | Seeded tree filtered by role; drives the sidebar |
| Departments | Tree with a leader (used by the workflow "department leader" rule) |
| Positions | Many-to-many with users |

**Granularity**: route middleware (Redis-cached permission sets) → API level; `usePermission().hasPerm(code)` → button level; role data scopes (all / department-and-below / self) auto-filter list queries → data level.

Seed new permission codes via SQL migration and grant them to `super_admin`; never hard-code role names in business logic.
