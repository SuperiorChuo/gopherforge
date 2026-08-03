# 演示模式（在线 Demo）

[在线 Demo](https://superiorchuo.github.io/gopherforge/) 是**同一个前端**用 `VITE_DEMO=1` 构建出来的：不连后端，靠 `src/demo/index.ts` 给 axios 装一个自定义 adapter，用**假数据**模拟所有 `/api` 请求。任意账号可登录。

## 构建入口

```bash
VITE_DEMO=1 VITE_BASE=/gopherforge/ npm run build
```

- `main.tsx` 里 `if (import.meta.env.VITE_DEMO === '1')` 动态 `import('./demo')` 并 `installDemoAdapter()`——正常构建时该分支被静态消除，不污染产物。
- `App.tsx` 在 demo 下会渲染一个「演示模式 · 纯前端假数据」角标。
- `VITE_BASE=/gopherforge/` 是为部署到 GitHub Pages 子路径（`deploy-demo.yml` 工作流），本地 `preview` 时用 `/`。

## adapter 机制（`src/demo/index.ts`）

1. `request.defaults.adapter = async (config) => ...` 接管所有请求。
2. 内部维护 `routes: Array<[method, RegExp, handler]>`，按 `method + URL 正则` 匹配，命中就返回构造的假数据。
3. 未覆盖的读接口（`GET`）返回**空列表** `{ list: [], total: 0 }`，写接口礼貌拒绝 `400 演示模式暂不支持该操作`——保证任何页面都不会崩，只是空。

## 给新页面补假数据

想让某个页面在 Demo 里可见、有内容，三步（参照最近加的「监控告警」示例）：

1. **菜单行**：在 `menuRows` 数组里按 `monitor-redis` 的样子加一行：

```ts
{ id: 42, name: 'monitor-alerts', title: '告警管理', icon: 'alert',
  path: '/monitor/alerts', component: 'monitor/alerts/index',
  parent_id: 30, sort: 5, status: 1, hidden: 0, permission: 'system:alert:list' }
```

   - `permission` 字段会被自动收进 `permissions` 数组（`demoUser` 拥有全部权限），按钮级权限随之生效。
   - `icon` 必须是 `MainLayout` 的 `ICON_MAP` 里存在的键。

2. **数据数组**：定义假数据（如 `alertRules` / `alertEvents`），覆盖多种状态/边界让页面看起来真实；类型用页面 API 的 TS 类型，多形态数组就显式定义类型（参照 `DemoAlertRule`）。

3. **路由条目**：在 `routes` 数组按 method + URL 正则注册，`GET` 用 `paged(list, query)` 做分页：

```ts
['get', /^\/api\/v1\/monitor\/alert-rules$/, (_m, _b, q) => paged(alertRules, q)],
['post', /^\/api\/v1\/monitor\/alert-rules$/, (_m, body) => { /* unshift 假记录 */ }],
['delete', /^\/api\/v1\/monitor\/alert-rules\/(\d+)$/, (m) => { /* splice */ }],
```

**前提**：真实前端路由表（`src/router/index.tsx`）里必须有该页面——demo 只负责数据，路由与页面是共用的。

## 验证

```bash
VITE_DEMO=1 npm run build && npm run preview   # 本地模拟，点进新页面看假数据渲染
npm run lint                                   # 别忘 lint
```
