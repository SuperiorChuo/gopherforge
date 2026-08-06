import request from '@/utils/request'
import type {
  LoginRequest,
  LoginResponse,
  VerifyTOTPLoginRequest,
  ChangePasswordRequest,
  UpdateProfileRequest,
  UserInfo,
  MenuItem,
} from '@/types'

export const login = (data: LoginRequest) =>
  request.post<unknown, LoginResponse>('/api/v1/login', data)

export const verifyTotpLogin = (data: VerifyTOTPLoginRequest) =>
  request.post<unknown, LoginResponse>('/api/v1/login/2fa/verify', data)

// 邀请注册：必须携带邀请链接里的 invite_token（无公开自注册）
export const register = (data: { username: string; password: string; email: string; invite_token: string }) =>
  request.post<unknown, { user: UserInfo }>('/api/v1/register', data)

export const logout = (data?: { refresh_token: string }) =>
  request.post('/api/v1/logout', data)

export const getCurrentUser = () =>
  request.get<unknown, UserInfo>('/api/v1/user/me')

export const getUserMenus = () =>
  request.get<unknown, MenuItem[]>('/api/v1/user/menus')

export const changePassword = (data: ChangePasswordRequest) =>
  request.put('/api/v1/user/password', data)

// 忘记密码：请求发送重置邮件；服务端对未知邮箱也返回成功（防枚举）。
export const forgotPassword = (data: { email: string }) =>
  request.post<unknown, { message: string }>('/api/v1/password/forgot', data)

export const resetPassword = (data: { token: string; new_password: string }) =>
  request.post<unknown, { message: string }>('/api/v1/password/reset', data)

export const updateProfile = (data: UpdateProfileRequest) =>
  request.put('/api/v1/user/profile', data)

export const refreshToken = (data: { refresh_token: string }) =>
  request.post<unknown, LoginResponse>('/api/v1/refresh', data)

export const generateTotpSetup = (data: { current_password: string }) =>
  request.post('/api/v1/user/2fa/setup', data)

export const enableTotp = (data: { code: string; current_password: string }) =>
  request.post('/api/v1/user/2fa/enable', data)

export const disableTotp = (data: { code: string; current_password: string }) =>
  request.post('/api/v1/user/2fa/disable', data)

export const regenerateTotpRecoveryCodes = (data: { code: string; current_password: string }) =>
  request.post('/api/v1/user/2fa/recovery-codes', data)

export interface CaptchaData {
  key: string
  type: string
  image: string
  width: number
  height: number
}

export const getCaptcha = () =>
  request.get<unknown, CaptchaData>('/api/v1/captcha')
