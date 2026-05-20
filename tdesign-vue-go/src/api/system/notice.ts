import { typedApi } from '@/api/generated/client';
import type { components } from '@/api/generated/schema';

type Schema<Name extends keyof components['schemas']> = components['schemas'][Name];

export type NoticeItem = Schema<'NoticeItem'>;
export type NoticeListResponse = Schema<'NoticeListResponse'>;
export type CreateNoticeRequest = Schema<'CreateNoticeRequest'>;
export type UpdateNoticeRequest = Schema<'UpdateNoticeRequest'>;

export interface NoticeListRequest {
  page?: number;
  page_size?: number;
  type?: number;
  status?: number;
  keyword?: string;
}

export function getNoticeList(params?: NoticeListRequest) {
  return typedApi.get('/api/v1/notices', {
    query: params,
  });
}

export function getActiveNotices(type?: number) {
  return typedApi.get('/api/v1/notices/active', {
    query: type !== undefined ? { type } : undefined,
  });
}

export function getNotice(id: number) {
  return typedApi.get('/api/v1/notices/{id}', {
    path: { id },
  });
}

export function createNotice(data: CreateNoticeRequest) {
  return typedApi.post('/api/v1/notices', {
    body: data,
  });
}

export function updateNotice(id: number, data: UpdateNoticeRequest) {
  return typedApi.put('/api/v1/notices/{id}', {
    path: { id },
    body: data,
  });
}

export function deleteNotice(id: number) {
  return typedApi.delete('/api/v1/notices/{id}', {
    path: { id },
  });
}

export function updateNoticeStatus(id: number, status: number) {
  return typedApi.put('/api/v1/notices/{id}/status', {
    path: { id },
    body: { status },
  });
}
