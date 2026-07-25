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

## Console pages

`/system/operation-log` (rich filters + masked body detail) · `/system/login-log` (filters + **geo distribution map** + 7-day success/failure trend) · `/system/audit-log` (keyword/action/target facets).

## Endpoints

`GET /api/v1/operation-logs` (+`/:id`, `/stats`, `/export` CSV) under `system:log:operation`; `DELETE /operation-logs/clear?days=N` under `system:log:operation:clear`. `GET /api/v1/login-logs` (+`/stats`, `/trend`; personal `/my`, `/last`) under `system:log:login`, `DELETE /login-logs/clear`. `GET /api/v1/logs/audit` under `system:log:audit`.

## Retention

Cleanup is **manual** (the clear endpoints, exposed in the console) — schedule them from your own cron for long-running deployments. NATS events expire independently after 7 days.
