# Code Generator

Pick a table, configure fields, generate CRUD backend + frontend. Console entry: "System → Code Generator" (`/system/codegen`).

## Three modes

| Mode | For | Conventions & output |
|------|------|----------|
| **Single table** | Plain entities | Standard list page + modal form + CRUD API |
| **Tree table** | Hierarchies (departments, categories) | Pick parent field (self-referencing, 0 = root), display field, optional sort field; extra `GET /tree` endpoint; delete rejected while children exist |
| **Master-detail** | Order-plus-lines style 1:N | Pick detail table + FK field (auto-suggested); create/update **replace detail rows in one transaction**; detail columns auto-derived |

## Flow (3-step wizard)

1. **Pick a table** — reflected from the database (write your migration first; system tables excluded).
2. **Configure fields** — module name (lowercase, drives routes/paths), page title, mode, then per column: label, in-list, in-search (string columns by default), in-form, required. Audit columns (`id/created_at/updated_at/deleted_at`) are excluded automatically. Form widgets are inferred from the column type (number → InputNumber, bool → Switch, else Input) — there is no widget picker.
3. **Preview or download** `codegen-<module>.zip`.

> The generator only produces files; it never writes into the repository.

## Output

```
server/<module>/{model,store,handlers,routes}.go
web/src/api/<module>.ts
web/src/pages/<module>/index.tsx
menu-<module>.sql        # adjust id/parent_id/sort, then execute
```

Generated code follows every house convention (response envelope, pagination, `tenant_id`, permission codes). Landing steps: unzip → call the generated `RegisterRoutes()` → seed permission codes (grant to `super_admin`) → run the menu SQL.

## Type mapping

PG int/serial → Go `int64` → TS `number`; numeric/float → `float64` → `number`; bool → `bool` → `boolean`; timestamp/date → `time.Time` → `string`; everything else → `string`.

## Known limits

No multi-table JOIN generation (master-detail covers 1:N); no dictionary-bound selects (swap in manually); `required` generates frontend validation only; permission/menu seeding is intentionally manual. Endpoints: `GET /api/v1/codegen/tables`, `GET /codegen/tables/:name/columns` (`system:codegen:list`), `POST /codegen/preview`, `POST /codegen/download` (`system:codegen:generate`).
