import request from '@/utils/request'
import type { PageRequest, PageResponse } from '@/types'

// ---------- 类型 ----------

export const CLIENT_TYPE = { CONFIDENTIAL: 1, PUBLIC: 2 } as const

export interface OAuth2Client {
  id: number
  tenant_id: number
  client_id: string
  name: string
  logo: string
  description: string
  client_type: number // 1 机密 / 2 公开
  redirect_uris: string[]
  scopes: string[]
  grant_types: string[]
  access_token_ttl: number
  refresh_token_ttl: number
  auto_approve: boolean
  status: number
  created_by: number
  created_at: string
  updated_at: string
}

export interface OAuth2AccessToken {
  id: number
  tenant_id: number
  client_id: string
  user_id?: number | null
  username: string
  scopes: string[]
  grant_type: string
  expires_at: string
  revoked_at?: string | null
  created_at: string
}

export interface OAuth2ClientSaveData {
  name: string
  logo?: string
  description?: string
  client_type: number
  redirect_uris: string[]
  scopes: string[]
  grant_types: string[]
  access_token_ttl?: number
  refresh_token_ttl?: number
  auto_approve?: boolean
  status?: number
}

// 创建 / 重置密钥的一次性返回：client_secret 仅此一次可见
export interface OAuth2CreateResult {
  client: OAuth2Client
  client_secret: string
}

export type OAuth2ClientListParams = PageRequest & { keyword?: string }
export type OAuth2TokenListParams = PageRequest & { client_id?: string }

export interface OAuth2Catalog {
  scopes: string[]
  grant_types: string[]
}

// 授权确认页视图（GET /oauth2/authorize 返回）
export interface OAuth2AuthorizeView {
  client_id: string
  client_name: string
  logo: string
  description: string
  scopes: string[]
  state: string
  redirect_uri: string
  auto_approve: boolean
  already_approved: boolean
}

// ---------- 应用管理 ----------

export const getOAuth2Catalog = () =>
  request.get<unknown, OAuth2Catalog>('/api/v1/oauth2/catalog')

export const getOAuth2ClientList = (params: OAuth2ClientListParams) =>
  request.get<unknown, PageResponse<OAuth2Client>>('/api/v1/oauth2/clients', { params })

export const getOAuth2Client = (id: number) =>
  request.get<unknown, OAuth2Client>(`/api/v1/oauth2/clients/${id}`)

export const createOAuth2Client = (data: OAuth2ClientSaveData) =>
  request.post<unknown, OAuth2CreateResult>('/api/v1/oauth2/clients', data)

export const updateOAuth2Client = (id: number, data: OAuth2ClientSaveData) =>
  request.put<unknown, OAuth2Client>(`/api/v1/oauth2/clients/${id}`, data)

export const resetOAuth2ClientSecret = (id: number) =>
  request.post<unknown, { client_secret: string }>(`/api/v1/oauth2/clients/${id}/reset-secret`)

export const deleteOAuth2Client = (id: number) =>
  request.delete<unknown, void>(`/api/v1/oauth2/clients/${id}`)

// ---------- 令牌管理 ----------

export const getOAuth2TokenList = (params: OAuth2TokenListParams) =>
  request.get<unknown, PageResponse<OAuth2AccessToken>>('/api/v1/oauth2/tokens', { params })

export const revokeOAuth2Token = (id: number) =>
  request.delete<unknown, void>(`/api/v1/oauth2/tokens/${id}`)

// ---------- 授权确认页 ----------

// authorize 走仓内响应封装（自家前端消费）；search 是原始 query 串（含 client_id/scope/state 等）
export const getOAuth2Authorize = (search: string) =>
  request.get<unknown, OAuth2AuthorizeView>(`/api/v1/oauth2/authorize${search}`)

export interface OAuth2ApproveData {
  client_id: string
  redirect_uri: string
  response_type: string
  scope: string
  state: string
  code_challenge: string
  code_challenge_method: string
  approved: boolean
}

export const postOAuth2Authorize = (data: OAuth2ApproveData) =>
  request.post<unknown, { redirect_url: string }>('/api/v1/oauth2/authorize', data)
