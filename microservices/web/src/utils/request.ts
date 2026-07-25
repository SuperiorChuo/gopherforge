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
const AUTH_TOKEN_PAIR_KEY = 'auth_token_pair'
const AUTH_TOKEN_PAIR_PENDING_KEY = 'auth_token_pair_pending'
const AUTH_TOKEN_PAIR_PENDING_TTL_MS = 5_000
const AUTH_TOKEN_CHANGE_KEY = 'auth_tokens_changed'
const AUTH_TOKEN_CHANNEL = 'go-admin-kit-auth'
const AUTH_REFRESH_LOCK_KEY = 'go-admin-kit-auth-refresh-lock'
const AUTH_REFRESH_LOCK_NAME = 'go-admin-kit-auth-refresh'
const AUTH_REFRESH_DB_NAME = 'go-admin-kit-auth'
const AUTH_REFRESH_DB_VERSION = 1
const AUTH_REFRESH_DB_STORE = 'leases'
const AUTH_REFRESH_WAIT_MS = 20_000
const AUTH_REFRESH_RECOVERY_WAIT_MS = 1_500
const AUTH_REFRESH_LOCK_TTL_MS = 20_000
const AUTH_REFRESH_REQUEST_TIMEOUT_MS = 15_000
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
  request<T>(
    name: string,
    options: { ifAvailable?: boolean; signal?: AbortSignal },
    callback: (lock: object | null) => Promise<T>,
  ): Promise<T>
}

type RefreshLease = {
  owner: string
  expiresAt: number
}

type RefreshLeaseHandle = {
  backend: 'indexeddb' | 'storage'
}

type IndexedDBLeaseResult = 'acquired' | 'busy' | 'unsupported' | 'unavailable' | 'deadline-exceeded'

type RefreshLockDatabaseResult =
  | { status: 'available'; database: IDBDatabase }
  | { status: 'unsupported' | 'unavailable' | 'deadline-exceeded' }

const tabID = `${Date.now()}-${Math.random().toString(36).slice(2)}`
let authChannel: BroadcastChannel | null | undefined
let refreshLockDatabasePromise: Promise<RefreshLockDatabaseResult> | null = null

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

function readLegacyTokenPair(): TokenPair | null {
  const access = localStorage.getItem(TOKEN_KEY)
  const refresh = localStorage.getItem(REFRESH_TOKEN_KEY)
  if (!access || !refresh) return null
  return { access, refresh }
}

function hasActiveTokenPairWrite() {
  const raw = localStorage.getItem(AUTH_TOKEN_PAIR_PENDING_KEY)
  if (!raw) return false
  const startedAt = Number(raw)
  if (!Number.isFinite(startedAt) || Date.now() - startedAt > AUTH_TOKEN_PAIR_PENDING_TTL_MS) {
    localStorage.removeItem(AUTH_TOKEN_PAIR_PENDING_KEY)
    return false
  }
  return true
}

function readStoredTokenPair(): TokenPair | null {
  const raw = localStorage.getItem(AUTH_TOKEN_PAIR_KEY)
  let pair: TokenPair | null = null
  if (raw) {
    try {
      const value = JSON.parse(raw) as Partial<TokenPair>
      if (typeof value.access === 'string' && typeof value.refresh === 'string') {
        pair = { access: value.access, refresh: value.refresh }
      }
    } catch {
      pair = null
    }
  }

  const legacy = readLegacyTokenPair()
  if (!pair) return legacy
  if (hasActiveTokenPairWrite()) return pair
  if (legacy && (legacy.access !== pair.access || legacy.refresh !== pair.refresh)) {
    // An older page only knows the two compatibility keys. Migrate its stable
    // pair into the atomic snapshot before the next request reads it.
    localStorage.setItem(AUTH_TOKEN_PAIR_KEY, JSON.stringify(legacy))
    return legacy
  }
  return pair
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

export const getToken = () => {
  const pair = readStoredTokenPair()
  return pair?.access || null
}
export const getRefreshToken = () => {
  const pair = readStoredTokenPair()
  return pair?.refresh || null
}
export const setTokens = (access: string, refresh: string) => {
  localStorage.setItem(AUTH_TOKEN_PAIR_PENDING_KEY, String(Date.now()))
  try {
    // Keep legacy readers working while the single snapshot protects new
    // readers from observing the two compatibility writes independently.
    localStorage.setItem(TOKEN_KEY, access)
    localStorage.setItem(REFRESH_TOKEN_KEY, refresh)
    localStorage.setItem(AUTH_TOKEN_PAIR_KEY, JSON.stringify({ access, refresh }))
  } finally {
    localStorage.removeItem(AUTH_TOKEN_PAIR_PENDING_KEY)
  }
  announceTokenChange()
}
export const clearTokens = () => {
  localStorage.setItem(AUTH_TOKEN_PAIR_PENDING_KEY, String(Date.now()))
  try {
    // Commit the empty snapshot first so new readers stop using the session
    // before the old compatibility keys are removed.
    localStorage.setItem(AUTH_TOKEN_PAIR_KEY, JSON.stringify({ access: '', refresh: '' }))
    localStorage.removeItem(TOKEN_KEY)
    localStorage.removeItem(REFRESH_TOKEN_KEY)
  } finally {
    localStorage.removeItem(AUTH_TOKEN_PAIR_PENDING_KEY)
  }
  announceTokenChange()
}

NProgress.configure({ showSpinner: false })

let refreshPromise: Promise<TokenPair> | null = null
let loginRedirectStarted = false

const readTokenPair = (): TokenPair => readStoredTokenPair() || { access: '', refresh: '' }

const hasUsableTokenPair = (pair: TokenPair) => Boolean(pair.access && pair.refresh)

const tokenPairChanged = (current: TokenPair, previous: TokenPair) =>
  hasUsableTokenPair(current) &&
  (current.access !== previous.access || current.refresh !== previous.refresh)

function getWebLockManager(): AuthLockManager | null {
  if (typeof navigator === 'undefined') return null
  const locks = (navigator as Navigator & { locks?: AuthLockManager }).locks
  return locks ?? null
}

function normalizeRefreshLease(value: unknown): RefreshLease | null {
  if (typeof value === 'string') {
    if (!value) return null
    try {
      value = JSON.parse(value)
    } catch {
      return null
    }
  }
  if (!value || typeof value !== 'object') return null
  const lease = value as Partial<RefreshLease>
  if (typeof lease.owner !== 'string' || typeof lease.expiresAt !== 'number') return null
  return { owner: lease.owner, expiresAt: lease.expiresAt }
}

function parseRefreshLease(value: string | null): RefreshLease | null {
  return normalizeRefreshLease(value)
}

function invalidateRefreshLockDatabase(database?: IDBDatabase) {
  refreshLockDatabasePromise = null
  try {
    database?.close()
  } catch {
    // The connection may already be closed.
  }
}

function openRefreshLockDatabase(deadline: number): Promise<RefreshLockDatabaseResult> {
  if (refreshLockDatabasePromise) return refreshLockDatabasePromise
  if (typeof indexedDB === 'undefined') return Promise.resolve({ status: 'unsupported' })

  let promise: Promise<RefreshLockDatabaseResult> | undefined
  let clearAfterCreation = false
  const createdPromise = new Promise<RefreshLockDatabaseResult>((resolve) => {
    let settled = false
    let timer: number | undefined
    const settleUnavailable = (status: 'unavailable' | 'deadline-exceeded') => {
      if (settled) return
      settled = true
      if (timer !== undefined) window.clearTimeout(timer)
      if (promise && refreshLockDatabasePromise === promise) refreshLockDatabasePromise = null
      else clearAfterCreation = true
      resolve({ status })
    }
    if (deadline <= Date.now()) {
      settleUnavailable('deadline-exceeded')
      return
    }

    let request: IDBOpenDBRequest
    try {
      request = indexedDB.open(AUTH_REFRESH_DB_NAME, AUTH_REFRESH_DB_VERSION)
    } catch {
      settleUnavailable('unavailable')
      return
    }
    const remaining = deadline - Date.now()
    if (remaining <= 0) {
      settleUnavailable('deadline-exceeded')
      return
    }

    request.onupgradeneeded = () => {
      const database = request.result
      if (!database.objectStoreNames.contains(AUTH_REFRESH_DB_STORE)) {
        database.createObjectStore(AUTH_REFRESH_DB_STORE)
      }
    }
    request.onsuccess = () => {
      const database = request.result
      if (settled) {
        database.close()
        return
      }
      settled = true
      if (timer !== undefined) window.clearTimeout(timer)
      database.onversionchange = () => invalidateRefreshLockDatabase(database)
      resolve({ status: 'available', database })
    }
    request.onerror = () => settleUnavailable('unavailable')
    request.onblocked = () => settleUnavailable('unavailable')
    timer = window.setTimeout(() => settleUnavailable('deadline-exceeded'), remaining)
  })

  promise = createdPromise
  refreshLockDatabasePromise = createdPromise
  if (clearAfterCreation) refreshLockDatabasePromise = null
  return createdPromise
}

async function tryAcquireIndexedDBLease(deadline: number): Promise<IndexedDBLeaseResult> {
  const databaseResult = await openRefreshLockDatabase(deadline)
  if (databaseResult.status !== 'available') return databaseResult.status
  const { database } = databaseResult

  return new Promise((resolve) => {
    let acquired = false
    let settled = false
    let timer: number | undefined
    const finish = (result: IndexedDBLeaseResult) => {
      if (settled) return
      settled = true
      if (timer !== undefined) window.clearTimeout(timer)
      if (result === 'unavailable' || result === 'deadline-exceeded') {
        invalidateRefreshLockDatabase(database)
      }
      resolve(result)
    }

    if (deadline <= Date.now()) {
      finish('deadline-exceeded')
      return
    }

    try {
      const transaction = database.transaction(AUTH_REFRESH_DB_STORE, 'readwrite')
      const store = transaction.objectStore(AUTH_REFRESH_DB_STORE)
      const request = store.get(AUTH_REFRESH_LOCK_KEY)
      request.onsuccess = () => {
        const current = normalizeRefreshLease(request.result)
        if (current && current.owner !== tabID && current.expiresAt > Date.now()) return

        store.put(
          {
            owner: tabID,
            expiresAt: Date.now() + AUTH_REFRESH_LOCK_TTL_MS,
          } satisfies RefreshLease,
          AUTH_REFRESH_LOCK_KEY,
        )
        acquired = true
      }
      request.onerror = () => finish('unavailable')
      transaction.oncomplete = () => finish(acquired ? 'acquired' : 'busy')
      transaction.onerror = () => finish('unavailable')
      transaction.onabort = () => finish('unavailable')
      const remaining = deadline - Date.now()
      if (remaining <= 0) {
        finish('deadline-exceeded')
        return
      }
      timer = window.setTimeout(() => finish('deadline-exceeded'), remaining)
    } catch {
      finish('unavailable')
    }
  })
}

async function releaseIndexedDBLease(deadline: number) {
  const databaseResult = await openRefreshLockDatabase(deadline)
  if (databaseResult.status !== 'available') return
  const { database } = databaseResult

  await new Promise<void>((resolve) => {
    let settled = false
    let timer: number | undefined
    const finish = (invalidate = false) => {
      if (settled) return
      settled = true
      if (timer !== undefined) window.clearTimeout(timer)
      if (invalidate) invalidateRefreshLockDatabase(database)
      resolve()
    }
    if (deadline <= Date.now()) {
      finish(true)
      return
    }

    try {
      const transaction = database.transaction(AUTH_REFRESH_DB_STORE, 'readwrite')
      const store = transaction.objectStore(AUTH_REFRESH_DB_STORE)
      const request = store.get(AUTH_REFRESH_LOCK_KEY)
      request.onsuccess = () => {
        const current = normalizeRefreshLease(request.result)
        if (current?.owner === tabID) store.delete(AUTH_REFRESH_LOCK_KEY)
      }
      transaction.oncomplete = () => finish()
      transaction.onerror = () => finish(true)
      transaction.onabort = () => finish(true)
      const remaining = deadline - Date.now()
      if (remaining <= 0) {
        finish(true)
        return
      }
      timer = window.setTimeout(() => finish(true), remaining)
    } catch {
      finish(true)
    }
  })
}

function tryAcquireStorageLease(): boolean {
  const current = parseRefreshLease(localStorage.getItem(AUTH_REFRESH_LOCK_KEY))
  if (current && current.owner !== tabID && current.expiresAt > Date.now()) return false

  const lease: RefreshLease = {
    owner: tabID,
    expiresAt: Date.now() + AUTH_REFRESH_LOCK_TTL_MS,
  }
  localStorage.setItem(AUTH_REFRESH_LOCK_KEY, JSON.stringify(lease))
  return parseRefreshLease(localStorage.getItem(AUTH_REFRESH_LOCK_KEY))?.owner === tabID
}

function releaseStorageLease() {
  const current = parseRefreshLease(localStorage.getItem(AUTH_REFRESH_LOCK_KEY))
  if (current?.owner === tabID) localStorage.removeItem(AUTH_REFRESH_LOCK_KEY)
}

async function acquireRefreshLease(deadline: number): Promise<RefreshLeaseHandle | null> {
  const indexedDBResult = await tryAcquireIndexedDBLease(deadline)
  if (indexedDBResult === 'acquired') return { backend: 'indexeddb' }
  if (indexedDBResult !== 'unsupported') return null
  return tryAcquireStorageLease() ? { backend: 'storage' } : null
}

async function releaseRefreshLease(handle: RefreshLeaseHandle, deadline: number) {
  if (handle.backend === 'indexeddb') {
    await releaseIndexedDBLease(deadline)
    return
  }
  releaseStorageLease()
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

async function requestFreshTokenPair(used: TokenPair, deadline: number): Promise<TokenPair> {
  try {
    const remaining = deadline - Date.now()
    if (remaining <= 0) throw new Error('refresh deadline exceeded')

    const res = await axios.post(
      '/api/v1/refresh',
      { refresh_token: used.refresh },
      { timeout: Math.min(AUTH_REFRESH_REQUEST_TIMEOUT_MS, remaining) },
    )
    const payload = res.data?.data ?? res.data
    const access = payload?.access_token
    const refresh = payload?.refresh_token
    if (typeof access !== 'string' || typeof refresh !== 'string' || !access || !refresh) {
      throw new Error('refresh response did not include a token pair')
    }

    // Do not overwrite a newer login/refresh that completed while this request
    // was in flight. The newer pair is already the source of truth.
    if (Date.now() >= deadline) throw new Error('refresh deadline exceeded')
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
      const remaining = Math.max(0, deadline - Date.now())
      const updated = await waitForTokenPairUpdate(
        used,
        Math.min(AUTH_REFRESH_RECOVERY_WAIT_MS, remaining),
      )
      if (updated) return updated
    }
    throw error
  }
}

function sameTokenPair(left: TokenPair, right: TokenPair) {
  return left.access === right.access && left.refresh === right.refresh
}

async function refreshWithStorageLease(previous: TokenPair, deadline: number): Promise<TokenPair> {
  while (Date.now() < deadline) {
    const current = readTokenPair()
    if (tokenPairChanged(current, previous)) return current
    if (!hasUsableTokenPair(current)) throw new Error('refresh token is missing')

    const lease = await acquireRefreshLease(deadline)
    if (lease) {
      try {
        const latest = readTokenPair()
        if (tokenPairChanged(latest, previous)) return latest
        return await requestFreshTokenPair(latest, deadline)
      } finally {
        await releaseRefreshLease(lease, deadline)
      }
    }

    const remaining = deadline - Date.now()
    if (remaining <= 0) break
    const updated = await waitForTokenPairUpdate(previous, Math.min(500, remaining))
    if (updated) return updated
  }
  throw new Error('timed out waiting for another page to refresh the session')
}

type WebLockCallbackResult<T> =
  | { type: 'completed'; value: T }
  | { type: 'callback-error'; error: unknown }

type WebLockRequestResult<T> =
  | WebLockCallbackResult<T>
  | { type: 'api-error'; error: unknown }
  | { type: 'deadline-exceeded' }

async function requestWebLock<T>(
  locks: AuthLockManager,
  callback: (lock: object | null) => Promise<T>,
  deadline: number,
): Promise<WebLockRequestResult<T>> {
  if (deadline <= Date.now()) return { type: 'deadline-exceeded' }

  const controller = typeof AbortController === 'undefined' ? null : new AbortController()
  let timer: number | undefined
  try {
    const request = locks.request<WebLockCallbackResult<T>>(
      AUTH_REFRESH_LOCK_NAME,
      { ifAvailable: true, ...(controller ? { signal: controller.signal } : {}) },
      async (lock) => {
        try {
          return { type: 'completed', value: await callback(lock) }
        } catch (error) {
          return { type: 'callback-error', error }
        }
      },
    )
    const remaining = deadline - Date.now()
    if (remaining <= 0) {
      controller?.abort()
      return { type: 'deadline-exceeded' }
    }
    return await Promise.race([
      request,
      new Promise<{ type: 'deadline-exceeded' }>((resolve) => {
        timer = window.setTimeout(() => {
          controller?.abort()
          resolve({ type: 'deadline-exceeded' })
        }, remaining)
      }),
    ])
  } catch (error) {
    return { type: 'api-error', error }
  } finally {
    if (timer !== undefined) window.clearTimeout(timer)
  }
}

async function refreshAcrossContexts(previous: TokenPair): Promise<TokenPair> {
  const deadline = Date.now() + AUTH_REFRESH_WAIT_MS
  const locks = getWebLockManager()
  if (locks) {
    while (Date.now() < deadline) {
      const result = await requestWebLock(locks, async (lock) => {
          if (!lock) return null
          const current = readTokenPair()
          if (tokenPairChanged(current, previous)) return current
          if (!hasUsableTokenPair(current)) throw new Error('refresh token is missing')
          return requestFreshTokenPair(current, deadline)
        }, deadline)
      if (result.type === 'api-error') return refreshWithStorageLease(previous, deadline)
      if (result.type === 'deadline-exceeded') throw new Error('timed out waiting for another page to refresh the session')
      if (result.type === 'callback-error') throw result.error
      if (result.value) return result.value

      const remaining = deadline - Date.now()
      if (remaining <= 0) break
      const updated = await waitForTokenPairUpdate(previous, Math.min(500, remaining))
      if (updated) return updated
    }
    throw new Error('timed out waiting for another page to refresh the session')
  }
  return refreshWithStorageLease(previous, deadline)
}

function refreshTokenPair(previous: TokenPair): Promise<TokenPair> {
  if (refreshPromise) return refreshPromise
  const current = readTokenPair()
  if (tokenPairChanged(current, previous)) return Promise.resolve(current)

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
  const storedPair = readTokenPair()
  const token = storedPair.access || null
  const authConfig = config as InternalAxiosRequestConfig & AuthRequestConfig
  authConfig._authAccessToken = storedPair.access
  authConfig._authRefreshToken = storedPair.refresh
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
