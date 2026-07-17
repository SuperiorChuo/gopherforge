import { useEffect, useState, useCallback } from 'react'
import {
  Table, Button, Space, Tag, Popconfirm, Modal, Form, Input, Select,
  Card, InputNumber,
} from 'antd'
import { message } from '@/utils/feedback'
import {
  PlusOutlined, ReloadOutlined, ClearOutlined, SearchOutlined,
  EditOutlined, DeleteOutlined, PlayCircleOutlined, PauseCircleOutlined, ThunderboltOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import type { ScheduledJob } from '@/types'
import {
  getJobList, createJob, updateJob, deleteJob,
  startJob, stopJob, runJob, cleanupJobLogs,
  getJobHealth, type JobHealth,
} from '@/api/monitor'
import TableToolbar from '@/components/TableToolbar'
import StatusPill from '@/components/StatusPill'
import GlassEmpty from '@/components/GlassEmpty'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'
import { usePermission } from '@/hooks/usePermission'

interface SearchParams {
  page: number
  page_size: number
  name?: string
  status?: number
}

export default function JobPage() {
  const [list, setList] = useState<ScheduledJob[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [modalOpen, setModalOpen] = useState(false)
  const [editRecord, setEditRecord] = useState<ScheduledJob | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [cleanupModalOpen, setCleanupModalOpen] = useState(false)
  const [cleanupSubmitting, setCleanupSubmitting] = useState(false)
  const [health, setHealth] = useState<JobHealth | null>(null)
  const [form] = Form.useForm()
  const [cleanupForm] = Form.useForm()
  const [searchForm] = Form.useForm()
  const { hasPerm } = usePermission()

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
    getJobHealth()
      .then(setHealth)
      .catch(() => {
        // ignore
      })
  }, [])

  useEffect(() => {
    fetchList(params)
  }, [params, fetchList])

  const handleSearch = (values: { name?: string; status?: number }) => {
    setParams({ ...params, page: 1, name: values.name, status: values.status })
  }

  const handleSearchReset = () => {
    searchForm.resetFields()
    setParams({ page: 1, page_size: 10 })
  }

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
      cron_expression: record.cron_expression,
      invoke_target: record.invoke_target,
      description: record.description,
      status: record.status,
    })
    setModalOpen(true)
  }

  const handleSubmit = async () => {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
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
      if (list.length === 1 && params.page > 1) {
        setParams({ ...params, page: params.page - 1 })
      } else {
        fetchList(params)
      }
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
    const values = await cleanupForm.validateFields().catch(() => null)
    if (!values) return
    setCleanupSubmitting(true)
    try {
      const res = await cleanupJobLogs(values.retention_days)
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
    {
      title: '分组',
      dataIndex: 'group_name',
      render: (v: string) => v && <Tag variant="filled">{v}</Tag>,
    },
    {
      title: 'Cron表达式',
      dataIndex: 'cron_expression',
      render: (v: string) => <Tag variant="filled" color="geekblue" className="cell-mono">{v}</Tag>,
    },
    {
      title: '调用目标',
      dataIndex: 'invoke_target',
      render: (v: string) => <span className="cell-mono cell-dim">{v}</span>,
    },
    { title: '说明', dataIndex: 'description', ellipsis: true },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (v: number) => (
        v === 1 ? <StatusPill tone="success" label="运行中" /> : <StatusPill tone="muted" label="已暂停" />
      ),
    },
    {
      title: '下次执行',
      dataIndex: 'next_run_time',
      width: 170,
      className: 'cell-time',
      render: formatDateTime,
    },
    {
      title: '操作',
      width: 240,
      render: (_, record) => (
        <Space size={0} className="table-actions">
          {hasPerm('system:job:update') && (
            <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(record)}>编辑</Button>
          )}
          {hasPerm('system:job:run') && (
            record.status === 0 ? (
              <Button type="link" size="small" icon={<PlayCircleOutlined />} onClick={() => handleStart(record.id)}>启动</Button>
            ) : (
              <Button type="link" size="small" icon={<PauseCircleOutlined />} onClick={() => handleStop(record.id)}>停止</Button>
            )
          )}
          {hasPerm('system:job:run') && (
            <Button type="link" size="small" icon={<ThunderboltOutlined />} onClick={() => handleRun(record.id)}>立即执行</Button>
          )}
          {hasPerm('system:job:delete') && (
            <Popconfirm title="确认删除该任务?" onConfirm={() => handleDelete(record.id)}>
              <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div className="page-list job-page">
      <Card className="list-filter-card" bordered={false}>
        <Form
          form={searchForm}
          layout="inline"
          className="list-filter-form"
          onFinish={handleSearch}
          initialValues={params}
        >
          <Form.Item name="name">
            <Input placeholder="搜索任务名称" prefix={<SearchOutlined />} allowClear style={{ width: 220 }} />
          </Form.Item>
          <Form.Item name="status">
            <Select placeholder="状态" style={{ width: 110 }} allowClear>
              <Select.Option value={1}>运行中</Select.Option>
              <Select.Option value={0}>已暂停</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item className="list-filter-actions">
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>查询</Button>
              <Button icon={<ReloadOutlined />} onClick={handleSearchReset}>重置</Button>
            </Space>
          </Form.Item>
          {health && (
            <Form.Item style={{ marginInlineEnd: 0, marginLeft: 'auto' }}>
              <Space size={8} wrap>
                <span className="health-pill">共 {health.total}</span>
                <span className="health-pill health-pill-success">
                  <span className="live-dot" />运行 {health.enabled}
                </span>
                <span className="health-pill">暂停 {health.paused}</span>
                <span className={`health-pill ${health.recent_failed > 0 ? 'health-pill-danger' : ''}`}>
                  近 {health.window_hours}h 失败 {health.recent_failed}
                </span>
              </Space>
            </Form.Item>
          )}
        </Form>
      </Card>

      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="定时任务"
          total={total}
          extra={
            <Space wrap>
              {hasPerm('system:job:run') && (
                <Button
                  icon={<ClearOutlined />}
                  onClick={() => { cleanupForm.resetFields(); setCleanupModalOpen(true) }}
                >
                  清理日志
                </Button>
              )}
              <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)}>刷新</Button>
              {hasPerm('system:job:create') && (
                <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新增任务</Button>
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
          locale={{ emptyText: <GlassEmpty text="暂无定时任务" compact /> }}
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
        title={editRecord ? '编辑任务' : '新增任务'}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        confirmLoading={submitting}
        destroyOnHidden
        width={560}
      >
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入任务名称' }]}>
            <Input />
          </Form.Item>
          <Form.Item name="group_name" label="分组">
            <Input />
          </Form.Item>
          <Form.Item name="cron_expression" label="Cron表达式" rules={[{ required: true, message: '请输入Cron表达式' }]}>
            <Input placeholder="如: 0 * * * * *（秒 分 时 日 月 周）" />
          </Form.Item>
          <Form.Item name="invoke_target" label="调用目标" rules={[{ required: true, message: '请输入调用目标' }]}>
            <Input />
          </Form.Item>
          <Form.Item name="description" label="说明">
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
        destroyOnHidden
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
