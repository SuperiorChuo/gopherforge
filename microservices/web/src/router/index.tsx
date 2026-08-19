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
    path: '/',
    element: lazyLoad(importMainLayout),
    children: [
      { index: true, element: <Navigate to="/dashboard" replace /> },
      { path: 'dashboard', element: lazyLoad(() => import('@/pages/dashboard'), [routeStyles.dashboard]) },
      // 种子里子项是 /dashboard/index，只因 MainLayout 把「单子项容器」折叠成
      // 容器路径才一直没暴露。管理员给该容器加第二个子菜单就不再折叠，菜单会
      // 按子项真实路径跳转——没有这条路由就当场 404。
      { path: 'dashboard/index', element: lazyLoad(() => import('@/pages/dashboard'), [routeStyles.dashboard]) },
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
      { path: 'system/login-log', element: lazyLoad(() => import('@/pages/system/login-log'), [routeStyles.logs]) },
      { path: 'system/operation-log', element: lazyLoad(() => import('@/pages/system/operation-log'), [routeStyles.logs]) },
      { path: 'system/audit-log', element: lazyLoad(() => import('@/pages/system/audit-log'), [routeStyles.logs]) },
      { path: 'system/security-events', element: lazyLoad(() => import('@/pages/system/security-events')) },
      { path: 'system/login-security', element: lazyLoad(() => import('@/pages/system/login-security'), [routeStyles.dashboard]) },
      { path: 'system/notice', element: lazyLoad(() => import('@/pages/system/notice')) },
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

      // BPM (bpm-service) 审批中心
      { path: 'bpm/start', element: lazyLoad(() => import('@/pages/bpm/start')) },
      { path: 'bpm/definitions', element: lazyLoad(() => import('@/pages/bpm/definitions')) },
      { path: 'bpm/tasks', element: lazyLoad(() => import('@/pages/bpm/tasks')) },
      { path: 'bpm/instances', element: lazyLoad(() => import('@/pages/bpm/instances')) },

      // Monitor
      { path: 'monitor/server', element: lazyLoad(() => import('@/pages/monitor/server'), [routeStyles.monitor]) },
      { path: 'monitor/mysql', element: lazyLoad(() => import('@/pages/monitor/mysql'), [routeStyles.monitor]) },
      { path: 'monitor/redis', element: lazyLoad(() => import('@/pages/monitor/redis'), [routeStyles.monitor]) },
      { path: 'monitor/job', element: lazyLoad(() => import('@/pages/monitor/job'), [routeStyles.dashboard, routeStyles.logs]) },
      { path: 'monitor/alerts', element: lazyLoad(() => import('@/pages/monitor/alerts')) },
      { path: 'monitor/jaeger', element: lazyLoad(() => import('@/pages/monitor/jaeger')) },

      // Error pages
      { path: '403', element: lazyLoad(() => import('@/pages/result/403')) },
      { path: '404', element: lazyLoad(() => import('@/pages/result/404')) },
      { path: '500', element: lazyLoad(() => import('@/pages/result/500')) },
      { path: '*', element: <Navigate to="/404" replace /> },
    ],
  },
]

export default routes
