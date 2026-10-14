import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Card, Col, Progress, Row, Segmented, Space, Statistic, Steps, Tag, Typography } from 'antd'
import {
  AimOutlined, FundOutlined, RiseOutlined, SearchOutlined, ThunderboltOutlined, TrophyOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { message } from '@/utils/feedback'
import TableToolbar from '@/components/common/TableToolbar'
import {
  getGeoLeaderboard, getVisibilityOverview, seedVisibilityDemo,
  type GeoLeaderboard, type VisOverview,
} from '@/api/visibility'

const { Text } = Typography

export default function VisibilityOverviewPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [days, setDays] = useState(30)
  const [stats, setStats] = useState<VisOverview | null>(null)
  const [loading, setLoading] = useState(false)
  const [board, setBoard] = useState<GeoLeaderboard | null>(null)
  // 演示数据入口是否可用，由后端 demo_mode 决定（生产下 seed-demo 路由不存在）
  const [demoMode, setDemoMode] = useState(false)

  useEffect(() => {
    // 快速切换 7/30/90 天时旧响应不得覆盖新数据
    let cancelled = false
    setLoading(true)
    Promise.all([getVisibilityOverview(days), getGeoLeaderboard()])
      .then(([res, lb]) => {
        if (cancelled) return
        setStats(res.stats)
        setBoard(lb)
        setDemoMode(Boolean(res.demo_mode))
      })
      .catch(() => {
        if (!cancelled) message.error(t('加载可见度概览失败'))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [days, t])

  async function load() {
    try {
      const [res, lb] = await Promise.all([getVisibilityOverview(days), getGeoLeaderboard()])
      setStats(res.stats)
      setBoard(lb)
      setDemoMode(Boolean(res.demo_mode))
    } catch {
      message.error(t('加载可见度概览失败'))
    }
  }

  async function onSeed() {
    try {
      await seedVisibilityDemo()
      message.success(t('已写入演示数据'))
      await load()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('初始化失败'))
    }
  }

  const rate = Math.round((stats?.geo_mention_rate ?? 0) * 100)
  // 新手引导：数据齐全后隐藏
  const setupDone = (stats?.brand_count ?? 0) > 0 && (stats?.keyword_count ?? 0) > 0 && (stats?.geo_prompt_count ?? 0) > 0
  const currentStep = !stats || stats.brand_count === 0 ? 0
    : stats.keyword_count === 0 ? 1
    : stats.geo_prompt_count === 0 ? 2
    : 3

  return (
    <div className="page-list">
      <Card className="list-main-card" bordered={false}>
        <TableToolbar
          title="搜索与 AI 可见度"
          icon={<AimOutlined />}
          gradient="linear-gradient(135deg, #38bdf8, #6366f1)"
          glow="rgba(99, 102, 241, 0.4)"
          description="一眼看到：用户搜索时能不能找到你，问 AI 时会不会提到你"
          extra={
            <Space wrap>
              <Segmented
                options={[
                  { label: t('近 7 天'), value: 7 },
                  { label: t('近 30 天'), value: 30 },
                  { label: t('近 90 天'), value: 90 },
                ]}
                value={days}
                onChange={(v) => setDays(Number(v))}
              />
              {/* 仅演示环境可见（demo_mode 由后端给）；已有真实数据后也隐藏，
                  防误点混入演示数据。生产下路由不存在，按钮也不渲染。 */}
              {demoMode && !loading && !setupDone ? <Button onClick={() => void onSeed()}>{t('初始化演示数据')}</Button> : null}
            </Space>
          }
        />

        {!loading && !setupDone ? (
          <Card bordered={false} style={{ marginTop: 12 }} title={t('四步开始使用')}>
            <Steps
              size="small"
              current={currentStep}
              items={[
                { title: t('加品牌'), description: t('填你的品牌名和竞品名') },
                { title: t('加关键词'), description: t('用户会搜什么词找你') },
                { title: t('加检测问题'), description: t('用户会怎么问 AI') },
                { title: t('开始检测'), description: t('查排名 + AI 提及检测') },
              ]}
            />
            <Space style={{ marginTop: 16 }} wrap>
              <Button type="primary" onClick={() => navigate(['/visibility/brands', '/visibility/seo', '/visibility/geo', '/visibility/geo'][currentStep])}>
                {t('去完成第 {{n}} 步', { n: currentStep + 1 })}
              </Button>
              {demoMode ? <Text type="secondary">{t('或点右上角「初始化演示数据」先看看效果')}</Text> : null}
            </Space>
          </Card>
        ) : null}

        <Row gutter={[16, 16]} style={{ marginTop: 12 }}>
          <Col xs={12} md={6}>
            <Card bordered={false} loading={loading}>
              <Statistic title={t('自有品牌')} value={stats?.brand_count ?? 0} prefix={<FundOutlined />} />
            </Card>
          </Col>
          <Col xs={12} md={6}>
            <Card bordered={false} loading={loading}>
              <Statistic title={t('竞品')} value={stats?.competitor_count ?? 0} />
            </Card>
          </Col>
          <Col xs={12} md={6}>
            <Card bordered={false} loading={loading}>
              <Statistic title={t('追踪关键词')} value={stats?.keyword_count ?? 0} prefix={<SearchOutlined />}
                suffix={<Text type="secondary" style={{ fontSize: 12 }}>Top10 {stats?.keyword_top10 ?? 0}</Text>}
              />
            </Card>
          </Col>
          <Col xs={12} md={6}>
            <Card bordered={false} loading={loading}>
              <Statistic title={t('AI 检测问题')} value={stats?.geo_prompt_count ?? 0} prefix={<ThunderboltOutlined />} />
            </Card>
          </Col>
        </Row>

        <Card
          bordered={false}
          loading={loading}
          style={{ marginTop: 16 }}
          title={<Space><TrophyOutlined /> {t('AI 推荐排行榜')}</Space>}
          extra={board?.mode === 'stub' && board.runs_used > 0 ? <Tag variant="filled">{t('模拟演示数据')}</Tag> : null}
        >
          {!board || board.runs_used === 0 || !Array.isArray(board.items) ? (
            <Space direction="vertical">
              <Text type="secondary">{t('还没有完成的检测。做一次「真实 AI 检测」，就能看到 AI 推荐同类产品时把谁排在前面、你排第几。')}</Text>
              <Button type="primary" onClick={() => navigate('/visibility/geo')}>{t('去做 AI 提及检测')}</Button>
            </Space>
          ) : (
            <Row gutter={[24, 16]}>
              <Col xs={24} md={9}>
                <Space direction="vertical" size={4}>
                  <Statistic
                    title={t('你的品牌「{{name}}」当前排名', { name: board.own_name || t('未录入') })}
                    value={board.own_rank > 0 ? board.own_rank : t('未上榜')}
                    prefix={board.own_rank > 0 ? t('第') : undefined}
                    suffix={board.own_rank > 0 ? t('名 / 共 {{n}} 个上榜品牌', { n: (board.items ?? []).length }) : undefined}
                  />
                  <Text type="secondary">
                    {t('近 {{a}} 次检测 {{b}} 题 · 提及率 {{c}}%{{d}}', {
                      a: board.runs_used,
                      b: board.total_questions,
                      c: board.mention_rate,
                      d: board.avg_position > 0
                        ? t(' · 推荐列表平均第 {{x}} 位（最好第 {{y}}）', { x: board.avg_position, y: board.best_position })
                        : '',
                    })}
                  </Text>
                  <Button size="small" onClick={() => navigate('/visibility/brands')}>{t('命名分析：撞名检测与改名建议')}</Button>
                </Space>
              </Col>
              <Col xs={24} md={15}>
                <Space direction="vertical" size={6} style={{ width: '100%' }}>
                  {(board.items ?? []).slice(0, 8).map((it, i) => {
                    const max = (board.items ?? [])[0]?.mentions || 1
                    const own = it.kind === 'brand'
                    return (
                      <Space key={it.name} style={{ width: '100%', display: 'flex' }}>
                        <Text type="secondary" style={{ flex: '0 0 28px' }}>{i + 1}.</Text>
                        {/* 品牌名自适应 + 长名省略；进度条弹性占剩余空间（390 窄屏不挤出视口） */}
                        <Text strong={own} style={{ flex: '0 1 auto', minWidth: 0, maxWidth: '40%', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          {it.name} {own ? <Tag color="blue" variant="filled">{t('你')}</Tag> : null}
                        </Text>
                        <Progress
                          percent={Math.round((it.mentions / max) * 100)}
                          showInfo={false}
                          size="small"
                          style={{ flex: 1, minWidth: 60, maxWidth: 160 }}
                          strokeColor={own ? 'var(--c-primary)' : 'rgba(148,163,184,.55)'}
                        />
                        <Text type="secondary" style={{ flex: '0 0 auto' }}>{t('{{n}} 次', { n: it.mentions })}</Text>
                      </Space>
                    )
                  })}
                  {board.own_rank === 0 ? (
                    <Text type="secondary">{t('你的品牌在最近检测中一次都没被提到——试试「命名分析」和检测详情里的「生成建议」。')}</Text>
                  ) : null}
                </Space>
              </Col>
            </Row>
          )}
        </Card>

        <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
          <Col xs={24} md={12}>
            <Card bordered={false} title={t('AI 提及率')} loading={loading}>
              <Space direction="vertical" style={{ width: '100%' }} size={12}>
                <Progress percent={rate} status={rate >= 40 ? 'success' : 'active'} />
                <Text type="secondary">
                  {t('近 {{a}} 天：AI 的 {{b}} 条回答中，{{c}} 条提到了你的品牌 · 检测 {{d}} 次', {
                    a: days,
                    b: stats?.geo_result_count ?? 0,
                    c: stats?.geo_mention_count ?? 0,
                    d: stats?.recent_runs ?? 0,
                  })}
                </Text>
                <Button type="primary" onClick={() => navigate('/visibility/geo')}>{t('去做 AI 提及检测')}</Button>
              </Space>
            </Card>
          </Col>
          <Col xs={24} md={12}>
            <Card bordered={false} title={t('搜索排名')} loading={loading}>
              <Space direction="vertical" style={{ width: '100%' }} size={12}>
                <Statistic title={t('进入前 10 名的关键词')} value={stats?.keyword_top10 ?? 0} prefix={<RiseOutlined />} />
                <Space size={16} wrap>
                  <Text type="secondary">{t('百度 Top10：{{n}}', { n: stats?.baidu_top10 ?? 0 })}</Text>
                  <Text type="secondary">{t('必应 Top10：{{n}}', { n: stats?.bing_top10 ?? 0 })}</Text>
                  <Text type="secondary">{t('Google Top10：{{n}}', { n: stats?.google_top10 ?? 0 })}</Text>
                </Space>
                <Text type="secondary">{t('还没查到排名：{{n}} 个', { n: stats?.keyword_no_rank ?? 0 })}</Text>
                <Space>
                  <Button onClick={() => navigate('/visibility/seo')}>{t('管理关键词')}</Button>
                  <Button onClick={() => navigate('/visibility/brands')}>{t('品牌与竞品')}</Button>
                </Space>
              </Space>
            </Card>
          </Col>
        </Row>
      </Card>
    </div>
  )
}
