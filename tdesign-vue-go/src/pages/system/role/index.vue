<template>
  <div class="role-page system-management-page">
    <console-page-header>
      <template #title>
        <h2>角色管理</h2>
        <t-tag :theme="disabledCount > 0 ? 'warning' : 'success'" variant="light">
          {{ disabledCount > 0 ? '存在停用角色' : '角色状态正常' }}
        </t-tag>
      </template>
      <template #meta>
        <span>权限角色</span>
        <span>数据范围</span>
        <span>权限矩阵</span>
        <span>共 {{ pagination.total }} 个角色</span>
        <span v-if="lastUpdatedAt">更新于 {{ lastUpdatedAt }}</span>
      </template>
      <template #actions>
        <t-tag theme="primary" variant="light">当前页 {{ tableData.length }} 条</t-tag>
        <t-button v-permission="'system:role:create'" theme="primary" @click="handleAdd">
          <template #icon><t-icon name="add" /></template>
          新增角色
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
            按角色名称、角色代码和描述查询权限角色
            <template v-if="activeFilterCount"> · 已应用 {{ activeFilterCount }} 个条件</template>
          </p>
        </div>
        <t-space size="small" break-line>
          <t-tag theme="primary" variant="light">权限覆盖 {{ permissionCoverage }}</t-tag>
          <t-tag :theme="broadScopeCount > 0 ? 'warning' : 'success'" variant="light">高范围 {{ broadScopeCount }}</t-tag>
        </t-space>
      </div>
      <t-form :data="searchForm" class="filter-form" layout="inline" @submit="handleSearch">
        <t-form-item label="关键字" name="keyword">
          <t-input
            v-model="searchForm.keyword"
            clearable
            class="keyword-input"
            placeholder="角色名称 / 代码 / 描述"
            @enter="handleSearch"
          />
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
          <t-tag theme="primary" variant="light">数据范围 {{ dataScopeTypeCount }}</t-tag>
          <t-tag :theme="noPermissionCount > 0 ? 'warning' : 'success'" variant="light">
            未分配权限 {{ noPermissionCount }}
          </t-tag>
        </t-space>
      </div>
    </t-card>

    <t-card :bordered="false" class="table-card">
      <div class="table-card__head">
        <div>
          <h3>角色列表</h3>
          <p>
            角色标识、数据范围、权限数量、描述和最近更新
            <template v-if="pagination.total"> · 共 {{ pagination.total }} 条</template>
          </p>
        </div>
        <t-space size="small">
          <t-tag :theme="enabledCount > 0 ? 'success' : 'default'" variant="light">启用 {{ enabledCount }}</t-tag>
          <t-tag theme="primary" variant="light">自定义范围 {{ customScopeCount }}</t-tag>
        </t-space>
      </div>

      <t-table
        row-key="id"
        hover
        class="role-table"
        table-layout="fixed"
        :data="tableData"
        :columns="columns"
        :loading="loading"
        :pagination="pagination"
        @page-change="handlePageChange"
        @page-size-change="handlePageSizeChange"
      >
        <template #empty>
          <console-state
            :type="loading ? 'loading' : 'empty'"
            size="compact"
            :title="loading ? '正在加载角色数据' : '暂无角色'"
            :description="loading ? '正在同步权限角色，请稍候。' : '当前筛选条件下暂无角色，可以重置条件或新增角色。'"
          />
        </template>
        <template #role="{ row }">
          <div class="role-cell">
            <span class="role-avatar">{{ roleInitial(row) }}</span>
            <div class="role-cell__main">
              <strong>{{ row.name || '未命名角色' }}</strong>
              <span class="mono-text">{{ row.code || '-' }} · ID {{ row.id }}</span>
            </div>
          </div>
        </template>
        <template #data_scope="{ row }">
          <div class="scope-cell">
            <t-tag :theme="dataScopeTheme(row.data_scope)" variant="light">
              {{ getDataScopeLabel(row.data_scope) }}
            </t-tag>
            <span>{{ dataScopeHint(row) }}</span>
          </div>
        </template>
        <template #permissions="{ row }">
          <div class="permission-cell">
            <strong>{{ row.permissions?.length || 0 }}</strong>
            <span>{{ row.permissions?.length ? '已分配权限' : '暂未分配权限' }}</span>
          </div>
        </template>
        <template #status="{ row }">
          <t-tag :theme="row.status === 1 ? 'success' : 'default'" variant="light">
            {{ row.status === 1 ? '启用' : '停用' }}
          </t-tag>
        </template>
        <template #description="{ row }">
          <span class="description-text" :title="row.description">{{ row.description || '暂无描述' }}</span>
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
            <t-link v-permission="'system:role:update'" theme="primary" hover="color" @click="handleEdit(row)">
              编辑
            </t-link>
            <t-link v-permission="'system:role:update'" theme="primary" hover="color" @click="handleAssignPermissions(row)">
              权限
            </t-link>
            <t-popconfirm content="确定删除该角色吗？" @confirm="handleDelete(row)">
              <t-link v-permission="'system:role:delete'" theme="danger" hover="color">删除</t-link>
            </t-popconfirm>
          </div>
        </template>
      </t-table>
    </t-card>

    <t-dialog v-model:visible="dialogVisible" :header="dialogTitle" width="760px" @confirm="handleSubmit">
      <t-form ref="formRef" :data="formData" :rules="formRules" label-width="92px" class="role-form">
        <div class="form-grid">
          <t-form-item label="角色名称" name="name">
            <t-input v-model="formData.name" placeholder="请输入角色名称" />
          </t-form-item>
          <t-form-item label="角色代码" name="code">
            <t-input v-model="formData.code" :disabled="isEdit" placeholder="请输入角色代码" />
          </t-form-item>
          <t-form-item class="form-grid__full" label="描述" name="description">
            <t-textarea v-model="formData.description" placeholder="请输入描述" :autosize="{ minRows: 3, maxRows: 5 }" />
          </t-form-item>
          <t-form-item class="form-grid__full" label="数据范围" name="data_scope">
            <t-radio-group v-model="formData.data_scope" class="scope-radio-group" variant="default-filled">
              <t-radio-button v-for="option in dataScopeOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </t-radio-button>
            </t-radio-group>
          </t-form-item>
          <t-form-item
            v-if="formData.data_scope === 'custom'"
            class="form-grid__full"
            label="自定义部门"
            name="data_scope_department_ids"
          >
            <div class="tree-box">
              <t-tree
                v-model="formData.data_scope_department_ids"
                :data="departmentTree"
                checkable
                expand-all
                :keys="{ value: 'id', label: 'name', children: 'children' }"
              />
            </div>
          </t-form-item>
        </div>
      </t-form>
    </t-dialog>

    <t-dialog
      v-model:visible="permissionDialogVisible"
      :header="permissionDialogTitle"
      width="760px"
      @confirm="handleAssignPermissionsSubmit"
    >
      <div class="permission-dialog">
        <div v-if="currentRole" class="permission-role">
          <span class="permission-role__avatar">{{ roleInitial(currentRole) }}</span>
          <div>
            <strong>{{ currentRole.name }}</strong>
            <span>{{ currentRole.code }} · 已选择 {{ selectedPermissionIds.length }} 个权限</span>
          </div>
          <t-tag :theme="dataScopeTheme(currentRole.data_scope)" variant="light">
            {{ getDataScopeLabel(currentRole.data_scope) }}
          </t-tag>
        </div>
        <div class="permission-tools">
          <t-input v-model="permissionKeyword" clearable placeholder="筛选权限名称 / 代码 / 路径" />
          <t-tag theme="primary" variant="light">权限节点 {{ flattenedPermissionCount }}</t-tag>
        </div>
        <div class="tree-box permission-tree-box">
          <t-tree
            v-model="selectedPermissionIds"
            :data="filteredPermissionTree"
            checkable
            expand-all
            :keys="{ value: 'id', label: 'name', children: 'children' }"
          />
        </div>
      </div>
    </t-dialog>

    <t-drawer v-model:visible="detailVisible" :header="detailTitle" size="680px" :footer="false">
      <t-loading :loading="detailLoading" size="small">
        <div v-if="currentRole" class="detail-panel">
          <div class="detail-hero" :class="{ 'detail-hero--disabled': currentRole.status !== 1 }">
            <span class="detail-hero__icon">
              <t-icon :name="currentRole.status === 1 ? 'user-safety' : 'user-blocked'" />
            </span>
            <div>
              <strong>{{ currentRole.name || '未命名角色' }}</strong>
              <span>{{ currentRole.status === 1 ? '角色启用中' : '角色已停用' }}</span>
            </div>
          </div>

          <t-descriptions bordered :column="1" class="detail-desc">
            <t-descriptions-item label="角色 ID">{{ currentRole.id }}</t-descriptions-item>
            <t-descriptions-item label="角色名称">{{ currentRole.name || '-' }}</t-descriptions-item>
            <t-descriptions-item label="角色代码">
              <span class="mono-text">{{ currentRole.code || '-' }}</span>
            </t-descriptions-item>
            <t-descriptions-item label="数据范围">{{ getDataScopeLabel(currentRole.data_scope) }}</t-descriptions-item>
            <t-descriptions-item label="自定义部门">{{ customDepartmentText(currentRole) }}</t-descriptions-item>
            <t-descriptions-item label="状态">{{ currentRole.status === 1 ? '启用' : '停用' }}</t-descriptions-item>
            <t-descriptions-item label="创建时间">{{ formatDateTime(currentRole.created_at) }}</t-descriptions-item>
            <t-descriptions-item label="更新时间">{{ formatDateTime(currentRole.updated_at) }}</t-descriptions-item>
            <t-descriptions-item label="描述">{{ currentRole.description || '暂无描述' }}</t-descriptions-item>
          </t-descriptions>

          <section class="detail-section">
            <div class="detail-section__head">
              <span>权限节点</span>
              <t-tag theme="primary" variant="light">{{ currentRole.permissions?.length || 0 }} 个权限</t-tag>
            </div>
            <div v-if="currentRole.permissions?.length" class="permission-id-list">
              <t-tag v-for="permission in currentRole.permissions" :key="permission.id" theme="primary" variant="light">
                #{{ permission.id }}
              </t-tag>
            </div>
            <console-state
              v-else
              type="permission"
              size="compact"
              title="暂未分配权限"
              description="该角色当前没有权限节点，可以在权限弹窗中勾选后保存。"
            />
          </section>
        </div>
      </t-loading>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { MessagePlugin } from 'tdesign-vue-next';
import { computed, onMounted, ref } from 'vue';

import { getDepartmentTree, type DepartmentItem } from '@/api/system/department';
import { getPermissionTree, type PermissionItem } from '@/api/system/permission';
import {
  assignPermissions,
  createRole,
  deleteRole,
  getRole,
  getRoleList,
  updateRole,
  type RoleDataScope,
  type RoleItem,
} from '@/api/system/role';
import { formatDateTime } from '@/utils/date';
import ConsolePageHeader from '@/components/common/ConsolePageHeader.vue';
import ConsoleState from '@/components/common/ConsoleState.vue';

type SummaryTone = 'blue' | 'green' | 'cyan' | 'orange' | 'red';
type TagTheme = 'default' | 'success' | 'primary' | 'warning' | 'danger';

interface RoleFormData {
  code: string;
  data_scope: RoleDataScope;
  data_scope_department_ids: number[];
  description: string;
  name: string;
}

defineOptions({
  name: 'SystemRole',
});

const loading = ref(false);
const detailLoading = ref(false);
const tableData = ref<RoleItem[]>([]);
const dialogVisible = ref(false);
const permissionDialogVisible = ref(false);
const detailVisible = ref(false);
const formRef = ref();
const isEdit = ref(false);
const currentRole = ref<RoleItem | null>(null);
const selectedPermissionIds = ref<number[]>([]);
const permissionTree = ref<PermissionItem[]>([]);
const departmentTree = ref<DepartmentItem[]>([]);
const departmentMap = ref<Map<number, string>>(new Map());
const permissionKeyword = ref('');
const lastUpdatedAt = ref('');

const searchForm = ref({
  keyword: '',
});

const defaultFormData = (): RoleFormData => ({
  name: '',
  code: '',
  description: '',
  data_scope: 'self',
  data_scope_department_ids: [],
});

const formData = ref<RoleFormData>(defaultFormData());

const dataScopeOptions: Array<{ label: string; value: RoleDataScope }> = [
  { label: '全部数据', value: 'all' },
  { label: '本部门', value: 'department' },
  { label: '本部门及下级', value: 'department_tree' },
  { label: '仅本人', value: 'self' },
  { label: '自定义部门', value: 'custom' },
  { label: '无数据', value: 'none' },
];

const dataScopeLabelMap: Record<RoleDataScope, string> = dataScopeOptions.reduce((map, option) => {
  map[option.value] = option.label;
  return map;
}, {} as Record<RoleDataScope, string>);

const formRules: any = {
  name: [{ required: true, message: '请输入角色名称' }],
  code: [{ required: true, message: '请输入角色代码' }],
  data_scope: [{ required: true, message: '请选择数据范围' }],
};

const pagination = ref({
  current: 1,
  pageSize: 10,
  total: 0,
});

const columns: any[] = [
  { colKey: 'role', title: '角色', width: 250, fixed: 'left' as const },
  { colKey: 'data_scope', title: '数据范围', width: 190 },
  { colKey: 'permissions', title: '权限', width: 110 },
  { colKey: 'status', title: '状态', width: 96 },
  { colKey: 'description', title: '描述', minWidth: 240 },
  { colKey: 'updated_at', title: '创建 / 更新', width: 220 },
  { colKey: 'operation', title: '操作', width: 190, fixed: 'right' as const },
];

const dialogTitle = computed(() => (isEdit.value ? '编辑角色' : '新增角色'));
const permissionDialogTitle = computed(() => (currentRole.value ? `分配权限：${currentRole.value.name}` : '分配权限'));
const detailTitle = computed(() => (currentRole.value ? `${currentRole.value.name || currentRole.value.code} · 角色详情` : '角色详情'));

const enabledCount = computed(() => tableData.value.filter((item) => item.status === 1).length);
const disabledCount = computed(() => tableData.value.filter((item) => item.status !== 1).length);
const assignedPermissionCount = computed(() => tableData.value.filter((item) => item.permissions?.length).length);
const noPermissionCount = computed(() => tableData.value.filter((item) => !item.permissions?.length).length);
const broadScopeCount = computed(() => tableData.value.filter((item) => item.data_scope === 'all' || item.data_scope === 'department_tree').length);
const customScopeCount = computed(() => tableData.value.filter((item) => item.data_scope === 'custom').length);
const dataScopeTypeCount = computed(() => new Set(tableData.value.map((item) => item.data_scope || 'self')).size);
const activeFilterCount = computed(() => (searchForm.value.keyword.trim() ? 1 : 0));
const permissionCoverage = computed(() => {
  if (!tableData.value.length) return '0%';
  return `${Math.round((assignedPermissionCount.value / tableData.value.length) * 100)}%`;
});

const flattenedPermissionCount = computed(() => flattenPermissions(permissionTree.value).length);
const filteredPermissionTree = computed(() => filterPermissionTree(permissionTree.value, permissionKeyword.value.trim().toLowerCase()));

const summaryItems = computed<Array<{ label: string; value: string | number; hint: string; icon: string; tone: SummaryTone }>>(() => [
  {
    label: '角色总数',
    value: pagination.value.total || tableData.value.length,
    hint: `当前页 ${tableData.value.length} 个角色`,
    icon: 'user-safety',
    tone: 'blue',
  },
  {
    label: '启用角色',
    value: enabledCount.value,
    hint: `停用 ${disabledCount.value} 个`,
    icon: 'check-circle',
    tone: 'green',
  },
  {
    label: '权限覆盖',
    value: permissionCoverage.value,
    hint: `${assignedPermissionCount.value} 个角色已分配权限`,
    icon: 'secured',
    tone: noPermissionCount.value > 0 ? 'orange' : 'green',
  },
  {
    label: '高范围角色',
    value: broadScopeCount.value,
    hint: `自定义范围 ${customScopeCount.value} 个`,
    icon: 'data-checked',
    tone: broadScopeCount.value > 0 ? 'orange' : 'cyan',
  },
]);

const updateTime = () => {
  lastUpdatedAt.value = new Date().toLocaleTimeString('zh-CN', { hour12: false });
};

const getDataScopeLabel = (dataScope?: RoleDataScope) => (dataScope ? dataScopeLabelMap[dataScope] || dataScope : '仅本人');

const dataScopeTheme = (dataScope?: RoleDataScope): TagTheme => {
  if (dataScope === 'all') return 'danger';
  if (dataScope === 'department_tree' || dataScope === 'custom') return 'warning';
  if (dataScope === 'department') return 'primary';
  if (dataScope === 'none') return 'default';
  return 'success';
};

const dataScopeHint = (row: RoleItem) => {
  if (row.data_scope === 'custom') return `${row.data_scope_department_ids?.length || 0} 个部门`;
  if (row.data_scope === 'all') return '全局数据权限';
  if (row.data_scope === 'none') return '无数据权限';
  return '按组织边界限制';
};

const buildSearchParams = () => ({
  page: pagination.value.current,
  page_size: pagination.value.pageSize,
  keyword: searchForm.value.keyword.trim() || undefined,
});

const loadData = async () => {
  loading.value = true;
  try {
    const res = await getRoleList(buildSearchParams());
    tableData.value = res.list || [];
    pagination.value.total = res.total || 0;
    updateTime();
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载角色数据失败');
  } finally {
    loading.value = false;
  }
};

const loadPermissionTree = async () => {
  try {
    const res = await getPermissionTree();
    permissionTree.value = res || [];
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载权限树失败');
  }
};

const loadDepartmentTree = async () => {
  try {
    const res = await getDepartmentTree(1);
    departmentTree.value = res || [];
    departmentMap.value = new Map();
    buildDepartmentMap(departmentTree.value);
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载部门树失败');
  }
};

const buildDepartmentMap = (items: DepartmentItem[]) => {
  items.forEach((item) => {
    departmentMap.value.set(item.id, item.name);
    if (item.children?.length) buildDepartmentMap(item.children);
  });
};

const handleAdd = () => {
  isEdit.value = false;
  currentRole.value = null;
  formData.value = defaultFormData();
  loadDepartmentTree();
  dialogVisible.value = true;
};

const handleEdit = async (row: RoleItem) => {
  isEdit.value = true;
  let role = row;
  try {
    role = await getRole(row.id);
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载角色详情失败');
  }
  currentRole.value = role;
  formData.value = {
    name: role.name,
    code: role.code,
    description: role.description || '',
    data_scope: role.data_scope || 'self',
    data_scope_department_ids: role.data_scope_department_ids || [],
  };
  await loadDepartmentTree();
  dialogVisible.value = true;
};

const handleSubmit = async () => {
  const valid = await formRef.value?.validate();
  if (valid !== true) return;

  if (formData.value.data_scope === 'custom' && formData.value.data_scope_department_ids.length === 0) {
    MessagePlugin.warning('请选择自定义部门');
    return;
  }

  const payload = {
    ...formData.value,
    data_scope_department_ids: formData.value.data_scope === 'custom' ? formData.value.data_scope_department_ids : [],
  };

  try {
    if (isEdit.value && currentRole.value) {
      await updateRole(currentRole.value.id, payload);
      MessagePlugin.success('角色已更新');
    } else {
      await createRole(payload);
      MessagePlugin.success('角色已创建');
    }
    dialogVisible.value = false;
    loadData();
  } catch (error: any) {
    MessagePlugin.error(error.message || '操作失败');
  }
};

const handleDelete = async (row: RoleItem) => {
  try {
    await deleteRole(row.id);
    MessagePlugin.success('角色已删除');
    loadData();
  } catch (error: any) {
    MessagePlugin.error(error.message || '删除失败');
  }
};

const handleAssignPermissions = async (row: RoleItem) => {
  let role = row;
  try {
    role = await getRole(row.id);
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载角色权限失败');
  }
  currentRole.value = role;
  selectedPermissionIds.value = role.permissions?.map((permission) => permission.id) || [];
  permissionKeyword.value = '';
  await loadPermissionTree();
  permissionDialogVisible.value = true;
};

const handleAssignPermissionsSubmit = async () => {
  if (!currentRole.value) return;
  try {
    await assignPermissions(currentRole.value.id, {
      permission_ids: selectedPermissionIds.value,
    });
    MessagePlugin.success('权限已分配');
    permissionDialogVisible.value = false;
    loadData();
  } catch (error: any) {
    MessagePlugin.error(error.message || '分配失败');
  }
};

const handleViewDetail = async (row: RoleItem) => {
  currentRole.value = row;
  detailVisible.value = true;
  detailLoading.value = true;
  try {
    currentRole.value = await getRole(row.id);
    if (!departmentMap.value.size) await loadDepartmentTree();
  } catch (error) {
    console.error('加载角色详情失败:', error);
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
    keyword: '',
  };
  handleSearch();
};

const handleRefresh = () => {
  loadData();
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

const roleInitial = (row: RoleItem) => {
  const source = row.name || row.code || 'R';
  return source.slice(0, 1).toUpperCase();
};

const customDepartmentText = (row: RoleItem) => {
  if (row.data_scope !== 'custom') return '-';
  if (!row.data_scope_department_ids?.length) return '未选择部门';
  return row.data_scope_department_ids.map((id) => departmentMap.value.get(id) || `部门 ${id}`).join('、');
};

const flattenPermissions = (items: PermissionItem[]): PermissionItem[] =>
  items.flatMap((item) => [item, ...(item.children?.length ? flattenPermissions(item.children) : [])]);

const filterPermissionTree = (items: PermissionItem[], keyword: string): PermissionItem[] => {
  if (!keyword) return items;
  return items
    .map((item) => {
      const children = item.children?.length ? filterPermissionTree(item.children, keyword) : [];
      const matched = [item.name, item.code, item.path, item.method]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(keyword));
      if (!matched && !children.length) return null;
      return {
        ...item,
        children,
      };
    })
    .filter(Boolean) as PermissionItem[];
};

onMounted(() => {
  loadData();
});
</script>

<style lang="less" scoped>
.role-page {
  --role-bg: #f5f7fb;
  --role-card-shadow: 0 12px 28px rgb(15 23 42 / 6%);
  --role-font: 'Inter', 'HarmonyOS Sans SC', 'MiSans', 'PingFang SC', 'Microsoft YaHei UI', 'Microsoft YaHei', 'Arial', sans-serif;
  --role-number-font: 'DIN Alternate', 'Bahnschrift', 'Inter', 'HarmonyOS Sans SC', 'Microsoft YaHei UI', sans-serif;

  display: flex;
  min-height: calc(100vh - 120px);
  min-width: 0;
  flex-direction: column;
  gap: 14px;
  margin: calc(-1 * var(--td-comp-paddingTB-xl)) calc(-1 * var(--td-comp-paddingLR-xl));
  padding: 14px 18px 18px;
  background: var(--role-bg);
  color: var(--td-text-color-primary);
  font-family: var(--role-font);
  font-feature-settings: 'tnum';
  font-variant-numeric: tabular-nums;
  letter-spacing: 0;
  text-rendering: optimizelegibility;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}

.role-page :deep(.t-card),
.role-page :deep(.t-table),
.role-page :deep(.t-form),
.role-page :deep(.t-button),
.role-page :deep(.t-tag),
.role-page :deep(.t-input),
.role-page :deep(.t-select),
.role-page :deep(.t-dialog),
.role-page :deep(.t-drawer),
.role-page :deep(.t-empty) {
  font-family: var(--role-font);
}

.role-head {
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

.role-head__main {
  min-width: 0;
}

.role-head__title {
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

.role-head__meta {
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

.role-head__actions {
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
    font-family: var(--role-number-font);
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
  box-shadow: var(--role-card-shadow);
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

.filter-card__actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 12px 20px 18px;
}

.role-table {
  width: 100%;
}

.role-table :deep(.t-table__header th) {
  background: #f8fafc;
  color: #475569;
  font-size: 13px;
  font-weight: 700;
}

.role-table :deep(.t-table__body tr:hover td) {
  background: #f8fbff;
}

.role-table :deep(.t-table__body td) {
  padding-top: 14px;
  padding-bottom: 14px;
  border-bottom-color: #eef2f7;
  color: #1f2937;
  vertical-align: top;
}

.role-cell {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
}

.role-avatar,
.permission-role__avatar {
  display: inline-flex;
  width: 34px;
  height: 34px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: linear-gradient(135deg, #2563eb, #14b8a6);
  color: #fff;
  font-size: 14px;
  font-weight: 800;
}

.role-cell__main,
.scope-cell,
.permission-cell,
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
  font-family: var(--role-number-font);
  font-variant-numeric: tabular-nums;
}

.description-text {
  display: -webkit-box;
  overflow: hidden;
  color: #475569;
  font-size: 13px;
  line-height: 20px;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.operation-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.role-form {
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

.scope-radio-group {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.tree-box {
  max-height: 300px;
  overflow: auto;
  padding: 10px 12px;
  border: 1px solid #e8edf5;
  border-radius: 12px;
  background: #f8fafc;
}

.permission-dialog {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.permission-role {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px;
  border: 1px solid #e8edf5;
  border-radius: 12px;
  background: #f8fafc;

  > div {
    min-width: 0;
    flex: 1;
  }

  strong {
    display: block;
    overflow: hidden;
    color: #111827;
    font-size: 15px;
    font-weight: 800;
    line-height: 22px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  span {
    color: #64748b;
    font-size: 12px;
    line-height: 18px;
  }
}

.permission-tools {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.permission-tools :deep(.t-input) {
  max-width: 360px;
}

.permission-tree-box {
  max-height: 420px;
}

.detail-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
  font-family: var(--role-font);
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
  padding: 14px;
  border: 1px solid #e8edf5;
  border-radius: 12px;
  background: #fff;
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

.permission-id-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

@media (width <= 1200px) {
  .summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (width <= 768px) {
  .role-page {
    margin: calc(-1 * var(--td-comp-paddingTB-l)) calc(-1 * var(--td-comp-paddingLR-l));
    padding: 12px;
  }

  .role-head,
  .filter-card__head,
  .table-card__head,
  .filter-card__actions,
  .permission-tools {
    align-items: stretch;
    flex-direction: column;
  }

  .summary-grid,
  .form-grid {
    grid-template-columns: 1fr;
  }

  .keyword-input {
    width: 100%;
  }

  .filter-form :deep(.t-form__item) {
    width: 100%;
  }

  .permission-tools :deep(.t-input) {
    max-width: none;
  }
}
</style>
