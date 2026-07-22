import { useEffect, useState } from 'react'
import {
  Button,
  Card,
  Drawer,
  Input,
  Modal,
  Popconfirm,
  Segmented,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd'
import {
  EditOutlined,
  EyeOutlined,
  ReloadOutlined,
  RollbackOutlined,
  SolutionOutlined,
  StopOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import {
  BPM_INSTANCE_STATUS_META,
  cancelInstance,
  getTask,
  listAllInstances,
  listMyInstances,
  listTodoTasks,
  terminateInstance,
  type BpmInstance,
} from '@/api/bpm'
import BpmInstanceTimeline from '@/components/BpmInstanceTimeline'
import BpmResubmitModal from '@/components/BpmResubmitModal'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
import { useAppSelector } from '@/hooks/store'
import { displayUserName, useUserNameMap } from '@/hooks/useUserNameMap'
import { useUrlParams } from '@/hooks/useUrlParams'
import { formatDateTime } from '@/utils/format'

interface SearchParams {
  status?: string
  page: number
  page_size: number
}

const STATUS_OPTIONS = Object.entries(BPM_INSTANCE_STATUS_META).map(([value, meta]) => ({
  value,
  label: meta.label,
}))

export default function BpmInstancesPage() {
  const isPlatform = !!useAppSelector((s) => s.auth.userInfo)?.is_platform_admin
  const userMap = useUserNameMap()
  const [list, setList] = useState<BpmInstance[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  // M3 管理视图：平台管理员可切「全部实例」，配合终止动作处置挂起/异常实例
  const [scope, setScope] = useState<'my' | 'all'>('my')
  const [params, setParams] = useUrlParams<SearchParams>(
    { page: 1, page_size: 10 },
    ['page', 'page_size'],
  )
  const [detail, setDetail] = useState<BpmInstance | null>(null)
  const [terminateFor, setTerminateFor] = useState<BpmInstance | null>(null)
  const [terminateReason, setTerminateReason] = useState('')
  const [terminating, setTerminating] = useState(false)
  // 被退回的实例（有 pending 重提任务的）：instance_id → 可重提。
  // 识别手法：我的待办里落在这些实例上的任务，其详情动作列表含 resubmit（契约唯一权威信号）。
  const [resubmitable, setResubmitable] = useState<Record<number, boolean>>({})
  const [resubmitFor, setResubmitFor] = useState<number | null>(null)

  const detectResubmitable = async (rows: BpmInstance[]) => {
    const running = new Set(rows.filter((i) => i.status === 'running').map((i) => i.id))
    if (!running.size) {
      setResubmitable({})
      return
    }
    try {
      const todo = await listTodoTasks({ page: 1, page_size: 100 }, true)
      const candidates = (todo?.list ?? []).filter((t) => running.has(t.instance_id))
      const map: Record<number, boolean> = {}
      await Promise.all(
        candidates.map(async (t) => {
          const d = await getTask(t.id, true).catch(() => null)
          if (d?.actions?.includes('resubmit')) map[t.instance_id] = true
        }),
      )
      setResubmitable(map)
    } catch {
      setResubmitable({})
    }
  }

  const fetchList = async (p: SearchParams, sc: 'my' | 'all' = scope) => {
    setLoading(true)
    try {
      const res = sc === 'all' ? await listAllInstances(p) : await listMyInstances(p)
      setList(res.list ?? [])
      setTotal(res.total ?? 0)
      if (sc === 'my') void detectResubmitable(res.list ?? [])
      else setResubmitable({})
    } catch {
      // 错误提示由 request 拦截器统一弹出
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void fetchList(params, scope)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [params, scope])

  const onTerminate = async () => {
    if (!terminateFor) return
    if (!terminateReason.trim()) {
      message.warning('请填写终止原因')
      return
    }
    setTerminating(true)
    try {
      await terminateInstance(terminateFor.id, terminateReason.trim())
      message.success('已终止')
      setTerminateFor(null)
      setTerminateReason('')
      setDetail(null)
      void fetchList(params)
    } catch {
      // 拦截器统一提示
    } finally {
      setTerminating(false)
    }
  }

  const onCancel = async (row: BpmInstance) => {
    try {
      await cancelInstance(row.id)
      message.success('已撤销')
      setDetail(null)
      void fetchList(params)
    } catch {
      // 已有人审批时后端会拒绝撤销，拦截器统一提示原因
    }
  }

  const columns: ColumnsType<BpmInstance> = [
    {
      title: '审批事项',
      dataIndex: 'title',
      render: (v: string, row) => (
        <div>
          <span style={{ fontWeight: 500 }}>{v}</span>
          <div>
            <Typography.Text type="secondary" style={{ fontSize: 12 }} className="cell-mono">
              {row.definition_key}
            </Typography.Text>
          </div>
        </div>
      ),
    },
    {
      title: '业务类型',
      dataIndex: 'biz_type',
      width: 130,
      render: (v?: string) => (v ? <Tag variant="filled">{v}</Tag> : <span className="cell-muted">—</span>),
    },
    ...(scope === 'all'
      ? [
          {
            title: '发起人',
            dataIndex: 'initiator_id',
            width: 110,
            render: (v: number, row: BpmInstance) =>
              row.initiator_name || displayUserName(userMap, v),
          } as ColumnsType<BpmInstance>[number],
        ]
      : []),
    {
      title: '状态',
      dataIndex: 'status',
      width: 110,
      render: (v: string) => {
        const meta = BPM_INSTANCE_STATUS_META[v] ?? { label: v, color: 'default' }
        return <Tag color={meta.color}>{meta.label}</Tag>
      },
    },
    { title: '发起时间', dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: '完成时间',
      dataIndex: 'finished_at',
      width: 170,
      className: 'cell-time',
      render: (v?: string) => (v ? formatDateTime(v) : <span className="cell-muted">—</span>),
    },
    {
      title: '操作',
      width: scope === 'all' ? 150 : 200,
      render: (_, row) => (
        <Space size={0} className="table-actions">
          <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => setDetail(row)}>
            详情
          </Button>
          {scope === 'my' && row.status === 'running' && resubmitable[row.id] && (
            <Button
              type="link"
              size="small"
              icon={<EditOutlined />}
              onClick={() => setResubmitFor(row.id)}
            >
              重新提交
            </Button>
          )}
          {scope === 'my' && row.status === 'running' && (
            <Popconfirm
              title="撤销该审批？"
              description="仅在尚无人审批时可撤销"
              onConfirm={() => void onCancel(row)}
            >
              <Button type="link" size="small" danger icon={<RollbackOutlined />}>
                撤销
              </Button>
            </Popconfirm>
          )}
          {isPlatform && (row.status === 'running' || row.status === 'suspended') && (
            <Button
              type="link"
              size="small"
              danger
              icon={<StopOutlined />}
              onClick={() => {
                setTerminateReason('')
                setTerminateFor(row)
              }}
            >
              终止
            </Button>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div className="page-list bpm-instances-page">
      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title={scope === 'all' ? '全部审批实例' : '我发起的审批'}
          total={total}
          icon={<SolutionOutlined />}
          gradient="linear-gradient(135deg, #2dd4bf, #0d9488)"
          glow="rgba(13, 148, 136, 0.4)"
          description={
            scope === 'all'
              ? '租户内全部审批实例（管理视图）；审批中/已挂起的实例可强制终止'
              : '我发起的审批实例与流转进度；无人审批前可撤销'
          }
          extra={
            <Space wrap>
              {isPlatform && (
                <Segmented
                  value={scope}
                  options={[
                    { label: '我发起的', value: 'my' },
                    { label: '全部（管理）', value: 'all' },
                  ]}
                  onChange={(v) => {
                    setScope(v as 'my' | 'all')
                    setParams({ ...params, page: 1 })
                  }}
                />
              )}
              <Select
                placeholder="状态"
                allowClear
                style={{ width: 120 }}
                options={STATUS_OPTIONS}
                value={params.status}
                onChange={(v) => setParams({ ...params, page: 1, status: v })}
              />
              <Button icon={<ReloadOutlined />} onClick={() => void fetchList(params)}>
                刷新
              </Button>
            </Space>
          }
        />
        <Table
          rowKey="id"
          className="list-table"
          columns={columns}
          dataSource={list}
          loading={loading}
          locale={{ emptyText: <GlassEmpty text="暂无发起的审批" compact /> }}
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

      <Drawer
        title={detail ? detail.title : '审批详情'}
        open={!!detail}
        onClose={() => setDetail(null)}
        width={520}
        destroyOnHidden
        extra={
          detail && (detail.status === 'running' || detail.status === 'suspended') ? (
            <Space size={8}>
              {scope === 'my' && detail.status === 'running' && resubmitable[detail.id] && (
                <Button
                  type="primary"
                  size="small"
                  icon={<EditOutlined />}
                  onClick={() => setResubmitFor(detail.id)}
                >
                  重新提交
                </Button>
              )}
              {scope === 'my' && detail.status === 'running' && (
                <Popconfirm
                  title="撤销该审批？"
                  description="仅在尚无人审批时可撤销"
                  onConfirm={() => detail && void onCancel(detail)}
                >
                  <Button danger size="small" icon={<RollbackOutlined />}>
                    撤销
                  </Button>
                </Popconfirm>
              )}
              {isPlatform && (
                <Button
                  danger
                  size="small"
                  icon={<StopOutlined />}
                  onClick={() => {
                    setTerminateReason('')
                    setTerminateFor(detail)
                  }}
                >
                  终止
                </Button>
              )}
            </Space>
          ) : null
        }
      >
        {detail && <BpmInstanceTimeline instanceId={detail.id} showHeader showForm />}
      </Drawer>

      <BpmResubmitModal
        instanceId={resubmitFor}
        open={resubmitFor !== null}
        onClose={() => setResubmitFor(null)}
        onDone={() => {
          setDetail(null)
          void fetchList(params)
        }}
      />

      <Modal
        title={`终止审批：${terminateFor?.title ?? ''}`}
        open={!!terminateFor}
        okText="确认终止"
        okButtonProps={{ danger: true, loading: terminating }}
        onOk={() => void onTerminate()}
        onCancel={() => {
          setTerminateFor(null)
          setTerminateReason('')
        }}
        destroyOnHidden
      >
        <Space direction="vertical" size={8} style={{ width: '100%' }}>
          <Typography.Text type="secondary">
            管理员强制结束该流程：全部待办作废，业务侧按“已撤销”回滚；操作不可恢复。
          </Typography.Text>
          <Input.TextArea
            rows={3}
            maxLength={200}
            placeholder="终止原因（必填，将记入流转时间线）"
            value={terminateReason}
            onChange={(e) => setTerminateReason(e.target.value)}
          />
        </Space>
      </Modal>
    </div>
  )
}
