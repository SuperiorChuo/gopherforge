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
}
export const clearTokens = () => {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(REFRESH_TOKEN_KEY)
}

NProgress.configure({ showSpinner: false })

let isRefreshing = false
let pendingQueue: Array<{
  resolve: (value: unknown) => void
  reject: (reason?: unknown) => void
  config: AxiosRequestConfig
}> = []

const instance: AxiosInstance = axios.create({
  baseURL: '',
  timeout: 15000,
  headers: { 'Content-Type': 'application/json' },
})

instance.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  NProgress.start()
  const token = getToken()
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
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
    const originalRequest = error.config as AxiosRequestConfig & { _retry?: boolean }

    if (error.response?.status === 401 && !originalRequest._retry) {
      const refresh = getRefreshToken()
      if (!refresh) {
        clearTokens()
        window.location.href = '/login'
        return Promise.reject(error)
      }

      if (isRefreshing) {
        return new Promise((resolve, reject) => {
          pendingQueue.push({ resolve, reject, config: originalRequest })
        })
      }

      originalRequest._retry = true
      isRefreshing = true

      try {
        const res = await axios.post('/api/v1/refresh', { refresh_token: refresh })
        const payload = res.data?.data ?? res.data
        const newAccess = payload.access_token
        const newRefresh = payload.refresh_token
        setTokens(newAccess, newRefresh)
        pendingQueue.forEach(({ resolve, config }) => {
          config.headers = { ...config.headers, Authorization: `Bearer ${newAccess}` }
          resolve(instance(config))
        })
        pendingQueue = []
        originalRequest.headers = {
          ...originalRequest.headers,
          Authorization: `Bearer ${newAccess}`,
        }
        return instance(originalRequest)
      } catch (refreshError) {
        // Fail queued requests too so their callers don't hang forever.
        pendingQueue.forEach(({ reject }) => reject(refreshError))
        pendingQueue = []
        clearTokens()
        window.location.href = '/login'
        return Promise.reject(error)
      } finally {
        isRefreshing = false
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
