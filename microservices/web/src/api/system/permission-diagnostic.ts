import request from '@/utils/request'

export interface PermissionDiagnosticRole {
  id: number
  name: string
  code: string
  data_scope: string
  permission_ids: number[]
  permissions: string[]
  matches: boolean
  match_reason?: string
}

export interface PermissionDiagnosticResult {
  allowed: boolean
  reason: string
  requested_permission: string
  matched_by?: string
  user: {
    id: number
    tenant_id: number
    username: string
    nickname?: string
    department_id: number
    status: number
  }
  roles: PermissionDiagnosticRole[]
  effective_permissions: string[]
  package: {
    bound: boolean
    id?: number
    name?: string
    status?: number
    allows_permission: boolean
    has_existing_overrun: boolean
  }
  data_scope: {
    scope: string
    department_id: number
    department_ids: number[]
  }
}

export const diagnosePermission = (data: { user_id: number; permission: string }) =>
  request.post<unknown, PermissionDiagnosticResult>('/api/v1/permissions/diagnose', data)
