export type MobileTileStatus = 'ready' | 'soon'

export type MobileTile = {
  key: string
  label: string
  hint: string
  to: string
  status: MobileTileStatus
  perm?: string
}

export type MobileWorkbenchGroup = {
  key: string
  title: string
  tiles: MobileTile[]
}

export const WORKBENCH_GROUPS: MobileWorkbenchGroup[] = [
  {
    key: 'act',
    title: '处理',
    tiles: [
      { key: 'tasks', label: '我的审批', hint: '通过 / 驳回', to: '/m/tasks', status: 'ready' },
      { key: 'notices', label: '公告', hint: '只读查看', to: '/m/notices', status: 'ready', perm: 'system:notice:list' },
    ],
  },
  {
    key: 'run',
    title: '运行',
    tiles: [
      { key: 'health', label: '服务健康', hint: '各服务是否存活', to: '/m/ops/health', status: 'ready', perm: 'system:monitor:server' },
      { key: 'alerts', label: '告警', hint: '正在触发的规则', to: '/m/ops/alerts', status: 'ready', perm: 'system:alert:list' },
      { key: 'jobs', label: '任务中心', hint: '近 24 小时失败', to: '/m/ops/jobs', status: 'ready', perm: 'system:job:list' },
      { key: 'online', label: '在线用户', hint: '查看并踢人', to: '/m/ops/online', status: 'ready', perm: 'system:online-user:list' },
    ],
  },
  {
    key: 'security',
    title: '安全',
    tiles: [
      { key: 'login-log', label: '登录日志', hint: '只读检索', to: '/m/security/login', status: 'ready', perm: 'system:log:login' },
      { key: 'security-events', label: '安全事件', hint: '只读检索', to: '/m/security/events', status: 'ready', perm: 'system:security:list' },
      { key: 'operation-log', label: '操作日志', hint: '只读检索', to: '/m/security/operation', status: 'ready', perm: 'system:log:operation' },
      { key: 'audit-log', label: '审计日志', hint: '只读检索', to: '/m/security/audit', status: 'ready', perm: 'system:log:audit' },
    ],
  },
  {
    key: 'org',
    title: '组织（只读）',
    tiles: [
      { key: 'users', label: '用户', hint: '查人，编辑回电脑', to: '/m/directory/users', status: 'ready', perm: 'system:user:list' },
      { key: 'roles', label: '角色', hint: '查看角色', to: '/m/directory/roles', status: 'ready', perm: 'system:role:list' },
      { key: 'depts', label: '部门', hint: '查看组织', to: '/m/directory/depts', status: 'ready', perm: 'system:department:list' },
      { key: 'tenants', label: '租户', hint: '查看租户', to: '/m/directory/tenants', status: 'ready', perm: 'system:tenant:list' },
    ],
  },
  {
    key: 'system',
    title: '系统（只读）',
    tiles: [
      { key: 'files', label: '文件', hint: '查看附件', to: '/m/catalog/files', status: 'ready', perm: 'system:file:list' },
      { key: 'dicts', label: '字典', hint: '查看字典', to: '/m/catalog/dicts', status: 'ready', perm: 'system:dict:list' },
    ],
  },
]

export function mobileTitleKey(path: string): string {
  if (path.startsWith('/m/tasks/')) return '审批详情'
  if (path.startsWith('/m/tasks')) return '待办审批'
  if (path.startsWith('/m/workbench')) return '工作台'
  if (path === '/m/me/password') return '修改密码'
  if (path === '/m/me/totp') return '二次验证'
  if (path === '/m/me/logins') return '最近登录'
  if (path.startsWith('/m/me')) return '我的'
  if (
    path.startsWith('/m/ops/')
    || path.startsWith('/m/security/')
    || path.startsWith('/m/directory/')
    || path.startsWith('/m/catalog/')
    || path.startsWith('/m/notices')
  ) {
    const tile = WORKBENCH_GROUPS.flatMap((group) => group.tiles).find((item) => item.to === path)
    if (tile) return tile.label
    if (path.startsWith('/m/directory/')) return '组织（只读）'
    if (path.startsWith('/m/catalog/')) return '系统（只读）'
    if (path.startsWith('/m/notices')) return '公告'
    return path.startsWith('/m/security/') ? '安全' : '运行'
  }
  if (path.startsWith('/m/soon/')) {
    for (const group of WORKBENCH_GROUPS) {
      const tile = group.tiles.find((item) => item.to === path)
      if (tile) return tile.label
    }
    return '即将开通'
  }
  return '态势'
}
