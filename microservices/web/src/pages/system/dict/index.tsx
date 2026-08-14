import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table, Button, Space, Tag, Form, Grid, Input, Select, Tabs, InputNumber,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  PlusOutlined, SearchOutlined, ReloadOutlined, DatabaseOutlined, BarsOutlined,
  EditOutlined, DeleteOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { DictType, DictItem } from '@/types'
import * as DictAPI from '@/api/system/dict'
import EntityFormModal from '@/components/EntityFormModal'
import ListFilterForm from '@/components/ListFilterForm'
import ListPageShell from '@/components/ListPageShell'
import TableRowActions from '@/components/TableRowActions'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
import { EnableStatusPill } from '@/components/StatusPill'
import { useConfirmAction } from '@/hooks/useConfirmAction'
import { useCrudModal } from '@/hooks/useCrudModal'
import { useTableQuery } from '@/hooks/useTableQuery'

interface PageParams {
  page: number
  page_size: number
  keyword?: string
  status?: number
}

function DictTypeCRUD() {
  const [params, setParams] = useState<PageParams>({ page: 1, page_size: 10 })
  const modal = useCrudModal<DictType>()
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { t } = useTranslation()
  const { hasPerm } = usePermission()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md

  const fetchList = useCallback((p: PageParams) => DictAPI.getDictTypeList(p), [])
  const onLoadError = useCallback(() => message.error(t('获取字典类型列表失败')), [t])
  const { list, total, loading, reload } = useTableQuery({
    params,
    fetcher: fetchList,
    onError: onLoadError,
  })

  const handleSearch = (values: { keyword?: string; status?: number }) => {
    setParams({ ...params, page: 1, ...values })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

  const openCreate = () => {
    form.resetFields()
    modal.openCreate()
  }

  const openEdit = (record: DictType) => {
    modal.openEdit(record)
    form.setFieldsValue({ name: record.name, code: record.code, status: record.status })
  }

  const confirmAction = useConfirmAction()
  const handleDelete = (id: number) => {
    void confirmAction.run({
      action: () => DictAPI.deleteDictType(id),
      onSuccess: () => {
        message.success(t('删除成功'))
        if (list.length === 1 && params.page > 1) {
          setParams({ ...params, page: params.page - 1 })
        } else {
          void reload()
        }
      },
      onError: () => message.error(t('删除失败')),
    })
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      if (modal.record) {
        await DictAPI.updateDictType(modal.record.id, values)
        message.success(t('更新成功'))
      } else {
        await DictAPI.createDictType(values)
        message.success(t('创建成功'))
      }
      modal.close()
      void reload()
    } catch {
      message.error(t('操作失败'))
    } finally {
      setSubmitting(false)
    }
  }

  const columns: ColumnsType<DictType> = [
    { title: 'ID', dataIndex: 'id', width: 60, responsive: ['lg'] },
    {
      title: t('名称'),
      dataIndex: 'name',
      width: 220,
      ellipsis: true,
      render: (value: string) => <span className="list-primary-cell">{value}</span>,
    },
    {
      title: t('编码'),
      dataIndex: 'code',
      width: 260,
      responsive: ['sm'],
      render: (v: string) => <Tag variant="filled" className="cell-mono list-code-tag">{v}</Tag>,
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 80,
      render: (v: number) => <EnableStatusPill value={v} />,
    },
    { title: t('创建时间'), dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime, responsive: ['lg'] },
    {
      title: t('操作'),
      width: compactActions ? 48 : 96,
      fixed: 'right',
      align: 'center',
      render: (_, record) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          ariaLabel={t('更多操作：{{name}}', { name: record.name })}
          actions={[
            {
              key: 'edit',
              label: t('编辑'),
              icon: <EditOutlined />,
              show: hasPerm('system:dict:update'),
              onClick: () => openEdit(record),
            },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              show: hasPerm('system:dict:delete'),
              confirm: t('确认删除?'),
              onClick: () => handleDelete(record.id),
            },
          ]}
        />
      ),
    },
  ]

  return (
    <>
      <ListPageShell
        className="dict-type-page"
        filter={(
          <ListFilterForm form={searchForm} onFinish={handleSearch}>
            <Form.Item name="keyword">
              <Input placeholder={t('搜索名称 / 编码')} prefix={<SearchOutlined />} allowClear style={{ width: 260 }} />
            </Form.Item>
            <Form.Item name="status">
              <Select placeholder={t('状态')} style={{ width: 100 }} allowClear>
                <Select.Option value={1}>{t('启用')}</Select.Option>
                <Select.Option value={0}>{t('禁用')}</Select.Option>
              </Select>
            </Form.Item>
            <Form.Item className="list-filter-actions">
              <Space>
                <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>{t('查询')}</Button>
                <Button icon={<ReloadOutlined />} onClick={handleReset}>{t('重置')}</Button>
              </Space>
            </Form.Item>
          </ListFilterForm>
        )}
        toolbar={(
          <TableToolbar
            title="字典类型"
            total={total}
            extra={
              <Space wrap>
                <Button icon={<ReloadOutlined />} onClick={() => void reload()}>{t('刷新')}</Button>
                {hasPerm('system:dict:create') && (
                  <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新增字典类型')}</Button>
                )}
              </Space>
            }
          />
        )}
      >
        <Table
          rowKey="id"
          className="list-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          rowClassName={(record) => record.status === 0 ? 'list-row-disabled' : ''}
          scroll={{ x: 'max-content' }}
          locale={{ emptyText: <GlassEmpty text="暂无字典类型" compact /> }}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (n) => t('共 {{n}} 条', { n }),
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
        />
      </ListPageShell>

      <EntityFormModal
        title={modal.record ? t('编辑字典类型') : t('新增字典类型')}
        open={modal.open}
        form={form}
        onClose={modal.close}
        onSubmit={handleSubmit}
        submitting={submitting}
        formProps={{ style: { marginTop: 16 } }}
      >
        <Form.Item name="name" label={t('名称')} rules={[{ required: true, message: t('请输入名称') }]}>
          <Input />
        </Form.Item>
        <Form.Item name="code" label={t('编码')} rules={[{ required: true, message: t('请输入编码') }]}>
          <Input />
        </Form.Item>
        <Form.Item name="status" label={t('状态')} initialValue={1}>
          <Select>
            <Select.Option value={1}>{t('启用')}</Select.Option>
            <Select.Option value={0}>{t('禁用')}</Select.Option>
          </Select>
        </Form.Item>
      </EntityFormModal>
    </>
  )
}

function DictItemCRUD() {
  const [dictTypes, setDictTypes] = useState<DictType[]>([])
  const [selectedTypeId, setSelectedTypeId] = useState<number | null>(null)
  const [params, setParams] = useState<PageParams>({ page: 1, page_size: 10 })
  const modal = useCrudModal<DictItem>()
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { t } = useTranslation()
  const { hasPerm } = usePermission()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md

  // 依赖 [] 而非 [t]：react-i18next 的 t 在切语言时变引用，若依赖 t 会在切语言时重跑本 effect，
  // 把用户选中的字典类型重置回第一个。message 文案在 effect 里用闭包 t 仍能取到当前语言。
  useEffect(() => {
    DictAPI.getDictTypeList({ page: 1, page_size: 200 }).then((res) => {
      setDictTypes(res.list)
      if (res.list.length > 0) {
        setSelectedTypeId(res.list[0].id)
      }
    }).catch(() => message.error(t('加载字典类型失败')))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const fetchItems = useCallback(
    (p: PageParams) => selectedTypeId ? DictAPI.getDictItemList(selectedTypeId, p) : Promise.resolve({ list: [], total: 0 }),
    [selectedTypeId],
  )
  const onItemsLoadError = useCallback(() => message.error(t('获取字典项列表失败')), [t])
  const { list, total, loading, reload } = useTableQuery({
    params,
    fetcher: fetchItems,
    onError: onItemsLoadError,
  })

  const handleSearch = (values: { keyword?: string; status?: number }) => {
    setParams({ ...params, page: 1, ...values })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

  const openCreate = () => {
    form.resetFields()
    form.setFieldsValue({ dict_type_id: selectedTypeId, status: 1, sort: 0 })
    modal.openCreate()
  }

  const openEdit = (record: DictItem) => {
    modal.openEdit(record)
    form.setFieldsValue({
      label: record.label,
      value: record.value,
      sort: record.sort,
      status: record.status,
      dict_type_id: record.dict_type_id,
    })
  }

  const confirmAction = useConfirmAction()
  const handleDelete = (id: number) => {
    void confirmAction.run({
      action: () => DictAPI.deleteDictItem(id),
      onSuccess: () => {
        message.success(t('删除成功'))
        if (list.length === 1 && params.page > 1) {
          setParams({ ...params, page: params.page - 1 })
        } else {
          void reload()
        }
      },
      onError: () => message.error(t('删除失败')),
    })
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      if (modal.record) {
        await DictAPI.updateDictItem(modal.record.id, values)
        message.success(t('更新成功'))
      } else {
        await DictAPI.createDictItem(values)
        message.success(t('创建成功'))
      }
      modal.close()
      void reload()
    } catch {
      message.error(t('操作失败'))
    } finally {
      setSubmitting(false)
    }
  }

  const columns: ColumnsType<DictItem> = [
    { title: 'ID', dataIndex: 'id', width: 60, responsive: ['lg'] },
    {
      title: t('标签'),
      dataIndex: 'label',
      width: 220,
      ellipsis: true,
      render: (value: string) => <span className="list-primary-cell">{value}</span>,
    },
    {
      title: t('值'),
      dataIndex: 'value',
      width: 260,
      responsive: ['sm'],
      render: (v: string) => <Tag variant="filled" className="cell-mono list-code-tag">{v}</Tag>,
    },
    { title: t('排序'), dataIndex: 'sort', width: 70, responsive: ['md'] },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 80,
      render: (v: number) => <EnableStatusPill value={v} />,
    },
    {
      title: t('操作'),
      width: compactActions ? 48 : 96,
      fixed: 'right',
      align: 'center',
      render: (_, record) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          ariaLabel={t('更多操作：{{name}}', { name: record.label })}
          actions={[
            {
              key: 'edit',
              label: t('编辑'),
              icon: <EditOutlined />,
              show: hasPerm('system:dict:update'),
              onClick: () => openEdit(record),
            },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              show: hasPerm('system:dict:delete'),
              confirm: t('确认删除?'),
              onClick: () => handleDelete(record.id),
            },
          ]}
        />
      ),
    },
  ]

  return (
    <>
      <ListPageShell
        className="dict-item-page"
        filter={(
          <ListFilterForm form={searchForm} onFinish={handleSearch}>
            <div className="dict-type-picker">
              <span className="dict-type-picker-label">{t('字典类型')}</span>
              <Select
                value={selectedTypeId}
                onChange={(v) => { setSelectedTypeId(v); setParams({ ...params, page: 1 }) }}
                placeholder={t('请选择字典类型')}
                showSearch
                optionFilterProp="children"
              >
                {dictTypes.map((item) => (
                  <Select.Option key={item.id} value={item.id}>{item.name} ({item.code})</Select.Option>
                ))}
              </Select>
            </div>
            <Form.Item name="keyword">
              <Input placeholder={t('搜索标签 / 值')} prefix={<SearchOutlined />} allowClear style={{ width: 220 }} />
            </Form.Item>
            <Form.Item name="status">
              <Select placeholder={t('状态')} style={{ width: 100 }} allowClear>
                <Select.Option value={1}>{t('启用')}</Select.Option>
                <Select.Option value={0}>{t('禁用')}</Select.Option>
              </Select>
            </Form.Item>
            <Form.Item className="list-filter-actions">
              <Space>
                <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>{t('查询')}</Button>
                <Button icon={<ReloadOutlined />} onClick={handleReset}>{t('重置')}</Button>
              </Space>
            </Form.Item>
          </ListFilterForm>
        )}
        toolbar={(
          <TableToolbar
            title="字典项"
            total={total}
            extra={
              <Space wrap>
                <Button
                  icon={<ReloadOutlined />}
                  onClick={() => void reload()}
                  disabled={!selectedTypeId}
                >
                  {t('刷新')}
                </Button>
                {hasPerm('system:dict:create') && (
                  <Button type="primary" icon={<PlusOutlined />} onClick={openCreate} disabled={!selectedTypeId}>
                    {t('新增字典项')}
                  </Button>
                )}
              </Space>
            }
          />
        )}
      >
        <Table
          rowKey="id"
          className="list-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          rowClassName={(record) => record.status === 0 ? 'list-row-disabled' : ''}
          scroll={{ x: 'max-content' }}
          locale={{ emptyText: <GlassEmpty text="该类型下暂无字典项" compact /> }}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (n) => t('共 {{n}} 条', { n }),
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
        />
      </ListPageShell>

      <EntityFormModal
        title={modal.record ? t('编辑字典项') : t('新增字典项')}
        open={modal.open}
        form={form}
        onClose={modal.close}
        onSubmit={handleSubmit}
        submitting={submitting}
        formProps={{ style: { marginTop: 16 } }}
      >
        <Form.Item name="dict_type_id" hidden>
          <InputNumber />
        </Form.Item>
        <Form.Item name="label" label={t('标签')} rules={[{ required: true, message: t('请输入标签') }]}>
          <Input />
        </Form.Item>
        <Form.Item name="value" label={t('值')} rules={[{ required: true, message: t('请输入值') }]}>
          <Input />
        </Form.Item>
        <Form.Item name="sort" label={t('排序')} initialValue={0}>
          <InputNumber style={{ width: '100%' }} min={0} />
        </Form.Item>
        <Form.Item name="status" label={t('状态')} initialValue={1}>
          <Select>
            <Select.Option value={1}>{t('启用')}</Select.Option>
            <Select.Option value={0}>{t('禁用')}</Select.Option>
          </Select>
        </Form.Item>
      </EntityFormModal>
    </>
  )
}

export default function DictPage() {
  const { t } = useTranslation()
  return (
    <Tabs
      className="page-tabs"
      defaultActiveKey="type"
      items={[
        { key: 'type', label: t('字典类型'), icon: <DatabaseOutlined />, children: <DictTypeCRUD /> },
        { key: 'item', label: t('字典项'), icon: <BarsOutlined />, children: <DictItemCRUD /> },
      ]}
    />
  )
}
