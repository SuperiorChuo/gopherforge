-- +goose Up
-- Hermes 技能市场打通官方 Skills Hub：hermes_skills 加导入来源 provenance 三列。
-- 从官方市场（GitHub 系源）导入的技能钉住 repo/path/commit（镜像 artifact 的
-- upstream_commit 哲学），手工上传的技能三列为空串。为将来升级检测铺路。
ALTER TABLE hermes_skills
  ADD COLUMN IF NOT EXISTS source_repo varchar(200) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS source_path varchar(300) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS source_commit varchar(64) NOT NULL DEFAULT '';

-- +goose Down
-- 完整产品升级含数据归属变化，不允许以删表/删除权限模拟回滚；应恢复升级前备份。
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Full product upgrade is forward-only; restore the pre-upgrade backup';
END $$;
-- +goose StatementEnd
