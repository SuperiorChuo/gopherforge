import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Card, Col, Progress, Rate, Row, Segmented, Space, Statistic, Table, Typography } from 'antd'
import {
  BarChartOutlined, CheckCircleOutlined, ClockCircleOutlined, MessageOutlined, StarOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import { getImStats, type ImAgentStat, type ImStats } from '@/api/im'
import TableToolbar from '@/components/common/TableToolbar'
import GlassEmpty from '@/components/common/GlassEmpty'

const { Text } = Typography

const RANGE_OPTIONS = [
  { label: '近 7 天', value: 7 },
  { label: '近 30 天', value: 30 },
  { label: '近 90 天', value: 90 },
]

export default function ImStatsPage() {
  const navigate = useNavigate()
  const { t } = useTranslation()
  const [days, setDays] = useState(7)
  const [stats, setStats] = useState<ImStats | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    setLoading(true)
    getImStats(days)
      .then((res) => setStats(res.stats))
      .catch(() => message.error(t('获取统计失败')))
      .finally(() => setLoading(false))
  }, [days, t])

  const ratedTotal = stats ? Object.values(stats.score_dist || {}).reduce((a, b) => a + b, 0) : 0

  const columns: ColumnsType<ImAgentStat> = [
    {
      title: t('坐席'),
      dataIndex: 'display_name',
      render: (v: string, row) => v || t('坐席 #{{id}}', { id: row.agent_user_id }),
    },
    { title: t('关闭会话'), dataIndex: 'closed', width: 110, sorter: (a, b) => a.closed - b.closed },
    { title: t('获评数'), dataIndex: 'rated', width: 100 },
    {
      title: t('满意度'),
      dataIndex: 'avg_rating',
      width: 220,
      render: (v?: number) =>
        v ? (
          <Space size={8}>
            <Rate disabled allowHalf value={Math.round(v * 2) / 2} />
            <Text type="secondary">{v.toFixed(2)}</Text>
          </Space>
        ) : (
          <Text type="secondary">—</Text>
        ),
    },
  ]

  return (
    <div className="page-list im-stats-page">
      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="客服统计"
          icon={<BarChartOutlined />}
          gradient="linear-gradient(135deg, #a78bfa, #7c3aed)"
          glow="rgba(124, 58, 237, 0.4)"
          description="会话量与访客满意度（CSAT），口径为区间内关闭的会话"
          extra={
            <Space wrap>
              <Button onClick={() => navigate('/im/low-ratings')}>{t('低分会话')}</Button>
              <Segmented options={RANGE_OPTIONS.map((o) => ({ ...o, label: t(o.label) }))} value={days} onChange={(v) => setDays(Number(v))} />
            </Space>
          }
        />
        <Row gutter={[16, 16]} style={{ marginTop: 8 }}>
          <Col xs={12} md={6}>
            <Card bordered={false} loading={loading}>
              <Statistic title={t('进行中会话')} value={stats?.total_open ?? 0} prefix={<MessageOutlined />} />
            </Card>
          </Col>
          <Col xs={12} md={6}>
            <Card bordered={false} loading={loading}>
              <Statistic title={t('排队中')} value={stats?.total_queued ?? 0} prefix={<ClockCircleOutlined />} />
            </Card>
          </Col>
          <Col xs={12} md={6}>
            <Card bordered={false} loading={loading}>
              <Statistic title={t('已关闭（{{n}}天）', { n: days })} value={stats?.closed_in_range ?? 0} prefix={<CheckCircleOutlined />} />
            </Card>
          </Col>
          <Col xs={12} md={6}>
            <Card bordered={false} loading={loading}>
              <Statistic
                title={t('平均满意度')}
                value={stats?.avg_rating ? stats.avg_rating.toFixed(2) : '—'}
                prefix={<StarOutlined />}
                suffix={stats?.avg_rating ? '/ 5' : undefined}
              />
            </Card>
          </Col>
        </Row>

        <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
          <Col xs={24} md={10}>
            <Card bordered={false} title={t('评分分布')} loading={loading}>
              {ratedTotal === 0 ? (
                <GlassEmpty text="区间内暂无评价" compact />
              ) : (
                <Space direction="vertical" style={{ width: '100%' }} size={8}>
                  {[5, 4, 3, 2, 1].map((score) => {
                    const n = stats?.score_dist?.[String(score)] ?? 0
                    return (
                      <Space key={score} style={{ width: '100%' }}>
                        <Text style={{ width: 34 }}>{t('{{n}} 星', { n: score })}</Text>
                        <Progress
                          style={{ flex: 1, minWidth: 180 }}
                          percent={ratedTotal ? Math.round((n / ratedTotal) * 100) : 0}
                          format={() => `${n}`}
                        />
                      </Space>
                    )
                  })}
                </Space>
              )}
            </Card>
          </Col>
          <Col xs={24} md={14}>
            <Card bordered={false} title={t('坐席明细')} loading={loading}>
              <Table
                rowKey="agent_user_id"
                size="small"
                columns={columns}
                dataSource={stats?.agents ?? []}
                pagination={false}
                locale={{ emptyText: <GlassEmpty text="区间内暂无坐席关闭会话" compact /> }}
               scroll={{ x: 960 }} />
            </Card>
          </Col>
        </Row>
      </Card>
    </div>
  )
}
