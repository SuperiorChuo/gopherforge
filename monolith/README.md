# 单体版（Go Admin Kit Monolith）

本目录是**独立的单体应用产品线**，与 `../microservices/` **业务零调用**（不依赖网关、不依赖 `services/*`）。

## 包含内容

| 路径 | 说明 |
|------|------|
| `server/` | 完整单体 Go API（认证/RBAC/系统/监控等单进程） |
| `web/` | React + Ant Design 前端（与微服务同技术栈，独立副本） |
| `docker-compose.yml` | postgres + redis + server + web（无 Traefik 多服务） |
| `.env.example` | 本线环境变量 |

默认端口与微服务错开，可同机并行：

| 服务 | 默认宿主机端口 |
|------|----------------|
| 前端 | `3001` |
| 后端 | `18081` |
| PostgreSQL | `5433` |
| Redis | `6380` |

## 快速启动

```bash
cd monolith
cp .env.example .env
docker compose up -d --build
```

- 前端：`http://localhost:3001`
- API：`http://localhost:18081/api/v1/health/ready`
- 开发账号：`admin` / `admin123`（仅本地）

## 本地开发

```bash
docker compose up -d go-admin-kit-mono-postgres go-admin-kit-mono-redis
# 配置 server 指向本机映射端口后：
cd server && go run ./cmd/main.go
cd web && npm ci && npm run dev
```

## 与微服务的差异

| 项 | 单体 | 微服务 |
|----|------|--------|
| 后端 | 单进程 `server/` | 多 `services/*` + 网关 |
| 前端入口 | 直连 backend | 经 Traefik |
| AI 能力 | 本线未内置 | `services/ai` + 前端 AI 页 |
| 数据卷/网络 | `go-admin-kit-mono-*` | `go-admin-kit-*` |

## 代码来源说明

- `server/`：取自拆分微服务之前的完整单体快照（PostgreSQL 迁移后、auth-service 拆出前）。
- `web/`：自当前微服务前端复制并去掉 AI 路由，代理改为单体 API。

二者之后各自演进，互不 import。
