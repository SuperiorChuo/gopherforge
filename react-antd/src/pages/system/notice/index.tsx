import { useEffect, useState } from 'react'
import {
  Table, Button, Space, Tag, Popconfirm, Modal, Form, Input, Select,
  Card, DatePicker, Switch,
} from 'antd'
import { message } from '@/utils/feedback'
import { PlusOutlined, SearchOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { Notice } from '@/types'
import * as NoticeAPI from '@/api/system/notice'
import TableToolbar from '@/components/TableToolbar'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'
import dayjs from 'dayjs'

const { RangePicker } = DatePicker

interface SearchParams {
  keyword?: string
  type?: number
  status?: number
  page: number
  page_size: number
}

const typeLabels: Record<number, string> = { 1: '通知', 2: '公告' }

export default function NoticePage() {
  const [list, setList] = useState<Notice[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<Notice | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [form] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { hasPerm } = usePermission()

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await NoticeAPI.getNoticeList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error('获取通知列表失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchList(params)
  }, [params])

  const handleSearch = (values: { keyword?: string; type?: number; status?: number }) => {
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

  const openEdit = (record: Notice) => {
    setEditRecord(record)
    form.setFieldsValue({
      title: record.title,
      content: record.content,
      type: record.type,
      status: record.status,
      time_range: record.start_time && record.end_time
        ? [dayjs(record.start_time), dayjs(record.end_time)]
        : undefined,
    })
    setModalOpen(true)
  }

  const handleDelete = async (id: number) => {
    try {
      await NoticeAPI.deleteNotice(id)
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

  const handleToggleStatus = async (record: Notice, checked: boolean) => {
    try {
      await NoticeAPI.updateNoticeStatus(record.id, checked ? 1 : 0)
      message.success(checked ? '已启用' : '已停用')
      fetchList(params)
    } catch {
      message.error('状态更新失败')
    }
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      const { time_range, ...rest } = values
      // 后端按 time.Time 解析，需 RFC3339 格式
      const payload = {
        ...rest,
        start_time: time_range?.[0]?.toISOString(),
        end_time: time_range?.[1]?.toISOString(),
      }
      if (editRecord) {
        await NoticeAPI.updateNotice(editRecord.id, payload)
        message.success('更新成功')
      } else {
        await NoticeAPI.createNotice(payload)
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

  const columns: ColumnsType<Notice> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '标题', dataIndex: 'title', ellipsis: true },
    {
      title: '类型',
      dataIndex: 'type',
      width: 80,
      render: (v: number) => <Tag color={v === 1 ? 'blue' : 'orange'}>{typeLabels[v] ?? v}</Tag>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      render: (v: number, record) => (
        <Switch
          size="small"
          checked={v === 1}
          checkedChildren="启用"
          unCheckedChildren="停用"
          disabled={!hasPerm('system:notice:update')}
          onChange={(checked) => handleToggleStatus(record, checked)}
        />
      ),
    },
    { title: '开始时间', dataIndex: 'start_time', width: 170, className: 'cell-time', render: formatDateTime },
    { title: '结束时间', dataIndex: 'end_time', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: '操作',
      width: 140,
      render: (_, record) => (
        <Space>
          {hasPerm('system:notice:update') && (
            <Button type="link" size="small" onClick={() => openEdit(record)}>编辑</Button>
          )}
          {hasPerm('system:notice:delete') && (
            <Popconfirm title="确认删除该通知?" onConfirm={() => handleDelete(record.id)}>
              <Button type="link" size="small" danger>删除</Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div>
      <Card style={{ marginBottom: 16 }}>
        <Form form={searchForm} layout="inline" onFinish={handleSearch} initialValues={params}>
          <Form.Item name="keyword">
            <Input placeholder="标题" prefix={<SearchOutlined />} allowClear />
          </Form.Item>
          <Form.Item name="type">
            <Select placeholder="类型" style={{ width: 100 }} allowClear>
              <Select.Option value={1}>通知</Select.Option>
              <Select.Option value={2}>公告</Select.Option>
            </Select>
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
        <TableToolbar
          title="通知公告"
          total={total}
          extra={
            <>
              <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>刷新</Button>
              {hasPerm('system:notice:create') && (
                <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新增通知</Button>
              )}
            </>
          }
        />
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
        title={editRecord ? '编辑通知' : '新增通知'}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnHidden
        width={600}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="title" label="标题" rules={[{ required: true, message: '请输入标题' }]}>
            <Input />
          </Form.Item>
          <Form.Item name="content" label="内容" rules={[{ required: true, message: '请输入内容' }]}>
            <Input.TextArea rows={5} />
          </Form.Item>
          <Form.Item name="type" label="类型" initialValue={1}>
            <Select>
              <Select.Option value={1}>通知</Select.Option>
              <Select.Option value={2}>公告</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="status" label="状态" initialValue={1}>
            <Select>
              <Select.Option value={1}>启用</Select.Option>
              <Select.Option value={0}>禁用</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="time_range" label="有效时间">
            <RangePicker showTime format="YYYY-MM-DD HH:mm:ss" style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
