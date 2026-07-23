import { useCallback, useEffect, useState } from 'react'
import { Badge, Button, Card, Drawer, Space, Table, Tabs, Tag, Typography } from 'antd'
import { AuditOutlined, EyeOutlined, ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import {
  BPM_TASK_STATUS_META,
  getTask,
  listDoneTasks,
  listMyCc,
  listTodoTasks,
  readCcRecord,
  type BpmCcRecord,
  type BpmTask,
} from '@/api/bpm'
import BpmInstanceTimeline from '@/components/BpmInstanceTimeline'
import BpmTaskActions from '@/components/BpmTaskActions'
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

export default function BpmTasksPage() {
  const [list, setList] = useState<BpmTask[]>([])
  const [ccList, setCcList] = useState<BpmCcRecord[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useUrlParams<SearchParams>({ page: 1, page_size: 10 })
  // 待办行动作按任务详情返回的动作列表动态渲染：列表加载后批量静默预取（task_id → actions）
  const [actionsMap, setActionsMap] = useState<Record<number, string[] | undefined>>({})
  // 抄送我的：未读数（Tab 徽标）与查看抽屉
  const [ccUnread, setCcUnread] = useState(0)
  const [ccDetail, setCcDetail] = useState<BpmCcRecord | null>(null)
  const userMap = useUserNameMap()

  const tab = params.tab === 'done' ? 'done' : params.tab === 'cc' ? 'cc' : 'todo'

  const loadActions = async (rows: BpmTask[]) => {
    const entries = await Promise.all(
      rows.map(async (t) => {
        const d = await getTask(t.id, true).catch(() => null)
        return [t.id, d?.actions] as const
      }),
    )
    setActionsMap((prev) => {
      const next = { ...prev }
      entries.forEach(([id, acts]) => {
        next[id] = acts
      })
      return next
    })
  }

  const fetchList = async (p: SearchParams) => {
    setLoading(true)
    try {
      if (p.tab === 'cc') {
        const res = await listMyCc({ page: p.page, page_size: p.page_size })
        setCcList(res.list ?? [])
        setTotal(res.total ?? 0)
      } else {
        const query = { page: p.page, page_size: p.page_size }
        const res = p.tab === 'done' ? await listDoneTasks(query) : await listTodoTasks(query)
        setList(res.list ?? [])
        setTotal(res.total ?? 0)
        if (p.tab !== 'done') void loadActions(res.list ?? [])
      }
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

  const refreshUnread = useCallback(() => {
    // 未读计数探针：unread_only + page_size=1，仅取 total；失败静默（后端未上线时不打扰）
    listMyCc({ page: 1, page_size: 1, unread_only: true }, true)
      .then((r) => setCcUnread(r?.total ?? 0))
      .catch(() => {})
  }, [])

  useEffect(() => {
    refreshUnread()
  }, [refreshUnread])

  const refresh = () => {
    void fetchList(params)
    refreshUnread()
  }

  // 查看抄送：打开实例详情抽屉并自动标已读
  const openCc = (row: BpmCcRecord) => {
    setCcDetail(row)
    if (!row.read_at) {
      readCcRecord(row.id, true)
        .then(() => {
          setCcList((prev) =>
            prev.map((r) => (r.id === row.id ? { ...r, read_at: new Date().toISOString() } : r)),
          )
          setCcUnread((n) => Math.max(0, n - 1))
        })
        .catch(() => {})
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
    width: 200,
    render: (v: string, row) => (
      <Space size={4} wrap>
        <Tag>{v || '-'}</Tag>
        {row.delegated_by ? <Tag color="geekblue">委派办理</Tag> : null}
        {row.add_sign_by ? <Tag color="purple">加签</Tag> : null}
      </Space>
    ),
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
      width: 250,
      render: (_, row) => <BpmTaskActions task={row} actions={actionsMap[row.id]} onDone={refresh} />,
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

  const ccColumns: ColumnsType<BpmCcRecord> = [
    {
      title: '审批事项',
      render: (_, row) => (
        <div>
          <Space size={6}>
            {!row.read_at && <Badge status="processing" />}
            <span style={{ fontWeight: row.read_at ? 400 : 600 }}>
              {row.instance_title || `实例 #${row.instance_id}`}
            </span>
          </Space>
          <div>
            <Text type="secondary" style={{ fontSize: 12 }}>
              {displayUserName(userMap, row.initiator_id)} 发起
            </Text>
          </div>
        </div>
      ),
    },
    {
      title: '抄送节点',
      dataIndex: 'node_name',
      width: 160,
      render: (v: string) => <Tag>{v || '-'}</Tag>,
    },
    {
      title: '状态',
      dataIndex: 'read_at',
      width: 100,
      render: (v?: string) => (v ? <Tag>已读</Tag> : <Tag color="processing">未读</Tag>),
    },
    { title: '抄送时间', dataIndex: 'created_at', width: 170, className: 'cell-time', render: formatDateTime },
    {
      title: '操作',
      width: 90,
      render: (_, row) => (
        <Button
          type="link"
          size="small"
          icon={<EyeOutlined />}
          onClick={(e) => {
            e.stopPropagation()
            openCc(row)
          }}
        >
          查看
        </Button>
      ),
    },
  ]

  const pagination = {
    total,
    current: params.page,
    pageSize: params.page_size,
    showSizeChanger: true,
    showTotal: (t: number) => `共 ${t} 条`,
    onChange: (page: number, page_size: number) => setParams({ ...params, page, page_size }),
  }

  return (
    <div className="page-list bpm-tasks-page">
      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="待办中心"
          total={total}
          icon={<AuditOutlined />}
          gradient="linear-gradient(135deg, #60a5fa, #2563eb)"
          glow="rgba(37, 99, 235, 0.4)"
          description="待我审批、我已处理与抄送我的审批；展开行可查看流转时间线"
          extra={
            <Button icon={<ReloadOutlined />} onClick={refresh}>
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
            {
              key: 'cc',
              label: (
                <Badge count={ccUnread} size="small" offset={[10, -2]}>
                  抄送我的
                </Badge>
              ),
            },
          ]}
        />
        {tab === 'cc' ? (
          <Table<BpmCcRecord>
            rowKey="id"
            className="list-table"
            columns={ccColumns}
            dataSource={ccList}
            loading={loading}
            locale={{ emptyText: <GlassEmpty text="暂无抄送我的审批" compact /> }}
            onRow={(row) => ({ onClick: () => openCc(row), style: { cursor: 'pointer' } })}
            pagination={pagination}
          />
        ) : (
          <Table<BpmTask>
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
                  {tab === 'todo' && row.status === 'pending' && (
                    <div style={{ marginBottom: 16 }}>
                      <BpmTaskActions
                        task={row}
                        actions={actionsMap[row.id]}
                        buttonType="default"
                        onDone={refresh}
                      />
                    </div>
                  )}
                  <BpmInstanceTimeline instanceId={row.instance_id} showHeader showForm />
                </div>
              ),
            }}
            pagination={pagination}
          />
        )}
      </Card>

      <Drawer
        title={ccDetail ? ccDetail.instance_title || `实例 #${ccDetail.instance_id}` : '审批详情'}
        open={!!ccDetail}
        onClose={() => setCcDetail(null)}
        width={520}
        destroyOnHidden
      >
        {ccDetail && <BpmInstanceTimeline instanceId={ccDetail.instance_id} showHeader showForm />}
      </Drawer>
    </div>
  )
}
