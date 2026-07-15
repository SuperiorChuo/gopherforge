# 双产品线说明

本仓库包含两个**互不调用**的独立交付物。开发时只进入其中一条线。

| 产品线 | 目录 | 启动 |
|--------|------|------|
| 微服务 | `microservices/` | `cd microservices && docker compose up -d --build` 或 `make compose-up` |
| 单体 | `monolith/` | `cd monolith && docker compose up -d --build` 或 `make mono-up` |
| 呼叫媒体 | `freeswitch-cc/` | `cd freeswitch-cc && docker compose up -d --build` |

## 能力对照

| 能力 | 微服务 | 单体 |
|------|:------:|:----:|
| 登录 / JWT / 刷新 / 吊销 | ✅ auth-service | ✅ 进程内 |
| RBAC 用户角色权限部门 | ✅ identity-service | ✅ 进程内 |
| 菜单字典公告设置在线用户 | ✅ system-service | ✅ 进程内 |
| 登录/操作/审计日志 | ✅ audit-service | ✅ 进程内 |
| 文件上传 | ✅ file-service | ✅ 进程内 |
| 服务器 / DB / Redis / 任务监控 | ✅ monitor-service | ✅ 进程内 |
| 健康检查 / metrics | ✅ monitor-service（兜底路由） | ✅ |
| Traefik 网关 + ForwardAuth | ✅ | ❌ |
| NATS 登录事件 | ✅ | ❌（不需要） |
| AI 对话 / 知识库 | ✅ ai-service + web | ❌（前端 stub，不启服务） |
| 前端技术栈 | React + Ant Design | React + Ant Design |
| 默认前端端口 | 3000 / 网关 8000 | 3001 |
| 默认 API 端口 | 经 8000 | 18081 |
| 默认 DB/Redis 宿主机端口 | 5432 / 6379 | 5433 / 6380 |

## 硬规则

1. **禁止** `monolith` import 或 HTTP 依赖 `microservices/services/*`。
2. **禁止** `microservices` 依赖 `monolith/server`。
3. 共享仅限：`platform/` 监控模板、仓库级规范（`AGENTS.md` / `CONTRIBUTING.md`）。
4. 前端各自目录演进，不要做成运行时「模式开关」连两套后端。

## 微服务进程一览

| 服务 | 目录 | 主要职责 |
|------|------|----------|
| auth-service | `services/auth` | 登录注册、令牌、验证码、OAuth、TOTP、网关验签 |
| identity-service | `services/identity` | 用户角色权限部门 |
| system-service | `services/system` | 菜单字典公告设置在线用户通知 |
| audit-service | `services/audit` | 日志查询与登录事件消费 |
| file-service | `services/file` | 文件与 uploads |
| ai-service | `services/ai` | 对话、知识库、日志洞察 |
| monitor-service | `services/monitor` | 监控、健康、metrics、**共享 goose 迁移**、网关 `/api` 兜底 |

## 迁移真源

- 微服务栈：`microservices/services/monitor/migrations/`（monitor 容器启动时执行）
- 单体栈：`monolith/server/migrations/`
