# Go Admin Kit

[![CI](https://github.com/SuperiorChuo/go-admin-kit/actions/workflows/ci.yml/badge.svg)](https://github.com/SuperiorChuo/go-admin-kit/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26.3-00ADD8?logo=go&logoColor=white)](microservices/legacy-backend/go.mod)
[![React](https://img.shields.io/badge/React-Ant%20Design-61DAFB?logo=react&logoColor=white)](microservices/web/package.json)

Go Admin Kit 是一套基于 Go + Gin 与 **React + Ant Design** 的后台管理脚手架仓库。仓库内包含**两个互不调用的独立产品线**：

| 产品线 | 目录 | 状态 |
|--------|------|------|
| **微服务版** | [`microservices/`](microservices/README.md) | **当前可运行（主交付）** |
| **单体版** | [`monolith/`](monolith/README.md) | 规划中，未交付 |

二者业务零调用；前端技术栈统一为 React Ant Design。公共监控模板见 [`platform/`](platform/README.md)。

## 项目截图

这些截图来自当前项目真实运行页面，覆盖登录、系统概览、系统管理和监控页面。

| 登录页 | 系统概览 |
| --- | --- |
| ![登录页](docs/screenshots/login.png) | ![系统概览](docs/screenshots/dashboard.png) |

| 用户管理 | 角色管理 |
| --- | --- |
| ![用户管理](docs/screenshots/users.png) | ![角色管理](docs/screenshots/roles.png) |

| 数据库（PostgreSQL）监控 | Redis 监控 |
| --- | --- |
| ![数据库（PostgreSQL）监控](docs/screenshots/mysql.png) | ![Redis 监控](docs/screenshots/redis.png) |

## 技术栈（微服务版）

- 后端：Go 1.26.3、Gin、GORM、goose、JWT、Redis、PostgreSQL 16、多微服务 + Traefik 网关 + NATS
- 前端：React、Vite、TypeScript、Ant Design（`microservices/web`）
- 工程：Docker Compose、GitHub Actions、OpenAPI 契约、可选 MinIO / Prometheus / Grafana / OTel

## 功能清单

- 登录、刷新 token、退出登录、token 撤销
- RBAC 权限、角色、菜单、部门和用户管理
- 字典、通知、文件上传、操作日志、登录日志
- 在线用户强制下线
- 任务调度、服务器监控、数据库（PostgreSQL）和 Redis 监控页面
- 健康检查、Prometheus metrics、请求 ID、审计日志
- OpenAPI JSON 生成
- Docker 一键启动依赖、网关、微服务与前端

## 目录结构

```text
.
├── microservices/       # ★ 微服务产品线（当前可运行）
│   ├── services/        # auth / identity / system / audit / file / ai
│   ├── legacy-backend/  # 瘦后端兜底（监控等，非完整单体）
│   ├── web/             # React + Ant Design 前端
│   ├── docker-compose.yml
│   └── README.md
├── monolith/            # 单体产品线（占位，阶段二）
├── platform/            # 公共监控等模板
├── tdesign-vue-go/      # 遗留 Vue 前端（非主路径）
├── docs/                # 工程文档
└── LOCAL_SETUP.md       # 本地联调说明
```

## 快速启动（微服务版）

```bash
git clone https://github.com/SuperiorChuo/go-admin-kit.git
cd go-admin-kit/microservices
cp .env.example .env
docker compose up -d --build
```

或在仓库根目录：`make compose-up`（转发到 `microservices/`）。

默认地址：

- 统一入口（Traefik 网关）：`http://localhost:8000`（页面与 API 都从这里走）
- 前端直连：`http://localhost:3000`
- 后端直连：`http://localhost:8081`
- 认证服务直连：`http://localhost:8082`
- 健康检查：`http://localhost:8000/api/v1/health/ready`

推荐通过网关访问。认证相关路径 → auth-service；其余 `/api`、`/uploads` 由对应微服务或 legacy-backend 承接；网关 ForwardAuth 统一验签。详情见 [`microservices/README.md`](microservices/README.md)。

默认管理员账号仅用于本地开发：

- 用户名：`admin`
- 密码：`admin123`

生产或共享环境请立即修改默认密码，并替换 `.env` 中的密钥和服务密码。

## 环境隔离

Docker Compose 使用项目专属容器、网络和数据卷，避免和其他项目的数据表混在一起：

- PostgreSQL 容器：`go-admin-kit-postgres`
- Redis 容器：`go-admin-kit-redis`
- PostgreSQL 数据卷：`go_admin_kit_postgres_data`
- Redis 数据卷：`go_admin_kit_redis_data`
- 默认数据库：`go_admin_kit`
- Docker 网络：`go-admin-kit-net`

如果本机已有 PostgreSQL、Redis、MinIO 或其他脚手架容器占用默认宿主机端口，可以先用 `docker ps --format "table {{.Names}}\t{{.Ports}}"` 查找冲突容器。确认不再需要时执行 `docker stop <container-name>` 释放端口；如果需要并行运行多个项目，则在 `.env` 中调整宿主机端口：

```env
POSTGRES_PORT=5433
REDIS_PORT=6380
MINIO_API_PORT=19000
MINIO_CONSOLE_PORT=19001
GATEWAY_PORT=18000
BACKEND_PORT=18081
FRONTEND_PORT=13000
```

这些变量只改变宿主机映射端口。容器内部仍然通过 `go-admin-kit-postgres:5432`、`go-admin-kit-redis:6379`、`go-admin-kit-minio:9000` 和后端内部 `8081` 通信，不会影响服务间配置。启用 MinIO 时使用 `docker compose --profile storage up -d --build`，如本机 `9000/9001` 已被占用，请同步调整 `MINIO_API_PORT` 和 `MINIO_CONSOLE_PORT`。

Python 辅助工具统一使用 `uv` 和项目内 `.venv`：

```powershell
uv sync
uv run python --version
```

## 本地开发

只启动依赖服务：

```powershell
docker compose up -d go-admin-kit-postgres go-admin-kit-redis
```

启动后端：

```powershell
cd microservices/legacy-backend
go run .\cmd\main.go
```

启动前端：

```powershell
cd microservices/web
npm install
npm run dev
```

## 数据库

后端容器会在启动主服务前幂等执行 goose 迁移；首次创建数据卷和后续升级都走同一条迁移路径：

```text
server/migrations/
```

如需离线或手动初始化环境，也可以导入基线 SQL（`db-import` 现在通过 `psql` 导入）：

```powershell
make db-import
```

后续结构变更使用版本化迁移：

```powershell
make migrate-status
make migrate-up
make migrate-create NAME=add_example_table
```

迁移说明见 `docs/development/MIGRATIONS.md`，最近一轮脚手架优化状态见 `docs/development/OPTIMIZATION_STATUS.md`。

## 验证

后端：

```powershell
cd microservices/legacy-backend
go test ./...
go vet ./...
```

前端：

```powershell
cd microservices/web
npm run test
npm run build:type
npm run lint
npm run stylelint
npm run build
npm audit --omit=dev
```

E2E：

```powershell
npm run e2e:frontend
```

完整栈启动后的 API smoke：

```powershell
npm run test:smoke:unit
npm run smoke:api
```

API 契约生成与测试：

```powershell
npm run api:contract
git diff --exit-code -- microservices/legacy-backend/docs/openapi.json tdesign-vue-go/src/api/generated/schema.d.ts
npm run test:contract
```

如果 `git diff --exit-code` 返回非零，说明 OpenAPI 或前端类型生成物发生漂移，需要提交 `microservices/legacy-backend/docs/openapi.json` 和 `tdesign-vue-go/src/api/generated/schema.d.ts` 的更新。

WebSocket 通知链路发布前也需要验证：先通过带 `Bearer` token 的 `POST /api/v1/ws/notifications/ticket` 获取一次性 `ticket`，再连接 `GET /api/v1/ws/notifications?ticket=...`。反向代理必须透传 WebSocket upgrade 头，例如 `Upgrade`、`Connection`、`Host`、`X-Forwarded-Proto` 和 `X-Forwarded-For`；生产环境的 `Origin` 必须与后端同源或包含在 `CORS_ALLOW_ORIGINS` 中，`CORS_ALLOW_CREDENTIALS=true` 时不要使用通配 `*`。

## 配置入口

- 后端默认配置：`server/configs/config.yaml`
- 后端示例配置：`server/configs/config.example.yaml`
- Docker 环境变量：`.env.example`
- 数据库基线 SQL（手动初始化参考）：`server/docs/go_admin_kit.sql`
- 数据库迁移：`server/migrations/`
- OpenAPI 契约：`microservices/legacy-backend/docs/openapi.json`
- 本地代码图谱：`CODE_GRAPH.md`

## 安全提示

生产环境部署前请至少替换：

- `JWT_SECRET`
- PostgreSQL 密码
- Redis 密码
- MinIO 密钥
- Grafana 密码
- 默认管理员密码策略
- `CORS_ALLOW_ORIGINS`

安全能力说明见 `docs/SECURITY.md`，发布前检查见 `docs/development/READINESS_CHECKLIST.md`，优化完成项和剩余收尾见 `docs/development/OPTIMIZATION_STATUS.md`。

## 开源协作

- 贡献指南：`CONTRIBUTING.md`（含**全中文提交信息**规范）
- 协作规范（人与 AI）：`AGENTS.md`
- 安全策略：`SECURITY.md`
- 问题反馈：`https://github.com/SuperiorChuo/go-admin-kit/issues`
- CI：`https://github.com/SuperiorChuo/go-admin-kit/actions`

发起 PR 前请运行相关验证命令，并在 PR 模板中说明影响范围、配置变化和测试结果。提交标题与正文须使用中文，勿写「中文标题 + 英文正文」。

## 路线图

- 将 Playwright E2E 纳入 GitHub Actions
- 补充 release notes 和 `v0.1.0` 首个开源版本
- 梳理更多二次开发示例页面
- 增加对象存储正式接入示例
- 补充部署到 Linux、Nginx、HTTPS 的生产指南

## License

本项目基于 [MIT License](LICENSE) 开源。
