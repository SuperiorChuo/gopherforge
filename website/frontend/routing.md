# 路由与权限

GopherForge 采用「**静态路由表 + 后端下发菜单**」的组合：路由组件由前端静态注册，侧栏菜单由后端 `/user/menus` 按当前用户权限下发。页面能不能**访问**由路由守卫决定，按钮能不能**点**由 `usePermission` 决定。

## 静态路由表（`src/router/index.tsx`）

所有页面路由集中在一个 `RouteObject[]`，例如：

```ts
{ path: 'system/user', element: lazyLoad(() => import('@/pages/system/user')) },
{ path: 'monitor/alerts', element: lazyLoad(() => import('@/pages/monitor/alerts')) },
```

- **`lazyLoad()`**：路由级按需加载——进入页面才拉对应 chunk，配合 `vite` 自动分包控制首屏体积。
- **`prefetchMainLayout()`**：登录后空闲时预取主布局，减少首次跳转的等待。
- 路由写在 `MainLayout` 的子路由下（`/` 是 `importMainLayout` 的懒加载）。

## 侧栏菜单来自后端

- 登录后前端调 `/user/menus`，拿到**已按 RBAC 过滤**的菜单树。
- `MainLayout.tsx` 里的 `apiMenusToDefs()` 把后端菜单转成 antd 侧栏项；`iconOf()` 把菜单的 `icon` 字符串（如 `'alert'`、`'server'`）映射到 antd 图标。
- 因此**新增页面 = 前端注册路由 + 后端菜单种子**（菜单没有该权限的用户在侧栏看不到、直接访问也被守卫拦下）。见[页面开发规范](/frontend/page-dev)。

## 路由守卫与权限码

- `src/router/route-permissions.ts`：维护「路由路径 ↔ 权限码」映射（`'/monitor/alerts': 'system:alert:list'`），路由守卫据此拦截无权限的直接访问。
- 后端菜单权限码必须与之**对齐**：同一个页面在菜单种子里也要给 `permission: 'system:alert:list'`（demo 的菜单行同理）。

## 按钮级权限：`usePermission`

```ts
import { usePermission } from '@/hooks/usePermission'

const { hasPerm } = usePermission()
{hasPerm('system:alert:create') && <Button>新增规则</Button>}
```

- `hasPerm(code)`：当前用户（super_admin 恒有）是否持有权限码。
- 只控制**可见性**，真正的鉴权仍由后端接口权限中间件兜底——前端隐藏只是体验。

## 新增一个页面的完整步骤

1. `src/pages/<module>/<page>/index.tsx` 写页面（参照[页面开发规范](/frontend/page-dev)）。
2. `src/api/<module>.ts`（或已有文件）写接口封装。
3. `src/router/index.tsx` 注册路由，包在 `lazyLoad(() => import(...))` 里。
4. `src/router/route-permissions.ts` 补「路径 ↔ 权限码」。
5. 后端菜单种子补一条（`parent_id`、`path`、`component`、`permission`）——RBAC 下发给有权限的用户。
6. 演示模式想能在[在线 Demo](/frontend/demo) 里看到：`src/demo/index.ts` 补菜单行 + 假数据路由。
