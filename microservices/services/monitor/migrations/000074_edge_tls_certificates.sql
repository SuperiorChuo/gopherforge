-- +goose Up
-- 边缘 HTTPS 免费证书（Let's Encrypt ACME HTTP-01）：控制台申请/管理，非服务间 gRPC mTLS。

CREATE TABLE IF NOT EXISTS edge_tls_certificates (
    id              BIGSERIAL PRIMARY KEY,
    domain          VARCHAR(253) NOT NULL,
    email           VARCHAR(255) NOT NULL DEFAULT '',
    status          VARCHAR(32)  NOT NULL DEFAULT 'draft',
    provider        VARCHAR(32)  NOT NULL DEFAULT 'letsencrypt',
    is_staging      BOOLEAN      NOT NULL DEFAULT FALSE,
    fullchain_pem   TEXT,
    private_key_pem TEXT,
    account_key_pem TEXT,
    not_before      TIMESTAMPTZ,
    not_after       TIMESTAMPTZ,
    last_error      TEXT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_edge_tls_domain UNIQUE (domain)
);

CREATE INDEX IF NOT EXISTS idx_edge_tls_status ON edge_tls_certificates (status);
CREATE INDEX IF NOT EXISTS idx_edge_tls_not_after ON edge_tls_certificates (not_after);

COMMENT ON TABLE edge_tls_certificates IS '边缘域名 TLS 证书（ACME 免费申请）';
COMMENT ON COLUMN edge_tls_certificates.status IS 'draft|pending|issued|failed|expired';

INSERT INTO permissions (name, code, type, path, method, parent_id, created_at, updated_at)
SELECT '边缘证书查看', 'system:edge-cert:list', 2, '/api/v1/edge-certs', 'GET', 0, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM permissions WHERE code = 'system:edge-cert:list');

INSERT INTO permissions (name, code, type, path, method, parent_id, created_at, updated_at)
SELECT '边缘证书申请', 'system:edge-cert:issue', 2, '/api/v1/edge-certs', 'POST', 0, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM permissions WHERE code = 'system:edge-cert:issue');

INSERT INTO permissions (name, code, type, path, method, parent_id, created_at, updated_at)
SELECT '边缘证书删除', 'system:edge-cert:delete', 2, '/api/v1/edge-certs/:id', 'DELETE', 0, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM permissions WHERE code = 'system:edge-cert:delete');

INSERT INTO role_permissions (role_id, permission_id)
SELECT 1, p.id FROM permissions p
WHERE p.code IN ('system:edge-cert:list', 'system:edge-cert:issue', 'system:edge-cert:delete')
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp WHERE rp.role_id = 1 AND rp.permission_id = p.id
  );

INSERT INTO menus (name, title, icon, path, component, parent_id, sort, status, hidden, permission, created_at, updated_at)
SELECT 'edge-certs', '边缘证书', 'safety-certificate', '/system/edge-certs', 'system/edge-certs/index', 10, 14, 1, 0, 'system:edge-cert:list', NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM menus WHERE path = '/system/edge-certs');

-- +goose Down
DELETE FROM role_permissions WHERE permission_id IN (
  SELECT id FROM permissions WHERE code LIKE 'system:edge-cert:%'
);
DELETE FROM permissions WHERE code LIKE 'system:edge-cert:%';
DELETE FROM menus WHERE path = '/system/edge-certs';
DROP TABLE IF EXISTS edge_tls_certificates;
