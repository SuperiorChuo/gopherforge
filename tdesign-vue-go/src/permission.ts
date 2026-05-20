import 'nprogress/nprogress.css'; // progress bar style

import NProgress from 'nprogress'; // progress bar
import { MessagePlugin } from 'tdesign-vue-next';
import type { RouteLocationNormalized, RouteRecordRaw } from 'vue-router';

import router from '@/router';
import { getPermissionStore, useUserStore } from '@/store';
import { PAGE_NOT_FOUND_ROUTE } from '@/utils/route/constant';

NProgress.configure({ showSpinner: false });

function routePermissions(value: unknown) {
  if (typeof value === 'string') return [value];
  if (Array.isArray(value)) return value.filter((item): item is string => typeof item === 'string');
  return [];
}

function hasRoutePermission(to: RouteLocationNormalized, userStore: ReturnType<typeof useUserStore>) {
  const required = routePermissions(to.meta.permission);
  if (!required.length) return true;

  const roles = userStore.roles || [];
  const permissions = userStore.userInfo.permissions || [];
  return (
    roles.includes('super_admin') ||
    permissions.includes('*') ||
    permissions.includes('*:*:*') ||
    required.some((permission) => permissions.includes(permission))
  );
}

router.beforeEach(async (to, from, next) => {
  NProgress.start();

  const permissionStore = getPermissionStore();
  const { whiteListRouters } = permissionStore;

  const userStore = useUserStore();

  if (userStore.token) {
    if (to.path === '/login') {
      next();
      NProgress.done();
      return;
    }
    try {
      // 如果用户信息为空，先获取用户信息
      if (!userStore.userInfo.name) {
        await userStore.getUserInfo();
      }
      if (userStore.userInfo.mustChangePassword && to.path !== '/profile/index') {
        next({ path: '/profile/index', query: { force_change_password: '1' }, replace: true });
        NProgress.done();
        return;
      }

      const { asyncRoutes } = permissionStore;

      if (asyncRoutes && asyncRoutes.length === 0) {
        const routeList = await permissionStore.buildAsyncRoutes();
        routeList.forEach((item: RouteRecordRaw) => {
          router.addRoute(item);
        });

        if (to.name === PAGE_NOT_FOUND_ROUTE.name) {
          // 动态添加路由后，此处应当重定向到fullPath，否则会加载404页面内容
          next({ path: to.path, replace: true, query: to.query });
        } else {
          const redirectQuery = from.query.redirect;
          const redirect = decodeURIComponent(typeof redirectQuery === 'string' ? redirectQuery : to.path);
          // 修复：只传递必要的属性，避免循环引用
          if (to.path === redirect) {
            next({ path: to.path, query: to.query, replace: true });
          } else {
            next({ path: redirect, query: to.query, replace: true });
          }
          return;
        }
      }
      if (to.name && router.hasRoute(to.name) && hasRoutePermission(to, userStore)) {
        next();
      } else if (to.name && router.hasRoute(to.name)) {
        next({ path: '/result/403', replace: true });
      } else {
        next(`/`);
      }
    } catch (error: any) {
      console.error('路由守卫错误:', error);
      MessagePlugin.error(error?.message || '获取用户信息失败');
      userStore.logout();
      next({
        path: '/login',
        query: { redirect: encodeURIComponent(to.fullPath) },
      });
      NProgress.done();
    }
  } else {
    /* white list router */
    if (whiteListRouters.includes(to.path)) {
      next();
    } else {
      next({
        path: '/login',
        query: { redirect: encodeURIComponent(to.fullPath) },
      });
    }
    NProgress.done();
  }
});

router.afterEach((to) => {
  if (to.path === '/login') {
    const userStore = useUserStore();
    const permissionStore = getPermissionStore();

    userStore.logout();
    permissionStore.restoreRoutes();
  }
  NProgress.done();
});
