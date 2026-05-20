import type { MenuListResult, RouteItem } from '@/api/model/permissionModel';
import { getUserMenus, type BackendMenu } from '@/api/auth';

// 转换后端菜单为前端路由格式
function transformMenuToRoute(menu: BackendMenu): RouteItem {
  const route: RouteItem = {
    path: menu.path,
    name: menu.name,
    component: menu.component || 'LAYOUT',
    meta: {
      title: menu.title,
      icon: menu.icon,
      hidden: menu.hidden === 1,
      orderNo: menu.sort,
    },
  };

  // 递归转换子菜单
  if (menu.children && menu.children.length > 0) {
    route.children = menu.children.map((child) => transformMenuToRoute(child));
  }

  return route;
}

export function getMenuList() {
  return getUserMenus().then((menus: BackendMenu[]) => {
    // 转换后端菜单格式为前端需要的格式
    const routeList: RouteItem[] = (menus || []).map((menu) => transformMenuToRoute(menu));

    return {
      list: routeList,
    } as MenuListResult;
  });
}
