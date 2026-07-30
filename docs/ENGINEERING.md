# 工程说明

本仓库是 **GopherForge 微服务脚手架**（单一产品线）：范围与更新方式见
`docs/sync-policy.md`，能力清单见 `docs/PRODUCT_LINES.md`。

## 协作与提交

- 提交信息要求**标题与正文均为中文**，规范见 `CONTRIBUTING.md`。

## 后端边界（`microservices/`）

- `services/auth|identity|system|audit|file|bpm`：基础微服务
- `services/monitor`：监控、健康、metrics、共享 goose 迁移、网关 `/api` 兜底
- `services/shared`：跨服务共享库（配置、中间件、遥测等），非独立进程
- `web/`：React + Ant Design 前端

## 验证

```bash
cd microservices
cd services/monitor && go test ./...
cd ../auth && go test ./...
cd ../../web && npm run lint && npm run build
```
