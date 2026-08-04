-- +goose Up
-- 租户删除权限码（000012 只 seed 了 list/detail/create/update，delete 是本次新增）。

INSERT INTO permissions (name, code, type, path, method, parent_id, created_at, updated_at)
SELECT '删除租户', 'system:tenant:delete', 2, '/api/v1/tenants/:id', 'DELETE', 0, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM permissions WHERE code = 'system:tenant:delete');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.code = 'super_admin'
  AND p.code = 'system:tenant:delete'
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

-- +goose Down
DELETE FROM role_permissions WHERE permission_id IN (
  SELECT id FROM permissions WHERE code = 'system:tenant:delete'
);
DELETE FROM permissions WHERE code = 'system:tenant:delete';
