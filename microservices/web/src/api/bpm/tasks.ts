import request from '@/utils/request'
import type { PageRequest, PageResponse } from '@/types'
import type { BpmInstanceStatus, BpmTask, BpmTaskDetail } from './types'

// API 封装 —— §4.3 任务端（审批人视角；M1 动作：同意/拒绝）
// ---------------------------------------------------------------------

export type BpmTaskListParams = PageRequest & { keyword?: string }

/** 我的待办（silent 供业务页探测 BPM 可用性复用） */
export const listTodoTasks = (params: BpmTaskListParams, silent = false) =>
  request.get<unknown, PageResponse<BpmTask>>('/api/v1/bpm/tasks/todo', { params, silent })

/** 我的已办 */
export const listDoneTasks = (params: BpmTaskListParams) =>
  request.get<unknown, PageResponse<BpmTask>>('/api/v1/bpm/tasks/done', { params })

/** 任务详情（含实例摘要 + form_snapshot + 我可用的动作列表）；silent 供列表批量预取动作用 */
export const getTask = (id: number, silent = false) =>
  request.get<unknown, BpmTaskDetail>(`/api/v1/bpm/tasks/${id}`, { silent })

/** 审批动作的返回体（后端确认形态） */
export interface BpmTaskActionResult {
  task_id: number
  instance_id: number
  /** 动作落库后的实例状态（据此可即时提示“流程已通过/已拒绝”） */
  instance_status: BpmInstanceStatus | string
}

/** 同意（意见可选） */
export const approveTask = (id: number, comment?: string) =>
  request.post<unknown, BpmTaskActionResult>(`/api/v1/bpm/tasks/${id}/approve`, {
    comment: comment || undefined,
  })

/** 拒绝（意见必填，前端强制 §3.3） */
export const rejectTask = (id: number, comment: string) =>
  request.post<unknown, BpmTaskActionResult>(`/api/v1/bpm/tasks/${id}/reject`, { comment })

/** 转办（M2）：任务换人保持 pending，不改变计数规则 */
export const transferTask = (id: number, targetUserId: number, comment?: string) =>
  request.post<unknown, BpmTaskActionResult>(`/api/v1/bpm/tasks/${id}/transfer`, {
    target_user_id: targetUserId,
    comment: comment || undefined,
  })

/** 加签（M3+）：往当前节点同轮次增加审批人，沿用节点多人模式参与收敛（SEQ 不支持） */
export const addSignTask = (id: number, userIds: number[], comment?: string) =>
  request.post<unknown, BpmTaskActionResult>(`/api/v1/bpm/tasks/${id}/add-sign`, {
    user_ids: userIds,
    comment: comment || undefined,
  })

/** 委派（M3+）：任务交受托人办理，办结后回到委派人再做决定；不改变计数规则 */
export const delegateTask = (id: number, targetUserId: number, comment?: string) =>
  request.post<unknown, BpmTaskActionResult>(`/api/v1/bpm/tasks/${id}/delegate`, {
    target_user_id: targetUserId,
    comment: comment || undefined,
  })

/** 委派办结（M3+）：受托人填写办理意见后任务回到委派人；意见必填 */
export const resolveDelegateTask = (id: number, comment: string) =>
  request.post<unknown, BpmTaskActionResult>(`/api/v1/bpm/tasks/${id}/delegate/resolve`, {
    comment,
  })

/** 退回（M2）：to=start 退回发起人 / to=prev 退回上一节点（须动作列表含 return_prev）；意见必填 */
export const returnTask = (id: number, to: 'start' | 'prev', comment: string) =>
  request.post<unknown, BpmTaskActionResult>(`/api/v1/bpm/tasks/${id}/return`, { to, comment })

// ---------------------------------------------------------------------
