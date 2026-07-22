import { useEffect, useState } from 'react'
import { Button, Card, Drawer, Popconfirm, Select, Space, Table, Tag, Typography } from 'antd'
import {
  EyeOutlined,
  ReloadOutlined,
  RollbackOutlined,
  SolutionOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import {
  BPM_INSTANCE_STATUS_META,
  cancelInstance,
  listMyInstances,
  type BpmInstance,
} from '@/api/bpm'
import BpmInstanceTimeline from '@/components/BpmInstanceTimeline'
import TableToolbar from '@/components/TableToolbar'
import GlassEmpty from '@/components/GlassEmpty'
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
  const [list, setList] = useState<BpmInstance[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>(
    { page: 1, page_size: 10 },
    ['page', 'page_size'],
  )
  const [detail, setDetail] = useState<BpmInstance | null>(null)

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      const res = await listMyInstances(p)
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
      width: 160,
      render: (_, row) => (
        <Space size={0} className="table-actions">
          <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => setDetail(row)}>
            详情
          </Button>
          {row.status === 'running' && (
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
        </Space>
      ),
    },
  ]

  return (
    <div className="page-list bpm-instances-page">
      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="我发起的审批"
          total={total}
          icon={<SolutionOutlined />}
          gradient="linear-gradient(135deg, #2dd4bf, #0d9488)"
          glow="rgba(13, 148, 136, 0.4)"
          description="我发起的审批实例与流转进度；无人审批前可撤销"
          extra={
            <Space wrap>
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
          detail?.status === 'running' ? (
            <Popconfirm
              title="撤销该审批？"
              description="仅在尚无人审批时可撤销"
              onConfirm={() => detail && void onCancel(detail)}
            >
              <Button danger size="small" icon={<RollbackOutlined />}>
                撤销
              </Button>
            </Popconfirm>
          ) : null
        }
      >
        {detail && <BpmInstanceTimeline instanceId={detail.id} showHeader showForm />}
      </Drawer>
    </div>
  )
}
