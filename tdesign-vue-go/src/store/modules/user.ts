import { defineStore } from 'pinia';

import { getCurrentUser, login as loginAPI, logout as logoutAPI, type LoginRequest } from '@/api/auth';
import { usePermissionStore } from '@/store';
import type { UserInfo } from '@/types/interface';

const InitUserInfo: UserInfo = {
  name: '', // 用户名，用于展示在页面右上角头像处
  roles: [], // 前端权限模型使用 如果使用请配置modules/permission-fe.ts使用
  permissions: [], // 用户权限列表
  mustChangePassword: false,
};

export const useUserStore = defineStore('user', {
  state: () => ({
    token: '', // 默认token不走权限
    refreshToken: '',
    userInfo: { ...InitUserInfo },
  }),
  getters: {
    roles: (state) => {
      return state.userInfo?.roles || [];
    },
  },
  actions: {
    async login(userInfo: LoginRequest) {
      // 登录请求流程
      const res = await loginAPI(userInfo);
      this.token = res.access_token;
      this.refreshToken = res.refresh_token;
      // 保存用户信息
      if (res.user) {
        this.userInfo = {
          name: res.user.username,
          nickname: res.user.nickname,
          username: res.user.username,
          roles: res.user.roles?.map((r) => r.code) || [],
          permissions: res.user.permissions || [],
          mustChangePassword: !!res.user.must_change_password,
        };
      }
    },
    async getUserInfo() {
      // 获取用户信息
      const res = await getCurrentUser();
      this.userInfo = {
        name: res.username,
        nickname: res.nickname,
        username: res.username,
        roles: res.roles?.map((r) => r.code) || [],
        permissions: res.permissions || [],
        mustChangePassword: !!res.must_change_password,
      };
    },
    async logout(remote = false) {
      const refreshToken = this.refreshToken;
      if (remote && this.token) {
        try {
          await logoutAPI(refreshToken ? { refresh_token: refreshToken } : undefined);
        } catch {
          // 本地退出不能被网络或 token 过期问题阻断。
        }
      }
      this.token = '';
      this.refreshToken = '';
      this.userInfo = { ...InitUserInfo };
    },
  },
  persist: {
    afterHydrate: () => {
      const permissionStore = usePermissionStore();
      permissionStore.initRoutes();
    },
    key: 'user',
    pick: ['token', 'refreshToken'],
  },
});
