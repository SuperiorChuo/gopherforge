import request from '@/utils/request'

// 用量日志接口类型
export interface AIUsageLog {
  id: number
  model_id: number
  model_name: string
  user_id?: number
  username?: string
  tenant_id?: number
  operation: string
  prompt_tokens?: number
  completion_tokens?: number
  total_tokens?: number
  image_count: number
  cost: number
  latency_ms?: number
  status: string
  error_msg?: string
  request_id?: string
  ip?: string
  user_agent?: string
  created_at: string
}

export interface AIUsageLogListParams {
  page?: number
  page_size?: number
  model_id?: number
  user_id?: number
  status?: string
  start_time?: string
  end_time?: string
}

export interface AIUsageLogListResponse {
  list: AIUsageLog[]
  total: number
}

export interface DailyStats {
  date: string
  total_requests: number
  success_requests: number
  total_tokens: number
  total_cost: number
  avg_latency_ms: number
}

export interface UserStats {
  user_id: number
  username: string
  total_requests: number
  total_tokens: number
  total_cost: number
}

export interface UsageSummary {
  total_requests: number
  success_requests: number
  total_tokens: number
  total_cost: number
}

// 查询调用记录
export const getAIUsageLogs = (params: AIUsageLogListParams) =>
  request.get<unknown, AIUsageLogListResponse>('/api/v1/ai/usage/logs', { params })

// 获取每日统计
export const getDailyStats = (params: { days?: number; user_id?: number; tenant_id?: number }) =>
  request.get<unknown, DailyStats[]>('/api/v1/ai/usage/stats/daily', { params })

// 获取 Top 用户排行
export const getTopUsers = (params: { days?: number; limit?: number }) =>
  request.get<unknown, UserStats[]>('/api/v1/ai/usage/stats/top-users', { params })

// 获取用量汇总
export const getUsageSummary = (params: {
  start_time?: string
  end_time?: string
  user_id?: number
  tenant_id?: number
}) => request.get<unknown, UsageSummary>('/api/v1/ai/usage/stats/summary', { params })
