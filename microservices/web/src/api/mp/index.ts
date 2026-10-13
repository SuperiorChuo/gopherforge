import request from '@/utils/request'

// ---------- 类型 ----------

export type MpAccountType = 'service' | 'subscribe'
export type MpEncryptMode = 'plain' | 'compat' | 'safe'

export type MpAccount = {
  id: number
  tenant_id: number
  app_id: string
  name: string
  type: MpAccountType | string
  gh_id: string
  secret_ref: string
  aes_key_ref: string
  encrypt_mode: MpEncryptMode | string
  auto_publish: boolean
  status: string
  created_at: string
  updated_at: string
  // token 后端不回显，仅用于表单提交
  token?: string
}

export type MpAccountPayload = {
  app_id: string
  name: string
  type: MpAccountType | string
  gh_id?: string
  token?: string
  secret_ref?: string
  aes_key_ref?: string
  encrypt_mode: MpEncryptMode | string
  auto_publish: boolean
}

export type MpMessageDirection = 'in' | 'out'

export type MpMessage = {
  id: number
  account_id: number
  open_id: string
  direction: MpMessageDirection | string
  msg_type: string
  content: string
  msg_id: string
  event: string
  created_at: string
}

export type MpCustomerMessagePayload = {
  msg_type: 'text' | 'image'
  content?: string
  media_id?: string
}

export type MpCustomerReplyResult = {
  sent: boolean
  archived: boolean
  message?: MpMessage
}

export type MpMenuButtonType =
  | 'click'
  | 'view'
  | 'miniprogram'
  | 'media_id'
  | 'view_limited'
  | 'article_id'
  | 'article_view_limited'
  | 'scancode_push'
  | 'scancode_waitmsg'
  | 'pic_sysphoto'
  | 'pic_photo_or_album'
  | 'pic_weixin'
  | 'location_select'

export type MpMenuButton = {
  name: string
  type?: MpMenuButtonType | string
  key?: string
  url?: string
  appid?: string
  pagepath?: string
  media_id?: string
  article_id?: string
  sub_button?: MpMenuButton[]
}

export type MpMenuState = {
  account_id: number
  buttons: MpMenuButton[]
  published: boolean
  updated_at?: string
}

export type MpFan = {
  id: number
  account_id: number
  open_id: string
  nickname: string
  avatar: string
  city: string
  tags: string
  subscribed: boolean
  subscribe_time: string
  created_at: string
}

export type MpFanSyncStatus = 'pending' | 'running' | 'succeeded' | 'failed'

export type MpFanSyncJob = {
  id: number
  account_id: number
  status: MpFanSyncStatus
  total_count: number
  listed_count: number
  synced_count: number
  attempts: number
  last_error?: string
  next_at?: string
  started_at?: string
  finished_at?: string
  created_at: string
  updated_at: string
}

type PageResult<T> = { list: T[]; total: number; page: number; page_size: number }

// ---------- 账号 ----------

export function listMpAccounts(params: { page?: number; page_size?: number; keyword?: string }) {
  return request.get('/api/v1/mp/accounts', { params }) as Promise<PageResult<MpAccount>>
}

export function getMpAccount(id: number, options: { silent?: boolean } = {}) {
  return request.get(`/api/v1/mp/accounts/${id}`, options) as Promise<MpAccount>
}

export function createMpAccount(body: MpAccountPayload) {
  return request.post('/api/v1/mp/accounts', body) as Promise<MpAccount>
}

export function updateMpAccount(id: number, body: MpAccountPayload) {
  return request.put(`/api/v1/mp/accounts/${id}`, body) as Promise<MpAccount>
}

export function deleteMpAccount(id: number) {
  return request.delete(`/api/v1/mp/accounts/${id}`) as Promise<{ deleted: boolean }>
}

// ---------- 自定义菜单 ----------

export function getMpMenu(accountId: number) {
  return request.get(`/api/v1/mp/accounts/${accountId}/menu`) as Promise<MpMenuState>
}

export function saveMpMenu(accountId: number, body: { buttons: MpMenuButton[] }) {
  return request.put(`/api/v1/mp/accounts/${accountId}/menu`, body) as Promise<MpMenuState>
}

export function publishMpMenu(accountId: number) {
  return request.post(`/api/v1/mp/accounts/${accountId}/menu/publish`, {}) as Promise<MpMenuState>
}

export function deleteMpMenu(accountId: number) {
  return request.delete(`/api/v1/mp/accounts/${accountId}/menu`) as Promise<{ deleted: boolean }>
}

// ---------- 消息 ----------

export function listMpMessages(params: {
  account_id?: number
  open_id?: string
  direction?: string
  page?: number
  page_size?: number
}) {
  return request.get('/api/v1/mp/messages', { params }) as Promise<PageResult<MpMessage>>
}

export function replyMpMessage(messageId: number, body: MpCustomerMessagePayload) {
  return request.post(`/api/v1/mp/messages/${messageId}/reply`, body) as Promise<MpCustomerReplyResult>
}

export function uploadMpCustomerImage(accountId: number, file: File) {
  const body = new FormData()
  body.append('file', file)
  return request.post(`/api/v1/mp/accounts/${accountId}/customer-media`, body) as Promise<{ media_id: string }>
}

// ---------- 粉丝 ----------

export function listMpFans(params: {
  account_id?: number
  keyword?: string
  page?: number
  page_size?: number
}) {
  return request.get('/api/v1/mp/fans', { params }) as Promise<PageResult<MpFan>>
}

export function getMpFanSync(accountId: number, options: { silent?: boolean } = {}) {
  return request.get(`/api/v1/mp/accounts/${accountId}/fans/sync`, options) as Promise<{
    job: MpFanSyncJob | null
  }>
}

export function startMpFanSync(accountId: number) {
  return request.post(`/api/v1/mp/accounts/${accountId}/fans/sync`, {}) as Promise<{
    job: MpFanSyncJob
    reused: boolean
  }>
}

// ---------- 展示辅助 ----------

export const MP_TYPE_META: Record<string, { label: string; color: string }> = {
  service: { label: '服务号', color: 'geekblue' },
  subscribe: { label: '订阅号', color: 'purple' },
}

export const MP_ENCRYPT_META: Record<string, { label: string; color: string }> = {
  plain: { label: '明文模式', color: 'default' },
  compat: { label: '兼容模式', color: 'orange' },
  safe: { label: '安全模式', color: 'green' },
}

export const MP_DIRECTION_META: Record<string, { label: string; tone: 'info' | 'success' }> = {
  in: { label: '接收', tone: 'info' },
  out: { label: '发送', tone: 'success' },
}

// ---------- AI 图文（M5）----------

export type MpArticle = {
  id: number
  tenant_id: number
  account_id: number
  title: string
  author: string
  digest: string
  content_html: string
  cover_url: string
  thumb_media_id: string
  draft_media_id: string
  publish_id: string
  article_url: string
  status: 'draft_ai' | 'drafted' | 'publishing' | 'published' | 'failed' | string
  ai_prompt: string
  created_by: number
  created_at: string
  updated_at: string
}

export const MP_ARTICLE_STATUS_META: Record<string, { label: string; color: string }> = {
  draft_ai: { label: 'AI 草稿', color: 'purple' },
  drafted: { label: '已入草稿箱', color: 'blue' },
  publishing: { label: '发布中', color: 'orange' },
  published: { label: '已发布', color: 'green' },
  failed: { label: '发布失败', color: 'red' },
}

export function listMpArticles(params: { account_id?: number; status?: string; page?: number; page_size?: number }) {
  return request.get('/api/v1/mp/articles', { params }) as Promise<PageResult<MpArticle>>
}

export function generateMpArticle(body: { account_id: number; prompt: string }) {
  return request.post('/api/v1/mp/articles/generate', body) as Promise<MpArticle>
}

export function getMpArticle(id: number) {
  return request.get(`/api/v1/mp/articles/${id}`) as Promise<MpArticle>
}

export function updateMpArticle(
  id: number,
  body: Partial<Pick<MpArticle, 'title' | 'author' | 'digest' | 'content_html' | 'cover_url'>>,
) {
  return request.put(`/api/v1/mp/articles/${id}`, body) as Promise<MpArticle>
}

export function deleteMpArticle(id: number) {
  return request.delete(`/api/v1/mp/articles/${id}`) as Promise<{ deleted: boolean }>
}

export function generateMpArticleCover(id: number, body: { prompt?: string; size?: string }) {
  return request.post(`/api/v1/mp/articles/${id}/cover`, body) as Promise<{ cover_url: string }>
}

export function publishMpArticle(id: number) {
  return request.post(`/api/v1/mp/articles/${id}/publish`, {}, { timeout: 120_000 }) as Promise<MpArticle>
}
