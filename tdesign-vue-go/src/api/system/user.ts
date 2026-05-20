import { typedApi } from '@/api/generated/client';
import type { components } from '@/api/generated/schema';

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];

export interface UserListRequest {
  page?: number;
  page_size?: number;
  keyword?: string;
  status?: number;
}

export type UserListResponse = Schema<'UserListResponse'>;
export type UserItem = Schema<'UserInfo'>;
export type RoleItem = Schema<'RoleInfo'>;
export type UpdateUserRequest = Schema<'UpdateUserRequest'>;
export type CreateUserRequest = Schema<'CreateUserRequest'>;
export type AssignRolesRequest = Schema<'AssignRolesRequest'>;

export function getUserList(params: UserListRequest) {
  return typedApi.get('/api/v1/users', {
    query: params,
  });
}

export function getUser(id: number) {
  return typedApi.get('/api/v1/users/{id}', {
    path: { id },
  });
}

export function updateUser(id: number, data: UpdateUserRequest) {
  return typedApi.put('/api/v1/users/{id}', {
    path: { id },
    body: data,
  });
}

export function createUser(data: CreateUserRequest) {
  return typedApi.post('/api/v1/users', {
    body: data,
  });
}

export function deleteUser(id: number) {
  return typedApi.delete('/api/v1/users/{id}', {
    path: { id },
  });
}

export function updateUserStatus(id: number, status: number) {
  return typedApi.put('/api/v1/users/{id}/status', {
    path: { id },
    body: { status },
  });
}

export function assignRoles(id: number, data: AssignRolesRequest) {
  return typedApi.post('/api/v1/users/{id}/roles', {
    path: { id },
    body: data,
  });
}
