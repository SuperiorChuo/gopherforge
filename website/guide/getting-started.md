# 快速上手（15 分钟）

GopherForge 是一套开源的企业级 Go 微服务后台管理系统脚手架。本文带你从零把全栈跑起来：网关 + 7 个 Go 服务 + React 前端 + PostgreSQL/Redis/NATS。

## 环境要求

- [Docker Desktop](https://www.docker.com/products/docker-desktop/)（唯一硬依赖）
- 可选本地开发：Go **1.26.3+**、Node.js **20.19+ / 22.12+**（推荐 24）

## 一键启动

```bash
git clone https://github.com/SuperiorChuo/gopherforge.git
cd gopherforge
cp microservices/.env.example microservices/.env
make compose-up
# 或在仓库根目录：make compose-up
```

首次构建约 3 分钟。完成后：

| 入口 | 地址 |
|------|------|
| 统一网关（推荐） | http://localhost:8000 |
| 前端直连 | http://localhost:3000 |
| 健康检查 | http://localhost:8000/api/v1/health/ready |

## 默认账号

| 用户名 | 密码 |
|--------|------|
| `admin` | `admin123` |

::: warning 上线必改
生产环境请立即修改默认密码，并替换 `.env` 中的 `JWT_SECRET`、数据库与中间件密钥。清单见[生产部署](/reference/deployment)。
:::

## 跑一遍验证

```bash
cd microservices
(cd services/monitor && go test ./... && go vet ./...)
(cd web && npm run lint && npm run build)
npm run test:smoke:unit && npm run test:contract
npm run api:contract
docker compose config --quiet
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml config --quiet
API_BASE_URL=http://127.0.0.1:8000/api/v1 npm run smoke:api
```

`smoke:api` 会用真实验证码识别器走完登录→接口冒烟全链路。

## 本地开发模式（可选）

只起依赖容器，服务与前端本机热更新：

下面的长时间运行命令请分别放在三个终端执行：

```bash
# 终端 1：依赖容器
cd microservices
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml up -d go-admin-kit-postgres go-admin-kit-redis go-admin-kit-nats

# 终端 2：要调试的 Go 服务（以 auth 为例）
cd microservices/services/auth
go run ./cmd

# 终端 3：前端 HMR
cd microservices/web
npm ci
npm run dev
```

## 端口冲突怎么办

默认端口：前端 `3000`、网关 `8000`、PostgreSQL `5432`、Redis `6379`。冲突时改 `microservices/.env` 里对应的 `*_PORT` 即可，compose 会读取。

## 下一步

- 了解服务是怎么拆的 → [架构总览](/guide/architecture)
- 直接加你的第一个业务模块 → [二次开发](/guide/extend)
- 逛一圈功能 → [功能模块](/modules/auth)，或先玩[在线 Demo](https://superiorchuo.github.io/gopherforge/)（纯前端假数据，任意账号可登录）
