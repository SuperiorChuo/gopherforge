import { typedApi } from '@/api/generated/client';
import type { components } from '@/api/generated/schema';
import { request } from '@/utils/request';

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];

export type LoginRequest = Schema<'LoginRequest'>;
export type LoginResponse = Schema<'LoginResponse'>;
export type VerifyTOTPLoginRequest = Schema<'VerifyTOTPLoginRequest'>;
export type RegisterRequest = Schema<'RegisterRequest'>;
export type UserInfo = Schema<'UserInfo'>;
export type RoleInfo = Schema<'RoleInfo'>;
export type ChangePasswordRequest = Schema<'ChangePasswordRequest'>;
export type UpdateProfileRequest = Schema<'UpdateProfileRequest'>;
export type RefreshTokenRequest = Schema<'RefreshTokenRequest'>;
export type TOTPSetupResponse = Schema<'TOTPSetupResponse'>;
export type TOTPSetupRequest = Schema<'TOTPSetupRequest'>;
export type TOTPVerifyRequest = Schema<'TOTPVerifyRequest'>;
export type TOTPRecoveryCodesResponse = Schema<'TOTPRecoveryCodesResponse'>;
export type NotificationTicketResponse = Schema<'NotificationTicketResponse'>;
export type BackendMenu = Schema<'MenuItem'>;

export function login(data: LoginRequest) {
  return typedApi.post('/api/v1/login', {
    body: data,
    withToken: false,
  });
}

export function verifyTotpLogin(data: VerifyTOTPLoginRequest) {
  return typedApi.post('/api/v1/login/2fa/verify', {
    body: data,
    withToken: false,
  });
}

export function createNotificationTicket() {
  return typedApi.post('/api/v1/ws/notifications/ticket');
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

export function generateTotpSetup(data: TOTPSetupRequest) {
  return typedApi.post('/api/v1/user/2fa/setup', {
    body: data,
  });
}

export function enableTotp(data: TOTPVerifyRequest) {
  return typedApi.post('/api/v1/user/2fa/enable', {
    body: data,
  });
}

export function disableTotp(data: TOTPVerifyRequest) {
  return typedApi.post('/api/v1/user/2fa/disable', {
    body: data,
  });
}

export function regenerateTotpRecoveryCodes(data: TOTPVerifyRequest) {
  return typedApi.post('/api/v1/user/2fa/recovery-codes', {
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
