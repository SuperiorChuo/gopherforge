# System Context Wrapper Cleanup Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**目标：** 清理 `server/internal/dao/system` 与 `server/internal/service/system` 中已经迁移到 `*Context` 版本的 Deprecated 兼容 wrapper。
**架构：** Phase 1 只处理 system service/DAO：先把测试调用点迁移到 `*Context(context.Background(), ...)`，再删除纯 `context.Background()` 兼容壳。架构测试从“允许 wrapper 但必须写 `Deprecated:` 注释”调整为“system service/DAO 禁止新增 legacy context wrapper”。
**技术栈：** Go 1.26、标准库 `context`、`go test`、`gofmt`。

---

## 文件结构

- Modify: `server/internal/dao/system/*.go`
  - 删除无生产调用的 Deprecated wrapper。
  - 保留真实业务方法、`*Context` 方法和非 context wrapper。
- Modify: `server/internal/dao/system/*_test.go`
  - 将仍调用 legacy wrapper 的测试迁移到 `*Context(context.Background(), ...)`。
- Modify: `server/internal/service/system/*.go`
  - 删除无生产调用的 Deprecated wrapper。
  - 保留真实业务方法、`*Context` 方法和新功能文件。
- Modify: `server/internal/service/system/*_test.go`
  - 将仍调用 legacy wrapper 的测试迁移到 `*Context(context.Background(), ...)`。
- Modify: `server/internal/architecture/legacy_context_wrappers_test.go`
  - 新增 system service/DAO 禁止 legacy context wrapper 的断言。
  - 继续保留非 system 区域的 `Deprecated:` 注释约束。
  - 识别直接传入 `context.Background()`、先赋给局部变量再传入 `*Context`，以及简单别名传播的写法。
  - 对 `AuditLogService.Record -> RecordContext` 保留绑定 receiver 类型的显式白名单，因为它是 `gin.Context` 到 `context.Context` 的桥接方法。
- Modify: `docs/development/OPTIMIZATION_STATUS.md`
  - 记录 Phase 1 已收敛 system service/DAO wrapper，非 system 区域进入后续阶段。

---

## Task 1: DAO System 清理

**Files:**
- Modify: `server/internal/dao/system/audit_log.go`
- Modify: `server/internal/dao/system/department.go`
- Modify: `server/internal/dao/system/dict.go`
- Modify: `server/internal/dao/system/file.go`
- Modify: `server/internal/dao/system/login_log.go`
- Modify: `server/internal/dao/system/menu.go`
- Modify: `server/internal/dao/system/menu_seed.go`
- Modify: `server/internal/dao/system/notice.go`
- Modify: `server/internal/dao/system/operation_log.go`
- Modify: `server/internal/dao/system/permission.go`
- Modify: `server/internal/dao/system/permission_cache.go`
- Modify: `server/internal/dao/system/role.go`
- Modify: `server/internal/dao/system/user.go`
- Modify tests in `server/internal/dao/system/*_test.go`

- [x] **Step 1: 迁移测试调用点**

将测试中的 legacy 调用替换为 `*Context(context.Background(), ...)`：
- `DepartmentDAO.GetAll`
- `FileDAO.GetByIDInScope`
- `FileDAO.GetByHashInScope`
- `FileDAO.GetList`
- `LoginLogDAO.GetList`
- `MenuDAO.GetMenuTree`
- `OperationLogDAO.GetLogList`
- `PermissionManageDAO.GetPermissionTree`
- `FindUserIDsByRoleIDs`
- `FindRoleIDsByPermissionIDs`
- `RoleDAO.GetRoleByCode`
- `UserDAO.GetUserList`

- [x] **Step 2: 删除 DAO wrapper**

删除 `server/internal/dao/system` 下所有只调用 `context.Background()` 的 Deprecated wrapper，保留对应的 `*Context(ctx context.Context, ...)` 方法。

- [x] **Step 3: 验证 DAO system**

Run:

```bash
go test ./internal/dao/system -count=1
```

Expected: PASS。

---

## Task 2: Service System 清理

**Files:**
- Modify: `server/internal/service/system/audit_log.go`
- Modify: `server/internal/service/system/cache.go`
- Modify: `server/internal/service/system/department.go`
- Modify: `server/internal/service/system/dict.go`
- Modify: `server/internal/service/system/file.go`
- Modify: `server/internal/service/system/login_log.go`
- Modify: `server/internal/service/system/menu.go`
- Modify: `server/internal/service/system/menu_seed.go`
- Modify: `server/internal/service/system/menu_user.go`
- Modify: `server/internal/service/system/notice.go`
- Modify: `server/internal/service/system/operation_log.go`
- Modify: `server/internal/service/system/permission.go`
- Modify: `server/internal/service/system/role.go`
- Modify: `server/internal/service/system/user.go`
- Modify: `server/internal/service/system/department_test.go`
- Modify: `server/internal/service/system/online_user_test.go`

- [x] **Step 1: 迁移测试调用点**

将测试中的 legacy 调用替换为 `*Context(context.Background(), ...)`：
- `DepartmentService.Create`
- `DepartmentService.Update`
- `DepartmentService.Delete`
- `OnlineUserService.SetOnlineUser`
- `OnlineUserService.RemoveOnlineUser`
- `OnlineUserService.GetOnlineUsers`
- `OnlineUserService.GetOnlineUserCount`
- `OnlineUserService.ForceLogout`
- `OnlineUserService.IsUserOnline`

- [x] **Step 2: 删除 Service wrapper**

删除 `server/internal/service/system` 下所有只调用 `context.Background()` 的 Deprecated wrapper，保留对应的 `*Context(ctx context.Context, ...)` 方法。

- [x] **Step 3: 验证 Service system**

Run:

```bash
go test ./internal/service/system -count=1
```

Expected: PASS。

---

## Task 3: 架构测试与文档收敛

**Files:**
- Modify: `server/internal/architecture/legacy_context_wrappers_test.go`
- Modify: `docs/development/OPTIMIZATION_STATUS.md`

- [x] **Step 1: 调整架构测试**

保留当前测试对非 system 区域的 `Deprecated:` 注释约束，同时新增 system service/DAO 禁止 wrapper 的断言。判定规则覆盖：
- 导出方法。
- 方法名不以 `Context` 结尾。
- 参数中没有 `context.Context`。
- 方法体调用 `*Context` 后继方法。
- 参数直接传入 `context.Background()`，或先赋值给局部变量再传入。
- 支持一层以上的简单局部别名传播，例如 `base := context.Background(); ctx := base`。

`AuditLogService.Record -> RecordContext` 保留为绑定 `AuditLogService` receiver 的显式例外，因为它会优先从 `gin.Context.Request.Context()` 取请求上下文，仅在没有 Gin 请求时 fallback 到 background。

- [x] **Step 2: 更新文档**

在 `docs/development/OPTIMIZATION_STATUS.md` 的 Deprecated wrapper 相关段落记录：
- Phase 1 已移除 `server/internal/dao/system` 和 `server/internal/service/system` 的 legacy context wrapper。
- 非 system 区域仍保留 `Deprecated:` 注释门禁，后续分阶段迁移。

- [x] **Step 3: 验证架构测试**

Run:

```bash
go test ./internal/architecture -count=1
```

Expected: PASS。

---

## Task 4: Phase 1 集成验证

**Files:**
- Verify only

- [x] **Step 1: 格式化**

Run:

```bash
gofmt -w server/internal/dao/system server/internal/service/system server/internal/architecture/legacy_context_wrappers_test.go
```

- [x] **Step 2: 定向测试**

Run:

```bash
go test ./internal/dao/system ./internal/service/system ./internal/architecture -count=1
```

Expected: PASS。

- [x] **Step 3: 后端全量测试**

Run:

```bash
go test ./... -count=1
```

Expected: PASS。

- [x] **Step 4: 静态检查**

Run:

```bash
go vet ./...
```

Expected: PASS。
