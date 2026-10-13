-- +goose Up
-- 000069/000070 使用 LIKE INCLUDING ALL 搬迁业务表时，目标 id default 仍依赖
-- public schema 的原序列；000073 DROP public.* CASCADE 后该 default 被级联删除，
-- 导致服务后续 INSERT 以 NULL id 失败。为每张受影响表补独立序列、OWNED BY、
-- default，并把序列推进到现有最大 id。表不存在（fresh install 尚待 AutoMigrate）
-- 时跳过；服务 AutoMigrate 会直接创建自身主键序列。
-- +goose StatementBegin
DO $$
DECLARE
    item RECORD;
    sequence_name TEXT;
    max_id BIGINT;
BEGIN
    FOR item IN
        SELECT * FROM (VALUES
    ('audit_svc', 'login_risk_events'),
    ('audit_svc', 'security_events'),
    ('ai_svc', 'ai_conversations'),
    ('ai_svc', 'ai_document_chunks'),
    ('ai_svc', 'ai_documents'),
    ('ai_svc', 'ai_messages'),
    ('ai_svc', 'ai_models'),
    ('ai_svc', 'ai_quotas'),
    ('ai_svc', 'ai_usage_logs'),
    ('ai_svc', 'hermes_artifacts'),
    ('ai_svc', 'hermes_cluster_mcp_servers'),
    ('ai_svc', 'hermes_cluster_skills'),
    ('ai_svc', 'hermes_clusters'),
    ('ai_svc', 'hermes_cron_jobs'),
    ('ai_svc', 'hermes_cron_runs'),
    ('ai_svc', 'hermes_device_codes'),
    ('ai_svc', 'hermes_device_events'),
    ('ai_svc', 'hermes_devices'),
    ('ai_svc', 'hermes_entitlements'),
    ('ai_svc', 'hermes_instances'),
    ('ai_svc', 'hermes_mcp_servers'),
    ('ai_svc', 'hermes_model_tokens'),
    ('ai_svc', 'hermes_office_floors'),
    ('ai_svc', 'hermes_office_seats'),
    ('ai_svc', 'hermes_policies'),
    ('ai_svc', 'hermes_skills'),
    ('ai_svc', 'hermes_task_runs'),
    ('ai_svc', 'hermes_tasks'),
    ('ai_svc', 'hermes_user_skills'),
    ('bpm_svc', 'bpm_callback_jobs'),
    ('bpm_svc', 'bpm_cc_record'),
    ('bpm_svc', 'bpm_process_definition'),
    ('bpm_svc', 'bpm_process_instance'),
    ('bpm_svc', 'bpm_process_log'),
    ('bpm_svc', 'bpm_task'),
    ('cc_svc', 'cc_account_txns'),
    ('cc_svc', 'cc_agents'),
    ('cc_svc', 'cc_billing_accounts'),
    ('cc_svc', 'cc_bills'),
    ('cc_svc', 'cc_call_charges'),
    ('cc_svc', 'cc_calls'),
    ('cc_svc', 'cc_contacts'),
    ('cc_svc', 'cc_qc_keywords'),
    ('cc_svc', 'cc_qc_results'),
    ('cc_svc', 'cc_rates'),
    ('cc_svc', 'cc_transcribe_jobs'),
    ('cc_svc', 'cc_transcripts'),
    ('cc_svc', 'cc_webhook_logs'),
    ('crm_svc', 'crm_assign_logs'),
    ('crm_svc', 'crm_contacts'),
    ('crm_svc', 'crm_contracts'),
    ('crm_svc', 'crm_customers'),
    ('crm_svc', 'crm_followups'),
    ('crm_svc', 'crm_leads'),
    ('crm_svc', 'crm_opportunities'),
    ('crm_svc', 'crm_opportunity_stage_logs'),
    ('crm_svc', 'crm_payments'),
    ('crm_svc', 'crm_pool_rules'),
    ('im_svc', 'im_agent_skills'),
    ('im_svc', 'im_conversations'),
    ('im_svc', 'im_kb_daily_stats'),
    ('im_svc', 'im_kb_misses'),
    ('im_svc', 'im_messages'),
    ('im_svc', 'im_quick_replies'),
    ('im_svc', 'im_sensitive_words'),
    ('im_svc', 'im_sites'),
    ('im_svc', 'im_skill_groups'),
    ('im_svc', 'im_visitors'),
    ('monitor_svc', 'monitor_alert_events'),
    ('monitor_svc', 'monitor_alert_rules'),
    ('monitor_svc', 'monitor_metric_samples'),
    ('monitor_svc', 'ops_job_heartbeats'),
    ('monitor_svc', 'scheduled_job_logs'),
    ('monitor_svc', 'scheduled_jobs'),
    ('mp_svc', 'channel_accounts'),
    ('mp_svc', 'mp_accounts'),
    ('mp_svc', 'mp_articles'),
    ('mp_svc', 'mp_fan_sync_jobs'),
    ('mp_svc', 'mp_fans'),
    ('mp_svc', 'mp_menus'),
    ('mp_svc', 'mp_messages'),
    ('mp_svc', 'mp_publish_result_jobs'),
    ('mp_svc', 'post_publications'),
    ('pay_svc', 'pay_callback_jobs'),
    ('pay_svc', 'pay_channels'),
    ('pay_svc', 'pay_notify_logs'),
    ('pay_svc', 'pay_orders'),
    ('pay_svc', 'pay_refunds'),
    ('ticket_svc', 'ticket_events'),
    ('ticket_svc', 'ticket_notification_jobs'),
    ('ticket_svc', 'ticket_tickets'),
    ('vis_svc', 'vis_brands'),
    ('vis_svc', 'vis_geo_prompts'),
    ('vis_svc', 'vis_geo_results'),
    ('vis_svc', 'vis_geo_runs'),
    ('vis_svc', 'vis_keywords'),
    ('vis_svc', 'vis_rank_snapshots'),
    ('vis_svc', 'vis_serp_tasks')
        ) AS affected(schema_name, table_name)
    LOOP
        IF to_regclass(format('%I.%I', item.schema_name, item.table_name)) IS NULL THEN
            CONTINUE;
        END IF;

        -- fresh install 在 000070 跳过源表，后续若表已由服务 AutoMigrate 建成
        -- identity/serial 且 default 正常，无需也不能改 identity sequence ownership。
        -- LIKE INCLUDING ALL copies may still reference an unowned public sequence.
        -- Retain that live default and its high-water mark instead of silently replacing it.
        sequence_name := COALESCE(
            pg_get_serial_sequence(format('%I.%I', item.schema_name, item.table_name), 'id'),
            (SELECT seq.oid::regclass::text
             FROM pg_attrdef def
             JOIN pg_attribute col ON col.attrelid = def.adrelid AND col.attnum = def.adnum
             JOIN pg_depend dep ON dep.classid = 'pg_attrdef'::regclass AND dep.objid = def.oid
                               AND dep.refclassid = 'pg_class'::regclass
             JOIN pg_class seq ON seq.oid = dep.refobjid AND seq.relkind = 'S'
             WHERE def.adrelid = to_regclass(format('%I.%I', item.schema_name, item.table_name))
               AND col.attname = 'id')
        );
        IF sequence_name IS NOT NULL THEN
            EXECUTE format('SELECT COALESCE(MAX(id), 0) FROM %I.%I', item.schema_name, item.table_name)
                INTO max_id;
            -- Never rewind a healthy sequence, including values consumed by deleted rows.
            IF max_id > 0 THEN
                EXECUTE format('SELECT setval(%L::regclass, GREATEST(last_value, %s), true) FROM %s',
                    sequence_name, max_id, sequence_name);
            END IF;
            CONTINUE;
        END IF;

        sequence_name := item.table_name || '_id_seq_repaired';
        EXECUTE format('CREATE SEQUENCE IF NOT EXISTS %I.%I', item.schema_name, sequence_name);
        EXECUTE format(
            'ALTER SEQUENCE %I.%I OWNED BY %I.%I.id',
            item.schema_name, sequence_name, item.schema_name, item.table_name
        );
        EXECUTE format(
            'ALTER TABLE %I.%I ALTER COLUMN id SET DEFAULT nextval(%L::regclass)',
            item.schema_name, item.table_name, format('%I.%I', item.schema_name, sequence_name)
        );
        EXECUTE format('SELECT COALESCE(MAX(id), 0) FROM %I.%I', item.schema_name, item.table_name)
            INTO max_id;
        IF max_id > 0 THEN
            EXECUTE format('SELECT setval(%L::regclass, GREATEST(last_value, %s), true) FROM %I.%I',
                format('%I.%I', item.schema_name, sequence_name), max_id, item.schema_name, sequence_name);
        END IF;
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- 完整产品升级含数据归属变化，不允许以删表/删除权限模拟回滚；应恢复升级前备份。
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Full product upgrade is forward-only; restore the pre-upgrade backup';
END $$;
-- +goose StatementEnd
