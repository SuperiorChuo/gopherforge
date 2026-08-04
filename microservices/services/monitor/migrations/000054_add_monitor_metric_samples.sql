-- +goose Up
CREATE TABLE IF NOT EXISTS monitor_metric_samples (
    id BIGSERIAL PRIMARY KEY,
    category VARCHAR(32) NOT NULL,
    metric VARCHAR(64) NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    collected_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_monitor_metric_samples_lookup
    ON monitor_metric_samples (metric, collected_at);

-- +goose Down
DROP INDEX IF EXISTS idx_monitor_metric_samples_lookup;
DROP TABLE IF EXISTS monitor_metric_samples;
