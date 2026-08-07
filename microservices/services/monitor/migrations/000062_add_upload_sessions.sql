-- +goose Up
-- 分片上传会话：大文件按固定分片上传，会话记录进度与存储键；complete 时
-- 拼接为对象并落 files 表。received_bitmap 是已到达分片号的 JSON 数组。
CREATE TABLE IF NOT EXISTS upload_sessions (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL DEFAULT 1,
    user_id BIGINT NOT NULL,
    file_name VARCHAR(255) NOT NULL,
    file_size BIGINT NOT NULL,
    chunk_size BIGINT NOT NULL,
    total_chunks INT NOT NULL,
    received_count INT NOT NULL DEFAULT 0,
    received_bitmap TEXT NOT NULL DEFAULT '[]',
    storage_type VARCHAR(20) NOT NULL DEFAULT 'local',
    object_key VARCHAR(512) NOT NULL DEFAULT '',
    hash VARCHAR(64) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_upload_sessions_tenant ON upload_sessions (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_upload_sessions_expires ON upload_sessions (expires_at);

-- +goose Down
DROP INDEX IF EXISTS idx_upload_sessions_expires;
DROP INDEX IF EXISTS idx_upload_sessions_tenant;
DROP TABLE IF EXISTS upload_sessions;
