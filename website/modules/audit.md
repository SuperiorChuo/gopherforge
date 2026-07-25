# 审计日志

audit 服务集中记录三类日志：**操作日志**（每个 API 请求）、**登录日志**（认证事件）、**业务审计日志**（关键对象的变更前后快照）。设计目标：可靠采集不拖慢业务、敏感信息不落盘。

## 三类日志

| 类型 | 数据表 | 记录内容 |
|------|--------|---------|
| 操作日志 | `operation_logs` | 操作人、模块/动作、方法与路径、请求/响应体（脱敏）、状态码、IP、UA、耗时、request_id |
| 登录日志 | `login_logs` | 用户、登录方式（密码 / GitHub / 微信 / TOTP）、成败与失败原因、IP 与归属地、设备/OS/浏览器 |
| 业务审计 | `audit_logs` | 操作者、动作、目标类型与 ID、变更前后 JSON、摘要 |

## 采集链路（为什么不拖慢业务）

**操作日志——HTTP 中间件异步落库。** 中间件拦截请求后缓存请求/响应体，先对 `password`、`token` 等敏感字段做脱敏，再写入进程内缓冲队列（容量 1000），由后台协程异步持久化（写库超时 2 秒）。**写库失败不影响业务请求**，仅记错误日志（可用 request_id 追踪）；`/api/v1/health`、`/api/v1/captcha` 等高频路径不记录。

**登录日志——NATS JetStream 事件。** auth 服务在登录成功/失败时向 `auth.login.success` / `auth.login.failed` 主题发布事件（异步 best-effort，2 秒超时不阻塞登录）；audit 服务用持久化消费者（`AUTH_EVENTS` 流，事件保留 7 天，失败 Nak 重试最多 5 次）落库。这也是脚手架**必须带 NATS** 的原因。

## IP 归属地

登录日志落库时解析 IP 归属地：优先走 `ip2region` 离线库（微秒级，`scripts/download-ip2region.sh` 下载 `ip2region.xdb` 约 11MB，不进 git），未部署或查不到时回退 ip-api.com 在线查询（结果缓存 1 小时）；内网/回环地址直接标记「内网」。离线库缺失不影响启动，只是降级。

## 前端页面

| 页面 | 路径 | 能力 |
|------|------|------|
| 操作日志 | `/system/operation-log` | 按用户/方法/路径/模块/状态码/时间范围筛选，详情含脱敏后的请求响应体 |
| 登录日志 | `/system/login-log` | 按用户/IP/成败/时间筛选；**地理分布图**（省市聚合，内网/海外单列）与 **7 天登录趋势**（成败分离） |
| 审计日志 | `/system/audit-log` | 关键词 + 动作 + 目标类型（facets 动态生成） |

## 接口速查

| 方法 | 路径 | 权限码 | 用途 |
|------|------|--------|------|
| GET | `/api/v1/operation-logs` | `system:log:operation` | 操作日志列表（多条件筛选） |
| GET | `/api/v1/operation-logs/:id` | `system:log:operation` | 详情 |
| GET | `/api/v1/operation-logs/stats` | `system:log:operation` | 统计 |
| GET | `/api/v1/operation-logs/export` | `system:log:operation` | 导出 CSV |
| DELETE | `/api/v1/operation-logs/clear` | `system:log:operation:clear` | 清理 N 天前记录（`days` 参数） |
| GET | `/api/v1/login-logs` | `system:log:login` | 登录日志列表 |
| GET | `/api/v1/login-logs/my` · `/last` | 登录即可 | 我的登录记录 / 最近一次登录 |
| GET | `/api/v1/login-logs/stats` · `/trend` | `system:log:login` | 统计 / 趋势 |
| DELETE | `/api/v1/login-logs/clear` | `system:log:login` | 清理 N 天前记录 |
| GET | `/api/v1/logs/audit` | `system:log:audit` | 业务审计日志 |

## 保留与清理

两条路径：

- **自动保留策略**：设 `AUDIT_LOG_RETENTION_DAYS=N`（默认 `0` = 关闭，绝不隐式删数据）后，audit 服务每天自动清理 N 天前的操作/登录日志（**跨全部租户**）；扫描周期可用 `AUDIT_LOG_RETENTION_SCAN_INTERVAL_SECONDS` 调整。每轮清理经 jobbeat 上报心跳，控制台「定时任务 → 服务任务心跳」可见 `audit.log_retention`，失败会标 error。
- **手动清理**：`DELETE …/clear?days=N`（前端日志页有入口），带租户与权限约束。

**业务审计日志（`audit_logs`）刻意不在自动清理范围**——它是合规取证面，要清只能走显式运维操作。NATS 侧事件由流的 7 天 MaxAge 自动过期，与 DB 记录互不影响。

## 给二次开发者

业务服务接入操作日志零成本——挂同一个中间件即可；要记录业务级变更（谁把订单从 A 改到 B），写 `audit_logs`（before/after JSON + 摘要），前端审计日志页自动可查。
