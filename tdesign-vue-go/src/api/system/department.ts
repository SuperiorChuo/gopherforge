import { typedApi } from '@/api/generated/client';
import type { components } from '@/api/generated/schema';

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];

export type DepartmentItem = Schema<'DepartmentItem'>;
export type DepartmentListResponse = Schema<'DepartmentListResponse'>;
export type CreateDepartmentRequest = Schema<'CreateDepartmentRequest'>;
export type UpdateDepartmentRequest = Schema<'UpdateDepartmentRequest'>;

export interface DepartmentListRequest {
  page?: number;
  page_size?: number;
  keyword?: string;
  status?: number;
}

export function getDepartmentList(params?: DepartmentListRequest) {
  return typedApi.get('/api/v1/departments', {
    query: params,
  });
}

export function getDepartmentTree(status?: number) {
  return typedApi.get('/api/v1/departments/tree', {
    query: status !== undefined ? { status } : undefined,
  });
}

export function getAllDepartments(status?: number) {
  return typedApi.get('/api/v1/departments/all', {
    query: status !== undefined ? { status } : undefined,
  });
}

export function getDepartment(id: number) {
  return typedApi.get('/api/v1/departments/{id}', {
    path: { id },
  });
}

export function createDepartment(data: CreateDepartmentRequest) {
  return typedApi.post('/api/v1/departments', {
    body: data,
  });
}

export function updateDepartment(id: number, data: UpdateDepartmentRequest) {
  return typedApi.put('/api/v1/departments/{id}', {
    path: { id },
    body: data,
  });
}

export function deleteDepartment(id: number) {
  return typedApi.delete('/api/v1/departments/{id}', {
    path: { id },
  });
}
