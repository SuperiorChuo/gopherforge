-- +goose Up
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
-- 完整产品升级含数据归属变化，不允许以删表/删除权限模拟回滚；应恢复升级前备份。
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Full product upgrade is forward-only; restore the pre-upgrade backup';
END $$;
-- +goose StatementEnd
