// 页面路由 → 后端权限码，与 system-service 的菜单种子（menu_seed.go 的
// defaultMenuSeed）及后续菜单迁移逐条对应。
//
// 为什么要这张手写表：`/user/menus` 下发的是**已按权限过滤**的树，用户无权的
// 菜单压根不在返回里，所以从菜单数据反推不出"该拦谁"——缺失既可能是无权限，
// 也可能是该菜单不存在。侧栏可见性可以用菜单自带的 perm，但路由守卫必须有这
// 张独立的表。防漂移由 system-service 的 route_permissions_test.go 兜住：种子里
// 带权限码的叶子若在此缺失，该测试转红。
//
// 未列出即"登录可见"；个人收件箱、AI 助手等自助页不设管理权限。
export const ROUTE_PERMISSIONS: Record<string, string> = {
  // 组织与权限
  '/system/user': 'system:user:list',
  '/system/role': 'system:role:list',
  '/system/permission': 'system:permission:list',
  '/system/permission-diagnostics': 'system:permission:diagnose',
  '/system/menu': 'system:menu:list',
  '/system/department': 'system:department:list',
  '/system/post': 'system:post:list',
  '/system/tenant': 'system:tenant:list',
  '/system/tenant-packages': 'system:tenant-package:list',
  '/system/setting': 'system:setting:list',
  '/system/edge-certs': 'system:edge-cert:list',
  // 消息中心（个人站内信登录即可见）
	'/system/notice': 'system:notice:list',
	'/system/notify-templates': 'notify:template:read',
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
  '/system/webhooks': 'system:webhook:list',
	'/system/ai-models': 'ai:model:list',
	'/system/ai-usage': 'ai:usage:list',
	'/system/ai-quotas': 'ai:quota:list',
	// 业务域
	'/im/desk': 'im:agent:access',
	'/im/sites': 'im:admin:manage',
	'/im/skills': 'im:admin:manage',
	'/im/stats': 'im:agent:access',
	'/im/kb-misses': 'im:agent:access',
	'/im/low-ratings': 'im:agent:access',
	'/im/sensitive': 'im:admin:manage',
	'/ticket': 'ticket:read',
	'/crm/dashboard': 'crm:read',
	'/crm/leads': 'crm:read',
	'/crm/customers': 'crm:read',
	'/crm/pipeline': 'crm:read',
	'/crm/contracts': 'crm:read',
	'/crm/pool-rules': 'crm:read',
	'/cc/workbench': 'cc:read',
	'/cc/calls': 'cc:read',
	'/cc/console': 'cc:read',
	'/cc/extensions': 'cc:read',
	'/cc/recordings': 'cc:read',
	'/cc/trunks': 'cc:read',
	'/cc/ivr': 'cc:read',
	'/cc/queues': 'cc:read',
	'/cc/moh': 'cc:read',
	'/cc/blacklist': 'cc:read',
	'/cc/prompts': 'cc:read',
	'/cc/conference': 'cc:read',
	'/cc/monitor': 'cc:read',
	'/cc/voicemail': 'cc:read',
	'/cc/timeroutes': 'cc:read',
	'/cc/ringgroups': 'cc:read',
	'/cc/reports': 'cc:read',
	'/cc/billing': 'cc:read',
	'/cc/qc': 'cc:read',
	'/cc/voice': 'cc:read',
	'/visibility/overview': 'visibility:read',
	'/visibility/brands': 'visibility:read',
	'/visibility/seo': 'visibility:read',
	'/visibility/geo': 'visibility:read',
	'/visibility/geo-compare': 'visibility:read',
	'/mp/accounts': 'social:read',
	'/mp/articles': 'social:read',
	'/mp/messages': 'social:read',
	'/mp/fans': 'social:read',
	'/mp/menu': 'social:read',
	'/social/content': 'social:read',
	'/social/publish': 'social:read',
  // 监控（/monitor/grafana 种子无权限码，不拦）
  '/monitor/server': 'system:monitor:server',
  '/monitor/mysql': 'system:monitor:mysql',
  '/monitor/redis': 'system:monitor:redis',
  '/monitor/job': 'system:job:list',
  '/monitor/alerts': 'system:alert:list',
  // 审批流：只有流程定义（设计器入口）要权限，我的待办/发起等不拦
  '/bpm/definitions': 'bpm:definition:list',
  // 支付中心：种子里是「/pay 容器 + /pay/index 单子项」，MainLayout 会把这种
  // 单子项容器折叠成叶子，实际落地路由是 /pay。两个键都留：守卫按真实路径 /pay
  // 生效，/pay/index 则保证折叠逻辑失效（管理员给 /pay 加第二个子菜单）时不裸奔。
	'/pay': 'pay:read',
	'/pay/index': 'pay:read',
  // Hermes 集群。/hermes/workbench 是员工自助页，所有登录用户可访问，故不设权限。
  '/hermes/overview': 'ai:hermes:device:list',
  '/hermes/office': 'ai:hermes:office:read',
  '/hermes/tasks': 'ai:hermes:device:list',
  '/hermes/operations': 'ai:hermes:device:list',
  '/hermes/ai-ops': 'ai:hermes:device:list',
  '/hermes/runtimes': 'ai:hermes:device:list',
  '/hermes/collaborations': 'ai:hermes:task:list',
  '/hermes/cron': 'ai:hermes:cron:list',
  '/hermes/skills-market': 'ai:hermes:skill:list',
  '/hermes/mcp-market': 'ai:hermes:mcp:list',
  '/hermes/instances': 'ai:hermes:instance:list',
  '/hermes/clusters': 'ai:hermes:cluster:list',
  '/hermes/models': 'ai:hermes:policy:read',
  '/hermes/bridges': 'ai:hermes:policy:read',
  '/hermes/skills': 'ai:hermes:policy:read',
  // 旧书签在 Navigate 生效前仍经过 MainLayout，保留同口径守卫。
  '/hermes/entitlements': 'ai:hermes:entitlement',
  '/hermes/devices': 'ai:hermes:device:list',
  '/hermes/policies': 'ai:hermes:policy:read',
  '/hermes/artifacts': 'ai:hermes:policy:read',
}

// 种子里带权限码、但**刻意不做路由守卫**的页面，附理由。
// route_permissions_test.go 读这张表放行，新增豁免必须在这里写明原因。
export const ROUTE_PERMISSION_EXEMPT: Record<string, string> = {
  // 登录后的落地页。种子给了 dashboard.view，但守卫它意味着角色少这个码就
  // 会在首页吃 403、连不到任何页面；可见性已由后端菜单过滤处理。
  '/dashboard/index': '登录落地页，守卫会导致无 dashboard.view 的角色进不了系统',
}
