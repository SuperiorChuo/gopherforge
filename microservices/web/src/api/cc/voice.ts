import request from '@/utils/request'

export type CcVoiceNotify = {
  id: number
  tenant_id: number
  to: string
  text: string
  gateway?: string
  prompt_file?: string
  status: 'pending' | 'running' | 'queued' | 'answered' | 'completed' | 'noanswer' | 'denied' | 'failed'
  error?: string
  reply?: string
  call_id?: string
  hangup_class?: string
  created_at: string
  updated_at: string
  finished_at?: string
}

export function listCcVoiceNotifies(params?: { page?: number; page_size?: number }) {
  return request.get('/api/v1/cc/voice', { params }) as Promise<{ list: CcVoiceNotify[]; total: number }>
}

export function createCcVoiceNotify(payload: { to: string; text: string; gateway?: string }) {
  return request.post('/api/v1/cc/voice', payload) as Promise<CcVoiceNotify>
}
