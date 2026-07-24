# 微服务版（GopherForge Microservices）

本目录是 **GopherForge** 的微服务脚手架发行线（曾用名 `go-admin-kit`），与 `../monolith/` 业务零调用。

> 当前发布候选版：`v0.2.0-rc.1`。0.x 期间 API、数据库表结构和生成代码格式可能变化；业务域不随本发行线引入。

## 包含内容

| 路径 | 说明 |
|------|------|
| `services/auth`、`identity`、`system`、`audit`、`file`、`monitor`、`bpm` | 7 个基础微服务 |
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
docker compose config --quiet
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml config --quiet
API_BASE_URL=http://127.0.0.1:8000/api/v1 npm run smoke:api
```

OpenAPI 契约生成与检查：

```bash
npm run api:contract
git diff --exit-code -- services/monitor/docs/openapi.json web/src/api/generated/schema.d.ts
```

完整教程、模块说明和生产部署清单见
[GopherForge 文档站](https://superiorchuo.github.io/gopherforge/docs/)；在线 Demo 是纯前端假数据，不能替代后端集成验证。
