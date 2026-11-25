import type {
  EdgeCert,
  EdgeCertCapabilities,
  EdgeCertCapability,
  EdgeCertCertificateState,
  EdgeCertDeploymentState,
  EdgeCertIssuanceState,
  EdgeCertRenewalState,
  EdgeCertServingState,
  EdgeCertTask,
  EdgeCertTaskKind,
  EdgeCertTaskStatus,
} from '@/api/system/edgeCert'

export type CertRow = EdgeCert & {
  certificate: EdgeCertCertificateState
  issuance: EdgeCertIssuanceState
  deployment: EdgeCertDeploymentState
  renewal: EdgeCertRenewalState
  serving: EdgeCertServingState
}

export type CapabilityName = keyof Pick<
  EdgeCertCapabilities,
  'issue' | 'renew' | 'deploy' | 'probe' | 'export'
>

export interface StatusMetaItem {
  label: string
  color?: string
}

export type TaskModalState = {
  row: CertRow
  tasks: EdgeCertTask[]
  loading: boolean
} | null

export interface CertFormValues {
  domain: string
  email: string
  is_staging: boolean
  deployment_mode: 'external' | 'traefik_file'
  deployment_provider: 'external' | 'caddy' | 'cdn' | 'other' | 'traefik'
  auto_renew_enabled: boolean
}

export interface ExportKeyFormValues {
  current_password: string
  totp_code?: string
  confirm_domain: string
}

export const terminalTaskStatuses = new Set<EdgeCertTaskStatus>(['succeeded', 'failed'])

export const CAPABILITY_NAMES: readonly CapabilityName[] = [
  'issue',
  'renew',
  'deploy',
  'probe',
  'export',
]

export const DOMAIN_PATTERN =
  /^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+$/

export const taskKindMeta: Record<EdgeCertTaskKind, { label: string; submitted: string }> = {
  issue: { label: '签发', submitted: '签发任务已提交' },
  renew: { label: '续期', submitted: '续期任务已提交' },
  deploy: { label: '部署', submitted: '部署任务已提交' },
  probe: { label: '探测', submitted: '线上探测任务已提交' },
}

export const taskStatusMeta: Record<EdgeCertTaskStatus, StatusMetaItem> = {
  queued: { label: '排队中', color: 'default' },
  running: { label: '执行中', color: 'processing' },
  succeeded: { label: '成功', color: 'success' },
  failed: { label: '失败', color: 'error' },
}

export const certificateStatusMeta: Record<string, StatusMetaItem> = {
  none: { label: '未签发' },
  draft: { label: '未签发' },
  pending: { label: '签发中', color: 'processing' },
  issued: { label: '已签发', color: 'success' },
  valid: { label: '有效', color: 'success' },
  expiring: { label: '30 天内到期', color: 'warning' },
  failed: { label: '签发失败', color: 'error' },
  expired: { label: '已过期', color: 'warning' },
  missing: { label: '无证书' },
}

export const issuanceStatusMeta: Record<string, StatusMetaItem> = {
  idle: { label: '未提交' },
  draft: { label: '未提交' },
  queued: { label: '等待签发' },
  pending: { label: '签发中', color: 'processing' },
  running: { label: '签发中', color: 'processing' },
  succeeded: { label: '签发成功', color: 'success' },
  issued: { label: '签发成功', color: 'success' },
  failed: { label: '签发失败', color: 'error' },
}

export const deploymentStatusMeta: Record<string, StatusMetaItem> = {
  external: { label: '外部托管', color: 'blue' },
  not_deployed: { label: '未部署' },
  not_installed: { label: '未部署' },
  queued: { label: '等待部署', color: 'default' },
  pending: { label: '等待部署', color: 'default' },
  running: { label: '部署中', color: 'processing' },
  installed: { label: '已部署', color: 'success' },
  deployed: { label: '已部署', color: 'success' },
  failed: { label: '部署失败', color: 'error' },
}

export const renewalStatusMeta: Record<string, StatusMetaItem> = {
  disabled: { label: '自动续期未开启' },
  awaiting_certificate: { label: '等待证书签发' },
  scheduled: { label: '已安排续期', color: 'blue' },
  due: { label: '需要续期', color: 'warning' },
  idle: { label: '续期空闲' },
  queued: { label: '等待续期' },
  running: { label: '续期中', color: 'processing' },
  completed: { label: '最近续期成功', color: 'success' },
  succeeded: { label: '最近续期成功', color: 'success' },
  failed: { label: '最近续期失败', color: 'error' },
}

export const servingStatusMeta: Record<string, StatusMetaItem> = {
  unchecked: { label: '未探测' },
  unknown: { label: '未探测' },
  healthy: { label: '线上匹配', color: 'success' },
  matching: { label: '线上匹配', color: 'success' },
  mismatch: { label: '线上不匹配', color: 'warning' },
  unreachable: { label: '无法连接', color: 'error' },
  invalid: { label: '线上证书无效', color: 'error' },
  failed: { label: '探测失败', color: 'error' },
}

export const capabilityOff: EdgeCertCapability = { enabled: false, reason: '服务端未声明此能力' }

export function readError(error: unknown, fallback: string): string {
  if (!error || typeof error !== 'object') return fallback
  const candidate = error as {
    message?: string
    response?: { data?: { message?: string } }
  }
  return candidate.response?.data?.message || candidate.message || fallback
}

export function normalizeRow(row: EdgeCert): CertRow {
  const certificateStatus = row.certificate?.status || row.status || (row.has_cert ? 'issued' : 'draft')
  return {
    ...row,
    certificate: row.certificate ?? {
      status: certificateStatus,
      has_certificate: !!row.has_cert,
      not_before: row.not_before,
      not_after: row.not_after,
    },
    issuance: row.issuance ?? { status: certificateStatus, last_error: row.last_error },
    deployment: row.deployment ?? {
      mode: row.deployment_mode || 'external',
      provider: row.deployment_provider || 'external',
      status: row.deployment_status || 'external',
    },
    renewal: row.renewal ?? {
      status: row.last_renewal_at ? 'completed' : 'idle',
      auto_enabled: !!row.auto_renew_enabled,
      renew_at: row.renew_at,
      last_renewal_at: row.last_renewal_at,
    },
    serving: row.serving ?? {
      status: row.serving_status || 'unchecked',
      not_after: row.serving_not_after,
      issuer: row.serving_issuer,
      checked_at: row.serving_checked_at,
      error_code: row.serving_error_code,
      error_message: row.serving_error_message,
    },
  }
}

export function saveBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  try {
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = filename
    document.body.appendChild(anchor)
    anchor.click()
    anchor.remove()
  } finally {
    URL.revokeObjectURL(url)
  }
}

export function hasActiveTask(row: CertRow): boolean {
  return !!row.active_task && !terminalTaskStatuses.has(row.active_task.status)
}

export function isExternal(row: CertRow): boolean {
  return (
    row.deployment.mode === 'external'
    || row.deployment.provider === 'external'
    || row.deployment.provider === 'caddy'
  )
}

export function isServing(row: CertRow): boolean {
  return ['healthy', 'matching'].includes(row.serving.status)
}

export function isInUse(row: CertRow): boolean {
  return (
    row.serving.managed_certificate_in_use === true
    || (!isExternal(row) && isServing(row))
    || ['installed', 'deployed'].includes(row.deployment.status)
  )
}

export function hasCertificate(row: CertRow): boolean {
  return row.certificate.has_certificate || ['issued', 'valid'].includes(row.certificate.status)
}

export function getCertFormInitialValues(row: CertRow | null): CertFormValues {
  if (!row) {
    return {
      domain: '',
      email: '',
      is_staging: true,
      deployment_mode: 'external',
      deployment_provider: 'caddy',
      auto_renew_enabled: false,
    }
  }
  return {
    domain: row.domain,
    email: row.email,
    is_staging: row.is_staging,
    deployment_mode: row.deployment.mode === 'traefik_file' ? 'traefik_file' : 'external',
    deployment_provider: (row.deployment.provider || 'external') as CertFormValues['deployment_provider'],
    auto_renew_enabled: row.renewal.auto_enabled,
  }
}

export function buildCapabilityWarnings(
  capabilities: EdgeCertCapabilities | null,
  capabilitiesReason: string,
  canExport: boolean,
  totpEnabled: boolean,
): string[] {
  const warnings = capabilities
    ? CAPABILITY_NAMES
      .map((name) => capabilities[name] ?? capabilityOff)
      .filter((value) => !value.enabled && value.reason)
      .map((value) => value.reason as string)
      .filter((value, index, all) => all.indexOf(value) === index)
    : capabilitiesReason
      ? [capabilitiesReason]
      : []
  if (canExport && !totpEnabled) {
    warnings.push('私钥导出已锁定：请先在个人安全设置中启用两步验证（TOTP）')
  }
  return warnings
}
