-- +goose Up
-- ai_conversations：列表固定为 tenant+user ORDER BY updated_at DESC。
CREATE INDEX IF NOT EXISTS idx_ai_conversations_tenant_user_updated ON ai_conversations (tenant_id, user_id, updated_at DESC);
DROP INDEX IF EXISTS idx_ai_conversations_tenant_id;
DROP INDEX IF EXISTS idx_ai_conversations_user_id;
DROP INDEX IF EXISTS idx_ai_conversations_tenant_user;

-- ai_messages：会话内按 id 顺序取（含 keyset 场景），复合索引免排序。
CREATE INDEX IF NOT EXISTS idx_ai_messages_conv_id ON ai_messages (conversation_id, id);
DROP INDEX IF EXISTS idx_ai_messages_conversation_id;

-- +goose Down
-- 完整产品升级含数据归属变化，不允许以删表/删除权限模拟回滚；应恢复升级前备份。
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Full product upgrade is forward-only; restore the pre-upgrade backup';
END $$;
-- +goose StatementEnd
