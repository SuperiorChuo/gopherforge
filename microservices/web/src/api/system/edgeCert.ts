import request from '@/utils/request'

export interface EdgeCert {
  id: number
  domain: string
  email: string
  status: string
  provider: string
  is_staging: boolean
  not_before?: string
  not_after?: string
  last_error?: string
  has_cert: boolean
  created_at: string
  updated_at: string
}

export interface EdgeCertDownload {
  domain: string
  fullchain_pem: string
  private_key_pem: string
}

export const listEdgeCerts = () =>
  request.get<unknown, EdgeCert[]>('/api/v1/edge-certs')

export const createEdgeCert = (body: { domain: string; email: string; is_staging?: boolean }) =>
  request.post<unknown, EdgeCert>('/api/v1/edge-certs', body)

export const issueEdgeCert = (id: number) =>
  request.post<unknown, EdgeCert>(`/api/v1/edge-certs/${id}/issue`, {}, { timeout: 180000 })

export const deleteEdgeCert = (id: number) =>
  request.delete<unknown, void>(`/api/v1/edge-certs/${id}`)

export const downloadEdgeCert = (id: number) =>
  request.get<unknown, EdgeCertDownload>(`/api/v1/edge-certs/${id}/download`)
