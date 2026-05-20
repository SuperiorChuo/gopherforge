import { typedApi } from '@/api/generated/client';
import type { components } from '@/api/generated/schema';
import { request } from '@/utils/request';

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];

export type LoginRequest = Schema<'LoginRequest'>;
export type LoginResponse = Schema<'LoginResponse'>;
export type RegisterRequest = Schema<'RegisterRequest'>;
export type UserInfo = Schema<'UserInfo'>;
export type RoleInfo = Schema<'RoleInfo'>;
export type ChangePasswordRequest = Schema<'ChangePasswordRequest'>;
export type UpdateProfileRequest = Schema<'UpdateProfileRequest'>;
export type RefreshTokenRequest = Schema<'RefreshTokenRequest'>;
export type BackendMenu = Schema<'MenuItem'>;

export function login(data: LoginRequest) {
  return typedApi.post('/api/v1/login', {
    body: data,
    withToken: false,
  });
}

export function logout(data?: RefreshTokenRequest) {
  return request.post({
    url: '/logout',
    data,
  });
}

export function register(data: RegisterRequest) {
  return typedApi.post('/api/v1/register', {
    body: data,
  });
}

export function getCurrentUser() {
  return typedApi.get('/api/v1/user/me');
}

export function changePassword(data: ChangePasswordRequest) {
  return typedApi.put('/api/v1/user/password', {
    body: data,
  });
}

export function updateProfile(data: UpdateProfileRequest) {
  return typedApi.put('/api/v1/user/profile', {
    body: data,
  });
}

export function refreshToken(data: RefreshTokenRequest) {
  return typedApi.post('/api/v1/refresh', {
    body: data,
  });
}

export function getUserMenus() {
  return typedApi.get('/api/v1/user/menus');
}
