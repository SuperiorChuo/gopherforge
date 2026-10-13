import request from '@/utils/request'

export type PayChannel = {
  id: number
  tenant_id: number
  code: string
  name: string
  status: number
  fee_rate_bps: number
  key_version: number
  remark?: string
  created_at: string
  updated_at: string
}

export type PayOrder = {
  id: number
  order_no: string
  biz_type: string
  biz_id: string
  subject: string
  amount_cents: number
  refunded_cents: number
  status: 'waiting' | 'paying' | 'paid' | 'closed' | string
  channel_code?: string
  channel_trade_no?: string
  paid_at?: string
  expire_at: string
  created_at: string
}

export type PayRefund = {
  id: number
  refund_no: string
  order_no: string
  amount_cents: number
  reason?: string
  status: 'pending' | 'processing' | 'refunded' | 'failed' | string
  channel_refund_no?: string
  operator_user_id: number
  finished_at?: string
  created_at: string
}

export type PayCallbackJob = {
  id: number
  order_id: number
  kind: 'order' | 'refund' | string
  attempts: number
  next_at: string
  status: 'pending' | 'dead' | string
  last_error?: string
  created_at: string
}

export type PayNotifyLog = {
  id: number
  kind: string
  channel_code: string
  order_no?: string
  sign_ok: boolean
  result: string
  created_at: string
}

export type PayStats = {
  waiting: number
  paying: number
  paid: number
  closed: number
  paid_amount_cents: number
  refunded_amount_cents: number
}

// 收银台（匿名，token 即凭据）
export type CashierInfo = {
  order_no: string
  subject: string
  amount_cents: number
  status: string
  channel_code?: string
  expire_at: string
  channels: { code: string; name: string }[]
  // mock_enabled 后端是否注册了 mock 演示渠道（PAY_DEMO_MODE，生产默认关）。
  // 「模拟支付」按钮只认这个字段——收银台是匿名页面，能力判断必须来自后端。
  mock_enabled?: boolean
}

export const payApi = {
  stats: () => request.get<unknown, PayStats>('/api/v1/pay/stats'),
  channelCodes: () => request.get<unknown, string[]>('/api/v1/pay/channel-codes'),
  listChannels: () => request.get<unknown, PayChannel[]>('/api/v1/pay/channels'),
  createChannel: (data: Partial<PayChannel>) => request.post<unknown, PayChannel>('/api/v1/pay/channels', data),
  updateChannel: (id: number, data: Partial<PayChannel>) => request.put(`/api/v1/pay/channels/${id}`, data),
  deleteChannel: (id: number) => request.delete(`/api/v1/pay/channels/${id}`),

  listOrders: (params: { status?: string; biz_type?: string; page?: number; page_size?: number }) =>
    request.get<unknown, { list: PayOrder[]; total: number }>('/api/v1/pay/orders', { params }),
  getOrder: (orderNo: string) => request.get<unknown, PayOrder>(`/api/v1/pay/orders/${orderNo}`),
  closeOrder: (orderNo: string) => request.post(`/api/v1/pay/orders/${orderNo}/close`),

  listRefunds: (params: { status?: string; page?: number; page_size?: number }) =>
    request.get<unknown, { list: PayRefund[]; total: number }>('/api/v1/pay/refunds', { params }),
  createRefund: (data: { order_no: string; amount_cents: number; reason?: string }) =>
    request.post<unknown, PayRefund>('/api/v1/pay/refunds', data),

  listCallbackJobs: (status?: string) =>
    request.get<unknown, PayCallbackJob[]>('/api/v1/pay/callback-jobs', { params: { status } }),
  replayCallbackJob: (id: number) => request.post(`/api/v1/pay/callback-jobs/${id}/replay`),
  listNotifyLogs: () => request.get<unknown, PayNotifyLog[]>('/api/v1/pay/notify-logs'),

  // 收银台（匿名端点，走同一 axios 实例；无 token 也可访问）
  cashierGet: (token: string) => request.get<unknown, CashierInfo>(`/api/v1/pay/cashier/${token}`),
  cashierPay: (token: string, channelCode: string) =>
    request.post<unknown, { order_no: string; status: string; trade: { qr_content?: string; redirect_url?: string } }>(
      `/api/v1/pay/cashier/${token}/pay`,
      { channel_code: channelCode },
    ),
  cashierMock: (token: string, succeed: boolean) =>
    request.post<unknown, { order_no: string; status: string }>(`/api/v1/pay/cashier/${token}/mock`, { succeed }),
}

export function formatCents(cents: number): string {
  return `¥${(cents / 100).toFixed(2)}`
}
