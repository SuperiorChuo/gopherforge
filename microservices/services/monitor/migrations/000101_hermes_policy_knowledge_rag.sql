-- +goose Up
-- Hermes M4：企业知识库 RAG。策略开关 enable_knowledge_rag——为真的策略，
-- Hermes 模型代理（proxy）在转发前检索企业知识库相关块并注入 system message。
-- 服务端开关，不进签名载荷（只影响 proxy 行为，不发往设备）。

ALTER TABLE hermes_policies
  ADD COLUMN IF NOT EXISTS enable_knowledge_rag boolean NOT NULL DEFAULT false;

-- +goose Down
-- 完整产品升级含数据归属变化，不允许以删表/删除权限模拟回滚；应恢复升级前备份。
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Full product upgrade is forward-only; restore the pre-upgrade backup';
END $$;
-- +goose StatementEnd
