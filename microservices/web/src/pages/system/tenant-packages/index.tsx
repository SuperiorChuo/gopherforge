import { useCallback, useEffect, useMemo, useState, type Key } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Card, Checkbox, Form, Input, InputNumber, Popconfirm, Select, Space, Table, Tag, Tooltip, Tree } from 'antd'
import {
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
  ReloadOutlined,
  SearchOutlined,
  ArrowsAltOutlined,
  ShrinkOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { DataNode } from 'antd/es/tree'
import { message } from '@/utils/feedback'
import type { Permission, TenantPackageInfo } from '@/types'
import * as PackageAPI from '@/api/system/tenantPackages'
import { getPermissionList } from '@/api/system/permission'
import EntityFormModal from '@/components/EntityFormModal'
import ListFilterForm from '@/components/ListFilterForm'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import StatusPill from '@/components/StatusPill'
import { useUrlParams } from '@/hooks/useUrlParams'
import { usePermission } from '@/hooks/usePermission'
import { useCrudModal } from '@/hooks/useCrudModal'
import { useTableQuery } from '@/hooks/useTableQuery'
import { formatDateTime } from '@/utils/format'
import './styles.css'

interface SearchParams {
  keyword?: string
  page: number
  page_size: number
}

/** 按 parent_id 将权限平铺列表组树（数据源与权限管理页一致） */
function buildPermissionTree(perms: Permission[]): DataNode[] {
  const byParent = new Map<number, Permission[]>()
  const ids = new Set(perms.map((p) => p.id))
  for (const p of perms) {
    // 父节点缺失（如分页截断）时归入根，避免节点丢失
    const parent = p.parent_id && ids.has(p.parent_id) ? p.parent_id : 0
    const list = byParent.get(parent) ?? []
    list.push(p)
    byParent.set(parent, list)
  }
  const build = (parentId: number): DataNode[] =>
    (byParent.get(parentId) ?? []).map((p) => ({
      key: p.code,
      title: `${p.name}（${p.code}）`,
      children: build(p.id),
    }))
  return build(0)
}

export default function TenantPackagePage() {
  const { t } = useTranslation()
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const editModal = useCrudModal<TenantPackageInfo>()
  const [submitting, setSubmitting] = useState(false)
  const [allPerms, setAllPerms] = useState<Permission[]>([])
  const [checkedCodes, setCheckedCodes] = useState<string[]>([])
  const [permissionKeyword, setPermissionKeyword] = useState('')
  const [showSelectedOnly, setShowSelectedOnly] = useState(false)
  const [expandedKeys, setExpandedKeys] = useState<Key[]>([])
  const [filteredExpandedKeys, setFilteredExpandedKeys] = useState<Key[]>([])
  const [pendingInitialExpand, setPendingInitialExpand] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { hasPerm } = usePermission()

  const allCodes = useMemo(() => allPerms.map((p) => p.code), [allPerms])
  const rootCodes = useMemo(() => {
    const ids = new Set(allPerms.map((p) => p.id))
    return allPerms
      .filter((p) => !p.parent_id || !ids.has(p.parent_id))
      .map((p) => p.code)
  }, [allPerms])
  const visiblePerms = useMemo(() => {
    const keyword = permissionKeyword.trim().toLowerCase()
    if (!keyword && !showSelectedOnly) return allPerms

    const byId = new Map(allPerms.map((p) => [p.id, p]))
    const selected = new Set(checkedCodes)
    const keepIds = new Set<number>()

    for (const permission of allPerms) {
      const matchesKeyword = !keyword
        || permission.name.toLowerCase().includes(keyword)
        || permission.code.toLowerCase().includes(keyword)
      if (!matchesKeyword || (showSelectedOnly && !selected.has(permission.code))) continue

      let current: Permission | undefined = permission
      while (current && !keepIds.has(current.id)) {
        keepIds.add(current.id)
        current = current.parent_id ? byId.get(current.parent_id) : undefined
      }
    }

    return allPerms.filter((p) => keepIds.has(p.id))
  }, [allPerms, checkedCodes, permissionKeyword, showSelectedOnly])
  const treeData = useMemo(() => buildPermissionTree(visiblePerms), [visiblePerms])
  const visibleCodes = useMemo(() => visiblePerms.map((p) => p.code), [visiblePerms])
  const permissionFilterActive = Boolean(permissionKeyword.trim() || showSelectedOnly)
  const effectiveExpandedKeys = permissionFilterActive ? filteredExpandedKeys : expandedKeys

  useEffect(() => {
    if (permissionFilterActive) setFilteredExpandedKeys(visibleCodes)
  }, [permissionFilterActive, visibleCodes])

  useEffect(() => {
    if (!editModal.open || !pendingInitialExpand || rootCodes.length === 0) return
    setExpandedKeys(rootCodes)
    setPendingInitialExpand(false)
  }, [editModal.open, pendingInitialExpand, rootCodes])

  const fetchList = useCallback(async (p: SearchParams) => {
    const res = await PackageAPI.getTenantPackageList({
      page: p.page,
      page_size: p.page_size,
      keyword: p.keyword || undefined,
    })
    return { list: res.list || [], total: res.total ?? 0 }
  }, [])
  const onLoadError = useCallback((error?: unknown) => {
    message.error(error instanceof Error ? error.message : t('获取套餐列表失败'))
  }, [t])
  const { list, total, loading, reload } = useTableQuery({ params, fetcher: fetchList, onError: onLoadError })

  useEffect(() => {
    // 权限树复用权限管理页的数据源（平铺列表，前端组树）
    getPermissionList({ page: 1, page_size: 500 })
      .then((res) => setAllPerms(res.list || []))
      .catch(() => message.error(t('加载权限树失败')))
  }, [t])

  const handleSearch = (values: { keyword?: string }) => {
    setParams({ ...params, page: 1, ...values })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

  function openCreate() {
    form.resetFields()
    form.setFieldsValue({ status: 1 })
    setCheckedCodes([])
    setPermissionKeyword('')
    setShowSelectedOnly(false)
    setExpandedKeys(rootCodes)
    setPendingInitialExpand(rootCodes.length === 0)
    editModal.openCreate()
  }

  function openEdit(row: TenantPackageInfo) {
    editModal.openEdit(row)
    form.setFieldsValue({ name: row.name, status: row.status, remark: row.remark, storage_quota_mb: row.storage_quota_mb ?? 0 })
    setCheckedCodes(row.permission_codes || [])
    setPermissionKeyword('')
    setShowSelectedOnly(false)
    setExpandedKeys(rootCodes)
    setPendingInitialExpand(rootCodes.length === 0)
  }

  function updateExpandedKeys(keys: Key[]) {
    if (permissionFilterActive) setFilteredExpandedKeys(keys)
    else setExpandedKeys(keys)
  }

  async function onSubmit() {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      if (editModal.record) {
        await PackageAPI.updateTenantPackage(editModal.record.id, {
          name: values.name,
          permission_codes: checkedCodes,
          storage_quota_mb: values.storage_quota_mb ?? 0,
          status: values.status,
          remark: values.remark ?? '',
        })
        message.success(t('套餐已更新（改小套餐不回收存量角色权限，仅拦截新分配）'))
      } else {
        await PackageAPI.createTenantPackage({
          name: values.name,
          permission_codes: checkedCodes,
          storage_quota_mb: values.storage_quota_mb ?? 0,
          status: values.status,
          remark: values.remark ?? '',
        })
        message.success(t('套餐已创建'))
      }
      editModal.close()
      void reload()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('保存失败'))
    } finally {
      setSubmitting(false)
    }
  }

  async function onDelete(row: TenantPackageInfo) {
    try {
      await PackageAPI.deleteTenantPackage(row.id)
      message.success(t('套餐已删除'))
      void reload()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('删除失败（有租户绑定时需先解绑）'))
    }
  }

  const columns: ColumnsType<TenantPackageInfo> = [
    { title: 'ID', dataIndex: 'id', width: 70, responsive: ['lg'] },
    {
      title: t('名称'),
      dataIndex: 'name',
      width: 220,
      ellipsis: true,
      render: (value: string) => <span className="list-primary-cell">{value}</span>,
    },
    {
      title: t('权限数'),
      dataIndex: 'permission_codes',
      width: 100,
      responsive: ['md'],
      render: (v: string[]) => <Tag variant="filled">{v?.length ?? 0}</Tag>,
    },
    { title: t('备注'), dataIndex: 'remark', width: 260, ellipsis: true, responsive: ['lg'] },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 90,
      render: (v: number) =>
        v === 1 ? <StatusPill tone="success" label="启用" /> : <StatusPill tone="muted" label="停用" />,
    },
    { title: t('创建时间'), dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime, responsive: ['lg'] },
    {
      title: t('操作'),
      width: 96,
      fixed: 'right',
      render: (_, row) => (
        <Space size={4} className="table-actions tenant-package-row-actions">
          {hasPerm('system:tenant-package:update') && (
            <Tooltip title={t('编辑')}>
              <Button type="text" size="small" aria-label={t('编辑租户套餐 {{name}}', { name: row.name })} icon={<EditOutlined />} onClick={() => openEdit(row)} />
            </Tooltip>
          )}
          {hasPerm('system:tenant-package:delete') && (
            <Popconfirm title={t('确定删除该套餐？有租户绑定时将拒绝删除。')} onConfirm={() => void onDelete(row)}>
              <Tooltip title={t('删除')}>
                <Button type="text" size="small" danger aria-label={t('删除租户套餐 {{name}}', { name: row.name })} icon={<DeleteOutlined />} />
              </Tooltip>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div className="page-list tenant-package-page">
      <Card className="list-filter-card" bordered={false}>
        <ListFilterForm
          form={searchForm}
          onFinish={handleSearch}
          initialValues={params}
        >
          <Form.Item name="keyword">
            <Input placeholder={t('搜索套餐名称')} prefix={<SearchOutlined />} allowClear style={{ width: 240 }} />
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>{t('查询')}</Button>
              <Button icon={<ReloadOutlined />} onClick={handleReset}>{t('重置')}</Button>
            </Space>
          </Form.Item>
        </ListFilterForm>
      </Card>

      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="租户套餐"
          total={total}
          extra={
            <Space wrap>
              <Button icon={<ReloadOutlined />} onClick={() => void reload()}>{t('刷新')}</Button>
              {hasPerm('system:tenant-package:create') && (
                <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
                  {t('新建套餐')}
                </Button>
              )}
            </Space>
          }
        />
        <Table
          rowKey="id"
          className="list-table"
          loading={loading}
          dataSource={list}
          columns={columns}
          rowClassName={(row) => row.status === 1 ? '' : 'list-row-disabled'}
          scroll={{ x: 'max-content' }}
          locale={{ emptyText: <GlassEmpty text="暂无套餐" compact /> }}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showTotal: (n) => t('共 {{n}} 条', { n }),
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
        />
      </Card>

      <EntityFormModal
        title={editModal.record ? t('编辑套餐 #{{id}}', { id: editModal.record.id }) : t('新建套餐')}
        open={editModal.open}
        form={form}
        onClose={editModal.close}
        onSubmit={onSubmit}
        submitting={submitting}
        width={640}
      >
          <Form.Item name="name" label={t('套餐名称')} rules={[{ required: true, message: t('必填') }]}>
            <Input placeholder={t('如：基础版 / 专业版')} maxLength={128} />
          </Form.Item>
          <Form.Item name="status" label={t('状态')}>
            <Select
              options={[
                { label: t('启用'), value: 1 },
                { label: t('停用'), value: 0 },
              ]}
            />
          </Form.Item>
          <Form.Item name="remark" label={t('备注')}>
            <Input.TextArea rows={2} maxLength={255} placeholder={t('套餐说明（可选）')} />
          </Form.Item>
          <Form.Item name="storage_quota_mb" label={t('存储配额（MB）')} extra={t('0 = 不限；租户累计文件大小超过配额后上传被拒')}>
            <InputNumber min={0} style={{ width: '100%' }} placeholder={t('0 = 不限')} />
          </Form.Item>
          <Form.Item
            label={t('套餐权限（已选 {{n}} 项）', { n: checkedCodes.length })}
            extra={t('严格勾选（父子不联动），与角色授权页的平铺勾选语义一致')}
          >
            <div className="tenant-permission-toolbar">
              <Input
                value={permissionKeyword}
                onChange={(event) => setPermissionKeyword(event.target.value)}
                placeholder={t('搜索权限名称 / 编码')}
                prefix={<SearchOutlined />}
                allowClear
              />
              <div className="tenant-permission-toolbar-actions">
                <Checkbox checked={showSelectedOnly} onChange={(event) => setShowSelectedOnly(event.target.checked)}>
                  {t('只看已选')}
                </Checkbox>
                <Tooltip title={t('全部展开')}>
                  <Button
                    size="small"
                    aria-label={t('全部展开套餐权限')}
                    icon={<ArrowsAltOutlined />}
                    onClick={() => updateExpandedKeys(permissionFilterActive ? visibleCodes : allCodes)}
                  />
                </Tooltip>
                <Tooltip title={t('全部收起')}>
                  <Button size="small" aria-label={t('全部收起套餐权限')} icon={<ShrinkOutlined />} onClick={() => updateExpandedKeys([])} />
                </Tooltip>
                <Button size="small" onClick={() => setCheckedCodes(allCodes)}>{t('全选')}</Button>
                <Button size="small" onClick={() => setCheckedCodes([])}>{t('清空')}</Button>
              </div>
            </div>
            <div className="tenant-permission-tree-frame">
              {/* height 触发 antd Tree 内置虚拟滚动；外层 maxHeight+overflow 只能裁视口，
                  500 条权限仍会全部进 DOM 且 defaultExpandAll 全展开 */}
              {treeData.length > 0 ? (
                <Tree
                  checkable
                  checkStrictly
                  selectable={false}
                  height={320}
                  treeData={treeData}
                  expandedKeys={effectiveExpandedKeys}
                  autoExpandParent={false}
                  onExpand={updateExpandedKeys}
                  checkedKeys={{ checked: checkedCodes, halfChecked: [] }}
                  onCheck={(keys) => {
                    const checked = Array.isArray(keys) ? keys : keys.checked
                    setCheckedCodes(checked.map(String))
                  }}
                />
              ) : (
                <GlassEmpty text="没有匹配的权限" compact />
              )}
            </div>
          </Form.Item>
      </EntityFormModal>
    </div>
  )
}
