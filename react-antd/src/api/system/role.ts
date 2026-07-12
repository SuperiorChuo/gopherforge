import request from '@/utils/request'
import type { PageRequest, PageResponse, SystemRole } from '@/types'

type RoleListParams = PageRequest & { keyword?: string; status?: number }
type RoleCreateData = Omit<SystemRole, 'id' | 'created_at'>
type RoleUpdateData = Partial<RoleCreateData>

export const getRoleList = (params: RoleListParams) =>
  request.get<unknown, PageResponse<SystemRole>>('/api/v1/system/roles', { params })

export const createRole = (data: RoleCreateData) =>
  request.post<unknown, SystemRole>('/api/v1/system/roles', data)

export const updateRole = (id: number, data: RoleUpdateData) =>
  request.put<unknown, SystemRole>(`/api/v1/system/roles/${id}`, data)

export const deleteRole = (id: number) =>
  request.delete<unknown, void>(`/api/v1/system/roles/${id}`)

export const getRolePermissions = (id: number) =>
  request.get<unknown, number[]>(`/api/v1/system/roles/${id}/permissions`)

export const assignRolePermissions = (id: number, permission_ids: number[]) =>
  request.put<unknown, void>(`/api/v1/system/roles/${id}/permissions`, { permission_ids })
