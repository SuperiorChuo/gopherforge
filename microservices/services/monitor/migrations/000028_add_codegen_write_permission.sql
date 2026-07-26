-- +goose Up
INSERT INTO permissions (name, code, type, path, method, parent_id, created_at, updated_at) VALUES
('代码生成仓库写入', 'system:codegen:write', 2, '/api/v1/codegen/write', 'POST', 2, NOW(), NOW())
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT 1, id FROM permissions WHERE code = 'system:codegen:write'
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM role_permissions WHERE role_id = 1 AND permission_id IN
  (SELECT id FROM permissions WHERE code = 'system:codegen:write');
DELETE FROM permissions WHERE code = 'system:codegen:write';
