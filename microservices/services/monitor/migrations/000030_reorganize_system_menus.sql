-- +goose Up
-- 系统管理菜单拆组：原 19 个子菜单全堆在 /system 下，拆为 4 个一级分组
--   系统管理（组织+权限）/ 消息中心(/msg) / 日志审计(/logs) / 系统工具(/tools)。
-- 子菜单 path/component/id 全部不变（保住既有关联与书签），只 UPDATE parent_id 与 sort。
-- 新分组容器沿用主项目的显式 id 134-136，与 menu_seed.go 对齐：seed 按 id 去重、
-- 本处按 path 去重。42 起的 identity id 已被租户/OAuth2 等迁移占用，不能复用。

INSERT INTO menus (id, name, title, icon, path, component, parent_id, sort, status, hidden, permission, created_at, updated_at)
SELECT v.id, v.name, v.title, v.icon, v.path, 'Layout', 0, v.sort, 1, 0, '', NOW(), NOW()
FROM (VALUES
  (135, 'msg-center', '消息中心', 'notification', '/msg',   2),
  (134, 'log-audit',  '日志审计', 'file-text',    '/logs',  3),
  (136, 'sys-tools',  '系统工具', 'tool',         '/tools', 4)
) AS v(id, name, title, icon, path, sort)
WHERE NOT EXISTS (SELECT 1 FROM menus m WHERE m.path = v.path);

-- 显式 id 不会自动推进 identity sequence；避免后续自动菜单最终撞到 134-136。
SELECT setval(pg_get_serial_sequence('menus','id'), (SELECT COALESCE(MAX(id),1) FROM menus));

-- 消息中心：通知公告 / 短信
UPDATE menus SET
  parent_id = (SELECT id FROM menus WHERE path = '/msg' AND parent_id = 0 LIMIT 1),
  sort = v.sort,
  updated_at = NOW()
FROM (VALUES
  ('/system/notice', 1),
  ('/system/sms', 2)
) AS v(path, sort)
WHERE menus.path = v.path;

-- 日志审计：三种日志 + 在线用户
UPDATE menus SET
  parent_id = (SELECT id FROM menus WHERE path = '/logs' AND parent_id = 0 LIMIT 1),
  sort = v.sort,
  updated_at = NOW()
FROM (VALUES
  ('/system/operation-log', 1),
  ('/system/login-log', 2),
  ('/system/audit-log', 3),
  ('/system/online-user', 4)
) AS v(path, sort)
WHERE menus.path = v.path;

-- 系统工具：代码生成 / 字典 / 文件 / 错误码 / OAuth2（OAuth2 由 000024 自增插入，按 path 定位）
UPDATE menus SET
  parent_id = (SELECT id FROM menus WHERE path = '/tools' AND parent_id = 0 LIMIT 1),
  sort = v.sort,
  updated_at = NOW()
FROM (VALUES
  ('/system/codegen', 1),
  ('/system/dict', 2),
  ('/system/file', 3),
  ('/system/errcodes', 4),
  ('/system/oauth2', 5)
) AS v(path, sort)
WHERE menus.path = v.path;

-- 系统管理留下的 9 项重排
UPDATE menus SET sort = v.sort, updated_at = NOW()
FROM (VALUES
  ('/system/user', 1),
  ('/system/role', 2),
  ('/system/permission', 3),
  ('/system/menu', 4),
  ('/system/department', 5),
  ('/system/post', 6),
  ('/system/tenant', 7),
  ('/system/tenant-packages', 8),
  ('/system/setting', 9)
) AS v(path, sort)
WHERE menus.path = v.path;

-- 顶级分组重排（给新分组腾出 2-4）
UPDATE menus SET sort = v.sort, updated_at = NOW()
FROM (VALUES
  ('/monitor', 5),
  ('/bpm', 6)
) AS v(path, sort)
WHERE menus.path = v.path AND menus.parent_id = 0;

-- +goose Down
-- 全部挂回 /system 并恢复拆组前的 sort（oauth2 恢复 000024 的 96）
UPDATE menus SET
  parent_id = (SELECT id FROM menus WHERE path = '/system' AND parent_id = 0 LIMIT 1),
  sort = v.sort,
  updated_at = NOW()
FROM (VALUES
  ('/system/file', 6),
  ('/system/dict', 7),
  ('/system/notice', 8),
  ('/system/online-user', 9),
  ('/system/operation-log', 10),
  ('/system/login-log', 11),
  ('/system/audit-log', 12),
  ('/system/setting', 13),
  ('/system/tenant', 14),
  ('/system/codegen', 15),
  ('/system/sms', 16),
  ('/system/errcodes', 17),
  ('/system/post', 18),
  ('/system/tenant-packages', 19),
  ('/system/oauth2', 96)
) AS v(path, sort)
WHERE menus.path = v.path;

UPDATE menus SET sort = v.sort, updated_at = NOW()
FROM (VALUES
  ('/system/user', 1),
  ('/system/role', 2),
  ('/system/permission', 3),
  ('/system/menu', 4),
  ('/system/department', 5)
) AS v(path, sort)
WHERE menus.path = v.path;

UPDATE menus SET sort = v.sort, updated_at = NOW()
FROM (VALUES
  ('/monitor', 2),
  ('/bpm', 3)
) AS v(path, sort)
WHERE menus.path = v.path AND menus.parent_id = 0;

DELETE FROM menus WHERE path IN ('/msg', '/logs', '/tools') AND parent_id = 0;
