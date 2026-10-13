import request from '@/utils/request'

export type ImConversation = {
  id: number
  public_id: string
  status: string
  visitor_id: number
  agent_user_id?: number
  skill_group_id?: number
  channel?: string
  context?: string
  close_reason?: string
  summary?: string
  last_message_preview?: string
  last_message_at?: string
  created_at: string
  agent_last_read_seq?: number
  visitor_last_read_seq?: number
  unread_count?: number
  rating_score?: number
  rating_comment?: string
  rated_at?: string
}

/** Parsed conversation.context from widget / H5 (page source + IP). */
export type ImVisitorContext = {
  page_url?: string
  page?: string
  page_title?: string
  referrer?: string
  parent_origin?: string
  client_ip?: string
  ip_location?: string
  user_agent?: string
  channel?: string
  [key: string]: unknown
}

export type ImMessage = {
  id: number
  conversation_id: number
  sender_type: string
  sender_id?: number
  msg_type: string
  content: string
  seq: number
  created_at: string
}

export type ImPresence = {
  agent_user_id: number
  status: 'online' | 'busy' | 'offline' | string
  display_name?: string
  last_seen_at?: string
  assigned_count?: number
}

export type ImSkillGroup = {
  id: number
  name: string
  code: string
  strategy: string
  status: number
  agent_count?: number
  created_at?: string
}

export type ImAgentSkill = {
  id: number
  agent_user_id: number
  skill_group_id: number
  max_concurrent: number
  status: number
  presence?: ImPresence
  assigned_count?: number
  skill_group?: ImSkillGroup
}

export type ImAgentMe = {
  user_id: number
  username: string
  presence: ImPresence
  skills: ImAgentSkill[]
  assigned_count: number
  skill_groups_all: ImSkillGroup[]
}

export function getAgentMe() {
  return request.get('/api/v1/im/agent/me') as Promise<ImAgentMe>
}

export function setAgentPresence(status: string, displayName?: string) {
  return request.put('/api/v1/im/agent/presence', {
    status,
    display_name: displayName,
  }) as Promise<ImPresence>
}

export function listAgentConversations(scope: 'all' | 'mine' | 'queue' | 'bot' | 'closed' = 'all', skillGroupId?: number) {
  return request.get('/api/v1/im/agent/conversations', {
    params: {
      scope,
      skill_group_id: skillGroupId || undefined,
    },
  }) as Promise<{ list: ImConversation[]; scope: string }>
}

export function getAgentQueue(skillGroupId?: number) {
  return request.get('/api/v1/im/agent/queue', {
    params: { skill_group_id: skillGroupId || undefined },
  }) as Promise<{ list: ImConversation[]; queue_size: number; online_agents: ImPresence[] }>
}

export function listOnlineAgents() {
  return request.get('/api/v1/im/agent/online') as Promise<{ list: ImPresence[] }>
}

export function acceptConversation(publicId: string) {
  return request.post(`/api/v1/im/agent/conversations/${publicId}/accept`) as Promise<ImConversation>
}

export function transferConversation(
  publicId: string,
  body: { target_agent_user_id?: number; skill_group_id?: number; note?: string },
) {
  return request.post(`/api/v1/im/agent/conversations/${publicId}/transfer`, body) as Promise<ImConversation>
}

export function closeConversation(publicId: string, reason?: string) {
  return request.post(`/api/v1/im/agent/conversations/${publicId}/close`, {
    reason: reason || 'agent',
  }) as Promise<ImConversation>
}

// 坐席标记已读；seq 省略则读到最新
export function markAgentRead(publicId: string, seq?: number) {
  return request.post(`/api/v1/im/agent/conversations/${publicId}/read`, {
    seq: seq || 0,
  }) as Promise<{ reader: string; seq: number }>
}

export function summarizeConversation(publicId: string) {
  return request.post(`/api/v1/im/agent/conversations/${publicId}/summary`) as Promise<{
    summary: string
    conversation: ImConversation
  }>
}

export function transferHuman(publicId: string, reason?: string) {
  return request.post(`/api/v1/im/conversations/${publicId}/transfer_human`, {
    reason: reason || 'visitor',
  }) as Promise<ImConversation>
}

export function listMessages(publicId: string, afterSeq = 0) {
  return request.get(`/api/v1/im/conversations/${publicId}/messages`, {
    params: { after_seq: afterSeq },
  }) as Promise<{ messages: ImMessage[]; conversation: ImConversation }>
}

export function sendAgentMessage(publicId: string, text: string, clientMsgId: string) {
  return request.post(`/api/v1/im/conversations/${publicId}/messages`, {
    client_msg_id: clientMsgId,
    msg_type: 'text',
    content: { text },
  }) as Promise<ImMessage>
}

export interface ImAttachment {
  url: string
  name: string
  size: number
  mime: string
  msg_type: 'image' | 'file'
}

// 附件先上传拿 URL，再作为 image/file 消息发送
export function uploadImAttachment(file: File) {
  const fd = new FormData()
  fd.append('file', file)
  return request.post('/api/v1/im/attachments', fd, {
    headers: { 'Content-Type': 'multipart/form-data' },
  }) as Promise<ImAttachment>
}

export function sendAgentAttachment(publicId: string, att: ImAttachment, clientMsgId: string) {
  return request.post(`/api/v1/im/conversations/${publicId}/messages`, {
    client_msg_id: clientMsgId,
    msg_type: att.msg_type,
    content: { url: att.url, name: att.name, size: att.size, mime: att.mime },
  }) as Promise<ImMessage>
}

// skill groups admin
export function listSkillGroups() {
  return request.get('/api/v1/im/admin/skill-groups') as Promise<{ list: ImSkillGroup[] }>
}

export function createSkillGroup(body: { name: string; code: string; strategy?: string; status?: number }) {
  return request.post('/api/v1/im/admin/skill-groups', body) as Promise<ImSkillGroup>
}

export function updateSkillGroup(
  id: number,
  body: { name?: string; code?: string; strategy?: string; status?: number },
) {
  return request.put(`/api/v1/im/admin/skill-groups/${id}`, body) as Promise<ImSkillGroup>
}

export function listSkillAgents(skillGroupId: number) {
  return request.get(`/api/v1/im/admin/skill-groups/${skillGroupId}/agents`) as Promise<{ list: ImAgentSkill[] }>
}

export function upsertAgentSkill(body: {
  agent_user_id: number
  skill_group_id: number
  max_concurrent?: number
  status?: number
}) {
  return request.post('/api/v1/im/admin/agent-skills', body) as Promise<ImAgentSkill>
}

export function deleteAgentSkill(id: number) {
  return request.delete(`/api/v1/im/admin/agent-skills/${id}`) as Promise<{ deleted: number }>
}

// ---------- M5: 快捷回复 / 访客信息卡 ----------

export type ImQuickReply = {
  id: number
  title: string
  content: string
  sort: number
  created_by: number
}

export function listQuickReplies() {
  return request.get('/api/v1/im/agent/quick-replies') as Promise<{ list: ImQuickReply[] }>
}

export function createQuickReply(body: { title: string; content: string }) {
  return request.post('/api/v1/im/agent/quick-replies', body) as Promise<ImQuickReply>
}

export function deleteQuickReply(id: number) {
  return request.delete(`/api/v1/im/agent/quick-replies/${id}`) as Promise<{ deleted: boolean }>
}

export type ImVisitorCard = {
  visitor: {
    id: number
    display_name: string
    guest_key: string
    phone?: string
    email?: string
    wechat?: string
    note?: string
    meta?: string
    banned?: boolean
    last_seen_at: string
    created_at: string
  }
  conversation_count: number
  context?: ImVisitorContext
  site_name?: string
  channel?: string
}

export function getConversationVisitor(publicId: string) {
  return request.get(`/api/v1/im/agent/conversations/${publicId}/visitor`) as Promise<ImVisitorCard>
}

export function setVisitorNote(publicId: string, note: string) {
  return request.put(`/api/v1/im/agent/conversations/${publicId}/visitor-note`, { note }) as Promise<{ saved: boolean }>
}

export function setVisitorContact(
  publicId: string,
  body: {
    display_name?: string
    phone?: string
    email?: string
    wechat?: string
    clear_phone?: boolean
    clear_email?: boolean
    clear_wechat?: boolean
  },
) {
  return request.put(`/api/v1/im/agent/conversations/${publicId}/visitor-contact`, body) as Promise<{
    visitor: ImVisitorCard['visitor']
  }>
}

// ---------- M6: 统计 / 拉黑 / 撤回 ----------

export type ImAgentStat = {
  agent_user_id: number
  display_name: string
  closed: number
  rated: number
  avg_rating?: number
}

export type ImStats = {
  total_open: number
  total_queued: number
  closed_in_range: number
  rated_in_range: number
  avg_rating?: number
  score_dist: Record<string, number>
  agents: ImAgentStat[]
}

export function getImStats(days = 7) {
  return request.get('/api/v1/im/agent/stats', { params: { days } }) as Promise<{ stats: ImStats; days: number }>
}

/** 低分会话（CSAT ≤ max_score）质检列表 */
export type ImLowRatedRow = ImConversation & {
  agent_display_name?: string
}

export function listLowRatings(params?: {
  max_score?: number
  days?: number
  agent_user_id?: number
  page?: number
  page_size?: number
}) {
  return request.get('/api/v1/im/agent/low-ratings', { params }) as Promise<{
    list: ImLowRatedRow[]
    total: number
    max_score: number
    days: number
  }>
}

export function setVisitorBan(publicId: string, banned: boolean) {
  return request.put(`/api/v1/im/agent/conversations/${publicId}/visitor-ban`, { banned }) as Promise<{ banned: boolean }>
}

export function recallMessage(publicId: string, messageId: number) {
  return request.post(`/api/v1/im/conversations/${publicId}/messages/${messageId}/recall`) as Promise<ImMessage>
}

// ---------- M7: 访客历史 / 消息搜索 ----------

export function getVisitorHistory(publicId: string, limit = 10) {
  return request.get(`/api/v1/im/agent/conversations/${publicId}/visitor-history`, {
    params: { limit },
  }) as Promise<{ list: ImConversation[]; visitor_id: number }>
}

export type ImMessageHit = ImMessage & {
  conversation_public_id: string
  conversation_status: string
}

export function searchImMessages(keyword: string, limit = 30) {
  return request.get('/api/v1/im/agent/messages/search', {
    params: { keyword, limit },
  }) as Promise<{ list: ImMessageHit[]; keyword: string }>
}

// ---------- P4: 按访客拉会话（CRM IM 对话历史） ----------

export function listVisitorConversations(visitorId: number, limit = 20) {
  return request.get(`/api/v1/im/agent/visitors/${visitorId}/conversations`, {
    params: { limit },
  }) as Promise<{ list: ImConversation[]; visitor: { id: number; display_name: string; guest_key: string } }>
}

// ---------- 敏感词管理（控制台词库，热生效） ----------

export type ImSensitiveWord = {
  id: number
  word: string
  action: 'mask' | 'block'
  tag?: string
  status: number
  created_by?: number
  created_at?: string
  updated_at?: string
}

export type ImSensitiveHit = {
  word: string
  action: 'mask' | 'block'
  start: number
  end: number
}

export function listSensitiveWords(params: {
  keyword?: string
  action?: string
  status?: number
  tag?: string
  page?: number
  page_size?: number
}) {
  return request.get('/api/v1/im/admin/sensitive-words', { params }) as Promise<{
    list: ImSensitiveWord[]
    total: number
    page: number
    page_size: number
  }>
}

// 批量导入：words 多行拆词；已存在的词自动跳过
export function createSensitiveWords(body: { words: string[]; action?: 'mask' | 'block'; tag?: string }) {
  return request.post('/api/v1/im/admin/sensitive-words', body) as Promise<{ created: number; skipped: number }>
}

export function updateSensitiveWord(
  id: number,
  body: { word?: string; action?: 'mask' | 'block'; tag?: string; status?: number },
) {
  return request.put(`/api/v1/im/admin/sensitive-words/${id}`, body) as Promise<ImSensitiveWord>
}

export function deleteSensitiveWord(id: number) {
  return request.delete(`/api/v1/im/admin/sensitive-words/${id}`) as Promise<{ deleted: boolean }>
}

export function testSensitiveText(text: string) {
  return request.post('/api/v1/im/admin/sensitive-words/test', { text }) as Promise<{
    masked_text: string
    blocked: boolean
    hits: ImSensitiveHit[] | null
  }>
}
