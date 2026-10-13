import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  Card, Col, Row, Statistic, Table, Typography, Space, Button, DatePicker, Tag, Tooltip,
} from 'antd'
import {
  BarChartOutlined, ReloadOutlined, PhoneOutlined, RiseOutlined,
  ClockCircleOutlined, DisconnectOutlined, TrophyOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import dayjs, { type Dayjs } from 'dayjs'
import { message } from '@/utils/feedback'
import {
  getCcReportOverview,
  type CcReportOverview, type CcReportAgentRow, type CcReportQueueRow,
} from '@/api/cc'
import TableToolbar from '@/components/common/TableToolbar'
import GlassEmpty from '@/components/common/GlassEmpty'

const { Text } = Typography
const { RangePicker } = DatePicker

// 秒 → 「M分S秒 / S秒」（比 utils.formatDuration 的分钟粒度更细，通话时长要看秒）
function fmtSec(sec: number, t: TFunction): string {
  if (!sec || sec <= 0) return t('0秒')
  const m = Math.floor(sec / 60)
  const s = sec % 60
  return m > 0 ? t('{{m}}分{{s}}秒', { m, s }) : t('{{s}}秒', { s })
}

export default function CcReportsPage() {
  const { t } = useTranslation()
  const [range, setRange] = useState<[Dayjs, Dayjs]>([dayjs().startOf('day'), dayjs().startOf('day')])
  const [data, setData] = useState<CcReportOverview | null>(null)
  const [loading, setLoading] = useState(false)

  const fetchReport = useCallback(async (r: [Dayjs, Dayjs]) => {
    setLoading(true)
    try {
      const res = await getCcReportOverview({
        from: r[0].format('YYYY-MM-DD'),
        to: r[1].format('YYYY-MM-DD'),
      })
      setData(res)
    } catch {
      message.error(t('获取呼叫报表失败'))
    } finally {
      setLoading(false)
    }
  }, [t])

  useEffect(() => { fetchReport(range) }, [fetchReport, range])

  // 24 小时柱条的最高值（呼入量），用于算相对宽度百分比
  const hourMax = Math.max(1, ...(data?.hour_buckets ?? []).map((b) => b.inbound))

  const agentColumns: ColumnsType<CcReportAgentRow> = [
    {
      title: t('排名'),
      key: 'rank',
      width: 70,
      render: (_, __, idx) => {
        const medal = ['#ffd700', '#c0c0c0', '#cd7f32'][idx]
        return medal
          ? <TrophyOutlined style={{ color: medal, fontSize: 16 }} />
          : <span className="cell-dim">{idx + 1}</span>
      },
    },
    { title: t('分机'), dataIndex: 'agent_ext', width: 120, render: (v: string) => <span className="cell-mono">{v}</span> },
    { title: t('接听数'), dataIndex: 'answered', width: 100, render: (v: number) => <Tag color="geekblue" variant="filled">{v}</Tag> },
    { title: t('总时长'), dataIndex: 'total_talk_sec', width: 120, render: (v: number) => fmtSec(v, t) },
    { title: t('均长'), dataIndex: 'avg_talk_sec', render: (v: number) => fmtSec(v, t) },
  ]

  const queueColumns: ColumnsType<CcReportQueueRow> = [
    { title: t('队列'), dataIndex: 'queue', render: (v: string) => <Tag variant="filled">{v}</Tag> },
    { title: t('进队数'), dataIndex: 'entered', width: 110 },
    { title: t('接通'), dataIndex: 'answered', width: 100, render: (v: number) => <Tag color="green" variant="filled">{v}</Tag> },
    {
      title: t('放弃'),
      dataIndex: 'abandoned',
      width: 100,
      render: (v: number) => (v > 0 ? <Tag color="volcano" variant="filled">{v}</Tag> : <span className="cell-dim">0</span>),
    },
  ]

  return (
    <div className="page-list cc-reports-page">
      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="呼叫报表"
          icon={<BarChartOutlined />}
          gradient="linear-gradient(135deg, #60a5fa, #6366f1)"
          glow="rgba(99, 102, 241, 0.4)"
          description="话务量、接通率、时段分布与坐席/队列绩效"
          extra={
            <Space size={12}>
              <RangePicker
                allowClear={false}
                value={range}
                onChange={(v) => { if (v && v[0] && v[1]) setRange([v[0], v[1]]) }}
                presets={[
                  { label: t('今天'), value: [dayjs().startOf('day'), dayjs().startOf('day')] },
                  { label: t('近 7 天'), value: [dayjs().startOf('day').subtract(6, 'day'), dayjs().startOf('day')] },
                  { label: t('近 30 天'), value: [dayjs().startOf('day').subtract(29, 'day'), dayjs().startOf('day')] },
                ]}
              />
              <Button icon={<ReloadOutlined />} onClick={() => fetchReport(range)}>{t('刷新')}</Button>
            </Space>
          }
        />

        {/* 四张核心指标卡 */}
        <Row gutter={[16, 16]} style={{ marginTop: 8 }}>
          <Col xs={12} md={6}>
            <Card bordered={false} loading={loading}>
              <Statistic
                title={t('接通率')}
                value={data?.answer_rate ?? 0}
                precision={1}
                suffix="%"
                prefix={<RiseOutlined />}
                valueStyle={{ color: (data?.answer_rate ?? 0) >= 80 ? 'var(--c-success)' : (data?.answer_rate ?? 0) >= 50 ? 'var(--c-warning)' : 'var(--c-error-strong)' }}
              />
              <Text type="secondary" style={{ fontSize: 13 }}>{t('接通 {{answered}} / 总 {{total}}', { answered: data?.answered ?? 0, total: data?.total ?? 0 })}</Text>
            </Card>
          </Col>
          <Col xs={12} md={6}>
            <Card bordered={false} loading={loading}>
              <Statistic title={t('总话务')} value={data?.total ?? 0} prefix={<PhoneOutlined />} />
              <Text type="secondary" style={{ fontSize: 13 }}>{t('呼入 {{n}} · 呼出 {{m}}', { n: data?.total_inbound ?? 0, m: data?.total_outbound ?? 0 })}</Text>
            </Card>
          </Col>
          <Col xs={12} md={6}>
            <Card bordered={false} loading={loading}>
              <Statistic title={t('平均通话')} value={fmtSec(data?.avg_talk_sec ?? 0, t)} prefix={<ClockCircleOutlined />} />
              <Text type="secondary" style={{ fontSize: 13 }}>{t('平均等待 {{time}}', { time: fmtSec(data?.avg_wait_sec ?? 0, t) })}</Text>
            </Card>
          </Col>
          <Col xs={12} md={6}>
            <Card bordered={false} loading={loading}>
              <Statistic
                title={t('放弃数')}
                value={data?.abandoned ?? 0}
                prefix={<DisconnectOutlined />}
                valueStyle={(data?.abandoned ?? 0) > 0 ? { color: 'var(--c-error-strong)' } : undefined}
              />
              <Text type="secondary" style={{ fontSize: 13 }}>{t('呼入未接通')}</Text>
            </Card>
          </Col>
        </Row>

        {/* 时段分布：24 小时横向柱条（纯 div + width%，深空玻璃风） */}
        <Card bordered={false} title={t('时段分布（呼入量 / 接通量）')} style={{ marginTop: 16 }} loading={loading}>
          {(data?.hour_buckets ?? []).every((b) => b.inbound === 0) ? (
            <GlassEmpty text="该时段无呼入" compact />
          ) : (
            <div className="cc-hour-chart">
              {(data?.hour_buckets ?? []).map((b) => {
                const inPct = Math.round((b.inbound / hourMax) * 100)
                const ansPct = b.inbound > 0 ? Math.round((b.answered / b.inbound) * 100) : 0
                return (
                  <div key={b.hour} className="cc-hour-row">
                    <span className="cc-hour-label">{String(b.hour).padStart(2, '0')}:00</span>
                    <Tooltip title={t('呼入 {{in}} · 接通 {{ans}}（接通率 {{pct}}%）', { in: b.inbound, ans: b.answered, pct: ansPct })}>
                      <div className="cc-hour-track">
                        <div
                          className="cc-hour-bar cc-hour-bar-in"
                          style={{ width: `${inPct}%` }}
                        >
                          {/* 接通量叠加在呼入柱条内（相对呼入的比例） */}
                          <div className="cc-hour-bar-ans" style={{ width: `${ansPct}%` }} />
                        </div>
                      </div>
                    </Tooltip>
                    <span className="cc-hour-num">{b.inbound}</span>
                  </div>
                )
              })}
            </div>
          )}
        </Card>

        <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
          <Col xs={24} lg={13}>
            <Card bordered={false} title={t('坐席榜（按接听数）')} loading={loading}>
              <Table
                rowKey="agent_ext"
                scroll={{ x: 800 }}
                size="small"
                columns={agentColumns}
                dataSource={data?.agent_leaderboard ?? []}
                pagination={false}
                locale={{ emptyText: <GlassEmpty text="该时段无坐席接听记录" compact /> }}
              />
            </Card>
          </Col>
          <Col xs={24} lg={11}>
            <Card bordered={false} title={t('队列榜（按进队数）')} loading={loading}>
              <Table
                rowKey="queue"
                scroll={{ x: 800 }}
                size="small"
                columns={queueColumns}
                dataSource={data?.queue_leaderboard ?? []}
                pagination={false}
                locale={{ emptyText: <GlassEmpty text="该时段无队列话务" compact /> }}
              />
            </Card>
          </Col>
        </Row>
      </Card>
    </div>
  )
}
