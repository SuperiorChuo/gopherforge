import request from '@/utils/request'

export type TicketStatus = 'open' | 'in_progress' | 'resolved' | 'closed' | string
export type TicketPriority = 'low' | 'normal' | 'high' | 'urgent' | string
export type TicketSource = 'manual' | 'im' | 'call' | 'other' | string

export type Ticket = {
  id: number
  ticket_no: string
  title: string
  description?: string
  status: TicketStatus
  priority: TicketPriority
  source: TicketSource
  creator_user_id: number
  assignee_user_id: number
  customer_id?: number
  conversation_public_id?: string
  im_visitor_id?: number
  contact_name?: string
  contact_phone?: string
  due_at?: string
  resolved_at?: string
  closed_at?: string
  resolve_note?: string
  created_at: string
  updated_at: string
}

export type TicketEvent = {
  id: number
  ticket_id: number
  kind: 'created' | 'comment' | 'status_change' | 'assign' | 'update' | string
  content?: string
  from_status?: string
  to_status?: string
  by_user_id: number
  created_at: string
}

export type TicketStats = {
  open: number
  in_progress: number
  resolved: number
  closed: number
  overdue: number
  mine_open: number
  mine_overdue: number
}

export type TicketSLAPolicy = {
  urgent_hours: number
  high_hours: number
  normal_hours: number
  low_hours: number
}

export const STATUS_META: Record<string, { label: string; color: string }> = {
  open: { label: '待处理', color: 'blue' },
  in_progress: { label: '处理中', color: 'orange' },
  resolved: { label: '已解决', color: 'green' },
  closed: { label: '已关闭', color: 'default' },
}

export const PRIORITY_META: Record<string, { label: string; color: string }> = {
  low: { label: '低', color: 'default' },
  normal: { label: '普通', color: 'blue' },
  high: { label: '高', color: 'orange' },
  urgent: { label: '紧急', color: 'red' },
}

export const SOURCE_META: Record<string, string> = {
  manual: '手工',
  im: '在线客服',
  call: '来电',
  other: '其他',
}

type PageResult<T> = { list: T[]; total: number }

export function listTickets(params: Record<string, unknown>) {
  return request.get('/api/v1/ticket/tickets', { params }) as Promise<PageResult<Ticket>>
}

export function getTicket(id: number) {
  return request.get(`/api/v1/ticket/tickets/${id}`) as Promise<{ ticket: Ticket; events: TicketEvent[] }>
}

export function createTicket(body: {
  title: string
  description?: string
  priority?: string
  source?: string
  assignee_user_id?: number
  customer_id?: number
  conversation_public_id?: string
  im_visitor_id?: number
  contact_name?: string
  contact_phone?: string
  due_at?: string
}) {
  return request.post('/api/v1/ticket/tickets', body) as Promise<Ticket>
}

export function updateTicket(
  id: number,
  body: {
    title?: string
    description?: string
    priority?: string
    customer_id?: number
    contact_name?: string
    contact_phone?: string
    due_at?: string | null
  },
) {
  return request.put(`/api/v1/ticket/tickets/${id}`, body) as Promise<Ticket>
}

export function changeTicketStatus(id: number, status: string, note?: string) {
  return request.post(`/api/v1/ticket/tickets/${id}/status`, { status, note }) as Promise<Ticket>
}

export function assignTicket(id: number, assignee_user_id: number) {
  return request.post(`/api/v1/ticket/tickets/${id}/assign`, { assignee_user_id }) as Promise<Ticket>
}

export function addTicketComment(id: number, content: string) {
  return request.post(`/api/v1/ticket/tickets/${id}/comments`, { content }) as Promise<TicketEvent>
}

export function listTicketEvents(id: number) {
  return request.get(`/api/v1/ticket/tickets/${id}/events`) as Promise<{ list: TicketEvent[] }>
}

export function getTicketStats() {
  return request.get('/api/v1/ticket/stats') as Promise<TicketStats>
}

export function getTicketSLAPolicy() {
  return request.get('/api/v1/ticket/sla') as Promise<TicketSLAPolicy>
}
