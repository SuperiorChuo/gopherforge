# 监控与可观测

monitor 服务 + platform/deploy 配置，提供从健康检查到告警闭环、链路追踪的完整可观测栈。

## 控制台内置监控页

- **服务器监控**：CPU/内存/磁盘/负载实时图表 + **微服务健康总览**（并发探测各服务 `/ready`，不健康的排最前，10 秒自刷）
- **数据库监控**：PostgreSQL 连接、慢查询、表体积
- **Redis 监控**：内存、命中率、键空间
- **定时任务**：cron 任务注册表与执行状态 + **服务任务心跳**（见下）
- **告警管理**：内置告警规则引擎的规则与事件（见「内置告警规则引擎」）

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

## 内置告警规则引擎（monitor 服务自带，SMTP 邮件通知）

上面「告警闭环」走的是基础设施层的 Prometheus + Alertmanager；这套**内置引擎**跑在 monitor 服务进程内、不依赖 Prometheus——面向应用可运维指标做**周期评估 + 持久化状态机 + 邮件通知**，规则可在控制台直接增删改查。两条路径互补：Prometheus 盯「服务 down / 磁盘 / 内存 / 5xx」这类基础设施，内置引擎盯可配阈值的应用指标并直接邮件告警。

### 指标与规则

| 指标键 | 含义 | 单位 |
|--------|------|------|
| `system.cpu.used_percent` | 主机 CPU 使用率 | percent |
| `system.memory.used_percent` | 主机内存使用率 | percent |
| `system.disk.used_percent` | 根文件系统使用率 | percent |
| `postgres.connections.percent` | PostgreSQL 连接数占 `max_connections` 比例 | percent |
| `redis.memory.used_bytes` | Redis `used_memory` | bytes |
| `redis.clients.connected` | Redis 客户端连接数 | count |

规则字段：名称、指标、运算符（`gt` / `gte` / `lt` / `lte`）、阈值、持续窗口（秒）、级别（`info` / `warning` / `critical`）、启用开关、是否发送恢复通知。数据落 `monitor_alert_rules`（迁移 `000032` 同步菜单与权限）。

### 状态机与事件

- `ok` → `pending`（指标持续超过阈值满持续窗口）→ `firing`；指标采集失败记 `error`（含 `last_error` 说明）。
- 每次 `pending → firing` 或 `firing → ok`（恢复）生成一条**告警事件**（`monitor_alert_events`，不可变留痕），记录当时值、阈值与通知结果。
- 评估由后台调度器周期执行，也支持**手动评估**：`POST /api/v1/monitor/alert-rules/:id/evaluate`（权限 `system:alert:evaluate`）。

### 通知

SMTP 邮件发送（含恢复通知）。配置项（`microservices/.env.example` 有全部占位）：

| 变量 | 说明 |
|------|------|
| `EMAIL_NOTIFICATION_ENABLED` | 总开关（默认 `false`） |
| `EMAIL_SMTP_HOST` / `EMAIL_SMTP_PORT` | SMTP 服务器 |
| `EMAIL_SMTP_USERNAME` / `EMAIL_SMTP_PASSWORD` | 认证 |
| `EMAIL_SENDER` | 发件人 |
| `EMAIL_ALERT_RECEIVER` / `EMAIL_ALERT_RECEIVERS` | 收件人（逗号分隔可多个） |
| `EMAIL_USE_TLS` / `EMAIL_START_TLS` | 加密方式 |

邮件未启用、或无收件人时，事件通知状态记 `skipped`；发送失败记 `failed` 并保留 `notify_error` 供排查。

### 管理页面

`/monitor/alerts`（权限 `system:alert:list`）：「告警规则」与「告警事件」两个页签，规则页支持新增 / 编辑 / 启用停用 / 手动评估 / 删除。

## 链路追踪（可选）

OpenTelemetry SDK 已埋好，配置 `OTEL_EXPORTER_OTLP_ENDPOINT` 指向 Jaeger/Collector 即启用，跨服务请求带 request_id 贯穿日志与响应头。

## 日志三件套

登录日志（NATS 事件持久消费）、操作日志（中间件级，含请求耗时与 request_id）、审计日志——audit 服务统一查询，操作日志支持按当前筛选导出 CSV。
