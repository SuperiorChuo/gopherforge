# shared/pkg 职责收拢（2026-08-11 架构评估 P5）

shared/pkg 下的共享包分为两类，新增包时先归位：

## 平台共享（通用基础设施，供所有服务复用）

跨服务通用能力，无业务语义，可放心 import：

| 包 | 职责 |
|---|---|
| `errors` | 应用错误类型 + HTTP 状态映射 |
| `logger` | zap 结构化日志 |
| `response` | 统一 HTTP 响应信封 + 错误码 |
| `middleware` | 共享 gin 中间件（error_handler/logger/request_id 等） |
| `model` | 跨服务共享数据模型（角色/用户/部门等底座实体） |
| `tenant` | 多租户 ctx 工具 + GORM 隔离插件 |
| `connpool` | 连接池护栏（Apply） |
| `grpcx` | gRPC server/client + Consul 发现 + TLS |
| `observability` | OTel 追踪 / Gin 追踪 |
| `resilience` | 重试 + 熔断 + 超时 |
| `audittrail` | 数据变更审计 GORM 插件 |
| `auditevents` | 审计事件发布（NATS） |
| `graceful` | 优雅关闭 |
| `consoleauth` | 网关会话令牌解析 |
| `gatewayauth` | 网关认证辅助 |
| `internalhttp` | 内部 HTTP 客户端基础 |
| `health` | 健康探针 |
| `iploc` | IP 定位 |
| `jobbeat` | 后台任务心跳 |
| `mailer` | SMTP 邮件 |
| `mask` | 敏感字段脱敏 |
| `metrics` | Prometheus 指标 |
| `pagination` | 分页工具 |
| `envsecret` | Swarm secrets / env sensitive config |
| `secretstrength` | 密钥强度校验 |
| `tlsutil` | mTLS 证书工具 |
| `excel` | Excel 处理 |

## 业务适配（特定业务服务的客户端，谨慎复用）

封装对特定服务的调用，含服务地址/契约知识，**仅业务服务使用**：

| 包 | 职责 |
|---|---|
| `identityclient` | identity-service 的 owner API 客户端（gRPC 优先 + HTTP 回退） |
| `notifyclient` | notify-service 的通知发送客户端 |

## 原则

- 新共享能力**先归「平台共享」**，除非它明确是某服务的客户端契约。
- 业务 client 包（identityclient/notifyclient）放 shared 是为了避免循环依赖（服务间调用），但应**保持最小面**——只暴露该服务必需的契约。
- 零消费者的共享包不保留（P0 已清理，见 2026-08-11 台账）。
