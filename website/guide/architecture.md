# 架构总览

GopherForge 采用**真微服务架构**：后端按域拆分为 7 个 Go 服务，另有 `shared` 公共库；前端单页应用，Traefik 网关统一入口与鉴权，全部经 Docker Compose 编排。

## 服务清单

| 服务 | 职责 |
|------|------|
| **auth** | 登录注册、JWT Access/Refresh 签发与吊销、验证码、TOTP 两步验证、OAuth 三方登录、网关 ForwardAuth 统一验签 |
| **identity** | 用户、角色、权限、部门、岗位、租户与套餐 CRUD，数据范围（数据权限），租户隔离 GORM 插件 |
| **system** | 菜单、字典、公告、系统设置（DB 热配置）、在线用户、短信、错误码、代码生成器 |
| **audit** | 登录日志、操作日志、审计日志查询；经 NATS 持久消费登录事件 |
| **file** | 文件上传下载（本地 / MinIO / S3 兼容云） |
| **monitor** | 服务器/PostgreSQL/Redis 监控、定时任务、健康检查、Prometheus metrics；持有共享 goose 迁移与网关兜底路由 |
| **bpm** | 轻量审批流引擎（详见[审批流](/modules/bpm)） |
| **shared** | 跨服务共享 Go module：日志、响应封装、脱敏、错误码、Excel、IP 归属地 |

## 请求链路

```
浏览器
  └─▶ Traefik 网关 :8000
        ├─ ForwardAuth ─▶ auth（验签，注入 X-Auth-* 头）
        └─ 按 PathPrefix 路由 ─▶ 各服务
                                   └─ 只信任网关注入的 X-Auth-User-ID /
                                      X-Auth-Tenant-ID 等头
```

三条安全约定：

1. **服务不裸奔**：宿主机端口默认只绑 loopback，外部流量一律经网关。
2. **鉴权集中**：受保护路由挂 ForwardAuth 中间件，业务服务不自行解析 JWT（内网直连场景才走 Bearer 兜底）。
3. **服务间内部调用**用 `X-Internal-Token` 共享密钥，不复用用户态凭证。

## 数据层

- **PostgreSQL 16**（pgvector 镜像）单实例共享库，各服务表按前缀隔离。
- **迁移单一真源**：goose 版本化 SQL 统一放在 `services/monitor/migrations/`，由 migrate 容器在启动时执行；实验线服务（如 bpm）的自管表走 GORM AutoMigrate（详见 [MIGRATIONS 约定](https://github.com/SuperiorChuo/gopherforge/blob/main/docs/development/MIGRATIONS.md)）。
- **Redis 7**：限流、在线用户、令牌黑名单、权限缓存。
- **NATS JetStream**：登录事件从 auth 解耦到 audit（持久消费，服务重启不丢）。

## 前端

React 19 + TypeScript + Vite 8 + Ant Design 6，Redux Toolkit 状态管理，Axios 拦截器统一解包 `{code, message, data}` 响应与无感刷新 Token。深空暗色 / 白蓝亮色双主题。

菜单与路由：菜单由后端种子数据下发（RBAC 过滤），前端静态路由表映射组件；新增页面需要同时补路由与菜单种子（见[二次开发](/guide/extend)）。

## 可观测（可选开启）

Prometheus 指标 + Grafana 看板配置在 `platform/deploy/`；OpenTelemetry + Jaeger 链路追踪按 env 开关接入。每个服务暴露 `/api/v1/health/live` 与 `/ready`。

## 工程门禁

CI（GitHub Actions）对每个服务独立跑 `go test` + `go vet`，前端 lint + build + 依赖审计，外加三道特色门禁：

- **OpenAPI 契约漂移检测**：路由改了但 `openapi.json` 没更新会红。
- **迁移彩排**：migration-rehearsal 在干净库上预演全部迁移。
- **集成冒烟**：全栈 compose 拉起后 API 冒烟 + Playwright E2E。
