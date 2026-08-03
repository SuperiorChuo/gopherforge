# State Management

The frontend uses **Redux Toolkit**, but only for state that must be shared across pages — there is currently a single `auth` slice. Page-local state always uses `useState`; don't move local data into Redux just for "consistency".

## Store (`src/store/index.ts`)

```ts
export const store = configureStore({
  reducer: { auth: authReducer },
})
export type RootState = ReturnType<typeof store.getState>
export type AppDispatch = typeof store.dispatch
```

## auth slice (`src/store/slices/authSlice.ts`)

Holds everything about the login session:

| State | Meaning |
|-------|---------|
| `token` / `refreshToken` | Token pair (initialised from localStorage; kept in sync with the request layer) |
| `userInfo` | Current user (incl. roles) |
| `menus` | The user's menu tree (from `/user/menus`, used by the sidebar) |
| `permissions` | Permission codes (read by button-level `usePermission`) |
| `loading` | Logging in |

**Async actions**:

- `login(data)`: calls the login API, persists the pair via `setTokens`, and **syncs `roles`/`permissions` from the login response user** — the code comment explains why: if not synced, the sidebar's permission filter yields an empty menu (`fetchCurrentUser` is skipped when `userInfo` already exists).
- `fetchCurrentUser()`: `Promise.all([getCurrentUser(), getUserMenus()])` — user + menus in parallel, used to restore the session after a refresh.
- `logout()`: revokes the currently rotated refresh token before clearing local state.

**Sync actions**: `setTokenPair` (write-back after a silent refresh), `clearAuth`.

## Typed Hooks (`src/hooks/store.ts`)

```ts
export const useAppDispatch = () => useDispatch<AppDispatch>()
export const useAppSelector = <T>(selector: (state: RootState) => T) => useSelector(selector)
```

```ts
const userInfo = useAppSelector((s) => s.auth.userInfo)
const dispatch = useAppDispatch()
```

## Conventions

1. **Only globally shared state goes into Redux** (session, permissions; theme lives in Context). Single-page data flow uses `useState` + `useEffect` (see [Page Development](/en/frontend/page-dev)).
2. **Read permissions via `usePermission`** (wraps `s.permissions` and treats `super_admin` as all-permitted) — don't reach into `s.auth.permissions` directly.
3. **Don't store entire API list responses in Redux** — list data is page-local and should be dropped when the page unmounts.
