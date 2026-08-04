-- +goose Up
ALTER TABLE monitor_alert_rules
    ADD COLUMN IF NOT EXISTS notify_channels TEXT NOT NULL DEFAULT '[]',
    ADD COLUMN IF NOT EXISTS silence_until TIMESTAMPTZ NULL;

-- 扩指标白名单：新增 pg 慢查询 / redis 逐出·过期 / 磁盘 IO / 网络吞吐（均为 rate 或 count）
ALTER TABLE monitor_alert_rules
    DROP CONSTRAINT IF EXISTS chk_monitor_alert_rule_metric,
    ADD CONSTRAINT chk_monitor_alert_rule_metric CHECK (metric IN (
        'system.cpu.used_percent',
        'system.memory.used_percent',
        'system.disk.used_percent',
        'postgres.connections.percent',
        'redis.memory.used_bytes',
        'redis.clients.connected',
        'pg.slow_queries',
        'redis.keys_evicted_per_sec',
        'redis.keys_expired_per_sec',
        'system.disk.read_bytes_per_sec',
        'system.disk.write_bytes_per_sec',
        'system.net.bytes_recv_per_sec',
        'system.net.bytes_sent_per_sec'
    )),
    DROP CONSTRAINT IF EXISTS chk_monitor_alert_rule_integer_threshold,
    ADD CONSTRAINT chk_monitor_alert_rule_integer_threshold CHECK (
        metric NOT IN (
            'redis.memory.used_bytes',
            'redis.clients.connected',
            'pg.slow_queries',
            'redis.keys_evicted_per_sec',
            'redis.keys_expired_per_sec',
            'system.disk.read_bytes_per_sec',
            'system.disk.write_bytes_per_sec',
            'system.net.bytes_recv_per_sec',
            'system.net.bytes_sent_per_sec'
        ) OR threshold = FLOOR(threshold)
    );

-- +goose Down
ALTER TABLE monitor_alert_rules
    DROP CONSTRAINT IF EXISTS chk_monitor_alert_rule_metric,
    ADD CONSTRAINT chk_monitor_alert_rule_metric CHECK (metric IN (
        'system.cpu.used_percent',
        'system.memory.used_percent',
        'system.disk.used_percent',
        'postgres.connections.percent',
        'redis.memory.used_bytes',
        'redis.clients.connected'
    )),
    DROP CONSTRAINT IF EXISTS chk_monitor_alert_rule_integer_threshold,
    ADD CONSTRAINT chk_monitor_alert_rule_integer_threshold CHECK (
        metric NOT IN ('redis.memory.used_bytes', 'redis.clients.connected')
        OR threshold = FLOOR(threshold)
    );
ALTER TABLE monitor_alert_rules
    DROP COLUMN IF EXISTS notify_channels,
    DROP COLUMN IF EXISTS silence_until;
