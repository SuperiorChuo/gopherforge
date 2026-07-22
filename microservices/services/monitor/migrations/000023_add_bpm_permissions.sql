-- +goose Up
-- 审批中心流程定义管理权限点（任务/实例动作按 assignee/发起人身份校验，不设权限码）。
-- 挂系统管理 parent_id=2；bpm 表由 bpm-service AutoMigrate 自管，此处仅补权限点。
INSERT INTO permissions (name, code, type, path, method, parent_id, created_at, updated_at) VALUES
('流程定义查看', 'bpm:definition:list', 2, '/api/v1/bpm/definitions', 'GET', 2, NOW(), NOW()),
('流程定义新建', 'bpm:definition:create', 2, '/api/v1/bpm/definitions', 'POST', 2, NOW(), NOW()),
('流程定义编辑', 'bpm:definition:update', 2, '/api/v1/bpm/definitions/:id', 'PUT', 2, NOW(), NOW()),
('流程定义发布', 'bpm:definition:publish', 2, '/api/v1/bpm/definitions/:id/publish', 'POST', 2, NOW(), NOW())
ON CONFLICT DO NOTHING;

-- 超管角色补挂新权限（与 000001 的全量挂载语义一致）
INSERT INTO role_permissions (role_id, permission_id)
SELECT 1, id FROM permissions WHERE code IN (
  'bpm:definition:list', 'bpm:definition:create', 'bpm:definition:update', 'bpm:definition:publish'
)
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM role_permissions WHERE permission_id IN
  (SELECT id FROM permissions WHERE code IN (
    'bpm:definition:list', 'bpm:definition:create', 'bpm:definition:update', 'bpm:definition:publish'
  ));
DELETE FROM permissions WHERE code IN (
  'bpm:definition:list', 'bpm:definition:create', 'bpm:definition:update', 'bpm:definition:publish'
);
