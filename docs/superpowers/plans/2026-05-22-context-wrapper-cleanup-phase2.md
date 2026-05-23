# Context Wrapper Cleanup Phase 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**目标：** 继续清理 Phase 1 之后剩余的纯 `context.Background()` Deprecated wrapper，优先处理生产无调用且行为等价的 `auth`、`monitor`、`pkg` 与 `middleware` 分片。
**架构：** 按前缀分阶段推进：清理完成的前缀在架构测试中切换为 forbidden policy；仍需兼容语义的 API 保持 `Deprecated:` 门禁或显式 allowlist。
**技术栈：** Go 1.26、标准库 `context`、AST 架构测试、`go test`、`go vet`、`gofmt`。

---

## 范围

Phase 2 清理以下低风险纯 wrapper：

- `server/internal/dao/auth`
- `server/internal/service/auth`
- `server/internal/dao/monitor`
- `server/internal/service/monitor`
- `server/internal/pkg/cache`
- `server/internal/pkg/captcha`
- `server/internal/pkg/upload`
- `server/internal/middleware/login_limit.go`
- `server/internal/pkg/authz/permissions.go`
- `server/internal/pkg/authz/data_scope.go` 中的 `InvalidateDepartmentTreeCache`

Phase 2 后续收口继续清理以下剩余项：

- `server/internal/dao/user.go` 的 6 个 root DAO 纯 wrapper。
- `pkg/ipinfo` 的 `GetIPInfo` 与 `GetLocation` wrapper，并修正默认 helper 与登录日志位置解析的 Context 链路。
- `authz.ResolveUserDataScope` wrapper；原有出错回退语义迁移为显式 `ResolveUserDataScopeFallbackContext`。

仍保留但不属于 Deprecated wrapper 的项：

- `authz.resolveDepartmentTreeIDs`：未导出内部 helper，失败时回退到当前部门，仍由后续 authz 内部重构处理。
- `monitor/job` 中调度启动、cron 执行等非 Deprecated 的后台 `context.Background()` 使用。

---

## Task 1: Auth 分片清理

**Files:**
- Modify: `server/internal/dao/auth/*.go`
- Modify: `server/internal/dao/auth/*_test.go`
- Modify: `server/internal/service/auth/*.go`
- Modify: `server/internal/service/auth/*_test.go`

- [x] 将测试中仍调用 legacy wrapper 的位置迁移到 `*Context(context.Background(), ...)`。
- [x] 删除 `dao/auth` 与 `service/auth` 中所有 `// Deprecated: use ...Context` 纯 wrapper。
- [x] 避免误删非目标方法，例如 `UserService.GetUserPermissions`。
- [x] Run:

```bash
go test ./internal/dao/auth ./internal/service/auth ./internal/api/auth -count=1
```

---

## Task 2: Monitor 分片清理

**Files:**
- Modify: `server/internal/dao/monitor/*.go`
- Modify: `server/internal/dao/monitor/*_test.go`
- Modify: `server/internal/service/monitor/*.go`
- Modify: `server/internal/service/monitor/*_test.go`

- [x] 将测试中仍调用 legacy wrapper 的位置迁移到 `*Context(context.Background(), ...)`。
- [x] 删除 `dao/monitor` 与 `service/monitor` 中所有 `// Deprecated: use ...Context` 纯 wrapper。
- [x] 保留调度生命周期中合理的 `context.Background()`，例如 cron 初始化、后台任务执行和 `context.WithoutCancel`。
- [x] Run:

```bash
go test ./internal/dao/monitor ./internal/service/monitor ./internal/api/monitor -count=1
```

---

## Task 3: Pkg 与 Middleware 低风险清理

**Files:**
- Modify: `server/internal/pkg/cache/cache.go`
- Modify: `server/internal/pkg/cache/cache_test.go`
- Modify: `server/internal/pkg/captcha/captcha.go`
- Modify: `server/internal/pkg/captcha/captcha_test.go`
- Modify: `server/internal/pkg/upload/upload.go`
- Modify: `server/internal/pkg/upload/upload_test.go`
- Modify: `server/internal/pkg/authz/permissions.go`
- Modify: `server/internal/pkg/authz/data_scope.go`
- Modify: `server/internal/pkg/authz/data_scope_cache_test.go`
- Modify: `server/internal/middleware/login_limit.go`

- [x] 将测试调用迁移到对应的 `*Context(context.Background(), ...)`。
- [x] 删除 `cache`、`captcha`、`upload`、`middleware/login_limit` 的纯 wrapper。
- [x] 删除 `authz.UserHasPermission` 与 `authz.InvalidateDepartmentTreeCache` 纯 wrapper。
- [x] 保留 `authz.ResolveUserDataScope` 与 `resolveDepartmentTreeIDs`。
- [x] Run:

```bash
go test ./internal/pkg/cache ./internal/pkg/captcha ./internal/pkg/upload ./internal/pkg/authz ./internal/middleware -count=1
```

---

## Task 4: 架构守卫升级

**Files:**
- Modify: `server/internal/architecture/legacy_context_wrappers_test.go`
- Modify: `docs/development/OPTIMIZATION_STATUS.md`

- [x] 将清理完成的前缀加入 forbidden policy：
  - `dao/auth/`
  - `dao/monitor/`
  - `service/auth/`
  - `service/monitor/`
  - `pkg/cache/`
  - `pkg/captcha/`
  - `pkg/upload/`
  - `middleware/`
- [x] 将 `authz.UserHasPermission` 与 `authz.InvalidateDepartmentTreeCache` 加入精确 forbidden key，避免误伤同文件暂留兼容 API。
- [x] 对暂留兼容 API 保持 `Deprecated:` 注释门禁或精确 allowlist。
- [x] 文档记录 Phase 2 已清理范围和暂留原因。
- [x] Run:

```bash
go test ./internal/architecture -count=1
```

---

## Task 5: 集成验证

**Files:**
- Verify only

- [ ] Run:

```bash
gofmt -w server/internal/dao/auth server/internal/service/auth server/internal/dao/monitor server/internal/service/monitor server/internal/pkg/cache server/internal/pkg/captcha server/internal/pkg/upload server/internal/pkg/authz server/internal/middleware server/internal/architecture/legacy_context_wrappers_test.go
```

- [ ] Run:

```bash
go test ./internal/dao/auth ./internal/service/auth ./internal/api/auth ./internal/dao/monitor ./internal/service/monitor ./internal/api/monitor ./internal/pkg/cache ./internal/pkg/captcha ./internal/pkg/upload ./internal/pkg/authz ./internal/middleware ./internal/architecture -count=1
```

- [ ] Run:

```bash
go test ./... -count=1
go vet ./...
git diff --check
```

---

## Task 6: 剩余 Deprecated Wrapper 归零

**Files:**
- Modify: `server/internal/dao/user.go`
- Modify: `server/internal/dao/user_test.go`
- Modify: `server/internal/pkg/ipinfo/ipinfo.go`
- Modify: `server/internal/pkg/ipinfo/ipinfo_test.go`
- Modify: `server/internal/service/system/login_log.go`
- Modify: `server/internal/pkg/authz/data_scope.go`
- Modify: `server/internal/pkg/authz/data_scope_test.go`
- Modify: `server/internal/architecture/legacy_context_wrappers_test.go`

- [x] 删除 root `UserDAO` 的 6 个纯 wrapper，并迁移测试调用到 `*Context(context.Background(), ...)`。
- [x] 删除 `IPInfoClient.GetIPInfo` 与 `GetLocation` wrapper。
- [x] 将 `GetIPInfoByQuery`、`GetLocationByIP`、`GetLocationAsync` 改为走 Context 链路。
- [x] 将 `LoginLogService.RecordContext` 的 IP 位置解析改为传递调用方 `ctx`。
- [x] 删除 `ResolveUserDataScope` wrapper，新增 `ResolveUserDataScopeFallbackContext` 承接旧的出错回退语义。
- [x] 将架构测试收紧到 root `dao/` 与 `pkg/ipinfo/`，并把 `authz` 三个已清理导出 wrapper 加入精确 forbidden key。
