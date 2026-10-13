-- +goose Up
-- Hermes 第三期：manager 下发通道（Gateway API server）。
--
-- 设备端自持 Gateway API server 密钥（长期，manager 本机生成，原始 key 只在
-- 端侧 .env 与 manager.json），控制面只见 sha256(key) 前 16 字节指纹（32 hex），
-- 随心跳上报并在此落库，供控制台显示「API server 已启用」。
--
-- 该列仅作可观测性：指纹不参与鉴权，缺失/清空不影响任务下发。
ALTER TABLE hermes_devices
  ADD COLUMN IF NOT EXISTS api_server_fingerprint varchar(64) NOT NULL DEFAULT '';

-- +goose Down
-- 完整产品升级含数据归属变化，不允许以删表/删除权限模拟回滚；应恢复升级前备份。
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Full product upgrade is forward-only; restore the pre-upgrade backup';
END $$;
-- +goose StatementEnd
