<template>
  <div class="job-page ops-list-page">
    <console-page-header title="定时任务管理" :status-theme="headerStatusTheme" :status-text="headerStatusText" :meta="headerMeta">
      <template #actions>
        <t-tag :theme="abnormalCount > 0 ? 'warning' : 'success'" variant="light">异常 {{ abnormalCount }}</t-tag>
        <t-tooltip content="刷新" placement="bottom">
          <t-button class="console-page-header__refresh" variant="outline" size="small" shape="square" :loading="loading || healthLoading" @click="handleRefreshAll">
            <template #icon><t-icon name="refresh" /></template>
          </t-button>
        </t-tooltip>
        <t-button v-permission="'system:job:run'" size="small" variant="outline" theme="warning" @click="openCleanupDialog">
          <template #icon><t-icon name="delete" /></template>
          清理日志
        </t-button>
        <t-button v-permission="'system:job:create'" size="small" theme="primary" @click="handleAdd">
          <template #icon><t-icon name="add" /></template>
          新增任务
        </t-button>
      </template>
    </console-page-header>

    <div class="summary-grid">
      <section
        v-for="item in healthCards"
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

    <t-alert
      v-if="abnormalMessage"
      class="job-alert"
      theme="warning"
      :message="abnormalMessage"
      close-btn
    />

    <t-card :bordered="false" class="table-card">
      <div class="table-card__head">
        <div>
          <h3>任务列表</h3>
          <p>维护系统定时任务、执行目标、启停状态和即时执行入口</p>
        </div>
        <t-space size="small" break-line>
          <t-tag theme="primary" variant="light">当前页 {{ jobs.length }} 条</t-tag>
          <t-tag theme="success" variant="light">启用 {{ enabledCount }}</t-tag>
          <t-tag :theme="pausedCount > 0 ? 'warning' : 'default'" variant="light">暂停 {{ pausedCount }}</t-tag>
        </t-space>
      </div>

      <div class="filter-card">
        <div class="filter-card__head">
          <div>
            <h4>筛选任务</h4>
            <p>
              按任务名称和调度状态查询
              <template v-if="activeFilterCount"> · 已应用 {{ activeFilterCount }} 个条件</template>
            </p>
          </div>
          <t-tag :theme="healthLoading ? 'primary' : 'default'" variant="light">
            {{ healthLoading ? '健康检查中' : `窗口 ${health?.window_hours || 24} 小时` }}
          </t-tag>
        </div>
        <t-form :data="searchForm" class="filter-form" layout="inline" @submit="handleSearch">
          <t-form-item label="任务名称" name="name">
            <t-input
              v-model="searchForm.name"
              clearable
              class="keyword-input"
              placeholder="请输入任务名称"
              @enter="handleSearch"
            >
              <template #prefix-icon><t-icon name="search" /></template>
            </t-input>
          </t-form-item>
          <t-form-item label="状态" name="status">
            <t-select v-model="searchForm.status" class="status-select" clearable placeholder="全部状态">
              <t-option :value="1" label="运行中" />
              <t-option :value="0" label="已暂停" />
            </t-select>
          </t-form-item>
        </t-form>
        <div class="filter-card__actions">
          <t-space size="small" break-line>
            <t-button theme="primary" :loading="loading" @click="handleSearch">
              <template #icon><t-icon name="search" /></template>
              查询
            </t-button>
            <t-button variant="base" :disabled="loading" @click="resetSearch">重置</t-button>
            <t-button variant="outline" :loading="healthLoading" @click="loadHealth">
              <template #icon><t-icon name="refresh" /></template>
              刷新健康
            </t-button>
          </t-space>
          <t-button v-permission="'system:job:create'" theme="primary" @click="handleAdd">
            <template #icon><t-icon name="add" /></template>
            新增任务
          </t-button>
        </div>
      </div>

      <t-table
        row-key="id"
        hover
        table-layout="fixed"
        class="job-table"
        :data="jobs"
        :columns="columns"
        :loading="loading"
        :pagination="pagination"
        @page-change="onPageChange"
      >
        <template #empty>
          <t-empty :description="loading ? '正在加载定时任务' : '当前筛选条件下暂无定时任务'" />
        </template>
        <template #job="{ row }">
          <div class="job-cell">
            <span class="job-avatar">{{ jobInitial(row.name || row.group_name) }}</span>
            <div class="job-cell__main">
              <strong>{{ row.name || '未命名任务' }}</strong>
              <span>{{ row.group_name || 'default' }} · ID {{ row.id }}</span>
            </div>
          </div>
        </template>
        <template #schedule="{ row }">
          <div class="schedule-cell">
            <span class="mono-text">{{ row.cron_expression || '-' }}</span>
            <small>{{ row.invoke_target || '-' }}</small>
          </div>
        </template>
        <template #status="{ row }">
          <t-tag :theme="row.status === 1 ? 'success' : 'warning'" variant="light">
            {{ row.status === 1 ? '运行中' : '已暂停' }}
          </t-tag>
        </template>
        <template #concurrent="{ row }">
          <t-tag :theme="row.concurrent === 1 ? 'primary' : 'default'" variant="light">
            {{ row.concurrent === 1 ? '允许' : '禁止' }}
          </t-tag>
        </template>
        <template #runtime="{ row }">
          <div class="date-cell">
            <strong>上次 {{ formatDateTime(row.last_run_time) }}</strong>
            <span>下次 {{ formatDateTime(row.next_run_time) }}</span>
          </div>
        </template>
        <template #operation="{ row }">
          <div class="operation-actions">
            <t-link theme="primary" hover="color" @click="handleEdit(row)">编辑</t-link>
            <t-link v-if="row.status === 1" theme="warning" hover="color" @click="handleStop(row.id)">停止</t-link>
            <t-link v-else theme="success" hover="color" @click="handleStart(row.id)">启动</t-link>
            <t-link v-permission="'system:job:run'" theme="primary" hover="color" @click="handleRun(row.id)">执行</t-link>
            <t-popconfirm content="确认删除该定时任务吗？" @confirm="handleDelete(row.id)">
              <t-link theme="danger" hover="color">删除</t-link>
            </t-popconfirm>
          </div>
        </template>
      </t-table>
    </t-card>

    <t-dialog
      v-model:visible="dialogVisible"
      :header="dialogTitle"
      :confirm-btn="{ content: '确定', theme: 'primary' }"
      width="720px"
      @confirm="handleSubmit"
    >
      <t-form ref="formRef" :data="formData" :rules="formRules" label-width="100px" class="job-form">
        <div class="form-grid">
          <t-form-item label="任务名称" name="name">
            <t-input v-model="formData.name" placeholder="请输入任务名称" />
          </t-form-item>
          <t-form-item label="任务组" name="group_name">
            <t-input v-model="formData.group_name" placeholder="请输入任务组" />
          </t-form-item>
          <t-form-item label="Cron 表达式" name="cron_expression">
            <t-input v-model="formData.cron_expression" placeholder="如: 0 */5 * * * ?" />
          </t-form-item>
          <t-form-item label="调用目标" name="invoke_target">
            <t-input v-model="formData.invoke_target" placeholder="函数名，如: CleanExpiredLogs" />
          </t-form-item>
          <t-form-item class="form-grid__full" label="任务描述" name="description">
            <t-textarea v-model="formData.description" placeholder="请输入任务描述" :autosize="{ minRows: 3, maxRows: 5 }" />
          </t-form-item>
          <t-form-item label="状态" name="status">
            <t-radio-group v-model="formData.status" variant="default-filled">
              <t-radio-button :value="1">启用</t-radio-button>
              <t-radio-button :value="0">禁用</t-radio-button>
            </t-radio-group>
          </t-form-item>
          <t-form-item label="并发执行" name="concurrent">
            <t-radio-group v-model="formData.concurrent" variant="default-filled">
              <t-radio-button :value="1">允许</t-radio-button>
              <t-radio-button :value="0">禁止</t-radio-button>
            </t-radio-group>
          </t-form-item>
        </div>
      </t-form>
    </t-dialog>

    <t-dialog
      v-model:visible="cleanupDialogVisible"
      header="清理任务日志"
      :confirm-btn="{ content: '清理', theme: 'warning' }"
      width="420px"
      @confirm="handleCleanupLogs"
    >
      <t-form :data="cleanupForm" label-width="100px">
        <t-form-item label="保留天数" name="retention_days">
          <t-input-number v-model="cleanupForm.retention_days" :min="1" :max="3650" style="width: 100%" />
        </t-form-item>
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { MessagePlugin } from 'tdesign-vue-next';

import {
  cleanupJobLogs,
  createJob,
  deleteJob,
  getJobHealth,
  getJobList,
  runJob,
  startJob,
  stopJob,
  type JobHealthCheck,
  type ScheduledJob,
  updateJob,
} from '@/api/monitor/job';
import { formatDateTime } from '@/utils/date';
import ConsolePageHeader from '@/components/common/ConsolePageHeader.vue';

type SummaryTone = 'blue' | 'green' | 'cyan' | 'orange' | 'red';
type HeaderTagTheme = 'success' | 'warning';

defineOptions({
  name: 'MonitorJob',
});

const loading = ref(false);
const healthLoading = ref(false);
const jobs = ref<ScheduledJob[]>([]);
const health = ref<JobHealthCheck | null>(null);
const dialogVisible = ref(false);
const cleanupDialogVisible = ref(false);
const dialogTitle = ref('新增任务');
const formRef = ref();

const searchForm = reactive({
  name: '',
  status: undefined as number | undefined,
});

const pagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
});

const formData = reactive<Partial<ScheduledJob>>({
  name: '',
  group_name: 'default',
  cron_expression: '',
  invoke_target: '',
  description: '',
  status: 1,
  concurrent: 0,
});

const cleanupForm = reactive({
  retention_days: 30,
});

const formRules = {
  name: [{ required: true, message: '请输入任务名称' }],
  cron_expression: [{ required: true, message: '请输入 Cron 表达式' }],
  invoke_target: [{ required: true, message: '请输入调用目标' }],
};

const columns: any[] = [
  { colKey: 'job', title: '任务', width: 260, fixed: 'left' as const },
  { colKey: 'schedule', title: '调度 / 调用目标', minWidth: 260 },
  { colKey: 'status', title: '状态', width: 96 },
  { colKey: 'concurrent', title: '并发', width: 88 },
  { colKey: 'runtime', title: '上次 / 下次运行', width: 240 },
  { colKey: 'operation', title: '操作', width: 210, fixed: 'right' as const },
];

const enabledCount = computed(() => health.value?.enabled ?? jobs.value.filter((item) => item.status === 1).length);
const pausedCount = computed(() => health.value?.paused ?? jobs.value.filter((item) => item.status !== 1).length);
const abnormalCount = computed(() => health.value?.abnormal_jobs?.length ?? 0);
const recentFailedCount = computed(() => health.value?.recent_failed ?? 0);
const activeFilterCount = computed(() => {
  let count = 0;
  if (searchForm.name.trim()) count += 1;
  if (searchForm.status !== undefined) count += 1;
  return count;
});
const headerStatusTheme = computed<HeaderTagTheme>(() => (abnormalCount.value > 0 || recentFailedCount.value > 0 ? 'warning' : 'success'));
const headerStatusText = computed(() => (abnormalCount.value > 0 || recentFailedCount.value > 0 ? '存在任务风险' : '调度状态正常'));
const headerMeta = computed(() => [
  '系统调度',
  'Cron 任务',
  '日志清理',
  `共 ${pagination.total} 个任务`,
  health.value?.checked_at ? `检查于 ${formatDateTime(health.value.checked_at)}` : '',
]);

const healthCards = computed<Array<{ label: string; value: number | string; hint: string; icon: string; tone: SummaryTone }>>(() => [
  {
    label: '任务总数',
    value: health.value?.total ?? pagination.total,
    hint: `当前页 ${jobs.value.length} 条`,
    icon: 'task',
    tone: 'blue',
  },
  {
    label: '启用任务',
    value: enabledCount.value,
    hint: `暂停 ${pausedCount.value} 个`,
    icon: 'check-circle',
    tone: 'green',
  },
  {
    label: '最近失败',
    value: recentFailedCount.value,
    hint: `近 ${health.value?.window_hours || 24} 小时`,
    icon: 'error-circle',
    tone: recentFailedCount.value > 0 ? 'red' : 'cyan',
  },
  {
    label: '异常任务',
    value: abnormalCount.value,
    hint: abnormalCount.value > 0 ? '需要处理调度异常' : '未发现异常',
    icon: 'error-circle',
    tone: abnormalCount.value > 0 ? 'orange' : 'green',
  },
]);

const abnormalMessage = computed(() => {
  const abnormalJobs = health.value?.abnormal_jobs || [];
  if (!abnormalJobs.length) return '';
  const preview = abnormalJobs
    .slice(0, 3)
    .map((job) => `${job.name}: ${job.reason}`)
    .join('；');
  return abnormalJobs.length > 3 ? `${preview} 等 ${abnormalJobs.length} 个异常任务` : preview;
});

const loadData = async () => {
  loading.value = true;
  try {
    const res = await getJobList({
      page: pagination.current,
      page_size: pagination.pageSize,
      name: searchForm.name.trim() || undefined,
      status: searchForm.status,
    });
    jobs.value = res.list || [];
    pagination.total = res.total || 0;
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载失败');
  } finally {
    loading.value = false;
  }
};

const loadHealth = async () => {
  healthLoading.value = true;
  try {
    health.value = await getJobHealth();
  } catch (error: any) {
    MessagePlugin.error(error.message || '健康检查加载失败');
  } finally {
    healthLoading.value = false;
  }
};

const handleRefreshAll = () => {
  loadData();
  loadHealth();
};

const handleSearch = () => {
  pagination.current = 1;
  loadData();
};

const onPageChange = (pageInfo: any) => {
  pagination.current = pageInfo.current;
  pagination.pageSize = pageInfo.pageSize;
  loadData();
};

const resetSearch = () => {
  searchForm.name = '';
  searchForm.status = undefined;
  pagination.current = 1;
  loadData();
};

const handleAdd = () => {
  dialogTitle.value = '新增任务';
  Object.assign(formData, {
    id: undefined,
    name: '',
    group_name: 'default',
    cron_expression: '',
    invoke_target: '',
    description: '',
    status: 1,
    concurrent: 0,
  });
  dialogVisible.value = true;
};

const handleEdit = (row: ScheduledJob) => {
  dialogTitle.value = '编辑任务';
  Object.assign(formData, row);
  dialogVisible.value = true;
};

const handleSubmit = async () => {
  const valid = await formRef.value?.validate();
  if (valid !== true) return;

  try {
    if (formData.id) {
      await updateJob(formData.id, formData);
      MessagePlugin.success('更新成功');
    } else {
      await createJob(formData);
      MessagePlugin.success('创建成功');
    }
    dialogVisible.value = false;
    loadData();
    loadHealth();
  } catch (error: any) {
    MessagePlugin.error(error.message || '操作失败');
  }
};

const handleDelete = async (id: number) => {
  try {
    await deleteJob(id);
    MessagePlugin.success('删除成功');
    loadData();
    loadHealth();
  } catch (error: any) {
    MessagePlugin.error(error.message || '删除失败');
  }
};

const handleStart = async (id: number) => {
  try {
    await startJob(id);
    MessagePlugin.success('启动成功');
    loadData();
    loadHealth();
  } catch (error: any) {
    MessagePlugin.error(error.message || '启动失败');
  }
};

const handleStop = async (id: number) => {
  try {
    await stopJob(id);
    MessagePlugin.success('停止成功');
    loadData();
    loadHealth();
  } catch (error: any) {
    MessagePlugin.error(error.message || '停止失败');
  }
};

const handleRun = async (id: number) => {
  try {
    await runJob(id);
    MessagePlugin.success('任务已开始执行');
    loadHealth();
  } catch (error: any) {
    MessagePlugin.error(error.message || '执行失败');
  }
};

const openCleanupDialog = () => {
  cleanupForm.retention_days = 30;
  cleanupDialogVisible.value = true;
};

const handleCleanupLogs = async () => {
  if (!cleanupForm.retention_days || cleanupForm.retention_days <= 0) {
    MessagePlugin.warning('保留天数必须大于0');
    return;
  }

  try {
    const res = await cleanupJobLogs({ retention_days: cleanupForm.retention_days });
    MessagePlugin.success(`已清理 ${res.deleted_rows || 0} 条任务日志`);
    cleanupDialogVisible.value = false;
    loadHealth();
  } catch (error: any) {
    MessagePlugin.error(error.message || '清理失败');
  }
};

const jobInitial = (value?: string) => (value || '任').slice(0, 1).toUpperCase();

onMounted(() => {
  loadData();
  loadHealth();
});
</script>

<style scoped lang="less">
.job-page {
  --job-bg: #f5f7fb;
  --job-card-shadow: 0 12px 28px rgb(15 23 42 / 6%);
  --job-font: 'Inter', 'HarmonyOS Sans SC', 'MiSans', 'PingFang SC', 'Microsoft YaHei UI', 'Microsoft YaHei', 'Arial',
    sans-serif;
  --job-number-font: 'DIN Alternate', 'Bahnschrift', 'Inter', 'HarmonyOS Sans SC', 'Microsoft YaHei UI', sans-serif;

  display: flex;
  min-height: calc(100vh - 120px);
  min-width: 0;
  flex-direction: column;
  gap: 14px;
  margin: calc(-1 * var(--td-comp-paddingTB-xl)) calc(-1 * var(--td-comp-paddingLR-xl));
  padding: 14px 18px 18px;
  background: var(--job-bg);
  color: var(--td-text-color-primary);
  font-family: var(--job-font);
  font-feature-settings: 'tnum';
  font-variant-numeric: tabular-nums;
  letter-spacing: 0;
}

.job-page :deep(.t-card),
.job-page :deep(.t-table),
.job-page :deep(.t-form),
.job-page :deep(.t-button),
.job-page :deep(.t-tag),
.job-page :deep(.t-input),
.job-page :deep(.t-select),
.job-page :deep(.t-alert),
.job-page :deep(.t-dialog),
.job-page :deep(.t-empty) {
  font-family: var(--job-font);
}

.job-head {
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

.job-head__main {
  min-width: 0;
}

.job-head__title {
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

.job-head__meta {
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

.job-head__actions {
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
    font-family: var(--job-number-font);
    font-size: 34px;
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

.job-alert {
  border-radius: 12px;
  box-shadow: 0 8px 18px rgb(15 23 42 / 5%);
}

.table-card {
  overflow: hidden;
  border: 1px solid #e8edf5;
  border-radius: 12px;
  background: #fff;
  box-shadow: var(--job-card-shadow);
}

.table-card :deep(.t-card__body) {
  padding: 0;
}

.table-card__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 14px;
  padding: 18px 20px 0;
  border-bottom: 1px solid #edf1f7;

  h3 {
    margin: 0;
    color: #111827;
    font-size: 18px;
    font-weight: 700;
    line-height: 24px;
  }

  p {
    margin: 5px 0 14px;
    color: #64748b;
    font-size: 13px;
    line-height: 20px;
  }
}

.filter-card {
  border-bottom: 1px solid #edf1f7;
  background: #fbfdff;
}

.filter-card__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 14px;
  padding: 18px 20px 12px;

  h4 {
    margin: 0;
    color: #111827;
    font-size: 16px;
    font-weight: 700;
    line-height: 22px;
  }

  p {
    margin: 5px 0 0;
    color: #64748b;
    font-size: 13px;
    line-height: 20px;
  }
}

.filter-form {
  display: grid;
  grid-template-columns: minmax(260px, 1.4fr) minmax(160px, 0.7fr);
  align-items: stretch;
  gap: 10px 12px;
  padding: 0 20px 4px;
}

.filter-form :deep(.t-form__item) {
  display: flex;
  min-width: 0;
  flex-direction: column;
  align-items: stretch;
  gap: 4px;
  margin: 0;
}

.filter-form :deep(.t-form__label) {
  width: auto !important;
  max-width: 100%;
  height: 20px;
  padding: 0;
  color: #475467;
  font-size: 12px;
  font-weight: 650;
  line-height: 20px;
  text-align: left;
}

.filter-form :deep(.t-form__controls),
.filter-form :deep(.t-form__controls-content) {
  min-width: 0;
  width: 100%;
  margin-left: 0 !important;
}

.keyword-input {
  width: 100%;
}

.status-select {
  width: 100%;
}

.filter-card__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 0 20px 18px;
}

.job-table {
  width: 100%;
}

.job-table :deep(.t-table__th-cell-inner) {
  color: #475569;
  font-size: 12px;
  font-weight: 700;
}

.job-table :deep(.t-table__body td) {
  vertical-align: middle;
}

.job-cell {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
}

.job-avatar {
  display: inline-flex;
  width: 32px;
  height: 32px;
  flex: 0 0 32px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: linear-gradient(135deg, #2563eb, #14b8a6);
  color: #fff;
  font-size: 14px;
  font-weight: 800;
}

.job-cell__main,
.schedule-cell,
.date-cell {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 3px;

  strong,
  .mono-text {
    overflow: hidden;
    color: #0f172a;
    font-size: 14px;
    font-weight: 700;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  span,
  small {
    overflow: hidden;
    color: #64748b;
    font-size: 12px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.mono-text {
  font-family: 'JetBrains Mono', SFMono-Regular, Consolas, 'Liberation Mono', monospace;
}

.operation-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 10px;
}

.job-form {
  padding-top: 4px;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 16px;
}

.form-grid__full {
  grid-column: 1 / -1;
}

.job-form .form-grid :deep(.t-form__item) {
  display: flex;
  min-width: 0;
  flex-direction: column;
  align-items: stretch;
  gap: 4px;
  margin: 0 0 14px;
}

.job-form .form-grid :deep(.t-form__label) {
  width: auto !important;
  max-width: 100%;
  height: 20px;
  padding: 0;
  color: #475569;
  font-size: 12px;
  font-weight: 700;
  line-height: 20px;
  text-align: left;
}

.job-form .form-grid :deep(.t-form__controls),
.job-form .form-grid :deep(.t-form__controls-content) {
  min-width: 0;
  width: 100%;
  margin-left: 0 !important;
}

.job-form .form-grid :deep(.t-input),
.job-form .form-grid :deep(.t-select),
.job-form .form-grid :deep(.t-textarea),
.job-form .form-grid :deep(.t-input-number) {
  width: 100%;
}

@media (width <= 1200px) {
  .summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (width <= 768px) {
  .job-page {
    margin: calc(-1 * var(--td-comp-paddingTB-l)) calc(-1 * var(--td-comp-paddingLR-l));
    padding: 12px;
  }

  .job-head,
  .table-card__head,
  .filter-card__head,
  .filter-card__actions {
    align-items: stretch;
    flex-direction: column;
  }

  .keyword-input,
  .status-select {
    width: 100%;
    min-width: 0;
  }

  .summary-grid,
  .filter-form,
  .form-grid {
    grid-template-columns: 1fr;
  }

  .filter-form :deep(.t-form__item) {
    width: 100%;
    margin-right: 0;
  }
}
</style>
