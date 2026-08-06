-- +goose Up
-- 忘记密码重置令牌：与 invites 同模式——只存 sha256(token) 哈希，明文仅经
-- 邮件一次性下发；原子消费（used_at 置位）防重放，30 分钟过期。
CREATE TABLE IF NOT EXISTS password_resets (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    tenant_id BIGINT NOT NULL DEFAULT 1,
    token_hash CHAR(64) NOT NULL,
    expires_at TIMESTAMPTZ(3) NOT NULL,
    used_at TIMESTAMPTZ(3) NULL,
    created_at TIMESTAMPTZ(3) NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_password_resets_token_hash ON password_resets (token_hash);
CREATE INDEX IF NOT EXISTS idx_password_resets_user_id ON password_resets (user_id);

-- +goose Down
DROP INDEX IF EXISTS idx_password_resets_user_id;
DROP INDEX IF EXISTS ux_password_resets_token_hash;
DROP TABLE IF EXISTS password_resets;
