# 多租户与套餐

GopherForge 内置 SaaS 化底座：**共享库 + `tenant_id` 行级隔离**，登录时带租户码进入对应租户空间，套餐（权限包）约束每个租户能用的功能上限。

## 隔离模型

- 所有租户级表统一带 `tenant_id`（`not null; default:1; index`，并参与业务唯一索引，如「用户名租户内唯一」）。
- **双层防线**：DAO 手写租户过滤是第一道；**租户隔离 GORM 插件**是第二道。凡模型带 `tenant_id` 列（Schema 自动识别，无需白名单），插件在四个生命周期钩子上兜底：
  - **查询**（Query/Row 前置）：自动追加 `WHERE tenant_id = 当前租户`；
  - **创建**：`TenantID` 为零值时自动补当前租户；
  - **更新/删除**：在已有 WHERE 上追加租户约束——跨租户按 id 猜测直接打空；空 WHERE 的全局更新/删除仍保留 GORM 的 `ErrMissingWhereClause` 保护。

  漏挂 scope 不再等于越权。
- 平台级表（`tenants`、`tenant_packages`、`permissions`、`menus` 等无 `tenant_id` 列）天然豁免；平台侧跨租户操作走显式 `tenant.DisableScope(ctx)` 逃生口。

## 登录与租户上下文

1. 登录请求可带 `tenant_code`（可选）；为空时从访问域名的子域提取（`acme.example.com` → `acme`），再取不到则落 `default` 租户。租户被禁用则拒绝登录。
2. 租户 ID 写入 JWT Claims 随 token 流转。
3. 请求经 Traefik ForwardAuth 校验后，网关把 `X-Auth-Tenant-ID`（以及 `X-Auth-User-ID` / `X-Auth-Platform-Admin` 等）注入转发头，各服务的鉴权中间件读头写入请求上下文——DAO 与 GORM 插件都从上下文取租户。

> 业务服务端口只绑 loopback（`SERVICES_BIND_IP=127.0.0.1`），`X-Auth-*` 头只能来自网关，杜绝伪造。

## 套餐 = 权限包

- 套餐定义一组权限码上限；租户绑定套餐后，其管理员给角色分配权限时**越界即拦截**（`POST /roles/:id/permissions` 校验分配集合 ⊆ 套餐集合，超出的权限码在错误信息里逐个列出）。
- 豁免情形：平台管理员操作、租户未绑定套餐。
- **套餐改小的语义**：不回收租户内已分配的越界权限，只拦截后续新分配——避免一次套餐调整引发存量角色大面积静默失效。

## 操作走查：开一个新租户

1. **建套餐**（系统管理 → 租户套餐）：新增套餐，勾选允许的权限码集合。
2. **建租户**（系统管理 → 租户管理）：填租户码/名称，绑定套餐，设置人数上限与状态。
3. **进入租户开号**：平台管理员通过请求头 `X-Act-Tenant-ID` 以租户身份操作（前端「以租户身份」入口），为租户建管理员角色与账号；或把租户码交给对方，由其管理员自行登录管理。
4. **验证约束**：用租户管理员给角色分配套餐外的权限点，应收到明确的越界报错。

## 接口速查

| 方法 | 路径 | 权限码 | 用途 |
|------|------|--------|------|
| GET | `/api/v1/tenants` | `system:tenant:list` | 租户列表（关键词/状态筛选） |
| POST | `/api/v1/tenants` | `system:tenant:create` | 创建租户（code/name/套餐/人数上限/状态） |
| GET | `/api/v1/tenants/:id` | `system:tenant:detail` | 租户详情（含用户数） |
| PUT | `/api/v1/tenants/:id` | `system:tenant:update` | 更新租户 |
| GET | `/api/v1/tenant-packages` | `system:tenant-package:list` | 套餐列表 |
| GET | `/api/v1/tenant-packages/all` | 同上 | 全量套餐（下拉用） |
| POST | `/api/v1/tenant-packages` | `system:tenant-package:create` | 创建套餐 |
| PUT | `/api/v1/tenant-packages/:id` | `system:tenant-package:update` | 更新套餐 |
| DELETE | `/api/v1/tenant-packages/:id` | `system:tenant-package:delete` | 删除套餐（有租户绑定则拒绝） |

租户与套餐管理页面仅平台侧可见（`/system/tenant`、`/system/tenant-packages`）。

## 给二次开发者的须知

- 新业务表要进租户隔离：模型加 `TenantID uint` 字段并映射 `tenant_id` 列即可，插件自动接管，无需注册。
- 平台级全局表：不加 `tenant_id` 列；需要在租户上下文里跨租户读写时用 `tenant.DisableScope(ctx)`，并把调用点当敏感代码评审。
- **已知边界**：GORM 插件不覆盖 `Raw()` / `Exec()` 原生 SQL（仓内 DAO 全走 ORM，写原生 SQL 时必须自带租户条件）；单体产品线无多租户，此能力仅微服务线。
