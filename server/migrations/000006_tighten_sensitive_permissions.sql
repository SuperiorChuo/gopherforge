-- +goose Up
INSERT INTO permissions (id,name,code,type,path,method,parent_id,created_at,updated_at) VALUES
(56,'操作日志清理','system:log:operation:clear',2,'/api/v1/operation-logs/clear','DELETE',24,NOW(),NOW())
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id,permission_id)
SELECT 1, id FROM permissions WHERE code = 'system:log:operation:clear'
ON CONFLICT DO NOTHING;

DELETE FROM role_permissions rp
USING permissions p
WHERE p.id = rp.permission_id
  AND rp.role_id = 2
  AND p.code IN ('system:setting:update','system:setting:delete','system:log:operation:clear');

-- Explicit id was inserted into an identity column; realign the sequence.
SELECT setval(pg_get_serial_sequence('permissions','id'), (SELECT COALESCE(MAX(id),1) FROM permissions));

-- +goose Down
DELETE FROM role_permissions rp
USING permissions p
WHERE p.id = rp.permission_id
  AND p.code = 'system:log:operation:clear';

DELETE FROM permissions WHERE code = 'system:log:operation:clear';

INSERT INTO role_permissions (role_id,permission_id)
SELECT 2, id FROM permissions WHERE code IN ('system:setting:update','system:setting:delete')
ON CONFLICT DO NOTHING;
