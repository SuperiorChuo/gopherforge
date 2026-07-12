import request from '@/utils/request'
import type { PageRequest, PageResponse, Permission } from '@/types'

type PermissionListParams = PageRequest & { keyword?: string; status?: number }
type PermissionCreateData = Omit<Permission, 'id' | 'created_at'>
type PermissionUpdateData = Partial<PermissionCreateData>

export const getPermissionList = (params: PermissionListParams) =>
  request.get<unknown, PageResponse<Permission>>('/api/v1/system/permissions', { params })

export const createPermission = (data: PermissionCreateData) =>
  request.post<unknown, Permission>('/api/v1/system/permissions', data)

export const updatePermission = (id: number, data: PermissionUpdateData) =>
  request.put<unknown, Permission>(`/api/v1/system/permissions/${id}`, data)

export const deletePermission = (id: number) =>
  request.delete<unknown, void>(`/api/v1/system/permissions/${id}`)
