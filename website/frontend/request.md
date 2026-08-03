# 请求层与 API 封装

所有请求走 `src/utils/request.ts` 导出的 axios 实例。它把「认证注入、信封解包、401 无感刷新、统一报错」都收敛在一处，业务代码只关心 `data`。

## Axios 实例

```ts
const instance = axios.create({
  baseURL: '',
  timeout: 15000,
  headers: { 'Content-Type': 'application/json' },
})
```

`baseURL` 为空——请求直接写网关相对路径（`/api/v1/...`），本地开发与生产经同一代理/网关。

## 请求拦截器（注入认证）

每次请求自动携带：

- `Authorization: Bearer <access_token>`：从本地存储读取当前登录对（token 双键管理，见下）。
- `X-Act-Tenant-ID`：多租户「切换租户」后携带，由 `getActTenantId()` 提供；未切换则不注入。
- 同时启动 `NProgress` 顶部进度条（响应后结束）。

## 响应拦截器（解包信封）

后端统一信封 `{ code, message, data }`：

```ts
if (body && typeof body === 'object' && 'code' in body) {
  if (body.code !== 200) {
    if (!response.config?.silent) message.error(body.message || '请求失败')
    return Promise.reject(new Error(body.message || '请求失败'))
  }
  return body.data   // 业务代码拿到的是 data
}
return body
```

要点：

- `code === 200` 直接返回 `data`，页面里 `const list = await getList()` 拿到的就是数据本体。
- `code !== 200` 自动 `message.error` 并 reject——**业务代码不需要 try/catch 来提示用户**。
- **`silent` 选项**：某个请求不想弹错误提示时，在配置里传 `{ silent: true }`（如进入页面时的兜底查询）。

## 401 无感刷新（多标签页协调）

收到 401 时，拦截器触发**单飞刷新**——同时只有一条刷新请求：

1. 用 `refresh_token` 调 `/api/v1/refresh` 换新对。
2. 多标签页场景：用 **Web Locks（IndexedDB）** 协调谁负责刷新，浏览器不支持时回退 **localStorage 租约**；其余标签页通过 `storage` 事件 / `BroadcastChannel` 等 token 更新后**自动重放原请求**。
3. 刷新成功后重试原请求（`_retry` 防循环）；刷新失败则清空登录态并跳登录页。

这意味着**并发的多个 401 不会各自刷一次**，也不会互踩 token。

## 封装惯例

每模块一个文件（`src/api/auth.ts`、`src/api/bpm.ts`、`src/api/monitor/index.ts`…），模式：

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

- 泛型第二参是返回值类型（因为拦截器解包后返回 `data`），第一参用 `unknown` 占位。
- 分页入参 `page` / `page_size`，出参 `{ list, total }`。
- 全局类型集中在 `src/types/index.ts`（`PageRequest` / `PageResponse` / `ApiResponse` / 各实体）；monitor 等模块的接口类型随后端 OpenAPI 契约生成到 `api/generated`。

## 反馈工具

弹提示**不要**直接用 antd 静态方法，用 `src/utils/feedback.ts` 导出的 `message` / `notification` / `modal`（经 `FeedbackBridge` 绑定到 `ConfigProvider` 主题，样式随主题一致）：

```ts
import { message } from '@/utils/feedback'
message.success('已保存')
```
