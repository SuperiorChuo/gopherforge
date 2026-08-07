import request from '@/utils/request'

export interface LoginRiskEvent {
  id: number
  tenant_id: number
  user_id: number
  username: string
  ip: string
  device_id: string
  reason: 'new_ip' | 'new_device'
  alerted: boolean
  notified_at?: string | null
  processed: boolean
  processed_by?: number | null
  processed_at?: string | null
  created_at: string
}

export interface LoginRiskEventListParams {
  page?: number
  page_size?: number
  username?: string
  ip?: string
  reason?: string
  processed?: string
}

export const getLoginRiskEvents = (params: LoginRiskEventListParams) =>
  request.get<unknown, { list: LoginRiskEvent[]; total: number }>('/api/v1/login-risk-events', { params })

export const processLoginRiskEvent = (id: number) =>
  request.post<unknown, { id: number }>(`/api/v1/login-risk-events/${id}/process`)
