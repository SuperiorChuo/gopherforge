import request from '@/utils/request'
import type { TenantInfo } from '@/types'

export function getTenantList(params?: { page?: number; page_size?: number; keyword?: string; status?: number }) {
  return request.get('/api/v1/tenants', { params }) as Promise<{
    list: TenantInfo[]
    total: number
    page: number
    page_size: number
  }>
}

export function getTenant(id: number) {
  return request.get(`/api/v1/tenants/${id}`) as Promise<{ tenant: TenantInfo; user_count: number }>
}

export function createTenant(data: {
  code: string
  name: string
  plan?: string
  max_users?: number
  status?: number
}) {
  return request.post('/api/v1/tenants', data) as Promise<TenantInfo>
}

export function updateTenant(
  id: number,
  data: { name?: string; plan?: string; max_users?: number; status?: number },
) {
  return request.put(`/api/v1/tenants/${id}`, data) as Promise<TenantInfo>
}
