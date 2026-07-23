# 微服务版（Go Admin Kit Microservices）

本目录是**可独立运行的微服务产品线**，与 `../monolith/` 业务零调用。

## 包含内容

| 路径 | 说明 |
|------|------|
| `services/auth` … `ai` | 业务微服务 |
| `services/monitor` | 监控 + 健康 + 共享迁移 + `/api` 兜底 |
| `web/` | React + Ant Design 前端 |
| `docker-compose.yml` | 应用栈：网关 + 服务 + 前端 |
| `docker-compose.infra.yml` | 有状态栈：PG / Redis / NATS / MinIO（独立 project，应用栈重建不触碰） |
| `go.work` | 本线 Go 工作区 |
| `tests/` / `scripts/` | 冒烟与契约 |

能力对照见 [`../docs/PRODUCT_LINES.md`](../docs/PRODUCT_LINES.md)。

## 快速启动

```bash
cp .env.example .env
make -C .. compose-up     # 自动：共享网络 → infra 数据栈 → 应用栈
```

等价的显式命令：

```bash
# 1. 共享网络（一次性）
docker network inspect go-admin-kit-net >/dev/null 2>&1 || \
  docker network create --subnet 172.28.0.0/16 go-admin-kit-net
# 2. 有状态栈（PG/Redis/NATS；要 MinIO 加 --profile storage）
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml up -d
# 3. 应用栈
docker compose up -d --build
```

- 网关：`http://localhost:8000`
- 前端：`http://localhost:3000`
- 开发账号：`admin` / `admin123`（仅本地）

## 验证

```bash
npm run test:smoke:unit
npm run test:contract
API_BASE_URL=http://127.0.0.1:8000/api/v1 npm run smoke:api
```
