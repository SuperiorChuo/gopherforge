-- +goose Up
ALTER TABLE `permissions`
  ADD COLUMN `description` varchar(255) NOT NULL DEFAULT '' AFTER `code`;

-- +goose Down
ALTER TABLE `permissions`
  DROP COLUMN `description`;
