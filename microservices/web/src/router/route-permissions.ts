// 页面路由 → 后端权限码（与 server/internal/api/routes.go 的 PermissionMiddleware 对应）
// 未列出的路由（仪表盘、个人中心、结果页）不需要权限
export const ROUTE_PERMISSIONS: Record<string, string> = {
  '/system/user': 'system:user:list',
  '/system/role': 'system:role:list',
  '/system/permission': 'system:permission:list',
  '/system/menu': 'system:menu:list',
  '/system/department': 'system:department:list',
  '/system/dict': 'system:dict:list',
  '/system/file': 'system:file:list',
  '/system/notice': 'system:notice:list',
  '/system/login-log': 'system:log:login',
  '/system/operation-log': 'system:log:operation',
  '/system/audit-log': 'system:log:audit',
  '/system/online-user': 'system:online-user:list',
  '/system/setting': 'system:setting:list',
  '/system/tenant': 'system:tenant:list',
  '/system/oauth2': 'system:oauth2-client:list',
  '/monitor/server': 'system:monitor:server',
  '/monitor/mysql': 'system:monitor:mysql',
  '/monitor/redis': 'system:monitor:redis',
  '/monitor/job': 'system:job:list',
}
