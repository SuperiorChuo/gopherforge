-- +goose Up
-- 升级路径中 crm-service 已在 public AutoMigrate CRM 主表，本迁移补客户标签并由
-- 000073 搬入 crm_svc。fresh install 先跑 goose、后启动业务服务，此时没有
-- crm_svc.crm_customers；应跳过，交 crm-service 直接在 crm_svc AutoMigrate。
-- +goose StatementBegin
DO $$
BEGIN
    IF to_regclass('crm_svc.crm_customers') IS NULL THEN
        RETURN;
    END IF;

    CREATE TABLE IF NOT EXISTS crm_svc.crm_tags (
        id BIGSERIAL PRIMARY KEY,
        tenant_id BIGINT NOT NULL DEFAULT 1,
        name VARCHAR(32) NOT NULL,
        color VARCHAR(7) NOT NULL DEFAULT '#1677ff',
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        UNIQUE(tenant_id, name)
    );

    CREATE TABLE IF NOT EXISTS crm_svc.crm_customer_tags (
        customer_id BIGINT NOT NULL REFERENCES crm_svc.crm_customers(id) ON DELETE CASCADE,
        tag_id BIGINT NOT NULL REFERENCES crm_svc.crm_tags(id) ON DELETE CASCADE,
        PRIMARY KEY (customer_id, tag_id)
    );

    CREATE INDEX IF NOT EXISTS idx_crm_customer_tags_tag ON crm_svc.crm_customer_tags(tag_id);
END $$;
-- +goose StatementEnd

-- +goose Down
-- 完整产品升级含数据归属变化，不允许以删表/删除权限模拟回滚；应恢复升级前备份。
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Full product upgrade is forward-only; restore the pre-upgrade backup';
END $$;
-- +goose StatementEnd
