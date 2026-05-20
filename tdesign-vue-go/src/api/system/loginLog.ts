import { typedApi } from '@/api/generated/client';
import type { components } from '@/api/generated/schema';

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];

export type LoginLogItem = Schema<'LoginLogItem'>;
export type LoginLogListResponse = Schema<'LoginLogListResponse'>;
export type LoginStats = Schema<'LoginStats'>;
export type LoginTrendItem = Schema<'LoginTrendItem'>;

export interface LoginLogListRequest {
  page?: number;
  page_size?: number;
  user_id?: number;
  username?: string;
  ip?: string;
  status?: number;
  login_type?: number;
  start_time?: string;
  end_time?: string;
}

export function getLoginLogs(params?: LoginLogListRequest) {
  return typedApi.get('/api/v1/login-logs', {
    query: params,
  });
}

export function getMyLoginLogs(params?: Omit<LoginLogListRequest, 'user_id'>) {
  return typedApi.get('/api/v1/login-logs/my', {
    query: params,
  });
}

export function getLoginStats(params?: { start_time?: string; end_time?: string }) {
  return typedApi.get('/api/v1/login-logs/stats', {
    query: params,
  });
}

export function getLoginTrend(days = 7) {
  return typedApi.get('/api/v1/login-logs/trend', {
    query: { days },
  });
}

export function getLastLogin() {
  return typedApi.get('/api/v1/login-logs/last');
}

export async function getUserLoginHistory(userId: number, params?: Omit<LoginLogListRequest, 'user_id'>) {
  const response = await typedApi.get('/api/v1/login-logs/user/{user_id}', {
    path: { user_id: userId },
    query: params,
  });
  return response.list || [];
}

export function clearLoginLogs(days = 30) {
  return typedApi.delete('/api/v1/login-logs/clear', {
    body: { days },
  });
}
