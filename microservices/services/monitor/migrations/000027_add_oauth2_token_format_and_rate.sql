-- +goose Up
-- OAuth2 服务端 B②：每客户端令牌形态与限流配额。
-- 两列都给默认值，存量客户端行为完全不变（opaque + 全局默认配额）。
ALTER TABLE oauth2_clients
    ADD COLUMN IF NOT EXISTS access_token_format VARCHAR(16) NOT NULL DEFAULT 'opaque',
    ADD COLUMN IF NOT EXISTS token_rate_per_minute INTEGER NOT NULL DEFAULT 0;

COMMENT ON COLUMN oauth2_clients.access_token_format IS 'access token 形态：opaque=不透明随机串（默认）；jwt=RFC 9068 自包含 JWT，资源服务器可经 JWKS 离线验签';
COMMENT ON COLUMN oauth2_clients.token_rate_per_minute IS 'token/introspect 端点每分钟配额（按 client_id 计）；0=用服务端默认值';

-- +goose Down
ALTER TABLE oauth2_clients
    DROP COLUMN IF EXISTS access_token_format,
    DROP COLUMN IF EXISTS token_rate_per_minute;
