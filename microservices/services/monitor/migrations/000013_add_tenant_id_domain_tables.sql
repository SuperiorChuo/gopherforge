-- +goose Up
-- Multi-tenant M3: tenant_id on file / audit log tables.

-- ===== files =====
ALTER TABLE files ADD COLUMN IF NOT EXISTS tenant_id bigint NOT NULL DEFAULT 1;
UPDATE files SET tenant_id = 1 WHERE tenant_id IS NULL OR tenant_id = 0;
CREATE INDEX IF NOT EXISTS idx_files_tenant_id ON files (tenant_id);
CREATE INDEX IF NOT EXISTS idx_files_tenant_user ON files (tenant_id, user_id);

-- ===== audit / ops logs =====
ALTER TABLE login_logs ADD COLUMN IF NOT EXISTS tenant_id bigint NOT NULL DEFAULT 1;
UPDATE login_logs SET tenant_id = 1 WHERE tenant_id IS NULL OR tenant_id = 0;
CREATE INDEX IF NOT EXISTS idx_login_logs_tenant_id ON login_logs (tenant_id);

ALTER TABLE operation_logs ADD COLUMN IF NOT EXISTS tenant_id bigint NOT NULL DEFAULT 1;
UPDATE operation_logs SET tenant_id = 1 WHERE tenant_id IS NULL OR tenant_id = 0;
CREATE INDEX IF NOT EXISTS idx_operation_logs_tenant_id ON operation_logs (tenant_id);

ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS tenant_id bigint NOT NULL DEFAULT 1;
UPDATE audit_logs SET tenant_id = 1 WHERE tenant_id IS NULL OR tenant_id = 0;
CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_id ON audit_logs (tenant_id);

-- +goose Down
DROP INDEX IF EXISTS idx_audit_logs_tenant_id;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS idx_operation_logs_tenant_id;
ALTER TABLE operation_logs DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS idx_login_logs_tenant_id;
ALTER TABLE login_logs DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS idx_files_tenant_user;
DROP INDEX IF EXISTS idx_files_tenant_id;
ALTER TABLE files DROP COLUMN IF EXISTS tenant_id;
