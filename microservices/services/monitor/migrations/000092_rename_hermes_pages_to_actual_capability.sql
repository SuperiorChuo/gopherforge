-- +goose Up
-- 000038 给四个新页面起了参考产品（OpenClaw 式）的集群词汇名，但它们的数据源
-- 都是 hermes_devices 的换名投影：/hermes/tasks 读 pending_action 单列、
-- /hermes/runtimes 与 /hermes/ai-ops 读设备列表、/hermes/skills 是 P1 工具白名单
-- 的前端常量。名字承诺了多 Agent 协作能力，实际只有端点治理，属对用户虚报。
--
-- 本迁移只改标题，不改 path：路径已被书签、菜单权限守卫与前端路由引用，
-- 改动收益低而破坏面大。多 Agent 协作真做出来时另立新页面与新路径。
--
-- 按 path 定位（title 可能已被管理员改过，用 path 更稳）；已改过则 0 行受影响。
UPDATE menus SET title = '设备动作队列', updated_at = NOW()
WHERE path = '/hermes/tasks' AND title = '互联任务';

UPDATE menus SET title = '模型用量', updated_at = NOW()
WHERE path = '/hermes/ai-ops' AND title = 'AI 运维';

UPDATE menus SET title = '运行节点', updated_at = NOW()
WHERE path = '/hermes/runtimes' AND title = '运行实例';

UPDATE menus SET title = '工具白名单', updated_at = NOW()
WHERE path = '/hermes/skills' AND title = '技能市场';

-- +goose Down
-- 完整产品升级含数据归属变化，不允许以删表/删除权限模拟回滚；应恢复升级前备份。
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Full product upgrade is forward-only; restore the pre-upgrade backup';
END $$;
-- +goose StatementEnd
