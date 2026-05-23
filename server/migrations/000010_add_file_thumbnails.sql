-- +goose Up
ALTER TABLE `files`
  ADD COLUMN `thumbnail_path` varchar(500) DEFAULT '' AFTER `image_height`,
  ADD COLUMN `thumbnail_url` varchar(500) DEFAULT '' AFTER `thumbnail_path`,
  ADD COLUMN `thumbnail_width` int NOT NULL DEFAULT 0 AFTER `thumbnail_url`,
  ADD COLUMN `thumbnail_height` int NOT NULL DEFAULT 0 AFTER `thumbnail_width`;

-- +goose Down
ALTER TABLE `files`
  DROP COLUMN `thumbnail_height`,
  DROP COLUMN `thumbnail_width`,
  DROP COLUMN `thumbnail_url`,
  DROP COLUMN `thumbnail_path`;
