# monitor-service

微服务线中的监控与兜底服务（原 `legacy-backend`）。

## 职责

- 服务器 / PostgreSQL / Redis / 定时任务监控 API
- 健康检查与 Prometheus metrics
- **全栈共享 goose 迁移**（库表与种子数据）
- Traefik 上作为 `/api`、`/uploads` 的较低优先级兜底（业务路由由各服务高优先级抢流）

## 本地运行

```bash
# 在 microservices/ 下
docker compose up -d go-admin-kit-postgres go-admin-kit-redis
cd services/monitor && go run ./cmd/main.go
```

模块路径仍为 `github.com/go-admin-kit/server`（历史包名，避免无意义全量改 import）。
