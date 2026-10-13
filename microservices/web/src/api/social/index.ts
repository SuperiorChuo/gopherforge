import request from '@/utils/request'

// ---------- 类型 ----------

export type SocialPlatform = '' | 'wechat' | 'xiaohongshu' | 'zhihu' | 'weibo' | 'douyin'
export type PostStatus = 'draft' | 'ready' | 'archived'

export type MediaItem = { kind: 'image' | 'video'; url: string; name?: string }

export type SocialPost = {
  id: number
  tenant_id: number
  title: string
  body_html: string
  summary: string
  cover_url: string
  media: MediaItem[] | null
  tags: string[] | null
  platform: SocialPlatform
  status: PostStatus | string
  ai_prompt: string
  created_by: number
  created_at: string
  updated_at: string
}

export type PostPublication = {
  id: number
  post_id: number
  channel: string
  account_id: number
  status: 'pending' | 'publishing' | 'published' | 'exporting' | 'exported' | 'failed' | string
  mode: 'api' | 'export' | string
  external_id: string
  external_url: string
  error?: string
  created_at: string
}

export type ChannelCapabilities = {
  publish: boolean
  export: boolean
  media_kinds: string[]
  max_title_len: number
  max_body_len: number
  note?: string
}

export type ChannelAccount = {
  id: number
  tenant_id: number
  channel: string
  name: string
  ref_id: number
  config_json: string
  status: string
  created_at: string
  updated_at: string
}

export type ExportPackage = {
  platform_text: string
  images?: MediaItem[]
  videos?: MediaItem[]
  tips?: string
}

type PageResult<T> = { list: T[]; total: number; page: number; page_size: number }

// ---------- 渠道 ----------

export function listSocialChannels() {
  return request.get('/api/v1/social/channels') as Promise<{ channels: Record<string, ChannelCapabilities> }>
}

export function listSocialChannelAccounts(channel?: string) {
  return request.get('/api/v1/social/channel-accounts', { params: { channel } }) as Promise<{ list: ChannelAccount[] }>
}

// ---------- Post CRUD ----------

export function listSocialPosts(params: { platform?: string; status?: string; keyword?: string; page?: number; page_size?: number }) {
  return request.get('/api/v1/social/posts', { params }) as Promise<PageResult<SocialPost>>
}

export function createSocialPost(body: Partial<Pick<SocialPost, 'title' | 'body_html' | 'summary' | 'cover_url' | 'platform' | 'media' | 'tags' | 'status'>>) {
  return request.post('/api/v1/social/posts', body) as Promise<SocialPost>
}

export function generateSocialPost(body: { prompt: string; platform?: string }) {
  return request.post('/api/v1/social/posts/generate', body) as Promise<SocialPost>
}

export function getSocialPost(id: number) {
  return request.get(`/api/v1/social/posts/${id}`) as Promise<{ post: SocialPost; publications: PostPublication[] }>
}

export function updateSocialPost(id: number, body: Partial<Pick<SocialPost, 'title' | 'body_html' | 'summary' | 'cover_url' | 'platform' | 'media' | 'tags' | 'status'>>) {
  return request.put(`/api/v1/social/posts/${id}`, body) as Promise<SocialPost>
}

export function deleteSocialPost(id: number) {
  return request.delete(`/api/v1/social/posts/${id}`) as Promise<{ deleted: boolean }>
}

// ---------- 发布 / 导出 ----------

export function publishSocialPost(id: number, body: { channel: string; account_id: number }) {
  return request.post(`/api/v1/social/posts/${id}/publish`, body) as Promise<PostPublication>
}

export function exportSocialPost(id: number, body: { channel: string; account_id?: number }) {
  return request.post(`/api/v1/social/posts/${id}/export`, body) as Promise<ExportPackage>
}

// ---------- 展示 meta ----------

export const SOCIAL_PLATFORM_META: Record<string, { label: string; color: string }> = {
  '': { label: '通用', color: 'default' },
  wechat: { label: '微信公众号', color: 'green' },
  xiaohongshu: { label: '小红书', color: 'red' },
  zhihu: { label: '知乎', color: 'blue' },
  weibo: { label: '微博', color: 'orange' },
  douyin: { label: '抖音', color: 'magenta' },
}

export const POST_STATUS_META: Record<string, { label: string; color: string }> = {
  draft: { label: '草稿', color: 'default' },
  ready: { label: '待发布', color: 'blue' },
  archived: { label: '已归档', color: 'purple' },
}

export const PUBLICATION_STATUS_META: Record<string, { label: string; color: string }> = {
  pending: { label: '待处理', color: 'default' },
  publishing: { label: '发布中', color: 'orange' },
  published: { label: '已发布', color: 'green' },
  exporting: { label: '导出中', color: 'orange' },
  exported: { label: '已导出', color: 'cyan' },
  failed: { label: '失败', color: 'red' },
}

export const SOCIAL_PLATFORM_OPTIONS = [
  { label: '通用', value: '' },
  { label: '微信公众号', value: 'wechat' },
  { label: '小红书', value: 'xiaohongshu' },
  { label: '知乎', value: 'zhihu' },
  { label: '微博', value: 'weibo' },
  { label: '抖音', value: 'douyin' },
]
