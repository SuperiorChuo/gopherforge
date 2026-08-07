import request from '@/utils/request'

export interface SecurityEvent {
  id: number
  tenant_id: number
  rule: string
  severity: 'info' | 'warning' | 'critical'
  summary: string
  actor_id: string
  actor_type: string
  target: string
  occurred_at: string
  notified_at?: string | null
  created_at: string
}

export interface SecurityEventListParams {
  page?: number
  page_size?: number
  rule?: string
  severity?: string
}

export const getSecurityEventList = (params: SecurityEventListParams) =>
  request.get<unknown, { list: SecurityEvent[]; total: number }>('/api/v1/security-events', { params })
