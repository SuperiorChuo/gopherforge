import request from '@/utils/request'
import type { Schema } from '@/api/generated'

export type WebhookSubscription = Schema<'WebhookSubscription'>
export type WebhookDelivery = Schema<'WebhookDelivery'>
export type WebhookMutation = Schema<'WebhookMutationRequest'>
export type WebhookSecretResult = Schema<'WebhookSecretResult'>
export type WebhookSubscriptionList = Schema<'WebhookSubscriptionList'>
export type WebhookDeliveryList = Schema<'WebhookDeliveryList'>

export const listWebhooks = (params?: { page?: number; page_size?: number }) =>
  request.get<unknown, WebhookSubscriptionList>('/api/v1/webhooks', { params })

export const createWebhook = (data: WebhookMutation) =>
  request.post<unknown, WebhookSecretResult>('/api/v1/webhooks', data)

export const updateWebhook = (id: number, data: WebhookMutation) =>
  request.put<unknown, WebhookSubscription>(`/api/v1/webhooks/${id}`, data)

export const deleteWebhook = (id: number) => request.delete(`/api/v1/webhooks/${id}`)

export const resetWebhookSecret = (id: number) =>
  request.post<unknown, WebhookSecretResult>(`/api/v1/webhooks/${id}/reset-secret`)

export const listWebhookDeliveries = (params?: { subscription_id?: number; page?: number; page_size?: number }) =>
  request.get<unknown, WebhookDeliveryList>('/api/v1/webhook-deliveries', { params })
