# 微服务版（Go Admin Kit Microservices）

本目录是**可独立运行的微服务产品线**，与 `../monolith/` 业务零调用。

## 包含内容

| 路径 | 说明 |
|------|------|
| `services/auth` … `ai` | 业务微服务 |
| `services/monitor` | 监控 + 健康 + 共享迁移 + `/api` 兜底 |
| `web/` | React + Ant Design 前端 |
| `docker-compose.yml` | 网关 + 依赖 + 服务 + 前端 |
| `go.work` | 本线 Go 工作区 |
| `tests/` / `scripts/` | 冒烟与契约 |

能力对照见 [`../docs/PRODUCT_LINES.md`](../docs/PRODUCT_LINES.md)。

## 快速启动

```bash
cp .env.example .env
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
