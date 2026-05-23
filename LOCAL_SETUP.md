# 本地联调说明

本文档说明如何在本机启动 Go Admin Kit。所有步骤都基于项目根目录 `C:\Users\Administrator\Desktop\go-admin-kit`。

## 环境要求

- Go 1.26.3+
- Node.js 20.19+ 或 22.12+，推荐 Node.js 24
- npm
- uv 0.11+
- Docker Desktop
- MySQL 客户端可选，仅在手动导入 SQL 时需要

## Docker 一键启动

```powershell
cd C:\Users\Administrator\Desktop\go-admin-kit
Copy-Item .env.example .env
docker compose up -d --build
```

后端容器会在启动主服务前幂等执行 `server/migrations/` 下的 goose 迁移；首次创建数据卷和后续升级都走同一条迁移路径。

查看服务：

```powershell
docker compose ps
```

停止服务：

```powershell
docker compose down
```

如果需要重建数据库：

```powershell
docker compose down -v
docker compose up -d --build
```

仅在离线或手动初始化环境中使用 `server/docs/go_admin_kit.sql`；日常 Docker 和本地开发优先使用 goose 迁移。

## 端口冲突处理

默认 Compose 会把 MySQL、Redis、后端、前端分别映射到宿主机 `3306`、`6379`、`8081`、`3000`，启用 `storage` profile 时 MinIO 还会占用 `9000` 和 `9001`。如果本机已有 scaffold 容器或本地服务占用这些端口，先查看冲突来源：

```powershell
docker ps --format "table {{.Names}}\t{{.Ports}}"
```

确认冲突容器不再需要时，可以停止它：

```powershell
docker stop <container-name>
```

如果需要保留冲突容器并行运行，请在 `.env` 中改宿主机映射端口：

```env
MYSQL_PORT=13306
REDIS_PORT=16379
MINIO_API_PORT=19000
MINIO_CONSOLE_PORT=19001
BACKEND_PORT=18081
FRONTEND_PORT=13000
```

这些变量只影响宿主机访问地址，容器内部仍使用 `go-admin-kit-mysql:3306`、`go-admin-kit-redis:6379`、`go-admin-kit-minio:9000` 和后端内部 `8081`。修改后重新执行 `docker compose up -d --build`；如果启用对象存储，使用 `docker compose --profile storage up -d --build`。

## 分别启动前后端

先启动依赖：

```powershell
docker compose up -d go-admin-kit-mysql go-admin-kit-redis
```

启动后端：

```powershell
cd server
go run .\cmd\main.go
```

启动前端：

```powershell
cd tdesign-vue-go
npm install
npm run dev
```

当前前端栈已升级到 Vue 3.5、Vite 8、TypeScript 6 和 TDesign Vue Next 1.20；后端健康检查会返回 Go 运行时版本，便于确认本地与容器环境是否一致。

## 数据库迁移

查看迁移状态：

```powershell
make migrate-status
```

应用迁移：

```powershell
make migrate-up
```

创建新迁移：

```powershell
make migrate-create NAME=add_example_table
```

迁移详情见 `docs/development/MIGRATIONS.md`。

## 验证

前端验证：

```powershell
cd tdesign-vue-go
npm run test
npm run e2e:frontend
npm run build:type
npm run lint
npm run stylelint
npm run build
npm audit --omit=dev
```

完整栈启动后的 API smoke：

```powershell
cd C:\Users\Administrator\Desktop\go-admin-kit
npm run test:smoke:unit
npm run smoke:api
```

API 契约生成与测试：

```powershell
npm run api:contract
git diff --exit-code -- server/docs/openapi.json tdesign-vue-go/src/api/generated/schema.d.ts
npm run test:contract
```

契约说明见 `docs/development/API_CONTRACT.md`。

## WebSocket 通知联调

通知 WebSocket 不直接使用长效 JWT 连接。前端或调试脚本需要先带 `Authorization: Bearer <access-token>` 请求：

```text
POST /api/v1/ws/notifications/ticket
```

响应中的 `data.ticket` 是一次性短期票据，随后用它建立连接：

```text
GET /api/v1/ws/notifications?ticket=...
```

如果通过 Nginx、网关或云负载均衡访问后端，请确认代理允许 WebSocket upgrade，并透传 `Upgrade`、`Connection`、`Host`、`X-Forwarded-Proto` 和 `X-Forwarded-For`。后端会校验 `Origin`：同源请求允许，跨域请求必须把前端域名加入 `CORS_ALLOW_ORIGINS`；当 `CORS_ALLOW_CREDENTIALS=true` 时不要配置 `*`。

## Python 辅助工具隔离环境

本项目的 Python 辅助脚本和临时工具统一通过 `uv` 运行，依赖只进入项目根目录的 `.venv`，不要使用全局 `pip install`。

```powershell
uv sync
uv run python --version
```

如果后续需要新增 Python 依赖，使用：

```powershell
uv add <package-name>
```

## 常用地址

- 前端：`http://localhost:3000`
- 后端：`http://localhost:8081`
- 健康检查：`http://localhost:8081/api/v1/health/ready`
- MinIO 控制台：`http://localhost:9001`，需要启用 `storage` profile
- Grafana：`http://localhost:3003`，需要启用 `monitoring` profile

## 默认账号

- 用户名：`admin`
- 密码：`admin123`

生产或共享环境中请及时修改默认密码，并替换 `.env` 中的敏感配置。
