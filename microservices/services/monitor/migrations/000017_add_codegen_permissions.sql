-- +goose Up
-- 代码生成器权限点：表/列查看 + 生成下载。超级管理员自动获得全部权限。
INSERT INTO permissions (name, code, type, path, method, parent_id, created_at, updated_at) VALUES
('代码生成查看', 'system:codegen:list', 2, '/api/v1/codegen/tables', 'GET', 2, NOW(), NOW()),
('代码生成执行', 'system:codegen:generate', 2, '/api/v1/codegen/preview', 'POST', 2, NOW(), NOW())
ON CONFLICT DO NOTHING;

-- 超管角色补挂新权限（与 000001 的全量挂载语义一致）
INSERT INTO role_permissions (role_id, permission_id)
SELECT 1, id FROM permissions WHERE code IN ('system:codegen:list', 'system:codegen:generate')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM role_permissions WHERE permission_id IN
  (SELECT id FROM permissions WHERE code IN ('system:codegen:list', 'system:codegen:generate'));
DELETE FROM permissions WHERE code IN ('system:codegen:list', 'system:codegen:generate');
