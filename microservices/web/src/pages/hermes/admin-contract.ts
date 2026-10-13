import type {
  HermesCluster,
  HermesClusterPayload,
  HermesInstance,
  HermesInstancePayload,
  HermesInstanceStatus,
  HermesPolicy,
  HermesPolicyPayload,
  HermesPolicyProfile,
} from '../../api/ai/hermes.ts'
import type { PageResponse } from '../../types'

export const HERMES_INSTANCE_STATUS_LABELS: Record<HermesInstanceStatus, string> = {
  unassigned: '未放置',
  pending: '待启动',
  running: '运行中',
  error: '异常',
}

export type HermesInstanceFormValues = Pick<HermesInstancePayload, 'name' | 'role' | 'enabled'> &
  Partial<Pick<HermesInstancePayload, 'owner_user_id' | 'device_id' | 'policy_id' | 'cluster_id'>>

export function instanceFormValues(record?: HermesInstance): HermesInstanceFormValues {
  return {
    name: record?.name ?? '',
    role: record?.role ?? 'default',
    owner_user_id: record?.owner_user_id || undefined,
    device_id: record?.device_id || undefined,
    policy_id: record?.policy_id || undefined,
    cluster_id: record?.cluster_id || undefined,
    enabled: record?.enabled ?? true,
  }
}

export function instancePayload(values: HermesInstanceFormValues): HermesInstancePayload {
  return {
    name: values.name,
    role: values.role || 'default',
    owner_user_id: values.owner_user_id ?? 0,
    device_id: values.device_id ?? '',
    policy_id: values.policy_id ?? 0,
    cluster_id: values.cluster_id ?? 0,
    enabled: values.enabled ?? true,
  }
}

export type HermesClusterFormValues = Pick<HermesClusterPayload, 'name' | 'enabled'> &
  Partial<Pick<HermesClusterPayload, 'description' | 'policy_id' | 'slug'>>

export function clusterFormValues(record?: HermesCluster): HermesClusterFormValues {
  return {
    name: record?.name ?? '',
    slug: record?.slug ?? '',
    description: record?.description ?? '',
    policy_id: record?.policy_id || undefined,
    enabled: record?.enabled ?? true,
  }
}

export function clusterPayload(values: HermesClusterFormValues): HermesClusterPayload {
  return {
    name: values.name,
    slug: values.slug ?? '',
    description: values.description ?? '',
    policy_id: values.policy_id ?? 0,
    enabled: values.enabled ?? true,
  }
}

export type HermesPolicyFormValues = Omit<HermesPolicyPayload, 'artifact_id' | 'model_id' | 'privacy'> & {
  artifact_id?: number
  model_id?: number
}

export function applyFulfilledValue<T>(result: PromiseSettledResult<T>, apply: (value: T) => void): boolean {
  if (result.status !== 'fulfilled') return false
  apply(result.value)
  return true
}

export type HermesRequestGate = { current: number }

export function beginHermesRequest(gate: HermesRequestGate): number {
  gate.current += 1
  return gate.current
}

export function invalidateHermesRequests(gate: HermesRequestGate): void {
  gate.current += 1
}

export function isLatestHermesRequest(requestID: number, currentRequestID: number): boolean {
  return requestID === currentRequestID
}

export async function runLatestHermesRequest<T>(
  gate: HermesRequestGate,
  requestID: number,
  load: () => Promise<T>,
  apply: (value: T) => void,
  settle: () => void,
): Promise<boolean> {
  try {
    const value = await load()
    if (!isLatestHermesRequest(requestID, gate.current)) return false
    apply(value)
    return true
  } finally {
    if (isLatestHermesRequest(requestID, gate.current)) settle()
  }
}

export function policyToolsetCatalog(p1Toolsets: readonly string[], clusterToolsets: readonly string[]): string[] {
  return [...new Set([...p1Toolsets, ...clusterToolsets])]
}

export function isPolicyToolsetValid(
  profile: HermesPolicyProfile,
  toolsets: readonly string[],
  p1Toolsets: readonly string[],
  clusterToolsets: readonly string[],
): boolean {
  const unique = new Set(toolsets)
  if (unique.size !== toolsets.length) return false
  const allowed = profile === 'company_cluster' ? clusterToolsets : p1Toolsets
  if (toolsets.some((tool) => !allowed.includes(tool))) return false
  if (profile === 'company_cluster') return unique.has('delegation')
  return toolsets.length === p1Toolsets.length && p1Toolsets.every((tool) => unique.has(tool))
}

const lockedPrivacy = {
  upload_conversations: false,
  upload_memory: false,
  upload_local_files: false,
} as const

export function policyFormValues(
  policy: HermesPolicy | undefined,
  defaultToolsets: string[],
): Partial<HermesPolicyFormValues> {
  if (policy) {
    return {
      profile: policy.profile,
      name: policy.name,
      desired_state: policy.desired_state,
      assistant_template_id: policy.assistant_template_id,
      assistant_template_revision: policy.assistant_template_revision,
      artifact_id: policy.artifact_id,
      model_id: policy.model_id,
      model_name: policy.model_name || '',
      model_base_url: policy.model_base_url || '',
      toolsets: policy.profile === 'company' ? [...defaultToolsets] : [...(policy.toolsets ?? [])],
      lease_seconds: policy.lease_seconds,
      enable_knowledge_rag: policy.enable_knowledge_rag,
    }
  }
  return {
    profile: 'company',
    desired_state: 'active',
    artifact_id: 0,
    assistant_template_revision: 0,
    lease_seconds: 3600,
    toolsets: [...defaultToolsets],
  }
}

export function policyPayload(
  values: HermesPolicyFormValues,
  p1Toolsets: readonly string[],
  clusterToolsets: readonly string[],
): HermesPolicyPayload {
  if (!isPolicyToolsetValid(values.profile, values.toolsets, p1Toolsets, clusterToolsets)) {
    throw new Error('invalid Hermes policy toolsets')
  }
  return {
    profile: values.profile as HermesPolicyProfile,
    name: values.name,
    desired_state: values.desired_state,
    assistant_template_id: values.assistant_template_id ?? '',
    assistant_template_revision: values.assistant_template_revision ?? 0,
    artifact_id: values.artifact_id ?? 0,
    model_id: values.model_id ?? 0,
    model_name: values.model_name ?? '',
    model_base_url: values.model_base_url ?? '',
    toolsets: [...values.toolsets],
    privacy: { ...lockedPrivacy },
    lease_seconds: values.lease_seconds,
    enable_knowledge_rag: values.enable_knowledge_rag ?? false,
  }
}

/** 每次请求真实分页的页上限，避免前端“放大 page_size 伪装全量”破坏服务端契约。 */
export const MAX_HERMES_STATS_PAGES = 100

/**
 * 按页循环拉取完整列表用于统计（工具栏 running/pending 等聚合数字）。
 * 不做任何 page_size 放大：逐页真实请求直到 total 收敛；空页或超过页上限即停。
 */
export async function fetchAllHermesPages<T>(
  fetcher: (page: number, pageSize: number) => Promise<PageResponse<T>>,
  pageSize = 100,
): Promise<T[]> {
  const collected: T[] = []
  let page = 1
  for (let guard = 0; guard < MAX_HERMES_STATS_PAGES; guard += 1) {
    const result = await fetcher(page, pageSize)
    const list = result.list ?? []
    collected.push(...list)
    if (list.length === 0 || collected.length >= (result.total ?? collected.length)) break
    page += 1
  }
  return collected
}
