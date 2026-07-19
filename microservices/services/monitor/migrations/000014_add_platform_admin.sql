-- +goose Up
-- Multi-tenant M4: platform operator flag on users.

ALTER TABLE users ADD COLUMN IF NOT EXISTS is_platform_admin boolean NOT NULL DEFAULT false;

-- Promote default admin (if present) to platform operator.
UPDATE users
SET is_platform_admin = true
WHERE tenant_id = 1
  AND username = 'admin'
  AND is_platform_admin = false;

-- Optional menu already has tenant management; ensure platform role can still use system:tenant:*
-- (super_admin already has those permissions from 000012)

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS is_platform_admin;
