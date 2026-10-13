-- +goose Up
-- Hermes skill 供应链验签：signature_status 记录技能签名核验状态。
--   verified        = 控制面已用配置的信任根验签通过
--   unverified      = 有信任根但未带签名（或签名无效被拒，不会落库）
--   not_applicable  = 官方市场导入（无企业签名，靠 repo@commit 来源锚定）
ALTER TABLE hermes_skills
  ADD COLUMN IF NOT EXISTS signature_status varchar(16) NOT NULL DEFAULT 'unverified',
  ADD CONSTRAINT chk_hermes_skill_signature_status
    CHECK (signature_status IN ('verified', 'unverified', 'not_applicable'));

-- +goose Down
-- 完整产品升级含数据归属变化，不允许以删表/删除权限模拟回滚；应恢复升级前备份。
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Full product upgrade is forward-only; restore the pre-upgrade backup';
END $$;
-- +goose StatementEnd
