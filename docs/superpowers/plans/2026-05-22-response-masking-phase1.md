# 响应脱敏第一阶段 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `POST /api/v1/login`、`GET /api/v1/user/me`、`PUT /api/v1/user/profile`、`GET /api/v1/online-users` 增加字段级动态脱敏能力，并保持第一阶段只覆盖 typed DTO 响应。

**Architecture:** 新增 `internal/pkg/mask` 负责纯脱敏与克隆逻辑，`internal/pkg/response` 提供可选脱敏出口，API 层通过一个共享策略 helper 决定 `shouldMask`。为避免第一阶段落到 `gin.H` / `map[string]any`，先把 `login` 与 `online-users` 响应收为 typed DTO。

**Tech Stack:** Go 1.26、Gin、反射、现有 `response` 包、现有 `auth/system` API DTO。

---

## 文件结构

- 新建 `server/internal/pkg/mask/mask.go`
  - 负责 `MaskValue`、结构克隆、递归脱敏、tag 解析、元数据缓存。
- 新建 `server/internal/pkg/mask/mask_test.go`
  - 覆盖 `email` / `phone` / `ip` / `token` / `path` / `full`、嵌套 struct、slice、指针、`time.Time`、原对象不污染。
- 修改 `server/internal/pkg/response/response.go`
  - 新增 `SuccessMasked`、`PageSuccessMasked`，必要时新增 `SuccessWithMessageMasked`。
- 新建或修改 `server/internal/pkg/response/response_test.go`
  - 覆盖 masked response 不污染原对象、`shouldMask=false` 保持原值。
- 修改 `server/internal/api/auth/user_dto.go`
  - 为 `UserInfoResponse` 增加 `mask` tag。
  - 新增 typed `LoginResponse` DTO。
- 修改 `server/internal/api/auth/user.go`
  - `login`、`user/me`、`user/profile` 改用 typed DTO 和 masked response。
- 修改 `server/internal/api/system/online_user.go`
  - 为在线用户列表新增 typed list envelope，并接入 masked response。
- 新建 `server/internal/api/shared/masking.go`
  - 共享 `shouldMask` 策略 helper：本人资料不脱敏、`super_admin` 不脱敏、其余脱敏。
- 新建 `server/internal/api/shared/masking_test.go`
  - 覆盖 shared policy helper。
- 视需要修改 `server/internal/openapi/schemas.go`
  - 仅在 DTO 结构变化导致 schema 不匹配时调整，保持外部契约不变。

## Task 1: 写失败测试并固定首批范围

**Files:**
- Create: `server/internal/pkg/mask/mask_test.go`
- Modify: `server/internal/pkg/response/response_test.go`
- Create: `server/internal/api/shared/masking_test.go`

- [ ] **Step 1: 写 `mask` 包的失败测试**

覆盖这些行为：

```go
func TestMaskValue(t *testing.T) {
	tests := []struct {
		name     string
		maskType string
		input    string
		want     string
	}{
		{name: "email", maskType: "email", input: "alice@example.com", want: "a***e@example.com"},
		{name: "phone", maskType: "phone", input: "13812345678", want: "138****5678"},
		{name: "ipv4", maskType: "ip", input: "192.168.10.25", want: "192.168.*.*"},
		{name: "token", maskType: "token", input: "abcd1234wxyz9876", want: "abcd***9876"},
		{name: "path", maskType: "path", input: "/data/uploads/a.png", want: "***/a.png"},
		{name: "full", maskType: "full", input: "secret", want: "***"},
	}
	// ...
}
```

再补一个复杂结构测试，验证：
- 指针
- slice
- 嵌套 struct
- `time.Time`
- 原对象不被污染

- [ ] **Step 2: 运行 `mask` 测试并确认失败**

Run: `go test ./internal/pkg/mask -count=1`

Expected: FAIL，报未定义 `MaskValue` / `CloneAndMask` 或包不存在。

- [ ] **Step 3: 写 `response` 包的失败测试**

新增两个测试：

```go
func TestSuccessMaskedMasksResponseDataWithoutMutatingOriginal(t *testing.T) {}
func TestSuccessMaskedLeavesDataUntouchedWhenShouldMaskIsFalse(t *testing.T) {}
```

测试数据使用一个本地 struct：

```go
type sample struct {
	Email string `json:"email" mask:"email"`
}
```

- [ ] **Step 4: 运行 `response` 测试并确认失败**

Run: `go test ./internal/pkg/response -count=1`

Expected: FAIL，报未定义 `SuccessMasked` 或返回值未脱敏。

- [ ] **Step 5: 写 shared policy helper 的失败测试**

覆盖三条规则：

```go
func TestShouldMaskOwnProfile(t *testing.T) {}
func TestShouldMaskSuperAdmin(t *testing.T) {}
func TestShouldMaskOtherUserForNonSuperAdmin(t *testing.T) {}
```

其中 helper 输入固定为：
- `actorUserID uint`
- `targetUserID *uint`
- `roleCodes []string`

- [ ] **Step 6: 运行 shared helper 测试并确认失败**

Run: `go test ./internal/api/shared -count=1`

Expected: FAIL，报 helper 未定义。

## Task 2: 实现 `mask` 包最小能力

**Files:**
- Create: `server/internal/pkg/mask/mask.go`
- Test: `server/internal/pkg/mask/mask_test.go`

- [ ] **Step 1: 写最小实现让 `MaskValue` 测试通过**

至少实现：

```go
func MaskValue(maskType string, input string) string
```

支持：
- `email`
- `phone`
- `ip`
- `token`
- `path`
- `full`

- [ ] **Step 2: 跑 `mask` 测试**

Run: `go test ./internal/pkg/mask -run TestMaskValue -count=1`

Expected: PASS

- [ ] **Step 3: 补结构克隆与递归脱敏实现**

实现入口建议：

```go
func CloneAndMask[T any](data T, shouldMask bool) T
func CloneAndMaskAny(data any, shouldMask bool) any
```

约束：
- `shouldMask=false` 时直接返回原值
- `shouldMask=true` 时返回脱敏后的副本
- 不修改原对象
- `time.Time` 视为标量
- 仅处理 struct / pointer / slice / array
- 不处理任意 `map[string]any`

- [ ] **Step 4: 跑 `mask` 全测试**

Run: `go test ./internal/pkg/mask -count=1`

Expected: PASS

## Task 3: 接入 `response` 层可选脱敏出口

**Files:**
- Modify: `server/internal/pkg/response/response.go`
- Test: `server/internal/pkg/response/response_test.go`

- [ ] **Step 1: 在 `response` 包新增可选脱敏出口**

建议新增：

```go
func SuccessMasked(c *gin.Context, data any, shouldMask bool)
func SuccessWithMessageMasked(c *gin.Context, message string, data any, shouldMask bool)
func PageSuccessMasked(c *gin.Context, data any, total int64, page, pageSize int, shouldMask bool)
```

实现要点：
- 只在 `shouldMask=true` 时调用 `mask.CloneAndMaskAny`
- 保持 `Success` / `SuccessWithMessage` / `PageSuccess` 原行为不变

- [ ] **Step 2: 运行 `response` 测试**

Run: `go test ./internal/pkg/response -count=1`

Expected: PASS

## Task 4: typed DTO 与 shared policy helper

**Files:**
- Modify: `server/internal/api/auth/user_dto.go`
- Create: `server/internal/api/shared/masking.go`
- Test: `server/internal/api/shared/masking_test.go`

- [ ] **Step 1: 在 `UserInfoResponse` 上加 `mask` tag**

至少为：

```go
Email string `json:"email" mask:"email"`
Phone string `json:"phone" mask:"phone"`
```

- [ ] **Step 2: 新增 typed login response DTO**

例如：

```go
type LoginResponse struct {
	User         *UserInfoResponse `json:"user"`
	AccessToken  string            `json:"access_token"`
	RefreshToken string            `json:"refresh_token"`
}
```

注意：第一阶段不对 token 做脱敏 tag。

- [ ] **Step 3: 实现 shared `shouldMask` helper**

建议函数：

```go
func ShouldMask(actorUserID uint, targetUserID *uint, roleCodes []string) bool
func HasRole(roleCodes []string, target string) bool
```

规则：
- `targetUserID != nil && *targetUserID == actorUserID` => `false`
- `roleCodes` 包含 `super_admin` => `false`
- 其余 => `true`

- [ ] **Step 4: 跑 shared helper 测试**

Run: `go test ./internal/api/shared -count=1`

Expected: PASS

## Task 5: 接入 `login` / `user/me` / `user/profile`

**Files:**
- Modify: `server/internal/api/auth/user.go`
- Modify: `server/internal/api/auth/user_dto.go`

- [ ] **Step 1: `login` 改成 typed login response**

将：

```go
loginResp := gin.H{
	"user":          ConvertUserToResponse(&resp.User, permissions),
	"access_token":  resp.AccessToken,
	"refresh_token": resp.RefreshToken,
}
response.SuccessWithMessage(c, "login success", loginResp)
```

改为 typed DTO，并使用：

```go
response.SuccessWithMessageMasked(c, "login success", loginResp, false)
```

这里必须是 `false`，因为登录返回的是当前登录用户自己的资料。

- [ ] **Step 2: `GET /api/v1/user/me` 与 `PUT /api/v1/user/profile` 改接 masked response**

这两个接口同样传 `shouldMask=false`，因为都是本人资料。

- [ ] **Step 3: 运行 auth 相关测试**

Run: `go test ./internal/api/auth ./internal/api -count=1`

Expected: PASS

## Task 6: 接入 `GET /api/v1/online-users`

**Files:**
- Modify: `server/internal/api/system/online_user.go`
- Test: `server/internal/api/system/online_user_test.go`

- [ ] **Step 1: 新增 typed online user list envelope，并给首批敏感字段加 tag**

建议新增：

```go
type onlineUserListResponse struct {
	List  []onlineUserListItem `json:"list"`
	Total int                  `json:"total"`
}
```

给 `onlineUserListItem` 增加：

```go
IP      string `json:"ip" mask:"ip"`
TokenID string `json:"token_id" mask:"token"`
```

第一阶段暂不处理 `Location` / `Browser` / `OS`。

- [ ] **Step 2: 在 handler 中计算 `shouldMask`**

最小做法：
- 从 `c.Get("user_id")` 取当前用户 ID
- 通过 `auth.UserDAO` 或已有 service 取一次当前用户角色
- 用 `shared.ShouldMask(actorID, nil, roleCodes)` 计算

`GET /api/v1/online-users`：
- `super_admin` => `shouldMask=false`
- 其他 => `shouldMask=true`

- [ ] **Step 3: 把响应改为 typed DTO + `response.SuccessMasked`**

- [ ] **Step 4: 补测试**

至少补：
- 非 `super_admin` 时 `ip` / `token_id` 被脱敏
- `super_admin` 时保留明文

如果直接打 handler 太重，可用 `httptest` + gin context + 伪造 `user_id` + 可注入 service/DAO 的最小 seam。

- [ ] **Step 5: 运行 system API 测试**

Run: `go test ./internal/api/system -count=1`

Expected: PASS

## Task 7: OpenAPI 与全量回归

**Files:**
- Modify if needed: `server/internal/openapi/schemas.go`

- [ ] **Step 1: 检查 typed DTO 是否改变现有契约**

如果 `login` 与 `online-users` 的 JSON shape 未变，则不需要改 schema；若测试失败再最小调整。

- [ ] **Step 2: 跑相关回归**

Run: `go test ./internal/openapi ./internal/pkg/mask ./internal/pkg/response ./internal/api/auth ./internal/api/system -count=1`

Expected: PASS

- [ ] **Step 3: 跑后端全量测试**

Run: `go test ./... -count=1`

Expected: PASS

- [ ] **Step 4: 代码格式与差异检查**

Run: `gofmt -w internal/pkg/mask/*.go internal/pkg/response/*.go internal/api/auth/*.go internal/api/system/*.go internal/api/shared/*.go`

Run: `git diff --check`

Expected: no output

## 自检

- spec 首批覆盖的四个接口都在 plan 中有落点。
- plan 明确避免了任意 `gin.H` / `map[string]any` 的泛化脱敏。
- plan 保持 token 响应、文件响应、`OperationLog` / `AuditLog` payload 不纳入第一阶段。
- `shouldMask` 规则固定为：本人资料不脱敏、`super_admin` 不脱敏、其他脱敏。
- `online-users` 采用整表单一策略，不做 item 级异构判断。
