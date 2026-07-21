import request from '@/utils/request'
import type { PageRequest, PageResponse } from '@/types'

// 错误码管理：错误码 → 对外文案映射，控制台在线修改，
// 各服务经 30s TTL 缓存读取，约 30 秒内热生效（无需重启）。
export interface ErrorCodeItem {
  id: number
  /** 错误码标识，与后端 response.ErrorCode 常量对齐（创建后不可改） */
  code: string
  /** 对外展示文案（命中时覆盖代码默认文案） */
  message: string
  /** 内部备注（不对外返回） */
  memo: string
  /** 来源服务/模块（如 system / auth / global） */
  scope: string
  status: number
  created_at?: string
  updated_at?: string
}

export type ErrorCodeListParams = PageRequest & {
  keyword?: string
  scope?: string
  status?: number
}

export type ErrorCodeCreateData = Omit<ErrorCodeItem, 'id' | 'created_at' | 'updated_at'>
// code 是稳定标识，更新时不允许修改
export type ErrorCodeUpdateData = Partial<Omit<ErrorCodeCreateData, 'code'>>

export const getErrCodeList = (params: ErrorCodeListParams) =>
  request.get<unknown, PageResponse<ErrorCodeItem>>('/api/v1/error-codes', { params })

/** 全量启用错误码（供前端/其他消费方整包拉取做本地缓存） */
export const getAllEnabledErrCodes = () =>
  request.get<unknown, ErrorCodeItem[]>('/api/v1/error-codes/all')

export const getErrCode = (id: number) =>
  request.get<unknown, ErrorCodeItem>(`/api/v1/error-codes/${id}`)

export const createErrCode = (data: ErrorCodeCreateData) =>
  request.post<unknown, ErrorCodeItem>('/api/v1/error-codes', data)

export const updateErrCode = (id: number, data: ErrorCodeUpdateData) =>
  request.put<unknown, ErrorCodeItem>(`/api/v1/error-codes/${id}`, data)

export const deleteErrCode = (id: number) =>
  request.delete<unknown, void>(`/api/v1/error-codes/${id}`)
