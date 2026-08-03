import { useEffect, useState } from 'react'
import {
  Table, Button, Space, Tag, Popconfirm, Modal, Form, Input, Select,
  Card, Tooltip,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  PlusOutlined, SearchOutlined, ReloadOutlined,
  EditOutlined, DeleteOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Permission } from '@/types'
import * as PermAPI from '@/api/system/permission'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'

interface SearchParams {
  keyword?: string
  page: number
  page_size: number
}

const typeLabels: Record<number, string> = { 1: '菜单', 2: '按钮', 3: 'API' }
const typeColors: Record<number, string> = { 1: 'blue', 2: 'green', 3: 'orange' }

export default function PermissionPage() {
  const [list, setList] = useState<Permission[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<Permission | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { hasPerm } = usePermission()

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await PermAPI.getPermissionList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error('获取权限列表失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchList(params)
  }, [params])

  const handleSearch = (values: { keyword?: string }) => {
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

  const openEdit = (record: Permission) => {
    setEditRecord(record)
    form.setFieldsValue({
      name: record.name,
      code: record.code,
      type: record.type,
      description: record.description,
    })
    setModalOpen(true)
  }

  const handleDelete = async (id: number) => {
    try {
      await PermAPI.deletePermission(id)
      message.success('删除成功')
      if (list.length === 1 && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else {
        fetchList(params)
      }
    } catch {
      message.error('删除失败')
    }
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      if (editRecord) {
        await PermAPI.updatePermission(editRecord.id, values)
        message.success('更新成功')
      } else {
        await PermAPI.createPermission(values)
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

  const columns: ColumnsType<Permission> = [
    { title: 'ID', dataIndex: 'id', width: 60, responsive: ['lg'] },
    {
      title: '名称',
      dataIndex: 'name',
      width: 220,
      ellipsis: true,
      render: (value: string) => <span className="list-primary-cell">{value}</span>,
    },
    {
      title: '编码',
      dataIndex: 'code',
      width: 320,
      responsive: ['sm'],
      render: (v: string) => <Tag variant="filled" className="cell-mono list-code-tag">{v}</Tag>,
    },
    { title: '描述', dataIndex: 'description', width: 260, ellipsis: true, responsive: ['md'] },
    {
      title: '类型',
      dataIndex: 'type',
      width: 90,
      render: (v: number) => <Tag variant="filled" color={typeColors[v]} className="list-type-tag">{typeLabels[v] ?? v}</Tag>,
    },
    { title: '创建时间', dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime, responsive: ['lg'] },
    {
      title: '操作',
      width: 96,
      fixed: 'right',
      render: (_, record) => (
        <Space size={4} className="table-actions compact-table-actions">
          {hasPerm('system:permission:update') && (
            <Tooltip title="编辑">
              <Button type="text" size="small" aria-label="编辑权限" icon={<EditOutlined />} onClick={() => openEdit(record)} />
            </Tooltip>
          )}
          {hasPerm('system:permission:delete') && (
            <Popconfirm title="确认删除该权限?" onConfirm={() => handleDelete(record.id)}>
              <Tooltip title="删除">
                <Button type="text" size="small" danger aria-label="删除权限" icon={<DeleteOutlined />} />
              </Tooltip>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div className="page-list permission-page">
      <Card className="list-filter-card" bordered={false}>
        <Form
          form={searchForm}
          layout="inline"
          className="list-filter-form"
          onFinish={handleSearch}
          initialValues={params}
        >
          <Form.Item name="keyword">
            <Input placeholder="搜索名称 / 编码" prefix={<SearchOutlined />} allowClear style={{ width: 260 }} />
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
              <Button icon={<ReloadOutlined />} onClick={handleReset}>重置</Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>

      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="权限列表"
          total={total}
          extra={
            <Space wrap>
              <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>刷新</Button>
              {hasPerm('system:permission:create') && (
                <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新增权限</Button>
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
          scroll={{ x: 'max-content' }}
          locale={{ emptyText: <GlassEmpty text="暂无权限" compact /> }}
          pagination={{
            total,
            current: params.page,
            pageSize: params.page_size,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (t) => `共 ${t} 条`,
            onChange: (page, page_size) => setParams({ ...params, page, page_size }),
          }}
        />
      </Card>

      <Modal
        title={editRecord ? '编辑权限' : '新增权限'}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input />
          </Form.Item>
          <Form.Item name="code" label="编码" rules={[{ required: true, message: '请输入编码' }]}>
            <Input />
          </Form.Item>
          <Form.Item name="type" label="类型" initialValue={1}>
            <Select>
              <Select.Option value={1}>菜单</Select.Option>
              <Select.Option value={2}>按钮</Select.Option>
              <Select.Option value={3}>API</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={3} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
