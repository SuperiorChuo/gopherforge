<template>
  <div class="dashboard-page">
    <console-page-header
      title="后台管理系统"
      status-text="模板"
      status-theme="primary"
      :meta="['接口服务', '前端控制台', '数据库', '缓存服务']"
    />

    <div class="metric-grid">
      <t-card v-for="item in metricCards" :key="item.label" :bordered="false" class="metric-card">
        <span class="metric-card__icon">
          <t-icon :name="item.icon" />
        </span>
        <div>
          <span>{{ item.label }}</span>
          <strong>{{ item.value }}</strong>
          <small>{{ item.hint }}</small>
        </div>
      </t-card>
    </div>

    <div class="panel-grid">
      <t-card :bordered="false" class="panel-card">
        <template #title>后端能力</template>
        <t-list :split="false">
          <t-list-item v-for="item in backendItems" :key="item">
            <t-icon name="check-circle-filled" class="item-icon" />
            {{ item }}
          </t-list-item>
        </t-list>
      </t-card>

      <t-card :bordered="false" class="panel-card">
        <template #title>前端能力</template>
        <t-list :split="false">
          <t-list-item v-for="item in frontendItems" :key="item">
            <t-icon name="check-circle-filled" class="item-icon" />
            {{ item }}
          </t-list-item>
        </t-list>
      </t-card>
    </div>

    <section ref="upgradePanel" class="upgrade-panel">
      <div class="upgrade-panel__header">
        <div>
          <span class="upgrade-panel__eyebrow">升级能力</span>
          <h3>新版运行栈</h3>
        </div>
        <t-input v-model="upgradeKeyword" clearable placeholder="搜索版本或能力" class="upgrade-panel__search">
          <template #prefix-icon>
            <t-icon name="search" />
          </template>
        </t-input>
      </div>

      <div class="upgrade-panel__tags">
        <t-tag theme="success" variant="light">Vue 3.5 新 API 已启用</t-tag>
        <t-tag :theme="runtimeInfo ? 'primary' : 'warning'" variant="light">
          {{ runtimeInfo ? '后端运行时已连接' : healthError || '读取后端运行时' }}
        </t-tag>
      </div>

      <div class="upgrade-grid">
        <article v-for="item in filteredUpgradeItems" :key="item.stack" class="upgrade-card">
          <span class="upgrade-card__tag">{{ item.tag }}</span>
          <strong>{{ item.stack }}</strong>
          <em>{{ item.version }}</em>
          <small>{{ item.detail }}</small>
        </article>
        <t-empty v-if="!filteredUpgradeItems.length" description="没有匹配的升级项" />
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onWatcherCleanup, ref, useTemplateRef, watch } from 'vue';

import { getHealth, type RuntimeInfo } from '@/api/common/health';
import ConsolePageHeader from '@/components/common/ConsolePageHeader.vue';

import pkg from '../../../package.json';

const metricCards = [
  { label: '认证', value: '令牌', hint: '登录、刷新、退出登录', icon: 'secured' },
  { label: '权限', value: '就绪', hint: '用户、角色、权限', icon: 'user-safety' },
  { label: '数据', value: '清爽', hint: '基础表结构与初始化数据', icon: 'data-base' },
  { label: '运维', value: '内置', hint: '日志、文件、任务、监控', icon: 'chart-analytics' },
];

const backendItems = [
  '后端路由注册',
  '模型数据层',
  '独立数据库表结构',
  '令牌撤销缓存',
  '操作日志与审计日志',
];

const frontendItems = [
  '控制台前端框架',
  '系统管理页面',
  '监控管理页面',
  '个人中心与认证流程',
  '动态菜单兼容路由',
];

const packageVersions = {
  ...pkg.dependencies,
  ...pkg.devDependencies,
} as Record<string, string | undefined>;

const runtimeInfo = ref<RuntimeInfo | null>(null);
const healthError = ref('');
const upgradeKeyword = ref('');
const debouncedUpgradeKeyword = ref('');
const upgradePanel = useTemplateRef<HTMLElement>('upgradePanel');

const normalizeVersion = (version?: string) => version?.replace(/^[~^]/, '') || '待确认';

const upgradeItems = computed(() => [
  {
    tag: '后端',
    stack: 'Go',
    version: runtimeInfo.value?.go_version || 'go1.26.3',
    detail: runtimeInfo.value
      ? `${runtimeInfo.value.compiler} / ${runtimeInfo.value.os}-${runtimeInfo.value.arch}`
      : '健康检查会返回当前 Go 运行时信息',
  },
  {
    tag: '前端',
    stack: 'Vue',
    version: normalizeVersion(packageVersions.vue),
    detail: 'useTemplateRef 与 onWatcherCleanup 已用于本页筛选交互',
  },
  {
    tag: '构建',
    stack: 'Vite',
    version: normalizeVersion(packageVersions.vite),
    detail: '迁移到新版 Bundler 模块解析，适配 TypeScript 6',
  },
  {
    tag: '组件',
    stack: 'TDesign Vue Next',
    version: normalizeVersion(packageVersions['tdesign-vue-next']),
    detail: '控制台组件库升级，继续沿用企业后台密度',
  },
]);

const filteredUpgradeItems = computed(() => {
  const keyword = debouncedUpgradeKeyword.value;
  if (!keyword) return upgradeItems.value;

  return upgradeItems.value.filter((item) =>
    [item.tag, item.stack, item.version, item.detail].some((text) => text.toLowerCase().includes(keyword)),
  );
});

watch(
  upgradeKeyword,
  (value) => {
    const timer = window.setTimeout(() => {
      debouncedUpgradeKeyword.value = value.trim().toLowerCase();
    }, 160);

    onWatcherCleanup(() => {
      window.clearTimeout(timer);
    });
  },
  { immediate: true },
);

async function loadRuntimeInfo() {
  try {
    const health = await getHealth();
    runtimeInfo.value = health.runtime || null;
    healthError.value = '';
  } catch (error) {
    healthError.value = error instanceof Error ? error.message : '读取失败';
  }
}

onMounted(() => {
  upgradePanel.value?.setAttribute('data-ready', 'true');
  void loadRuntimeInfo();
});
</script>

<style scoped>
.dashboard-page {
  display: flex;
  min-height: calc(100vh - 120px);
  flex-direction: column;
  gap: 16px;
  margin: calc(-1 * var(--td-comp-paddingTB-xl)) calc(-1 * var(--td-comp-paddingLR-xl));
  padding: 16px;
  background: #f5f7fb;
}

.metric-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.metric-card :deep(.t-card__body) {
  display: flex;
  min-height: 118px;
  align-items: center;
  gap: 14px;
  padding: 18px;
}

.metric-card,
.panel-card {
  border-radius: 8px;
  box-shadow: 0 10px 28px rgb(15 23 42 / 6%);
}

.metric-card__icon {
  display: inline-flex;
  width: 44px;
  height: 44px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: #e8f1ff;
  color: var(--td-brand-color);
  font-size: 22px;
}

.metric-card span {
  color: var(--td-text-color-secondary);
  font-size: 13px;
}

.metric-card strong {
  display: block;
  margin-top: 4px;
  color: var(--td-text-color-primary);
  font-size: 28px;
  line-height: 34px;
}

.metric-card small {
  display: block;
  margin-top: 4px;
  color: var(--td-text-color-placeholder);
  font-size: 12px;
}

.panel-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.upgrade-panel {
  padding: 18px;
  border: 1px solid #e8edf5;
  border-radius: 8px;
  background: #fff;
  box-shadow: 0 10px 28px rgb(15 23 42 / 6%);
}

.upgrade-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.upgrade-panel__eyebrow {
  color: var(--td-brand-color);
  font-size: 12px;
  font-weight: 700;
}

.upgrade-panel h3 {
  margin: 4px 0 0;
  color: var(--td-text-color-primary);
  font-size: 18px;
  line-height: 26px;
}

.upgrade-panel__search {
  width: min(320px, 100%);
  flex-shrink: 0;
}

.upgrade-panel__tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 12px;
}

.upgrade-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-top: 14px;
}

.upgrade-card {
  min-height: 126px;
  padding: 14px;
  border: 1px solid #e7edf7;
  border-radius: 8px;
  background: #fbfdff;
}

.upgrade-card__tag {
  display: inline-flex;
  align-items: center;
  height: 22px;
  padding: 0 8px;
  border-radius: 999px;
  background: #eef6ff;
  color: var(--td-brand-color);
  font-size: 12px;
  font-weight: 600;
}

.upgrade-card strong {
  display: block;
  margin-top: 12px;
  color: var(--td-text-color-primary);
  font-size: 16px;
  line-height: 24px;
}

.upgrade-card em {
  display: block;
  margin-top: 4px;
  color: #0f766e;
  font-size: 18px;
  font-style: normal;
  font-weight: 700;
  line-height: 26px;
}

.upgrade-card small {
  display: block;
  margin-top: 8px;
  color: var(--td-text-color-secondary);
  font-size: 12px;
  line-height: 18px;
}

.item-icon {
  margin-right: 8px;
  color: var(--td-success-color);
}

@media (width <= 960px) {
  .metric-grid,
  .panel-grid,
  .upgrade-grid {
    grid-template-columns: 1fr;
  }

  .upgrade-panel__header {
    align-items: stretch;
    flex-direction: column;
  }

  .upgrade-panel__search {
    width: 100%;
  }
}
</style>
