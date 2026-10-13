import { lazy, Suspense, type ComponentType } from 'react'
import { Navigate, type RouteObject } from 'react-router-dom'

// 管理台骨架与页面一样懒加载：静态引它会让登录页背上整个 MainLayout 依赖树
// （Layout/Menu/Dropdown 等 antd 组件），而登录页一个都用不到。
// 工厂只此一处，预取与路由渲染共用同一模块缓存。
const importMainLayout = () => import('@/layouts/MainLayout')

type RouteStyleLoader = () => Promise<unknown>

const routeStyles = {
  auth: () => import('@/styles/routes/auth'),
  dashboard: () => import('@/styles/routes/dashboard'),
  profile: () => import('@/styles/routes/profile'),
  monitor: () => import('@/styles/routes/monitor'),
  monitorCore: () => import('@/styles/routes/monitor-core'),
  logs: () => import('@/styles/routes/logs'),
  file: () => import('@/styles/routes/file'),
  systemAI: () => import('@/styles/routes/system-ai'),
  hermesOffice: () => import('@/styles/routes/hermes-office'),
  im: () => import('@/styles/routes/im'),
  ai: () => import('@/styles/routes/ai'),
  cc: () => import('@/styles/routes/cc'),
  ticket: () => import('@/styles/routes/ticket'),
} satisfies Record<string, RouteStyleLoader>

// 登录页挂载后空闲预取骨架：用户填账号密码的几秒足够下完，登录跳转时模块已在
// 缓存里，抵消懒加载带来的首跳等待。返回取消函数供 effect 清理。
export function prefetchMainLayout(): () => void {
  const run = () => {
    void importMainLayout()
  }
  if (typeof window !== 'undefined' && typeof window.requestIdleCallback === 'function') {
    const handle = window.requestIdleCallback(run, { timeout: 2000 })
    return () => window.cancelIdleCallback?.(handle)
  }
  const timer = setTimeout(run, 300)
  return () => clearTimeout(timer)
}

// 路由懒加载兜底:玻璃卡片骨架,比孤零零的 Spin 更接近成品布局
function RouteFallback() {
  return (
    <div className="route-fallback">
      <div className="route-fallback-bar" />
      <div className="route-fallback-card" />
    </div>
  )
}

function lazyLoad(
  factory: () => Promise<{ default: ComponentType }>,
  styles: RouteStyleLoader[] = [],
) {
  const Comp = lazy(
    styles.length
      ? async () => {
          const [page] = await Promise.all([factory(), ...styles.map((loadStyle) => loadStyle())])
          return page
        }
      : factory,
  )
  return (
    <Suspense fallback={<RouteFallback />}>
      <Comp />
    </Suspense>
  )
}

const routes: RouteObject[] = [
  {
    path: '/login',
    element: lazyLoad(() => import('@/pages/login'), [routeStyles.auth]),
  },
  {
    // 邀请注册页：独立于 MainLayout（未登录），邀请链接 /register?invite=<token>
    path: '/register',
    element: lazyLoad(() => import('@/pages/register'), [routeStyles.auth]),
  },
  {
    // 忘记密码 / 重置密码：独立于 MainLayout（未登录）
    path: '/forgot-password',
    element: lazyLoad(() => import('@/pages/forgot-password'), [routeStyles.auth]),
  },
  {
    path: '/reset-password',
    element: lazyLoad(() => import('@/pages/reset-password'), [routeStyles.auth]),
  },
  {
    // OAuth2 授权确认页：面向第三方授权流程，刻意放在 MainLayout 之外
    // （第三方用户不应看到管理台骨架），与 /login 平级
    path: '/oauth/authorize',
    element: lazyLoad(() => import('@/pages/oauth/authorize'), [routeStyles.auth]),
  },
  {
    // 收银台：匿名支付页（order_token 即凭据），面向付款用户，
    // 刻意放在 MainLayout 之外（付款人不应看到管理台骨架）
    path: '/cashier/:token',
    element: lazyLoad(() => import('@/pages/cashier')),
  },
  {
    // 管理员移动端登录：复用桌面登录页（账密 / 验证码 / TOTP），
    // 默认回跳 /m，不预取桌面 MainLayout。
    path: '/m/login',
    element: lazyLoad(() => import('@/pages/login'), [routeStyles.auth]),
  },
  {
    path: '/m',
    element: lazyLoad(() => import('@/pages/mobile/MobileLayout')),
    children: [
      { index: true, element: lazyLoad(() => import('@/pages/mobile/home')) },
      { path: 'workbench', element: lazyLoad(() => import('@/pages/mobile/workbench')) },
      { path: 'inbox', element: lazyLoad(() => import('@/pages/mobile/inbox')) },
      { path: 'me', element: lazyLoad(() => import('@/pages/mobile/me')) },
      { path: 'tasks', element: lazyLoad(() => import('@/pages/mobile/tasks')) },
      { path: 'tasks/:id', element: lazyLoad(() => import('@/pages/mobile/tasks')) },
      { path: 'ops/health', element: lazyLoad(() => import('@/pages/mobile/ops/health')) },
      { path: 'ops/alerts', element: lazyLoad(() => import('@/pages/mobile/ops/alerts')) },
      { path: 'ops/jobs', element: lazyLoad(() => import('@/pages/mobile/ops/jobs')) },
      { path: 'ops/online', element: lazyLoad(() => import('@/pages/mobile/ops/online')) },
      { path: 'security/login', element: lazyLoad(() => import('@/pages/mobile/security/login-log')) },
      { path: 'security/events', element: lazyLoad(() => import('@/pages/mobile/security/security-events')) },
      { path: 'security/operation', element: lazyLoad(() => import('@/pages/mobile/security/operation-log')) },
      { path: 'security/audit', element: lazyLoad(() => import('@/pages/mobile/security/audit-log')) },
      { path: 'directory/users', element: lazyLoad(() => import('@/pages/mobile/directory/users')) },
      { path: 'directory/roles', element: lazyLoad(() => import('@/pages/mobile/directory/roles')) },
      { path: 'directory/depts', element: lazyLoad(() => import('@/pages/mobile/directory/depts')) },
      { path: 'directory/tenants', element: lazyLoad(() => import('@/pages/mobile/directory/tenants')) },
      { path: 'catalog/files', element: lazyLoad(() => import('@/pages/mobile/catalog/files')) },
      { path: 'catalog/dicts', element: lazyLoad(() => import('@/pages/mobile/catalog/dicts')) },
      { path: 'catalog/templates', element: lazyLoad(() => import('@/pages/mobile/catalog/templates')) },
      { path: 'notices', element: lazyLoad(() => import('@/pages/mobile/notices')) },
      { path: 'me/password', element: lazyLoad(() => import('@/pages/mobile/me/password')) },
      { path: 'me/totp', element: lazyLoad(() => import('@/pages/mobile/me/totp')) },
      { path: 'me/logins', element: lazyLoad(() => import('@/pages/mobile/me/logins')) },
      { path: 'soon/:key', element: lazyLoad(() => import('@/pages/mobile/soon')) },
      { path: '*', element: <Navigate to="/m" replace /> },
    ],
  },
  {
    path: '/',
    element: lazyLoad(importMainLayout),
    children: [
      { index: true, element: <Navigate to="/dashboard" replace /> },
      { path: 'dashboard', element: lazyLoad(() => import('@/pages/dashboard'), [routeStyles.dashboard, routeStyles.cc]) },
      // 种子里子项是 /dashboard/index，只因 MainLayout 把「单子项容器」折叠成
      // 容器路径才一直没暴露。管理员给该容器加第二个子菜单就不再折叠，菜单会
      // 按子项真实路径跳转——没有这条路由就当场 404。
      { path: 'dashboard/index', element: lazyLoad(() => import('@/pages/dashboard'), [routeStyles.dashboard, routeStyles.cc]) },
      { path: 'profile', element: lazyLoad(() => import('@/pages/profile'), [routeStyles.profile]) },

      // System
      { path: 'system/user', element: lazyLoad(() => import('@/pages/system/user')) },
      { path: 'system/role', element: lazyLoad(() => import('@/pages/system/role')) },
      { path: 'system/permission', element: lazyLoad(() => import('@/pages/system/permission')) },
      { path: 'system/permission-diagnostics', element: lazyLoad(() => import('@/pages/system/permission-diagnostics')) },
      { path: 'system/menu', element: lazyLoad(() => import('@/pages/system/menu')) },
      { path: 'system/department', element: lazyLoad(() => import('@/pages/system/department')) },
      { path: 'system/dict', element: lazyLoad(() => import('@/pages/system/dict')) },
      { path: 'system/file', element: lazyLoad(() => import('@/pages/system/file'), [routeStyles.file]) },
      { path: 'system/login-log', element: lazyLoad(() => import('@/pages/system/login-log'), [routeStyles.logs, routeStyles.im]) },
      { path: 'system/operation-log', element: lazyLoad(() => import('@/pages/system/operation-log'), [routeStyles.logs]) },
      { path: 'system/audit-log', element: lazyLoad(() => import('@/pages/system/audit-log'), [routeStyles.logs]) },
      { path: 'system/security-events', element: lazyLoad(() => import('@/pages/system/security-events')) },
      { path: 'system/login-security', element: lazyLoad(() => import('@/pages/system/login-security'), [routeStyles.dashboard]) },
      { path: 'system/notice', element: lazyLoad(() => import('@/pages/system/notice')) },
      { path: 'system/inbox', element: lazyLoad(() => import('@/pages/system/inbox')) },
      { path: 'system/notify-templates', element: lazyLoad(() => import('@/pages/system/notify-templates')) },
      { path: 'system/online-user', element: lazyLoad(() => import('@/pages/system/online-user'), [routeStyles.dashboard, routeStyles.monitorCore]) },
      { path: 'system/setting', element: lazyLoad(() => import('@/pages/system/setting')) },
      { path: 'system/edge-certs', element: lazyLoad(() => import('@/pages/system/edge-certs')) },
      { path: 'system/tenant', element: lazyLoad(() => import('@/pages/system/tenant')) },
      { path: 'system/codegen', element: lazyLoad(() => import('@/pages/system/codegen')) },
      { path: 'system/sms', element: lazyLoad(() => import('@/pages/system/sms')) },
      { path: 'system/oauth2', element: lazyLoad(() => import('@/pages/system/oauth2')) },
      { path: 'system/webhooks', element: lazyLoad(() => import('@/pages/system/webhooks')) },
      { path: 'system/errcodes', element: lazyLoad(() => import('@/pages/system/errcodes')) },
      { path: 'system/post', element: lazyLoad(() => import('@/pages/system/posts')) },
      { path: 'system/tenant-packages', element: lazyLoad(() => import('@/pages/system/tenant-packages')) },
      { path: 'system/ai-models', element: lazyLoad(() => import('@/pages/system/ai-models'), [routeStyles.systemAI]) },
      { path: 'system/ai-usage', element: lazyLoad(() => import('@/pages/system/ai-usage'), [routeStyles.systemAI]) },
      { path: 'system/ai-quotas', element: lazyLoad(() => import('@/pages/system/ai-quotas'), [routeStyles.systemAI]) },

      // Monitor
      { path: 'monitor/server', element: lazyLoad(() => import('@/pages/monitor/server'), [routeStyles.monitor]) },
      { path: 'monitor/mysql', element: lazyLoad(() => import('@/pages/monitor/mysql'), [routeStyles.monitor]) },
      { path: 'monitor/redis', element: lazyLoad(() => import('@/pages/monitor/redis'), [routeStyles.monitor]) },
      { path: 'monitor/jaeger', element: lazyLoad(() => import('@/pages/monitor/jaeger')) },
      { path: 'monitor/grafana', element: lazyLoad(() => import('@/pages/monitor/grafana')) },
      { path: 'monitor/job', element: lazyLoad(() => import('@/pages/monitor/job'), [routeStyles.dashboard, routeStyles.logs]) },
      { path: 'monitor/alerts', element: lazyLoad(() => import('@/pages/monitor/alerts')) },

      // AI (ai-service)
      { path: 'ai/assistant', element: lazyLoad(() => import('@/pages/ai/assistant'), [routeStyles.ai]) },
      { path: 'ai/knowledge', element: lazyLoad(() => import('@/pages/ai/knowledge'), [routeStyles.ai]) },
      { path: 'ai/agents', element: lazyLoad(() => import('@/pages/ai/agents'), [routeStyles.ai]) },

      // Hermes 集群（顶级模块，按总览 / 协同运维 / 资源中心三组组织）
      //
      // /hermes 自身要可路由：apiMenusToDefs 对「只剩一个可见子项」的顶级容器会
      // 折叠成指向容器路径的叶子（MainLayout.tsx 内 kids.length===1 分支）。普通
      // 员工只看得到「我的 Hermes」，正好触发折叠，点击会落到 /hermes——没有这条
      // 重定向就是空白页。重定向到工作台，因为它是唯一无需权限的子页。
      { path: 'hermes', element: <Navigate to="/hermes/workbench" replace /> },
      { path: 'hermes/overview', element: lazyLoad(() => import('@/pages/hermes/overview')) },
      { path: 'hermes/office', element: lazyLoad(() => import('@/pages/hermes/office'), [routeStyles.hermesOffice]) },
      { path: 'hermes/workbench', element: lazyLoad(() => import('@/pages/hermes/mine'), [routeStyles.dashboard]) },
      { path: 'hermes/tasks', element: lazyLoad(() => import('@/pages/hermes/tasks')) },
      { path: 'hermes/operations', element: lazyLoad(() => import('@/pages/hermes/devices')) },
      { path: 'hermes/ai-ops', element: lazyLoad(() => import('@/pages/hermes/ai-ops')) },
      { path: 'hermes/runtimes', element: lazyLoad(() => import('@/pages/hermes/runtimes')) },
      { path: 'hermes/collaborations', element: lazyLoad(() => import('@/pages/hermes/collaborations')) },
      { path: 'hermes/cron', element: lazyLoad(() => import('@/pages/hermes/cron')) },
      { path: 'hermes/skills-market', element: lazyLoad(() => import('@/pages/hermes/skills-market')) },
      { path: 'hermes/mcp-market', element: lazyLoad(() => import('@/pages/hermes/mcp-market')) },
      { path: 'hermes/instances', element: lazyLoad(() => import('@/pages/hermes/instances')) },
      { path: 'hermes/clusters', element: lazyLoad(() => import('@/pages/hermes/clusters')) },
      { path: 'hermes/models', element: lazyLoad(() => import('@/pages/hermes/policies')) },
      { path: 'hermes/bridges', element: lazyLoad(() => import('@/pages/hermes/artifacts')) },
      { path: 'hermes/skills', element: lazyLoad(() => import('@/pages/hermes/skills')) },

      // 旧路径保留重定向，避免已保存的书签与外链失效。
      { path: 'hermes/mine', element: <Navigate to="/hermes/workbench" replace /> },
      { path: 'hermes/devices', element: <Navigate to="/hermes/operations" replace /> },
      // 员工权益有了自己的入口：/hermes/instances 现在真的管 Agent 实例
      { path: 'hermes/entitlements', element: lazyLoad(() => import('@/pages/hermes/entitlements')) },
      { path: 'hermes/policies', element: <Navigate to="/hermes/models" replace /> },
      { path: 'hermes/artifacts', element: <Navigate to="/hermes/bridges" replace /> },

      // IM (im-service)
      { path: 'im/desk', element: lazyLoad(() => import('@/pages/im/desk'), [routeStyles.im]) },
      { path: 'im/sites', element: lazyLoad(() => import('@/pages/im/sites')) },
      { path: 'im/skills', element: lazyLoad(() => import('@/pages/im/skills')) },
      { path: 'im/stats', element: lazyLoad(() => import('@/pages/im/stats')) },
      { path: 'im/kb-misses', element: lazyLoad(() => import('@/pages/im/kb-misses')) },
      { path: 'im/low-ratings', element: lazyLoad(() => import('@/pages/im/low-ratings')) },
      { path: 'im/sensitive', element: lazyLoad(() => import('@/pages/im/sensitive')) },

      // Ticket (ticket-service) 工单 · 挂智能客服闭环
      { path: 'ticket', element: lazyLoad(() => import('@/pages/ticket'), [routeStyles.ticket]) },

      // Pay (pay-service) 支付中心
      { path: 'pay', element: lazyLoad(() => import('@/pages/pay')) },
      // 同 dashboard：种子子项为 /pay/index，折叠失效时才会走到，补齐防 404
      { path: 'pay/index', element: lazyLoad(() => import('@/pages/pay')) },

      // CC (cc-service)
      { path: 'cc/workbench', element: lazyLoad(() => import('@/pages/cc/workbench'), [routeStyles.cc, routeStyles.im]) },
      { path: 'cc/calls', element: lazyLoad(() => import('@/pages/cc/calls'), [routeStyles.cc]) },
      { path: 'cc/console', element: lazyLoad(() => import('@/pages/cc/console'), [routeStyles.cc]) },
      { path: 'cc/monitor', element: lazyLoad(() => import('@/pages/cc/monitor'), [routeStyles.cc]) },
      { path: 'cc/extensions', element: lazyLoad(() => import('@/pages/cc/extensions'), [routeStyles.cc]) },
      { path: 'cc/recordings', element: lazyLoad(() => import('@/pages/cc/recordings'), [routeStyles.cc]) },
      { path: 'cc/voicemail', element: lazyLoad(() => import('@/pages/cc/voicemail'), [routeStyles.cc]) },
      { path: 'cc/trunks', element: lazyLoad(() => import('@/pages/cc/trunks'), [routeStyles.cc]) },
      { path: 'cc/ivr', element: lazyLoad(() => import('@/pages/cc/ivr'), [routeStyles.cc]) },
      { path: 'cc/queues', element: lazyLoad(() => import('@/pages/cc/queues'), [routeStyles.cc]) },
      { path: 'cc/moh', element: lazyLoad(() => import('@/pages/cc/moh'), [routeStyles.cc]) },
      { path: 'cc/prompts', element: lazyLoad(() => import('@/pages/cc/prompts'), [routeStyles.cc]) },
      { path: 'cc/blacklist', element: lazyLoad(() => import('@/pages/cc/blacklist'), [routeStyles.cc]) },
      { path: 'cc/conference', element: lazyLoad(() => import('@/pages/cc/conference'), [routeStyles.cc]) },
      { path: 'cc/timeroutes', element: lazyLoad(() => import('@/pages/cc/timeroutes'), [routeStyles.cc]) },
      { path: 'cc/ringgroups', element: lazyLoad(() => import('@/pages/cc/ringgroups'), [routeStyles.cc]) },
      { path: 'cc/reports', element: lazyLoad(() => import('@/pages/cc/reports'), [routeStyles.cc]) },
      { path: 'cc/billing', element: lazyLoad(() => import('@/pages/cc/billing'), [routeStyles.cc]) },
      { path: 'cc/qc', element: lazyLoad(() => import('@/pages/cc/qc'), [routeStyles.cc]) },
      { path: 'cc/voice', element: lazyLoad(() => import('@/pages/cc/voice'), [routeStyles.cc]) },

      // BPM (bpm-service) 审批中心
      { path: 'bpm/start', element: lazyLoad(() => import('@/pages/bpm/start')) },
      { path: 'bpm/definitions', element: lazyLoad(() => import('@/pages/bpm/definitions')) },
      { path: 'bpm/tasks', element: lazyLoad(() => import('@/pages/bpm/tasks')) },
      { path: 'bpm/instances', element: lazyLoad(() => import('@/pages/bpm/instances')) },

      // CRM (crm-service)
      { path: 'crm/dashboard', element: lazyLoad(() => import('@/pages/crm/dashboard'), [routeStyles.dashboard]) },
      { path: 'crm/tags', element: lazyLoad(() => import('@/pages/crm/tags')) },
      { path: 'crm/leads', element: lazyLoad(() => import('@/pages/crm/leads')) },
      { path: 'crm/customers', element: lazyLoad(() => import('@/pages/crm/customers')) },
      { path: 'crm/pipeline', element: lazyLoad(() => import('@/pages/crm/pipeline')) },
      { path: 'crm/contracts', element: lazyLoad(() => import('@/pages/crm/contracts')) },
      { path: 'crm/pool-rules', element: lazyLoad(() => import('@/pages/crm/pool-rules')) },

      // Visibility SEO + GEO
      // 菜单种子里概览页是 /visibility/overview（与容器 /visibility 区分）；
      // 旧路径保留为重定向，老书签与外链不断。
      { path: 'visibility', element: <Navigate to="/visibility/overview" replace /> },
      { path: 'visibility/overview', element: lazyLoad(() => import('@/pages/visibility/overview')) },
      { path: 'visibility/brands', element: lazyLoad(() => import('@/pages/visibility/brands'), [routeStyles.im]) },
      { path: 'visibility/seo', element: lazyLoad(() => import('@/pages/visibility/seo')) },
      { path: 'visibility/geo', element: lazyLoad(() => import('@/pages/visibility/geo'), [routeStyles.im]) },
      { path: 'visibility/geo-compare', element: lazyLoad(() => import('@/pages/visibility/geo-compare')) },

      // MP (mp-service) 公众号运营
      { path: 'mp/accounts', element: lazyLoad(() => import('@/pages/mp/accounts')) },
      { path: 'mp/articles', element: lazyLoad(() => import('@/pages/mp/articles')) },
      { path: 'mp/messages', element: lazyLoad(() => import('@/pages/mp/messages')) },
      { path: 'mp/fans', element: lazyLoad(() => import('@/pages/mp/fans')) },
      { path: 'mp/menu', element: lazyLoad(() => import('@/pages/mp/menu')) },

      // Social 社媒内容中台
      { path: 'social/content', element: lazyLoad(() => import('@/pages/social/content')) },
      { path: 'social/publish', element: lazyLoad(() => import('@/pages/social/publish')) },

      // Error pages
      { path: '403', element: lazyLoad(() => import('@/pages/result/403')) },
      { path: '404', element: lazyLoad(() => import('@/pages/result/404')) },
      { path: '500', element: lazyLoad(() => import('@/pages/result/500')) },
      { path: '*', element: <Navigate to="/404" replace /> },
    ],
  },
]

export default routes
