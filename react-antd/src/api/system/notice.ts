import request from '@/utils/request'
import type { PageRequest, PageResponse, Notice } from '@/types'

type NoticeListParams = PageRequest & { keyword?: string; type?: number; status?: number }
type NoticeCreateData = Omit<Notice, 'id' | 'created_at'>
type NoticeUpdateData = Partial<NoticeCreateData>

export const getNoticeList = (params: NoticeListParams) =>
  request.get<unknown, PageResponse<Notice>>('/api/v1/system/notices', { params })

export const createNotice = (data: NoticeCreateData) =>
  request.post<unknown, Notice>('/api/v1/system/notices', data)

export const updateNotice = (id: number, data: NoticeUpdateData) =>
  request.put<unknown, Notice>(`/api/v1/system/notices/${id}`, data)

export const deleteNotice = (id: number) =>
  request.delete<unknown, void>(`/api/v1/system/notices/${id}`)
