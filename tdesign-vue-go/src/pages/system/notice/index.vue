<template>
  <div class="notice-page system-management-page">
    <console-page-header>
      <template #title>
        <h2>通知公告</h2>
        <t-tag :theme="closedCount > 0 ? 'warning' : 'success'" variant="light">
          {{ closedCount > 0 ? '存在关闭公告' : '公告状态正常' }}
        </t-tag>
      </template>
      <template #meta>
        <span>站内通知</span>
        <span>系统公告</span>
        <span>状态发布</span>
        <span>共 {{ pagination.total }} 条公告</span>
        <span v-if="lastUpdatedAt">更新于 {{ lastUpdatedAt }}</span>
      </template>
      <template #actions>
        <t-tag theme="primary" variant="light">当前页 {{ tableData.length }} 条</t-tag>
        <t-button theme="primary" @click="handleAdd">
          <template #icon><t-icon name="add" /></template>
          新增公告
        </t-button>
        <t-button variant="outline" :loading="loading" @click="handleRefresh">
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
          <h3>筛选查询</h3>
          <p>
            按标题关键词、公告类型和发布状态筛选通知公告
            <template v-if="activeFilterCount"> · 已应用 {{ activeFilterCount }} 个条件</template>
          </p>
        </div>
        <t-space size="small" break-line>
          <t-tag theme="primary" variant="light">通知 {{ noticeCount }}</t-tag>
          <t-tag theme="warning" variant="light">公告 {{ announcementCount }}</t-tag>
        </t-space>
      </div>
      <t-form :data="searchParams" class="filter-form" layout="inline" @submit="handleSearch">
        <t-form-item label="关键词" name="keyword">
          <t-input
            v-model="searchParams.keyword"
            clearable
            class="keyword-input"
            placeholder="公告标题 / 内容关键词"
            @enter="handleSearch"
          >
            <template #prefix-icon><t-icon name="search" /></template>
          </t-input>
        </t-form-item>
        <t-form-item label="类型" name="type">
          <t-select v-model="searchParams.type" clearable placeholder="全部类型" class="filter-select">
            <t-option :value="1" label="通知" />
            <t-option :value="2" label="公告" />
          </t-select>
        </t-form-item>
        <t-form-item label="状态" name="status">
          <t-select v-model="searchParams.status" clearable placeholder="全部状态" class="filter-select">
            <t-option :value="1" label="正常" />
            <t-option :value="0" label="关闭" />
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
        <t-button theme="primary" @click="handleAdd">
          <template #icon><t-icon name="add" /></template>
          新增公告
        </t-button>
      </div>
    </t-card>

    <t-card :bordered="false" class="table-card">
      <div class="table-card__head">
        <div>
          <h3>公告列表</h3>
          <p>
            公告标题、类型、状态、创建人和发布时间
            <template v-if="pagination.total"> · 共 {{ pagination.total }} 条</template>
          </p>
        </div>
        <t-space size="small" break-line>
          <t-tag :theme="activeCount > 0 ? 'success' : 'default'" variant="light">正常 {{ activeCount }}</t-tag>
          <t-tag :theme="closedCount > 0 ? 'warning' : 'default'" variant="light">关闭 {{ closedCount }}</t-tag>
        </t-space>
      </div>

      <t-table
        row-key="id"
        hover
        class="notice-table"
        table-layout="fixed"
        :data="tableData"
        :columns="columns"
        :loading="loading"
        :pagination="pagination"
        @page-change="handlePageChange"
        @page-size-change="handlePageSizeChange"
      >
        <template #empty>
          <t-empty :description="loading ? '正在加载公告数据' : '当前筛选条件下暂无公告'" />
        </template>
        <template #notice="{ row }">
          <div class="notice-cell">
            <span class="notice-avatar" :class="{ 'notice-avatar--announcement': row.type === 2 }">
              <t-icon :name="row.type === 2 ? 'notification-circle' : 'notification'" />
            </span>
            <div class="notice-cell__main">
              <strong :title="row.title">{{ row.title || '未命名公告' }}</strong>
              <span class="description-text" :title="row.content">{{ contentPreview(row.content) }}</span>
            </div>
          </div>
        </template>
        <template #type="{ row }">
          <t-tag :theme="row.type === 1 ? 'primary' : 'warning'" variant="light">
            {{ row.type === 1 ? '通知' : '公告' }}
          </t-tag>
        </template>
        <template #status="{ row }">
          <div class="status-cell">
            <t-switch
              :value="row.status === 1"
              :loading="row.statusLoading"
              @change="(val: any) => handleStatusChange(row, Boolean(val))"
            />
            <span>{{ row.status === 1 ? '正常' : '关闭' }}</span>
          </div>
        </template>
        <template #creator="{ row }">
          <div class="creator-cell">
            <strong>{{ row.creator || '系统' }}</strong>
            <span>ID {{ row.creator_id || '-' }}</span>
          </div>
        </template>
        <template #created_at="{ row }">
          <div class="date-cell">
            <strong>{{ formatDateTime(row.created_at) }}</strong>
            <span>更新 {{ formatDateTime(row.updated_at) }}</span>
          </div>
        </template>
        <template #operation="{ row }">
          <div class="operation-actions">
            <t-link theme="primary" hover="color" @click="handleView(row)">详情</t-link>
            <t-link theme="primary" hover="color" @click="handleEdit(row)">编辑</t-link>
            <t-popconfirm content="确定删除该公告吗？" @confirm="handleDelete(row)">
              <t-link theme="danger" hover="color">删除</t-link>
            </t-popconfirm>
          </div>
        </template>
      </t-table>
    </t-card>

    <t-dialog
      v-model:visible="dialogVisible"
      :header="dialogTitle"
      width="720px"
      :confirm-btn="{ content: '提交', loading: submitLoading }"
      @confirm="handleSubmit"
    >
      <t-form ref="formRef" :data="formData" :rules="formRules" label-width="80px" class="notice-form">
        <div class="form-grid">
          <t-form-item class="form-grid__full" label="标题" name="title">
            <t-input v-model="formData.title" placeholder="请输入公告标题" />
          </t-form-item>
          <t-form-item label="类型" name="type">
            <t-radio-group v-model="formData.type" variant="default-filled">
              <t-radio-button :value="1">通知</t-radio-button>
              <t-radio-button :value="2">公告</t-radio-button>
            </t-radio-group>
          </t-form-item>
          <t-form-item label="状态" name="status">
            <t-radio-group v-model="formData.status" variant="default-filled">
              <t-radio-button :value="1">正常</t-radio-button>
              <t-radio-button :value="0">关闭</t-radio-button>
            </t-radio-group>
          </t-form-item>
          <t-form-item class="form-grid__full" label="内容" name="content">
            <t-textarea v-model="formData.content" placeholder="请输入公告内容" :autosize="{ minRows: 5, maxRows: 10 }" />
          </t-form-item>
        </div>
      </t-form>
    </t-dialog>

    <t-drawer v-model:visible="viewVisible" :header="detailTitle" size="620px" :footer="false">
      <div v-if="currentNotice" class="detail-panel">
        <div class="detail-hero" :class="{ 'detail-hero--closed': currentNotice.status !== 1 }">
          <span class="detail-hero__icon">
            <t-icon :name="currentNotice.type === 2 ? 'notification-circle' : 'notification'" />
          </span>
          <div>
            <strong>{{ currentNotice.title || '未命名公告' }}</strong>
            <span>{{ currentNotice.status === 1 ? '公告发布中' : '公告已关闭' }}</span>
          </div>
        </div>

        <t-descriptions bordered :column="1" class="detail-desc">
          <t-descriptions-item label="公告 ID">{{ currentNotice.id }}</t-descriptions-item>
          <t-descriptions-item label="标题">{{ currentNotice.title }}</t-descriptions-item>
          <t-descriptions-item label="类型">
            <t-tag :theme="currentNotice.type === 1 ? 'primary' : 'warning'" variant="light">
              {{ currentNotice.type === 1 ? '通知' : '公告' }}
            </t-tag>
          </t-descriptions-item>
          <t-descriptions-item label="状态">
            <t-tag :theme="currentNotice.status === 1 ? 'success' : 'default'" variant="light">
              {{ currentNotice.status === 1 ? '正常' : '关闭' }}
            </t-tag>
          </t-descriptions-item>
          <t-descriptions-item label="创建人">{{ currentNotice.creator || '系统' }}</t-descriptions-item>
          <t-descriptions-item label="创建时间">{{ formatDateTime(currentNotice.created_at) }}</t-descriptions-item>
          <t-descriptions-item label="更新时间">{{ formatDateTime(currentNotice.updated_at) }}</t-descriptions-item>
        </t-descriptions>

        <section class="detail-section">
          <div class="detail-section__head">
            <span>公告内容</span>
            <t-tag theme="primary" variant="light">{{ contentLengthText }}</t-tag>
          </div>
          <p>{{ currentNotice.content || '暂无内容' }}</p>
        </section>
      </div>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { MessagePlugin } from 'tdesign-vue-next';
import { computed, onMounted, ref } from 'vue';

import {
  createNotice,
  deleteNotice,
  getNoticeList,
  updateNotice,
  updateNoticeStatus,
  type NoticeItem,
} from '@/api/system/notice';
import { formatDateTime } from '@/utils/date';
import ConsolePageHeader from '@/components/common/ConsolePageHeader.vue';

type SummaryTone = 'blue' | 'green' | 'cyan' | 'orange';

defineOptions({
  name: 'SystemNotice',
});

const loading = ref(false);
const submitLoading = ref(false);
const tableData = ref<(NoticeItem & { statusLoading?: boolean })[]>([]);
const dialogVisible = ref(false);
const viewVisible = ref(false);
const isEdit = ref(false);
const currentNotice = ref<NoticeItem | null>(null);
const formRef = ref();
const lastUpdatedAt = ref('');

const searchParams = ref({
  keyword: '',
  type: undefined as number | undefined,
  status: undefined as number | undefined,
});

const pagination = ref({
  current: 1,
  pageSize: 10,
  total: 0,
});

const formData = ref({
  title: '',
  content: '',
  type: 1,
  status: 1,
});

const formRules: any = {
  title: [{ required: true, message: '请输入公告标题' }],
  content: [{ required: true, message: '请输入公告内容' }],
};

const columns: any[] = [
  { colKey: 'notice', title: '公告', minWidth: 300, fixed: 'left' as const },
  { colKey: 'type', title: '类型', width: 100 },
  { colKey: 'status', title: '状态', width: 130 },
  { colKey: 'creator', title: '创建人', width: 140 },
  { colKey: 'created_at', title: '创建 / 更新', width: 220 },
  { colKey: 'operation', title: '操作', width: 150, fixed: 'right' as const },
];

const dialogTitle = computed(() => (isEdit.value ? '编辑公告' : '新增公告'));
const detailTitle = computed(() => (currentNotice.value ? `${currentNotice.value.title || '公告'} · 详情` : '公告详情'));
const activeCount = computed(() => tableData.value.filter((item) => item.status === 1).length);
const closedCount = computed(() => tableData.value.filter((item) => item.status !== 1).length);
const noticeCount = computed(() => tableData.value.filter((item) => item.type === 1).length);
const announcementCount = computed(() => tableData.value.filter((item) => item.type === 2).length);
const contentLengthText = computed(() => `${currentNotice.value?.content?.length || 0} 字`);
const activeFilterCount = computed(() => {
  let count = 0;
  if (searchParams.value.keyword.trim()) count += 1;
  if (searchParams.value.type !== undefined) count += 1;
  if (searchParams.value.status !== undefined) count += 1;
  return count;
});

const summaryItems = computed<Array<{ label: string; value: string | number; hint: string; icon: string; tone: SummaryTone }>>(() => [
  {
    label: '公告总数',
    value: pagination.value.total || tableData.value.length,
    hint: `当前页 ${tableData.value.length} 条公告`,
    icon: 'notification',
    tone: 'blue',
  },
  {
    label: '正常公告',
    value: activeCount.value,
    hint: `关闭 ${closedCount.value} 条`,
    icon: 'check-circle',
    tone: 'green',
  },
  {
    label: '通知',
    value: noticeCount.value,
    hint: '面向站内消息提醒',
    icon: 'notification-add',
    tone: 'cyan',
  },
  {
    label: '公告',
    value: announcementCount.value,
    hint: '面向系统级说明发布',
    icon: 'notification-circle',
    tone: 'orange',
  },
]);

const updateTime = () => {
  lastUpdatedAt.value = new Date().toLocaleTimeString('zh-CN', { hour12: false });
};

const contentPreview = (content?: string) => {
  const text = (content || '').trim();
  if (!text) return '暂无公告内容';
  return text.length > 54 ? `${text.slice(0, 54)}...` : text;
};

const loadData = async () => {
  loading.value = true;
  try {
    const res = await getNoticeList({
      page: pagination.value.current,
      page_size: pagination.value.pageSize,
      ...searchParams.value,
      keyword: searchParams.value.keyword.trim() || undefined,
    });
    tableData.value = res.list || [];
    pagination.value.total = res.total || 0;
    updateTime();
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载公告列表失败');
  } finally {
    loading.value = false;
  }
};

const handleSearch = () => {
  pagination.value.current = 1;
  loadData();
};

const handleReset = () => {
  searchParams.value = {
    keyword: '',
    type: undefined,
    status: undefined,
  };
  handleSearch();
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

const handleAdd = () => {
  isEdit.value = false;
  currentNotice.value = null;
  formData.value = {
    title: '',
    content: '',
    type: 1,
    status: 1,
  };
  dialogVisible.value = true;
};

const handleEdit = (row: NoticeItem) => {
  isEdit.value = true;
  currentNotice.value = row;
  formData.value = {
    title: row.title,
    content: row.content,
    type: row.type,
    status: row.status,
  };
  dialogVisible.value = true;
};

const handleView = (row: NoticeItem) => {
  currentNotice.value = row;
  viewVisible.value = true;
};

const handleSubmit = async () => {
  const valid = await formRef.value?.validate();
  if (valid !== true) return;

  submitLoading.value = true;
  try {
    if (isEdit.value && currentNotice.value) {
      await updateNotice(currentNotice.value.id, formData.value);
      MessagePlugin.success('更新成功');
    } else {
      await createNotice(formData.value);
      MessagePlugin.success('创建成功');
    }
    dialogVisible.value = false;
    loadData();
  } catch (error: any) {
    MessagePlugin.error(error.message || '操作失败');
  } finally {
    submitLoading.value = false;
  }
};

const handleDelete = async (row: NoticeItem) => {
  try {
    await deleteNotice(row.id);
    MessagePlugin.success('删除成功');
    loadData();
  } catch (error: any) {
    MessagePlugin.error(error.message || '删除失败');
  }
};

const handleStatusChange = async (row: NoticeItem & { statusLoading?: boolean }, val: boolean) => {
  row.statusLoading = true;
  try {
    await updateNoticeStatus(row.id, val ? 1 : 0);
    row.status = val ? 1 : 0;
    MessagePlugin.success('状态更新成功');
  } catch (error: any) {
    MessagePlugin.error(error.message || '状态更新失败');
  } finally {
    row.statusLoading = false;
  }
};

const handleRefresh = () => {
  loadData();
};

onMounted(() => {
  loadData();
});
</script>

<style lang="less" scoped>
.notice-page {
  --notice-bg: #f5f7fb;
  --notice-card-shadow: 0 12px 28px rgb(15 23 42 / 6%);
  --notice-font: 'Inter', 'HarmonyOS Sans SC', 'MiSans', 'PingFang SC', 'Microsoft YaHei UI', 'Microsoft YaHei', 'Arial', sans-serif;
  --notice-number-font: 'DIN Alternate', 'Bahnschrift', 'Inter', 'HarmonyOS Sans SC', 'Microsoft YaHei UI', sans-serif;

  display: flex;
  min-height: calc(100vh - 120px);
  min-width: 0;
  flex-direction: column;
  gap: 14px;
  margin: calc(-1 * var(--td-comp-paddingTB-xl)) calc(-1 * var(--td-comp-paddingLR-xl));
  padding: 14px 18px 18px;
  background: var(--notice-bg);
  color: var(--td-text-color-primary);
  font-family: var(--notice-font);
  font-feature-settings: 'tnum';
  font-variant-numeric: tabular-nums;
  letter-spacing: 0;
  text-rendering: optimizelegibility;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}

.notice-page :deep(.t-card),
.notice-page :deep(.t-table),
.notice-page :deep(.t-form),
.notice-page :deep(.t-button),
.notice-page :deep(.t-tag),
.notice-page :deep(.t-input),
.notice-page :deep(.t-select),
.notice-page :deep(.t-dialog),
.notice-page :deep(.t-drawer),
.notice-page :deep(.t-empty) {
  font-family: var(--notice-font);
}

.notice-head {
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

.notice-head__main {
  min-width: 0;
}

.notice-head__title {
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

.notice-head__meta {
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

.notice-head__actions {
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
    font-family: var(--notice-number-font);
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
  box-shadow: var(--notice-card-shadow);
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
  width: 300px;
}

.filter-select {
  width: 160px;
}

.filter-card__actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 12px 20px 18px;
}

.notice-table {
  width: 100%;
}

.notice-table :deep(.t-table__header th) {
  background: #f8fafc;
  color: #475569;
  font-size: 13px;
  font-weight: 700;
}

.notice-table :deep(.t-table__body tr:hover td) {
  background: #f8fbff;
}

.notice-table :deep(.t-table__body td) {
  padding-top: 14px;
  padding-bottom: 14px;
  border-bottom-color: #eef2f7;
  color: #1f2937;
  vertical-align: top;
}

.notice-cell {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
}

.notice-avatar {
  display: inline-flex;
  width: 34px;
  height: 34px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border-radius: 11px;
  background: linear-gradient(135deg, #dbeafe, #cffafe);
  color: #2563eb;
  font-size: 18px;
}

.notice-avatar--announcement {
  background: linear-gradient(135deg, #ffedd5, #fed7aa);
  color: #ea580c;
}

.notice-cell__main,
.creator-cell,
.date-cell {
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

.description-text {
  display: block;
  overflow: hidden;
  max-width: 360px;
  color: #64748b;
  font-size: 12px;
  line-height: 18px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.status-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #64748b;
  font-size: 12px;
}

.operation-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.notice-form {
  padding-top: 4px;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 14px;
}

.form-grid__full {
  grid-column: 1 / -1;
}

.detail-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
  font-family: var(--notice-font);
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

.detail-hero--closed {
  border-color: #e2e8f0;
  background: linear-gradient(135deg, #f8fafc, #e2e8f0);

  strong {
    color: #334155;
  }

  span {
    color: #64748b;
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
  width: 96px;
  color: #64748b;
  font-weight: 600;
}

.detail-section {
  overflow: hidden;
  padding: 14px;
  border: 1px solid #e8edf5;
  border-radius: 12px;
  background: #fff;

  p {
    margin: 0;
    color: #334155;
    font-size: 14px;
    line-height: 24px;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
}

.detail-section__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
  color: #111827;
  font-size: 14px;
  font-weight: 800;
}

@media (width <= 1200px) {
  .summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (width <= 768px) {
  .notice-page {
    margin: calc(-1 * var(--td-comp-paddingTB-l)) calc(-1 * var(--td-comp-paddingLR-l));
    padding: 12px;
  }

  .notice-head,
  .filter-card__head,
  .table-card__head,
  .filter-card__actions {
    align-items: stretch;
    flex-direction: column;
  }

  .summary-grid,
  .form-grid {
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
