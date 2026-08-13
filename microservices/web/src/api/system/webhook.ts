import request from '@/utils/request'

export interface WebhookSubscription {
  id: number
  tenant_id: number
  name: string
  endpoint_url: string
  event_actions: string[]
  status: number
  consecutive_failures: number
  last_delivered_at?: string
  last_error?: string
  created_at: string
  updated_at: string
}

export interface WebhookDelivery {
  id: number
  subscription_id: number
  event_id: string
  event_action: string
  status: 'pending' | 'retrying' | 'sent' | 'failed'
  attempts: number
  response_status?: number
  response_body?: string
  last_error?: string
  delivered_at?: string
  created_at: string
}

export interface WebhookMutation {
  name: string
  endpoint_url: string
  event_actions: string[]
  status: number
}

export interface WebhookSecretResult {
  subscription: WebhookSubscription
  secret: string
}

export const listWebhooks = (params?: { page?: number; page_size?: number }) =>
  request.get<unknown, { list: WebhookSubscription[]; total: number; page: number; page_size: number }>('/api/v1/webhooks', { params })

export const createWebhook = (data: WebhookMutation) =>
  request.post<unknown, WebhookSecretResult>('/api/v1/webhooks', data)

export const updateWebhook = (id: number, data: WebhookMutation) =>
  request.put<unknown, WebhookSubscription>(`/api/v1/webhooks/${id}`, data)

export const deleteWebhook = (id: number) => request.delete(`/api/v1/webhooks/${id}`)

export const resetWebhookSecret = (id: number) =>
  request.post<unknown, WebhookSecretResult>(`/api/v1/webhooks/${id}/reset-secret`)

export const listWebhookDeliveries = (params?: { subscription_id?: number; page?: number; page_size?: number }) =>
  request.get<unknown, { list: WebhookDelivery[]; total: number; page: number; page_size: number }>('/api/v1/webhook-deliveries', { params })
