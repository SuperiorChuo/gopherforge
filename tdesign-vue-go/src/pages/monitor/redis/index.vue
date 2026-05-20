<template>
  <div class="redis-page">
    <console-page-header title="Redis 监控" :status-theme="statusTheme" :status-text="statusText" :meta="headerMeta">
      <template #actions>
        <t-select v-model="timeWindow" class="console-page-header__select" size="small" :options="timeWindowOptions" />
        <t-select v-model="refreshInterval" class="console-page-header__select" size="small" :options="refreshIntervalOptions" />
        <div class="console-page-header__auto-wrap" :class="{ 'console-page-header__auto-wrap--active': autoRefreshEnabled }">
          <span class="console-page-header__auto-label">自动刷新</span>
          <t-switch v-model="autoRefreshEnabled" size="small" />
        </div>
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
          <small>
            <template v-if="item.trend">
              <span class="summary-panel__trend" :class="`summary-panel__trend--${item.trend.tone}`">
                {{ item.trend.text }}
              </span>
              <span> / </span>
            </template>
            <span>{{ item.hint }}</span>
          </small>
        </div>
        <span class="summary-panel__icon">
          <t-icon :name="item.icon" />
        </span>
      </section>
    </div>

    <section class="trend-section">
      <div class="trend-section__head">
        <div class="card-title">
          <h3>关键趋势</h3>
          <p>{{ timeWindowLabel }} 内采样对比</p>
        </div>
        <t-tag theme="primary" variant="light">样本 {{ snapshotHistory.length }} / {{ samplesToKeep }}</t-tag>
      </div>
      <div class="trend-card-grid">
        <section
          v-for="item in trendRows"
          :key="item.label"
          class="trend-card"
          :class="`trend-card--${item.tone}`"
        >
          <div class="trend-card__head">
            <span>{{ item.label }}</span>
            <span class="trend-card__delta" :class="`trend-card__delta--${item.trend.tone}`">{{ item.trend.text }}</span>
          </div>
          <strong>{{ item.current }}</strong>
          <small>上次 {{ item.previous }}</small>
        </section>
      </div>
    </section>

    <t-loading :loading="loading" show-overlay>
      <div class="monitor-grid">
        <t-card :bordered="false" class="monitor-card">
          <template #title>
            <div class="card-title">
              <h3>基本信息</h3>
              <p>服务版本、运行模式和主机信息</p>
            </div>
          </template>
          <div class="info-list">
            <div class="info-item">
              <span>版本</span>
              <strong>{{ redisInfo?.server?.version || '-' }}</strong>
            </div>
            <div class="info-item">
              <span>运行模式</span>
              <strong>{{ modeText }}</strong>
            </div>
            <div class="info-item">
              <span>操作系统</span>
              <t-tooltip :content="redisInfo?.server?.os || '-'" placement="top-right">
                <strong class="info-item__value--truncate">{{ redisInfo?.server?.os || '-' }}</strong>
              </t-tooltip>
            </div>
            <div class="info-item">
              <span>运行时间</span>
              <strong>{{ formatUptime(redisInfo?.server?.uptime || redisInfo?.server?.uptime_seconds) }}</strong>
            </div>
            <div class="info-item">
              <span>端口 / 进程号</span>
              <strong>{{ redisInfo?.server?.tcp_port || '-' }} / {{ redisInfo?.server?.process_id || '-' }}</strong>
            </div>
          </div>
        </t-card>

        <t-card :bordered="false" class="monitor-card">
          <template #title>
            <div class="card-title">
              <h3>内存信息</h3>
              <p>缓存占用、峰值和碎片率</p>
            </div>
          </template>
          <div class="metric-list">
            <div class="metric-row">
              <span>已用内存</span>
              <strong>{{ formatBytes(redisInfo?.memory?.used_bytes, redisInfo?.memory?.used) }}</strong>
            </div>
            <div class="metric-row">
              <span>内存峰值</span>
              <strong>{{ formatBytes(redisInfo?.memory?.peak_bytes, redisInfo?.memory?.peak) }}</strong>
            </div>
            <div class="metric-row">
              <span>RSS / 最大内存</span>
              <strong>{{ normalizeMemoryText(redisInfo?.memory?.rss) }} / {{ normalizeMaxMemory(redisInfo?.memory?.maxmemory) }}</strong>
            </div>
            <div class="metric-row">
              <span>Lua 内存</span>
              <strong>{{ normalizeMemoryText(redisInfo?.memory?.lua) }}</strong>
            </div>
            <div class="metric-row">
              <span>碎片率</span>
              <strong>{{ redisInfo?.memory?.fragmentation || '-' }}</strong>
            </div>
          </div>
        </t-card>

        <t-card :bordered="false" class="monitor-card">
          <template #title>
            <div class="card-title">
              <h3>Redis 连接</h3>
              <p>客户端和连接池状态</p>
            </div>
          </template>
          <div class="resource-panel">
            <div class="resource-panel__head">
              <strong>{{ clientCount }}</strong>
              <t-tag :theme="blockedClients > 0 ? 'warning' : 'primary'" variant="light">当前客户端</t-tag>
            </div>
            <div class="info-list info-list--compact">
              <div class="info-item">
                <span>总连接 / 空闲连接</span>
                <strong>{{ formatNumber(redisInfo?.pool?.total_conns) }} / {{ formatNumber(redisInfo?.pool?.idle_conns) }}</strong>
              </div>
              <div class="info-item">
                <span>阻塞 / 跟踪客户端</span>
                <strong>{{ formatNumber(redisInfo?.clients?.blocked) }} / {{ formatNumber(redisInfo?.clients?.tracking) }}</strong>
              </div>
              <div class="info-item">
                <span>连接池命中 / 未命中</span>
                <strong>{{ formatNumber(redisInfo?.pool?.hits) }} / {{ formatNumber(redisInfo?.pool?.misses) }}</strong>
              </div>
            </div>
          </div>
        </t-card>

        <t-card :bordered="false" class="monitor-card">
          <template #title>
            <div class="card-title">
              <h3>查询与 Keyspace</h3>
              <p>操作速率、命中率与键空间</p>
            </div>
          </template>
          <div class="info-list">
            <div class="info-item">
              <span>OPS</span>
              <strong>{{ formatNumber(redisInfo?.stats?.ops) }}</strong>
            </div>
            <div class="info-item">
              <span>已处理指令</span>
              <strong>{{ formatNumber(redisInfo?.stats?.total_commands_processed) }}</strong>
            </div>
            <div class="info-item">
              <span>命中 / 未命中</span>
              <strong>{{ formatNumber(redisInfo?.stats?.keyspace_hits) }} / {{ formatNumber(redisInfo?.stats?.keyspace_misses) }}</strong>
            </div>
            <div class="info-item">
              <span>Key 数量</span>
              <strong>{{ keyCount }}</strong>
            </div>
            <div class="info-item">
              <span>过期 / 淘汰键</span>
              <strong>{{ formatNumber(redisInfo?.stats?.expired_keys) }} / {{ formatNumber(redisInfo?.stats?.evicted_keys) }}</strong>
            </div>
          </div>
        </t-card>
      </div>
    </t-loading>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { MessagePlugin } from 'tdesign-vue-next';

import { getRedisInfo, type RedisInfo } from '@/api/monitor/redis';
import ConsolePageHeader from '@/components/common/ConsolePageHeader.vue';

type SummaryTone = 'blue' | 'green' | 'cyan' | 'orange' | 'red';
type TrendTone = 'up' | 'down' | 'flat' | 'neutral';
type TrendCardTone = 'blue' | 'green' | 'cyan' | 'orange';
type TagTheme = 'success' | 'primary';

interface TrendState {
  text: string;
  tone: TrendTone;
}

interface SummaryItem {
  label: string;
  value: string | number;
  hint: string;
  icon: string;
  tone: SummaryTone;
  trend?: TrendState;
}

interface TrendItem {
  label: string;
  current: string;
  previous: string;
  tone: TrendCardTone;
  trend: TrendState;
}

type RefreshInterval = 5 | 10 | 30 | 60;

interface RedisSnapshot {
  ops: number;
  totalCommands: number;
  clients: number;
  hitRate: number;
  keys: number;
}

defineOptions({
  name: 'MonitorRedis',
});

const redisInfo = ref<RedisInfo | null>(null);
const loading = ref(false);
const loadError = ref('');
const lastUpdatedAt = ref('');
const autoRefreshEnabled = ref(false);
const refreshTimer = ref<ReturnType<typeof setInterval> | null>(null);
const refreshInterval = ref<RefreshInterval>(5);
const timeWindow = ref<'5m' | '15m' | '30m' | '60m'>('15m');
const snapshotHistory = ref<Array<RedisSnapshot>>([]);

const timeWindowOptions = [
  { label: '最近 5 分钟', value: '5m' },
  { label: '最近 15 分钟', value: '15m' },
  { label: '最近 30 分钟', value: '30m' },
  { label: '最近 60 分钟', value: '60m' },
];

const refreshIntervalOptions = [
  { label: '5 秒', value: 5 },
  { label: '10 秒', value: 10 },
  { label: '30 秒', value: 30 },
  { label: '60 秒', value: 60 },
];

const normalizedStatus = computed(() => redisInfo.value?.status?.toLowerCase() || '');
const isHealthy = computed(() => normalizedStatus.value === 'ok' || normalizedStatus.value === 'connected');
const statusText = computed(() => {
  if (isHealthy.value) return '正常';
  return redisInfo.value?.status || '等待数据';
});
const modeText = computed(() => redisInfo.value?.server?.mode || '运行模式');
const blockedClients = computed(() => parseNumeric(redisInfo.value?.clients?.blocked));

const statusTheme = computed<TagTheme>(() => (isHealthy.value ? 'success' : 'primary'));
const headerMeta = computed(() => [
  `Redis ${redisInfo.value?.server?.version || '-'}`,
  modeText.value,
  `端口 ${redisInfo.value?.server?.tcp_port || '-'}`,
  lastUpdatedAt.value ? `更新于 ${lastUpdatedAt.value}` : '',
]);
const keyCount = computed(() => formatNumber(redisInfo.value?.keyspace?.dbsize || redisInfo.value?.stats?.keys));
const clientCount = computed(() => formatNumber(redisInfo.value?.clients?.connected || redisInfo.value?.stats?.connections));

const timeWindowLabel = computed(() => timeWindowOptions.find((item) => item.value === timeWindow.value)?.label || '最近 15 分钟');
const samplesToKeep = computed(() => (timeWindow.value === '5m' ? 2 : timeWindow.value === '15m' ? 4 : timeWindow.value === '30m' ? 6 : 8));

const latestSnapshot = computed<RedisSnapshot | null>(() => {
  const history = snapshotHistory.value;
  return history.length ? history[history.length - 1] : null;
});

const previousSnapshot = computed<RedisSnapshot | null>(() => {
  const history = snapshotHistory.value;
  return history.length > 1 ? history[history.length - 2] : null;
});

const summaryItems = computed<Array<SummaryItem>>(() => {
  return [
    {
      label: '连接状态',
      value: isHealthy.value ? 'OK' : redisInfo.value?.status || '-',
      hint: `${modeText.value} / 端口 ${redisInfo.value?.server?.tcp_port || '-'}`,
      icon: 'data-base',
      tone: isHealthy.value ? 'green' : 'blue',
    },
    {
      label: 'OPS',
      value: formatNumber(redisInfo.value?.stats?.ops),
      hint: `累计 ${formatNumber(redisInfo.value?.stats?.total_commands_processed)}`,
      icon: 'data',
      tone: 'cyan',
      trend: calcTrend({
        current: parseNumeric(redisInfo.value?.stats?.ops),
        previous: previousSnapshot.value?.ops ?? null,
      }),
    },
    {
      label: '当前客户端',
      value: clientCount.value,
      hint: `阻塞 ${formatNumber(redisInfo.value?.clients?.blocked)}`,
      icon: 'server',
      tone: clientTone.value,
      trend: calcTrend({
        current: parseNumeric(redisInfo.value?.clients?.connected || redisInfo.value?.stats?.connections),
        previous: previousSnapshot.value?.clients ?? null,
      }),
    },
    {
      label: '命中率',
      value: redisInfo.value?.stats?.hit_rate || '-',
      hint: `Key ${keyCount.value}`,
      icon: 'check-circle',
      tone: hitRateTone(redisInfo.value?.stats?.hit_rate),
      trend: calcTrend({
        current: parsePercent(redisInfo.value?.stats?.hit_rate),
        previous: previousSnapshot.value?.hitRate ?? null,
      }),
    },
  ];
});

const clientTone = computed<SummaryTone>(() => {
  if (blockedClients.value >= 10) return 'red';
  if (blockedClients.value > 0) return 'orange';
  return 'blue';
});

const trendRows = computed<Array<TrendItem>>(() => {
  const snapshot = latestSnapshot.value;
  const previous = previousSnapshot.value;

  if (!snapshot) {
    return [];
  }

  return [
    {
      label: 'OPS',
      current: formatNumber(redisInfo.value?.stats?.ops),
      previous: previous ? formatNumber(previous.ops) : '-',
      tone: 'cyan',
      trend: calcTrend({ current: snapshot.ops, previous: previous?.ops ?? null }),
    },
    {
      label: '命中率',
      current: redisInfo.value?.stats?.hit_rate || '-',
      previous: previous ? `${previous.hitRate}%` : '-',
      tone: hitRateTone(redisInfo.value?.stats?.hit_rate) === 'orange' ? 'orange' : 'green',
      trend: calcTrend({ current: snapshot.hitRate, previous: previous?.hitRate ?? null }),
    },
    {
      label: '客户端',
      current: formatNumber(redisInfo.value?.clients?.connected || redisInfo.value?.stats?.connections),
      previous: previous ? formatNumber(previous.clients) : '-',
      tone: 'blue',
      trend: calcTrend({
        current: parseNumeric(redisInfo.value?.clients?.connected || redisInfo.value?.stats?.connections),
        previous: previous?.clients ?? null,
      }),
    },
    {
      label: 'Key 数量',
      current: keyCount.value,
      previous: previous ? formatNumber(previous.keys) : '-',
      tone: 'green',
      trend: calcTrend({
        current: parseNumeric(redisInfo.value?.keyspace?.dbsize || redisInfo.value?.stats?.keys),
        previous: previous?.keys ?? null,
      }),
    },
  ];
});

const loadData = async () => {
  if (loading.value) {
    return;
  }

  loading.value = true;
  loadError.value = '';

  try {
    const response = await getRedisInfo();
    redisInfo.value = response;
    lastUpdatedAt.value = new Date().toLocaleTimeString('zh-CN', { hour12: false });
    pushSnapshot(response);
  } catch (error: any) {
    loadError.value = error.message || '加载 Redis 监控失败';
    MessagePlugin.error(loadError.value);
  } finally {
    loading.value = false;
  }
};

const parseNumeric = (value?: number | string | null): number => {
  const num = Number(value ?? 0);
  return Number.isFinite(num) ? num : 0;
};

const parsePercent = (value?: string | number | null): number => {
  if (value === null || value === undefined) return 0;
  const raw = String(value).replace('%', '');
  const parsed = Number.parseFloat(raw);
  return Number.isFinite(parsed) ? parsed : 0;
};

const calcTrend = ({ current, previous }: { current: number; previous: number | null }): TrendState => {
  if (previous === null) {
    return {
      text: '首次采样',
      tone: 'neutral' as TrendTone,
    };
  }

  if (!previous) {
    return {
      text: current ? '新增采样' : '持平',
      tone: current ? 'up' : 'flat',
    };
  }

  const delta = ((current - previous) / previous) * 100;
  if (Math.abs(delta) < 0.05) {
    return {
      text: '持平',
      tone: 'flat',
    };
  }

  return {
    text: `${delta > 0 ? '上升' : '下降'} ${Math.abs(delta).toFixed(1)}%`,
    tone: delta > 0 ? 'up' : 'down',
  };
};

const formatUptime = (value?: string | number): string => {
  if (!value) return '-';
  const sec = Number.parseInt(String(value), 10);
  if (Number.isNaN(sec)) return String(value);
  const days = Math.floor(sec / 86400);
  const hours = Math.floor((sec % 86400) / 3600);
  const minutes = Math.floor((sec % 3600) / 60);
  return `${days}天${hours}小时 ${minutes}分钟`;
};

const formatNumber = (value?: number | string): string => {
  const num = Number(value || 0);
  if (!Number.isFinite(num)) return '0';
  return num.toLocaleString();
};

const formatBytes = (bytes?: number, fallback?: string): string => {
  if (typeof bytes === 'number' && Number.isFinite(bytes) && bytes > 0) {
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let value = bytes;
    let unitIndex = 0;
    while (value >= 1024 && unitIndex < units.length - 1) {
      value /= 1024;
      unitIndex += 1;
    }
    return `${value.toFixed(value >= 10 || unitIndex === 0 ? 0 : 2)} ${units[unitIndex]}`;
  }
  return normalizeMemoryText(fallback);
};

const normalizeMemoryText = (value?: string | number): string => {
  if (value === null || value === undefined || value === '') return '-';
  const raw = String(value).trim().split(/\s+/)[0];
  if (!raw || raw === '-') return '-';
  const matched = raw.match(/^([\d.]+)\s*([KMGT])?B?$/i);
  if (!matched) return raw;
  const numeric = matched[1];
  const unit = (matched[2] || 'B').toUpperCase();
  if (unit === 'B') return `${numeric} B`;
  if (unit === 'K') return `${numeric} KB`;
  if (unit === 'M') return `${numeric} MB`;
  if (unit === 'G') return `${numeric} GB`;
  if (unit === 'T') return `${numeric} TB`;
  return `${numeric} ${unit}`;
};

const normalizeMaxMemory = (value?: string): string => {
  const normalized = normalizeMemoryText(value);
  return normalized === '0 B' || normalized === '0B' ? '未限制' : normalized;
};

const hitRateTone = (value?: string): SummaryTone => {
  const rate = parsePercent(value);
  if (!rate) return 'blue';
  if (rate < 80) return 'orange';
  return 'green';
};

const pushSnapshot = (info: RedisInfo) => {
  const snapshot: RedisSnapshot = {
    ops: parseNumeric(info.stats?.ops),
    totalCommands: parseNumeric(info.stats?.total_commands_processed),
    clients: parseNumeric(info.clients?.connected || info.stats?.connections),
    hitRate: parsePercent(info.stats?.hit_rate),
    keys: parseNumeric(info.keyspace?.dbsize || info.stats?.keys),
  };
  const keep = samplesToKeep.value;
  snapshotHistory.value = [...snapshotHistory.value.slice(-Math.max(0, keep - 1)), snapshot];
};

const setupAutoRefresh = () => {
  if (refreshTimer.value) {
    clearInterval(refreshTimer.value);
    refreshTimer.value = null;
  }
  if (!autoRefreshEnabled.value) {
    return;
  }
  refreshTimer.value = setInterval(() => {
    loadData();
  }, refreshInterval.value * 1000);
};

watch([autoRefreshEnabled, refreshInterval], () => {
  setupAutoRefresh();
});

watch(timeWindow, () => {
  const keep = samplesToKeep.value;
  if (snapshotHistory.value.length > keep) {
    snapshotHistory.value = snapshotHistory.value.slice(-keep);
  }
});

onMounted(() => {
  loadData();
  setupAutoRefresh();
});

onBeforeUnmount(() => {
  if (refreshTimer.value) {
    clearInterval(refreshTimer.value);
    refreshTimer.value = null;
  }
});
</script>

<style scoped lang="less">
.redis-page {
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

.redis-page :deep(.t-card),
.redis-page :deep(.t-button),
.redis-page :deep(.t-tag),
.redis-page :deep(.t-alert),
.redis-page :deep(.t-empty),
.redis-page :deep(.t-select) {
  font-family: var(--monitor-font);
}

.redis-head {
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

.redis-head__main {
  min-width: 0;
}

.redis-head__title {
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

.redis-head__meta {
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

.redis-head__actions {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  flex-wrap: wrap;
}

.redis-head__select {
  width: 118px;
}

.redis-head__auto-wrap {
  display: inline-flex;
  height: 32px;
  align-items: center;
  gap: 6px;
  padding: 0 10px;
  border: 1px solid #e2e8f0;
  border-radius: 999px;
  background: #fff;
}

.redis-head__auto-wrap--active {
  border-color: #bfdbfe;
  background: #eff6ff;
}

.redis-head__auto-label {
  color: #334155;
  font-size: 12px;
  font-weight: 600;
}

.redis-head__refresh {
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

.summary-panel__trend {
  font-weight: 700;
}

.summary-panel__trend--up {
  color: #16a34a;
}

.summary-panel__trend--down {
  color: #dc2626;
}

.summary-panel__trend--flat {
  color: #0f766e;
}

.summary-panel__trend--neutral {
  color: #64748b;
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

.trend-section {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.trend-section__head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 12px;
}

.trend-card-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.trend-card {
  position: relative;
  min-height: 104px;
  overflow: hidden;
  padding: 14px 16px;
  border: 1px solid #e8edf5;
  border-radius: 12px;
  background: #fff;
  box-shadow: var(--monitor-card-shadow);

  &::before {
    position: absolute;
    top: 0;
    left: 0;
    width: 100%;
    height: 3px;
    background: var(--trend-accent);
    content: '';
  }

  strong {
    display: block;
    margin-top: 12px;
    color: #0f172a;
    font-family: var(--monitor-number-font);
    font-size: 26px;
    font-weight: 800;
    line-height: 30px;
  }

  small {
    display: block;
    margin-top: 6px;
    color: #64748b;
    font-size: 12px;
  }
}

.trend-card--blue {
  --trend-accent: #2563eb;
}

.trend-card--green {
  --trend-accent: #059669;
}

.trend-card--cyan {
  --trend-accent: #0284c7;
}

.trend-card--orange {
  --trend-accent: #ea580c;
}

.trend-card__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  color: #334155;
  font-size: 13px;
  font-weight: 700;
}

.trend-card__delta {
  flex-shrink: 0;
  font-size: 12px;
  font-weight: 700;
}

.trend-card__delta--up {
  color: #16a34a;
}

.trend-card__delta--down {
  color: #dc2626;
}

.trend-card__delta--flat {
  color: #0f766e;
}

.trend-card__delta--neutral {
  color: #64748b;
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

.info-item__value--truncate {
  display: block;
  max-width: 250px;
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
  .monitor-grid,
  .summary-grid,
  .trend-card-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (width <= 1200px) {
  .redis-head__actions {
    width: 100%;
  }

  .redis-head__select {
    width: 140px;
  }
}

@media (width <= 768px) {
  .redis-page {
    margin: calc(-1 * var(--td-comp-paddingTB-l)) calc(-1 * var(--td-comp-paddingLR-l));
    padding: 12px;
  }

  .redis-head {
    align-items: stretch;
    flex-direction: column;
  }

  .redis-head__actions {
    justify-content: flex-start;
  }

  .summary-grid,
  .trend-card-grid,
  .monitor-grid {
    grid-template-columns: 1fr;
  }

  .trend-section__head {
    align-items: flex-start;
    flex-direction: column;
  }

  .redis-head__select {
    width: 100%;
  }

  .redis-head__auto-wrap {
    width: 100%;
    justify-content: space-between;
  }
}
</style>
