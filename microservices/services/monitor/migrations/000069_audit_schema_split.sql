-- +goose Up
-- Phase 2A：audit 服务拆分 schema-per-service（模式验证）
-- 建 audit_svc schema + 拷贝 audit 独属表（audit_logs/login_risk_events/security_events）。
-- 数据双份保留：public 副本作回滚点，audit 服务 DSN search_path=audit_svc,public 切流后
-- 确认无误再清理 public 副本。共享表（console_sessions/system_settings 等）留在 public 经 search_path 兜底。
--
-- 注意：本迁移由 migrate 服务（search_path=public）执行，显式 schema 限定符。

CREATE SCHEMA IF NOT EXISTS audit_svc;

-- audit_logs（写路径，Phase 1 grpc WriteLog 已验证）
CREATE TABLE IF NOT EXISTS audit_svc.audit_logs (LIKE public.audit_logs INCLUDING ALL);
INSERT INTO audit_svc.audit_logs SELECT * FROM public.audit_logs ON CONFLICT (id) DO NOTHING;
SELECT setval(pg_get_serial_sequence('audit_svc.audit_logs', 'id'),
       GREATEST((SELECT COALESCE(max(id), 1) FROM audit_svc.audit_logs), 1));

-- login_risk_events（登录风控事件）
CREATE TABLE IF NOT EXISTS audit_svc.login_risk_events (LIKE public.login_risk_events INCLUDING ALL);
INSERT INTO audit_svc.login_risk_events SELECT * FROM public.login_risk_events ON CONFLICT (id) DO NOTHING;
SELECT setval(pg_get_serial_sequence('audit_svc.login_risk_events', 'id'),
       GREATEST((SELECT COALESCE(max(id), 1) FROM audit_svc.login_risk_events), 1));

-- security_events（安全事件）
CREATE TABLE IF NOT EXISTS audit_svc.security_events (LIKE public.security_events INCLUDING ALL);
INSERT INTO audit_svc.security_events SELECT * FROM public.security_events ON CONFLICT (id) DO NOTHING;
SELECT setval(pg_get_serial_sequence('audit_svc.security_events', 'id'),
       GREATEST((SELECT COALESCE(max(id), 1) FROM audit_svc.security_events), 1));

-- +goose Down
-- 回滚：删 audit_svc（public 副本仍保留，数据零丢失）
DROP SCHEMA IF EXISTS audit_svc CASCADE;
