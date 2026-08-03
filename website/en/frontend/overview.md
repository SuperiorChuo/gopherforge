# Frontend Overview

GopherForge's frontend is a **single-page application** that talks to the 7 Go microservices through the Traefik gateway. This group of docs covers the frontend only: stack, directory layout, request layer, routing & permissions, page conventions, state & theme, and demo mode. Backend modules live under [Modules](/en/modules/auth).

## Tech Stack

| Layer | Choice | Notes |
|-------|--------|-------|
| Framework | **React 19** + TypeScript | Function components + Hooks |
| Build | **Vite 8** | `@` alias → `src/`, route-level code splitting |
| UI | **Ant Design 6** | `ConfigProvider` theming, light/dark toggle |
| State | **Redux Toolkit 2** + react-redux | Global shared state only (auth); page-local state uses `useState` |
| Routing | **react-router-dom 6** | Static route table + `lazyLoad()` |
| Lint | **oxlint** (`npm run lint`) | Must pass before commit |

## Directory Layout (`microservices/web/src/`)

| Directory | Responsibility |
|-----------|----------------|
| `api/` | Per-module API wrappers (`auth.ts` / `bpm.ts` / `monitor/` / `system/` …), all through `src/utils/request` |
| `components/` | Shared components (`TableToolbar`, `StatusPill`, `GlassEmpty`, `ExcelImportModal`, `GeoMap` …) |
| `demo/` | Demo-mode (`VITE_DEMO=1`) fake-data adapter; statically eliminated from normal builds |
| `hooks/` | Custom hooks (`usePermission`, `useCountUp`, `useUrlParams`, typed store hooks …) |
| `layouts/` | `MainLayout`: sidebar, header, theme toggle, route guard |
| `pages/` | Feature pages: `dashboard` / `login` / `system` / `monitor` / `bpm` / `oauth` / `profile` / `result` |
| `router/` | Static route table `index.tsx` + route↔permission map `route-permissions.ts` |
| `store/` | Redux Toolkit store and slices (`slices/authSlice.ts`) |
| `theme/` | Theme context `ThemeContext.ts` (dark/light) |
| `types/` | Global TS types (`PageRequest` / `PageResponse` / `ApiResponse` / entities) |
| `utils/` | `request.ts` (axios instance & interceptors), `feedback.ts` (message/notification/modal), `format.ts`, `sse.ts` … |
| `App.tsx` | App composition: Store → Theme → ConfigProvider → Router |
| `main.tsx` | Entry: demo-mode gate + `createRoot` |

## Build & Run

```bash
npm run dev        # local dev (Vite HMR; see port cheat-sheet)
npm run dev:lan    # LAN access (--host 0.0.0.0 --port 13200)
npm run build      # must pass before commit: tsc -b (type-check) + vite build
npm run lint       # oxlint
npm run preview    # preview a build
```

- `vite.config.ts`: `base` from `VITE_BASE` (`/gopherforge/` when deploying to the GitHub Pages sub-path); `@` alias; build splits lazy route chunks and deliberately groups map GeoJSON (`geo-data`) and antd icons (`icons`) for a leaner first screen.
- Demo build: `VITE_DEMO=1 VITE_BASE=/gopherforge/ npm run build`, see [Demo Mode](/en/frontend/demo).

## How It Talks to the Backend

1. **Single entry**: all `/api/v1/*` requests go to the Traefik gateway, which routes by path prefix (see [Architecture](/en/guide/architecture)).
2. **Envelope**: every response is `{ code, message, data }`; the axios interceptor unwraps it to `data` and errors automatically on `code !== 200` — business code never sees `code` (see [Request Layer](/en/frontend/request)).
3. **Pagination**: request `{ page, page_size }`, response `data` is `{ list, total, page, page_size }`.
4. **Per-module wrappers**: one file/folder under `src/api/` per backend service; types come from the backend OpenAPI contract or are handwritten (see [Request Layer · conventions](/en/frontend/request#conventions)).
5. **Menu & permissions come from the backend**: the sidebar menu comes from `/user/menus` (RBAC-filtered); button-level permission uses `usePermission` (see [Routing & Permissions](/en/frontend/routing)).

Next: read the [Request Layer](/en/frontend/request), or jump straight to [Page Development](/en/frontend/page-dev) to write your first page.
