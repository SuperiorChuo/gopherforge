-- +goose Up
-- 设备指纹：前端全局 X-Device-ID 注入，登录事件透传，用于新设备登录检测。
ALTER TABLE login_logs ADD COLUMN IF NOT EXISTS device_id VARCHAR(64) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_login_logs_user_device ON login_logs (user_id, device_id);

-- +goose Down
DROP INDEX IF EXISTS idx_login_logs_user_device;
ALTER TABLE login_logs DROP COLUMN IF EXISTS device_id;
