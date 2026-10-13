-- +goose Up
-- Hermes 集群按「总览 / 协同运维 / 资源中心」重组。
--
-- 参考产品里的“容器管理”在 Hermes 中落为“运行实例”：Hermes 运行在员工
-- 电脑上，由 manager 管理进程，不在服务端为每名员工创建容器。

-- 1. 三个纯展示分组。
INSERT INTO menus (name, title, icon, path, component, parent_id, sort, status, hidden, permission, created_at, updated_at)
SELECT v.name, v.title, v.icon, v.path, 'Layout',
  COALESCE((SELECT id FROM menus WHERE path = '/hermes' LIMIT 1), 0),
  v.sort, 1, 0, '', NOW(), NOW()
FROM (VALUES
  ('hermes-general',       '总览',     'dashboard',   '/hermes/general',       1),
  ('hermes-collaboration', '协同运维', 'thunderbolt', '/hermes/collaboration', 2),
  ('hermes-resources',     '资源中心', 'appstore',    '/hermes/resources',     3)
) AS v(name, title, icon, path, sort)
WHERE NOT EXISTS (SELECT 1 FROM menus m WHERE m.path = v.path);

-- 2. 已有页面改名、改路径并移入对应分组，保留 menu id 与既有角色关联。
UPDATE menus SET title = '仪表盘', icon = 'dashboard', path = '/hermes/overview',
  component = 'hermes/overview/index',
  parent_id = COALESCE((SELECT id FROM menus WHERE path = '/hermes/general' LIMIT 1), 0),
  sort = 1, updated_at = NOW()
WHERE path = '/hermes/overview';

UPDATE menus SET title = '办公室', icon = 'global', path = '/hermes/office',
  component = 'hermes/office/index',
  parent_id = COALESCE((SELECT id FROM menus WHERE path = '/hermes/general' LIMIT 1), 0),
  sort = 2, updated_at = NOW()
WHERE path = '/hermes/office';

UPDATE menus SET title = '工作台', icon = 'robot', path = '/hermes/workbench',
  component = 'hermes/mine/index',
  parent_id = COALESCE((SELECT id FROM menus WHERE path = '/hermes/general' LIMIT 1), 0),
  sort = 3, updated_at = NOW()
WHERE path = '/hermes/mine';

UPDATE menus SET title = '互联运维', icon = 'secured', path = '/hermes/operations',
  component = 'hermes/devices/index',
  parent_id = COALESCE((SELECT id FROM menus WHERE path = '/hermes/collaboration' LIMIT 1), 0),
  sort = 2, updated_at = NOW()
WHERE path = '/hermes/devices';

UPDATE menus SET title = '实例管理', icon = 'desktop', path = '/hermes/instances',
  component = 'hermes/entitlements/index',
  parent_id = COALESCE((SELECT id FROM menus WHERE path = '/hermes/resources' LIMIT 1), 0),
  sort = 1, updated_at = NOW()
WHERE path = '/hermes/entitlements';

UPDATE menus SET title = '实例模型', icon = 'control', path = '/hermes/models',
  component = 'hermes/policies/index',
  parent_id = COALESCE((SELECT id FROM menus WHERE path = '/hermes/resources' LIMIT 1), 0),
  sort = 2, updated_at = NOW()
WHERE path = '/hermes/policies';

UPDATE menus SET title = '桥接组件', icon = 'gateway', path = '/hermes/bridges',
  component = 'hermes/artifacts/index',
  parent_id = COALESCE((SELECT id FROM menus WHERE path = '/hermes/resources' LIMIT 1), 0),
  sort = 3, updated_at = NOW()
WHERE path = '/hermes/artifacts';

-- 3. 新增四个基于现有真实 API 的操作面。
INSERT INTO menus (name, title, icon, path, component, parent_id, sort, status, hidden, permission, created_at, updated_at)
SELECT v.name, v.title, v.icon, v.path, v.component,
  COALESCE((SELECT id FROM menus WHERE path = v.parent_path LIMIT 1), 0),
  v.sort, 1, 0, v.permission, NOW(), NOW()
FROM (VALUES
  ('hermes-tasks',    '互联任务', 'fork',    '/hermes/tasks',    'hermes/tasks/index',    '/hermes/collaboration', 1, 'ai:hermes:device:list'),
  ('hermes-ai-ops',   'AI 运维', 'robot',   '/hermes/ai-ops',   'hermes/ai-ops/index',   '/hermes/collaboration', 3, 'ai:hermes:device:list'),
  ('hermes-runtimes', '运行实例', 'server',  '/hermes/runtimes', 'hermes/runtimes/index', '/hermes/collaboration', 4, 'ai:hermes:device:list'),
  ('hermes-skills',   '技能市场', 'appstore','/hermes/skills',   'hermes/skills/index',   '/hermes/resources',     4, 'ai:hermes:policy:read')
) AS v(name, title, icon, path, component, parent_path, sort, permission)
WHERE NOT EXISTS (SELECT 1 FROM menus m WHERE m.path = v.path);

INSERT INTO menu_permissions (menu_id, permission_id)
SELECT m.id, p.id
FROM menus m
JOIN permissions p ON p.code = m.permission
WHERE m.path IN ('/hermes/tasks', '/hermes/ai-ops', '/hermes/runtimes', '/hermes/skills')
ON CONFLICT DO NOTHING;

-- +goose Down
-- 完整产品升级含数据归属变化，不允许以删表/删除权限模拟回滚；应恢复升级前备份。
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Full product upgrade is forward-only; restore the pre-upgrade backup';
END $$;
-- +goose StatementEnd
