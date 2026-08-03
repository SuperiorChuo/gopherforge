# 前端架构总览

GopherForge 前端是**单页应用**（SPA），与后端 7 个微服务经 Traefik 网关统一交互。本组文档只讲前端：技术栈、目录结构、请求层、路由权限、页面规范、状态与主题，以及演示模式。后端模块见[功能模块](/modules/auth)。

## 技术栈

| 层 | 选型 | 说明 |
|----|------|------|
| 框架 | **React 19** + TypeScript | 函数组件 + Hooks |
| 构建 | **Vite 8** | `@` 别名指向 `src/`，产物按路由懒加载拆分 |
| UI | **Ant Design 6** | `ConfigProvider` 统一主题，双主题切换 |
| 状态 | **Redux Toolkit 2** + react-redux | 仅存全局共享态（认证），局部状态用 `useState` |
| 路由 | **react-router-dom 6** | 静态路由表 + `lazyLoad()` 按需加载 |
| 校验 | **oxlint**（`npm run lint`） | 零配置、快，提交前必须通过 |

## 目录结构（`microservices/web/src/`）

| 目录 | 职责 |
|------|------|
| `api/` | 按后端模块分的接口封装（`auth.ts` / `bpm.ts` / `monitor/` / `system/` …），统一用 `src/utils/request` |
| `components/` | 跨页面公共组件（`TableToolbar`、`StatusPill`、`GlassEmpty`、`ExcelImportModal`、`GeoMap` …） |
| `demo/` | 演示模式（`VITE_DEMO=1`）假数据 adapter，正常构建静态消除 |
| `hooks/` | 自定义 Hooks（`usePermission`、`useCountUp`、`useUrlParams`、typed store hooks …） |
| `layouts/` | `MainLayout`：侧栏、顶栏、主题切换、路由守卫 |
| `pages/` | 按模块的页面：`dashboard` / `login` / `system` / `monitor` / `bpm` / `oauth` / `profile` / `result` |
| `router/` | 静态路由表 `index.tsx` + 路由↔权限码映射 `route-permissions.ts` |
| `store/` | Redux Toolkit store 与 slices（`slices/authSlice.ts`） |
| `theme/` | 双主题上下文 `ThemeContext.ts` |
| `types/` | 全局 TS 类型（`PageRequest` / `PageResponse` / `ApiResponse` / 各类实体） |
| `utils/` | `request.ts`（Axios 实例与拦截器）、`feedback.ts`（message/notification/modal）、`format.ts`、`sse.ts` 等 |
| `App.tsx` | 应用组装：Store → 主题 → ConfigProvider → 路由 |
| `main.tsx` | 入口：演示模式门控 + `createRoot` |

## 构建与启动

```bash
npm run dev        # 本地开发（Vite HMR，端口 13100/13200，见项目端口速查）
npm run dev:lan    # 局域网访问（--host 0.0.0.0 --port 13200）
npm run build      # 提交前必须过：tsc -b（类型检查）+ vite build
npm run lint       # oxlint
npm run preview    # 预览构建产物
```

- `vite.config.ts`：`base` 由 `VITE_BASE` 决定（部署到 GitHub Pages 子路径时为 `/gopherforge/`）；`@` 别名；构建时对懒加载路由包做拆分，并刻意把地图 GeoJSON（`geo-data`）与 antd 图标（`icons`）单独分组优化首屏。
- 演示模式构建：`VITE_DEMO=1 VITE_BASE=/gopherforge/ npm run build`，见[演示模式](/frontend/demo)。

## 与后端如何协作

1. **统一入口**：所有 `/api/v1/*` 请求打到 Traefik 网关，由网关按路径前缀路由到对应服务（见[架构总览](/guide/architecture)）。
2. **响应信封**：后端统一返回 `{ code, message, data }`，前端 Axios 拦截器解包成 `data`，`code !== 200` 自动报错——业务代码里永远拿不到 `code`（见[请求层](/frontend/request)）。
3. **分页约定**：请求带 `{ page, page_size }`，响应 `data` 内是 `{ list, total, page, page_size }`。
4. **按模块封装**：每个后端服务对应 `src/api/` 下一个文件/目录，前端类型从后端 OpenAPI 契约或手写（见[请求层·封装惯例](/frontend/request#封装惯例)）。
5. **菜单与权限由后端下发**：侧栏菜单来自 `/user/menus`（RBAC 已过滤），按钮级权限用 `usePermission`（见[路由与权限](/frontend/routing)）。

下一步：看[请求层](/frontend/request)理解 axios 封装，或直接跳到[页面开发规范](/frontend/page-dev)写第一个页面。
