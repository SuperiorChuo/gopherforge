# 多租户 SaaS 底座设计

> 状态：**M1 落地中**（共享库 + `tenant_id` 行级隔离）  
> 范围：微服务线 `microservices/`；单体线本期不同步完整租户模型  
> 原则：信任 JWT / ForwardAuth 中的 `tenant_id`，不信任客户端自报租户

相关：[`../EXPANSION_PLAN.md`](../EXPANSION_PLAN.md) Phase 5 · [`../PRODUCT_LINES.md`](../PRODUCT_LINES.md)

---

## 1. 目标与非目标

### 1.1 M1 目标

1. 引入 `tenants` 表与默认租户 `default`（id=1）
2. 核心身份表带 `tenant_id`：`users` / `roles` / `departments`
3. JWT 携带 `tenant_id`；网关转发 `X-Auth-Tenant-ID`
4. 登录支持可选 `tenant_code`（缺省 `default`）
5. 管理台可 CRUD 租户；用户列表按租户过滤
6. 现有单租户演示路径零感迁移（全量回填 tenant_id=1）

### 1.2 非目标（M1 不做）

- Schema-per-tenant / DB-per-tenant
- 独立 `tenant-service` 进程（能力先挂 identity）
- 套餐计费、配额计量、插件市场
- 全表覆盖（file / audit / AI / IM 的 `tenant_id` 后续分期）
- 子域名自动解析租户（可后置）

---

## 2. 模型

### 2.1 `tenants`

| 字段 | 说明 |
|------|------|
| id | PK |
| code | 唯一 slug，登录用（如 `default`、`acme`） |
| name | 展示名 |
| status | 1 启用 0 停用 |
| plan | 预留套餐 code（M1 默认 `free`） |
| max_users | 预留配额（0=不限） |
| created_at / updated_at | |

### 2.2 行级字段

| 表 | 唯一性变化 |
|----|------------|
| users | `(tenant_id, username)`；email/phone 改为租户内唯一（非空时） |
| roles | `(tenant_id, code)` |
| departments | `(tenant_id, code)` |
| permissions / menus | **全局共享**（平台权限目录，不按租户复制） |

### 2.3 角色语义

| 角色 | M1 行为 |
|------|---------|
| `super_admin` | 仍在默认租户内拥有全部权限码；**可管理租户列表** |
| 租户内角色 | 仅见本租户 users/roles/depts |

跨租户「平台运营账号」完整模型（platform_admin）→ M2。

---

## 3. 鉴权链路

```text
Login(username, password, tenant_code?)
  → resolve tenant (default if empty)
  → user WHERE tenant_id=? AND username=?
  → JWT { user_id, username, tenant_id, token_type, jti }

Traefik forwardAuth /internal/verify
  → X-Auth-User-ID, X-Auth-Username, X-Auth-Tenant-ID

各服务 AuthMiddleware
  → c.Set("user_id"), c.Set("username"), c.Set("tenant_id")
```

Refresh / TOTP / WS ticket 均透传 `tenant_id`。

---

## 4. API（M1）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/login` | body 增加可选 `tenant_code` |
| GET | `/api/v1/tenants` | 列表（需 `system:tenant:list`） |
| POST | `/api/v1/tenants` | 创建 |
| PUT | `/api/v1/tenants/:id` | 更新 |
| GET | `/api/v1/tenants/:id` | 详情 |
| GET | `/api/v1/user/me` | 响应含 `tenant_id`（随 user 字段） |

权限码：`system:tenant:list|create|update|detail`（种子给 super_admin）。

---

## 5. 分期

| 阶段 | 内容 |
|------|------|
| **M1** | 本文：表 + JWT + 登录 + 租户 CRUD + 用户列表隔离 |
| **M2** | ✅ 角色/部门强制租户；创建用户绑 tenant；GORM 租户插件；分配角色/部门跨租户拒绝 |
| **M3** | file/audit/AI/IM 补 `tenant_id`；配额与 plan |
| **M4** | 子域名 / 独立登录页；platform_admin；计费对接 |

---

## 6. 迁移注意

- goose：`microservices/services/monitor/migrations/000012_add_tenants.sql`
- 回填后 drop 全局 username 唯一索引，再建复合唯一
- 空 email/phone：使用部分唯一索引 `WHERE email <> ''` 避免多空串冲突

---

## 7. 修订记录

| 日期 | 说明 |
|------|------|
| 2026-07-16 | M1 初稿并实现 |
| 2026-07-16 | M2：GORM tenant 插件 + 角色/部门/用户边界加固 |
