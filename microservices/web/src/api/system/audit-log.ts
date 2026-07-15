import request from '@/utils/request'

export interface AuditLog {
  id: number
  actor_type: string
  actor_id: string
  action: string
  target_type: string
  target_id: string
  before?: Record<string, unknown>
  after?: Record<string, unknown>
  summary?: string
  created_at: string
}

export interface AuditLogListParams {
  page?: number
  page_size?: number
  action?: string
  target_type?: string
  target_id?: string
  keyword?: string
  sort_by?: string
  sort_order?: string
}

// 后端返回 items + pagination + facets（facets 用来填筛选下拉）
export interface AuditLogListResult {
  items: AuditLog[]
  pagination: { page: number; page_size: number; total: number }
  facets: { actions: string[]; target_types: string[]; actor_types: string[] }
}

export const getAuditLogList = (params: AuditLogListParams) =>
  request.get<unknown, AuditLogListResult>('/api/v1/logs/audit', { params })
