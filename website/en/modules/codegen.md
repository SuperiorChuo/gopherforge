# Code Generator

Pick a table, configure fields, generate CRUD backend + frontend. Console entry: "System → Code Generator".

## Three modes

| Mode | For | Output |
|------|------|----------|
| **Single table** | Plain entities | Standard list page + drawer form + CRUD API |
| **Tree table** | Hierarchies (departments, categories) | Tree grid, parent picker, recursive API |
| **Master-detail** | Order-plus-lines style 1:N | Inline detail editing, transactional save |

## Flow

1. **Pick a table** (reflected from the database — write your migration first).
2. **Configure fields**: label, widget type, list/search/required flags.
3. **Preview or download** a zip and drop files into place.

Generated code follows every house convention (response envelope, pagination, tenant_id, permission codes) — compliant by construction.
