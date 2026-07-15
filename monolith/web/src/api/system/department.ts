import request from '@/utils/request'
import type { PageRequest, PageResponse, Department } from '@/types'

type DepartmentListParams = PageRequest & { keyword?: string; status?: number }
type DepartmentCreateData = Omit<Department, 'id' | 'created_at' | 'children'>
type DepartmentUpdateData = Partial<DepartmentCreateData>

export const getDepartmentList = (params: DepartmentListParams) =>
  request.get<unknown, PageResponse<Department>>('/api/v1/departments', { params })

export const getDepartmentTree = () =>
  request.get<unknown, Department[]>('/api/v1/departments/tree')

export const createDepartment = (data: DepartmentCreateData) =>
  request.post<unknown, Department>('/api/v1/departments', data)

export const updateDepartment = (id: number, data: DepartmentUpdateData) =>
  request.put<unknown, Department>(`/api/v1/departments/${id}`, data)

export const deleteDepartment = (id: number) =>
  request.delete<unknown, void>(`/api/v1/departments/${id}`)
