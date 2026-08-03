# 状态管理

前端用 **Redux Toolkit**，但**只在需要跨页面共享时才进全局 store**——当前只有一个 `auth` slice。页面局部状态一律 `useState`，不要为了「规范」把局部数据搬进 Redux。

## Store（`src/store/index.ts`）

```ts
export const store = configureStore({
  reducer: { auth: authReducer },
})
export type RootState = ReturnType<typeof store.getState>
export type AppDispatch = typeof store.dispatch
```

## auth slice（`src/store/slices/authSlice.ts`）

持有登录态相关：

| 状态 | 说明 |
|------|------|
| `token` / `refreshToken` | 登录对（`initialState` 从本地存储恢复，刷新后与请求层共享） |
| `userInfo` | 当前用户（含 roles） |
| `menus` | 当前用户菜单树（来自 `/user/menus`，侧栏用） |
| `permissions` | 权限码数组（按钮级 `usePermission` 读取） |
| `loading` | 登录中 |

**异步动作**：

- `login(data)`：调登录接口，成功即 `setTokens` 写本地存储，并把登录响应里 user 自带的 `roles/permissions` **同步进 state**——注释里特别说明：若不同步，侧栏按权限过滤会得到空菜单（`fetchCurrentUser` 在已有 `userInfo` 时会被跳过）。
- `fetchCurrentUser()`：`Promise.all([getCurrentUser(), getUserMenus()])` 并行的用户 + 菜单，页面刷新后恢复登录态用。
- `logout()`：注销前撤销当前轮换后的 refresh token，再 `clearTokens`。

**同步动作**：`setTokenPair`（无感刷新后回写）、`clearAuth`。

## Typed Hooks（`src/hooks/store.ts`）

```ts
export const useAppDispatch = () => useDispatch<AppDispatch>()
export const useAppSelector = <T>(selector: (state: RootState) => T) => useSelector(selector)
```

组件里用这两个 typed hooks 取全局状态：

```ts
const userInfo = useAppSelector((s) => s.auth.userInfo)
const dispatch = useAppDispatch()
```

## 使用约定

1. **全局共享态才进 Redux**：跨页面要读的状态（登录态、权限、主题在 Context 里）。单页内部的数据流用 `useState` + `useEffect`（见[页面开发规范](/frontend/page-dev)）。
2. **读权限用 `usePermission`**（`s.permissions` 的封装 + `super_admin` 恒有），不要直接 `useAppSelector((s) => s.auth.permissions)`。
3. **不把「接口返回的整张列表」存 Redux**：列表数据是页面局部的，页面卸载即清。
