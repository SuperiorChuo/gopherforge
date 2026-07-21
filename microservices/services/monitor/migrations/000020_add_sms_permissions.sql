-- +goose Up
-- 短信管理权限点（渠道/模板/日志/发送）。超级管理员自动获得全部权限。
INSERT INTO permissions (name, code, type, path, method, parent_id, created_at, updated_at) VALUES
('短信渠道查看', 'system:sms-channel:list', 2, '/api/v1/sms/channels', 'GET', 2, NOW(), NOW()),
('短信渠道新建', 'system:sms-channel:create', 2, '/api/v1/sms/channels', 'POST', 2, NOW(), NOW()),
('短信渠道更新', 'system:sms-channel:update', 2, '/api/v1/sms/channels/:id', 'PUT', 2, NOW(), NOW()),
('短信渠道删除', 'system:sms-channel:delete', 2, '/api/v1/sms/channels/:id', 'DELETE', 2, NOW(), NOW()),
('短信模板查看', 'system:sms-template:list', 2, '/api/v1/sms/templates', 'GET', 2, NOW(), NOW()),
('短信模板新建', 'system:sms-template:create', 2, '/api/v1/sms/templates', 'POST', 2, NOW(), NOW()),
('短信模板更新', 'system:sms-template:update', 2, '/api/v1/sms/templates/:id', 'PUT', 2, NOW(), NOW()),
('短信模板删除', 'system:sms-template:delete', 2, '/api/v1/sms/templates/:id', 'DELETE', 2, NOW(), NOW()),
('短信日志查看', 'system:sms-log:list', 2, '/api/v1/sms/logs', 'GET', 2, NOW(), NOW()),
('短信发送', 'system:sms:send', 2, '/api/v1/sms/send', 'POST', 2, NOW(), NOW())
ON CONFLICT DO NOTHING;

-- 超管角色补挂新权限（与 000001 的全量挂载语义一致）
INSERT INTO role_permissions (role_id, permission_id)
SELECT 1, id FROM permissions WHERE code IN (
  'system:sms-channel:list', 'system:sms-channel:create', 'system:sms-channel:update', 'system:sms-channel:delete',
  'system:sms-template:list', 'system:sms-template:create', 'system:sms-template:update', 'system:sms-template:delete',
  'system:sms-log:list', 'system:sms:send'
)
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM role_permissions WHERE permission_id IN
  (SELECT id FROM permissions WHERE code IN (
    'system:sms-channel:list', 'system:sms-channel:create', 'system:sms-channel:update', 'system:sms-channel:delete',
    'system:sms-template:list', 'system:sms-template:create', 'system:sms-template:update', 'system:sms-template:delete',
    'system:sms-log:list', 'system:sms:send'
  ));
DELETE FROM permissions WHERE code IN (
  'system:sms-channel:list', 'system:sms-channel:create', 'system:sms-channel:update', 'system:sms-channel:delete',
  'system:sms-template:list', 'system:sms-template:create', 'system:sms-template:update', 'system:sms-template:delete',
  'system:sms-log:list', 'system:sms:send'
);
