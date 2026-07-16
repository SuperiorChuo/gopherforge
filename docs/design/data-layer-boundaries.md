# 数据层按服务划界（设计稿）

> 状态：设计中，未实施。当前微服务线共享单库 `go_admin_kit`（公共 schema），
> 迁移真源在 `services/monitor/migrations/`，由 compose 的一次性 `migrate` job 执行。

## 1. 现状与问题

8 个服务连同一个库、同一 schema。实测各服务的实际读写面：

| 表（组） | 写入方 | 读取方 | 说明 |
|---|---|---|---|
| `users` / `password_history` / `totp_recovery_codes` / `oauth_bindings` | **auth**（登录、改密、2FA）、**identity**（用户管理 CRUD） | 全部服务 | 双写方是最大问题 |
| `roles` / `permissions` / `user_roles` / `role_permissions` / `role_data_scope_departments` / `departments` | **identity** | 全部服务（鉴权中间件查角色/数据范围） | |
| `menus` / `menu_permissions` / `dict_*` / `notices` | **system**（monitor 启动时也播种 menus） | system、前端经 system | 播种双源：system 与 monitor 各持一份 seed，人工同步 |
| `login_logs` / `operation_logs` / `wm_audit_log` | **audit**（NATS 消费写 login_logs）；**全部服务**的 operation_log 中间件直写 operation_logs | audit 查询 | 操作日志是"人人可写" |
| `files` | **file** | file、其他服务读 URL | |
| `ai_*` | **ai** | ai | 已天然隔离 |
| `im_*`（AutoMigrate） | **im** | im | 已天然隔离，且自管 schema |
| `wm_system_setting` | **system**（设置管理） | **全部服务**（runtimeconfig 轮询 + Redis 失效广播） | 运行时配置的事实总线 |
| `wm_console_route` / `wm_console_session` | auth | 全部服务（consoleauth） | |
| `tenants` | identity（管理）、auth（登录解析） | identity、auth | |
| `scheduled_job*` / 监控表 | monitor | monitor | 已天然隔离 |

核心结论：**"人人读 users/roles、人人写 operation_logs、人人读 wm_system_setting"是三条真正的跨界依赖**，
其余表的归属已经相当清晰。

## 2. 目标形态（阶段 B：单库多 schema）

不拆库（运维成本与当前团队规模不匹配），改为**单 PG 实例、按服务划 schema**：

```
go_admin_kit
├── auth      : password_history, totp_recovery_codes, oauth_bindings,
│               wm_console_route, wm_console_session
├── identity  : users, roles, permissions, user_roles, role_permissions,
│               role_data_scope_departments, departments, tenants
├── system    : menus, menu_permissions, dict_types, dict_items, notices,
│               wm_system_setting
├── audit     : login_logs, operation_logs, wm_audit_log
├── file      : files
├── ai        : ai_conversations, ai_messages, ai_documents, ai_document_chunks
├── im        : im_*（现状保持 AutoMigrate，后续并入 goose）
└── monitor   : scheduled_jobs, scheduled_job_logs
```

原则：**表的写入方唯一，写入方所在服务即 owner schema**。`users` 归 identity
（auth 对 users 的写收敛为"认证域副表"：password_history/totp/oauth 归 auth，
users 本体只有 identity 写，auth 改密等经由列级 UPDATE 白名单过渡）。

## 3. 跨界依赖的处理

| 依赖 | 短期（阶段 B 内） | 长期 |
|---|---|---|
| 各服务读 users/roles 做鉴权 | PG 授权跨 schema **只读**（GRANT SELECT），GORM 模型加 `TableName()` 带 schema 前缀 | 网关 ForwardAuth 响应头已带 user/tenant；把角色/权限也放进 JWT claims 或 auth 的 verify 响应，服务侧去掉对 identity 表的直读 |
| 人人写 operation_logs | 改为 **NATS 事件**：中间件发 `audit.operation`，audit-service 消费落库（login_logs 已是此模式，扩展即可） | 同左，这是终态 |
| 人人读 wm_system_setting | 保持跨 schema 只读 + 现有 Redis 失效广播 | system 暴露 `/internal/settings` 或经 NATS 推送快照 |
| monitor 播种 menus | 播种职责移交 system-service 启动时执行，monitor 不再持有 seed 副本 | 同左 |

## 4. 实施顺序（每步可独立回滚）

1. **operation_logs 事件化**（先斩断"人人可写"）：复用 audit 的 JetStream 消费者，
   六服务的 operation_log 中间件改为 publish；audit 不可用时降级 stdout。
   验收：网关打请求，operation_logs 仍有记录且带 request_id。
2. **menus 播种收敛到 system**：删 monitor 的 seed 副本与启动调用。
3. **迁移文件按 owner 分目录**：`migrations/{auth,identity,...}/`，migrate job
   按目录顺序执行（goose 每 schema 一个 version 表）。此步只动文件组织，不动表。
4. **建 schema + `ALTER TABLE ... SET SCHEMA`**（一次维护窗口，事务内可回滚）；
   GORM 模型补 `TableName()`；PG 角色按服务发放（owner 全权 / 其他只读）。
5. **收紧**：撤销跨 schema 写权限，观察一个迭代周期后撤销非必要的读权限。

## 5. 明确不做

- 不拆多个 PG 实例/库：备份、连接池、事务一致性成本在当前规模下收益为负。
- 不引入服务间同步 RPC 换取"纯净边界"：读 users 走 JWT claims / 只读授权即可，
  避免把登录路径变成分布式调用链。
- im 的 AutoMigrate 暂不动：实验特性，等转正时并入 goose。

## 6. 风险

| 风险 | 缓解 |
|---|---|
| `SET SCHEMA` 后旧连接 search_path 失效 | 模型显式 schema 前缀，不依赖 search_path |
| operation_logs 事件化丢日志 | JetStream 持久化 + 消费者 durable；发布失败降级本地落盘 |
| 跨 schema 外键（如 user_roles→users） | 同库跨 schema 外键 PG 原生支持，无需改 |
| 测试大面积 mock SQL 带 schema 前缀 | 第 3、4 步之间先改测试基建（sqlmock 正则统一处理前缀） |
