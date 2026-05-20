<template>
  <div class="online-user-page system-management-page">
    <console-page-header>
      <template #title>
        <h2>在线用户</h2>
        <t-tag :theme="onlineTotal > 0 ? 'success' : 'default'" variant="light">
          {{ onlineTotal > 0 ? '实时在线' : '暂无会话' }}
        </t-tag>
      </template>
      <template #meta>
        <span>实时会话</span>
        <span>设备来源</span>
        <span>登录地点</span>
        <span>30 秒自动刷新</span>
        <span v-if="lastUpdatedAt">更新于 {{ lastUpdatedAt }}</span>
      </template>
      <template #actions>
        <t-tag theme="primary" variant="light">在线 {{ onlineTotal }}</t-tag>
        <t-button variant="outline" :loading="loading || countLoading" @click="handleRefresh">
          <template #icon><t-icon name="refresh" /></template>
          刷新
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
          <h3>会话筛选</h3>
          <p>
            按用户、IP、地点、浏览器和系统快速定位在线会话
            <template v-if="activeFilterCount"> · 已应用 {{ activeFilterCount }} 个条件</template>
          </p>
        </div>
        <t-space size="small" break-line>
          <t-tag theme="primary" variant="light">当前显示 {{ filteredData.length }} 条</t-tag>
          <t-tag :theme="expiringCount > 0 ? 'warning' : 'success'" variant="light">临近过期 {{ expiringCount }}</t-tag>
        </t-space>
      </div>
      <t-form :data="searchForm" class="filter-form" layout="inline" @submit="handleSearch">
        <t-form-item label="关键字" name="keyword">
          <t-input
            v-model="searchForm.keyword"
            clearable
            class="keyword-input"
            placeholder="用户 / 昵称 / IP / 地点"
            @enter="handleSearch"
          />
        </t-form-item>
        <t-form-item label="浏览器" name="browser">
          <t-select v-model="searchForm.browser" clearable placeholder="全部浏览器" class="filter-select">
            <t-option v-for="browser in browserOptions" :key="browser" :label="browser" :value="browser" />
          </t-select>
        </t-form-item>
        <t-form-item label="操作系统" name="os">
          <t-select v-model="searchForm.os" clearable placeholder="全部系统" class="filter-select">
            <t-option v-for="os in osOptions" :key="os" :label="os" :value="os" />
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
          <t-tag v-if="topBrowser" theme="primary" variant="light">主浏览器 {{ topBrowser }}</t-tag>
          <t-tag v-if="topOs" theme="default" variant="light">主系统 {{ topOs }}</t-tag>
        </t-space>
      </div>
    </t-card>

    <t-card :bordered="false" class="table-card">
      <div class="table-card__head">
        <div>
          <h3>在线会话</h3>
          <p>当前仍有效的访问令牌、登录时间、来源设备和强制下线操作</p>
        </div>
        <t-space size="small">
          <t-tag :theme="uniqueUserCount > 0 ? 'success' : 'default'" variant="light">
            用户 {{ uniqueUserCount }}
          </t-tag>
          <t-tag theme="primary" variant="light">地区 {{ locationCount }}</t-tag>
        </t-space>
      </div>

      <t-table
        row-key="token_id"
        hover
        class="online-table"
        table-layout="fixed"
        :data="filteredData"
        :columns="columns"
        :loading="loading"
        :pagination="undefined"
      >
        <template #empty>
          <t-empty :description="loading ? '正在加载在线会话' : '当前筛选条件下暂无在线会话'" />
        </template>
        <template #user="{ row }">
          <div class="user-cell">
            <span class="user-avatar">{{ userInitial(row) }}</span>
            <div class="user-cell__main">
              <strong>{{ row.nickname || row.username || '未知用户' }}</strong>
              <span>{{ row.username || '-' }} · ID {{ row.user_id || '-' }}</span>
            </div>
          </div>
        </template>
        <template #source="{ row }">
          <div class="source-cell">
            <strong>
              <t-icon name="location" />
              {{ row.location || '未知地点' }}
            </strong>
            <span class="mono-text">{{ row.ip || '-' }}</span>
          </div>
        </template>
        <template #client="{ row }">
          <div class="client-cell">
            <strong>{{ row.browser || '未知浏览器' }}</strong>
            <span>{{ row.os || '未知系统' }}</span>
          </div>
        </template>
        <template #login_time="{ row }">
          <div class="time-cell">
            <strong>{{ formatDateTime(row.login_time) }}</strong>
            <span>在线 {{ sessionDuration(row.login_time) }}</span>
          </div>
        </template>
        <template #expires="{ row }">
          <t-tag :theme="expiresTheme(row.access_token_expires_at)" variant="light">
            {{ expiresLabel(row.access_token_expires_at) }}
          </t-tag>
        </template>
        <template #token_id="{ row }">
          <span class="mono-text token-text" :title="row.token_id">{{ row.token_id || '-' }}</span>
        </template>
        <template #operation="{ row }">
          <div class="operation-actions">
            <t-link theme="primary" hover="color" @click="openDetail(row)">详情</t-link>
            <t-popconfirm content="确定强制该用户下线吗？" @confirm="handleForceLogout(row)">
              <t-link theme="danger" hover="color">下线</t-link>
            </t-popconfirm>
          </div>
        </template>
      </t-table>
    </t-card>

    <t-drawer v-model:visible="detailVisible" :header="detailTitle" size="620px" :footer="false">
      <div v-if="currentUser" class="detail-panel">
        <div class="detail-hero">
          <span class="detail-hero__icon">
            <t-icon name="user-circle" />
          </span>
          <div>
            <strong>{{ currentUser.nickname || currentUser.username || '未知用户' }}</strong>
            <span>{{ currentUser.username || '-' }} · 在线 {{ sessionDuration(currentUser.login_time) }}</span>
          </div>
        </div>

        <t-descriptions bordered :column="1" class="detail-desc">
          <t-descriptions-item label="用户 ID">{{ currentUser.user_id || '-' }}</t-descriptions-item>
          <t-descriptions-item label="用户名">{{ currentUser.username || '-' }}</t-descriptions-item>
          <t-descriptions-item label="昵称">{{ currentUser.nickname || '-' }}</t-descriptions-item>
          <t-descriptions-item label="IP 地址">
            <span class="mono-text">{{ currentUser.ip || '-' }}</span>
          </t-descriptions-item>
          <t-descriptions-item label="登录地点">{{ currentUser.location || '未知地点' }}</t-descriptions-item>
          <t-descriptions-item label="浏览器">{{ currentUser.browser || '未知浏览器' }}</t-descriptions-item>
          <t-descriptions-item label="操作系统">{{ currentUser.os || '未知系统' }}</t-descriptions-item>
          <t-descriptions-item label="登录时间">{{ formatDateTime(currentUser.login_time) }}</t-descriptions-item>
          <t-descriptions-item label="令牌过期">{{ expiresDetail(currentUser.access_token_expires_at) }}</t-descriptions-item>
        </t-descriptions>

        <div class="token-box">
          <span>Token ID</span>
          <p>{{ currentUser.token_id || '-' }}</p>
        </div>

        <t-popconfirm content="确定强制该用户下线吗？" @confirm="handleForceLogout(currentUser)">
          <t-button block theme="danger" variant="outline">强制下线</t-button>
        </t-popconfirm>
      </div>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { MessagePlugin } from 'tdesign-vue-next';
import { computed, onMounted, onUnmounted, ref } from 'vue';

import { forceLogout, getOnlineUserCount, getOnlineUsers, type OnlineUserItem } from '@/api/system/onlineUser';
import { formatDateTime } from '@/utils/date';
import ConsolePageHeader from '@/components/common/ConsolePageHeader.vue';

type TagTheme = 'default' | 'success' | 'primary' | 'warning' | 'danger';
type SummaryTone = 'blue' | 'green' | 'cyan' | 'orange';

defineOptions({
  name: 'SystemOnlineUser',
});

const loading = ref(false);
const countLoading = ref(false);
const tableData = ref<OnlineUserItem[]>([]);
const total = ref(0);
const countTotal = ref(0);
const detailVisible = ref(false);
const currentUser = ref<OnlineUserItem | null>(null);
const lastUpdatedAt = ref('');

const searchForm = ref({
  keyword: '',
  browser: '',
  os: '',
});

let refreshTimer: ReturnType<typeof setInterval> | null = null;

const columns: any[] = [
  { colKey: 'user', title: '用户', width: 240, fixed: 'left' as const },
  { colKey: 'source', title: '来源', minWidth: 210 },
  { colKey: 'client', title: '客户端', minWidth: 190 },
  { colKey: 'login_time', title: '登录时间', width: 210 },
  { colKey: 'expires', title: '令牌状态', width: 120 },
  { colKey: 'token_id', title: 'Token ID', minWidth: 240 },
  { colKey: 'operation', title: '操作', width: 116, fixed: 'right' as const },
];

const filteredData = computed(() => {
  const keyword = searchForm.value.keyword.trim().toLowerCase();
  return tableData.value.filter((item) => {
    const keywordMatched = keyword
      ? [item.username, item.nickname, item.ip, item.location, item.token_id]
          .filter(Boolean)
          .some((value) => String(value).toLowerCase().includes(keyword))
      : true;
    const browserMatched = searchForm.value.browser ? item.browser === searchForm.value.browser : true;
    const osMatched = searchForm.value.os ? item.os === searchForm.value.os : true;
    return keywordMatched && browserMatched && osMatched;
  });
});

const onlineTotal = computed(() => countTotal.value || total.value || tableData.value.length);
const uniqueUserCount = computed(() => new Set(tableData.value.map((item) => item.user_id || item.username).filter(Boolean)).size);
const locationCount = computed(() => new Set(tableData.value.map((item) => item.location).filter(Boolean)).size);
const expiringCount = computed(() => tableData.value.filter((item) => isExpiringSoon(item.access_token_expires_at)).length);
const browserOptions = computed(() => uniqueSorted(tableData.value.map((item) => item.browser).filter(Boolean)));
const osOptions = computed(() => uniqueSorted(tableData.value.map((item) => item.os).filter(Boolean)));
const topBrowser = computed(() => getTopValue(tableData.value.map((item) => item.browser).filter(Boolean)));
const topOs = computed(() => getTopValue(tableData.value.map((item) => item.os).filter(Boolean)));
const activeFilterCount = computed(() => {
  let count = 0;
  if (searchForm.value.keyword.trim()) count += 1;
  if (searchForm.value.browser) count += 1;
  if (searchForm.value.os) count += 1;
  return count;
});

const summaryItems = computed<Array<{ label: string; value: string | number; hint: string; icon: string; tone: SummaryTone }>>(() => [
  {
    label: '在线会话',
    value: onlineTotal.value,
    hint: `当前显示 ${filteredData.value.length} 条`,
    icon: 'internet',
    tone: 'blue',
  },
  {
    label: '在线用户',
    value: uniqueUserCount.value,
    hint: `${locationCount.value} 个登录地点`,
    icon: 'user-circle',
    tone: 'green',
  },
  {
    label: '客户端类型',
    value: browserOptions.value.length,
    hint: topBrowser.value ? `主浏览器 ${topBrowser.value}` : '暂无浏览器信息',
    icon: 'desktop',
    tone: 'cyan',
  },
  {
    label: '令牌临期',
    value: expiringCount.value,
    hint: expiringCount.value > 0 ? '建议核查长连接状态' : '令牌状态正常',
    icon: 'time',
    tone: expiringCount.value > 0 ? 'orange' : 'cyan',
  },
]);

const detailTitle = computed(() => {
  if (!currentUser.value) return '会话详情';
  return `${currentUser.value.nickname || currentUser.value.username || '在线会话'} · ${currentUser.value.ip || '-'}`;
});

const updateTime = () => {
  lastUpdatedAt.value = new Date().toLocaleTimeString('zh-CN', { hour12: false });
};

const loadCount = async () => {
  countLoading.value = true;
  try {
    const res = await getOnlineUserCount();
    countTotal.value = res.count || 0;
  } catch (error) {
    console.error('加载在线用户数量失败:', error);
  } finally {
    countLoading.value = false;
  }
};

const loadData = async () => {
  loading.value = true;
  try {
    const res = await getOnlineUsers();
    tableData.value = res.list || [];
    total.value = res.total || 0;
    updateTime();
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载在线用户失败');
  } finally {
    loading.value = false;
  }
};

const handleSearch = () => {
  // 当前接口不提供筛选参数，筛选在前端基于真实会话数据完成。
};

const handleReset = () => {
  searchForm.value = {
    keyword: '',
    browser: '',
    os: '',
  };
};

const handleRefresh = () => {
  loadData();
  loadCount();
};

const openDetail = (row: OnlineUserItem) => {
  currentUser.value = row;
  detailVisible.value = true;
};

const handleForceLogout = async (row: OnlineUserItem) => {
  try {
    await forceLogout(row.token_id);
    MessagePlugin.success('用户已强制下线');
    if (currentUser.value?.token_id === row.token_id) {
      detailVisible.value = false;
      currentUser.value = null;
    }
    handleRefresh();
  } catch (error: any) {
    MessagePlugin.error(error.message || '操作失败');
  }
};

const userInitial = (row: OnlineUserItem) => {
  const source = row.nickname || row.username || 'U';
  return source.slice(0, 1).toUpperCase();
};

const uniqueSorted = (values: string[]) => Array.from(new Set(values)).sort((a, b) => a.localeCompare(b));

const getTopValue = (values: string[]) => {
  const countMap = values.reduce<Record<string, number>>((acc, value) => {
    acc[value] = (acc[value] || 0) + 1;
    return acc;
  }, {});
  return Object.entries(countMap).sort((a, b) => b[1] - a[1])[0]?.[0] || '';
};

const sessionDuration = (loginTime?: string) => {
  if (!loginTime) return '-';
  const diff = Date.now() - new Date(loginTime).getTime();
  if (!Number.isFinite(diff) || diff < 0) return '-';
  const minutes = Math.floor(diff / 60000);
  if (minutes < 1) return '刚刚';
  if (minutes < 60) return `${minutes} 分钟`;
  const hours = Math.floor(minutes / 60);
  const remainMinutes = minutes % 60;
  if (hours < 24) return remainMinutes ? `${hours} 小时 ${remainMinutes} 分钟` : `${hours} 小时`;
  const days = Math.floor(hours / 24);
  return `${days} 天 ${hours % 24} 小时`;
};

const expiresDate = (expiresAt?: string) => {
  if (!expiresAt) return null;
  const date = new Date(expiresAt);
  return Number.isNaN(date.getTime()) ? null : date;
};

const isExpiringSoon = (expiresAt?: string) => {
  const date = expiresDate(expiresAt);
  if (!date) return false;
  const diff = date.getTime() - Date.now();
  return diff > 0 && diff <= 60 * 60 * 1000;
};

const expiresTheme = (expiresAt?: string): TagTheme => {
  const date = expiresDate(expiresAt);
  if (!date) return 'default';
  const diff = date.getTime() - Date.now();
  if (diff <= 0) return 'danger';
  if (diff <= 60 * 60 * 1000) return 'warning';
  return 'success';
};

const expiresLabel = (expiresAt?: string) => {
  const date = expiresDate(expiresAt);
  if (!date) return '未记录';
  const diff = date.getTime() - Date.now();
  if (diff <= 0) return '已过期';
  if (diff <= 60 * 60 * 1000) return '临近过期';
  return '有效';
};

const expiresDetail = (expiresAt?: string) => {
  const date = expiresDate(expiresAt);
  if (!date) return '未记录';
  return `${formatDateTime(expiresAt)} · ${expiresLabel(expiresAt)}`;
};

onMounted(() => {
  handleRefresh();
  refreshTimer = setInterval(handleRefresh, 30000);
});

onUnmounted(() => {
  if (refreshTimer) {
    clearInterval(refreshTimer);
  }
});
</script>

<style lang="less" scoped>
.online-user-page {
  --online-bg: #f5f7fb;
  --online-card-shadow: 0 12px 28px rgb(15 23 42 / 6%);
  --online-font: 'Inter', 'HarmonyOS Sans SC', 'MiSans', 'PingFang SC', 'Microsoft YaHei UI', 'Microsoft YaHei', 'Arial', sans-serif;
  --online-number-font: 'DIN Alternate', 'Bahnschrift', 'Inter', 'HarmonyOS Sans SC', 'Microsoft YaHei UI', sans-serif;

  display: flex;
  min-height: calc(100vh - 120px);
  min-width: 0;
  flex-direction: column;
  gap: 14px;
  margin: calc(-1 * var(--td-comp-paddingTB-xl)) calc(-1 * var(--td-comp-paddingLR-xl));
  padding: 14px 18px 18px;
  background: var(--online-bg);
  color: var(--td-text-color-primary);
  font-family: var(--online-font);
  font-feature-settings: 'tnum';
  font-variant-numeric: tabular-nums;
  letter-spacing: 0;
  text-rendering: optimizelegibility;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}

.online-user-page :deep(.t-card),
.online-user-page :deep(.t-table),
.online-user-page :deep(.t-form),
.online-user-page :deep(.t-button),
.online-user-page :deep(.t-tag),
.online-user-page :deep(.t-input),
.online-user-page :deep(.t-select),
.online-user-page :deep(.t-drawer),
.online-user-page :deep(.t-empty) {
  font-family: var(--online-font);
}

.online-user-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--td-comp-margin-l);
  padding: 10px 12px;
  border: 1px solid #e8edf5;
  border-radius: 12px;
  background:
    radial-gradient(circle at 18% 0%, rgb(14 165 233 / 10%), transparent 28%),
    radial-gradient(circle at 92% 16%, rgb(34 197 94 / 12%), transparent 26%),
    #fff;
  box-shadow: 0 10px 24px rgb(15 23 42 / 5%);
}

.online-user-head__main {
  min-width: 0;
}

.online-user-head__title {
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

.online-user-head__meta {
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

.online-user-head__actions {
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
    font-family: var(--online-number-font);
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

.filter-card,
.table-card {
  overflow: hidden;
  border: 1px solid #e8edf5;
  border-radius: 12px;
  background: #fff;
  box-shadow: var(--online-card-shadow);
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

.keyword-input {
  width: 260px;
}

.filter-select {
  width: 180px;
}

.filter-card__actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 12px 20px 18px;
}

.online-table {
  width: 100%;
}

.online-table :deep(.t-table__header th) {
  background: #f8fafc;
  color: #475569;
  font-size: 13px;
  font-weight: 700;
}

.online-table :deep(.t-table__body tr:hover td) {
  background: #f8fbff;
}

.online-table :deep(.t-table__body td) {
  padding-top: 14px;
  padding-bottom: 14px;
  border-bottom-color: #eef2f7;
  color: #1f2937;
  vertical-align: top;
}

.user-cell {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
}

.user-avatar {
  display: inline-flex;
  width: 34px;
  height: 34px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: linear-gradient(135deg, #0284c7, #22c55e);
  color: #fff;
  font-size: 14px;
  font-weight: 800;
}

.user-cell__main,
.source-cell,
.client-cell,
.time-cell {
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

.source-cell strong {
  display: inline-flex;
  max-width: 100%;
  align-items: center;
  gap: 5px;

  :deep(.t-icon) {
    flex-shrink: 0;
    color: #0284c7;
  }
}

.mono-text {
  font-family: var(--online-number-font);
  font-variant-numeric: tabular-nums;
}

.token-text {
  display: inline-block;
  overflow: hidden;
  max-width: 100%;
  color: #334155;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.operation-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.detail-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
  font-family: var(--online-font);
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
  width: 100px;
  color: #64748b;
  font-weight: 600;
}

.token-box {
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
    font-family: var(--online-number-font);
    font-size: 12px;
    line-height: 20px;
    overflow-wrap: anywhere;
  }
}

@media (width <= 1200px) {
  .summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (width <= 768px) {
  .online-user-page {
    margin: calc(-1 * var(--td-comp-paddingTB-l)) calc(-1 * var(--td-comp-paddingLR-l));
    padding: 12px;
  }

  .online-user-head,
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
  .filter-select {
    width: 100%;
  }

  .filter-form :deep(.t-form__item) {
    width: 100%;
  }
}
</style>
