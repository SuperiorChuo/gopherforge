-- +goose Up
ALTER TABLE permissions
  ADD COLUMN description varchar(255) NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE permissions
  DROP COLUMN description;
