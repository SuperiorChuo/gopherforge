-- +goose Up
CREATE TABLE IF NOT EXISTS monitor_alert_rules (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    metric VARCHAR(80) NOT NULL,
    operator VARCHAR(8) NOT NULL,
    threshold DOUBLE PRECISION NOT NULL,
    duration_seconds BIGINT NOT NULL DEFAULT 0,
    severity VARCHAR(16) NOT NULL DEFAULT 'warning',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    notify_on_resolve BOOLEAN NOT NULL DEFAULT TRUE,
    state VARCHAR(16) NOT NULL DEFAULT 'ok',
    pending_since TIMESTAMPTZ NULL,
    firing_since TIMESTAMPTZ NULL,
    last_value DOUBLE PRECISION NULL,
    last_evaluated_at TIMESTAMPTZ NULL,
    last_error VARCHAR(1000) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_monitor_alert_rule_metric CHECK (metric IN (
        'system.cpu.used_percent',
        'system.memory.used_percent',
        'system.disk.used_percent',
        'postgres.connections.percent',
        'redis.memory.used_bytes',
        'redis.clients.connected'
    )),
    CONSTRAINT chk_monitor_alert_rule_operator CHECK (operator IN ('gt', 'gte', 'lt', 'lte')),
    CONSTRAINT chk_monitor_alert_rule_threshold_finite CHECK (
        threshold <> 'NaN'::DOUBLE PRECISION
        AND threshold <> 'Infinity'::DOUBLE PRECISION
        AND threshold <> '-Infinity'::DOUBLE PRECISION
    ),
    CONSTRAINT chk_monitor_alert_rule_threshold_nonnegative CHECK (threshold >= 0),
    CONSTRAINT chk_monitor_alert_rule_percent_threshold CHECK (
        metric NOT IN (
            'system.cpu.used_percent',
            'system.memory.used_percent',
            'system.disk.used_percent',
            'postgres.connections.percent'
        ) OR threshold <= 100
    ),
    CONSTRAINT chk_monitor_alert_rule_integer_threshold CHECK (
        metric NOT IN ('redis.memory.used_bytes', 'redis.clients.connected')
        OR threshold = FLOOR(threshold)
    ),
    CONSTRAINT chk_monitor_alert_rule_duration CHECK (duration_seconds BETWEEN 0 AND 604800),
    CONSTRAINT chk_monitor_alert_rule_severity CHECK (severity IN ('info', 'warning', 'critical')),
    CONSTRAINT chk_monitor_alert_rule_state CHECK (state IN ('ok', 'pending', 'firing', 'error'))
);

CREATE INDEX IF NOT EXISTS idx_monitor_alert_rules_enabled ON monitor_alert_rules (enabled, id);
CREATE INDEX IF NOT EXISTS idx_monitor_alert_rules_state ON monitor_alert_rules (state, severity);

CREATE TABLE IF NOT EXISTS monitor_alert_events (
    id BIGSERIAL PRIMARY KEY,
    rule_id BIGINT NULL REFERENCES monitor_alert_rules(id) ON DELETE SET NULL,
    rule_name VARCHAR(100) NOT NULL,
    metric VARCHAR(80) NOT NULL,
    severity VARCHAR(16) NOT NULL,
    status VARCHAR(16) NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    threshold DOUBLE PRECISION NOT NULL,
    message VARCHAR(1000) NOT NULL,
    notify_status VARCHAR(16) NOT NULL DEFAULT 'pending',
    notify_error VARCHAR(1000) NOT NULL DEFAULT '',
    notified_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_monitor_alert_event_severity CHECK (severity IN ('info', 'warning', 'critical')),
    CONSTRAINT chk_monitor_alert_event_status CHECK (status IN ('firing', 'resolved')),
    CONSTRAINT chk_monitor_alert_event_notify_status CHECK (notify_status IN ('pending', 'sent', 'skipped', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_monitor_alert_events_rule_created ON monitor_alert_events (rule_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_monitor_alert_events_status_created ON monitor_alert_events (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_monitor_alert_events_notify_status ON monitor_alert_events (notify_status, created_at DESC);

INSERT INTO permissions (name, code, type, path, method, parent_id, created_at, updated_at) VALUES
('告警规则查看', 'system:alert:list', 2, '/api/v1/monitor/alert-rules', 'GET', 0, NOW(), NOW()),
('告警规则创建', 'system:alert:create', 2, '/api/v1/monitor/alert-rules', 'POST', 0, NOW(), NOW()),
('告警规则更新', 'system:alert:update', 2, '/api/v1/monitor/alert-rules', 'PUT', 0, NOW(), NOW()),
('告警规则删除', 'system:alert:delete', 2, '/api/v1/monitor/alert-rules', 'DELETE', 0, NOW(), NOW()),
('告警规则评估', 'system:alert:evaluate', 2, '/api/v1/monitor/alert-rules', 'POST', 0, NOW(), NOW())
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.code = 'super_admin'
  AND p.code IN (
    'system:alert:list', 'system:alert:create', 'system:alert:update',
    'system:alert:delete', 'system:alert:evaluate'
  )
ON CONFLICT DO NOTHING;

INSERT INTO menus (name, title, icon, path, component, parent_id, sort, status, hidden, permission, created_at, updated_at)
SELECT
    'monitor-alerts', '告警规则', 'alert', '/monitor/alerts', 'monitor/alerts/index',
    parent.id, 6, 1, 0, 'system:alert:list', NOW(), NOW()
FROM menus parent
WHERE parent.path = '/monitor' AND parent.parent_id = 0
  AND NOT EXISTS (SELECT 1 FROM menus existing WHERE existing.path = '/monitor/alerts');

INSERT INTO menu_permissions (menu_id, permission_id)
SELECT m.id, p.id
FROM menus m
JOIN permissions p ON p.code = 'system:alert:list'
WHERE m.path = '/monitor/alerts'
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM menu_permissions
WHERE menu_id IN (SELECT id FROM menus WHERE path = '/monitor/alerts')
   OR permission_id IN (
       SELECT id FROM permissions WHERE code IN (
         'system:alert:list', 'system:alert:create', 'system:alert:update',
         'system:alert:delete', 'system:alert:evaluate'
       )
   );
DELETE FROM menus WHERE path = '/monitor/alerts';
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE code IN (
      'system:alert:list', 'system:alert:create', 'system:alert:update',
      'system:alert:delete', 'system:alert:evaluate'
    )
);
DELETE FROM permissions WHERE code IN (
    'system:alert:list', 'system:alert:create', 'system:alert:update',
    'system:alert:delete', 'system:alert:evaluate'
);
DROP TABLE IF EXISTS monitor_alert_events;
DROP TABLE IF EXISTS monitor_alert_rules;
