<template>
  <div class="dict-page system-management-page">
    <console-page-header>
      <template #title>
        <h2>字典管理</h2>
        <t-tag :theme="disabledTypeCount > 0 || disabledItemCount > 0 ? 'warning' : 'success'" variant="light">
          {{ disabledTypeCount > 0 || disabledItemCount > 0 ? '存在禁用配置' : '字典配置正常' }}
        </t-tag>
      </template>
      <template #meta>
        <span>字典类型</span>
        <span>业务枚举</span>
        <span>前端选项</span>
        <span>共 {{ typePagination.total }} 个类型</span>
        <span v-if="lastUpdatedAt">更新于 {{ lastUpdatedAt }}</span>
      </template>
      <template #actions>
        <t-tag theme="primary" variant="light">当前 {{ activeTabLabel }}</t-tag>
        <t-button v-if="activeTab === 'type'" v-permission="'system:dict:create'" theme="primary" @click="handleAddType">
          <template #icon><t-icon name="add" /></template>
          新增类型
        </t-button>
        <t-button
          v-else
          v-permission="'system:dict:create'"
          theme="primary"
          :disabled="!selectedTypeId"
          @click="handleAddItem"
        >
          <template #icon><t-icon name="add" /></template>
          新增字典项
        </t-button>
        <t-button variant="outline" :loading="currentLoading" @click="handleRefreshCurrent">
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

    <t-card :bordered="false" class="table-card">
      <div class="table-card__head">
        <div>
          <h3>字典配置</h3>
          <p>维护业务枚举、状态选项和前端下拉数据源</p>
        </div>
        <t-tabs v-model="activeTab" class="dict-tabs" @change="handleTabChange">
          <t-tab-panel value="type" label="字典类型" />
          <t-tab-panel value="item" label="字典项" />
        </t-tabs>
      </div>

      <section v-show="activeTab === 'type'" class="dict-section">
        <div class="filter-card">
          <div class="filter-card__head">
            <div>
              <h4>筛选类型</h4>
              <p>
                按字典名称、编码和状态查询类型
                <template v-if="typeActiveFilterCount"> · 已应用 {{ typeActiveFilterCount }} 个条件</template>
              </p>
            </div>
            <t-space size="small" break-line>
              <t-tag theme="primary" variant="light">当前页 {{ typeTableData.length }} 条</t-tag>
              <t-tag :theme="disabledTypeCount > 0 ? 'warning' : 'success'" variant="light">
                禁用 {{ disabledTypeCount }}
              </t-tag>
            </t-space>
          </div>
          <t-form :data="typeSearchForm" class="filter-form" layout="inline" @submit="handleTypeSearch">
            <t-form-item label="关键词" name="keyword">
              <t-input
                v-model="typeSearchForm.keyword"
                clearable
                class="keyword-input"
                placeholder="字典名称 / 编码"
                @enter="handleTypeSearch"
              >
                <template #prefix-icon><t-icon name="search" /></template>
              </t-input>
            </t-form-item>
            <t-form-item label="状态" name="status">
              <t-select v-model="typeSearchForm.status" clearable placeholder="全部状态" class="filter-select">
                <t-option label="启用" :value="1" />
                <t-option label="禁用" :value="0" />
              </t-select>
            </t-form-item>
          </t-form>
          <div class="filter-card__actions">
            <t-space size="small" break-line>
              <t-button theme="primary" :loading="typeLoading" @click="handleTypeSearch">
                <template #icon><t-icon name="search" /></template>
                查询
              </t-button>
              <t-button variant="base" :disabled="typeLoading" @click="handleTypeReset">重置</t-button>
              <t-button variant="outline" :loading="typeLoading" @click="handleRefreshType">
                <template #icon><t-icon name="refresh" /></template>
                刷新列表
              </t-button>
            </t-space>
            <t-button v-permission="'system:dict:create'" theme="primary" @click="handleAddType">
              <template #icon><t-icon name="add" /></template>
              新增类型
            </t-button>
          </div>
        </div>

        <t-table
          row-key="id"
          hover
          class="dict-table"
          table-layout="fixed"
          :data="typeTableData"
          :columns="typeColumns"
          :loading="typeLoading"
          :pagination="typePagination"
          @page-change="handleTypePageChange"
          @page-size-change="handleTypePageSizeChange"
        >
          <template #empty>
            <t-empty :description="typeLoading ? '正在加载字典类型' : '当前筛选条件下暂无字典类型'" />
          </template>
          <template #type="{ row }">
            <div class="type-cell">
              <span class="dict-avatar">{{ dictInitial(row.name || row.code) }}</span>
              <div class="type-cell__main">
                <strong>{{ row.name || '未命名类型' }}</strong>
                <span class="mono-text">{{ row.code || '-' }} · ID {{ row.id }}</span>
              </div>
            </div>
          </template>
          <template #description="{ row }">
            <span class="description-text" :title="row.description">{{ row.description || '暂无描述' }}</span>
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
              <t-link theme="primary" hover="color" @click="handleViewItems(row)">字典项</t-link>
              <t-link v-permission="'system:dict:update'" theme="primary" hover="color" @click="handleEditType(row)">
                编辑
              </t-link>
              <t-popconfirm content="确定删除该类型吗？" @confirm="handleDeleteType(row)">
                <t-link v-permission="'system:dict:delete'" theme="danger" hover="color">删除</t-link>
              </t-popconfirm>
            </div>
          </template>
        </t-table>
      </section>

      <section v-show="activeTab === 'item'" class="dict-section">
        <div class="filter-card">
          <div class="filter-card__head">
            <div>
              <h4>筛选字典项</h4>
              <p>
                选择字典类型后，按标签、值和状态查询具体选项
                <template v-if="itemActiveFilterCount"> · 已应用 {{ itemActiveFilterCount }} 个条件</template>
              </p>
            </div>
            <t-space size="small" break-line>
              <t-tag theme="primary" variant="light">{{ currentTypeName }}</t-tag>
              <t-tag :theme="disabledItemCount > 0 ? 'warning' : 'success'" variant="light">
                禁用 {{ disabledItemCount }}
              </t-tag>
            </t-space>
          </div>
          <t-form :data="itemSearchForm" class="filter-form" layout="inline" @submit="handleItemSearch">
            <t-form-item label="字典类型" name="type_id">
              <t-select
                v-model="selectedTypeId"
                placeholder="请选择字典类型"
                class="type-select"
                @change="handleTypeSelectChange"
              >
                <t-option v-for="type in typeList" :key="type.id" :value="type.id" :label="type.name" />
              </t-select>
            </t-form-item>
            <t-form-item label="关键词" name="keyword">
              <t-input
                v-model="itemSearchForm.keyword"
                clearable
                class="keyword-input"
                placeholder="字典标签 / 字典值"
                @enter="handleItemSearch"
              >
                <template #prefix-icon><t-icon name="search" /></template>
              </t-input>
            </t-form-item>
            <t-form-item label="状态" name="status">
              <t-select v-model="itemSearchForm.status" clearable placeholder="全部状态" class="filter-select">
                <t-option label="启用" :value="1" />
                <t-option label="禁用" :value="0" />
              </t-select>
            </t-form-item>
          </t-form>
          <div class="filter-card__actions">
            <t-space size="small" break-line>
              <t-button theme="primary" :loading="itemLoading" :disabled="!selectedTypeId" @click="handleItemSearch">
                <template #icon><t-icon name="search" /></template>
                查询
              </t-button>
              <t-button variant="base" :disabled="itemLoading" @click="handleItemReset">重置</t-button>
              <t-button variant="outline" :loading="itemLoading" @click="handleRefreshItem">
                <template #icon><t-icon name="refresh" /></template>
                刷新列表
              </t-button>
            </t-space>
            <t-button v-permission="'system:dict:create'" theme="primary" :disabled="!selectedTypeId" @click="handleAddItem">
              <template #icon><t-icon name="add" /></template>
              新增字典项
            </t-button>
          </div>
        </div>

        <t-table
          row-key="id"
          hover
          class="dict-table"
          table-layout="fixed"
          :data="itemTableData"
          :columns="itemColumns"
          :loading="itemLoading"
          :pagination="itemPagination"
          @page-change="handleItemPageChange"
          @page-size-change="handleItemPageSizeChange"
        >
          <template #empty>
            <t-empty :description="itemEmptyText" />
          </template>
          <template #item="{ row }">
            <div class="item-cell">
              <span class="dict-avatar dict-avatar--item">{{ dictInitial(row.label || row.value) }}</span>
              <div class="item-cell__main">
                <strong>{{ row.label || '未命名字典项' }}</strong>
                <span class="mono-text">{{ row.value || '-' }} · ID {{ row.id }}</span>
              </div>
            </div>
          </template>
          <template #type="{ row }">
            <div class="type-ref-cell">
              <strong>{{ typeName(row.dict_type_id) }}</strong>
              <span>类型 ID {{ row.dict_type_id }}</span>
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
          <template #remark="{ row }">
            <span class="description-text" :title="row.remark">{{ row.remark || '暂无备注' }}</span>
          </template>
          <template #updated_at="{ row }">
            <div class="date-cell">
              <strong>{{ formatDateTime(row.updated_at || row.created_at) }}</strong>
              <span>创建 {{ formatDateTime(row.created_at) }}</span>
            </div>
          </template>
          <template #operation="{ row }">
            <div class="operation-actions">
              <t-link v-permission="'system:dict:update'" theme="primary" hover="color" @click="handleEditItem(row)">
                编辑
              </t-link>
              <t-popconfirm content="确定删除该项吗？" @confirm="handleDeleteItem(row)">
                <t-link v-permission="'system:dict:delete'" theme="danger" hover="color">删除</t-link>
              </t-popconfirm>
            </div>
          </template>
        </t-table>
      </section>
    </t-card>

    <t-dialog
      v-model:visible="typeDialogVisible"
      :header="typeDialogTitle"
      width="720px"
      :confirm-btn="{ content: '提交', loading: typeSubmitLoading }"
      @confirm="handleTypeSubmit"
    >
      <t-form ref="typeFormRef" :data="typeFormData" :rules="typeFormRules" label-width="92px" class="dict-form">
        <div class="form-grid">
          <t-form-item label="字典名称" name="name">
            <t-input v-model="typeFormData.name" placeholder="请输入字典名称" />
          </t-form-item>
          <t-form-item label="字典编码" name="code">
            <t-input v-model="typeFormData.code" :disabled="isEditType" placeholder="请输入字典编码" />
          </t-form-item>
          <t-form-item class="form-grid__full" label="描述" name="description">
            <t-textarea
              v-model="typeFormData.description"
              placeholder="请输入描述"
              :autosize="{ minRows: 3, maxRows: 5 }"
            />
          </t-form-item>
          <t-form-item class="form-grid__full" label="状态" name="status">
            <t-radio-group v-model="typeFormData.status" variant="default-filled">
              <t-radio-button :value="1">启用</t-radio-button>
              <t-radio-button :value="0">禁用</t-radio-button>
            </t-radio-group>
          </t-form-item>
        </div>
      </t-form>
    </t-dialog>

    <t-dialog
      v-model:visible="itemDialogVisible"
      :header="itemDialogTitle"
      width="720px"
      :confirm-btn="{ content: '提交', loading: itemSubmitLoading }"
      @confirm="handleItemSubmit"
    >
      <t-form ref="itemFormRef" :data="itemFormData" :rules="itemFormRules" label-width="92px" class="dict-form">
        <div class="form-grid">
          <t-form-item label="字典标签" name="label">
            <t-input v-model="itemFormData.label" placeholder="请输入字典标签" />
          </t-form-item>
          <t-form-item label="字典值" name="value">
            <t-input v-model="itemFormData.value" placeholder="请输入字典值" />
          </t-form-item>
          <t-form-item label="排序" name="sort">
            <t-input-number v-model="itemFormData.sort" :min="0" style="width: 100%" />
          </t-form-item>
          <t-form-item label="状态" name="status">
            <t-radio-group v-model="itemFormData.status" variant="default-filled">
              <t-radio-button :value="1">启用</t-radio-button>
              <t-radio-button :value="0">禁用</t-radio-button>
            </t-radio-group>
          </t-form-item>
          <t-form-item class="form-grid__full" label="备注" name="remark">
            <t-textarea
              v-model="itemFormData.remark"
              placeholder="请输入备注"
              :autosize="{ minRows: 3, maxRows: 5 }"
            />
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
  createDictItem,
  createDictType,
  deleteDictItem,
  deleteDictType,
  getDictItemList,
  getDictTypeList,
  updateDictItem,
  updateDictType,
  type DictItem,
  type DictType,
} from '@/api/system/dict';
import { formatDateTime } from '@/utils/date';
import ConsolePageHeader from '@/components/common/ConsolePageHeader.vue';

type SummaryTone = 'blue' | 'green' | 'cyan' | 'orange';

interface DictTypeFormData {
  code: string;
  description: string;
  name: string;
  status: number;
}

interface DictItemFormData {
  label: string;
  remark: string;
  sort: number;
  status: number;
  value: string;
}

defineOptions({
  name: 'SystemDict',
});

const activeTab = ref<'type' | 'item'>('type');
const typeLoading = ref(false);
const itemLoading = ref(false);
const typeSubmitLoading = ref(false);
const itemSubmitLoading = ref(false);
const typeTableData = ref<DictType[]>([]);
const itemTableData = ref<DictItem[]>([]);
const typeList = ref<DictType[]>([]);
const selectedTypeId = ref<number | undefined>();
const typeDialogVisible = ref(false);
const itemDialogVisible = ref(false);
const typeFormRef = ref();
const itemFormRef = ref();
const isEditType = ref(false);
const isEditItem = ref(false);
const currentType = ref<DictType | null>(null);
const currentItem = ref<DictItem | null>(null);
const lastUpdatedAt = ref('');

const typeSearchForm = ref<{ keyword: string; status?: number }>({
  keyword: '',
  status: undefined,
});

const itemSearchForm = ref<{ keyword: string; status?: number }>({
  keyword: '',
  status: undefined,
});

const defaultTypeFormData = (): DictTypeFormData => ({
  name: '',
  code: '',
  description: '',
  status: 1,
});

const defaultItemFormData = (): DictItemFormData => ({
  label: '',
  value: '',
  sort: 0,
  status: 1,
  remark: '',
});

const typeFormData = ref<DictTypeFormData>(defaultTypeFormData());
const itemFormData = ref<DictItemFormData>(defaultItemFormData());

const typeFormRules: any = {
  name: [{ required: true, message: '请输入字典名称' }],
  code: [{ required: true, message: '请输入字典编码' }],
};

const itemFormRules: any = {
  label: [{ required: true, message: '请输入字典标签' }],
  value: [{ required: true, message: '请输入字典值' }],
};

const typePagination = ref({
  current: 1,
  pageSize: 10,
  total: 0,
});

const itemPagination = ref({
  current: 1,
  pageSize: 10,
  total: 0,
});

const typeColumns: any[] = [
  { colKey: 'type', title: '字典类型', width: 280, fixed: 'left' as const },
  { colKey: 'description', title: '描述', minWidth: 260 },
  { colKey: 'status', title: '状态', width: 96 },
  { colKey: 'updated_at', title: '创建 / 更新', width: 220 },
  { colKey: 'operation', title: '操作', width: 190, fixed: 'right' as const },
];

const itemColumns: any[] = [
  { colKey: 'item', title: '字典项', width: 280, fixed: 'left' as const },
  { colKey: 'type', title: '所属类型', width: 170 },
  { colKey: 'sort', title: '排序', width: 88 },
  { colKey: 'status', title: '状态', width: 96 },
  { colKey: 'remark', title: '备注', minWidth: 220 },
  { colKey: 'updated_at', title: '创建 / 更新', width: 220 },
  { colKey: 'operation', title: '操作', width: 130, fixed: 'right' as const },
];

const typeDialogTitle = computed(() => (isEditType.value ? '编辑字典类型' : '新增字典类型'));
const itemDialogTitle = computed(() => (isEditItem.value ? '编辑字典项' : '新增字典项'));
const activeTabLabel = computed(() => (activeTab.value === 'type' ? '字典类型' : '字典项'));
const currentLoading = computed(() => (activeTab.value === 'type' ? typeLoading.value : itemLoading.value));
const disabledTypeCount = computed(() => typeTableData.value.filter((item) => item.status !== 1).length);
const enabledTypeCount = computed(() => typeTableData.value.filter((item) => item.status === 1).length);
const disabledItemCount = computed(() => itemTableData.value.filter((item) => item.status !== 1).length);
const enabledItemCount = computed(() => itemTableData.value.filter((item) => item.status === 1).length);
const typeNameMap = computed(() => new Map(typeList.value.map((item) => [item.id, item.name])));
const currentTypeName = computed(() => {
  if (!selectedTypeId.value) return '未选择类型';
  return typeNameMap.value.get(selectedTypeId.value) || `类型 ID ${selectedTypeId.value}`;
});
const typeActiveFilterCount = computed(() => {
  let count = 0;
  if (typeSearchForm.value.keyword.trim()) count += 1;
  if (typeSearchForm.value.status !== undefined) count += 1;
  return count;
});
const itemActiveFilterCount = computed(() => {
  let count = 0;
  if (selectedTypeId.value) count += 1;
  if (itemSearchForm.value.keyword.trim()) count += 1;
  if (itemSearchForm.value.status !== undefined) count += 1;
  return count;
});
const itemEmptyText = computed(() => {
  if (itemLoading.value) return '正在加载字典项';
  if (!selectedTypeId.value) return '请先选择字典类型';
  return '当前筛选条件下暂无字典项';
});

const summaryItems = computed<Array<{ label: string; value: string | number; hint: string; icon: string; tone: SummaryTone }>>(() => [
  {
    label: '字典类型',
    value: typePagination.value.total || typeTableData.value.length,
    hint: `当前页 ${typeTableData.value.length} 个类型`,
    icon: 'app',
    tone: 'blue',
  },
  {
    label: '启用类型',
    value: enabledTypeCount.value,
    hint: `禁用 ${disabledTypeCount.value} 个`,
    icon: 'check-circle',
    tone: 'green',
  },
  {
    label: '当前字典项',
    value: selectedTypeId.value ? itemPagination.value.total || itemTableData.value.length : 0,
    hint: currentTypeName.value,
    icon: 'list',
    tone: 'cyan',
  },
  {
    label: '启用字典项',
    value: enabledItemCount.value,
    hint: `禁用 ${disabledItemCount.value} 个`,
    icon: 'filter',
    tone: 'orange',
  },
]);

const loadTypeData = async () => {
  typeLoading.value = true;
  try {
    const res = await getDictTypeList({
      page: typePagination.value.current,
      page_size: typePagination.value.pageSize,
      keyword: typeSearchForm.value.keyword.trim() || undefined,
      status: typeSearchForm.value.status,
    });
    typeTableData.value = res.list || [];
    typePagination.value.total = res.total || 0;
    updateTime();
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载数据失败');
  } finally {
    typeLoading.value = false;
  }
};

const loadAllTypes = async () => {
  try {
    const res = await getDictTypeList({ page: 1, page_size: 1000 });
    typeList.value = res.list || [];
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载字典类型失败');
  }
};

const loadItemData = async () => {
  if (!selectedTypeId.value) {
    itemTableData.value = [];
    itemPagination.value.total = 0;
    return;
  }
  itemLoading.value = true;
  try {
    const res = await getDictItemList({
      page: itemPagination.value.current,
      page_size: itemPagination.value.pageSize,
      type_id: selectedTypeId.value,
      keyword: itemSearchForm.value.keyword.trim() || undefined,
      status: itemSearchForm.value.status,
    });
    itemTableData.value = res.list || [];
    itemPagination.value.total = res.total || 0;
    updateTime();
  } catch (error: any) {
    MessagePlugin.error(error.message || '加载数据失败');
  } finally {
    itemLoading.value = false;
  }
};

const handleTypeSearch = () => {
  typePagination.value.current = 1;
  loadTypeData();
};

const handleItemSearch = () => {
  itemPagination.value.current = 1;
  loadItemData();
};

const handleTypeReset = () => {
  typeSearchForm.value = {
    keyword: '',
    status: undefined,
  };
  typePagination.value.current = 1;
  loadTypeData();
};

const handleItemReset = () => {
  itemSearchForm.value = {
    keyword: '',
    status: undefined,
  };
  itemPagination.value.current = 1;
  loadItemData();
};

const handleAddType = () => {
  isEditType.value = false;
  currentType.value = null;
  typeFormData.value = defaultTypeFormData();
  typeDialogVisible.value = true;
};

const handleEditType = (row: DictType) => {
  isEditType.value = true;
  currentType.value = row;
  typeFormData.value = {
    name: row.name,
    code: row.code,
    description: row.description || '',
    status: row.status,
  };
  typeDialogVisible.value = true;
};

const handleTypeSubmit = async () => {
  const valid = await typeFormRef.value?.validate();
  if (valid !== true) return;

  typeSubmitLoading.value = true;
  try {
    if (isEditType.value && currentType.value) {
      await updateDictType(currentType.value.id, typeFormData.value);
      MessagePlugin.success('更新成功');
    } else {
      await createDictType(typeFormData.value);
      MessagePlugin.success('创建成功');
    }
    typeDialogVisible.value = false;
    loadTypeData();
    loadAllTypes();
  } catch (error: any) {
    MessagePlugin.error(error.message || '操作失败');
  } finally {
    typeSubmitLoading.value = false;
  }
};

const handleDeleteType = async (row: DictType) => {
  try {
    await deleteDictType(row.id);
    MessagePlugin.success('删除成功');
    if (selectedTypeId.value === row.id) {
      selectedTypeId.value = undefined;
      itemTableData.value = [];
      itemPagination.value.total = 0;
    }
    loadTypeData();
    loadAllTypes();
  } catch (error: any) {
    MessagePlugin.error(error.message || '删除失败');
  }
};

const handleViewItems = (row: DictType) => {
  activeTab.value = 'item';
  selectedTypeId.value = row.id;
  itemPagination.value.current = 1;
  loadItemData();
};

const handleAddItem = () => {
  if (!selectedTypeId.value) {
    MessagePlugin.warning('请先选择字典类型');
    return;
  }
  isEditItem.value = false;
  currentItem.value = null;
  itemFormData.value = defaultItemFormData();
  itemDialogVisible.value = true;
};

const handleEditItem = (row: DictItem) => {
  isEditItem.value = true;
  currentItem.value = row;
  itemFormData.value = {
    label: row.label,
    value: row.value,
    sort: row.sort || 0,
    status: row.status,
    remark: row.remark || '',
  };
  itemDialogVisible.value = true;
};

const handleItemSubmit = async () => {
  const valid = await itemFormRef.value?.validate();
  if (valid !== true) return;

  if (!selectedTypeId.value) {
    MessagePlugin.warning('请先选择字典类型');
    return;
  }

  itemSubmitLoading.value = true;
  try {
    if (isEditItem.value && currentItem.value) {
      await updateDictItem(currentItem.value.id, itemFormData.value);
      MessagePlugin.success('更新成功');
    } else {
      await createDictItem({
        ...itemFormData.value,
        dict_type_id: selectedTypeId.value,
      });
      MessagePlugin.success('创建成功');
    }
    itemDialogVisible.value = false;
    loadItemData();
  } catch (error: any) {
    MessagePlugin.error(error.message || '操作失败');
  } finally {
    itemSubmitLoading.value = false;
  }
};

const handleDeleteItem = async (row: DictItem) => {
  try {
    await deleteDictItem(row.id);
    MessagePlugin.success('删除成功');
    loadItemData();
  } catch (error: any) {
    MessagePlugin.error(error.message || '删除失败');
  }
};

const handleTypeSelectChange = () => {
  itemPagination.value.current = 1;
  loadItemData();
};

const handleTabChange = (value: any) => {
  activeTab.value = String(value) === 'item' ? 'item' : 'type';
  if (activeTab.value === 'item' && !selectedTypeId.value && typeList.value.length > 0) {
    selectedTypeId.value = typeList.value[0].id;
    loadItemData();
  }
};

const handleRefreshCurrent = () => {
  if (activeTab.value === 'type') {
    handleRefreshType();
  } else {
    handleRefreshItem();
  }
};

const handleRefreshType = () => {
  loadTypeData();
  loadAllTypes();
};

const handleRefreshItem = () => {
  loadItemData();
};

const handleTypePageChange = (pageInfo: any) => {
  typePagination.value.current = pageInfo.current ?? pageInfo;
  loadTypeData();
};

const handleTypePageSizeChange = (pageSize: number) => {
  typePagination.value.pageSize = pageSize;
  typePagination.value.current = 1;
  loadTypeData();
};

const handleItemPageChange = (pageInfo: any) => {
  itemPagination.value.current = pageInfo.current ?? pageInfo;
  loadItemData();
};

const handleItemPageSizeChange = (pageSize: number) => {
  itemPagination.value.pageSize = pageSize;
  itemPagination.value.current = 1;
  loadItemData();
};

const updateTime = () => {
  lastUpdatedAt.value = new Date().toLocaleTimeString('zh-CN', { hour12: false });
};

const dictInitial = (value?: string) => (value || '字').slice(0, 1).toUpperCase();

const typeName = (typeId: number) => typeNameMap.value.get(typeId) || `类型 ID ${typeId}`;

onMounted(() => {
  loadTypeData();
  loadAllTypes();
});
</script>

<style lang="less" scoped>
.dict-page {
  --dict-bg: #f5f7fb;
  --dict-card-shadow: 0 12px 28px rgb(15 23 42 / 6%);
  --dict-font: 'Inter', 'HarmonyOS Sans SC', 'MiSans', 'PingFang SC', 'Microsoft YaHei UI', 'Microsoft YaHei', 'Arial', sans-serif;
  --dict-number-font: 'DIN Alternate', 'Bahnschrift', 'Inter', 'HarmonyOS Sans SC', 'Microsoft YaHei UI', sans-serif;

  display: flex;
  min-height: calc(100vh - 120px);
  min-width: 0;
  flex-direction: column;
  gap: 14px;
  margin: calc(-1 * var(--td-comp-paddingTB-xl)) calc(-1 * var(--td-comp-paddingLR-xl));
  padding: 14px 18px 18px;
  background: var(--dict-bg);
  color: var(--td-text-color-primary);
  font-family: var(--dict-font);
  font-feature-settings: 'tnum';
  font-variant-numeric: tabular-nums;
  letter-spacing: 0;
}

.dict-page :deep(.t-card),
.dict-page :deep(.t-table),
.dict-page :deep(.t-form),
.dict-page :deep(.t-button),
.dict-page :deep(.t-tag),
.dict-page :deep(.t-input),
.dict-page :deep(.t-select),
.dict-page :deep(.t-tabs),
.dict-page :deep(.t-dialog),
.dict-page :deep(.t-empty) {
  font-family: var(--dict-font);
}

.dict-head {
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

.dict-head__main {
  min-width: 0;
}

.dict-head__title {
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

.dict-head__meta {
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

.dict-head__actions {
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
    font-family: var(--dict-number-font);
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

.table-card {
  overflow: hidden;
  border: 1px solid #e8edf5;
  border-radius: 12px;
  background: #fff;
  box-shadow: var(--dict-card-shadow);
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

.dict-tabs {
  flex-shrink: 0;
  min-width: 260px;
}

.dict-tabs :deep(.t-tabs__nav) {
  justify-content: flex-end;
}

.dict-section {
  padding: 0;
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
  padding: 0 20px 4px;
}

.filter-form :deep(.t-form__item) {
  margin-right: 18px;
  margin-bottom: 12px;
}

.keyword-input {
  width: min(360px, 46vw);
}

.filter-select {
  width: 160px;
}

.type-select {
  width: 220px;
}

.filter-card__actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 0 20px 18px;
}

.dict-table {
  width: 100%;
}

.dict-table :deep(.t-table__th-cell-inner) {
  color: #475569;
  font-size: 12px;
  font-weight: 700;
}

.dict-table :deep(.t-table__body td) {
  vertical-align: middle;
}

.type-cell,
.item-cell {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
}

.dict-avatar {
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

.dict-avatar--item {
  background: linear-gradient(135deg, #0284c7, #22c55e);
}

.type-cell__main,
.item-cell__main,
.type-ref-cell,
.date-cell {
  display: flex;
  min-width: 0;
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

.description-text {
  display: block;
  overflow: hidden;
  color: #475569;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
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
  font-family: var(--dict-number-font);
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

.dict-form {
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
  .dict-page {
    margin: calc(-1 * var(--td-comp-paddingTB-l)) calc(-1 * var(--td-comp-paddingLR-l));
    padding: 12px;
  }

  .dict-head,
  .table-card__head,
  .filter-card__head,
  .filter-card__actions {
    align-items: stretch;
    flex-direction: column;
  }

  .dict-tabs,
  .keyword-input,
  .filter-select,
  .type-select {
    width: 100%;
    min-width: 0;
  }

  .summary-grid,
  .form-grid {
    grid-template-columns: 1fr;
  }

  .filter-form :deep(.t-form__item) {
    width: 100%;
    margin-right: 0;
  }
}
</style>
