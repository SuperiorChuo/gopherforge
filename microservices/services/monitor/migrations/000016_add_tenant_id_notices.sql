-- +goose Up
-- 公告租户化：此前 notices 全租户共见，SaaS 语义下租户公告必须隔离。
-- 存量数据归入默认租户；平台级公告仍可由平台管理员在各租户内分别发布。
ALTER TABLE notices ADD COLUMN IF NOT EXISTS tenant_id BIGINT NOT NULL DEFAULT 1;
CREATE INDEX IF NOT EXISTS idx_notices_tenant_id ON notices (tenant_id);

-- +goose Down
DROP INDEX IF EXISTS idx_notices_tenant_id;
ALTER TABLE notices DROP COLUMN IF EXISTS tenant_id;
