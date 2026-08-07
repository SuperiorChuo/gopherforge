import { useCallback, useEffect, useState } from 'react'
import { Button, Card, Select, Space, Table, Tag } from 'antd'
import { ReloadOutlined, SafetyCertificateOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getSecurityEventList, type SecurityEvent } from '@/api/system/security-events'
import GlassEmpty from '@/components/GlassEmpty'
import TableToolbar from '@/components/TableToolbar'
import { formatDateTime } from '@/utils/format'

const RULE_LABELS: Record<string, string> = {
  high_volume_write: '写入操作激增',
  permission_storm: '权限变更风暴',
  failure_burst: '失败操作激增',
}

const SEVERITY_META: Record<string, { label: string; color: string }> = {
  info: { label: '提示', color: 'blue' },
  warning: { label: '警告', color: 'gold' },
  critical: { label: '严重', color: 'red' },
}

export default function SecurityEventsPage() {
  const [list, setList] = useState<SecurityEvent[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [params, setParams] = useState<{ page: number; page_size: number; severity?: string }>({
    page: 1,
    page_size: 10,
  })

  const fetchList = useCallback(async (p: typeof params) => {
    setLoading(true)
    try {
      const res = await getSecurityEventList(p)
      setList(res.list ?? [])
      setTotal(res.total ?? 0)
    } catch {
      setList([])
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void fetchList(params)
  }, [params, fetchList])

  const columns: ColumnsType<SecurityEvent> = [
    {
      title: '级别',
      dataIndex: 'severity',
      width: 90,
      render: (v: string) => {
        const meta = SEVERITY_META[v] ?? SEVERITY_META.info
        return <Tag color={meta.color}>{meta.label}</Tag>
      },
    },
    {
      title: '规则',
      dataIndex: 'rule',
      width: 140,
      render: (v: string) => RULE_LABELS[v] ?? v,
    },
    { title: '操作者', dataIndex: 'actor_id', width: 160, ellipsis: true },
    { title: '摘要', dataIndex: 'summary', ellipsis: true },
    {
      title: '通知',
      dataIndex: 'notified_at',
      width: 100,
      render: (v: string | null) => (v ? <Tag color="green">已通知</Tag> : <Tag>未通知</Tag>),
    },
    {
      title: '发生时间',
      dataIndex: 'occurred_at',
      width: 180,
      render: (v: string) => formatDateTime(v),
    },
  ]

  return (
    <Card
      className="glass-rise"
      title={
        <Space>
          <SafetyCertificateOutlined className="card-title-icon" /> 安全事件
        </Space>
      }
    >
      <TableToolbar
        title="安全事件"
        total={total}
        icon={<SafetyCertificateOutlined />}
        extra={
          <Space>
            <Select
              allowClear
              placeholder="全部级别"
              style={{ width: 140 }}
              onChange={(v?: string) => setParams({ ...params, page: 1, severity: v })}
              options={Object.entries(SEVERITY_META).map(([value, meta]) => ({ value, label: meta.label }))}
            />
            <Button icon={<ReloadOutlined />} onClick={() => fetchList(params)} loading={loading}>
              刷新
            </Button>
          </Space>
        }
      />
      <Table
        rowKey="id"
        className="list-table"
        loading={loading}
        dataSource={list}
        columns={columns}
        locale={{ emptyText: <GlassEmpty text="暂无安全事件" compact /> }}
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
  )
}
