package system

var tplPermissionMigration = mustTpl("permission-migration", `-- +goose Up
INSERT INTO permissions (name, code, type, path, method, parent_id, created_at, updated_at) VALUES
('{{sqlText .Title}}查看', 'system:{{.Module}}:list', 2, '/api/v1/{{.Module}}', 'GET', 2, NOW(), NOW()),
('{{sqlText .Title}}新建', 'system:{{.Module}}:create', 2, '/api/v1/{{.Module}}', 'POST', 2, NOW(), NOW()),
('{{sqlText .Title}}更新', 'system:{{.Module}}:update', 2, '/api/v1/{{.Module}}/:id', 'PUT', 2, NOW(), NOW()),
('{{sqlText .Title}}删除', 'system:{{.Module}}:delete', 2, '/api/v1/{{.Module}}/:id', 'DELETE', 2, NOW(), NOW())
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT 1, id FROM permissions WHERE code IN (
  'system:{{.Module}}:list', 'system:{{.Module}}:create',
  'system:{{.Module}}:update', 'system:{{.Module}}:delete'
)
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM role_permissions WHERE role_id = 1 AND permission_id IN (
  SELECT id FROM permissions WHERE code IN (
    'system:{{.Module}}:list', 'system:{{.Module}}:create',
    'system:{{.Module}}:update', 'system:{{.Module}}:delete'
  )
);
DELETE FROM permissions WHERE code IN (
  'system:{{.Module}}:list', 'system:{{.Module}}:create',
  'system:{{.Module}}:update', 'system:{{.Module}}:delete'
);
`)
