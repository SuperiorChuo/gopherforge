-- +goose Up
-- 登录风控事件：audit 检测到新 IP / 新设备登录后落库（此前只发站内信不记录），
-- 供 /system/login-security 控制页查看与标记处理。alerted/notified_at 表示是否已提醒。
CREATE TABLE IF NOT EXISTS login_risk_events (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL DEFAULT 1,
    user_id BIGINT NOT NULL,
    username VARCHAR(100) NOT NULL DEFAULT '',
    ip VARCHAR(64) NOT NULL DEFAULT '',
    device_id VARCHAR(64) NOT NULL DEFAULT '',
    reason VARCHAR(16) NOT NULL,
    alerted BOOLEAN NOT NULL DEFAULT FALSE,
    notified_at TIMESTAMPTZ NULL,
    processed BOOLEAN NOT NULL DEFAULT FALSE,
    processed_by BIGINT NULL,
    processed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_login_risk_events_user_created ON login_risk_events (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_login_risk_events_processed_created ON login_risk_events (processed, created_at DESC);

-- 权限（控制页读取 + 解封/标记处理写操作）。ID 取 187/188（109 现状 max=186，fresh 库必小于此）。
INSERT INTO permissions (id,name,code,type,path,method,parent_id,created_at,updated_at) VALUES
(187,'登录安全读取','system:login-security:list',2,'/api/v1/login-security/blocked-ips','GET',2,NOW(),NOW()),
(188,'登录安全管理','system:login-security:manage',2,'/api/v1/login-risk-events/:id/process','POST',2,NOW(),NOW())
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id,permission_id)
SELECT 1, id FROM permissions WHERE code IN ('system:login-security:list','system:login-security:manage')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id,permission_id)
SELECT 2, id FROM permissions WHERE code IN ('system:login-security:list')
ON CONFLICT DO NOTHING;

SELECT setval(pg_get_serial_sequence('permissions','id'), (SELECT COALESCE(MAX(id),1) FROM permissions));

-- +goose Down
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE code IN ('system:login-security:list','system:login-security:manage'));
DELETE FROM permissions WHERE code IN ('system:login-security:list','system:login-security:manage');
DROP TABLE IF EXISTS login_risk_events;
