# 监控与可观测

monitor 服务 + platform/deploy 配置，提供从健康检查到链路追踪的完整可观测栈。

## 控制台内置监控页

- **服务器监控**：CPU/内存/磁盘/负载实时图表
- **数据库监控**：PostgreSQL 连接、慢查询、表体积
- **Redis 监控**：内存、命中率、键空间
- **定时任务**：cron 任务注册表与执行状态

## 健康检查

每个服务暴露 `GET /api/v1/health/live`（存活）与 `/ready`（就绪，含 DB ping），compose 健康检查与编排依赖据此工作。

## 指标与看板

Prometheus 抓取各服务 metrics，Grafana 看板配置在 `platform/deploy/grafana/`，`docker compose --profile observability` 可选拉起。

## 链路追踪（可选）

OpenTelemetry SDK 已埋好，配置 `OTEL_EXPORTER_OTLP_ENDPOINT` 指向 Jaeger/Collector 即启用，跨服务请求带 request_id 贯穿日志与响应头。

## 日志三件套

登录日志（NATS 事件持久消费）、操作日志（中间件级，含请求耗时与 request_id）、审计日志——audit 服务统一查询，操作日志支持按当前筛选导出 CSV。
