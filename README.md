# Go Admin Kit

[![CI](https://github.com/SuperiorChuo/go-admin-kit/actions/workflows/ci.yml/badge.svg)](https://github.com/SuperiorChuo/go-admin-kit/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26.3-00ADD8?logo=go&logoColor=white)](microservices/services/monitor/go.mod)
[![React](https://img.shields.io/badge/React-Ant%20Design-61DAFB?logo=react&logoColor=white)](microservices/web/package.json)

基于 **Go + Gin** 与 **React + Ant Design** 的后台管理脚手架。仓库内包含两条**互不调用**的独立产品线，可按场景二选一二次开发：

| 产品线 | 目录 | 形态 | 默认入口 |
|--------|------|------|----------|
| **微服务版** | [`microservices/`](microservices/README.md) | Traefik 网关 + 多服务 + React | [http://localhost:8000](http://localhost:8000) |
| **单体版** | [`monolith/`](monolith/README.md) | 单进程 API + React | [http://localhost:3001](http://localhost:3001) |

- 前端技术栈统一为 **React + Ant Design**（各线各有一份源码，独立演进）。
- 业务边界：单体不调用微服务，微服务不依赖单体。详见 [`docs/PRODUCT_LINES.md`](docs/PRODUCT_LINES.md)。
- 公共监控模板：[`platform/`](platform/README.md)。

---

## 项目截图

| 登录页 | 系统概览 |
| --- | --- |
| ![登录页](docs/screenshots/login.png) | ![系统概览](docs/screenshots/dashboard.png) |

| 用户管理 | 角色管理 |
| --- | --- |
| ![用户管理](docs/screenshots/users.png) | ![角色管理](docs/screenshots/roles.png) |

| 数据库监控 | Redis 监控 |
| --- | --- |
| ![数据库监控](docs/screenshots/mysql.png) | ![Redis 监控](docs/screenshots/redis.png) |

---

## 功能一览

| 能力 | 微服务 | 单体 |
|------|:------:|:----:|
| 登录 / JWT 刷新与撤销 / 验证码 / TOTP | ✅ | ✅ |
| RBAC（用户、角色、权限、部门、菜单） | ✅ | ✅ |
| 字典、公告、系统设置、在线用户 | ✅ | ✅ |
| 登录日志 / 操作日志 / 审计日志 | ✅ | ✅ |
| 文件上传 | ✅ | ✅ |
| 服务器 / PostgreSQL / Redis / 定时任务监控 | ✅ | ✅ |
| 健康检查、Prometheus metrics | ✅ | ✅ |
| Traefik 网关 + ForwardAuth | ✅ | — |
| NATS 登录事件 | ✅ | — |
| AI 对话 / 知识库 | ✅ | — |
| Docker Compose 一键启动 | ✅ | ✅ |

---

## 技术栈

**后端**

- Go 1.26、Gin、GORM、goose、JWT
- PostgreSQL 16、Redis
- 微服务额外：Traefik、NATS；可选 MinIO / Prometheus / Grafana / OpenTelemetry

**前端**

- React、Vite、TypeScript、Ant Design、Redux Toolkit

**工程**

- Docker Compose、GitHub Actions、OpenAPI 契约、全中文提交规范

---

## 仓库结构

```text
go-admin-kit/
├── microservices/                 # 微服务产品线
│   ├── services/
│   │   ├── auth/                  # 认证、令牌、网关验签
│   │   ├── identity/              # 用户 / 角色 / 权限 / 部门
│   │   ├── system/                # 菜单 / 字典 / 公告 / 设置 / 在线用户
│   │   ├── audit/                 # 日志查询与事件消费
│   │   ├── file/                  # 文件与 uploads
│   │   ├── ai/                    # AI 对话与知识库
│   │   └── monitor/               # 监控、健康、共享迁移、/api 兜底
│   ├── web/                       # React 前端
│   ├── docker-compose.yml
│   ├── go.work
│   └── README.md
├── monolith/                      # 单体产品线（与微服务零调用）
│   ├── server/                    # 完整单进程 API
│   ├── web/                       # React 前端（独立副本）
│   ├── docker-compose.yml
│   └── README.md
├── platform/deploy/               # Prometheus / Grafana / OTel 模板
├── docs/                          # 工程与安全文档
│   └── PRODUCT_LINES.md           # 双线能力对照
├── CONTRIBUTING.md                # 贡献与提交规范
├── AGENTS.md                      # 人与 AI 协作规范
└── LOCAL_SETUP.md                 # 本地联调摘要
```

---

## 快速开始

### 环境要求

- Docker Desktop
- 可选本地开发：Go 1.26.3+、Node.js 20.19+ / 22.12+（推荐 24）、npm

### 微服务版

```bash
git clone https://github.com/SuperiorChuo/go-admin-kit.git
cd go-admin-kit/microservices
cp .env.example .env
docker compose up -d --build
# 或在仓库根目录：make compose-up
```

| 入口 | 地址 |
|------|------|
| 统一网关（推荐） | http://localhost:8000 |
| 前端直连 | http://localhost:3000 |
| 健康检查 | http://localhost:8000/api/v1/health/ready |
| 认证服务调试 | http://localhost:8082 |

网关会将登录等认证路径路由到 `auth-service`，其余 `/api`、`/uploads` 由对应微服务或 `monitor` 承接，并在网关层 ForwardAuth 验签。详情见 [microservices/README.md](microservices/README.md)。

### 单体版

```bash
cd go-admin-kit/monolith
cp .env.example .env
docker compose up -d --build
# 或在仓库根目录：make mono-up
```

| 入口 | 地址 |
|------|------|
| 前端 | http://localhost:3001 |
| API | http://localhost:18081 |
| 健康检查 | http://localhost:18081/api/v1/health/ready |

单体默认端口与微服务错开，可同机并行。详情见 [monolith/README.md](monolith/README.md)。

### 默认账号（仅本地开发）

| 用户名 | 密码 |
|--------|------|
| `admin` | `admin123` |

生产或共享环境请立即修改密码，并替换 `.env` 中的 `JWT_SECRET`、数据库与中间件密码。

---

## 端口与并行运行

两条产品线默认宿主机端口：

| 用途 | 微服务 | 单体 |
|------|--------|------|
| 前端 | 3000（网关 8000） | 3001 |
| API | 经 8000 / 调试 8081+ | 18081 |
| PostgreSQL | 5432 | 5433 |
| Redis | 6379 | 6380 |

冲突时可在对应目录的 `.env` 中修改 `POSTGRES_PORT`、`REDIS_PORT`、`GATEWAY_PORT`、`BACKEND_PORT`、`FRONTEND_PORT` 等；容器内互通地址不变。

---

## 本地开发（可选）

### 微服务

```bash
cd microservices
docker compose up -d go-admin-kit-postgres go-admin-kit-redis go-admin-kit-nats
# 按需启动服务，例如：
cd services/auth && go run ./cmd
cd web && npm ci && npm run dev
```

### 单体

```bash
cd monolith
docker compose up -d go-admin-kit-mono-postgres go-admin-kit-mono-redis
cd server && go run ./cmd/main.go
cd web && npm ci && npm run dev
```

更完整的联调说明见 [LOCAL_SETUP.md](LOCAL_SETUP.md)。

---

## 验证

### 微服务

```bash
cd microservices
(cd services/monitor && go test ./... && go vet ./...)
(cd web && npm run lint && npm run build)
npm run test:smoke:unit
npm run test:contract
# 栈启动后：
API_BASE_URL=http://127.0.0.1:8000/api/v1 npm run smoke:api
```

### 单体

```bash
cd monolith
(cd server && go test ./... && go vet ./...)
(cd web && npm run lint && npm run build)
```

### OpenAPI（微服务）

```bash
cd microservices
npm run api:contract
git diff --exit-code -- services/monitor/docs/openapi.json
```

---

## 配置入口

| 说明 | 路径 |
|------|------|
| 微服务环境变量 | `microservices/.env.example` |
| 单体环境变量 | `monolith/.env.example` |
| 微服务迁移 / OpenAPI | `microservices/services/monitor/migrations/`、`.../docs/openapi.json` |
| 单体迁移 | `monolith/server/migrations/` |
| 产品线对照 | [`docs/PRODUCT_LINES.md`](docs/PRODUCT_LINES.md) |
| 安全说明 | [`docs/SECURITY.md`](docs/SECURITY.md) / [`SECURITY.md`](SECURITY.md) |

---

## 安全提示

上线前请至少替换：

- `JWT_SECRET`
- PostgreSQL / Redis / MinIO / Grafana 等密码与密钥
- 默认管理员密码策略
- `CORS_ALLOW_ORIGINS`

---

## 开源协作

- 贡献指南：[CONTRIBUTING.md](CONTRIBUTING.md)（**提交标题与正文须全中文**）
- 人与 AI 协作规范：[AGENTS.md](AGENTS.md)
- 问题反馈：https://github.com/SuperiorChuo/go-admin-kit/issues
- CI：https://github.com/SuperiorChuo/go-admin-kit/actions

发起 PR 前请按改动的产品线运行验证命令，并在模板中说明影响范围。

---

## 路线图

- Playwright E2E 纳入 GitHub Actions
- 首个正式 release notes / 版本标签
- 更多二次开发示例
- 生产部署指南（Linux / Nginx / HTTPS）

---

## License

[MIT License](LICENSE)
