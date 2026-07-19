import { createSlice, createAsyncThunk, type PayloadAction } from '@reduxjs/toolkit'
import { login as loginAPI, getCurrentUser, getUserMenus, logout as logoutAPI } from '@/api/auth'
import { setTokens, clearTokens } from '@/utils/request'
import type { LoginRequest, UserInfo, MenuItem } from '@/types'

interface AuthState {
  token: string | null
  refreshToken: string | null
  userInfo: UserInfo | null
  menus: MenuItem[]
  permissions: string[]
  loading: boolean
}

const initialState: AuthState = {
  token: localStorage.getItem('access_token'),
  refreshToken: localStorage.getItem('refresh_token'),
  userInfo: null,
  menus: [],
  permissions: [],
  loading: false,
}

export const login = createAsyncThunk('auth/login', async (data: LoginRequest) => {
  const res = await loginAPI(data)
  if (res.access_token && res.refresh_token) {
    setTokens(res.access_token, res.refresh_token)
  }
  return res
})

export const fetchCurrentUser = createAsyncThunk('auth/fetchCurrentUser', async () => {
  const [user, menus] = await Promise.all([getCurrentUser(), getUserMenus()])
  return { user, menus }
})

export const logout = createAsyncThunk('auth/logout', async (_, { getState }) => {
  const state = getState() as { auth: AuthState }
  const refreshToken = state.auth.refreshToken
  try {
    if (refreshToken) await logoutAPI({ refresh_token: refreshToken })
  } finally {
    clearTokens()
  }
})

const authSlice = createSlice({
  name: 'auth',
  initialState,
  reducers: {
    setTokenPair(state, action: PayloadAction<{ access: string; refresh: string }>) {
      state.token = action.payload.access
      state.refreshToken = action.payload.refresh
    },
    clearAuth(state) {
      state.token = null
      state.refreshToken = null
      state.userInfo = null
      state.menus = []
      state.permissions = []
    },
  },
  extraReducers: (builder) => {
    builder
      .addCase(login.pending, (state) => { state.loading = true })
      .addCase(login.fulfilled, (state, action) => {
        state.loading = false
        if (action.payload.access_token) state.token = action.payload.access_token
        if (action.payload.refresh_token) state.refreshToken = action.payload.refresh_token
        // 登录响应里的 user 已含 roles/permissions；必须同步 permissions，
        // 否则侧栏按权限过滤时会得到空菜单（fetchCurrentUser 在已有 userInfo 时会被跳过）。
        if (action.payload.user) {
          state.userInfo = action.payload.user
          state.permissions = action.payload.user.permissions || []
        }
      })
      .addCase(login.rejected, (state) => { state.loading = false })
      .addCase(fetchCurrentUser.fulfilled, (state, action) => {
        state.userInfo = action.payload.user
        state.menus = action.payload.menus || []
        state.permissions = action.payload.user.permissions || []
      })
      .addCase(logout.fulfilled, (state) => {
        state.token = null
        state.refreshToken = null
        state.userInfo = null
        state.menus = []
        state.permissions = []
      })
  },
})

export const { setTokenPair, clearAuth } = authSlice.actions
export default authSlice.reducer
