-- +goose Up
-- 权限诊断只读能力：独立权限码，默认授予平台超管；页面菜单由 system-service 种子补齐。
INSERT INTO permissions (name, code, description, type, path, method, parent_id, created_at, updated_at)
SELECT '权限诊断', 'system:permission:diagnose', '查看用户角色、权限、套餐与数据范围诊断链', 3,
       '/api/v1/permissions/diagnose', 'POST',
       COALESCE((SELECT id FROM permissions WHERE code = 'system:permission:list' LIMIT 1), 0), NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM permissions WHERE code = 'system:permission:diagnose');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code = 'system:permission:diagnose'
WHERE r.code = 'super_admin'
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp
    WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

-- +goose Down
DELETE FROM role_permissions
WHERE permission_id IN (
  SELECT id FROM permissions WHERE code = 'system:permission:diagnose'
);
DELETE FROM permissions WHERE code = 'system:permission:diagnose';
