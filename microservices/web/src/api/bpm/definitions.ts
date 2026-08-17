import request from '@/utils/request'
import type { PageRequest, PageResponse } from '@/types'
import type { BpmDefinition, BpmFormSchema, FlowSchema } from './types'

// API 封装 —— §4.1 管理端（流程定义，权限：bpm:definition:*）
// ---------------------------------------------------------------------

export type BpmDefinitionListParams = PageRequest & {
  keyword?: string
  biz_type?: string
}

export interface BpmDefinitionCreateData {
  key: string
  name: string
  biz_type?: string
  node_tree: FlowSchema
  /** 流程表单模式的表单 Schema（null=业务表单） */
  form_schema?: BpmFormSchema | null
  remark?: string
}

export type BpmDefinitionUpdateData = Partial<Omit<BpmDefinitionCreateData, 'key'>>

/** 定义列表（按 key 聚合显示最新版本，含 active 版本号） */
export const listDefinitions = (params: BpmDefinitionListParams) =>
  request.get<unknown, PageResponse<BpmDefinition>>('/api/v1/bpm/definitions', { params })

/** 新建定义 → version=1, status=draft */
export const createDefinition = (data: BpmDefinitionCreateData) =>
  request.post<unknown, BpmDefinition>('/api/v1/bpm/definitions', data)

/** 定义详情（含 node_tree） */
export const getDefinition = (id: number) =>
  request.get<unknown, BpmDefinition>(`/api/v1/bpm/definitions/${id}`)

/** 修改 draft 版本（active 版本不可改，需另存新版本） */
export const updateDefinition = (id: number, data: BpmDefinitionUpdateData) =>
  request.put<unknown, BpmDefinition>(`/api/v1/bpm/definitions/${id}`, data)

/** 发布：后端 Schema 校验 → 该版本 active，同 key 旧 active → archived */
export const publishDefinition = (id: number) =>
  request.post<unknown, BpmDefinition>(`/api/v1/bpm/definitions/${id}/publish`)

/** 以某版本为底复制出新 draft 版本（version=max+1） */
export const newDefinitionVersion = (id: number) =>
  request.post<unknown, BpmDefinition>(`/api/v1/bpm/definitions/${id}/new-version`)

/** 停用（不再允许新发起，在途实例不受影响） */
export const suspendDefinition = (id: number) =>
  request.post<unknown, BpmDefinition>(`/api/v1/bpm/definitions/${id}/suspend`)

/** 按 key 取当前 active 版本（发起端/业务端用） */
export const getActiveDefinitionByKey = (key: string) =>
  request.get<unknown, BpmDefinition>(`/api/v1/bpm/definitions/keys/${encodeURIComponent(key)}/active`)

// ---------------------------------------------------------------------
