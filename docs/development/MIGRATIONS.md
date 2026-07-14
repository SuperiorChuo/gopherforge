# 数据库迁移说明

本项目使用 `github.com/pressly/goose/v3` 管理数据库迁移。迁移命令封装在后端内置 CLI 中，不需要在全局安装 goose。

## 常用命令

在项目根目录执行：

```powershell
make migrate-status
make migrate-up
make migrate-create NAME=add_example_table
```

在 `server/` 目录执行：

```powershell
go run ./cmd/migrate -config ./configs/config.yaml -dir ./migrations status
go run ./cmd/migrate -config ./configs/config.yaml -dir ./migrations up
go run ./cmd/migrate -dir ./migrations create add_example_table sql
```

## 迁移演练

发布前建议在一次性数据库上完整跑一遍 `Up -> Down -> Up`，验证所有迁移都有可回滚路径：

```powershell
cd server
go run ./cmd/migration-rehearsal -config ./configs/config.yaml -dir ./migrations -database go_admin_kit_migration_rehearsal
```

也可以通过 Makefile 执行：

```powershell
cd server
make migrate-rehearse
```

`migration-rehearsal` 会使用当前 PostgreSQL 连接配置创建临时库，执行 `up`、`down-to 0`、再次 `up`，默认结束后删除临时库。数据库名只允许字母、数字和下划线，且会拒绝 `postgres`、`template0`、`template1`、`information_schema` 等系统库名；命令也会拒绝在 `APP_ENV=production` 下运行。

## 基线迁移

首个迁移文件是 `server/migrations/000001_init_go_admin_kit.sql`，用于创建项目初始表结构和基础数据。

为避免误伤已有本地库，基线迁移的 `Up` 部分必须保持非破坏式：

- 建表使用 `CREATE TABLE IF NOT EXISTS`。
- 种子数据使用 `INSERT ... ON CONFLICT DO NOTHING`。
- `Up` 部分不包含 `DROP TABLE`。

`Down` 部分会删除基线创建的表，只应在明确需要回滚或重置迁移环境时使用。

## 新增迁移

新增表结构或数据修正时，先创建迁移文件：

```powershell
make migrate-create NAME=add_audit_index
```

然后编辑生成的 SQL 文件：

```sql
-- +goose Up
CREATE INDEX idx_audit_logs_action ON audit_logs (action);

-- +goose Down
DROP INDEX idx_audit_logs_action;
```

迁移文件应满足：

- `Up` 和 `Down` 都能清楚表达变更与回滚路径。
- 不把本地测试数据写入迁移，只写脚手架必须的基线数据或结构变更。
- 涉及已有数据变更时，先在本地或预发环境备份数据库并验证回滚路径。
- 为已有表新增唯一索引时，迁移脚本必须先处理历史重复数据，避免生产库在 `CREATE UNIQUE INDEX` 阶段失败；例如 `000008_add_oauth_binding_user_provider_unique.sql` 会先删除同一 `user_id + provider` 下较旧的重复 OAuth 绑定，再增加唯一索引。

## SQL 快照

`server/docs/go_admin_kit.sql` 保留为手动初始化快照；Docker 后端容器会在主服务启动前执行 `server/migrations/` 下的 goose 迁移。

正常部署建议优先使用 goose 迁移路径。若手动导入 `server/docs/go_admin_kit.sql` 快照，不要再对同一个库重复跑完整迁移链，除非已经同步 goose 版本表状态。
该快照面向离线初始化和人工参考，不保证填充目标库的 goose 版本表；导入快照后继续执行 `goose up` 可能触发重复建表、重复加列或重复索引错误。

后续结构变更应优先写入 `server/migrations/`，并在必要时同步更新 SQL 快照，避免手动导入路径和迁移路径出现结构差异。
