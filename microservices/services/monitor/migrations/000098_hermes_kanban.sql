-- +goose Up
-- Hermes kanban 里程碑：每集群一 board。
--
-- 集群 slug 非空 = 该集群启用 kanban 通道（manager 按 slug 建 board、按任务
-- 建 kanban 任务、gateway dispatcher 派生 worker）；空 slug = 既有 push 通道
-- （phase-3 行为一字不变）。
--
-- 部分唯一索引：非 kanban 集群 slug 全为空串，不能互相冲突，因此只对非空
-- slug 建唯一约束。
ALTER TABLE hermes_clusters
  ADD COLUMN IF NOT EXISTS slug varchar(80) NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS uk_hermes_clusters_tenant_slug
  ON hermes_clusters (tenant_id, slug) WHERE slug <> '';

-- +goose Down
-- 完整产品升级含数据归属变化，不允许以删表/删除权限模拟回滚；应恢复升级前备份。
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Full product upgrade is forward-only; restore the pre-upgrade backup';
END $$;
-- +goose StatementEnd
