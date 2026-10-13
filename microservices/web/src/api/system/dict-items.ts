import request from '@/utils/request'

export interface DictItem {
  id: number
  dict_type_id: number
  label: string
  value: string
  sort: number
  status: number
  remark?: string
  created_at: string
  updated_at: string
}

/**
 * 根据字典类型 code 获取字典项列表
 */
export function getDictItems(code: string): Promise<DictItem[]> {
  return request.get(`/api/v1/dicts/${code}`) as Promise<DictItem[]>
}

/**
 * 批量获取多个字典类型的数据
 */
export function getMultipleDictData(codes: string[]): Promise<Record<string, DictItem[]>> {
  return request.get('/api/v1/dicts', {
    params: { codes: codes.join(',') },
  }) as Promise<Record<string, DictItem[]>>
}
