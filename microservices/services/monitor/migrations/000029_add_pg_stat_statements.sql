-- +goose Up
-- 慢 SQL 测量能力：pg_stat_statements 按查询指纹聚合执行次数/耗时/行数。
-- 前置：PG 须以 shared_preload_libraries=pg_stat_statements 启动（docker-compose.infra.yml
-- 已加 command 参数，存量库需重启 PG 容器一次）；未预载时本扩展可创建、
-- 查询 pg_stat_statements 视图时才报错，故本迁移放在预载生效之后执行。
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- +goose Down
DROP EXTENSION IF EXISTS pg_stat_statements;
