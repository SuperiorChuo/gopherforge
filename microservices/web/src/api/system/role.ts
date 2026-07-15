import request from '@/utils/request'
import type { PageRequest, PageResponse, SystemRole, Permission } from '@/types'

type RoleListParams = PageRequest & { keyword?: string }
type RoleCreateData = Omit<SystemRole, 'id' | 'created_at'>
type RoleUpdateData = Partial<RoleCreateData>
type RoleDetail = SystemRole & { permissions?: Permission[] }

export const getRoleList = (params: RoleListParams) =>
  request.get<unknown, PageResponse<SystemRole>>('/api/v1/roles', { params })

export const createRole = (data: RoleCreateData) =>
  request.post<unknown, SystemRole>('/api/v1/roles', data)

export const updateRole = (id: number, data: RoleUpdateData) =>
  request.put<unknown, SystemRole>(`/api/v1/roles/${id}`, data)

export const deleteRole = (id: number) =>
  request.delete<unknown, void>(`/api/v1/roles/${id}`)

// 后端没有独立的角色权限查询接口，角色详情中预加载了 permissions
export const getRolePermissions = (id: number) =>
  request
    .get<unknown, RoleDetail>(`/api/v1/roles/${id}`)
    .then((role) => (role.permissions ?? []).map((p) => p.id))

export const assignRolePermissions = (id: number, permission_ids: number[]) =>
  request.post<unknown, void>(`/api/v1/roles/${id}/permissions`, { permission_ids })
