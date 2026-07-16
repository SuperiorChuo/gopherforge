# 🚀 Go Admin Kit

<p align="center">
  <strong>✨ 企业级后台管理脚手架 · 中台 + 呼叫媒体 · 开箱即用 ✨</strong><br/>
  🐹 Go + Gin &nbsp;·&nbsp; ⚛️ React + Ant Design &nbsp;·&nbsp; 🧩 微服务 / 单体 / FreeSWITCH
</p>

<p align="center">
  <a href="https://github.com/SuperiorChuo/go-admin-kit/actions/workflows/ci.yml"><img src="https://github.com/SuperiorChuo/go-admin-kit/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg?logo=open-source-initiative&logoColor=white" alt="License" /></a>
  <a href="https://github.com/SuperiorChuo/go-admin-kit"><img src="https://img.shields.io/github/stars/SuperiorChuo/go-admin-kit?style=flat&logo=github" alt="Stars" /></a>
  <a href="https://github.com/SuperiorChuo/go-admin-kit/network/members"><img src="https://img.shields.io/github/forks/SuperiorChuo/go-admin-kit?style=flat&logo=github" alt="Forks" /></a>
  <a href="https://github.com/SuperiorChuo/go-admin-kit/issues"><img src="https://img.shields.io/github/issues/SuperiorChuo/go-admin-kit?logo=github" alt="Issues" /></a>
  <img src="https://img.shields.io/badge/PRs-Welcome-brightgreen.svg?logo=git&logoColor=white" alt="PRs Welcome" />
</p>

<p align="center">
  <img src="https://img.shields.io/badge/🐹_Go-1.26.3-00ADD8?logo=go&logoColor=white" alt="Go" />
  <img src="https://img.shields.io/badge/Gin-1.12-08A4E0?logo=go&logoColor=white" alt="Gin" />
  <img src="https://img.shields.io/badge/GORM-1.31-00ADD8?logo=go&logoColor=white" alt="GORM" />
  <img src="https://img.shields.io/badge/🔐_JWT-v5-000000?logo=jsonwebtokens&logoColor=white" alt="JWT" />
  <img src="https://img.shields.io/badge/🐘_PostgreSQL-16-4169E1?logo=postgresql&logoColor=white" alt="PostgreSQL" />
  <img src="https://img.shields.io/badge/🔴_Redis-7-DC382D?logo=redis&logoColor=white" alt="Redis" />
  <img src="https://img.shields.io/badge/goose-migrations-2E8B57?logo=databricks&logoColor=white" alt="goose" />
</p>

<p align="center">
  <img src="https://img.shields.io/badge/⚛️_React-19-61DAFB?logo=react&logoColor=black" alt="React" />
  <img src="https://img.shields.io/badge/📘_TypeScript-5%2B-3178C6?logo=typescript&logoColor=white" alt="TypeScript" />
  <img src="https://img.shields.io/badge/⚡_Vite-8-646CFF?logo=vite&logoColor=white" alt="Vite" />
  <img src="https://img.shields.io/badge/🎨_Ant%20Design-6-0170FE?logo=antdesign&logoColor=white" alt="Ant Design" />
  <img src="https://img.shields.io/badge/Redux%20Toolkit-2-764ABC?logo=redux&logoColor=white" alt="Redux Toolkit" />
  <img src="https://img.shields.io/badge/React%20Router-7-CA4245?logo=reactrouter&logoColor=white" alt="React Router" />
  <img src="https://img.shields.io/badge/Axios-HTTP-5A29E4?logo=axios&logoColor=white" alt="Axios" />
</p>

<p align="center">
  <img src="https://img.shields.io/badge/🚪_Traefik-Gateway-24A1C1?logo=traefikproxy&logoColor=white" alt="Traefik" />
  <img src="https://img.shields.io/badge/📡_NATS-JetStream-27AAE1?logo=natsdotio&logoColor=white" alt="NATS" />
  <img src="https://img.shields.io/badge/🐳_Docker-Compose-2496ED?logo=docker&logoColor=white" alt="Docker" />
  <img src="https://img.shields.io/badge/📜_OpenAPI-3.1-6BA539?logo=openapiinitiative&logoColor=white" alt="OpenAPI" />
  <img src="https://img.shields.io/badge/📈_Prometheus-E6522C?logo=prometheus&logoColor=white" alt="Prometheus" />
  <img src="https://img.shields.io/badge/📊_Grafana-F46800?logo=grafana&logoColor=white" alt="Grafana" />
  <img src="https://img.shields.io/badge/🔭_OpenTelemetry-000000?logo=opentelemetry&logoColor=white" alt="OpenTelemetry" />
  <img src="https://img.shields.io/badge/📦_MinIO-S3-C72E49?logo=minio&logoColor=white" alt="MinIO" />
  <img src="https://img.shields.io/badge/🤖_AI-Ready-7C3AED?logo=openai&logoColor=white" alt="AI Ready" />
  <img src="https://img.shields.io/badge/⚙️_GitHub%20Actions-2088FF?logo=githubactions&logoColor=white" alt="GitHub Actions" />
</p>

---

## ✨ 为什么选 Go Admin Kit

| 亮点 | 说明 |
|:-----|:-----|
| 🧩 **多产品线 monorepo** | 同一仓库：`microservices/`、`monolith/`、`freeswitch-cc/`；一人维护，进程仍可分开部署。 |
| ⚛️ **前端统一** | 两条线均使用 **React + Ant Design**，交互一致，降低学习成本。 |
| 🐳 **开箱即用** | Docker Compose 一键拉起依赖、网关/服务与前端；内置 RBAC、日志、监控、迁移。 |
| 🏗️ **工程完备** | CI、OpenAPI、健康检查、Prometheus、可选链路追踪与对象存储。 |
| 🔌 **可扩展** | 微服务按域拆分 + 网关标签接入，适合挂载 AI / IM / 客服等；呼叫媒体面建议 FreeSWITCH **独立项目** 对接。 |
| 🛠️ **可二次开发** | 已剥离业务耦合，适合内部运营后台、SaaS 控制台、中台管理端起点。 |

---

## 🧭 产品线一览

| 产品线 | 目录 | 形态 | 默认入口 |
|--------|------|------|----------|
| 🧩 **微服务版** | [`microservices/`](microservices/README.md) | Traefik + 多服务 + React | http://localhost:8000 |
| 📦 **单体版** | [`monolith/`](monolith/README.md) | 单进程 API + React | http://localhost:3001 |
| ☎️ **呼叫媒体** | [`freeswitch-cc/`](freeswitch-cc/README.md) | FreeSWITCH + control-api | SIP :5060 · API :8090 |

- 单体 **不调用** 微服务；微服务 **不依赖** 单体业务代码。
- 呼叫：**中台微服务可控制 FreeSWITCH**；媒体进程在 `freeswitch-cc/` 独立 compose，便于单独升级增强。
- 能力对照 👉 [`docs/PRODUCT_LINES.md`](docs/PRODUCT_LINES.md) · 呼叫设计 👉 [`docs/design/freeswitch-cc.md`](docs/design/freeswitch-cc.md)

```text
                    ┌──────────────────────┐
                    │   🚀 go-admin-kit    │  一人 monorepo
                    └──────────┬───────────┘
       ┌───────────────────────┼───────────────────────┐
       ▼                       ▼                       ▼
🧩 microservices/        📦 monolith/            ☎️ freeswitch-cc/
网关·业务服务·React       单进程·React            FS 媒体 + control-api
       │                       │                       ▲
       │     🚫 业务零调用      │                       │
       └───────────────────────┘              中台 HTTP 控制 / 事件回传
```

---

## 🖼️ 项目截图

| 🔐 登录页 | 📊 系统概览 |
| --- | --- |
| ![登录页](docs/screenshots/login.png) | ![系统概览](docs/screenshots/dashboard.png) |

| 👥 用户管理 | 🛡️ 角色管理 |
| --- | --- |
| ![用户管理](docs/screenshots/users.png) | ![角色管理](docs/screenshots/roles.png) |

| 🐘 数据库监控 | 🔴 Redis 监控 |
| --- | --- |
| ![数据库监控](docs/screenshots/mysql.png) | ![Redis 监控](docs/screenshots/redis.png) |

> 截图来自真实运行界面。UI 大改后可用本机截图覆盖 `docs/screenshots/*.png`。

---

## 🧰 技术栈全景

### 🐹 后端

| 层级 | 技术 | 说明 |
|------|------|------|
| 语言 | **Go 1.26** | 高性能、强类型 |
| HTTP | **Gin** | 路由与中间件 |
| ORM | **GORM** + **pgx** | PostgreSQL 访问 |
| 迁移 | **goose** | 版本化 SQL 迁移 |
| 认证 | **JWT v5** | Access / Refresh、吊销与轮转 |
| 缓存 | **Redis 7** | 限流、在线用户、黑名单等 |
| 数据库 | **PostgreSQL 16** | 主存储（pgvector 镜像便于 AI 扩展） |

### ⚛️ 前端

| 层级 | 技术 | 说明 |
|------|------|------|
| 框架 | **React 19** | 现代并发渲染 |
| 语言 | **TypeScript** | 类型安全 |
| 构建 | **Vite 8** | 极速开发与构建 |
| UI | **Ant Design 6** | 企业级组件库 |
| 状态 | **Redux Toolkit** | 可预测状态 |
| 路由 | **React Router 7** | 客户端路由 |
| 请求 | **Axios** | 拦截器 / Token 刷新 |

### 🧩 微服务架构（`microservices/`）

| 组件 | 技术 | 说明 |
|------|------|------|
| 网关 | **Traefik** | 路由、ForwardAuth 统一验签 |
| 消息 | **NATS JetStream** | 登录等事件解耦 |
| 服务 | auth / identity / system / audit / file / ai / **monitor** | 按域拆分 |
| 契约 | **OpenAPI 3.1** | 从路由生成 + 前端类型 |

### 🔭 可观测与存储（可选）

| 组件 | 技术 |
|------|------|
| 指标 | **Prometheus** 📈 |
| 看板 | **Grafana** 📊 |
| 链路 | **OpenTelemetry** + Jaeger 🔭 |
| 对象存储 | **MinIO**（S3 兼容） 📦 |

### ⚙️ 工程化

| 项 | 技术 |
|----|------|
| 容器 | **Docker Compose** 🐳 |
| CI | **GitHub Actions** ⚙️ |
| 提交规范 | **全中文** Conventional 风格（见 `CONTRIBUTING.md`） |
| 协作 | `AGENTS.md` 人与 AI 共用规范 🤖 |

---

## ✅ 功能矩阵

| 能力 | 微服务 | 单体 |
|------|:------:|:----:|
| 🔐 登录 / JWT 刷新与撤销 / 验证码 / TOTP | ✅ | ✅ |
| 🛡️ RBAC（用户、角色、权限、部门、菜单） | ✅ | ✅ |
| 📚 字典、公告、系统设置、在线用户 | ✅ | ✅ |
| 📝 登录日志 / 操作日志 / 审计日志 | ✅ | ✅ |
| 📁 文件上传 | ✅ | ✅ |
| 🖥️ 服务器 / PostgreSQL / Redis / 定时任务监控 | ✅ | ✅ |
| ❤️ 健康检查、Prometheus metrics | ✅ | ✅ |
| 🚪 Traefik 网关 + ForwardAuth | ✅ | — |
| 📡 NATS 登录事件 | ✅ | — |
| 🤖 AI 对话 / 知识库 | ✅ | — |
| 🐳 Docker Compose 一键启动 | ✅ | ✅ |

---

## 🗺️ 微服务拓扑

```text
Browser 🌐
   │
   ▼
Traefik :8000 🚪
   ├─ 🔐 认证路径 ──────► auth-service
   ├─ 👥 用户/角色/… ───► identity-service
   ├─ 📚 菜单/字典/… ───► system-service
   ├─ 📝 日志 ──────────► audit-service
   ├─ 📁 文件/uploads ──► file-service
   ├─ 🤖 AI ────────────► ai-service
   └─ 其余 /api ────────► monitor-service（健康·监控·迁移·兜底）
         ▲
         │ ForwardAuth ──► auth-service /internal/verify
```

---

## 📂 仓库结构

```text
go-admin-kit/
├── microservices/                 # 🧩 微服务产品线
│   ├── services/
│   │   ├── auth/                  # 🔐 认证、令牌、网关验签
│   │   ├── identity/              # 👥 用户 / 角色 / 权限 / 部门
│   │   ├── system/                # 📚 菜单 / 字典 / 公告 / 设置
│   │   ├── audit/                 # 📝 日志与事件消费
│   │   ├── file/                  # 📁 文件与 uploads
│   │   ├── ai/                    # 🤖 AI 对话与知识库
│   │   └── monitor/               # 📈 监控、健康、共享迁移、兜底
│   ├── web/                       # ⚛️ React + Ant Design
│   ├── docker-compose.yml
│   └── README.md
├── monolith/                      # 📦 单体产品线（零调用微服务）
│   ├── server/                    # 🐹 完整单进程 API
│   ├── web/                       # ⚛️ React + Ant Design（独立副本）
│   ├── docker-compose.yml
│   └── README.md
├── freeswitch-cc/                 # ☎️ 呼叫媒体（FS + control-api，中台可控）
├── platform/deploy/               # 🔭 Prometheus / Grafana / OTel
├── docs/                          # 📖 工程文档
│   ├── PRODUCT_LINES.md
│   └── design/                    # IM / FreeSWITCH 等专项设计
├── CONTRIBUTING.md                # 🤝 贡献与提交规范
├── AGENTS.md                      # 🤖 人与 AI 协作规范
└── LOCAL_SETUP.md                 # 💻 本地联调摘要
```

---

## 🚀 快速开始

### 📋 环境要求

- 🐳 [Docker Desktop](https://www.docker.com/products/docker-desktop/)
- 可选本地开发：🐹 Go **1.26.3+**、📦 Node.js **20.19+ / 22.12+**（推荐 24）、npm

### 🧩 微服务版

```bash
git clone https://github.com/SuperiorChuo/go-admin-kit.git
cd go-admin-kit/microservices
cp .env.example .env
docker compose up -d --build
# 或仓库根目录：make compose-up
```

| 入口 | 地址 |
|------|------|
| 🚪 统一网关（推荐） | http://localhost:8000 |
| ⚛️ 前端直连 | http://localhost:3000 |
| ❤️ 健康检查 | http://localhost:8000/api/v1/health/ready |
| 🔐 认证服务调试 | http://localhost:8082 |

更多 👉 [microservices/README.md](microservices/README.md)

### 📦 单体版

```bash
cd go-admin-kit/monolith
cp .env.example .env
docker compose up -d --build
# 或仓库根目录：make mono-up
```

| 入口 | 地址 |
|------|------|
| ⚛️ 前端 | http://localhost:3001 |
| 🔌 API | http://localhost:18081 |
| ❤️ 健康检查 | http://localhost:18081/api/v1/health/ready |

更多 👉 [monolith/README.md](monolith/README.md)

### 🔑 默认账号（仅本地开发）

| 用户名 | 密码 |
|--------|------|
| `admin` | `admin123` |

> ⚠️ 生产环境请立即修改密码，并替换 `.env` 中的 `JWT_SECRET`、数据库与中间件密钥。

---

## 🔌 端口一览（默认可并行）

| 用途 | 微服务 | 单体 |
|------|--------|------|
| 前端 | `3000`（网关 `8000`） | `3001` |
| API | 经 `8000` / 调试 `8081+` | `18081` |
| PostgreSQL | `5432` | `5433` |
| Redis | `6379` | `6380` |

冲突时在对应目录 `.env` 中修改 `*_PORT` 即可。

---

## 💻 本地开发（可选）

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

完整联调说明 👉 [LOCAL_SETUP.md](LOCAL_SETUP.md)

---

## 🧪 验证

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

## 📁 配置与文档

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

## 🔒 安全提示

上线前请至少替换：

- 🔑 `JWT_SECRET`
- 🐘 PostgreSQL / 🔴 Redis / 📦 MinIO / 📊 Grafana 等密码与密钥
- 👤 默认管理员密码策略
- 🌐 `CORS_ALLOW_ORIGINS`

---

## 🤝 开源协作

<p>
  <a href="CONTRIBUTING.md"><img src="https://img.shields.io/badge/Contributing-欢迎贡献-brightgreen?logo=git&logoColor=white" alt="Contributing" /></a>
  <a href="AGENTS.md"><img src="https://img.shields.io/badge/AGENTS.md-人与%20AI%20规范-black?logo=openai&logoColor=white" alt="AGENTS" /></a>
  <a href="https://github.com/SuperiorChuo/go-admin-kit/issues"><img src="https://img.shields.io/badge/Issues-反馈问题-red?logo=github" alt="Issues" /></a>
</p>

- 贡献指南 👉 [CONTRIBUTING.md](CONTRIBUTING.md)（**提交标题与正文须全中文**）
- 人与 AI 协作 👉 [AGENTS.md](AGENTS.md)
- CI 👉 https://github.com/SuperiorChuo/go-admin-kit/actions

---

## 🛣️ 路线图与产品愿景

> 完整扩展蓝图见 **[`docs/EXPANSION_PLAN.md`](docs/EXPANSION_PLAN.md)** · IM 设计 **[`docs/design/im-service.md`](docs/design/im-service.md)** · 呼叫 **[`docs/design/freeswitch-cc.md`](docs/design/freeswitch-cc.md)**。

> **定位**：Go Admin Kit 不只是「CRUD 后台模板」，而是面向 **运营中台 / 客服与触达 / AI 增强** 的可扩展底座。  
> 当前已具备：认证鉴权、RBAC、系统管理、文件、监控，以及微服务线上的 **AI 服务雏形**。  
> 下列能力将以 **可插拔域服务** 方式演进——优先挂在 `microservices/`，单体线保持精简或按需同步核心能力。

### 🎯 为什么这些方向合适？

| 方向 | 是否契合 | 理由 |
|------|:--------:|------|
| 🤖 AI / 智能体 | ⭐⭐⭐⭐⭐ | 已有 `ai-service`、pgvector 镜像基础，可自然扩展知识库、工具调用、多模型 |
| 💬 IM 即时通讯 | ⭐⭐⭐⭐ | 已有 JWT/用户域/WebSocket 通知雏形；适合独立 `im-service` + 网关路由 |
| 🎧 智能客服 | ⭐⭐⭐⭐⭐ | = IM + AI + 工单 + 知识库，正是运营后台的核心场景 |
| ☎️ 呼叫中心 | ⭐⭐⭐⭐ | **FreeSWITCH 增强做独立项目**；本仓只做运营台与对接层，避免媒体栈污染脚手架 |
| 📱 小程序 / 多端 | ⭐⭐⭐⭐ | 共享 identity/auth；小程序是触达通道，B 端管理仍走本脚手架 |
| 📊 数据中台大屏 | ⭐⭐⭐ | 可复用监控与审计数据；大屏可另仓，管理配置放本仓 |

**建议原则**：核心脚手架保持通用；业务域（IM / 呼叫 / 小程序）以 **新微服务 + 网关标签** 接入，避免把单体再做成巨石。

---

### Phase 0 · 底座夯实（进行中 / 近期）

- [x] 双产品线目录：`microservices/` + `monolith/`
- [x] 微服务拆分：auth / identity / system / audit / file / ai / monitor
- [x] React + Ant Design 统一前端
- [x] Traefik 网关、ForwardAuth、NATS 事件
- [ ] 正式 Release（`v0.1.0`）+ Changelog
- [ ] Playwright E2E 纳入 GitHub Actions
- [ ] 生产部署指南（Linux / Nginx / HTTPS）
- [ ] 更多二次开发示例页与最佳实践文档

---

### Phase 1 · 🤖 AI 能力深化

| 规划项 | 说明 |
|--------|------|
| 🧠 多模型路由 | OpenAI / DeepSeek / 通义 / Ollama / 兼容接口统一编排 |
| 📚 企业知识库 2.0 | 文档解析、切片、向量检索、权限隔离（按部门/角色） |
| 🛠️ Agent 工具调用 | 查用户、查工单、查订单等「可审计」工具链 |
| 🧾 运营助手 | 公告生成、日志洞察、报表摘要（扩展现有 AI 页面） |
| 🔐 AI 安全 | 提示词注入防护、敏感数据脱敏、调用审计日志 |

> 主战场：`microservices/services/ai` + 管理端 AI 控制台。

---

### Phase 2 · 💬 IM 与 🎧 智能客服

| 规划项 | 说明 |
|--------|------|
| 💬 IM 基础 | 单聊/群聊、会话列表、已读回执、消息漫游（独立 `im-service`） |
| 📎 富媒体消息 | 图片、文件、语音（复用 file / MinIO） |
| 🛎️ 客服工作台 | 排队、转接、会话分配、坐席状态、快捷回复 |
| 🤖 人机协同 | 机器人预答 → 人工接管；会话摘要与质检 |
| 🏷️ 工单联动 | 会话一键转工单，状态回写管理后台 |
| 📡 实时通道 | WebSocket / SSE 统一网关鉴权 |

> 与现有 RBAC、用户中心、通知体系天然衔接。

---

### Phase 3 · ☎️ 呼叫中心（FreeSWITCH 增强 · **独立项目**）

| 规划项 | 说明 |
|--------|------|
| 🎛️ **媒体面** | 基于 **FreeSWITCH** 开源增强：拨号计划、IVR、录音、会议、队列等 |
| 📦 **monorepo 子目录** | 媒体代码在 **`freeswitch-cc/`**（与 micro/mono 并列），compose 独立，不编进业务微服务 |
| 🔌 **与中台关系** | 中台做 **B 端运营台** + 以后 `cc-adapter`；经 API/Webhook **控制** FS，事件回传话单 |
| 👤 坐席台 | 签入签出、示忙示闲、软电话状态（UI 在本仓，信令/媒体在 FS 项目） |
| 📋 话单与录音 | 话单入库、录音索引/回放权限；录音可落 MinIO |
| 📊 呼叫报表 | 接通率、通话时长、放弃率、坐席效能 |
| 🔁 全渠道汇聚 | 电话 + IM + 小程序留言 → 统一客户视图（客户主数据仍在 identity 域） |

> **边界**：FreeSWITCH 集群、编解码、中继对接、高可用媒体 👉 独立项目维护；  
> 本仓库不内嵌 FS 二进制，只保留 **管理 API + 控制台页面 + 对接适配层**（可选 `cc-adapter` 微服务）。

---

### Phase 4 · 📱 小程序与多端触达

| 规划项 | 说明 |
|--------|------|
| 📱 微信小程序 | C 端入口：自助查询、提交工单、在线客服 |
| 🔗 统一账号 | 与 identity/auth 打通（OAuth / 手机号 / openId 绑定） |
| 📣 订阅消息 | 工单进度、预约提醒等触达 |
| 🧩 多小程序/多租户 | 配置化 appId、品牌、菜单（可与后续 SaaS 化结合） |
| 🌐 H5 / App | 同一套 API，差异只在客户端 |

> 原则：**小程序是通道，B 端运营仍用 Go Admin Kit 后台。**

---

### Phase 5 · 🏢 平台化与生态

| 规划项 | 说明 |
|--------|------|
| 🏢 多租户 / SaaS | M1：共享库 + `tenant_id`、登录 `tenant_code`、租户 CRUD（见 `docs/design/multi-tenant.md`） |
| 🔌 插件市场 | 域服务按插件启用（AI / IM / CC …） |
| 📊 数据看板 | 客服效能、AI 采纳率、渠道转化 |
| 🌍 国际化 i18n | 管理端多语言 |
| 🧩 开放平台 | OpenAPI 对外、Webhook、第三方应用授权 |

---

### 🗓️ 推荐落地顺序（务实版）

```text
1) 底座稳定 + 正式版发布
2) AI 知识库 / 运营助手（已有 ai-service，ROI 最高）
3) IM + 智能客服工作台（强绑定运营场景）
4) 小程序作为 C 端触达
5) 呼叫中心：`freeswitch-cc/` 媒体就绪后，再在 microservices 接管理台 adapter
6) 多租户与开放平台
```

欢迎在 [Issues](https://github.com/SuperiorChuo/go-admin-kit/issues) 投票你最想要的能力 🗳️

---

## 📄 License

本项目基于 [MIT License](LICENSE) 开源。

如果这个项目对你有帮助，欢迎 ⭐ **Star** · 🍴 **Fork** · 🔧 **PR**，一起把它打造成更好用的企业中台脚手架！
