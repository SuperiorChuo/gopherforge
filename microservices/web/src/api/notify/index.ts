import request from '@/utils/request'

export interface NotifyMessage {
  id: number
  tenant_id: number
  user_id: number
  template_code?: string
  title: string
  content: string
  link?: string
  type: string
  ref_type?: string
  ref_id?: string
  read_at?: string | null
  created_at: string
}

export function listNotifyMessages(
  params?: { page?: number; page_size?: number; unread?: string; type?: string },
  opts?: { silent?: boolean },
) {
  return request.get('/api/v1/notify/messages', {
    params,
    silent: opts?.silent ?? true,
  }) as Promise<{ list: NotifyMessage[]; total: number }>
}

export function getNotifyUnreadCount() {
  return request.get('/api/v1/notify/messages/unread-count', { silent: true }) as Promise<{ count: number }>
}

export function markNotifyRead(id: number) {
  return request.post(`/api/v1/notify/messages/${id}/read`, undefined, { silent: true }) as Promise<{ ok: boolean }>
}

export function markAllNotifyRead() {
  return request.post('/api/v1/notify/messages/read-all', undefined, { silent: true }) as Promise<{ updated: number }>
}

export interface NotifyChannelStatus {
  inbox?: boolean
  email?: { enabled: boolean; provider?: string }
  sms?: { enabled: boolean; provider?: string }
  voice?: { enabled: boolean; provider?: string }
}

export function getNotifyChannels() {
  return request.get('/api/v1/notify/channels', { silent: true }) as Promise<NotifyChannelStatus>
}

export interface NotifyTemplate {
  id: number
  code: string
  title_tpl: string
  body_tpl: string
  link_tpl?: string
  channel: string
  channels: string
  enabled: boolean
}

export function listNotifyTemplates() {
  return request.get('/api/v1/notify/templates') as Promise<{
    list: NotifyTemplate[]
    channels?: NotifyChannelStatus
  }>
}

export function getNotifyTemplate(id: number) {
  return request.get(`/api/v1/notify/templates/${id}`) as Promise<NotifyTemplate>
}

export function updateNotifyTemplate(
  id: number,
  body: {
    title_tpl?: string
    body_tpl?: string
    link_tpl?: string
    channels?: string
    enabled?: boolean
  },
) {
  return request.put(`/api/v1/notify/templates/${id}`, body) as Promise<NotifyTemplate>
}

export function previewNotifyTemplate(id: number, vars?: Record<string, string>) {
  return request.post(`/api/v1/notify/templates/${id}/preview`, { vars }) as Promise<{
    code: string
    title: string
    content: string
    link: string
    channels: string[]
  }>
}

export function testSendNotifyTemplate(id: number, vars?: Record<string, string>) {
  return request.post(`/api/v1/notify/templates/${id}/test-send`, { vars }) as Promise<{
    title: string
    content: string
    link?: string
    channels: string[]
    delivered?: { channel: string; status: string; detail?: string; target?: string }[]
  }>
}

export interface NotifyDelivery {
  id: number
  channel: string
  target?: string
  status: string
  detail?: string
  template_code?: string
  created_at: string
}

export function listNotifyDeliveries(limit = 20) {
  return request.get('/api/v1/notify/deliveries', {
    params: { limit },
    silent: true,
  }) as Promise<{ list: NotifyDelivery[] }>
}

export interface NotifyDeliveryAdmin extends NotifyDelivery {
  user_id: number
  ref_type?: string
  ref_id?: string
}

/** 全部发送日志（管理视图，仅平台管理员；邮件管理面，路线图第 12 项） */
export function listAllNotifyDeliveries(params: {
  channel?: string
  status?: string
  keyword?: string
  page?: number
  page_size?: number
}) {
  return request.get('/api/v1/notify/deliveries/all', { params }) as Promise<{
    list: NotifyDeliveryAdmin[]
    total: number
  }>
}
