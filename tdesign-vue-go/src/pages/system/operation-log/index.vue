<template>
  <div class="operation-log-page system-management-page">
    <console-page-header>
      <template #title>
        <h2>操作日志</h2>
        <t-tag :theme="failedCount > 0 ? 'warning' : 'success'" variant="light">
          {{ failedCount > 0 ? '存在异常操作' : '操作正常' }}
        </t-tag>
      </template>
      <template #meta>
        <span>审计追踪</span>
        <span>请求链路</span>
        <span>权限行为</span>
        <span>共 {{ totalCount }} 条记录</span>
        <span v-if="lastUpdatedAt">更新于 {{ lastUpdatedAt }}</span>
      </template>
      <template #actions>
        <t-tag :theme="successRateValue >= 95 ? 'success' : 'warning'" variant="light">成功率 {{ successRate }}</t-tag>
        <t-button variant="outline" :loading="loading || statsLoading" @click="handleRefresh">
          <template #icon><t-icon name="refresh" /></template>
          刷新
        </t-button>
        <t-button theme="primary" variant="outline" :loading="exportLoading" @click="handleExport">
          <template #icon><t-icon name="download" /></template>
          导出
        </t-button>
      </template>
    </console-page-header>

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

    <t-card :bordered="false" class="filter-card">
      <div class="filter-card__head">
        <div>
          <h3>筛选查询</h3>
          <p>
            按操作者、请求标识、方法、路径和状态码定位审计记录
            <template v-if="activeFilterCount"> · 已应用 {{ activeFilterCount }} 个条件</template>
          </p>
        </div>
        <t-space size="small" break-line>
          <t-tag theme="primary" variant="light">当前页 {{ tableData.length }} 条</t-tag>
          <t-tag :theme="currentPageFailedCount > 0 ? 'danger' : 'success'" variant="light">
            本页异常 {{ currentPageFailedCount }}
          </t-tag>
        </t-space>
      </div>

      <t-form :data="searchForm" class="filter-form" layout="inline" @submit="handleSearch">
        <t-form-item label="用户名" name="username">
          <t-input
            v-model="searchForm.username"
            clearable
            class="keyword-input"
            placeholder="输入用户名"
            @enter="handleSearch"
          />
        </t-form-item>
        <t-form-item label="Actor 类型" name="actor_type">
          <t-select v-model="searchForm.actor_type" clearable placeholder="全部类型" class="filter-select">
            <t-option label="操作员" value="operator" />
            <t-option label="客户端" value="client" />
            <t-option label="系统" value="system" />
          </t-select>
        </t-form-item>
        <t-form-item label="Actor ID" name="actor_id">
          <t-input
            v-model="searchForm.actor_id"
            clearable
            class="filter-input"
            placeholder="输入 Actor ID"
            @enter="handleSearch"
          />
        </t-form-item>
        <t-form-item label="Request ID" name="request_id">
          <t-input
            v-model="searchForm.request_id"
            clearable
            class="request-input"
            placeholder="输入 Request ID"
            @enter="handleSearch"
          />
        </t-form-item>
        <t-form-item label="方法" name="method">
          <t-select v-model="searchForm.method" clearable placeholder="全部方法" class="method-select">
            <t-option v-for="method in methodOptions" :key="method" :label="method" :value="method" />
          </t-select>
        </t-form-item>
        <t-form-item label="路径" name="path">
          <t-input v-model="searchForm.path" clearable class="path-input" placeholder="/api/path" @enter="handleSearch" />
        </t-form-item>
        <t-form-item label="状态码" name="status">
          <t-select v-model="searchForm.status" clearable placeholder="全部状态" class="status-select">
            <t-option label="200 成功" value="200" />
            <t-option label="400 请求失败" value="400" />
            <t-option label="401 未授权" value="401" />
            <t-option label="403 无权限" value="403" />
            <t-option label="500 服务异常" value="500" />
          </t-select>
        </t-form-item>
      </t-form>

      <div class="filter-card__actions">
        <t-space size="small" break-line>
          <t-button theme="primary" :loading="loading" @click="handleSearch">
            <template #icon><t-icon name="search" /></template>
            查询
          </t-button>
          <t-button variant="base" :disabled="loading" @click="handleReset">重置</t-button>
          <t-button variant="outline" :loading="loading" @click="loadData">
            <template #icon><t-icon name="refresh" /></template>
            刷新列表
          </t-button>
        </t-space>
        <t-space size="small" break-line>
          <t-tag v-if="topMethod" theme="primary" variant="light">高频方法 {{ topMethod }}</t-tag>
          <t-tag v-if="topStatus" :theme="isSuccessStatus(Number(topStatus)) ? 'success' : 'danger'" variant="light">
            高频状态 {{ topStatus }}
          </t-tag>
        </t-space>
      </div>
    </t-card>

    <t-card :bordered="false" class="table-card">
      <div class="table-card__head">
        <div>
          <h3>操作明细</h3>
          <p>
            操作者、请求路径、状态码、耗时和错误信息
            <template v-if="pagination.total"> · 共 {{ pagination.total }} 条</template>
          </p>
        </div>
        <t-space size="small">
          <t-tag :theme="slowRequestCount > 0 ? 'warning' : 'success'" variant="light">
            慢请求 {{ slowRequestCount }}
          </t-tag>
          <t-tag theme="default" variant="light">分页 {{ pagination.current }} / {{ totalPages }}</t-tag>
        </t-space>
      </div>
      <t-table
        row-key="id"
        hover
        class="operation-table"
        table-layout="fixed"
        :data="tableData"
        :columns="columns"
        :loading="loading"
        :pagination="pagination"
        @page-change="handlePageChange"
        @page-size-change="handlePageSizeChange"
      >
        <template #empty>
          <t-empty :description="loading ? '正在加载操作日志' : '当前筛选条件下暂无操作日志'" />
        </template>
        <template #actor="{ row }">
          <div class="actor-cell">
            <span class="actor-avatar">{{ actorInitial(row) }}</span>
            <div class="actor-cell__main">
              <strong>{{ row.username || actorTypeLabel(row.actor_type) }}</strong>
              <span>{{ actorTypeLabel(row.actor_type) }} · {{ row.actor_id || '-' }}</span>
            </div>
          </div>
        </template>
        <template #request="{ row }">
          <div class="request-cell">
            <div class="request-cell__line">
              <t-tag size="small" :theme="methodTheme(row.method)" variant="light">{{ row.method || '-' }}</t-tag>
              <strong :title="row.path">{{ row.path || '-' }}</strong>
            </div>
            <span class="mono-text" :title="row.request_id">RID: {{ row.request_id || '-' }}</span>
          </div>
        </template>
        <template #module="{ row }">
          <div class="module-cell">
            <strong>{{ row.module || '未归类' }}</strong>
            <span>{{ row.action || '未标记操作' }}</span>
          </div>
        </template>
        <template #status="{ row }">
          <t-tag :theme="statusTheme(row.status)" variant="light">{{ row.status }}</t-tag>
        </template>
        <template #latency="{ row }">
          <div class="latency-cell" :class="{ 'latency-cell--slow': isSlowRequest(row.latency) }">
            <strong>{{ formatLatency(row.latency) }}</strong>
            <span>{{ latencyLabel(row.latency) }}</span>
          </div>
        </template>
        <template #ip="{ row }">
          <span class="mono-text ip-text">{{ row.ip || '-' }}</span>
        </template>
        <template #message="{ row }">
          <span class="message-text" :class="{ 'message-text--danger': !isSuccessStatus(row.status) }" :title="row.error_msg">
            {{ row.error_msg || (isSuccessStatus(row.status) ? '请求完成' : '请求异常') }}
          </span>
        </template>
        <template #created_at="{ row }">
          <span class="mono-text">{{ formatDateTime(row.created_at) }}</span>
        </template>
        <template #operation="{ row }">
          <t-link theme="primary" hover="color" @click="handleViewDetail(row)">详情</t-link>
        </template>
      </t-table>
    </t-card>

    <t-drawer v-model:visible="detailVisible" :header="detailTitle" size="760px" :footer="false">
      <t-loading :loading="detailLoading" size="small">
        <div v-if="currentLog" class="detail-panel">
          <div class="detail-hero" :class="{ 'detail-hero--danger': !isSuccessStatus(currentLog.status) }">
            <span class="detail-hero__icon">
              <t-icon :name="isSuccessStatus(currentLog.status) ? 'check-circle' : 'shield-error'" />
            </span>
            <div>
              <strong>{{ currentLog.status }} · {{ isSuccessStatus(currentLog.status) ? '请求完成' : '请求异常' }}</strong>
              <span>{{ currentLog.error_msg || currentLog.action || currentLog.path || '-' }}</span>
            </div>
          </div>

          <t-descriptions bordered :column="2" class="detail-desc">
            <t-descriptions-item label="用户名">{{ currentLog.username || '-' }}</t-descriptions-item>
            <t-descriptions-item label="用户 ID">{{ currentLog.user_id || '-' }}</t-descriptions-item>
            <t-descriptions-item label="Actor 类型">{{ actorTypeLabel(currentLog.actor_type) }}</t-descriptions-item>
            <t-descriptions-item label="Actor ID">{{ currentLog.actor_id || '-' }}</t-descriptions-item>
            <t-descriptions-item label="Request ID">
              <span class="mono-text">{{ currentLog.request_id || '-' }}</span>
            </t-descriptions-item>
            <t-descriptions-item label="模块">{{ currentLog.module || '-' }}</t-descriptions-item>
            <t-descriptions-item label="操作">{{ currentLog.action || '-' }}</t-descriptions-item>
            <t-descriptions-item label="方法">{{ currentLog.method || '-' }}</t-descriptions-item>
            <t-descriptions-item label="路径">
              <span class="mono-text">{{ currentLog.path || '-' }}</span>
            </t-descriptions-item>
            <t-descriptions-item label="状态码">{{ currentLog.status }}</t-descriptions-item>
            <t-descriptions-item label="响应耗时">{{ formatLatency(currentLog.latency) }}</t-descriptions-item>
            <t-descriptions-item label="IP 地址">
              <span class="mono-text">{{ currentLog.ip || '-' }}</span>
            </t-descriptions-item>
            <t-descriptions-item label="登录地点">{{ currentLog.location || '-' }}</t-descriptions-item>
            <t-descriptions-item label="操作时间">{{ formatDateTime(currentLog.created_at) }}</t-descriptions-item>
          </t-descriptions>

          <div class="detail-code-grid">
            <section class="payload-panel">
              <div class="payload-panel__head">
                <span>请求体</span>
                <t-tag size="small" variant="light">{{ currentLog.request_body ? '已记录' : '无内容' }}</t-tag>
              </div>
              <pre>{{ formatPayload(currentLog.request_body) }}</pre>
            </section>
            <section class="payload-panel">
              <div class="payload-panel__head">
                <span>响应体</span>
                <t-tag size="small" :theme="currentLog.response_body ? 'primary' : 'default'" variant="light">
                  {{ currentLog.response_body ? '已记录' : '无内容' }}
                </t-tag>
              </div>
              <pre>{{ formatPayload(currentLog.response_body) }}</pre>
            </section>
          </div>

          <div class="user-agent-box">
            <span>用户代理</span>
            <p>{{ currentLog.user_agent || '暂无 User-Agent' }}</p>
          </div>
        </div>
      </t-loading>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { MessagePlugin } from 'tdesign-vue-next';
import { computed, onMounted, ref } from 'vue';

import {
  exportOperationLogs,
  getOperationLogDetail,
  getOperationLogs,
  getOperationLogStats,
  type OperationLogItem,
  type OperationLogStats,
} from '@/api/system/operationLog';
import { formatDateTime } from '@/utils/date';
import ConsolePageHeader from '@/components/common/ConsolePageHeader.vue';

type TagTheme = 'default' | 'success' | 'primary' | 'warning' | 'danger';
type SummaryTone = 'blue' | 'green' | 'cyan' | 'orange' | 'red';

defineOptions({
  name: 'SystemOperationLog',
});

const loading = ref(false);
const statsLoading = ref(false);
const exportLoading = ref(false);
const detailLoading = ref(false);
const tableData = ref<OperationLogItem[]>([]);
const stats = ref<OperationLogStats | null>(null);
const detailVisible = ref(false);
const currentLog = ref<OperationLogItem | null>(null);
const lastUpdatedAt = ref('');

const searchForm = ref({
  username: '',
  actor_type: '',
  actor_id: '',
  request_id: '',
  method: '',
  path: '',
  status: '',
});

const pagination = ref({
  current: 1,
  pageSize: 10,
  total: 0,
});

const methodOptions = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'];

const columns: any[] = [
  { colKey: 'actor', title: '操作者', width: 240, fixed: 'left' as const },
  { colKey: 'request', title: '请求', minWidth: 300 },
  { colKey: 'module', title: '模块 / 操作', width: 180 },
  { colKey: 'status', title: '状态码', width: 100 },
  { colKey: 'latency', title: '耗时', width: 110 },
  { colKey: 'ip', title: 'IP 地址', width: 150 },
  { colKey: 'message', title: '结果说明', minWidth: 200 },
  { colKey: 'created_at', title: '操作时间', width: 180 },
  { colKey: 'operation', title: '操作', width: 90, fixed: 'right' as const },
];

const currentPageSuccessCount = computed(() => tableData.value.filter((item) => isSuccessStatus(item.status)).length);
const currentPageFailedCount = computed(() => tableData.value.filter((item) => !isSuccessStatus(item.status)).length);
const slowRequestCount = computed(() => tableData.value.filter((item) => isSlowRequest(item.latency)).length);
const totalCount = computed(() => stats.value?.total ?? pagination.value.total ?? tableData.value.length);
const successCount = computed(() => stats.value?.success ?? currentPageSuccessCount.value);
const failedCount = computed(() => stats.value?.failed ?? currentPageFailedCount.value);
const todayCount = computed(() => stats.value?.today ?? currentTodayCount.value);
const totalPages = computed(() => Math.max(1, Math.ceil((pagination.value.total || 0) / pagination.value.pageSize)));
const successRateValue = computed(() => {
  if (!totalCount.value) return 0;
  return Math.round((successCount.value / totalCount.value) * 100);
});
const successRate = computed(() => `${successRateValue.value}%`);

const activeFilterCount = computed(() => {
  const values = searchForm.value;
  return Object.keys(values).filter((key) => String(values[key as keyof typeof values]).trim()).length;
});

const currentTodayCount = computed(() => {
  const today = new Date().toDateString();
  return tableData.value.filter((item) => {
    if (!item.created_at) return false;
    return new Date(item.created_at).toDateString() === today;
  }).length;
});

const topMethod = computed(() => getTopEntry(stats.value?.by_method));
const topStatus = computed(() => getTopEntry(stats.value?.by_status));

const summaryItems = computed<Array<{ label: string; value: string | number; hint: string; icon: string; tone: SummaryTone }>>(() => [
  {
    label: '日志总数',
    value: totalCount.value,
    hint: `当前筛选 ${pagination.value.total || tableData.value.length} 条`,
    icon: 'data-search',
    tone: 'blue',
  },
  {
    label: '成功操作',
    value: successCount.value,
    hint: `成功率 ${successRate.value}`,
    icon: 'check-circle',
    tone: 'green',
  },
  {
    label: '异常操作',
    value: failedCount.value,
    hint: failedCount.value > 0 ? '需要查看错误与请求体' : '暂无异常记录',
    icon: 'shield-error',
    tone: failedCount.value > 0 ? 'red' : 'cyan',
  },
  {
    label: '今日操作',
    value: todayCount.value,
    hint: slowRequestCount.value > 0 ? `${slowRequestCount.value} 条慢请求` : '当前页耗时正常',
    icon: 'time',
    tone: slowRequestCount.value > 0 ? 'orange' : 'cyan',
  },
]);

const detailTitle = computed(() => {
  if (!currentLog.value) return '操作详情';
  return `${currentLog.value.module || '操作日志'} · ${formatDateTime(currentLog.value.created_at)}`;
});

const updateTime = () => {
  lastUpdatedAt.value = new Date().toLocaleTimeString('zh-CN', { hour12: false });
};

const buildSearchParams = () => ({
  page: pagination.value.current,
  page_size: pagination.value.pageSize,
  username: searchForm.value.username.trim() || undefined,
  actor_type: searchForm.value.actor_type || undefined,
  actor_id: searchForm.value.actor_id.trim() || undefined,
  request_id: searchForm.value.request_id.trim() || undefined,
  method: searchForm.value.method || undefined,
  path: searchForm.value.path.trim() || undefined,
  status: searchForm.value.status ? Number(searchForm.value.status) : undefined,
});

const loadStats = async () => {
  statsLoading.value = true;
  try {
    stats.value = await getOperationLogStats();
  } catch (error) {
    console.error('加载操作日志统计失败:', error);
  } finally {
    statsLoading.value = false;
  }
};

const loadData = async () => {
  loading.value = true;
  try {
    const res = await getOperationLogs(buildSearchParams());
    tableData.value = res.list || [];
    pagination.value.total = res.total || 0;
    updateTime();
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载操作日志失败');
  } finally {
    loading.value = false;
  }
};

const openBlob = (blob: Blob, filename: string) => {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
};

const handleExport = async () => {
  exportLoading.value = true;
  try {
    const blob = await exportOperationLogs(buildSearchParams());
    const filename = `operation_logs_${new Date().toISOString().replace(/[-:T.Z]/g, '').slice(0, 14)}.csv`;
    openBlob(blob, filename);
    MessagePlugin.success('操作日志已导出');
  } catch (error: any) {
    MessagePlugin.error(error.message || '导出失败');
  } finally {
    exportLoading.value = false;
  }
};

const handleViewDetail = async (row: OperationLogItem) => {
  currentLog.value = row;
  detailVisible.value = true;
  detailLoading.value = true;
  try {
    currentLog.value = await getOperationLogDetail(row.id);
  } catch (error) {
    console.error('加载操作日志详情失败:', error);
  } finally {
    detailLoading.value = false;
  }
};

const handleSearch = () => {
  pagination.value.current = 1;
  loadData();
};

const handleReset = () => {
  searchForm.value = {
    username: '',
    actor_type: '',
    actor_id: '',
    request_id: '',
    method: '',
    path: '',
    status: '',
  };
  handleSearch();
};

const handleRefresh = () => {
  loadData();
  loadStats();
};

const handlePageChange = (pageInfo: { current?: number; pageSize?: number } | number) => {
  if (typeof pageInfo === 'number') {
    pagination.value.current = pageInfo;
  } else {
    pagination.value.current = pageInfo.current ?? pagination.value.current;
    pagination.value.pageSize = pageInfo.pageSize ?? pagination.value.pageSize;
  }
  loadData();
};

const handlePageSizeChange = (pageSize: number) => {
  pagination.value.pageSize = pageSize;
  pagination.value.current = 1;
  loadData();
};

const isSuccessStatus = (status: number) => status >= 200 && status < 400;
const isSlowRequest = (latency?: number) => Number(latency || 0) >= 1000;

const statusTheme = (status: number): TagTheme => {
  if (status >= 200 && status < 300) return 'success';
  if (status >= 300 && status < 400) return 'primary';
  if (status >= 400 && status < 500) return 'warning';
  return 'danger';
};

const methodTheme = (method?: string): TagTheme => {
  if (method === 'GET') return 'primary';
  if (method === 'POST') return 'success';
  if (method === 'DELETE') return 'danger';
  if (method === 'PUT' || method === 'PATCH') return 'warning';
  return 'default';
};

const actorTypeLabel = (type?: string) => {
  if (type === 'operator') return '操作员';
  if (type === 'client') return '客户端';
  if (type === 'system') return '系统';
  return '未知来源';
};

const actorInitial = (row: OperationLogItem) => {
  const source = row.username || actorTypeLabel(row.actor_type) || 'O';
  return source.slice(0, 1).toUpperCase();
};

const formatLatency = (latency?: number) => {
  if (latency === undefined || latency === null) return '-';
  if (latency >= 1000) return `${(latency / 1000).toFixed(2)}s`;
  return `${latency}ms`;
};

const latencyLabel = (latency?: number) => {
  if (latency === undefined || latency === null) return '未记录';
  if (latency >= 1000) return '偏慢';
  if (latency >= 500) return '一般';
  return '正常';
};

const getTopEntry = (record?: Record<string, number>) => {
  if (!record) return '';
  const sorted = Object.entries(record).sort((a, b) => b[1] - a[1]);
  return sorted[0]?.[0] || '';
};

const formatPayload = (payload?: string) => {
  if (!payload) return '暂无内容';
  try {
    return JSON.stringify(JSON.parse(payload), null, 2);
  } catch {
    return payload;
  }
};

onMounted(() => {
  loadData();
  loadStats();
});
</script>

<style lang="less" scoped>
.operation-log-page {
  --operation-bg: #f5f7fb;
  --operation-card-shadow: 0 12px 28px rgb(15 23 42 / 6%);
  --operation-font: 'Inter', 'HarmonyOS Sans SC', 'MiSans', 'PingFang SC', 'Microsoft YaHei UI', 'Microsoft YaHei', 'Arial', sans-serif;
  --operation-number-font: 'DIN Alternate', 'Bahnschrift', 'Inter', 'HarmonyOS Sans SC', 'Microsoft YaHei UI', sans-serif;

  display: flex;
  min-height: calc(100vh - 120px);
  min-width: 0;
  flex-direction: column;
  gap: 14px;
  margin: calc(-1 * var(--td-comp-paddingTB-xl)) calc(-1 * var(--td-comp-paddingLR-xl));
  padding: 14px 18px 18px;
  background: var(--operation-bg);
  color: var(--td-text-color-primary);
  font-family: var(--operation-font);
  font-feature-settings: 'tnum';
  font-variant-numeric: tabular-nums;
  letter-spacing: 0;
  text-rendering: optimizelegibility;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}

.operation-log-page :deep(.t-card),
.operation-log-page :deep(.t-table),
.operation-log-page :deep(.t-form),
.operation-log-page :deep(.t-button),
.operation-log-page :deep(.t-tag),
.operation-log-page :deep(.t-input),
.operation-log-page :deep(.t-select),
.operation-log-page :deep(.t-drawer),
.operation-log-page :deep(.t-empty) {
  font-family: var(--operation-font);
}

.operation-log-head {
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

.operation-log-head__main {
  min-width: 0;
}

.operation-log-head__title {
  display: flex;
  align-items: center;
  gap: 10px;

  h2 {
    margin: 0;
    color: #111827;
    font-size: 24px;
    font-weight: 700;
    line-height: 32px;
  }
}

.operation-log-head__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 7px 10px;
  margin-top: 6px;
  color: #64748b;
  font-size: 13px;
  line-height: 20px;

  span {
    display: inline-flex;
    align-items: center;

    &::after {
      width: 3px;
      height: 3px;
      margin-left: 10px;
      border-radius: 999px;
      background: #cbd5e1;
      content: '';
    }

    &:last-child::after {
      display: none;
    }
  }
}

.operation-log-head__actions {
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
    right: -26px;
    bottom: -30px;
    width: 96px;
    height: 96px;
    border-radius: 50%;
    background: rgb(255 255 255 / 44%);
    content: '';
  }
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
    font-weight: 600;
  }

  strong {
    color: #0f172a;
    font-family: var(--operation-number-font);
    font-size: 34px;
    font-weight: 800;
    line-height: 38px;
  }

  small {
    overflow: hidden;
    max-width: 190px;
    color: #64748b;
    font-size: 12px;
    line-height: 18px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.summary-panel__icon {
  position: relative;
  z-index: 1;
  display: inline-flex;
  width: 38px;
  height: 38px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border-radius: 12px;
  background: rgb(255 255 255 / 58%);
  color: var(--summary-color);
  font-size: 21px;
  box-shadow: inset 0 0 0 1px rgb(255 255 255 / 38%);
}

.summary-panel--blue {
  --summary-bg-start: #dbeafe;
  --summary-bg-end: #bfdbfe;
  --summary-border: #bfdbfe;
  --summary-color: #2563eb;
}

.summary-panel--green {
  --summary-bg-start: #dcfce7;
  --summary-bg-end: #bbf7d0;
  --summary-border: #bbf7d0;
  --summary-color: #059669;
}

.summary-panel--cyan {
  --summary-bg-start: #cffafe;
  --summary-bg-end: #bae6fd;
  --summary-border: #bae6fd;
  --summary-color: #0284c7;
}

.summary-panel--orange {
  --summary-bg-start: #ffedd5;
  --summary-bg-end: #fed7aa;
  --summary-border: #fed7aa;
  --summary-color: #ea580c;
}

.summary-panel--red {
  --summary-bg-start: #fee2e2;
  --summary-bg-end: #fecaca;
  --summary-border: #fecaca;
  --summary-color: #dc2626;
}

.filter-card,
.table-card {
  overflow: hidden;
  border: 1px solid #e8edf5;
  border-radius: 12px;
  background: #fff;
  box-shadow: var(--operation-card-shadow);
}

.filter-card :deep(.t-card__body),
.table-card :deep(.t-card__body) {
  padding: 0;
}

.filter-card__head,
.table-card__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 14px;
  padding: 18px 20px 14px;
  border-bottom: 1px solid #edf1f7;

  h3 {
    margin: 0;
    color: #111827;
    font-size: 18px;
    font-weight: 700;
    line-height: 24px;
  }

  p {
    margin: 5px 0 0;
    color: #64748b;
    font-size: 13px;
    line-height: 20px;
  }
}

.filter-form {
  display: flex;
  flex-wrap: wrap;
  gap: 12px 14px;
  padding: 16px 20px 4px;
}

.filter-form :deep(.t-form__item) {
  margin: 0;
}

.filter-form :deep(.t-form__label) {
  color: #475569;
  font-weight: 600;
}

.keyword-input,
.filter-input {
  width: 190px;
}

.filter-select,
.method-select,
.status-select {
  width: 150px;
}

.request-input {
  width: 230px;
}

.path-input {
  width: 260px;
}

.filter-card__actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 12px 20px 18px;
}

.operation-table {
  width: 100%;
}

.operation-table :deep(.t-table__header th) {
  background: #f8fafc;
  color: #475569;
  font-size: 13px;
  font-weight: 700;
}

.operation-table :deep(.t-table__body tr:hover td) {
  background: #f8fbff;
}

.operation-table :deep(.t-table__body td) {
  padding-top: 14px;
  padding-bottom: 14px;
  border-bottom-color: #eef2f7;
  color: #1f2937;
  vertical-align: top;
}

.actor-cell {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
}

.actor-avatar {
  display: inline-flex;
  width: 34px;
  height: 34px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: linear-gradient(135deg, #2563eb, #0f766e);
  color: #fff;
  font-size: 14px;
  font-weight: 800;
}

.actor-cell__main,
.module-cell,
.request-cell,
.latency-cell {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 3px;

  strong {
    overflow: hidden;
    color: #111827;
    font-size: 14px;
    font-weight: 700;
    line-height: 20px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  span {
    overflow: hidden;
    color: #64748b;
    font-size: 12px;
    line-height: 18px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.request-cell__line {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;

  strong {
    min-width: 0;
  }
}

.latency-cell {
  strong {
    color: #047857;
    font-family: var(--operation-number-font);
  }
}

.latency-cell--slow {
  strong,
  span {
    color: #d97706;
  }
}

.mono-text {
  font-family: var(--operation-number-font);
  font-variant-numeric: tabular-nums;
}

.ip-text {
  color: #334155;
  font-size: 13px;
}

.message-text {
  display: inline-flex;
  overflow: hidden;
  max-width: 100%;
  align-items: center;
  padding: 3px 9px;
  border-radius: 999px;
  background: #eefdf4;
  color: #047857;
  font-size: 12px;
  line-height: 18px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.message-text--danger {
  background: #fff1f2;
  color: #dc2626;
}

.detail-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
  font-family: var(--operation-font);
}

.detail-hero {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px;
  border: 1px solid #bbf7d0;
  border-radius: 12px;
  background: linear-gradient(135deg, #f0fdf4, #dcfce7);

  strong {
    display: block;
    color: #14532d;
    font-size: 18px;
    font-weight: 800;
    line-height: 26px;
  }

  span {
    color: #166534;
    font-size: 13px;
    line-height: 20px;
  }
}

.detail-hero--danger {
  border-color: #fecaca;
  background: linear-gradient(135deg, #fff1f2, #fee2e2);

  strong {
    color: #991b1b;
  }

  span {
    color: #b91c1c;
  }
}

.detail-hero__icon {
  display: inline-flex;
  width: 42px;
  height: 42px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border-radius: 14px;
  background: rgb(255 255 255 / 68%);
  font-size: 24px;
}

.detail-desc :deep(.t-descriptions__label) {
  width: 104px;
  color: #64748b;
  font-weight: 600;
}

.detail-code-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.payload-panel {
  overflow: hidden;
  border: 1px solid #e8edf5;
  border-radius: 12px;
  background: #0f172a;
}

.payload-panel__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 10px 12px;
  border-bottom: 1px solid rgb(255 255 255 / 10%);
  color: #e2e8f0;
  font-size: 13px;
  font-weight: 700;
}

.payload-panel pre {
  max-height: 260px;
  margin: 0;
  overflow: auto;
  padding: 12px;
  color: #dbeafe;
  font-family: var(--operation-number-font);
  font-size: 12px;
  line-height: 20px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.user-agent-box {
  overflow: hidden;
  border: 1px solid #e8edf5;
  border-radius: 12px;
  background: #f8fafc;

  span {
    display: block;
    padding: 10px 12px;
    border-bottom: 1px solid #e8edf5;
    color: #475569;
    font-size: 13px;
    font-weight: 700;
  }

  p {
    margin: 0;
    padding: 12px;
    color: #334155;
    font-family: var(--operation-number-font);
    font-size: 12px;
    line-height: 20px;
    word-break: break-all;
  }
}

@media (width <= 1320px) {
  .summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (width <= 980px) {
  .detail-code-grid {
    grid-template-columns: 1fr;
  }
}

@media (width <= 768px) {
  .operation-log-page {
    margin: calc(-1 * var(--td-comp-paddingTB-l)) calc(-1 * var(--td-comp-paddingLR-l));
    padding: 12px;
  }

  .operation-log-head,
  .filter-card__head,
  .table-card__head,
  .filter-card__actions {
    align-items: stretch;
    flex-direction: column;
  }

  .summary-grid {
    grid-template-columns: 1fr;
  }

  .keyword-input,
  .filter-input,
  .filter-select,
  .method-select,
  .status-select,
  .request-input,
  .path-input {
    width: 100%;
  }

  .filter-form :deep(.t-form__item) {
    width: 100%;
  }
}
</style>
