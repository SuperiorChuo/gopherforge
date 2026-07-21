import request from '@/utils/request'
import type { PageRequest, PageResponse } from '@/types'

// ---------- 类型 ----------

export type SmsProvider = 'debug' | 'aliyun' | 'tencent'

// 渠道 config：access_key / secret / sign_name 等；密钥服务端脱敏为 ******，
// 编辑时回传 ****** 或留空表示不修改。
export type SmsChannelConfig = Record<string, string>

export interface SmsChannel {
  id: number
  tenant_id: number
  name: string
  provider: SmsProvider
  config?: SmsChannelConfig | null
  status: number
  remark: string
  created_at: string
  updated_at: string
}

export interface SmsTemplate {
  id: number
  tenant_id: number
  code: string
  name: string
  channel_id: number
  content: string
  type: number // 1 验证码 / 2 通知 / 3 营销
  provider_template_id: string
  status: number
  remark: string
  created_at: string
  updated_at: string
}

export type SmsLogStatus = 'sending' | 'success' | 'failure'

export interface SmsLog {
  id: number
  tenant_id: number
  mobile: string
  template_code: string
  content: string
  params?: Record<string, string> | null
  channel_id: number
  channel_name: string
  provider: string
  status: SmsLogStatus
  provider_msg_id: string
  error: string
  created_at: string
  updated_at: string
}

export interface SendSmsResult {
  log_id: number
  status: SmsLogStatus
  content: string
  provider_msg_id?: string
  error?: string
}

export type SmsChannelListParams = PageRequest & {
  keyword?: string
  provider?: string
  status?: number
}

export type SmsTemplateListParams = PageRequest & {
  keyword?: string
  channel_id?: number
  type?: number
  status?: number
}

export type SmsLogListParams = PageRequest & {
  mobile?: string
  template_code?: string
  status?: string
}

export type SmsChannelSaveData = {
  name: string
  provider: SmsProvider
  config?: SmsChannelConfig
  status?: number
  remark?: string
}

export type SmsTemplateSaveData = {
  code: string
  name: string
  channel_id: number
  content: string
  type?: number
  provider_template_id?: string
  status?: number
  remark?: string
}

// ---------- 渠道 ----------

export const getSmsChannelList = (params: SmsChannelListParams) =>
  request.get<unknown, PageResponse<SmsChannel>>('/api/v1/sms/channels', { params })

// 启用中的渠道（模板表单下拉用）
export const getEnabledSmsChannels = () =>
  request.get<unknown, SmsChannel[]>('/api/v1/sms/channels/enabled')

export const createSmsChannel = (data: SmsChannelSaveData) =>
  request.post<unknown, SmsChannel>('/api/v1/sms/channels', data)

export const updateSmsChannel = (id: number, data: Partial<SmsChannelSaveData>) =>
  request.put<unknown, SmsChannel>(`/api/v1/sms/channels/${id}`, data)

export const updateSmsChannelStatus = (id: number, status: number) =>
  request.put<unknown, void>(`/api/v1/sms/channels/${id}/status`, { status })

export const deleteSmsChannel = (id: number) =>
  request.delete<unknown, void>(`/api/v1/sms/channels/${id}`)

// ---------- 模板 ----------

export const getSmsTemplateList = (params: SmsTemplateListParams) =>
  request.get<unknown, PageResponse<SmsTemplate>>('/api/v1/sms/templates', { params })

export const createSmsTemplate = (data: SmsTemplateSaveData) =>
  request.post<unknown, SmsTemplate>('/api/v1/sms/templates', data)

export const updateSmsTemplate = (id: number, data: Partial<SmsTemplateSaveData>) =>
  request.put<unknown, SmsTemplate>(`/api/v1/sms/templates/${id}`, data)

export const updateSmsTemplateStatus = (id: number, status: number) =>
  request.put<unknown, void>(`/api/v1/sms/templates/${id}/status`, { status })

export const deleteSmsTemplate = (id: number) =>
  request.delete<unknown, void>(`/api/v1/sms/templates/${id}`)

// ---------- 发送日志 ----------

export const getSmsLogList = (params: SmsLogListParams) =>
  request.get<unknown, PageResponse<SmsLog>>('/api/v1/sms/logs', { params })

// ---------- 发送（业务发送与模板测试发送共用） ----------

export const sendSms = (data: { mobile: string; template_code: string; params?: Record<string, string> }) =>
  request.post<unknown, SendSmsResult>('/api/v1/sms/send', data)
