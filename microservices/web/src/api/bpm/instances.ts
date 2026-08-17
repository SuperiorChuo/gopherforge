import request from '@/utils/request'
import type { PageRequest, PageResponse } from '@/types'
import type {
  BpmDiagram,
  BpmDefinition,
  BpmInstance,
  BpmTimelineItem,
} from './types'

// API 封装 —— §4.2 发起端 + §4.4 实例端
// 注：POST /api/v1/bpm/instances 仅接受流程表单定义（表单构建器 M1）；
// 业务表单走业务后端 internal 变体（表单快照由业务后端权威生成）。
// ---------------------------------------------------------------------

export type BpmInstanceListParams = PageRequest & { status?: string }

/** 可发起流程（active 且携带表单 Schema 的"流程表单"定义；登录即可） */
export const listStartableDefinitions = () =>
  request
    .get<unknown, { list?: BpmDefinition[] }>('/api/v1/bpm/startable')
    .then((d) => d?.list ?? [])

/** 通用发起（表单构建器 M1）：仅流程表单定义；biz 锚点由服务端生成 */
export const startFormInstance = (
  definitionKey: string,
  formSnapshot: Record<string, unknown>,
  title?: string,
) =>
  request.post<unknown, { instance_id: number; status: string }>('/api/v1/bpm/instances', {
    definition_key: definitionKey,
    form_snapshot: formSnapshot,
    title: title || undefined,
  })

/** 我发起的 */
export const listMyInstances = (params: BpmInstanceListParams) =>
  request.get<unknown, PageResponse<BpmInstance>>('/api/v1/bpm/instances/my', { params })

/** 撤销（仅发起人，且首个审批节点尚无人审过 §3.3） */
export const cancelInstance = (id: number) =>
  request.post<unknown, void>(`/api/v1/bpm/instances/${id}/cancel`)

/** 管理员终止（M3）：仅平台管理员；running/suspended 可终止，原因必填 */
export const terminateInstance = (id: number, comment: string) =>
  request.post<unknown, void>(`/api/v1/bpm/instances/${id}/terminate`, { comment })

// ---------------------------------------------------------------------
// 审批统计（收官项，仅平台管理员）
// ---------------------------------------------------------------------

export interface BpmStatsTrendItem {
  date: string
  count: number
}

export interface BpmDefStatsItem {
  definition_key: string
  name?: string
  total: number
  approved: number
  rejected: number
  running: number
  avg_hours: number
}

export interface BpmNodeStatsItem {
  node_name: string
  acted: number
  avg_hours: number
}

export interface BpmStats {
  status_counts: Record<string, number>
  trend: BpmStatsTrendItem[]
  definitions: BpmDefStatsItem[]
  node_bottlenecks: BpmNodeStatsItem[]
}

/** 审批统计（状态分布/30 天趋势/按定义通过率与均时长/节点瓶颈） */
export const getBpmStats = () => request.get<unknown, BpmStats>('/api/v1/bpm/stats')

/** 全部实例（M3 管理视图）：仅平台管理员可见（后端 403 拦非管理员） */
export const listAllInstances = (params: BpmInstanceListParams & { keyword?: string }) =>
  request.get<unknown, PageResponse<BpmInstance>>('/api/v1/bpm/instances', { params })

/** 被退回后修改快照重新提交（M2）：全链路 round+1 重新展开；form_snapshot 缺省=按原快照重提 */
export const resubmitInstance = (id: number, formSnapshot?: Record<string, unknown>) =>
  request.post<unknown, BpmInstance | void>(`/api/v1/bpm/instances/${id}/resubmit`, {
    form_snapshot: formSnapshot,
  })

/** 实例详情：基本信息 + form_snapshot + 当前节点 */
export const getInstance = (id: number, silent = false) =>
  request.get<unknown, BpmInstance>(`/api/v1/bpm/instances/${id}`, { silent })

/** 时间线：流转日志按时间正序；后端返回 {list}（兼容裸数组防御） */
export const getInstanceTimeline = (id: number, silent = false) =>
  request
    .get<unknown, BpmTimelineItem[] | { list?: BpmTimelineItem[] } | null>(
      `/api/v1/bpm/instances/${id}/timeline`,
      { silent },
    )
    .then((d) => (Array.isArray(d) ? d : (d?.list ?? [])))

/** 流转图数据：node_tree + 节点运行时标注 */
export const getInstanceDiagram = (id: number, silent = false) =>
  request.get<unknown, BpmDiagram>(`/api/v1/bpm/instances/${id}/diagram`, { silent })


// ---------------------------------------------------------------------
