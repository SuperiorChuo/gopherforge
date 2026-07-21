import { useEffect, useState } from 'react'
import {
  Table, Button, Space, Tag, Popconfirm, Modal, Form, Input, Select,
  Card, InputNumber, Row, Col,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  PlusOutlined, SearchOutlined, ReloadOutlined, IdcardOutlined,
  EditOutlined, DeleteOutlined, PoweroffOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import * as PostAPI from '@/api/system/posts'
import type { SystemPost } from '@/api/system/posts'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
import { EnableStatusPill } from '@/components/StatusPill'
import { useUrlParams } from '@/hooks/useUrlParams'

interface SearchParams {
  keyword?: string
  status?: number
  page: number
  page_size: number
}

export default function PostPage() {
  const [list, setList] = useState<SystemPost[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<SystemPost | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { hasPerm } = usePermission()

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await PostAPI.getPostList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error('获取岗位列表失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchList(params)
  }, [params])

  const handleSearch = (values: { keyword?: string; status?: number }) => {
    setParams({ ...params, page: 1, keyword: values.keyword, status: values.status })
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

  const openEdit = (record: SystemPost) => {
    setEditRecord(record)
    form.setFieldsValue({
      name: record.name,
      code: record.code,
      sort: record.sort,
      status: record.status,
      remark: record.remark,
    })
    setModalOpen(true)
  }

  const handleDelete = async (id: number) => {
    try {
      await PostAPI.deletePost(id)
      message.success('删除成功')
      if (list.length === 1 && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else {
        fetchList(params)
      }
    } catch {
      // 删除失败原因（如岗位仍有用户关联）由 request 拦截器统一弹出
    }
  }

  // 启用 / 停用切换
  const handleToggleStatus = async (record: SystemPost) => {
    const next = record.status === 1 ? 0 : 1
    try {
      await PostAPI.updatePost(record.id, { status: next })
      message.success(next === 1 ? '已启用' : '已停用')
      fetchList(params)
    } catch {
      // 错误提示由 request 拦截器统一弹出
    }
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      if (editRecord) {
        await PostAPI.updatePost(editRecord.id, values)
        message.success('更新成功')
      } else {
        await PostAPI.createPost(values)
        message.success('创建成功')
      }
      setModalOpen(false)
      fetchList(params)
    } catch {
      // 错误提示由 request 拦截器统一弹出
    } finally {
      setSubmitting(false)
    }
  }

  const columns: ColumnsType<SystemPost> = [
    {
      title: '岗位名称',
      dataIndex: 'name',
      render: (v: string) => (
        <span style={{ fontWeight: 500 }}>
          <IdcardOutlined className="tree-title-icon" />
          {v}
        </span>
      ),
    },
    {
      title: '编码',
      dataIndex: 'code',
      width: 180,
      render: (v: string) => <Tag variant="filled" className="cell-mono">{v}</Tag>,
    },
    { title: '排序', dataIndex: 'sort', width: 70 },
    {
      title: '状态',
      dataIndex: 'status',
      width: 80,
      render: (v: number) => <EnableStatusPill value={v} />,
    },
    {
      title: '备注',
      dataIndex: 'remark',
      ellipsis: true,
      render: (v: string) => v || <span className="cell-muted">—</span>,
    },
    { title: '创建时间', dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: '操作',
      width: 200,
      render: (_, record) => (
        <Space size={0} className="table-actions">
          {hasPerm('system:post:update') && (
            <Button
              type="link"
              size="small"
              icon={<PoweroffOutlined />}
              onClick={() => handleToggleStatus(record)}
            >
              {record.status === 1 ? '停用' : '启用'}
            </Button>
          )}
          {hasPerm('system:post:update') && (
            <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(record)}>编辑</Button>
          )}
          {hasPerm('system:post:delete') && (
            <Popconfirm title="确认删除该岗位?" description="仍有用户关联时将无法删除" onConfirm={() => handleDelete(record.id)}>
              <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div className="page-list post-page">
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
          <Form.Item name="status">
            <Select placeholder="状态" style={{ width: 100 }} allowClear>
              <Select.Option value={1}>启用</Select.Option>
              <Select.Option value={0}>停用</Select.Option>
            </Select>
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
          title="岗位列表"
          total={total}
          extra={
            <Space wrap>
              <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>刷新</Button>
              {hasPerm('system:post:create') && (
                <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新增岗位</Button>
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
          locale={{ emptyText: <GlassEmpty text="暂无岗位" compact /> }}
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
        title={editRecord ? '编辑岗位' : '新增岗位'}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnHidden
        width={560}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="name" label="岗位名称" rules={[{ required: true, message: '请输入岗位名称' }]}>
                <Input placeholder="如：研发工程师" />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="code" label="岗位编码" rules={[{ required: true, message: '请输入岗位编码' }]}>
                <Input placeholder="如：dev" />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="sort" label="排序" initialValue={0}>
                <InputNumber style={{ width: '100%' }} min={0} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="status" label="状态" initialValue={1}>
                <Select>
                  <Select.Option value={1}>启用</Select.Option>
                  <Select.Option value={0}>停用</Select.Option>
                </Select>
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="remark" label="备注">
            <Input.TextArea rows={3} maxLength={500} placeholder="可选" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
