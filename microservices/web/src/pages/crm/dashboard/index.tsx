import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Card, Col, Progress, Row, Segmented, Space, Statistic, Table, Tag, Typography } from 'antd'
import {
  FundOutlined, TeamOutlined, RiseOutlined, DollarOutlined, PayCircleOutlined,
  UserAddOutlined, SwapOutlined, AlertOutlined, TrophyOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import { fenToYuan, getCrmStats, STAGE_META, type CrmOwnerStat, type CrmStats } from '@/api/crm'
import TableToolbar from '@/components/common/TableToolbar'
import GlassEmpty from '@/components/common/GlassEmpty'
import i18n from '@/i18n/init'

const { Text } = Typography

// 漏斗层渐变（与商机管道列头同色系）
const FUNNEL_GRADIENTS: Record<string, string> = {
  initial: 'linear-gradient(90deg, #8c8c8c, #bfbfbf)',
  demand: 'linear-gradient(90deg, #1677ff, #36cfc9)',
  proposal: 'linear-gradient(90deg, #722ed1, #b37feb)',
  negotiation: 'linear-gradient(90deg, #f5c518, #fa8c16)',
}

const RANGE_OPTIONS = [
  { label: '近 30 天', value: 30 },
  { label: '近 90 天', value: 90 },
  { label: '近一年', value: 365 },
]

// 漏斗只画进行中 4 阶段（won/lost 单独展示成果）
const FUNNEL_STAGES = ['initial', 'demand', 'proposal', 'negotiation']

export default function CrmDashboardPage() {
  const { t } = useTranslation()
  const [days, setDays] = useState(30)
  const [stats, setStats] = useState<CrmStats | null>(null)
  const [loading, setLoading] = useState(false)

  // catch 文案用全局 i18n.t：取失败当下语言、不产生订阅依赖（见 MainLayout 先例），
  // 语言切换不触发统计重拉。
  useEffect(() => {
    setLoading(true)
    getCrmStats(days)
      .then((res) => setStats(res.stats))
      .catch(() => message.error(i18n.t('获取统计失败')))
      .finally(() => setLoading(false))
  }, [days])

  const funnel = (stats?.funnel ?? []).filter((f) => FUNNEL_STAGES.includes(f.stage))
  const funnelMax = Math.max(1, ...funnel.map((f) => f.count))
  const paidPct = stats && stats.contract_cents > 0 ? Math.round((stats.paid_cents / stats.contract_cents) * 100) : 0

  return (
    <div className="page-list crm-dashboard-page">
      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="销售看板"
          icon={<FundOutlined />}
          gradient="linear-gradient(135deg, #34d399, #059669)"
          glow="rgba(5, 150, 105, 0.4)"
          description="线索转化、商机漏斗与回款进度一览"
          extra={<Segmented options={RANGE_OPTIONS.map((o) => ({ ...o, label: t(o.label) }))} value={days} onChange={(v) => setDays(Number(v))} />}
        />

        <Row gutter={[16, 16]} style={{ marginTop: 8 }}>
          <Col xs={12} md={6}>
            <Card bordered={false} loading={loading}>
              <Statistic title={t('客户总数')} value={stats?.customer_count ?? 0} prefix={<TeamOutlined />} />
            </Card>
          </Col>
          <Col xs={12} md={6}>
            <Card bordered={false} loading={loading}>
              <Statistic title={t('新增线索（{{n}}天）', { n: days })} value={stats?.new_leads ?? 0} prefix={<UserAddOutlined />}
                suffix={stats?.converted_leads ? <Text type="secondary" style={{ fontSize: 13 }}>{t('转化 {{n}}', { n: stats.converted_leads })}</Text> : undefined} />
            </Card>
          </Col>
          <Col xs={12} md={6}>
            <Card bordered={false} loading={loading}>
              <Statistic title={t('进行中商机')} value={stats?.open_opportunity ?? 0} prefix={<SwapOutlined />} />
            </Card>
          </Col>
          <Col xs={12} md={6}>
            <Card bordered={false} loading={loading}>
              <Statistic title={t('逾期跟进')} value={stats?.followups_overdue ?? 0} prefix={<AlertOutlined />}
                valueStyle={stats?.followups_overdue ? { color: 'var(--c-error-strong)' } : undefined} />
            </Card>
          </Col>
        </Row>

        <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
          <Col xs={24} md={10}>
            <Card bordered={false} title={t('商机漏斗（当前快照）')} loading={loading}>
              {funnel.every((f) => f.count === 0) ? (
                <GlassEmpty text="暂无进行中商机" compact />
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 4, padding: '4px 0' }}>
                  {funnel.map((f, idx) => {
                    const meta = STAGE_META[f.stage] ?? { label: f.stage, color: 'default' }
                    // 真梯形漏斗：宽度按数量比例递减（保底 32% 保证文字可读），
                    // clip-path 切出上宽下窄的梯形
                    const widthPct = 32 + Math.round((f.count / funnelMax) * 68)
                    const nextCount = funnel[idx + 1]?.count ?? 0
                    const passRate = f.count > 0 ? Math.round((nextCount / f.count) * 100) : 0
                    return (
                      <div key={f.stage} style={{ width: '100%', display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
                        <div style={{
                          width: `${widthPct}%`, minWidth: 150,
                          background: FUNNEL_GRADIENTS[f.stage] || FUNNEL_GRADIENTS.initial,
                          clipPath: 'polygon(0 0, 100% 0, 94% 100%, 6% 100%)',
                          borderRadius: 4,
                          padding: '9px 0', textAlign: 'center', color: 'var(--text-on-accent)',
                          textShadow: '0 1px 2px rgba(0,0,0,0.35)',
                          transition: 'width 0.4s ease',
                        }}>
                          <span style={{ fontWeight: 600, fontSize: 13 }}>{t(meta.label)}</span>
                          <span style={{ fontSize: 12, marginLeft: 8, opacity: 0.95 }}>{t('{{n}} 个 · ¥{{amount}}', { n: f.count, amount: fenToYuan(f.amount_cents) })}</span>
                        </div>
                        {idx < funnel.length - 1 ? (
                          <Text type="secondary" style={{ fontSize: 11, lineHeight: '16px' }}>
                            ↓ {f.count > 0 ? `${passRate}%` : '—'}
                          </Text>
                        ) : null}
                      </div>
                    )
                  })}
                </div>
              )}
            </Card>
          </Col>
          <Col xs={24} md={14}>
            <Row gutter={[16, 16]}>
              <Col xs={24} sm={8}>
                <Card bordered={false} loading={loading}>
                  <Statistic title={t('赢单（{{n}}天）', { n: days })} value={stats?.won_count ?? 0} prefix={<RiseOutlined />}
                    valueStyle={{ color: 'var(--c-success)' }} />
                  <Text type="secondary">¥{fenToYuan(stats?.won_amount_cents)}</Text>
                </Card>
              </Col>
              <Col xs={24} sm={8}>
                <Card bordered={false} loading={loading}>
                  <Statistic title={t('签约额（{{n}}天）', { n: days })} value={fenToYuan(stats?.contract_cents)} prefix={<DollarOutlined />} />
                </Card>
              </Col>
              <Col xs={24} sm={8}>
                <Card bordered={false} loading={loading}>
                  <Statistic title={t('回款额（{{n}}天）', { n: days })} value={fenToYuan(stats?.paid_cents)} prefix={<PayCircleOutlined />}
                    valueStyle={{ color: 'var(--c-primary)' }} />
                </Card>
              </Col>
              <Col span={24}>
                <Card bordered={false} title={t('回款进度（区间内签约额口径）')} loading={loading}>
                  <Progress percent={paidPct} status={paidPct >= 100 ? 'success' : 'active'} />
                  <Text type="secondary">
                    {t('已回 ¥{{paid}} / 签约 ¥{{contract}}', { paid: fenToYuan(stats?.paid_cents), contract: fenToYuan(stats?.contract_cents) })}
                  </Text>
                </Card>
              </Col>
            </Row>
          </Col>
        </Row>

        <Row style={{ marginTop: 16 }}>
          <Col span={24}>
            <Card bordered={false} loading={loading}
              title={<Space size={8}><TrophyOutlined style={{ color: 'var(--c-warning)' }} />{t('销售业绩排行（近 {{n}} 天，按赢单金额）', { n: days })}</Space>}>
              <Table<CrmOwnerStat>
                rowKey="owner_user_id"
                size="small"
                dataSource={stats?.owners ?? []}
                pagination={false}
                locale={{ emptyText: <GlassEmpty text="区间内暂无业绩数据" compact /> }}
                columns={[
                  {
                    title: t('排名'), width: 70,
                    render: (_, __, idx) =>
                      idx < 3 ? <Tag color={['gold', 'default', 'orange'][idx]} variant="filled">{['🥇', '🥈', '🥉'][idx]}</Tag> : <Text type="secondary">{idx + 1}</Text>,
                  },
                  {
                    title: t('负责人'), dataIndex: 'display_name', width: 140,
                    render: (v: string, row) => v || t('用户 #{{id}}', { id: row.owner_user_id }),
                  },
                  {
                    title: t('赢单'), dataIndex: 'won_count', width: 130, align: 'right',
                    render: (v: number, row) => (
                      <Space size={6} wrap={false}>
                        <Text strong className="crm-owner-win-amount" style={{ color: 'var(--c-success)' }}>¥{fenToYuan(row.won_amount_cents)}</Text>
                        <Text type="secondary" className="crm-owner-win-count">{t('{{n}} 单', { n: v })}</Text>
                      </Space>
                    ),
                  },
                  { title: t('进行中'), dataIndex: 'open_count', width: 90, align: 'right', responsive: ['md'] },
                  {
                    title: t('签约额'), dataIndex: 'contract_cents', width: 130, align: 'right', responsive: ['md'],
                    render: (v: number) => `¥${fenToYuan(v)}`,
                  },
                  {
                    title: t('回款额'), dataIndex: 'paid_cents', width: 130, align: 'right', responsive: ['sm'],
                    render: (v: number) => <Text style={{ color: 'var(--c-primary)' }}>¥{fenToYuan(v)}</Text>,
                  },
                ] satisfies ColumnsType<CrmOwnerStat>}
              />
            </Card>
          </Col>
        </Row>

        {stats?.trend && stats.trend.length > 1 && (
          <Card bordered={false} title={t('月度趋势')} style={{ marginTop: 16 }} loading={loading}>
            <div style={{ display: 'flex', alignItems: 'flex-end', gap: 8, height: 120, padding: '8px 0' }}>
              {stats.trend.map((pt) => {
                const maxH = 100
                const maxVal = Math.max(1, ...stats.trend!.map((t) => Math.max(t.leads, t.won)))
                return (
                  <div key={pt.month} style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 2 }}>
                    <Text type="secondary" style={{ fontSize: 10 }}>{pt.won}</Text>
                    <div style={{ display: 'flex', gap: 2, alignItems: 'flex-end', height: maxH }}>
                      <div title={`${t('线索')} ${pt.leads}`} style={{ width: 12, height: Math.round((pt.leads / maxVal) * maxH) || 2, background: 'linear-gradient(180deg, #1677ff, #69b1ff)', borderRadius: '3px 3px 0 0', opacity: 0.7 }} />
                      <div title={`${t('赢单')} ¥${fenToYuan(pt.revenue)}`} style={{ width: 12, height: Math.round((pt.won / maxVal) * maxH) || 2, background: 'linear-gradient(180deg, #34d399, #059669)', borderRadius: '3px 3px 0 0' }} />
                    </div>
                    <Text type="secondary" style={{ fontSize: 10, transform: 'rotate(-45deg)', marginTop: 2, whiteSpace: 'nowrap' }}>{pt.month.slice(5)}</Text>
                  </div>
                )
              })}
            </div>
            <Space style={{ marginTop: 8 }}>
              <span style={{ display: 'inline-block', width: 10, height: 10, borderRadius: 2, background: '#1677ff', opacity: 0.7 }} />
              <Text type="secondary" style={{ fontSize: 12 }}>{t('线索')}</Text>
              <span style={{ display: 'inline-block', width: 10, height: 10, borderRadius: 2, background: '#34d399' }} />
              <Text type="secondary" style={{ fontSize: 12 }}>{t('赢单')}</Text>
            </Space>
          </Card>
        )}
      </Card>
    </div>
  )
}
