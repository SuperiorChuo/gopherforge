import { typedApi } from '@/api/generated/client';
import type { components } from '@/api/generated/schema';
import { request } from '@/utils/request';

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];

export type OperationLogItem = Schema<'OperationLogItem'> & {
  location?: string;
};
export type OperationLogListResponse = Schema<'OperationLogListResponse'>;
export type OperationLogStats = Schema<'OperationLogStats'>;

export interface OperationLogListRequest {
  page?: number;
  page_size?: number;
  user_id?: number;
  username?: string;
  actor_type?: string;
  actor_id?: string;
  request_id?: string;
  method?: string;
  path?: string;
  module?: string;
  action?: string;
  status?: number;
  start_time?: string;
  end_time?: string;
}

export function getOperationLogs(params?: OperationLogListRequest) {
  return typedApi.get('/api/v1/operation-logs', {
    query: params,
  });
}

export function getOperationLogStats(params?: { start_time?: string; end_time?: string }) {
  return typedApi.get('/api/v1/operation-logs/stats', {
    query: params,
  });
}

export function getOperationLogDetail(id: number) {
  return typedApi.get('/api/v1/operation-logs/{id}', {
    path: { id },
  });
}

export function exportOperationLogs(params?: OperationLogListRequest) {
  return request.get<Blob>(
    {
      url: '/operation-logs/export',
      params,
      responseType: 'blob',
    },
    {
      isTransformResponse: false,
    },
  );
}

export function clearOperationLogs(days = 30) {
  return typedApi.delete('/api/v1/operation-logs/clear', {
    body: { days },
  });
}
