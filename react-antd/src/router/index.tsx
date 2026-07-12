import { lazy, Suspense, type ComponentType } from 'react'
import { Navigate, type RouteObject } from 'react-router-dom'
import { Spin } from 'antd'
import MainLayout from '@/layouts/MainLayout'

function lazyLoad(factory: () => Promise<{ default: ComponentType }>) {
  const Comp = lazy(factory)
  return (
    <Suspense fallback={<div style={{ display: 'flex', justifyContent: 'center', paddingTop: 100 }}><Spin size="large" /></div>}>
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
      { path: 'system/notice', element: lazyLoad(() => import('@/pages/system/notice')) },
      { path: 'system/online-user', element: lazyLoad(() => import('@/pages/system/online-user')) },
      { path: 'system/setting', element: lazyLoad(() => import('@/pages/system/setting')) },

      // Monitor
      { path: 'monitor/server', element: lazyLoad(() => import('@/pages/monitor/server')) },
      { path: 'monitor/mysql', element: lazyLoad(() => import('@/pages/monitor/mysql')) },
      { path: 'monitor/redis', element: lazyLoad(() => import('@/pages/monitor/redis')) },
      { path: 'monitor/job', element: lazyLoad(() => import('@/pages/monitor/job')) },

      // Error pages
      { path: '403', element: lazyLoad(() => import('@/pages/result/403')) },
      { path: '404', element: lazyLoad(() => import('@/pages/result/404')) },
      { path: '500', element: lazyLoad(() => import('@/pages/result/500')) },
      { path: '*', element: <Navigate to="/404" replace /> },
    ],
  },
]

export default routes
