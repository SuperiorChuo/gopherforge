import request from '@/utils/request'
import type { PageRequest, PageResponse, LoginLog, OperationLog } from '@/types'

type LoginLogListParams = PageRequest & { username?: string; ip?: string; status?: number; start_time?: string; end_time?: string }
type OperationLogListParams = PageRequest & { username?: string; method?: string; path?: string; module?: string; status?: number; start_time?: string; end_time?: string }

export const getLoginLogList = (params: LoginLogListParams) =>
  request.get<unknown, PageResponse<LoginLog>>('/api/v1/login-logs', { params })

// 当前登录用户自己的登录记录（个人中心用）
export const getMyLoginLogs = (params: PageRequest) =>
  request.get<unknown, PageResponse<LoginLog>>('/api/v1/login-logs/my', { params, silent: true })

// 上一次成功登录（无记录时返回 null）
export const getLastLogin = () =>
  request.get<unknown, LoginLog | null>('/api/v1/login-logs/last', { silent: true })

export const getOperationLogList = (params: OperationLogListParams) =>
  request.get<unknown, PageResponse<OperationLog>>('/api/v1/operation-logs', { params })

export const getOperationLogDetail = (id: number) =>
  request.get<unknown, OperationLog>(`/api/v1/operation-logs/${id}`)

// 按当前筛选条件导出 CSV（后端流式返回，走 blob 触发下载）
export const exportOperationLogs = async (params: Omit<OperationLogListParams, 'page' | 'page_size'>) => {
  const blob = await request.get<unknown, Blob>('/api/v1/operation-logs/export', {
    params,
    responseType: 'blob',
  })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `operation_logs_${new Date().toISOString().slice(0, 19).replace(/[-:T]/g, '')}.csv`
  a.click()
  URL.revokeObjectURL(url)
}

export const clearOperationLogs = (days: number) =>
  request.delete<unknown, { deleted_count: number }>('/api/v1/operation-logs/clear', {
    data: { days },
  })

export const clearLoginLogs = (days: number) =>
  request.delete<unknown, { deleted_count: number }>('/api/v1/login-logs/clear', {
    data: { days },
  })

export interface LoginLogStats {
  total: number
  success: number
  failed: number
  today_users: number
  by_device?: Record<string, number>
  by_browser?: Record<string, number>
}

// 默认统计最近 7 天；无权限时静默降级
export const getLoginStats = () =>
  request.get<unknown, LoginLogStats>('/api/v1/login-logs/stats', { silent: true })

export interface OperationLogStats {
  total: number
  error_count: number
  by_module?: Record<string, number>
  by_method?: Record<string, number>
}

export const getOperationLogStats = () =>
  request.get<unknown, OperationLogStats>('/api/v1/operation-logs/stats', { silent: true })

export interface LoginTrendItem {
  date: string
  count: number
  success: number
  failed: number
}

export interface LoginGeoItem {
  location: string
  province: string
  city: string
  total: number
  success: number
  failed: number
}

// 登录地域分布（按 location 聚合，province/city 为后端尽力拆解）；窗口由服务端
// 按 days 计算（与 trend 同口径，避免时区偏移）；无权限时静默降级
export const getLoginGeoDistribution = (days = 7) =>
  request.get<unknown, LoginGeoItem[]>('/api/v1/login-logs/geo', {
    params: { days },
    silent: true,
  })

// 仪表盘可选模块：无权限时静默降级，不弹全局错误
export const getLoginTrend = (days = 7) =>
  request.get<unknown, LoginTrendItem[]>('/api/v1/login-logs/trend', {
    params: { days },
    silent: true,
  })
