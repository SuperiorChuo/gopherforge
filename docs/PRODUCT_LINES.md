# 微服务脚手架说明

本仓库是 **go-admin-kit 微服务脚手架**：只含平台无关的基础设施服务，不带任何业务功能。用它作为起点，业务能力按需自行增补服务。

| 产品线 | 目录 | 启动 |
|--------|------|------|
| 微服务脚手架 | `microservices/` | `make compose-up`（自动先起 infra 数据栈） |

## 内置能力

| 能力 | 微服务 |
|------|:------:|
| 登录 / JWT / 刷新 / 吊销 | ✅ auth-service |
| RBAC 用户角色权限部门 | ✅ identity-service |
| 菜单字典公告设置在线用户 | ✅ system-service |
| 登录/操作/审计日志 | ✅ audit-service |
| 文件上传 | ✅ file-service |
| 服务器 / DB / Redis / 任务监控 | ✅ monitor-service |
| 审批流引擎（流程定义 / 待办 / 会签或签） | ✅ bpm-service |
| 健康检查 / metrics | ✅ monitor-service（兜底路由） |
| Traefik 网关 + ForwardAuth | ✅ |
| NATS 登录事件 | ✅ |
| 前端技术栈 | React + Ant Design |
| 默认前端端口 | 3000 / 网关 8000 |
| 默认 API 端口 | 经 8000 |
| 默认 DB/Redis 宿主机端口 | 5432 / 6379 |

## 微服务进程一览

| 服务 | 目录 | 主要职责 |
|------|------|----------|
| auth-service | `services/auth` | 登录注册、令牌、验证码、OAuth、TOTP、网关验签 |
| identity-service | `services/identity` | 用户角色权限部门 |
| system-service | `services/system` | 菜单字典公告设置在线用户通知 |
| audit-service | `services/audit` | 日志查询与登录事件消费 |
| file-service | `services/file` | 文件与 uploads |
| monitor-service | `services/monitor` | 监控、健康、metrics、**共享 goose 迁移**、网关 `/api` 兜底 |
| bpm-service | `services/bpm` | 轻量审批流引擎（定义版本化 / 实例推进 / 会签或签 / 终态回调），设计见 `docs/design/bpm-approval-flow.md` |

> `services/shared` 为跨服务共享库（配置、中间件、遥测等），非独立进程。

## 硬规则

1. 共享仅限：`platform/` 监控模板、仓库级规范（`AGENTS.md` / `CONTRIBUTING.md`）。
2. 新增业务服务时，网关路由规则需同步补齐，否则经网关会 404。

## 迁移真源

- `microservices/services/monitor/migrations/`（monitor 容器启动时执行）
