<template>
  <div class="mysql-page">
    <console-page-header title="数据库监控" :status-theme="statusTheme" :status-text="statusText" :meta="headerMeta">
      <template #actions>
        <t-tag theme="primary" variant="light">PostgreSQL {{ mysqlInfo?.version || '-' }}</t-tag>
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
              <h3>基本信息</h3>
              <p>实例、字符集和数据规模</p>
            </div>
          </template>
          <div class="info-list">
            <div class="info-item">
              <span>版本</span>
              <strong>{{ mysqlInfo?.version || '-' }}</strong>
            </div>
            <div class="info-item">
              <span>数据库</span>
              <strong>{{ mysqlInfo?.database?.name || '-' }}</strong>
            </div>
            <div class="info-item">
              <span>字符集</span>
              <strong>{{ mysqlInfo?.database?.charset || '-' }}</strong>
            </div>
            <div class="info-item">
              <span>排序规则</span>
              <strong>{{ mysqlInfo?.database?.collation || '-' }}</strong>
            </div>
            <div class="info-item">
              <span>运行时间</span>
              <strong>{{ formatUptime(mysqlInfo?.uptime) }}</strong>
            </div>
          </div>
        </t-card>

        <t-card :bordered="false" class="monitor-card">
          <template #title>
            <div class="card-title">
              <h3>连接池状态</h3>
              <p>应用侧数据库连接池</p>
            </div>
          </template>
          <div class="metric-list">
            <div class="metric-row">
              <span>池当前连接</span>
              <strong>{{ mysqlInfo?.connections?.open_conns || 0 }}</strong>
            </div>
            <div class="metric-row">
              <span>使用中 / 空闲</span>
              <strong>{{ mysqlInfo?.connections?.in_use || 0 }} / {{ mysqlInfo?.connections?.idle || 0 }}</strong>
            </div>
            <div class="metric-row">
              <span>最大连接</span>
              <strong>{{ mysqlInfo?.connections?.max_open_conns || 0 }}</strong>
            </div>
            <div class="metric-row">
              <span>等待次数</span>
              <strong>{{ formatNumber(mysqlInfo?.connections?.wait_count) }}</strong>
            </div>
            <div class="metric-row">
              <span>等待耗时</span>
              <strong>{{ mysqlInfo?.connections?.wait_duration || '-' }}</strong>
            </div>
          </div>
        </t-card>

        <t-card :bordered="false" class="monitor-card">
          <template #title>
            <div class="card-title">
              <h3>数据库连接</h3>
              <p>服务端线程和历史峰值</p>
            </div>
          </template>
          <div class="resource-panel">
            <div class="resource-panel__head">
              <strong>{{ mysqlInfo?.connections?.threads_connected || 0 }}</strong>
              <t-tag theme="primary" variant="light">当前客户端</t-tag>
            </div>
            <div class="info-list info-list--compact">
              <div class="info-item">
                <span>运行线程</span>
                <strong>{{ mysqlInfo?.connections?.threads_running || 0 }}</strong>
              </div>
              <div class="info-item">
                <span>最大连接限制</span>
                <strong>{{ mysqlInfo?.connections?.max_connections || 0 }}</strong>
              </div>
              <div class="info-item">
                <span>历史峰值连接</span>
                <strong>{{ mysqlInfo?.connections?.max_used_connections || 0 }}</strong>
              </div>
            </div>
          </div>
        </t-card>

        <t-card :bordered="false" class="monitor-card">
          <template #title>
            <div class="card-title">
              <h3>查询与存储</h3>
              <p>吞吐、慢查询和数据体量</p>
            </div>
          </template>
          <div class="info-list">
            <div class="info-item">
              <span>QPS</span>
              <strong>{{ mysqlInfo?.queries?.qps || 0 }}</strong>
            </div>
            <div class="info-item">
              <span>累计查询</span>
              <strong>{{ formatNumber(mysqlInfo?.queries?.questions) }}</strong>
            </div>
            <div class="info-item">
              <span>慢查询</span>
              <strong>{{ formatNumber(mysqlInfo?.queries?.slow_queries) }}</strong>
            </div>
            <div class="info-item">
              <span>库大小 / 表数</span>
              <strong>{{ mysqlInfo?.database?.size || '-' }} / {{ mysqlInfo?.database?.table_count || 0 }}</strong>
            </div>
            <div class="info-item">
              <span>收发流量</span>
              <strong>{{ mysqlInfo?.traffic?.bytes_received_human || '-' }} / {{ mysqlInfo?.traffic?.bytes_sent_human || '-' }}</strong>
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

import { getMySQLInfo, type MySQLInfo } from '@/api/monitor/mysql';
import ConsolePageHeader from '@/components/common/ConsolePageHeader.vue';

type SummaryTone = 'blue' | 'green' | 'cyan' | 'orange' | 'red';
type TagTheme = 'success' | 'primary';

defineOptions({
  name: 'MonitorMySQL',
});

const mysqlInfo = ref<MySQLInfo | null>(null);
const loading = ref(false);
const loadError = ref('');
const lastUpdatedAt = ref('');

const isHealthy = computed(() => {
  const status = mysqlInfo.value?.status?.toLowerCase();
  return status === 'ok' || status === 'connected';
});
const statusTheme = computed<TagTheme>(() => (isHealthy.value ? 'success' : 'primary'));
const statusText = computed(() => (isHealthy.value ? '正常' : mysqlInfo.value?.status || '等待数据'));
const headerMeta = computed(() => [
  mysqlInfo.value?.database?.name || '数据库',
  `${mysqlInfo.value?.database?.host || '-'}:${mysqlInfo.value?.database?.port || '-'}`,
  mysqlInfo.value?.database?.charset || '-',
  lastUpdatedAt.value ? `更新于 ${lastUpdatedAt.value}` : '',
]);

const summaryItems = computed<Array<{ label: string; value: string | number; hint: string; icon: string; tone: SummaryTone }>>(() => [
  {
    label: '连接状态',
    value: mysqlInfo.value?.status || '-',
    hint: mysqlInfo.value?.database?.name || '等待数据',
    icon: 'data-base',
    tone: mysqlInfo.value?.status ? 'green' : 'blue',
  },
  {
    label: 'QPS',
    value: mysqlInfo.value?.queries?.qps || 0,
    hint: `累计 ${formatNumber(mysqlInfo.value?.queries?.questions)}`,
    icon: 'data',
    tone: 'cyan',
  },
  {
    label: '当前客户端',
    value: mysqlInfo.value?.connections?.threads_connected || 0,
    hint: `运行线程 ${mysqlInfo.value?.connections?.threads_running || 0}`,
    icon: 'server',
    tone: connectionTone(mysqlInfo.value?.connections?.threads_connected || 0, mysqlInfo.value?.connections?.max_connections || 0),
  },
  {
    label: '慢查询',
    value: formatNumber(mysqlInfo.value?.queries?.slow_queries),
    hint: `表数 ${mysqlInfo.value?.database?.table_count || 0}`,
    icon: 'error-circle',
    tone: (mysqlInfo.value?.queries?.slow_queries || 0) > 0 ? 'orange' : 'green',
  },
]);

const loadData = async () => {
  loading.value = true;
  loadError.value = '';
  try {
    mysqlInfo.value = await getMySQLInfo();
    lastUpdatedAt.value = new Date().toLocaleTimeString('zh-CN', { hour12: false });
  } catch (error: any) {
    loadError.value = error.message || '加载数据库监控失败';
    MessagePlugin.error(loadError.value);
  } finally {
    loading.value = false;
  }
};

const formatUptime = (seconds: string | undefined): string => {
  if (!seconds) return '-';
  const sec = Number.parseInt(seconds, 10);
  if (Number.isNaN(sec)) return seconds;
  const days = Math.floor(sec / 86400);
  const hours = Math.floor((sec % 86400) / 3600);
  const minutes = Math.floor((sec % 3600) / 60);
  return `${days}天 ${hours}小时 ${minutes}分钟`;
};

const formatNumber = (value?: number): string => Number(value || 0).toLocaleString();

const connectionTone = (current: number, max: number): SummaryTone => {
  if (!max) return 'blue';
  const ratio = current / max;
  if (ratio >= 0.9) return 'red';
  if (ratio >= 0.7) return 'orange';
  return 'green';
};

onMounted(() => {
  loadData();
});
</script>

<style scoped lang="less">
.mysql-page {
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

.mysql-page :deep(.t-card),
.mysql-page :deep(.t-button),
.mysql-page :deep(.t-tag),
.mysql-page :deep(.t-alert),
.mysql-page :deep(.t-empty) {
  font-family: var(--monitor-font);
}

.mysql-head {
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

.mysql-head__main {
  min-width: 0;
}

.mysql-head__title {
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

.mysql-head__meta {
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

.mysql-head__actions {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  flex-wrap: wrap;
}

.mysql-head__actions :deep(.t-tag) {
  height: 32px;
  align-items: center;
}

.mysql-head__refresh {
  width: 32px;
  height: 32px;
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
    overflow: hidden;
    color: #0f172a;
    font-family: var(--monitor-number-font);
    font-size: 32px;
    font-weight: 800;
    line-height: 34px;
    text-overflow: ellipsis;
    white-space: nowrap;
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

.info-list,
.metric-list {
  display: flex;
  flex-direction: column;
}

.info-list--compact {
  margin-top: 14px;
}

.info-item,
.metric-row {
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
  .mysql-page {
    margin: calc(-1 * var(--td-comp-paddingTB-l)) calc(-1 * var(--td-comp-paddingLR-l));
    padding: 12px;
  }

  .mysql-head {
    align-items: stretch;
    flex-direction: column;
  }

  .mysql-head__actions {
    justify-content: flex-start;
  }

  .summary-grid,
  .monitor-grid {
    grid-template-columns: 1fr;
  }
}
</style>
