import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, Drawer, Form, Input, InputNumber, Modal, Progress, Select, Space, Table, Tag, Tooltip, Typography , Grid } from 'antd'
import { BulbOutlined, HistoryOutlined, PlusOutlined, RadarChartOutlined, ReloadOutlined, SearchOutlined, EditOutlined, DeleteOutlined} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { message } from '@/utils/feedback'
import TableToolbar from '@/components/common/TableToolbar'
import TableRowActions from '@/components/common/TableRowActions'
import GlassEmpty from '@/components/common/GlassEmpty'
import { formatDateTime } from '@/utils/format'
import { usePollingTask } from '@/hooks/usePollingTask'
import {
  batchCreateKeywords, createKeyword, createSerpCheck, deleteKeyword, generateKeywords, getKeywordHistory,
  getSerpTask, listKeywords, listSerpTasks, updateKeyword,
  type KeywordCandidate, type VisKeyword, type VisRankSnapshot, type VisSerpTask,
} from '@/api/visibility'

const { Text, Paragraph } = Typography

const EXTRA_ENGINE_LABEL: Record<string, string> = {
  sogou: '搜狗',
  so360: '360',
  duckduckgo: 'DDG',
  github: 'GitHub',
}

const ENGINE_LABEL: Record<string, string> = {
  bing: '必应',
  baidu: '百度',
  google: 'Google',
  all: '全部',
  ...EXTRA_ENGINE_LABEL,
}

function parseExtraRanks(raw?: string): Array<{ engine: string; rank: number; prev_rank: number }> {
  if (!raw) return []
  try {
    const obj = JSON.parse(raw) as Record<string, { rank?: number; prev_rank?: number }>
    return Object.entries(obj).map(([engine, v]) => ({ engine, rank: v.rank ?? 0, prev_rank: v.prev_rank ?? 0 }))
  } catch {
    return []
  }
}

export default function VisibilitySeoPage() {
  const { t } = useTranslation()
  const screens = Grid.useBreakpoint()
  const compactActions = !screens.md
  const renderRank = (rank: number, prevRank: number) => {
    if (!rank) return <Tag variant="filled">{t('未收录')}</Tag>
    const delta = prevRank > 0 ? prevRank - rank : 0
    return (
      <Space size={4}>
        <strong>{rank}</strong>
        {delta > 0 ? <Tag color="green" variant="filled">↑{delta}</Tag> : null}
        {delta < 0 ? <Tag color="red" variant="filled">↓{-delta}</Tag> : null}
      </Space>
    )
  }
  const [list, setList] = useState<VisKeyword[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [kw, setKw] = useState('')
  const [page, setPage] = useState(1)
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<VisKeyword | null>(null)
  const [form] = Form.useForm()
  const [checking, setChecking] = useState(false)
  const [activeTaskId, setActiveTaskId] = useState<number | null>(null)
  // 排名历史与证据
  const [historyOpen, setHistoryOpen] = useState(false)
  const [historyKw, setHistoryKw] = useState<VisKeyword | null>(null)
  const [historyRows, setHistoryRows] = useState<VisRankSnapshot[]>([])
  const [historyLoading, setHistoryLoading] = useState(false)
  // 检查记录
  const [tasksOpen, setTasksOpen] = useState(false)
  const [taskRows, setTaskRows] = useState<VisSerpTask[]>([])
  const [tasksLoading, setTasksLoading] = useState(false)
  // 历史抽屉当前展示的关键词 id：慢的旧响应不得覆盖新行数据
  const historyIdRef = useRef<number | null>(null)
  // AI 生成关键词
  const [genOpen, setGenOpen] = useState(false)
  const [genForm] = Form.useForm()
  const [generating, setGenerating] = useState(false)
  const [savingGen, setSavingGen] = useState(false)
  const [candidates, setCandidates] = useState<KeywordCandidate[]>([])
  const [selectedGen, setSelectedGen] = useState<number[]>([])

  const { data: taskData, polling } = usePollingTask({
    taskId: activeTaskId,
    fetcher: getSerpTask,
    isDone: (d) => d.task.status !== 'running',
    onFinish: (d) => {
      setActiveTaskId(null)
      if (d.task.status === 'done') {
        message.success(t('排名检查完成：{{a}} 个关键词拿到了新排名{{b}}', {
          a: d.task.ok_count,
          b: d.task.fail_count > 0 ? t('，{{n}} 个失败', { n: d.task.fail_count }) : '',
        }))
      } else {
        message.error(d.task.error || t('排名检查失败'))
      }
      void load()
    },
    onError: () => {
      // 清掉 taskId 才能对同一任务重启轮询（后端 reused 返回相同 id）
      setActiveTaskId(null)
      message.warning(t('进度查询连续失败已暂停，任务仍在后台进行，稍后点「检查记录」查看结果'))
    },
  })

  // 显式传参覆盖 state，避免 onSearch 里 setState 后立即调用拿到旧值（stale closure）
  const load = useCallback(async (kwOverride?: string, pageOverride?: number) => {
    setLoading(true)
    try {
      const res = await listKeywords({
        keyword: (kwOverride ?? kw) || undefined,
        page: pageOverride ?? page,
        page_size: 20,
      })
      setList(res.list ?? [])
      setTotal(res.total ?? 0)
    } catch {
      message.error(t('加载关键词失败'))
    } finally {
      setLoading(false)
    }
  }, [kw, page, t])

  useEffect(() => {
    void load()
  }, [load])

  async function onCheck(keywordId?: number) {
    setChecking(true)
    try {
      const res = await createSerpCheck(keywordId ? { keyword_id: keywordId } : {})
      if (res.reused) {
        message.info(t('已有排名检查在进行，为你显示进度'))
      } else {
        message.success(t('已开始检查，在后台进行，做完会提醒你'))
      }
      setActiveTaskId(res.task.id)
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('启动检查失败'))
    } finally {
      setChecking(false)
    }
  }

  async function openHistory(row: VisKeyword) {
    setHistoryKw(row)
    setHistoryOpen(true)
    setHistoryLoading(true)
    setHistoryRows([])
    historyIdRef.current = row.id
    try {
      const res = await getKeywordHistory(row.id, { days: 30 })
      // 快速连点两行时，慢的旧响应不能覆盖当前行的数据
      if (historyIdRef.current !== row.id) return
      const rows = res.list ?? []
      rows.sort((a, b) => b.id - a.id)
      setHistoryRows(rows)
    } catch {
      if (historyIdRef.current === row.id) message.error(t('加载排名历史失败'))
    } finally {
      if (historyIdRef.current === row.id) setHistoryLoading(false)
    }
  }

  async function openTasks() {
    setTasksOpen(true)
    setTasksLoading(true)
    try {
      const res = await listSerpTasks({ page: 1, page_size: 20 })
      setTaskRows(res.list ?? [])
    } catch {
      message.error(t('加载检查记录失败'))
    } finally {
      setTasksLoading(false)
    }
  }

  function openCreate() {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ status: 'tracking', locale: 'zh-CN' })
    setOpen(true)
  }

  function openEdit(row: VisKeyword) {
    setEditing(row)
    // 先重置：omitempty 字段（target_url/note）为空时响应里没有该 key，
    // 只 setFieldsValue 会残留上一条记录的值并被保存进当前记录
    form.resetFields()
    form.setFieldsValue({
      keyword: row.keyword,
      locale: row.locale ?? 'zh-CN',
      target_url: row.target_url ?? '',
      search_volume: row.search_volume,
      status: row.status,
      note: row.note ?? '',
    })
    setOpen(true)
  }

  async function onSubmit() {
    const values = await form.validateFields().catch(() => null)
    if (!values) return
    try {
      if (editing) {
        await updateKeyword(editing.id, values)
        message.success(t('已更新'))
      } else {
        await createKeyword(values)
        message.success(t('已添加，点「查排名」即可获取当前排名'))
      }
      setOpen(false)
      await load()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('保存失败'))
    }
  }

  function openGen() {
    setCandidates([])
    setSelectedGen([])
    genForm.resetFields()
    genForm.setFieldsValue({ count: 10 })
    setGenOpen(true)
  }

  async function onGenerate() {
    const values = await genForm.validateFields().catch(() => null)
    if (!values) return
    setGenerating(true)
    try {
      const res = await generateKeywords({
        site_url: values.site_url?.trim() || undefined,
        hint: values.hint?.trim() || undefined,
        count: values.count,
      })
      const items = res.items ?? []
      setCandidates(items)
      setSelectedGen(items.map((_, i) => i))
      if (res.site_fetched === false) {
        message.warning(t('网页内容抓取失败（服务器访问不了该站点），已根据网址和 AI 已有认知生成，请核对后再入库'))
      }
      if (items.length === 0) message.info(t('AI 这次没给出关键词，补充说明后再试试'))
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('AI 生成失败'))
    } finally {
      setGenerating(false)
    }
  }

  function editCandidate(idx: number, patch: Partial<KeywordCandidate>) {
    setCandidates((prev) => prev.map((it, i) => (i === idx ? { ...it, ...patch } : it)))
  }

  async function onSaveGen() {
    const picks = selectedGen
      .map((i) => candidates[i])
      .filter((it): it is KeywordCandidate => Boolean(it && it.keyword.trim()))
    if (picks.length === 0) {
      message.warning(t('先勾选要加入的关键词'))
      return
    }
    const missingTarget = picks.some((p) => !p.target_url?.trim())
    setSavingGen(true)
    try {
      const res = await batchCreateKeywords(picks)
      message.success(res.skipped > 0
        ? t('已加入 {{a}} 个关键词，跳过 {{b}} 个重复', { a: res.created, b: res.skipped })
        : t('已加入 {{n}} 个关键词', { n: res.created }))
      if (missingTarget) message.info(t('部分关键词没填目标网页，记得补上才能查排名'))
      setGenOpen(false)
      await load()
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : t('加入失败'))
    } finally {
      setSavingGen(false)
    }
  }

  const columns: ColumnsType<VisKeyword> = [
    { title: t('关键词'), dataIndex: 'keyword', ellipsis: true },
    {
      title: t('最佳排名'),
      dataIndex: 'rank',
      width: 110,
      render: (v: number, r) => renderRank(v, r.prev_rank),
    },
    {
      title: t('百度'),
      dataIndex: 'baidu_rank',
      width: 100,
      render: (v: number, r) => renderRank(v, r.baidu_prev_rank),
    },
    {
      title: t('必应'),
      dataIndex: 'bing_rank',
      width: 100,
      render: (v: number, r) => renderRank(v, r.bing_prev_rank),
    },
    {
      title: (
        <Tooltip title={t('国内服务器可能访问不了 Google，检查失败时会保留上次的排名')}>
          <span>Google</span>
        </Tooltip>
      ),
      dataIndex: 'google_rank',
      width: 100,
      render: (v: number, r) => renderRank(v, r.google_prev_rank),
    },
    {
      title: (
        <Tooltip title={t('搜狗 / 360 / GitHub 仓库搜索等扩展平台的排名（GitHub 列是你的仓库在该词仓库搜索里的位次）')}>
          <span>{t('更多平台')}</span>
        </Tooltip>
      ),
      key: 'extra_ranks',
      width: 170,
      render: (_, r) => {
        const extras = parseExtraRanks(r.extra_ranks)
        if (extras.length === 0) return <Text type="secondary">—</Text>
        return (
          <Space size={4} wrap>
            {extras.map((e) => (
              <Tooltip
                key={e.engine}
                title={`${t(EXTRA_ENGINE_LABEL[e.engine] ?? '') || e.engine}：${e.rank > 0 ? t('第 {{n}} 位', { n: e.rank }) : t('未收录')}${e.prev_rank > 0 ? t('（上次第 {{n}}）', { n: e.prev_rank }) : ''}`}
              >
                <Tag variant="filled" color={e.rank > 0 && e.rank <= 10 ? 'green' : undefined}>
                  {t(EXTRA_ENGINE_LABEL[e.engine] ?? '') || e.engine} {e.rank > 0 ? e.rank : '—'}
                </Tag>
              </Tooltip>
            ))}
          </Space>
        )
      },
    },
    { title: t('目标网页'), dataIndex: 'target_url', ellipsis: true, render: (v) => v || '—' },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 90,
      render: (v) => ({ tracking: <Tag color="green" variant="filled">{t('追踪中')}</Tag>, paused: <Tag variant="filled">{t('暂停')}</Tag>, archived: <Tag variant="filled">{t('归档')}</Tag> }[v as string] ?? <Tag variant="filled">{v}</Tag>),
    },
    {
      title: t('操作'),
      width: compactActions ? 48 : 148,
      align: 'center' as const,
      fixed: 'right' as const,
      render: (_, r) => (
        <TableRowActions
          menuOnly={compactActions}
          maxInline={2}
          actions={[
            {
              key: 'check',
              label: t('查排名'),
              icon: <SearchOutlined />,
              disabled: polling || checking,
              onClick: () => { void onCheck(r.id) },
            },
            {
              key: 'history',
              label: t('历史'),
              icon: <HistoryOutlined />,
              onClick: () => { void openHistory(r) },
            },
            {
              key: 'edit',
              label: t('编辑'),
              icon: <EditOutlined />,
              onClick: () => openEdit(r),
            },
            {
              key: 'delete',
              label: t('删除'),
              icon: <DeleteOutlined />,
              danger: true,
              confirm: t('删除关键词？'),
              onClick: () => {
                void deleteKeyword(r.id).then(() => {
                  if (list.length === 1 && page > 1) setPage(page - 1)
                  else void load()
                })
              },
            },
          ]}
        />
      ),
    },
  ]

  const task = taskData?.task

  return (
    <div className="page-list">
      <div className="list-main-card" style={{ padding: 16 }}>
        <TableToolbar
          title="搜索排名"
          icon={<SearchOutlined />}
          gradient="linear-gradient(135deg, #34d399, #0ea5e9)"
          glow="rgba(14, 165, 233, 0.35)"
          description="自动检查关键词在百度 / 必应 / Google 的排名，每天凌晨自动更新，也可手动立即检查"
          extra={
            <Space wrap>
              <Input.Search
                allowClear
                placeholder={t('搜索关键词')}
                style={{ width: 180 }}
                onSearch={(v) => { setKw(v); setPage(1); void load(v, 1) }}
              />
              <Button icon={<ReloadOutlined />} onClick={() => void load()}>{t('刷新')}</Button>
              <Tooltip title={t('历次排名检查任务的状态与失败原因（如百度安全验证）')}>
                <Button icon={<HistoryOutlined />} onClick={() => void openTasks()}>{t('检查记录')}</Button>
              </Tooltip>
              <Tooltip title={t('填网址或用品牌库资料，让 AI 想出用户会搜的关键词')}>
                <Button icon={<BulbOutlined />} onClick={openGen}>{t('AI 生成')}</Button>
              </Tooltip>
              <Button icon={<PlusOutlined />} onClick={openCreate}>{t('加关键词')}</Button>
              <Tooltip title={t('立即检查所有「追踪中」关键词的最新排名，在后台进行')}>
                <Button type="primary" icon={<RadarChartOutlined />} loading={checking || polling} onClick={() => void onCheck()}>
                  {t('立即查排名')}
                </Button>
              </Tooltip>
            </Space>
          }
        />
        {polling && task ? (
          <div style={{ marginBottom: 12 }}>
            <Progress
              percent={task.total > 0 ? Math.round((task.done_count / task.total) * 100) : 0}
              status="active"
            />
            <Text type="secondary">{t('正在检查 {{a}}/{{b}} 个关键词（逐个查三个搜索引擎，会慢一些，请耐心等待）', { a: task.done_count, b: task.total })}</Text>
          </div>
        ) : null}
        <Table
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={list}
          locale={{ emptyText: <GlassEmpty text="还没有关键词，点「加关键词」开始" /> }}
          pagination={{ current: page, total, pageSize: 20, onChange: setPage }}
         scroll={{ x: 960 }} />
      </div>

      <Drawer
        title={historyKw ? t('「{{name}}」排名历史与证据', { name: historyKw.keyword }) : t('排名历史')}
        open={historyOpen}
        onClose={() => setHistoryOpen(false)}
        width={680}
      >
        <Paragraph type="secondary">
          {t('每一行是一次采集的结果：排名对应命中的真实网页可以点开核验；采集失败会显示原因（失败的平台保留上一次的排名不变）。')}
        </Paragraph>
        <Table
          rowKey="id"
          size="small"
          loading={historyLoading}
          pagination={false}
          scroll={{ y: 440 }}
          locale={{ emptyText: <GlassEmpty text="近 30 天还没有采集记录" compact /> }}
          columns={[
            { title: t('时间'), dataIndex: 'created_at', width: 150, render: (v) => formatDateTime(v) },
            {
              title: t('平台'),
              dataIndex: 'engine',
              width: 90,
              render: (v: string) => t(ENGINE_LABEL[v] ?? '') || v,
            },
            {
              title: t('结果'),
              width: 100,
              render: (_, s: VisRankSnapshot) => {
                if (s.error) return <Tooltip title={s.error}><Tag color="red" variant="filled">{t('采集失败')}</Tag></Tooltip>
                if (s.rank > 0) return <Tag color="green" variant="filled">{t('第 {{n}} 位', { n: s.rank })}</Tag>
                return <Tag variant="filled">{t('未收录')}</Tag>
              },
            },
            {
              title: t('命中网页（点开核验是不是你）'),
              render: (_, s: VisRankSnapshot) =>
                s.serp_url
                  ? <a href={s.serp_url.startsWith('http') ? s.serp_url : `https://${s.serp_url}`} target="_blank" rel="noreferrer">{s.serp_url}</a>
                  : <Text type="secondary">{s.error ? s.error.slice(0, 40) : '—'}</Text>,
              ellipsis: true,
            },
          ]}
          dataSource={historyRows}
        />
      </Drawer>

      <Drawer title={t('排名检查记录')} open={tasksOpen} onClose={() => setTasksOpen(false)} width={560}>
        <Table
          rowKey="id"
          size="small"
          loading={tasksLoading}
          pagination={false}
          locale={{ emptyText: <GlassEmpty text="还没有检查记录" compact /> }}
          columns={[
            { title: 'ID', dataIndex: 'id', width: 60 },
            {
              title: t('触发'),
              dataIndex: 'trigger',
              width: 80,
              render: (v) => (v === 'cron' ? <Tag variant="filled">{t('每日自动')}</Tag> : <Tag color="blue" variant="filled">{t('手动')}</Tag>),
            },
            {
              title: t('状态'),
              width: 150,
              render: (_, task: VisSerpTask) => {
                if (task.status === 'running') return <Tag color="processing" variant="filled">{t('进行中 {{a}}/{{b}}', { a: task.done_count, b: task.total })}</Tag>
                if (task.status === 'failed') return <Tooltip title={task.error}><Tag color="red" variant="filled">{t('失败')}</Tag></Tooltip>
                return (
                  <Space size={4}>
                    <Tag color="green" variant="filled">{t('完成')}</Tag>
                    <Text type="secondary" style={{ fontSize: 12 }}>{t('成功 {{n}}', { n: task.ok_count })}{task.fail_count > 0 ? t(' / 失败 {{n}}', { n: task.fail_count }) : ''}</Text>
                  </Space>
                )
              },
            },
            { title: t('时间'), dataIndex: 'created_at', render: (v) => formatDateTime(v) },
          ]}
          dataSource={taskRows}
         scroll={{ x: 960 }} />
      </Drawer>

      <Modal
        title={t('AI 生成关键词')}
        open={genOpen}
        onCancel={() => setGenOpen(false)}
        width={760}
        destroyOnHidden
        footer={[
          <Button key="cancel" onClick={() => setGenOpen(false)}>{t('取消')}</Button>,
          <Button key="gen" icon={<BulbOutlined />} loading={generating} onClick={() => void onGenerate()}>
            {candidates.length > 0 ? t('重新生成') : t('开始生成')}
          </Button>,
          <Button key="save" type="primary" loading={savingGen} disabled={selectedGen.length === 0} onClick={() => void onSaveGen()}>
            {t('加入选中（{{n}}）', { n: selectedGen.length })}
          </Button>,
        ]}
      >
        <Form form={genForm} layout="vertical" initialValues={{ count: 10 }}>
          <Form.Item
            name="site_url"
            label={t('网址（可选）')}
            rules={[{ pattern: /^https?:\/\/.+/i, message: t('以 http:// 或 https:// 开头') }]}
            help={t('填了就按这个网页的内容生成关键词，并自动填为目标网页；不填则按品牌库资料生成')}
          >
            <Input allowClear placeholder={t('例：https://github.com/你的账号/你的项目')} />
          </Form.Item>
          <Form.Item name="hint" label={t('行业/受众补充说明（可选）')} help={t('例如目标客户、使用场景、地域')}>
            <Input.TextArea rows={2} placeholder={t('例：面向国内中小企业技术团队的开源后台框架')} />
          </Form.Item>
          <Form.Item name="count" label={t('生成数量')}>
            <Select options={[5, 10, 15, 20].map((n) => ({ label: t('{{n}} 个', { n }), value: n }))} style={{ width: 140 }} />
          </Form.Item>
        </Form>
        {candidates.length > 0 ? (
          <>
            <Text type="secondary">{t('勾选要加入追踪的关键词，词和目标网页都可以直接修改；入库时自动跳过重复的：')}</Text>
            <Table
              style={{ marginTop: 8 }}
              rowKey={(_, i) => String(i)}
              size="small"
              pagination={false}
              scroll={{ y: 320 }}
              rowSelection={{
                selectedRowKeys: selectedGen.map(String),
                onChange: (keys) => setSelectedGen(keys.map((k) => Number(k))),
              }}
              columns={[
                {
                  title: t('关键词（可编辑）'),
                  dataIndex: 'keyword',
                  width: 220,
                  render: (v, _r, idx) => (
                    <Input size="small" value={v as string} onChange={(e) => editCandidate(idx, { keyword: e.target.value })} />
                  ),
                },
                {
                  title: t('地区'),
                  dataIndex: 'locale',
                  width: 110,
                  render: (v, _r, idx) => (
                    <Select
                      size="small"
                      value={v as string}
                      style={{ width: 96 }}
                      onChange={(nv) => editCandidate(idx, { locale: nv })}
                      options={[
                        { label: t('中文'), value: 'zh-CN' },
                        { label: 'EN', value: 'en-US' },
                      ]}
                    />
                  ),
                },
                {
                  title: t('目标网页（查排名用）'),
                  dataIndex: 'target_url',
                  render: (v, _r, idx) => (
                    <Input size="small" value={(v as string) || ''} placeholder="https://" onChange={(e) => editCandidate(idx, { target_url: e.target.value })} />
                  ),
                },
              ]}
              dataSource={candidates}
            />
          </>
        ) : (
          <Paragraph type="secondary" style={{ marginBottom: 0 }}>
            {t('填你的产品网址（或留空用品牌库资料），点「开始生成」，AI 会给出用户真实会搜的品类词、长尾词、对比词。生成后可编辑、勾选加入排名追踪。')}
          </Paragraph>
        )}
      </Modal>

      <Modal title={editing ? t('编辑关键词') : t('添加关键词')} open={open} onCancel={() => setOpen(false)} onOk={() => void onSubmit()} destroyOnHidden>
        <Form form={form} layout="vertical">
          <Form.Item
            name="keyword"
            label={t('关键词')}
            rules={[{ required: true, message: t('请输入关键词') }]}
            help={t('用户会在搜索引擎里输入什么词来找你？')}
          >
            <Input placeholder={t('例：开源后台管理系统')} />
          </Form.Item>
          <Form.Item
            name="target_url"
            label={t('目标网页')}
            help={t('填你希望用户搜到的页面地址。查排名时会在搜索结果里找这个域名，不填就查不了排名')}
          >
            <Input placeholder={t('https://你的网站.com/页面')} />
          </Form.Item>
          <Form.Item name="locale" label={t('地区 / 语言')}>
            <Select options={[
              { label: t('中文（中国）'), value: 'zh-CN' },
              { label: 'English (US)', value: 'en-US' },
            ]}
            />
          </Form.Item>
          <Form.Item name="search_volume" label={t('月搜索量（选填，估算值）')} help={t('大概每月有多少人搜这个词，不知道可以不填')}>
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="status" label={t('状态')} help={t('追踪中 = 每天自动查排名；暂停 = 先不查；归档 = 只留历史记录')}>
            <Select options={[
              { label: t('追踪中'), value: 'tracking' },
              { label: t('暂停'), value: 'paused' },
              { label: t('归档'), value: 'archived' },
            ]}
            />
          </Form.Item>
          <Form.Item name="note" label={t('备注（选填）')}>
            <Input.TextArea rows={2} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
