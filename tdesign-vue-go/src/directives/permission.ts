import type { App, DirectiveBinding } from 'vue';
import { useUserStore } from '@/store';

/**
 * 权限指令，用于控制元素的显示/隐藏
 * @param app Vue应用实例
 */
export function setupPermissionDirective(app: App) {
  // v-permission 指令
  app.directive('permission', {
    mounted(el: HTMLElement, binding: DirectiveBinding) {
      checkPermission(el, binding);
    },
    updated(el: HTMLElement, binding: DirectiveBinding) {
      checkPermission(el, binding);
    },
  });

  // v-role 指令
  app.directive('role', {
    mounted(el: HTMLElement, binding: DirectiveBinding) {
      checkRole(el, binding);
    },
    updated(el: HTMLElement, binding: DirectiveBinding) {
      checkRole(el, binding);
    },
  });
}

/**
 * 检查权限
 * @param el DOM元素
 * @param binding 指令绑定值
 */
function checkPermission(el: HTMLElement, binding: DirectiveBinding) {
  const { value } = binding;
  if (!value) return;
  cacheOriginalDisplay(el);

  const userStore = useUserStore();
  const userPermissions = userStore.userInfo.permissions || [];
  const userRoles = userStore.roles || [];

  let hasPermission = userRoles.includes('super_admin');

  if (!hasPermission && Array.isArray(value)) {
    // 数组格式：v-permission="['permission1', 'permission2']"，只要有一个权限就显示
    hasPermission = value.some((permission) => userPermissions.includes(permission) || permission === '*' || permission === '*:*:*');
  } else if (!hasPermission && typeof value === 'string') {
    // 字符串格式：v-permission="'permission'"，必须有该权限才显示
    hasPermission = userPermissions.includes(value) || value === '*' || value === '*:*:*';
  }

  el.style.display = hasPermission ? el.dataset.permissionDisplay || '' : 'none';
}

/**
 * 检查角色
 * @param el DOM元素
 * @param binding 指令绑定值
 */
function checkRole(el: HTMLElement, binding: DirectiveBinding) {
  const { value } = binding;
  if (!value) return;
  cacheOriginalDisplay(el);

  const userStore = useUserStore();
  const userRoles = userStore.roles || [];

  let hasRole = userRoles.includes('super_admin');

  if (!hasRole && Array.isArray(value)) {
    // 数组格式：v-role="['admin', 'editor']"，只要有一个角色就显示
    hasRole = value.some((role) => userRoles.includes(role));
  } else if (!hasRole && typeof value === 'string') {
    // 字符串格式：v-role="'admin'"，必须有该角色才显示
    hasRole = userRoles.includes(value);
  }

  el.style.display = hasRole ? el.dataset.permissionDisplay || '' : 'none';
}

function cacheOriginalDisplay(el: HTMLElement) {
  if (el.dataset.permissionDisplay === undefined) {
    el.dataset.permissionDisplay = el.style.display || '';
  }
}
