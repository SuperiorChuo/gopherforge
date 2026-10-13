-- +goose Up
-- AI 模型、用量和配额是平台级管理面。此前菜单和接口只要求登录，任意账号都能
-- 读取全局配置或改写配额；改为动作级权限，并只默认授予 super_admin。

INSERT INTO permissions (name, code, type, path, method, parent_id, created_at, updated_at) VALUES
('AI 模型查看', 'ai:model:list', 2, '/api/v1/ai/models', 'GET', 0, NOW(), NOW()),
('AI 模型新建', 'ai:model:create', 2, '/api/v1/ai/models', 'POST', 0, NOW(), NOW()),
('AI 模型修改', 'ai:model:update', 2, '/api/v1/ai/models/:id', 'PUT', 0, NOW(), NOW()),
('AI 模型删除', 'ai:model:delete', 2, '/api/v1/ai/models/:id', 'DELETE', 0, NOW(), NOW()),
('AI 用量查看', 'ai:usage:list', 2, '/api/v1/ai/usage', 'GET', 0, NOW(), NOW()),
('AI 配额查看', 'ai:quota:list', 2, '/api/v1/ai/quotas', 'GET', 0, NOW(), NOW()),
('AI 配额修改', 'ai:quota:update', 2, '/api/v1/ai/quotas', 'POST', 0, NOW(), NOW()),
('AI 配额删除', 'ai:quota:delete', 2, '/api/v1/ai/quotas/:id', 'DELETE', 0, NOW(), NOW())
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.code = 'super_admin'
  AND (p.code LIKE 'ai:model:%' OR p.code LIKE 'ai:usage:%' OR p.code LIKE 'ai:quota:%')
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

UPDATE menus SET permission = 'ai:model:list', updated_at = NOW()
WHERE path = '/system/ai-models';
UPDATE menus SET permission = 'ai:usage:list', updated_at = NOW()
WHERE path = '/system/ai-usage';
UPDATE menus SET permission = 'ai:quota:list', updated_at = NOW()
WHERE path = '/system/ai-quotas';

INSERT INTO menu_permissions (menu_id, permission_id)
SELECT m.id, p.id
FROM menus m
JOIN permissions p ON p.code = m.permission
WHERE m.path IN ('/system/ai-models', '/system/ai-usage', '/system/ai-quotas')
ON CONFLICT DO NOTHING;

-- +goose Down
-- 完整产品升级含数据归属变化，不允许以删表/删除权限模拟回滚；应恢复升级前备份。
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Full product upgrade is forward-only; restore the pre-upgrade backup';
END $$;
-- +goose StatementEnd
