-- +goose Up
-- 安全事件：audit 服务安全事件检测器（照 StartLogRetentionCleaner 模式）发现
-- 审计日志异常模式后落库 + 站内信通知。notified_at 非空表示已通知（去重依据）。
CREATE TABLE IF NOT EXISTS security_events (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL DEFAULT 1,
    rule VARCHAR(64) NOT NULL,
    severity VARCHAR(16) NOT NULL DEFAULT 'warning',
    summary VARCHAR(512) NOT NULL,
    actor_id VARCHAR(128) NOT NULL DEFAULT '',
    actor_type VARCHAR(32) NOT NULL DEFAULT 'operator',
    target VARCHAR(128) NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notified_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_security_event_severity CHECK (severity IN ('info', 'warning', 'critical'))
);

CREATE INDEX IF NOT EXISTS idx_security_events_rule_occurred ON security_events (rule, occurred_at);
CREATE INDEX IF NOT EXISTS idx_security_events_severity ON security_events (severity, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_security_events_severity;
DROP INDEX IF EXISTS idx_security_events_rule_occurred;
DROP TABLE IF EXISTS security_events;
