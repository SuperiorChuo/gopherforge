-- +goose Up
INSERT INTO permissions (id,name,code,type,path,method,parent_id,created_at,updated_at) VALUES
(53,'系统设置读取','system:setting:list',2,'/api/v1/system-settings','GET',2,NOW(),NOW()),
(54,'系统设置写入','system:setting:update',2,'/api/v1/system-settings/:key','PUT',2,NOW(),NOW()),
(55,'系统设置删除','system:setting:delete',2,'/api/v1/system-settings/:key','DELETE',2,NOW(),NOW())
ON CONFLICT DO NOTHING;

INSERT INTO menus (id,name,title,icon,path,component,parent_id,sort,status,hidden,permission,created_at,updated_at) VALUES
(22,'setting','系统设置','setting','/system/setting','system/setting/index',10,12,1,0,'system:setting:list',NOW(),NOW())
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id,permission_id)
SELECT 1, id FROM permissions WHERE code IN ('system:setting:list','system:setting:update','system:setting:delete')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id,permission_id)
SELECT 2, id FROM permissions WHERE code IN ('system:setting:list')
ON CONFLICT DO NOTHING;

INSERT INTO menu_permissions (menu_id,permission_id)
SELECT m.id, p.id
FROM menus m
JOIN permissions p ON p.code = m.permission
WHERE m.name = 'setting'
ON CONFLICT DO NOTHING;

-- Explicit ids were inserted into identity columns; realign the sequences.
SELECT setval(pg_get_serial_sequence('permissions','id'), (SELECT COALESCE(MAX(id),1) FROM permissions));
SELECT setval(pg_get_serial_sequence('menus','id'), (SELECT COALESCE(MAX(id),1) FROM menus));

-- +goose Down
DELETE FROM menu_permissions mp
USING menus m
WHERE m.id = mp.menu_id
  AND m.name = 'setting';

DELETE FROM menus WHERE id = 22 AND name = 'setting';
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE code IN ('system:setting:list','system:setting:update','system:setting:delete'));
DELETE FROM permissions WHERE code IN ('system:setting:list','system:setting:update','system:setting:delete');
