# Go Admin Kit

<p align="center">
  <strong>企业级后台管理脚手架 · 双产品线 · 开箱即用</strong><br/>
  Go + Gin &nbsp;·&nbsp; React + Ant Design &nbsp;·&nbsp; 微服务 / 单体任选
</p>

<p align="center">
  <!-- 状态 -->
  <a href="https://github.com/SuperiorChuo/go-admin-kit/actions/workflows/ci.yml"><img src="https://github.com/SuperiorChuo/go-admin-kit/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg?logo=open-source-initiative&logoColor=white" alt="License" /></a>
  <a href="https://github.com/SuperiorChuo/go-admin-kit"><img src="https://img.shields.io/github/stars/SuperiorChuo/go-admin-kit?style=flat&logo=github" alt="Stars" /></a>
  <a href="https://github.com/SuperiorChuo/go-admin-kit/network/members"><img src="https://img.shields.io/github/forks/SuperiorChuo/go-admin-kit?style=flat&logo=github" alt="Forks" /></a>
  <a href="https://github.com/SuperiorChuo/go-admin-kit/issues"><img src="https://img.shields.io/github/issues/SuperiorChuo/go-admin-kit?logo=github" alt="Issues" /></a>
</p>

<p align="center">
  <!-- 后端 -->
  <img src="https://img.shields.io/badge/Go-1.26.3-00ADD8?logo=go&logoColor=white" alt="Go" />
  <img src="https://img.shields.io/badge/Gin-1.12-08A4E0?logo=go&logoColor=white" alt="Gin" />
  <img src="https://img.shields.io/badge/GORM-1.31-00ADD8?logo=go&logoColor=white" alt="GORM" />
  <img src="https://img.shields.io/badge/JWT-v5-000000?logo=jsonwebtokens&logoColor=white" alt="JWT" />
  <img src="https://img.shields.io/badge/goose-migrations-2E8B57?logo=databricks&logoColor=white" alt="goose" />
  <img src="https://img.shields.io/badge/PostgreSQL-16-4169E1?logo=postgresql&logoColor=white" alt="PostgreSQL" />
  <img src="https://img.shields.io/badge/Redis-7-DC382D?logo=redis&logoColor=white" alt="Redis" />
</p>

<p align="center">
  <!-- 前端 -->
  <img src="https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black" alt="React" />
  <img src="https://img.shields.io/badge/TypeScript-5%2B-3178C6?logo=typescript&logoColor=white" alt="TypeScript" />
  <img src="https://img.shields.io/badge/Vite-8-646CFF?logo=vite&logoColor=white" alt="Vite" />
  <img src="https://img.shields.io/badge/Ant%20Design-6-0170FE?logo=antdesign&logoColor=white" alt="Ant Design" />
  <img src="https://img.shields.io/badge/Redux%20Toolkit-2-764ABC?logo=redux&logoColor=white" alt="Redux Toolkit" />
  <img src="https://img.shields.io/badge/React%20Router-7-CA4245?logo=reactrouter&logoColor=white" alt="React Router" />
  <img src="https://img.shields.io/badge/Axios-HTTP-5A29E4?logo=axios&logoColor=white" alt="Axios" />
</p>

<p align="center">
  <!-- 架构与工程 -->
  <img src="https://img.shields.io/badge/Traefik-Gateway-24A1C1?logo=traefikproxy&logoColor=white" alt="Traefik" />
  <img src="https://img.shields.io/badge/NATS-JetStream-27AAE1?logo=natsdotio&logoColor=white" alt="NATS" />
  <img src="https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white" alt="Docker" />
  <img src="https://img.shields.io/badge/OpenAPI-3.1-6BA539?logo=openapiinitiative&logoColor=white" alt="OpenAPI" />
  <img src="https://img.shields.io/badge/Prometheus-Metrics-E6522C?logo=prometheus&logoColor=white" alt="Prometheus" />
  <img src="https://img.shields.io/badge/Grafana-Dashboards-F46800?logo=grafana&logoColor=white" alt="Grafana" />
  <img src="https://img.shields.io/badge/OpenTelemetry-Tracing-000000?logo=opentelemetry&logoColor=white" alt="OpenTelemetry" />
  <img src="https://img.shields.io/badge/MinIO-S3-C72E49?logo=minio&logoColor=white" alt="MinIO" />
  <img src="https://img.shields.io/badge/GitHub%20Actions-CI-2088FF?logo=githubactions&logoColor=white" alt="GitHub Actions" />
</p>

---

## 为什么选 Go Admin Kit

| | |
|:---|:---|
| **双产品线** | 同一仓库提供 **微服务** 与 **单体** 两套可运行交付物，业务互不调用，按团队规模选型。 |
| **前端统一** | 两条线均使用 **React + Ant Design**，视觉与交互一致，降低学习成本。 |
| **开箱即用** | Docker Compose 一键拉起依赖、网关/服务与前端；内置 RBAC、日志、监控、迁移。 |
| **工程完备** | CI、OpenAPI、健康检查、Prometheus、可选链路追踪与对象存储。 |
| **可二次开发** | 已剥离业务耦合，适合内部运营后台、SaaS 控制台、中台管理端起点。 |

---

## 双产品线一览

| 产品线 | 目录 | 形态 | 默认入口 |
|--------|------|------|----------|
| **微服务版** | [`microservices/`](microservices/README.md) | Traefik + 多服务 + React | http://localhost:8000 |
| **单体版** | [`monolith/`](monolith/README.md) | 单进程 API + React | http://localhost:3001 |

- 单体 **不调用** 微服务；微服务 **不依赖** 单体。
- 能力对照与硬规则见 [`docs/PRODUCT_LINES.md`](docs/PRODUCT_LINES.md)。
- 公共监控模板：[`platform/`](platform/README.md)。

```text
                    ┌─────────────────────┐
                    │   go-admin-kit 仓   │
                    └──────────┬──────────┘
           ┌───────────────────┴───────────────────┐
           ▼                                       ▼
   microservices/                            monolith/
   网关 · 多服务 · React                      单进程 · React
   (auth/identity/system/…)                  (server + web)
           │                                       │
           └──────────── 业务零调用 ────────────────┘
```

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

> 截图来自真实运行界面。若 UI 大改，可用本机截图覆盖 `docs/screenshots/*.png`。

---

## 技术栈全景

### 后端

| 层级 | 技术 | 说明 |
|------|------|------|
| 语言 / 运行时 | **Go 1.26** | 高性能、强类型 |
| HTTP | **Gin** | 路由与中间件 |
| ORM | **GORM** + **pgx** | PostgreSQL 访问 |
| 迁移 | **goose** | 版本化 SQL 迁移 |
| 认证 | **JWT v5** | Access / Refresh、吊销与轮转 |
| 缓存 / 会话 | **Redis 7** | 限流、在线用户、黑名单等 |
| 数据库 | **PostgreSQL 16** | 主存储（pgvector 镜像便于 AI 扩展） |

### 前端

| 层级 | 技术 | 说明 |
|------|------|------|
| 框架 | **React 19** | 现代并发渲染 |
| 语言 | **TypeScript** | 类型安全 |
| 构建 | **Vite 8** | 极速开发与构建 |
| UI | **Ant Design 6** | 企业级组件库 |
| 状态 | **Redux Toolkit** | 可预测状态 |
| 路由 | **React Router 7** | 客户端路由 |
| 请求 | **Axios** | 拦截器 / Token 刷新 |

### 微服务架构（`microservices/`）

| 组件 | 技术 | 说明 |
|------|------|------|
| 网关 | **Traefik** | 路由、ForwardAuth 统一验签 |
| 消息 | **NATS JetStream** | 登录等事件解耦 |
| 服务 | auth / identity / system / audit / file / ai / **monitor** | 按域拆分 |
| 契约 | **OpenAPI 3.1** | 从路由生成 + 前端类型 |

### 可观测与存储（可选）

| 组件 | 技术 |
|------|------|
| 指标 | **Prometheus** |
| 看板 | **Grafana** |
| 链路 | **OpenTelemetry** + Jaeger |
| 对象存储 | **MinIO**（S3 兼容） |

### 工程化

| 项 | 技术 |
|----|------|
| 容器 | **Docker Compose** |
| CI | **GitHub Actions** |
| 提交规范 | **全中文** Conventional 风格（见 `CONTRIBUTING.md`） |
| 协作 | `AGENTS.md` 人与 AI 共用规范 |

---

## 功能矩阵

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

## 微服务拓扑（简图）

```text
Browser
   │
   ▼
Traefik :8000
   ├─ /login|/captcha|… ──► auth-service
   ├─ /users|/roles|…  ──► identity-service
   ├─ /menus|/dicts|…  ──► system-service
   ├─ /logs|…          ──► audit-service
   ├─ /files|/uploads  ──► file-service
   ├─ /ai|…            ──► ai-service
   └─ 其余 /api        ──► monitor-service（健康·监控·迁移·兜底）
         ▲
         │ ForwardAuth ──► auth-service /internal/verify
```

---

## 仓库结构

```text
go-admin-kit/
├── microservices/                 # 微服务产品线
│   ├── services/
│   │   ├── auth/                  # 认证、令牌、网关验签
│   │   ├── identity/              # 用户 / 角色 / 权限 / 部门
│   │   ├── system/                # 菜单 / 字典 / 公告 / 设置
│   │   ├── audit/                 # 日志与事件消费
│   │   ├── file/                  # 文件与 uploads
│   │   ├── ai/                    # AI 对话与知识库
│   │   └── monitor/               # 监控、健康、共享迁移、兜底
│   ├── web/                       # React + Ant Design
│   ├── docker-compose.yml
│   └── README.md
├── monolith/                      # 单体产品线（零调用微服务）
│   ├── server/                    # 完整单进程 API
│   ├── web/                       # React + Ant Design（独立副本）
│   ├── docker-compose.yml
│   └── README.md
├── platform/deploy/               # Prometheus / Grafana / OTel
├── docs/                          # 工程文档
│   └── PRODUCT_LINES.md
├── CONTRIBUTING.md
├── AGENTS.md
└── LOCAL_SETUP.md
```

---

## 快速开始

### 环境要求

- [Docker Desktop](https://www.docker.com/products/docker-desktop/)
- 可选本地开发：Go **1.26.3+**、Node.js **20.19+ / 22.12+**（推荐 24）、npm

### 微服务版

```bash
git clone https://github.com/SuperiorChuo/go-admin-kit.git
cd go-admin-kit/microservices
cp .env.example .env
docker compose up -d --build
# 或仓库根目录：make compose-up
```

| 入口 | 地址 |
|------|------|
| 统一网关（推荐） | http://localhost:8000 |
| 前端直连 | http://localhost:3000 |
| 健康检查 | http://localhost:8000/api/v1/health/ready |
| 认证服务调试 | http://localhost:8082 |

更多：[microservices/README.md](microservices/README.md)

### 单体版

```bash
cd go-admin-kit/monolith
cp .env.example .env
docker compose up -d --build
# 或仓库根目录：make mono-up
```

| 入口 | 地址 |
|------|------|
| 前端 | http://localhost:3001 |
| API | http://localhost:18081 |
| 健康检查 | http://localhost:18081/api/v1/health/ready |

更多：[monolith/README.md](monolith/README.md)

### 默认账号（仅本地开发）

| 用户名 | 密码 |
|--------|------|
| `admin` | `admin123` |

> 生产环境请立即修改密码，并替换 `.env` 中的 `JWT_SECRET`、数据库与中间件密钥。

---

## 端口一览（默认可并行）

| 用途 | 微服务 | 单体 |
|------|--------|------|
| 前端 | `3000`（网关 `8000`） | `3001` |
| API | 经 `8000` / 调试 `8081+` | `18081` |
| PostgreSQL | `5432` | `5433` |
| Redis | `6379` | `6380` |

冲突时在对应目录 `.env` 中修改 `*_PORT` 即可，容器内互通地址不变。

---

## 本地开发（可选）

**微服务**

```bash
cd microservices
docker compose up -d go-admin-kit-postgres go-admin-kit-redis go-admin-kit-nats
cd services/auth && go run ./cmd
cd web && npm ci && npm run dev
```

**单体**

```bash
cd monolith
docker compose up -d go-admin-kit-mono-postgres go-admin-kit-mono-redis
cd server && go run ./cmd/main.go
cd web && npm ci && npm run dev
```

完整联调说明：[LOCAL_SETUP.md](LOCAL_SETUP.md)

---

## 验证

```bash
# 微服务
cd microservices
(cd services/monitor && go test ./... && go vet ./...)
(cd web && npm run lint && npm run build)
npm run test:smoke:unit && npm run test:contract
API_BASE_URL=http://127.0.0.1:8000/api/v1 npm run smoke:api

# 单体
cd monolith
(cd server && go test ./... && go vet ./...)
(cd web && npm run lint && npm run build)
```

OpenAPI（微服务）：

```bash
cd microservices
npm run api:contract
git diff --exit-code -- services/monitor/docs/openapi.json
```

---

## 配置与文档

| 说明 | 路径 |
|------|------|
| 微服务环境变量 | [`microservices/.env.example`](microservices/.env.example) |
| 单体环境变量 | [`monolith/.env.example`](monolith/.env.example) |
| 微服务迁移 / OpenAPI | `microservices/services/monitor/migrations/`、`docs/openapi.json` |
| 单体迁移 | `monolith/server/migrations/` |
| 产品线对照 | [`docs/PRODUCT_LINES.md`](docs/PRODUCT_LINES.md) |
| 安全说明 | [`docs/SECURITY.md`](docs/SECURITY.md) · [`SECURITY.md`](SECURITY.md) |
| 工程说明 | [`docs/ENGINEERING.md`](docs/ENGINEERING.md) |

---

## 安全提示

上线前请至少替换：

- `JWT_SECRET`
- PostgreSQL / Redis / MinIO / Grafana 等密码与密钥
- 默认管理员密码策略
- `CORS_ALLOW_ORIGINS`

---

## 开源协作

<p>
  <a href="CONTRIBUTING.md"><img src="https://img.shields.io/badge/Contributing-欢迎贡献-brightgreen?logo=git&logoColor=white" alt="Contributing" /></a>
  <a href="AGENTS.md"><img src="https://img.shields.io/badge/AGENTS.md-人与%20AI%20规范-black?logo=openai&logoColor=white" alt="AGENTS" /></a>
  <a href="https://github.com/SuperiorChuo/go-admin-kit/issues"><img src="https://img.shields.io/badge/Issues-反馈问题-red?logo=github" alt="Issues" /></a>
</p>

- 贡献指南：[CONTRIBUTING.md](CONTRIBUTING.md)（**提交标题与正文须全中文**）
- 人与 AI 协作：[AGENTS.md](AGENTS.md)
- CI：https://github.com/SuperiorChuo/go-admin-kit/actions

---

## 路线图

- [ ] Playwright E2E 纳入 GitHub Actions  
- [ ] 首个正式 Release / Changelog  
- [ ] 更多二次开发示例页  
- [ ] 生产部署指南（Linux / Nginx / HTTPS）  

---

## License

本项目基于 [MIT License](LICENSE) 开源。欢迎 Star、Fork 与 PR。
