import type { RouteRecordRaw } from 'vue-router';

import { LAYOUT } from '@/utils/route/constant';

const hiddenMeta = {
  hidden: true,
};

const hiddenPermissionMeta = (permission: string) => ({
  ...hiddenMeta,
  permission,
});

const legacyFallbackRoutes: RouteRecordRaw[] = [
  {
    path: '/monitor',
    name: 'MonitorFallback',
    component: LAYOUT,
    redirect: '/monitor/redis',
    meta: hiddenMeta,
    children: [
      {
        path: 'redis',
        name: 'MonitorRedisFallback',
        component: () => import('@/pages/monitor/redis/index.vue'),
        meta: hiddenPermissionMeta('system:monitor:redis'),
      },
      {
        path: 'mysql',
        name: 'MonitorMysqlFallback',
        component: () => import('@/pages/monitor/mysql/index.vue'),
        meta: hiddenPermissionMeta('system:monitor:mysql'),
      },
      {
        path: 'server',
        name: 'MonitorServerFallback',
        component: () => import('@/pages/monitor/server/index.vue'),
        meta: hiddenPermissionMeta('system:monitor:server'),
      },
      {
        path: 'job',
        name: 'MonitorJobFallback',
        component: () => import('@/pages/monitor/job/index.vue'),
        meta: hiddenPermissionMeta('system:job:list'),
      },
    ],
  },
  {
    path: '/system',
    name: 'SystemFallback',
    component: LAYOUT,
    redirect: '/system/user',
    meta: hiddenMeta,
    children: [
      {
        path: 'user',
        name: 'SystemUserFallback',
        component: () => import('@/pages/system/user/index.vue'),
        meta: hiddenPermissionMeta('system:user:list'),
      },
      {
        path: 'role',
        name: 'SystemRoleFallback',
        component: () => import('@/pages/system/role/index.vue'),
        meta: hiddenPermissionMeta('system:role:list'),
      },
      {
        path: 'permission',
        name: 'SystemPermissionFallback',
        component: () => import('@/pages/system/permission/index.vue'),
        meta: hiddenPermissionMeta('system:permission:list'),
      },
      {
        path: 'menu',
        name: 'SystemMenuFallback',
        component: () => import('@/pages/system/menu/index.vue'),
        meta: hiddenPermissionMeta('system:menu:list'),
      },
      {
        path: 'department',
        name: 'SystemDepartmentFallback',
        component: () => import('@/pages/system/department/index.vue'),
        meta: hiddenPermissionMeta('system:department:list'),
      },
      {
        path: 'dict',
        name: 'SystemDictFallback',
        component: () => import('@/pages/system/dict/index.vue'),
        meta: hiddenPermissionMeta('system:dict:list'),
      },
      {
        path: 'file',
        name: 'SystemFileFallback',
        component: () => import('@/pages/system/file/index.vue'),
        meta: hiddenPermissionMeta('system:file:list'),
      },
      {
        path: 'login-log',
        name: 'SystemLoginLogFallback',
        component: () => import('@/pages/system/login-log/index.vue'),
        meta: hiddenPermissionMeta('system:log:login'),
      },
      {
        path: 'operation-log',
        name: 'SystemOperationLogFallback',
        component: () => import('@/pages/system/operation-log/index.vue'),
        meta: hiddenPermissionMeta('system:log:operation'),
      },
      {
        path: 'online-user',
        name: 'SystemOnlineUserFallback',
        component: () => import('@/pages/system/online-user/index.vue'),
        meta: hiddenPermissionMeta('system:online-user:list'),
      },
      {
        path: 'notice',
        name: 'SystemNoticeFallback',
        component: () => import('@/pages/system/notice/index.vue'),
        meta: hiddenPermissionMeta('system:notice:list'),
      },
      {
        path: 'setting',
        name: 'SystemSettingFallback',
        component: () => import('@/pages/system/setting/index.vue'),
        meta: hiddenPermissionMeta('system:setting:list'),
      },
    ],
  },
];

export default legacyFallbackRoutes;
