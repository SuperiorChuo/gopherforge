-- +goose Up
INSERT IGNORE INTO `permissions` (`id`,`name`,`code`,`type`,`path`,`method`,`parent_id`,`created_at`,`updated_at`) VALUES
(53,'系统设置读取','system:setting:list',2,'/api/v1/system-settings','GET',2,NOW(3),NOW(3)),
(54,'系统设置写入','system:setting:update',2,'/api/v1/system-settings/:key','PUT',2,NOW(3),NOW(3)),
(55,'系统设置删除','system:setting:delete',2,'/api/v1/system-settings/:key','DELETE',2,NOW(3),NOW(3));

INSERT IGNORE INTO `menus` (`id`,`name`,`title`,`icon`,`path`,`component`,`parent_id`,`sort`,`status`,`hidden`,`permission`,`created_at`,`updated_at`) VALUES
(22,'setting','系统设置','setting','/system/setting','system/setting/index',10,12,1,0,'system:setting:list',NOW(3),NOW(3));

INSERT IGNORE INTO `role_permissions` (`role_id`,`permission_id`)
SELECT 1, `id` FROM `permissions` WHERE `code` IN ('system:setting:list','system:setting:update','system:setting:delete');

INSERT IGNORE INTO `role_permissions` (`role_id`,`permission_id`)
SELECT 2, `id` FROM `permissions` WHERE `code` IN ('system:setting:list');

INSERT IGNORE INTO `menu_permissions` (`menu_id`,`permission_id`)
SELECT m.`id`, p.`id`
FROM `menus` m
JOIN `permissions` p ON p.`code` = m.`permission`
WHERE m.`name` = 'setting';

-- +goose Down
DELETE mp FROM `menu_permissions` mp
JOIN `menus` m ON m.`id` = mp.`menu_id`
WHERE m.`name` = 'setting';

DELETE FROM `menus` WHERE `id` = 22 AND `name` = 'setting';
DELETE FROM `role_permissions` WHERE `permission_id` IN (SELECT `id` FROM `permissions` WHERE `code` IN ('system:setting:list','system:setting:update','system:setting:delete'));
DELETE FROM `permissions` WHERE `code` IN ('system:setting:list','system:setting:update','system:setting:delete');
