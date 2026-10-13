-- +goose Up
-- 知识库写权限：改/删文档需 ai:kb:write（读/搜索登录即可）。仅默认授予 super_admin。
INSERT INTO permissions (name, code, type, path, method, parent_id, created_at, updated_at) VALUES
('知识库管理', 'ai:kb:write', 2, '/api/v1/ai/kb/documents', 'POST', 0, NOW(), NOW())
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code = 'ai:kb:write'
WHERE r.code = 'super_admin'
ON CONFLICT DO NOTHING;

-- +goose Down
-- 完整产品升级含数据归属变化，不允许以删表/删除权限模拟回滚；应恢复升级前备份。
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Full product upgrade is forward-only; restore the pre-upgrade backup';
END $$;
-- +goose StatementEnd
