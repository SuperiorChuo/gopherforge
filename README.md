# Go Admin Kit

Go Admin Kit 是一套干净的 Go 全栈后台管理脚手架。它保留后台系统常用能力，移除了旧业务页面、专项接口、历史迁移数据和运行产物。

## 包含内容

- `server/`：Go + Gin 后端 API，包含认证、RBAC、系统管理、文件上传、审计日志、监控接口、健康检查和数据库迁移命令。
- `tdesign-vue-go/`：Vue 3 + TDesign 控制台前端，包含登录、仪表盘、系统管理和监控页面。
- `server/docs/go_admin_kit.sql`：数据库基线 SQL，适合首次初始化。
- `server/migrations/`：基于 `goose` 的版本化数据库迁移。
- `docker-compose.yml`：本地 MySQL、Redis、后端、前端，以及可选 MinIO、Prometheus、Grafana、Tracing 服务。
- `deploy/`：监控和链路追踪配置。
- `tests/`：API smoke 和登录页 E2E 测试。

## 快速启动

当前运行栈：Go 1.26.3、Vue 3.5、Vite 8、TypeScript 6、TDesign Vue Next 1.20。前端建议使用 Node.js 24。Python 辅助工具统一使用 `uv` 创建项目内 `.venv`，不要向全局 Python 环境安装依赖。

```powershell
cd C:\Users\Administrator\Desktop\go-admin-kit
Copy-Item .env.example .env
docker compose up -d --build
```

默认地址：

- 前端：`http://localhost:3000`
- 后端：`http://localhost:8081`
- 健康检查：`http://localhost:8081/api/v1/health/ready`

默认管理员：

- 账号：`admin`
- 密码：`admin123`

## 本地开发

后端：

```powershell
cd server
go run .\cmd\main.go
```

前端：

```powershell
cd tdesign-vue-go
npm install
npm run dev
```

Python 辅助工具隔离环境：

```powershell
uv sync
uv run python --version
```

## 数据库

首次初始化可以继续使用基线 SQL：

```powershell
make db-import
```

后续结构变更使用版本化迁移：

```powershell
make migrate-status
make migrate-up
make migrate-create NAME=add_example_table
```

迁移说明见 [docs/development/MIGRATIONS.md](docs/development/MIGRATIONS.md)。

## 验证

后端：

```powershell
cd server
go test ./...
go vet ./...
```

前端：

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

完整栈启动后执行真实 API smoke：

```powershell
npm run test:smoke:unit
npm run smoke:api
```

API 契约生成与测试：

```powershell
npm run api:contract
npm run test:contract
```

说明见 [docs/development/API_CONTRACT.md](docs/development/API_CONTRACT.md)。

## 配置入口

- 后端默认配置：`server/configs/config.yaml`
- 后端示例配置：`server/configs/config.example.yaml`
- Docker 环境变量：`.env.example`
- 数据库基线：`server/docs/go_admin_kit.sql`
- 数据库迁移：`server/migrations/`
- 本地代码图谱：`CODE_GRAPH.md`

生产环境部署前请替换 `JWT_SECRET`、数据库密码、Redis 密码、MinIO 密钥、Grafana 密码和默认管理员密码策略。
