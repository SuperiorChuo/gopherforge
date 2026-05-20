import { typedApi } from '@/api/generated/client';
import type { components } from '@/api/generated/schema';

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];

export type PermissionItem = Schema<'PermissionItem'>;
export type PermissionListResponse = Schema<'PermissionListResponse'>;
export type CreatePermissionRequest = Schema<'CreatePermissionRequest'>;
export type UpdatePermissionRequest = Schema<'UpdatePermissionRequest'>;

export interface PermissionListRequest {
  page?: number;
  page_size?: number;
  keyword?: string;
  type?: number;
}

export function getPermissionList(params?: PermissionListRequest) {
  return typedApi.get('/api/v1/permissions', {
    query: params,
  });
}

export function getPermissionTree() {
  return typedApi.get('/api/v1/permissions/tree');
}

export function getPermission(id: number) {
  return typedApi.get('/api/v1/permissions/{id}', {
    path: { id },
  });
}

export function createPermission(data: CreatePermissionRequest) {
  return typedApi.post('/api/v1/permissions', {
    body: data,
  });
}

export function updatePermission(id: number, data: UpdatePermissionRequest) {
  return typedApi.put('/api/v1/permissions/{id}', {
    path: { id },
    body: data,
  });
}

export function deletePermission(id: number) {
  return typedApi.delete('/api/v1/permissions/{id}', {
    path: { id },
  });
}
