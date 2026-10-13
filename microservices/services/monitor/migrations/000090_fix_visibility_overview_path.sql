-- +goose Up
-- visibility 概览子菜单与其容器同路径（都是 /visibility），导致 antd Menu 的
-- 父 key 与子 key 撞车：重复 key 警告 + 选中态在父子间跳。
-- 改子项为 /visibility/overview，与 dashboard(/dashboard + /dashboard/index)、
-- pay(/pay + /pay/index) 的既有约定一致；容器 92 保持 /visibility 不动。
--
-- 定位条件不能只用 path：容器与子项此刻同为 '/visibility'，必须靠
-- component <> 'Layout' 把容器排除掉。已改过则 0 行受影响，可重复执行。
UPDATE menus
SET path = '/visibility/overview', updated_at = NOW()
WHERE path = '/visibility'
  AND component = 'visibility/overview/index'
  AND parent_id <> 0;

-- +goose Down
-- 完整产品升级含数据归属变化，不允许以删表/删除权限模拟回滚；应恢复升级前备份。
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Full product upgrade is forward-only; restore the pre-upgrade backup';
END $$;
-- +goose StatementEnd
