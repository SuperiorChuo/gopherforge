# Go Admin Kit 企业级扩展设计

## 文档信息

- 日期：2026-05-21
- 状态：已完成设计评审，待进入 implementation plan
- 范围：`Data Scope` 自动化、字段级响应脱敏、部门树 `L1+L2` 双层缓存
- 决策：按 `缓存 -> 脱敏 -> 插件` 的顺序分阶段落地

## 背景

当前仓库已经具备较好的基础能力：

- `authz.UserDataScope`、`ApplyUserEntityScope`、`ApplyOwnerScope` 已能正确表达数据权限。
- `response.Success`、`PageSuccess` 已形成统一 JSON 响应出口。
- `authz.DepartmentTreeCache` 已从解析逻辑中抽象出来，Redis L2 缓存与失效路径已经存在。

这意味着三项扩展都不需要从零重写，更适合沿着现有抽象增量演进，而不是引入一整套新的中间层。

## 目标

1. 降低手工调用 `ApplyUserEntityScope` / `ApplyOwnerScope` 的重复代码与遗漏风险。
2. 为高敏感字段提供可声明、可审计、低侵入的响应脱敏能力。
3. 降低部门树数据权限解析对 Redis 网络往返和 JSON 反序列化的依赖，提升高频列表接口吞吐。

## 非目标

1. 第一阶段不实现“所有 `gorm.DB` 查询的全自动权限接管”。
2. 第一阶段不对 `Raw SQL`、`Create`、`Update`、`Delete` 自动注入数据权限。
3. 第一阶段不把日志、`AuditLog.BeforeJSON/AfterJSON`、任意 `map[string]any` 响应都纳入统一脱敏策略。
4. 第一阶段不抽象出通用全站缓存框架，只处理 `authz:department_tree`。
5. 第一阶段不覆盖登录/刷新/OAuth 的 token 明文响应，不覆盖文件响应，不覆盖任意 `gin.H` / `map[string]any` 响应。
6. 第一阶段不做 item 级异构脱敏策略；混合列表采用整份响应单一策略。

## 当前实现观察

### 数据权限

- 重度依赖手工 scope 的 DAO 主要是：
  - `server/internal/dao/system/file.go`
  - `server/internal/dao/system/login_log.go`
  - `server/internal/dao/system/operation_log.go`
  - `server/internal/dao/system/user.go`
- 当前存在明确“故意不加数据权限”的查询，例如用户详情、文件去重、日志统计与趋势。如果直接做 blanket plugin，会改变这些既有语义。

### 响应脱敏

- 统一响应出口集中在 `server/internal/pkg/response/response.go`。
- 现有脱敏只覆盖操作日志 request body 的 JSON key 级遮罩，位于 `server/internal/middleware/operation_log.go`，不适合作为通用响应脱敏入口。
- 当前最适合试点的结构化对象是：
  - `server/internal/api/auth/user_dto.go` 中的 `UserInfoResponse`
  - `server/internal/api/system/online_user.go` 中的在线用户响应项

### 部门树缓存

- 当前 L2 Redis 缓存 key 为 `authz:department_tree`，TTL 为 5 分钟。
- 缓存只服务于 `authz.ResolveUserDataScopeContext()` 的部门树解析，不服务于 `/system/department/tree` 页面接口。
- 失效路径目前只在部门写操作后调用 `authz.InvalidateDepartmentTreeCache()`，且仅执行 Redis `DEL`。

## 总体方案

采用三阶段演进：

1. 在 `authz.DepartmentTreeCache` 默认实现上增加进程内 L1，并通过 Redis Pub/Sub 广播做跨实例失效。
2. 在 `response` 层增加可选脱敏出口，通过 struct tag 声明字段规则，先覆盖用户与在线用户响应。
3. 在 GORM 初始化阶段注册一个“显式启用、只处理读路径”的数据权限插件，用 `context.Context` 传递已经解析好的 `UserDataScope`。

这样做的原因：

- 第一阶段收益最大、风险最低，且已有抽象可复用。
- 第二阶段改动面主要在响应层与 DTO，不会碰数据库语义。
- 第三阶段价值最高，但必须建立在前两阶段已经稳定的前提上，避免同时变动查询路径和响应路径。

## 特性一：部门树 `L1+L2` 双层缓存

### 目标

降低 `ResolveUserDataScopeContext()` 高频调用下的 Redis 读放大与 JSON 反序列化开销，同时保证多实例部署下的高一致性。

### 设计

- 保持 `authz.DepartmentTreeCache` 接口不变。
- 将默认实现从“纯 Redis”替换为“共享 L1 + Redis L2”。
- L1 存储对象仍然只保留 `id` / `parent_id` 对应的数据结构，不缓存完整 `Department` 业务视图。
- L1 TTL 建议 30 到 60 秒，且不得超过现有 L2 TTL。
- 写侧失效顺序：
  1. 清本机 L1
  2. `DEL authz:department_tree`
  3. 仅在 `DEL` 成功后 `PUBLISH authz:department_tree:invalidate`
- 订阅侧只负责清本机 L1，不执行二次 `DEL` 或再广播。

### 代码落点

- `server/internal/pkg/authz/data_scope.go`
  - 增加 `L1+L2` 默认缓存实现
  - 增加 `InvalidateDepartmentTreeCacheContext(ctx context.Context) error`
  - 保留 `InvalidateDepartmentTreeCache()` 兼容 wrapper
- `server/internal/pkg/redis/redis.go`
  - 增加最小 `Publish` / `Subscribe` helper
  - 增加 `Close()`，便于主程序优雅关闭
- `server/cmd/main.go`
  - `redis.InitRedis()` 后显式启动部门树失效订阅器

### 风险控制

- Pub/Sub 无消息重放，所以 L1 必须带短 TTL，不能永久驻留。
- 第一阶段不处理绕过 `DepartmentService` 的直改库场景；这些场景今天也无法触发 L2 失效。
- 写侧广播必须在 L2 删除成功后触发，避免别的实例从旧 L2 回填脏数据。
- 第一阶段保持当前部门写接口语义：只要数据库写入成功，接口仍返回成功；`DEL` / `PUBLISH` 失败属于 best-effort 失效失败，必须记录结构化日志与指标。
- 该方案提供的是 TTL 有界的最终一致性，不承诺跨实例线性一致。
- subscriber 启动或运行失败不阻断服务可用性，但必须暴露日志、指标和存活信号；实例在该场景下退化为“本机 L1 TTL + Redis L2”模式。
- 即使 Redis `DEL` / `PUBLISH` 失败，也必须先清本机 L1，防止当前实例持续命中旧数据。

### 测试

- L1 命中时不访问 Redis。
- L1 miss + L2 hit 时正确回填 L1。
- 部门写操作后，本机 L1 和 L2 都被清理。
- 模拟跨实例订阅时，远端实例收到消息后清空本地 L1。
- 漏消息后依赖 L1 TTL 自愈。
- `InvalidateDepartmentTreeCacheContext(ctx)` 在 canceled ctx、`DEL` 失败、`PUBLISH` 失败场景下返回与本机 L1 清理行为明确。
- subscriber 启动失败、运行中断开、关闭顺序可被验证，且不会导致进程 panic。

## 特性二：基于 struct tag 的响应脱敏

### 目标

为后台用户响应提供字段级动态脱敏，避免手机号、邮箱、IP、token、文件路径等敏感数据被默认明文返回。

### 设计

- 新增轻量包，例如 `server/internal/pkg/mask`。
- 支持的首批 tag：
  - `mask:"email"`
  - `mask:"phone"`
  - `mask:"ip"`
  - `mask:"token"`
  - `mask:"path"`
  - `mask:"hash"`
  - `mask:"full"`
- 执行入口放在 `response` 层，不放在 middleware。
- 第一阶段只新增可选响应函数：
  - `SuccessMasked(...)`
  - `PageSuccessMasked(...)`
- 原 `Success(...)`、`PageSuccess(...)` 不改行为。
- 首批脱敏格式固定为：
  - `email`：`a***z@example.com`；local part 长度小于等于 1 时输出 `***@domain`
  - `phone`：`138****5678`
  - `ip`：IPv4 输出 `192.168.*.*`，IPv6 保留前两个 segment，其余输出 `*`
  - `token` / `hash`：保留前 4 位和后 4 位，中间以 `***` 替代；长度不足 8 时输出 `***`
  - `path`：仅保留 basename，目录部分替换为 `***/`
  - `full`：固定输出 `***`

### `shouldMask` 规则

`shouldMask` 必须由共享策略 helper 统一产出，再交给 response 层：

1. 描述当前登录用户自身资料的响应，例如 `/api/v1/login` 中的 `user`、`/api/v1/user/me`、更新个人资料后的返回：不脱敏
2. 角色包含 `super_admin`：不脱敏
3. 其他情况：脱敏

第一阶段不引入新的 RBAC 豁免权限；`system:sensitive:unmask` 留到后续阶段再决定是否进入权限种子。

对混合列表的约束：

- 第一阶段不做“列表里本人不脱敏、别人脱敏”的 item 级判定。
- `/api/v1/online-users` 对非 `super_admin` 一律整表脱敏。

### 试点对象

- `server/internal/api/auth/user_dto.go` 的 `UserInfoResponse`
- `server/internal/api/system/online_user.go` 的在线用户列表项

第一阶段明确覆盖的响应：

- `POST /api/v1/login` 的 `data.user`
- `GET /api/v1/user/me`
- `PUT /api/v1/user/profile`
- `GET /api/v1/online-users`

用户管理列表与详情如果要接入，需先补专用 DTO，不直接在 `model.User` 上打 tag。

### 实现约束

- 不能原地修改 service/dao 返回的原对象，必须复制后处理。
- 递归器要把 `time.Time` 当作标量，避免盲目深入。
- 反射元数据要缓存，避免大分页列表频繁反射。
- 第一阶段不处理 `OperationLog.RequestBody` / `ResponseBody` 这类字符串化 payload，也不处理 `AuditLog.BeforeJSON/AfterJSON`。
- 第一阶段不处理 token 响应体、文件响应体、任意 `gin.H`、任意 `map[string]any`、`AuditLog` 和 `OperationLog` 的详情类 payload。

### 测试

- `email`、`phone`、`ip`、`token` 等 tag 能按预期输出。
- `shouldMask=false` 时保持原值。
- 指针、切片、嵌套 struct 场景可正确递归。
- 原对象不被污染。
- `time.Time`、空值、nil 指针不出错。
- `/api/v1/online-users` 在非 `super_admin` 上下文中整表脱敏，在 `super_admin` 上下文中保留明文。

## 特性三：显式启用的 GORM 数据权限插件

### 目标

减少 DAO 中重复的 `ApplyUserEntityScope` / `ApplyOwnerScope` 样板代码，并降低遗漏调用导致的越权风险。

### 设计原则

- 第一阶段只处理读路径。
- 第一阶段只处理已验证的四类模型：
  - `User`
  - `File`
  - `LoginLog`
  - `OperationLog`
- 第一阶段只在 `tx.Statement.Schema` 可解析、无 `Raw SQL`、无 alias、无显式 `Joins`、且未被显式禁用时生效。
- 插件只在同时满足“context 中存在 `UserDataScope`”和“显式启用标记存在”时生效；缺失任一条件都必须 no-op。
- 插件只消费已注入到 `context.Context` 的 `UserDataScope`，绝不在 callback 中再次查询用户或角色。

### GORM 挂点

- 在 `server/internal/pkg/database/database.go` 的 `gorm.Open(...)` 后注册 plugin。
- 第一阶段先注册 `Query` callback；`First` / `Take` 走 `Query` 路径即可覆盖。
- 只有在明确需要处理 `Row()` / `Rows()` 时，才补 `Row` callback。

### 上下文策略

- 在鉴权后 middleware 中一次性 resolve 基础 `UserDataScope`。
- 将 scope 写入 `c.Request.Context()`。
- 额外提供三个 helper：
  - `EnableDataScope(ctx)`
  - `ForceSelfScope(ctx, userID uint)`
  - `DisableDataScope(ctx)`

这组 helper 用于：

- `GetMyFiles`
- `GetMyLoginLogs`
- 当前手工调用 `ApplyUserEntityScope` / `ApplyOwnerScope` 的列表查询
- 文件去重或日志统计这类明确不应自动套 scope 的路径

第一阶段不采用“所有已认证请求默认全局启用”的策略，避免误伤现有例外查询。

### 模型映射策略

不自行解析字符串式 `gorm` tag，优先使用 `tx.Statement.Schema` 获取字段信息，并辅以显式 model 约定：

- `User` 使用主体列 `id` 和部门列 `department_id`
- `File`、`LoginLog`、`OperationLog` 使用 owner 列 `user_id`

### 明确不做

- 不自动处理 `Raw SQL`
- 不自动处理 `Create` / `Update` / `Delete`
- 不尝试 blanket 覆盖所有 model
- 不自动接管插件内部生成的子查询；内部辅助 subquery 必须显式打 `skip` 标记，防止递归裁剪
- 不接管当前已明确保留为未加 scope 的例外查询：
  - `User` 详情与写接口 pre-read
  - `OperationLog` 详情
  - 文件 hash 去重
  - 登录日志统计与趋势
  - 操作日志统计

### 风险控制

- 当前存在故意不加 scope 的查询，插件不能默认“一刀切”。
- `Preload` 会触发额外查询，必须按模型 opt-in，不能对所有 schema 粗暴生效。
- `Count`、统计、趋势类查询即使技术上能继承 where，也可能改变现有报表口径。
- 如果未来要把当前例外查询也纳入插件，必须作为显式行为变更重新评审，不能通过默认 context 注入的副作用悄悄发生。

### 测试

- 四类目标模型在列表、分页、`Count`，以及明确 opt-in 的详情场景下 SQL 正确。
- `scope=all` 时不附加过滤。
- 缺失 scope 或缺失 enable 标记时，插件 no-op。
- `DisableDataScope(ctx)` 时插件 no-op。
- `EnableDataScope(ctx)` 后，当前手工 scoped 的查询可自动附加过滤。
- `ForceSelfScope(...)` 能覆盖更窄的“我的数据”口径。
- 文件去重、日志统计、用户详情等显式绕过路径保持现有行为。

## 交付顺序

### 第一阶段

- `DepartmentTreeCache` 升级为 `L1+L2`
- Redis 广播失效接入
- 文档与测试补齐

### 第二阶段

- `internal/pkg/mask`
- `response.SuccessMasked` / `PageSuccessMasked`
- 用户与在线用户响应试点

### 第三阶段

- 数据权限 plugin
- 中间件注入 `UserDataScope`
- 四类模型读路径接入

## 验收标准

1. 部门树数据权限解析在热点请求下可稳定命中 L1，跨实例写后可在短时间内失效。
2. 用户与在线用户接口在非特权上下文中返回脱敏数据，在本人/特权上下文中保留明文。
3. `User`、`File`、`LoginLog`、`OperationLog` 的读查询可以通过 plugin 自动附加既有数据权限过滤，且不破坏现有例外查询语义。
4. 所有新增能力都具备单元测试，并至少覆盖一个集成路径。

## 实施前提示

- 本设计文档只定义阶段划分、边界和落点，不直接包含补丁级实现细节。
- 进入编码前，应基于本设计生成 implementation plan，并把三阶段进一步拆成可独立提交的小任务。
