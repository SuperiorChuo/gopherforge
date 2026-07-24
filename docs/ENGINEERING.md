# 工程说明

本仓库含 **微服务** 与 **单体** 两条独立产品线，边界见 `docs/PRODUCT_LINES.md`。

## 协作与提交

- 提交信息要求**标题与正文均为中文**，规范见 `CONTRIBUTING.md` 与 `AGENTS.md`。

## 微服务后端边界（`microservices/`）

- `services/auth|identity|system|audit|file|bpm`：基础微服务
- `services/monitor`：监控、健康、metrics、共享 goose 迁移、网关 `/api` 兜底
- `web/`：React + Ant Design 前端

## 单体后端边界（`monolith/`）

- `server/`：单进程完整 API
- `web/`：React + Ant Design 前端（独立副本，不依赖微服务）

## 验证

```bash
# 微服务
cd microservices
cd services/monitor && go test ./...
cd ../auth && go test ./...
cd ../../web && npm run lint && npm run build

# 单体
cd monolith
cd server && go test ./...
cd ../web && npm run lint && npm run build
```
