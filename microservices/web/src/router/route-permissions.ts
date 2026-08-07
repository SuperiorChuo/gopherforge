// 页面路由 → 后端权限码，与 system-service 的菜单种子（menu_seed.go 的
// defaultMenuSeed）及后续菜单迁移逐条对应。
//
// 为什么要这张手写表：`/user/menus` 下发的是**已按权限过滤**的树，用户无权的
// 菜单压根不在返回里，所以从菜单数据反推不出"该拦谁"——缺失既可能是无权限，
// 也可能是该菜单不存在。侧栏可见性可以用菜单自带的 perm，但路由守卫必须有这
// 张独立的表。防漂移由 system-service 的 route_permissions_test.go 兜住：种子里
// 带权限码的叶子若在此缺失，该测试转红。
//
// 未列出即"登录可见"：种子里不带权限码的叶子按设计登录即可用，不该被守卫拦。
export const ROUTE_PERMISSIONS: Record<string, string> = {
  // 组织与权限
  '/system/user': 'system:user:list',
  '/system/role': 'system:role:list',
  '/system/permission': 'system:permission:list',
  '/system/menu': 'system:menu:list',
  '/system/department': 'system:department:list',
  '/system/post': 'system:post:list',
  '/system/tenant': 'system:tenant:list',
  '/system/tenant-packages': 'system:tenant-package:list',
  '/system/setting': 'system:setting:list',
  // 消息中心（站内信与通知模板无权限码，登录即可见）
  '/system/notice': 'system:notice:list',
  '/system/sms': 'system:sms-channel:list',
  // 日志审计
  '/system/operation-log': 'system:log:operation',
  '/system/login-log': 'system:log:login',
  '/system/audit-log': 'system:log:audit',
  '/system/security-events': 'system:security:list',
  '/system/login-security': 'system:login-security:list',
  '/system/online-user': 'system:online-user:list',
  // 系统工具
  '/system/codegen': 'system:codegen:list',
  '/system/dict': 'system:dict:list',
  '/system/file': 'system:file:list',
  '/system/errcodes': 'system:errcode:list',
  '/system/oauth2': 'system:oauth2-client:list',
  // 监控（/monitor/grafana 种子无权限码，不拦）
  '/monitor/server': 'system:monitor:server',
  '/monitor/mysql': 'system:monitor:mysql',
  '/monitor/redis': 'system:monitor:redis',
  '/monitor/job': 'system:job:list',
  '/monitor/alerts': 'system:alert:list',
  // 审批流：只有流程定义（设计器入口）要权限，我的待办/发起等不拦
  '/bpm/definitions': 'bpm:definition:list',
}

// 种子里带权限码、但**刻意不做路由守卫**的页面，附理由。
// route_permissions_test.go 读这张表放行，新增豁免必须在这里写明原因。
export const ROUTE_PERMISSION_EXEMPT: Record<string, string> = {
  // 登录后的落地页。种子给了 dashboard.view，但守卫它意味着角色少这个码就
  // 会在首页吃 403、连不到任何页面；可见性已由后端菜单过滤处理。
  '/dashboard/index': '登录落地页，守卫会导致无 dashboard.view 的角色进不了系统',
}
