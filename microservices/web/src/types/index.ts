export interface LoginRequest {
  username: string
  password: string
  captcha_id: string
  captcha_code: string
  /** SaaS tenant code; empty defaults to "default" on server */
  tenant_code?: string
}

export interface CaptchaResponse {
  key: string
  type: string
  image: string
  width: number
  height: number
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
  tenant_id?: number
  is_platform_admin?: boolean
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

export interface TenantInfo {
  id: number
  code: string
  name: string
  status: number
  plan: string
  max_users: number
  /** 绑定的租户套餐（权限包）；null/缺省 = 不限 */
  package_id?: number | null
  created_at?: string
  updated_at?: string
}

/** 租户套餐（权限包）：permission_codes 圈定租户内角色可分配的权限码 */
export interface TenantPackageInfo {
  id: number
  name: string
  permission_codes: string[]
  status: number
  remark?: string
  created_at?: string
  updated_at?: string
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

// /user/menus 返回的菜单树节点（后端 model.Menu 的 JSON 形状）
export interface MenuItem {
  id: number
  name: string
  title: string
  icon?: string
  path: string
  component?: string
  parent_id: number
  sort: number
  status: number
  hidden?: number
  permission?: string
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
  data_scope?: string
  created_at?: string
}

export interface Permission {
  id: number
  name: string
  code: string
  type: number
  description?: string
  path?: string
  method?: string
  parent_id?: number
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
  hidden?: number
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
  /** 部门主管用户 id（BPM dept_leader 审批人规则数据源；0/空=未设置） */
  leader_user_id?: number
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
  location?: string
  status: number
  login_type: number
  browser?: string
  os?: string
  message?: string
  created_at?: string
}

export interface OperationLog {
  id: number
  user_id?: number
  username?: string
  method: string
  path: string
  query?: string
  request_body?: string
  response_body?: string
  status: number
  module?: string
  action?: string
  request_id?: string
  ip?: string
  user_agent?: string
  latency?: number
  error_msg?: string
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
  cron_expression: string
  invoke_target: string
  description?: string
  status: number
  concurrent?: number
  last_run_time?: string
  next_run_time?: string
  created_at?: string
}

export interface OnlineUser {
  user_id: number
  username: string
  nickname?: string
  ip?: string
  location?: string
  browser?: string
  os?: string
  login_time?: string
  token_id: string
  access_token_expires_at?: string
}
