import { defineStore } from 'pinia';
import type { RouteRecordName } from 'vue-router';

import { store } from '@/store';
import type { TRouterInfo, TTabRouterType } from '@/types/interface';

const homeRoute: Array<TRouterInfo> = [
  {
    path: '/dashboard/index',
    routeIdx: 0,
    title: '仪表盘',
    name: 'DashboardIndex',
    isHome: true,
  },
];

const state = {
  tabRouterList: homeRoute,
  isRefreshing: false,
};

// 不需要做多标签tabs页缓存的列表 值为每个页面对应的name 如 DashboardDetail
// const ignoreCacheRoutes = ['DashboardDetail'];
const ignoreCacheRoutes = ['login'];

function isFallbackRouteName(name?: RouteRecordName) {
  return typeof name === 'string' && name.endsWith('Fallback');
}

function hasRouteTitle(title: TRouterInfo['title']) {
  if (typeof title === 'string') return title.trim().length > 0;
  return !!title && typeof title === 'object' && Object.keys(title).length > 0;
}

function shouldSkipTabRoute(route: TRouterInfo) {
  if (route.isHome) return false;
  return route.meta?.hidden === true || isFallbackRouteName(route.name) || !hasRouteTitle(route.title ?? route.meta?.title);
}

export const useTabsRouterStore = defineStore('tabsRouter', {
  state: () => state,
  getters: {
    tabRouters: (state: TTabRouterType) => state.tabRouterList,
    refreshing: (state: TTabRouterType) => state.isRefreshing,
  },
  actions: {
    // 处理刷新
    toggleTabRouterAlive(routeIdx: number) {
      this.isRefreshing = !this.isRefreshing;
      this.tabRouters[routeIdx].isAlive = !this.tabRouters[routeIdx].isAlive;
    },
    // 处理新增
    appendTabRouterList(newRoute: TRouterInfo) {
      if (shouldSkipTabRoute(newRoute)) return;
      // 不要将判断条件newRoute.meta.keepAlive !== false修改为newRoute.meta.keepAlive，starter默认开启保活，所以meta.keepAlive未定义时也需要进行保活，只有显式说明false才禁用保活。
      const needAlive = !ignoreCacheRoutes.includes(newRoute.name as string) && newRoute.meta?.keepAlive !== false;
      if (!this.tabRouters.find((route: TRouterInfo) => route.path === newRoute.path)) {
        this.tabRouterList = this.tabRouterList.concat({ ...newRoute, isAlive: needAlive });
      }
    },
    // 处理关闭当前
    subtractCurrentTabRouter(newRoute: TRouterInfo) {
      const { routeIdx } = newRoute;
      if (routeIdx === undefined) return;
      this.tabRouterList = this.tabRouterList.slice(0, routeIdx).concat(this.tabRouterList.slice(routeIdx + 1));
    },
    // 处理关闭右侧
    subtractTabRouterBehind(newRoute: TRouterInfo) {
      const { routeIdx } = newRoute;
      if (routeIdx === undefined) return;
      const homeIdx: number = this.tabRouters.findIndex((route: TRouterInfo) => route.isHome);
      let tabRouterList: Array<TRouterInfo> = this.tabRouterList.slice(0, routeIdx + 1);
      if (routeIdx < homeIdx) {
        tabRouterList = tabRouterList.concat(homeRoute);
      }
      this.tabRouterList = tabRouterList;
    },
    // 处理关闭左侧
    subtractTabRouterAhead(newRoute: TRouterInfo) {
      const { routeIdx } = newRoute;
      if (routeIdx === undefined) return;
      const homeIdx: number = this.tabRouters.findIndex((route: TRouterInfo) => route.isHome);
      let tabRouterList: Array<TRouterInfo> = this.tabRouterList.slice(routeIdx);
      if (routeIdx > homeIdx) {
        tabRouterList = homeRoute.concat(tabRouterList);
      }
      this.tabRouterList = tabRouterList;
    },
    // 处理关闭其他
    subtractTabRouterOther(newRoute: TRouterInfo) {
      const { routeIdx } = newRoute;
      if (routeIdx === undefined) return;
      const homeIdx: number = this.tabRouters.findIndex((route: TRouterInfo) => route.isHome);
      const targetRoute = this.tabRouterList[routeIdx];
      this.tabRouterList = routeIdx === homeIdx || !targetRoute ? homeRoute : homeRoute.concat([targetRoute]);
    },
    removeTabRouterList() {
      this.tabRouterList = [];
    },
    initTabRouterList(newRoutes: TRouterInfo[]) {
      newRoutes?.forEach((route: TRouterInfo) => this.appendTabRouterList(route));
    },
  },
  persist: true,
});

export function getTabsRouterStore() {
  return useTabsRouterStore(store);
}
