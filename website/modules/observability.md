# 监控与可观测

monitor 服务 + platform/deploy 配置，提供从健康检查到告警闭环、链路追踪的完整可观测栈。

## 控制台内置监控页

- **服务器监控**：CPU/内存/磁盘/负载实时图表 + **微服务健康总览**（并发探测各服务 `/ready`，不健康的排最前，10 秒自刷）
- **数据库监控**：PostgreSQL 连接、慢查询、表体积
- **Redis 监控**：内存、命中率、键空间
- **定时任务**：cron 任务注册表与执行状态 + **服务任务心跳**（见下）

## 健康检查

每个服务暴露 `GET /api/v1/health/live`（存活）与 `/ready`（就绪，含 DB ping），compose 健康检查与编排依赖据此工作；monitor 的 `/monitor/services` 聚合探测所有服务，供健康总览卡片消费。

## 任务中心：分布式任务心跳

进程内 cron、独立 worker、主机 shell 脚本——分散在各处的定时任务如何知道「还活着」？`shared/pkg/jobbeat` 提供一行上报：任务每轮执行完写一条心跳（`ops_job_heartbeats` 表，含间隔与状态），monitor 聚合后在「定时任务」页出「服务任务心跳」卡片，**超过 2 倍间隔未上报即标记 stale 亮红**。shell 脚本用 curl 上报同一接口即可接入。

## 指标与看板

Prometheus 抓取各服务 metrics（`shared/pkg/metrics` 零依赖指标包：HTTP 计数/错误/延迟直方图 + Go runtime + DB 连接池），node_exporter 提供主机指标；Grafana 看板配置在 `platform/deploy/grafana/`（预置服务概览：QPS/错误率/P95/goroutine/连接池），`docker compose --profile observability` 可选拉起。

## 告警闭环（可选）

从「有指标没人看」到主动通知：

1. **Prometheus 告警规则**（`platform/deploy/prometheus/rules/`）：服务 down、磁盘不足、内存过高、5xx 陡增——滚动更新场景用 `for` 持续窗口吸收抖动。
2. **Alertmanager**：分组、去抖、恢复通知、宕机时抑制衍生的错误率告警；内部 token 启动时从环境注入模板，不进 git。
3. **投递到站内信**：Alertmanager webhook 打到 notify 接收端（`/internal/alerts`），按 fingerprint 去重后落站内信——值班人在控制台铃铛里直接看到。脚手架未启 notify 时投递失败仅记日志，不影响其余组件。

## 链路追踪（可选）

OpenTelemetry SDK 已埋好，配置 `OTEL_EXPORTER_OTLP_ENDPOINT` 指向 Jaeger/Collector 即启用，跨服务请求带 request_id 贯穿日志与响应头。

## 日志三件套

登录日志（NATS 事件持久消费）、操作日志（中间件级，含请求耗时与 request_id）、审计日志——audit 服务统一查询，操作日志支持按当前筛选导出 CSV。
