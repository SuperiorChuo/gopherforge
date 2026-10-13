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
# 共享网络（一次性）
docker network inspect go-admin-kit-net >/dev/null 2>&1 || \
  docker network create --subnet 172.28.0.0/16 go-admin-kit-net
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml up -d go-admin-kit-postgres go-admin-kit-redis
cd services/monitor && go run ./cmd/main.go
```

模块路径仍为 `github.com/go-admin-kit/server`（历史包名，避免无意义全量改 import）。
