# Audit Logs

The audit service centralises three log kinds: **operation logs** (every API request), **login logs** (auth events) and **business audit logs** (before/after snapshots of key objects).

| Kind | Table | Captured |
|------|-------|----------|
| Operation | `operation_logs` | actor, module/action, method+path, masked request/response bodies, status, IP, UA, latency, request_id |
| Login | `login_logs` | user, login type (password/GitHub/WeChat/TOTP), success/failure + reason, IP + geo, device/OS/browser |
| Business audit | `audit_logs` | actor, action, target type/id, before/after JSON, summary |

## Collection pipeline

**Operation logs** are captured by an HTTP middleware: sensitive fields (`password`, `token`, …) are masked, entries go through an in-process buffer (1000 entries, 2 s DB write timeout) and persist asynchronously — **a failed write never affects the business request**. Health/captcha paths are skipped.

**Login logs** ride NATS JetStream: the auth service publishes `auth.login.success` / `auth.login.failed` events (best-effort, 2 s timeout, never blocks login); the audit service consumes them durably (7-day stream retention, 5 redelivery attempts). This is why NATS is a required dependency.

**IP geolocation**: offline `ip2region` first (download `ip2region.xdb` via `scripts/download-ip2region.sh`), falling back to ip-api.com (1 h cache); private/loopback IPs are tagged "internal". Missing the offline DB degrades gracefully.

## Core permission tables: transactional audit (on by default)

Single-row create/update/delete on `users`, `roles`, `departments` and `menus` is audited automatically via a GORM plugin (`shared/pkg/audittrail`) that snapshots before/after within the write transaction — **the audit row commits in the same transaction as the change, and a failed audit write rolls the business change back** (fail closed), so there is never a window where a permission change happens without a trace.

- **Scope**: single-row CRUD on the four tables' own columns; join tables and raw SQL are out of scope.
- **Masking**: passwords, TOTP, recovery codes and similar sensitive fields are recursively masked in snapshots — plaintext never lands.
- **Attribution**: snapshots carry actor, tenant and before/after JSON, queryable from the console's audit-log page (keyword / action / target facets).
- **How this differs from operation logs**: operation logs are middleware-level, async best-effort (a failed write never affects the request); this is same-transaction strong consistency — for permission changes that must be traceable and must not be lost.

## Tenant isolation

Audit rows are written with the **requesting tenant**, and list / summary / filter endpoints are always restricted to the current tenant — an admin can't read another tenant's audit data, and events are never mis-attributed to the default tenant. Retention cleanup runs across all tenants at the ops level, but reads stay tenant-scoped.

## Console pages

`/system/operation-log` (rich filters + masked body detail) · `/system/login-log` (filters + **geo distribution map** + 7-day success/failure trend) · `/system/audit-log` (keyword/action/target facets).

## Endpoints

`GET /api/v1/operation-logs` (+`/:id`, `/stats`, `/export` CSV) under `system:log:operation`; `DELETE /operation-logs/clear?days=N` under `system:log:operation:clear`. `GET /api/v1/login-logs` (+`/stats`, `/trend`; personal `/my`, `/last`) under `system:log:login`, `DELETE /login-logs/clear`. `GET /api/v1/logs/audit` under `system:log:audit`.

## Retention

Two paths: set `AUDIT_LOG_RETENTION_DAYS=N` (default `0` = off — no implicit data deletion) and the audit service prunes operation/login logs older than N days daily, **across all tenants**, reporting a `audit.log_retention` heartbeat to the ops job center; or use the manual clear endpoints exposed in the console (tenant- and permission-scoped). **Business audit logs (`audit_logs`) are deliberately excluded from automatic cleanup** — they are the compliance trail. NATS events expire independently after 7 days.
