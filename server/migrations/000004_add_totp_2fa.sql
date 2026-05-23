-- +goose Up
ALTER TABLE `users`
  ADD COLUMN `totp_secret` varchar(255) NOT NULL DEFAULT '' AFTER `password_changed_at`,
  ADD COLUMN `totp_enabled` tinyint(1) NOT NULL DEFAULT 0 AFTER `totp_secret`;

CREATE TABLE IF NOT EXISTS `totp_recovery_codes` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id` bigint unsigned NOT NULL,
  `code_hash` varchar(255) NOT NULL,
  `used_at` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_totp_recovery_codes_user_id` (`user_id`),
  KEY `idx_totp_recovery_codes_user_unused` (`user_id`,`used_at`),
  CONSTRAINT `fk_totp_recovery_codes_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS `totp_recovery_codes`;

ALTER TABLE `users`
  DROP COLUMN `totp_enabled`,
  DROP COLUMN `totp_secret`;
