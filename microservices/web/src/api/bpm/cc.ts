import request from '@/utils/request'
import type { PageRequest, PageResponse } from '@/types'
import type { BpmCcRecord } from './types'

// API 封装 —— §4.3 抄送（M2）
// ---------------------------------------------------------------------

export type BpmCcListParams = PageRequest & { unread_only?: boolean }

/** 抄送我的列表；unread_only=true 仅未读（page_size=1 可作未读计数探针） */
export const listMyCc = (params: BpmCcListParams, silent = false) =>
  request.get<unknown, PageResponse<BpmCcRecord>>('/api/v1/bpm/cc/my', { params, silent })

/** 标记抄送已读（幂等） */
export const readCcRecord = (id: number, silent = false) =>
  request.post<unknown, void>(`/api/v1/bpm/cc/${id}/read`, undefined, { silent })
