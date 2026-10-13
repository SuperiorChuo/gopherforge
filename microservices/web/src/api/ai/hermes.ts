import request from '@/utils/request'
import type { PageResponse } from '@/types'

export type HermesDesiredState = 'active' | 'stopped' | 'revoked'
export type HermesActualState = 'registered' | 'installing' | 'active' | 'stopped' | 'updating' | 'revoked' | 'error'
export type HermesHealthStatus = 'unknown' | 'healthy' | 'degraded' | 'unhealthy' | 'policy_drift'

export interface HermesEntitlement {
  id: number
  tenant_id: number
  user_id: number
  username?: string
  nickname?: string
  enabled: boolean
  policy_id: number
  device_limit: number
  active_device_count?: number
  last_seen_at?: string
  created_at: string
  updated_at: string
}

export interface HermesDevice {
  id: number
  tenant_id: number
  device_id: string
  user_id: number
  key_fingerprint: string
  device_name: string
  platform: 'windows' | 'darwin' | 'linux'
  arch: 'amd64' | 'arm64'
  manager_version: string
  hermes_version: string
  actual_state: HermesActualState
  health_status: HermesHealthStatus
  desired_state: HermesDesiredState | 'unknown'
  connection_status: 'never' | 'online' | 'offline'
  applied_policy_revision: number
  config_digest: string
  last_error_code: string
  /** 已排入、等待设备下次同步时执行的运维动作；空串表示无 */
  pending_action: HermesDeviceAction | ''
  pending_action_at?: string
  last_seen_at?: string
  revoked_at?: string
  created_at: string
  updated_at: string
  /** 能力预览（M6）：这台 Hermes 会什么技能/连了什么工具。MCP 仅名字。 */
  capabilities?: HermesCapabilities
}

/** 控制面允许下发的动作全集，与后端 HermesDeviceActions 及 DB CHECK 约束一致。
 *  控制面不下发 shell 字符串或任意参数，只在这三者间取值。 */
export type HermesDeviceAction = 'retry_update' | 'apply_policy' | 'restart'

export interface HermesPrivacy {
  upload_conversations: boolean
  upload_memory: boolean
  upload_local_files: boolean
}

export type HermesPolicyProfile = 'company' | 'company_cluster'

export interface HermesPolicy {
  id: number
  tenant_id: number
  revision: number
  name: string
  desired_state: HermesDesiredState
  profile: HermesPolicyProfile
  assistant_template_id: string
  assistant_template_revision: number
  artifact_id: number
  model_id: number
  model_name: string
  model_base_url: string
  toolsets: string[]
  privacy: HermesPrivacy
  lease_seconds: number
  enable_knowledge_rag: boolean
  signature?: string
  created_at: string
}

export interface HermesArtifact {
  id: number
  tenant_id: number
  kind: 'manager' | 'hermes'
  platform: 'windows' | 'darwin' | 'linux' | 'any'
  arch: 'amd64' | 'arm64' | 'any'
  version: string
  upstream_commit: string
  source_url: string
  sha256: string
  signature?: string
  status: 'pending' | 'approved' | 'rejected' | 'revoked'
  approved_at?: string
  created_at: string
  updated_at: string
}

export interface HermesDeviceEvent {
  id: number
  event_type: string
  result: 'started' | 'succeeded' | 'failed'
  error_code: string
  desired_policy_revision: number
  applied_policy_revision: number
  occurred_at: string
}

export interface HermesMe {
  entitlement: HermesEntitlement | null
  devices: HermesDevice[]
  downloads: HermesArtifact[]
}

export const listHermesEntitlements = (params: { page: number; page_size: number }) =>
  request.get<unknown, PageResponse<HermesEntitlement>>('/api/v1/ai/hermes/entitlements', { params })

export const updateHermesEntitlement = (
  userId: number,
  data: { enabled: boolean; policy_id: number; device_limit: number },
) => request.put<unknown, void>(`/api/v1/ai/hermes/users/${userId}/entitlement`, data)

export const listHermesDevices = (params: { page: number; page_size: number; user_id?: number }) =>
  request.get<unknown, PageResponse<HermesDevice>>('/api/v1/ai/hermes/devices', { params })

export const getHermesDevice = (deviceId: string) =>
  request.get<unknown, { device: HermesDevice; events: HermesDeviceEvent[] }>(`/api/v1/ai/hermes/devices/${deviceId}`)

export const revokeHermesDevice = (deviceId: string) =>
  request.post<unknown, void>(`/api/v1/ai/hermes/devices/${deviceId}/revoke`)

/** 排一个运维动作；传空串撤销尚未执行的动作。 */
export const requestHermesDeviceAction = (deviceId: string, action: HermesDeviceAction | '') =>
  request.post<unknown, void>(`/api/v1/ai/hermes/devices/${deviceId}/actions`, { action })

// Agent 实例：逻辑体（角色、负责人、模型策略），与承载它的运行节点（设备）分开。
// device_id 为空 = 尚未放置到节点；policy_id 为 0 = 不覆盖，回落员工权益策略。
export type HermesInstanceStatus = 'unassigned' | 'pending' | 'running' | 'error'

export interface HermesInstance {
  id: number
  tenant_id: number
  name: string
  role: string
  owner_user_id: number
  device_id: string
  policy_id: number
  cluster_id: number
  enabled: boolean
  status: HermesInstanceStatus
  created_at: string
  updated_at: string
}

export interface HermesInstancePayload {
  name: string
  role: string
  owner_user_id: number
  device_id: string
  policy_id: number
  cluster_id: number
  enabled: boolean
}

export const listHermesInstances = (params: { page: number; page_size: number; device_id?: string }) =>
  request.get<unknown, PageResponse<HermesInstance>>('/api/v1/ai/hermes/instances', { params })

export const createHermesInstance = (data: HermesInstancePayload) =>
  request.post<unknown, HermesInstance>('/api/v1/ai/hermes/instances', data)

export const updateHermesInstance = (id: number, data: HermesInstancePayload) =>
  request.put<unknown, void>(`/api/v1/ai/hermes/instances/${id}`, data)

export const deleteHermesInstance = (id: number) =>
  request.delete<unknown, void>(`/api/v1/ai/hermes/instances/${id}`)

export const listHermesPolicies = (params: { page: number; page_size: number }) =>
  request.get<unknown, PageResponse<HermesPolicy>>('/api/v1/ai/hermes/policies', { params })

export interface HermesPolicyPayload {
  profile: HermesPolicyProfile
  name: string
  desired_state: HermesDesiredState
  assistant_template_id?: string
  assistant_template_revision?: number
  artifact_id: number
  model_id: number
  model_name: string
  model_base_url: string
  toolsets: string[]
  privacy: HermesPrivacy
  lease_seconds: number
  enable_knowledge_rag?: boolean
}

export const createHermesPolicy = (data: HermesPolicyPayload) =>
  request.post<unknown, HermesPolicy>('/api/v1/ai/hermes/policies', data)

export const updateHermesPolicy = (id: number, data: HermesPolicyPayload) =>
  request.put<unknown, HermesPolicy>(`/api/v1/ai/hermes/policies/${id}`, data)

export const deleteHermesPolicy = (id: number) =>
  request.delete<unknown, void>(`/api/v1/ai/hermes/policies/${id}`)

export const listHermesArtifacts = (params: { page: number; page_size: number }) =>
  request.get<unknown, PageResponse<HermesArtifact>>('/api/v1/ai/hermes/artifacts', { params })

export const approveHermesArtifact = (artifactId: number) =>
  request.post<unknown, void>(`/api/v1/ai/hermes/artifacts/${artifactId}/approve`)

export const rejectHermesArtifact = (artifactId: number) =>
  request.post<unknown, void>(`/api/v1/ai/hermes/artifacts/${artifactId}/reject`)

export const revokeHermesArtifact = (artifactId: number) =>
  request.post<unknown, void>(`/api/v1/ai/hermes/artifacts/${artifactId}/revoke`)

/** 制品录入。服务端强制：sha256 为 64 位小写十六进制、来源必须 https、
 *  版本不能是 latest/main 这类移动引用、kind=hermes 必须钉上游 commit。 */
export const createHermesArtifact = (data: {
  kind: 'manager' | 'hermes'
  platform: 'windows' | 'darwin' | 'linux' | 'any'
  arch: 'amd64' | 'arm64' | 'any'
  version: string
  upstream_commit?: string
  source_url: string
  sha256: string
  signature?: string
}) => request.post<unknown, HermesArtifact>('/api/v1/ai/hermes/artifacts', data)

export const getMyHermes = () => request.get<unknown, HermesMe>('/api/v1/ai/hermes/me')

export const approveMyHermesDevice = (userCode: string) =>
  request.post<unknown, void>('/api/v1/ai/hermes/device-authorizations/approve', { user_code: userCode })

export const revokeMyHermesDevice = (deviceId: string) =>
  request.post<unknown, void>(`/api/v1/ai/hermes/me/devices/${deviceId}/revoke`)

// ── 模型代理用量（只有元数据：次数/token/费用/耗时/错误码，无请求正文）──────

export interface HermesUsageSummary {
  total_requests: number
  failed_requests: number
  total_tokens: number
  total_cost: number
  avg_latency_ms: number
  active_devices: number
}

export interface HermesUsageByDevice {
  device_id: string
  user_id: number
  total_requests: number
  failed_requests: number
  total_tokens: number
  total_cost: number
  last_call_at?: string
}

export interface HermesUsageDaily {
  day: string
  total_requests: number
  failed_requests: number
  total_tokens: number
  total_cost: number
}

export interface HermesUsageOverview {
  days: number
  summary: HermesUsageSummary
  daily: HermesUsageDaily[]
  devices: HermesUsageByDevice[]
}

export interface HermesUsageFailure {
  model_name: string
  status: string
  error_msg: string
  latency_ms: number
  created_at: string
}

export interface HermesDeviceUsage {
  summary: HermesUsageSummary
  failures: HermesUsageFailure[]
}

export const getHermesUsageOverview = (params?: { days?: number; top?: number }) =>
  request.get<unknown, HermesUsageOverview>('/api/v1/ai/hermes/usage/overview', { params })

export const getHermesDeviceUsage = (deviceId: string, days = 7) =>
  request.get<unknown, HermesDeviceUsage>(`/api/v1/ai/hermes/usage/devices/${deviceId}`, {
    params: { days },
  })

// ── 多 Agent 协作任务（第二期）：集群 + 任务 + 执行记录，仅控制面编排 ──────

export interface HermesCluster {
  id: number
  tenant_id: number
  name: string
  slug: string
  description: string
  policy_id: number
  enabled: boolean
  instance_count?: number
  created_at: string
  updated_at: string
}

export interface HermesClusterPayload {
  name: string
  description: string
  policy_id: number
  slug?: string
  enabled: boolean
}

export type HermesTaskStatus = 'pending' | 'active' | 'completed' | 'cancelled'
export type HermesTaskRunStatus = 'assigned' | 'running' | 'completed' | 'failed'

export interface HermesTask {
  id: number
  tenant_id: number
  name: string
  description: string
  status: HermesTaskStatus
  cluster_id: number
  policy_id: number
  // HermesTaskView 附加字段（后端 ListTasks/GetTask 返回）
  cluster_name?: string
  run_total?: number
  run_done?: number
  started_at?: string
  finished_at?: string
  created_at: string
  updated_at: string
}

export interface HermesTaskPayload {
  name: string
  description: string
  cluster_id: number
  policy_id: number
}

export interface HermesTaskRun {
  id: number
  tenant_id: number
  task_id: number
  instance_id: number
  status: HermesTaskRunStatus
  result: string
  started_at?: string
  finished_at?: string
  created_at: string
  updated_at: string
}

export interface HermesTaskDetail {
  task: HermesTask
  cluster?: HermesCluster
  policy?: HermesPolicy
  runs: (HermesTaskRun & { instance_name: string })[]
}

export const listHermesClusters = (params: { page: number; page_size: number }) =>
  request.get<unknown, PageResponse<HermesCluster>>('/api/v1/ai/hermes/clusters', { params })

export const createHermesCluster = (data: HermesClusterPayload) =>
  request.post<unknown, HermesCluster>('/api/v1/ai/hermes/clusters', data)

export const getHermesCluster = (id: number) =>
  request.get<unknown, { cluster: HermesCluster; instances: HermesInstance[] }>(`/api/v1/ai/hermes/clusters/${id}`)

export const updateHermesCluster = (id: number, data: HermesClusterPayload) =>
  request.put<unknown, void>(`/api/v1/ai/hermes/clusters/${id}`, data)

export const deleteHermesCluster = (id: number) =>
  request.delete<unknown, void>(`/api/v1/ai/hermes/clusters/${id}`)

export const listHermesTasks = (params: { page: number; page_size: number; status?: HermesTaskStatus }) =>
  request.get<unknown, PageResponse<HermesTask>>('/api/v1/ai/hermes/tasks', { params })

export const createHermesTask = (data: HermesTaskPayload) =>
  request.post<unknown, HermesTask>('/api/v1/ai/hermes/tasks', data)

export const getHermesTask = (id: number) =>
  request.get<unknown, HermesTaskDetail>(`/api/v1/ai/hermes/tasks/${id}`)

export const activateHermesTask = (id: number) =>
  request.post<unknown, HermesTask>(`/api/v1/ai/hermes/tasks/${id}/activate`)

export const completeHermesTask = (id: number) =>
  request.post<unknown, void>(`/api/v1/ai/hermes/tasks/${id}/complete`)

export const cancelHermesTask = (id: number) =>
  request.post<unknown, void>(`/api/v1/ai/hermes/tasks/${id}/cancel`)

export const updateHermesTaskRun = (
  runId: number,
  data: { status: Extract<HermesTaskRunStatus, 'running' | 'completed' | 'failed'>; result?: string },
) => request.put<unknown, void>(`/api/v1/ai/hermes/task-runs/${runId}`, data)

// ── 技能市场（M2）：技能 CRUD + 集群绑定 ──────────────────────────────

export type HermesSkillStatus = 'pending' | 'approved' | 'rejected' | 'revoked'

export interface HermesSkill {
  id: number
  tenant_id: number
  name: string
  display_name: string
  description: string
  version: string
  sha256: string
  signature?: string
  status: HermesSkillStatus
  /** 官方市场导入来源（repo/path/commit）；手工上传为空串 */
  source_repo?: string
  source_path?: string
  source_commit?: string
  /** 供应链验签状态：verified / unverified / not_applicable */
  signature_status?: string
  approved_by: number
  approved_at?: string
  created_at: string
  updated_at: string
}

export const listHermesSkills = (params: { page: number; page_size: number }) =>
  request.get<unknown, PageResponse<HermesSkill>>('/api/v1/ai/hermes/skills', { params })

export const uploadHermesSkill = (data: FormData) =>
  request.post<unknown, HermesSkill>('/api/v1/ai/hermes/skills', data, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })

export const approveHermesSkill = (id: number) =>
  request.post<unknown, void>(`/api/v1/ai/hermes/skills/${id}/approve`)

export const rejectHermesSkill = (id: number) =>
  request.post<unknown, void>(`/api/v1/ai/hermes/skills/${id}/reject`)

export const revokeHermesSkill = (id: number) =>
  request.post<unknown, void>(`/api/v1/ai/hermes/skills/${id}/revoke`)

export const listClusterSkills = (clusterId: number) =>
  request.get<unknown, HermesSkill[]>(`/api/v1/ai/hermes/clusters/${clusterId}/skills`)

export const bindClusterSkill = (clusterId: number, skillId: number) =>
  request.post<unknown, void>(`/api/v1/ai/hermes/clusters/${clusterId}/skills`, { skill_id: skillId })

export const unbindClusterSkill = (clusterId: number, skillId: number) =>
  request.delete<unknown, void>(`/api/v1/ai/hermes/clusters/${clusterId}/skills/${skillId}`)

// ── MCP 管理（M3）：MCP server CRUD + 集群绑定 ────────────────────────────

export type HermesMcpServerType = 'stdio' | 'sse' | 'http'
export type HermesMcpStatus = 'pending' | 'approved' | 'rejected' | 'revoked'

export interface HermesMcpServer {
  id: number
  tenant_id: number
  name: string
  display_name: string
  description: string
  type: HermesMcpServerType
  command: string
  args: string[]
  url: string
  env: Record<string, string>
  headers: Record<string, string>
  tools_include: string[]
  tools_exclude: string[]
  timeout: number
  status: HermesMcpStatus
  approved_by: number
  approved_at?: string
  secret_revision: number
  created_at: string
  updated_at: string
}

export interface HermesMcpServerPayload {
  name: string
  display_name: string
  description: string
  type: HermesMcpServerType
  command?: string
  args?: string[]
  url?: string
  env?: Record<string, string>
  headers?: Record<string, string>
  tools_include?: string[]
  tools_exclude?: string[]
  timeout?: number
  secret_json?: Record<string, string>
}

export const listHermesMcpServers = (params: { page: number; page_size: number }) =>
  request.get<unknown, PageResponse<HermesMcpServer>>('/api/v1/ai/hermes/mcp-servers', { params })

export const createHermesMcpServer = (data: HermesMcpServerPayload) =>
  request.post<unknown, HermesMcpServer>('/api/v1/ai/hermes/mcp-servers', data)

export const updateHermesMcpServer = (id: number, data: HermesMcpServerPayload) =>
  request.put<unknown, void>(`/api/v1/ai/hermes/mcp-servers/${id}`, data)

export const approveHermesMcpServer = (id: number) =>
  request.post<unknown, void>(`/api/v1/ai/hermes/mcp-servers/${id}/approve`)

export const rejectHermesMcpServer = (id: number) =>
  request.post<unknown, void>(`/api/v1/ai/hermes/mcp-servers/${id}/reject`)

export const revokeHermesMcpServer = (id: number) =>
  request.post<unknown, void>(`/api/v1/ai/hermes/mcp-servers/${id}/revoke`)

export const listClusterMcpServers = (clusterId: number) =>
  request.get<unknown, HermesMcpServer[]>(`/api/v1/ai/hermes/clusters/${clusterId}/mcp-servers`)

export const bindClusterMcpServer = (clusterId: number, serverId: number) =>
  request.post<unknown, void>(`/api/v1/ai/hermes/clusters/${clusterId}/mcp-servers`, { server_id: serverId })

export const unbindClusterMcpServer = (clusterId: number, serverId: number) =>
  request.delete<unknown, void>(`/api/v1/ai/hermes/clusters/${clusterId}/mcp-servers/${serverId}`)

// ── 定时任务（M5）：cron job CRUD + 触发记录 ─────────────────────────
export type HermesCronRunStatus = 'triggered' | 'completed' | 'failed'

export interface HermesCronJob {
  id: number
  tenant_id: number
  name: string
  description: string
  cron_expression: string
  action_type: string
  prompt: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface HermesCronJobView extends HermesCronJob {
  run_count?: number
  last_run_at?: string
}

export interface HermesCronJobPayload {
  name: string
  description?: string
  cron_expression: string
  action_type?: 'prompt'
  prompt: string
  enabled: boolean
}

export interface HermesCronRun {
  id: number
  job_id: number
  device_id: string
  status: HermesCronRunStatus
  result: string
  triggered_at?: string
  finished_at?: string
  created_at: string
}

export const listHermesCronJobs = (params: { page: number; page_size: number }) =>
  request.get<unknown, PageResponse<HermesCronJobView>>('/api/v1/ai/hermes/cron', { params })

export const createHermesCronJob = (data: HermesCronJobPayload) =>
  request.post<unknown, HermesCronJob>('/api/v1/ai/hermes/cron', data)

export const updateHermesCronJob = (id: number, data: HermesCronJobPayload) =>
  request.put<unknown, HermesCronJob>(`/api/v1/ai/hermes/cron/${id}`, data)

export const toggleHermesCronJob = (id: number) =>
  request.patch<unknown, HermesCronJob>(`/api/v1/ai/hermes/cron/${id}/toggle`)

export const deleteHermesCronJob = (id: number) =>
  request.delete<unknown, void>(`/api/v1/ai/hermes/cron/${id}`)

export const listHermesCronRuns = (jobId: number, params: { page: number; page_size: number }) =>
  request.get<unknown, PageResponse<HermesCronRun>>(`/api/v1/ai/hermes/cron/${jobId}/runs`, { params })

// ── 员工自助（M6）：技能自助安装 + 能力预览 + 个人用量 ────────────────
export interface HermesCapabilities {
  skills: { name: string; version: string }[]
  mcp_servers: string[]
  toolsets: string[]
}

export interface HermesSkillCatalogItem extends HermesSkill {
  installed: boolean
}

export interface HermesMyUsage {
  days: number
  summary: HermesUsageSummary
  daily: HermesUsageDaily[]
}

export const listMyHermesSkillCatalog = (params: { page: number; page_size: number }) =>
  request.get<unknown, PageResponse<HermesSkillCatalogItem>>('/api/v1/ai/hermes/me/skills/catalog', { params })

export const installMyHermesSkill = (id: number) =>
  request.post<unknown, void>(`/api/v1/ai/hermes/me/skills/${id}`)

export const uninstallMyHermesSkill = (id: number) =>
  request.delete<unknown, void>(`/api/v1/ai/hermes/me/skills/${id}`)

export const getMyHermesUsage = (days = 7) =>
  request.get<unknown, HermesMyUsage>('/api/v1/ai/hermes/me/usage', { params: { days } })

// ── 官方 Skills Hub（GitHub 系源浏览 + 导入企业目录）──────────────────
export interface HermesSkillHubSource {
  key: string
  name: string
  repo: string
  path: string
}

export interface HermesSkillHubEntry {
  name: string
  path: string
}

export interface HermesSkillHubBrowse {
  repo: string
  path: string
  commit: string
  entries: HermesSkillHubEntry[]
  sources: HermesSkillHubSource[]
}

export const browseHermesSkillHub = (params: { source: string; repo?: string; path?: string }) =>
  request.get<unknown, HermesSkillHubBrowse>('/api/v1/ai/hermes/skills/hub', { params })

export const importHermesSkillHub = (data: {
  source: string
  custom_repo?: string
  custom_path?: string
  skill_path: string
  ref: string
}) => request.post<unknown, HermesSkill>('/api/v1/ai/hermes/skills/hub/import', data)
