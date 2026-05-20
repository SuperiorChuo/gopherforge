import { typedApi } from '@/api/generated/client';
import type { components } from '@/api/generated/schema';

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];

export type RoleDataScope = Schema<'RoleItem'>['data_scope'];

export interface RoleListRequest {
  page?: number;
  page_size?: number;
  keyword?: string;
}

export type RoleListResponse = Schema<'RoleListResponse'>;
export type RoleItem = Schema<'RoleItem'>;
export type CreateRoleRequest = Schema<'CreateRoleRequest'>;
export type UpdateRoleRequest = Schema<'UpdateRoleRequest'>;
export type AssignPermissionsRequest = Schema<'AssignPermissionsRequest'>;

export function getRoleList(params: RoleListRequest) {
  return typedApi.get('/api/v1/roles', {
    query: params,
  });
}

export function getAllRoles() {
  return typedApi.get('/api/v1/roles/all');
}

export function getRole(id: number) {
  return typedApi.get('/api/v1/roles/{id}', {
    path: { id },
  });
}

export function createRole(data: CreateRoleRequest) {
  return typedApi.post('/api/v1/roles', {
    body: data,
  });
}

export function updateRole(id: number, data: UpdateRoleRequest) {
  return typedApi.put('/api/v1/roles/{id}', {
    path: { id },
    body: data,
  });
}

export function deleteRole(id: number) {
  return typedApi.delete('/api/v1/roles/{id}', {
    path: { id },
  });
}

export function assignPermissions(id: number, data: AssignPermissionsRequest) {
  return typedApi.post('/api/v1/roles/{id}/permissions', {
    path: { id },
    body: data,
  });
}
