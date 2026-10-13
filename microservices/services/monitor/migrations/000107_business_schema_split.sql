-- +goose Up
-- 保留公开版已存在的 monitor_svc/audit_svc 及 public 兼容副本。
-- 新业务表移动 OID，不复制后级联删表；双端同时存在时拒绝猜测哪份数据更新。
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.gak_move_public_table_if_exists(
    target_schema TEXT, table_name TEXT
) RETURNS VOID LANGUAGE plpgsql AS $$
BEGIN
    IF to_regclass(format('%I.%I', 'public', table_name)) IS NULL THEN
        RETURN;
    END IF;
    IF to_regclass(format('%I.%I', target_schema, table_name)) IS NOT NULL THEN
        RAISE EXCEPTION 'Both public.% and %.% exist; reconcile before upgrade',
            table_name, target_schema, table_name;
    END IF;
    EXECUTE format('ALTER TABLE %I.%I SET SCHEMA %I', 'public', table_name, target_schema);
END;
$$;
-- +goose StatementEnd
CREATE SCHEMA IF NOT EXISTS ai_svc;
CREATE SCHEMA IF NOT EXISTS bpm_svc;
CREATE SCHEMA IF NOT EXISTS cc_svc;
CREATE SCHEMA IF NOT EXISTS crm_svc;
CREATE SCHEMA IF NOT EXISTS im_svc;
CREATE SCHEMA IF NOT EXISTS mp_svc;
CREATE SCHEMA IF NOT EXISTS pay_svc;
CREATE SCHEMA IF NOT EXISTS ticket_svc;
CREATE SCHEMA IF NOT EXISTS vis_svc;
SELECT public.gak_move_public_table_if_exists('ai_svc', 'ai_conversations');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'ai_document_chunks');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'ai_documents');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'ai_messages');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'ai_models');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'ai_quotas');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'ai_usage_logs');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'hermes_artifacts');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'hermes_cluster_mcp_servers');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'hermes_cluster_skills');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'hermes_clusters');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'hermes_cron_jobs');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'hermes_cron_runs');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'hermes_device_codes');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'hermes_device_events');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'hermes_devices');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'hermes_entitlements');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'hermes_instances');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'hermes_mcp_servers');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'hermes_model_tokens');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'hermes_office_floors');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'hermes_office_seats');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'hermes_policies');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'hermes_skills');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'hermes_task_runs');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'hermes_tasks');
SELECT public.gak_move_public_table_if_exists('ai_svc', 'hermes_user_skills');
SELECT public.gak_move_public_table_if_exists('bpm_svc', 'bpm_callback_jobs');
SELECT public.gak_move_public_table_if_exists('bpm_svc', 'bpm_cc_record');
SELECT public.gak_move_public_table_if_exists('bpm_svc', 'bpm_process_definition');
SELECT public.gak_move_public_table_if_exists('bpm_svc', 'bpm_process_instance');
SELECT public.gak_move_public_table_if_exists('bpm_svc', 'bpm_process_log');
SELECT public.gak_move_public_table_if_exists('bpm_svc', 'bpm_task');
SELECT public.gak_move_public_table_if_exists('cc_svc', 'cc_account_txns');
SELECT public.gak_move_public_table_if_exists('cc_svc', 'cc_agents');
SELECT public.gak_move_public_table_if_exists('cc_svc', 'cc_billing_accounts');
SELECT public.gak_move_public_table_if_exists('cc_svc', 'cc_bills');
SELECT public.gak_move_public_table_if_exists('cc_svc', 'cc_call_charges');
SELECT public.gak_move_public_table_if_exists('cc_svc', 'cc_calls');
SELECT public.gak_move_public_table_if_exists('cc_svc', 'cc_contacts');
SELECT public.gak_move_public_table_if_exists('cc_svc', 'cc_qc_keywords');
SELECT public.gak_move_public_table_if_exists('cc_svc', 'cc_qc_results');
SELECT public.gak_move_public_table_if_exists('cc_svc', 'cc_rates');
SELECT public.gak_move_public_table_if_exists('cc_svc', 'cc_transcribe_jobs');
SELECT public.gak_move_public_table_if_exists('cc_svc', 'cc_transcripts');
SELECT public.gak_move_public_table_if_exists('cc_svc', 'cc_webhook_logs');
SELECT public.gak_move_public_table_if_exists('crm_svc', 'crm_assign_logs');
SELECT public.gak_move_public_table_if_exists('crm_svc', 'crm_contacts');
SELECT public.gak_move_public_table_if_exists('crm_svc', 'crm_contracts');
SELECT public.gak_move_public_table_if_exists('crm_svc', 'crm_customers');
SELECT public.gak_move_public_table_if_exists('crm_svc', 'crm_followups');
SELECT public.gak_move_public_table_if_exists('crm_svc', 'crm_leads');
SELECT public.gak_move_public_table_if_exists('crm_svc', 'crm_opportunities');
SELECT public.gak_move_public_table_if_exists('crm_svc', 'crm_opportunity_stage_logs');
SELECT public.gak_move_public_table_if_exists('crm_svc', 'crm_payments');
SELECT public.gak_move_public_table_if_exists('crm_svc', 'crm_pool_rules');
SELECT public.gak_move_public_table_if_exists('im_svc', 'im_agent_presence');
SELECT public.gak_move_public_table_if_exists('im_svc', 'im_agent_skills');
SELECT public.gak_move_public_table_if_exists('im_svc', 'im_conversations');
SELECT public.gak_move_public_table_if_exists('im_svc', 'im_kb_daily_stats');
SELECT public.gak_move_public_table_if_exists('im_svc', 'im_kb_misses');
SELECT public.gak_move_public_table_if_exists('im_svc', 'im_messages');
SELECT public.gak_move_public_table_if_exists('im_svc', 'im_quick_replies');
SELECT public.gak_move_public_table_if_exists('im_svc', 'im_sensitive_words');
SELECT public.gak_move_public_table_if_exists('im_svc', 'im_sites');
SELECT public.gak_move_public_table_if_exists('im_svc', 'im_skill_groups');
SELECT public.gak_move_public_table_if_exists('im_svc', 'im_visitors');
SELECT public.gak_move_public_table_if_exists('mp_svc', 'channel_accounts');
SELECT public.gak_move_public_table_if_exists('mp_svc', 'mp_accounts');
SELECT public.gak_move_public_table_if_exists('mp_svc', 'mp_articles');
SELECT public.gak_move_public_table_if_exists('mp_svc', 'mp_fan_sync_jobs');
SELECT public.gak_move_public_table_if_exists('mp_svc', 'mp_fans');
SELECT public.gak_move_public_table_if_exists('mp_svc', 'mp_menus');
SELECT public.gak_move_public_table_if_exists('mp_svc', 'mp_messages');
SELECT public.gak_move_public_table_if_exists('mp_svc', 'mp_publish_result_jobs');
SELECT public.gak_move_public_table_if_exists('mp_svc', 'post_publications');
SELECT public.gak_move_public_table_if_exists('pay_svc', 'pay_callback_jobs');
SELECT public.gak_move_public_table_if_exists('pay_svc', 'pay_channels');
SELECT public.gak_move_public_table_if_exists('pay_svc', 'pay_notify_logs');
SELECT public.gak_move_public_table_if_exists('pay_svc', 'pay_orders');
SELECT public.gak_move_public_table_if_exists('pay_svc', 'pay_refunds');
SELECT public.gak_move_public_table_if_exists('ticket_svc', 'ticket_events');
SELECT public.gak_move_public_table_if_exists('ticket_svc', 'ticket_notification_jobs');
SELECT public.gak_move_public_table_if_exists('ticket_svc', 'ticket_tickets');
SELECT public.gak_move_public_table_if_exists('vis_svc', 'vis_brands');
SELECT public.gak_move_public_table_if_exists('vis_svc', 'vis_geo_prompts');
SELECT public.gak_move_public_table_if_exists('vis_svc', 'vis_geo_results');
SELECT public.gak_move_public_table_if_exists('vis_svc', 'vis_geo_runs');
SELECT public.gak_move_public_table_if_exists('vis_svc', 'vis_keywords');
SELECT public.gak_move_public_table_if_exists('vis_svc', 'vis_rank_snapshots');
SELECT public.gak_move_public_table_if_exists('vis_svc', 'vis_serp_tasks');
DROP FUNCTION public.gak_move_public_table_if_exists(TEXT, TEXT);

-- +goose Down
-- 完整产品升级含数据归属变化，不允许以删表/删除权限模拟回滚；应恢复升级前备份。
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Full product upgrade is forward-only; restore the pre-upgrade backup';
END $$;
-- +goose StatementEnd
