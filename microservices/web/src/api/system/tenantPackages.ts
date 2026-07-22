import request from '@/utils/request'
import type { TenantPackageInfo } from '@/types'

export function getTenantPackageList(params?: {
  page?: number
  page_size?: number
  keyword?: string
  status?: number
}) {
  return request.get('/api/v1/tenant-packages', { params }) as Promise<{
    list: TenantPackageInfo[]
    total: number
    page: number
    page_size: number
  }>
}

/** 全量套餐（租户管理页下拉选择用） */
export function getAllTenantPackages() {
  return request.get('/api/v1/tenant-packages/all') as Promise<TenantPackageInfo[]>
}

export function getTenantPackage(id: number) {
  return request.get(`/api/v1/tenant-packages/${id}`) as Promise<TenantPackageInfo>
}

export function createTenantPackage(data: {
  name: string
  permission_codes: string[]
  status?: number
  remark?: string
}) {
  return request.post('/api/v1/tenant-packages', data) as Promise<TenantPackageInfo>
}

export function updateTenantPackage(
  id: number,
  data: { name?: string; permission_codes?: string[]; status?: number; remark?: string },
) {
  return request.put(`/api/v1/tenant-packages/${id}`, data) as Promise<TenantPackageInfo>
}

export function deleteTenantPackage(id: number) {
  return request.delete(`/api/v1/tenant-packages/${id}`) as Promise<void>
}
