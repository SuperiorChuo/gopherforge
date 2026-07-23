-- +goose Up
-- 任务中心 M1：分布式任务心跳表。散落各服务进程内的后台循环（余额预警、
-- 自动出账、GEO 日检…）与主机 shell cron（PG 备份、磁盘清理）跑完各上报一行，
-- monitor 聚合成「服务任务心跳」视图——补上 scheduled_jobs（monitor 进程内
-- cron）覆盖不到的静默失败盲区。写入方：shared/pkg/jobbeat（隔离构建上下文
-- 的服务持同源副本）与 shell psql upsert；读方：monitor /monitor/jobs/heartbeats。
CREATE TABLE IF NOT EXISTS ops_job_heartbeats (
    id           BIGSERIAL PRIMARY KEY,
    -- job_key 全局唯一任务标识，约定 <服务>.<任务>（如 cc.balance_warn、ops.pg_backup）
    job_key      VARCHAR(100) NOT NULL UNIQUE,
    service      VARCHAR(50)  NOT NULL,
    description  VARCHAR(255) NOT NULL DEFAULT '',
    -- interval_sec 期望运行间隔（秒）；聚合侧按 now - last_run_at > 2*interval 判超期
    interval_sec BIGINT       NOT NULL DEFAULT 0,
    last_run_at  TIMESTAMPTZ  NOT NULL,
    -- last_status ok / error；last_error 最近一次失败原因（成功时清空）
    last_status  VARCHAR(16)  NOT NULL DEFAULT 'ok',
    last_error   TEXT         NOT NULL DEFAULT '',
    last_duration_ms BIGINT   NOT NULL DEFAULT 0,
    runs         BIGINT       NOT NULL DEFAULT 0,
    fails        BIGINT       NOT NULL DEFAULT 0,
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS ops_job_heartbeats;
