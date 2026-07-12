import request from '@/utils/request'
import type { PageRequest, PageResponse, SystemUser } from '@/types'

type UserListParams = PageRequest & { keyword?: string; status?: number }
type UserCreateData = Omit<SystemUser, 'id' | 'created_at'>
type UserUpdateData = Partial<UserCreateData>

export const getUserList = (params: UserListParams) =>
  request.get<unknown, PageResponse<SystemUser>>('/api/v1/system/users', { params })

export const createUser = (data: UserCreateData) =>
  request.post<unknown, SystemUser>('/api/v1/system/users', data)

export const updateUser = (id: number, data: UserUpdateData) =>
  request.put<unknown, SystemUser>(`/api/v1/system/users/${id}`, data)

export const deleteUser = (id: number) =>
  request.delete<unknown, void>(`/api/v1/system/users/${id}`)

export const updateUserStatus = (id: number, status: number) =>
  request.put<unknown, void>(`/api/v1/system/users/${id}/status`, { status })

export const assignUserRoles = (id: number, role_ids: number[]) =>
  request.put<unknown, void>(`/api/v1/system/users/${id}/roles`, { role_ids })

export const resetUserPassword = (id: number, password: string) =>
  request.put<unknown, void>(`/api/v1/system/users/${id}/password`, { password })
