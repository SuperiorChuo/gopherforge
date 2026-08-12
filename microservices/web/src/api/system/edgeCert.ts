import request from '@/utils/request'

export type EdgeCertTaskKind = 'issue' | 'renew' | 'deploy' | 'probe'
export type EdgeCertTaskStatus = 'queued' | 'running' | 'succeeded' | 'failed'

export interface EdgeCertTask {
  id: number
  certificate_id: number
  kind: EdgeCertTaskKind
  status: EdgeCertTaskStatus
  step?: string
  environment?: 'staging' | 'production'
  attempt_count?: number
  error_code?: string
  error_message?: string
  error_hint?: string
  created_at: string
  updated_at?: string
  started_at?: string
  finished_at?: string
}

export interface EdgeCertCertificateState {
  status: string
  has_certificate: boolean
  not_before?: string
  not_after?: string
  fingerprint_sha256?: string
}

export interface EdgeCertIssuanceState {
  status: string
  last_error?: string
}

export interface EdgeCertDeploymentState {
  mode: string
  provider: string
  status: string
  deployed_fingerprint_sha256?: string
  deployed_at?: string
  last_error?: string
}

export interface EdgeCertRenewalState {
  status: string
  auto_enabled: boolean
  renew_at?: string
  last_renewal_at?: string
  last_error?: string
}

export interface EdgeCertServingState {
  status: string
  managed_certificate_in_use?: boolean
  fingerprint_sha256?: string
  not_after?: string
  issuer?: string
  checked_at?: string
  error_code?: string
  error_message?: string
}

export interface EdgeCert {
  id: number
  domain: string
  email: string
  provider: string
  is_staging: boolean
  certificate: EdgeCertCertificateState
  issuance: EdgeCertIssuanceState
  deployment: EdgeCertDeploymentState
  renewal: EdgeCertRenewalState
  serving: EdgeCertServingState
  active_task?: EdgeCertTask | null
  created_at: string
  updated_at: string

  // V1 transition fields. The page normalizes these when talking to an older service.
  status?: string
  not_before?: string
  not_after?: string
  last_error?: string
  has_cert?: boolean
  deployment_mode?: string
  deployment_provider?: string
  deployment_status?: string
  auto_renew_enabled?: boolean
  renew_at?: string
  last_renewal_at?: string
  serving_status?: string
  serving_not_after?: string
  serving_issuer?: string
  serving_checked_at?: string
  serving_error_code?: string
  serving_error_message?: string
}

export interface EdgeCertCapability {
  enabled: boolean
  reason?: string
}

export interface EdgeCertCapabilities {
  issue: EdgeCertCapability
  renew: EdgeCertCapability
  deploy: EdgeCertCapability
  probe: EdgeCertCapability
  export: EdgeCertCapability
  deployment_mode?: string
  deployment_provider?: string
  external_tls_managed?: boolean
}

export interface EdgeCertTaskList {
  list: EdgeCertTask[]
}

export interface EdgeCertTaskAccepted {
  task: EdgeCertTask
  reused?: boolean
}

export interface EdgeCertStepUpRequest {
  current_password: string
  totp_code?: string
  certificate_id: number
}

export interface EdgeCertStepUpProof {
  proof: string
  step_up_proof?: string
  expires_in_seconds?: number
}

export const listEdgeCerts = () =>
  request.get<unknown, EdgeCert[]>('/api/v1/edge-certs')

export const getEdgeCertCapabilities = () =>
  request.get<unknown, EdgeCertCapabilities>('/api/v1/edge-certs/capabilities', { silent: true })

export const createEdgeCert = (body: {
  domain: string
  email: string
  is_staging?: boolean
  deployment_mode?: 'external' | 'traefik_file'
  deployment_provider?: 'external' | 'caddy' | 'cdn' | 'other' | 'traefik'
  auto_renew_enabled?: boolean
}) =>
  request.post<unknown, EdgeCert>('/api/v1/edge-certs', body)

const enqueueTask = (id: number, kind: EdgeCertTaskKind) =>
  request.post<unknown, EdgeCertTaskAccepted>(`/api/v1/edge-certs/${id}/${kind}`, {}, { silent: true })

export const issueEdgeCert = (id: number) => enqueueTask(id, 'issue')
export const renewEdgeCert = (id: number) => enqueueTask(id, 'renew')
export const deployEdgeCert = (id: number) => enqueueTask(id, 'deploy')
export const probeEdgeCert = (id: number) => enqueueTask(id, 'probe')

export const listEdgeCertTasks = (id: number) =>
  request.get<unknown, EdgeCertTaskList | EdgeCertTask[]>(`/api/v1/edge-certs/${id}/tasks`, { silent: true })

export const getEdgeCertTask = (certificateId: number, taskId: number) =>
  request.get<unknown, EdgeCertTask>(`/api/v1/edge-certs/${certificateId}/tasks/${taskId}`, { silent: true })

export const deleteEdgeCert = (id: number) =>
  request.delete<unknown, void>(`/api/v1/edge-certs/${id}`)

export const downloadEdgeCertificate = (id: number) =>
  request.get<unknown, Blob>(`/api/v1/edge-certs/${id}/certificate`, {
    responseType: 'blob',
    timeout: 30_000,
  })

export const requestEdgeCertExportStepUp = (body: EdgeCertStepUpRequest) =>
  request.post<unknown, EdgeCertStepUpProof>('/api/v1/auth/step-up/edge-cert-export', body, { silent: true })

export const exportEdgeCertPrivateKey = (
  id: number,
  body: { step_up_proof: string; confirm_domain: string },
) => request.post<unknown, Blob>(`/api/v1/edge-certs/${id}/export`, body, {
  responseType: 'blob',
  timeout: 30_000,
  silent: true,
})
