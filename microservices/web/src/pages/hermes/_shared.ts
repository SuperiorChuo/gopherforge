// Hermes 集群各页共用的常量与状态映射。原先散在单页 4-Tab 组件里，拆页后抽出，
// 保证设备列表、总览与 3D 地图对同一状态的着色/措辞完全一致。
import type { StatusTone } from '@/components/common/StatusPill'
import type { HermesHealthStatus } from '@/api/ai/hermes'

/** 各页列表统一的拉取参数（保持 100：entitlements 等列表带 JOIN，别全局放大）。 */
export const pageParams = { page: 1, page_size: 100 }

/** 设备类列表专用：hermes_devices 单表无 JOIN，允许整页拉全量后客户端分页/统计。
 *  与后端 maxHermesPageSize=1000 匹配。 */
export const devicePageParams = { page: 1, page_size: 1000 }

/** Hermes 设备健康态 → 中文文案；bad 标记异常态（标红）。3D 办公室与运行节点/设备页共用。 */
export const HEALTH_LABELS: Record<HermesHealthStatus, { label: string; bad?: boolean }> = {
  unknown: { label: '未知' },
  healthy: { label: '健康' },
  degraded: { label: '降级' },
  unhealthy: { label: '不健康', bad: true },
  policy_drift: { label: '策略漂移', bad: true },
}

/** P1 固定的工具白名单，与后端 service 侧校验保持一致 */
export const p1Toolsets = ['clarify', 'memory', 'no_mcp', 'todo']

/** company_cluster 策略允许的工具集（dispatcher 开放）。与后端
 *  hermesClusterToolsets 保持一致。company_cluster 本期不可应用到真实设备。 */
export const p2ClusterToolsets = ['clarify', 'memory', 'no_mcp', 'todo', 'delegation', 'skills', 'mcp']

/** 任务状态标签与色调（第二期）。 */
export const taskStateLabels: Record<string, string> = {
  pending: '待激活',
  active: '执行中',
  completed: '已完成',
  cancelled: '已取消',
}

export const taskRunStateLabels: Record<string, string> = {
  assigned: '已指派',
  running: '执行中',
  completed: '已完成',
  failed: '失败',
}

export function taskStateTone(state: string): StatusTone {
  if (state === 'completed') return 'success'
  if (state === 'cancelled' || state === 'failed') return 'danger'
  if (state === 'active' || state === 'running') return 'info'
  return 'muted'
}

export const stateLabels: Record<string, string> = {
  registered: '已注册',
  installing: '安装中',
  active: '运行中',
  stopped: '已停止',
  updating: '更新中',
  revoked: '已吊销',
  error: '异常',
}

export function stateTone(state: string): StatusTone {
  if (state === 'active' || state === 'healthy' || state === 'completed') return 'success'
  if (state === 'error' || state === 'unhealthy' || state === 'revoked' || state === 'failed') return 'danger'
  if (state === 'degraded' || state === 'policy_drift' || state === 'updating') return 'warning'
  return 'muted'
}
