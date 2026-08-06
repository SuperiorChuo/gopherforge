import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Table, Button, Space, Tag, Popconfirm, Modal, Form, Input, Select,
  Card, Tabs, InputNumber, Tooltip,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  PlusOutlined, SearchOutlined, ReloadOutlined, DatabaseOutlined, BarsOutlined,
  EditOutlined, DeleteOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { DictType, DictItem } from '@/types'
import * as DictAPI from '@/api/system/dict'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
import { EnableStatusPill } from '@/components/StatusPill'

interface PageParams {
  page: number
  page_size: number
  keyword?: string
  status?: number
}

function DictTypeCRUD() {
  const [list, setList] = useState<DictType[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useState<PageParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<DictType | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { t } = useTranslation()
  const { hasPerm } = usePermission()

  const fetchList = async (p: PageParams) => {
    setLoading(true)
    try {
      const res = await DictAPI.getDictTypeList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error(t('获取字典类型列表失败'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { fetchList(params) }, [params])

  const handleSearch = (values: { keyword?: string; status?: number }) => {
    setParams({ ...params, page: 1, ...values })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

  const openCreate = () => {
    setEditRecord(null)
    form.resetFields()
    setModalOpen(true)
  }

  const openEdit = (record: DictType) => {
    setEditRecord(record)
    form.setFieldsValue({ name: record.name, code: record.code, status: record.status })
    setModalOpen(true)
  }

  const handleDelete = async (id: number) => {
    try {
      await DictAPI.deleteDictType(id)
      message.success(t('删除成功'))
      if (list.length === 1 && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else {
        fetchList(params)
      }
    } catch {
      message.error(t('删除失败'))
    }
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      if (editRecord) {
        await DictAPI.updateDictType(editRecord.id, values)
        message.success(t('更新成功'))
      } else {
        await DictAPI.createDictType(values)
        message.success(t('创建成功'))
      }
      setModalOpen(false)
      fetchList(params)
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
      width: 96,
      fixed: 'right',
      render: (_, record) => (
        <Space size={4} className="table-actions compact-table-actions">
          {hasPerm('system:dict:update') && (
            <Tooltip title={t('编辑')}>
              <Button type="text" size="small" aria-label={t('编辑字典类型')} icon={<EditOutlined />} onClick={() => openEdit(record)} />
            </Tooltip>
          )}
          {hasPerm('system:dict:delete') && (
            <Popconfirm title={t('确认删除?')} onConfirm={() => handleDelete(record.id)}>
              <Tooltip title={t('删除')}>
                <Button type="text" size="small" danger aria-label={t('删除字典类型')} icon={<DeleteOutlined />} />
              </Tooltip>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div className="page-list dict-type-page">
      <Card className="list-filter-card" bordered={false}>
        <Form form={searchForm} layout="inline" className="list-filter-form" onFinish={handleSearch}>
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
        </Form>
      </Card>
      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="字典类型"
          total={total}
          extra={
            <Space wrap>
              <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>{t('刷新')}</Button>
              {hasPerm('system:dict:create') && (
                <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('新增字典类型')}</Button>
              )}
            </Space>
          }
        />
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
      </Card>
      <Modal
        title={editRecord ? t('编辑字典类型') : t('新增字典类型')}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
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
        </Form>
      </Modal>
    </div>
  )
}

function DictItemCRUD() {
  const [dictTypes, setDictTypes] = useState<DictType[]>([])
  const [selectedTypeId, setSelectedTypeId] = useState<number | null>(null)
  const [list, setList] = useState<DictItem[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useState<PageParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<DictItem | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { t } = useTranslation()
  const { hasPerm } = usePermission()

  useEffect(() => {
    DictAPI.getDictTypeList({ page: 1, page_size: 200 }).then((res) => {
      setDictTypes(res.list)
      if (res.list.length > 0) {
        setSelectedTypeId(res.list[0].id)
      }
    }).catch(() => message.error(t('加载字典类型失败')))
  }, [])

  const fetchItems = async (typeId: number, p: PageParams) => {
    setLoading(true)
    try {
      const res = await DictAPI.getDictItemList(typeId, p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error(t('获取字典项列表失败'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (selectedTypeId) fetchItems(selectedTypeId, params)
  }, [selectedTypeId, params])

  const handleSearch = (values: { keyword?: string; status?: number }) => {
    setParams({ ...params, page: 1, ...values })
  }

  const handleReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

  const openCreate = () => {
    setEditRecord(null)
    form.resetFields()
    form.setFieldsValue({ dict_type_id: selectedTypeId, status: 1, sort: 0 })
    setModalOpen(true)
  }

  const openEdit = (record: DictItem) => {
    setEditRecord(record)
    form.setFieldsValue({
      label: record.label,
      value: record.value,
      sort: record.sort,
      status: record.status,
      dict_type_id: record.dict_type_id,
    })
    setModalOpen(true)
  }

  const handleDelete = async (id: number) => {
    try {
      await DictAPI.deleteDictItem(id)
      message.success(t('删除成功'))
      if (list.length === 1 && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else if (selectedTypeId) {
        fetchItems(selectedTypeId, params)
      }
    } catch {
      message.error(t('删除失败'))
    }
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      if (editRecord) {
        await DictAPI.updateDictItem(editRecord.id, values)
        message.success(t('更新成功'))
      } else {
        await DictAPI.createDictItem(values)
        message.success(t('创建成功'))
      }
      setModalOpen(false)
      if (selectedTypeId) fetchItems(selectedTypeId, params)
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
      width: 96,
      fixed: 'right',
      render: (_, record) => (
        <Space size={4} className="table-actions compact-table-actions">
          {hasPerm('system:dict:update') && (
            <Tooltip title={t('编辑')}>
              <Button type="text" size="small" aria-label={t('编辑字典项')} icon={<EditOutlined />} onClick={() => openEdit(record)} />
            </Tooltip>
          )}
          {hasPerm('system:dict:delete') && (
            <Popconfirm title={t('确认删除?')} onConfirm={() => handleDelete(record.id)}>
              <Tooltip title={t('删除')}>
                <Button type="text" size="small" danger aria-label={t('删除字典项')} icon={<DeleteOutlined />} />
              </Tooltip>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div className="page-list dict-item-page">
      <Card className="list-filter-card" bordered={false}>
        <Form form={searchForm} layout="inline" className="list-filter-form" onFinish={handleSearch}>
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
        </Form>
      </Card>
      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="字典项"
          total={total}
          extra={
            <Space wrap>
              <Button
                icon={<ReloadOutlined />}
                onClick={() => selectedTypeId && fetchItems(selectedTypeId, params)}
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
      </Card>
      <Modal
        title={editRecord ? t('编辑字典项') : t('新增字典项')}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
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
        </Form>
      </Modal>
    </div>
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
