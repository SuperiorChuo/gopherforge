<template>
  <div class="user-page system-management-page">
    <console-page-header>
      <template #title>
        <h2>用户管理</h2>
        <t-tag :theme="disabledCount > 0 ? 'warning' : 'success'" variant="light">
          {{ disabledCount > 0 ? '存在禁用用户' : '账号状态正常' }}
        </t-tag>
      </template>
      <template #meta>
        <span>账号权限</span>
        <span>组织部门</span>
        <span>角色分配</span>
        <span>共 {{ pagination.total }} 个用户</span>
        <span v-if="lastUpdatedAt">更新于 {{ lastUpdatedAt }}</span>
      </template>
      <template #actions>
        <t-tag theme="primary" variant="light">当前页 {{ tableData.length }} 条</t-tag>
        <t-button v-permission="'system:user:create'" theme="primary" @click="handleAdd">
          <template #icon><t-icon name="add" /></template>
          新增用户
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
            按用户名、昵称、邮箱、手机号和账号状态筛选用户
            <template v-if="activeFilterCount"> · 已应用 {{ activeFilterCount }} 个条件</template>
          </p>
        </div>
        <t-space size="small" break-line>
          <t-tag theme="primary" variant="light">角色覆盖 {{ roleCoverage }}</t-tag>
          <t-tag :theme="disabledCount > 0 ? 'warning' : 'success'" variant="light">禁用 {{ disabledCount }}</t-tag>
        </t-space>
      </div>
      <t-form :data="searchForm" class="filter-form" layout="inline" @submit="handleSearch">
        <t-form-item label="关键字" name="keyword">
          <t-input
            v-model="searchForm.keyword"
            clearable
            class="keyword-input"
            placeholder="用户名 / 昵称 / 邮箱 / 手机号"
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
        </t-space>
      </div>
    </t-card>

    <t-card :bordered="false" class="table-card">
      <div class="table-card__head">
        <div>
          <h3>用户列表</h3>
          <p>
            账号资料、组织部门、角色权限和账号状态
            <template v-if="pagination.total"> · 共 {{ pagination.total }} 条</template>
          </p>
        </div>
        <t-space size="small">
          <t-tag :theme="enabledCount > 0 ? 'success' : 'default'" variant="light">启用 {{ enabledCount }}</t-tag>
          <t-tag theme="primary" variant="light">部门 {{ departmentCoveredCount }}</t-tag>
        </t-space>
      </div>

      <t-table
        row-key="id"
        hover
        class="user-table"
        table-layout="fixed"
        :data="tableData"
        :columns="columns"
        :loading="loading"
        :pagination="pagination"
        @page-change="handlePageChange"
        @page-size-change="handlePageSizeChange"
      >
        <template #empty>
          <t-empty :description="loading ? '正在加载用户数据' : '当前筛选条件下暂无用户'" />
        </template>
        <template #user="{ row }">
          <div class="user-cell">
            <t-avatar class="user-avatar" size="34px" :image="row.avatar">
              {{ userInitial(row) }}
            </t-avatar>
            <div class="user-cell__main">
              <strong>{{ row.nickname || row.username || '未知用户' }}</strong>
              <span>{{ row.username || '-' }} · ID {{ row.id }}</span>
            </div>
          </div>
        </template>
        <template #department="{ row }">
          <div class="department-cell">
            <strong>{{ departmentName(row.department_id) }}</strong>
            <span>{{ row.department_id ? `部门 ID ${row.department_id}` : '未分配部门' }}</span>
          </div>
        </template>
        <template #contact="{ row }">
          <div class="contact-cell">
            <strong>{{ row.email || '未填写邮箱' }}</strong>
            <span>{{ row.phone || '未填写手机号' }}</span>
          </div>
        </template>
        <template #status="{ row }">
          <t-tag :theme="row.status === 1 ? 'success' : 'default'" variant="light">
            {{ row.status === 1 ? '启用' : '禁用' }}
          </t-tag>
        </template>
        <template #roles="{ row }">
          <t-space v-if="row.roles?.length" size="4px" break-line>
            <t-tag v-for="role in row.roles" :key="role.id" theme="primary" variant="light" size="small">
              {{ role.name }}
            </t-tag>
          </t-space>
          <t-tag v-else theme="warning" variant="light" size="small">未分配</t-tag>
        </template>
        <template #created_at="{ row }">
          <div class="date-cell">
            <strong>{{ formatDateTime(row.created_at) }}</strong>
            <span>更新 {{ formatDateTime(row.updated_at) }}</span>
          </div>
        </template>
        <template #operation="{ row }">
          <div class="operation-actions">
            <t-link theme="primary" hover="color" @click="handleViewDetail(row)">详情</t-link>
            <t-link v-permission="'system:user:update'" theme="primary" hover="color" @click="handleEdit(row)">
              编辑
            </t-link>
            <t-link v-permission="'system:user:update'" theme="primary" hover="color" @click="handleAssignRoles(row)">
              角色
            </t-link>
            <t-link v-permission="'system:user:update'" theme="primary" hover="color" @click="handleToggleStatus(row)">
              {{ row.status === 1 ? '禁用' : '启用' }}
            </t-link>
            <t-popconfirm content="确定删除该用户吗？" @confirm="handleDelete(row)">
              <t-link v-permission="'system:user:delete'" theme="danger" hover="color">删除</t-link>
            </t-popconfirm>
          </div>
        </template>
      </t-table>
    </t-card>

    <t-dialog v-model:visible="dialogVisible" :header="dialogTitle" width="680px" @confirm="handleSubmit">
      <t-form ref="formRef" :data="formData" :rules="formRules" label-width="92px" class="user-form">
        <div class="form-grid">
          <t-form-item label="用户名" name="username">
            <t-input v-model="formData.username" :disabled="isEdit" placeholder="请输入用户名" />
          </t-form-item>
          <t-form-item v-if="!isEdit" label="密码" name="password">
            <t-input v-model="formData.password" type="password" placeholder="请输入密码" />
          </t-form-item>
          <t-form-item label="昵称" name="nickname">
            <t-input v-model="formData.nickname" placeholder="请输入昵称" />
          </t-form-item>
          <t-form-item label="邮箱" name="email">
            <t-input v-model="formData.email" placeholder="请输入邮箱" />
          </t-form-item>
          <t-form-item label="手机号" name="phone">
            <t-input v-model="formData.phone" placeholder="请输入手机号" />
          </t-form-item>
          <t-form-item label="状态" name="status">
            <t-radio-group v-model="formData.status" variant="default-filled">
              <t-radio-button :value="1">启用</t-radio-button>
              <t-radio-button :value="0">禁用</t-radio-button>
            </t-radio-group>
          </t-form-item>
          <t-form-item class="form-grid__full" label="部门" name="department_id">
            <t-tree-select
              v-model="formData.department_id"
              :data="departmentTree"
              :tree-props="{ keys: { value: 'id', label: 'name', children: 'children' }, expandAll: true }"
              placeholder="请选择部门"
              clearable
            />
          </t-form-item>
        </div>
      </t-form>
    </t-dialog>

    <t-dialog
      v-model:visible="roleDialogVisible"
      :header="roleDialogTitle"
      width="560px"
      @confirm="handleAssignRolesSubmit"
    >
      <div class="role-dialog">
        <div v-if="currentUser" class="role-user">
          <span class="role-user__avatar">{{ userInitial(currentUser) }}</span>
          <div>
            <strong>{{ currentUser.nickname || currentUser.username }}</strong>
            <span>{{ currentUser.username }} · 当前 {{ currentUser.roles?.length || 0 }} 个角色</span>
          </div>
        </div>
        <t-checkbox-group v-model="selectedRoleIds" class="role-options" :options="roleOptions" />
      </div>
    </t-dialog>

    <t-drawer v-model:visible="detailVisible" :header="detailTitle" size="640px" :footer="false">
      <t-loading :loading="detailLoading" size="small">
        <div v-if="currentUser" class="detail-panel">
          <div class="detail-hero" :class="{ 'detail-hero--disabled': currentUser.status !== 1 }">
            <span class="detail-hero__icon">
              <t-icon :name="currentUser.status === 1 ? 'user-circle' : 'user-blocked'" />
            </span>
            <div>
              <strong>{{ currentUser.nickname || currentUser.username || '未知用户' }}</strong>
              <span>{{ currentUser.status === 1 ? '账号启用中' : '账号已禁用' }}</span>
            </div>
          </div>

          <t-descriptions bordered :column="1" class="detail-desc">
            <t-descriptions-item label="用户 ID">{{ currentUser.id }}</t-descriptions-item>
            <t-descriptions-item label="用户名">{{ currentUser.username || '-' }}</t-descriptions-item>
            <t-descriptions-item label="昵称">{{ currentUser.nickname || '-' }}</t-descriptions-item>
            <t-descriptions-item label="邮箱">{{ currentUser.email || '-' }}</t-descriptions-item>
            <t-descriptions-item label="手机号">{{ currentUser.phone || '-' }}</t-descriptions-item>
            <t-descriptions-item label="部门">{{ departmentName(currentUser.department_id) }}</t-descriptions-item>
            <t-descriptions-item label="状态">{{ currentUser.status === 1 ? '启用' : '禁用' }}</t-descriptions-item>
            <t-descriptions-item label="创建时间">{{ formatDateTime(currentUser.created_at) }}</t-descriptions-item>
            <t-descriptions-item label="更新时间">{{ formatDateTime(currentUser.updated_at) }}</t-descriptions-item>
          </t-descriptions>

          <section class="detail-section">
            <div class="detail-section__head">
              <span>角色权限</span>
              <t-tag theme="primary" variant="light">{{ currentUser.roles?.length || 0 }} 个角色</t-tag>
            </div>
            <t-space v-if="currentUser.roles?.length" size="6px" break-line>
              <t-tag v-for="role in currentUser.roles" :key="role.id" theme="primary" variant="light">
                {{ role.name }} / {{ role.code }}
              </t-tag>
            </t-space>
            <t-empty v-else description="暂未分配角色" />
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
import { getAllRoles } from '@/api/system/role';
import {
  assignRoles,
  createUser,
  deleteUser,
  getUser,
  getUserList,
  updateUser,
  updateUserStatus,
  type UserItem,
} from '@/api/system/user';
import { formatDateTime } from '@/utils/date';
import ConsolePageHeader from '@/components/common/ConsolePageHeader.vue';

type SummaryTone = 'blue' | 'green' | 'cyan' | 'orange' | 'red';

defineOptions({
  name: 'SystemUser',
});

const loading = ref(false);
const detailLoading = ref(false);
const tableData = ref<UserItem[]>([]);
const dialogVisible = ref(false);
const roleDialogVisible = ref(false);
const detailVisible = ref(false);
const formRef = ref();
const isEdit = ref(false);
const currentUser = ref<UserItem | null>(null);
const selectedRoleIds = ref<number[]>([]);
const roleOptions = ref<Array<{ label: string; value: number }>>([]);
const departmentTree = ref<DepartmentItem[]>([]);
const departmentMap = ref<Map<number, string>>(new Map());
const lastUpdatedAt = ref('');

const searchForm = ref({
  keyword: '',
  status: undefined as number | undefined,
});

const formData = ref({
  username: '',
  password: '',
  nickname: '',
  email: '',
  phone: '',
  status: 1,
  department_id: 0,
});

const formRules: any = {
  username: [{ required: true, message: '请输入用户名' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
  email: [{ type: 'email', message: '请输入正确的邮箱地址' }],
};

const pagination = ref({
  current: 1,
  pageSize: 10,
  total: 0,
});

const columns: any[] = [
  { colKey: 'user', title: '用户', width: 240, fixed: 'left' as const },
  { colKey: 'department', title: '部门', width: 170 },
  { colKey: 'contact', title: '联系方式', minWidth: 240 },
  { colKey: 'status', title: '状态', width: 96 },
  { colKey: 'roles', title: '角色', minWidth: 220 },
  { colKey: 'created_at', title: '创建 / 更新', width: 220 },
  { colKey: 'operation', title: '操作', width: 220, fixed: 'right' as const },
];

const dialogTitle = computed(() => (isEdit.value ? '编辑用户' : '新增用户'));
const roleDialogTitle = computed(() => (currentUser.value ? `分配角色：${currentUser.value.nickname || currentUser.value.username}` : '分配角色'));
const detailTitle = computed(() => (currentUser.value ? `${currentUser.value.nickname || currentUser.value.username} · 用户详情` : '用户详情'));

const enabledCount = computed(() => tableData.value.filter((item) => item.status === 1).length);
const disabledCount = computed(() => tableData.value.filter((item) => item.status !== 1).length);
const roleAssignedCount = computed(() => tableData.value.filter((item) => item.roles?.length).length);
const departmentCoveredCount = computed(() => new Set(tableData.value.map((item) => item.department_id).filter(Boolean)).size);
const activeFilterCount = computed(() => {
  let count = 0;
  if (searchForm.value.keyword.trim()) count += 1;
  if (searchForm.value.status !== undefined) count += 1;
  return count;
});
const roleCoverage = computed(() => {
  if (!tableData.value.length) return '0%';
  return `${Math.round((roleAssignedCount.value / tableData.value.length) * 100)}%`;
});

const summaryItems = computed<Array<{ label: string; value: string | number; hint: string; icon: string; tone: SummaryTone }>>(() => [
  {
    label: '用户总数',
    value: pagination.value.total || tableData.value.length,
    hint: `当前页 ${tableData.value.length} 个账号`,
    icon: 'user-circle',
    tone: 'blue',
  },
  {
    label: '启用账号',
    value: enabledCount.value,
    hint: `禁用 ${disabledCount.value} 个`,
    icon: 'check-circle',
    tone: 'green',
  },
  {
    label: '角色覆盖',
    value: roleCoverage.value,
    hint: `${roleAssignedCount.value} 个账号已分配角色`,
    icon: 'usergroup',
    tone: roleAssignedCount.value === tableData.value.length && tableData.value.length ? 'green' : 'orange',
  },
  {
    label: '部门覆盖',
    value: departmentCoveredCount.value,
    hint: departmentTree.value.length ? '组织树已加载' : '等待部门数据',
    icon: 'root-list',
    tone: 'cyan',
  },
]);

const updateTime = () => {
  lastUpdatedAt.value = new Date().toLocaleTimeString('zh-CN', { hour12: false });
};

const buildSearchParams = () => ({
  page: pagination.value.current,
  page_size: pagination.value.pageSize,
  keyword: searchForm.value.keyword.trim() || undefined,
  status: searchForm.value.status,
});

const loadData = async () => {
  loading.value = true;
  try {
    const res = await getUserList(buildSearchParams());
    tableData.value = res.list || [];
    pagination.value.total = res.total || 0;
    updateTime();
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载用户数据失败');
  } finally {
    loading.value = false;
  }
};

const loadDepartments = async () => {
  try {
    const data = await getDepartmentTree(1);
    departmentTree.value = data || [];
    departmentMap.value = new Map();
    const buildMap = (items: DepartmentItem[]) => {
      for (const item of items) {
        departmentMap.value.set(item.id, item.name);
        if (item.children?.length) buildMap(item.children);
      }
    };
    buildMap(departmentTree.value);
  } catch (error) {
    console.error('加载部门失败:', error);
  }
};

const loadRoles = async () => {
  try {
    const res = await getAllRoles();
    roleOptions.value = res.map((role) => ({
      label: `${role.name} / ${role.code}`,
      value: role.id,
    }));
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载角色列表失败');
  }
};

const handleAdd = () => {
  isEdit.value = false;
  currentUser.value = null;
  formData.value = {
    username: '',
    password: '',
    nickname: '',
    email: '',
    phone: '',
    status: 1,
    department_id: 0,
  };
  dialogVisible.value = true;
};

const handleEdit = (row: UserItem) => {
  isEdit.value = true;
  currentUser.value = row;
  formData.value = {
    username: row.username,
    password: '',
    nickname: row.nickname || '',
    email: row.email || '',
    phone: row.phone || '',
    status: row.status,
    department_id: row.department_id || 0,
  };
  dialogVisible.value = true;
};

const handleSubmit = async () => {
  const valid = await formRef.value?.validate();
  if (!valid) return;

  try {
    if (isEdit.value && currentUser.value) {
      await updateUser(currentUser.value.id, {
        nickname: formData.value.nickname,
        email: formData.value.email,
        phone: formData.value.phone,
        status: formData.value.status,
        department_id: formData.value.department_id,
      });
      MessagePlugin.success('用户已更新');
    } else {
      await createUser({
        username: formData.value.username,
        password: formData.value.password,
        nickname: formData.value.nickname,
        email: formData.value.email,
        phone: formData.value.phone,
        department_id: formData.value.department_id,
        status: formData.value.status,
      });
      MessagePlugin.success('用户已创建');
    }
    dialogVisible.value = false;
    loadData();
  } catch (error: any) {
    MessagePlugin.error(error.message || '操作失败');
  }
};

const handleDelete = async (row: UserItem) => {
  try {
    await deleteUser(row.id);
    MessagePlugin.success('用户已删除');
    loadData();
  } catch (error: any) {
    MessagePlugin.error(error.message || '删除失败');
  }
};

const handleToggleStatus = async (row: UserItem) => {
  try {
    await updateUserStatus(row.id, row.status === 1 ? 0 : 1);
    MessagePlugin.success(row.status === 1 ? '用户已禁用' : '用户已启用');
    loadData();
  } catch (error: any) {
    MessagePlugin.error(error.message || '操作失败');
  }
};

const handleAssignRoles = async (row: UserItem) => {
  currentUser.value = row;
  selectedRoleIds.value = row.roles?.map((r) => r.id) || [];
  await loadRoles();
  roleDialogVisible.value = true;
};

const handleAssignRolesSubmit = async () => {
  if (!currentUser.value) return;
  try {
    await assignRoles(currentUser.value.id, {
      role_ids: selectedRoleIds.value,
    });
    MessagePlugin.success('角色已分配');
    roleDialogVisible.value = false;
    loadData();
  } catch (error: any) {
    MessagePlugin.error(error.message || '分配失败');
  }
};

const handleViewDetail = async (row: UserItem) => {
  currentUser.value = row;
  detailVisible.value = true;
  detailLoading.value = true;
  try {
    currentUser.value = await getUser(row.id);
  } catch (error) {
    console.error('加载用户详情失败:', error);
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
    status: undefined,
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

const departmentName = (departmentId?: number) => {
  if (!departmentId) return '未分配部门';
  return departmentMap.value.get(departmentId) || `部门 ${departmentId}`;
};

const userInitial = (row: UserItem) => {
  const source = row.nickname || row.username || 'U';
  return source.slice(0, 1).toUpperCase();
};

onMounted(() => {
  loadData();
  loadDepartments();
});
</script>

<style lang="less" scoped>
.user-page {
  --user-bg: #f5f7fb;
  --user-card-shadow: 0 12px 28px rgb(15 23 42 / 6%);
  --user-font: 'Inter', 'HarmonyOS Sans SC', 'MiSans', 'PingFang SC', 'Microsoft YaHei UI', 'Microsoft YaHei', 'Arial', sans-serif;
  --user-number-font: 'DIN Alternate', 'Bahnschrift', 'Inter', 'HarmonyOS Sans SC', 'Microsoft YaHei UI', sans-serif;

  display: flex;
  min-height: calc(100vh - 120px);
  min-width: 0;
  flex-direction: column;
  gap: 14px;
  margin: calc(-1 * var(--td-comp-paddingTB-xl)) calc(-1 * var(--td-comp-paddingLR-xl));
  padding: 14px 18px 18px;
  background: var(--user-bg);
  color: var(--td-text-color-primary);
  font-family: var(--user-font);
  font-feature-settings: 'tnum';
  font-variant-numeric: tabular-nums;
  letter-spacing: 0;
  text-rendering: optimizelegibility;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}

.user-page :deep(.t-card),
.user-page :deep(.t-table),
.user-page :deep(.t-form),
.user-page :deep(.t-button),
.user-page :deep(.t-tag),
.user-page :deep(.t-input),
.user-page :deep(.t-select),
.user-page :deep(.t-dialog),
.user-page :deep(.t-drawer),
.user-page :deep(.t-empty) {
  font-family: var(--user-font);
}

.user-head {
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

.user-head__main {
  min-width: 0;
}

.user-head__title {
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

.user-head__meta {
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

.user-head__actions {
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
    font-family: var(--user-number-font);
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
  box-shadow: var(--user-card-shadow);
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
  width: 280px;
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

.user-table {
  width: 100%;
}

.user-table :deep(.t-table__header th) {
  background: #f8fafc;
  color: #475569;
  font-size: 13px;
  font-weight: 700;
}

.user-table :deep(.t-table__body tr:hover td) {
  background: #f8fbff;
}

.user-table :deep(.t-table__body td) {
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
  flex-shrink: 0;
  background: linear-gradient(135deg, #2563eb, #14b8a6);
  color: #fff;
  font-weight: 800;
}

.user-cell__main,
.department-cell,
.contact-cell,
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

.operation-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.user-form {
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

.role-dialog {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.role-user {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px;
  border: 1px solid #e8edf5;
  border-radius: 12px;
  background: #f8fafc;

  strong {
    display: block;
    color: #111827;
    font-size: 15px;
    font-weight: 800;
    line-height: 22px;
  }

  span {
    color: #64748b;
    font-size: 12px;
    line-height: 18px;
  }
}

.role-user__avatar {
  display: inline-flex;
  width: 38px;
  height: 38px;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: linear-gradient(135deg, #2563eb, #14b8a6);
  color: #fff;
  font-weight: 800;
}

.role-options {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.role-options :deep(.t-checkbox) {
  margin: 0;
  padding: 10px 12px;
  border: 1px solid #e8edf5;
  border-radius: 10px;
  background: #fff;
}

.detail-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
  font-family: var(--user-font);
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
  .user-page {
    margin: calc(-1 * var(--td-comp-paddingTB-l)) calc(-1 * var(--td-comp-paddingLR-l));
    padding: 12px;
  }

  .user-head,
  .filter-card__head,
  .table-card__head,
  .filter-card__actions {
    align-items: stretch;
    flex-direction: column;
  }

  .summary-grid,
  .form-grid,
  .role-options {
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
