-- +goose Up
INSERT IGNORE INTO `permissions` (`id`,`name`,`code`,`type`,`path`,`method`,`parent_id`,`created_at`,`updated_at`) VALUES
(56,'操作日志清理','system:log:operation:clear',2,'/api/v1/operation-logs/clear','DELETE',24,NOW(3),NOW(3));

INSERT IGNORE INTO `role_permissions` (`role_id`,`permission_id`)
SELECT 1, `id` FROM `permissions` WHERE `code` = 'system:log:operation:clear';

DELETE rp FROM `role_permissions` rp
JOIN `permissions` p ON p.`id` = rp.`permission_id`
WHERE rp.`role_id` = 2
  AND p.`code` IN ('system:setting:update','system:setting:delete','system:log:operation:clear');

-- +goose Down
DELETE rp FROM `role_permissions` rp
JOIN `permissions` p ON p.`id` = rp.`permission_id`
WHERE p.`code` = 'system:log:operation:clear';

DELETE FROM `permissions` WHERE `code` = 'system:log:operation:clear';

INSERT IGNORE INTO `role_permissions` (`role_id`,`permission_id`)
SELECT 2, `id` FROM `permissions` WHERE `code` IN ('system:setting:update','system:setting:delete');
