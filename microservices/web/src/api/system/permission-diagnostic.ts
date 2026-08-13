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

export interface PermissionDiagnosticOption {
  id: number
  name: string
  code: string
  path?: string
  method?: string
}

export interface PermissionMenuBinding {
  id: number
  title: string
  path?: string
  component?: string
  parent_id: number
  status: number
  hidden: number
  permission?: string
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
  resource: {
    registered: boolean
    id?: number
    name?: string
    description?: string
    type?: number
    path?: string
    method?: string
  }
}

export const getPermissionDiagnosticOptions = (params?: { keyword?: string; limit?: number }) =>
  request.get<unknown, PermissionDiagnosticOption[]>('/api/v1/permissions/diagnose/options', { params })

export const getPermissionDiagnosticMenus = (permission: string) =>
  request.get<unknown, { permission: string; menus: PermissionMenuBinding[] }>('/api/v1/menus/permission-diagnostics', {
    params: { permission },
  })

export const diagnosePermission = (data: { user_id: number; permission: string }) =>
  request.post<unknown, PermissionDiagnosticResult>('/api/v1/permissions/diagnose', data)
