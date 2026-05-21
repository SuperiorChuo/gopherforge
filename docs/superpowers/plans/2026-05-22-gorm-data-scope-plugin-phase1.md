# GORM 数据权限插件第一阶段 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `User / File / LoginLog / OperationLog` 的首批读查询引入显式启用的 GORM 数据权限插件，在不破坏例外查询语义的前提下移除重复的手工 `ApplyUserEntityScope` / `ApplyOwnerScope`。

**Architecture:** 在 `authz` 中新增一个只处理读查询的 GORM callback plugin，并通过 `context.Context` 上的 directive 显式启用；`database.InitDatabase()` 注册插件，但插件默认 no-op；目标 DAO 在 scoped 方法内部把现有 `UserDataScope` 转成 context directive，然后让 callback 自动补 where。

**Tech Stack:** Go 1.26、GORM callbacks/plugin、现有 `authz.UserDataScope`、现有 dry-run/sqlmock 测试模式。

---

## 文件结构

- 新建 `server/internal/pkg/authz/data_scope_plugin.go`
  - plugin、directive helper、模型映射、callback 逻辑。
- 新建 `server/internal/pkg/authz/data_scope_plugin_test.go`
  - dry-run SQL 级测试：enabled/disabled/self/all/unsupported/subquery recursion。
- 修改 `server/internal/pkg/database/database.go`
  - 注册 plugin。
- 修改 `server/internal/dao/system/data_scope_test.go`
  - 让测试 DB 注册 plugin，继续断言 DAO SQL。
- 修改以下 DAO：
  - `server/internal/dao/system/user.go`
  - `server/internal/dao/system/file.go`
  - `server/internal/dao/system/login_log.go`
  - `server/internal/dao/system/operation_log.go`
- 如有必要，修改以下 API/Service 调用面中的 self-scope 路径以统一 helper 用法：
  - `server/internal/api/system/file.go`
  - `server/internal/api/system/login_log.go`

## Task 1: 写 plugin 失败测试

**Files:**
- Create: `server/internal/pkg/authz/data_scope_plugin_test.go`

- [ ] **Step 1: 写 dry-run 测试，固定 directive helper API**

新增测试，明确这些 helper：

```go
func EnableDataScope(ctx context.Context, scope UserDataScope) context.Context
func DisableDataScope(ctx context.Context) context.Context
func ForceSelfScope(ctx context.Context, userID uint) context.Context
func RegisterDataScopePlugin(db *gorm.DB) error
```

核心测试：

```go
func TestDataScopePluginNoDirectiveNoOps(t *testing.T) {}
func TestDataScopePluginScopesUserQueries(t *testing.T) {}
func TestDataScopePluginScopesOwnerQueries(t *testing.T) {}
func TestDataScopePluginDisableDirectiveNoOps(t *testing.T) {}
func TestDataScopePluginForceSelfScope(t *testing.T) {}
```

其中：
- `User` 期望 SQL 走 `department_id IN (?,?)` 或 `id = ?`
- `File` 期望 SQL 走 `user_id IN (SELECT id FROM users ...)` 或 `user_id = ?`
- unsupported model 必须保持原 SQL

- [ ] **Step 2: 加一个子查询防递归测试**

```go
func TestDataScopePluginOwnerScopeSubqueryDoesNotReenterPlugin(t *testing.T) {}
```

目标是验证 `File` 的 department scope SQL 仍然只有一层 `users` 子查询，而不会在子查询里再次套 scope。

- [ ] **Step 3: 运行 authz plugin 测试并确认失败**

Run: `go test ./internal/pkg/authz -run "TestDataScopePlugin" -count=1`

Expected: FAIL，报 helper/plugin 未定义。

## Task 2: 实现 plugin 与 directive helper

**Files:**
- Create: `server/internal/pkg/authz/data_scope_plugin.go`
- Test: `server/internal/pkg/authz/data_scope_plugin_test.go`

- [ ] **Step 1: 实现 directive helper**

建议内部结构：

```go
type dataScopeDirective struct {
	enabled  bool
	disabled bool
	scope    UserDataScope
}
```

语义：
- context 中没有 directive => no-op
- `DisableDataScope` => no-op
- `EnableDataScope` => enabled，且 zero-value `scope.Scope == ""` 视为 `DataScopeNone`
- `ForceSelfScope` => enabled + `DataScopeSelf`

- [ ] **Step 2: 实现 plugin callback**

建议：

```go
type DataScopePlugin struct{}

func NewDataScopePlugin() *DataScopePlugin
func RegisterDataScopePlugin(db *gorm.DB) error
```

callback 规则：
- 只挂 `Query`
- `Statement == nil` / `Schema == nil` / ctx 无 directive => no-op
- `directive.disabled` => no-op
- 仅支持：
  - `model.User` => `ApplyUserEntityScope(..., "id", "department_id")`
  - `model.File` / `model.LoginLog` / `model.OperationLog` => `ApplyOwnerScope(..., "user_id")`
- `joins` / alias / unsupported schema => no-op

- [ ] **Step 3: 修正 `ApplyOwnerScope` 子查询防递归**

在内部子查询上显式禁用 directive，避免 plugin 递归套用到 `SELECT id FROM users ...`。

- [ ] **Step 4: 运行 authz plugin 测试**

Run: `go test ./internal/pkg/authz -run "TestDataScopePlugin" -count=1`

Expected: PASS

## Task 3: 注册 plugin 并接入 DAO 测试 DB

**Files:**
- Modify: `server/internal/pkg/database/database.go`
- Modify: `server/internal/dao/system/data_scope_test.go`

- [ ] **Step 1: 在数据库初始化中注册 plugin**

在 `gorm.Open(...)` 后、`DB = db` 前：

```go
if err := authz.RegisterDataScopePlugin(db); err != nil {
	return fmt.Errorf("register data scope plugin: %w", err)
}
```

- [ ] **Step 2: 在 DAO SQLMock 测试 DB 上也注册 plugin**

`setupSystemDAOTestDB(t)` 中测试 DB 创建后调用 `authz.RegisterDataScopePlugin(db)`，否则迁移后的 DAO scoped 方法不会生效。

- [ ] **Step 3: 跑现有 DAO data scope 测试并确认仍然失败/或开始失败在旧实现上**

Run: `go test ./internal/dao/system -run "Test(UserDAOGetUserList|FileDAOGetByIDInScope|FileDAOGetByHashInScope|FileDAOGetList|LoginLogDAOGetList|OperationLogDAOGetLogList)" -count=1`

Expected before DAO migration:
- 可能 FAIL，因为 DAO 还在手工 scope，或因为测试 DB 还未注册 plugin。

## Task 4: 迁移 `UserDAO` 和 `FileDAO`

**Files:**
- Modify: `server/internal/dao/system/user.go`
- Modify: `server/internal/dao/system/file.go`

- [ ] **Step 1: `UserDAO.GetUserListContext` 改为 context opt-in**

从：

```go
query := d.dbWithContext(ctx).Model(&model.User{})
query = authz.ApplyUserEntityScope(query, dataScope, "id", "department_id")
```

改为：

```go
scopedCtx := authz.EnableDataScope(ctx, dataScope)
query := d.dbWithContext(scopedCtx).Model(&model.User{})
```

- [ ] **Step 2: `FileDAO` scoped 读查询改为 context opt-in**

替换这些方法里的手工 `ApplyOwnerScope`：
- `GetByIDInScopeContext`
- `GetByHashInScopeContext`
- `GetListContext`

写法同上，使用 `EnableDataScope(ctx, dataScope)`。

- [ ] **Step 3: `GetStatsInScopeContext` / `getStatsContext` 收敛**

保留：
- `GetStatsContext`：plain ctx，不启 plugin
- `GetStatsInScopeContext`：先 `EnableDataScope(ctx, dataScope)` 再进入内部查询

确保两条聚合查询都使用同一个 scoped ctx。

- [ ] **Step 4: 跑 DAO 测试**

Run: `go test ./internal/dao/system -run "TestUserDAOGetUserListAppliesDepartmentScope|TestFileDAOGetByIDInScopeAppliesOwnerDepartmentScope|TestFileDAOGetByHashInScopeAppliesSelfScope|TestFileDAOGetListAppliesNoScope" -count=1`

Expected: PASS

## Task 5: 迁移 `LoginLogDAO` 和 `OperationLogDAO`

**Files:**
- Modify: `server/internal/dao/system/login_log.go`
- Modify: `server/internal/dao/system/operation_log.go`

- [ ] **Step 1: `LoginLogDAO.GetListContext` 改为 context opt-in**

删除手工 `ApplyOwnerScope`，改为：

```go
scopedCtx := authz.EnableDataScope(ctx, dataScope)
query := d.dbWithContext(scopedCtx).Model(&model.LoginLog{})
```

- [ ] **Step 2: `OperationLogDAO.GetLogListContext` 改为 context opt-in**

同理替换手工 `ApplyOwnerScope`。

- [ ] **Step 3: 跑 DAO 测试**

Run: `go test ./internal/dao/system -run "TestLoginLogDAOGetListAppliesDepartmentScopeAndFilters|TestOperationLogDAOGetLogListAppliesSelfScope" -count=1`

Expected: PASS

## Task 6: self-scope API 收敛与全量回归

**Files:**
- Modify if needed: `server/internal/api/system/file.go`
- Modify if needed: `server/internal/api/system/login_log.go`

- [ ] **Step 1: 评估 `GetMyFiles` / `GetMyLoginLogs` 是否要改成 `ForceSelfScope`**

如果 DAO scoped 方法已经统一走 `EnableDataScope(ctx, dataScope)`，而 API 仍通过 request struct 传 `DataScopeSelf`，可以先保持不变；如要进一步收敛，则改成：

```go
ctx := authz.ForceSelfScope(c.Request.Context(), uid)
```

并去掉手工 `req.DataScope = ...`。

- [ ] **Step 2: 跑目标 service/API 测试**

Run: `go test ./internal/service/system ./internal/api/system -count=1`

Expected: PASS

- [ ] **Step 3: 跑 openapi 与全量后端测试**

Run: `go test ./internal/openapi ./internal/pkg/authz ./internal/dao/system -count=1`

Run: `go test ./... -count=1`

Expected: PASS

- [ ] **Step 4: 格式化与差异检查**

Run: `gofmt -w internal/pkg/authz/*.go internal/pkg/database/*.go internal/dao/system/*.go internal/api/system/*.go`

Run: `git diff --check`

Expected: no output

## 自检

- plan 只迁移首批已手工 scope 的四类读查询，不碰 write 路径。
- plan 显式保留 file hash 去重、日志 stats/trend/detail 等例外查询。
- plugin 默认 no-op，只在 DAO 内部显式 `EnableDataScope(...)` 时生效。
- 子查询递归问题有单独测试与显式禁用方案。
