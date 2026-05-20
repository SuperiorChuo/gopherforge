<template>
  <div class="department-page system-management-page">
    <console-page-header>
      <template #title>
        <h2>部门管理</h2>
        <t-tag :theme="disabledCount > 0 ? 'warning' : 'success'" variant="light">
          {{ disabledCount > 0 ? '存在禁用部门' : '组织状态正常' }}
        </t-tag>
      </template>
      <template #meta>
        <span>组织架构</span>
        <span>负责人维护</span>
        <span>部门状态</span>
        <span>共 {{ totalDepartmentCount }} 个部门</span>
        <span v-if="lastUpdatedAt">更新于 {{ lastUpdatedAt }}</span>
      </template>
      <template #actions>
        <t-tag theme="primary" variant="light">根部门 {{ rootDepartmentCount }}</t-tag>
        <t-button v-permission="'system:department:create'" theme="primary" @click="handleAdd">
          <template #icon><t-icon name="add" /></template>
          新增部门
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
            按部门名称、编码、负责人、电话和邮箱定位组织节点
            <template v-if="activeFilterCount"> · 已应用 {{ activeFilterCount }} 个条件</template>
          </p>
        </div>
        <t-space size="small" break-line>
          <t-tag theme="primary" variant="light">显示 {{ visibleDepartmentCount }} 个节点</t-tag>
          <t-tag :theme="isExpandAll ? 'success' : 'default'" variant="light">
            {{ isExpandAll ? '已展开' : '已折叠' }}
          </t-tag>
        </t-space>
      </div>
      <t-form :data="searchForm" class="filter-form" layout="inline" @submit="handleSearch">
        <t-form-item label="关键词" name="keyword">
          <t-input
            v-model="searchForm.keyword"
            clearable
            class="keyword-input"
            placeholder="部门名称 / 编码 / 负责人 / 联系方式"
            @enter="handleSearch"
          >
            <template #prefix-icon><t-icon name="search" /></template>
          </t-input>
        </t-form-item>
        <t-form-item label="状态" name="status">
          <t-select v-model="searchForm.status" clearable placeholder="全部状态" class="filter-select">
            <t-option label="启用" :value="1" />
            <t-option label="禁用" :value="0" />
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
          <t-button variant="outline" @click="toggleExpand">
            <template #icon><t-icon :name="isExpandAll ? 'folder-open' : 'folder'" /></template>
            {{ isExpandAll ? '折叠全部' : '展开全部' }}
          </t-button>
        </t-space>
        <t-space size="small" break-line>
          <t-tag theme="primary" variant="light">最大层级 {{ maxDepth }}</t-tag>
          <t-tag :theme="unstaffedCount > 0 ? 'warning' : 'success'" variant="light">
            未配负责人 {{ unstaffedCount }}
          </t-tag>
        </t-space>
      </div>
    </t-card>

    <t-card :bordered="false" class="table-card">
      <div class="table-card__head">
        <div>
          <h3>部门结构</h3>
          <p>组织层级、部门编码、负责人、联系方式、排序和状态</p>
        </div>
        <t-space size="small">
          <t-tag :theme="enabledCount > 0 ? 'success' : 'default'" variant="light">启用 {{ enabledCount }}</t-tag>
          <t-tag theme="primary" variant="light">叶子部门 {{ leafDepartmentCount }}</t-tag>
        </t-space>
      </div>

      <t-table
        row-key="id"
        hover
        class="department-table"
        table-layout="fixed"
        :data="filteredTableData"
        :columns="columns"
        :loading="loading"
        :tree="{ childrenKey: 'children', indent: 24, expandTreeNodeOnClick: true }"
        :expanded-tree-nodes="expandedNodes"
        @expand-change="onExpandChange"
      >
        <template #empty>
          <t-empty :description="loading ? '正在加载部门结构' : '当前筛选条件下暂无部门'" />
        </template>
        <template #department="{ row }">
          <div class="department-cell">
            <span class="department-avatar">{{ departmentInitial(row) }}</span>
            <div class="department-cell__main">
              <strong>{{ row.name || '未命名部门' }}</strong>
              <span class="mono-text">{{ row.code || '-' }} · ID {{ row.id }}</span>
            </div>
          </div>
        </template>
        <template #leader="{ row }">
          <div class="leader-cell">
            <strong>{{ row.leader || '未配置负责人' }}</strong>
            <span>{{ parentLabel(row) }}</span>
          </div>
        </template>
        <template #contact="{ row }">
          <div class="contact-cell">
            <strong>{{ row.phone || '未填写电话' }}</strong>
            <span>{{ row.email || '未填写邮箱' }}</span>
          </div>
        </template>
        <template #structure="{ row }">
          <div class="structure-cell">
            <strong>{{ row.children?.length || 0 }}</strong>
            <span>{{ row.children?.length ? '个下级部门' : '末级部门' }}</span>
          </div>
        </template>
        <template #sort="{ row }">
          <span class="sort-badge">{{ row.sort ?? 0 }}</span>
        </template>
        <template #status="{ row }">
          <t-tag :theme="row.status === 1 ? 'success' : 'default'" variant="light">
            {{ row.status === 1 ? '启用' : '禁用' }}
          </t-tag>
        </template>
        <template #updated_at="{ row }">
          <div class="date-cell">
            <strong>{{ formatDateTime(row.updated_at || row.created_at) }}</strong>
            <span>创建 {{ formatDateTime(row.created_at) }}</span>
          </div>
        </template>
        <template #operation="{ row }">
          <div class="operation-actions">
            <t-link v-permission="'system:department:update'" theme="primary" hover="color" @click="handleEdit(row)">
              编辑
            </t-link>
            <t-link v-permission="'system:department:create'" theme="primary" hover="color" @click="handleAddChild(row)">
              子部门
            </t-link>
            <t-popconfirm content="确定删除该部门吗？" @confirm="handleDelete(row)">
              <t-link v-permission="'system:department:delete'" theme="danger" hover="color">删除</t-link>
            </t-popconfirm>
          </div>
        </template>
      </t-table>
    </t-card>

    <t-dialog
      v-model:visible="dialogVisible"
      :header="dialogTitle"
      :confirm-on-enter="true"
      :confirm-btn="{ content: '提交', loading: submitLoading }"
      width="720px"
      @confirm="handleSubmit"
    >
      <t-form ref="formRef" :data="formData" :rules="formRules" label-width="92px" class="department-form">
        <div class="form-grid">
          <t-form-item class="form-grid__full" label="上级部门" name="parent_id">
            <t-tree-select
              v-model="formData.parent_id"
              :data="parentOptions"
              :tree-props="{ keys: { value: 'id', label: 'name', children: 'children' }, expandAll: true }"
              placeholder="请选择上级部门"
              clearable
            />
          </t-form-item>
          <t-form-item label="部门名称" name="name">
            <t-input v-model="formData.name" placeholder="请输入部门名称" />
          </t-form-item>
          <t-form-item label="部门编码" name="code">
            <t-input v-model="formData.code" placeholder="请输入部门编码" :disabled="isEdit" />
          </t-form-item>
          <t-form-item label="负责人" name="leader">
            <t-input v-model="formData.leader" placeholder="请输入负责人" />
          </t-form-item>
          <t-form-item label="联系电话" name="phone">
            <t-input v-model="formData.phone" placeholder="请输入联系电话" />
          </t-form-item>
          <t-form-item label="邮箱" name="email">
            <t-input v-model="formData.email" placeholder="请输入邮箱" />
          </t-form-item>
          <t-form-item label="排序" name="sort">
            <t-input-number v-model="formData.sort" :min="0" style="width: 100%" />
          </t-form-item>
          <t-form-item class="form-grid__full" label="状态" name="status">
            <t-radio-group v-model="formData.status" variant="default-filled">
              <t-radio-button :value="1">启用</t-radio-button>
              <t-radio-button :value="0">禁用</t-radio-button>
            </t-radio-group>
          </t-form-item>
        </div>
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { MessagePlugin } from 'tdesign-vue-next';
import { computed, onMounted, ref } from 'vue';

import {
  createDepartment,
  deleteDepartment,
  getDepartmentTree,
  updateDepartment,
  type DepartmentItem,
} from '@/api/system/department';
import { formatDateTime } from '@/utils/date';
import ConsolePageHeader from '@/components/common/ConsolePageHeader.vue';

type SummaryTone = 'blue' | 'green' | 'cyan' | 'orange';

interface DepartmentFormData {
  code: string;
  email: string;
  leader: string;
  name: string;
  parent_id: number;
  phone: string;
  sort: number;
  status: number;
}

defineOptions({
  name: 'SystemDepartment',
});

const loading = ref(false);
const submitLoading = ref(false);
const tableData = ref<DepartmentItem[]>([]);
const dialogVisible = ref(false);
const formRef = ref();
const isEdit = ref(false);
const currentDept = ref<DepartmentItem | null>(null);
const isExpandAll = ref(true);
const expandedNodes = ref<number[]>([]);
const lastUpdatedAt = ref('');

const searchForm = ref<{ keyword: string; status?: number }>({
  keyword: '',
  status: undefined,
});

const defaultFormData = (): DepartmentFormData => ({
  name: '',
  code: '',
  parent_id: 0,
  leader: '',
  phone: '',
  email: '',
  sort: 0,
  status: 1,
});

const formData = ref<DepartmentFormData>(defaultFormData());

const formRules: any = {
  name: [{ required: true, message: '请输入部门名称' }],
  code: [{ required: true, message: '请输入部门编码' }],
};

const columns: any[] = [
  { colKey: 'department', title: '部门', width: 280, fixed: 'left' as const },
  { colKey: 'leader', title: '负责人', width: 170 },
  { colKey: 'contact', title: '联系方式', width: 220 },
  { colKey: 'structure', title: '结构', width: 120 },
  { colKey: 'sort', title: '排序', width: 88 },
  { colKey: 'status', title: '状态', width: 96 },
  { colKey: 'updated_at', title: '创建 / 更新', width: 220 },
  { colKey: 'operation', title: '操作', width: 170, fixed: 'right' as const },
];

const allDepartments = computed(() => flattenDepartments(tableData.value));
const departmentNameMap = computed(() => new Map(allDepartments.value.map((item) => [item.id, item.name])));

const filteredTableData = computed(() =>
  filterDepartmentTree(tableData.value, searchForm.value.keyword, searchForm.value.status),
);
const visibleDepartmentCount = computed(() => flattenDepartments(filteredTableData.value).length);
const totalDepartmentCount = computed(() => allDepartments.value.length);
const rootDepartmentCount = computed(() => tableData.value.length);
const enabledCount = computed(() => allDepartments.value.filter((item) => item.status === 1).length);
const disabledCount = computed(() => allDepartments.value.filter((item) => item.status !== 1).length);
const staffedCount = computed(() => allDepartments.value.filter((item) => Boolean(item.leader?.trim())).length);
const unstaffedCount = computed(() => Math.max(totalDepartmentCount.value - staffedCount.value, 0));
const leafDepartmentCount = computed(() => allDepartments.value.filter((item) => !item.children?.length).length);
const maxDepth = computed(() => getMaxDepth(tableData.value));
const activeFilterCount = computed(() => {
  let count = 0;
  if (searchForm.value.keyword.trim()) count += 1;
  if (searchForm.value.status !== undefined) count += 1;
  return count;
});
const leaderCoverage = computed(() => {
  if (!totalDepartmentCount.value) return '0%';
  return `${Math.round((staffedCount.value / totalDepartmentCount.value) * 100)}%`;
});
const dialogTitle = computed(() => (isEdit.value ? '编辑部门' : '新增部门'));

const summaryItems = computed<Array<{ label: string; value: string | number; hint: string; icon: string; tone: SummaryTone }>>(() => [
  {
    label: '部门总数',
    value: totalDepartmentCount.value,
    hint: `根部门 ${rootDepartmentCount.value} 个`,
    icon: 'tree-list',
    tone: 'blue',
  },
  {
    label: '启用部门',
    value: enabledCount.value,
    hint: `禁用 ${disabledCount.value} 个`,
    icon: 'check-circle',
    tone: 'green',
  },
  {
    label: '负责人覆盖',
    value: leaderCoverage.value,
    hint: `${staffedCount.value} 个部门已配置`,
    icon: 'user-talk',
    tone: 'cyan',
  },
  {
    label: '组织深度',
    value: maxDepth.value,
    hint: `末级部门 ${leafDepartmentCount.value} 个`,
    icon: 'git-branch',
    tone: 'orange',
  },
]);

const parentOptions = computed(() => {
  const source = currentDept.value ? removeDepartmentFromTree(tableData.value, currentDept.value.id) : tableData.value;
  return [
    { id: 0, name: '无上级部门', children: [] },
    ...cloneDepartmentTree(source),
  ];
});

const loadData = async () => {
  loading.value = true;
  try {
    const data = await getDepartmentTree(searchForm.value.status);
    tableData.value = data || [];
    if (isExpandAll.value) {
      expandedNodes.value = collectAllIds(filteredTableData.value);
    }
    updateTime();
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载数据失败');
  } finally {
    loading.value = false;
  }
};

const handleSearch = () => {
  loadData();
};

const handleRefresh = () => {
  loadData();
};

const handleReset = () => {
  searchForm.value = {
    keyword: '',
    status: undefined,
  };
  loadData();
};

const toggleExpand = () => {
  isExpandAll.value = !isExpandAll.value;
  expandedNodes.value = isExpandAll.value ? collectAllIds(filteredTableData.value) : [];
};

const onExpandChange = (keys: Array<string | number>) => {
  expandedNodes.value = keys.map((key) => Number(key));
};

const handleAdd = () => {
  isEdit.value = false;
  currentDept.value = null;
  formData.value = defaultFormData();
  dialogVisible.value = true;
};

const handleAddChild = (row: DepartmentItem) => {
  isEdit.value = false;
  currentDept.value = null;
  formData.value = {
    ...defaultFormData(),
    parent_id: row.id,
  };
  dialogVisible.value = true;
};

const handleEdit = (row: DepartmentItem) => {
  isEdit.value = true;
  currentDept.value = row;
  formData.value = {
    name: row.name,
    code: row.code,
    parent_id: row.parent_id,
    leader: row.leader || '',
    phone: row.phone || '',
    email: row.email || '',
    sort: row.sort || 0,
    status: row.status,
  };
  dialogVisible.value = true;
};

const handleDelete = async (row: DepartmentItem) => {
  try {
    await deleteDepartment(row.id);
    MessagePlugin.success('删除成功');
    loadData();
  } catch (error: any) {
    MessagePlugin.error(error.message || '删除失败');
  }
};

const handleSubmit = async () => {
  const valid = await formRef.value?.validate();
  if (valid !== true) return;

  submitLoading.value = true;
  try {
    if (isEdit.value && currentDept.value) {
      await updateDepartment(currentDept.value.id, {
        name: formData.value.name,
        parent_id: formData.value.parent_id,
        leader: formData.value.leader,
        phone: formData.value.phone,
        email: formData.value.email,
        sort: formData.value.sort,
        status: formData.value.status,
      });
      MessagePlugin.success('更新成功');
    } else {
      await createDepartment(formData.value);
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

const updateTime = () => {
  lastUpdatedAt.value = new Date().toLocaleTimeString('zh-CN', { hour12: false });
};

const collectAllIds = (items: DepartmentItem[]): number[] => flattenDepartments(items).map((item) => item.id);

const flattenDepartments = (items: DepartmentItem[]): DepartmentItem[] => {
  const result: DepartmentItem[] = [];
  const walk = (list: DepartmentItem[]) => {
    for (const item of list) {
      result.push(item);
      if (item.children?.length) walk(item.children);
    }
  };
  walk(items);
  return result;
};

const filterDepartmentTree = (items: DepartmentItem[], keyword: string, status?: number): DepartmentItem[] => {
  const normalizedKeyword = keyword.trim().toLowerCase();
  const result: DepartmentItem[] = [];
  for (const item of items) {
    const children = filterDepartmentTree(item.children || [], keyword, status);
    const values = [item.name, item.code, item.leader, item.phone, item.email].filter(Boolean).join(' ').toLowerCase();
    const keywordMatched = !normalizedKeyword || values.includes(normalizedKeyword);
    const statusMatched = status === undefined || item.status === status;
    if ((keywordMatched && statusMatched) || children.length) {
      result.push({
        ...item,
        children,
      });
    }
  }
  return result;
};

const cloneDepartmentTree = (items: DepartmentItem[]): DepartmentItem[] =>
  items.map((item) => ({
    ...item,
    children: item.children?.length ? cloneDepartmentTree(item.children) : [],
  }));

const removeDepartmentFromTree = (items: DepartmentItem[], departmentId: number): DepartmentItem[] =>
  items
    .filter((item) => item.id !== departmentId)
    .map((item) => ({
      ...item,
      children: item.children?.length ? removeDepartmentFromTree(item.children, departmentId) : [],
    }));

const getMaxDepth = (items: DepartmentItem[], depth = 1): number => {
  if (!items.length) return 0;
  return Math.max(...items.map((item) => (item.children?.length ? getMaxDepth(item.children, depth + 1) : depth)));
};

const departmentInitial = (row: DepartmentItem) => (row.name || row.code || '部').slice(0, 1).toUpperCase();

const parentLabel = (row: DepartmentItem) => {
  if (!row.parent_id) return '根部门';
  return departmentNameMap.value.get(row.parent_id) || `上级 ID ${row.parent_id}`;
};

onMounted(() => {
  loadData();
});
</script>

<style lang="less" scoped>
.department-page {
  --department-bg: #f5f7fb;
  --department-card-shadow: 0 12px 28px rgb(15 23 42 / 6%);
  --department-font: 'Inter', 'HarmonyOS Sans SC', 'MiSans', 'PingFang SC', 'Microsoft YaHei UI', 'Microsoft YaHei', 'Arial', sans-serif;
  --department-number-font: 'DIN Alternate', 'Bahnschrift', 'Inter', 'HarmonyOS Sans SC', 'Microsoft YaHei UI', sans-serif;

  display: flex;
  min-height: calc(100vh - 120px);
  min-width: 0;
  flex-direction: column;
  gap: 14px;
  margin: calc(-1 * var(--td-comp-paddingTB-xl)) calc(-1 * var(--td-comp-paddingLR-xl));
  padding: 14px 18px 18px;
  background: var(--department-bg);
  color: var(--td-text-color-primary);
  font-family: var(--department-font);
  font-feature-settings: 'tnum';
  font-variant-numeric: tabular-nums;
  letter-spacing: 0;
}

.department-page :deep(.t-card),
.department-page :deep(.t-table),
.department-page :deep(.t-form),
.department-page :deep(.t-button),
.department-page :deep(.t-tag),
.department-page :deep(.t-input),
.department-page :deep(.t-select),
.department-page :deep(.t-dialog),
.department-page :deep(.t-empty) {
  font-family: var(--department-font);
}

.department-head {
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

.department-head__main {
  min-width: 0;
}

.department-head__title {
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

.department-head__meta {
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

.department-head__actions {
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
    font-family: var(--department-number-font);
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

.filter-card,
.table-card {
  overflow: hidden;
  border: 1px solid #e8edf5;
  border-radius: 12px;
  background: #fff;
  box-shadow: var(--department-card-shadow);
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
  padding: 16px 20px 4px;
}

.filter-form :deep(.t-form__item) {
  margin-right: 18px;
  margin-bottom: 12px;
}

.keyword-input {
  width: min(420px, 52vw);
}

.filter-select {
  width: 180px;
}

.filter-card__actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 0 20px 18px;
}

.department-table {
  width: 100%;
}

.department-table :deep(.t-table__th-cell-inner) {
  color: #475569;
  font-size: 12px;
  font-weight: 700;
}

.department-table :deep(.t-table__body td) {
  vertical-align: middle;
}

.department-cell,
.leader-cell,
.contact-cell,
.structure-cell,
.date-cell {
  min-width: 0;
}

.department-cell {
  display: flex;
  align-items: center;
  gap: 10px;
}

.department-avatar {
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

.department-cell__main,
.leader-cell,
.contact-cell,
.date-cell {
  display: flex;
  flex-direction: column;
  gap: 3px;

  strong {
    overflow: hidden;
    color: #0f172a;
    font-size: 14px;
    font-weight: 700;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  span {
    overflow: hidden;
    color: #64748b;
    font-size: 12px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.structure-cell {
  display: flex;
  align-items: baseline;
  gap: 5px;

  strong {
    color: #0f172a;
    font-family: var(--department-number-font);
    font-size: 22px;
    font-weight: 800;
  }

  span {
    color: #64748b;
    font-size: 12px;
  }
}

.sort-badge {
  display: inline-flex;
  min-width: 32px;
  height: 26px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: #eef4ff;
  color: #2563eb;
  font-family: var(--department-number-font);
  font-size: 14px;
  font-weight: 800;
}

.mono-text {
  font-family: 'JetBrains Mono', SFMono-Regular, Consolas, 'Liberation Mono', monospace;
}

.operation-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 10px;
}

.department-form {
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

@media (width <= 1200px) {
  .summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (width <= 768px) {
  .department-page {
    margin: calc(-1 * var(--td-comp-paddingTB-l)) calc(-1 * var(--td-comp-paddingLR-l));
    padding: 12px;
  }

  .department-head,
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
    margin-right: 0;
  }
}
</style>
