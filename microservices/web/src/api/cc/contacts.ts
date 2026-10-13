import request from '@/utils/request'

// ---- 来显归属：联系人库（号码 → 客户名，来电弹屏/话机显名）----

export type CcContact = {
  id: number
  tenant_id: number
  number: string
  name: string
  remark: string
  created_at: string
}

// 话单页"备注姓名"按号码 upsert（下次该号来电即显示名字）
export function upsertCcContact(data: { number: string; name: string; remark?: string }) {
  return request.post('/api/v1/cc/contacts', data) as Promise<CcContact>
}

export function listCcContacts(params: { q?: string; page?: number; page_size?: number }) {
  return request.get('/api/v1/cc/contacts', { params }) as Promise<{ list: CcContact[]; total: number }>
}

export function deleteCcContact(number: string) {
  return request.delete(`/api/v1/cc/contacts/${encodeURIComponent(number)}`) as Promise<{ deleted: boolean }>
}
