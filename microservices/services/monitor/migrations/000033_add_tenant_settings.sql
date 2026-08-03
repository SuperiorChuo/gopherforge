-- +goose Up
-- 租户级配置覆盖表：system_settings 保持平台默认，租户上下文请求先查
-- tenant_settings、回落平台默认。可配键白名单见 system-service
-- tenantConfigurableKeys（ai.provider / notification.email / weather.provider）。
CREATE TABLE IF NOT EXISTS tenant_settings (
  tenant_id   bigint NOT NULL,
  setting_key varchar(128) NOT NULL,
  value_json  jsonb DEFAULT NULL,
  updated_at  timestamptz(3) DEFAULT NULL,
  PRIMARY KEY (tenant_id, setting_key)
);
CREATE INDEX IF NOT EXISTS idx_tenant_settings_updated_at ON tenant_settings (updated_at);

-- +goose Down
DROP TABLE IF EXISTS tenant_settings;
