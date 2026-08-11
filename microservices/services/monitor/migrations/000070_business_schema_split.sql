-- +goose Up
-- Phase 2B（公开脚手架适配）：只保留公开线实际提供的基础设施服务。
-- 业务域（AI/CC/CRM/IM/MP/PAY/TICKET/VIS）只属于主仓，禁止进入 micro。

CREATE SCHEMA IF NOT EXISTS bpm_svc;
CREATE TABLE IF NOT EXISTS bpm_svc.bpm_callback_jobs (LIKE public.bpm_callback_jobs INCLUDING ALL);
INSERT INTO bpm_svc.bpm_callback_jobs SELECT * FROM public.bpm_callback_jobs ON CONFLICT DO NOTHING;
SELECT setval(pg_get_serial_sequence('bpm_svc.bpm_callback_jobs', 'id'), GREATEST((SELECT COALESCE(max(id), 1) FROM bpm_svc.bpm_callback_jobs), 1));
CREATE TABLE IF NOT EXISTS bpm_svc.bpm_cc_record (LIKE public.bpm_cc_record INCLUDING ALL);
INSERT INTO bpm_svc.bpm_cc_record SELECT * FROM public.bpm_cc_record ON CONFLICT DO NOTHING;
SELECT setval(pg_get_serial_sequence('bpm_svc.bpm_cc_record', 'id'), GREATEST((SELECT COALESCE(max(id), 1) FROM bpm_svc.bpm_cc_record), 1));
CREATE TABLE IF NOT EXISTS bpm_svc.bpm_process_definition (LIKE public.bpm_process_definition INCLUDING ALL);
INSERT INTO bpm_svc.bpm_process_definition SELECT * FROM public.bpm_process_definition ON CONFLICT DO NOTHING;
SELECT setval(pg_get_serial_sequence('bpm_svc.bpm_process_definition', 'id'), GREATEST((SELECT COALESCE(max(id), 1) FROM bpm_svc.bpm_process_definition), 1));
CREATE TABLE IF NOT EXISTS bpm_svc.bpm_process_instance (LIKE public.bpm_process_instance INCLUDING ALL);
INSERT INTO bpm_svc.bpm_process_instance SELECT * FROM public.bpm_process_instance ON CONFLICT DO NOTHING;
SELECT setval(pg_get_serial_sequence('bpm_svc.bpm_process_instance', 'id'), GREATEST((SELECT COALESCE(max(id), 1) FROM bpm_svc.bpm_process_instance), 1));
CREATE TABLE IF NOT EXISTS bpm_svc.bpm_process_log (LIKE public.bpm_process_log INCLUDING ALL);
INSERT INTO bpm_svc.bpm_process_log SELECT * FROM public.bpm_process_log ON CONFLICT DO NOTHING;
SELECT setval(pg_get_serial_sequence('bpm_svc.bpm_process_log', 'id'), GREATEST((SELECT COALESCE(max(id), 1) FROM bpm_svc.bpm_process_log), 1));
CREATE TABLE IF NOT EXISTS bpm_svc.bpm_task (LIKE public.bpm_task INCLUDING ALL);
INSERT INTO bpm_svc.bpm_task SELECT * FROM public.bpm_task ON CONFLICT DO NOTHING;
SELECT setval(pg_get_serial_sequence('bpm_svc.bpm_task', 'id'), GREATEST((SELECT COALESCE(max(id), 1) FROM bpm_svc.bpm_task), 1));

CREATE SCHEMA IF NOT EXISTS monitor_svc;
CREATE TABLE IF NOT EXISTS monitor_svc.monitor_alert_events (LIKE public.monitor_alert_events INCLUDING ALL);
INSERT INTO monitor_svc.monitor_alert_events SELECT * FROM public.monitor_alert_events ON CONFLICT DO NOTHING;
SELECT setval(pg_get_serial_sequence('monitor_svc.monitor_alert_events', 'id'), GREATEST((SELECT COALESCE(max(id), 1) FROM monitor_svc.monitor_alert_events), 1));
CREATE TABLE IF NOT EXISTS monitor_svc.monitor_alert_rules (LIKE public.monitor_alert_rules INCLUDING ALL);
INSERT INTO monitor_svc.monitor_alert_rules SELECT * FROM public.monitor_alert_rules ON CONFLICT DO NOTHING;
SELECT setval(pg_get_serial_sequence('monitor_svc.monitor_alert_rules', 'id'), GREATEST((SELECT COALESCE(max(id), 1) FROM monitor_svc.monitor_alert_rules), 1));
CREATE TABLE IF NOT EXISTS monitor_svc.monitor_metric_samples (LIKE public.monitor_metric_samples INCLUDING ALL);
INSERT INTO monitor_svc.monitor_metric_samples SELECT * FROM public.monitor_metric_samples ON CONFLICT DO NOTHING;
SELECT setval(pg_get_serial_sequence('monitor_svc.monitor_metric_samples', 'id'), GREATEST((SELECT COALESCE(max(id), 1) FROM monitor_svc.monitor_metric_samples), 1));
CREATE TABLE IF NOT EXISTS monitor_svc.ops_job_heartbeats (LIKE public.ops_job_heartbeats INCLUDING ALL);
INSERT INTO monitor_svc.ops_job_heartbeats SELECT * FROM public.ops_job_heartbeats ON CONFLICT DO NOTHING;
SELECT setval(pg_get_serial_sequence('monitor_svc.ops_job_heartbeats', 'id'), GREATEST((SELECT COALESCE(max(id), 1) FROM monitor_svc.ops_job_heartbeats), 1));
CREATE TABLE IF NOT EXISTS monitor_svc.scheduled_job_logs (LIKE public.scheduled_job_logs INCLUDING ALL);
INSERT INTO monitor_svc.scheduled_job_logs SELECT * FROM public.scheduled_job_logs ON CONFLICT DO NOTHING;
SELECT setval(pg_get_serial_sequence('monitor_svc.scheduled_job_logs', 'id'), GREATEST((SELECT COALESCE(max(id), 1) FROM monitor_svc.scheduled_job_logs), 1));
CREATE TABLE IF NOT EXISTS monitor_svc.scheduled_jobs (LIKE public.scheduled_jobs INCLUDING ALL);
INSERT INTO monitor_svc.scheduled_jobs SELECT * FROM public.scheduled_jobs ON CONFLICT DO NOTHING;
SELECT setval(pg_get_serial_sequence('monitor_svc.scheduled_jobs', 'id'), GREATEST((SELECT COALESCE(max(id), 1) FROM monitor_svc.scheduled_jobs), 1));

-- +goose Down
-- 回滚：删公开线 schema（public 副本保留）。
DROP SCHEMA IF EXISTS bpm_svc CASCADE;
DROP SCHEMA IF EXISTS monitor_svc CASCADE;
