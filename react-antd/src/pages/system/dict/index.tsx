import { useEffect, useState } from 'react'
import {
  Table, Button, Space, Tag, Popconfirm, Modal, Form, Input, Select,
  message, Card, Tabs, InputNumber,
} from 'antd'
import { PlusOutlined, SearchOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { DictType, DictItem } from '@/types'
import * as DictAPI from '@/api/system/dict'

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

  const fetchList = async (p: PageParams) => {
    setLoading(true)
    try {
      const res = await DictAPI.getDictTypeList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error('获取字典类型列表失败')
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
      message.success('删除成功')
      fetchList(params)
    } catch {
      message.error('删除失败')
    }
  }

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields()
      setSubmitting(true)
      if (editRecord) {
        await DictAPI.updateDictType(editRecord.id, values)
        message.success('更新成功')
      } else {
        await DictAPI.createDictType(values)
        message.success('创建成功')
      }
      setModalOpen(false)
      fetchList(params)
    } catch {
      message.error('操作失败')
    } finally {
      setSubmitting(false)
    }
  }

  const columns: ColumnsType<DictType> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '名称', dataIndex: 'name' },
    { title: '编码', dataIndex: 'code' },
    {
      title: '状态',
      dataIndex: 'status',
      render: (v: number) => <Tag color={v === 1 ? 'success' : 'default'}>{v === 1 ? '启用' : '禁用'}</Tag>,
    },
    { title: '创建时间', dataIndex: 'created_at', width: 170 },
    {
      title: '操作',
      width: 140,
      render: (_, record) => (
        <Space>
          <Button type="link" size="small" onClick={() => openEdit(record)}>编辑</Button>
          <Popconfirm title="确认删除?" onConfirm={() => handleDelete(record.id)}>
            <Button type="link" size="small" danger>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <Card style={{ marginBottom: 16 }}>
        <Form form={searchForm} layout="inline" onFinish={handleSearch}>
          <Form.Item name="keyword">
            <Input placeholder="名称/编码" prefix={<SearchOutlined />} allowClear />
          </Form.Item>
          <Form.Item name="status">
            <Select placeholder="状态" style={{ width: 100 }} allowClear>
              <Select.Option value={1}>启用</Select.Option>
              <Select.Option value={0}>禁用</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item>
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
              <Button icon={<ReloadOutlined />} onClick={handleReset}>重置</Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>
      <Card>
        <div style={{ marginBottom: 16 }}>
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新增字典类型</Button>
        </div>
        <Table
          rowKey="id"
          columns={columns}
          dataSource={list}
          loading={loading}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showTotal: (t) => `共 ${t} 条`,
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
        />
      </Card>
      <Modal
        title={editRecord ? '编辑字典类型' : '新增字典类型'}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnClose
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input />
          </Form.Item>
          <Form.Item name="code" label="编码" rules={[{ required: true, message: '请输入编码' }]}>
            <Input />
          </Form.Item>
          <Form.Item name="status" label="状态" initialValue={1}>
            <Select>
              <Select.Option value={1}>启用</Select.Option>
              <Select.Option value={0}>禁用</Select.Option>
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

  useEffect(() => {
    DictAPI.getDictTypeList({ page: 1, page_size: 200 }).then((res) => {
      setDictTypes(res.list)
      if (res.list.length > 0) {
        setSelectedTypeId(res.list[0].id)
      }
    }).catch(() => message.error('加载字典类型失败'))
  }, [])

  const fetchItems = async (typeId: number, p: PageParams) => {
    setLoading(true)
    try {
      const res = await DictAPI.getDictItemList(typeId, p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error('获取字典项列表失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (selectedTypeId) fetchItems(selectedTypeId, params)
  }, [selectedTypeId, params])

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
      message.success('删除成功')
      if (selectedTypeId) fetchItems(selectedTypeId, params)
    } catch {
      message.error('删除失败')
    }
  }

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields()
      setSubmitting(true)
      if (editRecord) {
        await DictAPI.updateDictItem(editRecord.id, values)
        message.success('更新成功')
      } else {
        await DictAPI.createDictItem(values)
        message.success('创建成功')
      }
      setModalOpen(false)
      if (selectedTypeId) fetchItems(selectedTypeId, params)
    } catch {
      message.error('操作失败')
    } finally {
      setSubmitting(false)
    }
  }

  const columns: ColumnsType<DictItem> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '标签', dataIndex: 'label' },
    { title: '值', dataIndex: 'value' },
    { title: '排序', dataIndex: 'sort', width: 60 },
    {
      title: '状态',
      dataIndex: 'status',
      render: (v: number) => <Tag color={v === 1 ? 'success' : 'default'}>{v === 1 ? '启用' : '禁用'}</Tag>,
    },
    {
      title: '操作',
      width: 140,
      render: (_, record) => (
        <Space>
          <Button type="link" size="small" onClick={() => openEdit(record)}>编辑</Button>
          <Popconfirm title="确认删除?" onConfirm={() => handleDelete(record.id)}>
            <Button type="link" size="small" danger>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <Card style={{ marginBottom: 16 }}>
        <Space>
          <span>选择字典类型：</span>
          <Select
            style={{ width: 200 }}
            value={selectedTypeId}
            onChange={(v) => { setSelectedTypeId(v); setParams({ page: 1, page_size: 10 }) }}
            placeholder="请选择字典类型"
          >
            {dictTypes.map((t) => (
              <Select.Option key={t.id} value={t.id}>{t.name} ({t.code})</Select.Option>
            ))}
          </Select>
        </Space>
      </Card>
      <Card>
        <div style={{ marginBottom: 16 }}>
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate} disabled={!selectedTypeId}>
            新增字典项
          </Button>
        </div>
        <Table
          rowKey="id"
          columns={columns}
          dataSource={list}
          loading={loading}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showTotal: (t) => `共 ${t} 条`,
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
        />
      </Card>
      <Modal
        title={editRecord ? '编辑字典项' : '新增字典项'}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnClose
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="dict_type_id" hidden>
            <InputNumber />
          </Form.Item>
          <Form.Item name="label" label="标签" rules={[{ required: true, message: '请输入标签' }]}>
            <Input />
          </Form.Item>
          <Form.Item name="value" label="值" rules={[{ required: true, message: '请输入值' }]}>
            <Input />
          </Form.Item>
          <Form.Item name="sort" label="排序" initialValue={0}>
            <InputNumber style={{ width: '100%' }} min={0} />
          </Form.Item>
          <Form.Item name="status" label="状态" initialValue={1}>
            <Select>
              <Select.Option value={1}>启用</Select.Option>
              <Select.Option value={0}>禁用</Select.Option>
            </Select>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export default function DictPage() {
  return (
    <Tabs
      defaultActiveKey="type"
      items={[
        { key: 'type', label: '字典类型', children: <DictTypeCRUD /> },
        { key: 'item', label: '字典项', children: <DictItemCRUD /> },
      ]}
    />
  )
}
