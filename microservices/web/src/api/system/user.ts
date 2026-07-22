import request from '@/utils/request'
import type { PageRequest, PageResponse, SystemUser } from '@/types'

type UserListParams = PageRequest & { keyword?: string; status?: number }
type UserCreateData = Omit<SystemUser, 'id' | 'created_at'> & { password?: string; post_ids?: number[] }
type UserUpdateData = Partial<UserCreateData>

export const getUserList = (params: UserListParams) =>
  request.get<unknown, PageResponse<SystemUser>>('/api/v1/users', { params })

export const createUser = (data: UserCreateData) =>
  request.post<unknown, SystemUser>('/api/v1/users', data)

// 后端支持更新 nickname/email/phone/avatar/post_ids；状态用 updateUserStatus，角色用 assignUserRoles
export const updateUser = (id: number, data: UserUpdateData) =>
  request.put<unknown, SystemUser>(`/api/v1/users/${id}`, data)

export const deleteUser = (id: number) =>
  request.delete<unknown, void>(`/api/v1/users/${id}`)

export const updateUserStatus = (id: number, status: number) =>
  request.put<unknown, void>(`/api/v1/users/${id}/status`, { status })

export const assignUserRoles = (id: number, role_ids: number[]) =>
  request.post<unknown, void>(`/api/v1/users/${id}/roles`, { role_ids })

// ---- Excel 导出 / 导入（路线图第 11 项）----

const saveBlob = (blob: Blob, filename: string) => {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

/** 按当前筛选条件导出用户 xlsx（同列表权限与数据范围） */
export const exportUsers = async (params: Omit<UserListParams, 'page' | 'page_size'>) => {
  const blob = await request.get<unknown, Blob>('/api/v1/users/export', {
    params,
    responseType: 'blob',
  })
  saveBlob(blob, `users_${new Date().toISOString().slice(0, 19).replace(/[-:T]/g, '')}.xlsx`)
}

/** 下载批量导入模板 */
export const downloadUserImportTemplate = async () => {
  const blob = await request.get<unknown, Blob>('/api/v1/users/import-template', {
    responseType: 'blob',
  })
  saveBlob(blob, 'user_import_template.xlsx')
}

export interface UserImportRowError {
  row: number
  username: string
  reason: string
}

export interface UserImportResult {
  total: number
  success: number
  failed: number
  errors?: UserImportRowError[]
}

/** 批量导入用户（部分成功语义：单行失败不中断，逐行错误明细返回） */
export const importUsers = (file: File) => {
  const form = new FormData()
  form.append('file', file)
  return request.post<unknown, UserImportResult>('/api/v1/users/import', form)
}
