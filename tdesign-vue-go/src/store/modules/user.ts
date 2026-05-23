import { defineStore } from 'pinia';

import { getCurrentUser, login as loginAPI, logout as logoutAPI, type LoginRequest, type LoginResponse } from '@/api/auth';
import { usePermissionStore } from '@/store';
import type { UserInfo } from '@/types/interface';

const InitUserInfo: UserInfo = {
  name: '',
  roles: [],
  permissions: [],
  mustChangePassword: false,
  totpEnabled: false,
};

export const useUserStore = defineStore('user', {
  state: () => ({
    token: '',
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
      const res = await loginAPI(userInfo);
      this.applyLoginSession(res);
      return res;
    },
    applyLoginSession(res: LoginResponse) {
      if (res.access_token) {
        this.token = res.access_token;
      }
      if (res.refresh_token) {
        this.refreshToken = res.refresh_token;
      }
      if (res.user) {
        this.applyUserInfo(res.user);
      }
    },
    async getUserInfo() {
      const res = await getCurrentUser();
      this.applyUserInfo(res);
    },
    applyUserInfo(res: NonNullable<LoginResponse['user']>) {
      this.userInfo = {
        name: res.username,
        nickname: res.nickname,
        username: res.username,
        roles: res.roles?.map((r) => r.code) || [],
        permissions: res.permissions || [],
        mustChangePassword: !!res.must_change_password,
        totpEnabled: !!res.totp_enabled,
      };
    },
    async logout(remote = false) {
      const refreshToken = this.refreshToken;
      if (remote && this.token) {
        try {
          await logoutAPI(refreshToken ? { refresh_token: refreshToken } : undefined);
        } catch {
          // Local logout should not be blocked by network or token-expiry errors.
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
