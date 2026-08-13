-- +goose Up

-- 边缘证书 V2：密钥加密、持久化任务/challenge、部署与线上观测状态。
-- legacy PEM 列刻意保留一个发布窗口：首阶段事务化回填密文但保留旧值，
-- 确认 V2 回滚点可用后以 EDGE_CERT_CLEAR_LEGACY_SECRETS=true 清空旧值。

ALTER TABLE edge_tls_certificates
    ADD COLUMN IF NOT EXISTS private_key_enc TEXT,
    ADD COLUMN IF NOT EXISTS account_key_enc TEXT,
    ADD COLUMN IF NOT EXISTS cert_fingerprint_sha256 VARCHAR(64),
    ADD COLUMN IF NOT EXISTS deployment_mode VARCHAR(32) NOT NULL DEFAULT 'external',
    ADD COLUMN IF NOT EXISTS deployment_provider VARCHAR(32) NOT NULL DEFAULT 'external',
    ADD COLUMN IF NOT EXISTS auto_renew_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS renew_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_renewal_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deployment_status VARCHAR(32) NOT NULL DEFAULT 'external',
    ADD COLUMN IF NOT EXISTS deployed_fingerprint_sha256 VARCHAR(64),
    ADD COLUMN IF NOT EXISTS deployed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS serving_status VARCHAR(32) NOT NULL DEFAULT 'unchecked',
    ADD COLUMN IF NOT EXISTS serving_fingerprint_sha256 VARCHAR(64),
    ADD COLUMN IF NOT EXISTS serving_not_after TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS serving_issuer VARCHAR(255),
    ADD COLUMN IF NOT EXISTS serving_checked_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS serving_error_code VARCHAR(64),
    ADD COLUMN IF NOT EXISTS serving_error_message TEXT;

CREATE INDEX IF NOT EXISTS idx_edge_tls_renew_due
    ON edge_tls_certificates (renew_at, id)
    WHERE auto_renew_enabled = TRUE AND renew_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS edge_cert_tasks (
    id              BIGSERIAL PRIMARY KEY,
    certificate_id  BIGINT NOT NULL REFERENCES edge_tls_certificates(id) ON DELETE CASCADE,
    kind            VARCHAR(16) NOT NULL,
    status          VARCHAR(16) NOT NULL DEFAULT 'queued',
    step            VARCHAR(32) NOT NULL DEFAULT 'queued',
    environment     VARCHAR(16) NOT NULL DEFAULT 'production',
    requested_by    BIGINT NOT NULL DEFAULT 0,
    attempt_count   INTEGER NOT NULL DEFAULT 0,
    run_after       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_owner     VARCHAR(64),
    lease_until     TIMESTAMPTZ,
    provider_order_uri TEXT,
    provider_cert_key_enc TEXT,
    error_code      VARCHAR(64),
    error_message   TEXT,
    error_hint      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ,
    CONSTRAINT chk_edge_cert_task_kind
        CHECK (kind IN ('issue', 'renew', 'deploy', 'probe')),
    CONSTRAINT chk_edge_cert_task_status
        CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
    CONSTRAINT chk_edge_cert_task_environment
        CHECK (environment IN ('staging', 'production'))
);

-- CREATE TABLE IF NOT EXISTS does not repair a table left by an interrupted or
-- partially rolled-out migration, so guard both recovery columns explicitly.
ALTER TABLE edge_cert_tasks
    ADD COLUMN IF NOT EXISTS provider_order_uri TEXT,
    ADD COLUMN IF NOT EXISTS provider_cert_key_enc TEXT;

-- 同一域名一次只允许一条冲突操作；重复点击不会重复向 CA 发起请求。
CREATE UNIQUE INDEX IF NOT EXISTS uq_edge_cert_tasks_one_active
    ON edge_cert_tasks (certificate_id)
    WHERE status IN ('queued', 'running');
CREATE INDEX IF NOT EXISTS idx_edge_cert_tasks_ready
    ON edge_cert_tasks (run_after, id)
    WHERE status = 'queued';
CREATE INDEX IF NOT EXISTS idx_edge_cert_tasks_lease
    ON edge_cert_tasks (lease_until, id)
    WHERE status = 'running';
CREATE INDEX IF NOT EXISTS idx_edge_cert_tasks_history
    ON edge_cert_tasks (certificate_id, created_at DESC);

CREATE TABLE IF NOT EXISTS edge_acme_challenges (
    token               VARCHAR(255) PRIMARY KEY,
    certificate_id      BIGINT NOT NULL REFERENCES edge_tls_certificates(id) ON DELETE CASCADE,
    key_authorization   TEXT NOT NULL,
    expires_at          TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_edge_acme_challenges_expires
    ON edge_acme_challenges (expires_at);

INSERT INTO permissions (name, code, type, path, method, parent_id, created_at, updated_at)
SELECT '边缘证书私钥导出', 'system:edge-cert:export', 2,
       '/api/v1/edge-certs/:id/export', 'POST', 0, NOW(), NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM permissions WHERE code = 'system:edge-cert:export'
);

-- 默认只授予 super_admin，不假定某个环境的角色主键恒为 1。
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code = 'system:edge-cert:export'
WHERE r.code = 'super_admin'
  AND NOT EXISTS (
      SELECT 1
      FROM role_permissions rp
      WHERE rp.role_id = r.id AND rp.permission_id = p.id
  );

COMMENT ON COLUMN edge_tls_certificates.private_key_enc IS
    'AES-256-GCM versioned envelope; plaintext legacy column must remain NULL';
COMMENT ON COLUMN edge_tls_certificates.account_key_enc IS
    'AES-256-GCM versioned envelope; plaintext legacy column must remain NULL';
COMMENT ON COLUMN edge_tls_certificates.deployment_status IS
    'external|not_deployed|queued|running|installed|failed';
COMMENT ON COLUMN edge_tls_certificates.serving_status IS
    'unchecked|healthy|mismatch|unreachable|invalid';
COMMENT ON TABLE edge_cert_tasks IS
    '边缘证书签发/续期/部署/探测的持久化 lease 任务';
COMMENT ON COLUMN edge_cert_tasks.provider_order_uri IS
    'Opaque ACME order reference; never expose through API or logs';
COMMENT ON COLUMN edge_cert_tasks.provider_cert_key_enc IS
    'Temporary AES-256-GCM certificate-key envelope, cleared atomically at issued';
COMMENT ON TABLE edge_acme_challenges IS
    'HTTP-01 持久化挑战，任意 system 副本可应答';

-- +goose Down

-- 密文或任务一旦产生，回退到不认识 envelope/task 的旧镜像会造成密钥或
-- 生命周期状态丢失。宁可显式拒绝 down，也绝不能悄悄 DROP 生产密钥。
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM edge_tls_certificates
        WHERE COALESCE(private_key_enc, '') <> ''
           OR COALESCE(account_key_enc, '') <> ''
    ) THEN
        RAISE EXCEPTION 'refusing edge certificate V2 down migration: encrypted certificate secrets exist';
    END IF;
    IF EXISTS (
        SELECT 1 FROM edge_cert_tasks
        WHERE COALESCE(provider_order_uri, '') <> ''
           OR COALESCE(provider_cert_key_enc, '') <> ''
    ) THEN
        RAISE EXCEPTION 'refusing edge certificate V2 down migration: recoverable ACME provider state exists';
    END IF;
    IF EXISTS (SELECT 1 FROM edge_cert_tasks) THEN
        RAISE EXCEPTION 'refusing edge certificate V2 down migration: task history exists';
    END IF;
END $$;
-- +goose StatementEnd

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE code = 'system:edge-cert:export'
);
DELETE FROM permissions WHERE code = 'system:edge-cert:export';

DROP TABLE IF EXISTS edge_acme_challenges;
DROP TABLE IF EXISTS edge_cert_tasks;

DROP INDEX IF EXISTS idx_edge_tls_renew_due;

ALTER TABLE edge_tls_certificates
    DROP COLUMN IF EXISTS serving_error_message,
    DROP COLUMN IF EXISTS serving_error_code,
    DROP COLUMN IF EXISTS serving_checked_at,
    DROP COLUMN IF EXISTS serving_issuer,
    DROP COLUMN IF EXISTS serving_not_after,
    DROP COLUMN IF EXISTS serving_fingerprint_sha256,
    DROP COLUMN IF EXISTS serving_status,
    DROP COLUMN IF EXISTS deployed_at,
    DROP COLUMN IF EXISTS deployed_fingerprint_sha256,
    DROP COLUMN IF EXISTS deployment_status,
    DROP COLUMN IF EXISTS last_renewal_at,
    DROP COLUMN IF EXISTS renew_at,
    DROP COLUMN IF EXISTS auto_renew_enabled,
    DROP COLUMN IF EXISTS deployment_provider,
    DROP COLUMN IF EXISTS deployment_mode,
    DROP COLUMN IF EXISTS cert_fingerprint_sha256,
    DROP COLUMN IF EXISTS account_key_enc,
    DROP COLUMN IF EXISTS private_key_enc;
