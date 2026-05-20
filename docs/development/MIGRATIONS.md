# 数据库迁移说明

本项目使用 `github.com/pressly/goose/v3` 管理数据库迁移，迁移命令封装在后端内置 CLI 中，不需要在全局安装 goose。

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

## 基线迁移

首个迁移文件是 `server/migrations/000001_init_go_admin_kit.sql`，由 `server/docs/go_admin_kit.sql` 的当前基线生成。

为了避免误伤已有本地库，基线迁移的 `Up` 部分是非破坏式的：

- 建表使用 `CREATE TABLE IF NOT EXISTS`
- 种子数据使用 `INSERT IGNORE INTO`
- `Up` 部分不包含 `DROP TABLE`

`Down` 部分会删除基线创建的表，只应该在明确需要回滚或重置迁移环境时使用。

## 新增迁移

新增表结构或数据修正时，先创建迁移文件：

```powershell
make migrate-create NAME=add_audit_index
```

然后编辑生成的 SQL 文件：

```sql
-- +goose Up
ALTER TABLE `wm_audit_log` ADD INDEX `idx_wm_audit_log_action` (`action`);

-- +goose Down
ALTER TABLE `wm_audit_log` DROP INDEX `idx_wm_audit_log_action`;
```

迁移文件应满足：

- `Up` 和 `Down` 都能重复理解，回滚路径清晰。
- 不把本地测试数据写入迁移，只写脚手架必须的基线数据或结构变更。
- 涉及已有数据变更时，先在本地备份数据库。

## 与旧 SQL 基线的关系

`server/docs/go_admin_kit.sql` 仍保留为一键初始化基线，方便 Docker 首次启动或手动导入。后续结构变更应优先写入 `server/migrations/`，并在必要时同步更新基线 SQL，避免新环境和迁移环境出现差异。
