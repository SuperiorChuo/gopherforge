-- +goose Up
-- 业务服务此前只验证登录身份和租户，缺少动作级授权。新增读/写权限并只默认授予
-- super_admin；其他角色由管理员按岗位显式分配。

INSERT INTO permissions (name, code, type, path, method, parent_id, created_at, updated_at) VALUES
('智能客服坐席访问', 'im:agent:access', 2, '/api/v1/im/agent', '*', 0, NOW(), NOW()),
('智能客服后台管理', 'im:admin:manage', 2, '/api/v1/im/admin', '*', 0, NOW(), NOW()),
('工单查看', 'ticket:read', 2, '/api/v1/ticket', 'GET', 0, NOW(), NOW()),
('工单操作', 'ticket:write', 2, '/api/v1/ticket', '*', 0, NOW(), NOW()),
('CRM 查看', 'crm:read', 2, '/api/v1/crm', 'GET', 0, NOW(), NOW()),
('CRM 操作', 'crm:write', 2, '/api/v1/crm', '*', 0, NOW(), NOW()),
('呼叫中心查看', 'cc:read', 2, '/api/v1/cc', 'GET', 0, NOW(), NOW()),
('呼叫中心操作', 'cc:write', 2, '/api/v1/cc', '*', 0, NOW(), NOW()),
('可见度查看', 'visibility:read', 2, '/api/v1/visibility', 'GET', 0, NOW(), NOW()),
('可见度操作', 'visibility:write', 2, '/api/v1/visibility', '*', 0, NOW(), NOW()),
('社媒内容查看', 'social:read', 2, '/api/v1/mp', 'GET', 0, NOW(), NOW()),
('社媒内容操作', 'social:write', 2, '/api/v1/mp', '*', 0, NOW(), NOW()),
('通知模板查看', 'notify:template:read', 2, '/api/v1/notify/templates', 'GET', 0, NOW(), NOW()),
('通知模板操作', 'notify:template:write', 2, '/api/v1/notify/templates', '*', 0, NOW(), NOW()),
('支付中心查看', 'pay:read', 2, '/api/v1/pay', 'GET', 0, NOW(), NOW()),
('支付中心操作', 'pay:write', 2, '/api/v1/pay', '*', 0, NOW(), NOW())
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.code = 'super_admin'
  AND p.code IN (
    'im:agent:access', 'im:admin:manage', 'ticket:read', 'ticket:write',
    'crm:read', 'crm:write', 'cc:read', 'cc:write',
    'visibility:read', 'visibility:write', 'social:read', 'social:write',
    'notify:template:read', 'notify:template:write', 'pay:read', 'pay:write'
  )
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

UPDATE menus SET permission = CASE
  WHEN path IN ('/im/sites', '/im/skills', '/im/sensitive') THEN 'im:admin:manage'
  WHEN path IN ('/im/desk', '/im/stats', '/im/kb-misses', '/im/low-ratings') THEN 'im:agent:access'
  WHEN path = '/ticket' THEN 'ticket:read'
  WHEN path LIKE '/crm/%' THEN 'crm:read'
  WHEN path LIKE '/cc/%' THEN 'cc:read'
  WHEN path LIKE '/visibility/%' THEN 'visibility:read'
  WHEN path LIKE '/mp/%' OR path LIKE '/social/%' THEN 'social:read'
  WHEN path = '/system/notify-templates' THEN 'notify:template:read'
  WHEN path = '/pay/index' THEN 'pay:read'
  ELSE permission
END, updated_at = NOW()
WHERE path IN (
  '/im/desk', '/im/sites', '/im/skills', '/im/stats', '/im/kb-misses', '/im/low-ratings', '/im/sensitive', '/ticket',
  '/crm/dashboard', '/crm/leads', '/crm/customers', '/crm/pipeline', '/crm/contracts', '/crm/pool-rules',
  '/cc/workbench', '/cc/calls', '/cc/console', '/cc/extensions', '/cc/recordings', '/cc/trunks', '/cc/ivr',
  '/cc/queues', '/cc/moh', '/cc/blacklist', '/cc/prompts', '/cc/conference', '/cc/monitor', '/cc/voicemail',
  '/cc/timeroutes', '/cc/ringgroups', '/cc/reports', '/cc/billing', '/cc/qc',
  '/visibility/overview', '/visibility/brands', '/visibility/seo', '/visibility/geo',
  '/mp/accounts', '/mp/articles', '/mp/messages', '/mp/fans', '/mp/menu', '/social/content', '/social/publish',
  '/system/notify-templates', '/pay/index'
);

DELETE FROM menu_permissions
WHERE menu_id = (SELECT id FROM menus WHERE path = '/pay/index' LIMIT 1)
  AND permission_id IN (SELECT id FROM permissions WHERE code = 'system:pay-order:list');

INSERT INTO menu_permissions (menu_id, permission_id)
SELECT m.id, p.id
FROM menus m
JOIN permissions p ON p.code = m.permission
WHERE m.path IN (
  '/im/desk', '/im/sites', '/im/skills', '/im/stats', '/im/kb-misses', '/im/low-ratings', '/im/sensitive', '/ticket',
  '/crm/dashboard', '/crm/leads', '/crm/customers', '/crm/pipeline', '/crm/contracts', '/crm/pool-rules',
  '/cc/workbench', '/cc/calls', '/cc/console', '/cc/extensions', '/cc/recordings', '/cc/trunks', '/cc/ivr',
  '/cc/queues', '/cc/moh', '/cc/blacklist', '/cc/prompts', '/cc/conference', '/cc/monitor', '/cc/voicemail',
  '/cc/timeroutes', '/cc/ringgroups', '/cc/reports', '/cc/billing', '/cc/qc',
  '/visibility/overview', '/visibility/brands', '/visibility/seo', '/visibility/geo',
  '/mp/accounts', '/mp/articles', '/mp/messages', '/mp/fans', '/mp/menu', '/social/content', '/social/publish',
  '/system/notify-templates', '/pay/index'
)
ON CONFLICT DO NOTHING;

-- +goose Down
-- 完整产品升级含数据归属变化，不允许以删表/删除权限模拟回滚；应恢复升级前备份。
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Full product upgrade is forward-only; restore the pre-upgrade backup';
END $$;
-- +goose StatementEnd
