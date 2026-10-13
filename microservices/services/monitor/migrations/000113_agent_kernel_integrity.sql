-- +goose Up
-- 版本工具授权是不可变 Spec 的关系化投影；发布后不得被改写或删除，
-- 否则仅靠运行时 hash 校验仍可能被数据库层静默替换授权契约。
DROP TRIGGER IF EXISTS trg_agent_version_tools_immutable ON ai_svc.agent_version_tools;
CREATE TRIGGER trg_agent_version_tools_immutable
BEFORE UPDATE OR DELETE ON ai_svc.agent_version_tools
FOR EACH ROW EXECUTE FUNCTION ai_svc.gak_reject_agent_version_mutation();

-- +goose Down
-- 完整产品升级含数据归属变化，不允许以删表/删除权限模拟回滚；应恢复升级前备份。
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Full product upgrade is forward-only; restore the pre-upgrade backup';
END $$;
-- +goose StatementEnd
