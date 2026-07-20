<p align="center">
  <img src="docs/screenshots/wechat-group.jpg" alt="go-admin-kit 微信交流群二维码" width="420" />
</p>

# 🚀 Go Admin Kit · 微服务脚手架

<p align="center">
  <strong>✨ 企业级微服务后台脚手架 · 只含基础设施 · 开箱即用 ✨</strong><br/>
  🐹 Go + Gin &nbsp;·&nbsp; ⚛️ React + Ant Design &nbsp;·&nbsp; 🧩 Traefik 网关 + 7 服务
</p>

<p align="center">
  <a href="https://superiorchuo.github.io/go-admin-kit/"><strong>🖥️ 在线体验 Demo →</strong></a><br/>
  <sub>纯前端演示模式（假数据，任意账号可登录）；完整功能克隆后 <code>docker compose</code> 一键启动</sub>
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
  <img src="https://img.shields.io/badge/⚙️_GitHub%20Actions-2088FF?logo=githubactions&logoColor=white" alt="GitHub Actions" />
</p>

---

## ✨ 为什么选 Go Admin Kit 脚手架

| 亮点 | 说明 |
|:-----|:-----|
| 🧩 **纯脚手架** | 只含平台无关的基础设施服务（认证/RBAC/系统/日志/文件/监控），不带业务耦合，是干净的起点。 |
| ⚛️ **前端统一** | React + Ant Design 深空/亮色双主题，一套视觉语言（渐变卡片墙 + 抽屉）。 |
| 🐳 **开箱即用** | Docker Compose 一键拉起依赖、网关/服务与前端；内置 RBAC、日志、监控、迁移。 |
| 🏗️ **工程完备** | CI、OpenAPI、健康检查、Prometheus、可选链路追踪与对象存储。 |
| 🛠️ **易扩展** | 加业务能力 = 加一个微服务 + 网关标签，不污染底座。 |

---

## 🖼️ 项目截图

| 页面 | 🌌 深空暗色 | ☁️ 白蓝亮色 |
| --- | --- | --- |
| 🔐 登录页 | ![深空暗色登录页](docs/screenshots/login.png) | ![白蓝亮色登录页](docs/screenshots/login-light.png) |
| 📊 系统概览 | ![深空暗色系统概览](docs/screenshots/dashboard.png) | ![白蓝亮色系统概览](docs/screenshots/dashboard-light.png) |
| 👥 用户管理 | ![深空暗色用户管理](docs/screenshots/users.png) | ![白蓝亮色用户管理](docs/screenshots/users-light.png) |
| 🛡️ 角色管理 | ![深空暗色角色管理](docs/screenshots/roles.png) | ![白蓝亮色角色管理](docs/screenshots/roles-light.png) |
| 🐘 数据库监控 | ![深空暗色数据库监控](docs/screenshots/mysql.png) | ![白蓝亮色数据库监控](docs/screenshots/mysql-light.png) |
| 🔴 Redis 监控 | ![深空暗色 Redis 监控](docs/screenshots/redis.png) | ![白蓝亮色 Redis 监控](docs/screenshots/redis-light.png) |

> 截图来自真实运行界面，统一使用桌面视口。

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
| 数据库 | **PostgreSQL 16** | 主存储（pgvector 镜像便于扩展） |

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
| 消息 | **NATS JetStream** | 登录事件解耦 |
| 服务 | auth / identity / system / audit / file / monitor + shared | 按域拆分的基础设施服务 |
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

---

## ✅ 功能矩阵

| 能力 | 状态 |
|------|:----:|
| 🔐 登录 / JWT 刷新与撤销 / 验证码 / TOTP / OAuth | ✅ |
| 🛡️ RBAC（用户、角色、权限、部门、菜单） | ✅ |
| 🏢 多租户（共享库 + tenant_id，登录带租户码） | ✅ |
| 📚 字典、公告、系统设置（DB 热配置）、在线用户 | ✅ |
| 📝 登录日志 / 操作日志 / 审计日志 | ✅ |
| 📁 文件上传（MinIO / 本地） | ✅ |
| 🖥️ 服务器 / PostgreSQL / Redis / 定时任务监控 | ✅ |
| ❤️ 健康检查、Prometheus metrics | ✅ |
| 🚪 Traefik 网关 + ForwardAuth | ✅ |
| 📡 NATS 登录事件 | ✅ |
| 🐳 Docker Compose 一键启动 | ✅ |

---

## 🗺️ 架构总览

```mermaid
flowchart LR
    B["🌐 浏览器"] --> GW["🚪 Traefik 网关 :8000"]

    subgraph MS["🧩 microservices/"]
        AUTH["🔐 auth"]
        IDT["👥 identity"]
        SYS["📚 system"]
        AUD["📝 audit"]
        FILE["📁 file"]
        MON["📈 monitor · 迁移/兜底"]
    end

    GW -. "ForwardAuth 统一验签" .-> AUTH
    GW --> AUTH & IDT & SYS & AUD & FILE & MON

    subgraph INFRA["🗄️ 共享基础设施"]
        PG[("🐘 PostgreSQL 16 + pgvector")]
        RD[("🔴 Redis 7")]
        NATS["📡 NATS JetStream"]
        MC["📦 MinIO"]
    end

    MS --- PG & RD
    AUTH -- "登录事件" --> NATS -- "持久消费" --> AUD
    FILE --- MC
```

- 业务服务只信任网关注入的 `X-Auth-*` 头，宿主机端口默认只绑 loopback，外部流量一律经网关。
- `monitor` 持有 `PathPrefix(/api)` 兜底路由（priority 1），并负责共享 goose 迁移。

---

## 📂 仓库结构

```text
go-admin-kit/
├── microservices/                 # 🧩 微服务脚手架
│   ├── services/
│   │   ├── auth/                  # 🔐 认证、令牌、网关验签
│   │   ├── identity/              # 👥 用户 / 角色 / 权限 / 部门
│   │   ├── system/                # 📚 菜单 / 字典 / 公告 / 设置
│   │   ├── audit/                 # 📝 日志与事件消费
│   │   ├── file/                  # 📁 文件与 uploads
│   │   ├── shared/                # 🧰 跨服务共享包（日志/响应/脱敏）
│   │   └── monitor/               # 📈 监控、健康、共享迁移、兜底
│   ├── web/                       # ⚛️ React + Ant Design
│   ├── docker-compose.yml
│   └── README.md
├── platform/deploy/               # 🔭 Prometheus / Grafana / OTel
├── docs/                          # 📖 工程文档
│   ├── PRODUCT_LINES.md
│   └── design/                    # 数据边界 / 多租户等专项设计
├── CONTRIBUTING.md                # 🤝 贡献与提交规范
└── LOCAL_SETUP.md                 # 💻 本地联调摘要
```

---

## 🚀 快速开始

### 📋 环境要求

- 🐳 [Docker Desktop](https://www.docker.com/products/docker-desktop/)
- 可选本地开发：🐹 Go **1.26.3+**、📦 Node.js **20.19+ / 22.12+**（推荐 24）、npm

### 🧩 启动

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

### 🔑 默认账号（仅本地开发）

| 用户名 | 密码 |
|--------|------|
| `admin` | `admin123` |

> ⚠️ 生产环境请立即修改密码，并替换 `.env` 中的 `JWT_SECRET`、数据库与中间件密钥。

---

## 🔌 端口一览

| 用途 | 端口 |
|------|------|
| 前端 | `3000`（推荐走网关 `8000`） |
| API | 经网关 `8000` / 各服务调试 `8081+` |
| PostgreSQL | `5432` |
| Redis | `6379` |

冲突时在 `microservices/.env` 中修改 `*_PORT` 即可。

---

## 💻 本地开发（可选）

```bash
cd microservices
docker compose up -d go-admin-kit-postgres go-admin-kit-redis go-admin-kit-nats
cd services/auth && go run ./cmd
cd web && npm ci && npm run dev
```

完整联调说明 👉 [LOCAL_SETUP.md](LOCAL_SETUP.md)

---

## 🧪 验证

```bash
cd microservices
(cd services/monitor && go test ./... && go vet ./...)
(cd web && npm run lint && npm run build)
npm run test:smoke:unit && npm run test:contract
API_BASE_URL=http://127.0.0.1:8000/api/v1 npm run smoke:api
```

OpenAPI：

```bash
cd microservices
npm run api:contract
git diff --exit-code -- services/monitor/docs/openapi.json
```

---

## 📁 配置与文档

| 说明 | 路径 |
|------|------|
| 环境变量 | [`microservices/.env.example`](microservices/.env.example) |
| 迁移 / OpenAPI | `microservices/services/monitor/migrations/`、`docs/openapi.json` |
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

## 🛠️ 如何扩展业务

这是一个**干净的脚手架**。要加业务能力（如 IM、CRM、AI 等），推荐：

1. 在 `microservices/services/` 下新建一个业务微服务，加入 `go.work`。
2. 在 `docker-compose.yml` 补一个服务块 + Traefik 路由标签（`PathPrefix` + 可选 ForwardAuth）。
3. 迁移落到 `monitor/migrations/`（共享 goose 真源）。
4. 前端在 `web/src/pages/` 加页面、`api/` 加接口、菜单种子补条目。

> 原则：底座保持通用，业务域以「新微服务 + 网关标签」接入，避免把脚手架做成巨石。

---

## 🤝 开源协作

<p>
  <a href="CONTRIBUTING.md"><img src="https://img.shields.io/badge/Contributing-欢迎贡献-brightgreen?logo=git&logoColor=white" alt="Contributing" /></a>
  <a href="https://github.com/SuperiorChuo/go-admin-kit/issues"><img src="https://img.shields.io/badge/Issues-反馈问题-red?logo=github" alt="Issues" /></a>
</p>

- 贡献指南 👉 [CONTRIBUTING.md](CONTRIBUTING.md)（**提交标题与正文须全中文**）
- CI 👉 https://github.com/SuperiorChuo/go-admin-kit/actions

---

## 📄 License

本项目基于 [MIT License](LICENSE) 开源。

如果这个项目对你有帮助，欢迎 ⭐ **Star** · 🍴 **Fork** · 🔧 **PR**，一起把它打造成更好用的企业中台脚手架！
