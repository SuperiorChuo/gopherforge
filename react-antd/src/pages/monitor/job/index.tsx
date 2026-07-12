import { useEffect, useState, useCallback } from 'react'
import {
  Table, Button, Space, Tag, Popconfirm, Modal, Form, Input, Select,
  message, Card, InputNumber,
} from 'antd'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { ScheduledJob } from '@/types'
import {
  getJobList, createJob, updateJob, deleteJob,
  startJob, stopJob, runJob, cleanupJobLogs,
} from '@/api/monitor'

interface SearchParams {
  page: number
  page_size: number
  keyword?: string
  status?: number
}

export default function JobPage() {
  const [list, setList] = useState<ScheduledJob[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useState<SearchParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<ScheduledJob | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [cleanupModalOpen, setCleanupModalOpen] = useState(false)
  const [cleanupSubmitting, setCleanupSubmitting] = useState(false)
  const [form] = Form.useForm()
  const [cleanupForm] = Form.useForm()

  const fetchList = useCallback(async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await getJobList(p)
      setList(res.list)
      setTotal(res.total)
    } catch {
      message.error('获取任务列表失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchList(params)
  }, [params, fetchList])

  const openCreate = () => {
    setEditRecord(null)
    form.resetFields()
    setModalOpen(true)
  }

  const openEdit = (record: ScheduledJob) => {
    setEditRecord(record)
    form.setFieldsValue({
      name: record.name,
      group_name: record.group_name,
      cron_expr: record.cron_expr,
      handler: record.handler,
      args: record.args,
      status: record.status,
    })
    setModalOpen(true)
  }

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields()
      setSubmitting(true)
      if (editRecord) {
        await updateJob(editRecord.id, values)
        message.success('更新成功')
      } else {
        await createJob(values)
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

  const handleDelete = async (id: number) => {
    try {
      await deleteJob(id)
      message.success('删除成功')
      fetchList(params)
    } catch {
      message.error('删除失败')
    }
  }

  const handleStart = async (id: number) => {
    try {
      await startJob(id)
      message.success('启动成功')
      fetchList(params)
    } catch {
      message.error('启动失败')
    }
  }

  const handleStop = async (id: number) => {
    try {
      await stopJob(id)
      message.success('停止成功')
      fetchList(params)
    } catch {
      message.error('停止失败')
    }
  }

  const handleRun = async (id: number) => {
    try {
      await runJob(id)
      message.success('执行成功')
    } catch {
      message.error('执行失败')
    }
  }

  const handleCleanup = async () => {
    try {
      const { retention_days } = await cleanupForm.validateFields()
      setCleanupSubmitting(true)
      const res = await cleanupJobLogs(retention_days)
      message.success(`清理成功，共删除 ${res.deleted_rows} 条日志`)
      setCleanupModalOpen(false)
      cleanupForm.resetFields()
    } catch {
      message.error('清理失败')
    } finally {
      setCleanupSubmitting(false)
    }
  }

  const columns: ColumnsType<ScheduledJob> = [
    { title: 'ID', dataIndex: 'id', width: 60 },
    { title: '名称', dataIndex: 'name' },
    { title: '分组', dataIndex: 'group_name' },
    { title: 'Cron表达式', dataIndex: 'cron_expr' },
    { title: '处理器', dataIndex: 'handler' },
    { title: '参数', dataIndex: 'args' },
    {
      title: '状态',
      dataIndex: 'status',
      render: (v: number) => (
        <Tag color={v === 1 ? 'success' : 'default'}>{v === 1 ? '运行中' : '已暂停'}</Tag>
      ),
    },
    {
      title: '操作',
      width: 240,
      render: (_, record) => (
        <Space>
          <Button type="link" size="small" onClick={() => openEdit(record)}>编辑</Button>
          {record.status === 0 ? (
            <Button type="link" size="small" onClick={() => handleStart(record.id)}>启动</Button>
          ) : (
            <Button type="link" size="small" onClick={() => handleStop(record.id)}>停止</Button>
          )}
          <Button type="link" size="small" onClick={() => handleRun(record.id)}>立即执行</Button>
          <Popconfirm title="确认删除该任务?" onConfirm={() => handleDelete(record.id)}>
            <Button type="link" size="small" danger>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <Card>
        <div style={{ marginBottom: 16, display: 'flex', gap: 8 }}>
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新增任务</Button>
          <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>刷新</Button>
          <Button onClick={() => { cleanupForm.resetFields(); setCleanupModalOpen(true) }}>清理日志</Button>
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
        title={editRecord ? '编辑任务' : '新增任务'}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnClose
        width={560}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入任务名称' }]}>
            <Input />
          </Form.Item>
          <Form.Item name="group_name" label="分组">
            <Input />
          </Form.Item>
          <Form.Item name="cron_expr" label="Cron表达式" rules={[{ required: true, message: '请输入Cron表达式' }]}>
            <Input placeholder="如: 0 * * * *" />
          </Form.Item>
          <Form.Item name="handler" label="处理器" rules={[{ required: true, message: '请输入处理器' }]}>
            <Input />
          </Form.Item>
          <Form.Item name="args" label="参数">
            <Input />
          </Form.Item>
          <Form.Item name="status" label="状态" initialValue={0}>
            <Select>
              <Select.Option value={1}>运行中</Select.Option>
              <Select.Option value={0}>已暂停</Select.Option>
            </Select>
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="清理任务日志"
        open={cleanupModalOpen}
        onOk={handleCleanup}
        onCancel={() => setCleanupModalOpen(false)}
        confirmLoading={cleanupSubmitting}
        destroyOnClose
      >
        <Form form={cleanupForm} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item
            name="retention_days"
            label="保留天数"
            rules={[{ required: true, message: '请输入保留天数' }]}
            initialValue={30}
          >
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
