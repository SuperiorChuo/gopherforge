import { useEffect, useState } from 'react'
import { Button, Card, Form, Input, Modal, Space, Table, Tabs, Tag, Typography } from 'antd'
import {
  AuditOutlined,
  CheckOutlined,
  CloseOutlined,
  ReloadOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import {
  approveTask,
  BPM_TASK_STATUS_META,
  listDoneTasks,
  listTodoTasks,
  rejectTask,
  type BpmTask,
} from '@/api/bpm'
import BpmInstanceTimeline from '@/components/BpmInstanceTimeline'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import StatusPill from '@/components/StatusPill'
import { displayUserName, useUserNameMap } from '@/hooks/useUserNameMap'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'

const { Text } = Typography

interface SearchParams {
  tab?: string
  page: number
  page_size: number
}

type ActionState = { mode: 'approve' | 'reject'; task: BpmTask } | null

export default function BpmTasksPage() {
  const [list, setList] = useState<BpmTask[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  const [action, setAction] = useState<ActionState>(null)
  const [submitting, setSubmitting] = useState(false)
  const [actionForm] = Form.useForm()
  const userMap = useUserNameMap()

  const tab = params.tab === 'done' ? 'done' : 'todo'

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const query = { page: p.page, page_size: p.page_size }
      const res = p.tab === 'done' ? await listDoneTasks(query) : await listTodoTasks(query)
      setList(res.list ?? [])
      setTotal(res.total ?? 0)
    } catch {
      // 错误提示由 request 拦截器统一弹出
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void fetchList(params)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [params])

  const openAction = (mode: 'approve' | 'reject', task: BpmTask) => {
    actionForm.resetFields()
    setAction({ mode, task })
  }

  const onSubmitAction = async () => {
    if (!action) return
    const values = await actionForm.validateFields().catch(() => null)
    if (!values) return
    setSubmitting(true)
    try {
      if (action.mode === 'approve') {
        const res = await approveTask(action.task.id, values.comment)
        message.success(res?.instance_status === 'approved' ? '已同意，流程审批通过' : '已同意')
      } else {
        const res = await rejectTask(action.task.id, values.comment)
        message.success(res?.instance_status === 'rejected' ? '已拒绝，流程结束' : '已拒绝')
      }
      setAction(null)
      void fetchList(params)
    } catch {
      // 错误提示由 request 拦截器统一弹出
    } finally {
      setSubmitting(false)
    }
  }

  const titleColumn: ColumnsType<BpmTask>[number] = {
    title: '审批事项',
    render: (_, row) => (
      <div>
        <span style={{ fontWeight: 500 }}>{row.instance_title || `实例 #${row.instance_id}`}</span>
        <div>
          <Text type="secondary" style={{ fontSize: 12 }}>
            {displayUserName(userMap, row.initiator_id)} 发起
            {row.biz_type ? ` · ${row.biz_type}` : ''}
          </Text>
        </div>
      </div>
    ),
  }

  const nodeColumn: ColumnsType<BpmTask>[number] = {
    title: '节点',
    dataIndex: 'node_name',
    width: 160,
    render: (v: string) => <Tag>{v || '-'}</Tag>,
  }

  const todoColumns: ColumnsType<BpmTask> = [
    titleColumn,
    nodeColumn,
    { title: '到达时间', dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: '超时提醒',
      dataIndex: 'timeout_at',
      width: 170,
      render: (v?: string) => {
        if (!v) return <span className="cell-muted">—</span>
        const overdue = new Date(v).getTime() < Date.now()
        return (
          <Text type={overdue ? 'danger' : 'secondary'} style={{ fontSize: 13 }}>
            {overdue ? '已超时 · ' : ''}
            {formatDateTime(v)}
          </Text>
        )
      },
    },
    {
      title: '操作',
      width: 170,
      render: (_, row) => (
        <Space size={0} className="table-actions">
          <Button type="link" size="small" icon={<CheckOutlined />} onClick={() => openAction('approve', row)}>
            同意
          </Button>
          <Button type="link" size="small" danger icon={<CloseOutlined />} onClick={() => openAction('reject', row)}>
            拒绝
          </Button>
        </Space>
      ),
    },
  ]

  const doneColumns: ColumnsType<BpmTask> = [
    titleColumn,
    nodeColumn,
    {
      title: '处理结果',
      dataIndex: 'status',
      width: 110,
      render: (v: string) => {
        const meta = BPM_TASK_STATUS_META[v]
        return meta ? <StatusPill tone={meta.tone} label={meta.label} pulse={false} /> : <Tag>{v}</Tag>
      },
    },
    {
      title: '审批意见',
      dataIndex: 'comment',
      ellipsis: true,
      render: (v?: string) => v || <span className="cell-muted">—</span>,
    },
    { title: '处理时间', dataIndex: 'acted_at', width: 170, className: 'cell-time', render: formatDateTime },
  ]

  return (
    <div className="page-list bpm-tasks-page">
      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="待办中心"
          total={total}
          icon={<AuditOutlined />}
          gradient="linear-gradient(135deg, #60a5fa, #2563eb)"
          glow="rgba(37, 99, 235, 0.4)"
          description="待我审批与我已处理的审批任务；展开行可查看流转时间线"
          extra={
            <Button icon={<ReloadOutlined />} onClick={() => void fetchList(params)}>
              刷新
            </Button>
          }
        />
        <Tabs
          activeKey={tab}
          onChange={(key) => setParams({ ...params, tab: key, page: 1 })}
          items={[
            { key: 'todo', label: '待我审批' },
            { key: 'done', label: '我已处理' },
          ]}
        />
        <Table
          rowKey="id"
          className="list-table"
          columns={tab === 'done' ? doneColumns : todoColumns}
          dataSource={list}
          loading={loading}
          locale={{
            emptyText: (
              <GlassEmpty text={tab === 'done' ? '暂无已处理的审批' : '暂无待审批任务'} compact />
            ),
          }}
          expandable={{
            expandedRowRender: (row) => (
              <div style={{ padding: '8px 12px', maxWidth: 720 }}>
                <BpmInstanceTimeline instanceId={row.instance_id} showHeader showForm />
              </div>
            ),
          }}
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
        title={
          action
            ? `${action.mode === 'approve' ? '同意' : '拒绝'}：${action.task.instance_title || `实例 #${action.task.instance_id}`}`
            : ''
        }
        open={!!action}
        onOk={() => void onSubmitAction()}
        onCancel={() => setAction(null)}
        confirmLoading={submitting}
        okText={action?.mode === 'approve' ? '确认同意' : '确认拒绝'}
        okButtonProps={action?.mode === 'reject' ? { danger: true } : undefined}
        destroyOnHidden
      >
        <Form form={actionForm} layout="vertical" style={{ marginTop: 12 }}>
          <Form.Item
            name="comment"
            label="审批意见"
            rules={
              action?.mode === 'reject'
                ? [{ required: true, message: '拒绝时必须填写审批意见' }]
                : []
            }
          >
            <Input.TextArea
              rows={3}
              maxLength={512}
              placeholder={action?.mode === 'reject' ? '请说明拒绝原因（必填）' : '可选'}
            />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
