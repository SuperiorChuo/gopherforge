export interface LoginRequest {
  username: string
  password: string
  captcha?: string
  captcha_id?: string
}

export interface LoginResponse {
  access_token?: string
  refresh_token?: string
  require_totp?: boolean
  totp_challenge_id?: string
  user?: UserInfo
}

export interface VerifyTOTPLoginRequest {
  challenge_id: string
  code: string
}

export interface UserInfo {
  id?: number
  username: string
  nickname?: string
  email?: string
  phone?: string
  avatar?: string
  status?: number
  roles?: RoleInfo[]
  permissions?: string[]
  must_change_password?: boolean
  totp_enabled?: boolean
  department_id?: number
  created_at?: string
}

export interface RoleInfo {
  id: number
  name: string
  code: string
}

export interface PageRequest {
  page: number
  page_size: number
}

export interface PageResponse<T> {
  list: T[]
  total: number
}

export interface ApiResponse<T = unknown> {
  code: number
  message: string
  data: T
}

export interface ChangePasswordRequest {
  old_password: string
  new_password: string
}

export interface UpdateProfileRequest {
  nickname?: string
  email?: string
  phone?: string
}

export interface MenuItem {
  id?: number
  route_key: string
  path: string
  name: string
  component_key?: string
  redirect?: string
  parent_key?: string
  sort_order?: number
  hidden?: boolean
  public?: boolean
  enabled?: boolean
  permissions?: string[]
  roles?: string[]
  meta?: Record<string, unknown>
  children?: MenuItem[]
}

export interface SystemUser {
  id: number
  username: string
  nickname?: string
  email?: string
  phone?: string
  status: number
  must_change_password?: boolean
  department_id?: number
  roles?: RoleInfo[]
  created_at?: string
}

export interface SystemRole {
  id: number
  name: string
  code: string
  description?: string
  status: number
  created_at?: string
}

export interface Permission {
  id: number
  name: string
  code: string
  type: number
  description?: string
  status: number
  created_at?: string
}

export interface Menu {
  id: number
  name: string
  title: string
  path: string
  component?: string
  icon?: string
  parent_id: number
  sort: number
  status: number
  hidden?: boolean
  created_at?: string
  children?: Menu[]
}

export interface Department {
  id: number
  name: string
  code: string
  parent_id: number
  sort: number
  status: number
  leader?: string
  phone?: string
  email?: string
  created_at?: string
  children?: Department[]
}

export interface DictType {
  id: number
  name: string
  code: string
  status: number
  created_at?: string
}

export interface DictItem {
  id: number
  label: string
  value: string
  sort: number
  status: number
  dict_type_id: number
  created_at?: string
}

export interface FileRecord {
  id: number
  file_name: string
  file_path: string
  file_size: number
  file_type: string
  storage_type: string
  user_id: number
  created_at?: string
}

export interface LoginLog {
  id: number
  user_id: number
  username: string
  ip: string
  status: number
  login_type: number
  browser?: string
  os?: string
  created_at?: string
}

export interface OperationLog {
  id: number
  user_id?: number
  username?: string
  method: string
  path: string
  status: number
  module?: string
  action?: string
  request_id?: string
  created_at?: string
}

export interface Notice {
  id: number
  title: string
  content: string
  type: number
  status: number
  start_time?: string
  end_time?: string
  created_at?: string
}

export interface SystemSetting {
  setting_key: string
  value_json: Record<string, unknown>
  updated_at?: string
}

export interface ScheduledJob {
  id: number
  name: string
  group_name?: string
  cron_expr: string
  handler: string
  args?: string
  status: number
  created_at?: string
}

export interface OnlineUser {
  session_id: string
  user_id: number
  username: string
  ip?: string
  last_seen_at?: string
  created_at?: string
}
