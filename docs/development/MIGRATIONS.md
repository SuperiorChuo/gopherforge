# 数据库迁移说明

本项目使用 `github.com/pressly/goose/v3` 管理数据库迁移。迁移命令封装在后端内置 CLI 中，无需全局安装 goose。

## 真源路径

| 产品线 | 迁移目录 |
|--------|----------|
| 微服务 | `microservices/services/monitor/migrations/` |
| 单体 | `monolith/server/migrations/` |

Docker 启动对应 backend/monitor 容器时会幂等执行 `migrate up`。

## 常用命令

**微服务**（在 `microservices/` 下）：

```bash
make migrate-status
make migrate-up
make migrate-create NAME=add_example_table
```

或：

```bash
cd microservices/services/monitor
go run ./cmd/migrate -config ./configs/config.yaml -dir ./migrations status
go run ./cmd/migrate -config ./configs/config.yaml -dir ./migrations up
```

**单体**（在 `monolith/server/` 下）：

```bash
go run ./cmd/migrate -config ./configs/config.yaml -dir ./migrations status
go run ./cmd/migrate -config ./configs/config.yaml -dir ./migrations up
```

## 迁移演练

发布前建议在一次性数据库上完整跑 `Up -> Down -> Up`：

```bash
cd microservices/services/monitor   # 或 monolith/server
go run ./cmd/migration-rehearsal -config ./configs/config.yaml -dir ./migrations -database go_admin_kit_migration_rehearsal
```

`migration-rehearsal` 会创建临时库执行 `up`、`down-to 0`、再次 `up`，默认结束后删除临时库。数据库名只允许字母、数字和下划线，并拒绝系统库名；`APP_ENV=production` 下会拒绝运行。
