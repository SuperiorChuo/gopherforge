-- +goose Up
-- 断点续传查询 (hash, tenant_id, status) 缺前置索引，补 (tenant_id, status, hash)。
CREATE INDEX IF NOT EXISTS idx_upload_sessions_resume
    ON upload_sessions (tenant_id, status, hash);

-- +goose Down
DROP INDEX IF EXISTS idx_upload_sessions_resume;
