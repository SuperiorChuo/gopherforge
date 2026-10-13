-- +goose Up
-- 事务性 Outbox 表：业务事务内写事件 + 业务数据，worker 异步投递到 NATS JetStream。
-- 表落 public（平台共享），各服务 search_path 兜底可写；audit-service 跑 worker 投递。
-- 投递成功 status=sent；失败指数退避 next_at；耗尽 status=failed（人工/DLQ）。

CREATE TABLE IF NOT EXISTS outbox_events (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL DEFAULT 1,
    subject     VARCHAR(200) NOT NULL,
    payload     BYTEA NOT NULL,
    status      VARCHAR(16) NOT NULL DEFAULT 'pending',
    attempts    INT NOT NULL DEFAULT 0,
    last_error  VARCHAR(500),
    next_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_outbox_poll ON outbox_events (status, next_at);
CREATE INDEX IF NOT EXISTS idx_outbox_tenant ON outbox_events (tenant_id, status);

COMMENT ON TABLE outbox_events IS '事务性 Outbox：业务事务内写入，worker 异步投递到 NATS JetStream';
COMMENT ON COLUMN outbox_events.status IS 'pending=待投递, sending=投递中, sent=已投递, failed=耗尽重试';
COMMENT ON COLUMN outbox_events.next_at IS '下次可投递时间（退避后）';

-- +goose Down
-- 完整产品升级含数据归属变化，不允许以删表/删除权限模拟回滚；应恢复升级前备份。
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Full product upgrade is forward-only; restore the pre-upgrade backup';
END $$;
-- +goose StatementEnd
