<template>
  <div :class="layoutCls">
    <t-head-menu :class="menuCls" :theme="menuTheme" expand-type="popup" :value="active">
      <template #logo>
        <span v-if="showLogo" class="header-logo-container" @click="handleNav('/dashboard/index')">
          <span class="admin-header-brand">
            <span class="admin-header-brand__text">
              <strong>后台管理系统</strong>
              <small>管理控制台</small>
            </span>
          </span>
        </span>
        <div v-else class="header-operate-left">
          <t-button theme="default" shape="square" variant="text" @click="changeCollapsed">
            <t-icon class="collapsed-icon" name="view-list" />
          </t-button>
          <search :layout="layout" />
        </div>
      </template>
      <template v-if="layout !== 'side'" #default>
        <menu-content class="header-menu" :nav-data="menu" />
      </template>
      <template #operations>
        <div class="operations-container">
          <!-- 搜索框 -->
          <search v-if="layout !== 'side'" :layout="layout" />

          <!-- 全局通知 -->
          <notice />

          <t-tooltip placement="bottom" :content="t('layout.header.code')">
            <t-button theme="default" shape="square" variant="text" @click="navToGitHub">
              <t-icon name="logo-github" />
            </t-button>
          </t-tooltip>
          <t-tooltip placement="bottom" :content="theme === 'dark' ? '浅色模式' : '深色模式'">
            <t-button theme="default" shape="square" variant="text" @click="toggleDarkMode">
              <t-icon :name="theme === 'dark' ? 'sunny' : 'moon'" />
            </t-button>
          </t-tooltip>
          <t-tooltip placement="bottom" :content="t('layout.header.help')">
            <t-button theme="default" shape="square" variant="text" @click="navToHelper">
              <t-icon name="help-circle" />
            </t-button>
          </t-tooltip>
          <t-dropdown :min-column-width="120" trigger="click">
            <template #dropdown>
              <t-dropdown-item class="operations-dropdown-container-item" @click="handleNav('/profile')">
                <user-circle-icon />{{ t('layout.header.user') }}
              </t-dropdown-item>
              <t-dropdown-item class="operations-dropdown-container-item" @click="handleLogout">
                <poweroff-icon />{{ t('layout.header.signOut') }}
              </t-dropdown-item>
            </template>
            <t-button class="header-user-btn" theme="default" variant="text">
              <template #icon>
                <t-icon class="header-user-avatar" name="user-circle" />
              </template>
              <div class="header-user-account">{{ user.userInfo.name }}</div>
              <template #suffix><chevron-down-icon /></template>
            </t-button>
          </t-dropdown>
          <t-tooltip placement="bottom" :content="t('layout.header.setting')">
            <t-button theme="default" shape="square" variant="text" @click="toggleSettingPanel">
              <setting-icon />
            </t-button>
          </t-tooltip>
        </div>
      </template>
    </t-head-menu>
  </div>
</template>
<script setup lang="ts">
import { ChevronDownIcon, PoweroffIcon, SettingIcon, UserCircleIcon } from 'tdesign-icons-vue-next';
import type { PropType } from 'vue';
import { computed } from 'vue';
import { useRouter } from 'vue-router';

import { prefix } from '@/config/global';
import { t } from '@/locales';
import { getActive } from '@/router';
import { useSettingStore, useUserStore } from '@/store';
import type { MenuRoute, ModeType } from '@/types/interface';

import MenuContent from './MenuContent.vue';
import Notice from './Notice.vue';
import Search from './Search.vue';

const { theme, layout, showLogo, menu, isFixed, isCompact } = defineProps({
  theme: {
    type: String,
    default: 'light',
  },
  layout: {
    type: String,
    default: 'top',
  },
  showLogo: {
    type: Boolean,
    default: true,
  },
  menu: {
    type: Array as PropType<MenuRoute[]>,
    default: () => [],
  },
  isFixed: {
    type: Boolean,
    default: false,
  },
  isCompact: {
    type: Boolean,
    default: false,
  },
  maxLevel: {
    type: Number,
    default: 3,
  },
});

const router = useRouter();
const settingStore = useSettingStore();
const user = useUserStore();

const toggleSettingPanel = () => {
  settingStore.updateConfig({
    showSettingPanel: true,
  });
};

const toggleDarkMode = () => {
  const newMode = theme === 'dark' ? 'light' : 'dark';
  settingStore.updateConfig({
    displayMode: newMode,
  });
};

const active = computed(() => getActive());

const layoutCls = computed(() => [`${prefix}-header-layout`]);

const menuCls = computed(() => {
  return [
    {
      [`${prefix}-header-menu`]: !isFixed,
      [`${prefix}-header-menu-fixed`]: isFixed,
      [`${prefix}-header-menu-fixed-side`]: layout === 'side' && isFixed,
      [`${prefix}-header-menu-fixed-side-compact`]: layout === 'side' && isFixed && isCompact,
    },
  ];
});
const menuTheme = computed(() => theme as ModeType);

const changeCollapsed = () => {
  settingStore.updateConfig({
    isSidebarCompact: !settingStore.isSidebarCompact,
  });
};

const handleNav = (url: string) => {
  router.push(url);
};

const handleLogout = async () => {
  await user.logout(true);
  router.push({
    path: '/login',
    query: { redirect: encodeURIComponent(router.currentRoute.value.fullPath) },
  });
};

const navToGitHub = () => {
  window.open('https://github.com/tencent/tdesign-vue-next-starter');
};

const navToHelper = () => {
  window.open('http://tdesign.tencent.com/starter/docs/get-started');
};
</script>
<style lang="less" scoped>
.@{starter-prefix}-header {
  &-menu-fixed {
    position: fixed;
    top: 0;
    z-index: 1001;

    :deep(.t-head-menu__inner) {
      height: 56px;
      padding: 0 18px 0 12px;
      border-bottom: 1px solid #e8edf5;
      background: rgb(255 255 255 / 96%);
      box-shadow: 0 8px 22px rgb(15 23 42 / 5%);
      backdrop-filter: blur(10px);
    }

    &-side {
      left: 232px;
      right: 0;
      z-index: 10;
      width: auto;
      transition: all 0.3s;

      &-compact {
        left: 64px;
      }
    }
  }

  &-logo-container {
    cursor: pointer;
    display: inline-flex;
  }
}

.header-menu {
  flex: 1 1 auto;
  display: inline-flex;

  :deep(.t-menu__item) {
    min-width: unset;
  }
}

.operations-container {
  display: flex;
  align-items: center;
  gap: 6px;

  .t-popup__reference {
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .t-button {
    width: 32px;
    height: 32px;
    margin-left: 0;
    border-radius: 8px;
    color: #334155;

    &:hover {
      background: #f1f5fb;
      color: var(--td-brand-color);
    }
  }
}

.header-operate-left {
  display: flex;
  align-items: center;
  gap: 8px;
  line-height: 0;

  .t-button {
    width: 32px;
    height: 32px;
    border-radius: 8px;
    color: #334155;

    &:hover {
      background: #f1f5fb;
      color: var(--td-brand-color);
    }
  }
}

.header-logo-container {
  display: flex;
  width: 196px;
  height: 42px;
  align-items: center;
  margin-left: 24px;
  color: var(--td-text-color-primary);
  cursor: pointer;
}

.admin-header-brand {
  display: inline-flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
}

.admin-header-brand__text {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 1px;

  strong {
    color: #0f172a;
    font-size: 15px;
    font-weight: 800;
    line-height: 18px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  small {
    color: #64748b;
    font-size: 11px;
    font-weight: 700;
    line-height: 14px;
    text-transform: uppercase;
  }
}

.header-user-account {
  display: inline-flex;
  align-items: center;
  color: #0f172a;
  font-weight: 600;
  font-size: 13px;
}

:deep(.t-head-menu__inner) {
  border-bottom: 1px solid #e8edf5;
}

.t-menu--light {
  background: transparent;

  .header-user-account {
    color: #0f172a;
  }
}

.t-menu--dark {
  .t-head-menu__inner {
    border-bottom: 1px solid var(--td-gray-color-10);
  }

  .header-user-account {
    color: rgb(255 255 255 / 55%);
  }
}

.operations-dropdown-container-item {
  width: 100%;
  display: flex;
  align-items: center;

  :deep(.t-dropdown__item-text) {
    display: flex;
    align-items: center;
  }

  .t-icon {
    font-size: var(--td-comp-size-xxxs);
    margin-right: var(--td-comp-margin-s);
  }

  :deep(.t-dropdown__item) {
    width: 100%;
    margin-bottom: 0;
  }

  &:last-child {
    :deep(.t-dropdown__item) {
      margin-bottom: 8px;
    }
  }
}

.operations-container .header-user-btn {
  width: auto;
  min-width: 96px;
  padding: 0 10px;
  border: 1px solid transparent;
  border-radius: 999px;
  background: #f8fafc;
  box-shadow: inset 0 0 0 1px rgb(226 232 240 / 80%);

  &:hover {
    border-color: #dbe5f2;
    background: #f1f5fb;
  }
}

.header-user-avatar {
  color: #334155;
  font-size: 18px;
}
</style>
<!-- eslint-disable-next-line vue-scoped-css/enforce-style-type -->
<style lang="less">
.operations-dropdown-container-item {
  .t-dropdown__item-text {
    display: flex;
    align-items: center;
  }
}
</style>
