import request from '@/utils/request'
import type { PageRequest, PageResponse } from '@/types'
import type {
  BpmDiagram,
  BpmDefinition,
  BpmInstance,
  BpmTimelineItem,
} from './types'

// ---------------------------------------------------------------------
// API 封装 —— §4.2 发起端 + §4.4 实例端
// 注：POST /api/v1/bpm/instances 的 CRM 发起走业务后端 internal 变体
//（表单快照由业务后端权威生成），前端不封装裸发起接口。
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

// 注：bpm 的 by-biz 反查仅有 internal 变体（/api/v1/bpm/internal/instances/by-biz，
// X-Internal-Token，前端不可用）。业务侧反查走各业务后端的代理端点，
// 例如 CRM 合同：GET /api/v1/crm/contracts/:id/approval（见 findContractApprovalInstance）。

/**
 * CRM 合同审批实例反查：走 CRM 后端代理（其经 internal by-biz 向 bpm 查询）。
 * 始终 silent；返回形态宽松归一（单实例 / {instance} / {running,history} / {list} / 数组，
 * 在途优先，其次最近一条历史）。失败或无实例返回 null，调用方据此优雅降级隐藏。
 */
export async function findContractApprovalInstance(
  contractId: string | number,
): Promise<BpmInstance | null> {
  const data = await request.get<unknown, unknown>(
    `/api/v1/crm/contracts/${contractId}/approval`,
    { silent: true },
  )
  if (!data || typeof data !== 'object') return null
  const obj = data as {
    id?: unknown
    instance?: BpmInstance | null
    running?: BpmInstance | null
    history?: BpmInstance[]
    list?: BpmInstance[]
  }
  if (typeof obj.id === 'number') return data as BpmInstance
  if (obj.instance && typeof obj.instance === 'object') return obj.instance
  if (obj.running && typeof obj.running === 'object') return obj.running
  const arr = Array.isArray(data) ? (data as BpmInstance[]) : (obj.history ?? obj.list ?? [])
  if (!Array.isArray(arr) || !arr.length) return null
  return arr.find((it) => it.status === 'running') ?? arr[0]
}
