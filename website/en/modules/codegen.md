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
3. **Preview, download or write** — see delivery modes below.

## Delivery modes

**Preview** (per-file diff, including changes to existing wiring files) · **Download ZIP** · **Write into the repository**.

::: warning Repository write is high-risk and off by default
It requires all four: `CODEGEN_WRITE_ENABLED=true`, `CODEGEN_REPO_ROOT` pointing at a real repo root, a platform-admin caller, and the `system:codegen:write` permission. Paths are canonicalised (symlinks resolved) and anything escaping the root is rejected (`..`, absolute paths, volume names, backslashes, NUL); writes go through staging + backup + a lock. **Enable in development only** — in production keep it off and use the ZIP download so changes go through normal review.
:::

A repository snapshot is baked into the image (only the source files the generator reads/patches), so preview and download work out of the box without mounting the host repo.

## Output

Seven files in the repo's own layered structure:

```
microservices/services/system/internal/model/<module>.go
microservices/services/system/internal/dao/system/<module>.go
microservices/services/system/internal/service/system/<module>.go
microservices/services/system/internal/api/system/<module>.go
microservices/web/src/api/<module>.ts
microservices/web/src/pages/system/<module>/index.tsx
microservices/services/monitor/migrations/000000_codegen_<module>.sql
```

Beyond new files the generator also **patches four wiring points**: service route registration, the frontend route table, the sidebar layout and the menu seed. Write mode applies them; in download mode the preview shows the diffs for you to merge (the `000000` migration number is a placeholder — renumber it to your repo's next free slot).

Generated code follows every house convention (response envelope, pagination, `tenant_id`, permission codes).

## Type mapping

PG int/serial → Go `int64` → TS `number`; numeric/float → `float64` → `number`; bool → `bool` → `boolean`; timestamp/date → `time.Time` → `string`; everything else → `string`.

## Relations & dictionaries

Many-to-many (declare the join table and both foreign keys — the generated code handles the association plus multi-select tags in the UI), dictionary-bound fields (rendered as selects rather than plain inputs), and master-detail (detail rows replaced transactionally).

## Known limits

No arbitrary multi-table JOIN generation (1:N via master-detail, N:M via the m2m declaration, anything else is manual); `required` generates frontend validation only; the permission migration ships with a placeholder `000000` number that you must renumber.

## Endpoints

| Method | Path | Permission |
|--------|------|------------|
| GET | `/api/v1/codegen/capabilities` | `system:codegen:list` |
| GET | `/api/v1/codegen/tables` · `/tables/:name/columns` · `/tables/:name/schema` | `system:codegen:list` |
| POST | `/api/v1/codegen/preview` · `/download` | `system:codegen:generate` |
| POST | `/api/v1/codegen/write` | `system:codegen:write` + platform admin + both server switches |
