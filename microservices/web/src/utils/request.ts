import axios, {
  type AxiosInstance,
  type AxiosRequestConfig,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from 'axios'
import NProgress from 'nprogress'
import 'nprogress/nprogress.css'
import { message } from './feedback'

const TOKEN_KEY = 'access_token'
const REFRESH_TOKEN_KEY = 'refresh_token'
const AUTH_TOKEN_CHANGE_KEY = 'auth_tokens_changed'
const AUTH_TOKEN_CHANNEL = 'go-admin-kit-auth'
const AUTH_REFRESH_LOCK_KEY = 'go-admin-kit-auth-refresh-lock'
const AUTH_REFRESH_LOCK_NAME = 'go-admin-kit-auth-refresh'
const AUTH_REFRESH_WAIT_MS = 10_000
const AUTH_REFRESH_RECOVERY_WAIT_MS = 1_500
const AUTH_REFRESH_LOCK_TTL_MS = 20_000
/** Platform admin act-as tenant (M4); honored only when JWT platform_admin=true */
const ACT_TENANT_KEY = 'act_tenant_id'

type TokenPair = {
  access: string
  refresh: string
}

type AuthRequestConfig = AxiosRequestConfig & {
  _retry?: boolean
  _authAccessToken?: string
  _authRefreshToken?: string
}

type AuthLockManager = {
  request<T>(name: string, callback: () => Promise<T>): Promise<T>
}

type RefreshLease = {
  owner: string
  expiresAt: number
}

const tabID = `${Date.now()}-${Math.random().toString(36).slice(2)}`
let authChannel: BroadcastChannel | null | undefined

function getAuthChannel() {
  if (authChannel !== undefined) return authChannel
  if (typeof BroadcastChannel === 'undefined') {
    authChannel = null
    return authChannel
  }
  try {
    authChannel = new BroadcastChannel(AUTH_TOKEN_CHANNEL)
  } catch {
    authChannel = null
  }
  return authChannel
}

function announceTokenChange() {
  localStorage.setItem(AUTH_TOKEN_CHANGE_KEY, `${Date.now()}:${tabID}:${Math.random()}`)
  getAuthChannel()?.postMessage({ type: 'tokens-updated' })
}

export const getActTenantId = () => localStorage.getItem(ACT_TENANT_KEY)
export const setActTenantId = (id: string | number | null) => {
  if (id === null || id === undefined || id === '') {
    localStorage.removeItem(ACT_TENANT_KEY)
    return
  }
  localStorage.setItem(ACT_TENANT_KEY, String(id))
}
export const clearActTenantId = () => localStorage.removeItem(ACT_TENANT_KEY)

// 允许单个请求关闭全局错误提示（如仪表盘的可选模块，无权限时静默降级）
declare module 'axios' {
  export interface AxiosRequestConfig {
    silent?: boolean
  }
}

export const getToken = () => localStorage.getItem(TOKEN_KEY)
export const getRefreshToken = () => localStorage.getItem(REFRESH_TOKEN_KEY)
export const setTokens = (access: string, refresh: string) => {
  localStorage.setItem(TOKEN_KEY, access)
  localStorage.setItem(REFRESH_TOKEN_KEY, refresh)
  announceTokenChange()
}
export const clearTokens = () => {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(REFRESH_TOKEN_KEY)
  announceTokenChange()
}

NProgress.configure({ showSpinner: false })

let refreshPromise: Promise<TokenPair> | null = null
let loginRedirectStarted = false

const readTokenPair = (): TokenPair => ({
  access: getToken() || '',
  refresh: getRefreshToken() || '',
})

const hasUsableTokenPair = (pair: TokenPair) => Boolean(pair.access && pair.refresh)

const tokenPairChanged = (current: TokenPair, previous: TokenPair) =>
  hasUsableTokenPair(current) &&
  (current.access !== previous.access || current.refresh !== previous.refresh)

function getWebLockManager(): AuthLockManager | null {
  if (typeof navigator === 'undefined') return null
  const locks = (navigator as Navigator & { locks?: AuthLockManager }).locks
  return locks ?? null
}

function parseRefreshLease(value: string | null): RefreshLease | null {
  if (!value) return null
  try {
    const lease = JSON.parse(value) as Partial<RefreshLease>
    if (typeof lease.owner !== 'string' || typeof lease.expiresAt !== 'number') return null
    return { owner: lease.owner, expiresAt: lease.expiresAt }
  } catch {
    return null
  }
}

function tryAcquireRefreshLease(): boolean {
  const current = parseRefreshLease(localStorage.getItem(AUTH_REFRESH_LOCK_KEY))
  if (current && current.owner !== tabID && current.expiresAt > Date.now()) return false

  const lease: RefreshLease = {
    owner: tabID,
    expiresAt: Date.now() + AUTH_REFRESH_LOCK_TTL_MS,
  }
  localStorage.setItem(AUTH_REFRESH_LOCK_KEY, JSON.stringify(lease))
  return parseRefreshLease(localStorage.getItem(AUTH_REFRESH_LOCK_KEY))?.owner === tabID
}

function releaseRefreshLease() {
  const current = parseRefreshLease(localStorage.getItem(AUTH_REFRESH_LOCK_KEY))
  if (current?.owner === tabID) localStorage.removeItem(AUTH_REFRESH_LOCK_KEY)
}

function waitForTokenPairUpdate(previous: TokenPair, timeoutMs: number): Promise<TokenPair | null> {
  const current = readTokenPair()
  if (tokenPairChanged(current, previous)) return Promise.resolve(current)

  return new Promise((resolve) => {
    let settled = false
    let timer: number | undefined
    let poller: number | undefined
    const channel = getAuthChannel()

    const cleanup = () => {
      window.removeEventListener('storage', onStorage)
      channel?.removeEventListener('message', onMessage)
      if (timer !== undefined) window.clearTimeout(timer)
      if (poller !== undefined) window.clearInterval(poller)
    }

    const finish = () => {
      if (settled) return
      settled = true
      cleanup()
      const updated = readTokenPair()
      resolve(tokenPairChanged(updated, previous) ? updated : null)
    }

    const onStorage = (event: StorageEvent) => {
      if (event.key === AUTH_TOKEN_CHANGE_KEY) finish()
    }
    const onMessage = () => finish()

    window.addEventListener('storage', onStorage)
    channel?.addEventListener('message', onMessage)
    poller = window.setInterval(() => {
      if (tokenPairChanged(readTokenPair(), previous)) finish()
    }, 100)
    timer = window.setTimeout(finish, timeoutMs)
  })
}

async function requestFreshTokenPair(used: TokenPair): Promise<TokenPair> {
  try {
    const res = await axios.post('/api/v1/refresh', { refresh_token: used.refresh })
    const payload = res.data?.data ?? res.data
    const access = payload?.access_token
    const refresh = payload?.refresh_token
    if (typeof access !== 'string' || typeof refresh !== 'string' || !access || !refresh) {
      throw new Error('refresh response did not include a token pair')
    }

    // Do not overwrite a newer login/refresh that completed while this request
    // was in flight. The newer pair is already the source of truth.
    const current = readTokenPair()
    if (!sameTokenPair(current, used)) {
      if (hasUsableTokenPair(current)) return current
      throw new Error('token state changed while refreshing')
    }

    setTokens(access, refresh)
    return { access, refresh }
  } catch (error) {
    const current = readTokenPair()
    if (tokenPairChanged(current, used)) return current

    // A fallback storage lease can still lose a set/read race. Give the
    // winning page a short window to publish its rotated pair before treating
    // the 401 as a real authentication failure.
    const status = (error as { response?: { status?: number } })?.response?.status
    if (status === 401) {
      const updated = await waitForTokenPairUpdate(used, AUTH_REFRESH_RECOVERY_WAIT_MS)
      if (updated) return updated
    }
    throw error
  }
}

function sameTokenPair(left: TokenPair, right: TokenPair) {
  return left.access === right.access && left.refresh === right.refresh
}

async function refreshWithStorageLease(previous: TokenPair): Promise<TokenPair> {
  const deadline = Date.now() + AUTH_REFRESH_WAIT_MS
  while (Date.now() < deadline) {
    const current = readTokenPair()
    if (tokenPairChanged(current, previous)) return current
    if (!hasUsableTokenPair(current)) throw new Error('refresh token is missing')

    if (tryAcquireRefreshLease()) {
      try {
        const latest = readTokenPair()
        if (tokenPairChanged(latest, previous)) return latest
        return await requestFreshTokenPair(latest)
      } finally {
        releaseRefreshLease()
      }
    }

    const updated = await waitForTokenPairUpdate(previous, Math.min(500, deadline - Date.now()))
    if (updated) return updated
  }
  throw new Error('timed out waiting for another page to refresh the session')
}

async function refreshAcrossContexts(previous: TokenPair): Promise<TokenPair> {
  const locks = getWebLockManager()
  if (locks) {
    return locks.request(AUTH_REFRESH_LOCK_NAME, async () => {
      const current = readTokenPair()
      if (tokenPairChanged(current, previous)) return current
      if (!hasUsableTokenPair(current)) throw new Error('refresh token is missing')
      return requestFreshTokenPair(current)
    })
  }
  return refreshWithStorageLease(previous)
}

function refreshTokenPair(previous: TokenPair): Promise<TokenPair> {
  const current = readTokenPair()
  if (tokenPairChanged(current, previous)) return Promise.resolve(current)
  if (refreshPromise) return refreshPromise

  const next = refreshAcrossContexts(previous)
  refreshPromise = next
  void next.then(
    () => {
      if (refreshPromise === next) refreshPromise = null
    },
    () => {
      if (refreshPromise === next) refreshPromise = null
    },
  )
  return next
}

function redirectToLogin() {
  if (loginRedirectStarted) return
  loginRedirectStarted = true
  clearTokens()
  window.location.href = '/login'
}

const instance: AxiosInstance = axios.create({
  baseURL: '',
  timeout: 15000,
  headers: { 'Content-Type': 'application/json' },
})

instance.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  NProgress.start()
  const token = getToken()
  const authConfig = config as InternalAxiosRequestConfig & AuthRequestConfig
  authConfig._authAccessToken = token || ''
  authConfig._authRefreshToken = getRefreshToken() || ''
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  const actTenant = getActTenantId()
  if (actTenant) {
    config.headers['X-Act-Tenant-ID'] = actTenant
  }
  return config
})

instance.interceptors.response.use(
  (response: AxiosResponse) => {
    NProgress.done()
    const body = response.data
    // Unwrap the standard envelope { code, message, data }. Paginated payloads
    // carry { list, total, page, page_size } inside data already.
    if (body && typeof body === 'object' && 'code' in body) {
      if (body.code !== 200) {
        if (!response.config?.silent) {
          message.error(body.message || '请求失败')
        }
        return Promise.reject(new Error(body.message || '请求失败'))
      }
      return body.data
    }
    return body
  },
  async (error) => {
    NProgress.done()
    const originalRequest = error.config as AuthRequestConfig

    if (error.response?.status === 401 && !originalRequest._retry) {
      const previous: TokenPair = {
        access: originalRequest._authAccessToken || '',
        refresh: originalRequest._authRefreshToken || getRefreshToken() || '',
      }
      if (!previous.refresh) {
        redirectToLogin()
        return Promise.reject(error)
      }

      originalRequest._retry = true

      try {
        const tokens = await refreshTokenPair(previous)
        originalRequest.headers = {
          ...originalRequest.headers,
          Authorization: `Bearer ${tokens.access}`,
        }
        return instance(originalRequest)
      } catch {
        const current = readTokenPair()
        if (tokenPairChanged(current, previous)) {
          originalRequest.headers = {
            ...originalRequest.headers,
            Authorization: `Bearer ${current.access}`,
          }
          return instance(originalRequest)
        }
        redirectToLogin()
        return Promise.reject(error)
      }
    }

    const msg = error.response?.data?.message || error.message || '请求失败'
    if (error.response?.status !== 401 && !originalRequest?.silent) {
      message.error(msg)
    }
    return Promise.reject(error)
  },
)

export default instance
