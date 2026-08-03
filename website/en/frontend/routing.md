# Routing & Permissions

GopherForge combines a **static route table** with **backend-issued menus**: route components are registered statically in the frontend, while the sidebar menu comes from `/user/menus`, filtered by the current user's RBAC. Whether a page can be **opened** is decided by the route guard; whether a button can be **clicked** is decided by `usePermission`.

## Static Route Table (`src/router/index.tsx`)

All page routes live in one `RouteObject[]`, e.g.:

```ts
{ path: 'system/user', element: lazyLoad(() => import('@/pages/system/user')) },
{ path: 'monitor/alerts', element: lazyLoad(() => import('@/pages/monitor/alerts')) },
```

- **`lazyLoad()`**: route-level code splitting — the chunk is fetched when the page is entered, keeping the first screen lean.
- **`prefetchMainLayout()`**: prefetches the main layout in idle time after login.
- Routes are nested under `MainLayout` (`/` is the lazy-loaded `importMainLayout`).

## Sidebar Menu Comes From the Backend

- After login the frontend calls `/user/menus` and gets an **RBAC-filtered** menu tree.
- `MainLayout.tsx`'s `apiMenusToDefs()` turns backend menus into antd sidebar items; `iconOf()` maps the menu's `icon` string (e.g. `'alert'`, `'server'`) to an antd icon.
- So **adding a page = frontend route + backend menu seed**. Users without that menu permission don't see it in the sidebar, and direct navigation is blocked by the guard. See [Page Development](/en/frontend/page-dev).

## Route Guard & Permission Codes

- `src/router/route-permissions.ts`: a `path → permission code` map (`'/monitor/alerts': 'system:alert:list'`); the guard uses it to block direct access without the permission.
- The backend menu seed must carry the **same** permission code (and the demo menu row must match).

## Button-Level Permission: `usePermission`

```ts
import { usePermission } from '@/hooks/usePermission'

const { hasPerm } = usePermission()
{hasPerm('system:alert:create') && <Button>New rule</Button>}
```

- `hasPerm(code)`: whether the current user holds the code (`super_admin` always does).
- It only controls **visibility** — real enforcement is still the backend permission middleware; hiding in the UI is just UX.

## Adding a New Page

1. Write the page at `src/pages/<module>/<page>/index.tsx` (see [Page Development](/en/frontend/page-dev)).
2. Add API wrappers in `src/api/<module>.ts` (or an existing file).
3. Register the route in `src/router/index.tsx`, wrapped in `lazyLoad(() => import(...))`.
4. Add the `path ↔ permission code` entry to `src/router/route-permissions.ts`.
5. Add a backend menu seed (with `parent_id`, `path`, `component`, `permission`) — RBAC delivers it to entitled users.
6. To also see it in the [online demo](/en/frontend/demo): add a menu row + fake-data routes in `src/demo/index.ts`.
