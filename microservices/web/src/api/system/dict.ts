import request from '@/utils/request'
import type { PageRequest, PageResponse, DictType, DictItem } from '@/types'

type DictTypeListParams = PageRequest & { keyword?: string; status?: number }
type DictTypeCreateData = Omit<DictType, 'id' | 'created_at'>
type DictTypeUpdateData = Partial<DictTypeCreateData>

type DictItemListParams = PageRequest & { keyword?: string; status?: number }
type DictItemCreateData = Omit<DictItem, 'id' | 'created_at'>
type DictItemUpdateData = Partial<DictItemCreateData>

export const getDictTypeList = (params: DictTypeListParams) =>
  request.get<unknown, PageResponse<DictType>>('/api/v1/dict-types', { params })

export const createDictType = (data: DictTypeCreateData) =>
  request.post<unknown, DictType>('/api/v1/dict-types', data)

export const updateDictType = (id: number, data: DictTypeUpdateData) =>
  request.put<unknown, DictType>(`/api/v1/dict-types/${id}`, data)

export const deleteDictType = (id: number) =>
  request.delete<unknown, void>(`/api/v1/dict-types/${id}`)

// 后端分页的字典项列表在 /dict-items 上，用 type_id 过滤
export const getDictItemList = (typeId: number, params: DictItemListParams) =>
  request.get<unknown, PageResponse<DictItem>>('/api/v1/dict-items', {
    params: { ...params, type_id: typeId },
  })

export const createDictItem = (data: DictItemCreateData) =>
  request.post<unknown, DictItem>('/api/v1/dict-items', data)

export const updateDictItem = (id: number, data: DictItemUpdateData) =>
  request.put<unknown, DictItem>(`/api/v1/dict-items/${id}`, data)

export const deleteDictItem = (id: number) =>
  request.delete<unknown, void>(`/api/v1/dict-items/${id}`)
