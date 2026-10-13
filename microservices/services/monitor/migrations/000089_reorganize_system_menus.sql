-- +goose Up
-- AI 三件套挪入「AI 智能」（/ai 容器），排在 AI 助手(1)/知识库(2) 之后
UPDATE menus SET
  parent_id = (SELECT id FROM menus WHERE path = '/ai' AND parent_id = 0 LIMIT 1),
  sort = v.sort,
  updated_at = NOW()
FROM (VALUES
  ('/system/ai-models', 3),
  ('/system/ai-usage', 4),
  ('/system/ai-quotas', 5)
) AS v(path, sort)
WHERE menus.path = v.path;

-- +goose Down
-- 完整产品升级含数据归属变化，不允许以删表/删除权限模拟回滚；应恢复升级前备份。
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Full product upgrade is forward-only; restore the pre-upgrade backup';
END $$;
-- +goose StatementEnd
