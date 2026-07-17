-- +goose Up
-- Multi-tenant M3: tenant_id on file / audit logs / AI / IM domain tables.

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

-- ===== AI =====
ALTER TABLE ai_conversations ADD COLUMN IF NOT EXISTS tenant_id bigint NOT NULL DEFAULT 1;
UPDATE ai_conversations SET tenant_id = 1 WHERE tenant_id IS NULL OR tenant_id = 0;
CREATE INDEX IF NOT EXISTS idx_ai_conversations_tenant_id ON ai_conversations (tenant_id);
CREATE INDEX IF NOT EXISTS idx_ai_conversations_tenant_user ON ai_conversations (tenant_id, user_id);

ALTER TABLE ai_documents ADD COLUMN IF NOT EXISTS tenant_id bigint NOT NULL DEFAULT 1;
UPDATE ai_documents SET tenant_id = 1 WHERE tenant_id IS NULL OR tenant_id = 0;
CREATE INDEX IF NOT EXISTS idx_ai_documents_tenant_id ON ai_documents (tenant_id);

-- ===== IM =====
-- IM tables are owned by im-service AutoMigrate (experimental track): on a
-- fresh database they do not exist yet at migration time (AutoMigrate later
-- creates them WITH tenant_id). Only backfill when the tables are present.
-- +goose StatementBegin
DO $$
BEGIN
  IF to_regclass('im_sites') IS NOT NULL THEN
    ALTER TABLE im_sites ADD COLUMN IF NOT EXISTS tenant_id bigint NOT NULL DEFAULT 1;
    UPDATE im_sites SET tenant_id = 1 WHERE tenant_id IS NULL OR tenant_id = 0;
    CREATE INDEX IF NOT EXISTS idx_im_sites_tenant_id ON im_sites (tenant_id);
    -- app_key stays globally unique for widget public id; tenant scopes admin listing
  END IF;

  IF to_regclass('im_skill_groups') IS NOT NULL THEN
    ALTER TABLE im_skill_groups ADD COLUMN IF NOT EXISTS tenant_id bigint NOT NULL DEFAULT 1;
    UPDATE im_skill_groups SET tenant_id = 1 WHERE tenant_id IS NULL OR tenant_id = 0;
    DROP INDEX IF EXISTS idx_im_skill_groups_code;
    -- gorm uniqueIndex on code may be named differently
    DROP INDEX IF EXISTS uni_im_skill_groups_code;
    CREATE UNIQUE INDEX IF NOT EXISTS idx_im_skill_groups_tenant_code ON im_skill_groups (tenant_id, code);
    CREATE INDEX IF NOT EXISTS idx_im_skill_groups_tenant_id ON im_skill_groups (tenant_id);
  END IF;

  IF to_regclass('im_conversations') IS NOT NULL THEN
    ALTER TABLE im_conversations ADD COLUMN IF NOT EXISTS tenant_id bigint NOT NULL DEFAULT 1;
    UPDATE im_conversations SET tenant_id = 1 WHERE tenant_id IS NULL OR tenant_id = 0;
    CREATE INDEX IF NOT EXISTS idx_im_conversations_tenant_id ON im_conversations (tenant_id);
    CREATE INDEX IF NOT EXISTS idx_im_conversations_tenant_status ON im_conversations (tenant_id, status);

    -- Backfill IM conversation tenant from site when possible
    UPDATE im_conversations c
    SET tenant_id = s.tenant_id
    FROM im_sites s
    WHERE c.site_id = s.id AND s.tenant_id IS NOT NULL AND s.tenant_id > 0;
  END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF to_regclass('im_conversations') IS NOT NULL THEN
    DROP INDEX IF EXISTS idx_im_conversations_tenant_status;
    DROP INDEX IF EXISTS idx_im_conversations_tenant_id;
    ALTER TABLE im_conversations DROP COLUMN IF EXISTS tenant_id;
  END IF;

  IF to_regclass('im_skill_groups') IS NOT NULL THEN
    DROP INDEX IF EXISTS idx_im_skill_groups_tenant_id;
    DROP INDEX IF EXISTS idx_im_skill_groups_tenant_code;
    ALTER TABLE im_skill_groups DROP COLUMN IF EXISTS tenant_id;
    CREATE UNIQUE INDEX IF NOT EXISTS idx_im_skill_groups_code ON im_skill_groups (code);
  END IF;

  IF to_regclass('im_sites') IS NOT NULL THEN
    DROP INDEX IF EXISTS idx_im_sites_tenant_id;
    ALTER TABLE im_sites DROP COLUMN IF EXISTS tenant_id;
  END IF;
END $$;
-- +goose StatementEnd

DROP INDEX IF EXISTS idx_ai_documents_tenant_id;
ALTER TABLE ai_documents DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS idx_ai_conversations_tenant_user;
DROP INDEX IF EXISTS idx_ai_conversations_tenant_id;
ALTER TABLE ai_conversations DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS idx_audit_logs_tenant_id;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS idx_operation_logs_tenant_id;
ALTER TABLE operation_logs DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS idx_login_logs_tenant_id;
ALTER TABLE login_logs DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS idx_files_tenant_user;
DROP INDEX IF EXISTS idx_files_tenant_id;
ALTER TABLE files DROP COLUMN IF EXISTS tenant_id;
