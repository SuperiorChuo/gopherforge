import request from '@/utils/request'
import type { PageRequest, PageResponse, SystemUser } from '@/types'

type UserListParams = PageRequest & { keyword?: string; status?: number }
type UserCreateData = Omit<SystemUser, 'id' | 'created_at'> & { password?: string }
type UserUpdateData = Partial<UserCreateData>

export const getUserList = (params: UserListParams) =>
  request.get<unknown, PageResponse<SystemUser>>('/api/v1/users', { params })

export const createUser = (data: UserCreateData) =>
  request.post<unknown, SystemUser>('/api/v1/users', data)

// 后端仅支持更新 nickname/email/phone/avatar；状态用 updateUserStatus，角色用 assignUserRoles
export const updateUser = (id: number, data: UserUpdateData) =>
  request.put<unknown, SystemUser>(`/api/v1/users/${id}`, data)

export const deleteUser = (id: number) =>
  request.delete<unknown, void>(`/api/v1/users/${id}`)

export const updateUserStatus = (id: number, status: number) =>
  request.put<unknown, void>(`/api/v1/users/${id}/status`, { status })

export const assignUserRoles = (id: number, role_ids: number[]) =>
  request.post<unknown, void>(`/api/v1/users/${id}/roles`, { role_ids })
