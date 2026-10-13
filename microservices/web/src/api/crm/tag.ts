import request from '@/utils/request'

export interface CrmTag {
  id?: number
  tenant_id?: number
  name: string
  color: string
}

export const listTags = () => request.get<unknown, CrmTag[]>('/api/v1/crm/tags')
export const createTag = (data: CrmTag) => request.post<unknown, CrmTag>('/api/v1/crm/tags', data)
export const updateTag = (id: number, data: CrmTag) => request.put<unknown, CrmTag>(`/api/v1/crm/tags/${id}`, data)
export const deleteTag = (id: number) => request.delete(`/api/v1/crm/tags/${id}`)
