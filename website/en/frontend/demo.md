# Demo Mode (Online Demo)

The [online demo](https://superiorchuo.github.io/gopherforge/) is the **same frontend** built with `VITE_DEMO=1`: no backend is contacted — `src/demo/index.ts` installs a custom axios adapter that answers every `/api` request with **fake data**. Any account/password logs in.

## Build Entry

```bash
VITE_DEMO=1 VITE_BASE=/gopherforge/ npm run build
```

- `main.tsx` dynamically `import('./demo')` + `installDemoAdapter()` only when `VITE_DEMO === '1'` — the branch is statically eliminated from normal builds.
- `App.tsx` renders a "Demo mode · pure-frontend fake data" badge under demo mode.
- `VITE_BASE=/gopherforge/` is for the GitHub Pages sub-path deployment (the `deploy-demo.yml` workflow); use `/` for local `preview`.

## The Adapter (`src/demo/index.ts`)

1. `request.defaults.adapter = async (config) => ...` takes over every request.
2. It keeps `routes: Array<[method, RegExp, handler]>` and matches on `method + URL regex`; a hit returns constructed fake data.
3. Uncovered `GET` endpoints return an **empty list** `{ list: [], total: 0 }`; write endpoints politely reject with `400` — so no page crashes, it's just empty.

## Adding Fake Data for a New Page

Three steps (the recently added "monitoring alerts" example is a good reference):

1. **Menu row** — add one to `menuRows` like `monitor-redis`:

```ts
{ id: 42, name: 'monitor-alerts', title: '告警管理', icon: 'alert',
  path: '/monitor/alerts', component: 'monitor/alerts/index',
  parent_id: 30, sort: 5, status: 1, hidden: 0, permission: 'system:alert:list' }
```

   - The `permission` field is auto-collected into the `permissions` array (`demoUser` holds all of them), so button-level permission works too.
   - `icon` must be a key present in `MainLayout`'s `ICON_MAP`.

2. **Data arrays** — define fake data (e.g. `alertRules` / `alertEvents`) covering several states/edges to look realistic; type multi-shaped arrays explicitly (see `DemoAlertRule`).

3. **Route entries** — register in `routes` by method + URL regex; `GET` uses `paged(list, query)`:

```ts
['get', /^\/api\/v1\/monitor\/alert-rules$/, (_m, _b, q) => paged(alertRules, q)],
['post', /^\/api\/v1\/monitor\/alert-rules$/, (_m, body) => { /* unshift fake record */ }],
['delete', /^\/api\/v1\/monitor\/alert-rules\/(\d+)$/, (m) => { /* splice */ }],
```

**Prerequisite**: the page must already exist in the real route table (`src/router/index.tsx`) — demo only provides data; routes and pages are shared.

## Verification

```bash
VITE_DEMO=1 npm run build && npm run preview   # open the page and check fake data renders
npm run lint                                   # don't forget lint
```
