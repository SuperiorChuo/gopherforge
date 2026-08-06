-- +goose Up
-- 存储配额：0 = 不限（默认），正数 = 该租户可用对象存储上限（MB）。
ALTER TABLE tenant_packages ADD COLUMN IF NOT EXISTS storage_quota_mb BIGINT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE tenant_packages DROP COLUMN IF EXISTS storage_quota_mb;
