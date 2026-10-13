// Agent Kernel 管理页共用的状态映射，与后端 agentcore 状态机措辞一致。
import type { StatusTone } from '@/components/common/StatusPill'
import type { AgentDefinitionStatus, AgentRunStatus, AgentStepStatus } from '@/api/ai/agent'

export const defStatusMeta: Record<AgentDefinitionStatus, { label: string; tone: StatusTone }> = {
  active: { label: '启用', tone: 'success' },
  disabled: { label: '禁用', tone: 'muted' },
}

export const runStatusMeta: Record<AgentRunStatus, { label: string; tone: StatusTone }> = {
  queued: { label: '排队中', tone: 'warning' },
  running: { label: '运行中', tone: 'info' },
  cancelling: { label: '取消中', tone: 'warning' },
  succeeded: { label: '已成功', tone: 'success' },
  failed: { label: '已失败', tone: 'danger' },
  cancelled: { label: '已取消', tone: 'muted' },
}

export const stepStatusMeta: Record<AgentStepStatus, { label: string; tone: StatusTone }> = {
  running: { label: '进行中', tone: 'info' },
  succeeded: { label: '成功', tone: 'success' },
  failed: { label: '失败', tone: 'danger' },
  cancelled: { label: '已取消', tone: 'muted' },
}

export function runStatusTone(status: string): StatusTone {
  return runStatusMeta[status as AgentRunStatus]?.tone ?? 'muted'
}

/** 排队/运行中才允许取消，与后端 CancelRun 的状态校验一致。 */
export function canCancelRun(status: string): boolean {
  return status === 'queued' || status === 'running'
}
