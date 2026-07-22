import { useEffect, useState } from 'react'
import { Card, Col, Row, Skeleton, Statistic, Table, Tooltip, Typography } from 'antd'
import { getBpmStats, type BpmStats } from '@/api/bpm'

const { Text } = Typography

const STATUS_CARDS: { key: string; label: string; color?: string }[] = [
  { key: 'running', label: '审批中', color: '#1677ff' },
  { key: 'approved', label: '已通过', color: '#52c41a' },
  { key: 'rejected', label: '已拒绝', color: '#ff4d4f' },
  { key: 'canceled', label: '已撤销' },
  { key: 'suspended', label: '已挂起', color: '#faad14' },
]

/**
 * 审批统计面板（收官项，仅平台管理员）：状态分布 / 近 30 天发起趋势 /
 * 按定义通过率与均时长 / 节点瓶颈。趋势用纯 div 迷你柱状，不引图表库。
 */
export default function BpmStatsPanel() {
  const [stats, setStats] = useState<BpmStats | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let alive = true
    getBpmStats()
      .then((d) => {
        if (alive) setStats(d)
      })
      .catch(() => {})
      .finally(() => {
        if (alive) setLoading(false)
      })
    return () => {
      alive = false
    }
  }, [])

  if (loading) return <Skeleton active paragraph={{ rows: 6 }} />
  if (!stats) return <Text type="secondary">统计数据加载失败</Text>

  const maxTrend = Math.max(1, ...stats.trend.map((t) => t.count))

  return (
    <div>
      <Row gutter={[12, 12]}>
        {STATUS_CARDS.map((c) => (
          <Col key={c.key} xs={12} sm={8} lg={4}>
            <Card size="small">
              <Statistic
                title={c.label}
                value={stats.status_counts[c.key] ?? 0}
                valueStyle={c.color ? { color: c.color } : undefined}
              />
            </Card>
          </Col>
        ))}
      </Row>

      <Card size="small" title="近 30 天发起趋势" style={{ marginTop: 12 }}>
        <div style={{ display: 'flex', alignItems: 'flex-end', gap: 3, height: 80 }}>
          {stats.trend.map((t) => (
            <Tooltip key={t.date} title={`${t.date}：${t.count} 件`}>
              <div
                style={{
                  flex: 1,
                  minWidth: 4,
                  height: `${Math.max(4, (t.count / maxTrend) * 100)}%`,
                  borderRadius: 2,
                  background:
                    t.count > 0 ? 'linear-gradient(180deg, #a78bfa, #7c3aed)' : 'rgba(128,128,128,0.15)',
                }}
              />
            </Tooltip>
          ))}
        </div>
      </Card>

      <Card size="small" title="按流程定义" style={{ marginTop: 12 }}>
        <Table
          size="small"
          rowKey="definition_key"
          dataSource={stats.definitions}
          pagination={false}
          columns={[
            {
              title: '流程',
              dataIndex: 'name',
              render: (v: string | undefined, row) => (
                <span>
                  {v || row.definition_key}{' '}
                  <Text type="secondary" className="cell-mono" style={{ fontSize: 12 }}>
                    {row.definition_key}
                  </Text>
                </span>
              ),
            },
            { title: '发起', dataIndex: 'total', width: 70 },
            { title: '通过', dataIndex: 'approved', width: 70 },
            { title: '拒绝', dataIndex: 'rejected', width: 70 },
            { title: '在途', dataIndex: 'running', width: 70 },
            {
              title: '通过率',
              width: 90,
              render: (_, row) => {
                const done = row.approved + row.rejected
                return done > 0 ? `${Math.round((row.approved / done) * 100)}%` : '—'
              },
            },
            {
              title: '平均耗时',
              dataIndex: 'avg_hours',
              width: 100,
              render: (v: number) => (v > 0 ? `${v} 小时` : '—'),
            },
          ]}
        />
      </Card>

      <Card size="small" title="节点瓶颈（平均处理时长 Top 10）" style={{ marginTop: 12 }}>
        <Table
          size="small"
          rowKey="node_name"
          dataSource={stats.node_bottlenecks}
          pagination={false}
          columns={[
            { title: '节点', dataIndex: 'node_name' },
            { title: '已处理任务', dataIndex: 'acted', width: 110 },
            {
              title: '平均处理时长',
              dataIndex: 'avg_hours',
              width: 120,
              render: (v: number) => `${v} 小时`,
            },
          ]}
        />
      </Card>
    </div>
  )
}
