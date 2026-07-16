import request from '@/utils/request'

export type ImConversation = {
  id: number
  public_id: string
  status: string
  visitor_id: number
  agent_user_id?: number
  skill_group_id?: number
  close_reason?: string
  summary?: string
  last_message_preview?: string
  last_message_at?: string
  created_at: string
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

export function listAgentConversations(scope: 'all' | 'mine' | 'queue' | 'bot' = 'all', skillGroupId?: number) {
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
