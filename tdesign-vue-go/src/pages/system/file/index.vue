<template>
  <div class="file-page system-management-page">
    <console-page-header>
      <template #title>
        <h2>文件管理</h2>
        <t-tag :theme="tableData.length > 0 ? 'success' : 'default'" variant="light">
          {{ tableData.length > 0 ? '文件索引正常' : '等待文件上传' }}
        </t-tag>
      </template>
      <template #meta>
        <span>上传文件</span>
        <span>对象存储</span>
        <span>预览下载</span>
        <span>共 {{ pagination.total }} 个文件</span>
        <span v-if="lastUpdatedAt">更新于 {{ lastUpdatedAt }}</span>
      </template>
      <template #actions>
        <t-tag theme="primary" variant="light">当前页 {{ tableData.length }} 条</t-tag>
        <t-button theme="primary" @click="handleUpload">
          <template #icon><t-icon name="upload" /></template>
          上传文件
        </t-button>
        <t-button variant="outline" :loading="loading || statsLoading" @click="handleRefresh">
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
            按文件名、扩展名和文件类型定位上传记录
            <template v-if="activeFilterCount"> · 已应用 {{ activeFilterCount }} 个条件</template>
          </p>
        </div>
        <t-space size="small" break-line>
          <t-tag theme="primary" variant="light">存储类型 {{ storageTypeCount }}</t-tag>
          <t-tag theme="success" variant="light">总容量 {{ totalSizeText }}</t-tag>
        </t-space>
      </div>
      <t-form :data="searchForm" class="filter-form" layout="inline" @submit="handleSearch">
        <t-form-item label="关键词" name="keyword">
          <t-input
            v-model="searchForm.keyword"
            clearable
            class="keyword-input"
            placeholder="文件名 / 扩展名 / MIME"
            @enter="handleSearch"
          >
            <template #prefix-icon><t-icon name="search" /></template>
          </t-input>
        </t-form-item>
        <t-form-item label="文件类型" name="file_type">
          <t-select v-model="searchForm.file_type" clearable placeholder="全部类型" class="filter-select">
            <t-option v-for="type in fileTypeOptions" :key="type" :label="type" :value="type" />
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
        <t-button theme="primary" @click="handleUpload">
          <template #icon><t-icon name="upload" /></template>
          上传文件
        </t-button>
      </div>
    </t-card>

    <t-card :bordered="false" class="table-card">
      <div class="table-card__head">
        <div>
          <h3>文件列表</h3>
          <p>
            文件名称、大小、类型、存储位置和上传时间
            <template v-if="pagination.total"> · 共 {{ pagination.total }} 条</template>
          </p>
        </div>
        <t-space size="small" break-line>
          <t-tag theme="primary" variant="light">当前页容量 {{ currentPageSizeText }}</t-tag>
          <t-tag theme="success" variant="light">类型 {{ fileTypeCount }}</t-tag>
        </t-space>
      </div>

      <t-table
        row-key="id"
        hover
        class="file-table"
        table-layout="fixed"
        :data="tableData"
        :columns="columns"
        :loading="loading"
        :pagination="pagination"
        @page-change="handlePageChange"
        @page-size-change="handlePageSizeChange"
      >
        <template #empty>
          <t-empty :description="loading ? '正在加载文件数据' : '当前筛选条件下暂无文件'" />
        </template>
        <template #file="{ row }">
          <div class="file-cell">
            <span class="file-avatar">
              <t-icon :name="fileIcon(row)" />
            </span>
            <div class="file-cell__main">
              <t-link theme="primary" hover="color" :title="row.file_name" @click="handlePreview(row)">
                {{ row.file_name || '未命名文件' }}
              </t-link>
              <span class="mono-text">{{ row.extension || fileExtension(row.file_name) || '-' }} · ID {{ row.id }}</span>
            </div>
          </div>
        </template>
        <template #file_size="{ row }">
          <div class="size-cell">
            <strong>{{ formatFileSize(row.file_size) }}</strong>
            <span>{{ row.file_size || 0 }} B</span>
          </div>
        </template>
        <template #file_type="{ row }">
          <t-tag theme="primary" variant="light">{{ row.file_type || 'unknown' }}</t-tag>
        </template>
        <template #mime_type="{ row }">
          <span class="description-text" :title="row.mime_type">{{ row.mime_type || '-' }}</span>
        </template>
        <template #storage_type="{ row }">
          <t-tag :theme="row.storage_type === 'local' ? 'default' : 'success'" variant="light">
            {{ row.storage_type || 'local' }}
          </t-tag>
        </template>
        <template #created_at="{ row }">
          <div class="date-cell">
            <strong>{{ formatDateTime(row.created_at) }}</strong>
            <span>更新 {{ formatDateTime(row.updated_at) }}</span>
          </div>
        </template>
        <template #operation="{ row }">
          <div class="operation-actions">
            <t-link theme="primary" hover="color" @click="handleDownload(row)">下载</t-link>
            <t-link theme="primary" hover="color" @click="handlePreview(row)">预览</t-link>
            <t-popconfirm content="确定删除该文件吗？" @confirm="handleDelete(row)">
              <t-link theme="danger" hover="color">删除</t-link>
            </t-popconfirm>
          </div>
        </template>
      </t-table>
    </t-card>

    <t-dialog v-model:visible="uploadVisible" header="上传文件" width="560px" :footer="false">
      <div class="upload-panel">
        <div class="upload-panel__hero">
          <span><t-icon name="cloud-upload" /></span>
          <div>
            <strong>文件上传</strong>
            <small>支持多文件上传，完成后自动刷新列表</small>
          </div>
        </div>
        <t-upload
          v-model="fileList"
          :action="uploadAction"
          :headers="uploadHeaders"
          :data="uploadData"
          multiple
          @success="handleUploadSuccess"
        />
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { MessagePlugin } from 'tdesign-vue-next';
import { computed, onMounted, ref } from 'vue';

import {
  deleteFile,
  downloadFile,
  getFileList,
  getFileStats,
  previewFile,
  type FileItem,
  type FileStats,
} from '@/api/system/file';
import { useUserStore } from '@/store';
import { formatDateTime } from '@/utils/date';
import ConsolePageHeader from '@/components/common/ConsolePageHeader.vue';

type SummaryTone = 'blue' | 'green' | 'cyan' | 'orange';

defineOptions({
  name: 'SystemFile',
});

const userStore = useUserStore();
const loading = ref(false);
const statsLoading = ref(false);
const tableData = ref<FileItem[]>([]);
const stats = ref<FileStats | null>(null);
const uploadVisible = ref(false);
const fileList = ref<any[]>([]);
const lastUpdatedAt = ref('');

const searchForm = ref({
  keyword: '',
  file_type: '',
});

const pagination = ref({
  current: 1,
  pageSize: 10,
  total: 0,
});

const columns: any[] = [
  { colKey: 'file', title: '文件', width: 280, fixed: 'left' as const },
  { colKey: 'file_size', title: '大小', width: 130 },
  { colKey: 'file_type', title: '类型', width: 120 },
  { colKey: 'mime_type', title: 'MIME 类型', minWidth: 220 },
  { colKey: 'storage_type', title: '存储', width: 120 },
  { colKey: 'created_at', title: '上传 / 更新', width: 220 },
  { colKey: 'operation', title: '操作', width: 150, fixed: 'right' as const },
];

const uploadAction = '/api/v1/files/upload';
const uploadHeaders = computed(() => ({
  Authorization: `Bearer ${userStore.token}`,
}));
const uploadData = ref({});

const activeFilterCount = computed(() => {
  let count = 0;
  if (searchForm.value.keyword.trim()) count += 1;
  if (searchForm.value.file_type) count += 1;
  return count;
});

const fileTypeOptions = computed(() => {
  const fromStats = stats.value?.by_type ? Object.keys(stats.value.by_type) : [];
  const fromTable = tableData.value.map((item) => item.file_type).filter(Boolean);
  return Array.from(new Set([...fromStats, ...fromTable])).sort();
});

const fileTypeCount = computed(() => fileTypeOptions.value.length);
const storageTypeCount = computed(() => new Set(tableData.value.map((item) => item.storage_type || 'local')).size);
const currentPageSize = computed(() => tableData.value.reduce((sum, item) => sum + (item.file_size || 0), 0));
const totalSize = computed(() => stats.value?.total_size ?? currentPageSize.value);
const currentPageSizeText = computed(() => formatFileSize(currentPageSize.value));
const totalSizeText = computed(() => formatFileSize(totalSize.value));

const summaryItems = computed<Array<{ label: string; value: string | number; hint: string; icon: string; tone: SummaryTone }>>(() => [
  {
    label: '文件总数',
    value: stats.value?.total_count ?? stats.value?.total ?? pagination.value.total ?? tableData.value.length,
    hint: `当前页 ${tableData.value.length} 个文件`,
    icon: 'file',
    tone: 'blue',
  },
  {
    label: '总容量',
    value: totalSizeText.value,
    hint: `当前页 ${currentPageSizeText.value}`,
    icon: 'data',
    tone: 'green',
  },
  {
    label: '文件类型',
    value: fileTypeCount.value,
    hint: fileTypeOptions.value.length ? fileTypeOptions.value.slice(0, 3).join(' / ') : '等待文件类型',
    icon: 'file-search',
    tone: 'cyan',
  },
  {
    label: '存储来源',
    value: storageTypeCount.value,
    hint: tableData.value.length ? '按上传记录统计' : '暂无存储记录',
    icon: 'cloud-upload',
    tone: 'orange',
  },
]);

const updateTime = () => {
  lastUpdatedAt.value = new Date().toLocaleTimeString('zh-CN', { hour12: false });
};

const formatFileSize = (bytes?: number) => {
  const size = Number(bytes || 0);
  if (size <= 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const index = Math.min(Math.floor(Math.log(size) / Math.log(k)), sizes.length - 1);
  return `${(size / Math.pow(k, index)).toFixed(index === 0 ? 0 : 2)} ${sizes[index]}`;
};

const fileExtension = (fileName?: string) => {
  if (!fileName || !fileName.includes('.')) return '';
  return fileName.split('.').pop()?.toUpperCase() || '';
};

const fileIcon = (row: FileItem) => {
  const type = `${row.file_type || row.mime_type || row.extension || ''}`.toLowerCase();
  if (type.includes('image')) return 'file-image';
  if (type.includes('pdf')) return 'file-pdf';
  if (type.includes('zip') || type.includes('rar')) return 'file-zip';
  if (type.includes('excel') || type.includes('sheet')) return 'file-excel';
  if (type.includes('word') || type.includes('document')) return 'file-word';
  return 'file';
};

const loadData = async () => {
  loading.value = true;
  try {
    const res = await getFileList({
      page: pagination.value.current,
      page_size: pagination.value.pageSize,
      keyword: searchForm.value.keyword.trim() || undefined,
      file_type: searchForm.value.file_type || undefined,
    });
    tableData.value = res.list || [];
    pagination.value.total = res.total || 0;
    updateTime();
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载文件数据失败');
  } finally {
    loading.value = false;
  }
};

const loadStats = async () => {
  statsLoading.value = true;
  try {
    stats.value = await getFileStats();
  } catch {
    stats.value = null;
  } finally {
    statsLoading.value = false;
  }
};

const handleSearch = () => {
  pagination.value.current = 1;
  loadData();
};

const handleReset = () => {
  searchForm.value = {
    keyword: '',
    file_type: '',
  };
  handleSearch();
};

const handleUpload = () => {
  uploadVisible.value = true;
};

const handleUploadSuccess = () => {
  MessagePlugin.success('上传成功');
  uploadVisible.value = false;
  fileList.value = [];
  loadData();
  loadStats();
};

const openBlob = (blob: Blob, filename?: string) => {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.target = '_blank';
  if (filename) {
    link.download = filename;
  }
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
};

const handleDownload = async (row: FileItem) => {
  try {
    const blob = await downloadFile(row.id);
    openBlob(blob, row.file_name);
  } catch (error: any) {
    MessagePlugin.error(error.message || '下载失败');
  }
};

const handlePreview = async (row: FileItem) => {
  try {
    const blob = await previewFile(row.id);
    openBlob(blob);
  } catch (error: any) {
    MessagePlugin.error(error.message || '预览失败');
  }
};

const handleDelete = async (row: FileItem) => {
  try {
    await deleteFile(row.id);
    MessagePlugin.success('删除成功');
    loadData();
    loadStats();
  } catch (error: any) {
    MessagePlugin.error(error.message || '删除失败');
  }
};

const handleRefresh = () => {
  loadData();
  loadStats();
};

const handlePageChange = (pageInfo: any) => {
  pagination.value.current = pageInfo.current ?? pageInfo;
  pagination.value.pageSize = pageInfo.pageSize ?? pagination.value.pageSize;
  loadData();
};

const handlePageSizeChange = (pageSize: number) => {
  pagination.value.pageSize = pageSize;
  pagination.value.current = 1;
  loadData();
};

onMounted(() => {
  loadData();
  loadStats();
});
</script>

<style lang="less" scoped>
.file-page {
  --file-bg: #f5f7fb;
  --file-card-shadow: 0 12px 28px rgb(15 23 42 / 6%);
  --file-font: 'Inter', 'HarmonyOS Sans SC', 'MiSans', 'PingFang SC', 'Microsoft YaHei UI', 'Microsoft YaHei', 'Arial', sans-serif;
  --file-number-font: 'DIN Alternate', 'Bahnschrift', 'Inter', 'HarmonyOS Sans SC', 'Microsoft YaHei UI', sans-serif;

  display: flex;
  min-height: calc(100vh - 120px);
  min-width: 0;
  flex-direction: column;
  gap: 14px;
  margin: calc(-1 * var(--td-comp-paddingTB-xl)) calc(-1 * var(--td-comp-paddingLR-xl));
  padding: 14px 18px 18px;
  background: var(--file-bg);
  color: var(--td-text-color-primary);
  font-family: var(--file-font);
  font-feature-settings: 'tnum';
  font-variant-numeric: tabular-nums;
  letter-spacing: 0;
  text-rendering: optimizelegibility;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}

.file-page :deep(.t-card),
.file-page :deep(.t-table),
.file-page :deep(.t-form),
.file-page :deep(.t-button),
.file-page :deep(.t-tag),
.file-page :deep(.t-input),
.file-page :deep(.t-select),
.file-page :deep(.t-dialog),
.file-page :deep(.t-empty),
.file-page :deep(.t-upload) {
  font-family: var(--file-font);
}

.file-head {
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

.file-head__main {
  min-width: 0;
}

.file-head__title {
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

.file-head__meta {
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

.file-head__actions {
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
    overflow: hidden;
    color: #0f172a;
    font-family: var(--file-number-font);
    font-size: 34px;
    font-weight: 800;
    line-height: 38px;
    text-overflow: ellipsis;
    white-space: nowrap;
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
  box-shadow: var(--file-card-shadow);
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
  width: 180px;
}

.filter-card__actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 12px 20px 18px;
}

.file-table {
  width: 100%;
}

.file-table :deep(.t-table__header th) {
  background: #f8fafc;
  color: #475569;
  font-size: 13px;
  font-weight: 700;
}

.file-table :deep(.t-table__body tr:hover td) {
  background: #f8fbff;
}

.file-table :deep(.t-table__body td) {
  padding-top: 14px;
  padding-bottom: 14px;
  border-bottom-color: #eef2f7;
  color: #1f2937;
  vertical-align: top;
}

.file-cell {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
}

.file-avatar {
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

.file-cell__main,
.size-cell,
.date-cell {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 3px;

  strong,
  :deep(.t-link) {
    overflow: hidden;
    max-width: 100%;
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
  color: #475569;
  font-size: 13px;
  line-height: 20px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mono-text {
  font-family: var(--file-number-font);
  font-variant-numeric: tabular-nums;
}

.operation-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.upload-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.upload-panel__hero {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px;
  border: 1px solid #dbeafe;
  border-radius: 12px;
  background: #f8fbff;

  > span {
    display: inline-flex;
    width: 40px;
    height: 40px;
    flex-shrink: 0;
    align-items: center;
    justify-content: center;
    border-radius: 13px;
    background: #dbeafe;
    color: #2563eb;
    font-size: 22px;
  }

  strong {
    display: block;
    color: #111827;
    font-size: 15px;
    font-weight: 800;
    line-height: 22px;
  }

  small {
    color: #64748b;
    font-size: 12px;
    line-height: 18px;
  }
}

@media (width <= 1200px) {
  .summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (width <= 768px) {
  .file-page {
    margin: calc(-1 * var(--td-comp-paddingTB-l)) calc(-1 * var(--td-comp-paddingLR-l));
    padding: 12px;
  }

  .file-head,
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
