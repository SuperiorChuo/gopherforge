-- +goose Up
ALTER TABLE `files`
  ADD COLUMN `image_width` int NOT NULL DEFAULT 0 AFTER `file_size`,
  ADD COLUMN `image_height` int NOT NULL DEFAULT 0 AFTER `image_width`;

-- +goose Down
ALTER TABLE `files`
  DROP COLUMN `image_height`,
  DROP COLUMN `image_width`;
