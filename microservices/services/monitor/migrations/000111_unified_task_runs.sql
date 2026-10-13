-- +goose Up
-- 将公开版真实任务账本原表移动，保留全部记录、约束和序列。
-- +goose StatementBegin
DO $$ BEGIN
    IF to_regclass('public.ops_task_runs') IS NULL THEN
        RAISE EXCEPTION 'Missing public task ledger; expected public v0.7.0 schema';
    END IF;
    IF to_regclass('monitor_svc.ops_task_runs') IS NOT NULL THEN
        RAISE EXCEPTION 'Both task ledgers exist; reconcile before upgrade';
    END IF;
    ALTER TABLE public.ops_task_runs SET SCHEMA monitor_svc;
    PERFORM setval(pg_get_serial_sequence('monitor_svc.ops_task_runs', 'id'),
        GREATEST((SELECT COALESCE(MAX(id), 1) FROM monitor_svc.ops_task_runs),
                 (SELECT last_value FROM monitor_svc.ops_task_runs_id_seq)));
END $$;
-- +goose StatementEnd

-- +goose Down
-- 完整产品升级含数据归属变化，不允许以删表/删除权限模拟回滚；应恢复升级前备份。
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Full product upgrade is forward-only; restore the pre-upgrade backup';
END $$;
-- +goose StatementEnd
