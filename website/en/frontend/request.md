# Request Layer & API Wrappers

All requests go through the axios instance exported by `src/utils/request.ts`. It centralises auth injection, envelope unwrapping, silent 401 refresh and unified error toasts — business code only ever deals with `data`.

## Axios Instance

```ts
const instance = axios.create({
  baseURL: '',
  timeout: 15000,
  headers: { 'Content-Type': 'application/json' },
})
```

`baseURL` is empty — requests are written as gateway-relative paths (`/api/v1/...`), and both local dev and production reach the backend through the same proxy/gateway.

## Request Interceptor (auth injection)

Every request automatically carries:

- `Authorization: Bearer <access_token>` — from the persisted token pair (dual-key management, see below).
- `X-Act-Tenant-ID` — set when the user has switched tenant (`getActTenantId()`); omitted otherwise.
- It also starts the `NProgress` top progress bar (finished on response).

## Response Interceptor (envelope unwrap)

The backend always returns `{ code, message, data }`:

```ts
if (body && typeof body === 'object' && 'code' in body) {
  if (body.code !== 200) {
    if (!response.config?.silent) message.error(body.message || '请求失败')
    return Promise.reject(new Error(body.message || '请求失败'))
  }
  return body.data   // business code gets data
}
return body
```

Key points:

- On `code === 200` it returns `data` — `const list = await getList()` is already the payload body.
- On `code !== 200` it toasts `message.error` and rejects — **no try/catch needed in pages just to surface errors**.
- **`silent` option**: pass `{ silent: true }` in the request config to suppress the toast (e.g. for a best-effort fallback query on page mount).

## Silent 401 Refresh (multi-tab coordination)

On a 401 the interceptor triggers a **single-flight refresh** — only one refresh request at a time:

1. Exchange `refresh_token` for a new pair via `/api/v1/refresh`.
2. Multi-tab: **Web Locks (IndexedDB)** coordinates which tab performs the refresh, falling back to a **localStorage lease** when unsupported; other tabs wait for the token update (`storage` event / `BroadcastChannel`) and **replay the original request**.
3. On success the original request is retried (`_retry` guards against loops); on failure the login state is cleared and the user is redirected to the login page.

This means concurrent 401s never trigger parallel refreshes and never clobber each other's tokens.

## Conventions

One file per module (`src/api/auth.ts`, `src/api/bpm.ts`, `src/api/monitor/index.ts` …):

```ts
import request from '@/utils/request'
import type { PageRequest, PageResponse } from '@/types'

export const getList = (params: PageRequest & { name?: string }) =>
  request.get<unknown, PageResponse<Item>>('/api/v1/xxx', { params })

export const createItem = (data: Payload) =>
  request.post<unknown, Item>('/api/v1/xxx', data)

export const updateItem = (id: number, data: Payload) =>
  request.put<unknown, Item>(`/api/v1/xxx/${id}`, data)

export const deleteItem = (id: number) =>
  request.delete<unknown, void>(`/api/v1/xxx/${id}`)
```

- The second generic is the return type (the interceptor unwraps to `data`); the first is `unknown`.
- Pagination params `page` / `page_size`; response `{ list, total }`.
- Global types live in `src/types/index.ts` (`PageRequest` / `PageResponse` / `ApiResponse` / entities); monitor and friends generate types into `api/generated` from the backend OpenAPI contract.

## Feedback

For toasts, don't call antd static methods directly — use `src/utils/feedback.ts`'s `message` / `notification` / `modal` (bound to the `ConfigProvider` theme via `FeedbackBridge`):

```ts
import { message } from '@/utils/feedback'
message.success('Saved')
```
