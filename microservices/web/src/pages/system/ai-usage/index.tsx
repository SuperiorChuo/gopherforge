import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Card,
  Row,
  Col,
  Statistic,
  Table,
  Select,
  Tag,
  Spin,
} from 'antd'
import {
  LineChartOutlined,
  DollarOutlined,
  ThunderboltOutlined,
  CheckCircleOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import { formatDateTime } from '@/utils/format'
import {
  getDailyStats,
  getTopUsers,
  getUsageSummary,
  getAIUsageLogs,
  type DailyStats,
  type UserStats,
  type AIUsageLog,
} from '@/api/ai/usage'
import GlassEmpty from '@/components/common/GlassEmpty'
import TableToolbar from '@/components/common/TableToolbar'

// 轻量趋势图：纯 SVG 折线，避免引入图表库（仓库未装 echarts）。
// 三条线各自归一化到 [0,1]，右侧图例标注真实峰值。
function TrendChart({ data }: { data: DailyStats[] }) {
  const { t } = useTranslation()
  if (!data.length) {
    return <GlassEmpty text="暂无数据" compact />
  }
  const width = 900
  const height = 320
  const padX = 48
  const padY = 24
  const innerW = width - padX * 2
  const innerH = height - padY * 2
  const n = data.length

  const series = [
    { key: 'total_requests', label: '请求数', color: 'var(--c-primary)', values: data.map((d) => d.total_requests) },
    { key: 'total_tokens', label: 'Token', color: 'var(--c-success)', values: data.map((d) => d.total_tokens) },
    { key: 'total_cost', label: '费用(元)', color: 'var(--c-warning)', values: data.map((d) => d.total_cost) },
  ]

  const xOf = (i: number) => (n === 1 ? padX + innerW / 2 : padX + (innerW * i) / (n - 1))
  const pathFor = (values: number[]) => {
    const max = Math.max(...values, 1)
    return values
      .map((v, i) => {
        const x = xOf(i)
        const y = padY + innerH - (innerH * v) / max
        return `${i === 0 ? 'M' : 'L'}${x.toFixed(1)},${y.toFixed(1)}`
      })
      .join(' ')
  }

  return (
    <div style={{ overflowX: 'auto' }}>
      <svg width={width} height={height} style={{ maxWidth: '100%' }}>
        {/* 横向网格线 */}
        {[0, 0.25, 0.5, 0.75, 1].map((t) => {
          const y = padY + innerH * t
          return <line key={t} x1={padX} y1={y} x2={width - padX} y2={y} stroke="rgba(148,163,184,0.2)" />
        })}
        {/* 折线 */}
        {series.map((s) => (
          <path key={s.key} d={pathFor(s.values)} fill="none" stroke={s.color} strokeWidth={2} />
        ))}
        {/* X 轴日期（最多标 8 个，避免拥挤） */}
        {data.map((d, i) => {
          const step = Math.ceil(n / 8)
          if (i % step !== 0 && i !== n - 1) return null
          return (
            <text key={d.date} x={xOf(i)} y={height - 4} fontSize={11} fill="var(--text-secondary)" textAnchor="middle">
              {d.date.slice(5)}
            </text>
          )
        })}
      </svg>
      <div className="ai-usage-trend-legend" style={{ paddingLeft: padX }}>
        {series.map((s) => (
          <span key={s.key} style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontSize: 12 }}>
            <span style={{ width: 12, height: 3, background: s.color, display: 'inline-block' }} />
            {t(s.label)}
            <span style={{ color: 'var(--text-secondary)' }}>{t('峰值')} {Math.max(...s.values, 0).toLocaleString()}</span>
          </span>
        ))}
      </div>
    </div>
  )
}

export default function AIUsageReport() {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [days, setDays] = useState(7)
  const [daily, setDaily] = useState<DailyStats[]>([])
  const [summary, setSummary] = useState({
    total_requests: 0,
    success_requests: 0,
    total_tokens: 0,
    total_cost: 0,
  })
  const [topUsers, setTopUsers] = useState<UserStats[]>([])
  const [logs, setLogs] = useState<AIUsageLog[]>([])
  const [logsTotal, setLogsTotal] = useState(0)
  const [logsPage, setLogsPage] = useState(1)
  const [logsPageSize, setLogsPageSize] = useState(10)

  const loadStats = async () => {
    setLoading(true)
    try {
      const end = new Date()
      const start = new Date(Date.now() - days * 86400000)
      const fmt = (d: Date) => d.toISOString().slice(0, 10)
      const [dailyRes, topUsersRes, summaryRes] = await Promise.all([
        getDailyStats({ days }),
        getTopUsers({ days: 30, limit: 10 }),
        getUsageSummary({ start_time: fmt(start), end_time: fmt(end) }),
      ])
      setDaily(dailyRes || [])
      setTopUsers(topUsersRes || [])
      setSummary(
        summaryRes || { total_requests: 0, success_requests: 0, total_tokens: 0, total_cost: 0 },
      )
    } catch (e) {
      message.error(e instanceof Error ? e.message : t('加载失败'))
    } finally {
      setLoading(false)
    }
  }

  const loadLogs = async () => {
    try {
      const res = await getAIUsageLogs({ page: logsPage, page_size: logsPageSize })
      setLogs(res.list || [])
      setLogsTotal(res.total || 0)
    } catch (e) {
      message.error(e instanceof Error ? e.message : t('加载调用记录失败'))
    }
  }

  // 列表页惯用式：按参数键重拉，load 函数身份刻意排除。
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    void loadStats()
  }, [days])

  // 同上：按分页参数重拉。
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    void loadLogs()
  }, [logsPage, logsPageSize])

  const topUsersColumns: ColumnsType<UserStats> = [
    { title: t('排名'), width: 60, render: (_, __, i) => i + 1 },
    { title: t('用户'), dataIndex: 'username', width: 120 },
    { title: t('请求数'), dataIndex: 'total_requests', width: 100, render: (v: number) => v.toLocaleString() },
    { title: 'Token', dataIndex: 'total_tokens', width: 120, responsive: ['sm'], render: (v: number) => `${(v / 1000).toFixed(1)}K` },
    { title: t('费用'), dataIndex: 'total_cost', width: 100, responsive: ['md'], render: (v: number) => `¥${v.toFixed(2)}` },
  ]

  const logsColumns: ColumnsType<AIUsageLog> = [
    { title: t('时间'), dataIndex: 'created_at', width: 160, render: formatDateTime },
    { title: t('模型'), dataIndex: 'model_name', width: 140 },
    { title: t('用户'), dataIndex: 'username', width: 90, responsive: ['md'], render: (v?: string) => v || '-' },
    { title: t('操作'), dataIndex: 'operation', width: 80, responsive: ['sm'], render: (v: string) => <Tag color="blue">{v}</Tag> },
    {
      title: 'Token',
      width: 100,
      responsive: ['md'],
      render: (_, r) => (r.total_tokens ? r.total_tokens.toLocaleString() : '-'),
    },
    { title: t('费用'), dataIndex: 'cost', width: 90, responsive: ['md'], render: (v: number) => `¥${v.toFixed(4)}` },
    { title: t('耗时'), dataIndex: 'latency_ms', width: 80, responsive: ['lg'], render: (v?: number) => (v ? `${v}ms` : '-') },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 80,
      render: (s: string) => (
        <Tag color={s === 'success' ? 'success' : 'error'}>{s === 'success' ? t('成功') : t('失败')}</Tag>
      ),
    },
  ]

  const successRate =
    summary.total_requests > 0
      ? ((summary.success_requests / summary.total_requests) * 100).toFixed(1)
      : '0'

  return (
    <div className="page-list ai-usage-page">
      <TableToolbar
        title="AI 用量统计"
        extra={
          <Select value={days} onChange={setDays} style={{ width: 120 }} aria-label={t('统计天数')}>
            <Select.Option value={7}>{t('最近 7 天')}</Select.Option>
            <Select.Option value={14}>{t('最近 14 天')}</Select.Option>
            <Select.Option value={30}>{t('最近 30 天')}</Select.Option>
          </Select>
        }
      />
      <Spin spinning={loading}>
        <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
          <Col xs={12} md={6}>
            <Card>
              <Statistic title={t('总请求数')} value={summary.total_requests} prefix={<ThunderboltOutlined />} valueStyle={{ color: 'var(--c-success)' }} />
            </Card>
          </Col>
          <Col xs={12} md={6}>
            <Card>
              <Statistic title={t('成功率')} value={successRate} suffix="%" prefix={<CheckCircleOutlined />} valueStyle={{ color: 'var(--c-primary)' }} />
            </Card>
          </Col>
          <Col xs={12} md={6}>
            <Card>
              <Statistic title={t('Token 用量')} value={(summary.total_tokens / 1000).toFixed(1)} suffix="K" prefix={<LineChartOutlined />} />
            </Card>
          </Col>
          <Col xs={12} md={6}>
            <Card>
              <Statistic title={t('总费用')} value={summary.total_cost} precision={2} prefix={<DollarOutlined />} suffix={t('元')} valueStyle={{ color: 'var(--c-error-strong)' }} />
            </Card>
          </Col>
        </Row>

        <Card title={t('用量趋势')} style={{ marginBottom: 16 }}>
          <TrendChart data={daily} />
        </Card>

        <Row gutter={[16, 16]}>
          <Col xs={24} lg={12}>
            <Card title={t('Top 用户排行（最近 30 天）')}>
              <Table
                dataSource={topUsers}
                columns={topUsersColumns}
                rowKey="user_id"
                pagination={false}
                size="small"
                scroll={{ x: 520 }}
                locale={{ emptyText: <GlassEmpty text="暂无数据" compact /> }}
              />
            </Card>
          </Col>
          <Col xs={24} lg={12}>
            <Card title={t('最近调用记录')}>
              <Table
                dataSource={logs}
                columns={logsColumns}
                rowKey="id"
                size="small"
                scroll={{ x: 860 }}
                locale={{ emptyText: <GlassEmpty text="暂无数据" compact /> }}
                pagination={{
                  current: logsPage,
                  pageSize: logsPageSize,
                  total: logsTotal,
                  showSizeChanger: false,
                  onChange: (p, ps) => {
                    setLogsPage(p)
                    setLogsPageSize(ps)
                  },
                }}
              />
            </Card>
          </Col>
        </Row>
      </Spin>
    </div>
  )
}
