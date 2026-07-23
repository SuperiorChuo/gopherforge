# 本地联调说明

本仓库两条产品线互不调用，请任选其一。

## 环境要求

- Go 1.26.3+
- Node.js 20.19+ 或 22.12+（推荐 24）
- npm、Docker Desktop

## 微服务版

有状态服务（PG/Redis/NATS/MinIO）在独立的 infra 栈（`docker-compose.infra.yml`），应用栈随便 down/up/rebuild 都不会碰数据：

```bash
cd microservices
cp .env.example .env
# 1. 共享网络（一次性）
docker network inspect go-admin-kit-net >/dev/null 2>&1 || \
  docker network create --subnet 172.28.0.0/16 go-admin-kit-net
# 2. 数据栈（要 MinIO 加 --profile storage）
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml up -d
# 3. 应用栈
docker compose up -d --build
```

- 网关：`http://localhost:8000`
- 迁移由 `migrate` 一次性 job 执行（PG 未就绪会自动重试）

本地只起依赖（数据栈就够了）：

```bash
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml up -d
cd services/auth && go run ./cmd
cd web && npm ci && npm run dev
```

验证：

```bash
cd microservices
npm run test:smoke:unit
npm run test:contract
API_BASE_URL=http://127.0.0.1:8000/api/v1 npm run smoke:api
```

## 单体版

```bash
cd monolith
cp .env.example .env
docker compose up -d --build
```

- 前端：`http://localhost:3001`
- API：`http://localhost:18081`

本地：

```bash
docker compose up -d go-admin-kit-mono-postgres go-admin-kit-mono-redis
cd server && go run ./cmd/main.go
cd web && npm ci && npm run dev
```

## 端口冲突

两条线默认端口已错开。若仍冲突，在各自 `.env` 中调整 `POSTGRES_PORT`、`REDIS_PORT`、`BACKEND_PORT`、`FRONTEND_PORT`、`GATEWAY_PORT`。

更多边界与能力对照：`docs/PRODUCT_LINES.md`。
