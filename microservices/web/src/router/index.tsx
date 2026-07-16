import { lazy, Suspense, type ComponentType } from 'react'
import { Navigate, type RouteObject } from 'react-router-dom'
import MainLayout from '@/layouts/MainLayout'

// 路由懒加载兜底:玻璃卡片骨架,比孤零零的 Spin 更接近成品布局
function RouteFallback() {
  return (
    <div className="route-fallback">
      <div className="route-fallback-bar" />
      <div className="route-fallback-card" />
    </div>
  )
}

function lazyLoad(factory: () => Promise<{ default: ComponentType }>) {
  const Comp = lazy(factory)
  return (
    <Suspense fallback={<RouteFallback />}>
      <Comp />
    </Suspense>
  )
}

const routes: RouteObject[] = [
  {
    path: '/login',
    element: lazyLoad(() => import('@/pages/login')),
  },
  {
    path: '/',
    element: <MainLayout />,
    children: [
      { index: true, element: <Navigate to="/dashboard" replace /> },
      { path: 'dashboard', element: lazyLoad(() => import('@/pages/dashboard')) },
      { path: 'profile', element: lazyLoad(() => import('@/pages/profile')) },

      // System
      { path: 'system/user', element: lazyLoad(() => import('@/pages/system/user')) },
      { path: 'system/role', element: lazyLoad(() => import('@/pages/system/role')) },
      { path: 'system/permission', element: lazyLoad(() => import('@/pages/system/permission')) },
      { path: 'system/menu', element: lazyLoad(() => import('@/pages/system/menu')) },
      { path: 'system/department', element: lazyLoad(() => import('@/pages/system/department')) },
      { path: 'system/dict', element: lazyLoad(() => import('@/pages/system/dict')) },
      { path: 'system/file', element: lazyLoad(() => import('@/pages/system/file')) },
      { path: 'system/login-log', element: lazyLoad(() => import('@/pages/system/login-log')) },
      { path: 'system/operation-log', element: lazyLoad(() => import('@/pages/system/operation-log')) },
      { path: 'system/audit-log', element: lazyLoad(() => import('@/pages/system/audit-log')) },
      { path: 'system/notice', element: lazyLoad(() => import('@/pages/system/notice')) },
      { path: 'system/online-user', element: lazyLoad(() => import('@/pages/system/online-user')) },
      { path: 'system/setting', element: lazyLoad(() => import('@/pages/system/setting')) },
      { path: 'system/tenant', element: lazyLoad(() => import('@/pages/system/tenant')) },

      // Monitor
      { path: 'monitor/server', element: lazyLoad(() => import('@/pages/monitor/server')) },
      { path: 'monitor/mysql', element: lazyLoad(() => import('@/pages/monitor/mysql')) },
      { path: 'monitor/redis', element: lazyLoad(() => import('@/pages/monitor/redis')) },
      { path: 'monitor/job', element: lazyLoad(() => import('@/pages/monitor/job')) },

      // AI (ai-service)
      { path: 'ai/assistant', element: lazyLoad(() => import('@/pages/ai/assistant')) },
      { path: 'ai/knowledge', element: lazyLoad(() => import('@/pages/ai/knowledge')) },

      // IM (im-service)
      { path: 'im/desk', element: lazyLoad(() => import('@/pages/im/desk')) },
      { path: 'im/sites', element: lazyLoad(() => import('@/pages/im/sites')) },
      { path: 'im/skills', element: lazyLoad(() => import('@/pages/im/skills')) },

      // Error pages
      { path: '403', element: lazyLoad(() => import('@/pages/result/403')) },
      { path: '404', element: lazyLoad(() => import('@/pages/result/404')) },
      { path: '500', element: lazyLoad(() => import('@/pages/result/500')) },
      { path: '*', element: <Navigate to="/404" replace /> },
    ],
  },
]

export default routes
