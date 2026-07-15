import request from '@/utils/request'

export type ImConversation = {
  id: number
  public_id: string
  status: string
  visitor_id: number
  agent_user_id?: number
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

export function listAgentConversations() {
  return request.get('/api/v1/im/agent/conversations') as Promise<{ list: ImConversation[] }>
}

export function acceptConversation(publicId: string) {
  return request.post(`/api/v1/im/agent/conversations/${publicId}/accept`) as Promise<ImConversation>
}

export function closeConversation(publicId: string) {
  return request.post(`/api/v1/im/agent/conversations/${publicId}/close`) as Promise<ImConversation>
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
