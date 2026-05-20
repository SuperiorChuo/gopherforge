<template>
  <div class="menu-page system-management-page">
    <console-page-header>
      <template #title>
        <h2>菜单管理</h2>
        <t-tag :theme="disabledCount > 0 ? 'warning' : 'success'" variant="light">
          {{ disabledCount > 0 ? '存在禁用菜单' : '导航状态正常' }}
        </t-tag>
      </template>
      <template #meta>
        <span>导航结构</span>
        <span>路由组件</span>
        <span>菜单 Meta</span>
        <span>共 {{ totalMenus }} 个菜单</span>
        <span v-if="lastUpdatedAt">更新于 {{ lastUpdatedAt }}</span>
      </template>
      <template #actions>
        <t-tag theme="primary" variant="light">根菜单 {{ rootMenuCount }}</t-tag>
        <t-button theme="primary" @click="handleAdd">
          <template #icon><t-icon name="add" /></template>
          新增菜单
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
            按菜单标题、路由名称、路径、组件和状态定位导航节点
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
        <t-form-item label="关键字" name="keyword">
          <t-input
            v-model="searchForm.keyword"
            clearable
            class="keyword-input"
            placeholder="标题 / 路由 / 路径 / 组件"
            @enter="handleSearch"
          />
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
          <t-button variant="outline" @click="handleExpandAll">
            <template #icon><t-icon :name="isExpandAll ? 'menu-fold' : 'menu-unfold'" /></template>
            {{ isExpandAll ? '折叠所有' : '展开所有' }}
          </t-button>
        </t-space>
        <t-space size="small" break-line>
          <t-tag theme="primary" variant="light">最大层级 {{ maxDepth }}</t-tag>
          <t-tag :theme="hiddenCount > 0 ? 'warning' : 'success'" variant="light">隐藏 {{ hiddenCount }}</t-tag>
        </t-space>
      </div>
    </t-card>

    <t-card :bordered="false" class="table-card">
      <div class="table-card__head">
        <div>
          <h3>菜单结构</h3>
          <p>导航层级、路由路径、组件入口、图标、排序和 Meta 配置</p>
        </div>
        <t-space size="small">
          <t-tag :theme="enabledCount > 0 ? 'success' : 'default'" variant="light">启用 {{ enabledCount }}</t-tag>
          <t-tag theme="primary" variant="light">缓存 {{ keepAliveCount }}</t-tag>
        </t-space>
      </div>

      <t-table
        row-key="id"
        hover
        class="menu-table"
        table-layout="fixed"
        :data="visibleTableData"
        :columns="columns"
        :loading="loading"
      >
        <template #empty>
          <t-empty :description="loading ? '正在加载菜单结构' : '当前筛选条件下暂无菜单'" />
        </template>
        <template #title="{ row }">
          <div class="menu-title-cell" :style="{ paddingLeft: `${(row.__level || 0) * 20}px` }">
            <button
              v-if="row.children?.length"
              class="expand-button"
              type="button"
              :aria-label="row.__expanded ? '折叠菜单' : '展开菜单'"
              @click.stop="toggleExpand(row)"
            >
              <t-icon :name="row.__expanded ? 'chevron-down' : 'chevron-right'" />
            </button>
            <span v-else class="expand-placeholder" />
            <span class="menu-icon-preview">
              <t-icon v-if="row.icon" :name="row.icon" />
              <t-icon v-else name="menu" />
            </span>
            <div class="menu-title-cell__main">
              <strong>{{ row.title || row.name || '-' }}</strong>
              <span>{{ levelLabel(row) }} · {{ row.children?.length || 0 }} 个子菜单</span>
            </div>
          </div>
        </template>
        <template #route="{ row }">
          <div class="route-cell">
            <strong class="mono-text" :title="row.path">{{ row.path || '-' }}</strong>
            <span class="mono-text" :title="row.name">name: {{ row.name || '-' }}</span>
          </div>
        </template>
        <template #component="{ row }">
          <div class="component-cell">
            <strong class="mono-text" :title="row.component">{{ row.component || '未配置组件' }}</strong>
            <span>{{ parentLabel(row) }}</span>
          </div>
        </template>
        <template #meta="{ row }">
          <t-space size="4px" break-line>
            <t-tag :theme="row.meta?.hidden ? 'warning' : 'success'" variant="light" size="small">
              {{ row.meta?.hidden ? '隐藏' : '展示' }}
            </t-tag>
            <t-tag :theme="row.meta?.keepAlive ? 'primary' : 'default'" variant="light" size="small">
              {{ row.meta?.keepAlive ? '缓存' : '不缓存' }}
            </t-tag>
          </t-space>
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
            <t-link theme="primary" hover="color" @click="handleViewDetail(row)">详情</t-link>
            <t-link theme="primary" hover="color" @click="handleEdit(row)">编辑</t-link>
            <t-link theme="primary" hover="color" @click="handleAddChild(row)">子级</t-link>
            <t-popconfirm content="确定删除该菜单吗？" @confirm="handleDelete(row)">
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
      <t-form ref="formRef" :data="formData" :rules="formRules" label-width="92px" class="menu-form">
        <div class="form-grid">
          <t-form-item label="菜单标题" name="title">
            <t-input v-model="formData.title" placeholder="请输入菜单标题" />
          </t-form-item>
          <t-form-item label="路由名称" name="name">
            <t-input v-model="formData.name" placeholder="英文唯一标识" />
          </t-form-item>
          <t-form-item label="路由路径" name="path">
            <t-input v-model="formData.path" placeholder="/system/menu" />
          </t-form-item>
          <t-form-item label="组件路径" name="component">
            <t-input v-model="formData.component" placeholder="@/pages/system/menu/index.vue" />
          </t-form-item>
          <t-form-item label="图标" name="icon">
            <icon-picker v-model="formData.icon" />
          </t-form-item>
          <t-form-item label="父级菜单" name="parent_id">
            <t-cascader
              v-model="formData.parent_id"
              :options="menuOptions"
              placeholder="请选择父级菜单"
              check-strictly
              clearable
              :keys="{ value: 'id', label: 'title', children: 'children' }"
            />
          </t-form-item>
          <t-form-item label="排序" name="sort">
            <t-input-number v-model="formData.sort" :min="0" placeholder="排序值" />
          </t-form-item>
          <t-form-item label="状态" name="status">
            <t-radio-group v-model="formData.status" variant="default-filled">
              <t-radio-button :value="1">启用</t-radio-button>
              <t-radio-button :value="0">禁用</t-radio-button>
            </t-radio-group>
          </t-form-item>
          <t-form-item label="显示" name="hidden">
            <t-radio-group v-model="formData.hidden" variant="default-filled">
              <t-radio-button :value="false">展示</t-radio-button>
              <t-radio-button :value="true">隐藏</t-radio-button>
            </t-radio-group>
          </t-form-item>
          <t-form-item label="缓存" name="keepAlive">
            <t-radio-group v-model="formData.keepAlive" variant="default-filled">
              <t-radio-button :value="true">缓存</t-radio-button>
              <t-radio-button :value="false">不缓存</t-radio-button>
            </t-radio-group>
          </t-form-item>
        </div>
      </t-form>
    </t-dialog>

    <t-drawer v-model:visible="detailVisible" :header="detailTitle" size="640px" :footer="false">
      <t-loading :loading="detailLoading" size="small">
        <div v-if="currentMenu" class="detail-panel">
          <div class="detail-hero" :class="{ 'detail-hero--disabled': currentMenu.status !== 1 }">
            <span class="detail-hero__icon">
              <t-icon v-if="currentMenu.icon" :name="currentMenu.icon" />
              <t-icon v-else name="menu" />
            </span>
            <div>
              <strong>{{ currentMenu.title || currentMenu.name || '未命名菜单' }}</strong>
              <span>{{ currentMenu.status === 1 ? '菜单启用中' : '菜单已禁用' }}</span>
            </div>
          </div>

          <t-descriptions bordered :column="1" class="detail-desc">
            <t-descriptions-item label="菜单 ID">{{ currentMenu.id }}</t-descriptions-item>
            <t-descriptions-item label="菜单标题">{{ currentMenu.title || '-' }}</t-descriptions-item>
            <t-descriptions-item label="路由名称">{{ currentMenu.name || '-' }}</t-descriptions-item>
            <t-descriptions-item label="路由路径">
              <span class="mono-text">{{ currentMenu.path || '-' }}</span>
            </t-descriptions-item>
            <t-descriptions-item label="组件路径">
              <span class="mono-text">{{ currentMenu.component || '-' }}</span>
            </t-descriptions-item>
            <t-descriptions-item label="父级菜单">{{ parentLabel(currentMenu) }}</t-descriptions-item>
            <t-descriptions-item label="排序">{{ currentMenu.sort ?? 0 }}</t-descriptions-item>
            <t-descriptions-item label="状态">{{ currentMenu.status === 1 ? '启用' : '禁用' }}</t-descriptions-item>
            <t-descriptions-item label="创建时间">{{ formatDateTime(currentMenu.created_at) }}</t-descriptions-item>
            <t-descriptions-item label="更新时间">{{ formatDateTime(currentMenu.updated_at) }}</t-descriptions-item>
          </t-descriptions>

          <section class="detail-section">
            <div class="detail-section__head">
              <span>Meta 配置</span>
              <t-tag theme="primary" variant="light">{{ currentMenu.meta ? '已配置' : '未配置' }}</t-tag>
            </div>
            <pre>{{ formatMeta(currentMenu.meta) }}</pre>
          </section>
        </div>
      </t-loading>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { MessagePlugin } from 'tdesign-vue-next';
import { computed, onMounted, ref } from 'vue';

import { createMenu, deleteMenu, getMenu, getMenuTree, updateMenu, type MenuItem } from '@/api/system/menu';
import IconPicker from '@/components/icon-picker/index.vue';
import { formatDateTime } from '@/utils/date';
import ConsolePageHeader from '@/components/common/ConsolePageHeader.vue';

type SummaryTone = 'blue' | 'green' | 'cyan' | 'orange';

interface MenuItemUI extends MenuItem {
  __expanded?: boolean;
  __level?: number;
  __visible?: boolean;
}

interface MenuFormData {
  component: string;
  hidden: boolean;
  icon: string;
  keepAlive: boolean;
  name: string;
  parent_id: number;
  path: string;
  sort: number;
  status: number;
  title: string;
}

defineOptions({
  name: 'SystemMenu',
});

const loading = ref(false);
const submitLoading = ref(false);
const detailLoading = ref(false);
const allFlatData = ref<MenuItemUI[]>([]);
const menuOptions = ref<MenuItem[]>([]);
const dialogVisible = ref(false);
const detailVisible = ref(false);
const formRef = ref();
const isEdit = ref(false);
const currentMenu = ref<MenuItemUI | null>(null);
const isExpandAll = ref(false);
const lastUpdatedAt = ref('');

const searchForm = ref({
  keyword: '',
  status: undefined as number | undefined,
});

const formData = ref<MenuFormData>(createDefaultFormData());

const formRules: any = {
  name: [{ required: true, message: '请输入路由名称' }],
  title: [{ required: true, message: '请输入菜单标题' }],
  path: [{ required: true, message: '请输入路由路径' }],
};

const columns: any[] = [
  { colKey: 'title', title: '菜单标题', minWidth: 280, fixed: 'left' as const },
  { colKey: 'route', title: '路由', minWidth: 230 },
  { colKey: 'component', title: '组件 / 父级', minWidth: 260 },
  { colKey: 'meta', title: 'Meta', width: 140 },
  { colKey: 'sort', title: '排序', width: 88 },
  { colKey: 'status', title: '状态', width: 96 },
  { colKey: 'updated_at', title: '更新时间', width: 210 },
  { colKey: 'operation', title: '操作', width: 180, fixed: 'right' as const },
];

const isFiltering = computed(() => Boolean(searchForm.value.keyword.trim() || searchForm.value.status !== undefined));
const visibleTableData = computed(() =>
  allFlatData.value.filter((item) => {
    const visibilityMatched = isFiltering.value ? true : item.__visible;
    return visibilityMatched && matchesSearch(item);
  }),
);

const totalMenus = computed(() => allFlatData.value.length);
const rootMenuCount = computed(() => allFlatData.value.filter((item) => (item.__level || 0) === 0).length);
const enabledCount = computed(() => allFlatData.value.filter((item) => item.status === 1).length);
const disabledCount = computed(() => allFlatData.value.filter((item) => item.status !== 1).length);
const hiddenCount = computed(() => allFlatData.value.filter((item) => item.meta?.hidden).length);
const keepAliveCount = computed(() => allFlatData.value.filter((item) => item.meta?.keepAlive).length);
const maxDepth = computed(() => Math.max(0, ...allFlatData.value.map((item) => (item.__level || 0) + 1)));
const activeFilterCount = computed(() => {
  let count = 0;
  if (searchForm.value.keyword.trim()) count += 1;
  if (searchForm.value.status !== undefined) count += 1;
  return count;
});

const summaryItems = computed<Array<{ label: string; value: string | number; hint: string; icon: string; tone: SummaryTone }>>(() => [
  {
    label: '菜单总数',
    value: totalMenus.value,
    hint: `根菜单 ${rootMenuCount.value} 个`,
    icon: 'menu-application',
    tone: 'blue',
  },
  {
    label: '启用菜单',
    value: enabledCount.value,
    hint: `禁用 ${disabledCount.value} 个`,
    icon: 'check-circle',
    tone: 'green',
  },
  {
    label: '菜单层级',
    value: maxDepth.value,
    hint: `当前显示 ${visibleTableData.value.length} 条`,
    icon: 'tree-square-dot',
    tone: 'cyan',
  },
  {
    label: 'Meta 标记',
    value: keepAliveCount.value,
    hint: `隐藏 ${hiddenCount.value} 个菜单`,
    icon: 'component-layout',
    tone: hiddenCount.value > 0 ? 'orange' : 'cyan',
  },
]);

const dialogTitle = computed(() => (isEdit.value ? '编辑菜单' : '新增菜单'));
const detailTitle = computed(() => (currentMenu.value ? `${currentMenu.value.title || currentMenu.value.name} · 菜单详情` : '菜单详情'));

function createDefaultFormData(): MenuFormData {
  return {
    name: '',
    title: '',
    path: '',
    component: '',
    icon: '',
    parent_id: 0,
    sort: 0,
    status: 1,
    hidden: false,
    keepAlive: false,
  };
}

const updateTime = () => {
  lastUpdatedAt.value = new Date().toLocaleTimeString('zh-CN', { hour12: false });
};

const matchesSearch = (item: MenuItemUI) => {
  const keyword = searchForm.value.keyword.trim().toLowerCase();
  const keywordMatched = keyword
    ? [item.title, item.name, item.path, item.component, item.icon]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(keyword))
    : true;
  const statusMatched = searchForm.value.status === undefined ? true : item.status === searchForm.value.status;
  return keywordMatched && statusMatched;
};

const flattenTree = (nodes: MenuItem[], level = 0, parentVisible = true): MenuItemUI[] => {
  let result: MenuItemUI[] = [];
  nodes.forEach((node) => {
    const uiNode: MenuItemUI = {
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

const updateChildrenVisibility = (node: MenuItemUI, parentExpanded: boolean) => {
  if (!node.children?.length) return;

  node.children.forEach((childRaw) => {
    const child = allFlatData.value.find((item) => item.id === childRaw.id);
    if (!child) return;
    child.__visible = parentExpanded;
    updateChildrenVisibility(child, parentExpanded && Boolean(child.__expanded));
  });
};

const toggleExpand = (row: MenuItemUI) => {
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
    const res = await getMenuTree();
    const treeData = JSON.parse(JSON.stringify(res || [])) as MenuItem[];
    allFlatData.value = flattenTree(treeData, 0, true);
    menuOptions.value = [
      {
        id: 0,
        name: 'root',
        title: '无（顶级菜单）',
        path: '',
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
    MessagePlugin.error(error.message || '加载菜单数据失败');
    console.error('加载菜单数据失败:', error);
  } finally {
    loading.value = false;
  }
};

const handleAdd = () => {
  isEdit.value = false;
  currentMenu.value = null;
  formData.value = createDefaultFormData();
  dialogVisible.value = true;
};

const handleAddChild = (row: MenuItemUI) => {
  isEdit.value = false;
  currentMenu.value = null;
  formData.value = {
    ...createDefaultFormData(),
    parent_id: row.id,
  };
  dialogVisible.value = true;
};

const handleEdit = (row: MenuItemUI) => {
  isEdit.value = true;
  currentMenu.value = row;
  formData.value = {
    name: row.name,
    title: row.title || '',
    path: row.path,
    component: row.component || '',
    icon: row.icon || '',
    parent_id: row.parent_id || 0,
    sort: row.sort || 0,
    status: row.status,
    hidden: Boolean(row.meta?.hidden),
    keepAlive: Boolean(row.meta?.keepAlive),
  };
  dialogVisible.value = true;
};

const buildMenuPayload = () => ({
  name: formData.value.name,
  title: formData.value.title,
  path: formData.value.path,
  component: formData.value.component,
  icon: formData.value.icon,
  parent_id: formData.value.parent_id,
  sort: formData.value.sort,
  status: formData.value.status,
  meta: {
    hidden: formData.value.hidden,
    icon: formData.value.icon,
    keepAlive: formData.value.keepAlive,
    title: formData.value.title,
  },
});

const handleSubmit = async () => {
  const valid = await formRef.value?.validate();
  if (!valid) return;

  submitLoading.value = true;
  try {
    const payload = buildMenuPayload();
    if (isEdit.value && currentMenu.value) {
      await updateMenu(currentMenu.value.id, payload);
      MessagePlugin.success('菜单已更新');
    } else {
      await createMenu(payload);
      MessagePlugin.success('菜单已创建');
    }
    dialogVisible.value = false;
    loadData();
  } catch (error: any) {
    MessagePlugin.error(error.message || '操作失败');
  } finally {
    submitLoading.value = false;
  }
};

const handleDelete = async (row: MenuItemUI) => {
  try {
    await deleteMenu(row.id);
    MessagePlugin.success('菜单已删除');
    loadData();
  } catch (error: any) {
    MessagePlugin.error(error.message || '删除失败');
  }
};

const handleViewDetail = async (row: MenuItemUI) => {
  currentMenu.value = row;
  detailVisible.value = true;
  detailLoading.value = true;
  try {
    currentMenu.value = {
      ...row,
      ...(await getMenu(row.id)),
      __expanded: row.__expanded,
      __level: row.__level,
      __visible: row.__visible,
    };
  } catch (error) {
    console.error('加载菜单详情失败:', error);
  } finally {
    detailLoading.value = false;
  }
};

const handleSearch = () => {
  if (isFiltering.value) return;
  loadData();
};

const handleReset = () => {
  searchForm.value = {
    keyword: '',
    status: undefined,
  };
};

const handleRefresh = () => {
  loadData();
};

const parentLabel = (row: MenuItem) => {
  if (!row.parent_id) return '顶级菜单';
  return allFlatData.value.find((item) => item.id === row.parent_id)?.title || `父级 ${row.parent_id}`;
};

const levelLabel = (row: MenuItemUI) => `第 ${(row.__level || 0) + 1} 级`;

const formatMeta = (meta?: MenuItem['meta']) => {
  if (!meta) return '暂无 Meta 配置';
  return JSON.stringify(meta, null, 2);
};

onMounted(() => {
  loadData();
});
</script>

<style lang="less" scoped>
.menu-page {
  --menu-bg: #f5f7fb;
  --menu-card-shadow: 0 12px 28px rgb(15 23 42 / 6%);
  --menu-font: 'Inter', 'HarmonyOS Sans SC', 'MiSans', 'PingFang SC', 'Microsoft YaHei UI', 'Microsoft YaHei', 'Arial', sans-serif;
  --menu-number-font: 'DIN Alternate', 'Bahnschrift', 'Inter', 'HarmonyOS Sans SC', 'Microsoft YaHei UI', sans-serif;

  display: flex;
  min-height: calc(100vh - 120px);
  min-width: 0;
  flex-direction: column;
  gap: 14px;
  margin: calc(-1 * var(--td-comp-paddingTB-xl)) calc(-1 * var(--td-comp-paddingLR-xl));
  padding: 14px 18px 18px;
  background: var(--menu-bg);
  color: var(--td-text-color-primary);
  font-family: var(--menu-font);
  font-feature-settings: 'tnum';
  font-variant-numeric: tabular-nums;
  letter-spacing: 0;
  text-rendering: optimizelegibility;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}

.menu-page :deep(.t-card),
.menu-page :deep(.t-table),
.menu-page :deep(.t-form),
.menu-page :deep(.t-button),
.menu-page :deep(.t-tag),
.menu-page :deep(.t-input),
.menu-page :deep(.t-select),
.menu-page :deep(.t-dialog),
.menu-page :deep(.t-drawer),
.menu-page :deep(.t-empty) {
  font-family: var(--menu-font);
}

.menu-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--td-comp-margin-l);
  padding: 10px 12px;
  border: 1px solid #e8edf5;
  border-radius: 12px;
  background:
    radial-gradient(circle at 18% 0%, rgb(37 99 235 / 10%), transparent 28%),
    radial-gradient(circle at 92% 16%, rgb(14 165 233 / 12%), transparent 26%),
    #fff;
  box-shadow: 0 10px 24px rgb(15 23 42 / 5%);
}

.menu-head__main {
  min-width: 0;
}

.menu-head__title {
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

.menu-head__meta {
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

.menu-head__actions {
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
    font-family: var(--menu-number-font);
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
  box-shadow: var(--menu-card-shadow);
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

.menu-table {
  width: 100%;
}

.menu-table :deep(.t-table__header th) {
  background: #f8fafc;
  color: #475569;
  font-size: 13px;
  font-weight: 700;
}

.menu-table :deep(.t-table__body tr:hover td) {
  background: #f8fbff;
}

.menu-table :deep(.t-table__body td) {
  padding-top: 14px;
  padding-bottom: 14px;
  border-bottom-color: #eef2f7;
  color: #1f2937;
  vertical-align: top;
}

.menu-title-cell {
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

.menu-icon-preview {
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

.menu-title-cell__main,
.route-cell,
.component-cell,
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
  font-family: var(--menu-number-font);
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
  font-family: var(--menu-number-font);
  font-size: 12px;
  font-weight: 700;
}

.operation-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.menu-form {
  padding-top: 4px;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 14px;
}

.detail-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
  font-family: var(--menu-font);
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

.detail-hero--disabled {
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
  width: 104px;
  color: #64748b;
  font-weight: 600;
}

.detail-section {
  overflow: hidden;
  border: 1px solid #e8edf5;
  border-radius: 12px;
  background: #0f172a;
}

.detail-section__head {
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

.detail-section pre {
  max-height: 280px;
  margin: 0;
  overflow: auto;
  padding: 12px;
  color: #dbeafe;
  font-family: var(--menu-number-font);
  font-size: 12px;
  line-height: 20px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

@media (width <= 1200px) {
  .summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (width <= 768px) {
  .menu-page {
    margin: calc(-1 * var(--td-comp-paddingTB-l)) calc(-1 * var(--td-comp-paddingLR-l));
    padding: 12px;
  }

  .menu-head,
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
