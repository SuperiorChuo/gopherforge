import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, Button, Col, Row, Select, Space, Statistic, Table, Tag, Tooltip, Typography } from 'antd'
import { PlayCircleOutlined, ReloadOutlined, SwapOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import TableToolbar from '@/components/common/TableToolbar'
import GlassEmpty from '@/components/common/GlassEmpty'
import { formatDateTime } from '@/utils/format'
import { createGeoRun, getGeoRun, listGeoRuns, type VisGeoResult, type VisGeoRun } from '@/api/visibility'

const { Text, Paragraph } = Typography

// engineName 归一：老 run engine 为空且 mode=llm → selfhost；stub 保持演示。
const engineName = (r: VisGeoRun) => {
  const e = r.engine || (r.mode === 'stub' ? 'stub' : 'selfhost')
  if (e === 'chatgpt') return 'chatgpt'
  if (e === 'stub') return 'stub'
  return 'selfhost'
}
const engineLabel = (e: string) => ({ chatgpt: 'ChatGPT 联网', selfhost: '自建检索', stub: '模拟演示' }[e] ?? e)

type RunBundle = { run: VisGeoRun; results: VisGeoResult[] }

type Row = { key: string; question: string; a?: VisGeoResult; b?: VisGeoResult }

export default function VisibilityGeoComparePage() {
  const { t } = useTranslation()
  const [allRuns, setAllRuns] = useState<VisGeoRun[]>([])
  const [runIdA, setRunIdA] = useState<number | null>(null)
  const [runIdB, setRunIdB] = useState<number | null>(null)
  const [bundleA, setBundleA] = useState<RunBundle | null>(null)
  const [bundleB, setBundleB] = useState<RunBundle | null>(null)
  const [loading, setLoading] = useState(false)
  const [kicking, setKicking] = useState(false)
  const [kickedIds, setKickedIds] = useState<number[]>([])

  async function load(keepSelection = true) {
    setLoading(true)
    try {
      const res = await listGeoRuns({ page: 1, page_size: 50 })
      const runs = res.list ?? []
      setAllRuns(runs)
      const done = runs.filter((r) => r.status === 'done')
      const latest = (e: string) => done.filter((r) => engineName(r) === e).sort((a, b) => b.id - a.id)[0]
      // 默认各引擎最近一次完成；用户手动选过则不覆盖
      setRunIdA((prev) => (keepSelection && prev != null ? prev : latest('chatgpt')?.id ?? prev))
      setRunIdB((prev) => (keepSelection && prev != null ? prev : latest('selfhost')?.id ?? prev))
    } catch {
      message.error(t('加载数据失败'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load(false)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function fetchBundle(id: number | null, set: (b: RunBundle | null) => void) {
    if (id == null) {
      set(null)
      return
    }
    try {
      const res = await getGeoRun(id)
      set({ run: res.run, results: res.results ?? [] })
    } catch {
      set(null)
    }
  }
  useEffect(() => {
    void fetchBundle(runIdA, setBundleA)
  }, [runIdA])
  useEffect(() => {
    void fetchBundle(runIdB, setBundleB)
  }, [runIdB])

  // 跑一轮对比：并发发起 chatgpt + selfhost，轮询等两 run 完成
  async function onKick() {
    setKicking(true)
    try {
      const [c, s] = await Promise.all([
        createGeoRun({ mode: 'llm', engine: 'chatgpt', note: '跨引擎对比' }),
        createGeoRun({ mode: 'llm', engine: 'selfhost', note: '跨引擎对比' }),
      ])
      const ids = [c.run.id, s.run.id]
      setKickedIds(ids)
      setRunIdA(c.run.id)
      setRunIdB(s.run.id)
      message.info(t('已发起对比：ChatGPT #{{a}} / 自建 #{{b}}。ChatGPT 联网搜索每题约 1-3 分钟，完成自动刷新', { a: c.run.id, b: s.run.id }))
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('发起对比失败'))
      setKicking(false)
    }
  }

  useEffect(() => {
    if (!kickedIds.length) return
    const timer = setInterval(async () => {
      try {
        const res = await listGeoRuns({ page: 1, page_size: 50 })
        const done = kickedIds.every((id) => res.list?.some((r) => r.id === id && r.status === 'done'))
        if (done) {
          clearInterval(timer)
          setKicking(false)
          setKickedIds([])
          message.success(t('两轮对比均已完成，已刷新'))
          await load(false)
        }
      } catch {
        /* 单次轮询失败忽略，下轮再试 */
      }
    }, 15000)
    return () => clearInterval(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [kickedIds])

  const summarize = (b: RunBundle | null) => {
    if (!b) return null
    const r = b.run
    const total = r.prompt_count || b.results.length
    const mention = r.mention_count ?? b.results.filter((x) => x.mention_brand).length
    const cited = b.results.filter((x) => x.has_link).length
    const sources = b.results.reduce((n, x) => n + (x.sources?.length ?? 0), 0)
    const brands = Array.from(new Set(b.results.flatMap((x) => (x.matched_brands || '').split(',').map((s) => s.trim()).filter(Boolean))))
    return {
      id: r.id,
      total,
      mention,
      rate: total > 0 ? Math.round((mention / total) * 100) : 0,
      cited,
      sources,
      brands,
      running: r.status === 'running',
      done: r.done_count ?? 0,
      prompt: r.prompt_count ?? 0,
    }
  }
  const sumA = summarize(bundleA)
  const sumB = summarize(bundleB)

  const rows: Row[] = useMemo(() => {
    const qs = Array.from(new Set([
      ...(bundleA?.results ?? []).map((r) => r.question),
      ...(bundleB?.results ?? []).map((r) => r.question),
    ]))
    return qs.map((q) => ({
      key: q,
      question: q,
      a: bundleA?.results.find((r) => r.question === q),
      b: bundleB?.results.find((r) => r.question === q),
    }))
  }, [bundleA, bundleB])

  const cell = (res?: VisGeoResult) => {
    if (!res) return <Text type="secondary">—</Text>
    const cites = res.sources?.filter((s) => s.cited).length ?? (res.has_link ? 1 : 0)
    return (
      <Space size={6} wrap>
        {res.mention_brand ? <Tag color="green" variant="filled">{t('提及')}</Tag> : <Tag variant="filled">{t('未提及')}</Tag>}
        <Text type="secondary" style={{ fontSize: 12 }}>{t('{{n}} 引用', { n: cites })}</Text>
      </Space>
    )
  }

  const cols: ColumnsType<Row> = [
    { title: t('问题'), dataIndex: 'question', ellipsis: true },
    { title: bundleA ? t(engineLabel(engineName(bundleA.run))) : t('引擎 A'), dataIndex: 'a', width: 160, render: (_, row) => cell(row.a) },
    { title: bundleB ? t(engineLabel(engineName(bundleB.run))) : t('引擎 B'), dataIndex: 'b', width: 160, render: (_, row) => cell(row.b) },
  ]

  const runOptions = useMemo(() => allRuns.map((r) => {
    const rate = r.prompt_count > 0 ? Math.round(((r.mention_count ?? 0) / r.prompt_count) * 100) : 0
    const status = r.status === 'running' ? `· ${t('进行中')}` : ''
    return {
      value: r.id,
      label: `#${r.id} ${t(engineLabel(engineName(r)))} · ${formatDateTime(r.created_at)} · ${rate}%${status}`,
    }
  }), [allRuns, t])

  const summaryCard = (title: string, s: ReturnType<typeof summarize>, b: RunBundle | null) => (
    <Col span={12}>
      <div style={{ border: '1px solid rgba(148,163,184,.2)', borderRadius: 10, padding: 16, height: '100%' }}>
        <Space style={{ width: '100%', justifyContent: 'space-between', marginBottom: 8 }}>
          <Text strong>{t(title)}</Text>
          {s?.running ? <Tag color="processing">{t('进行中 {{a}}/{{b}}', { a: s.done, b: s.prompt })}</Tag> : null}
        </Space>
        {s ? (
          <>
            <Row gutter={16}>
              <Col span={8}><Statistic title={t('提及率')} value={s.rate} suffix="%" /></Col>
              <Col span={8}><Statistic title={t('引用题数')} value={s.cited} suffix={`/${s.total}`} /></Col>
              <Col span={8}><Statistic title={t('引用总数')} value={s.sources} /></Col>
            </Row>
            {s.brands.length > 0 ? (
              <div style={{ marginTop: 8 }}>
                <Text type="secondary" style={{ fontSize: 12 }}>{t('回答中提到：')}</Text>
                {s.brands.map((x) => <Tag key={x} variant="filled">{x}</Tag>)}
              </div>
            ) : null}
          </>
        ) : (
          <Paragraph type="secondary" style={{ marginBottom: 0 }}>{t('未选择该引擎的检测记录')}</Paragraph>
        )}
        <div style={{ marginTop: 8 }}>
          <Text type="secondary" style={{ fontSize: 12 }}>run #{(b?.run.id ?? s?.id) ?? '—'}</Text>
        </div>
      </div>
    </Col>
  )

  const hasAnyRun = allRuns.some((r) => r.status === 'done')
  const hasBothEngines = allRuns.some((r) => engineName(r) === 'chatgpt' && r.status === 'done') &&
    allRuns.some((r) => engineName(r) === 'selfhost' && r.status === 'done')

  return (
    <div className="page-list">
      <div className="list-main-card" style={{ padding: 16 }}>
        <TableToolbar
          title="跨引擎对比"
          icon={<SwapOutlined />}
          gradient="linear-gradient(135deg, #f472b6, #8b5cf6)"
          glow="rgba(139, 92, 246, 0.4)"
          description="并排对比不同引擎（ChatGPT 联网搜索 / 自建检索）在同一批问题上的提及与引用情况"
          extra={
            <Space wrap>
              <Button icon={<ReloadOutlined />} onClick={() => void load()}>{t('刷新')}</Button>
              <Tooltip title={t('发起一轮对比：同时用 ChatGPT 联网搜索和自建检索各跑一遍当前检测问题。ChatGPT 较慢（约 30-40 分钟），且联网搜索按次计费')}>
                <Button type="primary" icon={<PlayCircleOutlined />} loading={kicking} onClick={() => void onKick()}>
                  {t('跑一轮对比')}
                </Button>
              </Tooltip>
            </Space>
          }
        />
        <Paragraph type="secondary">
          {t('对比的是一次检测在两个引擎下的结果：哪个引擎更常提到你的品牌、引用了哪些来源。没有某引擎的检测记录时，先去「AI 提及检测」用对应引擎跑一次，再回来看对比。')}
        </Paragraph>

        {!hasAnyRun ? (
          <GlassEmpty text="还没有任何检测记录，去「AI 提及检测」先跑一次" />
        ) : (
          <>
            {!hasBothEngines ? (
              <Alert
                type="info"
                showIcon
                style={{ marginBottom: 16 }}
                message={t('目前只有一种引擎的检测记录。先跑一轮对比（或去「AI 提及检测」用另一种引擎跑一次），才能横向比较。')}
              />
            ) : null}

            <Space size="large" style={{ marginBottom: 16, width: '100%' }}>
              <Space>
                <Text strong>{t('引擎 A')}</Text>
                <Select
                  showSearch
                  style={{ width: 320 }}
                  value={runIdA}
                  onChange={setRunIdA}
                  options={runOptions}
                  placeholder={t('选择一次检测')}
                />
              </Space>
              <Space>
                <Text strong>{t('引擎 B')}</Text>
                <Select
                  showSearch
                  style={{ width: 320 }}
                  value={runIdB}
                  onChange={setRunIdB}
                  options={runOptions}
                  placeholder={t('选择一次检测')}
                />
              </Space>
            </Space>

            <Row gutter={16} style={{ marginBottom: 24 }}>
              {summaryCard(bundleA ? engineLabel(engineName(bundleA.run)) : '引擎 A', sumA, bundleA)}
              {summaryCard(bundleB ? engineLabel(engineName(bundleB.run)) : '引擎 B', sumB, bundleB)}
            </Row>

            <Text strong>{t('逐题对比（{{n}} 题）', { n: rows.length })}</Text>
            <Table
              style={{ marginTop: 8 }}
              rowKey="key"
              size="small"
              loading={loading}
              columns={cols}
              dataSource={rows}
              pagination={false}
              locale={{ emptyText: <GlassEmpty text="选两个引擎的检测记录后看逐题对比" compact /> }}
             scroll={{ x: 960 }} />
          </>
        )}
      </div>
    </div>
  )
}
