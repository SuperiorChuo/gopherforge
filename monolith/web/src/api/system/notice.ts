import request from '@/utils/request'
import type { PageRequest, PageResponse, Notice } from '@/types'

type NoticeListParams = PageRequest & { keyword?: string; type?: number; status?: number }
type NoticeCreateData = Omit<Notice, 'id' | 'created_at'>
type NoticeUpdateData = Partial<NoticeCreateData>

export const getNoticeList = (params: NoticeListParams) =>
  request.get<unknown, PageResponse<Notice>>('/api/v1/notices', { params })

export const createNotice = (data: NoticeCreateData) =>
  request.post<unknown, Notice>('/api/v1/notices', data)

export const updateNotice = (id: number, data: NoticeUpdateData) =>
  request.put<unknown, Notice>(`/api/v1/notices/${id}`, data)

export const deleteNotice = (id: number) =>
  request.delete<unknown, void>(`/api/v1/notices/${id}`)

export const updateNoticeStatus = (id: number, status: number) =>
  request.put<unknown, void>(`/api/v1/notices/${id}/status`, { status })

// WebSocket 通知的一次性连接票据（1 分钟有效）
export const createNotificationTicket = () =>
  request.post<unknown, { ticket: string }>('/api/v1/ws/notifications/ticket', undefined, {
    silent: true,
  })

// 当前生效的公告（仪表盘展示用），返回普通数组
export const getActiveNotices = (type?: number) =>
  request.get<unknown, Notice[]>('/api/v1/notices/active', {
    params: type !== undefined ? { type } : undefined,
    silent: true,
  })
