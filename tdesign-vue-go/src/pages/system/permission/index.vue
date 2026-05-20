<template>
  <div class="permission-page system-management-page">
    <console-page-header>
      <template #title>
        <h2>权限管理</h2>
        <t-tag :theme="buttonPermissionCount > 0 ? 'success' : 'warning'" variant="light">
          {{ buttonPermissionCount > 0 ? '权限节点正常' : '等待按钮权限' }}
        </t-tag>
      </template>
      <template #meta>
        <span>权限树</span>
        <span>菜单权限</span>
        <span>按钮权限</span>
        <span>共 {{ totalPermissionCount }} 个节点</span>
        <span v-if="lastUpdatedAt">更新于 {{ lastUpdatedAt }}</span>
      </template>
      <template #actions>
        <t-tag theme="primary" variant="light">根节点 {{ rootPermissionCount }}</t-tag>
        <t-button theme="primary" @click="handleAdd">
          <template #icon><t-icon name="add" /></template>
          新增权限
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
            按权限名称、代码、路径、方法和节点类型筛选权限树
            <template v-if="activeFilterCount"> · 已应用 {{ activeFilterCount }} 个条件</template>
          </p>
        </div>
        <t-space size="small" break-line>
          <t-tag theme="primary" variant="light">显示 {{ visibleTableData.length }} 条</t-tag>
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
            placeholder="名称 / 代码 / 路径 / 描述"
            @enter="handleSearch"
          >
            <template #prefix-icon><t-icon name="search" /></template>
          </t-input>
        </t-form-item>
        <t-form-item label="类型" name="type">
          <t-select v-model="searchForm.type" clearable placeholder="全部类型" class="filter-select">
            <t-option :value="1" label="菜单" />
            <t-option :value="2" label="按钮" />
          </t-select>
        </t-form-item>
        <t-form-item label="方法" name="method">
          <t-select v-model="searchForm.method" clearable placeholder="全部方法" class="filter-select">
            <t-option v-for="method in methodOptions" :key="method" :label="method" :value="method" />
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
          <t-button variant="outline" @click="handleExpandAll">
            <template #icon><t-icon :name="isExpandAll ? 'menu-fold' : 'menu-unfold'" /></template>
            {{ isExpandAll ? '折叠所有' : '展开所有' }}
          </t-button>
        </t-space>
        <t-space size="small" break-line>
          <t-tag theme="primary" variant="light">层级 {{ maxDepth }}</t-tag>
          <t-tag :theme="methodCount > 0 ? 'success' : 'default'" variant="light">方法 {{ methodCount }}</t-tag>
        </t-space>
      </div>
    </t-card>

    <t-card :bordered="false" class="table-card">
      <div class="table-card__head">
        <div>
          <h3>权限结构</h3>
          <p>权限层级、权限代码、接口路径、HTTP 方法、排序和更新时间</p>
        </div>
        <t-space size="small" break-line>
          <t-tag theme="primary" variant="light">菜单 {{ menuPermissionCount }}</t-tag>
          <t-tag theme="success" variant="light">按钮 {{ buttonPermissionCount }}</t-tag>
        </t-space>
      </div>

      <t-table
        row-key="id"
        hover
        class="permission-table"
        table-layout="fixed"
        :data="visibleTableData"
        :columns="columns"
        :loading="loading"
      >
        <template #empty>
          <t-empty :description="loading ? '正在加载权限树' : '当前筛选条件下暂无权限节点'" />
        </template>
        <template #permission="{ row }">
          <div class="permission-cell" :style="{ paddingLeft: `${(row.__level || 0) * 20}px` }">
            <button
              v-if="row.children?.length"
              class="expand-button"
              type="button"
              :aria-label="row.__expanded ? '折叠权限' : '展开权限'"
              @click.stop="toggleExpand(row)"
            >
              <t-icon :name="row.__expanded ? 'chevron-down' : 'chevron-right'" />
            </button>
            <span v-else class="expand-placeholder" />
            <span class="permission-icon" :class="{ 'permission-icon--button': permissionTypeValue(row.type) === 2 }">
              <t-icon :name="permissionTypeValue(row.type) === 1 ? 'menu-application' : 'secured'" />
            </span>
            <div class="permission-cell__main">
              <strong>{{ row.name || '未命名权限' }}</strong>
              <span>{{ levelLabel(row) }} · {{ row.children?.length || 0 }} 个子节点</span>
            </div>
          </div>
        </template>
        <template #code="{ row }">
          <div class="code-cell">
            <strong class="mono-text" :title="row.code">{{ row.code || '-' }}</strong>
            <span>{{ parentLabel(row) }}</span>
          </div>
        </template>
        <template #type="{ row }">
          <t-tag :theme="permissionTypeValue(row.type) === 1 ? 'primary' : 'success'" variant="light">
            {{ permissionTypeLabel(row.type) }}
          </t-tag>
        </template>
        <template #path="{ row }">
          <div class="path-cell">
            <strong class="mono-text" :title="row.path">{{ row.path || '未配置路径' }}</strong>
            <span>{{ row.description || '暂无描述' }}</span>
          </div>
        </template>
        <template #method="{ row }">
          <t-tag v-if="row.method" :theme="methodTheme(row.method)" variant="light">{{ row.method }}</t-tag>
          <t-tag v-else theme="default" variant="light">未配置</t-tag>
        </template>
        <template #sort="{ row }">
          <span class="sort-badge">{{ row.sort ?? 0 }}</span>
        </template>
        <template #status="{ row }">
          <t-tag :theme="row.status === 0 ? 'default' : 'success'" variant="light">
            {{ row.status === 0 ? '停用' : '启用' }}
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
            <t-link theme="primary" hover="color" @click="handleViewDetail(row)">详情</t-link>
            <t-link theme="primary" hover="color" @click="handleEdit(row)">编辑</t-link>
            <t-link theme="primary" hover="color" @click="handleAddChild(row)">子级</t-link>
            <t-popconfirm content="确定删除该权限吗？" @confirm="handleDelete(row)">
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
      <t-form ref="formRef" :data="formData" :rules="formRules" label-width="92px" class="permission-form">
        <div class="form-grid">
          <t-form-item label="权限名称" name="name">
            <t-input v-model="formData.name" placeholder="请输入权限名称" />
          </t-form-item>
          <t-form-item label="权限代码" name="code">
            <t-input v-model="formData.code" :disabled="isEdit" placeholder="请输入权限代码" />
          </t-form-item>
          <t-form-item label="权限类型" name="type">
            <t-radio-group v-model="formData.type" variant="default-filled">
              <t-radio-button :value="1">菜单</t-radio-button>
              <t-radio-button :value="2">按钮</t-radio-button>
            </t-radio-group>
          </t-form-item>
          <t-form-item label="请求方法" name="method">
            <t-select v-model="formData.method" placeholder="请选择请求方法" clearable>
              <t-option value="GET">GET</t-option>
              <t-option value="POST">POST</t-option>
              <t-option value="PUT">PUT</t-option>
              <t-option value="DELETE">DELETE</t-option>
              <t-option value="PATCH">PATCH</t-option>
            </t-select>
          </t-form-item>
          <t-form-item class="form-grid__full" label="请求路径" name="path">
            <t-input v-model="formData.path" placeholder="请输入请求路径" />
          </t-form-item>
          <t-form-item class="form-grid__full" label="父级权限" name="parent_id">
            <t-cascader
              v-model="formData.parent_id"
              :options="permissionOptions"
              placeholder="请选择父级权限"
              check-strictly
              clearable
              :keys="{ value: 'id', label: 'name', children: 'children' }"
            />
          </t-form-item>
          <t-form-item label="排序" name="sort">
            <t-input-number v-model="formData.sort" :min="0" placeholder="请输入排序" style="width: 100%" />
          </t-form-item>
          <t-form-item class="form-grid__full" label="描述" name="description">
            <t-textarea v-model="formData.description" placeholder="请输入描述" :autosize="{ minRows: 3, maxRows: 5 }" />
          </t-form-item>
        </div>
      </t-form>
    </t-dialog>

    <t-drawer v-model:visible="detailVisible" :header="detailTitle" size="640px" :footer="false">
      <div v-if="currentPermission" class="detail-panel">
        <div class="detail-hero" :class="{ 'detail-hero--button': permissionTypeValue(currentPermission.type) === 2 }">
          <span class="detail-hero__icon">
            <t-icon :name="permissionTypeValue(currentPermission.type) === 1 ? 'menu-application' : 'secured'" />
          </span>
          <div>
            <strong>{{ currentPermission.name || '未命名权限' }}</strong>
            <span>{{ permissionTypeLabel(currentPermission.type) }} · {{ currentPermission.code || '-' }}</span>
          </div>
        </div>

        <t-descriptions bordered :column="1" class="detail-desc">
          <t-descriptions-item label="权限 ID">{{ currentPermission.id }}</t-descriptions-item>
          <t-descriptions-item label="权限名称">{{ currentPermission.name || '-' }}</t-descriptions-item>
          <t-descriptions-item label="权限代码">
            <span class="mono-text">{{ currentPermission.code || '-' }}</span>
          </t-descriptions-item>
          <t-descriptions-item label="类型">{{ permissionTypeLabel(currentPermission.type) }}</t-descriptions-item>
          <t-descriptions-item label="父级">{{ parentLabel(currentPermission) }}</t-descriptions-item>
          <t-descriptions-item label="请求路径">
            <span class="mono-text">{{ currentPermission.path || '-' }}</span>
          </t-descriptions-item>
          <t-descriptions-item label="请求方法">{{ currentPermission.method || '-' }}</t-descriptions-item>
          <t-descriptions-item label="排序">{{ currentPermission.sort ?? 0 }}</t-descriptions-item>
          <t-descriptions-item label="创建时间">{{ formatDateTime(currentPermission.created_at) }}</t-descriptions-item>
          <t-descriptions-item label="更新时间">{{ formatDateTime(currentPermission.updated_at) }}</t-descriptions-item>
        </t-descriptions>

        <section class="detail-section">
          <div class="detail-section__head">
            <span>权限描述</span>
            <t-tag theme="primary" variant="light">{{ currentPermission.description ? '已填写' : '未填写' }}</t-tag>
          </div>
          <p>{{ currentPermission.description || '暂无描述' }}</p>
        </section>
      </div>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { MessagePlugin } from 'tdesign-vue-next';
import { computed, onMounted, ref } from 'vue';

import {
  createPermission,
  deletePermission,
  getPermissionTree,
  updatePermission,
  type PermissionItem,
} from '@/api/system/permission';
import { formatDateTime } from '@/utils/date';
import ConsolePageHeader from '@/components/common/ConsolePageHeader.vue';

type SummaryTone = 'blue' | 'green' | 'cyan' | 'orange';
type TagTheme = 'default' | 'success' | 'primary' | 'warning' | 'danger';

interface PermissionItemUI extends PermissionItem {
  __expanded?: boolean;
  __level?: number;
  __visible?: boolean;
}

defineOptions({
  name: 'SystemPermission',
});

const loading = ref(false);
const submitLoading = ref(false);
const allFlatData = ref<PermissionItemUI[]>([]);
const permissionOptions = ref<PermissionItem[]>([]);
const dialogVisible = ref(false);
const detailVisible = ref(false);
const formRef = ref();
const isEdit = ref(false);
const currentPermission = ref<PermissionItemUI | null>(null);
const isExpandAll = ref(false);
const lastUpdatedAt = ref('');

const searchForm = ref({
  keyword: '',
  type: undefined as number | undefined,
  method: '',
});

const createDefaultFormData = () => ({
  name: '',
  code: '',
  type: 1,
  path: '',
  method: '',
  parent_id: 0,
  sort: 0,
  description: '',
});

const formData = ref(createDefaultFormData());

const formRules: any = {
  name: [{ required: true, message: '请输入权限名称' }],
  code: [{ required: true, message: '请输入权限代码' }],
  type: [{ required: true, message: '请选择权限类型' }],
};

const columns: any[] = [
  { colKey: 'permission', title: '权限节点', minWidth: 290, fixed: 'left' as const },
  { colKey: 'code', title: '权限代码 / 父级', minWidth: 230 },
  { colKey: 'type', title: '类型', width: 96 },
  { colKey: 'path', title: '路径 / 描述', minWidth: 260 },
  { colKey: 'method', title: '方法', width: 100 },
  { colKey: 'sort', title: '排序', width: 88 },
  { colKey: 'status', title: '状态', width: 96 },
  { colKey: 'updated_at', title: '创建 / 更新', width: 220 },
  { colKey: 'operation', title: '操作', width: 190, fixed: 'right' as const },
];

const isFiltering = computed(() => Boolean(searchForm.value.keyword.trim() || searchForm.value.type !== undefined || searchForm.value.method));
const visibleTableData = computed(() =>
  allFlatData.value.filter((item) => {
    const visibilityMatched = isFiltering.value ? true : item.__visible;
    return visibilityMatched && matchesSearch(item);
  }),
);
const totalPermissionCount = computed(() => allFlatData.value.length);
const rootPermissionCount = computed(() => allFlatData.value.filter((item) => (item.__level || 0) === 0).length);
const menuPermissionCount = computed(() => allFlatData.value.filter((item) => permissionTypeValue(item.type) === 1).length);
const buttonPermissionCount = computed(() => allFlatData.value.filter((item) => permissionTypeValue(item.type) === 2).length);
const methodOptions = computed(() =>
  Array.from(new Set(allFlatData.value.map((item) => item.method).filter(Boolean) as string[])).sort(),
);
const methodCount = computed(() => methodOptions.value.length);
const maxDepth = computed(() => Math.max(0, ...allFlatData.value.map((item) => (item.__level || 0) + 1)));
const activeFilterCount = computed(() => {
  let count = 0;
  if (searchForm.value.keyword.trim()) count += 1;
  if (searchForm.value.type !== undefined) count += 1;
  if (searchForm.value.method) count += 1;
  return count;
});

const summaryItems = computed<Array<{ label: string; value: string | number; hint: string; icon: string; tone: SummaryTone }>>(() => [
  {
    label: '权限总数',
    value: totalPermissionCount.value,
    hint: `根节点 ${rootPermissionCount.value} 个`,
    icon: 'secured',
    tone: 'blue',
  },
  {
    label: '菜单权限',
    value: menuPermissionCount.value,
    hint: '用于导航和页面访问',
    icon: 'menu-application',
    tone: 'green',
  },
  {
    label: '按钮权限',
    value: buttonPermissionCount.value,
    hint: '用于操作级权限控制',
    icon: 'check-circle',
    tone: 'cyan',
  },
  {
    label: '权限层级',
    value: maxDepth.value,
    hint: `当前显示 ${visibleTableData.value.length} 条`,
    icon: 'tree-square-dot',
    tone: 'orange',
  },
]);

const dialogTitle = computed(() => (isEdit.value ? '编辑权限' : '新增权限'));
const detailTitle = computed(() => (currentPermission.value ? `${currentPermission.value.name || currentPermission.value.code} · 权限详情` : '权限详情'));

const updateTime = () => {
  lastUpdatedAt.value = new Date().toLocaleTimeString('zh-CN', { hour12: false });
};

const permissionTypeValue = (type?: string | number) => Number(type || 0);

const permissionTypeLabel = (type?: string | number) => (permissionTypeValue(type) === 1 ? '菜单' : '按钮');

const methodTheme = (method?: string): TagTheme => {
  const value = (method || '').toUpperCase();
  if (value === 'GET') return 'success';
  if (value === 'POST') return 'primary';
  if (value === 'PUT' || value === 'PATCH') return 'warning';
  if (value === 'DELETE') return 'danger';
  return 'default';
};

const matchesSearch = (item: PermissionItemUI) => {
  const keyword = searchForm.value.keyword.trim().toLowerCase();
  const keywordMatched = keyword
    ? [item.name, item.code, item.path, item.method, item.description]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(keyword))
    : true;
  const typeMatched = searchForm.value.type === undefined ? true : permissionTypeValue(item.type) === searchForm.value.type;
  const methodMatched = searchForm.value.method ? item.method === searchForm.value.method : true;
  return keywordMatched && typeMatched && methodMatched;
};

const flattenTree = (nodes: PermissionItem[], level = 0, parentVisible = true): PermissionItemUI[] => {
  let result: PermissionItemUI[] = [];
  nodes.forEach((node) => {
    const uiNode: PermissionItemUI = {
      ...node,
      __expanded: false,
      __level: level,
      __visible: parentVisible,
    };
    result.push(uiNode);
    if (node.children?.length) {
      result = result.concat(flattenTree(node.children, level + 1, false));
    }
  });
  return result;
};

const updateChildrenVisibility = (node: PermissionItemUI, parentExpanded: boolean) => {
  if (!node.children?.length) return;

  node.children.forEach((childRaw) => {
    const child = allFlatData.value.find((item) => item.id === childRaw.id);
    if (!child) return;
    child.__visible = parentExpanded;
    updateChildrenVisibility(child, parentExpanded && Boolean(child.__expanded));
  });
};

const toggleExpand = (row: PermissionItemUI) => {
  row.__expanded = !row.__expanded;
  updateChildrenVisibility(row, Boolean(row.__expanded));
};

const handleExpandAll = () => {
  isExpandAll.value = !isExpandAll.value;
  const expand = isExpandAll.value;

  allFlatData.value.forEach((item) => {
    item.__expanded = expand;
    item.__visible = item.__level === 0 || expand;
  });
};

const loadData = async () => {
  loading.value = true;
  try {
    const res = await getPermissionTree();
    const treeData = JSON.parse(JSON.stringify(res || [])) as PermissionItem[];
    allFlatData.value = flattenTree(treeData, 0, true);
    permissionOptions.value = [
      {
        id: 0,
        name: '无（顶级权限）',
        code: 'root',
        type: 1,
        parent_id: 0,
        sort: 0,
        status: 1,
        created_at: '',
        updated_at: '',
        children: [],
      },
      ...treeData,
    ];
    isExpandAll.value = false;
    updateTime();
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载权限树数据失败');
  } finally {
    loading.value = false;
  }
};

const handleSearch = () => {
  // 本页使用本地树形数据筛选，输入变化会自动驱动列表更新。
};

const handleReset = () => {
  searchForm.value = {
    keyword: '',
    type: undefined,
    method: '',
  };
};

const handleAdd = () => {
  isEdit.value = false;
  currentPermission.value = null;
  formData.value = createDefaultFormData();
  dialogVisible.value = true;
};

const handleAddChild = (row: PermissionItemUI) => {
  isEdit.value = false;
  currentPermission.value = null;
  formData.value = {
    ...createDefaultFormData(),
    parent_id: row.id,
  };
  dialogVisible.value = true;
};

const handleEdit = (row: PermissionItemUI) => {
  isEdit.value = true;
  currentPermission.value = row;
  formData.value = {
    name: row.name,
    code: row.code,
    type: permissionTypeValue(row.type) || 1,
    path: row.path || '',
    method: row.method || '',
    parent_id: row.parent_id || 0,
    sort: row.sort || 0,
    description: row.description || '',
  };
  dialogVisible.value = true;
};

const handleViewDetail = (row: PermissionItemUI) => {
  currentPermission.value = row;
  detailVisible.value = true;
};

const handleSubmit = async () => {
  const valid = await formRef.value?.validate();
  if (valid !== true) return;

  submitLoading.value = true;
  try {
    if (isEdit.value && currentPermission.value) {
      await updatePermission(currentPermission.value.id, formData.value as any);
      MessagePlugin.success('更新成功');
    } else {
      await createPermission(formData.value as any);
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

const handleDelete = async (row: PermissionItemUI) => {
  try {
    await deletePermission(row.id);
    MessagePlugin.success('删除成功');
    loadData();
  } catch (error: any) {
    MessagePlugin.error(error.message || '删除失败');
  }
};

const handleRefresh = () => {
  loadData();
};

const parentLabel = (row: Pick<PermissionItem, 'parent_id'>) => {
  if (!row.parent_id) return '顶级权限';
  return allFlatData.value.find((item) => item.id === row.parent_id)?.name || `父级 ${row.parent_id}`;
};

const levelLabel = (row: PermissionItemUI) => `第 ${(row.__level || 0) + 1} 级`;

onMounted(() => {
  loadData();
});
</script>

<style lang="less" scoped>
.permission-page {
  --permission-bg: #f5f7fb;
  --permission-card-shadow: 0 12px 28px rgb(15 23 42 / 6%);
  --permission-font: 'Inter', 'HarmonyOS Sans SC', 'MiSans', 'PingFang SC', 'Microsoft YaHei UI', 'Microsoft YaHei', 'Arial', sans-serif;
  --permission-number-font: 'DIN Alternate', 'Bahnschrift', 'Inter', 'HarmonyOS Sans SC', 'Microsoft YaHei UI', sans-serif;

  display: flex;
  min-height: calc(100vh - 120px);
  min-width: 0;
  flex-direction: column;
  gap: 14px;
  margin: calc(-1 * var(--td-comp-paddingTB-xl)) calc(-1 * var(--td-comp-paddingLR-xl));
  padding: 14px 18px 18px;
  background: var(--permission-bg);
  color: var(--td-text-color-primary);
  font-family: var(--permission-font);
  font-feature-settings: 'tnum';
  font-variant-numeric: tabular-nums;
  letter-spacing: 0;
  text-rendering: optimizelegibility;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}

.permission-page :deep(.t-card),
.permission-page :deep(.t-table),
.permission-page :deep(.t-form),
.permission-page :deep(.t-button),
.permission-page :deep(.t-tag),
.permission-page :deep(.t-input),
.permission-page :deep(.t-select),
.permission-page :deep(.t-dialog),
.permission-page :deep(.t-drawer),
.permission-page :deep(.t-empty) {
  font-family: var(--permission-font);
}

.permission-head {
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

.permission-head__main {
  min-width: 0;
}

.permission-head__title {
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

.permission-head__meta {
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

.permission-head__actions {
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
    font-family: var(--permission-number-font);
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
  box-shadow: var(--permission-card-shadow);
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
  width: 320px;
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

.permission-table {
  width: 100%;
}

.permission-table :deep(.t-table__header th) {
  background: #f8fafc;
  color: #475569;
  font-size: 13px;
  font-weight: 700;
}

.permission-table :deep(.t-table__body tr:hover td) {
  background: #f8fbff;
}

.permission-table :deep(.t-table__body td) {
  padding-top: 14px;
  padding-bottom: 14px;
  border-bottom-color: #eef2f7;
  color: #1f2937;
  vertical-align: top;
}

.permission-cell {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
}

.expand-button,
.expand-placeholder {
  width: 24px;
  height: 24px;
  flex-shrink: 0;
}

.expand-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 8px;
  background: #eef6ff;
  color: #2563eb;
  cursor: pointer;
}

.expand-button:hover {
  background: #dbeafe;
}

.permission-icon {
  display: inline-flex;
  width: 34px;
  height: 34px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  background: linear-gradient(135deg, #dbeafe, #cffafe);
  color: #2563eb;
  font-size: 18px;
}

.permission-icon--button {
  background: linear-gradient(135deg, #dcfce7, #bbf7d0);
  color: #059669;
}

.permission-cell__main,
.code-cell,
.path-cell,
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

.mono-text {
  font-family: var(--permission-number-font);
  font-variant-numeric: tabular-nums;
}

.sort-badge {
  display: inline-flex;
  min-width: 32px;
  height: 24px;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  background: #f1f5f9;
  color: #334155;
  font-family: var(--permission-number-font);
  font-size: 12px;
  font-weight: 700;
}

.operation-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.permission-form {
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
  font-family: var(--permission-font);
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

.detail-hero--button {
  border-color: #bfdbfe;
  background: linear-gradient(135deg, #eff6ff, #dbeafe);

  strong {
    color: #1e3a8a;
  }

  span {
    color: #2563eb;
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
  .permission-page {
    margin: calc(-1 * var(--td-comp-paddingTB-l)) calc(-1 * var(--td-comp-paddingLR-l));
    padding: 12px;
  }

  .permission-head,
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
