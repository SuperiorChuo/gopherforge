# 多租户与套餐

GopherForge 内置 SaaS 化底座：**共享库 + tenant_id 行级隔离**，登录时带租户码进入对应租户空间。

## 隔离模型

- 所有租户级表统一带 `tenant_id`（`not null;default:1;index`）。
- 请求经网关注入 `X-Auth-Tenant-ID`，服务写入请求上下文。
- **双层防线**：DAO 手写租户过滤是第一道；**租户隔离 GORM 插件**是第二道——凡模型带 `tenant_id` 列，查询自动过滤、创建自动补值、改删按租户约束（跨租户按 id 猜测直接打空）。漏挂 scope 不再等于越权。
- 平台级表（tenants、tenant_packages 等无 `tenant_id` 列）天然豁免；平台管理员跨租户操作走显式 `DisableScope` 逃生口。

## 租户套餐 = 权限包

- 套餐定义一组权限点上限；租户绑定套餐后，其管理员给角色分配权限时**越界即拦截**。
- 平台管理员可代租户操作（前端"以租户身份"切换），套餐/租户 CRUD 仅平台侧可见。

## 已知边界

- GORM 插件不覆盖 `Raw`/`Exec` 原生 SQL（仓内 DAO 全走 ORM）。
- 单体产品线（Monolith 形态）无多租户，此能力仅微服务线。
