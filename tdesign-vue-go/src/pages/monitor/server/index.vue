<template>
  <div class="server-page">
    <console-page-header title="服务器监控" :status-theme="headerStatusTheme" :status-text="headerStatusText" :meta="headerMeta">
      <template #actions>
        <t-tag theme="primary" variant="light">Go {{ serverInfo?.os?.go_version || '-' }}</t-tag>
        <t-tooltip content="刷新" placement="bottom">
          <t-button class="console-page-header__refresh" variant="outline" size="small" shape="square" :loading="loading" @click="loadData">
            <template #icon><t-icon name="refresh" /></template>
          </t-button>
        </t-tooltip>
      </template>
    </console-page-header>

    <t-alert
      v-if="loadError"
      class="monitor-alert"
      theme="error"
      :message="loadError"
      close-btn
      @close="loadError = ''"
    />

    <div class="summary-grid">
      <section
        v-for="item in summaryItems"
        :key="item.label"
        class="summary-panel"
        :class="`summary-panel--${item.tone}`"
      >
        <div class="summary-panel__main">
          <span>{{ item.label }}</span>
          <strong>{{ item.value }}</strong>
          <small>{{ item.hint }}</small>
        </div>
        <span class="summary-panel__icon">
          <t-icon :name="item.icon" />
        </span>
      </section>
    </div>

    <t-loading :loading="loading" show-overlay>
      <div class="monitor-grid">
        <t-card :bordered="false" class="monitor-card">
          <template #title>
            <div class="card-title">
              <h3>操作系统</h3>
              <p>主机、运行时和启动信息</p>
            </div>
          </template>
          <div class="info-list">
            <div class="info-item">
              <span>主机名</span>
              <strong>{{ serverInfo?.os?.hostname || '-' }}</strong>
            </div>
            <div class="info-item">
              <span>平台</span>
              <strong>{{ serverInfo?.os?.platform || '-' }}</strong>
            </div>
            <div class="info-item">
              <span>架构</span>
              <strong>{{ serverInfo?.os?.arch || '-' }}</strong>
            </div>
            <div class="info-item">
              <span>Go 版本</span>
              <strong>{{ serverInfo?.os?.go_version || '-' }}</strong>
            </div>
            <div class="info-item">
              <span>启动时间</span>
              <strong>{{ serverInfo?.os?.boot_time || '-' }}</strong>
            </div>
          </div>
        </t-card>

        <t-card :bordered="false" class="monitor-card">
          <template #title>
            <div class="card-title">
              <h3>CPU</h3>
              <p>{{ serverInfo?.cpu?.model_name || '处理器信息' }}</p>
            </div>
          </template>
          <div class="resource-panel">
            <div class="resource-panel__head">
              <strong>{{ formatPercent(serverInfo?.cpu?.used_percent) }}</strong>
              <t-tag :theme="progressTheme(serverInfo?.cpu?.used_percent || 0)" variant="light">使用率</t-tag>
            </div>
            <t-progress :percentage="safePercent(serverInfo?.cpu?.used_percent)" :label="false" :status="progressStatus(serverInfo?.cpu?.used_percent || 0)" />
            <div class="info-list info-list--compact">
              <div class="info-item">
                <span>核心数</span>
                <strong>{{ serverInfo?.cpu?.cores || 0 }}</strong>
              </div>
              <div class="info-item">
                <span>Go 协程</span>
                <strong>{{ serverInfo?.os?.num_goroutine || serverInfo?.runtime?.num_goroutine || 0 }}</strong>
              </div>
            </div>
          </div>
        </t-card>

        <t-card :bordered="false" class="monitor-card">
          <template #title>
            <div class="card-title">
              <h3>内存</h3>
              <p>总量、已用与可用空间</p>
            </div>
          </template>
          <div class="resource-panel">
            <div class="resource-panel__head">
              <strong>{{ formatPercent(serverInfo?.memory?.used_percent) }}</strong>
              <t-tag :theme="progressTheme(serverInfo?.memory?.used_percent || 0)" variant="light">使用率</t-tag>
            </div>
            <t-progress :percentage="safePercent(serverInfo?.memory?.used_percent)" :label="false" :status="progressStatus(serverInfo?.memory?.used_percent || 0)" />
            <div class="info-list info-list--compact">
              <div class="info-item">
                <span>总内存</span>
                <strong>{{ formatBytes(serverInfo?.memory?.total || 0) }}</strong>
              </div>
              <div class="info-item">
                <span>已用 / 可用</span>
                <strong>{{ formatBytes(serverInfo?.memory?.used || 0) }} / {{ formatBytes(serverInfo?.memory?.free || 0) }}</strong>
              </div>
            </div>
          </div>
        </t-card>

        <t-card :bordered="false" class="monitor-card">
          <template #title>
            <div class="card-title">
              <h3>磁盘</h3>
              <p>容量占用与剩余空间</p>
            </div>
          </template>
          <div class="resource-panel">
            <div class="resource-panel__head">
              <strong>{{ formatPercent(serverInfo?.disk?.used_percent) }}</strong>
              <t-tag :theme="progressTheme(serverInfo?.disk?.used_percent || 0)" variant="light">使用率</t-tag>
            </div>
            <t-progress :percentage="safePercent(serverInfo?.disk?.used_percent)" :label="false" :status="progressStatus(serverInfo?.disk?.used_percent || 0)" />
            <div class="info-list info-list--compact">
              <div class="info-item">
                <span>总容量</span>
                <strong>{{ formatBytes(serverInfo?.disk?.total || 0) }}</strong>
              </div>
              <div class="info-item">
                <span>已用 / 可用</span>
                <strong>{{ formatBytes(serverInfo?.disk?.used || 0) }} / {{ formatBytes(serverInfo?.disk?.free || 0) }}</strong>
              </div>
            </div>
          </div>
        </t-card>
      </div>
    </t-loading>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { MessagePlugin } from 'tdesign-vue-next';

import { getServerInfo, type ServerInfo } from '@/api/monitor/server';
import ConsolePageHeader from '@/components/common/ConsolePageHeader.vue';

type SummaryTone = 'blue' | 'green' | 'cyan' | 'orange' | 'red';
type TagTheme = 'default' | 'success' | 'primary' | 'warning' | 'danger';

defineOptions({
  name: 'MonitorServer',
});

const serverInfo = ref<ServerInfo | null>(null);
const loading = ref(false);
const loadError = ref('');
const lastUpdatedAt = ref('');

const resourceRiskCount = computed(() => {
  const values = [
    serverInfo.value?.cpu?.used_percent || 0,
    serverInfo.value?.memory?.used_percent || 0,
    serverInfo.value?.disk?.used_percent || 0,
  ];
  return values.filter((value) => value >= 70).length;
});
const headerStatusTheme = computed<TagTheme>(() => (resourceRiskCount.value > 0 ? 'warning' : 'success'));
const headerStatusText = computed(() => (resourceRiskCount.value > 0 ? `存在 ${resourceRiskCount.value} 项资源压力` : '服务器状态正常'));
const headerMeta = computed(() => [
  serverInfo.value?.os?.hostname || '未知主机',
  serverInfo.value?.os?.platform || '-',
  serverInfo.value?.os?.arch || '-',
  lastUpdatedAt.value ? `更新于 ${lastUpdatedAt.value}` : '',
]);

const summaryItems = computed<Array<{ label: string; value: string | number; hint: string; icon: string; tone: SummaryTone }>>(() => [
  {
    label: 'CPU 使用率',
    value: formatPercent(serverInfo.value?.cpu?.used_percent),
    hint: `${serverInfo.value?.cpu?.cores || 0} 核`,
    icon: 'server',
    tone: resourceTone(serverInfo.value?.cpu?.used_percent || 0),
  },
  {
    label: '内存使用率',
    value: formatPercent(serverInfo.value?.memory?.used_percent),
    hint: formatBytes(serverInfo.value?.memory?.used || 0),
    icon: 'data',
    tone: resourceTone(serverInfo.value?.memory?.used_percent || 0),
  },
  {
    label: '磁盘使用率',
    value: formatPercent(serverInfo.value?.disk?.used_percent),
    hint: formatBytes(serverInfo.value?.disk?.used || 0),
    icon: 'data-base',
    tone: resourceTone(serverInfo.value?.disk?.used_percent || 0),
  },
  {
    label: 'Go 协程',
    value: serverInfo.value?.os?.num_goroutine || serverInfo.value?.runtime?.num_goroutine || 0,
    hint: serverInfo.value?.os?.go_version || '-',
    icon: 'time',
    tone: 'cyan',
  },
]);

const loadData = async () => {
  loading.value = true;
  loadError.value = '';
  try {
    serverInfo.value = await getServerInfo();
    lastUpdatedAt.value = new Date().toLocaleTimeString('zh-CN', { hour12: false });
  } catch (error: any) {
    loadError.value = error.message || '加载服务器监控失败';
    MessagePlugin.error(loadError.value);
  } finally {
    loading.value = false;
  }
};

const formatBytes = (bytes: number): string => {
  if (!bytes) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1);
  return `${(bytes / Math.pow(k, i)).toFixed(2)} ${sizes[i]}`;
};

const formatPercent = (percent?: number): string => `${Number(percent || 0).toFixed(2)}%`;

const safePercent = (percent?: number) => Math.max(0, Math.min(100, Number(percent || 0)));

const progressStatus = (percent: number): 'success' | 'warning' | 'error' => {
  if (percent >= 90) return 'error';
  if (percent >= 70) return 'warning';
  return 'success';
};

const progressTheme = (percent: number): TagTheme => {
  if (percent >= 90) return 'danger';
  if (percent >= 70) return 'warning';
  return 'success';
};

const resourceTone = (percent: number): SummaryTone => {
  if (percent >= 90) return 'red';
  if (percent >= 70) return 'orange';
  if (percent > 0) return 'green';
  return 'blue';
};

onMounted(() => {
  loadData();
});
</script>

<style scoped lang="less">
.server-page {
  --monitor-bg: #f5f7fb;
  --monitor-card-shadow: 0 12px 28px rgb(15 23 42 / 6%);
  --monitor-font: 'Inter', 'HarmonyOS Sans SC', 'MiSans', 'PingFang SC', 'Microsoft YaHei UI', 'Microsoft YaHei',
    'Arial', sans-serif;
  --monitor-number-font: 'DIN Alternate', 'Bahnschrift', 'Inter', 'HarmonyOS Sans SC', 'Microsoft YaHei UI', sans-serif;

  display: flex;
  min-height: calc(100vh - 120px);
  min-width: 0;
  flex-direction: column;
  gap: 14px;
  margin: calc(-1 * var(--td-comp-paddingTB-xl)) calc(-1 * var(--td-comp-paddingLR-xl));
  padding: 14px 18px 18px;
  background: var(--monitor-bg);
  color: var(--td-text-color-primary);
  font-family: var(--monitor-font);
  font-feature-settings: 'tnum';
  font-variant-numeric: tabular-nums;
  letter-spacing: 0;
}

.server-page :deep(.t-card),
.server-page :deep(.t-button),
.server-page :deep(.t-tag),
.server-page :deep(.t-alert),
.server-page :deep(.t-empty),
.server-page :deep(.t-progress) {
  font-family: var(--monitor-font);
}

.server-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--td-comp-margin-l);
  padding: 10px 12px;
  border: 1px solid #e8edf5;
  border-radius: 12px;
  background:
    radial-gradient(circle at 18% 0%, rgb(37 99 235 / 10%), transparent 28%),
    radial-gradient(circle at 92% 16%, rgb(20 184 166 / 12%), transparent 26%),
    #fff;
  box-shadow: 0 10px 24px rgb(15 23 42 / 5%);
}

.server-head__main {
  min-width: 0;
}

.server-head__title {
  display: flex;
  align-items: center;
  gap: 10px;

  h2 {
    margin: 0;
    color: #0f172a;
    font-size: 22px;
    font-weight: 800;
    line-height: 30px;
  }
}

.server-head__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 10px;
  margin-top: 6px;
  color: #52627a;
  font-size: 12px;

  span + span::before {
    margin-right: 10px;
    color: #c7d0df;
    content: '/';
  }
}

.server-head__actions {
  flex-shrink: 0;
  justify-content: flex-end;
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.summary-panel {
  position: relative;
  display: flex;
  min-height: 118px;
  overflow: hidden;
  align-items: center;
  justify-content: space-between;
  padding: 18px 18px 16px;
  border: 1px solid var(--summary-border);
  border-radius: 14px;
  background: linear-gradient(135deg, var(--summary-bg-start), var(--summary-bg-end));
  box-shadow: 0 14px 28px rgb(15 23 42 / 7%);

  &::after {
    position: absolute;
    right: -30px;
    bottom: -36px;
    width: 96px;
    height: 96px;
    border-radius: 50%;
    background: rgb(255 255 255 / 46%);
    content: '';
  }
}

.summary-panel--blue {
  --summary-bg-start: #dbeafe;
  --summary-bg-end: #bfdbfe;
  --summary-border: #c7ddff;
  --summary-icon: #2563eb;
}

.summary-panel--green {
  --summary-bg-start: #dcfce7;
  --summary-bg-end: #bbf7d0;
  --summary-border: #b8ecc8;
  --summary-icon: #059669;
}

.summary-panel--cyan {
  --summary-bg-start: #d9f3ff;
  --summary-bg-end: #bae6fd;
  --summary-border: #b8e4f8;
  --summary-icon: #0284c7;
}

.summary-panel--orange {
  --summary-bg-start: #ffedd5;
  --summary-bg-end: #fed7aa;
  --summary-border: #fbd0a1;
  --summary-icon: #ea580c;
}

.summary-panel--red {
  --summary-bg-start: #ffe0e5;
  --summary-bg-end: #ffc2cc;
  --summary-border: #f9b9c4;
  --summary-icon: #d54941;
}

.summary-panel__main {
  position: relative;
  z-index: 1;
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 6px;

  span {
    color: #334155;
    font-size: 13px;
    font-weight: 700;
  }

  strong {
    color: #0f172a;
    font-family: var(--monitor-number-font);
    font-size: 32px;
    font-weight: 800;
    line-height: 34px;
  }

  small {
    overflow: hidden;
    color: #64748b;
    font-size: 12px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.summary-panel__icon {
  position: relative;
  z-index: 1;
  display: inline-flex;
  width: 36px;
  height: 36px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: rgb(255 255 255 / 68%);
  color: var(--summary-icon);
  font-size: 19px;
}

.monitor-alert {
  border-radius: 12px;
  box-shadow: 0 8px 18px rgb(15 23 42 / 5%);
}

.monitor-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.monitor-card {
  min-width: 0;
  overflow: hidden;
  border: 1px solid #e8edf5;
  border-radius: 12px;
  background: #fff;
  box-shadow: var(--monitor-card-shadow);
}

.monitor-card :deep(.t-card__header) {
  align-items: flex-start;
  padding: 18px 18px 10px;
  border-bottom: 1px solid #edf1f7;
}

.monitor-card :deep(.t-card__body) {
  padding: 16px 18px 18px;
}

.card-title h3 {
  margin: 0;
  color: #111827;
  font-size: 16px;
  font-weight: 750;
  line-height: 24px;
}

.card-title p {
  overflow: hidden;
  margin: 4px 0 0;
  color: #64748b;
  font-size: 13px;
  line-height: 20px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.info-list {
  display: flex;
  flex-direction: column;
}

.info-list--compact {
  margin-top: 14px;
}

.info-item {
  display: flex;
  min-height: 40px;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  border-bottom: 1px solid #edf1f7;

  &:last-child {
    border-bottom: 0;
  }

  span {
    flex-shrink: 0;
    color: #64748b;
    font-size: 13px;
  }

  strong {
    overflow: hidden;
    color: #0f172a;
    font-size: 13px;
    font-weight: 700;
    text-align: right;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.resource-panel {
  display: flex;
  min-height: 196px;
  flex-direction: column;
  justify-content: space-between;
  gap: 14px;
}

.resource-panel__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;

  strong {
    color: #0f172a;
    font-family: var(--monitor-number-font);
    font-size: 34px;
    font-weight: 800;
    line-height: 38px;
  }
}

@media (width <= 1400px) {
  .monitor-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (width <= 1200px) {
  .summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (width <= 768px) {
  .server-page {
    margin: calc(-1 * var(--td-comp-paddingTB-l)) calc(-1 * var(--td-comp-paddingLR-l));
    padding: 12px;
  }

  .server-head {
    align-items: stretch;
    flex-direction: column;
  }

  .summary-grid,
  .monitor-grid {
    grid-template-columns: 1fr;
  }
}
</style>
