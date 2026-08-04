import request from '@/utils/request'

export type InviteInfo = {
  id: number
  tenant_id: number
  role_id?: number
  email?: string
  expires_at: string
  used_at?: string
  revoked_at?: string
  created_at: string
}

export type InviteCreateResult = {
  id: number
  token: string // 一次性明文，仅创建时返回
  link: string
  role_id?: number
  email?: string
  expires_at: string
}

export const createInvite = (data: { role_id?: number; email?: string }) =>
  request.post<unknown, InviteCreateResult>('/api/v1/invites', data)

export const listInvites = () =>
  request.get<unknown, InviteInfo[]>('/api/v1/invites')

export const revokeInvite = (id: number) =>
  request.delete<unknown, void>(`/api/v1/invites/${id}`)
