-- +goose Up
ALTER TABLE `users`
  ADD COLUMN `password_changed_at` datetime(3) DEFAULT NULL AFTER `must_change_password`;

UPDATE `users`
SET `password_changed_at` = `created_at`
WHERE `password_changed_at` IS NULL
  AND `created_at` IS NOT NULL;

CREATE TABLE IF NOT EXISTS `password_history` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id` bigint unsigned NOT NULL,
  `password_hash` varchar(255) NOT NULL,
  `changed_at` datetime(3) NOT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_password_history_user_id` (`user_id`),
  KEY `idx_password_history_user_changed` (`user_id`,`changed_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS `password_history`;

ALTER TABLE `users`
  DROP COLUMN `password_changed_at`;
