# RBAC 权限体系

identity + system 两个服务合力提供完整的 RBAC：**用户—角色—权限** 三级 + **菜单 / 按钮 / 数据** 三个控制粒度。

## 组成

| 对象 | 说明 |
|------|------|
| 用户 | 支持部门归属、多岗位、启用/禁用、Excel 批量导入导出 |
| 角色 | 挂权限点集合 + 数据范围；`super_admin` 直通全部 |
| 权限点 | 码约定 `{domain}:{resource}:{action}`，如 `system:user:create` |
| 菜单 | 树结构种子下发，按角色权限过滤；前端据此渲染侧栏 |
| 部门 | 树结构，含主管（`leader_user_id`，审批流「部门主管」规则据此选人） |
| 岗位 | 用户多对多关联，用户表单可选岗 |

## 三个控制粒度

**1. 接口级**：路由挂权限中间件。用户权限集合缓存在 Redis Set（key `user:permissions:<userID>`，TTL 1 小时），miss 时回源 DB（users → user_roles → roles → role_permissions → permissions 联查）并写回。`super_admin` 角色与 `*` / `*:*:*` 通配权限码直通。

**2. 按钮级**：前端 `usePermission()` 钩子控制显隐：

```tsx
const { hasPerm } = usePermission()

{hasPerm('system:user:create') && (
  <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新增用户</Button>
)}
{hasPerm('system:user:delete') && (
  <Popconfirm title="确认删除该用户？" onConfirm={() => handleDelete(record.id)}>
    <Button type="link" danger>删除</Button>
  </Popconfirm>
)}
```

**3. 数据级（数据范围）**：角色的 `data_scope` 字段决定列表查询能看到谁的数据，经 GORM 数据范围插件在查询前自动注入过滤条件（已接入用户、文件、登录日志、操作日志等模型）。六档取值：

| 取值 | 含义 |
|------|------|
| `all` | 全部数据 |
| `department` | 仅本部门 |
| `department_tree` | 本部门及以下（树状展开） |
| `self` | 仅本人（新角色默认值） |
| `custom` | 自定义部门列表（存 `role_data_scope_departments` 关联表） |
| `none` | 无数据权限 |

## 操作走查：给新同事开账号并授权

1. **建部门**（系统管理 → 部门管理）：树上选父节点新增；如后续要用审批流的「部门主管」规则，记得指定主管用户。
2. **建角色**（系统管理 → 角色管理）：新增角色 → 在权限树上勾选权限点 → 选数据范围（如「本部门及以下」）。
3. **建用户**（系统管理 → 用户管理）：填用户名/初始密码，挂部门与岗位，分配上一步的角色。批量开号用 Excel 导入（见下）。
4. **验证**：用新账号登录，侧栏只出现有权限的菜单，无权按钮不渲染，列表数据按数据范围过滤。

> 权限缓存 TTL 为 1 小时，但「分配角色权限」接口成功后会主动清缓存，改权限即时生效，无需等待。

## Excel 批量导入 / 导出

| 端点 | 权限码 | 说明 |
|------|--------|------|
| `GET /api/v1/users/import-template` | `system:user:create` | 下载导入模板 |
| `POST /api/v1/users/import` | `system:user:create` | multipart 上传（字段 `file`，≤5MB） |
| `GET /api/v1/users/export` | `system:user:list` | 导出（应用数据范围过滤） |

模板列：用户名*（租户内唯一）、昵称、初始密码（留空用默认密码，≥6 位）、邮箱、手机号、部门名称（须已存在）、状态（启用/禁用）。**部分成功语义**：单行失败不中断其他行，逐行错误明细回传前端展示。

## 接口速查

六组资源同构（`list / create / update / delete` 权限码按资源替换），以下列代表性端点：

| 方法 | 路径 | 权限码 | 用途 |
|------|------|--------|------|
| GET | `/api/v1/users` | `system:user:list` | 用户列表（分页 + 数据范围） |
| POST | `/api/v1/users` | `system:user:create` | 创建用户 |
| PUT | `/api/v1/users/:id` | `system:user:update` | 更新用户 |
| PUT | `/api/v1/users/:id/status` | `system:user:update` | 启用/禁用 |
| POST | `/api/v1/users/:id/roles` | `system:user:update` | 分配用户角色 |
| GET | `/api/v1/roles` · `/roles/all` | `system:role:list` | 角色列表 / 全量下拉 |
| POST | `/api/v1/roles/:id/permissions` | `system:role:update` | 分配角色权限（受租户套餐约束，见[多租户](/modules/tenant)） |
| GET | `/api/v1/permissions/tree` | `system:permission:list` | 权限树 |
| GET | `/api/v1/menus/tree` | `system:menu:list` | 菜单树 |
| GET | `/api/v1/departments/tree` | `system:department:list` | 部门树 |
| GET | `/api/v1/posts` | `system:post:list` | 岗位列表 |

角色 / 权限 / 部门 / 岗位的 CRUD 端点与用户同构：`POST /api/v1/roles`（`system:role:create`）、`DELETE /api/v1/permissions/:id`（`system:permission:delete`）等，完整清单见 OpenAPI 契约（`make api-contract` 生成）。

## 默认种子

初始迁移播种：`super_admin`（全部权限、数据范围 `all`）与一个只读默认角色；顶层菜单为仪表盘 / 系统管理 / 系统监控 / 个人中心，系统管理下含用户、角色、权限、菜单、部门、文件、字典、公告、在线用户、操作日志、登录日志等子项。默认账号 `admin / admin123`（生产必改）。

## 给二次开发者的建议

- 新业务的权限点用 SQL 迁移播种，并补挂到 `super_admin`；菜单同理（代码生成器会顺手产出 `menu-<module>.sql`）。
- 不要在代码里硬编码角色名做判断——`super_admin` 直通已由中间件统一处理，业务代码只关心权限码。
- 权限码沿用 `{domain}:{resource}:{action}` 约定，前后端共用同一套码。
